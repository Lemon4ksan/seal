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

	// 1. Pack directory
	if err := seal.ArchiveDir(srcDir, seal.ToFilePath(destZip)); err != nil {
		t.Fatalf("ArchiveDir failed: %v", err)
	}

	// 2. Count & Exists
	count, err := seal.FileCount(seal.FromFilePath(destZip))
	if err != nil {
		t.Fatalf("FileCount failed: %v", err)
	}
	if count < 2 {
		t.Fatalf("expected at least 2 files, got %d", count)
	}

	exists, err := seal.ExistsIn(seal.FromFilePath(destZip), "hello.txt")
	if err != nil || !exists {
		t.Fatalf("ExistsIn hello.txt failed: exists=%v, err=%v", exists, err)
	}

	// 3. Read single file directly
	data, err := seal.ReadFile(seal.FromFilePath(destZip), "hello.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "Hello Seal!" {
		t.Errorf("ReadFile content mismatch: got %q", string(data))
	}

	// 4. Extract single file
	extractedSingle := filepath.Join(tempDir, "single_hello.txt")
	if err := seal.ExtractFile(seal.FromFilePath(destZip), "hello.txt", extractedSingle); err != nil {
		t.Fatalf("ExtractFile failed: %v", err)
	}
	singleData, err := os.ReadFile(extractedSingle)
	if err != nil || string(singleData) != "Hello Seal!" {
		t.Fatalf("single file content mismatch: got %q", string(singleData))
	}

	// 5. Unarchive all
	if err := seal.Unarchive(seal.FromFilePath(destZip), outDir); err != nil {
		t.Fatalf("Unarchive failed: %v", err)
	}

	extractedDoc, err := os.ReadFile(filepath.Join(outDir, "nested", "doc.md"))
	if err != nil {
		t.Fatalf("failed reading extracted file: %v", err)
	}
	if string(extractedDoc) != "# Seal Documentation" {
		t.Errorf("extracted content mismatch: got %q", string(extractedDoc))
	}

	// 6. Verify integrity
	if err := seal.Verify(seal.FromFilePath(destZip)); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	// 7. Tree rendering
	tree, err := seal.Tree(seal.FromFilePath(destZip))
	if err != nil || tree == "" {
		t.Fatalf("Tree failed: %v", err)
	}
}

func TestArchiver_ArchiveDirAndUnarchive_TAR(t *testing.T) {
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
	if err := archiver.ArchiveDir(srcDir, seal.ToFilePath(destTar)); err != nil {
		t.Fatalf("ArchiveDir (tar) failed: %v", err)
	}

	data, err := seal.ReadFile(seal.FromFilePath(destTar), "data.bin")
	if err != nil {
		t.Fatalf("ReadFile (tar) failed: %v", err)
	}
	if string(data) != "Tar payload 12345" {
		t.Errorf("Tar content mismatch: got %q", string(data))
	}

	if err := seal.Unarchive(seal.FromFilePath(destTar), outDir); err != nil {
		t.Fatalf("Unarchive (tar) failed: %v", err)
	}

	outData, err := os.ReadFile(filepath.Join(outDir, "data.bin"))
	if err != nil || string(outData) != "Tar payload 12345" {
		t.Fatalf("Unarchived tar content mismatch: got %q", string(outData))
	}
}

func TestArchiver_ArchiveDirAndUnarchive_TarGz(t *testing.T) {
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
	if err := archiver.ArchiveDir(srcDir, seal.ToFilePath(destTgz)); err != nil {
		t.Fatalf("ArchiveDir (tar.gz) failed: %v", err)
	}

	data, err := seal.ReadFile(seal.FromFilePath(destTgz), "config.yaml")
	if err != nil {
		t.Fatalf("ReadFile (tar.gz) failed: %v", err)
	}
	if string(data) != "env: production\nport: 8080" {
		t.Errorf("Tar.gz content mismatch: got %q", string(data))
	}

	if err := seal.Unarchive(seal.FromFilePath(destTgz), outDir); err != nil {
		t.Fatalf("Unarchive (tar.gz) failed: %v", err)
	}

	outData, err := os.ReadFile(filepath.Join(outDir, "config.yaml"))
	if err != nil || string(outData) != "env: production\nport: 8080" {
		t.Fatalf("Unarchived tar.gz content mismatch: got %q", string(outData))
	}
}
