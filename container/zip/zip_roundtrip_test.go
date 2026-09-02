// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/file"
)

func TestZip_Roundtrip_Plain(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)

	file1, err := file.New("hello.txt", false)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	file1.WithModTime(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))

	content1 := []byte("Hello, Seal ZIP engine!")
	if err := w.WriteEntry(file1, bytes.NewReader(content1)); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	entries := r.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name() != "hello.txt" {
		t.Fatalf("expected hello.txt, got %s", entries[0].Name())
	}

	rc, err := entries[0].Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	readBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(readBytes) != string(content1) {
		t.Fatalf("content mismatch: got %q, want %q", string(readBytes), string(content1))
	}
}

func TestZip_Roundtrip_AES256(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)
	w.SetPassword("SecretPassword123")

	f, err := file.New("secure.txt", false)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	f.WithEncryption(file.AES256, "SecretPassword123")

	content := []byte("Highly confidential data protected with AES-256 standard.")
	if err := w.WriteEntry(f, bytes.NewReader(content)); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify authentication and decryption succeeds when matching passphrase is provided
	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "SecretPassword123", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	entry, ok := r.Entry("secure.txt")
	if !ok {
		t.Fatalf("entry secure.txt not found")
	}
	if entry.Encryption() != file.AES256 {
		t.Fatalf("expected AES256 encryption, got %v", entry.Encryption())
	}

	rc, err := entry.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	decrypted, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(decrypted) != string(content) {
		t.Fatalf("decrypted mismatch: got %q, want %q", string(decrypted), string(content))
	}

	// Verify authentication failure when incorrect passphrase is provided
	rBad, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "WrongPassword", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader bad: %v", err)
	}

	entryBad, ok := rBad.Entry("secure.txt")
	if !ok {
		t.Fatalf("entry secure.txt not found in bad reader")
	}

	rcBad, err := entryBad.Open()
	if err == nil {
		_, err = io.ReadAll(rcBad)
		_ = rcBad.Close()
	}
	if err == nil {
		t.Fatalf("expected error decrypting with wrong password, got nil")
	}
}

func TestZip_Roundtrip_AES128(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)
	w.SetPassword("SecretAES128Password")

	f, err := file.New("secure128.txt", false)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	f.WithEncryption(file.AES128, "SecretAES128Password")

	content := []byte("Highly confidential data protected with WinZip AES-128 standard.")
	if err := w.WriteEntry(f, bytes.NewReader(content)); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "SecretAES128Password", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	entry, ok := r.Entry("secure128.txt")
	if !ok {
		t.Fatalf("entry secure128.txt not found")
	}
	if entry.Encryption() != file.AES128 {
		t.Fatalf("expected AES128 encryption, got %v", entry.Encryption())
	}

	rc, err := entry.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	decrypted, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(decrypted) != string(content) {
		t.Fatalf("decrypted mismatch: got %q, want %q", string(decrypted), string(content))
	}
}

func TestZip_SFX_SelfExtractingArchive(t *testing.T) {
	// Create a standard ZIP payload
	zipBuf := new(bytes.Buffer)
	w := NewWriter(zipBuf, codec.DefaultRegistry)

	f, err := file.New("setup.cfg", false)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	content := []byte("[SFX Config]\nAppVersion=2026.1\nAutoRun=true\n")
	if err := w.WriteEntry(f, bytes.NewReader(content)); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Prepend a 1024-byte mock PE32 executable stub (e.g. SFX unpacker binary)
	sfxStub := make([]byte, 1024)
	stubHeader := []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xFF\xFF\x00\x00PE-SFX")
	copy(sfxStub, stubHeader)

	sfxExe := make([]byte, 0, len(sfxStub)+zipBuf.Len())
	sfxExe = append(sfxExe, sfxStub...)
	sfxExe = append(sfxExe, zipBuf.Bytes()...)

	// Open SFX .exe as a ZIP archive
	r, err := NewReader(bytes.NewReader(sfxExe), int64(len(sfxExe)), "", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader on SFX failed: %v", err)
	}

	entry, ok := r.Entry("setup.cfg")
	if !ok {
		t.Fatalf("entry setup.cfg not found in SFX archive")
	}

	rc, err := entry.Open()
	if err != nil {
		t.Fatalf("entry.Open failed on SFX: %v", err)
	}
	defer rc.Close()

	readBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed on SFX: %v", err)
	}

	if !bytes.Equal(readBytes, content) {
		t.Fatalf("SFX content mismatch: got %q, want %q", string(readBytes), string(content))
	}
}

func TestZip_StrictCompliance_APPNOTE412_EmptyArchive(t *testing.T) {
	// Under RFC 2119 / APPNOTE 4.1.2 ("SHOULD contain at least one file"),
	// an empty archive containing only the 22-byte EOCD record is valid and permitted.
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)
	if err := w.Close(); err != nil {
		t.Fatalf("unexpected error on empty archive: %v", err)
	}

	if buf.Len() != 22 {
		t.Fatalf("expected empty archive to be exactly 22 bytes (EOCD record), got %d bytes", buf.Len())
	}

	// Verify the resulting empty archive can be read
	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader on empty archive failed: %v", err)
	}
	if len(r.Entries()) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(r.Entries()))
	}
}

func TestZip_StrictCompliance_APPNOTE444_DataDescriptorBit3(t *testing.T) {
	// Streaming mode without seeker MUST set Bit 3 and zero out CRC32 & sizes in Local Header (APPNOTE 4.4.4)
	buf := new(bytes.Buffer)
	// Passing a non-seeker writer wrapper
	type nonSeekerWriter struct{ io.Writer }
	w := NewWriter(nonSeekerWriter{buf}, codec.DefaultRegistry)

	f, _ := file.New("streamed.txt", false)
	content := []byte("Testing APPNOTE 4.4.4 Data Descriptor zeroing.")
	if err := w.WriteEntry(f, bytes.NewReader(content)); err != nil {
		t.Fatalf("WriteEntry failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Parse Local File Header directly from the raw stream
	sr := bytes.NewReader(buf.Bytes())
	var sig [4]byte
	_, _ = io.ReadFull(sr, sig[:])
	lfh, err := ReadLocalFileHeader(sr)
	if err != nil {
		t.Fatalf("ReadLocalFileHeader failed: %v", err)
	}

	if (lfh.GeneralPurposeBitFlag & FlagDataDescriptor) == 0 {
		t.Fatalf("expected FlagDataDescriptor (Bit 3) to be set in streaming mode")
	}
	if lfh.CRC32 != 0 {
		t.Errorf("APPNOTE 4.4.4 violation: Local Header CRC32 MUST be 0 when Bit 3 is set, got 0x%08x", lfh.CRC32)
	}
	if lfh.CompressedSize != 0 {
		t.Errorf(
			"APPNOTE 4.4.4 violation: Local Header CompressedSize MUST be 0 when Bit 3 is set, got %d",
			lfh.CompressedSize,
		)
	}
	if lfh.UncompressedSize != 0 {
		t.Errorf(
			"APPNOTE 4.4.4 violation: Local Header UncompressedSize MUST be 0 when Bit 3 is set, got %d",
			lfh.UncompressedSize,
		)
	}
}

func TestZip_StrictCompliance_APPNOTE444_UTF8EFS(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)

	// UTF-8 non-ASCII filename
	f, _ := file.New("документ.pdf", false)
	if err := w.WriteEntry(f, bytes.NewReader([]byte("PDF Data"))); err != nil {
		t.Fatalf("WriteEntry failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	entry, ok := r.Entry("документ.pdf")
	if !ok {
		t.Fatalf("entry not found")
	}
	if (uint16(entry.Flags()) & FlagLanguageEncodingUTF8) == 0 {
		t.Errorf("APPNOTE 4.4.4 violation: FlagLanguageEncodingUTF8 (Bit 11) MUST be set on non-ASCII names")
	}
}

func TestZip_StrictCompliance_APPNOTE441_VersionNeeded(t *testing.T) {
	tests := []struct {
		method      codec.Method
		isDir       bool
		isEncrypted bool
		isZip64     bool
		wantVersion uint16
	}{
		{codec.Store, false, false, false, VersionDefault},        // 1.0 (10)
		{codec.Deflate, false, false, false, VersionDeflate},      // 2.0 (20)
		{codec.Store, true, false, false, VersionFolder},          // 2.0 (20)
		{codec.Store, false, true, false, VersionZipCrypto},       // 2.0 (20)
		{codec.Deflate64, false, false, false, VersionDeflate64},  // 2.1 (21)
		{codec.PKWAREDCL, false, false, false, VersionDCLImplode}, // 2.5 (25)
		{codec.BZIP2, false, false, false, VersionBZIP2},          // 4.6 (46)
		{codec.LZMA, false, false, false, VersionLZMA},            // 6.3 (63)
		{codec.ZStandard, false, false, false, VersionLZMA},       // 6.3 (63)
		{codec.Store, false, false, true, VersionZip64},           // 4.5 (45)
	}

	for _, tt := range tests {
		got := ComputeVersionNeeded(tt.method, tt.isDir, tt.isEncrypted, tt.isZip64)
		if got != tt.wantVersion {
			t.Errorf("ComputeVersionNeeded(%v, %v, %v, %v) = %d, want %d",
				tt.method, tt.isDir, tt.isEncrypted, tt.isZip64, got, tt.wantVersion)
		}
	}
}
