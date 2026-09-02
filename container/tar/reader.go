// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tar

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/lemon4ksan/seal/vfs"
)

// Reader provides sequential access to the entries of a TAR archive.
type Reader struct {
	r         io.Reader
	curr      *entryReader
	pad       int64
	block     [BlockSize]byte
	paxGlobal map[string]string
	paxLocal  map[string]string
	gnuName   string
	gnuLink   string
	eof       bool
}

// NewReader creates a new TAR stream reader reading from r.
func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

// Next advances to the next entry in the TAR archive.
// The io.Reader returned allows streaming the uncompressed payload of the entry.
func (tr *Reader) Next() (*Header, io.Reader, error) {
	if tr.eof {
		return nil, nil, io.EOF
	}

	// 1. Drain any unread payload and alignment padding from the previous entry
	if tr.curr != nil {
		if err := tr.curr.discard(); err != nil {
			return nil, nil, err
		}
		tr.curr = nil
	}
	if tr.pad > 0 {
		if _, err := io.CopyN(io.Discard, tr.r, tr.pad); err != nil {
			return nil, nil, err
		}
		tr.pad = 0
	}

	for {
		if _, err := io.ReadFull(tr.r, tr.block[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				tr.eof = true
				return nil, nil, io.EOF
			}
			return nil, nil, err
		}

		h, err := parseHeaderBlock(tr.block[:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Encountered an all-zero block. Check for consecutive zero block
				_, _ = io.ReadFull(tr.r, tr.block[:])
				tr.eof = true
				return nil, nil, io.EOF
			}
			return nil, nil, err
		}

		// Calculate 512-byte payload alignment padding for the current block
		if h.Size > 0 {
			tr.pad = (BlockSize - (h.Size % BlockSize)) & (BlockSize - 1)
		} else {
			tr.pad = 0
		}

		switch h.Typeflag {
		case TypeXGlobalHeader:
			// POSIX.1-2001 Global Extended Header (applies to all subsequent records)
			global, err := parsePAX(io.LimitReader(tr.r, h.Size))
			if err != nil {
				return nil, nil, err
			}
			if tr.pad > 0 {
				if _, err := io.CopyN(io.Discard, tr.r, tr.pad); err != nil {
					return nil, nil, err
				}
				tr.pad = 0
			}
			tr.paxGlobal = global
			continue

		case TypeXHeader:
			// POSIX.1-2001 Extended Header (applies only to the immediate next record)
			local, err := parsePAX(io.LimitReader(tr.r, h.Size))
			if err != nil {
				return nil, nil, err
			}
			if tr.pad > 0 {
				if _, err := io.CopyN(io.Discard, tr.r, tr.pad); err != nil {
					return nil, nil, err
				}
				tr.pad = 0
			}
			tr.paxLocal = local
			continue

		case TypeGNULongName:
			// GNU Long File Name extension
			data, err := io.ReadAll(io.LimitReader(tr.r, h.Size))
			if err != nil {
				return nil, nil, err
			}
			if tr.pad > 0 {
				if _, err := io.CopyN(io.Discard, tr.r, tr.pad); err != nil {
					return nil, nil, err
				}
				tr.pad = 0
			}
			tr.gnuName = parseString(data)
			continue

		case TypeGNULongLink:
			// GNU Long Link Name extension
			data, err := io.ReadAll(io.LimitReader(tr.r, h.Size))
			if err != nil {
				return nil, nil, err
			}
			if tr.pad > 0 {
				if _, err := io.CopyN(io.Discard, tr.r, tr.pad); err != nil {
					return nil, nil, err
				}
				tr.pad = 0
			}
			tr.gnuLink = parseString(data)
			continue

		default:
			// Normal entry: Merge GNU and PAX overrides
			if tr.gnuName != "" {
				h.Name = tr.gnuName
				tr.gnuName = ""
			}
			if tr.gnuLink != "" {
				h.Linkname = tr.gnuLink
				tr.gnuLink = ""
			}

			// Apply global PAX attributes
			mergePAX(h, tr.paxGlobal)
			// Apply local PAX attributes
			mergePAX(h, tr.paxLocal)
			tr.paxLocal = nil

			h.Name = vfs.CleanPath(h.Name)
			if h.Typeflag == TypeDir && !strings.HasSuffix(h.Name, "/") {
				h.Name += "/"
			}

			tr.curr = &entryReader{r: tr.r, rem: h.Size}
			return h, tr.curr, nil
		}
	}
}

// mergePAX applies parsed PAX key-value records onto Header h.
func mergePAX(h *Header, pax map[string]string) {
	if len(pax) == 0 {
		return
	}
	if h.PAXRecords == nil {
		h.PAXRecords = make(map[string]string)
	}
	for k, v := range pax {
		h.PAXRecords[k] = v
		switch k {
		case "path":
			h.Name = v
		case "linkpath":
			h.Linkname = v
		case "size":
			if sz, err := strconv.ParseInt(v, 10, 64); err == nil {
				h.Size = sz
			}
		case "uid":
			if uid, err := strconv.Atoi(v); err == nil {
				h.UID = uid
			}
		case "gid":
			if gid, err := strconv.Atoi(v); err == nil {
				h.GID = gid
			}
		case "uname":
			h.Uname = v
		case "gname":
			h.Gname = v
		case "mtime":
			if sec, nsec, ok := parsePAXTime(v); ok {
				h.ModTime = time.Unix(sec, nsec)
			}
		}
	}
}

func parsePAXTime(s string) (int64, int64, bool) {
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		sec, err := strconv.ParseInt(s, 10, 64)
		return sec, 0, err == nil
	}
	sec, err := strconv.ParseInt(s[:dot], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	fracStr := s[dot+1:]
	if len(fracStr) > 9 {
		fracStr = fracStr[:9]
	}
	for len(fracStr) < 9 {
		fracStr += "0"
	}
	nsec, err := strconv.ParseInt(fracStr, 10, 64)
	return sec, nsec, err == nil
}

// entryReader wraps the underlying reader limited to the entry size.
type entryReader struct {
	r   io.Reader
	rem int64
}

func (er *entryReader) Read(p []byte) (int, error) {
	if er.rem <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > er.rem {
		p = p[:er.rem]
	}
	n, err := er.r.Read(p)
	er.rem -= int64(n)
	if er.rem <= 0 && err == nil {
		err = io.EOF
	}
	return n, err
}

func (er *entryReader) discard() error {
	if er.rem > 0 {
		_, err := io.CopyN(io.Discard, er.r, er.rem)
		er.rem = 0
		return err
	}
	return nil
}
