// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"os"

	"github.com/lemon4ksan/przm"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

// PRZMCodec implements the PRZM (PrismLZ) structured dictionary compression format (PRZM-SPEC-0.5).
type PRZMCodec struct{}

// ID returns [spec.CodecPRZM] (0x06).
func (p *PRZMCodec) ID() uint8 {
	return spec.CodecPRZM
}

// Name returns "przm".
func (p *PRZMCodec) Name() string {
	return "przm"
}

// Compress encodes raw source bytes using the PRZM engine.
func (p *PRZMCodec) Compress(src []byte, level int) ([]byte, error) {
	opts := przm.CompressOptions{Level: level}
	if os.Getenv("PRZM_GPU") == "1" {
		opts.GPUAcceleration = true
	}
	return przm.CompressWithOptions(src, opts)
}

// Decompress decodes PRZM-compressed payload back to original bytes.
func (p *PRZMCodec) Decompress(src []byte) ([]byte, error) {
	return przm.Decode(src)
}

func init() {
	Register(&PRZMCodec{})
}
