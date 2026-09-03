// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package forensics

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal"
)

// TestCmd verifies archive integrity.
type TestCmd struct{}

func (c *TestCmd) Name() string      { return "test" }
func (c *TestCmd) Aliases() []string { return []string{"t"} }
func (c *TestCmd) Synopsis() string  { return "Verify checksum and decompression integrity" }
func (c *TestCmd) Usage() string     { return "seal test [flags] <archive-path>" }

func (c *TestCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	pwd := fs.String("p", "", "Decryption password")

	posArgs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(posArgs) < 1 {
		return errors.New("archive file not specified")
	}
	archivePath := posArgs[0]

	fmt.Fprintf(stdout, "\nVerifying integrity of %s...\n", tuikit.Bold(archivePath))
	start := time.Now()

	archiver := seal.NewArchiver(seal.WithPassword(*pwd))
	err = archiver.Verify(ctx, seal.FromFilePath(archivePath), seal.WithPassword(*pwd))
	if err != nil {
		return fmt.Errorf("integrity verification failed: %w", err)
	}

	fmt.Fprintf(stdout, "%s All checksums and data streams verified successfully in %v\n\n",
		tuikit.BadgePassed(), time.Since(start))
	return nil
}
