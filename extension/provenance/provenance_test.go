// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provenance_test

import (
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

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
	require.NoError(t, err)

	readProv := provenance.New(provenance.Info{})
	err = readProv.OnRead(t.Context(), payload, extension.ArchiveInfo{})
	require.NoError(t, err)

	info := readProv.Info()
	assert.Equal(t, origInfo.Commit, info.Commit)
	assert.Equal(t, origInfo.Branch, info.Branch)
	assert.Equal(t, origInfo.RepoURL, info.RepoURL)
}

func TestProvenance_Detect(t *testing.T) {
	t.Parallel()

	p := provenance.Detect(t.Context())
	info := p.Info()
	assert.NotEmpty(t, info.GoVersion)
	assert.NotEmpty(t, info.OS)
	assert.NotEmpty(t, info.Arch)
}
