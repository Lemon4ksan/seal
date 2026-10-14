// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"sync"

	"github.com/lemon4ksan/przm"

	"github.com/lemon4ksan/seal/codec/compress/brotli"
)

// DictIDBrotliWeb defines the 32-bit PRZM dictionary ID for the RFC 7932 Brotli-derived external web dictionary.
const DictIDBrotliWeb uint32 = 0x4252544C // "BRTL"

var brotliDictOnce sync.Once

// RegisterBrotliWebDictionary registers the 122KB RFC 7932 Brotli static dictionary in PRZM's raw dictionary registry.
func RegisterBrotliWebDictionary() {
	brotliDictOnce.Do(func() {
		rawData := brotli.StaticDictionaryData()
		przm.RegisterRawDictionary(DictIDBrotliWeb, rawData)
	})
}

// GetBrotliWebDictionaryData returns the full RFC 7932 122,784 byte static dictionary buffer.
func GetBrotliWebDictionaryData() []byte {
	return brotli.StaticDictionaryData()
}
