// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/forensics"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

func TestSevenZipCLIEndToEnd(t *testing.T) {
	tempDir := t.TempDir()

	// Prepare test file hierarchy
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("Hello World 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file2.log"), []byte("Log Data 2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "file3.txt"), []byte("Nested Text 3"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "ignore.tmp"), []byte("Temporary data"), 0o644); err != nil {
		t.Fatal(err)
	}

	arcPath := filepath.Join(tempDir, "test.eon")

	// 1. Test 'a' (create) with 7-zip syntax: seal a <archive> <src> -mx=9 -pSecret -x!*.tmp
	createCmd := &CreateCmd{}
	var stdout bytes.Buffer
	ctx := context.Background()

	createArgs := []string{
		arcPath, srcDir,
		"-mx=9", "-pSecret", "-mmt=4", "-x!*.tmp",
	}
	if err := createCmd.Run(ctx, createArgs, &stdout, ioDiscard{}); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if _, err := os.Stat(arcPath); err != nil {
		t.Fatalf("archive was not created: %v", err)
	}

	// 2. Test 'l' (list) with password and pattern filter: seal l <archive> "*.txt" -pSecret
	listCmd := &ListCmd{}
	stdout.Reset()
	listArgs := []string{
		arcPath, "*.txt", "-pSecret",
	}
	if err := listCmd.Run(ctx, listArgs, &stdout, ioDiscard{}); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	outStr := stdout.String()
	if !bytes.Contains(stdout.Bytes(), []byte("file1.txt")) {
		t.Errorf("list output missing file1.txt:\n%s", outStr)
	}
	if bytes.Contains(stdout.Bytes(), []byte("file2.log")) {
		t.Errorf("list output should not contain file2.log due to *.txt filter:\n%s", outStr)
	}

	// 3. Test 't' (test integrity): seal t <archive> -pSecret
	testCmd := &forensics.TestCmd{}
	stdout.Reset()
	testArgs := []string{
		arcPath, "-pSecret",
	}
	if err := testCmd.Run(ctx, testArgs, &stdout, ioDiscard{}); err != nil {
		t.Fatalf("integrity test failed: %v", err)
	}

	// 4. Test 'x' (extract full paths): selective extract of only *.txt files
	extractDir := filepath.Join(tempDir, "out_x")
	extractCmd := &ExtractCmd{}
	stdout.Reset()
	extractArgs := []string{
		arcPath, "*.txt", "-o" + extractDir, "-pSecret",
	}
	if err := extractCmd.Run(ctx, extractArgs, &stdout, ioDiscard{}); err != nil {
		t.Fatalf("extract full failed: %v", err)
	}
	// Check that file1.txt and sub/file3.txt exist, but not file2.log
	if _, err := os.Stat(filepath.Join(extractDir, "file1.txt")); err != nil {
		t.Errorf("expected file1.txt in %s: %v", extractDir, err)
	}
	if _, err := os.Stat(filepath.Join(extractDir, "sub", "file3.txt")); err != nil {
		t.Errorf("expected sub/file3.txt in %s: %v", extractDir, err)
	}
	if _, err := os.Stat(filepath.Join(extractDir, "file2.log")); err == nil {
		t.Errorf("file2.log should not have been extracted")
	}

	// 5. Test 'e' (flat extract): all files extracted directly into destination directory
	flatDir := filepath.Join(tempDir, "out_flat")
	stdout.Reset()
	// Simulate calling 'e' by passing -d and checking flat behavior
	os.Args = []string{"seal", "e"}
	flatArgs := []string{
		arcPath, "-o" + flatDir, "-pSecret",
	}
	if err := extractCmd.Run(ctx, flatArgs, &stdout, ioDiscard{}); err != nil {
		t.Fatalf("flat extract failed: %v", err)
	}
	// In flat mode, sub/file3.txt should be extracted directly as out_flat/file3.txt!
	if _, err := os.Stat(filepath.Join(flatDir, "file3.txt")); err != nil {
		t.Errorf("flat extract did not place file3.txt at root of output dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(flatDir, "sub")); err == nil {
		t.Errorf("flat extract should not have created directory 'sub'")
	}

	// 6. Test 'd' (delete): delete file2.log from the archive
	deleteCmd := &DeleteCmd{}
	stdout.Reset()
	deleteArgs := []string{
		arcPath, "*.log", "-pSecret",
	}
	if err := deleteCmd.Run(ctx, deleteArgs, &stdout, ioDiscard{}); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// Verify file2.log is gone from the archive via list
	stdout.Reset()
	if err := listCmd.Run(ctx, []string{arcPath, "-pSecret"}, &stdout, ioDiscard{}); err != nil {
		t.Fatalf("list after delete failed: %v", err)
	}
	if bytes.Contains(stdout.Bytes(), []byte("file2.log")) {
		t.Errorf("file2.log was not deleted from archive:\n%s", stdout.String())
	}
}

func TestContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create several files
	for i := 0; i < 50; i++ {
		name := filepath.Join(srcDir, "file"+string(rune('A'+(i%26)))+".bin")
		_ = os.WriteFile(name, bytes.Repeat([]byte{byte(i)}, 64*1024), 0o644)
	}

	arcPath := filepath.Join(tempDir, "cancelled.eon")
	createCmd := &CreateCmd{}
	var stdout bytes.Buffer

	// Pre-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	createArgs := []string{arcPath, srcDir}
	err := createCmd.Run(ctx, createArgs, &stdout, ioDiscard{})
	if err == nil {
		t.Fatalf("expected error on cancelled context, got nil")
	}

	// Archive file should have been cleaned up if creation was cancelled
	if _, err := os.Stat(arcPath); err == nil {
		t.Errorf("cancelled archive file should be removed from disk on context cancellation")
	}
}

func TestOverwritePolicies(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(srcDir, "target.txt")
	if err := os.WriteFile(testFile, []byte("ARCHIVE_DATA"), 0o644); err != nil {
		t.Fatal(err)
	}

	arcPath := filepath.Join(tempDir, "policy_test.eon")
	createCmd := &CreateCmd{}
	ctx := context.Background()
	if err := createCmd.Run(ctx, []string{arcPath, srcDir}, ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	extractDir := filepath.Join(tempDir, "out")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	existingTarget := filepath.Join(extractDir, "target.txt")
	extractCmd := &ExtractCmd{}

	// Case 1: -aos (skip)
	_ = os.WriteFile(existingTarget, []byte("ORIGINAL_LOCAL_DATA"), 0o644)
	os.Args = []string{"seal", "e"}
	if err := extractCmd.Run(ctx, []string{arcPath, "-o" + extractDir, "-aos"}, ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatalf("extract with -aos failed: %v", err)
	}
	content, _ := os.ReadFile(existingTarget)
	if string(content) != "ORIGINAL_LOCAL_DATA" {
		t.Errorf("expected -aos to preserve original file, got: %s", string(content))
	}

	// Case 2: -aou (rename new)
	if err := extractCmd.Run(ctx, []string{arcPath, "-o" + extractDir, "-aou"}, ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatalf("extract with -aou failed: %v", err)
	}
	renamedTarget := filepath.Join(extractDir, "target_1.txt")
	if _, err := os.Stat(renamedTarget); err != nil {
		t.Fatalf("expected target_1.txt to be created under -aou: %v", err)
	}
	renamedContent, _ := os.ReadFile(renamedTarget)
	if string(renamedContent) != "ARCHIVE_DATA" {
		t.Errorf("expected target_1.txt to have ARCHIVE_DATA, got: %s", string(renamedContent))
	}

	// Case 3: -aoa (overwrite all)
	if err := extractCmd.Run(ctx, []string{arcPath, "-o" + extractDir, "-aoa"}, ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatalf("extract with -aoa failed: %v", err)
	}
	contentOverwritten, _ := os.ReadFile(existingTarget)
	if string(contentOverwritten) != "ARCHIVE_DATA" {
		t.Errorf("expected -aoa to overwrite file with ARCHIVE_DATA, got: %s", string(contentOverwritten))
	}
}

func TestSkippingLockedFiles(t *testing.T) {
	tempDir := t.TempDir()
	arcPath := filepath.Join(tempDir, "test_skip.eon")

	arc := seal.NewArchive(seal.WithContainer(container.Eon))

	// Add a good file
	fGood, err := file.New("good.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	fGood.SetSizes(5, 0, 0)
	fGood.SetOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("HELLO"))), nil
	})
	arc.Add(fGood)

	// Add a locked file whose OpenFunc fails with sharing violation
	fLocked, err := file.New("locked.nvph", false)
	if err != nil {
		t.Fatal(err)
	}
	fLocked.SetSizes(1024, 0, 0)
	fLocked.SetOpenFunc(func() (io.ReadCloser, error) {
		return nil, errors.New("The process cannot access the file because it is being used by another process.")
	})
	arc.Add(fLocked)

	outF, err := os.Create(arcPath)
	if err != nil {
		t.Fatal(err)
	}
	defer outF.Close()

	var skippedCount int
	_, err = arc.WriteTo(context.Background(), outF, seal.WithOnFileDone(func(name string, err error) {
		if err != nil {
			skippedCount++
		}
	}))
	if err != nil {
		t.Fatalf("expected archive creation to succeed despite locked file, got error: %v", err)
	}
	if skippedCount != 1 {
		t.Errorf("expected 1 file to be reported as skipped/errored, got %d", skippedCount)
	}

	// Verify the archive only has good.txt and is 100% valid
	entries, err := seal.NewArchiver().GetEntries(context.Background(), seal.FromFilePath(arcPath))
	if err != nil {
		t.Fatalf("failed to read created archive: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "good.txt" {
		t.Errorf("expected only good.txt in archive, got: %v", entries)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
