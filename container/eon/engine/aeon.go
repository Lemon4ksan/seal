// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package aeon is the reference engine and SDK for the EON Container Format (.eon).
package engine

import (
	"io"

	"github.com/lemon4ksan/seal/container/eon/vfs"
)

// Version is the current semver release of the Aeon engine.
const Version = "0.1.0"

// Pack convenience helper creates an EON container archive from a map of relative paths to byte payloads.
func Pack(w io.Writer, files map[string][]byte, opts ...WriterOption) error {
	wr := NewWriter(w, opts...)
	for name, data := range files {
		if err := wr.AddFile(name, data); err != nil {
			return err
		}
	}
	return wr.Close()
}

// OpenFS returns an io/fs.FS virtual filesystem backed by the EON container.
func OpenFS(rAt io.ReaderAt, size int64) (*vfs.FileSystem, error) {
	r, err := OpenReader(rAt, size)
	if err != nil {
		return nil, err
	}
	return vfs.New(r), nil
}
