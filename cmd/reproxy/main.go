package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/utada1stlove/reproxy/internal/app"
	"github.com/utada1stlove/reproxy/internal/httpapi"
	"github.com/utada1stlove/reproxy/internal/nginx"
	runtimecfg "github.com/utada1stlove/reproxy/internal/runtime"
	"github.com/utada1stlove/reproxy/internal/store"
)

type startupSyncer interface {
	Sync(ctx context.Context) error
}

func main() {
	logger := log.New(os.Stdout, "reproxy ", log.Ldate|log.Ltime|log.Lmsgprefix)
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := runtimecfg.Load()
	routeStore := store.NewFileStore(cfg.StoragePath)

	syncer, err := nginx.NewSyncer(cfg)
	if err != nil {
		logger.Fatalf("build nginx syncer: %v", err)
	}

	manager := app.NewManager(routeStore, syncer)
	syncOnStartup(context.Background(), logger, manager)

	server := httpapi.NewServer(cfg.ListenAddr, logger, manager)
	logger.Printf("serving API on %s", cfg.ListenAddr)

	go func() {
		<-rootCtx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Printf("graceful shutdown failed: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("server stopped: %v", err)
	}

	logger.Printf("server stopped cleanly")
}

func syncOnStartup(ctx context.Context, logger *log.Logger, syncer startupSyncer) {
	if err := syncer.Sync(ctx); err != nil {
		logger.Printf("initial nginx sync failed, continuing in degraded mode: %v", err)
	}
}
