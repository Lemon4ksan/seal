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
	_ "github.com/lemon4ksan/seal/container/ar"
)

func TestAR_EndToEnd_Integration(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(srcDir, "debian-binary"), []byte("2.0\n"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "control.tar.gz"), []byte("MOCK_CONTROL_TAR_GZ"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "data.tar.xz"), []byte("MOCK_DATA_TAR_XZ"), 0o644)

	outDir := t.TempDir()
	debPath := filepath.Join(outDir, "package_1.0.0_amd64.deb")

	// 1. ArchiveDir with container.Ar
	err := seal.ArchiveDir(
		t.Context(),
		srcDir,
		seal.ToFilePath(debPath),
		seal.WithContainer(container.Ar),
	)
	if err != nil {
		t.Fatalf("ArchiveDir failed: %v", err)
	}

	// 2. Inspect with seal.List
	entries, err := seal.List(t.Context(), seal.FromFilePath(debPath))
	if err != nil {
		t.Fatalf("seal.List failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries in AR catalog, got %d", len(entries))
	}

	// 3. Check file presence with seal.ExistsIn
	exists, err := seal.ExistsIn(t.Context(), seal.FromFilePath(debPath), "debian-binary")
	if err != nil || !exists {
		t.Fatalf("ExistsIn debian-binary failed: exists=%v, err=%v", exists, err)
	}

	exists, err = seal.ExistsIn(t.Context(), seal.FromFilePath(debPath), "data.tar.xz")
	if err != nil || !exists {
		t.Fatalf("ExistsIn data.tar.xz failed: exists=%v, err=%v", exists, err)
	}

	// 4. Test tree rendering
	tree, err := seal.Tree(t.Context(), seal.FromFilePath(debPath))
	if err != nil {
		t.Fatalf("seal.Tree failed: %v", err)
	}
	if tree == "" {
		t.Fatalf("seal.Tree returned empty string")
	}

	// 5. Extract with seal.Unarchive
	extractDir := t.TempDir()
	if err := seal.Unarchive(t.Context(), seal.FromFilePath(debPath), extractDir); err != nil {
		t.Fatalf("Unarchive failed: %v", err)
	}

	binData, err := os.ReadFile(filepath.Join(extractDir, "debian-binary"))
	if err != nil || string(binData) != "2.0\n" {
		t.Fatalf("Extracted debian-binary mismatch: %v, data=%q", err, string(binData))
	}

	ctrlData, err := os.ReadFile(filepath.Join(extractDir, "control.tar.gz"))
	if err != nil || string(ctrlData) != "MOCK_CONTROL_TAR_GZ" {
		t.Fatalf("Extracted control.tar.gz mismatch: %v, data=%q", err, string(ctrlData))
	}

	dataData, err := os.ReadFile(filepath.Join(extractDir, "data.tar.xz"))
	if err != nil || string(dataData) != "MOCK_DATA_TAR_XZ" {
		t.Fatalf("Extracted data.tar.xz mismatch: %v, data=%q", err, string(dataData))
	}
}
