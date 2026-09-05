// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package filter provides reversible data transformation filters for EON chunk pipelines.
package filter

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/foundation/codec/filter/bcj"
	"github.com/lemon4ksan/foundation/codec/filter/delta"
	"github.com/lemon4ksan/foundation/codec/filter/shuffle"
)

var (
	// ErrFilterNotFound indicates an unregistered filter identifier.
	ErrFilterNotFound = errors.New("filter: unregistered filter ID")
	// ErrInvalidParams indicates invalid parameters for a filter.
	ErrInvalidParams = errors.New("filter: invalid parameters")
)

// Filter defines the interface for a reversible data pre/post-processing stage in a chunk pipeline.
type Filter interface {
	// ID returns the 8-bit filter identifier (spec.FilterID* or custom >= 0x80).
	ID() uint8

	// Name returns the human-readable identifier of this filter.
	Name() string

	// Encode applies the forward transformation on raw source bytes.
	Encode(src []byte, params []byte) ([]byte, error)

	// Decode reverses the transformation to recover original uncompressed bytes.
	Decode(src []byte, params []byte) ([]byte, error)
}

var (
	registryMu sync.RWMutex
	filters    = make(map[uint8]Filter)
)

// Register registers a filter implementation in the global registry.
func Register(f Filter) {
	registryMu.Lock()
	defer registryMu.Unlock()
	filters[f.ID()] = f
}

// Get resolves a registered filter by its 8-bit ID.
func Get(id uint8) (Filter, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := filters[id]
	if !ok {
		return nil, fmt.Errorf("%w: 0x%02X", ErrFilterNotFound, id)
	}
	return f, nil
}

// ByName resolves a registered filter by its name (case-insensitive).
func ByName(name string) (Filter, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, f := range filters {
		if strings.EqualFold(f.Name(), name) {
			return f, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrFilterNotFound, name)
}

// ShuffleFilter implements byte-shuffling for structured/numeric/tensor data.
type ShuffleFilter struct{}

func (s *ShuffleFilter) ID() uint8     { return spec.FilterIDShuffle }
func (s *ShuffleFilter) Name() string { return "shuffle" }

func (s *ShuffleFilter) Encode(src []byte, params []byte) ([]byte, error) {
	stride := 4
	if len(params) > 0 && params[0] > 0 {
		stride = int(params[0])
	}
	return shuffle.Encode(src, nil, stride)
}

func (s *ShuffleFilter) Decode(src []byte, params []byte) ([]byte, error) {
	stride := 4
	if len(params) > 0 && params[0] > 0 {
		stride = int(params[0])
	}
	return shuffle.Decode(src, nil, stride)
}

// BCJX86Filter implements Branch-Call-Jump relative address translation for x86 / x86-64 machine code.
type BCJX86Filter struct{}

func (b *BCJX86Filter) ID() uint8     { return spec.FilterIDBCJX86 }
func (b *BCJX86Filter) Name() string { return "bcj-x86" }

func (b *BCJX86Filter) Encode(src []byte, params []byte) ([]byte, error) {
	dst := make([]byte, len(src))
	copy(dst, src)
	bcj.Filter(bcj.X86, dst, 0, true)
	return dst, nil
}

func (b *BCJX86Filter) Decode(src []byte, params []byte) ([]byte, error) {
	dst := make([]byte, len(src))
	copy(dst, src)
	bcj.Filter(bcj.X86, dst, 0, false)
	return dst, nil
}

// BCJARM64Filter implements branch relative address translation for AArch64 (ARM64) machine code.
type BCJARM64Filter struct{}

func (b *BCJARM64Filter) ID() uint8     { return spec.FilterIDBCJARM64 }
func (b *BCJARM64Filter) Name() string { return "bcj-arm64" }

func (b *BCJARM64Filter) Encode(src []byte, params []byte) ([]byte, error) {
	dst := make([]byte, len(src))
	copy(dst, src)
	bcj.Filter(bcj.ARM64, dst, 0, true)
	return dst, nil
}

func (b *BCJARM64Filter) Decode(src []byte, params []byte) ([]byte, error) {
	dst := make([]byte, len(src))
	copy(dst, src)
	bcj.Filter(bcj.ARM64, dst, 0, false)
	return dst, nil
}

// DeltaFilter implements the byte distance prediction filter.
type DeltaFilter struct{}

func (d *DeltaFilter) ID() uint8     { return spec.FilterIDDelta }
func (d *DeltaFilter) Name() string { return "delta" }

func (d *DeltaFilter) Encode(src []byte, params []byte) ([]byte, error) {
	dist := 1
	if len(params) > 0 && params[0] > 0 {
		dist = int(params[0])
	}
	f, err := delta.NewFilter(dist)
	if err != nil {
		return nil, fmt.Errorf("delta filter init: %w", err)
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	f.Encode(dst)
	return dst, nil
}

func (d *DeltaFilter) Decode(src []byte, params []byte) ([]byte, error) {
	dist := 1
	if len(params) > 0 && params[0] > 0 {
		dist = int(params[0])
	}
	f, err := delta.NewFilter(dist)
	if err != nil {
		return nil, fmt.Errorf("delta filter init: %w", err)
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	f.Decode(dst)
	return dst, nil
}

func init() {
	Register(&ShuffleFilter{})
	Register(&BCJX86Filter{})
	Register(&BCJARM64Filter{})
	Register(&DeltaFilter{})
}
