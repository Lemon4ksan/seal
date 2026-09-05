// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package codec provides compression and decompression abstractions for EON chunks.
package codec

import (
	"fmt"
	"strings"
	"sync"
)

// Codec defines the compression and decompression contract for an EON solid chunk.
type Codec interface {
	// ID returns the 8-bit codec identifier.
	ID() uint8

	// Name returns the human-readable codec label.
	Name() string

	// Compress encodes raw uncompressed source bytes into compressed destination.
	Compress(src []byte, level int) ([]byte, error)

	// Decompress decodes compressed source bytes back into original uncompressed form.
	Decompress(src []byte) ([]byte, error)
}

var (
	registryMu sync.RWMutex
	codecs     = make(map[uint8]Codec)
)

// Register registers a codec implementation for its ID.
func Register(c Codec) {
	registryMu.Lock()
	defer registryMu.Unlock()
	codecs[c.ID()] = c
}

// Get resolves a registered codec by its ID.
func Get(id uint8) (Codec, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	c, ok := codecs[id]
	if !ok {
		return nil, fmt.Errorf("aeon/codec: unsupported codec ID 0x%02X", id)
	}
	return c, nil
}

// ByName resolves a registered codec by its name (case-insensitive).
func ByName(name string) (Codec, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, c := range codecs {
		if strings.EqualFold(c.Name(), name) {
			return c, nil
		}
	}
	return nil, fmt.Errorf("aeon/codec: unknown codec %q", name)
}

func init() {
	Register(&StoreCodec{})
	Register(NewZstdCodec())
	Register(&OpenZLCodec{})
	Register(&LZMA2Codec{})
	Register(&BrotliCodec{})
	Register(&LZ4Codec{})
	Register(&PRZMCodec{})
}
