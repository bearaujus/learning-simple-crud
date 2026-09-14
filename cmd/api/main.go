package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"learning-simple-crud/internal/app"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	seed := flag.Bool("seed", false, "add sample products to an empty database, then exit")
	flag.Parse()
	db, err := app.OpenDatabase(env("DATABASE_PATH", "products.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	if *seed {
		added, err := app.SeedProducts(db)
		if err == nil {
			log.Printf("Added %d sample products (existing products are preserved).", added)
		}
		return err
	}

	server := &http.Server{
		Addr:              env("ADDR", "127.0.0.1:8080"),
		Handler:           app.NewHandler(db, strings.Split(env("CORS_ORIGINS", "*"), ",")),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Shutdown: %v", err)
		}
	}()
	log.Printf("CRUD playground: http://%s (Ctrl+C to stop)", server.Addr)
	err = server.ListenAndServe()
	stop()
	<-done
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
