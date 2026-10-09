// Package store provides read-only access to the places database.
package store

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type Place struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	City         string   `json:"city"`
	Type         string   `json:"type"`
	Area         string   `json:"area"`
	Address      string   `json:"address"`
	Timings      string   `json:"timings"`
	Cost         string   `json:"cost"`
	Lat          *float64 `json:"lat"`
	Lng          *float64 `json:"lng"`
	GeoPrecision string   `json:"geo_precision"`
	DistanceKm   *float64 `json:"distance_km,omitempty"`
}

type CityCount struct {
	City  string `json:"city"`
	Count int    `json:"count"`
}

// Query describes a places lookup. Zero values mean "no constraint".
type Query struct {
	Center   *[2]float64 // lat, lng; enables radius search sorted by distance
	RadiusKm float64
	City     string
	Types    []string
	Text     string
	Limit    int
}

type Store struct{ db *sql.DB }

// Open opens the database read-only and immutable: no writes are possible
// and SQLite skips locking entirely.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1&_pragma=query_only(1)")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Cities(ctx context.Context) ([]CityCount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT city, COUNT(*) FROM places GROUP BY city ORDER BY city`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CityCount
	for rows.Next() {
		var c CityCount
		if err := rows.Scan(&c.City, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) Places(ctx context.Context, q Query) ([]Place, error) {
	var (
		sb   strings.Builder
		args []any
	)
	sb.WriteString(`SELECT p.id,p.name,p.city,p.type,p.area,p.address,p.timings,p.cost,p.lat,p.lng,p.geo_precision FROM `)
	if q.Center != nil {
		// R*Tree narrows to a bounding box; exact distance is checked below.
		dLat := q.RadiusKm / 111.0
		dLng := q.RadiusKm / (111.0 * math.Max(math.Cos(q.Center[0]*math.Pi/180), 0.01))
		sb.WriteString(`places_rt r JOIN places p ON p.id = r.id
			WHERE r.min_lat >= ? AND r.max_lat <= ? AND r.min_lng >= ? AND r.max_lng <= ?`)
		args = append(args, q.Center[0]-dLat, q.Center[0]+dLat, q.Center[1]-dLng, q.Center[1]+dLng)
	} else {
		sb.WriteString(`places p WHERE 1=1`)
	}
	if q.City != "" {
		sb.WriteString(` AND p.city = ?`)
		args = append(args, q.City)
	}
	if len(q.Types) > 0 {
		sb.WriteString(` AND p.type IN (?` + strings.Repeat(`,?`, len(q.Types)-1) + `)`)
		for _, t := range q.Types {
			args = append(args, t)
		}
	}
	if q.Text != "" {
		like := "%" + escapeLike(q.Text) + "%"
		sb.WriteString(` AND (p.name LIKE ? ESCAPE '\' OR p.area LIKE ? ESCAPE '\' OR p.address LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Place
	for rows.Next() {
		var p Place
		var lat, lng sql.NullFloat64
		if err := rows.Scan(&p.ID, &p.Name, &p.City, &p.Type, &p.Area, &p.Address, &p.Timings, &p.Cost, &lat, &lng, &p.GeoPrecision); err != nil {
			return nil, err
		}
		if lat.Valid && lng.Valid {
			p.Lat, p.Lng = &lat.Float64, &lng.Float64
		}
		if q.Center != nil {
			d := haversineKm(q.Center[0], q.Center[1], *p.Lat, *p.Lng)
			if d > q.RadiusKm {
				continue
			}
			d = math.Round(d*100) / 100
			p.DistanceKm = &d
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if q.Center != nil {
		sort.Slice(out, func(i, j int) bool { return *out[i].DistanceKm < *out[j].DistanceKm })
	} else {
		sort.Slice(out, func(i, j int) bool {
			if out[i].City != out[j].City {
				return out[i].City < out[j].City
			}
			return out[i].Name < out[j].Name
		})
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0088
	rad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*rad, (lng2-lng1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}
