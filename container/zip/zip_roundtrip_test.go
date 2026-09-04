// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/codec"
	_ "github.com/lemon4ksan/seal/codec/legacy/cmpsc"
	"github.com/lemon4ksan/seal/file"
)

func TestZip_Roundtrip_Plain(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)

	file1, err := file.New("hello.txt", false)
	require.NoError(t, err)
	file1.WithModTime(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))

	content1 := []byte("Hello, Seal ZIP engine!")
	err = w.WriteEntry(file1, bytes.NewReader(content1))
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", codec.DefaultRegistry)
	require.NoError(t, err)

	entries := r.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "hello.txt", entries[0].Name())

	rc, err := entries[0].Open()
	require.NoError(t, err)
	defer rc.Close()

	readBytes, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, string(content1), string(readBytes))
}

func TestZip_Roundtrip_AES256(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)
	w.SetPassword("SecretPassword123")

	f, err := file.New("secure.txt", false)
	require.NoError(t, err)
	f.WithEncryption(file.AES256, "SecretPassword123")

	content := []byte("Highly confidential data protected with AES-256 standard.")
	err = w.WriteEntry(f, bytes.NewReader(content))
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	// Verify authentication and decryption succeeds when matching passphrase is provided
	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "SecretPassword123", codec.DefaultRegistry)
	require.NoError(t, err)

	entry, ok := r.Entry("secure.txt")
	require.True(t, ok)
	assert.Equal(t, file.AES256, entry.Encryption())

	rc, err := entry.Open()
	require.NoError(t, err)
	defer rc.Close()

	decrypted, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, string(content), string(decrypted))

	// Verify authentication failure when incorrect passphrase is provided
	rBad, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "WrongPassword", codec.DefaultRegistry)
	require.NoError(t, err)

	entryBad, ok := rBad.Entry("secure.txt")
	require.True(t, ok)

	rcBad, err := entryBad.Open()
	if err == nil {
		_, err = io.ReadAll(rcBad)
		_ = rcBad.Close()
	}
	require.Error(t, err)
}

func TestZip_Roundtrip_AES128(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)
	w.SetPassword("SecretAES128Password")

	f, err := file.New("secure128.txt", false)
	require.NoError(t, err)
	f.WithEncryption(file.AES128, "SecretAES128Password")

	content := []byte("Highly confidential data protected with WinZip AES-128 standard.")
	err = w.WriteEntry(f, bytes.NewReader(content))
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "SecretAES128Password", codec.DefaultRegistry)
	require.NoError(t, err)

	entry, ok := r.Entry("secure128.txt")
	require.True(t, ok)
	assert.Equal(t, file.AES128, entry.Encryption())

	rc, err := entry.Open()
	require.NoError(t, err)
	defer rc.Close()

	decrypted, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, string(content), string(decrypted))
}

func TestZip_SFX_SelfExtractingArchive(t *testing.T) {
	// Create a standard ZIP payload
	zipBuf := new(bytes.Buffer)
	w := NewWriter(zipBuf, codec.DefaultRegistry)

	f, err := file.New("setup.cfg", false)
	require.NoError(t, err)
	content := []byte("[SFX Config]\nAppVersion=2026.1\nAutoRun=true\n")
	err = w.WriteEntry(f, bytes.NewReader(content))
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	// Prepend a 1024-byte mock PE32 executable stub (e.g. SFX unpacker binary)
	sfxStub := make([]byte, 1024)
	stubHeader := []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xFF\xFF\x00\x00PE-SFX")
	copy(sfxStub, stubHeader)

	sfxExe := make([]byte, 0, len(sfxStub)+zipBuf.Len())
	sfxExe = append(sfxExe, sfxStub...)
	sfxExe = append(sfxExe, zipBuf.Bytes()...)

	// Open SFX .exe as a ZIP archive
	r, err := NewReader(bytes.NewReader(sfxExe), int64(len(sfxExe)), "", codec.DefaultRegistry)
	require.NoError(t, err)

	entry, ok := r.Entry("setup.cfg")
	require.True(t, ok)

	rc, err := entry.Open()
	require.NoError(t, err)
	defer rc.Close()

	readBytes, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, content, readBytes)
}

func TestZip_StrictCompliance_APPNOTE412_EmptyArchive(t *testing.T) {
	// Under RFC 2119 / APPNOTE 4.1.2 ("SHOULD contain at least one file"),
	// an empty archive containing only the 22-byte EOCD record is valid and permitted.
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)
	err := w.Close()
	require.NoError(t, err)

	assert.Equal(t, 22, buf.Len())

	// Verify the resulting empty archive can be read
	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", codec.DefaultRegistry)
	require.NoError(t, err)
	assert.Empty(t, r.Entries())
}

func TestZip_StrictCompliance_APPNOTE444_DataDescriptorBit3(t *testing.T) {
	// Streaming mode without seeker MUST set Bit 3 and zero out CRC32 & sizes in Local Header (APPNOTE 4.4.4)
	buf := new(bytes.Buffer)
	// Passing a non-seeker writer wrapper
	type nonSeekerWriter struct{ io.Writer }
	w := NewWriter(nonSeekerWriter{buf}, codec.DefaultRegistry)

	f, _ := file.New("streamed.txt", false)
	content := []byte("Testing APPNOTE 4.4.4 Data Descriptor zeroing.")
	err := w.WriteEntry(f, bytes.NewReader(content))
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	// Parse Local File Header directly from the raw stream
	sr := bytes.NewReader(buf.Bytes())
	var sig [4]byte
	_, _ = io.ReadFull(sr, sig[:])
	lfh, err := ReadLocalFileHeader(sr)
	require.NoError(t, err)

	assert.NotZero(t, lfh.GeneralPurposeBitFlag&FlagDataDescriptor)
	assert.Equal(t, uint32(0), lfh.CRC32)
	assert.Equal(t, uint32(0), lfh.CompressedSize)
	assert.Equal(t, uint32(0), lfh.UncompressedSize)
}

func TestZip_StrictCompliance_APPNOTE444_UTF8EFS(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)

	// UTF-8 non-ASCII filename
	f, _ := file.New("документ.pdf", false)
	err := w.WriteEntry(f, bytes.NewReader([]byte("PDF Data")))
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", codec.DefaultRegistry)
	require.NoError(t, err)

	entry, ok := r.Entry("документ.pdf")
	require.True(t, ok)
	assert.NotZero(t, uint16(entry.Flags())&FlagLanguageEncodingUTF8)
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
		assert.Equalf(t, tt.wantVersion, got, "ComputeVersionNeeded(%v, %v, %v, %v)", tt.method, tt.isDir, tt.isEncrypted, tt.isZip64)
	}
}

func TestZip_IBM_CMPSC_Roundtrip(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)

	file1, err := file.New("mainframe_job.jcl", false)
	require.NoError(t, err)
	file1.WithMethod(codec.IBMCmpsc)
	content := []byte("//JOB01 EXEC PGM=IEFBR14\n//DD1 DD DSN=SYS1.LINKLIB,DISP=SHR\n//SYSIN DD *\n/*\n")

	err = w.WriteEntry(file1, bytes.NewReader(content))
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", codec.DefaultRegistry)
	require.NoError(t, err)

	entries := r.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, codec.IBMCmpsc, entries[0].Method())

	rc, err := entries[0].Open()
	require.NoError(t, err)
	defer rc.Close()

	decomp, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, content, decomp)
}
