// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ar

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/lemon4ksan/seal/file"
)

// Reader provides stream and catalog parsing for Unix AR archives and Debian packages.
type Reader struct {
	r           io.Reader
	stringTable []byte
}

// NewReader constructs an AR stream reader and validates the global magic signature.
func NewReader(r io.Reader) (*Reader, error) {
	var magic [8]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, ErrInvalidMagic
	}
	if string(magic[:]) != Magic {
		return nil, ErrInvalidMagic
	}
	return &Reader{r: r}, nil
}

// Next reads the next file member from the AR archive.
// Returns io.EOF when the archive has been completely read.
func (ar *Reader) Next() (*Header, []byte, error) {
	for {
		var hdr [HeaderSize]byte
		n, err := io.ReadFull(ar.r, hdr[:])
		if err != nil {
			if errors.Is(err, io.EOF) || (errors.Is(err, io.ErrUnexpectedEOF) && n == 0) {
				return nil, nil, io.EOF
			}
			return nil, nil, ErrCorruptedHeader
		}

		if hdr[58] != '`' || hdr[59] != '\n' {
			return nil, nil, ErrCorruptedHeader
		}

		rawName := strings.TrimSpace(string(hdr[0:16]))
		mtimeSec, _ := strconv.ParseInt(strings.TrimSpace(string(hdr[16:28])), 10, 64)
		uid, _ := strconv.Atoi(strings.TrimSpace(string(hdr[28:34])))
		gid, _ := strconv.Atoi(strings.TrimSpace(string(hdr[34:40])))
		mode, _ := strconv.ParseInt(strings.TrimSpace(string(hdr[40:48])), 8, 64)
		size, err := strconv.ParseInt(strings.TrimSpace(string(hdr[48:58])), 10, 64)
		if err != nil || size < 0 {
			return nil, nil, ErrCorruptedHeader
		}

		origSize := size

		// 1. GNU String Table (member name "//")
		if rawName == "//" {
			table := make([]byte, size)
			if _, err := io.ReadFull(ar.r, table); err != nil {
				return nil, nil, ErrCorruptedHeader
			}
			ar.stringTable = table
			if origSize%2 != 0 {
				var pad [1]byte
				_, _ = io.ReadFull(ar.r, pad[:])
			}
			continue
		}

		// 2. GNU Symbol Table (member name "/" or "/SYM64/")
		if rawName == "/" || rawName == "/SYM64/" {
			if _, err := io.CopyN(io.Discard, ar.r, size); err != nil {
				return nil, nil, ErrCorruptedHeader
			}
			if origSize%2 != 0 {
				var pad [1]byte
				_, _ = io.ReadFull(ar.r, pad[:])
			}
			continue
		}

		fileName := rawName

		// 3. GNU long name lookup (e.g. "/12")
		if strings.HasPrefix(rawName, "/") && len(rawName) > 1 && rawName[1] >= '0' && rawName[1] <= '9' {
			off, err := strconv.Atoi(rawName[1:])
			if err == nil && ar.stringTable != nil && off < len(ar.stringTable) {
				sub := ar.stringTable[off:]
				end := bytes.IndexAny(sub, "/\n\x00")
				if end != -1 {
					fileName = string(sub[:end])
				} else {
					fileName = string(sub)
				}
			}
		} else if strings.HasPrefix(rawName, "#1/") {
			// 4. BSD extended name (#1/<length>)
			nameLen, err := strconv.Atoi(rawName[3:])
			if err == nil && int64(nameLen) <= size {
				nameBuf := make([]byte, nameLen)
				if _, err := io.ReadFull(ar.r, nameBuf); err != nil {
					return nil, nil, ErrCorruptedHeader
				}
				fileName = strings.TrimRight(string(nameBuf), "\x00")
				size -= int64(nameLen)
			}
		} else {
			// 5. Standard GNU name (slash-terminated)
			fileName = strings.TrimSuffix(rawName, "/")
		}

		// Read member data payload
		data := make([]byte, size)
		if size > 0 {
			if _, err := io.ReadFull(ar.r, data); err != nil {
				return nil, nil, ErrCorruptedHeader
			}
		}

		// Discard 2-byte alignment padding ('\n')
		if origSize%2 != 0 {
			var pad [1]byte
			if _, err := io.ReadFull(ar.r, pad[:]); err != nil && !errors.Is(err, io.EOF) {
				return nil, nil, ErrCorruptedHeader
			}
		}

		h := &Header{
			Name:    fileName,
			ModTime: time.Unix(mtimeSec, 0).UTC(),
			UID:     uid,
			GID:     gid,
			Mode:    mode,
			Size:    size,
		}

		return h, data, nil
	}
}

// ReadCatalog parses all file entries from an AR archive.
func ReadCatalog(r io.Reader) ([]*file.File, error) {
	reader, err := NewReader(r)
	if err != nil {
		return nil, err
	}

	var entries []*file.File

	for {
		hdr, data, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		cleanName := strings.TrimPrefix(hdr.Name, "/")
		if cleanName == "" {
			continue
		}

		isDir := strings.HasSuffix(cleanName, "/")

		f, err := file.New(cleanName, isDir)
		if err != nil {
			return nil, fmt.Errorf("ar: create file record %q: %w", cleanName, err)
		}

		f.SetSizes(hdr.Size, hdr.Size, 0)
		if !hdr.ModTime.IsZero() {
			f.WithModTime(hdr.ModTime)
		}
		if hdr.Mode > 0 {
			f.WithMode(fs.FileMode(hdr.Mode & 0o777))
		}

		if !isDir && hdr.Size > 0 {
			f.WithData(bytes.Clone(data))
		}

		entries = append(entries, f)
	}

	return entries, nil
}
