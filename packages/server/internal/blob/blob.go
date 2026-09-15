// Package blob is the low-level object store shared by the package
// registry and the backup feature:
// one interface with a local-directory and an S3-compatible driver.
package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"
)

var ErrNotExist = errors.New("blob: object does not exist")

const (
	BackendLocal = "local"
	BackendS3    = "s3"

	DefaultPath = "./storage"
)

type Object struct {
	Key      string
	Size     int64
	Modified time.Time
}

// FS is a flat key/object store. Delete is idempotent; reads and lists of
// missing keys return ErrNotExist.
type FS interface {
	Backend() string
	Put(ctx context.Context, key string, data io.Reader) (int64, error)
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]Object, error)
}

// FromEnv builds the blob filesystem selected by STORAGE_BACKEND:
// "local" (default) stores objects under STORAGE_PATH, "s3" stores them
// in the S3-compatible bucket described by the S3_* variables. The s3
// driver verifies credentials and bucket existence before returning.
func FromEnv(log *slog.Logger) (FS, error) {
	switch backend := os.Getenv("STORAGE_BACKEND"); backend {
	case "", BackendLocal:
		root := envOr("STORAGE_PATH", DefaultPath)
		log.Info("storage selected", "backend", BackendLocal, "path", root)
		return NewLocal(root), nil
	case BackendS3:
		cfg, err := s3ConfigFromEnv()
		if err != nil {
			return nil, err
		}
		s3, err := NewS3(cfg)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s3.Ping(ctx); err != nil {
			return nil, err
		}
		log.Info("storage selected", "backend", BackendS3,
			"endpoint", cfg.Endpoint, "bucket", cfg.Bucket, "region", cfg.Region)
		return s3, nil
	default:
		return nil, fmt.Errorf("blob: unknown backend %q, want local or s3", backend)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ValidKey reports whether key is a safe object key: slash-separated
// segments with no empty parts or traversal.
func ValidKey(key string) bool {
	return key != "" && !strings.Contains(key, "\\") && validSegments(key)
}

// ValidPrefix is ValidKey with an optional trailing slash.
func ValidPrefix(prefix string) bool {
	return prefix == "" || ValidKey(strings.TrimSuffix(prefix, "/"))
}

func validSegments(key string) bool {
	for seg := range strings.SplitSeq(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func sortObjects(objs []Object) {
	sort.Slice(objs, func(i, j int) bool { return objs[i].Key < objs[j].Key })
}
