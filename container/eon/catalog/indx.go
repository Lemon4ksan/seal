// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package catalog

import (
	"bufio"
	"cmp"
	"encoding/binary"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/lemon4ksan/seal/container/eon/spec"
)

// Extent represents an addressable entity in the universal catalog.
type Extent struct {
	Namespace   uint16
	ChunkIndex  uint32
	ChunkCount  uint16
	ExtentFlags uint16
	Timestamp   int64
	Hash        [32]byte
	Target      string
	Name        string
	ExtraTLVs   []byte
	ChunkOffset uint64
	PayloadSize uint64
}

// Index represents the full extent catalog.
type Index struct {
	Extents []Extent
}

// Write serializes the INDX catalog into w.
func (idx *Index) Write(w io.Writer) (int64, error) {
	var total int64
	var hdr [spec.INDXHeaderSize]byte
	copy(hdr[0:4], spec.MagicINDX[:])
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(idx.Extents)))

	if _, err := w.Write(hdr[:]); err != nil {
		return total, err
	}
	total += spec.INDXHeaderSize

	for _, ext := range idx.Extents {
		targetBytes := []byte(ext.Target)
		nameBytes := []byte(ext.Name)

		var varints []byte
		varints = spec.AppendULEB128(varints, ext.ChunkOffset)
		varints = spec.AppendULEB128(varints, ext.PayloadSize)

		recLen := spec.FixedExtentPrefixSize + len(varints) + len(targetBytes) + len(nameBytes) + len(ext.ExtraTLVs)
		if recLen > 0xFFFF {
			return total, fmt.Errorf("eon: extent %q record length (%d) exceeds 65535 bytes", ext.Name, recLen)
		}
		if len(ext.ExtraTLVs) > 0xFFFF {
			return total, fmt.Errorf(
				"eon: extent %q ExtraTLVs length (%d) exceeds 65535 bytes",
				ext.Name,
				len(ext.ExtraTLVs),
			)
		}

		var prefix [spec.FixedExtentPrefixSize]byte
		binary.LittleEndian.PutUint16(prefix[0:2], uint16(recLen))
		binary.LittleEndian.PutUint16(prefix[2:4], ext.Namespace)
		binary.LittleEndian.PutUint32(prefix[4:8], ext.ChunkIndex)
		binary.LittleEndian.PutUint16(prefix[8:10], ext.ChunkCount)
		binary.LittleEndian.PutUint16(prefix[10:12], ext.ExtentFlags)
		binary.LittleEndian.PutUint64(prefix[12:20], uint64(ext.Timestamp))
		copy(prefix[20:52], ext.Hash[:])
		binary.LittleEndian.PutUint16(prefix[52:54], uint16(len(targetBytes)))
		binary.LittleEndian.PutUint16(prefix[54:56], uint16(len(nameBytes)))
		binary.LittleEndian.PutUint16(prefix[56:58], uint16(len(ext.ExtraTLVs)))

		if _, err := w.Write(prefix[:]); err != nil {
			return total, err
		}
		if _, err := w.Write(varints); err != nil {
			return total, err
		}
		if len(targetBytes) > 0 {
			if _, err := w.Write(targetBytes); err != nil {
				return total, err
			}
		}
		if len(nameBytes) > 0 {
			if _, err := w.Write(nameBytes); err != nil {
				return total, err
			}
		}
		if len(ext.ExtraTLVs) > 0 {
			if _, err := w.Write(ext.ExtraTLVs); err != nil {
				return total, err
			}
		}
		total += int64(recLen)
	}

	return total, nil
}

// ReadIndex parses the INDX catalog from r and returns the parsed Index alongside total byte length on disk.
func ReadIndex(r io.Reader) (*Index, int64, error) {
	br, ok := r.(io.ByteReader)
	reader := r
	if !ok {
		buf := bufio.NewReader(r)
		br = buf
		reader = buf
	}

	var hdr [spec.INDXHeaderSize]byte
	if _, err := io.ReadFull(reader, hdr[:]); err != nil {
		return nil, 0, err
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != spec.MagicINDXLE {
		return nil, 0, spec.ErrInvalidINDX
	}

	totalExtents := binary.LittleEndian.Uint32(hdr[4:8])
	if totalExtents > 50_000_000 {
		return nil, 0, fmt.Errorf("%w: extent count (%d) exceeds limit", spec.ErrInvalidINDX, totalExtents)
	}
	initialCap := totalExtents
	if initialCap > 1_000_000 {
		initialCap = 1_000_000
	}
	extents := make([]Extent, 0, initialCap)
	var totalBytes int64 = spec.INDXHeaderSize

	for i := uint32(0); i < totalExtents; i++ {
		var prefix [spec.FixedExtentPrefixSize]byte
		if _, err := io.ReadFull(reader, prefix[:]); err != nil {
			return nil, 0, fmt.Errorf("read extent prefix %d: %w", i, err)
		}

		recLen := int64(binary.LittleEndian.Uint16(prefix[0:2]))
		if recLen < spec.FixedExtentPrefixSize {
			return nil, 0, fmt.Errorf("%w: record length %d too small", spec.ErrInvalidINDX, recLen)
		}
		totalBytes += recLen

		ns := binary.LittleEndian.Uint16(prefix[2:4])
		chunkIdx := binary.LittleEndian.Uint32(prefix[4:8])
		chunkCount := binary.LittleEndian.Uint16(prefix[8:10])
		extFlags := binary.LittleEndian.Uint16(prefix[10:12])
		ts := int64(binary.LittleEndian.Uint64(prefix[12:20]))
		var hash [32]byte
		copy(hash[:], prefix[20:52])
		targetLen := binary.LittleEndian.Uint16(prefix[52:54])
		nameLen := binary.LittleEndian.Uint16(prefix[54:56])
		extraLen := binary.LittleEndian.Uint16(prefix[56:58])

		chunkOffset, err := spec.ReadULEB128(br)
		if err != nil {
			return nil, 0, fmt.Errorf("read chunk offset varint: %w", err)
		}
		payloadSize, err := spec.ReadULEB128(br)
		if err != nil {
			return nil, 0, fmt.Errorf("read payload size varint: %w", err)
		}

		targetBytes := make([]byte, targetLen)
		if targetLen > 0 {
			if _, err := io.ReadFull(reader, targetBytes); err != nil {
				return nil, 0, fmt.Errorf("read target: %w", err)
			}
		}

		nameBytes := make([]byte, nameLen)
		if nameLen > 0 {
			if _, err := io.ReadFull(reader, nameBytes); err != nil {
				return nil, 0, fmt.Errorf("read name: %w", err)
			}
		}

		extraBytes := make([]byte, extraLen)
		if extraLen > 0 {
			if _, err := io.ReadFull(reader, extraBytes); err != nil {
				return nil, 0, fmt.Errorf("read extra: %w", err)
			}
		}

		extents = append(extents, Extent{
			Namespace:   ns,
			ChunkIndex:  chunkIdx,
			ChunkCount:  chunkCount,
			ExtentFlags: extFlags,
			Timestamp:   ts,
			Hash:        hash,
			Target:      string(targetBytes),
			Name:        string(nameBytes),
			ExtraTLVs:   extraBytes,
			ChunkOffset: chunkOffset,
			PayloadSize: payloadSize,
		})
	}

	return &Index{Extents: extents}, totalBytes, nil
}

// Sort orders extents according to the canonical specification:
// Primary key: Namespace ASC; Secondary key: Name ASC (UTF-8 binary comparison).
func (idx *Index) Sort() {
	slices.SortFunc(idx.Extents, func(a, b Extent) int {
		if a.Namespace != b.Namespace {
			return cmp.Compare(a.Namespace, b.Namespace)
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// BinarySearch performs O(log N) lookup for an extent matching (namespace, name) on a canonically sorted index.
func (idx *Index) BinarySearch(namespace uint16, name string) (Extent, bool) {
	i, found := slices.BinarySearchFunc(
		idx.Extents,
		Extent{Namespace: namespace, Name: name},
		func(e, target Extent) int {
			if e.Namespace != target.Namespace {
				return cmp.Compare(e.Namespace, target.Namespace)
			}
			return strings.Compare(e.Name, target.Name)
		},
	)
	if found {
		return idx.Extents[i], true
	}
	return Extent{}, false
}

// SetChunkList encodes a non-contiguous sequence of chunk indices into the TagChunkRuns or TagChunkList TLV
// and sets the ExtentFlagChunkList flag.
func (e *Extent) SetChunkList(chunks []uint32) {
	if len(chunks) == 0 {
		return
	}
	e.ExtentFlags |= spec.ExtentFlagChunkList
	e.ChunkIndex = chunks[0]
	if len(chunks) > 0xFFFF {
		e.ChunkCount = 0xFFFF
	} else {
		e.ChunkCount = uint16(len(chunks))
	}

	// Calculate contiguous runs
	type chunkRun struct {
		start uint32
		count uint32
	}
	var runs []chunkRun
	currentRun := chunkRun{start: chunks[0], count: 1}
	for i := 1; i < len(chunks); i++ {
		if chunks[i] == chunks[i-1]+1 {
			currentRun.count++
		} else {
			runs = append(runs, currentRun)
			currentRun = chunkRun{start: chunks[i], count: 1}
		}
	}
	runs = append(runs, currentRun)

	rawListSize := len(chunks) * 4
	runsSize := len(runs) * 8

	e.removeTLV(spec.TagChunkList)
	e.removeTLV(spec.TagChunkRuns)

	if runsSize < rawListSize || rawListSize > 60000 {
		// Use compact TagChunkRuns
		tlv := make([]byte, 4+runsSize)
		binary.LittleEndian.PutUint16(tlv[0:2], spec.TagChunkRuns)
		binary.LittleEndian.PutUint16(tlv[2:4], uint16(runsSize))
		for i, r := range runs {
			binary.LittleEndian.PutUint32(tlv[4+i*8:8+i*8], r.start)
			binary.LittleEndian.PutUint32(tlv[8+i*8:12+i*8], r.count)
		}
		e.ExtraTLVs = append(e.ExtraTLVs, tlv...)
	} else {
		// Use TagChunkList
		tlv := make([]byte, 4+rawListSize)
		binary.LittleEndian.PutUint16(tlv[0:2], spec.TagChunkList)
		binary.LittleEndian.PutUint16(tlv[2:4], uint16(rawListSize))
		for i, c := range chunks {
			binary.LittleEndian.PutUint32(tlv[4+i*4:8+i*4], c)
		}
		e.ExtraTLVs = append(e.ExtraTLVs, tlv...)
	}
}

// GetChunkList retrieves the discrete sequence of chunk indices.
// If ExtentFlagChunkList is set, it extracts them from the TagChunkRuns or TagChunkList TLV.
// Otherwise, it returns the contiguous sequence [ChunkIndex .. ChunkIndex+ChunkCount-1].
func (e *Extent) GetChunkList() []uint32 {
	count := int(e.ChunkCount)
	if count <= 0 {
		count = 1
	}

	if (e.ExtentFlags & spec.ExtentFlagChunkList) != 0 {
		tlvs := e.ExtraTLVs
		for len(tlvs) >= 4 {
			tag := binary.LittleEndian.Uint16(tlvs[0:2])
			tLen := int(binary.LittleEndian.Uint16(tlvs[2:4]))
			if len(tlvs) < 4+tLen {
				break
			}
			if tag == spec.TagChunkRuns {
				data := tlvs[4 : 4+tLen]
				numRuns := len(data) / 8
				var totalChunks int
				for i := 0; i < numRuns; i++ {
					totalChunks += int(binary.LittleEndian.Uint32(data[i*8+4 : (i+1)*8]))
				}
				res := make([]uint32, 0, totalChunks)
				for i := 0; i < numRuns; i++ {
					start := binary.LittleEndian.Uint32(data[i*8 : i*8+4])
					cCount := binary.LittleEndian.Uint32(data[i*8+4 : (i+1)*8])
					for c := uint32(0); c < cCount; c++ {
						res = append(res, start+c)
					}
				}
				return res
			}
			if tag == spec.TagChunkList {
				data := tlvs[4 : 4+tLen]
				numChunks := len(data) / 4
				res := make([]uint32, numChunks)
				for i := 0; i < numChunks; i++ {
					res[i] = binary.LittleEndian.Uint32(data[i*4 : (i+1)*4])
				}
				return res
			}
			tlvs = tlvs[4+tLen:]
		}
	}

	res := make([]uint32, count)
	for i := 0; i < count; i++ {
		res[i] = e.ChunkIndex + uint32(i)
	}
	return res
}

func (e *Extent) removeTLV(targetTag uint16) {
	if len(e.ExtraTLVs) == 0 {
		return
	}
	var filtered []byte
	tlvs := e.ExtraTLVs
	for len(tlvs) >= 4 {
		tag := binary.LittleEndian.Uint16(tlvs[0:2])
		tLen := int(binary.LittleEndian.Uint16(tlvs[2:4]))
		if len(tlvs) < 4+tLen {
			break
		}
		if tag != targetTag {
			filtered = append(filtered, tlvs[:4+tLen]...)
		}
		tlvs = tlvs[4+tLen:]
	}
	e.ExtraTLVs = filtered
}
