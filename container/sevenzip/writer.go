// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash"
	"hash/crc32"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/lemon4ksan/foundation/codec/compress/lzma"
	"github.com/lemon4ksan/foundation/codec/filter/bcj"
	"github.com/lemon4ksan/seal/checkpoint"
	"github.com/lemon4ksan/seal/codec/bcj2"
	"github.com/lemon4ksan/seal/codec/ppmd"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/internal/pool"
)

// FilterType defines bytecode preprocessing filters.
type FilterType int

const (
	FilterNone FilterType = iota
	FilterBCJ
	FilterARM
	FilterBCJ2
)

// MethodType defines primary compression codecs for 7z containers.
type MethodType uint64

const (
	MethodLZMA2Default MethodType = MethodLZMA2
	MethodPPMDDefault  MethodType = MethodPPMD
)

// WriterOptions specifies configuration options for the 7z Writer.
type WriterOptions struct {
	LZMA           lzma.Options
	Filter         FilterType
	Method         MethodType
	PPMdOrder      int
	PPMdMem        uint32
	Password       string
	EncryptHeaders bool
	CheckpointData []byte
	Context        context.Context
	Resumable      bool
	SessionID      string
	SourceDir      string
	OnProgress     func(phase string, current, total int64)
	OnFileDone     func(*file.File, error)
}

// ErrArchiveFrozen indicates that archive creation was gracefully frozen on context cancellation.
var ErrArchiveFrozen = container.ErrArchiveFrozen

// Writer implements 7-Zip archive serialization.
type Writer struct {
	w          io.Writer
	options    WriterOptions
	OnProgress func(phase string, current, total int64)
}

// NewWriter constructs a new 7z Writer with optional LZMA options.
func NewWriter(w io.Writer, opts ...lzma.Options) *Writer {
	var lzmaOpts lzma.Options
	if len(opts) > 0 {
		lzmaOpts = opts[0]
	}
	return &Writer{
		w: w,
		options: WriterOptions{
			LZMA:   lzmaOpts,
			Method: MethodLZMA2Default,
		},
	}
}

// NewWriterWithOptions constructs a new 7z Writer with detailed WriterOptions.
func NewWriterWithOptions(w io.Writer, opts WriterOptions) *Writer {
	if opts.Method == 0 {
		opts.Method = MethodLZMA2Default
	}
	return &Writer{
		w:          w,
		options:    opts,
		OnProgress: opts.OnProgress,
	}
}

type prefetchBlock struct {
	buf    *[]byte
	is256K bool
	data   []byte
	err    error
}

type asyncPrefetchPipe struct {
	files           []*file.File
	fileEntries     []FileEntry
	unpackSizes     []uint64
	totalCRC        hash.Hash32
	totalUnpackSize uint64
	totalFilesCount int64
	filter          FilterType
	bcjIP           uint32
	onProgress      func(phase string, current, total int64)
	onFileDone      func(*file.File, error)
	ctx             context.Context
	stoppedAtIndex  int

	blockChan chan prefetchBlock
	currBlock prefetchBlock
	currOff   int
	doneChan  chan struct{}
}

func newAsyncPrefetchPipe(
	files []*file.File,
	filter FilterType,
	onProgress func(phase string, current, total int64),
	onFileDone func(*file.File, error),
	ctx context.Context,
) *asyncPrefetchPipe {
	pipe := &asyncPrefetchPipe{
		files:           files,
		fileEntries:     make([]FileEntry, len(files)),
		totalCRC:        crc32.NewIEEE(),
		totalFilesCount: int64(len(files)),
		filter:          filter,
		onProgress:      onProgress,
		onFileDone:      onFileDone,
		ctx:             ctx,
		blockChan:       make(chan prefetchBlock, 256), // 16MB async prefetch buffer queue
		doneChan:        make(chan struct{}),
	}

	for i, f := range files {
		var winAttr uint32 = 0x20
		posixMode := uint32(f.Mode().Perm())
		if f.IsDir() {
			winAttr = 0x10
			if posixMode == 0 {
				posixMode = 0o755
			}
			posixMode |= 0o040000
		} else {
			if posixMode == 0 {
				posixMode = 0o644
			}
			posixMode |= 0o100000
		}
		attrib := (posixMode << 16) | winAttr

		pipe.fileEntries[i] = FileEntry{
			Name:          f.Name(),
			IsDir:         f.IsDir(),
			IsEmptyStream: f.IsDir(),
			IsEmptyFile:   f.IsDir(),
			ModTime:       f.ModTime(),
			HasModTime:    true,
			Attrib:        attrib,
			HasAttrib:     true,
			Size:          0,
		}
	}

	// Start background async disk prefetch worker
	go pipe.prefetchLoop()

	return pipe
}

func (p *asyncPrefetchPipe) prefetchLoop() {
	defer close(p.blockChan)
	defer close(p.doneChan)

	var readDone int64

	for i, f := range p.files {
		if p.ctx != nil && p.ctx.Err() != nil && i > 0 {
			p.stoppedAtIndex = i
			break
		}

		if f.IsDir() {
			p.fileEntries[i].IsEmptyStream = true
			p.fileEntries[i].IsEmptyFile = true
			p.fileEntries[i].HasCRC = false
			readDone++
			if p.onProgress != nil && (readDone%100 == 0 || readDone == p.totalFilesCount) {
				p.onProgress("Reading", readDone, p.totalFilesCount)
			}
			continue
		}

		rc, err := f.Open()
		if err != nil {
			// Gracefully skip locked or invalid OS handles
			p.fileEntries[i].IsEmptyStream = true
			p.fileEntries[i].IsEmptyFile = true
			p.fileEntries[i].HasCRC = false
			readDone++
			if p.onProgress != nil && (readDone%100 == 0 || readDone == p.totalFilesCount) {
				p.onProgress("Reading", readDone, p.totalFilesCount)
			}
			continue
		}

		curCRC := crc32.NewIEEE()
		var fileBytesRead uint64

		// Small file fast-path: single syscall read
		sz := f.UncompressedSize()
		if sz > 0 && sz <= 64*1024 {
			bufPtr := pool.Get64K()
			bufSlice := (*bufPtr)[:sz]

			nr, rErr := io.ReadFull(rc, bufSlice)
			_ = rc.Close()
			if nr > 0 {
				_, _ = curCRC.Write(bufSlice[:nr])
				_, _ = p.totalCRC.Write(bufSlice[:nr])
				fileBytesRead = uint64(nr)

				// Apply BCJ branch conversion if enabled
				switch p.filter {
				case FilterBCJ:
					bcj.Filter(bcj.X86, bufSlice[:nr], p.bcjIP, true)
					p.bcjIP += uint32(nr)
				case FilterARM:
					bcj.Filter(bcj.ARM, bufSlice[:nr], p.bcjIP, true)
					p.bcjIP += uint32(nr)
				}

				p.blockChan <- prefetchBlock{
					buf:  bufPtr,
					data: bufSlice[:nr],
				}
			} else {
				pool.Put64K(bufPtr)
			}
			_ = rErr
		} else {
			for {
				bufPtr := pool.Get256K()
				bufSlice := (*bufPtr)

				nr, rErr := rc.Read(bufSlice)
				if nr > 0 {
					_, _ = curCRC.Write(bufSlice[:nr])
					_, _ = p.totalCRC.Write(bufSlice[:nr])
					fileBytesRead += uint64(nr)

					// Apply BCJ branch conversion if enabled
					switch p.filter {
					case FilterBCJ:
						bcj.Filter(bcj.X86, bufSlice[:nr], p.bcjIP, true)
						p.bcjIP += uint32(nr)
					case FilterARM:
						bcj.Filter(bcj.ARM, bufSlice[:nr], p.bcjIP, true)
						p.bcjIP += uint32(nr)
					}

					p.blockChan <- prefetchBlock{
						buf:    bufPtr,
						is256K: true,
						data:   bufSlice[:nr],
					}
				} else {
					pool.Put256K(bufPtr)
				}

				if rErr != nil {
					_ = rc.Close()
					break
				}
			}
		}

		p.fileEntries[i].Size = fileBytesRead
		if fileBytesRead > 0 {
			p.fileEntries[i].CRC = curCRC.Sum32()
			p.fileEntries[i].HasCRC = true
			p.fileEntries[i].IsEmptyStream = false
			p.fileEntries[i].IsEmptyFile = false
			p.unpackSizes = append(p.unpackSizes, fileBytesRead)
			p.totalUnpackSize += fileBytesRead
		} else {
			p.fileEntries[i].IsEmptyStream = true
			p.fileEntries[i].IsEmptyFile = true
			p.fileEntries[i].HasCRC = false
		}

		readDone++
		if p.onProgress != nil {
			p.onProgress("Reading", readDone, p.totalFilesCount)
		}
		if p.onFileDone != nil {
			p.onFileDone(f, nil)
		}
	}
}

func (p *asyncPrefetchPipe) Read(b []byte) (int, error) {
	for {
		if p.currOff < len(p.currBlock.data) {
			n := copy(b, p.currBlock.data[p.currOff:])
			p.currOff += n
			if p.currOff >= len(p.currBlock.data) {
				if p.currBlock.buf != nil {
					if p.currBlock.is256K {
						pool.Put256K(p.currBlock.buf)
					} else {
						pool.Put64K(p.currBlock.buf)
					}
					p.currBlock.buf = nil
				}
				p.currBlock.data = nil
			}
			return n, nil
		}

		blk, ok := <-p.blockChan
		if !ok {
			<-p.doneChan
			return 0, io.EOF
		}
		if blk.err != nil {
			return 0, blk.err
		}

		p.currBlock = blk
		p.currOff = 0
	}
}

func extensionGroup(ext string) int {
	switch ext {
	case ".txt", ".json", ".xml", ".html", ".htm", ".css", ".js", ".ts", ".jsx", ".tsx",
		".go", ".c", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".cs", ".py", ".rs", ".java",
		".md", ".log", ".yaml", ".yml", ".ini", ".cfg", ".conf", ".toml", ".sql", ".bat", ".cmd", ".ps1", ".sh":
		return 0 // Code & Text

	case ".exe",
		".dll",
		".sys",
		".node",
		".so",
		".dylib",
		".pdb",
		".lib",
		".obj",
		".o",
		".a",
		".ocx",
		".bin",
		".dat",
		".rsrc":
		return 1 // Executables & Binaries

	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".rtf", ".csv", ".tsv":
		return 2 // Documents

	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".ico", ".svg", ".bmp", ".mp3", ".wav", ".ogg", ".flac",
		".mp4", ".mkv", ".avi", ".mov", ".zip", ".gz", ".xz", ".7z", ".tar", ".bz2", ".zst", ".rar", ".iso":
		return 3 // Media & Compressed

	default:
		return 4 // Other
	}
}

// WriteArchive serializes files into a standard 7-Zip archive with streaming constant memory.
func (zw *Writer) WriteArchive(files []*file.File) (int64, error) {
	initWritten, err := writeSignatureSlot(zw.w)
	if err != nil {
		return 0, err
	}

	sortedFiles := sortFilesForSolid(files)

	payload, err := zw.compressSolidPayload(sortedFiles)
	if err != nil {
		return initWritten, err
	}

	headerBytes := buildArchiveHeader(payload, zw.options)

	hdrWritten, nextOffset, nextSize, nextCRC, err := writeHeaderData(
		zw.w, headerBytes, uint64(payload.packedBytes), zw.options,
	)
	if err != nil {
		return initWritten + payload.packedBytes + hdrWritten, err
	}

	totalWritten := initWritten + payload.packedBytes + hdrWritten

	if err := patchStartHeader(zw.w, nextOffset, nextSize, nextCRC); err != nil {
		return totalWritten, err
	}

	if payload.wasFrozen {
		return totalWritten, ErrArchiveFrozen
	}

	return totalWritten, nil
}

func writeSignatureSlot(w io.Writer) (int64, error) {
	var sigAndStart [StartHeaderSize]byte
	copy(sigAndStart[:SignatureSize], Signature[:])
	sigAndStart[6] = 0 // Major 0
	sigAndStart[7] = 4 // Minor 4

	n, err := w.Write(sigAndStart[:])
	return int64(n), err
}

func sortFilesForSolid(files []*file.File) []*file.File {
	sortedFiles := make([]*file.File, len(files))
	copy(sortedFiles, files)
	slices.SortStableFunc(sortedFiles, func(a, b *file.File) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		extA := strings.ToLower(filepath.Ext(a.Name()))
		extB := strings.ToLower(filepath.Ext(b.Name()))

		grpA := extensionGroup(extA)
		grpB := extensionGroup(extB)
		if grpA != grpB {
			return grpA - grpB
		}

		if extA != extB {
			return strings.Compare(extA, extB)
		}
		return strings.Compare(a.Name(), b.Name())
	})
	return sortedFiles
}

type solidPayload struct {
	packSizes         []uint64
	totalUnpackSize   uint64
	unpackCRC         uint32
	fileEntries       []FileEntry
	unpackSizes       []uint64
	compSizeBeforeAES uint64
	bcj2UnpackSizes   [3]uint64
	customCoder       CustomCoder
	customProps       []byte
	hasEncryption     bool
	aesProps          []byte
	packedBytes       int64
	wasFrozen         bool
}

func (zw *Writer) compressSolidPayload(sortedFiles []*file.File) (*solidPayload, error) {
	filter := zw.options.Filter
	method := zw.options.Method
	if method == 0 {
		method = MethodLZMA2Default
	}

	onProg := zw.OnProgress
	if onProg == nil {
		onProg = zw.options.OnProgress
	}

	solidReader := newAsyncPrefetchPipe(sortedFiles, filter, onProg, zw.options.OnFileDone, zw.options.Context)

	var totalUnpackEstimate int64
	for _, f := range sortedFiles {
		if !f.IsDir() && f.UncompressedSize() > 0 {
			totalUnpackEstimate += f.UncompressedSize()
		}
	}
	totalEstimatedChunks := (totalUnpackEstimate + 65535) / 65536
	if totalEstimatedChunks <= 0 {
		totalEstimatedChunks = 1
	}

	cw := &countingWriter{w: zw.w}
	var encW io.WriteCloser
	var aesProps []byte
	targetW := io.Writer(cw)

	var compCounter countingWriter
	if zw.options.Password != "" {
		var err error
		encW, aesProps, err = EncryptStream(cw, zw.options.Password, 19)
		if err != nil {
			return nil, err
		}
		compCounter = countingWriter{w: encW}
		targetW = &compCounter
	}

	payload := &solidPayload{
		hasEncryption: len(aesProps) > 0,
		aesProps:      aesProps,
	}

	if c, ok := LookupCoder(uint64(method)); ok {
		payload.customCoder = c
	}

	switch {
	case filter == FilterBCJ2:
		packSizes, unpackSizes, err := compressBCJ2Parallel(solidReader, targetW, zw.options.LZMA)
		if err != nil {
			return nil, err
		}
		payload.packSizes = packSizes
		payload.bcj2UnpackSizes = unpackSizes

	case payload.customCoder != nil:
		var err error
		_, payload.customProps, err = payload.customCoder.Encode(solidReader, targetW, zw.options)
		if err != nil {
			return nil, err
		}
		payload.packSizes = []uint64{uint64(cw.n)}

	case method == MethodPPMDDefault:
		if err := compressPPMd(solidReader, targetW, zw.options); err != nil {
			return nil, err
		}
		payload.packSizes = []uint64{uint64(cw.n)}

	default:
		if err := compressLZMA2(solidReader, targetW, zw.options, onProg, totalEstimatedChunks); err != nil {
			return nil, err
		}
		payload.packSizes = []uint64{uint64(cw.n)}
	}

	if encW != nil {
		if err := encW.Close(); err != nil {
			return nil, err
		}
	}

	payload.packedBytes = cw.n
	payload.totalUnpackSize = solidReader.totalUnpackSize
	payload.unpackCRC = solidReader.totalCRC.Sum32()
	payload.fileEntries = solidReader.fileEntries
	payload.unpackSizes = solidReader.unpackSizes

	if solidReader.stoppedAtIndex > 0 {
		payload.wasFrozen = true
		payload.fileEntries = solidReader.fileEntries[:solidReader.stoppedAtIndex]
		payload.unpackSizes = solidReader.unpackSizes[:solidReader.stoppedAtIndex]

		if zw.options.Resumable {
			remFiles := sortedFiles[solidReader.stoppedAtIndex:]
			remNames := make([]string, len(remFiles))
			for j, rem := range remFiles {
				remNames[j] = rem.Name()
			}
			st := checkpoint.NewState(
				zw.options.SessionID,
				zw.options.SourceDir,
				"sevenzip",
				len(sortedFiles),
				solidReader.stoppedAtIndex,
				remNames,
			)
			if cpBytes, err := st.Encode(); err == nil {
				zw.options.CheckpointData = cpBytes
			}
		}
	}

	payload.compSizeBeforeAES = uint64(cw.n)
	if encW != nil {
		payload.compSizeBeforeAES = uint64(compCounter.n)
	}

	if onProg != nil {
		onProg("Finalizing", 1, 1)
	}

	return payload, nil
}

func compressBCJ2Parallel(
	solidReader io.Reader,
	targetW io.Writer,
	lzmaOpts lzma.Options,
) ([]uint64, [3]uint64, error) {
	var allUncomp bytes.Buffer
	if _, err := io.Copy(&allUncomp, solidReader); err != nil {
		return nil, [3]uint64{}, err
	}

	var mainBuf, callBuf, jumpBuf, rcBuf bytes.Buffer
	if err := bcj2.EncodeStreams(&allUncomp, &mainBuf, &callBuf, &jumpBuf, &rcBuf); err != nil {
		return nil, [3]uint64{}, err
	}

	var bcj2UnpackSizes [3]uint64
	bcj2UnpackSizes[0] = uint64(mainBuf.Len())
	bcj2UnpackSizes[1] = uint64(callBuf.Len())
	bcj2UnpackSizes[2] = uint64(jumpBuf.Len())

	opts := lzmaOpts
	if opts.Level == 0 && opts.DictSize == 0 && opts.ChainLength == 0 {
		opts = lzma.DefaultOptions()
	}

	var packBufs [3]bytes.Buffer
	var wg sync.WaitGroup
	var errOnce sync.Once
	var compErr error

	compressBranch := func(idx int, in *bytes.Buffer) {
		defer wg.Done()
		comp := lzma.NewCompressor2WithOptions(opts)
		if _, err := comp.Compress(in, &packBufs[idx]); err != nil {
			errOnce.Do(func() { compErr = err })
		}
	}

	wg.Add(3)
	go compressBranch(0, &mainBuf)
	go compressBranch(1, &callBuf)
	go compressBranch(2, &jumpBuf)
	wg.Wait()

	if compErr != nil {
		return nil, bcj2UnpackSizes, compErr
	}

	var packSizes []uint64
	for i := 0; i < 3; i++ {
		b := packBufs[i].Bytes()
		if _, err := targetW.Write(b); err != nil {
			return nil, bcj2UnpackSizes, err
		}
		packSizes = append(packSizes, uint64(len(b)))
	}

	rcBytes := rcBuf.Bytes()
	if _, err := targetW.Write(rcBytes); err != nil {
		return nil, bcj2UnpackSizes, err
	}
	packSizes = append(packSizes, uint64(len(rcBytes)))

	return packSizes, bcj2UnpackSizes, nil
}

func compressPPMd(src io.Reader, dst io.Writer, opts WriterOptions) error {
	order := opts.PPMdOrder
	if order < 2 {
		order = ppmd.DefaultOrder
	}
	mem := opts.PPMdMem
	if mem < 1024*1024 {
		mem = ppmd.DefaultMemSize
	}
	comp := ppmd.NewCompressor(order, mem)
	_, err := comp.Compress(src, dst)
	return err
}

func compressLZMA2(
	src io.Reader,
	dst io.Writer,
	opts WriterOptions,
	onProg func(string, int64, int64),
	estChunks int64,
) error {
	lzOpts := opts.LZMA
	if lzOpts.Level == 0 && lzOpts.DictSize == 0 && lzOpts.ChainLength == 0 {
		lzOpts = lzma.DefaultOptions()
	}
	if onProg != nil {
		lzOpts.OnProgress = func(processed, _ int64) {
			onProg("Compressing", processed, estChunks)
		}
	}
	comp2 := lzma.NewCompressor2WithOptions(lzOpts)
	_, err := comp2.Compress(src, dst)
	return err
}

func buildArchiveHeader(payload *solidPayload, opts WriterOptions) []byte {
	var headerBuf bytes.Buffer
	headerBuf.WriteByte(NIDHeader)

	if len(opts.CheckpointData) > 0 {
		writeArchiveProperties(&headerBuf, opts.CheckpointData)
	}

	headerBuf.WriteByte(NIDMainStreamsInfo)

	writePackInfo(&headerBuf, payload)
	writeUnpackInfo(&headerBuf, payload, opts)
	writeSubStreamsInfo(&headerBuf, payload)

	headerBuf.WriteByte(NIDEnd) // End MainStreamsInfo

	writeFilesInfo(&headerBuf, payload.fileEntries)

	headerBuf.WriteByte(NIDEnd) // End Header
	return headerBuf.Bytes()
}

func writeArchiveProperties(buf *bytes.Buffer, data []byte) {
	buf.WriteByte(NIDArchiveProperties) // 0x02
	buf.WriteByte(0x53)                 // PropertyType: 0x53 ('S' for Seal)
	_ = WriteNumber(buf, uint64(len(data)))
	buf.Write(data)
	buf.WriteByte(NIDEnd) // 0x00 end of properties
}

func writePackInfo(buf *bytes.Buffer, p *solidPayload) {
	buf.WriteByte(NIDPackInfo)
	_ = WriteNumber(buf, 0) // PackPos = 0
	_ = WriteNumber(buf, uint64(len(p.packSizes)))
	buf.WriteByte(NIDSize)
	for _, sz := range p.packSizes {
		_ = WriteNumber(buf, sz)
	}
	buf.WriteByte(NIDEnd)
}

func writeUnpackInfo(buf *bytes.Buffer, p *solidPayload, opts WriterOptions) {
	buf.WriteByte(NIDUnpackInfo)
	buf.WriteByte(NIDFolder)
	_ = WriteNumber(buf, 1) // 1 Folder
	buf.WriteByte(0)        // External = 0

	lzmaDictProp := lzma.PropFromDictSize(opts.LZMA.DictSize)
	if lzmaDictProp == 0 {
		lzmaDictProp = 0x18
	}
	lzmaPropBytes := []byte{lzmaDictProp}

	switch {
	case opts.Filter == FilterBCJ2:
		_ = WriteNumber(buf, 4) // NumCoders = 4
		writeCoder(buf, MethodLZMA2, lzmaPropBytes)
		writeCoder(buf, MethodLZMA2, lzmaPropBytes)
		writeCoder(buf, MethodLZMA2, lzmaPropBytes)
		writeComplexCoder(buf, MethodBCJ2, 4, 1, nil)

		// 3 BindPairs
		_ = WriteNumber(buf, 3)
		_ = WriteNumber(buf, 0)
		_ = WriteNumber(buf, 4)
		_ = WriteNumber(buf, 1)
		_ = WriteNumber(buf, 5)
		_ = WriteNumber(buf, 2)

		// 4 PackedIndices
		_ = WriteNumber(buf, 0)
		_ = WriteNumber(buf, 1)
		_ = WriteNumber(buf, 2)
		_ = WriteNumber(buf, 6)

		buf.WriteByte(NIDCodersUnpackSize)
		_ = WriteNumber(buf, p.bcj2UnpackSizes[0])
		_ = WriteNumber(buf, p.bcj2UnpackSizes[1])
		_ = WriteNumber(buf, p.bcj2UnpackSizes[2])
		_ = WriteNumber(buf, p.totalUnpackSize)

	case opts.Filter == FilterBCJ || opts.Filter == FilterARM:
		filterMethod := uint64(MethodBCJ)
		if opts.Filter == FilterARM {
			filterMethod = uint64(MethodARM)
		}

		if p.hasEncryption {
			_ = WriteNumber(buf, 3)
			writeCoder(buf, filterMethod, nil)
			writeCoder(buf, MethodLZMA2, lzmaPropBytes)
			writeCoder(buf, MethodAES, p.aesProps)

			_ = WriteNumber(buf, 0)
			_ = WriteNumber(buf, 1)
			_ = WriteNumber(buf, 1)
			_ = WriteNumber(buf, 2)

			buf.WriteByte(NIDCodersUnpackSize)
			_ = WriteNumber(buf, p.totalUnpackSize)
			_ = WriteNumber(buf, p.totalUnpackSize)
			_ = WriteNumber(buf, p.compSizeBeforeAES)
		} else {
			_ = WriteNumber(buf, 2)
			writeCoder(buf, filterMethod, nil)
			writeCoder(buf, MethodLZMA2, lzmaPropBytes)

			_ = WriteNumber(buf, 0)
			_ = WriteNumber(buf, 1)

			buf.WriteByte(NIDCodersUnpackSize)
			_ = WriteNumber(buf, p.totalUnpackSize)
			_ = WriteNumber(buf, p.totalUnpackSize)
		}

	case opts.Method == MethodPPMDDefault:
		order := byte(opts.PPMdOrder)
		if order < 2 {
			order = byte(ppmd.DefaultOrder)
		}
		mem := opts.PPMdMem
		if mem < 1024*1024 {
			mem = uint32(ppmd.DefaultMemSize)
		}
		var propB [5]byte
		propB[0] = order
		binary.LittleEndian.PutUint32(propB[1:5], mem)

		if p.hasEncryption {
			_ = WriteNumber(buf, 2)
			writeCoder(buf, MethodPPMD, propB[:])
			writeCoder(buf, MethodAES, p.aesProps)

			_ = WriteNumber(buf, 0)
			_ = WriteNumber(buf, 1)

			buf.WriteByte(NIDCodersUnpackSize)
			_ = WriteNumber(buf, p.totalUnpackSize)
			_ = WriteNumber(buf, p.compSizeBeforeAES)
		} else {
			_ = WriteNumber(buf, 1)
			writeCoder(buf, MethodPPMD, propB[:])

			buf.WriteByte(NIDCodersUnpackSize)
			_ = WriteNumber(buf, p.totalUnpackSize)
		}

	default:
		actualMethod := uint64(MethodLZMA2)
		actualProps := lzmaPropBytes
		if p.customCoder != nil {
			actualMethod = p.customCoder.MethodID()
			actualProps = p.customProps
		}

		if p.hasEncryption {
			_ = WriteNumber(buf, 2)
			writeCoder(buf, actualMethod, actualProps)
			writeCoder(buf, MethodAES, p.aesProps)

			_ = WriteNumber(buf, 0)
			_ = WriteNumber(buf, 1)

			buf.WriteByte(NIDCodersUnpackSize)
			_ = WriteNumber(buf, p.totalUnpackSize)
			_ = WriteNumber(buf, p.compSizeBeforeAES)
		} else {
			_ = WriteNumber(buf, 1)
			writeCoder(buf, actualMethod, actualProps)

			buf.WriteByte(NIDCodersUnpackSize)
			_ = WriteNumber(buf, p.totalUnpackSize)
		}
	}

	// Folder CRC
	buf.WriteByte(NIDCRC)
	buf.WriteByte(1) // AllDefined = 1
	var crcB [4]byte
	binary.LittleEndian.PutUint32(crcB[:], p.unpackCRC)
	buf.Write(crcB[:])

	buf.WriteByte(NIDEnd) // End UnpackInfo
}

func writeSubStreamsInfo(buf *bytes.Buffer, p *solidPayload) {
	if len(p.unpackSizes) <= 1 {
		return
	}

	buf.WriteByte(NIDSubStreamsInfo)
	buf.WriteByte(NIDNumUnpackStream)
	_ = WriteNumber(buf, uint64(len(p.unpackSizes)))

	buf.WriteByte(NIDSize)
	for _, sz := range p.unpackSizes[:len(p.unpackSizes)-1] {
		_ = WriteNumber(buf, sz)
	}

	buf.WriteByte(NIDCRC)
	buf.WriteByte(1) // AllDefined
	var crcB [4]byte
	for _, fe := range p.fileEntries {
		if !fe.IsDir && fe.Size > 0 {
			binary.LittleEndian.PutUint32(crcB[:], fe.CRC)
			buf.Write(crcB[:])
		}
	}

	buf.WriteByte(NIDEnd)
}

func writeFilesInfo(buf *bytes.Buffer, fileEntries []FileEntry) {
	buf.WriteByte(NIDFilesInfo)
	_ = WriteNumber(buf, uint64(len(fileEntries)))

	// Check EmptyStreams
	var emptyCount int
	for _, fe := range fileEntries {
		if fe.IsEmptyStream {
			emptyCount++
		}
	}
	if emptyCount > 0 {
		buf.WriteByte(NIDEmptyStream)
		numBytes := (len(fileEntries) + 7) / 8
		_ = WriteNumber(buf, uint64(numBytes))
		bitmask := make([]byte, numBytes)
		for i, fe := range fileEntries {
			if fe.IsEmptyStream {
				byteIdx := i / 8
				bitIdx := 7 - (i % 8)
				bitmask[byteIdx] |= (1 << bitIdx)
			}
		}
		buf.Write(bitmask)
	}

	// Names (UTF-16LE + 2 zero bytes)
	buf.WriteByte(NIDName)
	var namesBuf bytes.Buffer
	namesBuf.WriteByte(0) // External = 0
	for _, fe := range fileEntries {
		u16 := utf16.Encode([]rune(fe.Name))
		for _, v := range u16 {
			var b [2]byte
			binary.LittleEndian.PutUint16(b[:], v)
			namesBuf.Write(b[:])
		}
		namesBuf.Write([]byte{0, 0}) // Zero terminator
	}
	_ = WriteNumber(buf, uint64(namesBuf.Len()))
	buf.Write(namesBuf.Bytes())

	// WinAttrib
	buf.WriteByte(NIDWinAttrib)
	var attrBuf bytes.Buffer
	attrBuf.WriteByte(1) // AllDefined = 1
	attrBuf.WriteByte(0) // External = 0
	for _, fe := range fileEntries {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], fe.Attrib)
		attrBuf.Write(b[:])
	}
	_ = WriteNumber(buf, uint64(attrBuf.Len()))
	buf.Write(attrBuf.Bytes())

	buf.WriteByte(NIDEnd) // End FilesInfo
}

func writeHeaderData(
	w io.Writer,
	headerBytes []byte,
	packSize uint64,
	opts WriterOptions,
) (int64, uint64, uint64, uint32, error) {
	if opts.EncryptHeaders && opts.Password != "" {
		return writeEncryptedHeader(w, headerBytes, packSize, opts.Password)
	}

	headerSize := uint64(len(headerBytes))
	headerCRC := crc32.ChecksumIEEE(headerBytes)

	if _, err := w.Write(headerBytes); err != nil {
		return 0, 0, 0, 0, err
	}

	return int64(headerSize), packSize, headerSize, headerCRC, nil
}

func writeEncryptedHeader(
	w io.Writer,
	headerBytes []byte,
	packSize uint64,
	password string,
) (int64, uint64, uint64, uint32, error) {
	var compHeaderBuf bytes.Buffer
	encHeaderW, headerAESProps, err := EncryptStream(&compHeaderBuf, password, 19)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	headerCompCounter := countingWriter{w: encHeaderW}
	compH := lzma.NewCompressor2WithOptions(lzma.DefaultOptions())
	if _, err := compH.Compress(bytes.NewReader(headerBytes), &headerCompCounter); err != nil {
		return 0, 0, 0, 0, err
	}
	if err := encHeaderW.Close(); err != nil {
		return 0, 0, 0, 0, err
	}

	headerPackSize := uint64(compHeaderBuf.Len())
	headerUnpackSize := uint64(len(headerBytes))
	headerCRC := crc32.ChecksumIEEE(headerBytes)

	if _, err := w.Write(compHeaderBuf.Bytes()); err != nil {
		return 0, 0, 0, 0, err
	}

	var outerBuf bytes.Buffer
	outerBuf.WriteByte(NIDEncodedHeader)
	outerBuf.WriteByte(NIDMainStreamsInfo)

	outerBuf.WriteByte(NIDPackInfo)
	_ = WriteNumber(&outerBuf, packSize)
	_ = WriteNumber(&outerBuf, 1)
	outerBuf.WriteByte(NIDSize)
	_ = WriteNumber(&outerBuf, headerPackSize)
	outerBuf.WriteByte(NIDEnd)

	outerBuf.WriteByte(NIDUnpackInfo)
	outerBuf.WriteByte(NIDFolder)
	_ = WriteNumber(&outerBuf, 1)
	outerBuf.WriteByte(0)

	_ = WriteNumber(&outerBuf, 2)
	writeCoder(&outerBuf, MethodLZMA2, []byte{0x18})
	writeCoder(&outerBuf, MethodAES, headerAESProps)
	_ = WriteNumber(&outerBuf, 0)
	_ = WriteNumber(&outerBuf, 1)

	outerBuf.WriteByte(NIDCodersUnpackSize)
	_ = WriteNumber(&outerBuf, headerUnpackSize)
	_ = WriteNumber(&outerBuf, uint64(headerCompCounter.n))

	outerBuf.WriteByte(NIDCRC)
	outerBuf.WriteByte(1)
	var hCrcB [4]byte
	binary.LittleEndian.PutUint32(hCrcB[:], headerCRC)
	outerBuf.Write(hCrcB[:])

	outerBuf.WriteByte(NIDEnd)
	outerBuf.WriteByte(NIDEnd)
	outerBuf.WriteByte(NIDEnd)

	finalHeaderBytes := outerBuf.Bytes()
	finalHeaderSize := uint64(len(finalHeaderBytes))
	finalHeaderCRC := crc32.ChecksumIEEE(finalHeaderBytes)

	if _, err := w.Write(finalHeaderBytes); err != nil {
		return int64(headerPackSize), 0, 0, 0, err
	}

	written := int64(headerPackSize) + int64(finalHeaderSize)
	nextHeaderOffset := packSize + headerPackSize
	return written, nextHeaderOffset, finalHeaderSize, finalHeaderCRC, nil
}

func patchStartHeader(w io.Writer, nextHeaderOffset, nextHeaderSize uint64, nextHeaderCRC uint32) error {
	var startHeader [20]byte
	binary.LittleEndian.PutUint64(startHeader[0:8], nextHeaderOffset)
	binary.LittleEndian.PutUint64(startHeader[8:16], nextHeaderSize)
	binary.LittleEndian.PutUint32(startHeader[16:20], nextHeaderCRC)

	startCRC := crc32.ChecksumIEEE(startHeader[:])

	var fullStartHeader [24]byte
	binary.LittleEndian.PutUint32(fullStartHeader[0:4], startCRC)
	copy(fullStartHeader[4:24], startHeader[:])

	if ws, ok := w.(io.WriteSeeker); ok {
		currentPos, _ := ws.Seek(0, io.SeekCurrent)
		if _, err := ws.Seek(8, io.SeekStart); err == nil {
			_, _ = ws.Write(fullStartHeader[:])
			_, _ = ws.Seek(currentPos, io.SeekStart)
		}
	} else if bb, ok := w.(*bytes.Buffer); ok {
		b := bb.Bytes()
		if len(b) >= 32 {
			copy(b[8:32], fullStartHeader[:])
		}
	}
	return nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.n += int64(n)
	return n, err
}

func writeCoder(buf *bytes.Buffer, method uint64, props []byte) {
	writeComplexCoder(buf, method, 1, 1, props)
}

func writeComplexCoder(buf *bytes.Buffer, method uint64, inStreams, outStreams int, props []byte) {
	var idBytes []byte
	switch {
	case method <= 0xFF:
		idBytes = []byte{byte(method)}
	case method <= 0xFFFF:
		idBytes = []byte{byte(method >> 8), byte(method)}
	case method <= 0xFFFFFF:
		idBytes = []byte{byte(method >> 16), byte(method >> 8), byte(method)}
	default:
		idBytes = []byte{byte(method >> 24), byte(method >> 16), byte(method >> 8), byte(method)}
	}

	mainByte := byte(len(idBytes))
	if inStreams != 1 || outStreams != 1 {
		mainByte |= 0x10
	}
	if len(props) > 0 {
		mainByte |= 0x20
	}
	buf.WriteByte(mainByte)
	buf.Write(idBytes)
	if inStreams != 1 || outStreams != 1 {
		_ = WriteNumber(buf, uint64(inStreams))
		_ = WriteNumber(buf, uint64(outStreams))
	}
	if len(props) > 0 {
		_ = WriteNumber(buf, uint64(len(props)))
		buf.Write(props)
	}
}
