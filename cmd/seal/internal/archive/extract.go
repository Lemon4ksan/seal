// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal"
)

// ExtractCmd implements archive extraction.
type ExtractCmd struct{}

func (c *ExtractCmd) Name() string      { return "extract" }
func (c *ExtractCmd) Aliases() []string { return []string{"x", "e"} }
func (c *ExtractCmd) Synopsis() string  { return "Extract an archive to destination directory" }
func (c *ExtractCmd) Usage() string     { return "seal extract [flags] <archive-path> -d <output-dir>" }

func (c *ExtractCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	destDir := fs.String("d", ".", "Destination extraction directory")
	pwd := fs.String("p", "", "Decryption password")

	posArgs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(posArgs) < 1 {
		return errors.New("archive file not specified")
	}
	archivePath := posArgs[0]

	fmt.Fprintf(stdout, "\nExtracting %s to %s...\n", tuikit.Bold(archivePath), tuikit.Bold(*destDir))
	start := time.Now()

	var extractedCount int64
	archiver := seal.NewArchiver(
		seal.WithPassword(*pwd),
		seal.WithOnFileDone(func(name string, err error) {
			if err == nil {
				atomic.AddInt64(&extractedCount, 1)
			}
		}),
	)
	err = archiver.UnarchiveWithContext(ctx, seal.FromFilePath(archivePath), *destDir, seal.WithPassword(*pwd))
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "%s Extraction completed in %v (%d files extracted)\n\n",
		tuikit.BadgePass(), time.Since(start), extractedCount)
	return nil
}
