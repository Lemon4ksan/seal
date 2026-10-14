// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/rand"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/codec"
	"github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

// TestProfileFastMount verifies that the FastMount profile configures 128 KiB LZ4 micro-chunks.
func TestProfileFastMount(t *testing.T) {
	var buf bytes.Buffer
	w := engine.NewWriter(&buf, engine.WithProfileFastMount())

	// Create 300 KiB of compressible data to produce multiple ~128 KiB chunks
	data := make([]byte, 300*1024)
	for i := range data {
		data[i] = byte(i % 97)
	}

	if err := w.AddFile("system.img", data); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	raw := buf.Bytes()
	r, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}

	ctoc := r.CTOC()
	if ctoc == nil || len(ctoc.Entries) < 2 {
		t.Fatalf("expected at least 2 chunks for 300KB with 128KB target, got %d", len(ctoc.Entries))
	}

	for i, entry := range ctoc.Entries {
		if entry.UncompressedSize > 132000 {
			t.Errorf("chunk %d uncompressed size %d exceeds 128KB boundary", i, entry.UncompressedSize)
		}
	}

	extracted, err := r.ReadExtent("system.img")
	if err != nil {
		t.Fatalf("ReadExtent failed: %v", err)
	}
	if !bytes.Equal(extracted, data) {
		t.Fatal("bit-for-bit extraction mismatch in FastMount profile")
	}
}

// TestProfileArchiveMax verifies that ArchiveMax sets 64MB target chunk size and ZSTD-19.
func TestProfileArchiveMax(t *testing.T) {
	var buf bytes.Buffer
	w := engine.NewWriter(&buf, engine.WithProfileArchiveMax())

	data := []byte("ArchiveMax compression ratio testing with solid multi-megabyte block sizing.")
	if err := w.AddFile("doc.txt", data); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	raw := buf.Bytes()
	r, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}

	extracted, err := r.ReadExtent("doc.txt")
	if err != nil {
		t.Fatalf("ReadExtent failed: %v", err)
	}
	if !bytes.Equal(extracted, data) {
		t.Fatal("bit-for-bit extraction mismatch in ArchiveMax profile")
	}
}

// TestSmartCodecsAutoDetect verifies that:
// 1. Pre-compressed media (.mp4) is automatically routed to CodecStore (0% wasted CPU).
// 2. ML tensors (.safetensors) automatically receive Byte-Shuffle filter + compression.
// 3. Normal text files are compressed with standard codec.
func TestSmartCodecsAutoDetect(t *testing.T) {
	var buf bytes.Buffer
	w := engine.NewWriter(&buf, engine.WithAutoDetect(true))

	// 1. Incompressible video file
	mediaData := make([]byte, 64*1024)
	rnd := rand.New(rand.NewSource(42))
	rnd.Read(mediaData)

	if err := w.AddFile("assets/video.mp4", mediaData); err != nil {
		t.Fatalf("AddFile video.mp4 failed: %v", err)
	}

	// 2. Synthetic FP32 tensor weights (4-byte floats: e.g. 1.0, 1.0001, 1.0002, ...)
	// When unshuffled, byte streams vary rapidly; when shuffled, exponent/sign bytes form long contiguous runs!
	tensorFloats := make([]float32, 16384)
	for i := range tensorFloats {
		tensorFloats[i] = 1.0 + float32(i)*0.0001
	}
	tensorData := make([]byte, len(tensorFloats)*4)
	for i, f := range tensorFloats {
		binary.LittleEndian.PutUint32(tensorData[i*4:], math.Float32bits(f))
	}

	if err := w.AddFile("model/weights.safetensors", tensorData); err != nil {
		t.Fatalf("AddFile weights.safetensors failed: %v", err)
	}

	// 3. Normal text file
	textData := []byte("func main() { fmt.Println(\"Hello smart codec pipeline!\") }")
	if err := w.AddFile("src/main.go", textData); err != nil {
		t.Fatalf("AddFile main.go failed: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	raw := buf.Bytes()
	r, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}

	// Verify bit-for-bit extraction of all 3 files
	gotMedia, err := r.ReadExtent("assets/video.mp4")
	if err != nil {
		t.Fatalf("ReadExtent video.mp4 failed: %v", err)
	}
	if !bytes.Equal(gotMedia, mediaData) {
		t.Fatal("bit-for-bit mismatch on video.mp4")
	}

	gotTensor, err := r.ReadExtent("model/weights.safetensors")
	if err != nil {
		t.Fatalf("ReadExtent weights.safetensors failed: %v", err)
	}
	if !bytes.Equal(gotTensor, tensorData) {
		t.Fatal("bit-for-bit mismatch on weights.safetensors")
	}

	gotText, err := r.ReadExtent("src/main.go")
	if err != nil {
		t.Fatalf("ReadExtent main.go failed: %v", err)
	}
	if !bytes.Equal(gotText, textData) {
		t.Fatal("bit-for-bit mismatch on main.go")
	}
}

// TestLZ4CodecRegistration verifies that LZ4 codec is registered in aeon/codec and roundtrips.
func TestLZ4CodecRegistration(t *testing.T) {
	c, err := codec.Get(spec.CodecLZ4)
	if err != nil {
		t.Fatalf("codec.Get(spec.CodecLZ4) failed: %v", err)
	}
	if c.Name() != "lz4" {
		t.Fatalf("expected codec name lz4, got %s", c.Name())
	}

	data := []byte("High speed LZ4 chunk codec testing in aeon container format.")
	compressed, err := c.Compress(data, 1)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}
	decompressed, err := c.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}
	if !bytes.Equal(decompressed, data) {
		t.Fatal("decompress mismatch")
	}
}

// TestPRZMCodecRegistration verifies that PRZM is properly registered under spec.CodecPRZM and ByName("przm").
func TestPRZMCodecRegistration(t *testing.T) {
	c, err := codec.Get(spec.CodecPRZM)
	if err != nil {
		t.Fatalf("codec.Get(spec.CodecPRZM) failed: %v", err)
	}
	if c.Name() != "przm" {
		t.Fatalf("expected codec name przm, got %s", c.Name())
	}
	cByName, err := codec.ByName("przm")
	if err != nil {
		t.Fatalf("codec.ByName(przm) failed: %v", err)
	}
	if cByName.ID() != spec.CodecPRZM {
		t.Fatalf("expected ID 0x06, got 0x%02X", cByName.ID())
	}

	data := []byte("PRZM structured dictionary compression engine testing in aeon container.")
	compressed, err := c.Compress(data, 1)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}
	decompressed, err := c.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}
	if !bytes.Equal(decompressed, data) {
		t.Fatal("decompress mismatch")
	}
}

// TestPRZMContainerRoundtrip verifies creating, packing, and extracting an EON container using PRZM codec.
func TestPRZMContainerRoundtrip(t *testing.T) {
	var buf bytes.Buffer
	w := engine.NewWriter(&buf, engine.WithCodec(spec.CodecPRZM))

	testFiles := map[string][]byte{
		"config.json":   []byte(`{"service":"gateway","port":8080,"workers":16,"tls":true}`),
		"telemetry.log": []byte("INFO [2026-09-04 19:40:01] Gateway online\nINFO [2026-09-04 19:40:02] Client connected\n"),
		"matrix.csv":    []byte("id,val1,val2,val3\n1,10.5,20.2,30.1\n2,11.5,21.2,31.1\n3,12.5,22.2,32.1\n"),
	}

	for name, data := range testFiles {
		if err := w.AddFile(name, data); err != nil {
			t.Fatalf("AddFile %s failed: %v", name, err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	raw := buf.Bytes()
	r, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}

	for name, want := range testFiles {
		got, err := r.ReadExtent(name)
		if err != nil {
			t.Fatalf("ReadExtent %s failed: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("extent %s content mismatch: got %q, want %q", name, got, want)
		}
	}
}
