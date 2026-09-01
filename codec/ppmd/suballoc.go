// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ppmd

import (
	"encoding/binary"
	"errors"
)

const (
	kUnitSize   = 12
	kNumIndexes = 38
	kMinMemSize = 1 << 11
	kMaxMemSize = 0xFFFFFFFF - 12*3
)

// SubAllocator manages PPMd dynamic context tree allocations.
type SubAllocator struct {
	base       []byte
	size       uint32
	unitsStart uint32
	loUnit     uint32
	hiUnit     uint32
	glueCount  uint32
	indx2Units [kNumIndexes + 2]byte
	units2Indx [128]byte
	freeList   [kNumIndexes]uint32
}

// Init initializes the sub-allocator tables.
func (sa *SubAllocator) Init(size uint32) error {
	if size < kMinMemSize {
		size = kMinMemSize
	}
	sa.size = size
	sa.base = make([]byte, size)

	var k byte
	for i := byte(0); i < kNumIndexes; i++ {
		var step byte
		if i >= 12 {
			step = 4
		} else {
			step = (i >> 2) + 1
		}
		for step > 0 {
			if int(k) < len(sa.units2Indx) {
				sa.units2Indx[k] = i
			}
			k++
			step--
		}
		sa.indx2Units[i] = k
	}

	sa.StartModel()
	return nil
}

// StartModel resets the suballocator state for a new compression session.
func (sa *SubAllocator) StartModel() {
	for i := range sa.freeList {
		sa.freeList[i] = 0
	}
	sa.glueCount = 0
	sa.hiUnit = sa.size
	sa.loUnit = 0
	sa.unitsStart = 0
}

// AllocUnits allocates count units (each 12 bytes) from the pool.
func (sa *SubAllocator) AllocUnits(numUnits int) (uint32, error) {
	if numUnits <= 0 {
		return 0, errors.New("ppmd: invalid unit allocation count")
	}

	indx := int(sa.units2Indx[numUnits-1])
	if sa.freeList[indx] != 0 {
		ptr := sa.freeList[indx]
		sa.freeList[indx] = binary.LittleEndian.Uint32(sa.base[ptr:])
		return ptr, nil
	}

	reqBytes := uint32(numUnits * kUnitSize)
	if sa.hiUnit-sa.loUnit >= reqBytes {
		ptr := sa.hiUnit - reqBytes
		sa.hiUnit = ptr
		return ptr, nil
	}

	return 0, errors.New("ppmd: out of memory in suballocator")
}

// AllocContext allocates a single context unit.
func (sa *SubAllocator) AllocContext() (uint32, error) {
	if sa.hiUnit-sa.loUnit >= kUnitSize {
		ptr := sa.hiUnit - kUnitSize
		sa.hiUnit = ptr
		return ptr, nil
	}
	return sa.AllocUnits(1)
}

// FreeUnits returns units back to the free list.
func (sa *SubAllocator) FreeUnits(ptr uint32, numUnits int) {
	if ptr == 0 || numUnits <= 0 {
		return
	}
	indx := int(sa.units2Indx[numUnits-1])
	binary.LittleEndian.PutUint32(sa.base[ptr:], sa.freeList[indx])
	sa.freeList[indx] = ptr
}
