package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"server/internal/audit"
	"server/internal/auth"
	"server/internal/database"
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

	db, err := database.FromEnv(log)
	if err != nil {
		log.Error("database initialization failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	authSvc, err := auth.FromEnv(db.DB, log)
	if err != nil {
		log.Error("auth initialization failed", "err", err)
		os.Exit(1)
	}

	auditor, err := audit.New(filepath.Join(store.Root(), "audit.jsonl"), log)
	if err != nil {
		log.Error("audit initialization failed", "err", err)
		os.Exit(1)
	}
	defer auditor.Close()

	r := router.New(log, store, authSvc, auditor)

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
