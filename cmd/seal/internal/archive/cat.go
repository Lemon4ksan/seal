// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive

import (
	"context"
	"errors"
	"flag"
	"io"

	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal"
)

// CatCmd streams a file entry to stdout.
type CatCmd struct{}

func (c *CatCmd) Name() string      { return "cat" }
func (c *CatCmd) Aliases() []string { return nil }
func (c *CatCmd) Synopsis() string  { return "Stream a specific file entry directly to stdout" }
func (c *CatCmd) Usage() string     { return "seal cat [flags] <archive-path> <entry-path>" }

func (c *CatCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("cat", flag.ContinueOnError)
	pwd := fs.String("p", "", "Decryption password")

	posArgs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(posArgs) < 2 {
		return errors.New("usage: seal cat <archive> <entry-path>")
	}
	archivePath := posArgs[0]
	targetEntry := posArgs[1]

	archiver := seal.NewArchiver(seal.WithPassword(*pwd))
	data, err := archiver.ReadFile(ctx, seal.FromFilePath(archivePath), targetEntry)
	if err != nil {
		return err
	}

	_, err = stdout.Write(data)
	return err
}
