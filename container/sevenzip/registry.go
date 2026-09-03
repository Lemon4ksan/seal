// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"io"
	"sync"
)

// CustomCoder defines the extension interface for plugging arbitrary compression
// algorithms, preprocessors, or filters into 7z containers.
type CustomCoder interface {
	MethodID() uint64
	Name() string
	Decode(coder CoderInfo, in io.Reader, unpackSizes []uint64, password string) (io.ReadCloser, error)
	Encode(in io.Reader, out io.Writer, opts any) (uncompressedBytes int64, props []byte, err error)
}

var (
	customCodersMu sync.RWMutex
	customCoders   = make(map[uint64]CustomCoder)
)

// RegisterCoder installs a custom coder for processing 7z folders with the specified MethodID.
func RegisterCoder(coder CustomCoder) {
	customCodersMu.Lock()
	defer customCodersMu.Unlock()
	customCoders[coder.MethodID()] = coder
}

// LookupCoder retrieves a registered custom coder by its 64-bit MethodID.
func LookupCoder(methodID uint64) (CustomCoder, bool) {
	customCodersMu.RLock()
	defer customCodersMu.RUnlock()
	c, ok := customCoders[methodID]
	return c, ok
}
