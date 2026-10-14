// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cloud

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/lemon4ksan/ctrl/argkit"

	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	aeon "github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/server"
)

// ServeCmd starts a high-performance HTTP Range CDN edge gateway.
type ServeCmd struct{}

func (c *ServeCmd) Name() string      { return "serve" }
func (c *ServeCmd) Aliases() []string { return nil }
func (c *ServeCmd) Synopsis() string {
	return "Start high-performance HTTP Range CDN edge gateway"
}

func (c *ServeCmd) Usage() string {
	return "seal serve [flags] <archive-path-or-uri>"
}

func (c *ServeCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("serve", flag.ContinueOnError)
	fset.SetOutput(stderr)

	port := fset.Int("port", 8080, "HTTP port to bind")
	fset.IntVar(port, "P", 8080, "HTTP port to bind")
	bindAddr := fset.String("bind", "127.0.0.1", "Network interface address to bind")
	cors := fset.Bool("cors", true, "Enable CORS headers for web browsers")
	token := fset.String("token", "", "Optional Bearer auth token requirement")
	cacheDir := fset.String("cache", os.Getenv("AEON_CACHE_DIR"), "Local chunk cache directory")
	passwords, shares, kmsKey, vaultAddr, vaultToken := util.SetupUnlockFlags(fset)

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(remaining) < 1 {
		return errors.New(
			"usage: aeon serve [-p port] [-bind addr] [--cors] [--token secret] [--share s1] [--kms keyID] <archive-path-or-uri>\n  examples:\n    aeon serve archive.eon\n    aeon serve s3://my-bucket/models/llama3\n    aeon serve r2://dataset-lake/coco-2024",
		)
	}

	sourcePath := remaining[0]
	opts, err := util.BuildUnlockOptions(*passwords, *shares, *kmsKey, *vaultAddr, *vaultToken)
	if err != nil {
		return fmt.Errorf("configuring unlock credentials: %w", err)
	}
	if *cacheDir != "" {
		opts = append(opts, aeon.WithDiskCache(*cacheDir))
	}

	r, closer, err := aeon.OpenFile(sourcePath, opts...)
	if err != nil {
		return fmt.Errorf("opening container %s: %w", sourcePath, err)
	}
	defer closer.Close()

	srv := server.New(r, server.Config{
		Addr:       *bindAddr,
		Port:       *port,
		EnableCORS: *cors,
		AuthToken:  *token,
		CacheDir:   *cacheDir,
	})

	baseURL := fmt.Sprintf("http://%s:%d", *bindAddr, *port)
	fmt.Fprintf(stdout, "\n🚀 Aeon Edge CDN Gateway active on %s\n", baseURL)
	fmt.Fprintf(stdout, "   Source:      %s\n", sourcePath)
	fmt.Fprintf(stdout, "   Files:       %d virtual extents\n", len(r.Extents()))
	fmt.Fprintf(stdout, "   Chunks:      %d content-addressed chunks\n", r.TotalChunks())
	fmt.Fprintf(stdout, "   Endpoints:\n")
	fmt.Fprintf(stdout, "     • Web / VFS:  %s/<file-path> (supports HTTP Range 206 Partial Content)\n", baseURL)
	fmt.Fprintf(stdout, "     • CAS Chunks: %s/chunks/<hash> (Cache-Control: immutable)\n", baseURL)
	fmt.Fprintf(stdout, "     • Manifest:   %s/index.eon\n", baseURL)
	fmt.Fprintf(stdout, "     • API Stats:  %s/api/v1/info\n\n", baseURL)

	serverErrCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-serverErrCh:
		return fmt.Errorf("server error: %w", err)
	}
}
