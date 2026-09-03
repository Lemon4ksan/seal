// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package eon

import (
	"crypto/sha256"
	"encoding/binary"
	"hash/crc32"
	"io"
	"strings"
	"time"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

// FixedExtentPrefixSize is the fixed size of an Extent record header before varints.
const FixedExtentPrefixSize = 58

// Writer serializes canonical files into an EON container archive.
type Writer struct {
	w          io.Writer
	blockShift uint8
	targetSize uint32
	profile    uint8
}

// NewWriter constructs an EON container writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w:          w,
		blockShift: DefaultBlockSizeShift,
		targetSize: DefaultChunkTargetSize,
		profile:    ProfileVFS,
	}
}

type extentMeta struct {
	namespace   uint16
	chunkIndex  uint32
	chunkCount  uint16
	flags       uint16
	timestamp   int64
	hash        [32]byte
	name        string
	target      string
	extraTLVs   []byte
	chunkOffset uint64
	payloadSize uint64
}

// WriteArchive serializes the slice of files into an EON container archive stream.
func (ew *Writer) WriteArchive(files []*file.File, cfg container.Config) (int64, error) {
	var totalWritten int64
	blockSize := int64(1 << ew.blockShift)
	zeroBuf := make([]byte, blockSize)

	// 1. Write 32-byte Archive Header at offset 0
	var header [HeaderSize]byte
	copy(header[0:4], MagicHeader[:])
	header[4] = VersionMajor
	header[5] = VersionMinor
	header[6] = ew.profile
	header[7] = 0 // Agnostic OS
	header[8] = 0 // Agnostic Arch
	header[9] = ew.blockShift
	flags := FlagCentralDir | FlagAlignedBlocks
	binary.LittleEndian.PutUint16(header[10:12], flags)
	binary.LittleEndian.PutUint32(header[12:16], ew.targetSize)
	binary.LittleEndian.PutUint64(header[16:24], uint64(time.Now().UnixNano()))
	hCRC := crc32.ChecksumIEEE(header[0:24])
	binary.LittleEndian.PutUint32(header[24:28], hCRC)

	if _, err := ew.w.Write(header[:]); err != nil {
		return totalWritten, err
	}
	totalWritten += HeaderSize

	// Pad header to first block boundary so Chunk 0 is block-aligned
	padHeader := blockSize - (totalWritten % blockSize)
	if padHeader < blockSize {
		if _, err := ew.w.Write(zeroBuf[:padHeader]); err != nil {
			return totalWritten, err
		}
		totalWritten += padHeader
	}

	// 2. Prepare chunks and extents
	var ctocEntries [][CTOCEntrySize]byte
	var extents []extentMeta

	currentRaw := make([]byte, 0, ew.targetSize)
	currentItems := 0

	flushChunk := func() error {
		if len(currentRaw) == 0 && currentItems == 0 {
			return nil
		}

		chunkIdx := uint32(len(ctocEntries))
		uncompressedSize := uint64(len(currentRaw))
		uncompressedHash := sha256.Sum256(currentRaw)

		// Compress with Zstandard
		compressedPayload, err := codec.CompressZstd(currentRaw, nil)
		if err != nil {
			return err
		}
		compressedSize := uint64(len(compressedPayload))
		blockIdx := uint64(totalWritten >> ew.blockShift)

		// 64-byte Chunk Header
		var chnkHdr [ChunkHeaderSize]byte
		copy(chnkHdr[0:4], MagicChunk[:])
		binary.LittleEndian.PutUint32(chnkHdr[4:8], chunkIdx)
		chnkHdr[8] = CodecZSTD
		chnkHdr[9] = 3 // default level
		chunkFlags := ChunkFlagSolid
		binary.LittleEndian.PutUint16(chnkHdr[10:12], chunkFlags)
		binary.LittleEndian.PutUint64(chnkHdr[12:20], uncompressedSize)
		binary.LittleEndian.PutUint64(chnkHdr[20:28], compressedSize)
		binary.LittleEndian.PutUint32(chnkHdr[28:32], uint32(currentItems))
		copy(chnkHdr[32:64], uncompressedHash[:])

		if _, err := ew.w.Write(chnkHdr[:]); err != nil {
			return err
		}
		totalWritten += ChunkHeaderSize

		if _, err := ew.w.Write(compressedPayload); err != nil {
			return err
		}
		totalWritten += int64(compressedSize)

		// Pad chunk to logical block boundary
		rem := totalWritten % blockSize
		if rem != 0 {
			pad := blockSize - rem
			if _, err := ew.w.Write(zeroBuf[:pad]); err != nil {
				return err
			}
			totalWritten += pad
		}

		// Record CTOC Entry (16 bytes)
		var ctocEntry [CTOCEntrySize]byte
		binary.LittleEndian.PutUint64(ctocEntry[0:8], blockIdx)
		binary.LittleEndian.PutUint32(ctocEntry[8:12], uint32(compressedSize))
		binary.LittleEndian.PutUint32(ctocEntry[12:16], uint32(uncompressedSize))
		ctocEntries = append(ctocEntries, ctocEntry)

		// Reset chunk accumulator
		currentRaw = currentRaw[:0]
		currentItems = 0
		return nil
	}

	for _, f := range files {
		cleanName := strings.TrimPrefix(f.Name(), "/")
		if cleanName == "" {
			continue
		}

		isDir := f.IsDir()
		if isDir && !strings.HasSuffix(cleanName, "/") {
			cleanName += "/"
		}

		var payload []byte
		if !isDir {
			rc, err := f.Open()
			if err != nil {
				return totalWritten, err
			}
			payload, err = io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return totalWritten, err
			}
		}

		// Flush if this file payload pushes raw chunk beyond target size
		if len(currentRaw) > 0 && uint32(len(currentRaw)+len(payload)) > ew.targetSize {
			if err := flushChunk(); err != nil {
				return totalWritten, err
			}
		}

		fileHash := sha256.Sum256(payload)
		chunkIdx := uint32(len(ctocEntries))
		chunkOffset := uint64(len(currentRaw))
		payloadSize := uint64(len(payload))

		if len(payload) > 0 {
			currentRaw = append(currentRaw, payload...)
		}
		currentItems++

		// Build POSIX mode TLV ('PX')
		mode := uint32(f.Mode().Perm())
		if isDir {
			mode |= 0x4000 // S_IFDIR
		} else {
			mode |= 0x8000 // S_IFREG
		}
		var pxTLV [16]byte
		binary.LittleEndian.PutUint16(pxTLV[0:2], TagPOSIXPermissions)
		binary.LittleEndian.PutUint16(pxTLV[2:4], 12) // length
		binary.LittleEndian.PutUint32(pxTLV[4:8], mode)
		binary.LittleEndian.PutUint32(pxTLV[8:12], 0) // UID
		binary.LittleEndian.PutUint32(pxTLV[12:16], 0) // GID

		extents = append(extents, extentMeta{
			namespace:   NamespaceVFS,
			chunkIndex:  chunkIdx,
			chunkCount:  1,
			flags:       0,
			timestamp:   f.ModTime().UnixNano(),
			hash:        fileHash,
			name:        cleanName,
			target:      "",
			extraTLVs:   pxTLV[:],
			chunkOffset: chunkOffset,
			payloadSize: payloadSize,
		})

		if cfg.OnFileDone != nil {
			cfg.OnFileDone(f, nil)
		}
	}

	// Flush last chunk
	if err := flushChunk(); err != nil {
		return totalWritten, err
	}

	// 3. Central Directory Section
	centralDirOffset := uint64(totalWritten)

	// Write CTOC Header (16 bytes)
	var ctocHdr [CTOCHeaderSize]byte
	copy(ctocHdr[0:4], MagicCTOC[:])
	binary.LittleEndian.PutUint32(ctocHdr[4:8], uint32(len(ctocEntries)))
	binary.LittleEndian.PutUint32(ctocHdr[8:12], 0) // HotChunksCount
	binary.LittleEndian.PutUint32(ctocHdr[12:16], 0) // WarmChunksCount
	if _, err := ew.w.Write(ctocHdr[:]); err != nil {
		return totalWritten, err
	}
	totalWritten += CTOCHeaderSize

	// Write CTOC Entries
	for _, entry := range ctocEntries {
		if _, err := ew.w.Write(entry[:]); err != nil {
			return totalWritten, err
		}
		totalWritten += CTOCEntrySize
	}

	// Write INDX Header (8 bytes)
	var indxHdr [INDXHeaderSize]byte
	copy(indxHdr[0:4], MagicINDX[:])
	binary.LittleEndian.PutUint32(indxHdr[4:8], uint32(len(extents)))
	if _, err := ew.w.Write(indxHdr[:]); err != nil {
		return totalWritten, err
	}
	totalWritten += INDXHeaderSize

	// Write Universal Extent Records
	for _, ext := range extents {
		nameBytes := []byte(ext.name)
		targetBytes := []byte(ext.target)

		var varints []byte
		varints = AppendULEB128(varints, ext.chunkOffset)
		varints = AppendULEB128(varints, ext.payloadSize)

		recLen := FixedExtentPrefixSize + len(varints) + len(targetBytes) + len(nameBytes) + len(ext.extraTLVs)

		var prefix [FixedExtentPrefixSize]byte
		binary.LittleEndian.PutUint16(prefix[0:2], uint16(recLen))
		binary.LittleEndian.PutUint16(prefix[2:4], ext.namespace)
		binary.LittleEndian.PutUint32(prefix[4:8], ext.chunkIndex)
		binary.LittleEndian.PutUint16(prefix[8:10], ext.chunkCount)
		binary.LittleEndian.PutUint16(prefix[10:12], ext.flags)
		binary.LittleEndian.PutUint64(prefix[12:20], uint64(ext.timestamp))
		copy(prefix[20:52], ext.hash[:])
		binary.LittleEndian.PutUint16(prefix[52:54], uint16(len(targetBytes)))
		binary.LittleEndian.PutUint16(prefix[54:56], uint16(len(nameBytes)))
		binary.LittleEndian.PutUint16(prefix[56:58], uint16(len(ext.extraTLVs)))

		if _, err := ew.w.Write(prefix[:]); err != nil {
			return totalWritten, err
		}
		if _, err := ew.w.Write(varints); err != nil {
			return totalWritten, err
		}
		if len(targetBytes) > 0 {
			if _, err := ew.w.Write(targetBytes); err != nil {
				return totalWritten, err
			}
		}
		if len(nameBytes) > 0 {
			if _, err := ew.w.Write(nameBytes); err != nil {
				return totalWritten, err
			}
		}
		if len(ext.extraTLVs) > 0 {
			if _, err := ew.w.Write(ext.extraTLVs); err != nil {
				return totalWritten, err
			}
		}

		totalWritten += int64(recLen)
	}

	// 4. Archive Trailer (32 bytes at EOF)
	var trailer [TrailerSize]byte
	binary.LittleEndian.PutUint64(trailer[0:8], centralDirOffset)
	binary.LittleEndian.PutUint64(trailer[8:16], 0) // RecoveryOffset (0 = absent)
	binary.LittleEndian.PutUint64(trailer[16:24], 0) // ArchiveStartOffset (0 = no stub)
	flagsCopy := uint32(FlagCentralDir | FlagAlignedBlocks)
	binary.LittleEndian.PutUint32(trailer[24:28], flagsCopy)
	copy(trailer[28:32], MagicTrailer[:])

	if _, err := ew.w.Write(trailer[:]); err != nil {
		return totalWritten, err
	}
	totalWritten += TrailerSize

	return totalWritten, nil
}
