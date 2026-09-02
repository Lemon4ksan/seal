// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"encoding/binary"
	"hash/crc32"
	"iter"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/foundation/binkit"

	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/internal/sys"
	"github.com/lemon4ksan/seal/vfs"
)

// HeaderContext provides ambient header metadata and mutable pointers for Extra Field handlers.
type HeaderContext struct {
	IsLocalHeader bool
	RawName       string
	RawComment    string
	RawFlags      uint16
	Uncompressed  *int64
	Compressed    *int64
	LocalOffset   *int64
}

// ExtraFieldHandler defines a modular plugin for parsing and applying an Extra Field tag.
type ExtraFieldHandler interface {
	// Tag returns the 16-bit Header ID handled by this plugin.
	Tag() uint16

	// Parse extracts metadata from payload and applies it to File and HeaderContext.
	Parse(payload []byte, f *file.File, ctx *HeaderContext) error
}

// ExtraRegistry provides a thread-safe registry of Extra Field plugins.
type ExtraRegistry struct {
	mu       sync.RWMutex
	handlers map[uint16]ExtraFieldHandler
}

// NewExtraRegistry constructs an [ExtraRegistry] initialized with standard PKWARE & Info-ZIP handlers.
func NewExtraRegistry() *ExtraRegistry {
	r := &ExtraRegistry{
		handlers: make(map[uint16]ExtraFieldHandler),
	}
	r.RegisterDefaultHandlers()
	return r
}

// DefaultExtraRegistry is the shared global registry with built-in standard handlers.
var DefaultExtraRegistry = NewExtraRegistry()

// Register installs or overrides an Extra Field handler.
func (r *ExtraRegistry) Register(handler ExtraFieldHandler) {
	if handler == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[handler.Tag()] = handler
}

// Lookup retrieves a registered handler for a given tag.
func (r *ExtraRegistry) Lookup(tag uint16) (ExtraFieldHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[tag]
	return h, ok
}

// ParseAll procedurally scans the raw Extra Field slice and executes matching registered plugins.
// It achieves zero heap allocation during scanning.
func (r *ExtraRegistry) ParseAll(extra []byte, f *file.File, ctx *HeaderContext) {
	if len(extra) == 0 {
		return
	}
	for tag, payload := range ExtraFields(extra) {
		if handler, ok := r.Lookup(tag); ok {
			_ = handler.Parse(payload, f, ctx)
		}
	}
}

// ExtraFields returns a standard Go range iterator over all (tag, payload) tuples in extra.
// Zero heap allocations.
//
// Usage:
//
//	for tag, payload := range zip.ExtraFields(extra) {
//	    ...
//	}
func ExtraFields(extra []byte) iter.Seq2[uint16, []byte] {
	return func(yield func(uint16, []byte) bool) {
		for len(extra) >= 4 {
			tag := uint16(extra[0]) | (uint16(extra[1]) << 8)
			length := int(uint16(extra[2]) | (uint16(extra[3]) << 8))
			if 4+length > len(extra) {
				break
			}
			payload := extra[4 : 4+length]
			extra = extra[4+length:]
			if !yield(tag, payload) {
				return
			}
		}
	}
}

// FindExtraField scans the raw extra fields without heap allocation and returns the full TLV block for targetTag.
func FindExtraField(extra []byte, targetTag uint16) ([]byte, bool) {
	for len(extra) >= 4 {
		tag := uint16(extra[0]) | (uint16(extra[1]) << 8)
		size := int(extra[2]) | (int(extra[3]) << 8)
		if 4+size > len(extra) {
			break
		}
		if tag == targetTag {
			return extra[:4+size], true
		}
		extra = extra[4+size:]
	}
	return nil, false
}

// RegisterDefaultHandlers loads the core standard handlers into the registry.
func (r *ExtraRegistry) RegisterDefaultHandlers() {
	r.Register(&extendedTimestampHandler{})
	r.Register(&unicodePathHandler{})
	r.Register(&unicodeCommentHandler{})
	r.Register(&ntfsExtraHandler{})
	r.Register(&unixUIDGIDHandler{})
	r.Register(&zip64ExtraHandler{})
}

// -----------------------------------------------------------------------------
// 1. Extended Timestamp Plugin (Tag 0x5455 - Info-ZIP 'UT')
// -----------------------------------------------------------------------------

const (
	ExtTimeModTimePresent    byte = 1 << 0 // Bit 0: mtime is present
	ExtTimeAccessTimePresent byte = 1 << 1 // Bit 1: atime is present
	ExtTimeCreateTimePresent byte = 1 << 2 // Bit 2: ctime is present
)

type extendedTimestampHandler struct{}

func (h *extendedTimestampHandler) Tag() uint16 { return TagExtendedTimestamp }

func (h *extendedTimestampHandler) Parse(payload []byte, f *file.File, _ *HeaderContext) error {
	mtime, _, _, ok := ParseExtendedTimestamp(payload)
	if ok && !mtime.IsZero() && f != nil {
		f.WithModTime(mtime)
	}
	return nil
}

// ParseExtendedTimestamp parses Info-ZIP 0x5455 extended timestamp payload.
func ParseExtendedTimestamp(data []byte) (mtime, atime, ctime time.Time, ok bool) {
	if len(data) < 1 {
		return time.Time{}, time.Time{}, time.Time{}, false
	}

	flags := data[0]
	r := binkit.NewReader(data[1:])

	if flags&ExtTimeModTimePresent != 0 && r.Remaining() >= 4 {
		mtime = time.Unix(int64(r.U32LE()), 0).UTC()
		ok = true
	}
	if flags&ExtTimeAccessTimePresent != 0 && r.Remaining() >= 4 {
		atime = time.Unix(int64(r.U32LE()), 0).UTC()
		ok = true
	}
	if flags&ExtTimeCreateTimePresent != 0 && r.Remaining() >= 4 {
		ctime = time.Unix(int64(r.U32LE()), 0).UTC()
		ok = true
	}

	return mtime, atime, ctime, ok
}

// EncodeExtendedTimestamp encodes Info-ZIP 0x5455 extended timestamp extra field.
func EncodeExtendedTimestamp(mtime, atime, ctime time.Time, isCentralDir bool) []byte {
	var flags byte
	numTimes := 0

	if !mtime.IsZero() {
		flags |= ExtTimeModTimePresent
		numTimes++
	}

	if !isCentralDir {
		if !atime.IsZero() {
			flags |= ExtTimeAccessTimePresent
			numTimes++
		}
		if !ctime.IsZero() {
			flags |= ExtTimeCreateTimePresent
			numTimes++
		}
	}

	if flags == 0 {
		return nil
	}

	dataSize := 1 + numTimes*4
	w := binkit.NewWriter(nil, 4+dataSize)
	w.U16LE(TagExtendedTimestamp).
		U16LE(uint16(dataSize)).
		U8(flags)

	if flags&ExtTimeModTimePresent != 0 {
		w.U32LE(uint32(mtime.Unix()))
	}
	if !isCentralDir {
		if flags&ExtTimeAccessTimePresent != 0 {
			w.U32LE(uint32(atime.Unix()))
		}
		if flags&ExtTimeCreateTimePresent != 0 {
			w.U32LE(uint32(ctime.Unix()))
		}
	}

	return w.Bytes()
}

// -----------------------------------------------------------------------------
// 2. Info-ZIP Unicode Path Plugin (Tag 0x7075 'up') & Comment (Tag 0x6375 'uc')
// -----------------------------------------------------------------------------

type unicodePathHandler struct{}

func (h *unicodePathHandler) Tag() uint16 { return TagInfoZipUnicodePath }

func (h *unicodePathHandler) Parse(payload []byte, f *file.File, ctx *HeaderContext) error {
	rawName := ""
	if ctx != nil {
		rawName = ctx.RawName
	}
	if uName, ok := ParseUnicodePath(payload, rawName); ok && uName != "" && f != nil {
		clean := vfs.CleanPath(uName)
		if f.IsDir() && !strings.HasSuffix(clean, "/") {
			clean += "/"
		}
		f.WithName(clean)
	}
	return nil
}

// ParseUnicodePath parses Info-ZIP 0x7075 Unicode Path extra field.
func ParseUnicodePath(data []byte, rawName string) (string, bool) {
	if len(data) < 5 {
		return "", false
	}
	version := data[0]
	if version != 1 {
		return "", false
	}

	expectedCRC := binary.LittleEndian.Uint32(data[1:5])
	actualCRC := crc32.ChecksumIEEE([]byte(rawName))
	if expectedCRC != actualCRC {
		return "", false
	}

	return string(data[5:]), true
}

// EncodeUnicodePath formats Info-ZIP 0x7075 Unicode Path extra field.
func EncodeUnicodePath(utf8Name, rawName string) []byte {
	rawCRC := crc32.ChecksumIEEE([]byte(rawName))
	dataLen := 5 + len(utf8Name)
	w := binkit.NewWriter(nil, 4+dataLen)
	w.U16LE(TagInfoZipUnicodePath).
		U16LE(uint16(dataLen)).
		U8(1).
		U32LE(rawCRC).
		Raw([]byte(utf8Name))
	return w.Bytes()
}

type unicodeCommentHandler struct{}

func (h *unicodeCommentHandler) Tag() uint16 { return TagInfoZipUnicodeComment }

func (h *unicodeCommentHandler) Parse(payload []byte, f *file.File, ctx *HeaderContext) error {
	rawComment := ""
	if ctx != nil {
		rawComment = ctx.RawComment
	}
	if uComment, ok := ParseUnicodeComment(payload, rawComment); ok && f != nil {
		f.WithComment(uComment)
	}
	return nil
}

// ParseUnicodeComment parses Info-ZIP 0x6375 Unicode Comment extra field.
func ParseUnicodeComment(data []byte, rawComment string) (string, bool) {
	if len(data) < 5 {
		return "", false
	}
	version := data[0]
	if version != 1 {
		return "", false
	}

	expectedCRC := binary.LittleEndian.Uint32(data[1:5])
	actualCRC := crc32.ChecksumIEEE([]byte(rawComment))
	if expectedCRC != actualCRC {
		return "", false
	}

	return string(data[5:]), true
}

// EncodeUnicodeComment formats Info-ZIP 0x6375 Unicode Comment extra field.
func EncodeUnicodeComment(utf8Comment, rawComment string) []byte {
	rawCRC := crc32.ChecksumIEEE([]byte(rawComment))
	dataLen := 5 + len(utf8Comment)
	w := binkit.NewWriter(nil, 4+dataLen)
	w.U16LE(TagInfoZipUnicodeComment).
		U16LE(uint16(dataLen)).
		U8(1).
		U32LE(rawCRC).
		Raw([]byte(utf8Comment))
	return w.Bytes()
}

// -----------------------------------------------------------------------------
// 3. NTFS Extra Field Plugin (Tag 0x000A)
// -----------------------------------------------------------------------------

type ntfsExtraHandler struct{}

func (h *ntfsExtraHandler) Tag() uint16 { return TagNTFS }

func (h *ntfsExtraHandler) Parse(payload []byte, f *file.File, _ *HeaderContext) error {
	meta := ParseNTFSExtraField(payload)
	if meta.LastWriteTime != 0 && f != nil {
		f.WithMetadata(meta)
		f.WithModTime(winFiletimeToTime(meta.LastWriteTime))
	}
	return nil
}

// ParseNTFSExtraField extracts high-precision FILETIMEs from tag 0x000A payload or full TLV block.
func ParseNTFSExtraField(data []byte) sys.Metadata {
	if len(data) >= 36 {
		data = data[4:]
	}
	if len(data) < 32 {
		return sys.Metadata{}
	}
	r := binkit.NewReader(data)
	r.Skip(8) // reserved(4) + attrTag(2) + attrSize(2)
	return sys.Metadata{
		LastWriteTime:  r.U64LE(),
		LastAccessTime: r.U64LE(),
		CreationTime:   r.U64LE(),
	}
}

// EncodeNTFSExtraField formats high-precision FILETIMEs into tag 0x000A.
func EncodeNTFSExtraField(metadata sys.Metadata) []byte {
	w := binkit.NewWriter(nil, 36)
	w.U16LE(TagNTFS).
		U16LE(32).
		U32LE(0).
		U16LE(1).
		U16LE(24).
		U64LE(metadata.LastWriteTime).
		U64LE(metadata.LastAccessTime).
		U64LE(metadata.CreationTime)
	return w.Bytes()
}

// -----------------------------------------------------------------------------
// 4. Info-ZIP UNIX UID/GID Plugin (Tag 0x7875 'ux')
// -----------------------------------------------------------------------------

type unixUIDGIDHandler struct{}

func (h *unixUIDGIDHandler) Tag() uint16 { return TagInfoZipUIDGID }

func (h *unixUIDGIDHandler) Parse(payload []byte, _ *file.File, _ *HeaderContext) error {
	// Extracts UID/GID
	_, _, ok := ParseInfoZipUIDGID(payload)
	if !ok {
		return nil
	}
	return nil
}

// ParseInfoZipUIDGID parses Info-ZIP 0x7875 (version 1) UID/GID extra field payload.
func ParseInfoZipUIDGID(data []byte) (uid, gid uint32, ok bool) {
	if len(data) < 3 {
		return 0, 0, false
	}
	version := data[0]
	if version != 1 {
		return 0, 0, false
	}

	uidSize := int(data[1])
	if 2+uidSize > len(data) {
		return 0, 0, false
	}
	rUID := binkit.NewReader(data[2 : 2+uidSize])
	switch uidSize {
	case 1:
		uid = uint32(rUID.U8())
	case 2:
		uid = uint32(rUID.U16LE())
	case 4:
		uid = rUID.U32LE()
	}

	pos := 2 + uidSize
	if pos >= len(data) {
		return 0, 0, false
	}
	gidSize := int(data[pos])
	pos++
	if pos+gidSize > len(data) {
		return 0, 0, false
	}
	rGID := binkit.NewReader(data[pos : pos+gidSize])
	switch gidSize {
	case 1:
		gid = uint32(rGID.U8())
	case 2:
		gid = uint32(rGID.U16LE())
	case 4:
		gid = rGID.U32LE()
	}

	return uid, gid, true
}

// EncodeInfoZipUIDGID formats Info-ZIP 0x7875 UID/GID extra field.
func EncodeInfoZipUIDGID(uid, gid uint32) []byte {
	w := binkit.NewWriter(nil, 4+1+1+4+1+4)
	w.U16LE(TagInfoZipUIDGID).
		U16LE(11).
		U8(1).
		U8(4).
		U32LE(uid).
		U8(4).
		U32LE(gid)
	return w.Bytes()
}

// -----------------------------------------------------------------------------
// 5. Zip64 Extra Field Plugin (Tag 0x0001)
// -----------------------------------------------------------------------------

type zip64ExtraHandler struct{}

func (h *zip64ExtraHandler) Tag() uint16 { return TagZip64 }

func (h *zip64ExtraHandler) Parse(payload []byte, _ *file.File, ctx *HeaderContext) error {
	if ctx == nil {
		return nil
	}
	r := binkit.NewReader(payload)
	if ctx.Uncompressed != nil && *ctx.Uncompressed == int64(MaxUint32) && r.Remaining() >= 8 {
		*ctx.Uncompressed = int64(r.U64LE())
	}
	if ctx.Compressed != nil && *ctx.Compressed == int64(MaxUint32) && r.Remaining() >= 8 {
		*ctx.Compressed = int64(r.U64LE())
	}
	if ctx.LocalOffset != nil && *ctx.LocalOffset == int64(MaxUint32) && r.Remaining() >= 8 {
		*ctx.LocalOffset = int64(r.U64LE())
	}
	return nil
}

// -----------------------------------------------------------------------------
// 6. Data Stream Alignment Helper (Tag 0xa11e & 0xd935)
// -----------------------------------------------------------------------------

// EncodeDataStreamAlignment formats tag 0xa11e (Apache Commons-Compress) or tag 0xd935 (Android APK).
func EncodeDataStreamAlignment(alignment uint16, paddingCount int) []byte {
	dataSize := 2 + max(paddingCount, 0)

	w := binkit.NewWriter(nil, 4+dataSize)
	w.U16LE(TagDataStreamAlignment).
		U16LE(uint16(dataSize)).
		U16LE(alignment & 0x7fff)

	for range paddingCount {
		w.U8(0)
	}

	return w.Bytes()
}
