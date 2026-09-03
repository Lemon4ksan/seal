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
	_ "github.com/lemon4ksan/seal/container/iso"
)

func TestISO_EndToEnd_Integration(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(srcDir, "boot", "isolinux"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "boot", "isolinux", "isolinux.bin"), []byte("BOOTLOADER BINARY DATA"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "autorun.inf"), []byte("[autorun]\nopen=setup.exe\n"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "setup.exe"), []byte("MZ EXECUTABLE STUB"), 0o755)

	outDir := t.TempDir()
	isoPath := filepath.Join(outDir, "live_system.iso")

	// 1. ArchiveDir with container.Iso
	err := seal.ArchiveDir(
		t.Context(),
		srcDir,
		seal.ToFilePath(isoPath),
		seal.WithContainer(container.Iso),
	)
	if err != nil {
		t.Fatalf("ArchiveDir failed: %v", err)
	}

	// 2. Inspect with seal.List
	entries, err := seal.List(t.Context(), seal.FromFilePath(isoPath))
	if err != nil {
		t.Fatalf("seal.List failed: %v", err)
	}
	if len(entries) < 3 {
		t.Fatalf("Expected at least 3 entries in ISO catalog, got %d", len(entries))
	}

	// 3. Check file presence with seal.ExistsIn
	exists, err := seal.ExistsIn(t.Context(), seal.FromFilePath(isoPath), "autorun.inf")
	if err != nil || !exists {
		t.Fatalf("ExistsIn autorun.inf failed: exists=%v, err=%v", exists, err)
	}

	exists, err = seal.ExistsIn(t.Context(), seal.FromFilePath(isoPath), "boot/isolinux/isolinux.bin")
	if err != nil || !exists {
		t.Fatalf("ExistsIn boot/isolinux/isolinux.bin failed: exists=%v, err=%v", exists, err)
	}

	// 4. Test tree rendering
	tree, err := seal.Tree(t.Context(), seal.FromFilePath(isoPath))
	if err != nil {
		t.Fatalf("seal.Tree failed: %v", err)
	}
	if tree == "" {
		t.Fatalf("seal.Tree returned empty string")
	}

	// 5. Extract with seal.Unarchive
	extractDir := t.TempDir()
	if err := seal.Unarchive(t.Context(), seal.FromFilePath(isoPath), extractDir); err != nil {
		t.Fatalf("Unarchive failed: %v", err)
	}

	infData, err := os.ReadFile(filepath.Join(extractDir, "autorun.inf"))
	if err != nil || string(infData) != "[autorun]\nopen=setup.exe\n" {
		t.Fatalf("Extracted autorun.inf mismatch: %v, data=%q", err, string(infData))
	}

	binData, err := os.ReadFile(filepath.Join(extractDir, "boot", "isolinux", "isolinux.bin"))
	if err != nil || string(binData) != "BOOTLOADER BINARY DATA" {
		t.Fatalf("Extracted isolinux.bin mismatch: %v, data=%q", err, string(binData))
	}
}
