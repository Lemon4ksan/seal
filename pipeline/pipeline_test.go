// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"bytes"
	"hash/crc32"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/crypto"
)

func TestWritePipeline_FullChain(t *testing.T) {
	rawContent := []byte(
		"The quick brown fox jumps over the lazy dog. A quick brown fox jumps over the lazy dog again and again!",
	)
	expectedCRC := crc32.ChecksumIEEE(rawContent)

	var outputBuf bytes.Buffer

	// Build composite pipeline: Hasher -> Deflate -> AES-CTR (WinZip) -> Output
	pipe := NewWritePipeline(&outputBuf).
		WithHasher(crc32.NewIEEE()).
		WithCipher(func(w io.Writer) (io.WriteCloser, error) {
			return crypto.NewAESCTRWriter(w, "PipelineSecret123", 32, 16)
		}).
		WithCompressor(func(src io.Reader, dst io.Writer) (int64, error) {
			comp, err := codec.DefaultRegistry.Compressor(codec.Deflate, 6)
			if err != nil {
				return 0, err
			}
			return comp.Compress(src, dst)
		})

	res, err := pipe.Execute(bytes.NewReader(rawContent))
	require.NoError(t, err)

	assert.Equal(t, int64(len(rawContent)), res.UncompressedBytes)
	assert.Equal(t, expectedCRC, res.Checksum)
	assert.Equal(t, int64(outputBuf.Len()), res.CompressedBytes)

	// Verify decryption and decompression unwrapping
	aesReader, err := crypto.NewAESCTRReader(&outputBuf, "PipelineSecret123", int64(outputBuf.Len()), 32, 16)
	require.NoError(t, err)

	decomp, err := codec.DefaultRegistry.Decompressor(codec.Deflate)
	require.NoError(t, err)

	decompReader, err := decomp.Decompress(aesReader)
	require.NoError(t, err)
	defer decompReader.Close()

	var decompBuf bytes.Buffer
	_, err = io.Copy(&decompBuf, decompReader)
	require.NoError(t, err)

	assert.Equal(t, string(rawContent), decompBuf.String())
}
