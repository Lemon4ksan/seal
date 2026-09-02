// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wim

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/lemon4ksan/seal/file"
)

type blobRecord struct {
	sha1     [20]byte
	offset   uint64
	size     uint64
	refCount uint32
	flags    uint8
}

type wimNode struct {
	name       string
	isDir      bool
	size       uint64
	modTime    time.Time
	accessTime uint64
	createTime uint64
	sha1       [20]byte
	children   []*wimNode
}

// Writer creates compliant Windows Imaging Format (WIM) archives.
type Writer struct {
	w io.Writer
}

// NewWriter constructs a new WIM Writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteArchive serializes files into w as a WIM image with content deduplication.
func (ww *Writer) WriteArchive(files []*file.File) (int64, error) {
	// If writer is not a WriteSeeker, buffer payload
	seeker, isSeeker := ww.w.(io.WriteSeeker)
	var bodyBuf *bytes.Buffer
	var out io.Writer

	if isSeeker {
		out = seeker
		// Write 208-byte blank header placeholder
		placeholder := make([]byte, HeaderSize)
		if _, err := out.Write(placeholder); err != nil {
			return 0, err
		}
	} else {
		bodyBuf = new(bytes.Buffer)
		out = bodyBuf
	}

	var currentOffset uint64 = HeaderSize
	blobMap := make(map[[20]byte]*blobRecord)
	var blobs []*blobRecord

	// 1. Process files and write data streams with SHA-1 content deduplication
	type fileBlobRef struct {
		file *file.File
		sha1 [20]byte
	}
	fileRefs := make([]fileBlobRef, len(files))

	var totalFileBytes uint64
	var fileCount uint32
	var dirCount uint32

	for i, f := range files {
		if f.IsDir() {
			dirCount++
			fileRefs[i] = fileBlobRef{file: f}
			continue
		}
		fileCount++
		totalFileBytes += uint64(f.UncompressedSize())

		rc, err := f.Open()
		if err != nil {
			return 0, fmt.Errorf("wim: opening file %s: %w", f.Name(), err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return 0, fmt.Errorf("wim: reading file %s: %w", f.Name(), err)
		}

		if len(data) == 0 {
			fileRefs[i] = fileBlobRef{file: f}
			continue
		}

		hash := sha1.Sum(data)
		fileRefs[i] = fileBlobRef{file: f, sha1: hash}

		if existing, ok := blobMap[hash]; ok {
			existing.refCount++
		} else {
			// Write uncompressed data blob
			n, err := out.Write(data)
			if err != nil {
				return 0, err
			}
			rec := &blobRecord{
				sha1:     hash,
				offset:   currentOffset,
				size:     uint64(n),
				refCount: 1,
				flags:    0,
			}
			blobMap[hash] = rec
			blobs = append(blobs, rec)
			currentOffset += uint64(n)
		}
	}

	// 2. Build hierarchical directory tree
	root := &wimNode{name: "", isDir: true}
	for _, ref := range fileRefs {
		cleanPath := strings.Trim(path.Clean(ref.file.Name()), "/\\")
		if cleanPath == "." || cleanPath == "" {
			continue
		}
		parts := strings.Split(cleanPath, "/")
		curr := root
		for pIdx, part := range parts {
			isLast := pIdx == len(parts)-1
			var child *wimNode
			for _, c := range curr.children {
				if strings.EqualFold(c.name, part) {
					child = c
					break
				}
			}
			if child == nil {
				aTime := ref.file.Metadata().LastAccessTime
				if aTime == 0 {
					aTime = TimeToFiletime(ref.file.ModTime())
				}
				cTime := ref.file.Metadata().CreationTime
				if cTime == 0 {
					cTime = TimeToFiletime(ref.file.ModTime())
				}
				child = &wimNode{
					name:       part,
					isDir:      !isLast || ref.file.IsDir(),
					modTime:    ref.file.ModTime(),
					accessTime: aTime,
					createTime: cTime,
				}
				if isLast && !ref.file.IsDir() {
					child.size = uint64(ref.file.UncompressedSize())
					child.sha1 = ref.sha1
				}
				curr.children = append(curr.children, child)
			} else if isLast && !ref.file.IsDir() {
				child.size = uint64(ref.file.UncompressedSize())
				child.sha1 = ref.sha1
				child.modTime = ref.file.ModTime()
			}
			curr = child
		}
	}

	// 3. Serialize metadata resource (Security data + Directory tree)
	metaData := serializeDentryTree(root)
	metaHash := sha1.Sum(metaData)

	metaOffset := currentOffset
	nMeta, err := out.Write(metaData)
	if err != nil {
		return 0, err
	}
	currentOffset += uint64(nMeta)

	metaBlob := &blobRecord{
		sha1:     metaHash,
		offset:   metaOffset,
		size:     uint64(nMeta),
		refCount: 1,
		flags:    ResFlagMetadata,
	}
	blobs = append(blobs, metaBlob)

	// 4. Write Lookup Table
	lookupOffset := currentOffset
	var lookupTableBuf bytes.Buffer
	for _, b := range blobs {
		entry := LookupEntry{
			PartNumber: 1,
			RefCount:   b.refCount,
			SHA1:       b.sha1,
		}
		entry.ResHdr.Set(b.size, b.flags, b.offset, b.size)
		if err := entry.Write(&lookupTableBuf); err != nil {
			return 0, err
		}
	}

	nLookup, err := out.Write(lookupTableBuf.Bytes())
	if err != nil {
		return 0, err
	}
	currentOffset += uint64(nLookup)

	// 5. Generate and write XML Data
	xmlData := generateWimXML(currentOffset, totalFileBytes, fileCount, dirCount)
	xmlUTF16 := EncodeUTF16LE(xmlData)
	var xmlBuf bytes.Buffer
	// UTF-16LE BOM
	xmlBuf.Write([]byte{0xFF, 0xFE})
	xmlBuf.Write(xmlUTF16)

	xmlOffset := currentOffset
	nXML, err := out.Write(xmlBuf.Bytes())
	if err != nil {
		return 0, err
	}
	currentOffset += uint64(nXML)

	// 6. Build final 208-byte WIM Header
	var header Header
	copy(header.Magic[:], Magic)
	header.HeaderSize = HeaderSize
	header.Version = Version
	header.Flags = FlagHeaderRPFix
	header.ChunkSize = 0
	header.PartNumber = 1
	header.TotalParts = 1
	header.ImageCount = 1
	_, _ = rand.Read(header.GUID[:])

	header.LookupTable.Set(uint64(nLookup), ResFlagMetadata, lookupOffset, uint64(nLookup))
	header.XMLData.Set(uint64(nXML), ResFlagMetadata, xmlOffset, uint64(nXML))

	if isSeeker {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
		if err := header.Write(seeker); err != nil {
			return 0, err
		}
		if _, err := seeker.Seek(0, io.SeekEnd); err != nil {
			return 0, err
		}
		return int64(currentOffset), nil
	}

	// For stream writers: write header, then body
	var fullHeader bytes.Buffer
	if err := header.Write(&fullHeader); err != nil {
		return 0, err
	}
	writtenHdr, err := ww.w.Write(fullHeader.Bytes())
	if err != nil {
		return int64(writtenHdr), err
	}
	writtenBody, err := ww.w.Write(bodyBuf.Bytes())
	return int64(writtenHdr + writtenBody), err
}

func serializeDentryTree(root *wimNode) []byte {
	// Breadth-First Search layout matching WIM specification:
	// Offset 0: Security Data (secTotal=8, secCount=0)
	// Offset 8: Root dentry
	// Followed by child groups for each directory
	type dirGroup struct {
		node         *wimNode
		parentBufOff uint64
	}

	groups := []*dirGroup{{node: root, parentBufOff: 8}}
	groupOffsets := make([]uint64, 0, 16)

	// Pass 1: compute layout and offsets
	pos := uint64(8) // security data size
	rootSize := dentrySize(root)
	pos += rootSize
	pos += 8 // Root's 8-byte null terminator

	gi := 0
	for gi < len(groups) {
		parent := groups[gi].node
		groupOffsets = append(groupOffsets, pos)

		// Sort children: directories first, then alphabetical
		sort.Slice(parent.children, func(i, j int) bool {
			if parent.children[i].isDir != parent.children[j].isDir {
				return parent.children[i].isDir
			}
			return parent.children[i].name < parent.children[j].name
		})

		for _, child := range parent.children {
			childSize := dentrySize(child)
			childOff := pos
			pos += childSize

			if child.isDir {
				groups = append(groups, &dirGroup{
					node:         child,
					parentBufOff: childOff,
				})
			}
		}
		pos += 8 // 8-byte null terminator
		gi++
	}

	// Pass 2: allocate and serialize
	buf := make([]byte, pos)
	binary.LittleEndian.PutUint32(buf[0:4], 8) // secTotal
	binary.LittleEndian.PutUint32(buf[4:8], 0) // secCount

	// Serialize root dentry
	var rootSubdir uint64
	if len(groups) > 0 && len(root.children) > 0 {
		rootSubdir = groupOffsets[0]
	}
	serializeOneDentry(root, rootSubdir, buf[8:8+rootSize])

	// Serialize each child group
	dirIndex := 1
	for gIdx, grp := range groups {
		cPos := groupOffsets[gIdx]
		for _, child := range grp.node.children {
			childSize := dentrySize(child)
			var childSubdir uint64
			if child.isDir {
				if dirIndex < len(groupOffsets) && len(child.children) > 0 {
					childSubdir = groupOffsets[dirIndex]
				}
				dirIndex++
			}
			serializeOneDentry(child, childSubdir, buf[cPos:cPos+childSize])
			cPos += childSize
		}
		// 8-byte null terminator remains zeros
	}

	return buf
}

func dentrySize(n *wimNode) uint64 {
	nameLen := uint64(len([]rune(n.name)) * 2)
	total := uint64(DentryFixedSize)
	if nameLen > 0 {
		total += nameLen + 2 // UTF-16LE + null terminator
	}
	// Align to 8-byte boundary
	return (total + 7) &^ 7
}

func serializeOneDentry(n *wimNode, subdirOffset uint64, buf []byte) {
	nameUTF16 := EncodeUTF16LE(n.name)
	nameLen := uint16(len(nameUTF16))
	totalLen := uint64(len(buf))

	binary.LittleEndian.PutUint64(buf[0:8], totalLen)
	attr := AttributeArchive
	if n.isDir {
		attr = AttributeDirectory
	}
	binary.LittleEndian.PutUint32(buf[8:12], attr)
	binary.LittleEndian.PutUint32(buf[12:16], 0xFFFFFFFF) // SecurityID: -1
	binary.LittleEndian.PutUint64(buf[16:24], subdirOffset)
	// buf[24:40] unused1, unused2 = 0
	cTime := n.createTime
	aTime := n.accessTime
	mTime := TimeToFiletime(n.modTime)
	binary.LittleEndian.PutUint64(buf[40:48], cTime)
	binary.LittleEndian.PutUint64(buf[48:56], aTime)
	binary.LittleEndian.PutUint64(buf[56:64], mTime)
	copy(buf[64:84], n.sha1[:])
	// buf[84:96] reparseTag, hardlinkID = 0
	binary.LittleEndian.PutUint16(buf[96:98], 0)  // numStreams
	binary.LittleEndian.PutUint16(buf[98:100], 0) // shortNameLen
	binary.LittleEndian.PutUint16(buf[100:102], nameLen)

	if nameLen > 0 {
		copy(buf[DentryFixedSize:], nameUTF16)
		// 2-byte null terminator is already 0 in zero-initialized buffer
	}
}

func generateWimXML(totalBytes, totalFileBytes uint64, fileCount, dirCount uint32) string {
	now := time.Now()
	ft := TimeToFiletime(now)
	hi := uint32(ft >> 32)
	lo := uint32(ft & 0xFFFFFFFF)

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<WIM>
<TOTALBYTES>%d</TOTALBYTES>
<IMAGE INDEX="1">
<DIRCOUNT>%d</DIRCOUNT>
<FILECOUNT>%d</FILECOUNT>
<TOTALBYTES>%d</TOTALBYTES>
<CREATIONTIME><HIGHPART>0x%08X</HIGHPART><LOWPART>0x%08X</LOWPART></CREATIONTIME>
<LASTMODIFICATIONTIME><HIGHPART>0x%08X</HIGHPART><LOWPART>0x%08X</LOWPART></LASTMODIFICATIONTIME>
<NAME>1</NAME>
</IMAGE>
</WIM>`, totalBytes, dirCount, fileCount, totalFileBytes, hi, lo, hi, lo)
}
