// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/codec"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	os.Args = append([]string{os.Args[0]}, os.Args[2:]...)

	var err error
	switch cmd {
	case "create":
		err = runCreate()
	case "extract":
		err = runExtract()
	case "list":
		err = runList()
	case "cat":
		err = runCat()
	case "test":
		err = runTest()
	case "tree":
		err = runTree()
	case "bench":
		err = runBench()
	case "version":
		fmt.Printf("seal version %s\n", seal.Version)
	case "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "seal: unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "seal error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`seal - High-Performance Universal Archiving Engine

Usage:
  seal <command> [arguments]

Commands:
  create    Create a new archive (ZIP, TAR, TAR.GZ)
  extract   Extract an archive into a destination directory
  list      List all entries and metadata in an archive
  cat       Stream a file entry directly to stdout
  test      Verify checksum and decompression integrity
  tree      Display an ASCII visual directory tree of the archive
  bench     Run compression benchmark (Serial vs Parallel multi-core)
  version   Print seal engine version`)
}

func runCreate() error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	outFile := fs.String("o", "archive.zip", "Output archive destination path")
	pwd := fs.String("p", "", "Password for encryption")
	methodStr := fs.String("m", "deflate", "Compression method (store, deflate, gzip)")
	level := fs.Int("l", 6, "Compression level (0-9)")
	comment := fs.String("c", "", "Archive comment")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	inputs := fs.Args()

	if len(inputs) == 0 {
		return errors.New("no input files or directories specified")
	}

	method := codec.Deflate
	switch strings.ToLower(*methodStr) {
	case "store", "none":
		method = codec.Store
	case "gzip", "gz":
		method = codec.Gzip
	}

	archive := seal.NewArchive(
		seal.WithCompression(method, *level),
		seal.WithPassword(*pwd),
		seal.WithComment(*comment),
	)

	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return err
		}

		if info.IsDir() {
			fmt.Printf("Adding directory: %s\n", input)
			if _, err := archive.AddDir(input); err != nil {
				return err
			}
		} else {
			fmt.Printf("Adding file: %s\n", input)
			if _, err := archive.AddFile(input); err != nil {
				return err
			}
		}
	}

	fmt.Printf("Writing archive to %s...\n", *outFile)
	start := time.Now()

	err := seal.UseSink(seal.ToFilePath(*outFile), func(w io.Writer) error {
		_, err := archive.WriteTo(w)
		return err
	})
	if err != nil {
		_ = os.Remove(*outFile)
		return err
	}

	fmt.Printf("Success! Archive created in %v\n", time.Since(start))
	return nil
}

func runExtract() error {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	destDir := fs.String("d", ".", "Destination extraction directory")
	pwd := fs.String("p", "", "Decryption password")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("archive file not specified")
	}
	archivePath := fs.Arg(0)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Extracting %s to %s...\n", archivePath, *destDir)
	start := time.Now()

	archiver := seal.NewArchiver(seal.WithPassword(*pwd))
	err := archiver.UnarchiveWithContext(ctx, seal.FromFilePath(archivePath), *destDir, seal.WithPassword(*pwd))
	if err != nil {
		return err
	}

	fmt.Printf("Extraction completed in %v\n", time.Since(start))
	return nil
}

func runList() error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	pwd := fs.String("p", "", "Decryption password")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("archive file not specified")
	}
	archivePath := fs.Arg(0)

	archiver := seal.NewArchiver(seal.WithPassword(*pwd))
	entries, err := archiver.GetEntries(seal.FromFilePath(archivePath))
	if err != nil {
		return err
	}

	fmt.Printf("Archive: %s (%d entries)\n\n", archivePath, len(entries))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Mode\tSize\tCompressed\tRatio\tName")
	fmt.Fprintln(w, "----\t----\t----------\t-----\t----")

	var totalUncomp, totalComp int64
	for _, e := range entries {
		totalUncomp += e.Size()
		totalComp += e.CompressedSize()

		ratio := 0.0
		if e.Size() > 0 {
			ratio = float64(e.CompressedSize()) / float64(e.Size()) * 100.0
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%.1f%%\t%s\n",
			e.Mode(),
			formatBytes(e.Size()),
			formatBytes(e.CompressedSize()),
			ratio,
			e.Name(),
		)
	}

	fmt.Fprintln(w, "\t\t\t\t")
	fmt.Fprintf(w, "Total:\t%s\t%s\t\t\n", formatBytes(totalUncomp), formatBytes(totalComp))
	_ = w.Flush()

	return nil
}

func runCat() error {
	fs := flag.NewFlagSet("cat", flag.ExitOnError)
	pwd := fs.String("p", "", "Decryption password")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return errors.New("usage: seal cat <archive> <entry-path>")
	}
	archivePath := fs.Arg(0)
	targetEntry := fs.Arg(1)

	archiver := seal.NewArchiver(seal.WithPassword(*pwd))
	data, err := archiver.ReadFile(seal.FromFilePath(archivePath), targetEntry)
	if err != nil {
		return err
	}

	_, err = os.Stdout.Write(data)
	return err
}

func runTest() error {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	pwd := fs.String("p", "", "Decryption password")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("archive file not specified")
	}
	archivePath := fs.Arg(0)

	fmt.Printf("Verifying integrity of %s...\n", archivePath)
	start := time.Now()

	archiver := seal.NewArchiver(seal.WithPassword(*pwd))
	err := archiver.Verify(seal.FromFilePath(archivePath), seal.WithPassword(*pwd))
	if err != nil {
		return fmt.Errorf("integrity verification failed: %w", err)
	}

	fmt.Printf("OK. All checksums verified in %v\n", time.Since(start))
	return nil
}

func runTree() error {
	fs := flag.NewFlagSet("tree", flag.ExitOnError)
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("archive file not specified")
	}
	archivePath := fs.Arg(0)

	archive := seal.NewArchive()
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	if _, err := archive.Load(f, stat.Size()); err != nil {
		return err
	}

	fmt.Println(archive.Tree())
	return nil
}

func runBench() error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	sizeMB := fs.Int("size", 50, "Total payload size in MB")
	filesCount := fs.Int("files", 100, "Number of generated entries")
	workers := fs.Int("j", runtime.NumCPU(), "Worker concurrency")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	fmt.Printf("=== Seal Silicon Benchmark ===\n")
	fmt.Printf("Payload: %d MB across %d entries, CPU cores: %d\n", *sizeMB, *filesCount, *workers)

	pattern := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Integer nec odio. Praesent libero. ")
	totalBytes := (*sizeMB) * 1024 * 1024
	perFileBytes := totalBytes / (*filesCount)

	data := bytes.Repeat(pattern, (perFileBytes/len(pattern))+1)[:perFileBytes]

	runRun := func() time.Duration {
		runtime.GC()
		archive := seal.NewArchive(seal.WithCompression(codec.Deflate, 6))
		for i := 0; i < *filesCount; i++ {
			archive.AddBytes(data, fmt.Sprintf("bench_%04d.txt", i))
		}

		start := time.Now()
		_, err := archive.WriteTo(io.Discard)
		if err != nil {
			panic(err)
		}
		return time.Since(start)
	}

	// Warmup
	_ = runRun()

	elapsed := runRun()
	throughputMB := float64(*sizeMB) / elapsed.Seconds()
	fmt.Printf("Throughput: %.2f MB/s (%v)\n", throughputMB, elapsed)

	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cib", float64(b)/float64(div), "KMGTPE"[exp])
}
