package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smartinbox17-droid/canvas/backend/internal/database"
	"github.com/smartinbox17-droid/canvas/backend/internal/handler"
	"github.com/smartinbox17-droid/canvas/backend/internal/platform"
	"github.com/smartinbox17-droid/canvas/backend/internal/service"
)

func main() {
	cfg := platform.LoadConfigFromEnv()

	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	coord, err := platform.NewCoordinator(cfg.RedisURL)
	if err != nil {
		log.Fatalf("failed to init coordinator: %v", err)
	}

	app, err := service.NewApplication(cfg, db, coord)
	if err != nil {
		log.Fatalf("failed to init application: %v", err)
	}
	defer func() { _ = app.Close() }()

	h := handler.New(app)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           h.NewRouter(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("canvas backend listening on %s (driver=%s)", cfg.Addr, cfg.DatabaseDriver)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	app.SetReady(false)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
}
