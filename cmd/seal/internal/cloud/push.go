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
	"path/filepath"
	"time"

	"github.com/lemon4ksan/ctrl/argkit"

	aeon "github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/container/eon/objstore"
)

// PushCmd uploads decomposed container chunks and manifest to remote object storage.
type PushCmd struct{}

func (c *PushCmd) Name() string      { return "push" }
func (c *PushCmd) Aliases() []string { return []string{"publish"} }
func (c *PushCmd) Synopsis() string {
	return "Decompose and upload container to S3/R2/local store with CAS deduplication"
}
func (c *PushCmd) Usage() string {
	return "aeon push [flags] <archive.eon> <target-uri>"
}

func (c *PushCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("push", flag.ContinueOnError)
	fset.SetOutput(stderr)

	verbose := fset.Bool("v", false, "Verbose upload progress")

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(remaining) < 2 {
		return errors.New("usage: aeon push [-v] <archive.eon> <target-uri>\n  target-uri examples:\n    s3://my-bucket/prefix\n    r2://my-bucket/prefix\n    file:///path/to/decomposed_store\n    ./local_dir")
	}

	archivePath := remaining[0]
	targetURI := remaining[1]

	store, prefix, err := objstore.ParseURI(targetURI)
	if err != nil {
		return fmt.Errorf("parsing target URI %q: %w", targetURI, err)
	}

	fmt.Fprintf(stdout, "Decomposing and pushing %s to %s...\n", filepath.Base(archivePath), targetURI)
	start := time.Now()
	stats, err := aeon.DecomposeArchive(archivePath, store, prefix)
	if err != nil {
		return fmt.Errorf("push error: %w", err)
	}

	elapsed := time.Since(start)
	totalBytes := stats.UploadedBytes + stats.ReusedBytes
	dedupRate := 0.0
	if totalBytes > 0 {
		dedupRate = float64(stats.ReusedBytes) / float64(totalBytes) * 100.0
	}

	fmt.Fprintf(stdout, "\n✓ Successfully pushed in %s\n", elapsed.Round(time.Millisecond))
	fmt.Fprintf(stdout, "  Total chunks:    %d\n", stats.TotalChunks)
	fmt.Fprintf(stdout, "  Uploaded chunks: %d (%s)\n", stats.UploadedChunks, util.FormatHumanBytes(stats.UploadedBytes))
	fmt.Fprintf(stdout, "  Reused chunks:   %d (%s) [%.1f%% deduplicated]\n", stats.ReusedChunks, util.FormatHumanBytes(stats.ReusedBytes), dedupRate)
	fmt.Fprintf(stdout, "  Index size:      %s (index.eon)\n", util.FormatHumanBytes(uint64(stats.IndexBytes)))
	if *verbose {
		fmt.Fprintf(stdout, "  Target URL:      %s\n", store.URL(prefix))
	}
	return nil
}
