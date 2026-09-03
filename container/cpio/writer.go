// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpio

import (
	"fmt"
	"io"
	"strings"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

// Writer serializes file entries into a standard SVR4 newc CPIO archive.
type Writer struct {
	w io.Writer
}

// NewWriter constructs a CPIO archive serializer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteArchive serializes the slice of files into a standard CPIO archive stream.
func (cw *Writer) WriteArchive(files []*file.File, cfg container.Config) (int64, error) {
	var totalWritten int64
	zeroPad := []byte{0, 0, 0, 0}

	ino := uint64(1)

	for _, f := range files {
		cleanName := strings.TrimPrefix(f.Name(), "/")
		if cleanName == "" {
			continue
		}

		isDir := f.IsDir()
		if isDir && !strings.HasSuffix(cleanName, "/") {
			cleanName += "/"
		}

		mode := uint32(f.Mode().Perm())
		if isDir {
			mode |= ModeTypeDir
		} else {
			mode |= ModeTypeReg
		}

		var fileSize int64
		if !isDir {
			fileSize = f.UncompressedSize()
		}

		mtime := f.ModTime().Unix()
		if mtime < 0 {
			mtime = 0
		}

		nameBytes := []byte(cleanName + "\x00")
		nameSize := int64(len(nameBytes))

		// Build 110-byte SVR4 newc header
		hdr := fmt.Sprintf("%6s%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X",
			MagicNewc,
			ino,
			mode,
			0, // UID
			0, // GID
			1, // NLink
			uint32(mtime),
			uint32(fileSize),
			0, // DevMajor
			0, // DevMinor
			0, // RDevMajor
			0, // RDevMinor
			uint32(nameSize),
			0, // Checksum
		)

		if len(hdr) != HeaderSizeNewc {
			return totalWritten, fmt.Errorf("cpio: internal error: header size %d != %d", len(hdr), HeaderSizeNewc)
		}

		// Write header
		n, err := io.WriteString(cw.w, hdr)
		totalWritten += int64(n)
		if err != nil {
			return totalWritten, err
		}

		// Write filename with null terminator
		n, err = cw.w.Write(nameBytes)
		totalWritten += int64(n)
		if err != nil {
			return totalWritten, err
		}

		// Header + name padding to 4-byte boundary
		headerPad := Pad4(int64(HeaderSizeNewc) + nameSize)
		if headerPad > 0 {
			n, err := cw.w.Write(zeroPad[:headerPad])
			totalWritten += int64(n)
			if err != nil {
				return totalWritten, err
			}
		}

		// Write payload
		if !isDir && fileSize > 0 {
			r, err := f.Open()
			if err != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(f, err)
				}
				return totalWritten, fmt.Errorf("cpio: open file %q: %w", f.Name(), err)
			}
			written, err := io.Copy(cw.w, r)
			_ = r.Close()
			totalWritten += written
			if err != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(f, err)
				}
				return totalWritten, fmt.Errorf("cpio: copy file %q: %w", f.Name(), err)
			}

			// Data padding to 4-byte boundary
			dataPad := Pad4(fileSize)
			if dataPad > 0 {
				n, err := cw.w.Write(zeroPad[:dataPad])
				totalWritten += int64(n)
				if err != nil {
					return totalWritten, err
				}
			}
		}

		if cfg.OnFileDone != nil {
			cfg.OnFileDone(f, nil)
		}
		ino++
	}

	// Write terminating "TRAILER!!!" record
	trailerBytes := []byte(TrailerName + "\x00")
	trailerNameSize := int64(len(trailerBytes))

	trailerHdr := fmt.Sprintf("%6s%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X",
		MagicNewc,
		0, // Ino
		0, // Mode
		0, // UID
		0, // GID
		1, // NLink
		0, // MTime
		0, // FileSize
		0, // DevMajor
		0, // DevMinor
		0, // RDevMajor
		0, // RDevMinor
		uint32(trailerNameSize),
		0, // Checksum
	)

	n, err := io.WriteString(cw.w, trailerHdr)
	totalWritten += int64(n)
	if err != nil {
		return totalWritten, err
	}

	n, err = cw.w.Write(trailerBytes)
	totalWritten += int64(n)
	if err != nil {
		return totalWritten, err
	}

	trailerPad := Pad4(int64(HeaderSizeNewc) + trailerNameSize)
	if trailerPad > 0 {
		n, err := cw.w.Write(zeroPad[:trailerPad])
		totalWritten += int64(n)
		if err != nil {
			return totalWritten, err
		}
	}

	return totalWritten, nil
}
