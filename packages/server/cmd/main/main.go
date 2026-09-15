package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"server/internal/access"
	"server/internal/audit"
	"server/internal/auth"
	"server/internal/backup"
	"server/internal/blob"
	"server/internal/database"
	"server/internal/identity"
	"server/internal/logger"
	"server/internal/router"
	"server/internal/storage"
)

func main() {
	log := logger.New(os.Stdout)
	slog.SetDefault(log)
	addr := envOr("SERVER_ADDR", ":8080")
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Error("env file loading failed", "err", err)
		os.Exit(1)
	}

	fsys, err := blob.FromEnv(log)
	if err != nil {
		log.Error("storage initialization failed", "err", err)
		os.Exit(1)
	}
	store := storage.New(fsys)

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

	identitySvc, err := identity.FromEnv(db.DB, log)
	if err != nil {
		log.Error("identity initialization failed", "err", err)
		os.Exit(1)
	}

	rules, err := access.FromEnv(log)
	if err != nil {
		log.Error("access config failed", "err", err)
		os.Exit(1)
	}

	dataDir := envOr("STORAGE_PATH", blob.DefaultPath)
	auditPath := envOr("AUDIT_PATH", filepath.Join(dataDir, "audit.jsonl"))
	auditor, err := audit.New(auditPath, log)
	if err != nil {
		log.Error("audit initialization failed", "err", err)
		os.Exit(1)
	}
	defer auditor.Close()

	backups := backup.New(fsys, db.DB, envOr("DATABASE_PATH", database.DefaultPath),
		auditPath, dataDir, log)

	if spec := os.Getenv("BACKUP_CRON"); spec != "" {
		keep, err := strconv.Atoi(os.Getenv("BACKUP_KEEP"))
		if err != nil {
			log.Error("invalid BACKUP_KEEP", "value", os.Getenv("BACKUP_KEEP"))
			os.Exit(1)
		}
		if err := backups.StartScheduler(spec, keep); err != nil {
			log.Error("backup scheduler failed", "err", err)
			os.Exit(1)
		}
	}

	r := router.New(log, store, authSvc, identitySvc, auditor, rules)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	listenErr := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", addr)
		listenErr <- srv.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-listenErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", "err", err)
	}
	backups.StopScheduler()
	log.Info("server stopped")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
