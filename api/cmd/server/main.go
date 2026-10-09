package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"foodmap/internal/api"
	"foodmap/internal/config"
	"foodmap/internal/store"
)
// redeploy
func main() {
	envPath := flag.String("env", ".env", "path to .env file (optional; real env vars take precedence)")
	dbPath := flag.String("db", "data/places.db", "path to read-only sqlite db")
	trustProxy := flag.Bool("trust-proxy", false, "trust X-Forwarded-For from a reverse proxy")
	flag.Parse()

	cfg, err := config.Load(*envPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if len(cfg.FrontendOrigins) == 0 {
		log.Print("warning: FRONTEND_URL is empty; browsers on other origins cannot call this API")
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer st.Close()

	a, err := api.New(context.Background(), st)
	if err != nil {
		log.Fatalf("init api: %v", err)
	}

	apiMux := http.NewServeMux()
	a.Register(apiMux)

	mux := http.NewServeMux()
	mux.Handle("/api/", api.NewRateLimit(10, 30, *trustProxy).Wrap(apiMux))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	var h http.Handler = mux
	h = api.CORS(cfg.FrontendOrigins, h)
	h = api.Gzip(h)
	h = api.SecurityHeaders(h)
	h = api.Recover(h)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}

	go func() {
		log.Printf("listening on :%s", cfg.Port)
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
