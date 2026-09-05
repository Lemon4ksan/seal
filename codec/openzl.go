// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"errors"
)

// ErrOpenZLNotConfigured is returned when OpenZL graph pipeline is invoked without registered node graphs.
var ErrOpenZLNotConfigured = errors.New("openzl: transformation graph engine not configured")

// OpenZLCodec provides an interface for Meta's OpenZL (Open Zero-Loss) compression graph pipelines.
type OpenZLCodec struct {
	CustomTransformer func(src []byte, level int) ([]byte, error)
	CustomRestorer    func(src []byte) ([]byte, error)
}

// Name returns "openzl".
func (o *OpenZLCodec) Name() string {
	return "openzl"
}

// Compress executes the OpenZL compression graph.
func (o *OpenZLCodec) Compress(src []byte, level int) ([]byte, error) {
	if o.CustomTransformer != nil {
		return o.CustomTransformer(src, level)
	}
	return nil, ErrOpenZLNotConfigured
}

// Decompress inverts the OpenZL compression graph.
func (o *OpenZLCodec) Decompress(src []byte) ([]byte, error) {
	if o.CustomRestorer != nil {
		return o.CustomRestorer(src)
	}
	return nil, ErrOpenZLNotConfigured
}
