// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/chunk"
	"github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/filter"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

// customMeshFilter is a simulated third-party filter (ID: 0x88) that XORs payload with a byte mask.
type customMeshFilter struct{}

func (c *customMeshFilter) ID() uint8    { return 0x88 }
func (c *customMeshFilter) Name() string { return "custom-mesh-quantizer" }

func (c *customMeshFilter) Encode(src []byte, params []byte) ([]byte, error) {
	mask := byte(0x5A)
	if len(params) > 0 {
		mask = params[0]
	}
	dst := make([]byte, len(src))
	for i, b := range src {
		dst[i] = b ^ mask
	}
	return dst, nil
}

func (c *customMeshFilter) Decode(src []byte, params []byte) ([]byte, error) {
	mask := byte(0x5A)
	if len(params) > 0 {
		mask = params[0]
	}
	dst := make([]byte, len(src))
	for i, b := range src {
		dst[i] = b ^ mask
	}
	return dst, nil
}

func TestChunkPipeline_Standalone(t *testing.T) {
	// Register custom third-party filter
	filter.Register(&customMeshFilter{})

	// Synthesize test payload mimicking executable machine code with periodic structures
	testData := make([]byte, 8192)
	for i := range testData {
		testData[i] = byte(i%251) ^ byte((i>>3)&0xFF)
	}

	// 1. Build pipeline: BCJ-x86 -> Delta(4) -> CustomMesh(0x88) -> ZSTD
	p := filter.NewPipeline(spec.CodecZSTD, 3)
	p.AddStage(spec.FilterIDBCJX86)
	p.AddStage(spec.FilterIDDelta, 4)
	p.AddStage(0x88, 0xA5) // Custom filter with parameter 0xA5

	// 2. Create chunk with pipeline
	chk := chunk.NewWithPipeline(0, p)
	if err := chk.Compress(testData); err != nil {
		t.Fatalf("chunk compress with pipeline: %v", err)
	}

	if (chk.Flags & spec.ChunkFlagPipeline) == 0 {
		t.Fatalf("expected ChunkFlagPipeline to be set in chunk flags, got: 0x%04X", chk.Flags)
	}

	if chk.CompressedSize == 0 || len(chk.Payload) == 0 {
		t.Fatalf("compressed payload is empty")
	}

	// 3. Decompress chunk
	decompressed, err := chk.Decompress()
	if err != nil {
		t.Fatalf("chunk decompress with pipeline: %v", err)
	}

	if !bytes.Equal(testData, decompressed) {
		t.Fatalf("decompressed data does not match original: len=%d vs %d", len(decompressed), len(testData))
	}
}

func TestChunkPipeline_ContainerEndToEnd(t *testing.T) {
	filter.Register(&customMeshFilter{})

	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "pipeline_test.eon")

	outFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive file: %v", err)
	}

	// Create writer with standard ZSTD
	w := engine.NewWriter(outFile, engine.WithCodec(spec.CodecZSTD))

	// Add a normal file (standard pipeline)
	fileData := []byte("Standard file data in VFS namespace.")
	if err := w.AddFile("docs/readme.txt", fileData); err != nil {
		t.Fatalf("add file: %v", err)
	}

	// Add an extent with a custom multi-stage pipeline: Shuffle(4) -> Delta(2) -> CustomMesh(0x88) -> LZ4
	customExtentData := bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04, 0x10, 0x20, 0x30, 0x40}, 512)
	customPipeline := filter.NewPipeline(spec.CodecLZ4, 1)
	customPipeline.AddStage(spec.FilterIDShuffle, 4)
	customPipeline.AddStage(spec.FilterIDDelta, 2)
	customPipeline.AddStage(0x88, 0x3C)

	ext := catalog.Extent{
		Namespace: 0x8888, // Custom domain
		Timestamp: time.Now().UnixNano(),
		Name:      "models/dragon.geo",
	}

	if err := w.AddExtentWithPipeline(ext, customExtentData, customPipeline); err != nil {
		t.Fatalf("add extent with pipeline: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("writer close: %v", err)
	}
	outFile.Close()

	// Re-open container and verify contents
	r, f, err := engine.OpenFile(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close()

	// Verify standard file
	readReadme, err := r.ReadExtent("docs/readme.txt")
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if !bytes.Equal(readReadme, fileData) {
		t.Fatalf("readme mismatch: expected %q, got %q", string(fileData), string(readReadme))
	}

	// Verify extent processed through custom pipeline
	readGeo, err := r.ReadExtent("models/dragon.geo")
	if err != nil {
		t.Fatalf("read extent dragon.geo: %v", err)
	}
	if !bytes.Equal(readGeo, customExtentData) {
		t.Fatalf("dragon.geo payload mismatch: len=%d vs %d", len(readGeo), len(customExtentData))
	}
}

func TestChunkPipeline_UnregisteredFilterError(t *testing.T) {
	// Craft a pipeline with an unregistered filter ID (0x99)
	p := filter.NewPipeline(spec.CodecStore, 0)
	p.AddStage(0x99) // Unregistered!

	chk := chunk.NewWithPipeline(0, p)
	// Compression will fail looking up unregistered filter
	err := chk.Compress([]byte("some data"))
	if !errors.Is(err, filter.ErrFilterNotFound) {
		t.Fatalf("expected ErrFilterNotFound on unregistered filter, got: %v", err)
	}
}
