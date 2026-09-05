// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package eon

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash/crc32"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/seal/codec/przm"
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
	codecID    uint8
	codecLevel int
}

// NewWriter constructs an EON container writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w:          w,
		blockShift: DefaultBlockSizeShift,
		targetSize: DefaultChunkTargetSize,
		profile:    ProfileVFS,
		codecID:    CodecZSTD,
		codecLevel: 3,
	}
}

// WithCodec sets the chunk compression algorithm and level.
func (ew *Writer) WithCodec(codecID uint8, level int) *Writer {
	ew.codecID = codecID
	ew.codecLevel = level
	return ew
}

// WithTargetSize sets the uncompressed chunk target threshold.
func (ew *Writer) WithTargetSize(targetSize uint32) *Writer {
	ew.targetSize = targetSize
	return ew
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
	extraTLVs   [16]byte
	extraLen    uint16
	chunkOffset uint64
	payloadSize uint64
}

// WriteArchive serializes the slice of files into an EON container archive stream.
func (ew *Writer) WriteArchive(files []*file.File, cfg container.Config) (int64, error) {
	if cfg.DefaultMethod == codec.PRZM {
		ew.codecID = CodecPRZM
		if cfg.DefaultLevel > 0 {
			ew.codecLevel = cfg.DefaultLevel
		}
		if ew.targetSize == DefaultChunkTargetSize || ew.targetSize > 4*1024*1024 {
			ew.targetSize = 4 * 1024 * 1024
		}
	} else if cfg.DefaultMethod == codec.Store {
		ew.codecID = CodecStore
		if ew.targetSize == DefaultChunkTargetSize || ew.targetSize > 4*1024*1024 {
			ew.targetSize = 4 * 1024 * 1024
		}
	} else if cfg.DefaultMethod == codec.LZMA2 || cfg.DefaultMethod == codec.ZStandard {
		ew.codecID = CodecZSTD
		if cfg.DefaultLevel > 0 {
			ew.codecLevel = cfg.DefaultLevel
		}
		if ew.targetSize == DefaultChunkTargetSize || ew.targetSize > 4*1024*1024 {
			ew.targetSize = 4 * 1024 * 1024
		}
	}

	bw := bufio.NewWriterSize(ew.w, 256*1024)
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

	if _, err := bw.Write(header[:]); err != nil {
		return totalWritten, err
	}
	totalWritten += HeaderSize

	// Pad header to first block boundary so Chunk 0 is block-aligned
	padHeader := blockSize - (totalWritten % blockSize)
	if padHeader < blockSize {
		if _, err := bw.Write(zeroBuf[:padHeader]); err != nil {
			return totalWritten, err
		}
		totalWritten += padHeader
	}

	// 2. Prepare chunks and extents
	type fileChunkTask struct {
		file       *file.File
		cleanName  string
		isDir      bool
		chunkIdx   uint32
		offset     uint64
		size       uint64
		fileOffset int64
		totalSize  uint64
		isFirst    bool
		isLast     bool
	}

	type chunkJob struct {
		idx   uint32
		tasks []fileChunkTask
	}

	type compChunk struct {
		idx              uint32
		uncompressedSize uint64
		compressedSize   uint64
		itemsCount       int
		hash             [32]byte
		payload          []byte
		extents          []extentMeta
		err              error
	}

	targetChunkSize := int64(ew.targetSize)
	if targetChunkSize <= 0 {
		targetChunkSize = 4 * 1024 * 1024
	}

	var jobs []*chunkJob
	curJob := &chunkJob{idx: 0}
	var curJobSize int64
	var currentChunkIdx uint32

	for _, f := range files {
		cleanName := strings.TrimPrefix(f.Name(), "/")
		if cleanName == "" {
			continue
		}

		isDir := f.IsDir()
		if isDir {
			if !strings.HasSuffix(cleanName, "/") {
				cleanName += "/"
			}
			curJob.tasks = append(curJob.tasks, fileChunkTask{
				file:      f,
				cleanName: cleanName,
				isDir:     true,
				chunkIdx:  curJob.idx,
				isFirst:   true,
				isLast:    true,
			})
			continue
		}

		fSize := f.UncompressedSize()
		if fSize == 0 {
			curJob.tasks = append(curJob.tasks, fileChunkTask{
				file:      f,
				cleanName: cleanName,
				isDir:     false,
				chunkIdx:  curJob.idx,
				totalSize: 0,
				isFirst:   true,
				isLast:    true,
			})
			continue
		}

		if curJobSize+fSize <= targetChunkSize {
			curJob.tasks = append(curJob.tasks, fileChunkTask{
				file:       f,
				cleanName:  cleanName,
				chunkIdx:   curJob.idx,
				offset:     uint64(curJobSize),
				size:       uint64(fSize),
				fileOffset: 0,
				totalSize:  uint64(fSize),
				isFirst:    true,
				isLast:     true,
			})
			curJobSize += fSize
			if curJobSize >= targetChunkSize {
				jobs = append(jobs, curJob)
				currentChunkIdx++
				curJob = &chunkJob{idx: currentChunkIdx}
				curJobSize = 0
			}
		} else {
			if curJobSize > 0 {
				jobs = append(jobs, curJob)
				currentChunkIdx++
				curJob = &chunkJob{idx: currentChunkIdx}
				curJobSize = 0
			}

			if fSize <= targetChunkSize {
				curJob.tasks = append(curJob.tasks, fileChunkTask{
					file:       f,
					cleanName:  cleanName,
					chunkIdx:   curJob.idx,
					offset:     0,
					size:       uint64(fSize),
					fileOffset: 0,
					totalSize:  uint64(fSize),
					isFirst:    true,
					isLast:     true,
				})
				curJobSize += fSize
			} else {
				remaining := fSize
				fileOff := int64(0)
				isFirst := true
				for remaining > 0 {
					part := min(remaining, targetChunkSize)
					isLast := (remaining - part) == 0
					curJob.tasks = append(curJob.tasks, fileChunkTask{
						file:       f,
						cleanName:  cleanName,
						chunkIdx:   curJob.idx,
						offset:     0,
						size:       uint64(part),
						fileOffset: fileOff,
						totalSize:  uint64(fSize),
						isFirst:    isFirst,
						isLast:     isLast,
					})
					isFirst = false
					remaining -= part
					fileOff += part

					jobs = append(jobs, curJob)
					currentChunkIdx++
					curJob = &chunkJob{idx: currentChunkIdx}
					curJobSize = 0
				}
			}
		}
	}
	if len(curJob.tasks) > 0 {
		jobs = append(jobs, curJob)
	}
	if len(jobs) == 0 {
		jobs = append(jobs, &chunkJob{idx: 0})
	}

	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}
	if numWorkers > 32 {
		numWorkers = 32
	}

	parentCtx := cfg.Context
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	jobChan := make(chan *chunkJob, numWorkers*2)
	compQueue := make(chan *compChunk, numWorkers*2)

	var workerWg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			buf := make([]byte, 0, targetChunkSize+64*1024)

			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobChan:
					if !ok {
						return
					}
					buf = buf[:0]
					var jobExtents []extentMeta
					itemsCount := 0

					for _, task := range job.tasks {
						var modUnixNano int64
						if !task.file.ModTime().IsZero() {
							modUnixNano = task.file.ModTime().UnixNano()
							if modUnixNano < 0 {
								modUnixNano = 0
							}
						}

						mode := uint32(task.file.Mode().Perm())
						if task.isDir {
							mode |= 0x4000
						} else {
							mode |= 0x8000
						}
						var pxTLV [16]byte
						binary.LittleEndian.PutUint16(pxTLV[0:2], TagPOSIXPermissions)
						binary.LittleEndian.PutUint16(pxTLV[2:4], 8)
						binary.LittleEndian.PutUint32(pxTLV[4:8], 0)
						binary.LittleEndian.PutUint32(pxTLV[8:12], mode)
						binary.LittleEndian.PutUint32(pxTLV[12:16], 0)

						if task.isDir || task.totalSize == 0 {
							jobExtents = append(jobExtents, extentMeta{
								name:        task.cleanName,
								target:      "",
								timestamp:   modUnixNano,
								flags:       0,
								namespace:   NamespaceVFS,
								hash:        [32]byte{},
								extraTLVs:   pxTLV,
								extraLen:    16,
								chunkIndex:  task.chunkIdx,
								chunkOffset: uint64(len(buf)),
								chunkCount:  1,
								payloadSize: 0,
							})
							itemsCount++
							if cfg.OnFileDone != nil {
								cfg.OnFileDone(task.file, nil)
							}
							continue
						}

						rc, err := task.file.Open()
						if err != nil {
							// Gracefully skip locked or inaccessible OS files (e.g. active cache, antivirus, locked handles)
							if cfg.OnFileDone != nil {
								cfg.OnFileDone(task.file, err)
							}
							continue
						}

						if task.fileOffset > 0 {
							if rs, ok := rc.(io.Seeker); ok {
								_, _ = rs.Seek(task.fileOffset, io.SeekStart)
							} else {
								_, _ = io.CopyN(io.Discard, rc, task.fileOffset)
							}
						}

						chunkStartOffset := uint64(len(buf))
						startBufLen := len(buf)
						needed := startBufLen + int(task.size)
						if needed > cap(buf) {
							newCap := (needed + 64*1024) * 3 / 2
							newBuf := make([]byte, startBufLen, newCap)
							copy(newBuf, buf)
							buf = newBuf
						}
						buf = buf[:needed]
						targetSlice := buf[startBufLen:needed]
						_, _ = io.ReadFull(rc, targetSlice)
						_ = rc.Close()

						if task.isFirst {
							var hash [32]byte
							if task.isLast {
								hash = sha256.Sum256(targetSlice)
							} else {
								fileHash := sha256.New()
								fileHash.Write(targetSlice)
								copy(hash[:], fileHash.Sum(nil))
							}

							chunkCount := uint16(1)
							if task.totalSize > uint64(targetChunkSize) {
								chunkCount = uint16((task.totalSize + uint64(targetChunkSize) - 1) / uint64(targetChunkSize))
							}

							jobExtents = append(jobExtents, extentMeta{
								name:        task.cleanName,
								target:      "",
								timestamp:   modUnixNano,
								flags:       0,
								namespace:   NamespaceVFS,
								hash:        hash,
								extraTLVs:   pxTLV,
								extraLen:    16,
								chunkIndex:  task.chunkIdx,
								chunkOffset: chunkStartOffset,
								chunkCount:  chunkCount,
								payloadSize: task.totalSize,
							})
							itemsCount++
						}

						if task.isLast && cfg.OnFileDone != nil {
							cfg.OnFileDone(task.file, nil)
						}
					}

					var compPayload []byte
					var compErr error
					switch ew.codecID {
					case CodecStore:
						compPayload = buf
					case CodecPRZM:
						compPayload, compErr = przm.Compress(buf, ew.codecLevel)
					default:
						compPayload, compErr = codec.CompressZstd(buf, nil)
					}

					c := &compChunk{
						idx:              job.idx,
						uncompressedSize: uint64(len(buf)),
						compressedSize:   uint64(len(compPayload)),
						itemsCount:       itemsCount,
						hash:             sha256.Sum256(buf),
						payload:          compPayload,
						extents:          jobExtents,
						err:              compErr,
					}
					select {
					case <-ctx.Done():
						return
					case compQueue <- c:
					}
				}
			}
		}()
	}

	go func() {
		defer close(jobChan)
		for _, job := range jobs {
			select {
			case <-ctx.Done():
				return
			case jobChan <- job:
			}
		}
	}()

	var ctocEntries [][CTOCEntrySize]byte
	var extents []extentMeta
	var writerErr error
	writerDone := make(chan struct{})

	go func() {
		defer close(writerDone)
		pending := make(map[uint32]*compChunk)
		expectedIdx := uint32(0)

		writeOne := func(c *compChunk) error {
			if c.err != nil {
				return c.err
			}
			blockIdx := uint64(totalWritten >> ew.blockShift)

			// 64-byte Chunk Header
			var chnkHdr [ChunkHeaderSize]byte
			copy(chnkHdr[0:4], MagicChunk[:])
			binary.LittleEndian.PutUint32(chnkHdr[4:8], c.idx)
			chnkHdr[8] = ew.codecID
			chnkHdr[9] = byte(ew.codecLevel)
			chunkFlags := ChunkFlagSolid
			binary.LittleEndian.PutUint16(chnkHdr[10:12], chunkFlags)
			binary.LittleEndian.PutUint64(chnkHdr[12:20], c.uncompressedSize)
			binary.LittleEndian.PutUint64(chnkHdr[20:28], c.compressedSize)
			binary.LittleEndian.PutUint32(chnkHdr[28:32], uint32(c.itemsCount))
			copy(chnkHdr[32:64], c.hash[:])

			if _, err := bw.Write(chnkHdr[:]); err != nil {
				return err
			}
			totalWritten += ChunkHeaderSize

			if _, err := bw.Write(c.payload); err != nil {
				return err
			}
			totalWritten += int64(c.compressedSize)

			// Pad chunk to logical block boundary
			rem := totalWritten % blockSize
			if rem != 0 {
				pad := blockSize - rem
				if _, err := bw.Write(zeroBuf[:pad]); err != nil {
					return err
				}
				totalWritten += pad
			}

			// Record CTOC Entry (16 bytes)
			var ctocEntry [CTOCEntrySize]byte
			binary.LittleEndian.PutUint64(ctocEntry[0:8], blockIdx)
			binary.LittleEndian.PutUint32(ctocEntry[8:12], uint32(c.compressedSize))
			binary.LittleEndian.PutUint32(ctocEntry[12:16], uint32(c.uncompressedSize))
			ctocEntries = append(ctocEntries, ctocEntry)
			return nil
		}

		for c := range compQueue {
			if writerErr != nil {
				continue
			}
			pending[c.idx] = c
			for {
				ready, ok := pending[expectedIdx]
				if !ok {
					break
				}
				delete(pending, expectedIdx)
				if err := writeOne(ready); err != nil {
					writerErr = err
					cancel()
					break
				}
				extents = append(extents, ready.extents...)
				if cfg.OnProgress != nil {
					cfg.OnProgress(int64(ready.uncompressedSize), "")
				}
				expectedIdx++
			}
		}
	}()

	workerWg.Wait()
	close(compQueue)
	<-writerDone

	if writerErr != nil {
		return totalWritten, writerErr
	}
	if err := ctx.Err(); err != nil {
		return totalWritten, err
	}

	// 3. Central Directory Section
	centralDirOffset := uint64(totalWritten)

	// Write CTOC Header (16 bytes)
	var ctocHdr [CTOCHeaderSize]byte
	copy(ctocHdr[0:4], MagicCTOC[:])
	binary.LittleEndian.PutUint32(ctocHdr[4:8], uint32(len(ctocEntries)))
	binary.LittleEndian.PutUint32(ctocHdr[8:12], 0)  // HotChunksCount
	binary.LittleEndian.PutUint32(ctocHdr[12:16], 0) // WarmChunksCount
	if _, err := bw.Write(ctocHdr[:]); err != nil {
		return totalWritten, err
	}
	totalWritten += CTOCHeaderSize

	// Write CTOC Entries
	for _, entry := range ctocEntries {
		if _, err := bw.Write(entry[:]); err != nil {
			return totalWritten, err
		}
		totalWritten += CTOCEntrySize
	}

	// Write INDX Header (8 bytes)
	var indxHdr [INDXHeaderSize]byte
	copy(indxHdr[0:4], MagicINDX[:])
	binary.LittleEndian.PutUint32(indxHdr[4:8], uint32(len(extents)))
	if _, err := bw.Write(indxHdr[:]); err != nil {
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

		recLen := FixedExtentPrefixSize + len(varints) + len(targetBytes) + len(nameBytes) + int(ext.extraLen)

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
		binary.LittleEndian.PutUint16(prefix[56:58], ext.extraLen)

		if _, err := bw.Write(prefix[:]); err != nil {
			return totalWritten, err
		}
		if _, err := bw.Write(varints); err != nil {
			return totalWritten, err
		}
		if len(targetBytes) > 0 {
			if _, err := bw.Write(targetBytes); err != nil {
				return totalWritten, err
			}
		}
		if len(nameBytes) > 0 {
			if _, err := bw.Write(nameBytes); err != nil {
				return totalWritten, err
			}
		}
		if ext.extraLen > 0 {
			if _, err := bw.Write(ext.extraTLVs[:ext.extraLen]); err != nil {
				return totalWritten, err
			}
		}

		totalWritten += int64(recLen)
	}

	// 4. Archive Trailer (32 bytes at EOF)
	var trailer [TrailerSize]byte
	binary.LittleEndian.PutUint64(trailer[0:8], centralDirOffset)
	binary.LittleEndian.PutUint64(trailer[8:16], 0)  // RecoveryOffset (0 = absent)
	binary.LittleEndian.PutUint64(trailer[16:24], 0) // ArchiveStartOffset (0 = no stub)
	flagsCopy := uint32(FlagCentralDir | FlagAlignedBlocks)
	binary.LittleEndian.PutUint32(trailer[24:28], flagsCopy)
	copy(trailer[28:32], MagicTrailer[:])

	if _, err := bw.Write(trailer[:]); err != nil {
		return totalWritten, err
	}
	totalWritten += TrailerSize

	if err := bw.Flush(); err != nil {
		return totalWritten, err
	}

	return totalWritten, nil
}
