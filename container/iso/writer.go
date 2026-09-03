// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iso

import (
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

// Writer serializes file entries into a compliant ISO 9660 + Joliet disc image (.iso).
type Writer struct {
	dest io.Writer
}

// NewWriter constructs an ISO image serializer.
func NewWriter(dest io.Writer) *Writer {
	return &Writer{dest: dest}
}

type isoItem struct {
	f           *file.File
	isDir       bool
	name        string
	cleanPath   string
	dirPath     string
	data        []byte
	extentLBA   uint32
	sectorCount uint32
}

// WriteArchive builds and writes a complete ISO 9660 image with Joliet Unicode extensions.
func (w *Writer) WriteArchive(files []*file.File, cfg container.Config) (int64, error) {
	// Normalize items and read payloads
	itemsByDir := make(map[string][]*isoItem)
	var allDirs []string
	dirSeen := make(map[string]bool)

	ensureDirTree := func(d string) {
		d = path.Clean(d)
		if d == "." || d == "" {
			return
		}
		parts := strings.Split(d, "/")
		curr := ""
		for _, part := range parts {
			parent := curr
			if curr == "" {
				curr = part
			} else {
				curr = curr + "/" + part
			}
			if !dirSeen[curr] {
				dirSeen[curr] = true
				allDirs = append(allDirs, curr)
				dirItem := &isoItem{
					isDir:     true,
					name:      part,
					cleanPath: curr,
					dirPath:   parent,
				}
				itemsByDir[parent] = append(itemsByDir[parent], dirItem)
			}
		}
	}

	for _, f := range files {
		clean := strings.TrimPrefix(f.Name(), "/")
		clean = strings.TrimSuffix(clean, "/")
		if clean == "" || clean == "." {
			continue
		}

		dir := path.Dir(clean)
		base := path.Base(clean)
		isDir := f.IsDir()

		var data []byte
		if !isDir && f.UncompressedSize() > 0 {
			r, err := f.Open()
			if err != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(f, err)
				}
				return 0, fmt.Errorf("iso: open file %q: %w", f.Name(), err)
			}
			buf, err := io.ReadAll(r)
			_ = r.Close()
			if err != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(f, err)
				}
				return 0, fmt.Errorf("iso: read file %q: %w", f.Name(), err)
			}
			data = buf
		}

		if isDir {
			ensureDirTree(clean)
		} else {
			if dir != "." && dir != "" {
				ensureDirTree(dir)
			}
			parentDir := dir
			if parentDir == "." {
				parentDir = ""
			}

			item := &isoItem{
				f:         f,
				isDir:     false,
				name:      base,
				cleanPath: clean,
				dirPath:   parentDir,
				data:      data,
			}

			itemsByDir[parentDir] = append(itemsByDir[parentDir], item)
		}

		if cfg.OnFileDone != nil {
			cfg.OnFileDone(f, nil)
		}
	}

	// Sort directories by depth
	sort.Strings(allDirs)

	// Calculate sector layout
	// Sectors 0..15: System area (32768 bytes)
	// Sector 16: PVD
	// Sector 17: Joliet SVD
	// Sector 18: VDST
	// Sectors 19..: Directory sectors & File payload sectors
	nextLBA := uint32(19)

	// Assign LBAs for Joliet directories
	dirLBA := make(map[string]uint32)
	dirLBA[""] = nextLBA // Root directory
	nextLBA++

	for _, d := range allDirs {
		dirLBA[d] = nextLBA
		nextLBA++
	}

	// Assign LBAs for file data sectors
	for _, items := range itemsByDir {
		for _, item := range items {
			if !item.isDir {
				item.extentLBA = nextLBA
				size := len(item.data)
				secCount := uint32((size + SectorSize - 1) / SectorSize)
				if secCount == 0 {
					secCount = 1
				}
				item.sectorCount = secCount
				nextLBA += secCount
			}
		}
	}

	totalSectors := nextLBA
	totalBytes := int64(totalSectors) * SectorSize

	// 1. Write Sectors 0..15 (System Area: 32 KB of zeroes)
	systemArea := make([]byte, SystemAreaSize)
	if _, err := w.dest.Write(systemArea); err != nil {
		return 0, err
	}

	// 2. Write Sector 16: Primary Volume Descriptor (PVD)
	pvd := make([]byte, SectorSize)
	buildPVD(pvd, totalSectors, dirLBA[""], SectorSize)
	if _, err := w.dest.Write(pvd); err != nil {
		return 0, err
	}

	// 3. Write Sector 17: Supplementary Volume Descriptor (Joliet SVD)
	svd := make([]byte, SectorSize)
	buildJolietSVD(svd, totalSectors, dirLBA[""], SectorSize)
	if _, err := w.dest.Write(svd); err != nil {
		return 0, err
	}

	// 4. Write Sector 18: Volume Descriptor Set Terminator (VDST)
	vdst := make([]byte, SectorSize)
	vdst[0] = VDTypeVolumeDescriptorSetTerminator
	copy(vdst[1:6], StandardIdentifier)
	vdst[6] = 0x01
	if _, err := w.dest.Write(vdst); err != nil {
		return 0, err
	}

	// 5. Write Directory Sectors (Joliet UTF-16BE)
	// Write root directory
	if err := writeDirectorySector(w.dest, "", itemsByDir[""], dirLBA); err != nil {
		return 0, err
	}

	// Write subdirectories
	for _, d := range allDirs {
		if err := writeDirectorySector(w.dest, d, itemsByDir[d], dirLBA); err != nil {
			return 0, err
		}
	}

	// 6. Write File Data Extents
	zeroPad := make([]byte, SectorSize)
	for _, items := range itemsByDir {
		for _, item := range items {
			if !item.isDir {
				if _, err := w.dest.Write(item.data); err != nil {
					return 0, err
				}
				pad := int(item.sectorCount*SectorSize) - len(item.data)
				if pad > 0 {
					if _, err := w.dest.Write(zeroPad[:pad]); err != nil {
						return 0, err
					}
				}
			}
		}
	}

	return totalBytes, nil
}

func buildPVD(b []byte, totalSectors, rootLBA uint32, rootSize uint32) {
	b[0] = VDTypePrimaryVolumeDescriptor
	copy(b[1:6], StandardIdentifier)
	b[6] = 0x01
	copy(b[40:72], "SEAL_ISO_IMAGE                  ")

	PutBothEndianUint32(b[80:88], totalSectors)
	PutBothEndianUint16(b[120:124], 1)
	PutBothEndianUint16(b[124:128], 1)
	PutBothEndianUint16(b[128:132], SectorSize)

	// Root Directory Record at b[156:190]
	buildDirRecord(b[156:190], rootLBA, rootSize, time.Now(), true, []byte{0})
}

func buildJolietSVD(b []byte, totalSectors, rootLBA uint32, rootSize uint32) {
	b[0] = VDTypeSupplementaryVolumeDescriptor
	copy(b[1:6], StandardIdentifier)
	b[6] = 0x01
	copy(b[40:72], "SEAL_JOLIET_IMAGE               ")

	PutBothEndianUint32(b[80:88], totalSectors)
	// Joliet escape sequence (UCS-2 Level 3)
	b[88] = 0x25
	b[89] = 0x2F
	b[90] = 0x45

	PutBothEndianUint16(b[120:124], 1)
	PutBothEndianUint16(b[124:128], 1)
	PutBothEndianUint16(b[128:132], SectorSize)

	// Root Directory Record at b[156:190]
	buildDirRecord(b[156:190], rootLBA, rootSize, time.Now(), true, []byte{0})
}

func writeDirectorySector(w io.Writer, dirPath string, items []*isoItem, dirLBA map[string]uint32) error {
	sec := make([]byte, SectorSize)
	pos := 0

	now := time.Now()
	selfLBA := dirLBA[dirPath]
	parentLBA := selfLBA
	if dirPath != "" {
		parentDir := path.Dir(dirPath)
		if parentDir == "." {
			parentDir = ""
		}
		parentLBA = dirLBA[parentDir]
	}

	// 1. Record 0: '.' (current directory)
	dotRec := make([]byte, 34)
	buildDirRecord(dotRec, selfLBA, SectorSize, now, true, []byte{0})
	copy(sec[pos:], dotRec)
	pos += len(dotRec)

	// 2. Record 1: '..' (parent directory)
	dotDotRec := make([]byte, 34)
	buildDirRecord(dotDotRec, parentLBA, SectorSize, now, true, []byte{1})
	copy(sec[pos:], dotDotRec)
	pos += len(dotDotRec)

	// 3. Child entries (using Joliet UTF-16BE names)
	for _, item := range items {
		nameWithVer := item.name + ";1"
		u16Name := EncodeUTF16BE(nameWithVer)

		recLen := 33 + len(u16Name)
		if recLen%2 != 0 {
			recLen++ // 16-bit alignment padding
		}

		if pos+recLen > SectorSize {
			break
		}

		var extLBA uint32
		var dataLen uint32
		if item.isDir {
			fullSub := item.cleanPath
			extLBA = dirLBA[fullSub]
			dataLen = SectorSize
		} else {
			extLBA = item.extentLBA
			dataLen = uint32(len(item.data))
		}

		rec := make([]byte, recLen)
		modTime := now
		if item.f != nil && !item.f.ModTime().IsZero() {
			modTime = item.f.ModTime()
		}

		buildDirRecord(rec, extLBA, dataLen, modTime, item.isDir, u16Name)
		copy(sec[pos:], rec)
		pos += recLen
	}

	_, err := w.Write(sec)
	return err
}

func buildDirRecord(b []byte, extentLBA, dataLength uint32, modTime time.Time, isDir bool, nameBytes []byte) {
	b[0] = byte(len(b))
	b[1] = 0 // ExtAttrLength
	PutBothEndianUint32(b[2:10], extentLBA)
	PutBothEndianUint32(b[10:18], dataLength)

	dt := FormatDateTime(modTime)
	copy(b[18:25], dt[:])

	if isDir {
		b[25] = FileFlagDirectory
	} else {
		b[25] = 0
	}

	b[26] = 0 // FileUnitSize
	b[27] = 0 // InterleaveGapSize
	PutBothEndianUint16(b[28:32], 1)

	b[32] = byte(len(nameBytes))
	copy(b[33:], nameBytes)
}
