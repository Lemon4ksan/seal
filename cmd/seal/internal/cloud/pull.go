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
	"os"
	"time"

	"github.com/lemon4ksan/ctrl/argkit"

	aeon "github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/container/eon/objstore"
)

// PullCmd downloads and composes remote container into monolithic .eon archive.
type PullCmd struct{}

func (c *PullCmd) Name() string      { return "pull" }
func (c *PullCmd) Aliases() []string { return nil }
func (c *PullCmd) Synopsis() string {
	return "Download and compose remote container from S3/R2/local store into monolithic .eon"
}
func (c *PullCmd) Usage() string {
	return "aeon pull [flags] <source-uri> <archive.eon>"
}

func (c *PullCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("pull", flag.ContinueOnError)
	fset.SetOutput(stderr)

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(remaining) < 2 {
		return errors.New("usage: aeon pull <source-uri> <archive.eon>")
	}

	sourceURI := remaining[0]
	outPath := remaining[1]

	store, prefix, err := objstore.ParseURI(sourceURI)
	if err != nil {
		return fmt.Errorf("parsing source URI %q: %w", sourceURI, err)
	}

	fmt.Fprintf(stdout, "Pulling and composing %s -> %s...\n", sourceURI, outPath)
	start := time.Now()

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	if err := aeon.ComposeArchive(store, prefix, f); err != nil {
		return fmt.Errorf("pull error: %w", err)
	}

	fi, _ := f.Stat()
	elapsed := time.Since(start)
	fmt.Fprintf(stdout, "\n✓ Successfully pulled monolithic archive in %s (%s)\n", elapsed.Round(time.Millisecond), util.FormatHumanBytes(uint64(fi.Size())))
	return nil
}
