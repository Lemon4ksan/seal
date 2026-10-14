// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prolly

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"sync"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

const (
	// DefaultMinItems is the minimum number of items in a non-root node before boundary cuts are considered.
	DefaultMinItems = 16
	// DefaultMaxItems is the hard maximum number of items in a node before an unconditional cut.
	DefaultMaxItems = 128
	// BoundaryMask represents average 64 items per node (prob 1/64).
	BoundaryMask = 0x3F
)

// NodeStore retrieves and persists immutable content-addressed nodes.
type NodeStore interface {
	GetNode(hash [32]byte) (*Node, error)
	PutNode(node *Node) error
}

// MemoryNodeStore is an in-memory thread-safe node repository.
type MemoryNodeStore struct {
	mu    sync.RWMutex
	nodes map[[32]byte]*Node
}

// NewMemoryNodeStore constructs an in-memory node store.
func NewMemoryNodeStore() *MemoryNodeStore {
	return &MemoryNodeStore{
		nodes: make(map[[32]byte]*Node),
	}
}

// GetNode retrieves a node by its SHA-256 content hash.
func (m *MemoryNodeStore) GetNode(hash [32]byte) (*Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, ok := m.nodes[hash]
	if !ok {
		return nil, fmt.Errorf("prolly: node not found: %x", hash)
	}
	return n, nil
}

// PutNode serializes (if needed) and stores a node.
func (m *MemoryNodeStore) PutNode(node *Node) error {
	if len(node.Serialized) == 0 {
		if _, err := node.Serialize(); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes[node.Hash] = node
	return nil
}

// Nodes returns all nodes currently held in store.
func (m *MemoryNodeStore) Nodes() []*Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		res = append(res, n)
	}
	return res
}

// ReadTreeNodes reads a block of serialized tree nodes from r and stores them in store.
func ReadTreeNodes(r io.Reader, store NodeStore) (int, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, err
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != spec.MagicTREELE {
		return 0, errors.New("prolly: invalid tree nodes magic")
	}

	count := int(binary.LittleEndian.Uint32(hdr[4:8]))
	for i := 0; i < count; i++ {
		var szBuf [4]byte
		if _, err := io.ReadFull(r, szBuf[:]); err != nil {
			return i, fmt.Errorf("read node size %d: %w", i, err)
		}
		nodeLen := int(binary.LittleEndian.Uint32(szBuf[:]))
		nodeData := make([]byte, nodeLen)
		if _, err := io.ReadFull(r, nodeData); err != nil {
			return i, fmt.Errorf("read node data %d: %w", i, err)
		}
		node, err := DeserializeNode(nodeData)
		if err != nil {
			return i, fmt.Errorf("deserialize node %d: %w", i, err)
		}
		if err := store.PutNode(node); err != nil {
			return i, err
		}
	}
	return count, nil
}

// Tree represents a content-addressed Prolly Tree.
type Tree struct {
	RootHash  [32]byte
	Store     NodeStore
	ItemCount uint64
}

// isBoundary determines if item represents a probabilistic node split point.
func isBoundary(item Item) bool {
	var nsBuf [2]byte
	binary.LittleEndian.PutUint16(nsBuf[:], item.Namespace)
	h := crc32.ChecksumIEEE(nsBuf[:])
	h = crc32.Update(h, crc32.IEEETable, []byte(item.Name))
	return (h & BoundaryMask) == 0
}

// BuildTree constructs a balanced Prolly Tree from sorted extents and saves nodes into store.
func BuildTree(extents []catalog.Extent, store NodeStore) (*Tree, error) {
	if len(extents) == 0 {
		// Empty tree: single empty leaf node
		emptyNode := &Node{Level: 0}
		if _, err := emptyNode.Serialize(); err != nil {
			return nil, err
		}
		if err := store.PutNode(emptyNode); err != nil {
			return nil, err
		}
		return &Tree{
			RootHash:  emptyNode.Hash,
			Store:     store,
			ItemCount: 0,
		}, nil
	}

	// 1. Build Level 0 Leaf Nodes
	var leafNodes []*Node
	var currentItems []Item

	for i, ext := range extents {
		extCopy := ext
		it := Item{
			Namespace: ext.Namespace,
			Name:      ext.Name,
			Extent:    &extCopy,
		}
		currentItems = append(currentItems, it)

		isLast := (i == len(extents)-1)
		shouldCut := false

		if len(currentItems) >= DefaultMaxItems {
			shouldCut = true
		} else if len(currentItems) >= DefaultMinItems && isBoundary(it) {
			shouldCut = true
		}

		if (shouldCut && !isLast) || (isLast && len(currentItems) > 0) {
			node := &Node{
				Level: 0,
				Items: currentItems,
			}
			if _, err := node.Serialize(); err != nil {
				return nil, err
			}
			if err := store.PutNode(node); err != nil {
				return nil, err
			}
			leafNodes = append(leafNodes, node)
			currentItems = nil
		}
	}

	// 2. Build Higher Level Internal Nodes recursively
	currentLevelNodes := leafNodes
	level := uint8(1)

	for len(currentLevelNodes) > 1 {
		var nextLevelNodes []*Node
		var parentItems []Item

		for i, child := range currentLevelNodes {
			lastNS, lastName := child.Key()
			parentIt := Item{
				Namespace: lastNS,
				Name:      lastName,
				ChildHash: child.Hash,
			}
			parentItems = append(parentItems, parentIt)

			isLast := (i == len(currentLevelNodes)-1)
			shouldCut := false

			if len(parentItems) >= DefaultMaxItems {
				shouldCut = true
			} else if len(parentItems) >= DefaultMinItems && isBoundary(parentIt) {
				shouldCut = true
			}

			if (shouldCut && !isLast) || (isLast && len(parentItems) > 0) {
				parentNode := &Node{
					Level: level,
					Items: parentItems,
				}
				if _, err := parentNode.Serialize(); err != nil {
					return nil, err
				}
				if err := store.PutNode(parentNode); err != nil {
					return nil, err
				}
				nextLevelNodes = append(nextLevelNodes, parentNode)
				parentItems = nil
			}
		}

		currentLevelNodes = nextLevelNodes
		level++
	}

	root := currentLevelNodes[0]
	return &Tree{
		RootHash:  root.Hash,
		Store:     store,
		ItemCount: uint64(len(extents)),
	}, nil
}

// Lookup finds an extent by (namespace, name) with O(log N) node accesses.
func (t *Tree) Lookup(namespace uint16, name string) (catalog.Extent, bool, error) {
	currHash := t.RootHash

	for {
		node, err := t.Store.GetNode(currHash)
		if err != nil {
			return catalog.Extent{}, false, fmt.Errorf("prolly lookup get node %x: %w", currHash, err)
		}

		if len(node.Items) == 0 {
			return catalog.Extent{}, false, nil
		}

		idx, exact := node.BinarySearch(namespace, name)

		if node.Level == 0 {
			if exact {
				return *node.Items[idx].Extent, true, nil
			}
			return catalog.Extent{}, false, nil
		}

		// Internal node: route to child pointer
		// If exact match or idx is within bounds:
		childIdx := idx
		if childIdx >= len(node.Items) {
			childIdx = len(node.Items) - 1
		}

		currHash = node.Items[childIdx].ChildHash
	}
}

// CollectExtents walks the entire tree in canonical order and gathers all extents.
func (t *Tree) CollectExtents() ([]catalog.Extent, error) {
	var result []catalog.Extent
	var walk func(hash [32]byte) error

	walk = func(hash [32]byte) error {
		node, err := t.Store.GetNode(hash)
		if err != nil {
			return err
		}
		if node.Level == 0 {
			for _, it := range node.Items {
				if it.Extent != nil {
					result = append(result, *it.Extent)
				}
			}
			return nil
		}
		for _, it := range node.Items {
			if err := walk(it.ChildHash); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(t.RootHash); err != nil {
		return nil, err
	}
	return result, nil
}
