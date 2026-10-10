// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"testing"
)

func TestParseSevenZipArgs(t *testing.T) {
	// Test create command in 7-zip syntax: seal a archive.eon dir/ file.txt -mx=9 -pSecret -mmt=8 -sdel -y
	args := []string{
		"archive.eon", "dir/", "file.txt",
		"-mx=9", "-pSecret", "-mmt=8", "-sdel", "-y", "-filter=bcj",
	}
	sz, err := ParseSevenZipArgs("a", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sz.Archive != "archive.eon" {
		t.Errorf("expected archive.eon, got %s", sz.Archive)
	}
	if len(sz.Items) != 2 || sz.Items[0] != "dir/" || sz.Items[1] != "file.txt" {
		t.Errorf("expected 2 items, got %v", sz.Items)
	}
	if sz.Level != 9 {
		t.Errorf("expected level 9, got %d", sz.Level)
	}
	if sz.Password != "Secret" {
		t.Errorf("expected password Secret, got %s", sz.Password)
	}
	if sz.Workers != 8 {
		t.Errorf("expected workers 8, got %d", sz.Workers)
	}
	if !sz.DeleteSource {
		t.Errorf("expected DeleteSource true")
	}
	if !sz.AssumeYes {
		t.Errorf("expected AssumeYes true")
	}
	if sz.Filter != "bcj" {
		t.Errorf("expected filter bcj, got %s", sz.Filter)
	}

	// Test extract command in 7-zip syntax: seal x archive.eon "*.go" -oD:\out -aos -x!*.tmp
	xArgs := []string{
		"archive.eon", "*.go", "src/*",
		"-oD:\\out", "-aos", "-x!*.tmp", "-x!test/*",
	}
	szX, err := ParseSevenZipArgs("x", xArgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if szX.Archive != "archive.eon" {
		t.Errorf("expected archive.eon, got %s", szX.Archive)
	}
	if szX.OutputDir != "D:\\out" {
		t.Errorf("expected D:\\out, got %s", szX.OutputDir)
	}
	if szX.Overwrite != OverwriteSkip {
		t.Errorf("expected OverwriteSkip, got %v", szX.Overwrite)
	}
	if len(szX.IncludePatterns) != 2 || szX.IncludePatterns[0] != "*.go" || szX.IncludePatterns[1] != "src/*" {
		t.Errorf("unexpected include patterns: %v", szX.IncludePatterns)
	}
	if len(szX.ExcludePatterns) != 2 || szX.ExcludePatterns[0] != "*.tmp" || szX.ExcludePatterns[1] != "test/*" {
		t.Errorf("unexpected exclude patterns: %v", szX.ExcludePatterns)
	}
}

func TestMatchPattern(t *testing.T) {
	// Base wildcard
	if !MatchPattern("*.go", "main.go", true) {
		t.Errorf("expected *.go to match main.go")
	}
	if !MatchPattern("*.go", "cmd/seal/main.go", true) {
		t.Errorf("expected *.go to match cmd/seal/main.go with recurse=true")
	}
	if MatchPattern("*.go", "cmd/seal/main.go", false) {
		t.Errorf("expected *.go NOT to match subpath with recurse=false")
	}

	// Directory wildcard
	if !MatchPattern("docs/*", "docs/PRZM_SPEC.md", true) {
		t.Errorf("expected docs/* to match docs/PRZM_SPEC.md")
	}
	if !MatchPattern("docs/*", "docs/eon/PRZM_SPEC.md", true) {
		t.Errorf("expected docs/* to match docs/eon/PRZM_SPEC.md")
	}

	// Arbitrary depth
	if !MatchPattern("src/**/*.go", "src/pkg/a/b.go", true) {
		t.Errorf("expected src/**/*.go to match src/pkg/a/b.go")
	}

	// Exclude filter
	if !MatchFilters("docs/README.md", false, []string{"docs/*"}, []string{"*.tmp"}, true) {
		t.Errorf("expected docs/README.md to be included")
	}
	if MatchFilters("docs/test.tmp", false, []string{"docs/*"}, []string{"*.tmp"}, true) {
		t.Errorf("expected docs/test.tmp to be excluded by *.tmp")
	}
}

func TestParseSevenZipArgs_SolidFlags(t *testing.T) {
	tests := []struct {
		args       []string
		wantSolid  bool
		wantSize   int64
		wantExt    bool
		wantExp    bool
	}{
		{args: []string{"a.eon", "f", "-ms"}, wantSolid: true, wantExp: true},
		{args: []string{"a.eon", "f", "-ms=on"}, wantSolid: true, wantExp: true},
		{args: []string{"a.eon", "f", "-ms=off"}, wantSolid: false, wantExp: true},
		{args: []string{"a.eon", "f", "-ms=e"}, wantSolid: true, wantExt: true, wantExp: true},
		{args: []string{"a.eon", "f", "-ms=f"}, wantSolid: true, wantSize: 1 << 30, wantExp: true},
		{args: []string{"a.eon", "f", "-ms=64m"}, wantSolid: true, wantSize: 64 * 1024 * 1024, wantExp: true},
		{args: []string{"a.eon", "f", "-solid"}, wantSolid: true, wantExp: true},
		{args: []string{"a.eon", "f", "-solid=off"}, wantSolid: false, wantExp: true},
		{args: []string{"a.eon", "f", "--solid=256mb"}, wantSolid: true, wantSize: 256 * 1024 * 1024, wantExp: true},
		{args: []string{"a.eon", "f", "--no-solid"}, wantSolid: false, wantExp: true},
	}

	for _, tc := range tests {
		sz, err := ParseSevenZipArgs("a", tc.args)
		if err != nil {
			t.Fatalf("ParseSevenZipArgs failed for %v: %v", tc.args, err)
		}
		if sz.Solid != tc.wantSolid {
			t.Errorf("args %v: Solid got %v, want %v", tc.args, sz.Solid, tc.wantSolid)
		}
		if sz.SolidExplicit != tc.wantExp {
			t.Errorf("args %v: SolidExplicit got %v, want %v", tc.args, sz.SolidExplicit, tc.wantExp)
		}
		if sz.SolidSize != tc.wantSize {
			t.Errorf("args %v: SolidSize got %d, want %d", tc.args, sz.SolidSize, tc.wantSize)
		}
		if sz.SolidExtension != tc.wantExt {
			t.Errorf("args %v: SolidExtension got %v, want %v", tc.args, sz.SolidExtension, tc.wantExt)
		}
	}
}

func TestParseSevenZipArgs_Encoding(t *testing.T) {
	tests := []struct {
		args     []string
		expected string
	}{
		{[]string{"x", "legacy.zip", "-mcp=866"}, "866"},
		{[]string{"x", "legacy.zip", "-mcp:cp866"}, "cp866"},
		{[]string{"x", "legacy.zip", "-mcp", "1251"}, "1251"},
		{[]string{"x", "legacy.zip", "--encoding=shift_jis"}, "shift_jis"},
		{[]string{"x", "legacy.zip", "--encoding", "gbk"}, "gbk"},
		{[]string{"x", "legacy.zip", "-charset=big5"}, "big5"},
	}

	for _, tc := range tests {
		sz, err := ParseSevenZipArgs(tc.args[0], tc.args[1:])
		if err != nil {
			t.Fatalf("ParseSevenZipArgs failed for %v: %v", tc.args, err)
		}
		if sz.Encoding != tc.expected {
			t.Errorf("args %v: expected encoding %q, got %q", tc.args, tc.expected, sz.Encoding)
		}
	}
}

