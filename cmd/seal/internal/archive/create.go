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
	"os"
	"strings"
	"time"

	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/option"
)

// CreateCmd implements archive creation.
type CreateCmd struct{}

func (c *CreateCmd) Name() string      { return "create" }
func (c *CreateCmd) Aliases() []string { return []string{"a", "c"} }
func (c *CreateCmd) Synopsis() string {
	return "Create an archive (7z, ZIP, TAR, TAR.GZ, TAR.XZ, TAR.ZST, TAR.BZ2)"
}
func (c *CreateCmd) Usage() string { return "seal create [flags] <files...> -o <archive-path>" }

func (c *CreateCmd) Run(_ context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	outFile := fs.String("o", "archive.7z", "Output archive destination path")
	pwd := fs.String("p", "", "Password for encryption")
	methodStr := fs.String("m", "", "Compression method (lzma2, ppmd, zstd, deflate, gzip, store, cmpsc)")
	filterStr := fs.String("filter", "none", "Bytecode filter: bcj (x86), arm, none")
	level := fs.Int("l", 6, "Compression level (1-9)")
	comment := fs.String("c", "", "Archive comment")

	inputs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return errors.New("no input files or directories specified")
	}

	cType := util.DetectContainerType(*outFile)
	method := codec.Deflate
	if *methodStr != "" {
		switch strings.ToLower(*methodStr) {
		case "store", "none":
			method = codec.Store
		case "gzip", "gz":
			method = codec.Gzip
		case "lzma2", "7z":
			method = codec.LZMA2
		case "ppmd":
			method = codec.PPMd
		case "zstd", "zstandard":
			method = codec.ZStandard
		case "cmpsc", "method16":
			method = codec.IBMCmpsc
		}
	} else {
		switch cType {
		case container.SevenZip:
			method = codec.LZMA2
		case container.TarGz:
			method = codec.Gzip
		case container.TarZst:
			method = codec.ZStandard
		case container.TarXz:
			method = codec.LZMA2
		default:
			method = codec.Deflate
		}
	}

	opts := []seal.Option{
		option.WithFormat(cType),
		seal.WithCompression(method, *level),
		seal.WithPassword(*pwd),
		seal.WithComment(*comment),
	}

	archiveInstance := seal.NewArchive(opts...)

	fmt.Fprintf(stdout, "\nScanning inputs for %s (%s)...\n", tuikit.Bold(*outFile), tuikit.Magenta(cType.String()))
	var totalFiles, totalBytes int64

	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return err
		}

		if info.IsDir() {
			if _, err := archiveInstance.AddDir(input); err != nil {
				return err
			}
		} else {
			if _, err := archiveInstance.AddFile(input); err != nil {
				return err
			}
		}
	}

	for _, f := range archiveInstance.Files() {
		totalFiles++
		totalBytes += f.UncompressedSize()
	}

	fmt.Fprintf(stdout, "Compressing %d entries (%s uncompressed) via %s (Level %d, Filter: %s)...\n",
		totalFiles, util.FormatBytes(totalBytes), tuikit.Bold(method.String()), *level, *filterStr)

	start := time.Now()

	err = seal.UseSink(seal.ToFilePath(*outFile), func(w io.Writer) error {
		_, err := archiveInstance.WriteTo(w)
		return err
	})
	if err != nil {
		_ = os.Remove(*outFile)
		return err
	}

	elapsed := time.Since(start)
	stat, _ := os.Stat(*outFile)
	var compSize int64
	if stat != nil {
		compSize = stat.Size()
	}

	ratio := 0.0
	if totalBytes > 0 {
		ratio = float64(compSize) / float64(totalBytes) * 100
	}
	speed := (float64(totalBytes) / (1024 * 1024)) / elapsed.Seconds()

	box := tuikit.NewBox("Archive Created Successfully", 60).
		SetStyle(tuikit.BorderRounded).
		SetIndent(0).
		AddRow("Output Archive", tuikit.Bold(*outFile)).
		AddRow("Entries Count", fmt.Sprintf("%d files", totalFiles)).
		AddRow("Original Size", util.FormatBytes(totalBytes)).
		AddRow("Compressed Size", util.FormatBytes(compSize)).
		AddRow("Compression Ratio", fmt.Sprintf("%.2f%%", ratio)).
		AddRow("Throughput", fmt.Sprintf("%.2f MB/s (%v)", speed, elapsed))

	fmt.Println()
	return box.Render(stdout)
}
