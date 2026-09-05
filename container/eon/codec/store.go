// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"bytes"

	"github.com/lemon4ksan/seal/container/eon/spec"
)

// StoreCodec implements uncompressed storage.
type StoreCodec struct{}

// ID returns [spec.CodecStore].
func (s *StoreCodec) ID() uint8 {
	return spec.CodecStore
}

// Name returns "store".
func (s *StoreCodec) Name() string {
	return "store"
}

// Compress copies uncompressed bytes.
func (s *StoreCodec) Compress(src []byte, _ int) ([]byte, error) {
	return bytes.Clone(src), nil
}

// Decompress copies uncompressed bytes.
func (s *StoreCodec) Decompress(src []byte) ([]byte, error) {
	return bytes.Clone(src), nil
}
