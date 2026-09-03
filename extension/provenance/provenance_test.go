// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provenance_test

import (
	"testing"
	"time"

	"github.com/lemon4ksan/seal/extension"
	"github.com/lemon4ksan/seal/extension/provenance"
)

func TestProvenance_Roundtrip(t *testing.T) {
	t.Parallel()

	origInfo := provenance.Info{
		Commit:    "abcdef0123456789",
		Branch:    "main",
		Tag:       "v1.0.0",
		RepoURL:   "https://github.com/lemon4ksan/seal",
		BuildTime: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC),
		GoVersion: "go1.27.0",
		OS:        "linux",
		Arch:      "amd64",
		Builder:   "CI-Runner-01",
	}

	p := provenance.New(origInfo)
	payload, err := p.OnWrite(t.Context(), extension.ArchiveInfo{})
	if err != nil {
		t.Fatalf("OnWrite failed: %v", err)
	}

	readProv := provenance.New(provenance.Info{})
	if err := readProv.OnRead(t.Context(), payload, extension.ArchiveInfo{}); err != nil {
		t.Fatalf("OnRead failed: %v", err)
	}

	info := readProv.Info()
	if info.Commit != origInfo.Commit || info.Branch != origInfo.Branch || info.RepoURL != origInfo.RepoURL {
		t.Fatalf("Provenance info mismatch: got %+v, want %+v", info, origInfo)
	}
}

func TestProvenance_Detect(t *testing.T) {
	t.Parallel()

	p := provenance.Detect(t.Context())
	info := p.Info()
	if info.GoVersion == "" || info.OS == "" || info.Arch == "" {
		t.Fatalf("Expected GoVersion, OS, and Arch to be detected: %+v", info)
	}
}
