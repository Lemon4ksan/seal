// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package objstore provides a zero-dependency object storage abstraction
// supporting local filesystems, AWS S3, Cloudflare R2, Google Cloud Storage (S3-compatible),
// and MinIO.
package objstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	// ErrNotFound is returned when an object does not exist in the store.
	ErrNotFound = errors.New("objstore: object not found")

	// ErrInvalidURI is returned when a storage URI is malformed.
	ErrInvalidURI = errors.New("objstore: invalid storage URI")
)

// ObjectInfo metadata for a stored object.
type ObjectInfo struct {
	Key          string
	Size         int64
	ETag         string
	LastModified time.Time
}

// Store defines the contract for an abstract object storage provider.
type Store interface {
	// Get opens the object at key for sequential reading.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// GetRange opens a byte-range [offset, offset+length) of the object at key.
	GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)

	// Put writes an object from r with the given size.
	Put(ctx context.Context, key string, r io.Reader, size int64) error

	// Head retrieves metadata for key without fetching its body.
	Head(ctx context.Context, key string) (ObjectInfo, error)

	// Delete removes the object at key.
	Delete(ctx context.Context, key string) error

	// List enumerates objects starting with prefix.
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)

	// Exists reports whether the object at key exists.
	Exists(ctx context.Context, key string) (bool, error)

	// URL returns a canonical or diagnostic string URL for key.
	URL(key string) string
}

// ParseURI parses an object storage URI and returns an instantiated Store and sub-prefix.
// Supported schemes:
//   - s3://<bucket>/<prefix...>
//   - r2://<bucket>/<prefix...> (uses Cloudflare R2 endpoint if configured)
//   - file://<absolute-path> or plain filesystem path
func ParseURI(uri string) (Store, string, error) {
	if uri == "" {
		return nil, "", ErrInvalidURI
	}

	// Plain directory path check
	if !strings.Contains(uri, "://") {
		absPath, err := filepath.Abs(uri)
		if err != nil {
			return nil, "", fmt.Errorf("resolve local path: %w", err)
		}
		return NewFSStore(absPath), "", nil
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrInvalidURI, err)
	}

	switch strings.ToLower(u.Scheme) {
	case "file":
		path := u.Path
		if u.Host != "" {
			path = filepath.Join(u.Host, path)
		}
		return NewFSStore(filepath.Clean(path)), "", nil

	case "s3":
		bucket := u.Host
		prefix := strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
		cfg := S3Config{
			Bucket:          bucket,
			Region:          os.Getenv("AWS_REGION"),
			AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
			Endpoint:        os.Getenv("AWS_ENDPOINT_URL"),
		}
		if cfg.Region == "" {
			cfg.Region = "us-east-1"
		}
		client, err := NewS3Store(cfg)
		if err != nil {
			return nil, "", err
		}
		return client, prefix, nil

	case "r2":
		bucket := u.Host
		prefix := strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
		endpoint := os.Getenv("R2_ENDPOINT_URL")
		if endpoint == "" && os.Getenv("CF_ACCOUNT_ID") != "" {
			endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", os.Getenv("CF_ACCOUNT_ID"))
		}
		cfg := S3Config{
			Bucket:          bucket,
			Region:          "auto",
			AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
			Endpoint:        endpoint,
		}
		if cfg.AccessKeyID == "" {
			cfg.AccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
			cfg.SecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
		}
		client, err := NewS3Store(cfg)
		if err != nil {
			return nil, "", err
		}
		return client, prefix, nil

	default:
		return nil, "", fmt.Errorf("%w: unsupported scheme %q", ErrInvalidURI, u.Scheme)
	}
}
