// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ar

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

// Writer serializes file entries into a standard Unix AR archive or Debian package.
type Writer struct {
	w io.Writer
}

// NewWriter constructs an AR archive serializer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteArchive serializes the slice of files into a standard Unix AR archive.
func (aw *Writer) WriteArchive(files []*file.File, cfg container.Config) (int64, error) {
	var totalWritten int64

	// Write global archive magic: "!<arch>\n"
	n, err := io.WriteString(aw.w, Magic)
	totalWritten += int64(n)
	if err != nil {
		return totalWritten, err
	}

	type memberInfo struct {
		f        *file.File
		name     string
		nameHdr  string
		data     []byte
		size     int64
		mtimeSec int64
		mode     int64
	}

	var members []memberInfo
	var stringTable bytes.Buffer

	// Check if string table is needed for long filenames (> 15 chars)
	for _, f := range files {
		cleanName := strings.TrimPrefix(f.Name(), "/")
		if cleanName == "" || f.IsDir() {
			continue
		}

		var data []byte
		if f.UncompressedSize() > 0 {
			r, err := f.Open()
			if err != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(f, err)
				}
				return totalWritten, fmt.Errorf("ar: open file %q: %w", f.Name(), err)
			}
			buf, err := io.ReadAll(r)
			_ = r.Close()
			if err != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(f, err)
				}
				return totalWritten, fmt.Errorf("ar: read file %q: %w", f.Name(), err)
			}
			data = buf
		}

		mtime := f.ModTime().Unix()
		if mtime < 0 {
			mtime = 0
		}

		mode := int64(f.Mode().Perm())
		if mode == 0 {
			mode = 0o644
		}

		m := memberInfo{
			f:        f,
			name:     cleanName,
			data:     data,
			size:     int64(len(data)),
			mtimeSec: mtime,
			mode:     mode,
		}

		if len(cleanName) <= 15 {
			m.nameHdr = cleanName + "/"
		} else {
			offset := stringTable.Len()
			stringTable.WriteString(cleanName + "/\n")
			m.nameHdr = fmt.Sprintf("/%d", offset)
		}

		members = append(members, m)
	}

	// 1. If long filenames exist, write GNU String Table ("//")
	if stringTable.Len() > 0 {
		stBytes := stringTable.Bytes()
		stSize := int64(len(stBytes))

		hdr := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n",
			"//",
			0, // mtime
			0, // uid
			0, // gid
			0, // mode
			stSize,
		)

		n, err := io.WriteString(aw.w, hdr)
		totalWritten += int64(n)
		if err != nil {
			return totalWritten, err
		}

		n, err = aw.w.Write(stBytes)
		totalWritten += int64(n)
		if err != nil {
			return totalWritten, err
		}

		if stSize%2 != 0 {
			n, err := aw.w.Write([]byte{'\n'})
			totalWritten += int64(n)
			if err != nil {
				return totalWritten, err
			}
		}
	}

	// 2. Write archive members
	for _, m := range members {
		hdr := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n",
			m.nameHdr,
			m.mtimeSec,
			0, // uid
			0, // gid
			m.mode,
			m.size,
		)

		if len(hdr) != HeaderSize {
			return totalWritten, fmt.Errorf("ar: internal error: header size %d != %d", len(hdr), HeaderSize)
		}

		n, err := io.WriteString(aw.w, hdr)
		totalWritten += int64(n)
		if err != nil {
			return totalWritten, err
		}

		if m.size > 0 {
			n, err := aw.w.Write(m.data)
			totalWritten += int64(n)
			if err != nil {
				return totalWritten, err
			}
		}

		// 2-byte alignment padding ('\n')
		if m.size%2 != 0 {
			n, err := aw.w.Write([]byte{'\n'})
			totalWritten += int64(n)
			if err != nil {
				return totalWritten, err
			}
		}

		if cfg.OnFileDone != nil {
			cfg.OnFileDone(m.f, nil)
		}
	}

	return totalWritten, nil
}
