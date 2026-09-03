// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cab

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

// Writer serializes files into a standard Microsoft Cabinet (.cab) archive.
type Writer struct {
	dest        io.Writer
	compression uint16
}

// NewWriter constructs a Cabinet archive serializer.
func NewWriter(dest io.Writer, compMethod uint16) *Writer {
	if compMethod != CompTypeNone {
		compMethod = CompTypeMSZip
	}
	return &Writer{
		dest:        dest,
		compression: compMethod,
	}
}

// WriteArchive serializes the slice of files into a fully compliant Microsoft Cabinet file.
func (w *Writer) WriteArchive(files []*file.File, cfg container.Config) (int64, error) {
	type fileRecord struct {
		f        *file.File
		uoff     uint32
		size     uint32
		date     uint16
		time     uint16
		attribs  uint16
		diskName string
	}

	var records []fileRecord
	var folderBytes bytes.Buffer
	currentUoff := uint32(0)

	for _, f := range files {
		cleanName := strings.TrimPrefix(f.Name(), "/")
		if cleanName == "" {
			continue
		}

		// CAB uses backslashes for on-disk file path separation
		diskName := strings.ReplaceAll(cleanName, "/", "\\")
		date, dosTime := TimeToDos(f.ModTime())

		attribs := AttrArchive
		if f.IsDir() {
			attribs |= AttrDirectory
		}

		var fileSize uint32
		if !f.IsDir() && f.UncompressedSize() > 0 {
			r, err := f.Open()
			if err != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(f, err)
				}
				return 0, fmt.Errorf("cab: open file %q: %w", f.Name(), err)
			}
			data, err := io.ReadAll(r)
			_ = r.Close()
			if err != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(f, err)
				}
				return 0, fmt.Errorf("cab: read file %q: %w", f.Name(), err)
			}
			fileSize = uint32(len(data))
			folderBytes.Write(data)
		}

		records = append(records, fileRecord{
			f:        f,
			uoff:     currentUoff,
			size:     fileSize,
			date:     date,
			time:     dosTime,
			attribs:  uint16(attribs),
			diskName: diskName,
		})

		currentUoff += fileSize
		if cfg.OnFileDone != nil {
			cfg.OnFileDone(f, nil)
		}
	}

	// Partition folderBytes into MaxBlockUncompressedSize (32KB) blocks
	type dataBlock struct {
		cbUncomp uint16
		payload  []byte
	}

	uncompData := folderBytes.Bytes()
	var blocks []dataBlock

	if len(uncompData) == 0 {
		// Empty archive or directories only: single empty block
		blocks = append(blocks, dataBlock{
			cbUncomp: 0,
			payload:  nil,
		})
	} else {
		for len(uncompData) > 0 {
			chunkSize := len(uncompData)
			if chunkSize > MaxBlockUncompressedSize {
				chunkSize = MaxBlockUncompressedSize
			}
			chunk := uncompData[:chunkSize]
			uncompData = uncompData[chunkSize:]

			if w.compression == CompTypeMSZip {
				var compBuf bytes.Buffer
				compBuf.WriteByte('C')
				compBuf.WriteByte('K')

				flateW, err := flate.NewWriter(&compBuf, flate.DefaultCompression)
				if err != nil {
					return 0, fmt.Errorf("cab: create flate writer: %w", err)
				}
				if _, err := flateW.Write(chunk); err != nil {
					_ = flateW.Close()
					return 0, fmt.Errorf("cab: compress block: %w", err)
				}
				if err := flateW.Close(); err != nil {
					return 0, fmt.Errorf("cab: close flate writer: %w", err)
				}

				blocks = append(blocks, dataBlock{
					cbUncomp: uint16(chunkSize),
					payload:  compBuf.Bytes(),
				})
			} else {
				// CompTypeNone
				blocks = append(blocks, dataBlock{
					cbUncomp: uint16(chunkSize),
					payload:  chunk,
				})
			}
		}
	}

	// Calculate layout offsets
	// CFHEADER = 36 bytes
	// CFFOLDER = 8 bytes
	headerSize := uint32(36)
	folderTableSize := uint32(8)
	coffFiles := headerSize + folderTableSize

	filesTableSize := uint32(0)
	for _, rec := range records {
		// 16 bytes fixed header + name + 1 null byte
		filesTableSize += 16 + uint32(len(rec.diskName)) + 1
	}

	coffCabStart := coffFiles + filesTableSize
	totalCabinetSize := coffCabStart

	for _, blk := range blocks {
		// CFDATA = 8 bytes header + len(payload)
		totalCabinetSize += 8 + uint32(len(blk.payload))
	}

	// Write CFHEADER
	var cfHdr [36]byte
	binary.LittleEndian.PutUint32(cfHdr[0:4], MagicSignature)
	binary.LittleEndian.PutUint32(cfHdr[4:8], 0) // Reserved1
	binary.LittleEndian.PutUint32(cfHdr[8:12], totalCabinetSize)
	binary.LittleEndian.PutUint32(cfHdr[12:16], 0) // Reserved2
	binary.LittleEndian.PutUint32(cfHdr[16:20], coffFiles)
	binary.LittleEndian.PutUint32(cfHdr[20:24], 0) // Reserved3
	cfHdr[24] = 3                                  // VersionMinor
	cfHdr[25] = 1                                  // VersionMajor
	binary.LittleEndian.PutUint16(cfHdr[26:28], 1) // CFolders
	binary.LittleEndian.PutUint16(cfHdr[28:30], uint16(len(records)))
	binary.LittleEndian.PutUint16(cfHdr[30:32], 0) // Flags
	binary.LittleEndian.PutUint16(cfHdr[32:34], 0) // SetID
	binary.LittleEndian.PutUint16(cfHdr[34:36], 0) // ICabinet

	if _, err := w.dest.Write(cfHdr[:]); err != nil {
		return 0, err
	}

	// Write CFFOLDER
	var fHdr [8]byte
	binary.LittleEndian.PutUint32(fHdr[0:4], coffCabStart)
	binary.LittleEndian.PutUint16(fHdr[4:6], uint16(len(blocks)))
	binary.LittleEndian.PutUint16(fHdr[6:8], w.compression)

	if _, err := w.dest.Write(fHdr[:]); err != nil {
		return 0, err
	}

	// Write CFFILE entries
	for _, rec := range records {
		var feHdr [16]byte
		binary.LittleEndian.PutUint32(feHdr[0:4], rec.size)
		binary.LittleEndian.PutUint32(feHdr[4:8], rec.uoff)
		binary.LittleEndian.PutUint16(feHdr[8:10], 0) // iFolder = 0
		binary.LittleEndian.PutUint16(feHdr[10:12], rec.date)
		binary.LittleEndian.PutUint16(feHdr[12:14], rec.time)
		binary.LittleEndian.PutUint16(feHdr[14:16], rec.attribs)

		if _, err := w.dest.Write(feHdr[:]); err != nil {
			return 0, err
		}
		if _, err := io.WriteString(w.dest, rec.diskName); err != nil {
			return 0, err
		}
		if _, err := w.dest.Write([]byte{0}); err != nil {
			return 0, err
		}
	}

	// Write CFDATA blocks
	for _, blk := range blocks {
		var dataHdr [8]byte
		binary.LittleEndian.PutUint32(dataHdr[0:4], 0) // csum
		binary.LittleEndian.PutUint16(dataHdr[4:6], uint16(len(blk.payload)))
		binary.LittleEndian.PutUint16(dataHdr[6:8], blk.cbUncomp)

		if _, err := w.dest.Write(dataHdr[:]); err != nil {
			return 0, err
		}
		if len(blk.payload) > 0 {
			if _, err := w.dest.Write(blk.payload); err != nil {
				return 0, err
			}
		}
	}

	return int64(totalCabinetSize), nil
}
