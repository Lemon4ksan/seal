// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prolly

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"slices"
	"strings"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

// Item represents a single entry in a Prolly Tree node.
// At Level 0 (leaf), Extent is non-nil.
// At Level > 0 (internal), ChildHash points to the descendant node.
type Item struct {
	Namespace uint16
	Name      string
	Extent    *catalog.Extent
	ChildHash [32]byte
}

// CompareItems orders items canonically: Namespace ASC, Name ASC.
func CompareItems(a, b Item) int {
	if a.Namespace != b.Namespace {
		return cmp.Compare(a.Namespace, b.Namespace)
	}
	return strings.Compare(a.Name, b.Name)
}

// CompareKey compares an item against a search key (namespace, name).
func CompareKey(it Item, ns uint16, name string) int {
	if it.Namespace != ns {
		return cmp.Compare(it.Namespace, ns)
	}
	return strings.Compare(it.Name, name)
}

// Node represents an immutable, content-addressed node in a Prolly Tree.
type Node struct {
	Level      uint8
	Items      []Item
	Hash       [32]byte
	Serialized []byte
}

// Key returns the boundary key of this node (the key of its last item).
func (n *Node) Key() (uint16, string) {
	if len(n.Items) == 0 {
		return 0, ""
	}
	last := n.Items[len(n.Items)-1]
	return last.Namespace, last.Name
}

// Serialize encodes the node into its canonical binary representation and sets n.Hash.
func (n *Node) Serialize() ([]byte, error) {
	if len(n.Items) > 65535 {
		return nil, errors.New("prolly: too many items in node")
	}

	var buf bytes.Buffer

	// 8-byte header: Magic (4B), Level (1B), Reserved (1B), ItemCount (2B)
	var hdr [8]byte
	copy(hdr[0:4], spec.MagicTREE[:])
	hdr[4] = n.Level
	hdr[5] = 0 // Reserved
	binary.LittleEndian.PutUint16(hdr[6:8], uint16(len(n.Items)))
	buf.Write(hdr[:])

	for _, it := range n.Items {
		nameBytes := []byte(it.Name)
		var itHdr [4]byte
		binary.LittleEndian.PutUint16(itHdr[0:2], it.Namespace)
		binary.LittleEndian.PutUint16(itHdr[2:4], uint16(len(nameBytes)))
		buf.Write(itHdr[:])
		buf.Write(nameBytes)

		if n.Level == 0 {
			if it.Extent == nil {
				return nil, errors.New("prolly: leaf item missing extent")
			}
			ext := it.Extent
			targetBytes := []byte(ext.Target)

			var extBuf [66]byte
			binary.LittleEndian.PutUint32(extBuf[0:4], ext.ChunkIndex)
			binary.LittleEndian.PutUint16(extBuf[4:6], ext.ChunkCount)
			binary.LittleEndian.PutUint16(extBuf[6:8], ext.ExtentFlags)
			binary.LittleEndian.PutUint64(extBuf[8:16], uint64(ext.Timestamp))
			binary.LittleEndian.PutUint64(extBuf[16:24], ext.ChunkOffset)
			binary.LittleEndian.PutUint64(extBuf[24:32], ext.PayloadSize)
			copy(extBuf[32:64], ext.Hash[:])
			binary.LittleEndian.PutUint16(extBuf[64:66], uint16(len(targetBytes)))
			buf.Write(extBuf[:])
			buf.Write(targetBytes)

			var tlvHdr [2]byte
			binary.LittleEndian.PutUint16(tlvHdr[0:2], uint16(len(ext.ExtraTLVs)))
			buf.Write(tlvHdr[:])
			buf.Write(ext.ExtraTLVs)
		} else {
			buf.Write(it.ChildHash[:])
		}
	}

	// 4-byte CRC32 checksum
	raw := buf.Bytes()
	crc := crc32.ChecksumIEEE(raw)
	var crcBuf [4]byte
	binary.LittleEndian.PutUint32(crcBuf[:], crc)
	buf.Write(crcBuf[:])

	serialized := buf.Bytes()
	n.Serialized = serialized
	n.Hash = sha256.Sum256(serialized)
	return serialized, nil
}

// DeserializeNode decodes a node from its canonical binary representation.
func DeserializeNode(data []byte) (*Node, error) {
	if len(data) < 12 {
		return nil, errors.New("prolly: node data too short")
	}

	// Check CRC32
	contentLen := len(data) - 4
	expectedCRC := binary.LittleEndian.Uint32(data[contentLen:])
	actualCRC := crc32.ChecksumIEEE(data[:contentLen])
	if expectedCRC != actualCRC {
		return nil, fmt.Errorf("prolly: node crc32 mismatch: expected 0x%08X, got 0x%08X", expectedCRC, actualCRC)
	}

	if binary.LittleEndian.Uint32(data[0:4]) != spec.MagicTREELE {
		return nil, errors.New("prolly: invalid node magic")
	}

	level := data[4]
	itemCount := int(binary.LittleEndian.Uint16(data[6:8]))

	items := make([]Item, 0, itemCount)
	offset := 8

	for i := 0; i < itemCount; i++ {
		if offset+4 > contentLen {
			return nil, errors.New("prolly: unexpected eof reading item header")
		}
		ns := binary.LittleEndian.Uint16(data[offset : offset+2])
		nameLen := int(binary.LittleEndian.Uint16(data[offset+2 : offset+4]))
		offset += 4

		if offset+nameLen > contentLen {
			return nil, errors.New("prolly: unexpected eof reading item name")
		}
		name := string(data[offset : offset+nameLen])
		offset += nameLen

		item := Item{
			Namespace: ns,
			Name:      name,
		}

		if level == 0 {
			if offset+66 > contentLen {
				return nil, errors.New("prolly: unexpected eof reading leaf extent")
			}
			chunkIdx := binary.LittleEndian.Uint32(data[offset : offset+4])
			chunkCount := binary.LittleEndian.Uint16(data[offset+4 : offset+6])
			flags := binary.LittleEndian.Uint16(data[offset+6 : offset+8])
			ts := int64(binary.LittleEndian.Uint64(data[offset+8 : offset+16]))
			chunkOff := binary.LittleEndian.Uint64(data[offset+16 : offset+24])
			payloadSz := binary.LittleEndian.Uint64(data[offset+24 : offset+32])
			var h [32]byte
			copy(h[:], data[offset+32:offset+64])
			targetLen := int(binary.LittleEndian.Uint16(data[offset+64 : offset+66]))
			offset += 66

			if offset+targetLen > contentLen {
				return nil, errors.New("prolly: unexpected eof reading target")
			}
			target := string(data[offset : offset+targetLen])
			offset += targetLen

			if offset+2 > contentLen {
				return nil, errors.New("prolly: unexpected eof reading tlv length")
			}
			tlvLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
			offset += 2

			if offset+tlvLen > contentLen {
				return nil, errors.New("prolly: unexpected eof reading extra tlvs")
			}
			extraTLVs := make([]byte, tlvLen)
			copy(extraTLVs, data[offset:offset+tlvLen])
			offset += tlvLen

			item.Extent = &catalog.Extent{
				Namespace:   ns,
				ChunkIndex:  chunkIdx,
				ChunkCount:  chunkCount,
				ExtentFlags: flags,
				Timestamp:   ts,
				ChunkOffset: chunkOff,
				PayloadSize: payloadSz,
				Hash:        h,
				Target:      target,
				Name:        name,
				ExtraTLVs:   extraTLVs,
			}
		} else {
			if offset+32 > contentLen {
				return nil, errors.New("prolly: unexpected eof reading child hash")
			}
			copy(item.ChildHash[:], data[offset:offset+32])
			offset += 32
		}

		items = append(items, item)
	}

	node := &Node{
		Level:      level,
		Items:      items,
		Hash:       sha256.Sum256(data),
		Serialized: data,
	}
	return node, nil
}

// BinarySearch looks up an item matching (namespace, name) within the node.
// Returns the index and whether an exact match was found.
// If exact match not found, returns the insertion/branch index.
func (n *Node) BinarySearch(ns uint16, name string) (int, bool) {
	return slices.BinarySearchFunc(n.Items, Item{Namespace: ns, Name: name}, CompareItems)
}
