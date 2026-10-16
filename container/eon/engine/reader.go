// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/chunk"
	"github.com/lemon4ksan/seal/container/eon/crypto"
	"github.com/lemon4ksan/seal/container/eon/objstore"
	"github.com/lemon4ksan/seal/container/eon/prolly"
	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/seal/crypto/shamir"
)

// ReaderOption configures container reader behavior.
type ReaderOption func(*readerOptions)

type readerOptions struct {
	passwords    []string
	kmsClient    crypto.KMSClient
	shamirShares []string
	chunkStore   objstore.Store
	chunkPrefix  string
	cacheDir     string
}

// WithReaderPassword configures the passphrase to unlock an encrypted container.
func WithReaderPassword(password string) ReaderOption {
	return func(o *readerOptions) {
		if password != "" {
			o.passwords = append(o.passwords, password)
		}
	}
}

// WithReaderPasswords configures multiple passphrases to unlock multiple vaults.
func WithReaderPasswords(passwords ...string) ReaderOption {
	return func(o *readerOptions) {
		for _, p := range passwords {
			if p != "" {
				o.passwords = append(o.passwords, p)
			}
		}
	}
}

// WithKMS configures a KMS or Vault client to unlock envelope encrypted containers.
func WithKMS(client crypto.KMSClient) ReaderOption {
	return func(o *readerOptions) {
		o.kmsClient = client
	}
}

// WithShamirShares provides M-of-N threshold secret shares to reconstruct the container master DEK.
func WithShamirShares(shares ...string) ReaderOption {
	return func(o *readerOptions) {
		for _, s := range shares {
			if s != "" {
				o.shamirShares = append(o.shamirShares, s)
			}
		}
	}
}

// WithChunkStore configures an object store backend and key prefix for detached chunk retrieval.
func WithChunkStore(store objstore.Store, prefix string) ReaderOption {
	return func(o *readerOptions) {
		o.chunkStore = store
		o.chunkPrefix = prefix
	}
}

// WithDiskCache configures a local directory to cache downloaded remote chunks.
func WithDiskCache(dir string) ReaderOption {
	return func(o *readerOptions) {
		o.cacheDir = dir
	}
}

// Reader provides random-access traversal and extraction for EON containers.
type Reader struct {
	rAt         io.ReaderAt
	size        int64
	blockShift  uint8
	blockSize   int64
	profile     uint8
	headerFlags uint16
	mmapData    []byte

	ctoc        *catalog.CTOC
	indx        *catalog.Index
	chunkHashes *catalog.ChunkHashTable
	dsig        *catalog.DSIG
	dirDigest   []byte

	prollyTree   *prolly.Tree
	treeStore    *prolly.MemoryNodeStore
	snapshots    *prolly.SnapshotTable
	activeSnapID string

	encr             *crypto.ENCR
	dek              []byte
	chunkAEAD        cipher.AEAD
	chunkAEADs       []cipher.AEAD
	dirAEAD          cipher.AEAD
	vaultDescriptors []*crypto.VaultDescriptor
	unlockedVaults   []string

	extentMap  map[string]catalog.Extent
	chunkCache map[uint32][]byte
	cacheMu    sync.Mutex

	chunkStore  objstore.Store
	chunkPrefix string
	cacheDir    string
}

// OpenFile opens an EON container from the local filesystem path.
// It automatically maps the file into virtual memory via mmap for zero-copy read performance.
func OpenFile(path string, opts ...ReaderOption) (*Reader, io.Closer, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return OpenURL(path, opts...)
	}

	if strings.HasPrefix(path, "s3://") || strings.HasPrefix(path, "r2://") {
		store, prefix, err := objstore.ParseURI(path)
		if err != nil {
			return nil, nil, err
		}
		r, err := OpenStore(store, prefix, opts...)
		if err != nil {
			return nil, nil, err
		}
		return r, io.NopCloser(nil), nil
	}

	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		store := objstore.NewFSStore(path)
		r, err := OpenStore(store, "", opts...)
		if err != nil {
			return nil, nil, err
		}
		return r, io.NopCloser(nil), nil
	}

	mm, err := OpenMmap(path)
	if err == nil {
		r, err := OpenReader(mm, int64(len(mm.Bytes())), opts...)
		if err == nil {
			return r, mm, nil
		}
		_ = mm.Close()
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	r, err := OpenReader(f, stat.Size(), opts...)
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return r, f, nil
}

// OpenReader opens an EON container backed by a random-access source.
func OpenReader(rAt io.ReaderAt, size int64, opts ...ReaderOption) (*Reader, error) {
	var ro readerOptions
	for _, opt := range opts {
		opt(&ro)
	}

	if rAt == nil || size < spec.HeaderSize+spec.TrailerSize {
		return nil, spec.ErrInvalidTrailer
	}

	// 1. Read fixed 32-byte trailer at [size - 32]
	var trailer [spec.TrailerSize]byte
	if _, err := rAt.ReadAt(trailer[:], size-spec.TrailerSize); err != nil {
		return nil, fmt.Errorf("read trailer: %w", err)
	}
	if binary.LittleEndian.Uint32(trailer[28:32]) != spec.MagicTrailerLE {
		return nil, spec.ErrInvalidTrailer
	}

	centralDirOffset := binary.LittleEndian.Uint64(trailer[0:8])
	archiveStartOffset := binary.LittleEndian.Uint64(trailer[16:24])
	flagsCopy := binary.LittleEndian.Uint32(trailer[24:28])

	// 2. Read 32-byte Archive Header
	var hdr [spec.HeaderSize]byte
	if _, err := rAt.ReadAt(hdr[:], int64(archiveStartOffset)); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != spec.MagicHeaderLE {
		return nil, spec.ErrInvalidHeader
	}

	profile := hdr[6]
	blockShift := hdr[9]
	if blockShift == 0 {
		blockShift = spec.DefaultBlockSizeShift
	}
	blockSize := int64(1 << blockShift)
	headerFlags := binary.LittleEndian.Uint16(hdr[10:12])
	effectiveFlags := headerFlags | uint16(flagsCopy)

	var (
		ctoc             *catalog.CTOC
		indx             *catalog.Index
		chunkHashes      *catalog.ChunkHashTable
		prollyTree       *prolly.Tree
		treeStore        *prolly.MemoryNodeStore
		snapshots        *prolly.SnapshotTable
		dsig             *catalog.DSIG
		dirDigest        []byte
		encr             *crypto.ENCR
		dek              []byte
		chunkAEAD        cipher.AEAD
		chunkAEADs       []cipher.AEAD
		dirAEAD          cipher.AEAD
		vaultDescriptors []*crypto.VaultDescriptor
		unlockedVaults   []string
	)

	isEncrypted := (flagsCopy & uint32(spec.FlagEncrypted)) != 0

	if isEncrypted {
		if len(ro.passwords) == 0 && ro.kmsClient == nil && len(ro.shamirShares) == 0 {
			return nil, spec.ErrPasswordRequired
		}

		encrOffset := int64(archiveStartOffset) + spec.HeaderSize
		encrReader := io.NewSectionReader(rAt, encrOffset, size-spec.TrailerSize-encrOffset)
		var err error
		encr, err = crypto.ReadENCR(encrReader)
		if err != nil {
			return nil, fmt.Errorf("read encr header: %w", err)
		}

		var unlockedVDs []*crypto.VaultDescriptor

		if ro.kmsClient != nil {
			vd, kErr := encr.UnlockWithKMS(context.Background(), ro.kmsClient)
			if kErr == nil {
				unlockedVDs = append(unlockedVDs, vd)
			}
		}

		if len(ro.passwords) > 0 {
			vds, pErr := encr.UnlockAll(ro.passwords...)
			if pErr == nil {
				unlockedVDs = append(unlockedVDs, vds...)
			}
		}

		if len(ro.shamirShares) > 0 {
			parsedShares := make([]shamir.Share, 0, len(ro.shamirShares))
			for _, s := range ro.shamirShares {
				sh, _, _, sErr := shamir.ParseShare(s)
				if sErr == nil {
					parsedShares = append(parsedShares, sh)
				}
			}
			recoveredDEK, cErr := shamir.Combine(parsedShares)
			if cErr == nil && len(recoveredDEK) == 32 {
				var dekArr [32]byte
				copy(dekArr[:], recoveredDEK)
				vDirSize := uint64(0)
				if (size - spec.TrailerSize) > int64(centralDirOffset) {
					vDirSize = uint64((size - spec.TrailerSize) - int64(centralDirOffset))
				}
				vd := &crypto.VaultDescriptor{
					VaultID:    0,
					VaultFlags: 0,
					VaultName:  "default",
					DirOffset:  centralDirOffset,
					DirSize:    vDirSize,
					DEK:        dekArr,
				}
				unlockedVDs = append(unlockedVDs, vd)
			}
		}

		if len(unlockedVDs) == 0 {
			return nil, spec.ErrInvalidPassword
		}
		vaultDescriptors = unlockedVDs

		var mergedExtents []catalog.Extent
		extentSeen := make(map[string]bool)
		var lastCTOC *catalog.CTOC

		for _, vd := range unlockedVDs {
			unlockedVaults = append(unlockedVaults, vd.VaultName)

			vDirOffset := int64(vd.DirOffset)
			vDirSize := int64(vd.DirSize)
			if vDirOffset == 0 {
				vDirOffset = int64(centralDirOffset)
				vDirSize = (size - spec.TrailerSize) - vDirOffset
			}

			if vDirSize <= 0 {
				continue
			}

			encDirBytes := make([]byte, vDirSize)
			if _, err := rAt.ReadAt(encDirBytes, vDirOffset); err != nil {
				return nil, fmt.Errorf("read encrypted central directory (%s): %w", vd.VaultName, err)
			}

			dirKey := crypto.DeriveSubkey(vd.DEK[:], "eon-central-directory-key", 32)
			if vd.VaultName != "" && vd.VaultName != "default" {
				dirKey = crypto.DeriveSubkey(vd.DEK[:], "eon-central-directory-key-"+vd.VaultName, 32)
			}

			dirAEAD, err = crypto.NewAEAD(encr.CipherID, dirKey)
			if err != nil {
				return nil, fmt.Errorf("init dir aead (%s): %w", vd.VaultName, err)
			}

			dirNonce := crypto.DeriveDirectoryNonce(encr.MasterIV[:])
			decDirBytes, err := dirAEAD.Open(nil, dirNonce, encDirBytes, hdr[:])
			if err != nil {
				// Fallback to default dirKey
				fbKey := crypto.DeriveSubkey(vd.DEK[:], "eon-central-directory-key", 32)
				if fbAEAD, fbErr := crypto.NewAEAD(encr.CipherID, fbKey); fbErr == nil {
					decDirBytes, err = fbAEAD.Open(nil, dirNonce, encDirBytes, hdr[:])
				}
				if err != nil {
					return nil, fmt.Errorf("decrypt central directory (%s): %w", vd.VaultName, err)
				}
			}

			dirReader := bytes.NewReader(decDirBytes)
			curCTOC, err := catalog.ReadCTOC(dirReader)
			if err != nil {
				return nil, fmt.Errorf("read ctoc (%s): %w", vd.VaultName, err)
			}
			lastCTOC = curCTOC

			curINDX, indxByteLen, err := catalog.ReadIndex(dirReader)
			if err != nil {
				return nil, fmt.Errorf("read indx (%s): %w", vd.VaultName, err)
			}

			for _, ext := range curINDX.Extents {
				key := fmt.Sprintf("%d:%s", ext.Namespace, ext.Name)
				if !extentSeen[key] {
					extentSeen[key] = true
					mergedExtents = append(mergedExtents, ext)
				}
			}

			// Add Chunk AEAD for this vault
			cKey := crypto.DeriveSubkey(vd.DEK[:], "eon-chunk-payload-key", 32)
			cAe, cErr := crypto.NewAEAD(encr.CipherID, cKey)
			if cErr == nil {
				chunkAEADs = append(chunkAEADs, cAe)
			}

			if (flagsCopy&uint32(spec.FlagDigitalSig)) != 0 && dsig == nil {
				if d, dErr := catalog.ReadDSIG(dirReader); dErr == nil {
					dsig = d
					hasher := sha256.New()
					hasher.Write(
						decDirBytes[:spec.CTOCHeaderSize+int64(curCTOC.TotalChunks)*spec.CTOCEntrySize+indxByteLen],
					)
					dirDigest = hasher.Sum(nil)
				}
			}
		}

		if len(chunkAEADs) > 0 {
			chunkAEAD = chunkAEADs[0]
		}
		dek = unlockedVDs[0].DEK[:]
		ctoc = lastCTOC

		indx = &catalog.Index{Extents: mergedExtents}
		indx.Sort()
	} else {
		// Unencrypted container
		ctocReader := io.NewSectionReader(rAt, int64(centralDirOffset), size-int64(centralDirOffset))
		var err error
		ctoc, err = catalog.ReadCTOC(ctocReader)
		if err != nil {
			return nil, fmt.Errorf("read ctoc: %w", err)
		}

		indxOffset := int64(centralDirOffset) + spec.CTOCHeaderSize + int64(ctoc.TotalChunks)*spec.CTOCEntrySize
		indxReader := io.NewSectionReader(rAt, indxOffset, size-indxOffset)
		var indxByteLen int64
		indx, indxByteLen, err = catalog.ReadIndex(indxReader)
		if err != nil {
			return nil, fmt.Errorf("read indx: %w", err)
		}

		curOffset := indxOffset + indxByteLen

		if (flagsCopy & uint32(spec.FlagProllyTree)) != 0 {
			treeStore = prolly.NewMemoryNodeStore()
			treeReader := io.NewSectionReader(rAt, curOffset, size-curOffset)
			var err error
			snapshots, err = prolly.ReadSnapshotTable(treeReader)
			if err == nil && snapshots != nil && len(snapshots.Snapshots) > 0 {
				_, _ = prolly.ReadTreeNodes(treeReader, treeStore)
				latest := snapshots.Latest()
				if latest != nil {
					prollyTree = &prolly.Tree{
						RootHash:  latest.RootHash,
						Store:     treeStore,
						ItemCount: latest.FileCount,
					}
				}
				readPos, _ := treeReader.Seek(0, io.SeekCurrent)
				curOffset += readPos
			}
		}

		var peek [4]byte
		if _, err := rAt.ReadAt(
			peek[:],
			curOffset,
		); err == nil &&
			binary.LittleEndian.Uint32(peek[:]) == spec.MagicCHSHLE {
			chshReader := io.NewSectionReader(rAt, curOffset, size-curOffset)
			if chsh, err := catalog.ReadChunkHashTable(chshReader); err == nil && chsh != nil {
				chunkHashes = chsh
				readPos, _ := chshReader.Seek(0, io.SeekCurrent)
				curOffset += readPos
			}
		}

		if (effectiveFlags & spec.FlagDigitalSig) != 0 {
			dsigOffset := curOffset
			dsigReader := io.NewSectionReader(rAt, dsigOffset, size-dsigOffset)
			dsig, err = catalog.ReadDSIG(dsigReader)
			if err != nil {
				return nil, fmt.Errorf("read dsig: %w", err)
			}

			dirTotalLen := curOffset - int64(centralDirOffset)
			dirBytes := make([]byte, dirTotalLen)
			if _, err := rAt.ReadAt(dirBytes, int64(centralDirOffset)); err != nil {
				return nil, fmt.Errorf("read directory for signature check: %w", err)
			}
			hasher := sha256.New()
			hasher.Write(dirBytes)
			dirDigest = hasher.Sum(nil)
		}
	}

	var mmapData []byte
	if mf, ok := rAt.(MmapFile); ok {
		mmapData = mf.Bytes()
	}

	r := &Reader{
		rAt:              rAt,
		size:             size,
		blockShift:       blockShift,
		blockSize:        blockSize,
		profile:          profile,
		headerFlags:      effectiveFlags,
		mmapData:         mmapData,
		ctoc:             ctoc,
		indx:             indx,
		chunkHashes:      chunkHashes,
		prollyTree:       prollyTree,
		treeStore:        treeStore,
		snapshots:        snapshots,
		dsig:             dsig,
		dirDigest:        dirDigest,
		encr:             encr,
		dek:              dek,
		chunkAEAD:        chunkAEAD,
		chunkAEADs:       chunkAEADs,
		dirAEAD:          dirAEAD,
		vaultDescriptors: vaultDescriptors,
		unlockedVaults:   unlockedVaults,
		extentMap:        make(map[string]catalog.Extent, len(indx.Extents)),
		chunkCache:       make(map[uint32][]byte),
		chunkStore:       ro.chunkStore,
		chunkPrefix:      ro.chunkPrefix,
		cacheDir:         ro.cacheDir,
	}

	for _, ext := range indx.Extents {
		r.extentMap[ext.Name] = ext
	}

	return r, nil
}

// UnlockedVaults returns the names of all vaults successfully unlocked in this session.
func (r *Reader) UnlockedVaults() []string {
	return r.unlockedVaults
}

// ActiveVault returns the name of the unlocked vault, or a "+"-joined list if multiple vaults were merged.
func (r *Reader) ActiveVault() string {
	if len(r.unlockedVaults) == 0 {
		return ""
	}
	if len(r.unlockedVaults) == 1 {
		return r.unlockedVaults[0]
	}
	return strings.Join(r.unlockedVaults, "+")
}

// VaultDescriptors returns the unlocked vault descriptors.
func (r *Reader) VaultDescriptors() []*crypto.VaultDescriptor {
	return r.vaultDescriptors
}

// HasSignature returns true if the container includes a valid DSIG digital signature record.
func (r *Reader) HasSignature() bool {
	return r.dsig != nil
}

// DSIG returns the digital signature record of the container, or nil if not signed.
func (r *Reader) DSIG() *catalog.DSIG {
	return r.dsig
}

// VerifySignature cryptographically validates the Central Directory signature against trustedPubKey.
func (r *Reader) VerifySignature(trustedPubKey ed25519.PublicKey) error {
	if r.dsig == nil {
		return spec.ErrSignatureMissing
	}

	pubKey := trustedPubKey
	if len(pubKey) == 0 {
		pubKey = r.dsig.PublicKey
	}

	if len(pubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: invalid public key length (%d bytes)", spec.ErrInvalidSignature, len(pubKey))
	}

	if !bytes.Equal(pubKey, r.dsig.PublicKey) {
		return fmt.Errorf("%w: public key mismatch", spec.ErrInvalidSignature)
	}

	switch r.dsig.AlgorithmID {
	case spec.SigAlgoEd25519:
		if !ed25519.Verify(pubKey, r.dirDigest, r.dsig.Signature) {
			return spec.ErrInvalidSignature
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported signature algorithm ID %d", spec.ErrInvalidSignature, r.dsig.AlgorithmID)
	}
}

// IsEncrypted returns true if the container is protected with authenticated encryption.
func (r *Reader) IsEncrypted() bool {
	return r.encr != nil
}

// CipherID returns the symmetric AEAD algorithm ID if encrypted, or 0.
func (r *Reader) CipherID() uint8 {
	if r.encr != nil {
		return r.encr.CipherID
	}
	return 0
}

// ENCR returns the encryption header and key slots, or nil if unencrypted.
func (r *Reader) ENCR() *crypto.ENCR {
	return r.encr
}

// DEK returns a copy of the decrypted 32-byte master Data Encryption Key, or nil if unencrypted or locked.
func (r *Reader) DEK() []byte {
	if len(r.dek) == 0 {
		return nil
	}
	out := make([]byte, len(r.dek))
	copy(out, r.dek)
	return out
}

// FindExtent looks up an extent by namespace and name using Prolly Tree or O(log N) binary search over the sorted catalog.
func (r *Reader) FindExtent(namespace uint16, name string) (catalog.Extent, bool) {
	if r.prollyTree != nil {
		ext, ok, err := r.prollyTree.Lookup(namespace, name)
		if err == nil && ok {
			return ext, true
		}
	}
	if r.indx != nil {
		return r.indx.BinarySearch(namespace, name)
	}
	ext, ok := r.extentMap[name]
	return ext, ok
}

// HasProllyTree returns true if the container has a hierarchical Prolly Tree index.
func (r *Reader) HasProllyTree() bool {
	return r.prollyTree != nil
}

// TreeStore returns the in-memory Prolly Tree node store.
func (r *Reader) TreeStore() *prolly.MemoryNodeStore {
	return r.treeStore
}

// Snapshots returns all point-in-time snapshots recorded in the container.
func (r *Reader) Snapshots() []prolly.Snapshot {
	if r.snapshots == nil {
		return nil
	}
	return r.snapshots.Snapshots
}

// LatestSnapshot returns the most recent snapshot or nil.
func (r *Reader) LatestSnapshot() *prolly.Snapshot {
	if r.snapshots == nil {
		return nil
	}
	return r.snapshots.Latest()
}

// Diff compares two snapshots by ID prefix in O(Δ) time.
// If snapID2 is empty, it defaults to the latest snapshot.
// If snapID1 is also empty, it diffs the latest snapshot against its parent snapshot.
func (r *Reader) Diff(snapID1, snapID2 string) (*prolly.DiffResult, error) {
	if r.snapshots == nil || r.treeStore == nil {
		return nil, errors.New("aeon: no snapshot table available in container")
	}
	var s1, s2 *prolly.Snapshot
	if snapID2 == "" {
		s2 = r.LatestSnapshot()
		if s2 == nil {
			return nil, errors.New("aeon: no snapshots available")
		}
		if snapID1 == "" {
			if s2.ParentID == "" {
				return nil, errors.New("aeon: latest snapshot has no parent snapshot to diff against")
			}
			snapID1 = s2.ParentID
		}
	} else {
		var ok bool
		s2, ok = r.snapshots.Find(snapID2)
		if !ok {
			return nil, fmt.Errorf("aeon: snapshot %q not found", snapID2)
		}
	}

	var ok bool
	s1, ok = r.snapshots.Find(snapID1)
	if !ok {
		return nil, fmt.Errorf("aeon: snapshot %q not found", snapID1)
	}

	tree1 := &prolly.Tree{RootHash: s1.RootHash, Store: r.treeStore, ItemCount: s1.FileCount}
	tree2 := &prolly.Tree{RootHash: s2.RootHash, Store: r.treeStore, ItemCount: s2.FileCount}

	return prolly.Diff(tree1, tree2)
}

// DiffAcross compares snapshot snapID1 from r1 with snapshot snapID2 from r2 in O(Δ) time.
// If either snapshot ID is empty, the latest snapshot from that reader is used.
func DiffAcross(r1 *Reader, snapID1 string, r2 *Reader, snapID2 string) (*prolly.DiffResult, error) {
	if r1 == nil || r2 == nil {
		return nil, errors.New("aeon: readers cannot be nil")
	}
	if r1.snapshots == nil || r1.treeStore == nil {
		return nil, errors.New("aeon: reader 1 has no snapshot table or tree store")
	}
	if r2.snapshots == nil || r2.treeStore == nil {
		return nil, errors.New("aeon: reader 2 has no snapshot table or tree store")
	}

	var s1 *prolly.Snapshot
	if snapID1 == "" {
		s1 = r1.LatestSnapshot()
	} else {
		var ok bool
		s1, ok = r1.snapshots.Find(snapID1)
		if !ok {
			return nil, fmt.Errorf("aeon: snapshot %q not found in first container", snapID1)
		}
	}
	if s1 == nil {
		return nil, errors.New("aeon: first container has no snapshots")
	}

	var s2 *prolly.Snapshot
	if snapID2 == "" {
		s2 = r2.LatestSnapshot()
	} else {
		var ok bool
		s2, ok = r2.snapshots.Find(snapID2)
		if !ok {
			return nil, fmt.Errorf("aeon: snapshot %q not found in second container", snapID2)
		}
	}
	if s2 == nil {
		return nil, errors.New("aeon: second container has no snapshots")
	}

	combinedStore := prolly.NewMemoryNodeStore()
	for _, n := range r1.treeStore.Nodes() {
		_ = combinedStore.PutNode(n)
	}
	for _, n := range r2.treeStore.Nodes() {
		_ = combinedStore.PutNode(n)
	}

	tree1 := &prolly.Tree{RootHash: s1.RootHash, Store: combinedStore, ItemCount: s1.FileCount}
	tree2 := &prolly.Tree{RootHash: s2.RootHash, Store: combinedStore, ItemCount: s2.FileCount}

	return prolly.Diff(tree1, tree2)
}

// OpenSnapshot returns a virtual reader view pinned to the specified snapshot.
func (r *Reader) OpenSnapshot(snapshotID string) (*Reader, error) {
	if r.snapshots == nil || r.treeStore == nil {
		return nil, errors.New("aeon: no snapshot table available in container")
	}
	s, ok := r.snapshots.Find(snapshotID)
	if !ok {
		return nil, fmt.Errorf("aeon: snapshot %q not found", snapshotID)
	}

	tree := &prolly.Tree{RootHash: s.RootHash, Store: r.treeStore, ItemCount: s.FileCount}
	extents, err := tree.CollectExtents()
	if err != nil {
		return nil, fmt.Errorf("aeon: collect snapshot extents: %w", err)
	}

	snapIdx := &catalog.Index{Extents: extents}
	clone := &Reader{
		rAt:              r.rAt,
		size:             r.size,
		blockShift:       r.blockShift,
		blockSize:        r.blockSize,
		profile:          r.profile,
		headerFlags:      r.headerFlags,
		mmapData:         r.mmapData,
		ctoc:             r.ctoc,
		indx:             snapIdx,
		chunkHashes:      r.chunkHashes,
		dsig:             r.dsig,
		dirDigest:        r.dirDigest,
		prollyTree:       tree,
		treeStore:        r.treeStore,
		snapshots:        r.snapshots,
		activeSnapID:     s.ID,
		encr:             r.encr,
		dek:              r.dek,
		chunkAEAD:        r.chunkAEAD,
		chunkAEADs:       r.chunkAEADs,
		dirAEAD:          r.dirAEAD,
		vaultDescriptors: r.vaultDescriptors,
		unlockedVaults:   r.unlockedVaults,
		chunkCache:       make(map[uint32][]byte),
		chunkStore:       r.chunkStore,
		chunkPrefix:      r.chunkPrefix,
		cacheDir:         r.cacheDir,
		extentMap:        make(map[string]catalog.Extent, len(extents)),
	}
	for _, ext := range extents {
		clone.extentMap[ext.Name] = ext
	}

	return clone, nil
}

// Extents returns all raw universal extents recorded in the container.
func (r *Reader) Extents() []catalog.Extent {
	if r.indx == nil {
		return nil
	}
	return r.indx.Extents
}

// CTOC returns the Chunk Table of Contents.
func (r *Reader) CTOC() *catalog.CTOC {
	return r.ctoc
}

func (r *Reader) chunkObjectKey(chunkIdx uint32) (string, error) {
	if r.chunkHashes != nil {
		if h, ok := r.chunkHashes.Get(chunkIdx); ok {
			return spec.ChunkObjectKey(r.chunkPrefix, h), nil
		}
	}
	key := fmt.Sprintf("chunks/%d.chnk", chunkIdx)
	if r.chunkPrefix != "" {
		key = strings.TrimSuffix(r.chunkPrefix, "/") + "/" + key
	}
	return key, nil
}

func (r *Reader) fetchRemoteChunk(chunkIdx uint32) ([]byte, error) {
	chunkKey, err := r.chunkObjectKey(chunkIdx)
	if err != nil {
		return nil, err
	}

	if r.cacheDir != "" {
		cachePath := filepath.Join(r.cacheDir, filepath.FromSlash(chunkKey))
		if data, err := os.ReadFile(cachePath); err == nil && len(data) >= spec.ChunkHeaderSize {
			return data, nil
		}
	}

	if r.chunkStore == nil {
		return nil, fmt.Errorf("aeon: chunk %d is detached but no object store configured", chunkIdx)
	}

	ctx := context.Background()
	rc, err := r.chunkStore.Get(ctx, chunkKey)
	if err != nil {
		return nil, fmt.Errorf("fetch remote chunk %s: %w", chunkKey, err)
	}
	rawChunk, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return nil, fmt.Errorf("read remote chunk %s: %w", chunkKey, err)
	}
	if len(rawChunk) < spec.ChunkHeaderSize {
		return nil, fmt.Errorf("remote chunk %s too small (%d bytes)", chunkKey, len(rawChunk))
	}

	if r.cacheDir != "" {
		cachePath := filepath.Join(r.cacheDir, filepath.FromSlash(chunkKey))
		dir := filepath.Dir(cachePath)
		if err := os.MkdirAll(dir, 0o755); err == nil {
			tmpFile := fmt.Sprintf("%s.tmp.%d", cachePath, time.Now().UnixNano())
			if err := os.WriteFile(tmpFile, rawChunk, 0o644); err == nil {
				_ = os.Rename(tmpFile, cachePath)
			}
		}
	}

	return rawChunk, nil
}

func (r *Reader) decompressChunk(chunkIdx uint32) ([]byte, error) {
	r.cacheMu.Lock()
	if raw, ok := r.chunkCache[chunkIdx]; ok {
		r.cacheMu.Unlock()
		return raw, nil
	}
	r.cacheMu.Unlock()

	if int(chunkIdx) >= len(r.ctoc.Entries) {
		return nil, errors.New("aeon: chunk index out of bounds")
	}

	entry := r.ctoc.Entries[chunkIdx]

	var c *chunk.Chunk
	var payload []byte

	if r.chunkStore != nil || (r.headerFlags&spec.FlagDetachedChunks) != 0 {
		rawChunk, err := r.fetchRemoteChunk(chunkIdx)
		if err != nil {
			return nil, err
		}
		c, err = chunk.ParseHeader(bytes.NewReader(rawChunk[:spec.ChunkHeaderSize]))
		if err != nil {
			return nil, fmt.Errorf("parse chunk header %d: %w", chunkIdx, err)
		}
		payload = rawChunk[spec.ChunkHeaderSize:]
	} else {
		diskOffset := int64(entry.BlockIndex) << r.blockShift

		// Read Chunk Header
		hdrReader := io.NewSectionReader(r.rAt, diskOffset, spec.ChunkHeaderSize)
		var err error
		c, err = chunk.ParseHeader(hdrReader)
		if err != nil {
			return nil, fmt.Errorf("parse chunk header %d: %w", chunkIdx, err)
		}

		// Read compressed payload (zero-copy if backed by memory-mapped file)
		if r.mmapData != nil {
			start := diskOffset + spec.ChunkHeaderSize
			end := start + int64(entry.CompressedSize)
			if start >= 0 && end <= int64(len(r.mmapData)) {
				payload = r.mmapData[start:end]
			}
		}
		if payload == nil {
			payload = make([]byte, entry.CompressedSize)
			if _, err := r.rAt.ReadAt(payload, diskOffset+spec.ChunkHeaderSize); err != nil {
				return nil, fmt.Errorf("read chunk payload %d: %w", chunkIdx, err)
			}
		}
	}

	if (c.Flags & spec.ChunkFlagEncrypted) != 0 {
		if r.chunkAEAD == nil && len(r.chunkAEADs) == 0 {
			return nil, spec.ErrPasswordRequired
		}
		var aad [21]byte
		copy(aad[0:4], spec.MagicChunk[:])
		binary.BigEndian.PutUint32(aad[4:8], chunkIdx)
		aad[8] = c.CodecID
		binary.BigEndian.PutUint64(aad[9:17], c.UncompressedSize)
		binary.BigEndian.PutUint32(aad[17:21], c.ItemCount)

		nonce := crypto.DeriveChunkNonce(r.encr.MasterIV[:], chunkIdx)
		var decrypted []byte
		var authErr error

		// Try all available chunkAEADs
		for _, aead := range r.chunkAEADs {
			decrypted, authErr = aead.Open(nil, nonce, payload, aad[:])
			if authErr == nil {
				break
			}
		}
		if authErr != nil && len(r.chunkAEADs) == 0 && r.chunkAEAD != nil {
			decrypted, authErr = r.chunkAEAD.Open(nil, nonce, payload, aad[:])
		}

		if authErr != nil {
			return nil, fmt.Errorf("chunk %d authentication failed: %w", chunkIdx, authErr)
		}
		c.Payload = decrypted
	} else {
		c.Payload = payload
	}

	raw, err := c.Decompress()
	if err != nil {
		return nil, fmt.Errorf("decompress chunk %d: %w", chunkIdx, err)
	}

	r.cacheMu.Lock()
	r.chunkCache[chunkIdx] = raw
	r.cacheMu.Unlock()

	return raw, nil
}

// Hydrate pre-downloads and caches chunks for the specified paths (or all chunks if paths is empty).
// This enables streaming hydration of remote archives into local disk cache.
func (r *Reader) Hydrate(paths ...string) error {
	var chunkIndices []uint32
	if len(paths) == 0 {
		chunkIndices = make([]uint32, len(r.ctoc.Entries))
		for i := range r.ctoc.Entries {
			chunkIndices[i] = uint32(i)
		}
	} else {
		chunkSet := make(map[uint32]struct{})
		for _, path := range paths {
			ext, ok := r.FindExtent(spec.NamespaceVFS, path)
			if !ok {
				ext, ok = r.extentMap[path]
			}
			if !ok {
				continue
			}
			for _, cIdx := range ext.GetChunkList() {
				chunkSet[cIdx] = struct{}{}
			}
		}
		for cIdx := range chunkSet {
			chunkIndices = append(chunkIndices, cIdx)
		}
	}

	for _, cIdx := range chunkIndices {
		if _, err := r.decompressChunk(cIdx); err != nil {
			return fmt.Errorf("hydrate chunk %d: %w", cIdx, err)
		}
	}
	return nil
}

// ReadExtent returns the uncompressed byte slice of the named extent.
func (r *Reader) ReadExtent(name string) ([]byte, error) {
	ext, ok := r.FindExtent(spec.NamespaceVFS, name)
	if !ok {
		ext, ok = r.extentMap[name]
		if !ok {
			return nil, os.ErrNotExist
		}
	}

	if ext.PayloadSize == 0 {
		return []byte{}, nil
	}

	if ext.ChunkCount <= 1 {
		raw, err := r.decompressChunk(ext.ChunkIndex)
		if err != nil {
			return nil, err
		}

		if ext.ChunkOffset+ext.PayloadSize > uint64(len(raw)) {
			return nil, io.ErrUnexpectedEOF
		}

		return raw[ext.ChunkOffset : ext.ChunkOffset+ext.PayloadSize], nil
	}

	// Multi-chunk spanning extent
	out := make([]byte, ext.PayloadSize)
	var readTotal uint64
	chunkList := ext.GetChunkList()
	curOffset := ext.ChunkOffset

	for _, chunkIdx := range chunkList {
		if readTotal >= ext.PayloadSize {
			break
		}
		raw, err := r.decompressChunk(chunkIdx)
		if err != nil {
			return nil, err
		}
		if curOffset >= uint64(len(raw)) {
			curOffset = 0
			continue
		}
		avail := uint64(len(raw)) - curOffset
		needed := ext.PayloadSize - readTotal
		toCopy := min(avail, needed)
		copy(out[readTotal:], raw[curOffset:curOffset+toCopy])
		readTotal += toCopy
		curOffset = 0
	}
	return out, nil
}

// Open opens a stream reader for the named extent.
func (r *Reader) Open(name string) (io.ReadCloser, error) {
	data, err := r.ReadExtent(name)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// TotalChunks returns the total number of chunks.
func (r *Reader) TotalChunks() int {
	if r.ctoc == nil {
		return 0
	}
	return len(r.ctoc.Entries)
}

// Verify validates container integrity by decompressing all chunks and verifying checksums.
func (r *Reader) Verify(cb func(curChunk, totalChunks int, extName string, err error)) error {
	if r.ctoc == nil {
		return nil
	}
	totalChunks := len(r.ctoc.Entries)
	for i, entry := range r.ctoc.Entries {
		chunkIdx := uint32(i)
		raw, err := r.decompressChunk(chunkIdx)
		if err != nil {
			if cb != nil {
				cb(i+1, totalChunks, "", err)
			}
			return fmt.Errorf("chunk #%d decompression error: %w", chunkIdx, err)
		}

		if uint64(len(raw)) != uint64(entry.UncompressedSize) {
			err := fmt.Errorf(
				"chunk #%d size mismatch: expected %d, got %d",
				chunkIdx,
				entry.UncompressedSize,
				len(raw),
			)
			if cb != nil {
				cb(i+1, totalChunks, "", err)
			}
			return err
		}

		if cb != nil {
			cb(i+1, totalChunks, "", nil)
		}
	}

	if r.indx != nil {
		for _, ext := range r.indx.Extents {
			if cb != nil {
				cb(totalChunks, totalChunks, ext.Name, nil)
			}
		}
	}

	return nil
}

// DiagnosticReport contains detailed findings from container verification.
type DiagnosticReport struct {
	Quick           bool
	HeaderValid     bool
	TrailerValid    bool
	HasSignature    bool
	SignatureValid  bool
	SignatureError  error
	TotalChunks     int
	ValidChunks     int
	CorruptedChunks []ChunkDiagnostic
	TotalExtents    int
	ValidExtents    int
	CorruptedFiles  []FileDiagnostic
}

// ChunkDiagnostic describes an error encountered in a specific container chunk.
type ChunkDiagnostic struct {
	ChunkIndex uint32
	BlockIndex uint64
	Error      error
}

// FileDiagnostic describes an error associated with a specific file extent.
type FileDiagnostic struct {
	Name       string
	ChunkIndex uint32
	Error      error
}

// OK returns true if no errors were encountered during verification.
func (d *DiagnosticReport) OK() bool {
	return d.HeaderValid && d.TrailerValid &&
		(!d.HasSignature || d.SignatureValid) &&
		len(d.CorruptedChunks) == 0 &&
		len(d.CorruptedFiles) == 0
}

// Diagnose runs detailed diagnostics on the container.
// If quick is true, it validates headers, catalog tables, chunk magic/bounds, and signatures without full decompression.
func (r *Reader) Diagnose(quick bool) *DiagnosticReport {
	rep := &DiagnosticReport{
		Quick:        quick,
		HeaderValid:  true,
		TrailerValid: true,
	}

	if r.HasSignature() {
		rep.HasSignature = true
		if err := r.VerifySignature(nil); err != nil {
			rep.SignatureValid = false
			rep.SignatureError = err
		} else {
			rep.SignatureValid = true
		}
	}

	if r.ctoc == nil {
		return rep
	}

	rep.TotalChunks = len(r.ctoc.Entries)
	failedChunks := make(map[uint32]error)

	for i, entry := range r.ctoc.Entries {
		chunkIdx := uint32(i)
		chunkOffset := int64(entry.BlockIndex) * r.blockSize

		// 1. Read and validate chunk header
		var hdrBuf [spec.ChunkHeaderSize]byte
		if r.chunkStore != nil || (r.headerFlags&spec.FlagDetachedChunks) != 0 {
			rawChunk, fErr := r.fetchRemoteChunk(chunkIdx)
			if fErr != nil {
				errDesc := fmt.Errorf("fetch remote chunk %d: %w", chunkIdx, fErr)
				rep.CorruptedChunks = append(rep.CorruptedChunks, ChunkDiagnostic{
					ChunkIndex: chunkIdx,
					BlockIndex: entry.BlockIndex,
					Error:      errDesc,
				})
				failedChunks[chunkIdx] = errDesc
				continue
			}
			copy(hdrBuf[:], rawChunk[:spec.ChunkHeaderSize])
		} else {
			if _, err := r.rAt.ReadAt(hdrBuf[:], chunkOffset); err != nil {
				errDesc := fmt.Errorf("read chunk header at offset 0x%X: %w", chunkOffset, err)
				rep.CorruptedChunks = append(rep.CorruptedChunks, ChunkDiagnostic{
					ChunkIndex: chunkIdx,
					BlockIndex: entry.BlockIndex,
					Error:      errDesc,
				})
				failedChunks[chunkIdx] = errDesc
				continue
			}
		}

		if binary.LittleEndian.Uint32(hdrBuf[0:4]) != spec.MagicChunkLE {
			errDesc := fmt.Errorf("chunk magic mismatch at offset 0x%X (expected 'CHNK')", chunkOffset)
			rep.CorruptedChunks = append(rep.CorruptedChunks, ChunkDiagnostic{
				ChunkIndex: chunkIdx,
				BlockIndex: entry.BlockIndex,
				Error:      errDesc,
			})
			failedChunks[chunkIdx] = errDesc
			continue
		}

		if quick {
			rep.ValidChunks++
			continue
		}

		// Full deep decompression and AEAD authentication check
		raw, err := r.decompressChunk(chunkIdx)
		if err != nil {
			errDesc := fmt.Errorf("chunk decompression / AEAD authentication failed: %w", err)
			rep.CorruptedChunks = append(rep.CorruptedChunks, ChunkDiagnostic{
				ChunkIndex: chunkIdx,
				BlockIndex: entry.BlockIndex,
				Error:      errDesc,
			})
			failedChunks[chunkIdx] = errDesc
			continue
		}

		if uint64(len(raw)) != uint64(entry.UncompressedSize) {
			errDesc := fmt.Errorf(
				"chunk size mismatch: expected %d uncompressed, got %d",
				entry.UncompressedSize,
				len(raw),
			)
			rep.CorruptedChunks = append(rep.CorruptedChunks, ChunkDiagnostic{
				ChunkIndex: chunkIdx,
				BlockIndex: entry.BlockIndex,
				Error:      errDesc,
			})
			failedChunks[chunkIdx] = errDesc
			continue
		}

		rep.ValidChunks++
	}

	if r.indx != nil {
		rep.TotalExtents = len(r.indx.Extents)
		for _, ext := range r.indx.Extents {
			chunkList := ext.GetChunkList()
			var extentCorrupted bool

			for _, cIdx := range chunkList {
				// Check if extent references a known failed chunk
				if err, failed := failedChunks[cIdx]; failed {
					rep.CorruptedFiles = append(rep.CorruptedFiles, FileDiagnostic{
						Name:       ext.Name,
						ChunkIndex: cIdx,
						Error:      fmt.Errorf("underlying chunk #%d is corrupted: %w", cIdx, err),
					})
					extentCorrupted = true
					break
				}

				// Validate chunk index bounds
				if int(cIdx) >= rep.TotalChunks {
					rep.CorruptedFiles = append(rep.CorruptedFiles, FileDiagnostic{
						Name:       ext.Name,
						ChunkIndex: cIdx,
						Error:      fmt.Errorf("chunk index #%d out of bounds (total %d)", cIdx, rep.TotalChunks),
					})
					extentCorrupted = true
					break
				}
			}

			if extentCorrupted {
				continue
			}

			// Single-chunk extent capacity check
			if len(chunkList) == 1 {
				targetChunk := r.ctoc.Entries[ext.ChunkIndex]
				if ext.ChunkOffset+ext.PayloadSize > uint64(targetChunk.UncompressedSize) {
					rep.CorruptedFiles = append(rep.CorruptedFiles, FileDiagnostic{
						Name:       ext.Name,
						ChunkIndex: ext.ChunkIndex,
						Error: fmt.Errorf(
							"extent offset+size (%d+%d=%d) exceeds chunk uncompressed capacity (%d)",
							ext.ChunkOffset,
							ext.PayloadSize,
							ext.ChunkOffset+ext.PayloadSize,
							targetChunk.UncompressedSize,
						),
					})
					continue
				}
			}

			rep.ValidExtents++
		}
	}

	return rep
}

// ChunkHash returns the SHA-256 content hash of chunkIdx.
func (r *Reader) ChunkHash(chunkIdx uint32) ([32]byte, error) {
	if r.chunkHashes != nil {
		if h, ok := r.chunkHashes.Get(chunkIdx); ok {
			return h, nil
		}
	}
	if r.ctoc == nil || int(chunkIdx) >= len(r.ctoc.Entries) {
		return [32]byte{}, errors.New("aeon: chunk index out of bounds")
	}

	entry := r.ctoc.Entries[chunkIdx]
	diskOffset := int64(entry.BlockIndex) << r.blockShift
	hdrReader := io.NewSectionReader(r.rAt, diskOffset, spec.ChunkHeaderSize)
	c, err := chunk.ParseHeader(hdrReader)
	if err != nil {
		return [32]byte{}, fmt.Errorf("parse chunk %d: %w", chunkIdx, err)
	}
	return c.Hash, nil
}

// RawChunkBytes returns the full compressed chunk bytes (64-byte header + compressed payload) for chunkIdx.
func (r *Reader) RawChunkBytes(chunkIdx uint32) ([]byte, error) {
	if r.ctoc == nil || int(chunkIdx) >= len(r.ctoc.Entries) {
		return nil, errors.New("aeon: chunk index out of bounds")
	}

	if r.chunkStore != nil || (r.headerFlags&spec.FlagDetachedChunks) != 0 {
		return r.fetchRemoteChunk(chunkIdx)
	}

	entry := r.ctoc.Entries[chunkIdx]
	diskOffset := int64(entry.BlockIndex) << r.blockShift
	unpaddedLen := spec.ChunkHeaderSize + int64(entry.CompressedSize)

	raw := make([]byte, unpaddedLen)
	if _, err := r.rAt.ReadAt(raw, diskOffset); err != nil {
		return nil, fmt.Errorf("read raw chunk %d: %w", chunkIdx, err)
	}
	return raw, nil
}

// IndexBytes returns the bytes of index.eon if this is a detached container, or error if not.
func (r *Reader) IndexBytes() ([]byte, error) {
	if (r.headerFlags & spec.FlagDetachedChunks) != 0 {
		buf := make([]byte, r.size)
		if _, err := r.rAt.ReadAt(buf, 0); err != nil {
			return nil, err
		}
		return buf, nil
	}
	return nil, errors.New("aeon: container is monolithic (index is embedded)")
}
