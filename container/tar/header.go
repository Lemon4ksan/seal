// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tar

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"
)

const (
	// BlockSize is the fixed size in bytes of each TAR record block.
	BlockSize = 512

	// Type flags for TAR entry records.
	TypeReg           byte = '0'    // Regular file
	TypeRegA          byte = '\x00' // Regular file (legacy alternate)
	TypeLink          byte = '1'    // Hard link
	TypeSymlink       byte = '2'    // Symbolic link
	TypeChar          byte = '3'    // Character device node
	TypeBlock         byte = '4'    // Block device node
	TypeDir           byte = '5'    // Directory
	TypeFifo          byte = '6'    // FIFO node (named pipe)
	TypeCont          byte = '7'    // Contiguous file
	TypeXHeader       byte = 'x'    // Extended header referring to the next file in the archive (PAX)
	TypeXGlobalHeader byte = 'g'    // Global extended header referring to all subsequent files (PAX)
	TypeGNULongName   byte = 'L'    // GNU long file name metadata
	TypeGNULongLink   byte = 'K'    // GNU long link name metadata
	TypeGNUSparse     byte = 'S'    // GNU sparse file metadata

	magicUSTAR   = "ustar\x00"
	versionUSTAR = "00"
	magicGNU     = "ustar "
	versionGNU   = " \x00"
)

// Format specifies the TAR archive dialect format.
type Format int

const (
	FormatUnknown Format = 0
	FormatUSTAR   Format = 1 << iota
	FormatPAX
	FormatGNU
)

// Common TAR errors.
var (
	ErrHeader           = errors.New("tar: invalid header block")
	ErrChecksumMismatch = errors.New("tar: header checksum mismatch")
	ErrWriteTooLong     = errors.New("tar: write exceeded declared size")
	ErrFieldTooLong     = errors.New("tar: field too long for format")
)

// Header represents a single TAR entry header block and its POSIX / GNU / PAX metadata.
type Header struct {
	Typeflag byte
	Name     string
	Linkname string
	Size     int64
	Mode     fs.FileMode
	UID      int
	GID      int
	Uname    string
	Gname    string
	ModTime  time.Time
	Devmajor int64
	Devminor int64

	PAXRecords map[string]string
	Format     Format
}

// FileInfo returns an fs.FileInfo corresponding to the Header metadata.
func (h *Header) FileInfo() fs.FileInfo {
	return headerFileInfo{h}
}

type headerFileInfo struct {
	h *Header
}

func (fi headerFileInfo) Name() string       { return fi.h.Name }
func (fi headerFileInfo) Size() int64        { return fi.h.Size }
func (fi headerFileInfo) Mode() fs.FileMode  { return fi.h.Mode }
func (fi headerFileInfo) ModTime() time.Time { return fi.h.ModTime }
func (fi headerFileInfo) IsDir() bool {
	return fi.h.Typeflag == TypeDir || strings.HasSuffix(fi.h.Name, "/")
}
func (fi headerFileInfo) Sys() any { return fi.h }

// parseOctal parses an ASCII octal number or GNU base-256 binary integer from b.
func parseOctal(b []byte) (int64, error) {
	// Strip leading and trailing NULs and spaces
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\x00') {
		b = b[1:]
	}
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\x00') {
		b = b[:len(b)-1]
	}
	if len(b) == 0 {
		return 0, nil
	}

	// GNU base-256 binary encoding (used when values exceed octal field capacity, e.g. size > 8GB)
	if b[0]&0x80 != 0 {
		isNegative := (b[0] & 0x40) != 0
		var val int64
		for i, v := range b {
			var byteVal byte
			if i == 0 {
				byteVal = v & 0x7F
			} else {
				byteVal = v
			}
			if isNegative {
				val = (val << 8) | int64(^byteVal)
			} else {
				val = (val << 8) | int64(byteVal)
			}
		}
		if isNegative {
			return -1 - val, nil
		}
		return val, nil
	}

	// Standard ASCII octal parsing
	var val int64
	for _, c := range b {
		if c < '0' || c > '7' {
			return 0, ErrHeader
		}
		val = (val << 3) | int64(c-'0')
	}
	return val, nil
}

// formatOctal formats val as an octal string right-aligned with trailing zero/space.
func formatOctal(b []byte, val int64) bool {
	if val < 0 {
		return false
	}
	s := strconv.FormatInt(val, 8)
	n := len(s)
	if n >= len(b) {
		// Value exceeds field capacity; try GNU base-256 binary representation
		return formatBinary(b, val)
	}

	// Zero-fill or space-pad
	pad := len(b) - n - 1
	for i := range pad {
		b[i] = '0'
	}
	copy(b[pad:], s)
	b[len(b)-1] = 0
	return true
}

// formatBinary encodes val using GNU base-256 binary representation.
func formatBinary(b []byte, val int64) bool {
	if len(b) == 0 {
		return false
	}
	b[0] = 0x80 // Positive GNU base-256 indicator
	u := uint64(val)
	for i := len(b) - 1; i > 0; i-- {
		b[i] = byte(u)
		u >>= 8
	}
	return u == 0
}

// computeChecksum calculates both unsigned and signed checksums of a 512-byte header block.
// Bytes 148..155 (checksum field) are treated as ASCII spaces (' ', 0x20).
func computeChecksum(block []byte) (unsigned uint32, signed int32) {
	for i := 0; i < BlockSize; i++ {
		if i >= 148 && i < 156 {
			unsigned += uint32(' ')
			signed += int32(int8(' '))
		} else {
			unsigned += uint32(block[i])
			signed += int32(int8(block[i]))
		}
	}
	return unsigned, signed
}

// parseHeaderBlock parses a 512-byte raw TAR block into a Header.
func parseHeaderBlock(block []byte) (*Header, error) {
	if len(block) != BlockSize {
		return nil, ErrHeader
	}

	// Check if this is an empty block
	allZero := true
	for _, b := range block {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return nil, io.EOF
	}

	// Verify checksum
	chksumVal, err := parseOctal(block[148:156])
	if err != nil {
		return nil, ErrChecksumMismatch
	}
	uChk, sChk := computeChecksum(block)
	if uint32(chksumVal) != uChk && int32(chksumVal) != sChk {
		return nil, ErrChecksumMismatch
	}

	h := &Header{}
	h.Name = parseString(block[0:100])
	mode, _ := parseOctal(block[100:108])
	h.Mode = fs.FileMode(mode & 07777)
	uid, _ := parseOctal(block[108:116])
	h.UID = int(uid)
	gid, _ := parseOctal(block[116:124])
	h.GID = int(gid)
	size, _ := parseOctal(block[124:136])
	h.Size = size
	mtime, _ := parseOctal(block[136:148])
	h.ModTime = time.Unix(mtime, 0)
	h.Typeflag = block[156]
	if h.Typeflag == TypeRegA {
		h.Typeflag = TypeReg
	}
	h.Linkname = parseString(block[157:257])

	magic := string(block[257:263])
	switch magic {
	case magicUSTAR:
		h.Format = FormatUSTAR
		h.Uname = parseString(block[265:297])
		h.Gname = parseString(block[297:329])
		devmaj, _ := parseOctal(block[329:337])
		h.Devmajor = devmaj
		devmin, _ := parseOctal(block[337:345])
		h.Devminor = devmin
		prefix := parseString(block[345:500])
		if prefix != "" {
			h.Name = prefix + "/" + h.Name
		}
	case magicGNU:
		h.Format = FormatGNU
		h.Uname = parseString(block[265:297])
		h.Gname = parseString(block[297:329])
		devmaj, _ := parseOctal(block[329:337])
		h.Devmajor = devmaj
		devmin, _ := parseOctal(block[337:345])
		h.Devminor = devmin
	}

	if h.Typeflag == TypeDir {
		h.Mode |= fs.ModeDir
	} else if h.Typeflag == TypeSymlink {
		h.Mode |= fs.ModeSymlink
	}

	return h, nil
}

// formatHeaderBlock serializes Header h into a 512-byte raw TAR block.
func formatHeaderBlock(dst []byte, h *Header) error {
	if len(dst) < BlockSize {
		return ErrHeader
	}
	for i := range dst[:BlockSize] {
		dst[i] = 0
	}

	name := h.Name
	var prefix string
	if len(name) > 100 {
		// Split name into prefix and suffix for standard USTAR if possible
		idx := strings.LastIndex(name, "/")
		if idx > 0 && idx <= 155 && len(name)-idx-1 <= 100 {
			prefix = name[:idx]
			name = name[idx+1:]
		}
	}

	copyString(dst[0:100], name)
	formatOctal(dst[100:108], int64(h.Mode.Perm()))
	formatOctal(dst[108:116], int64(h.UID))
	formatOctal(dst[116:124], int64(h.GID))
	formatOctal(dst[124:136], h.Size)
	formatOctal(dst[136:148], h.ModTime.Unix())
	dst[156] = h.Typeflag
	if dst[156] == 0 {
		if h.Mode.IsDir() {
			dst[156] = TypeDir
		} else {
			dst[156] = TypeReg
		}
	}
	copyString(dst[157:257], h.Linkname)

	copy(dst[257:263], magicUSTAR)
	copy(dst[263:265], versionUSTAR)
	copyString(dst[265:297], h.Uname)
	copyString(dst[297:329], h.Gname)
	if h.Devmajor != 0 {
		formatOctal(dst[329:337], h.Devmajor)
	}
	if h.Devminor != 0 {
		formatOctal(dst[337:345], h.Devminor)
	}
	if prefix != "" {
		copyString(dst[345:500], prefix)
	}

	// Compute and format checksum into 148..156
	uChk, _ := computeChecksum(dst[:BlockSize])
	chkStr := strconv.FormatUint(uint64(uChk), 8)
	pad := 6 - len(chkStr)
	for i := range pad {
		dst[148+i] = '0'
	}
	copy(dst[148+pad:], chkStr)
	dst[154] = 0
	dst[155] = ' '

	return nil
}

func parseString(b []byte) string {
	idx := bytes.IndexByte(b, 0)
	if idx >= 0 {
		return string(b[:idx])
	}
	return string(b)
}

func copyString(dst []byte, s string) {
	n := copy(dst, s)
	if n < len(dst) {
		dst[n] = 0
	}
}

// parsePAX parses POSIX.1-2001 PAX extended header key-value records.
// Format of each record: "%d %s=%s\n", totalRecordLength, key, value
func parsePAX(r io.Reader) (map[string]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	records := make(map[string]string)
	for len(data) > 0 {
		sp := bytes.IndexByte(data, ' ')
		if sp <= 0 {
			break
		}
		length, err := strconv.Atoi(string(data[:sp]))
		if err != nil || length > len(data) || length <= sp {
			break
		}
		rec := data[sp+1 : length]
		if len(rec) > 0 && rec[len(rec)-1] == '\n' {
			rec = rec[:len(rec)-1]
		}
		eq := bytes.IndexByte(rec, '=')
		if eq > 0 {
			key := string(rec[:eq])
			val := string(rec[eq+1:])
			records[key] = val
		}
		data = data[length:]
	}
	return records, nil
}

// formatPAX serializes extended attributes into a PAX payload byte stream.
func formatPAX(records map[string]string) []byte {
	if len(records) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for k, v := range records {
		// Calculate record length including the length prefix itself
		// "%d %s=%s\n"
		overhead := 1 + len(k) + 1 + len(v) + 1
		lenDigits := len(strconv.Itoa(overhead))
		totalLen := overhead + lenDigits
		if len(strconv.Itoa(totalLen)) > lenDigits {
			totalLen++
		}
		fmt.Fprintf(&buf, "%d %s=%s\n", totalLen, k, v)
	}
	return buf.Bytes()
}
