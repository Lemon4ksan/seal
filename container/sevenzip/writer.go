// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"encoding/binary"
	"hash"
	"hash/crc32"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/lemon4ksan/seal/codec/bcj"
	"github.com/lemon4ksan/seal/codec/lzma"
	"github.com/lemon4ksan/seal/codec/ppmd"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/internal/pool"
)

// FilterType defines bytecode preprocessing filters.
type FilterType int

const (
	FilterNone FilterType = iota
	FilterBCJ
	FilterARM
)

// MethodType defines primary compression codecs for 7z containers.
type MethodType uint64

const (
	MethodLZMA2Default MethodType = MethodLZMA2
	MethodPPMDDefault   MethodType = MethodPPMD
)

// WriterOptions specifies configuration options for the 7z Writer.
type WriterOptions struct {
	LZMA       lzma.Options
	Filter     FilterType
	Method     MethodType
	PPMdOrder  int
	PPMdMem    uint32
	OnProgress func(phase string, current, total int64)
}

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
	buf   *[]byte
	data  []byte
	err   error
	isEOF bool
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

	blockChan chan prefetchBlock
	currBlock prefetchBlock
	currOff   int
	doneChan  chan struct{}
}

func newAsyncPrefetchPipe(files []*file.File, filter FilterType, onProgress func(phase string, current, total int64)) *asyncPrefetchPipe {
	pipe := &asyncPrefetchPipe{
		files:           files,
		fileEntries:     make([]FileEntry, len(files)),
		totalCRC:        crc32.NewIEEE(),
		totalFilesCount: int64(len(files)),
		filter:          filter,
		onProgress:      onProgress,
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
				curCRC.Write(bufSlice[:nr])
				p.totalCRC.Write(bufSlice[:nr])
				fileBytesRead = uint64(nr)

				// Apply BCJ branch conversion if enabled
				if p.filter == FilterBCJ {
					bcj.Filter(bcj.X86, bufSlice[:nr], p.bcjIP, true)
					p.bcjIP += uint32(nr)
				} else if p.filter == FilterARM {
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
				bufPtr := pool.Get64K()
				bufSlice := (*bufPtr)[:]

				nr, rErr := rc.Read(bufSlice)
				if nr > 0 {
					curCRC.Write(bufSlice[:nr])
					p.totalCRC.Write(bufSlice[:nr])
					fileBytesRead += uint64(nr)

					// Apply BCJ branch conversion if enabled
					if p.filter == FilterBCJ {
						bcj.Filter(bcj.X86, bufSlice[:nr], p.bcjIP, true)
						p.bcjIP += uint32(nr)
					} else if p.filter == FilterARM {
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
		if p.onProgress != nil && (readDone%100 == 0 || readDone == p.totalFilesCount) {
			p.onProgress("Reading", readDone, p.totalFilesCount)
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
					pool.Put64K(p.currBlock.buf)
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

	case ".exe", ".dll", ".sys", ".node", ".so", ".dylib", ".pdb", ".lib", ".obj", ".o", ".a", ".ocx", ".bin", ".dat", ".rsrc":
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
	var totalWritten int64

	// Emit 7z magic signature and allocate 32-byte StartHeader slot for deferred CRC patch
	var sigAndStart [StartHeaderSize]byte
	copy(sigAndStart[:SignatureSize], Signature[:])
	sigAndStart[6] = 0 // Major 0
	sigAndStart[7] = 4 // Minor 4

	if _, err := zw.w.Write(sigAndStart[:]); err != nil {
		return 0, err
	}
	totalWritten += int64(StartHeaderSize)

	// Smart Solid File Sorting: group files by structural redundancy category, extension, and path
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

	filter := zw.options.Filter
	method := zw.options.Method
	if method == 0 {
		method = MethodLZMA2Default
	}

	onProg := zw.OnProgress
	if onProg == nil {
		onProg = zw.options.OnProgress
	}

	solidReader := newAsyncPrefetchPipe(sortedFiles, filter, onProg)

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

	if method == MethodPPMDDefault {
		// PPMd Compression
		order := zw.options.PPMdOrder
		if order < 2 {
			order = ppmd.DefaultOrder
		}
		mem := zw.options.PPMdMem
		if mem < 1024*1024 {
			mem = ppmd.DefaultMemSize
		}
		comp := ppmd.NewCompressor(order, mem)
		if _, err := comp.Compress(solidReader, cw); err != nil {
			return totalWritten, err
		}
	} else {
		// LZMA2 Compression
		opts := zw.options.LZMA
		if opts.Level == 0 && opts.DictSize == 0 && opts.ChainLength == 0 {
			opts = lzma.DefaultOptions()
		}
		if onProg != nil {
			opts.OnProgress = func(processed, _ int64) {
				onProg("Compressing", processed, totalEstimatedChunks)
			}
		}
		comp2 := lzma.NewCompressor2WithOptions(opts)
		if _, err := comp2.Compress(solidReader, cw); err != nil {
			return totalWritten, err
		}
	}

	totalWritten += cw.n

	packSize := uint64(cw.n)
	totalUnpackSize := solidReader.totalUnpackSize
	unpackCRC := solidReader.totalCRC.Sum32()
	fileEntries := solidReader.fileEntries
	unpackSizes := solidReader.unpackSizes

	if onProg != nil {
		onProg("Finalizing", 1, 1)
	}

	// Construct 7z binary metadata structures (PackInfo, UnpackInfo, SubStreamsInfo, FilesInfo)
	var headerBuf bytes.Buffer

	// NIDHeader
	headerBuf.WriteByte(NIDHeader)

	// MainStreamsInfo
	headerBuf.WriteByte(NIDMainStreamsInfo)

	// PackInfo
	headerBuf.WriteByte(NIDPackInfo)
	_ = WriteNumber(&headerBuf, 0) // PackPos = 0
	_ = WriteNumber(&headerBuf, 1) // NumPackStreams = 1
	headerBuf.WriteByte(NIDSize)
	_ = WriteNumber(&headerBuf, packSize)
	headerBuf.WriteByte(NIDEnd)

	// UnpackInfo
	headerBuf.WriteByte(NIDUnpackInfo)
	headerBuf.WriteByte(NIDFolder)
	_ = WriteNumber(&headerBuf, 1) // 1 Folder
	headerBuf.WriteByte(0)         // External = 0

	if method == MethodPPMDDefault {
		// PPMd Coder: 0x030401 (3 bytes ID, 5-byte properties)
		_ = WriteNumber(&headerBuf, 1) // NumCoders = 1
		headerBuf.WriteByte(0x23)      // MainByte: 3-byte ID, has properties
		headerBuf.Write([]byte{0x03, 0x04, 0x01})
		_ = WriteNumber(&headerBuf, 5) // PropSize = 5
		order := byte(zw.options.PPMdOrder)
		if order < 2 {
			order = byte(ppmd.DefaultOrder)
		}
		mem := zw.options.PPMdMem
		if mem < 1024*1024 {
			mem = uint32(ppmd.DefaultMemSize)
		}
		var propB [5]byte
		propB[0] = order
		binary.LittleEndian.PutUint32(propB[1:5], mem)
		headerBuf.Write(propB[:])

		// CodersUnpackSize
		headerBuf.WriteByte(NIDCodersUnpackSize)
		_ = WriteNumber(&headerBuf, totalUnpackSize)

	} else if filter == FilterBCJ || filter == FilterARM {
		// 2 Coders: Coder 0 = BCJ/ARM, Coder 1 = LZMA2
		_ = WriteNumber(&headerBuf, 2) // NumCoders = 2

		// Coder 0: BCJ (0x03030103) or ARM (0x03030501)
		if filter == FilterBCJ {
			headerBuf.WriteByte(0x04) // 4-byte ID, no properties
			headerBuf.Write([]byte{0x03, 0x03, 0x01, 0x03})
		} else {
			headerBuf.WriteByte(0x04)
			headerBuf.Write([]byte{0x03, 0x03, 0x05, 0x01})
		}

		// Coder 1: LZMA2 (0x21)
		headerBuf.WriteByte(0x21) // 1-byte ID, has properties
		headerBuf.WriteByte(0x21) // ID = 0x21
		_ = WriteNumber(&headerBuf, 1)
		headerBuf.WriteByte(0x18) // 8MB dict

		// BindPairs (NumBindPairs = 1: Coder 0 in <- Coder 1 out)
		_ = WriteNumber(&headerBuf, 0) // InIndex = 0
		_ = WriteNumber(&headerBuf, 1) // OutIndex = 1

		// CodersUnpackSize (both coders output totalUnpackSize)
		headerBuf.WriteByte(NIDCodersUnpackSize)
		_ = WriteNumber(&headerBuf, totalUnpackSize)
		_ = WriteNumber(&headerBuf, totalUnpackSize)

	} else {
		// 1 Coder (LZMA2: 0x21, 1-byte prop 0x18 = 8MB)
		_ = WriteNumber(&headerBuf, 1) // NumCoders = 1
		headerBuf.WriteByte(0x21)      // MainByte: 1-byte ID, Simple coder, has properties
		headerBuf.WriteByte(0x21)      // ID = 0x21 (LZMA2)
		_ = WriteNumber(&headerBuf, 1) // PropSize = 1
		headerBuf.WriteByte(0x18)      // 8MB dict

		// CodersUnpackSize
		headerBuf.WriteByte(NIDCodersUnpackSize)
		_ = WriteNumber(&headerBuf, totalUnpackSize)
	}

	// CRC
	headerBuf.WriteByte(NIDCRC)
	headerBuf.WriteByte(1) // AllDefined = 1
	var crcB [4]byte
	binary.LittleEndian.PutUint32(crcB[:], unpackCRC)
	headerBuf.Write(crcB[:])

	headerBuf.WriteByte(NIDEnd) // End UnpackInfo

	// SubStreamsInfo
	if len(unpackSizes) > 1 {
		headerBuf.WriteByte(NIDSubStreamsInfo)
		headerBuf.WriteByte(NIDNumUnpackStream)
		_ = WriteNumber(&headerBuf, uint64(len(unpackSizes)))

		headerBuf.WriteByte(NIDSize)
		for _, sz := range unpackSizes[:len(unpackSizes)-1] {
			_ = WriteNumber(&headerBuf, sz)
		}

		headerBuf.WriteByte(NIDCRC)
		headerBuf.WriteByte(1) // AllDefined
		for _, fe := range fileEntries {
			if !fe.IsDir && fe.Size > 0 {
				binary.LittleEndian.PutUint32(crcB[:], fe.CRC)
				headerBuf.Write(crcB[:])
			}
		}

		headerBuf.WriteByte(NIDEnd)
	}

	headerBuf.WriteByte(NIDEnd) // End MainStreamsInfo

	// FilesInfo
	headerBuf.WriteByte(NIDFilesInfo)
	_ = WriteNumber(&headerBuf, uint64(len(fileEntries)))

	// Check EmptyStreams
	var emptyCount int
	for _, fe := range fileEntries {
		if fe.IsEmptyStream {
			emptyCount++
		}
	}
	if emptyCount > 0 {
		headerBuf.WriteByte(NIDEmptyStream)
		numBytes := (len(fileEntries) + 7) / 8
		_ = WriteNumber(&headerBuf, uint64(numBytes))
		bitmask := make([]byte, numBytes)
		for i, fe := range fileEntries {
			if fe.IsEmptyStream {
				byteIdx := i / 8
				bitIdx := 7 - (i % 8)
				bitmask[byteIdx] |= (1 << bitIdx)
			}
		}
		headerBuf.Write(bitmask)
	}

	// Names (UTF-16LE + 2 zero bytes)
	headerBuf.WriteByte(NIDName)
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
	_ = WriteNumber(&headerBuf, uint64(namesBuf.Len()))
	headerBuf.Write(namesBuf.Bytes())

	// WinAttrib
	headerBuf.WriteByte(NIDWinAttrib)
	var attrBuf bytes.Buffer
	attrBuf.WriteByte(1) // AllDefined = 1
	attrBuf.WriteByte(0) // External = 0
	for _, fe := range fileEntries {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], fe.Attrib)
		attrBuf.Write(b[:])
	}
	_ = WriteNumber(&headerBuf, uint64(attrBuf.Len()))
	headerBuf.Write(attrBuf.Bytes())

	headerBuf.WriteByte(NIDEnd) // End FilesInfo
	headerBuf.WriteByte(NIDEnd) // End Header

	headerBytes := headerBuf.Bytes()
	headerSize := uint64(len(headerBytes))
	headerCRC := crc32.ChecksumIEEE(headerBytes)

	if _, err := zw.w.Write(headerBytes); err != nil {
		return totalWritten, err
	}
	totalWritten += int64(len(headerBytes))

	// Re-patch the 32-byte StartHeader
	nextHeaderOffset := uint64(packSize)

	var startHeader [20]byte
	binary.LittleEndian.PutUint64(startHeader[0:8], nextHeaderOffset)
	binary.LittleEndian.PutUint64(startHeader[8:16], headerSize)
	binary.LittleEndian.PutUint32(startHeader[16:20], headerCRC)

	startCRC := crc32.ChecksumIEEE(startHeader[:])

	var fullStartHeader [24]byte
	binary.LittleEndian.PutUint32(fullStartHeader[0:4], startCRC)
	copy(fullStartHeader[4:24], startHeader[:])

	if ws, ok := zw.w.(io.WriteSeeker); ok {
		currentPos, _ := ws.Seek(0, io.SeekCurrent)
		if _, err := ws.Seek(8, io.SeekStart); err == nil {
			_, _ = ws.Write(fullStartHeader[:])
			_, _ = ws.Seek(currentPos, io.SeekStart)
		}
	} else if bb, ok := zw.w.(*bytes.Buffer); ok {
		b := bb.Bytes()
		if len(b) >= 32 {
			copy(b[8:32], fullStartHeader[:])
		}
	}

	return totalWritten, nil
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
