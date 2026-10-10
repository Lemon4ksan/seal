// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package forensics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lemon4ksan/ctrl/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
)

// TestCmd verifies archive integrity with full 7-Zip CLI compatibility (command 't').
type TestCmd struct{}

func (c *TestCmd) Name() string      { return "test" }
func (c *TestCmd) Aliases() []string { return []string{"t"} }
func (c *TestCmd) Synopsis() string  { return "Verify checksum and decompression integrity" }
func (c *TestCmd) Usage() string     { return "seal t [flags] <archive-path> [patterns...]" }

func (c *TestCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	sz, err := util.ParseSevenZipArgs("t", args)
	if err != nil {
		return err
	}

	if sz.Archive == "" {
		return errors.New("archive file not specified")
	}

	filterInfo := ""
	if len(sz.IncludePatterns) > 0 {
		filterInfo = fmt.Sprintf(" (matching %s)", strings.Join(sz.IncludePatterns, ", "))
	}

	fmt.Fprintf(stdout, "\nVerifying integrity of %s%s...\n", tuikit.Bold(sz.Archive), filterInfo)
	start := time.Now()

	opts := []seal.Option{
		seal.WithPassword(sz.Password),
	}

	if len(sz.IncludePatterns) > 0 || len(sz.ExcludePatterns) > 0 {
		filterFn := func(entryName string, isDir bool) bool {
			return util.MatchFilters(entryName, isDir, sz.IncludePatterns, sz.ExcludePatterns, sz.Recurse)
		}
		opts = append(opts, seal.WithFilter(filterFn))
	}

	if sz.Encoding != "" {
		opts = append(opts, seal.WithEncoding(sz.Encoding))
	}

	archiver := seal.NewArchiver(opts...)
	err = archiver.Verify(ctx, seal.FromFilePath(sz.Archive), opts...)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			fmt.Fprintf(stdout, "\n%s Integrity test cancelled by user\n\n", tuikit.BadgeFail())
			return ctx.Err()
		}
		return fmt.Errorf("integrity verification failed: %w", err)
	}

	fmt.Fprintf(stdout, "%s All checksums and data streams verified successfully in %v\n\n",
		tuikit.BadgePassed(), time.Since(start))
	return nil
}
