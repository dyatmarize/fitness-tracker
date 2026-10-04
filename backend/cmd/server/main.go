package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/dyatmarize/fitness-tracker/backend/internal/api"
	"github.com/dyatmarize/fitness-tracker/backend/internal/config"
	syncpkg "github.com/dyatmarize/fitness-tracker/backend/internal/sync"
	"github.com/dyatmarize/fitness-tracker/backend/internal/coros"
	"github.com/dyatmarize/fitness-tracker/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := store.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	log.Printf("database ready")

	client := coros.New(cfg, pool)
	go func() {
		if err := client.StartCallbackServer(ctx); err != nil {
			log.Printf("oauth callback server: %v", err)
		}
	}()

	worker := syncpkg.NewWorker(cfg, pool, client)
	go worker.Run(ctx)

	srv := api.New(cfg, pool, client, worker)
	httpSrv := &http.Server{Addr: cfg.ListenAddr, Handler: srv.Router()}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Close()
	}()
	log.Printf("api listening on %s", cfg.ListenAddr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
