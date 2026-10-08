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
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

// CreateCmd implements archive creation with full 7-Zip CLI compatibility (command 'a').
type CreateCmd struct{}

func (c *CreateCmd) Name() string      { return "create" }
func (c *CreateCmd) Aliases() []string { return []string{"a", "c", "pack"} }
func (c *CreateCmd) Synopsis() string {
	return "Create an archive (EON, 7z, ZIP, TAR, TAR.GZ, TAR.XZ, TAR.ZST, TAR.BZ2)"
}
func (c *CreateCmd) Usage() string {
	return `seal a [flags] <archive-path> <files...> [-o <archive-path>]

Flags (7-Zip Compatible):
  -mx=<0..9>       Set compression level (0 = Store, 1 = Fastest, 9 = Ultra)
  -m=<method>      Set compression method (lzma2, zstd, przm, brotli, gzip, deflate, store)
  -p<password>     Encrypt the archive with a password
  -w<dir>          Set working directory for temporary files
  -mmt=<N>         Set number of CPU threads to use (default: auto)
  -sdel            Delete source files after successful compression
  -x!<pattern>     Exclude files matching pattern (e.g. -x!*.tmp)
  -r               Recurse into subdirectories (default: on)

Examples:
  seal a backup.eon ./docs ./images
  seal a -mx=9 -m=zstd archive.tar.zst ./data
  seal c -pSecretPassword secure.7z ./confidential
`
}

func (c *CreateCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	sz, err := util.ParseSevenZipArgs("a", args)
	if err != nil {
		return err
	}

	outFile := sz.Archive
	if outFile == "" {
		outFile = "archive.7z"
	}

	rawInputs := sz.Items
	if len(rawInputs) == 0 {
		return errors.New("no input files or directories specified")
	}

	// Expand shell wildcard globs if unexpanded by shell (e.g. Windows CMD / PowerShell)
	var inputs []string
	for _, input := range rawInputs {
		if strings.ContainsAny(input, "*?[") {
			matches, err := filepath.Glob(input)
			if err == nil && len(matches) > 0 {
				inputs = append(inputs, matches...)
				continue
			}
		}
		inputs = append(inputs, input)
	}

	cType := util.DetectContainerType(outFile)
	method := codec.Deflate
	if sz.Method != "" {
		switch strings.ToLower(sz.Method) {
		case "store", "none", "copy":
			method = codec.Store
		case "gzip", "gz":
			method = codec.Gzip
		case "lzma2", "7z":
			method = codec.LZMA2
		case "ppmd":
			method = codec.PPMd
		case "zstd", "zstandard":
			method = codec.ZStandard
		case "przm", "prism":
			method = codec.PRZM
		case "cmpsc", "method16":
			method = codec.IBMCmpsc
		case "brotli", "br":
			method = codec.Brotli
		}
	} else {
		switch cType {
		case container.SevenZip:
			method = codec.LZMA2
		case container.Eon:
			method = codec.PRZM
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

	level := 6
	if sz.Level >= 0 {
		level = sz.Level
	} else if method == codec.PRZM {
		level = 3
	}

	opts := []seal.Option{
		seal.WithContainer(cType),
		seal.WithCompression(method, level),
		seal.WithPassword(sz.Password),
		seal.WithComment(sz.Comment),
		seal.WithBytecodeFilter(sz.Filter),
	}
	if sz.Workers > 0 {
		opts = append(opts, seal.WithWorkers(sz.Workers))
	}

	archiveInstance := seal.NewArchive(opts...)

	fmt.Fprintf(stdout, "\nScanning inputs for %s (%s)...\n", tuikit.Bold(outFile), tuikit.Magenta(cType.String()))
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

	// Apply exclude filters if specified via -x!*.tmp
	if len(sz.ExcludePatterns) > 0 {
		archiveInstance.Filter(func(f *file.File) bool {
			for _, excl := range sz.ExcludePatterns {
				if util.MatchPattern(excl, f.Name(), sz.Recurse) {
					return false
				}
			}
			return true
		})
	}

	for _, f := range archiveInstance.Files() {
		totalFiles++
		totalBytes += f.UncompressedSize()
	}

	displayFilter := sz.Filter
	if method == codec.PRZM {
		if sz.Filter == "none" || sz.Filter == "" {
			displayFilter = "auto (x86-bcj+split)"
		} else {
			displayFilter = sz.Filter + " (przm-internal)"
		}
	}

	fmt.Fprintf(stdout, "Compressing %d entries (%s uncompressed) via %s (Level %d, Filter: %s)...\n",
		totalFiles, util.FormatBytes(totalBytes), tuikit.Bold(method.String()), level, displayFilter)

	var p *tuikit.Progress
	if tuikit.ProbeTerminal(stdout) {
		p = tuikit.NewProgress(
			tuikit.WithWriter(stdout),
			tuikit.WithPrefix("Compressing"),
			tuikit.WithTotal(totalBytes),
			tuikit.WithUnit(tuikit.UnitBytes),
			tuikit.WithBarWidth(24),
		)
	}

	start := time.Now()
	var skippedFiles atomic.Int64

	err = seal.UseSink(seal.ToFilePath(outFile), func(w io.Writer) error {
		writeOpts := []seal.Option{
			seal.WithResumable(true),
			seal.WithBytecodeFilter(sz.Filter),
			seal.WithOnFileDone(func(entryName string, err error) {
				if err != nil {
					skippedFiles.Add(1)
				}
			}),
		}
		if sz.Workers > 0 {
			writeOpts = append(writeOpts, seal.WithWorkers(sz.Workers))
		}
		if len(inputs) > 0 {
			writeOpts = append(writeOpts, seal.WithSourceDir(inputs[0]))
		}
		if p != nil {
			writeOpts = append(writeOpts, seal.WithProgress(func(stats seal.ProgressStats) {
				p.SetCurrent(stats.TotalRead)
			}))
		}
		_, err := archiveInstance.WriteTo(ctx, w, writeOpts...)
		return err
	})
	if p != nil {
		p.Done()
	}
	if err != nil {
		if errors.Is(err, seal.ErrArchiveFrozen) {
			box := tuikit.NewBox("Archive Creation Frozen (Checkpoint Saved)", 60).
				SetStyle(tuikit.BorderRounded).
				SetIndent(0).
				AddRow("Target Archive", tuikit.Bold(outFile)).
				AddRow("Interrupted", tuikit.Yellow("Ctrl+C / Cancel received")).
				AddRow("Integrity", tuikit.Green("Archive is 100% valid & inspectable")).
				AddRow("Resumption", tuikit.Cyan("Run create again to resume without data loss"))

			fmt.Println()
			return box.Render(stdout)
		}
		_ = os.Remove(outFile)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			fmt.Fprintf(stdout, "\n%s Archive creation cancelled by user\n\n", tuikit.BadgeFail())
			return ctx.Err()
		}
		return err
	}

	// Delete source files if -sdel was specified
	if sz.DeleteSource {
		for _, input := range inputs {
			_ = os.RemoveAll(input)
		}
	}

	elapsed := time.Since(start)
	stat, _ := os.Stat(outFile)
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
		AddRow("Output Archive", tuikit.Bold(outFile)).
		AddRow("Entries Count", fmt.Sprintf("%d files", totalFiles)).
		AddRow("Original Size", util.FormatBytes(totalBytes)).
		AddRow("Compressed Size", util.FormatBytes(compSize)).
		AddRow("Compression Ratio", fmt.Sprintf("%.2f%%", ratio)).
		AddRow("Throughput", fmt.Sprintf("%.2f MB/s (%v)", speed, elapsed))

	if skipped := skippedFiles.Load(); skipped > 0 {
		box.AddRow("Warnings", tuikit.Yellow(fmt.Sprintf("%d locked files skipped", skipped)))
	}

	fmt.Println()
	return box.Render(stdout)
}

