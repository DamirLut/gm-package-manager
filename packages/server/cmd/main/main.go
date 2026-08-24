package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"server/internal/logger"
	"server/internal/router"
	"server/internal/storage"
)

const addr = ":8080"

func main() {
	log := logger.New(os.Stdout)
	slog.SetDefault(log)

	store, err := storage.FromEnv(log)
	if err != nil {
		log.Error("storage initialization failed", "err", err)
		os.Exit(1)
	}

	r := router.New(log, store)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", "err", err)
	}
	log.Info("server stopped")
}
