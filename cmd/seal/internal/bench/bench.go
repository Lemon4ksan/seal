// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bench

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/lemon4ksan/ctrl/argkit"
	"github.com/lemon4ksan/ctrl/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/codec"
)

// BenchCmd runs the synthetic compression speed benchmark.
type BenchCmd struct{}

func (c *BenchCmd) Name() string      { return "bench" }
func (c *BenchCmd) Aliases() []string { return nil }
func (c *BenchCmd) Synopsis() string  { return "Run compression speed benchmark" }
func (c *BenchCmd) Usage() string     { return "seal bench [-size <MB>] [-files <count>] [-j <workers>]" }

func (c *BenchCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	sizeMB := fs.Int("size", 50, "Total payload size in MB")
	filesCount := fs.Int("files", 100, "Number of generated entries")
	workers := fs.Int("j", runtime.NumCPU(), "Worker concurrency")

	if _, err := argkit.ParseInterspersedFlags(fs, args); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "\n%s\n", tuikit.Bold("=== Seal Silicon Benchmark ==="))
	fmt.Fprintf(stdout, "Payload: %d MB across %d entries, CPU cores: %d\n\n", *sizeMB, *filesCount, *workers)

	pattern := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Integer nec odio. Praesent libero. ")
	totalBytes := (*sizeMB) * 1024 * 1024
	perFileBytes := totalBytes / (*filesCount)

	data := bytes.Repeat(pattern, (perFileBytes/len(pattern))+1)[:perFileBytes]

	runRun := func() time.Duration {
		runtime.GC()
		archiveInstance := seal.NewArchive(seal.WithCompression(codec.Deflate, 6))
		for i := 0; i < *filesCount; i++ {
			archiveInstance.AddBytes(data, fmt.Sprintf("bench_%04d.txt", i))
		}

		start := time.Now()
		_, err := archiveInstance.WriteTo(ctx, io.Discard)
		if err != nil {
			panic(err)
		}
		return time.Since(start)
	}

	// Warmup
	_ = runRun()

	elapsed := runRun()
	throughputMB := float64(*sizeMB) / elapsed.Seconds()

	box := tuikit.NewBox("Benchmark Results", 55).
		SetStyle(tuikit.BorderRounded).
		SetIndent(0).
		AddRow("Payload Size", fmt.Sprintf("%d MB", *sizeMB)).
		AddRow("Entries Count", fmt.Sprintf("%d files", *filesCount)).
		AddRow("Compression Codec", "DEFLATE (Level 6)").
		AddRow("Elapsed Time", fmt.Sprintf("%v", elapsed)).
		AddRow("Throughput", tuikit.Bold(fmt.Sprintf("%.2f MB/s", throughputMB)))

	return box.Render(stdout)
}
