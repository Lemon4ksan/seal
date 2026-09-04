// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/container/sevenzip"
	"github.com/lemon4ksan/seal/file"
)

// xorMockCoder implements sevenzip.CustomCoder for testing custom algorithm integration.
type xorMockCoder struct {
	key byte
}

func (c *xorMockCoder) MethodID() uint64 {
	return 0x04F79901 // Custom user-defined 64-bit MethodID
}

func (c *xorMockCoder) Name() string {
	return "XOR-Mock"
}

func (c *xorMockCoder) Encode(in io.Reader, out io.Writer, _ any) (int64, []byte, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return 0, nil, err
	}
	encoded := make([]byte, len(data))
	for i, b := range data {
		encoded[i] = b ^ c.key
	}
	n, err := out.Write(encoded)
	return int64(n), []byte{c.key}, err
}

func (c *xorMockCoder) Decode(coder sevenzip.CoderInfo, in io.Reader, _ []uint64, _ string) (io.ReadCloser, error) {
	key := c.key
	if len(coder.Properties) > 0 {
		key = coder.Properties[0]
	}
	data, err := io.ReadAll(in)
	if err != nil {
		return nil, err
	}
	decoded := make([]byte, len(data))
	for i, b := range data {
		decoded[i] = b ^ key
	}
	return io.NopCloser(bytes.NewReader(decoded)), nil
}

func TestSevenZip_CustomCoder_Registration(t *testing.T) {
	mock := &xorMockCoder{key: 0x77}
	sevenzip.RegisterCoder(mock)

	// Verify lookup
	lookedUp, ok := sevenzip.LookupCoder(mock.MethodID())
	require.True(t, ok)
	assert.Equal(t, mock.Name(), lookedUp.Name())

	// Create an archive using the custom coder
	original := []byte("Testing CustomCoder registration and pipeline execution in 7z!")
	f1, err := file.New("secret.dat", false)
	require.NoError(t, err)
	f1.WithData(original)

	var archiveBuf bytes.Buffer
	zw := sevenzip.NewWriterWithOptions(&archiveBuf, sevenzip.WriterOptions{
		Method: sevenzip.MethodType(mock.MethodID()),
	})

	written, err := zw.WriteArchive([]*file.File{f1})
	require.NoError(t, err)
	assert.Greater(t, written, int64(0))

	// Read back the archive using Seal reader
	rAt := bytes.NewReader(archiveBuf.Bytes())
	zr, err := sevenzip.NewReader(rAt, int64(archiveBuf.Len()), "")
	require.NoError(t, err)

	entries := zr.Entries()
	require.Len(t, entries, 1)

	rc, err := entries[0].Open()
	require.NoError(t, err)
	decompressed, err := io.ReadAll(rc)
	_ = rc.Close()
	require.NoError(t, err)

	assert.Equal(t, original, decompressed)
}
