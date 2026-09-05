// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prolly

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/lemon4ksan/seal/container/eon/spec"
)

// Snapshot records an immutable point-in-time state of the container.
type Snapshot struct {
	ID         string
	ParentID   string
	Timestamp  int64
	Message    string
	RootHash   [32]byte
	FileCount  uint64
	TotalBytes uint64
}

// SnapshotTable maintains the versioned history of snapshots in the container.
type SnapshotTable struct {
	Snapshots []Snapshot
}

// NewSnapshotTable constructs an empty snapshot table.
func NewSnapshotTable() *SnapshotTable {
	return &SnapshotTable{}
}

// Add appends a new snapshot to the history.
func (st *SnapshotTable) Add(s Snapshot) {
	st.Snapshots = append(st.Snapshots, s)
}

// Latest returns the most recent snapshot or nil if empty.
func (st *SnapshotTable) Latest() *Snapshot {
	if len(st.Snapshots) == 0 {
		return nil
	}
	return &st.Snapshots[len(st.Snapshots)-1]
}

// Find looks up a snapshot by full ID or prefix.
func (st *SnapshotTable) Find(idPrefix string) (*Snapshot, bool) {
	for i := range st.Snapshots {
		s := &st.Snapshots[i]
		if s.ID == idPrefix || (len(idPrefix) >= 6 && len(s.ID) >= len(idPrefix) && s.ID[:len(idPrefix)] == idPrefix) {
			return s, true
		}
	}
	return nil, false
}

// Write serializes the snapshot table into w with binary magic 'SNPS'.
func (st *SnapshotTable) Write(w io.Writer) (int64, error) {
	var buf bytes.Buffer

	// 8-byte header: Magic (4B), Version (2B), Count (2B)
	var hdr [8]byte
	copy(hdr[0:4], spec.MagicSNPS[:])
	binary.LittleEndian.PutUint16(hdr[4:6], 1) // version 1
	binary.LittleEndian.PutUint16(hdr[6:8], uint16(len(st.Snapshots)))
	buf.Write(hdr[:])

	for _, s := range st.Snapshots {
		idBytes := []byte(s.ID)
		pBytes := []byte(s.ParentID)
		msgBytes := []byte(s.Message)

		var prefix [62]byte
		binary.LittleEndian.PutUint16(prefix[0:2], uint16(len(idBytes)))
		binary.LittleEndian.PutUint16(prefix[2:4], uint16(len(pBytes)))
		binary.LittleEndian.PutUint16(prefix[4:6], uint16(len(msgBytes)))
		binary.LittleEndian.PutUint64(prefix[6:14], uint64(s.Timestamp))
		copy(prefix[14:46], s.RootHash[:])
		binary.LittleEndian.PutUint64(prefix[46:54], s.FileCount)
		binary.LittleEndian.PutUint64(prefix[54:62], s.TotalBytes)
		buf.Write(prefix[:])

		buf.Write(idBytes)
		buf.Write(pBytes)
		buf.Write(msgBytes)
	}

	n, err := w.Write(buf.Bytes())
	return int64(n), err
}

// ReadSnapshotTable deserializes a snapshot table from r.
func ReadSnapshotTable(r io.Reader) (*SnapshotTable, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != spec.MagicSNPSLE {
		return nil, errors.New("prolly: invalid snapshot table magic")
	}

	count := int(binary.LittleEndian.Uint16(hdr[6:8]))
	st := &SnapshotTable{
		Snapshots: make([]Snapshot, 0, count),
	}

	for i := 0; i < count; i++ {
		var prefix [62]byte
		if _, err := io.ReadFull(r, prefix[:]); err != nil {
			return nil, fmt.Errorf("read snapshot record %d: %w", i, err)
		}

		idLen := int(binary.LittleEndian.Uint16(prefix[0:2]))
		pLen := int(binary.LittleEndian.Uint16(prefix[2:4]))
		msgLen := int(binary.LittleEndian.Uint16(prefix[4:6]))
		ts := int64(binary.LittleEndian.Uint64(prefix[6:14]))
		var rootHash [32]byte
		copy(rootHash[:], prefix[14:46])
		fc := binary.LittleEndian.Uint64(prefix[46:54])
		tb := binary.LittleEndian.Uint64(prefix[54:62])

		strBuf := make([]byte, idLen+pLen+msgLen)
		if _, err := io.ReadFull(r, strBuf); err != nil {
			return nil, fmt.Errorf("read snapshot strings %d: %w", i, err)
		}

		id := string(strBuf[:idLen])
		parentID := string(strBuf[idLen : idLen+pLen])
		msg := string(strBuf[idLen+pLen:])

		st.Snapshots = append(st.Snapshots, Snapshot{
			ID:         id,
			ParentID:   parentID,
			Timestamp:  ts,
			Message:    msg,
			RootHash:   rootHash,
			FileCount:  fc,
			TotalBytes: tb,
		})
	}

	return st, nil
}
