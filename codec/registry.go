// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"fmt"
	"sync"
)

// Registry manages a thread-safe mapping of compression and decompression engines.
type Registry struct {
	mu            sync.RWMutex
	compressors   map[Method]CompressorFactory
	decompressors map[Method]DecompressorFactory
}

// NewRegistry constructs a new Registry initialized with standard library codecs.
func NewRegistry() *Registry {
	r := &Registry{
		compressors:   make(map[Method]CompressorFactory),
		decompressors: make(map[Method]DecompressorFactory),
	}

	// Register default codecs
	r.RegisterCompressor(Store, NewStoreCompressor)
	r.RegisterDecompressor(Store, NewStoreDecompressor)

	r.RegisterCompressor(Deflate, NewDeflateCompressor)
	r.RegisterDecompressor(Deflate, NewDeflateDecompressor)

	r.RegisterCompressor(Gzip, NewGzipCompressor)
	r.RegisterDecompressor(Gzip, NewGzipDecompressor)

	r.RegisterCompressor(Zlib, NewZlibCompressor)
	r.RegisterDecompressor(Zlib, NewZlibDecompressor)

	r.RegisterDecompressor(BZIP2, NewBzip2Decompressor)

	return r
}

// DefaultRegistry is the global default codec registry.
var DefaultRegistry = NewRegistry()

// RegisterCompressor registers a compressor factory for the specified compression method.
func (r *Registry) RegisterCompressor(method Method, factory CompressorFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compressors[method] = factory
}

// RegisterDecompressor registers a decompressor factory for the specified compression method.
func (r *Registry) RegisterDecompressor(method Method, factory DecompressorFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.decompressors[method] = factory
}

// Compressor returns a configured [Compressor] for the method and compression level.
func (r *Registry) Compressor(method Method, level int) (Compressor, error) {
	r.mu.RLock()
	factory, ok := r.compressors[method]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported compression method: %s (%d)", method, method)
	}
	return factory(level), nil
}

// Decompressor returns a new [Decompressor] for the method.
func (r *Registry) Decompressor(method Method) (Decompressor, error) {
	r.mu.RLock()
	factory, ok := r.decompressors[method]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported decompression method: %s (%d)", method, method)
	}
	return factory(), nil
}

// Clone creates a shallow copy of the registry for local archive isolation.
func (r *Registry) Clone() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clone := &Registry{
		compressors:   make(map[Method]CompressorFactory, len(r.compressors)),
		decompressors: make(map[Method]DecompressorFactory, len(r.decompressors)),
	}
	for k, v := range r.compressors {
		clone.compressors[k] = v
	}
	for k, v := range r.decompressors {
		clone.decompressors[k] = v
	}
	return clone
}
