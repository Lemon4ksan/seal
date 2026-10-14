// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/seal/file"
)

func TestSortByExtension(t *testing.T) {
	names := []string{
		"archive.zip",
		"binary.exe",
		"main.go",
		"index.html",
		"picture.png",
		"deep/nested/code.cpp",
		"readme.md",
		"app.dll",
		"music.wav",
		"document.pdf",
	}

	var files []*file.File
	for _, n := range names {
		f, err := file.New(n, false)
		if err != nil {
			t.Fatalf("failed to create file %s: %v", n, err)
		}
		files = append(files, f)
	}

	// Add a directory entry
	dir, err := file.New("somedir", true)
	if err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	files = append(files, dir)

	sorted := file.SortByExtension(files)

	// Directory must come first
	if !sorted[0].IsDir() {
		t.Fatalf("expected first entry to be directory, got %s", sorted[0].Name())
	}

	// Code files (Group 0) must precede executables (Group 1), docs (Group 2), media (Group 3)
	for i := 1; i < len(sorted)-1; i++ {
		extA := filepath.Ext(sorted[i].Name())
		extB := filepath.Ext(sorted[i+1].Name())
		grpA := file.ExtensionGroup(extA)
		grpB := file.ExtensionGroup(extB)
		if grpA > grpB {
			t.Errorf(
				"group ordering violated: %s (grp %d) after %s (grp %d)",
				sorted[i].Name(),
				grpA,
				sorted[i+1].Name(),
				grpB,
			)
		}
	}
}
