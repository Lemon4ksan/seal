// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/eon"
	"github.com/lemon4ksan/seal/file"
)

func TestEON_SMX_SelfMountingExecutable(t *testing.T) {
	// Create mock files to package into EON
	f1, err := file.New("app.config", false)
	if err != nil {
		t.Fatal(err)
	}
	f1.WithData([]byte(`{"app": "WallpaperEngine", "mode": "smx", "threads": 16}`))

	f2, err := file.New("shaders/bloom.spv", false)
	if err != nil {
		t.Fatal(err)
	}
	f2.WithData(bytes.Repeat([]byte{0x03, 0x02, 0x23, 0x07, 0x00, 0x00, 0x01, 0x00}, 2048)) // 16 KiB dummy SPIR-V

	files := []*file.File{f1, f2}

	// 1. Serialize EON archive
	var eonBuf bytes.Buffer
	writer := eon.NewWriter(&eonBuf)
	cfg := container.Config{
		DefaultMethod: codec.PRZM,
		DefaultLevel:  1,
	}
	if _, err := writer.WriteArchive(files, cfg); err != nil {
		t.Fatalf("failed to write archive: %v", err)
	}

	// 2. Prepend a simulated 8192-byte PE32+ executable stub (SMX launcher)
	stubSize := 8192
	stub := make([]byte, stubSize)
	copy(stub, []byte("MZ\x90\x00PE\x00\x00EON-SMX-LAUNCHER-STUB"))

	var smxExe bytes.Buffer
	smxExe.Write(stub)
	smxExe.Write(eonBuf.Bytes())

	// 3. Mount SMX directly from memory via eon.NewReader
	rAt := bytes.NewReader(smxExe.Bytes())
	reader, err := eon.NewReader(rAt, int64(smxExe.Len()))
	if err != nil {
		t.Fatalf("eon.NewReader failed to mount SMX: %v", err)
	}

	mountedFiles := reader.Files()
	if len(mountedFiles) != 2 {
		t.Fatalf("expected 2 mounted files, got %d", len(mountedFiles))
	}

	// 4. Verify on-the-fly streaming decompression of assets from SMX binary
	var content1 bytes.Buffer
	rc1, err := mountedFiles[0].Open()
	if err != nil {
		t.Fatalf("failed to open mounted file 0: %v", err)
	}
	defer rc1.Close()
	if _, err := io.Copy(&content1, rc1); err != nil {
		t.Fatalf("failed to stream file 0: %v", err)
	}
	if !bytes.Equal(content1.Bytes(), []byte(`{"app": "WallpaperEngine", "mode": "smx", "threads": 16}`)) {
		t.Fatalf("file 0 content mismatch")
	}

	var content2 bytes.Buffer
	rc2, err := mountedFiles[1].Open()
	if err != nil {
		t.Fatalf("failed to open mounted file 1: %v", err)
	}
	defer rc2.Close()
	if _, err := io.Copy(&content2, rc2); err != nil {
		t.Fatalf("failed to stream file 1: %v", err)
	}
	if len(content2.Bytes()) != 16384 {
		t.Fatalf("expected 16384 bytes for shader, got %d", len(content2.Bytes()))
	}
}
