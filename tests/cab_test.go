// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
	_ "github.com/lemon4ksan/seal/container/cab"
)

func TestCAB_EndToEnd_Integration(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(srcDir, "drivers", "net"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "drivers", "net", "adapter.sys"), []byte("SYS DRIVER BINARY CONTENT"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "setup.inf"), []byte("[Version]\nSignature=\"$Windows NT$\"\n"), 0o644)

	outDir := t.TempDir()
	cabPath := filepath.Join(outDir, "driver_package.cab")

	// 1. ArchiveDir with container.Cab
	err := seal.ArchiveDir(
		t.Context(),
		srcDir,
		seal.ToFilePath(cabPath),
		seal.WithContainer(container.Cab),
	)
	if err != nil {
		t.Fatalf("ArchiveDir failed: %v", err)
	}

	// 2. Inspect with seal.List
	entries, err := seal.List(t.Context(), seal.FromFilePath(cabPath))
	if err != nil {
		t.Fatalf("seal.List failed: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("Expected 4 entries in CAB catalog (2 dirs + 2 files), got %d", len(entries))
	}

	// 3. Check file presence with seal.ExistsIn
	exists, err := seal.ExistsIn(t.Context(), seal.FromFilePath(cabPath), "setup.inf")
	if err != nil || !exists {
		t.Fatalf("ExistsIn setup.inf failed: exists=%v, err=%v", exists, err)
	}

	exists, err = seal.ExistsIn(t.Context(), seal.FromFilePath(cabPath), "drivers/net/adapter.sys")
	if err != nil || !exists {
		t.Fatalf("ExistsIn drivers/net/adapter.sys failed: exists=%v, err=%v", exists, err)
	}

	// 4. Test tree rendering
	tree, err := seal.Tree(t.Context(), seal.FromFilePath(cabPath))
	if err != nil {
		t.Fatalf("seal.Tree failed: %v", err)
	}
	if tree == "" {
		t.Fatalf("seal.Tree returned empty string")
	}

	// 5. Extract with seal.Unarchive
	extractDir := t.TempDir()
	if err := seal.Unarchive(t.Context(), seal.FromFilePath(cabPath), extractDir); err != nil {
		t.Fatalf("Unarchive failed: %v", err)
	}

	infData, err := os.ReadFile(filepath.Join(extractDir, "setup.inf"))
	if err != nil || string(infData) != "[Version]\nSignature=\"$Windows NT$\"\n" {
		t.Fatalf("Extracted setup.inf mismatch: %v, data=%q", err, string(infData))
	}

	sysData, err := os.ReadFile(filepath.Join(extractDir, "drivers", "net", "adapter.sys"))
	if err != nil || string(sysData) != "SYS DRIVER BINARY CONTENT" {
		t.Fatalf("Extracted adapter.sys mismatch: %v, data=%q", err, string(sysData))
	}
}
