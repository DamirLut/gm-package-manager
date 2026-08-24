package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
)

var ErrNotExist = errors.New("storage: entry does not exist")

type Storage interface {
	GetManifest(ctx context.Context, pkg string) ([]byte, error)
	PutManifest(ctx context.Context, pkg string, data []byte) error
	GetTarball(ctx context.Context, pkg, filename string) (io.ReadCloser, int64, error)
	PutTarball(ctx context.Context, pkg, filename string, data io.Reader) (int64, error)
	ListPackages(ctx context.Context) ([]string, error)
	Lock(pkg string) (unlock func())
}

const (
	BackendLocal = "local"
	BackendS3    = "s3"
	DefaultPath  = "./storage"
)

func FromEnv(log *slog.Logger) (Storage, error) {
	switch backend := os.Getenv("STORAGE_BACKEND"); backend {
	case "", BackendLocal:
		root := os.Getenv("STORAGE_PATH")
		if root == "" {
			root = DefaultPath
		}
		log.Info("storage selected", "backend", BackendLocal, "path", root)
		return NewLocal(root), nil
	case BackendS3:
		return nil, fmt.Errorf("storage: backend %q is not implemented yet", backend)
	default:
		return nil, fmt.Errorf("storage: unknown backend %q, want local or s3", backend)
	}
}
