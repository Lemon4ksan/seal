// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iso

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lemon4ksan/seal/file"
)

// Reader provides random-access inspection and file extraction for ISO 9660 and Joliet disc images.
type Reader struct {
	rAt      io.ReaderAt
	size     int64
	volID    string
	isJoliet bool
	entries  []*file.File
	rootExt  uint32
	rootLen  uint32
}

// NewReader opens an ISO 9660 / Joliet disc image from an io.ReaderAt and parses its directory tree.
func NewReader(rAt io.ReaderAt, size int64) (*Reader, error) {
	if rAt == nil || size < int64(SystemAreaSize+SectorSize) {
		return nil, ErrInvalidISO
	}

	r := &Reader{
		rAt:  rAt,
		size: size,
	}

	if err := r.parseVolumeDescriptors(); err != nil {
		return nil, err
	}

	if err := r.readDirectoryTree(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Reader) parseVolumeDescriptors() error {
	var pvdRootExt, pvdRootLen uint32
	var jolietRootExt, jolietRootLen uint32
	var foundPVD, foundJoliet bool

	sector := make([]byte, SectorSize)
	sectorNum := SystemAreaSectors

	for {
		offset := int64(sectorNum) * SectorSize
		if offset+SectorSize > r.size {
			break
		}

		if _, err := r.rAt.ReadAt(sector, offset); err != nil {
			break
		}

		// Check "CD001" signature at byte 1..6
		if string(sector[1:6]) != StandardIdentifier {
			break
		}

		vdType := sector[0]
		if vdType == VDTypeVolumeDescriptorSetTerminator {
			break
		}

		switch vdType {
		case VDTypePrimaryVolumeDescriptor:
			foundPVD = true
			r.volID = strings.TrimSpace(string(sector[40:72]))
			pvdRoot, _, err := ParseDirectoryRecord(sector[156:190], false)
			if err == nil && pvdRoot != nil {
				pvdRootExt = pvdRoot.ExtentLBA
				pvdRootLen = pvdRoot.DataLength
			}

		case VDTypeSupplementaryVolumeDescriptor:
			// Check Joliet escape sequences (bytes 88..120)
			escapes := string(sector[88:120])
			if strings.Contains(escapes, "\x25\x2F\x40") ||
				strings.Contains(escapes, "\x25\x2F\x43") ||
				strings.Contains(escapes, "\x25\x2F\x45") {
				jolietRoot, _, err := ParseDirectoryRecord(sector[156:190], true)
				if err == nil && jolietRoot != nil {
					foundJoliet = true
					jolietRootExt = jolietRoot.ExtentLBA
					jolietRootLen = jolietRoot.DataLength
				}
			}
		}

		sectorNum++
	}

	if !foundPVD && !foundJoliet {
		return ErrInvalidISO
	}

	if foundJoliet {
		r.isJoliet = true
		r.rootExt = jolietRootExt
		r.rootLen = jolietRootLen
	} else {
		r.isJoliet = false
		r.rootExt = pvdRootExt
		r.rootLen = pvdRootLen
	}

	return nil
}

func (r *Reader) readDirectoryTree() error {
	return r.scanDirectory(r.rootExt, r.rootLen, "")
}

func (r *Reader) scanDirectory(extentLBA, dataLength uint32, parentPath string) error {
	if dataLength == 0 {
		return nil
	}

	data := make([]byte, dataLength)
	offset := int64(extentLBA) * SectorSize
	if offset+int64(dataLength) > r.size {
		dataLength = uint32(r.size - offset)
		data = data[:dataLength]
	}

	if _, err := r.rAt.ReadAt(data, offset); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("iso: read directory extent %d: %w", extentLBA, err)
	}

	// Directory records cannot cross 2048-byte sector boundaries
	for secStart := 0; secStart < len(data); secStart += SectorSize {
		secEnd := min(secStart+SectorSize, len(data))
		secData := data[secStart:secEnd]
		pos := 0

		for pos < len(secData) {
			dr, recLen, err := ParseDirectoryRecord(secData[pos:], r.isJoliet)
			if err != nil {
				return err
			}
			if recLen == 0 || dr == nil {
				// Remaining bytes in this sector are zero padding; advance to next sector
				break
			}
			pos += recLen

			// Skip '.' and '..'
			if dr.Name == "." || dr.Name == ".." || dr.Name == "" {
				continue
			}

			fullPath := dr.Name
			if parentPath != "" {
				fullPath = parentPath + "/" + dr.Name
			}

			cleanPath := strings.TrimPrefix(fullPath, "/")
			isDir := dr.IsDir()
			if isDir && !strings.HasSuffix(cleanPath, "/") {
				cleanPath += "/"
			}

			f, err := file.New(cleanPath, isDir)
			if err != nil {
				return fmt.Errorf("iso: create file record %q: %w", cleanPath, err)
			}

			f.SetSizes(int64(dr.DataLength), int64(dr.DataLength), 0)
			modTime := dr.ModTime()
			if !modTime.IsZero() {
				f.WithModTime(modTime)
			}

			if !isDir {
				fileOffset := int64(dr.ExtentLBA) * SectorSize
				fileSize := int64(dr.DataLength)
				f.WithOpenFunc(func() (io.ReadCloser, error) {
					if fileOffset+fileSize > r.size {
						return nil, io.ErrUnexpectedEOF
					}
					return io.NopCloser(io.NewSectionReader(r.rAt, fileOffset, fileSize)), nil
				})
			}

			r.entries = append(r.entries, f)

			if isDir {
				subParent := strings.TrimSuffix(cleanPath, "/")
				if err := r.scanDirectory(dr.ExtentLBA, dr.DataLength, subParent); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// Entries returns all parsed file entries in the ISO image.
func (r *Reader) Entries() []*file.File {
	return r.entries
}

// VolumeID returns the ISO volume identifier string.
func (r *Reader) VolumeID() string {
	return r.volID
}

// IsJoliet returns whether the image contains and uses Joliet Unicode extensions.
func (r *Reader) IsJoliet() bool {
	return r.isJoliet
}
