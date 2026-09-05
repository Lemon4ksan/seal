// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/cdc"
	"github.com/lemon4ksan/seal/container/eon/chunk"
	"github.com/lemon4ksan/seal/container/eon/codec"
	"github.com/lemon4ksan/seal/container/eon/crypto"
	"github.com/lemon4ksan/seal/container/eon/filter"
	"github.com/lemon4ksan/seal/container/eon/prolly"
	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/foundation/codec/filter/shuffle"
)

// WriterOption configures the EON container serialization engine.
type WriterOption func(*Writer)

// WithBlockSizeShift configures the logical sector size shift (default: 12 -> 4096 bytes).
func WithBlockSizeShift(shift uint8) WriterOption {
	return func(w *Writer) {
		w.blockShift = shift
	}
}

// WithChunkTargetSize sets the uncompressed size boundary for solid chunks (default: 32 MiB).
func WithChunkTargetSize(size uint32) WriterOption {
	return func(w *Writer) {
		w.targetChunkSize = size
	}
}

// WithProfile sets the container profile hint (e.g. ProfileVFS, ProfileFastMount, ProfileArchiveMax).
func WithProfile(profile uint8) WriterOption {
	return func(w *Writer) {
		w.profile = profile
		switch profile {
		case spec.ProfileFastMount:
			w.targetChunkSize = 128 * 1024 // 128 KiB micro-chunks for sub-millisecond seek latency
			w.codecID = spec.CodecLZ4
			w.codecLevel = 1
		case spec.ProfileArchiveMax:
			w.targetChunkSize = 64 * 1024 * 1024 // 64 MiB solid chunks for maximum dictionary compression
			w.codecID = spec.CodecZSTD
			w.codecLevel = 19
		}
	}
}

// WithProfileFastMount configures the container for ultra-low seek latency (128KB LZ4 micro-frames).
func WithProfileFastMount() WriterOption {
	return WithProfile(spec.ProfileFastMount)
}

// WithProfileArchiveMax configures the container for maximum solid compression ratio (64MB ZSTD-19).
func WithProfileArchiveMax() WriterOption {
	return WithProfile(spec.ProfileArchiveMax)
}

// WithAutoDetect enables content-aware codec dispatching (Store for incompressible, Byte-Shuffle for tensors).
func WithAutoDetect(enable bool) WriterOption {
	return func(w *Writer) {
		w.autoDetect = enable
	}
}

// WithCodec sets the default compression algorithm for chunks (default: CodecZSTD).
func WithCodec(codecID uint8) WriterOption {
	return func(w *Writer) {
		w.codecID = codecID
	}
}

// WithCodecLevel sets the compression tuning level (1..9, or 0 for default).
func WithCodecLevel(level uint8) WriterOption {
	return func(w *Writer) {
		w.codecLevel = level
	}
}

// WithChunkPipeline configures a default filter and codec pipeline for container chunks.
func WithChunkPipeline(p *filter.Pipeline) WriterOption {
	return func(w *Writer) {
		w.chunkPipeline = p
		if p != nil {
			w.codecID = p.CodecID
			w.codecLevel = p.CodecLevel
		}
	}
}

// WithSigner configures an Ed25519 private key to digitally sign the Central Directory.
func WithSigner(privKey ed25519.PrivateKey, keyID string) WriterOption {
	return func(w *Writer) {
		w.signerKey = privKey
		w.signerKeyID = keyID
	}
}

// WithPassword enables authenticated envelope encryption with the specified passphrase.
func WithPassword(password string) WriterOption {
	return func(w *Writer) {
		w.password = password
	}
}

// WithCipher configures the symmetric AEAD algorithm (default: spec.CipherAES256GCM).
func WithCipher(cipherID uint8) WriterOption {
	return func(w *Writer) {
		w.cipherID = cipherID
	}
}

// WithKDFParams sets Argon2id memory hardness (KiB), iterations, and parallelism.
func WithKDFParams(memKiB, iter uint32, threads uint8) WriterOption {
	return func(w *Writer) {
		w.kdfMem = memKiB
		w.kdfIter = iter
		w.kdfThreads = threads
	}
}

// WithKMSKey configures envelope encryption using Key Management Service (HashiCorp Vault or custom KMS).
func WithKMSKey(keyID string, client crypto.KMSClient) WriterOption {
	return func(w *Writer) {
		w.kmsKeyID = keyID
		w.kmsClient = client
	}
}

// WithReproducible configures bit-for-bit deterministic archive generation with fixed epoch timestamps.
// All file modification times and header timestamps are clamped to epochNano.
func WithReproducible(epochNano int64) WriterOption {
	return func(w *Writer) {
		w.reproducible = true
		w.sourceDateEpoch = epochNano
	}
}

// WithCDC enables Content-Defined Chunking (FastCDC) with optional average chunk size.
func WithCDC(enable bool, avgSize ...uint32) WriterOption {
	return func(w *Writer) {
		w.useCDC = enable
		if len(avgSize) > 0 && avgSize[0] > 0 {
			w.cdcOpts.AvgSize = avgSize[0]
			w.cdcOpts.MinSize = avgSize[0] / 4
			w.cdcOpts.MaxSize = avgSize[0] * 4
		}
	}
}

// WithCDCOptions configures explicit FastCDC chunking parameters.
func WithCDCOptions(opts cdc.Options) WriterOption {
	return func(w *Writer) {
		w.useCDC = true
		w.cdcOpts = opts
	}
}

// WithProllyTree enables the hierarchical content-addressed Prolly Tree catalog.
func WithProllyTree(enable bool) WriterOption {
	return func(w *Writer) {
		w.useProllyTree = enable
	}
}

// WithSnapshot configures a commit message and parent snapshot ID.
func WithSnapshot(message, parentID string) WriterOption {
	return func(w *Writer) {
		w.useProllyTree = true
		w.snapshotMsg = message
		w.snapshotParent = parentID
	}
}

// WithSnapshotsFrom imports existing snapshots, tree nodes, and extent payloads from a parent reader.
func WithSnapshotsFrom(r *Reader) WriterOption {
	return func(w *Writer) {
		w.useProllyTree = true
		if r == nil {
			return
		}
		w.parentReader = r
		if r.snapshots != nil {
			for _, s := range r.snapshots.Snapshots {
				w.snapshotTable.Add(s)
			}
		}
		if r.treeStore != nil {
			for _, n := range r.treeStore.Nodes() {
				_ = w.treeStore.PutNode(n)
			}
		}
		if r.indx != nil {
			for _, ext := range r.indx.Extents {
				w.parentExtents[ext.Name] = ext
				if ext.PayloadSize > 0 {
					w.payloadDedup[ext.Hash] = ext
				}
			}
		}
		if r.snapshots != nil && r.treeStore != nil {
			latest := r.snapshots.Latest()
			if latest != nil {
				tree := &prolly.Tree{RootHash: latest.RootHash, Store: r.treeStore, ItemCount: latest.FileCount}
				if exts, err := tree.CollectExtents(); err == nil {
					for _, ext := range exts {
						w.parentExtents[ext.Name] = ext
						if ext.PayloadSize > 0 {
							w.payloadDedup[ext.Hash] = ext
						}
					}
				}
			}
		}
	}
}

type chunkTask struct {
	index      uint32
	codecID    uint8
	codecLevel uint8
	flags      uint16
	itemCount  uint32
	raw        []byte
	pipeline   *filter.Pipeline

	chunk     *chunk.Chunk
	sectorBuf []byte
	done      chan struct{}
	err       error
}

type vaultState struct {
	name       string
	password   string
	descriptor *crypto.VaultDescriptor
	extents    []catalog.Extent
	dirAEAD    cipher.AEAD
}

// Writer constructs and serializes a standard EON Container with a high-speed pipelined chunk engine.
type Writer struct {
	w               io.Writer
	blockShift      uint8
	targetChunkSize uint32
	profile         uint8
	codecID         uint8
	codecLevel      uint8
	signerKey       ed25519.PrivateKey
	signerKeyID     string

	password      string
	kmsClient     crypto.KMSClient
	kmsKeyID      string
	cipherID      uint8
	kdfMem        uint32
	kdfIter       uint32
	kdfThreads    uint8
	chunkPipeline *filter.Pipeline

	reproducible    bool
	sourceDateEpoch int64

	encr      *crypto.ENCR
	dek       []byte
	chunkAEAD cipher.AEAD
	dirAEAD   cipher.AEAD
	hdrBytes  [spec.HeaderSize]byte

	vaults       []*vaultState
	vaultMap     map[string]*vaultState
	targetVault  string
	defaultVD    *crypto.VaultDescriptor
	payloadDedup map[[32]byte]catalog.Extent

	totalWritten  int64
	headerWritten bool
	closed        bool

	currentChunkIndex uint32
	inFlight          []*chunkTask

	useCDC            bool
	cdcOpts           cdc.Options
	cdcChunker        *cdc.FastCDC
	chunkDedup        map[[32]byte]uint32
	dedupReusedChunks uint32
	dedupSavedBytes   uint64

	useProllyTree  bool
	snapshotMsg    string
	snapshotParent string
	snapshotTable  *prolly.SnapshotTable
	treeStore      *prolly.MemoryNodeStore
	parentReader   *Reader
	parentExtents  map[string]catalog.Extent

	ctocEntries []catalog.CTOCEntry
	extents     []catalog.Extent

	autoDetect             bool
	currentChunkCodecID    uint8
	currentChunkCodecLevel uint8
	currentChunkFlags      uint16

	currentRaw   []byte
	currentItems uint32
}

// NewWriter constructs an EON container serializer writing to w.
func NewWriter(w io.Writer, opts ...WriterOption) *Writer {
	wr := &Writer{
		w:               w,
		blockShift:      spec.DefaultBlockSizeShift,
		targetChunkSize: spec.DefaultChunkTargetSize,
		profile:         spec.ProfileVFS,
		codecID:         spec.CodecZSTD,
		autoDetect:      true,
		cipherID:        spec.CipherAES256GCM,
		kdfMem:          spec.DefaultArgon2Memory,
		kdfIter:         spec.DefaultArgon2Iterations,
		kdfThreads:      spec.DefaultArgon2Parallelism,
		currentRaw:      make([]byte, 0, spec.DefaultChunkTargetSize),
		vaultMap:        make(map[string]*vaultState),
		payloadDedup:    make(map[[32]byte]catalog.Extent),
		chunkDedup:      make(map[[32]byte]uint32),
		cdcOpts:         cdc.DefaultOptions(),
		snapshotTable:   prolly.NewSnapshotTable(),
		treeStore:       prolly.NewMemoryNodeStore(),
		parentExtents:   make(map[string]catalog.Extent),
	}
	if val := os.Getenv("SOURCE_DATE_EPOCH"); val != "" {
		if ep, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil {
			wr.reproducible = true
			wr.sourceDateEpoch = ep * int64(time.Second)
		}
	}
	for _, opt := range opts {
		opt(wr)
	}
	wr.currentChunkCodecID = wr.codecID
	wr.currentChunkCodecLevel = wr.codecLevel
	return wr
}

// AddVault registers an isolated virtual vault within the container with its own passphrase.
func (w *Writer) AddVault(name, password string) error {
	if w.headerWritten {
		return fmt.Errorf("cannot add vault after writing has started")
	}
	if _, exists := w.vaultMap[name]; exists {
		return fmt.Errorf("vault %q already registered", name)
	}

	vs := &vaultState{
		name:     name,
		password: password,
	}
	w.vaults = append(w.vaults, vs)
	w.vaultMap[name] = vs
	if w.targetVault == "" {
		w.targetVault = name
	}
	return nil
}

// SetTargetVault selects which vault incoming AddFile/AddExtent calls target.
func (w *Writer) SetTargetVault(name string) error {
	if _, ok := w.vaultMap[name]; !ok {
		return fmt.Errorf("vault %q not found", name)
	}
	w.targetVault = name
	return nil
}

// AddVaultFile encodes an uncompressed file entry into a specific named vault.
func (w *Writer) AddVaultFile(vaultName, name string, payload []byte) error {
	if err := w.SetTargetVault(vaultName); err != nil {
		return err
	}
	return w.AddFile(name, payload)
}

// AddVaultFileStream encodes an incoming stream into a specific named vault.
func (w *Writer) AddVaultFileStream(vaultName, name string, size int64, modTime time.Time, r io.Reader) error {
	if err := w.SetTargetVault(vaultName); err != nil {
		return err
	}
	return w.AddFileStream(name, size, modTime, r)
}

func (w *Writer) ensureHeader() error {
	if w.headerWritten {
		return nil
	}

	blockSize := int64(1 << w.blockShift)
	zeroBuf := make([]byte, blockSize)

	flags := spec.FlagCentralDir | spec.FlagAlignedBlocks
	if w.useCDC {
		flags |= spec.FlagCDC
	}
	if w.useProllyTree {
		flags |= spec.FlagProllyTree
	}
	if len(w.signerKey) > 0 {
		flags |= spec.FlagDigitalSig
	}

	if len(w.vaults) > 0 {
		flags |= spec.FlagEncrypted
		var masterDEK [32]byte
		if _, err := io.ReadFull(rand.Reader, masterDEK[:]); err != nil {
			return fmt.Errorf("generate master dek: %w", err)
		}
		var masterIV [16]byte
		if _, err := io.ReadFull(rand.Reader, masterIV[:]); err != nil {
			return fmt.Errorf("generate master iv: %w", err)
		}

		w.encr = &crypto.ENCR{
			CipherID: w.cipherID,
			MasterIV: masterIV,
		}
		w.dek = masterDEK[:]

		chunkKey := crypto.DeriveSubkey(masterDEK[:], "eon-chunk-payload-key", 32)
		chunkAEAD, err := crypto.NewAEAD(w.cipherID, chunkKey)
		if err != nil {
			return fmt.Errorf("init chunk aead: %w", err)
		}
		w.chunkAEAD = chunkAEAD

		for i, vs := range w.vaults {
			vd := &crypto.VaultDescriptor{
				VaultID:    uint8(i),
				VaultFlags: 0,
				VaultName:  vs.name,
				DEK:        masterDEK,
			}
			vs.descriptor = vd

			dirKey := crypto.DeriveSubkey(masterDEK[:], "eon-central-directory-key-"+vs.name, 32)
			vsDirAEAD, err := crypto.NewAEAD(w.cipherID, dirKey)
			if err != nil {
				return fmt.Errorf("init dir aead (%s): %w", vs.name, err)
			}
			vs.dirAEAD = vsDirAEAD

			if _, err := w.encr.AddVaultSlot(vs.password, vd, w.kdfMem, w.kdfIter, w.kdfThreads); err != nil {
				return fmt.Errorf("add vault slot (%s): %w", vs.name, err)
			}
		}
	} else if w.password != "" || (w.kmsClient != nil && w.kmsKeyID != "") {
		flags |= spec.FlagEncrypted
		var encr *crypto.ENCR
		var vd *crypto.VaultDescriptor
		var err error

		if w.password != "" {
			encr, vd, err = crypto.CreateENCR(w.password, w.cipherID, w.kdfMem, w.kdfIter, w.kdfThreads)
			if err != nil {
				return fmt.Errorf("init encryption: %w", err)
			}
			if w.kmsClient != nil && w.kmsKeyID != "" {
				if _, err := encr.AddKMSSlot(context.Background(), w.kmsClient, w.kmsKeyID, vd); err != nil {
					return fmt.Errorf("init kms slot: %w", err)
				}
			}
		} else {
			encr, vd, err = crypto.CreateKMSENCR(context.Background(), w.kmsClient, w.kmsKeyID, w.cipherID)
			if err != nil {
				return fmt.Errorf("init kms encryption: %w", err)
			}
		}

		w.encr = encr
		w.dek = vd.DEK[:]
		w.defaultVD = vd

		chunkKey := crypto.DeriveSubkey(vd.DEK[:], "eon-chunk-payload-key", 32)
		dirKey := crypto.DeriveSubkey(vd.DEK[:], "eon-central-directory-key", 32)

		chunkAEAD, err := crypto.NewAEAD(w.cipherID, chunkKey)
		if err != nil {
			return fmt.Errorf("init chunk aead: %w", err)
		}
		w.chunkAEAD = chunkAEAD

		dirAEAD, err := crypto.NewAEAD(w.cipherID, dirKey)
		if err != nil {
			return fmt.Errorf("init dir aead: %w", err)
		}
		w.dirAEAD = dirAEAD

		vs := &vaultState{
			name:       "default",
			password:   w.password,
			descriptor: vd,
			dirAEAD:    dirAEAD,
		}
		w.vaults = []*vaultState{vs}
		w.vaultMap["default"] = vs
		w.targetVault = "default"
	}

	// Build 32-byte Archive Header
	var hdr [spec.HeaderSize]byte
	copy(hdr[0:4], spec.MagicHeader[:])
	hdr[4] = spec.VersionMajor
	hdr[5] = spec.VersionMinor
	hdr[6] = w.profile
	hdr[7] = 0 // Agnostic OS
	hdr[8] = 0 // Agnostic Arch
	hdr[9] = w.blockShift
	binary.LittleEndian.PutUint16(hdr[10:12], flags)
	binary.LittleEndian.PutUint32(hdr[12:16], w.targetChunkSize)
	var headerTimestamp uint64
	if w.reproducible {
		headerTimestamp = uint64(w.sourceDateEpoch)
	} else {
		headerTimestamp = uint64(time.Now().UnixNano())
	}
	binary.LittleEndian.PutUint64(hdr[16:24], headerTimestamp)
	crc := crc32.ChecksumIEEE(hdr[0:24])
	binary.LittleEndian.PutUint32(hdr[24:28], crc)

	w.hdrBytes = hdr
	if _, err := w.w.Write(hdr[:]); err != nil {
		return err
	}
	w.totalWritten += spec.HeaderSize

	if w.encr != nil {
		written, err := w.encr.Write(w.w)
		if err != nil {
			return fmt.Errorf("write encr header: %w", err)
		}
		w.totalWritten += written
	}

	// Pad header to logical block boundary so Chunk 0 begins block-aligned
	pad := blockSize - (w.totalWritten % blockSize)
	if pad < blockSize {
		if _, err := w.w.Write(zeroBuf[:pad]); err != nil {
			return err
		}
		w.totalWritten += pad
	}

	w.headerWritten = true
	if w.parentReader != nil {
		if err := w.importParentChunks(); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) importParentChunks() error {
	r := w.parentReader
	if r == nil || r.ctoc == nil {
		return nil
	}

	blockSize := int64(1 << w.blockShift)
	for i, entry := range r.ctoc.Entries {
		chunkOffset := int64(entry.BlockIndex) << r.blockShift
		unpaddedLen := spec.ChunkHeaderSize + int64(entry.CompressedSize)
		rem := unpaddedLen % blockSize
		pad := int64(0)
		if rem != 0 {
			pad = blockSize - rem
		}
		totalSectorSize := unpaddedLen + pad

		rawChunk := make([]byte, totalSectorSize)
		if _, err := r.rAt.ReadAt(rawChunk, chunkOffset); err != nil {
			return fmt.Errorf("import chunk %d from parent: %w", i, err)
		}

		currentBlock := uint64(w.totalWritten >> w.blockShift)
		if _, err := w.w.Write(rawChunk); err != nil {
			return fmt.Errorf("write imported chunk %d: %w", i, err)
		}
		w.totalWritten += int64(len(rawChunk))

		newEntry := entry
		newEntry.BlockIndex = currentBlock
		w.ctocEntries = append(w.ctocEntries, newEntry)
	}

	w.currentChunkIndex = uint32(len(w.ctocEntries))
	return nil
}

func (w *Writer) processChunkTask(task *chunkTask) {
	defer close(task.done)

	raw := task.raw
	chunkFlags := task.flags
	if chunkFlags == 0 {
		chunkFlags = spec.ChunkFlagSolid
	}

	p := task.pipeline

	var compressed []byte
	var err error
	codecID := task.codecID
	codecLevel := task.codecLevel

	if p != nil {
		chunkFlags |= spec.ChunkFlagPipeline
		codecID = p.CodecID
		codecLevel = p.CodecLevel
		compressed, err = p.Encode(raw)
		if err != nil {
			task.err = fmt.Errorf("pipeline compress chunk %d: %w", task.index, err)
			return
		}
	} else {
		if (chunkFlags & spec.ChunkFlagByteShuffle) != 0 {
			raw, err = shuffle.Encode(raw, nil, 4)
			if err != nil {
				task.err = fmt.Errorf("chunk shuffle encode %d: %w", task.index, err)
				return
			}
		}

		lvl := codecLevel
		if lvl == 0 && codecID != spec.CodecStore {
			lvl = 3
		}

		encoder, err := codec.Get(codecID)
		if err != nil {
			task.err = fmt.Errorf("encoder init %d: %w", codecID, err)
			return
		}

		compressed, err = encoder.Compress(raw, int(lvl))
		if err != nil {
			task.err = fmt.Errorf("compress chunk %d: %w", task.index, err)
			return
		}
	}

	c := &chunk.Chunk{
		Index:            task.index,
		CodecID:          codecID,
		CodecLevel:       codecLevel,
		Flags:            chunkFlags,
		UncompressedSize: uint64(len(task.raw)),
		CompressedSize:   uint64(len(compressed)),
		ItemCount:        task.itemCount,
		Hash:             sha256.Sum256(task.raw),
		Payload:          compressed,
		Pipeline:         p,
	}

	if w.chunkAEAD != nil {
		c.Flags |= spec.ChunkFlagEncrypted

		var aad [21]byte
		copy(aad[0:4], spec.MagicChunk[:])
		binary.BigEndian.PutUint32(aad[4:8], task.index)
		aad[8] = c.CodecID
		binary.BigEndian.PutUint64(aad[9:17], c.UncompressedSize)
		binary.BigEndian.PutUint32(aad[17:21], task.itemCount)

		nonce := crypto.DeriveChunkNonce(w.encr.MasterIV[:], task.index)
		c.Payload = w.chunkAEAD.Seal(nil, nonce, c.Payload, aad[:])
		c.CompressedSize = uint64(len(c.Payload))
	}

	task.chunk = c

	// Build single-syscall sector-aligned buffer: Header (64B) + Payload + Sector Padding
	blockSize := int(1 << w.blockShift)
	unpaddedLen := spec.ChunkHeaderSize + int(c.CompressedSize)
	rem := unpaddedLen % blockSize
	pad := 0
	if rem != 0 {
		pad = blockSize - rem
	}
	totalSectorSize := unpaddedLen + pad

	sectorBuf := make([]byte, totalSectorSize)
	hdr := c.BuildHeader()
	copy(sectorBuf[0:spec.ChunkHeaderSize], hdr[:])
	copy(sectorBuf[spec.ChunkHeaderSize:unpaddedLen], c.Payload)

	task.sectorBuf = sectorBuf
}

func (w *Writer) flushChunk() error {
	if len(w.currentRaw) == 0 {
		return nil
	}

	task := &chunkTask{
		index:      w.currentChunkIndex,
		codecID:    w.currentChunkCodecID,
		codecLevel: w.currentChunkCodecLevel,
		flags:      w.currentChunkFlags,
		itemCount:  w.currentItems,
		raw:        w.currentRaw,
		pipeline:   w.chunkPipeline,
		done:       make(chan struct{}),
	}

	w.currentChunkIndex++
	w.currentRaw = make([]byte, 0, w.targetChunkSize)
	w.currentItems = 0

	// Launch background compression worker
	go w.processChunkTask(task)

	w.inFlight = append(w.inFlight, task)

	// Keep pipeline bounded: scale with NumCPU concurrent workers in flight
	maxInFlight := max(runtime.NumCPU(), 2)
	if len(w.inFlight) >= maxInFlight {
		if err := w.drainOne(); err != nil {
			return err
		}
	}

	return nil
}

func (w *Writer) drainOne() error {
	if len(w.inFlight) == 0 {
		return nil
	}

	task := w.inFlight[0]
	w.inFlight = w.inFlight[1:]

	<-task.done
	if task.err != nil {
		return task.err
	}

	blockIndex := uint64(w.totalWritten >> w.blockShift)
	task.chunk.BlockIndex = blockIndex

	// Single-syscall write: emit complete sector-aligned chunk
	n, err := w.w.Write(task.sectorBuf)
	if err != nil {
		return err
	}
	w.totalWritten += int64(n)

	// Append CTOC entry
	w.ctocEntries = append(w.ctocEntries, catalog.CTOCEntry{
		BlockIndex:       blockIndex,
		CompressedSize:   uint32(task.chunk.CompressedSize),
		UncompressedSize: uint32(task.chunk.UncompressedSize),
	})

	return nil
}

// AddFile encodes an uncompressed file entry into the POSIX VFS namespace.
func (w *Writer) AddFile(name string, payload []byte) error {
	cleanName := strings.TrimPrefix(name, "/")
	if len(payload) > 0 && w.payloadDedup != nil {
		h := sha256.Sum256(payload)
		if existing, ok := w.payloadDedup[h]; ok {
			ext := existing
			ext.Name = cleanName
			if w.reproducible {
				ext.Timestamp = w.sourceDateEpoch
			} else {
				ext.Timestamp = time.Now().UnixNano()
			}
			w.extents = append(w.extents, ext)
			if w.targetVault != "" && w.vaultMap != nil {
				if vs, ok := w.vaultMap[w.targetVault]; ok {
					vs.extents = append(vs.extents, ext)
				}
			}
			w.currentItems++
			return nil
		}
	}
	var modTime time.Time
	if w.reproducible {
		modTime = time.Unix(0, w.sourceDateEpoch)
	} else {
		modTime = time.Now()
	}
	return w.AddFileStream(name, int64(len(payload)), modTime, bytes.NewReader(payload))
}

// AddFileStream encodes an incoming stream into the POSIX VFS namespace with bounded memory and pipelined chunking.
func (w *Writer) AddFileStream(name string, size int64, modTime time.Time, r io.Reader) error {
	cleanName := strings.TrimPrefix(name, "/")
	isDir := strings.HasSuffix(cleanName, "/")

	if w.reproducible {
		if w.sourceDateEpoch == 0 || modTime.UnixNano() > w.sourceDateEpoch {
			modTime = time.Unix(0, w.sourceDateEpoch)
		}
	} else if modTime.IsZero() {
		modTime = time.Now()
	}

	var pxTLV [16]byte
	binary.LittleEndian.PutUint16(pxTLV[0:2], spec.TagPOSIXPermissions)
	binary.LittleEndian.PutUint16(pxTLV[2:4], 12)
	mode := uint32(0o644 | 0x8000)
	if isDir {
		mode = uint32(0o755 | 0x4000)
	}
	binary.LittleEndian.PutUint32(pxTLV[4:8], mode)

	if err := w.ensureHeader(); err != nil {
		return err
	}

	if isDir || size == 0 {
		ext := catalog.Extent{
			Namespace:   spec.NamespaceVFS,
			ChunkIndex:  w.currentChunkIndex,
			ChunkCount:  1,
			Timestamp:   modTime.UnixNano(),
			Name:        cleanName,
			ExtraTLVs:   pxTLV[:],
			ChunkOffset: uint64(len(w.currentRaw)),
			PayloadSize: 0,
		}
		w.extents = append(w.extents, ext)
		if w.targetVault != "" && w.vaultMap != nil {
			if vs, ok := w.vaultMap[w.targetVault]; ok {
				vs.extents = append(vs.extents, ext)
			}
		}
		w.currentItems++
		return nil
	}

	if w.parentExtents != nil && size > 0 {
		if prev, ok := w.parentExtents[cleanName]; ok {
			if prev.PayloadSize == uint64(size) && prev.Timestamp == modTime.UnixNano() {
				ext := prev
				w.extents = append(w.extents, ext)
				if w.targetVault != "" && w.vaultMap != nil {
					if vs, ok := w.vaultMap[w.targetVault]; ok {
						vs.extents = append(vs.extents, ext)
					}
				}
				w.currentItems++
				w.dedupReusedChunks++
				w.dedupSavedBytes += ext.PayloadSize
				return nil
			}
		}
	}

	fileCodecID := w.codecID
	fileCodecLevel := w.codecLevel
	var fileChunkFlags uint16

	if w.autoDetect {
		if codec.IsIncompressibleName(cleanName) {
			fileCodecID = spec.CodecStore
			fileCodecLevel = 0
		} else if codec.IsTensorName(cleanName) {
			fileChunkFlags |= spec.ChunkFlagByteShuffle
		}
	}

	if w.useCDC {
		return w.addFileStreamCDC(cleanName, size, modTime, pxTLV[:], r, fileCodecID, fileCodecLevel, fileChunkFlags)
	}

	// If current chunk has different codec or filter flags, flush it first
	if len(w.currentRaw) > 0 && (w.currentChunkCodecID != fileCodecID || w.currentChunkFlags != fileChunkFlags) {
		if err := w.flushChunk(); err != nil {
			return err
		}
	}

	w.currentChunkCodecID = fileCodecID
	w.currentChunkCodecLevel = fileCodecLevel
	w.currentChunkFlags = fileChunkFlags

	if len(w.currentRaw) >= int(w.targetChunkSize) {
		if err := w.flushChunk(); err != nil {
			return err
		}
	}

	startChunkIdx := w.currentChunkIndex
	startChunkOffset := uint64(len(w.currentRaw))
	hasher := sha256.New()

	var buf [64 * 1024]byte
	var writtenTotal int64
	var doneReading bool

	// If starting fresh chunk, sample first read for incompressible detection
	if w.autoDetect && fileCodecID != spec.CodecStore && (fileChunkFlags&spec.ChunkFlagByteShuffle == 0) && len(w.currentRaw) == 0 {
		avail := int(w.targetChunkSize)
		toRead := min(len(buf), avail)
		n, err := r.Read(buf[:toRead])
		if n > 0 {
			if n >= 512 && codec.IsIncompressibleSample(buf[:n]) {
				fileCodecID = spec.CodecStore
				fileCodecLevel = 0
				w.currentChunkCodecID = fileCodecID
				w.currentChunkCodecLevel = fileCodecLevel
			}
			hasher.Write(buf[:n])
			w.currentRaw = append(w.currentRaw, buf[:n]...)
			writtenTotal += int64(n)
			if len(w.currentRaw) >= int(w.targetChunkSize) {
				if err := w.flushChunk(); err != nil {
					return err
				}
			}
		}
		if err == io.EOF {
			doneReading = true
		} else if err != nil {
			return err
		}
	}

	if !doneReading {
		for {
			avail := int(w.targetChunkSize) - len(w.currentRaw)
			toRead := min(len(buf), avail)

			n, err := r.Read(buf[:toRead])
			if n > 0 {
				hasher.Write(buf[:n])
				w.currentRaw = append(w.currentRaw, buf[:n]...)
				writtenTotal += int64(n)
			}
			if len(w.currentRaw) >= int(w.targetChunkSize) {
				if err := w.flushChunk(); err != nil {
					return err
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
		}
	}

	var hash [32]byte
	copy(hash[:], hasher.Sum(nil))

	endChunkIdx := w.currentChunkIndex
	if len(w.currentRaw) == 0 && endChunkIdx > startChunkIdx {
		endChunkIdx--
	}
	chunkCount := uint16(endChunkIdx - startChunkIdx + 1)

	ext := catalog.Extent{
		Namespace:   spec.NamespaceVFS,
		ChunkIndex:  startChunkIdx,
		ChunkCount:  chunkCount,
		Timestamp:   modTime.UnixNano(),
		Name:        cleanName,
		ExtraTLVs:   pxTLV[:],
		ChunkOffset: startChunkOffset,
		PayloadSize: uint64(size),
		Hash:        hash,
	}
	w.extents = append(w.extents, ext)
	if w.targetVault != "" && w.vaultMap != nil {
		if vs, ok := w.vaultMap[w.targetVault]; ok {
			vs.extents = append(vs.extents, ext)
		}
	}
	if len(w.payloadDedup) < 100000 && size > 0 {
		w.payloadDedup[hash] = ext
	}
	w.currentItems++
	return nil
}

func (w *Writer) addFileStreamCDC(cleanName string, size int64, modTime time.Time, pxTLV []byte, r io.Reader, fileCodecID uint8, fileCodecLevel uint8, fileChunkFlags uint16) error {
	if len(w.currentRaw) > 0 {
		if err := w.flushChunk(); err != nil {
			return err
		}
	}

	chunker, err := cdc.NewChunker(r, w.cdcOpts)
	if err != nil {
		return fmt.Errorf("init cdc chunker: %w", err)
	}

	hasher := sha256.New()
	var chunkList []uint32
	var writtenTotal int64

	for {
		chunkBytes, err := chunker.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("cdc read chunk: %w", err)
		}
		if len(chunkBytes) == 0 {
			continue
		}

		hasher.Write(chunkBytes)
		writtenTotal += int64(len(chunkBytes))
		chunkHash := sha256.Sum256(chunkBytes)

		// CAS deduplication check
		if existingIdx, ok := w.chunkDedup[chunkHash]; ok {
			w.dedupReusedChunks++
			w.dedupSavedBytes += uint64(len(chunkBytes))
			chunkList = append(chunkList, existingIdx)
			continue
		}

		// New chunk: submit for compression
		cIdx := w.currentChunkIndex
		w.currentChunkIndex++

		task := &chunkTask{
			index:      cIdx,
			codecID:    fileCodecID,
			codecLevel: fileCodecLevel,
			flags:      fileChunkFlags,
			itemCount:  1,
			raw:        chunkBytes,
			done:       make(chan struct{}),
		}
		w.inFlight = append(w.inFlight, task)
		go w.processChunkTask(task)

		w.chunkDedup[chunkHash] = cIdx
		chunkList = append(chunkList, cIdx)

		for len(w.inFlight) >= 2 {
			if err := w.drainOne(); err != nil {
				return err
			}
		}
	}

	var hash [32]byte
	copy(hash[:], hasher.Sum(nil))

	if len(chunkList) == 0 {
		ext := catalog.Extent{
			Namespace:   spec.NamespaceVFS,
			ChunkIndex:  w.currentChunkIndex,
			ChunkCount:  1,
			Timestamp:   modTime.UnixNano(),
			Name:        cleanName,
			ExtraTLVs:   pxTLV,
			ChunkOffset: 0,
			PayloadSize: 0,
			Hash:        hash,
		}
		w.extents = append(w.extents, ext)
		w.currentItems++
		return nil
	}

	startChunkIdx := chunkList[0]
	chunkCount := uint16(len(chunkList))

	ext := catalog.Extent{
		Namespace:   spec.NamespaceVFS,
		ChunkIndex:  startChunkIdx,
		ChunkCount:  chunkCount,
		Timestamp:   modTime.UnixNano(),
		Name:        cleanName,
		ExtraTLVs:   pxTLV,
		ChunkOffset: 0,
		PayloadSize: uint64(writtenTotal),
		Hash:        hash,
	}

	// Check if chunks are contiguous
	isContiguous := true
	for i := 1; i < len(chunkList); i++ {
		if chunkList[i] != chunkList[i-1]+1 {
			isContiguous = false
			break
		}
	}

	if !isContiguous {
		ext.SetChunkList(chunkList)
	}

	w.extents = append(w.extents, ext)
	if w.targetVault != "" && w.vaultMap != nil {
		if vs, ok := w.vaultMap[w.targetVault]; ok {
			vs.extents = append(vs.extents, ext)
		}
	}
	if len(w.payloadDedup) < 100000 && writtenTotal > 0 {
		w.payloadDedup[hash] = ext
	}
	w.currentItems++
	return nil
}

// DeduplicationStats returns the count of reused chunks and total uncompressed bytes saved by CAS deduplication.
func (w *Writer) DeduplicationStats() (reusedChunks uint32, savedBytes uint64) {
	return w.dedupReusedChunks, w.dedupSavedBytes
}

// AddExtent registers an arbitrary domain extent (e.g. KV, Tensor, Block, Custom).
func (w *Writer) AddExtent(ext catalog.Extent, payload []byte) error {
	if err := w.ensureHeader(); err != nil {
		return err
	}

	if len(w.currentRaw) > 0 && uint32(len(w.currentRaw)+len(payload)) > w.targetChunkSize {
		if err := w.flushChunk(); err != nil {
			return err
		}
	}

	ext.ChunkIndex = w.currentChunkIndex
	ext.ChunkOffset = uint64(len(w.currentRaw))
	ext.ChunkCount = 1
	ext.PayloadSize = uint64(len(payload))

	if len(payload) > 0 {
		h := sha256.Sum256(payload)
		ext.Hash = h
	}

	w.currentRaw = append(w.currentRaw, payload...)
	w.extents = append(w.extents, ext)
	if w.targetVault != "" && w.vaultMap != nil {
		if vs, ok := w.vaultMap[w.targetVault]; ok {
			vs.extents = append(vs.extents, ext)
		}
	}
	w.currentItems++
	return nil
}

// AddExtentWithPipeline registers an extent processed via a dedicated custom filter and codec pipeline.
func (w *Writer) AddExtentWithPipeline(ext catalog.Extent, payload []byte, p *filter.Pipeline) error {
	if err := w.ensureHeader(); err != nil {
		return err
	}

	// Flush any pending data in current chunk to keep this pipeline isolated
	if len(w.currentRaw) > 0 {
		if err := w.flushChunk(); err != nil {
			return err
		}
	}

	savedPipeline := w.chunkPipeline
	savedCodec := w.currentChunkCodecID
	savedLevel := w.currentChunkCodecLevel
	savedFlags := w.currentChunkFlags

	if p != nil {
		w.chunkPipeline = p
		w.currentChunkCodecID = p.CodecID
		w.currentChunkCodecLevel = p.CodecLevel
		w.currentChunkFlags = spec.ChunkFlagSolid | spec.ChunkFlagPipeline
	}

	if err := w.AddExtent(ext, payload); err != nil {
		w.chunkPipeline = savedPipeline
		w.currentChunkCodecID = savedCodec
		w.currentChunkCodecLevel = savedLevel
		w.currentChunkFlags = savedFlags
		return err
	}

	if err := w.flushChunk(); err != nil {
		w.chunkPipeline = savedPipeline
		w.currentChunkCodecID = savedCodec
		w.currentChunkCodecLevel = savedLevel
		w.currentChunkFlags = savedFlags
		return err
	}

	w.chunkPipeline = savedPipeline
	w.currentChunkCodecID = savedCodec
	w.currentChunkCodecLevel = savedLevel
	w.currentChunkFlags = savedFlags
	return nil
}

// Close finalizes all pending chunks, writes Central Directory records (per-vault if multi-vault),
// and appends the 32-byte Archive Trailer.
func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	if err := w.ensureHeader(); err != nil {
		return err
	}

	// Flush any pending uncompressed data in current chunk
	if len(w.currentRaw) > 0 {
		if err := w.flushChunk(); err != nil {
			return err
		}
	}

	// Drain all in-flight compression tasks
	for len(w.inFlight) > 0 {
		if err := w.drainOne(); err != nil {
			return err
		}
	}

	var primaryDirOffset uint64
	flagsCopy := uint32(spec.FlagCentralDir | spec.FlagAlignedBlocks)
	if w.useCDC {
		flagsCopy |= uint32(spec.FlagCDC)
	}
	if w.useProllyTree {
		flagsCopy |= uint32(spec.FlagProllyTree)
	}
	if len(w.signerKey) > 0 {
		flagsCopy |= uint32(spec.FlagDigitalSig)
	}

	if w.encr != nil {
		flagsCopy |= uint32(spec.FlagEncrypted)

		if len(w.vaults) > 1 {
			for i, vs := range w.vaults {
				dirOffset := uint64(w.totalWritten)
				if i == 0 {
					primaryDirOffset = dirOffset
				}

				idx := &catalog.Index{Extents: vs.extents}
				idx.Sort()

				var dirBuf bytes.Buffer
				dirHasher := sha256.New()
				dirWriter := io.MultiWriter(&dirBuf, dirHasher)

				ctoc := &catalog.CTOC{Entries: w.ctocEntries}
				if _, err := ctoc.Write(dirWriter); err != nil {
					return err
				}
				if _, err := idx.Write(dirWriter); err != nil {
					return err
				}

				if len(w.signerKey) > 0 {
					dirDigest := dirHasher.Sum(nil)
					sig := ed25519.Sign(w.signerKey, dirDigest)
					pubKey := w.signerKey.Public().(ed25519.PublicKey)
					dsig := &catalog.DSIG{
						AlgorithmID: spec.SigAlgoEd25519,
						KeyID:       w.signerKeyID,
						PublicKey:   pubKey,
						Signature:   sig,
					}
					if _, err := dsig.Write(dirWriter); err != nil {
						return err
					}
				}

				dirNonce := crypto.DeriveDirectoryNonce(w.encr.MasterIV[:])
				encDir := vs.dirAEAD.Seal(nil, dirNonce, dirBuf.Bytes(), w.hdrBytes[:])
				if _, err := w.w.Write(encDir); err != nil {
					return err
				}
				w.totalWritten += int64(len(encDir))

				vs.descriptor.DirOffset = dirOffset
				vs.descriptor.DirSize = uint64(len(encDir))
				_ = w.encr.Slots[i].Wrap(vs.password, w.cipherID, w.encr.MasterIV, vs.descriptor)
			}

			w.rewriteENCR()
		} else {
			// Single vault
			centralDirOffset := uint64(w.totalWritten)
			primaryDirOffset = centralDirOffset

			indx := &catalog.Index{Extents: w.extents}
			indx.Sort()

			var dirBuf bytes.Buffer
			dirHasher := sha256.New()
			dirWriter := io.MultiWriter(&dirBuf, dirHasher)

			ctoc := &catalog.CTOC{Entries: w.ctocEntries}
			if _, err := ctoc.Write(dirWriter); err != nil {
				return err
			}
			if _, err := indx.Write(dirWriter); err != nil {
				return err
			}

			if len(w.signerKey) > 0 {
				dirDigest := dirHasher.Sum(nil)
				sig := ed25519.Sign(w.signerKey, dirDigest)
				pubKey := w.signerKey.Public().(ed25519.PublicKey)
				dsig := &catalog.DSIG{
					AlgorithmID: spec.SigAlgoEd25519,
					KeyID:       w.signerKeyID,
					PublicKey:   pubKey,
					Signature:   sig,
				}
				if _, err := dsig.Write(dirWriter); err != nil {
					return err
				}
			}

			dirNonce := crypto.DeriveDirectoryNonce(w.encr.MasterIV[:])
			encDir := w.dirAEAD.Seal(nil, dirNonce, dirBuf.Bytes(), w.hdrBytes[:])
			if _, err := w.w.Write(encDir); err != nil {
				return err
			}
			w.totalWritten += int64(len(encDir))

			if len(w.vaults) == 1 {
				vs := w.vaults[0]
				vs.descriptor.DirOffset = centralDirOffset
				vs.descriptor.DirSize = uint64(len(encDir))
				_ = w.encr.Slots[0].Wrap(vs.password, w.cipherID, w.encr.MasterIV, vs.descriptor)

				w.rewriteENCR()
			}
		}
	} else {
		// Unencrypted
		centralDirOffset := uint64(w.totalWritten)
		primaryDirOffset = centralDirOffset

		indx := &catalog.Index{Extents: w.extents}
		indx.Sort()

		var treeWritten int64
		if w.useProllyTree {
			tree, err := prolly.BuildTree(indx.Extents, w.treeStore)
			if err != nil {
				return fmt.Errorf("build prolly tree: %w", err)
			}

			var totalPayload uint64
			for _, ext := range indx.Extents {
				totalPayload += ext.PayloadSize
			}

			snapID := fmt.Sprintf("snap-%x", tree.RootHash[:6])
			snapTime := time.Now().UnixNano()
			if w.reproducible {
				snapTime = w.sourceDateEpoch
			}
			msg := w.snapshotMsg
			if msg == "" {
				msg = "Archive snapshot"
			}
			w.snapshotTable.Add(prolly.Snapshot{
				ID:         snapID,
				ParentID:   w.snapshotParent,
				Timestamp:  snapTime,
				Message:    msg,
				RootHash:   tree.RootHash,
				FileCount:  uint64(len(indx.Extents)),
				TotalBytes: totalPayload,
			})
		}

		dirHasher := sha256.New()
		dirWriter := io.MultiWriter(w.w, dirHasher)

		ctoc := &catalog.CTOC{Entries: w.ctocEntries}
		ctocWritten, err := ctoc.Write(dirWriter)
		if err != nil {
			return err
		}
		indxWritten, err := indx.Write(dirWriter)
		if err != nil {
			return err
		}

		if w.useProllyTree {
			snpsWritten, err := w.snapshotTable.Write(dirWriter)
			if err != nil {
				return fmt.Errorf("write snapshot table: %w", err)
			}
			nodesWritten, err := w.writeTreeNodes(dirWriter)
			if err != nil {
				return fmt.Errorf("write tree nodes: %w", err)
			}
			treeWritten = snpsWritten + nodesWritten
		}

		var dsigWritten int64
		if len(w.signerKey) > 0 {
			dirDigest := dirHasher.Sum(nil)
			sig := ed25519.Sign(w.signerKey, dirDigest)
			pubKey := w.signerKey.Public().(ed25519.PublicKey)
			dsig := &catalog.DSIG{
				AlgorithmID: spec.SigAlgoEd25519,
				KeyID:       w.signerKeyID,
				PublicKey:   pubKey,
				Signature:   sig,
			}
			dsigWritten, err = dsig.Write(dirWriter)
			if err != nil {
				return err
			}
		}
		w.totalWritten += ctocWritten + indxWritten + treeWritten + dsigWritten
	}

	// Write 32-byte Archive Trailer at exact EOF
	var trailer [spec.TrailerSize]byte
	binary.LittleEndian.PutUint64(trailer[0:8], primaryDirOffset)
	binary.LittleEndian.PutUint64(trailer[8:16], 0) // RecoveryOffset (0 = absent)
	binary.LittleEndian.PutUint64(trailer[16:24], 0) // ArchiveStartOffset (0 = no stub)
	binary.LittleEndian.PutUint32(trailer[24:28], flagsCopy)
	copy(trailer[28:32], spec.MagicTrailer[:])

	if _, err := w.w.Write(trailer[:]); err != nil {
		return err
	}
	w.totalWritten += spec.TrailerSize

	return nil
}

// TotalWritten returns the total number of bytes serialized to the underlying writer.
func (w *Writer) TotalWritten() int64 {
	return w.totalWritten
}


func (w *Writer) rewriteENCR() {
	if ws, ok := w.w.(io.WriteSeeker); ok {
		cur := w.totalWritten
		if _, err := ws.Seek(spec.HeaderSize, io.SeekStart); err == nil {
			_, _ = w.encr.Write(ws)
			_, _ = ws.Seek(cur, io.SeekStart)
		}
	} else if wa, ok := w.w.(io.WriterAt); ok {
		var encrBuf bytes.Buffer
		_, _ = w.encr.Write(&encrBuf)
		_, _ = wa.WriteAt(encrBuf.Bytes(), spec.HeaderSize)
	} else if bb, ok := w.w.(*bytes.Buffer); ok {
		var encrBuf bytes.Buffer
		_, _ = w.encr.Write(&encrBuf)
		data := bb.Bytes()
		if len(data) >= spec.HeaderSize+encrBuf.Len() {
			copy(data[spec.HeaderSize:], encrBuf.Bytes())
		}
	}
}

func (w *Writer) writeTreeNodes(out io.Writer) (int64, error) {
	return writeTreeNodes(out, w.treeStore)
}

func writeTreeNodes(out io.Writer, store *prolly.MemoryNodeStore) (int64, error) {
	var total int64
	var buf bytes.Buffer

	nodes := store.Nodes()
	var hdr [8]byte
	copy(hdr[0:4], spec.MagicTREE[:])
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(nodes)))
	buf.Write(hdr[:])

	for _, node := range nodes {
		var szBuf [4]byte
		binary.LittleEndian.PutUint32(szBuf[:], uint32(len(node.Serialized)))
		buf.Write(szBuf[:])
		buf.Write(node.Serialized)
	}

	n, err := out.Write(buf.Bytes())
	total += int64(n)
	return total, err
}


