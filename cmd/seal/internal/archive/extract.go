// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/ctrl/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
)

// ExtractCmd implements archive extraction with full 7-Zip compatibility (commands 'x' and 'e').
type ExtractCmd struct{}

func (c *ExtractCmd) Name() string      { return "extract" }
func (c *ExtractCmd) Aliases() []string { return []string{"x", "e"} }
func (c *ExtractCmd) Synopsis() string {
	return "Extract archive files (x: preserve paths, e: flat extract without directories)"
}
func (c *ExtractCmd) Usage() string {
	return `seal x [flags] <archive-path> [include-patterns...]

Flags (7-Zip Compatible):
  -o<dir>          Set output directory (default: current directory)
  -p<password>     Provide password for encrypted archives
  -y               Assume Yes on all queries (overwrite without prompt)
  -mmt=<N>         Set number of CPU threads to use (default: auto)

Examples:
  seal x backup.eon
  seal x archive.7z -o./extracted
  seal e -pSecretPassword secure.zip
`
}

func (c *ExtractCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	cmdAlias := "x"
	if len(os.Args) > 1 {
		first := strings.ToLower(os.Args[1])
		if first == "e" || first == "x" {
			cmdAlias = first
		}
	}

	sz, err := util.ParseSevenZipArgs(cmdAlias, args)
	if err != nil {
		return err
	}

	if sz.Archive == "" {
		return errors.New("archive file not specified")
	}

	isFlat := (cmdAlias == "e")
	destDir := sz.OutputDir
	if destDir == "" {
		destDir = "."
	}

	modeDesc := "with full paths"
	if isFlat {
		modeDesc = "flat (stripping directories)"
	}

	fmt.Fprintf(stdout, "\nExtracting %s to %s (%s)...\n",
		tuikit.Bold(sz.Archive), tuikit.Bold(destDir), tuikit.Cyan(modeDesc))

	if len(sz.IncludePatterns) > 0 {
		fmt.Fprintf(stdout, "Filters (include): %s\n", tuikit.Yellow(strings.Join(sz.IncludePatterns, ", ")))
	}
	if len(sz.ExcludePatterns) > 0 {
		fmt.Fprintf(stdout, "Filters (exclude): %s\n", tuikit.Red(strings.Join(sz.ExcludePatterns, ", ")))
	}

	var policy seal.OverwritePolicy
	switch sz.Overwrite {
	case util.OverwriteSkip:
		policy = seal.OverwriteSkip
	case util.OverwriteRenameNew:
		policy = seal.OverwriteRenameNew
	case util.OverwriteRenameExisting:
		policy = seal.OverwriteRenameExisting
	default:
		policy = seal.OverwriteAll
	}

	var extractedCount int64
	opts := []seal.Option{
		seal.WithPassword(sz.Password),
		seal.WithFlat(isFlat),
		seal.WithOverwritePolicy(policy),
		seal.WithOnFileDone(func(name string, err error) {
			if err == nil {
				atomic.AddInt64(&extractedCount, 1)
			}
		}),
	}

	if sz.Workers > 0 {
		opts = append(opts, seal.WithWorkers(sz.Workers))
	}

	if len(sz.IncludePatterns) > 0 || len(sz.ExcludePatterns) > 0 {
		filterFn := func(entryName string, isDir bool) bool {
			return util.MatchFilters(entryName, isDir, sz.IncludePatterns, sz.ExcludePatterns, sz.Recurse)
		}
		opts = append(opts, seal.WithFilter(filterFn))
	}

	start := time.Now()
	archiver := seal.NewArchiver(opts...)

	err = archiver.Unarchive(ctx, seal.FromFilePath(sz.Archive), destDir, opts...)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			fmt.Fprintf(stdout, "\n%s Extraction interrupted by user (context cancelled)\n\n", tuikit.BadgeFail())
			return ctx.Err()
		}
		return err
	}

	fmt.Fprintf(stdout, "%s Extraction completed in %v (%d files extracted)\n\n",
		tuikit.BadgePass(), time.Since(start), extractedCount)
	return nil
}

