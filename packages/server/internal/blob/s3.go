package blob

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config describes an S3-compatible endpoint
type S3Config struct {
	Endpoint  string // host[:port], no scheme
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	Secure    bool // TLS
	PathStyle bool // <endpoint>/<bucket> addressing, required by MinIO/R2
}

func s3ConfigFromEnv() (S3Config, error) {
	cfg := S3Config{
		Endpoint:  strings.TrimSpace(os.Getenv("S3_ENDPOINT")),
		Bucket:    strings.TrimSpace(os.Getenv("S3_BUCKET")),
		Region:    envOr("S3_REGION", "us-east-1"),
		AccessKey: os.Getenv("S3_ACCESS_KEY"),
		SecretKey: os.Getenv("S3_SECRET_KEY"),
		Secure:    os.Getenv("S3_SECURE") != "false",
		PathStyle: os.Getenv("S3_PATH_STYLE") == "true",
	}
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return cfg, fmt.Errorf("blob: s3 backend requires S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY and S3_SECRET_KEY")
	}
	return cfg, nil
}

// S3 is the FS driver over an S3-compatible bucket. Object keys map
// one-to-one onto bucket keys.
type S3 struct {
	client *minio.Client
	bucket string
}

func NewS3(cfg S3Config) (*S3, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.Secure,
		Region: cfg.Region,
		BucketLookup: func() minio.BucketLookupType {
			if cfg.PathStyle {
				return minio.BucketLookupPath
			}
			return minio.BucketLookupAuto
		}(),
	})
	if err != nil {
		return nil, fmt.Errorf("blob: s3 client: %w", err)
	}
	return &S3{client: client, bucket: cfg.Bucket}, nil
}

// Ping verifies the credentials and that the bucket exists — the startup
// equivalent of PocketBase's settings connection test.
func (s *S3) Ping(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("blob: s3 check bucket %q: %w", s.bucket, err)
	}
	if !exists {
		return fmt.Errorf("blob: s3 bucket %q does not exist", s.bucket)
	}
	return nil
}

func (s *S3) Backend() string { return BackendS3 }

func (s *S3) Put(ctx context.Context, key string, data io.Reader) (int64, error) {
	if !ValidKey(key) {
		return 0, fmt.Errorf("blob: invalid key %q", key)
	}
	info, err := s.client.PutObject(ctx, s.bucket, key, data, -1, minio.PutObjectOptions{})
	if err != nil {
		return 0, fmt.Errorf("blob: s3 put %q: %w", key, err)
	}
	return info.Size, nil
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if !ValidKey(key) {
		return nil, 0, fmt.Errorf("blob: invalid key %q", key)
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("blob: s3 get %q: %w", key, err)
	}
	stat, err := obj.Stat()
	if err != nil {
		obj.Close()
		if s3NotExist(err) {
			return nil, 0, ErrNotExist
		}
		return nil, 0, fmt.Errorf("blob: s3 stat %q: %w", key, err)
	}
	return obj, stat.Size, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	if !ValidKey(key) {
		return fmt.Errorf("blob: invalid key %q", key)
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("blob: s3 delete %q: %w", key, err)
	}
	return nil
}

func (s *S3) List(ctx context.Context, prefix string) ([]Object, error) {
	if !ValidPrefix(prefix) {
		return nil, fmt.Errorf("blob: invalid prefix %q", prefix)
	}
	var objs []Object
	for item := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if item.Err != nil {
			return nil, fmt.Errorf("blob: s3 list %q: %w", prefix, item.Err)
		}
		if item.Key == "" {
			continue
		}
		objs = append(objs, Object{Key: item.Key, Size: item.Size, Modified: item.LastModified})
	}
	sortObjects(objs)
	return objs, nil
}

func s3NotExist(err error) bool {
	resp := minio.ToErrorResponse(err)
	return resp.Code == "NoSuchKey" || resp.Code == "NotFound"
}
