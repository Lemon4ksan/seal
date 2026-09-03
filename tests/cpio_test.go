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
	_ "github.com/lemon4ksan/seal/container/cpio"
)

func TestCPIO_EndToEnd_Integration(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(srcDir, "bin"), 0o755)
	_ = os.MkdirAll(filepath.Join(srcDir, "lib"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "init"), []byte("#!/bin/sh\nexec /bin/busybox\n"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "bin", "busybox"), []byte("BUSYBOX MULTI-CALL BINARY"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "lib", "ld-linux.so.2"), []byte("ELF DYNAMIC LINKER"), 0o755)

	outDir := t.TempDir()
	cpioPath := filepath.Join(outDir, "initramfs.cpio")

	// 1. ArchiveDir with container.Cpio
	err := seal.ArchiveDir(
		t.Context(),
		srcDir,
		seal.ToFilePath(cpioPath),
		seal.WithContainer(container.Cpio),
	)
	if err != nil {
		t.Fatalf("ArchiveDir failed: %v", err)
	}

	// 2. Inspect with seal.List
	entries, err := seal.List(t.Context(), seal.FromFilePath(cpioPath))
	if err != nil {
		t.Fatalf("seal.List failed: %v", err)
	}
	if len(entries) < 3 {
		t.Fatalf("Expected at least 3 entries in CPIO catalog, got %d", len(entries))
	}

	// 3. Check file presence with seal.ExistsIn
	exists, err := seal.ExistsIn(t.Context(), seal.FromFilePath(cpioPath), "init")
	if err != nil || !exists {
		t.Fatalf("ExistsIn init failed: exists=%v, err=%v", exists, err)
	}

	exists, err = seal.ExistsIn(t.Context(), seal.FromFilePath(cpioPath), "bin/busybox")
	if err != nil || !exists {
		t.Fatalf("ExistsIn bin/busybox failed: exists=%v, err=%v", exists, err)
	}

	// 4. Test tree rendering
	tree, err := seal.Tree(t.Context(), seal.FromFilePath(cpioPath))
	if err != nil {
		t.Fatalf("seal.Tree failed: %v", err)
	}
	if tree == "" {
		t.Fatalf("seal.Tree returned empty string")
	}

	// 5. Extract with seal.Unarchive
	extractDir := t.TempDir()
	if err := seal.Unarchive(t.Context(), seal.FromFilePath(cpioPath), extractDir); err != nil {
		t.Fatalf("Unarchive failed: %v", err)
	}

	initData, err := os.ReadFile(filepath.Join(extractDir, "init"))
	if err != nil || string(initData) != "#!/bin/sh\nexec /bin/busybox\n" {
		t.Fatalf("Extracted init mismatch: %v, data=%q", err, string(initData))
	}

	boxData, err := os.ReadFile(filepath.Join(extractDir, "bin", "busybox"))
	if err != nil || string(boxData) != "BUSYBOX MULTI-CALL BINARY" {
		t.Fatalf("Extracted busybox mismatch: %v, data=%q", err, string(boxData))
	}

	ldData, err := os.ReadFile(filepath.Join(extractDir, "lib", "ld-linux.so.2"))
	if err != nil || string(ldData) != "ELF DYNAMIC LINKER" {
		t.Fatalf("Extracted ld-linux mismatch: %v, data=%q", err, string(ldData))
	}
}
