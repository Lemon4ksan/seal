# EON Container Format Specification

```text
Document:     EON-SPEC-0.1
Version:      0.1.0
Status:       Standard Specification
Extension:    .eon
MIME Type:    application/x-eon-archive
Byte Order:   Little-Endian
```

> _«Entropy destroys all data — let EON preserve reality for eternity.»_

## 1. Scope and Architecture

This document specifies the binary layout, encoding rules, and retrieval algorithms of the EON Container Format.

EON is a block-aligned, fault-tolerant container format engineered for low-latency random access, in-place delta updates, bounded-memory decompression, and long-term media preservation. The specification establishes a separation between the physical block substrate (compression, LBA alignment, erasure coding) and logical interpretation (filesystems, block devices, multidimensional arrays, or application-defined structures).

### 1.1. Key Technical Properties

1. **Deterministic O(1) Traversal**: A fixed-size 32-byte trailer at the physical end of the file permits index resolution in a single range read, eliminating sequential archive scanning.
2. **Configurable Logical Block Addressing (LBA)**: Physical chunk offsets align to $2^{\text{BlockSizeShift}}$ boundaries (default 4096 bytes for operating system memory pages; 65536 bytes for NVMe and GPU direct memory access).
3. **Bounded Decompression Memory**: Solid chunks (up to 32 MiB) contain byte-aligned entropy decoder checkpoints every 64 KiB. Random-access reading of a single extent requires at most 64 KiB of decompression buffer space.
4. **Universal Extent Model**: Data entities are addressed via a 16-bit namespace and an identifier string, decoupling storage from domain-specific abstractions.
5. **In-Place Append Merging**: Incremental updates append new chunks and an updated catalog to the end of the existing file without rewriting unchanged base data.
6. **Single-Instance Deduplication**: Duplicate byte sequences across extents map to the same chunk coordinates at zero additional storage cost.
7. **Erasure Coding**: Integrated Cauchy Reed-Solomon coding over $GF(2^8)$ provides configurable parity reconstruction against media degradation.
8. **In-Situ Execution**: The format permits prepending a native executable stub (PE, ELF, Mach-O) that mounts archive contents in-memory without staging files in temporary directories.

## 2. File Organization

An EON archive consists of five contiguous sections, bounded by fixed 32-byte structures at the archive boundary offsets:

```text
+-------------------------------------------------------------------------------+
| 1. Executable Stub (Optional, 0 .. ArchiveStartOffset Bytes)                  |
|    Native runtime for in-situ mounting (PE32+, ELF64, Mach-O)                 |
+-------------------------------------------------------------------------------+
| 2. Archive Header (32 Bytes Fixed at Offset ArchiveStartOffset)               |
|    Magic 'EON!', Version, Profile, BlockShift, Flags, ChunkTargetSize          |
+-------------------------------------------------------------------------------+
| 3. Chunk Sequence (Contiguous Solid Chunks 0 .. N-1)                          |
|    +------------------------------------------------------------------------+ |
|    | Chunk 0: Header (64 B) + Compressed Payload (64 KiB Micro-Frames) + Pad| |
|    +------------------------------------------------------------------------+ |
|    | Chunk 1: Header (64 B) + Compressed Payload (64 KiB Micro-Frames) + Pad| |
|    +------------------------------------------------------------------------+ |
|    | ...                                                                    | |
|    +------------------------------------------------------------------------+ |
|    | Chunk N-1: Header (64 B) + Compressed Payload + Pad                    | |
|    +------------------------------------------------------------------------+ |
+-------------------------------------------------------------------------------+
| 4. Recovery Record (Optional, 'RECV')                                         |
|    Cauchy Reed-Solomon parity shards over GF(2^8)                             |
+-------------------------------------------------------------------------------+
| 5. Central Directory Section                                                  |
|    +------------------------------------------------------------------------+ |
|    | 5.1. Chunk Table of Contents ('CTOC'): LBA offsets and chunk sizes     | |
|    +------------------------------------------------------------------------+ |
|    | 5.2. Extent Catalog ('INDX'): Namespace records, hashes, attributes    | |
|    +------------------------------------------------------------------------+ |
+-------------------------------------------------------------------------------+
| 6. Archive Trailer (32 Bytes Fixed at [EOF - 32 Bytes])                       |
|    CentralDirOffset, RecoveryOffset, ArchiveStartOffset, FlagsCopy, '!EON'   |
+-------------------------------------------------------------------------------+
```

## 3. Byte-Level Structure Definitions

### 3.1. Archive Header (32 Bytes Fixed)

Located at byte offset `ArchiveStartOffset` (typically `0x00000000`):

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `Magic` | `[4]byte` | ASCII `'E', 'O', 'N', '!'` (`0x214E4F45` in little-endian uint32) |
| `0x04` | 1 | `VersionMajor` | `uint8` | Major format version. Current standard: `0x01` |
| `0x05` | 1 | `VersionMinor` | `uint8` | Minor format revision. Current standard: `0x00` |
| `0x06` | 1 | `Profile` | `uint8` | Structural profile hint (see Table 3.1.1) |
| `0x07` | 1 | `TargetOS` | `uint8` | Operating system identifier (0: Agnostic, 1: Linux, 2: Windows, 3: Darwin) |
| `0x08` | 1 | `TargetArch` | `uint8` | Architecture identifier (0: Agnostic, 1: amd64, 2: arm64, 3: riscv64, 4: wasm) |
| `0x09` | 1 | `BlockSizeShift`| `uint8` | LBA block exponent $S$, where block size is $2^S$ bytes (default: `12`) |
| `0x0A` | 2 | `Flags` | `uint16` | Global archive flags (see Table 3.1.2) |
| `0x0C` | 4 | `ChunkTargetSize`| `uint32` | Maximum uncompressed chunk payload (default: `0x02000000` = 32 MiB) |
| `0x10` | 8 | `CreatedTimeNano`| `int64` | Creation timestamp in nanoseconds since Unix epoch |
| `0x18` | 4 | `HeaderCRC32` | `uint32` | IEEE 802.3 CRC32 computed over bytes `0x00` through `0x17` |
| `0x1C` | 4 | `Reserved` | `uint32` | Reserved for future alignment. Must be set to `0x00000000` |

#### Table 3.1.1: Structural Profiles (`Profile`)
* `0x00`: `PROFILE_VFS` — File and directory hierarchy.
* `0x01`: `PROFILE_BLOCK` — Block device image (virtual disk).
* `0x02`: `PROFILE_TENSOR` — Multidimensional arrays and tensor buffers.
* `0x03`: `PROFILE_STREAM` — Sequential time-stamped packet stream.
* `0xFF`: `PROFILE_CUSTOM` — Application-defined structure.

#### Table 3.1.2: Header Flags (`Flags`)
* `Bit 0` (`0x0001`): `FLAG_CENTRAL_DIRECTORY` — Central Directory is present at EOF.
* `Bit 1` (`0x0002`): `FLAG_RECOVERY_RECORD` — Reed-Solomon parity stream is present.
* `Bit 2` (`0x0004`): `FLAG_DIGITAL_SIGNATURE` — Directory includes cryptographic signature.
* `Bit 3` (`0x0008`): `FLAG_ENCRYPTED` — Chunks are encrypted with AEAD cipher.
* `Bit 4` (`0x0010`): `FLAG_STREAM_PIPE` — Archive written to non-seekable output.
* `Bit 5` (`0x0020`): `FLAG_ALIGNED_BLOCKS` — Chunks align to $(1 \ll \text{BlockSizeShift})$ bytes.
* `Bit 6` (`0x0040`): `FLAG_TIERED_INGEST` — Chunks ordered by ingest tier (Hot, Warm, Cold).
* `Bit 7` (`0x0080`): `FLAG_SELF_MOUNTING` — Executable stub precedes header (`ArchiveStartOffset` > 0).

### 3.2. Chunk Header (64 Bytes Fixed)

Every compressed chunk begins with a 64-byte record:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `ChunkMagic` | `[4]byte` | ASCII `'C', 'H', 'N', 'K'` (`0x4B4E4843`) |
| `0x04` | 4 | `ChunkIndex` | `uint32` | Zero-based chunk sequence index |
| `0x08` | 1 | `CodecID` | `uint8` | Compression algorithm (see Section 4) |
| `0x09` | 1 | `CodecLevel` | `uint8` | Compressor configuration level |
| `0x0A` | 2 | `ChunkFlags` | `uint16` | Bit 0: Solid (`0x01`), Bit 1: Encrypted (`0x02`), Bit 2: MicroFrames (`0x04`) |
| `0x0C` | 8 | `UncompressedSize`| `uint64`| Total uncompressed size of extent data within this chunk |
| `0x14` | 8 | `CompressedSize` | `uint64` | Byte length of compressed payload following header |
| `0x1C` | 4 | `ItemCount` | `uint32` | Number of distinct extents referencing this chunk |
| `0x20` | 32 | `UncompressedHash`| `[32]byte`| BLAKE3-256 hash of uncompressed chunk data |

#### 3.2.1. Sub-Chunk Micro-Framing
When `ChunkFlags` bit 2 (`0x0004`) is set, the compressed byte stream contains byte-aligned entropy decoder state resets at fixed 64 KiB uncompressed intervals while retaining the sliding LZ77 dictionary history. A decoder seeking an extent located at uncompressed offset $O$ initializes at checkpoint $\lfloor O / 65536 \rfloor \times 65536$, bounding maximum active memory to 64 KiB.

If `FLAG_ALIGNED_BLOCKS` is enabled, the end of the compressed payload is padded with zeros to the next $(1 \ll \text{BlockSizeShift})$ boundary. Otherwise, it is padded to a 4-byte boundary.

### 3.3. Central Directory Section

The Central Directory is located via `CentralDirOffset` in the trailer. It contains two consecutive binary structures: the Chunk Table of Contents (`CTOC`) followed by the Extent Catalog (`INDX`).

#### 3.3.1. Chunk Table of Contents (`CTOC`)
Permits $O(1)$ random seek resolution to any chunk offset on physical media:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `CTOCMagic` | `[4]byte` | ASCII `'C', 'T', 'O', 'C'` (`0x434F5443`) |
| `0x04` | 4 | `TotalChunks` | `uint32` | Number of entries in chunk table |
| `0x08` | 4 | `HotChunksCount`| `uint32` | Number of chunks in Tier 0 (Bootstrap) |
| `0x0C` | 4 | `WarmChunksCount`| `uint32`| Number of chunks in Tier 1 (Runtime Steady-State) |

Following the header, an array of `TotalChunks` contiguous 16-byte records is serialized:

```text
struct CTOCEntry {
    BlockIndex       uint64  // Physical disk offset: BlockIndex << BlockSizeShift bytes
    CompressedSize   uint32  // Byte length of compressed payload
    UncompressedSize uint32  // Byte length of uncompressed payload (<= ChunkTargetSize)
}
```

#### 3.3.2. Extent Catalog Header (`INDX`)
Follows immediately after the `CTOCEntry` array:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `IndexMagic` | `[4]byte` | ASCII `'I', 'N', 'D', 'X'` (`0x58444E49`) |
| `0x04` | 4 | `TotalExtents` | `uint32` | Total number of extent records in catalog |

#### 3.3.3. Universal Extent Record
Each extent is represented by a variable-length record:

| Field Name | Type | Encoding | Description |
|---|---|---|---|
| `RecordLength` | `uint16` | Fixed LE | Total byte length of this record, including all trailing fields |
| `Namespace` | `uint16` | Fixed LE | Domain classification identifier (see Section 3.3.4) |
| `ChunkIndex` | `uint32` | Fixed LE | Starting solid chunk index |
| `ChunkCount` | `uint16` | Fixed LE | Number of contiguous chunks spanned (typically 1; >1 for large items) |
| `ExtentFlags` | `uint16` | Fixed LE | Extent behavior flags (Bit 0: Sparse, Bit 1: Executable, Bit 2: DirectIO) |
| `TimestampNano`| `int64` | Fixed LE | Modification or presentation timestamp in Unix epoch nanoseconds |
| `PayloadBLAKE3`| `[32]byte` | Fixed | BLAKE3-256 hash of uncompressed extent payload |
| `TargetLength` | `uint16` | Fixed LE | Byte length of `Target` field (0 if unused) |
| `NameLength` | `uint16` | Fixed LE | Byte length of `Name` field |
| `ExtraLength` | `uint16` | Fixed LE | Byte length of `ExtraTLVs` field |
| `ChunkOffset` | `uint64` | ULEB128 | Byte offset of extent within uncompressed chunk |
| `PayloadSize` | `uint64` | ULEB128 | Exact uncompressed payload size in bytes |
| `Target` | `[]byte` | Raw bytes | Secondary target identifier (e.g. symlink destination) |
| `Name` | `[]byte` | UTF-8 | Primary identifier (path, sector range, or key) |
| `ExtraTLVs` | `[]byte` | Raw bytes | Concatenated Type-Length-Value attribute records |

### 3.3.4. Namespace Allocations

The 16-bit `Namespace` field establishes context for `Name` and `ExtraTLVs`:

| Range | Domain | Description |
|---|---|---|
| `0x0000` | POSIX VFS | File path hierarchy (`Name` = normalized relative path) |
| `0x0001` | Block Device | Disk sector ranges (`Name` = `"sectors:start-end"`) |
| `0x0002` | Tensor Store | Named arrays and tensor buffers (`Name` = tensor key) |
| `0x0003` | Key-Value Store | Random-access key/blob pairs (`Name` = key identifier) |
| `0x0004..0x7FFF` | Standard Registry | Reserved for future format-standardized domains |
| `0x8000..0xFFFF` | Private / Enterprise | Application-defined namespaces for proprietary integration |

### 3.3.5. Extensible TLV Metadata Plane

Metadata attributes are stored as concatenated Type-Length-Value records within `ExtraTLVs`:
* `Tag` (`uint16`): Attribute identifier.
* `Length` (`uint16`): Value length in bytes.
* `Value` (`[Length]byte`): Attribute payload.

| Tag | Hex | Name | Payload Structure |
|---|---|---|---|
| `'PX'` | `0x5058` | POSIX Permissions | `Mode uint32`, `UID uint32`, `GID uint32` |
| `'SI'` | `0x5349` | Digital Signature | Ed25519 public key (32B) + signature (64B) + Signer ID |
| `'PV'` | `0x5056` | Provenance | Commit hash, repository URI, builder signature |
| `'EP'` | `0x4550` | Entrypoint | UTF-8 execution command string for container/bundle runtimes |
| `'EV'` | `0x4556` | Environment | Null-delimited list of `KEY=VALUE` environment variables |
| `'CM'` | `0x434D` | Comment | UTF-8 descriptive text |
| `'NT'` | `0x4E54` | Windows Metadata | Win32 file attribute flags and alternate stream markers |
| `'XA'` | `0x5841` | Extended Attributes | Security labels (SELinux, POSIX ACLs) |
| `'PR'` | `0x5052` | Ingest Priority | Priority tier: `0` (Hot/Bootstrap), `1` (Warm), `2` (Cold) |
| `'TS'` | `0x5453` | Tensor Descriptor | Element datatype, rank, dimension array |

Implementations encountering unrecognized `Tag` identifiers SHALL skip `Length` bytes and continue catalog processing.

### 3.4. In-Situ Self-Mounting Executables (SMX)

When `FLAG_SELF_MOUNTING` (`0x0080`) is set, a native executable launcher stub occupies bytes `0x00` through `ArchiveStartOffset - 1`.

#### Execution Sequence:
1. The host operating system kernel initiates execution of the binary stub via standard process loader interfaces (`execve` on POSIX, `CreateProcess` on Windows).
2. The launcher stub obtains a read-only handle to its own executable image via process argument vector inspection (`argv[0]`) or platform APIs (`/proc/self/exe`, `GetModuleFileNameW`).
3. The stub seeks to `[EOF - 32]` to read the trailer, extracting `ArchiveStartOffset` and `CentralDirOffset`.
4. The stub constructs an in-memory virtual filesystem mount (via native kernel mechanisms: loop VFS, FUSE, or Projected File System) directly from the backing file handle.
5. The stub executes the entrypoint entity designated by the `'EP'` metadata attribute.
6. Upon process exit, the virtual filesystem mapping is torn down. Temporary host filesystem extraction SHALL NOT occur.

### 3.5. Single-Instance Deduplication

When multiple entities across any namespace share identical binary content:
* Encoders SHALL store the content once within the chunk sequence.
* Encoders SHALL emit duplicate extent catalog records pointing to the same `(ChunkIndex, ChunkOffset, PayloadSize)` tuple.
* Decoders SHALL resolve data from the existing chunk coordinates without secondary disk reads.

### 3.6. Reed-Solomon Recovery Record

Self-healing parity is optionally appended after the final data chunk and referenced by `RecoveryOffset` in the trailer:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `RecoveryMagic` | `[4]byte` | ASCII `'R', 'E', 'C', 'V'` (`0x56434552`) |
| `0x04` | 1 | `Polynomial` | `uint8` | Primitive polynomial: `0x01` ($x^8 + x^4 + x^3 + x^2 + 1$) |
| `0x05` | 1 | `ParityPercent` | `uint8` | Configured parity ratio (e.g. `3` = 3%) |
| `0x06` | 2 | `ShardSizeKB` | `uint16` | Parity shard size in KiB (default: `64`) |
| `0x08` | 8 | `ProtectedOffset` | `uint64` | Starting byte offset of protected range |
| `0x10` | 8 | `ProtectedLength` | `uint64` | Byte span covered by parity computation |
| `0x18` | 4 | `DataShards` | `uint32` | Number of primary data shards ($K$) |
| `0x1C` | 4 | `ParityShards` | `uint32` | Number of generated parity shards ($M$) |
| `0x20` | `M * ShardSize` | `ParityPayload` | `[]byte` | Cauchy Reed-Solomon parity matrix output |

Decoders reconstruct corrupted or missing shards via Cauchy matrix inversion over $GF(2^8)$ provided the number of failed shards does not exceed $M$.

### 3.7. Archive Trailer (32 Bytes Fixed)

The trailer occupies the exact last 32 bytes of the file (`[file_size - 32]` to `file_size`):

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 8 | `CentralDirOffset` | `uint64` | Byte offset to start of `CTOC` |
| `0x08` | 8 | `RecoveryOffset` | `uint64` | Byte offset to `RECV` section (`0` if parity absent) |
| `0x10` | 8 | `ArchiveStartOffset`| `uint64` | Byte offset to Archive Header (`0` for clean files; >0 for SMX) |
| `0x18` | 4 | `FlagsCopy` | `uint32` | Exact copy of archive flags for fast inspection |
| `0x1C` | 4 | `TrailerMagic` | `[4]byte` | ASCII `'!', 'E', 'O', 'N'` (`0x4E4F4521` in little-endian uint32) |

## 4. Codec Identifiers

The 1-byte `CodecID` in the Chunk Header determines payload decompression:

| Codec ID | Name | Specification |
|---|---|---|
| `0x00` | STORE | Uncompressed byte pass-through |
| `0x01` | ZSTD | Zstandard (RFC 8878) |
| `0x02` | OPENZL | OpenZL transform graph and FSE |
| `0x03` | LZMA2 | 7-Zip LZMA2 dictionary coder |
| `0x04` | BROTLI | Brotli compression format (RFC 7932) |

## 5. Delta Update and Append Protocol

To update an archive from version $A$ to version $B$ in-place:

1. The writer opens the container in append mode.
2. The writer calculates block changes by comparing `UncompressedHash` entries in `CTOC`.
3. The writer seeks to `[file_size - 32]`, overwriting the existing trailer.
4. New and modified solid chunks are appended sequentially.
5. A revised `CTOC` is appended, referencing prior block offsets for unmodified chunks and new offsets for appended chunks.
6. A revised `INDX` catalog is appended.
7. A new 32-byte trailer is written at the new file boundary.

The computational and write complexity of an update is $O(\text{DeltaSize})$, independent of total container size.

## 6. Binary Reference Dump

### 6.1. Archive Header (Offset 0x0000)
```text
Offset  Hex Data                                        Interpretation
0x0000  45 4F 4E 21                                     Magic: 'EON!'
0x0004  01 00                                           Version 1.0
0x0006  00                                              Profile: VFS (0x00)
0x0007  01                                              TargetOS: Linux (0x01)
0x0008  01                                              TargetArch: amd64 (0x01)
0x0009  0C                                              BlockSizeShift: 12 (4096-byte blocks)
0x000A  67 00                                           Flags: INDEX | RECV | SIG | ALIGNED | TIERED
0x000C  00 00 00 02                                     ChunkTargetSize: 32 MiB
0x0010  00 68 39 4F 35 01 00 00                         Timestamp: Unix nanoseconds
0x0018  B4 82 1A F2                                     Header CRC32: 0xF21A82B4
0x001C  00 00 00 00                                     Reserved
```

### 6.2. Archive Trailer (Offset EOF - 32)
```text
Offset  Hex Data                                        Interpretation
EOF-32  00 10 20 00 00 00 00 00                         CentralDirOffset: 0x00201000
EOF-24  00 00 20 00 00 00 00 00                         RecoveryOffset: 0x00200000
EOF-16  00 00 00 00 00 00 00 00                         ArchiveStartOffset: 0x00000000
EOF-08  67 00 00 00                                     FlagsCopy: 0x00000067
EOF-04  21 4F 45 4E                                     TrailerMagic: '!EON'
```

## 7. Retrieval Algorithms

### 7.1. Algorithm 1: Container Resolution and Index Parsing

```text
Input:  Random-access file handle F of length L
Output: Extent mapping M, chunk block table T, block shift S

1. Seek to (L - 32) in F
2. Read Trailer [32 bytes]
3. If Trailer.TrailerMagic != 0x4E4F4521 ('!EON'):
       Raise InvalidFormatError
4. CentralDirOffset   := Trailer.CentralDirOffset
5. ArchiveStartOffset := Trailer.ArchiveStartOffset

6. Seek to ArchiveStartOffset in F
7. Read Archive Header [32 bytes]
8. S := ArchiveHeader.BlockSizeShift

9. Seek to CentralDirOffset in F
10. Read CTOC Header [16 bytes]
11. TotalChunks := CTOC.TotalChunks
12. Initialize array T of size TotalChunks
13. For i := 0 to TotalChunks - 1:
        Read CTOCEntry [16 bytes]
        T[i].BlockIndex       := CTOCEntry.BlockIndex
        T[i].CompressedSize   := CTOCEntry.CompressedSize
        T[i].UncompressedSize := CTOCEntry.UncompressedSize

14. Read INDX Header [8 bytes]
15. TotalExtents := INDX.TotalExtents
16. Initialize map M
17. For i := 0 to TotalExtents - 1:
        Read fixed record prefix [56 bytes]
        Namespace    := prefix.Namespace
        ChunkIndex   := prefix.ChunkIndex
        NameLength   := prefix.NameLength
        ExtraLength  := prefix.ExtraLength
        ChunkOffset  := ReadULEB128(F)
        PayloadSize  := ReadULEB128(F)
        Name         := ReadBytes(F, NameLength)
        SkipBytes(F, ExtraLength)
        M[Name] := {
            Namespace:   Namespace,
            ChunkIndex:  ChunkIndex,
            ChunkOffset: ChunkOffset,
            PayloadSize: PayloadSize
        }

18. Return (M, T, S)
```

### 7.2. Algorithm 2: Random Extent Extraction

```text
Input:  File handle F, Extent descriptor E, block table T, block shift S
Output: Extracted uncompressed byte sequence B

1. PhysicalByteOffset := T[E.ChunkIndex].BlockIndex << S
2. Seek to (PhysicalByteOffset + 64) in F // Skip 64-byte CHNK header
3. PayloadStream := InitializeDecompressor(F, T[E.ChunkIndex].CompressedSize)
4. If Chunk has micro-framing (ChunkFlags & 0x0004):
       TargetCheckpoint := floor(E.ChunkOffset / 65536) * 65536
       SeekStreamToCheckpoint(PayloadStream, TargetCheckpoint)
       LocalOffset := E.ChunkOffset - TargetCheckpoint
   Else:
       LocalOffset := E.ChunkOffset
5. DiscardBytes(PayloadStream, LocalOffset)
6. B := ReadBytes(PayloadStream, E.PayloadSize)
7. CloseDecompressor(PayloadStream)
8. Return B
```

## 8. Conformance Requirements

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", and "MAY" in this document are to be interpreted as described in BCP 14 / RFC 2119.

1. **Unrecognized Fields**: Implementations encountering unknown flags, namespaces, or TLV attribute tags MUST NOT fail. They MUST ignore unknown attributes and continue parsing valid records.
2. **Deterministic Allocation Limit**: Encoders SHALL NOT produce uncompressed solid chunks exceeding `ChunkTargetSize` (maximum 32 MiB).
3. **Block Alignment**: When `FLAG_ALIGNED_BLOCKS` (`0x0020`) is set, writers MUST pad chunk boundaries to full $(1 \ll \text{BlockSizeShift})$ multiples using zero bytes. Readers MUST compute byte offsets using `BlockIndex << BlockSizeShift`.
4. **Single-Instance Invariance**: When two or more catalog records reference identical byte sequences, encoders SHOULD emit shared chunk extents rather than duplicate payloads.
5. **Zero-Staging Execution**: Self-mounting stubs SHALL NOT extract payload contents into host temporary directories. Mount operations MUST occur in-memory or via kernel direct-mapping facilities.
6. **Trailing Pad Invariance**: Decoders MUST tolerate zero-padding between directory structures or at archive boundaries.

*EON Container Specification. Dedicated to the Public Domain under Creative Commons CC0 1.0 Universal.*
