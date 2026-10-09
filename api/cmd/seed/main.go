// Command seed builds the read-only places.db from data/places.json.
//
// Addresses are geocoded once via Nominatim and cached in
// data/geocode_cache.json, so re-running is offline and deterministic.
// The server never geocodes our own data at request time.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type place struct {
	Name    string `json:"name"`
	City    string `json:"city"`
	Type    string `json:"type"`
	Area    string `json:"area"`
	Address string `json:"address"`
	Timings string `json:"timings"`
	Cost    string `json:"cost"`
}

// geo is a cached geocoding result. Precision is "address", "area" or "none".
type geo struct {
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	Precision string  `json:"precision"`
}

type bbox struct{ minLat, maxLat, minLng, maxLng float64 }

var cityBounds = map[string]bbox{
	"Delhi":     {28.40, 28.90, 76.80, 77.40},
	"Bangalore": {12.80, 13.20, 77.40, 77.80},
}

// genericAreas are too vague to geocode by name; a wrong pin is worse than none.
var genericAreas = map[string]bool{"JJ Cluster": true}

const schema = `
CREATE TABLE places (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL,
  city        TEXT NOT NULL,
  type        TEXT NOT NULL CHECK (type IN ('Government','NGO','Religious')),
  area        TEXT NOT NULL,
  address     TEXT NOT NULL,
  timings     TEXT NOT NULL,
  cost        TEXT NOT NULL,
  lat         REAL,
  lng         REAL,
  geo_precision TEXT NOT NULL DEFAULT 'none'
);
CREATE INDEX idx_places_city      ON places(city);
CREATE INDEX idx_places_type      ON places(type);
CREATE INDEX idx_places_city_type ON places(city, type);
CREATE VIRTUAL TABLE places_rt USING rtree(id, min_lat, max_lat, min_lng, max_lng);
`

func main() {
	in := flag.String("in", "data/places.json", "input places JSON")
	cachePath := flag.String("cache", "data/geocode_cache.json", "geocode cache")
	out := flag.String("out", "data/places.db", "output sqlite db")
	offline := flag.Bool("offline", false, "never call the geocoder; uncached rows get no coordinates")
	flag.Parse()

	var places []place
	mustReadJSON(*in, &places)

	cache := map[string]geo{}
	if b, err := os.ReadFile(*cachePath); err == nil {
		if err := json.Unmarshal(b, &cache); err != nil {
			log.Fatalf("cache: %v", err)
		}
	}

	last := time.Time{}
	lookup := func(q string, bb bbox) (geo, bool) {
		if g, ok := cache["q:"+q]; ok {
			return g, g.Precision != "none"
		}
		if *offline {
			return geo{}, false
		}
		if d := time.Since(last); d < 1100*time.Millisecond { // Nominatim: max 1 req/s
			time.Sleep(1100*time.Millisecond - d)
		}
		last = time.Now()
		lat, lng, ok := nominatim(q, bb)
		g := geo{Lat: lat, Lng: lng, Precision: "none"}
		if ok {
			g.Precision = "found"
		}
		cache["q:"+q] = g
		return g, ok
	}

	_ = os.Remove(*out)
	db, err := sql.Open("sqlite", *out)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		log.Fatal(err)
	}

	tx, _ := db.Begin()
	var nAddr, nArea, nNone int
	for i, p := range places {
		bb, known := cityBounds[p.City]
		if !known {
			log.Fatalf("unknown city %q", p.City)
		}
		var lat, lng sql.NullFloat64
		prec := "none"
		if !isMultiPoint(p) {
			if g, ok := lookup(p.Address+", "+p.City, bb); ok {
				lat, lng, prec = nf(g.Lat), nf(g.Lng), "address"
				nAddr++
			} else if g, ok := lookup(p.Area+", "+p.City, bb); ok && !genericAreas[p.Area] {
				lat, lng, prec = nf(g.Lat), nf(g.Lng), "area"
				nArea++
			}
		}
		if prec == "none" {
			nNone++
		}
		id := i + 1
		if _, err := tx.Exec(`INSERT INTO places(id,name,city,type,area,address,timings,cost,lat,lng,geo_precision)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, p.Name, p.City, p.Type, p.Area, p.Address, p.Timings, p.Cost, lat, lng, prec); err != nil {
			log.Fatal(err)
		}
		if lat.Valid {
			if _, err := tx.Exec(`INSERT INTO places_rt VALUES(?,?,?,?,?)`, id, lat.Float64, lat.Float64, lng.Float64, lng.Float64); err != nil {
				log.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(`ANALYZE; VACUUM;`); err != nil {
		log.Fatal(err)
	}

	b, _ := json.MarshalIndent(cache, "", " ")
	if err := os.WriteFile(*cachePath, b, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d places: %d address-level, %d area-level, %d without coordinates\n", len(places), nAddr, nArea, nNone)
}

// isMultiPoint reports entries that have no single location (citywide
// services, "multiple points"). They stay in the list but not on the map.
func isMultiPoint(p place) bool {
	a := strings.ToLower(p.Area + " " + p.Address)
	return strings.Contains(a, "citywide") || strings.Contains(a, "multiple") || strings.Contains(a, "200+")
}

func nf(f float64) sql.NullFloat64 { return sql.NullFloat64{Float64: f, Valid: true} }

func nominatim(q string, bb bbox) (lat, lng float64, ok bool) {
	u := "https://nominatim.openstreetmap.org/search?format=json&limit=1&countrycodes=in" +
		fmt.Sprintf("&viewbox=%f,%f,%f,%f&bounded=1", bb.minLng, bb.maxLat, bb.maxLng, bb.minLat) +
		"&q=" + url.QueryEscape(q)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "foodmap-seed/0.1 (ansari@pharmaflow.org)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("geocode %q: %v", q, err)
		return 0, 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatalf("geocode %q: HTTP %d (stop and retry later; cache is not written)", q, resp.StatusCode)
	}
	var r []struct{ Lat, Lon string }
	if json.NewDecoder(resp.Body).Decode(&r) != nil || len(r) == 0 {
		return 0, 0, false
	}
	lat, _ = strconv.ParseFloat(r[0].Lat, 64)
	lng, _ = strconv.ParseFloat(r[0].Lon, 64)
	if lat < bb.minLat || lat > bb.maxLat || lng < bb.minLng || lng > bb.maxLng {
		return 0, 0, false
	}
	return lat, lng, true
}

func mustReadJSON(path string, v any) {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		log.Fatalf("%s: %v", path, err)
	}
}
