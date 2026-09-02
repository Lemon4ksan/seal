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
	"sync/atomic"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/option"
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
	case "create", "a", "c":
		err = runCreate()
	case "extract", "x", "e":
		err = runExtract()
	case "list", "l":
		err = runList()
	case "cat":
		err = runCat()
	case "test", "t":
		err = runTest()
	case "tree":
		err = runTree()
	case "bench":
		err = runBench()
	case "version", "-v", "--version":
		fmt.Printf("seal version %s (%s/%s)\n", seal.Version, runtime.GOOS, runtime.GOARCH)
	case "help", "-h", "--help":
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
	fmt.Println(`🦭 seal - High-Performance Universal Archiving Engine

Usage:
  seal <command> [options] <arguments>

Commands:
  create, a   Create an archive (7z, ZIP, TAR, TAR.GZ, TAR.XZ, TAR.ZST, TAR.BZ2)
  extract, x  Extract an archive to destination directory
  list, l     List all entries and metadata in an archive
  cat         Stream a specific file entry directly to stdout
  test, t     Verify checksum and decompression integrity
  tree        Display visual ASCII directory tree of the archive
  bench       Run compression speed benchmark
  version     Print seal version

Options for 'create':
  -o <path>       Output archive path (default: archive.7z)
  -m <method>     Method: lzma2, ppmd, zstd, deflate, gzip, store
  -l <level>      Compression level 1..9 (default: 6)
  -filter <type>  BCJ filter for executables: bcj (x86), arm, none (default: none)
  -p <password>   Password for encryption (AES-256)
  -c <comment>    Archive comment`)
}

func detectContainerType(path string) container.Type {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".7z"):
		return container.SevenZip
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return container.TarGz
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"):
		return container.TarXz
	case strings.HasSuffix(lower, ".tar.zst"), strings.HasSuffix(lower, ".tzst"):
		return container.TarZst
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"):
		return container.TarBz2
	case strings.HasSuffix(lower, ".tar"):
		return container.Tar
	default:
		return container.Zip
	}
}

func runCreate() error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	outFile := fs.String("o", "archive.7z", "Output archive destination path")
	pwd := fs.String("p", "", "Password for encryption")
	methodStr := fs.String("m", "", "Compression method (lzma2, ppmd, zstd, deflate, gzip, store)")
	filterStr := fs.String("filter", "none", "Bytecode filter: bcj (x86), arm, none")
	level := fs.Int("l", 6, "Compression level (1-9)")
	comment := fs.String("c", "", "Archive comment")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	inputs := fs.Args()

	if len(inputs) == 0 {
		return errors.New("no input files or directories specified")
	}

	cType := detectContainerType(*outFile)

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
		}
	} else {
		// Default method based on container
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

	fmt.Printf("🔍 Scanning and adding inputs to %s (%s)...\n", *outFile, cType)
	var totalFiles int64
	var totalBytes int64

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

	fmt.Printf("⚡ Compressing %d entries (%s uncompressed) with %s (Level %d, Filter: %s)...\n",
		totalFiles, formatBytes(totalBytes), method, *level, *filterStr)

	start := time.Now()

	err := seal.UseSink(seal.ToFilePath(*outFile), func(w io.Writer) error {
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

	fmt.Printf("✓ Archive created successfully in %v (%.2f MB/s)!\n", elapsed, speed)
	fmt.Printf("  • Original Size  : %s\n", formatBytes(totalBytes))
	fmt.Printf("  • Compressed Size: %s (Ratio: %.2f%%)\n", formatBytes(compSize), ratio)
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

	fmt.Printf("📦 Extracting %s to %s...\n", archivePath, *destDir)
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
	err := archiver.UnarchiveWithContext(ctx, seal.FromFilePath(archivePath), *destDir, seal.WithPassword(*pwd))
	if err != nil {
		return err
	}

	fmt.Printf("✓ Extraction completed in %v (%d files extracted)\n", time.Since(start), extractedCount)
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

	fmt.Printf("🔒 Verifying integrity of %s...\n", archivePath)
	start := time.Now()

	archiver := seal.NewArchiver(seal.WithPassword(*pwd))
	err := archiver.Verify(seal.FromFilePath(archivePath), seal.WithPassword(*pwd))
	if err != nil {
		return fmt.Errorf("integrity verification failed: %w", err)
	}

	fmt.Printf("✓ OK. All checksums verified in %v\n", time.Since(start))
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

	archiveInstance := seal.NewArchive()
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	if _, err := archiveInstance.Load(f, stat.Size()); err != nil {
		return err
	}

	fmt.Println(archiveInstance.Tree())
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
		archiveInstance := seal.NewArchive(seal.WithCompression(codec.Deflate, 6))
		for i := 0; i < *filesCount; i++ {
			archiveInstance.AddBytes(data, fmt.Sprintf("bench_%04d.txt", i))
		}

		start := time.Now()
		_, err := archiveInstance.WriteTo(io.Discard)
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
	return fmt.Sprintf("%.2f %cib", float64(b)/float64(div), "KMGTPE"[exp])
}
