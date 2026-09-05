// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

func TestContainer_MultiVaultEndToEnd(t *testing.T) {
	var buf bytes.Buffer

	// 1. Create a container with 2 independent vaults
	w := engine.NewWriter(&buf,
		engine.WithKDFParams(1024, 1, 1), // fast for testing
		engine.WithCipher(spec.CipherAES256GCM),
	)

	if err := w.AddVault("personal", "PassPersonal123!"); err != nil {
		t.Fatalf("AddVault personal: %v", err)
	}
	if err := w.AddVault("work", "PassWork456!"); err != nil {
		t.Fatalf("AddVault work: %v", err)
	}

	// Add files to personal vault
	if err := w.AddVaultFile("personal", "diary.txt", []byte("Dear diary, today was awesome.")); err != nil {
		t.Fatalf("AddVaultFile diary: %v", err)
	}
	// Add shared file to personal vault
	sharedContent := []byte("# Project Readme\nShared across personal and work.")
	if err := w.AddVaultFile("personal", "readme.md", sharedContent); err != nil {
		t.Fatalf("AddVaultFile readme (personal): %v", err)
	}

	// Add files to work vault
	if err := w.AddVaultFile("work", "project.go", []byte("package main\n\nfunc main() {}\n")); err != nil {
		t.Fatalf("AddVaultFile project: %v", err)
	}
	// Add identical shared file to work vault (tests deduplication!)
	if err := w.AddVaultFile("work", "readme.md", sharedContent); err != nil {
		t.Fatalf("AddVaultFile readme (work): %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Writer.Close: %v", err)
	}

	containerBytes := buf.Bytes()
	rAt := bytes.NewReader(containerBytes)
	containerSize := int64(len(containerBytes))

	// 2. Open with personal password
	rPersonal, err := engine.OpenReader(rAt, containerSize, engine.WithReaderPassword("PassPersonal123!"))
	if err != nil {
		t.Fatalf("OpenReader (personal): %v", err)
	}
	if rPersonal.ActiveVault() != "personal" {
		t.Errorf("expected active vault 'personal', got %q", rPersonal.ActiveVault())
	}
	if _, ok := rPersonal.FindExtent(spec.NamespaceVFS, "diary.txt"); !ok {
		t.Errorf("diary.txt should be visible in personal vault")
	}
	if _, ok := rPersonal.FindExtent(spec.NamespaceVFS, "readme.md"); !ok {
		t.Errorf("readme.md should be visible in personal vault")
	}
	if _, ok := rPersonal.FindExtent(spec.NamespaceVFS, "project.go"); ok {
		t.Errorf("SECURITY LEAK: project.go from work vault MUST NOT be visible in personal vault!")
	}
	diaryData, err := rPersonal.ReadExtent("diary.txt")
	if err != nil || string(diaryData) != "Dear diary, today was awesome." {
		t.Errorf("unexpected diary content: %s, err: %v", diaryData, err)
	}

	// 3. Open with work password
	rWork, err := engine.OpenReader(rAt, containerSize, engine.WithReaderPassword("PassWork456!"))
	if err != nil {
		t.Fatalf("OpenReader (work): %v", err)
	}
	if rWork.ActiveVault() != "work" {
		t.Errorf("expected active vault 'work', got %q", rWork.ActiveVault())
	}
	if _, ok := rWork.FindExtent(spec.NamespaceVFS, "project.go"); !ok {
		t.Errorf("project.go should be visible in work vault")
	}
	if _, ok := rWork.FindExtent(spec.NamespaceVFS, "readme.md"); !ok {
		t.Errorf("readme.md should be visible in work vault")
	}
	if _, ok := rWork.FindExtent(spec.NamespaceVFS, "diary.txt"); ok {
		t.Errorf("SECURITY LEAK: diary.txt from personal vault MUST NOT be visible in work vault!")
	}
	projData, err := rWork.ReadExtent("project.go")
	if err != nil || string(projData) != "package main\n\nfunc main() {}\n" {
		t.Errorf("unexpected project content: %s, err: %v", projData, err)
	}

	// 4. Verify Deduplication across vaults
	extPersonalReadme, _ := rPersonal.FindExtent(spec.NamespaceVFS, "readme.md")
	extWorkReadme, _ := rWork.FindExtent(spec.NamespaceVFS, "readme.md")
	if extPersonalReadme.ChunkIndex != extWorkReadme.ChunkIndex || extPersonalReadme.ChunkOffset != extWorkReadme.ChunkOffset {
		t.Errorf("expected readme.md to share chunk coordinates: personal=(#%d, off=%d), work=(#%d, off=%d)",
			extPersonalReadme.ChunkIndex, extPersonalReadme.ChunkOffset, extWorkReadme.ChunkIndex, extWorkReadme.ChunkOffset)
	}

	// 5. Open with BOTH passwords (Union View A U B)
	rUnion, err := engine.OpenReader(rAt, containerSize, engine.WithReaderPasswords("PassPersonal123!", "PassWork456!"))
	if err != nil {
		t.Fatalf("OpenReader (union): %v", err)
	}
	if len(rUnion.UnlockedVaults()) != 2 {
		t.Errorf("expected 2 unlocked vaults, got %d", len(rUnion.UnlockedVaults()))
	}
	// All 3 files must be visible
	if _, ok := rUnion.FindExtent(spec.NamespaceVFS, "diary.txt"); !ok {
		t.Errorf("diary.txt should be visible in union")
	}
	if _, ok := rUnion.FindExtent(spec.NamespaceVFS, "project.go"); !ok {
		t.Errorf("project.go should be visible in union")
	}
	if _, ok := rUnion.FindExtent(spec.NamespaceVFS, "readme.md"); !ok {
		t.Errorf("readme.md should be visible in union")
	}
	if len(rUnion.Extents()) != 3 {
		t.Errorf("expected 3 deduplicated extents in union, got %d", len(rUnion.Extents()))
	}

	// 6. Test wrong password
	_, err = engine.OpenReader(rAt, containerSize, engine.WithReaderPassword("WrongPassword123!"))
	if !errors.Is(err, spec.ErrInvalidPassword) {
		t.Errorf("expected ErrInvalidPassword, got %v", err)
	}
}
