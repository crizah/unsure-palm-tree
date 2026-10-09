package main

import (
	"context"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"foodmap/internal/api"
	"foodmap/internal/store"
	"foodmap/web"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "data/places.db", "path to read-only sqlite db")
	trustProxy := flag.Bool("trust-proxy", false, "trust X-Forwarded-For from a reverse proxy")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer st.Close()

	a, err := api.New(context.Background(), st)
	if err != nil {
		log.Fatalf("init api: %v", err)
	}

	site := fs.FS(web.FS)

	mux := http.NewServeMux()
	apiMux := http.NewServeMux()
	a.Register(apiMux)
	limited := api.NewRateLimit(10, 30, *trustProxy).Wrap(apiMux)
	mux.Handle("/api/", limited)

	mux.HandleFunc("GET /{$}", page(site, "home.html"))
	mux.HandleFunc("GET /locate", page(site, "locate.html"))
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticHandler(site)))

	var h http.Handler = mux
	h = api.Gzip(h)
	h = api.SecurityHeaders("https://tile.openstreetmap.org", h)
	h = api.Recover(h)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}

	go func() {
		log.Printf("listening on %s", *addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// page serves one embedded HTML file. HTML is revalidated; assets are not.
func page(site fs.FS, name string) http.HandlerFunc {
	b, err := fs.ReadFile(site, name)
	if err != nil {
		log.Fatalf("missing page %s: %v", name, err)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(b)
	}
}

func staticHandler(site fs.FS) http.Handler {
	fileServer := http.FileServerFS(site)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// HTML pages are only reachable via their routes.
		if len(r.URL.Path) > 5 && r.URL.Path[len(r.URL.Path)-5:] == ".html" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		fileServer.ServeHTTP(w, r)
	})
}
