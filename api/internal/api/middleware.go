package api

import (
	"compress/gzip"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SecurityHeaders sets defensive headers for a JSON-only API: nothing it
// returns should ever be rendered, framed or sniffed.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Strict-Transport-Security", "max-age=31536000")
		next.ServeHTTP(w, r)
	})
}

// CORS allows only the listed browser origins to call the API. With none
// configured nothing is added, so browsers on other origins can't read
// responses. The API is GET-only, so preflights are answered and nothing
// else is allowed.
func CORS(allowedOrigins []string, next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range allowedOrigins {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if o := r.Header.Get("Origin"); allowed[o] {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", o)
			h.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimit is a per-IP token bucket. Idle buckets are evicted.
type RateLimit struct {
	mu         sync.Mutex
	buckets    map[string]*bucket
	rate       float64 // tokens per second
	burst      float64
	trustProxy bool
}

type bucket struct {
	tokens float64
	last   time.Time
}

func NewRateLimit(perSecond float64, burst int, trustProxy bool) *RateLimit {
	rl := &RateLimit{buckets: map[string]*bucket{}, rate: perSecond, burst: float64(burst), trustProxy: trustProxy}
	go func() {
		for range time.Tick(5 * time.Minute) {
			rl.mu.Lock()
			for k, b := range rl.buckets {
				if time.Since(b.last) > 10*time.Minute {
					delete(rl.buckets, k)
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

func (rl *RateLimit) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(rl.clientIP(r)) {
			w.Header().Set("Retry-After", "2")
			writeJSON(w, http.StatusTooManyRequests, 0, map[string]string{"error": "too many requests"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimit) allow(ip string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[ip] = b
	}
	b.tokens = minf(rl.burst, b.tokens+now.Sub(b.last).Seconds()*rl.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// clientIP uses the connection address; X-Forwarded-For is honoured only when
// the operator says a trusted reverse proxy sets it.
func (rl *RateLimit) clientIP(r *http.Request) string {
	if rl.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// the proxy appends the real client last
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Gzip compresses text responses for clients that accept it.
func Gzip(next http.Handler) http.Handler {
	pool := sync.Pool{New: func() any { return gzip.NewWriter(nil) }}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gz := pool.Get().(*gzip.Writer)
		gz.Reset(w)
		defer func() { gz.Close(); pool.Put(gz) }()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		w.Header().Del("Content-Length")
		next.ServeHTTP(&gzWriter{ResponseWriter: w, w: gz}, r)
	})
}

type gzWriter struct {
	http.ResponseWriter
	w *gzip.Writer
}

func (g *gzWriter) Write(b []byte) (int, error) { return g.w.Write(b) }

// Recover turns panics into 500s instead of dropping the connection.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				serverError(w)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
