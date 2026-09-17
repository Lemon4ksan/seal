// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/extension"
	"github.com/lemon4ksan/seal/extension/provenance"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/option"
)

type TestConfig struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Tags    []string `json:"tags"`
}

func TestGenericReadToAndReadInto(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test_generic.zip")

	// 1. Build archive with json payload
	cfgExpected := TestConfig{
		Name:    "seal-core",
		Version: "1.0.0",
		Tags:    []string{"archive", "fast", "generic"},
	}
	cfgBytes, err := json.Marshal(cfgExpected)
	require.NoError(t, err)

	arc := seal.NewArchive()
	arc.AddBytes(cfgBytes, "config.json")
	arc.AddString("plain text content", "hello.txt")

	outF, err := os.Create(archivePath)
	require.NoError(t, err)
	_, err = arc.WriteTo(ctx, outF)
	require.NoError(t, err)
	require.NoError(t, outF.Close())

	// 2. Test seal.ReadTo[TestConfig]
	gotCfg, err := seal.ReadTo[TestConfig](ctx, seal.FromFilePath(archivePath), "config.json")
	require.NoError(t, err)
	require.NotNil(t, gotCfg)
	assert.Equal(t, cfgExpected.Name, gotCfg.Name)
	assert.Equal(t, cfgExpected.Version, gotCfg.Version)
	assert.Equal(t, cfgExpected.Tags, gotCfg.Tags)

	// 3. Test seal.ReadInto[TestConfig]
	var targetCfg TestConfig
	err = seal.ReadInto(ctx, seal.FromFilePath(archivePath), "config.json", &targetCfg)
	require.NoError(t, err)
	assert.Equal(t, cfgExpected.Name, targetCfg.Name)
	assert.Equal(t, cfgExpected.Version, targetCfg.Version)

	// 4. Test Archiver.ReadTo and ReadInto
	archiver := seal.NewArchiver()
	archiverCfg, err := archiver.ReadTo[TestConfig](ctx, seal.FromFilePath(archivePath), "config.json")
	require.NoError(t, err)
	assert.Equal(t, cfgExpected.Name, archiverCfg.Name)

	var archiverTarget TestConfig
	err = archiver.ReadInto(ctx, seal.FromFilePath(archivePath), "config.json", &archiverTarget)
	require.NoError(t, err)
	assert.Equal(t, cfgExpected.Version, archiverTarget.Version)

	// 5. Test File.ReadTo and ReadInto
	entries, err := seal.List(ctx, seal.FromFilePath(archivePath))
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	cfgFile, ok := arc.Find(func(f *file.File) bool {
		return f.Name() == "config.json"
	})
	require.True(t, ok)
	require.NotNil(t, cfgFile)

	fileCfg, err := cfgFile.ReadTo[TestConfig]()
	require.NoError(t, err)
	assert.Equal(t, cfgExpected.Name, fileCfg.Name)

	var fileTarget TestConfig
	err = cfgFile.ReadInto(&fileTarget)
	require.NoError(t, err)
	assert.Equal(t, cfgExpected.Name, fileTarget.Name)

	// 6. Test Archive.ReadTo and Archive.ReadInto
	arcCfg, err := arc.ReadTo[TestConfig]("config.json")
	require.NoError(t, err)
	assert.Equal(t, cfgExpected.Name, arcCfg.Name)

	var arcTarget TestConfig
	err = arc.ReadInto("config.json", &arcTarget)
	require.NoError(t, err)
	assert.Equal(t, cfgExpected.Name, arcTarget.Name)

	// 7. Test missing file errors
	_, err = seal.ReadTo[TestConfig](ctx, seal.FromFilePath(archivePath), "nonexistent.json")
	require.Error(t, err)

	_, err = arc.ReadTo[TestConfig]("nonexistent.json")
	require.Error(t, err)
}

func TestGenericArchiveFindAndFilter(t *testing.T) {
	arc := seal.NewArchive()
	arc.AddString("content 1", "main.go")
	arc.AddString("content 2", "readme.md")
	arc.AddString("content 3", "util.go")
	arc.AddString("content 4", "data.json")

	// Test Find
	found, ok := arc.Find(func(f *file.File) bool {
		return strings.HasSuffix(f.Name(), ".md")
	})
	require.True(t, ok)
	require.NotNil(t, found)
	assert.Equal(t, "readme.md", found.Name())

	notFound, ok := arc.Find(func(f *file.File) bool {
		return f.Name() == "unknown.txt"
	})
	assert.False(t, ok)
	assert.Nil(t, notFound)

	// Test FindAll (non-mutating filter)
	goFiles := arc.FindAll(func(f *file.File) bool {
		return strings.HasSuffix(f.Name(), ".go")
	})
	require.Equal(t, 2, len(goFiles))
	assert.Equal(t, "main.go", goFiles[0].Name())
	assert.Equal(t, "util.go", goFiles[1].Name())
}

func TestGenericGetExtensionTo(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test_provenance.zip")

	// Create archive with provenance extension
	prov := provenance.New(provenance.Info{
		Commit:  "abc1234",
		Branch:  "feature/generic",
		Builder: "SealCI",
	})

	arc := seal.NewArchive(seal.WithExtension(prov))
	arc.AddString("hello", "file.txt")

	outF, err := os.Create(archivePath)
	require.NoError(t, err)
	_, err = arc.WriteTo(ctx, outF, seal.WithExtension(prov))
	require.NoError(t, err)
	require.NoError(t, outF.Close())

	// Test GetExtensionTo
	info, err := seal.GetExtensionTo[provenance.Info](ctx, archivePath, provenance.ExtensionID)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "abc1234", info.Commit)
	assert.Equal(t, "feature/generic", info.Branch)
	assert.Equal(t, "SealCI", info.Builder)

	// Test InspectProvenance
	inspected, err := seal.InspectProvenance(ctx, archivePath)
	require.NoError(t, err)
	require.NotNil(t, inspected)
	assert.Equal(t, "abc1234", inspected.Commit)

	// Test nonexistent extension
	_, err = seal.GetExtensionTo[provenance.Info](ctx, archivePath, extension.ID("nonexistent"))
	require.Error(t, err)
}

func TestGenericOptionFilters(t *testing.T) {
	// Test FilterOnly with generic.Set
	flt := option.FilterOnly("src/main.go", "docs/index.html")
	assert.True(t, flt("src/main.go", false))
	assert.True(t, flt("docs/index.html", false))
	assert.False(t, flt("src/util.go", false))

	// Test PreCompressed extensions with generic.Set
	assert.True(t, option.IsPreCompressedExtension("archive.zip"))
	assert.True(t, option.IsPreCompressedExtension("video.mp4"))
	assert.True(t, option.IsPreCompressedExtension("photo.JPEG"))
	assert.False(t, option.IsPreCompressedExtension("source.go"))
	assert.False(t, option.IsPreCompressedExtension("text.txt"))
}
