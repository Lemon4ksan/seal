// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/option"
)

func TestArchiver_ArchiveDirAndUnarchive_ZIP(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	destZip := filepath.Join(tempDir, "bundle.zip")
	outDir := filepath.Join(tempDir, "extracted")

	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("Hello Seal!"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(srcDir, "nested", "doc.md"),
		[]byte("# Seal Documentation"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Create initial archive
	if err := seal.ArchiveDir(ctx, srcDir, seal.ToFilePath(destZip)); err != nil {
		t.Fatalf("ArchiveDir failed: %v", err)
	}

	// Verify catalog metadata
	count, err := seal.FileCount(ctx, seal.FromFilePath(destZip))
	if err != nil {
		t.Fatalf("FileCount failed: %v", err)
	}
	if count < 2 {
		t.Fatalf("expected at least 2 files, got %d", count)
	}

	exists, err := seal.ExistsIn(ctx, seal.FromFilePath(destZip), "hello.txt")
	if err != nil || !exists {
		t.Fatalf("ExistsIn hello.txt failed: exists=%v, err=%v", exists, err)
	}

	// Stream payload without unpacking container
	data, err := seal.ReadFile(ctx, seal.FromFilePath(destZip), "hello.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "Hello Seal!" {
		t.Errorf("ReadFile content mismatch: got %q", string(data))
	}

	// Extract individual file entry
	extractedSingle := filepath.Join(tempDir, "single_hello.txt")
	if err := seal.ExtractFile(ctx, seal.FromFilePath(destZip), "hello.txt", extractedSingle); err != nil {
		t.Fatalf("ExtractFile failed: %v", err)
	}
	singleData, err := os.ReadFile(extractedSingle)
	if err != nil || string(singleData) != "Hello Seal!" {
		t.Fatalf("single file content mismatch: got %q", string(singleData))
	}

	// Extract complete filesystem tree
	if err := seal.Unarchive(ctx, seal.FromFilePath(destZip), outDir); err != nil {
		t.Fatalf("Unarchive failed: %v", err)
	}

	extractedDoc, err := os.ReadFile(filepath.Join(outDir, "nested", "doc.md"))
	if err != nil {
		t.Fatalf("failed reading extracted file: %v", err)
	}
	if string(extractedDoc) != "# Seal Documentation" {
		t.Errorf("extracted content mismatch: got %q", string(extractedDoc))
	}

	// Validate container checksums and structural integrity
	if err := seal.Verify(ctx, seal.FromFilePath(destZip)); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	// Generate ASCII directory visualization
	tree, err := seal.Tree(ctx, seal.FromFilePath(destZip))
	if err != nil || tree == "" {
		t.Fatalf("Tree failed: %v", err)
	}
}

func TestArchiver_ArchiveDirAndUnarchive_TAR(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src_tar")
	destTar := filepath.Join(tempDir, "bundle.tar")
	outDir := filepath.Join(tempDir, "extracted_tar")

	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "data.bin"), []byte("Tar payload 12345"), 0o644); err != nil {
		t.Fatal(err)
	}

	archiver := seal.NewArchiver(option.WithContainer(container.Tar))
	if err := archiver.ArchiveDir(ctx, srcDir, seal.ToFilePath(destTar)); err != nil {
		t.Fatalf("ArchiveDir (tar) failed: %v", err)
	}

	data, err := seal.ReadFile(ctx, seal.FromFilePath(destTar), "data.bin")
	if err != nil {
		t.Fatalf("ReadFile (tar) failed: %v", err)
	}
	if string(data) != "Tar payload 12345" {
		t.Errorf("Tar content mismatch: got %q", string(data))
	}

	if err := seal.Unarchive(ctx, seal.FromFilePath(destTar), outDir); err != nil {
		t.Fatalf("Unarchive (tar) failed: %v", err)
	}

	outData, err := os.ReadFile(filepath.Join(outDir, "data.bin"))
	if err != nil || string(outData) != "Tar payload 12345" {
		t.Fatalf("Unarchived tar content mismatch: got %q", string(outData))
	}
}

func TestArchiver_ArchiveDirAndUnarchive_TarGz(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src_tgz")
	destTgz := filepath.Join(tempDir, "bundle.tar.gz")
	outDir := filepath.Join(tempDir, "extracted_tgz")

	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(srcDir, "config.yaml"),
		[]byte("env: production\nport: 8080"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	archiver := seal.NewArchiver(option.WithContainer(container.TarGz))
	if err := archiver.ArchiveDir(ctx, srcDir, seal.ToFilePath(destTgz)); err != nil {
		t.Fatalf("ArchiveDir (tar.gz) failed: %v", err)
	}

	data, err := seal.ReadFile(ctx, seal.FromFilePath(destTgz), "config.yaml")
	if err != nil {
		t.Fatalf("ReadFile (tar.gz) failed: %v", err)
	}
	if string(data) != "env: production\nport: 8080" {
		t.Errorf("Tar.gz content mismatch: got %q", string(data))
	}

	if err := seal.Unarchive(ctx, seal.FromFilePath(destTgz), outDir); err != nil {
		t.Fatalf("Unarchive (tar.gz) failed: %v", err)
	}

	outData, err := os.ReadFile(filepath.Join(outDir, "config.yaml"))
	if err != nil || string(outData) != "env: production\nport: 8080" {
		t.Fatalf("Unarchived tar.gz content mismatch: got %q", string(outData))
	}
}

func TestArchiver_ArchiveDirAndUnarchive_SevenZip(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src_7z")
	dest7z := filepath.Join(tempDir, "bundle.7z")
	outDir := filepath.Join(tempDir, "extracted_7z")

	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("Hello 7-Zip Universal!"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(srcDir, "nested", "binary.dat"),
		[]byte("7z Substream payload"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	archiver := seal.NewArchiver(option.WithContainer(container.SevenZip))
	if err := archiver.ArchiveDir(ctx, srcDir, seal.ToFilePath(dest7z)); err != nil {
		t.Fatalf("ArchiveDir (7z) failed: %v", err)
	}

	// Verify direct file payload extraction using container format auto-detection
	data, err := seal.ReadFile(ctx, seal.FromFilePath(dest7z), "hello.txt")
	if err != nil {
		t.Fatalf("ReadFile (7z) failed: %v", err)
	}
	if string(data) != "Hello 7-Zip Universal!" {
		t.Errorf("7z content mismatch: got %q", string(data))
	}

	// Verify recursive extraction of entire 7z directory hierarchy
	if err := seal.Unarchive(ctx, seal.FromFilePath(dest7z), outDir); err != nil {
		t.Fatalf("Unarchive (7z) failed: %v", err)
	}

	outHello, err := os.ReadFile(filepath.Join(outDir, "hello.txt"))
	if err != nil || string(outHello) != "Hello 7-Zip Universal!" {
		t.Fatalf("Unarchived 7z hello.txt mismatch: got %q", string(outHello))
	}

	outBinary, err := os.ReadFile(filepath.Join(outDir, "nested", "binary.dat"))
	if err != nil || string(outBinary) != "7z Substream payload" {
		t.Fatalf("Unarchived 7z nested binary mismatch: got %q", string(outBinary))
	}
}
