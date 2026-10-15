// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/lemon4ksan/seal/file"
)

// Reader provides sequential stream parsing for CPIO archives.
type Reader struct {
	r io.Reader
}

// NewReader constructs a new CPIO stream reader.
func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

// Next reads the next file record from the CPIO stream.
// When the end of the archive (the "TRAILER!!!" record) is encountered, it returns io.EOF.
func (cr *Reader) Next() (*Header, []byte, error) {
	var magicBuf [6]byte
	if _, err := io.ReadFull(cr.r, magicBuf[:]); err != nil {
		return nil, nil, err
	}

	magic := string(magicBuf[:])
	switch magic {
	case MagicNewc, MagicCRC:
		return cr.readNewcHeader(magic)
	case MagicODC:
		return cr.readODCHeader(magic)
	default:
		return nil, nil, fmt.Errorf("%w: %q", ErrInvalidMagic, magic)
	}
}

func (cr *Reader) readNewcHeader(magic string) (*Header, []byte, error) {
	var rest [104]byte
	if _, err := io.ReadFull(cr.r, rest[:]); err != nil {
		return nil, nil, ErrCorruptedHeader
	}

	parseHex := func(s []byte) (uint64, error) {
		return strconv.ParseUint(string(s), 16, 64)
	}

	ino, err := parseHex(rest[0:8])
	if err != nil {
		return nil, nil, ErrCorruptedHeader
	}
	mode, err := parseHex(rest[8:16])
	if err != nil {
		return nil, nil, ErrCorruptedHeader
	}
	uid, _ := parseHex(rest[16:24])
	gid, _ := parseHex(rest[24:32])
	nlink, _ := parseHex(rest[32:40])
	mtimeSec, _ := parseHex(rest[40:48])
	fileSize, err := parseHex(rest[48:56])
	if err != nil {
		return nil, nil, ErrCorruptedHeader
	}
	devMaj, _ := parseHex(rest[56:64])
	devMin, _ := parseHex(rest[64:72])
	rdevMaj, _ := parseHex(rest[72:80])
	rdevMin, _ := parseHex(rest[80:88])
	nameSize, err := parseHex(rest[88:96])
	if err != nil || nameSize == 0 || nameSize > 65536 {
		return nil, nil, ErrCorruptedHeader
	}
	chksum, _ := parseHex(rest[96:104])

	// Read name
	nameBuf := make([]byte, nameSize)
	if _, err := io.ReadFull(cr.r, nameBuf); err != nil {
		return nil, nil, ErrCorruptedHeader
	}

	// Discard header + name padding to 4-byte boundary
	namePad := Pad4(int64(HeaderSizeNewc) + int64(nameSize))
	if namePad > 0 {
		var padBuf [4]byte
		if _, err := io.ReadFull(cr.r, padBuf[:namePad]); err != nil {
			return nil, nil, ErrCorruptedHeader
		}
	}

	name := strings.TrimRight(string(nameBuf), "\x00")
	if name == TrailerName {
		return nil, nil, io.EOF
	}

	// Read payload
	var data []byte
	if fileSize > 0 {
		data = make([]byte, fileSize)
		if _, err := io.ReadFull(cr.r, data); err != nil {
			return nil, nil, ErrCorruptedHeader
		}
		// Discard data padding to 4-byte boundary
		dataPad := Pad4(int64(fileSize))
		if dataPad > 0 {
			var padBuf [4]byte
			if _, err := io.ReadFull(cr.r, padBuf[:dataPad]); err != nil {
				return nil, nil, ErrCorruptedHeader
			}
		}
	}

	hdr := &Header{
		Magic:     magic,
		Ino:       ino,
		Mode:      uint32(mode),
		UID:       uid,
		GID:       gid,
		NLink:     nlink,
		MTime:     time.Unix(int64(mtimeSec), 0).UTC(),
		FileSize:  int64(fileSize),
		DevMajor:  devMaj,
		DevMinor:  devMin,
		RDevMajor: rdevMaj,
		RDevMinor: rdevMin,
		Name:      name,
		Checksum:  uint32(chksum),
	}

	return hdr, data, nil
}

func (cr *Reader) readODCHeader(magic string) (*Header, []byte, error) {
	var rest [70]byte
	if _, err := io.ReadFull(cr.r, rest[:]); err != nil {
		return nil, nil, ErrCorruptedHeader
	}

	parseOctal := func(s []byte) (uint64, error) {
		return strconv.ParseUint(strings.TrimSpace(string(s)), 8, 64)
	}

	dev, _ := parseOctal(rest[0:6])
	ino, err := parseOctal(rest[6:12])
	if err != nil {
		return nil, nil, ErrCorruptedHeader
	}
	mode, err := parseOctal(rest[12:18])
	if err != nil {
		return nil, nil, ErrCorruptedHeader
	}
	uid, _ := parseOctal(rest[18:24])
	gid, _ := parseOctal(rest[24:30])
	nlink, _ := parseOctal(rest[30:36])
	rdev, _ := parseOctal(rest[36:42])
	mtimeSec, _ := parseOctal(rest[42:53])
	nameSize, err := parseOctal(rest[53:59])
	if err != nil || nameSize == 0 || nameSize > 65536 {
		return nil, nil, ErrCorruptedHeader
	}
	fileSize, err := parseOctal(rest[59:70])
	if err != nil {
		return nil, nil, ErrCorruptedHeader
	}

	nameBuf := make([]byte, nameSize)
	if _, err := io.ReadFull(cr.r, nameBuf); err != nil {
		return nil, nil, ErrCorruptedHeader
	}

	name := strings.TrimRight(string(nameBuf), "\x00")
	if name == TrailerName {
		return nil, nil, io.EOF
	}

	var data []byte
	if fileSize > 0 {
		data = make([]byte, fileSize)
		if _, err := io.ReadFull(cr.r, data); err != nil {
			return nil, nil, ErrCorruptedHeader
		}
	}

	hdr := &Header{
		Magic:     magic,
		DevMajor:  dev,
		Ino:       ino,
		Mode:      uint32(mode),
		UID:       uid,
		GID:       gid,
		NLink:     nlink,
		RDevMajor: rdev,
		MTime:     time.Unix(int64(mtimeSec), 0).UTC(),
		FileSize:  int64(fileSize),
		Name:      name,
	}

	return hdr, data, nil
}

// ReadCatalog parses all entries from a CPIO stream into a slice of [*file.File].
func ReadCatalog(r io.Reader) ([]*file.File, error) {
	cr := NewReader(r)
	var entries []*file.File

	for {
		hdr, data, err := cr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		cleanName := strings.TrimPrefix(hdr.Name, "./")
		cleanName = strings.TrimPrefix(cleanName, "/")
		if cleanName == "" {
			continue
		}

		isDir := hdr.IsDir()
		if isDir && !strings.HasSuffix(cleanName, "/") {
			cleanName += "/"
		}

		f, err := file.New(cleanName, isDir)
		if err != nil {
			return nil, fmt.Errorf("cpio: create file entry %q: %w", cleanName, err)
		}

		f.SetSizes(hdr.FileSize, hdr.FileSize, 0)
		if !hdr.MTime.IsZero() {
			f.WithModTime(hdr.MTime)
		}
		f.WithMode(hdr.FileMode())

		if !isDir && hdr.FileSize > 0 {
			f.WithData(bytes.Clone(data))
		}

		entries = append(entries, f)
	}

	return entries, nil
}
