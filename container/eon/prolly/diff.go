// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prolly

import (
	"bytes"

	"github.com/lemon4ksan/seal/container/eon/catalog"
)

// ExtentModification describes an extent modified between two snapshots.
type ExtentModification struct {
	Old catalog.Extent
	New catalog.Extent
}

// DiffResult holds granular changes detected between two trees.
type DiffResult struct {
	Added    []catalog.Extent
	Modified []ExtentModification
	Deleted  []catalog.Extent
}

// TotalChanges returns the sum of additions, modifications, and deletions.
func (d *DiffResult) TotalChanges() int {
	return len(d.Added) + len(d.Modified) + len(d.Deleted)
}

// Diff compares two Prolly Trees in O(Δ) time, skipping identical subtrees via content hashes.
func Diff(treeA, treeB *Tree) (*DiffResult, error) {
	res := &DiffResult{}
	if treeA.RootHash == treeB.RootHash {
		return res, nil
	}

	nodeA, err := treeA.Store.GetNode(treeA.RootHash)
	if err != nil {
		return nil, err
	}
	nodeB, err := treeB.Store.GetNode(treeB.RootHash)
	if err != nil {
		return nil, err
	}

	differ := &treeDiffer{
		storeA: treeA.Store,
		storeB: treeB.Store,
		res:    res,
	}

	if err := differ.diff(nodeA, nodeB); err != nil {
		return nil, err
	}
	return res, nil
}

type treeDiffer struct {
	storeA NodeStore
	storeB NodeStore
	res    *DiffResult
}

func (d *treeDiffer) diff(a, b *Node) error {
	// Subtree hash equality: O(1) prune!
	if a.Hash == b.Hash {
		return nil
	}

	// Case 1: Both nodes are Level 0 leaves
	if a.Level == 0 && b.Level == 0 {
		return d.diffLeaves(a, b)
	}

	// Case 2: Different levels (e.g. tree B grew higher than tree A)
	if a.Level > b.Level {
		for _, itA := range a.Items {
			childA, err := d.storeA.GetNode(itA.ChildHash)
			if err != nil {
				return err
			}
			if err := d.diff(childA, b); err != nil {
				return err
			}
		}
		return nil
	}
	if b.Level > a.Level {
		for _, itB := range b.Items {
			childB, err := d.storeB.GetNode(itB.ChildHash)
			if err != nil {
				return err
			}
			if err := d.diff(a, childB); err != nil {
				return err
			}
		}
		return nil
	}

	// Case 3: Both nodes are internal nodes at the same Level > 0
	return d.diffInternals(a, b)
}

func (d *treeDiffer) diffLeaves(a, b *Node) error {
	var i, j int
	for i < len(a.Items) && j < len(b.Items) {
		cmp := CompareItems(a.Items[i], b.Items[j])
		if cmp < 0 {
			// Item in A but not in B: Deleted
			if a.Items[i].Extent != nil {
				d.res.Deleted = append(d.res.Deleted, *a.Items[i].Extent)
			}
			i++
		} else if cmp > 0 {
			// Item in B but not in A: Added
			if b.Items[j].Extent != nil {
				d.res.Added = append(d.res.Added, *b.Items[j].Extent)
			}
			j++
		} else {
			// Same key (namespace, name)
			extA, extB := a.Items[i].Extent, b.Items[j].Extent
			if extA != nil && extB != nil {
				if extA.PayloadSize != extB.PayloadSize || extA.Hash != extB.Hash || !bytes.Equal(extA.ExtraTLVs, extB.ExtraTLVs) {
					d.res.Modified = append(d.res.Modified, ExtentModification{
						Old: *extA,
						New: *extB,
					})
				}
			}
			i++
			j++
		}
	}

	for i < len(a.Items) {
		if a.Items[i].Extent != nil {
			d.res.Deleted = append(d.res.Deleted, *a.Items[i].Extent)
		}
		i++
	}
	for j < len(b.Items) {
		if b.Items[j].Extent != nil {
			d.res.Added = append(d.res.Added, *b.Items[j].Extent)
		}
		j++
	}
	return nil
}

func (d *treeDiffer) diffInternals(a, b *Node) error {
	// Walk and match internal node pointers
	for _, itA := range a.Items {
		childA, err := d.storeA.GetNode(itA.ChildHash)
		if err != nil {
			return err
		}

		// Find corresponding child or range in b
		idx, exact := b.BinarySearch(itA.Namespace, itA.Name)
		childIdx := idx
		if childIdx >= len(b.Items) {
			childIdx = len(b.Items) - 1
		}

		itB := b.Items[childIdx]
		if exact && itA.ChildHash == itB.ChildHash {
			// Identical subtree: skip!
			continue
		}

		childB, err := d.storeB.GetNode(itB.ChildHash)
		if err != nil {
			return err
		}

		if err := d.diff(childA, childB); err != nil {
			return err
		}
	}
	return nil
}
