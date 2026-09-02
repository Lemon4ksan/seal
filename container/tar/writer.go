// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tar

import (
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/lemon4ksan/seal/internal/pool"
)

// Writer provides sequential writing of TAR archives.
type Writer struct {
	w       io.Writer
	currRem int64
	pad     int64
	closed  bool
	block   [BlockSize]byte
}

// NewWriter creates a new TAR stream writer writing to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteHeader writes a Header to the TAR archive stream.
// If the filename exceeds 100 bytes, or the size exceeds 8GB, or PAX records exist,
// a POSIX.1-2001 PAX extended header is automatically emitted beforehand.
func (tw *Writer) WriteHeader(h *Header) error {
	if tw.closed {
		return errors.New("tar: writer is closed")
	}

	// 1. Flush any remaining padding from the previous entry
	if tw.currRem > 0 {
		return errors.New("tar: previous entry was not fully written")
	}
	if tw.pad > 0 {
		var zero [BlockSize]byte
		if _, err := tw.w.Write(zero[:tw.pad]); err != nil {
			return err
		}
		tw.pad = 0
	}

	pax := make(map[string]string)
	for k, v := range h.PAXRecords {
		pax[k] = v
	}

	// Emit PAX if name exceeds standard 100-byte USTAR field
	if len(h.Name) > 100 {
		pax["path"] = h.Name
	}
	if len(h.Linkname) > 100 {
		pax["linkpath"] = h.Linkname
	}
	if h.Size > 077777777777 { // > 8GB
		pax["size"] = strconv.FormatInt(h.Size, 10)
	}

	// Emit PAX extended header block if needed
	if len(pax) > 0 {
		paxData := formatPAX(pax)
		paxHdr := &Header{
			Typeflag: TypeXHeader,
			Name:     "PaxHeaders.0/" + h.Name,
			Size:     int64(len(paxData)),
			ModTime:  time.Now(),
			Mode:     0644,
		}
		if err := formatHeaderBlock(tw.block[:], paxHdr); err != nil {
			return err
		}
		if _, err := tw.w.Write(tw.block[:]); err != nil {
			return err
		}
		if _, err := tw.w.Write(paxData); err != nil {
			return err
		}
		paxPad := (BlockSize - (len(paxData) % BlockSize)) & (BlockSize - 1)
		if paxPad > 0 {
			var zero [BlockSize]byte
			if _, err := tw.w.Write(zero[:paxPad]); err != nil {
				return err
			}
		}
	}

	if err := formatHeaderBlock(tw.block[:], h); err != nil {
		return err
	}
	if _, err := tw.w.Write(tw.block[:]); err != nil {
		return err
	}

	tw.currRem = h.Size
	if h.Size > 0 {
		tw.pad = (BlockSize - (h.Size % BlockSize)) & (BlockSize - 1)
	} else {
		tw.pad = 0
	}

	return nil
}

// Write writes payload data for the current TAR entry.
func (tw *Writer) Write(p []byte) (int, error) {
	if tw.closed {
		return 0, errors.New("tar: writer is closed")
	}
	if int64(len(p)) > tw.currRem {
		return 0, ErrWriteTooLong
	}
	n, err := tw.w.Write(p)
	tw.currRem -= int64(n)
	return n, err
}

// WritePayload streams entry contents from src into the TAR archive.
func (tw *Writer) WritePayload(src io.Reader) (int64, error) {
	bufPtr := pool.Get64K()
	defer pool.Put64K(bufPtr)
	buf := (*bufPtr)[:]

	var written int64
	for {
		nr, rErr := src.Read(buf)
		if nr > 0 {
			nw, wErr := tw.Write(buf[:nr])
			written += int64(nw)
			if wErr != nil {
				return written, wErr
			}
		}
		if rErr != nil {
			if errors.Is(rErr, io.EOF) {
				break
			}
			return written, rErr
		}
	}
	return written, nil
}

// Close finishes writing the TAR archive by writing two 512-byte zero blocks.
func (tw *Writer) Close() error {
	if tw.closed {
		return nil
	}
	tw.closed = true

	// Pad any unwritten trailing payload bytes
	if tw.currRem > 0 {
		var zero [BlockSize]byte
		for tw.currRem > 0 {
			toWrite := min(tw.currRem, BlockSize)
			if _, err := tw.w.Write(zero[:toWrite]); err != nil {
				return err
			}
			tw.currRem -= toWrite
		}
	}
	if tw.pad > 0 {
		var zero [BlockSize]byte
		if _, err := tw.w.Write(zero[:tw.pad]); err != nil {
			return err
		}
		tw.pad = 0
	}

	// Write two 512-byte zero blocks (1024 zero bytes) as standard end-of-archive marker
	var zeroTail [BlockSize * 2]byte
	_, err := tw.w.Write(zeroTail[:])
	return err
}
