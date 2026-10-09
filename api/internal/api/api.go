// Package api implements the read-only JSON API.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"foodmap/internal/store"
)

const (
	defaultRadiusKm = 5
	maxRadiusKm     = 50
	defaultLimit    = 50
	maxLimit        = 100
	maxTextLen      = 64
)

var validTypes = map[string]bool{"Government": true, "NGO": true, "Religious": true}

type API struct {
	store  *store.Store
	cities map[string]bool
}

func New(ctx context.Context, s *store.Store) (*API, error) {
	cs, err := s.Cities(ctx)
	if err != nil {
		return nil, err
	}
	a := &API{store: s, cities: map[string]bool{}}
	for _, c := range cs {
		a.cities[c.City] = true
	}
	return a, nil
}

func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/places", a.places)
	mux.HandleFunc("GET /api/cities", a.citiesHandler)
}

func (a *API) citiesHandler(w http.ResponseWriter, r *http.Request) {
	cs, err := a.store.Cities(r.Context())
	if err != nil {
		serverError(w)
		return
	}
	writeJSON(w, http.StatusOK, 3600, map[string]any{"cities": cs})
}

// places: GET /api/places?lat&lng&radius&city&type&q&limit
// lat and lng must be given together. With them, results are within radius km
// sorted by distance; without, results are filtered by city/type/q only.
func (a *API) places(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	q := store.Query{Limit: defaultLimit}

	if v.Has("lat") || v.Has("lng") {
		lat, ok1 := parseFloat(v.Get("lat"), -90, 90)
		lng, ok2 := parseFloat(v.Get("lng"), -180, 180)
		if !ok1 || !ok2 {
			badRequest(w, "lat and lng must both be valid coordinates")
			return
		}
		// Round to ~100m so identical nearby requests are cache-friendly
		// and we never handle more precision than the search needs.
		q.Center = &[2]float64{round3(lat), round3(lng)}
		q.RadiusKm = defaultRadiusKm
		if v.Has("radius") {
			rad, ok := parseFloat(v.Get("radius"), 0.1, maxRadiusKm)
			if !ok {
				badRequest(w, "radius must be between 0.1 and 50 (km)")
				return
			}
			q.RadiusKm = rad
		}
	}
	if c := v.Get("city"); c != "" {
		if !a.cities[c] {
			badRequest(w, "unknown city")
			return
		}
		q.City = c
	}
	if t := v.Get("type"); t != "" {
		for _, part := range strings.Split(t, ",") {
			if !validTypes[part] {
				badRequest(w, "unknown type")
				return
			}
			q.Types = append(q.Types, part)
		}
	}
	if t := strings.TrimSpace(v.Get("q")); t != "" {
		if utf8.RuneCountInString(t) > maxTextLen {
			badRequest(w, "q too long")
			return
		}
		q.Text = t
	}
	if l := v.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > maxLimit {
			badRequest(w, "limit must be 1-100")
			return
		}
		q.Limit = n
	}

	ps, err := a.store.Places(r.Context(), q)
	if err != nil {
		serverError(w)
		return
	}
	if ps == nil {
		ps = []store.Place{}
	}
	writeJSON(w, http.StatusOK, 60, map[string]any{"places": ps})
}

func parseFloat(s string, lo, hi float64) (float64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f != f || f < lo || f > hi { // f != f rejects NaN
		return 0, false
	}
	return f, true
}

func round3(f float64) float64 { return float64(int64(f*1000+sign(f)*0.5)) / 1000 }

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

func writeJSON(w http.ResponseWriter, status, maxAge int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if maxAge > 0 {
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(maxAge))
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, 0, map[string]string{"error": msg})
}

func serverError(w http.ResponseWriter) {
	writeJSON(w, http.StatusInternalServerError, 0, map[string]string{"error": "internal error"})
}
