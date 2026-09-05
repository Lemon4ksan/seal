# EON Container Format Specification

```text
Document:     EON-SPEC-0.1
Version:      0.1.0
Status:       Standard Specification
Extension:    .eon
MIME Type:    application/x-eon-archive
Byte Order:   Little-Endian
```

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
|    Magic 'EON!', Version, Profile, BlockShift, Flags, ChunkTargetSize         |
+-------------------------------------------------------------------------------+
| 2.1. Encryption & Key Slots Header (Optional, 'ENCR')                         |
|      Present if FLAG_ENCRYPTED (0x0008) is set. Multi-key slots, Argon2id, IV |
+-------------------------------------------------------------------------------+
| 3. Chunk Sequence (Contiguous Solid Chunks 0 .. N-1)                          |
|    +------------------------------------------------------------------------+ |
|    | Chunk 0: Header (64 B) + (AEAD) Payload (64 KiB Micro-Frames) + Pad    | |
|    +------------------------------------------------------------------------+ |
|    | Chunk 1: Header (64 B) + (AEAD) Payload (64 KiB Micro-Frames) + Pad    | |
|    +------------------------------------------------------------------------+ |
|    | ...                                                                    | |
|    +------------------------------------------------------------------------+ |
|    | Chunk N-1: Header (64 B) + (AEAD) Payload + Pad                        | |
|    +------------------------------------------------------------------------+ |
+-------------------------------------------------------------------------------+
| 4. Recovery Record (Optional, 'RECV')                                         |
|    Cauchy Reed-Solomon parity shards over GF(2^8)                             |
+-------------------------------------------------------------------------------+
| 5. Central Directory Section (Encrypted if FLAG_ENCRYPTED is set)             |
|    +------------------------------------------------------------------------+ |
|    | 5.1. Chunk Table of Contents ('CTOC'): LBA offsets and chunk sizes     | |
|    +------------------------------------------------------------------------+ |
|    | 5.2. Extent Catalog ('INDX'): Canonical sorted extent records          | |
|    +------------------------------------------------------------------------+ |
|    | 5.3. Digital Signature Record ('DSIG', Optional): Directory signature  | |
|    +------------------------------------------------------------------------+ |
+-------------------------------------------------------------------------------+
| 6. Archive Trailer (32 Bytes Fixed at [EOF - 32 Bytes])                       |
|    CentralDirOffset, RecoveryOffset, ArchiveStartOffset, FlagsCopy, '!EON'    |
+-------------------------------------------------------------------------------+
```

## 3. Byte-Level Structure Definitions

### 3.1. Archive Header (32 Bytes Fixed)

Located at byte offset `ArchiveStartOffset` (typically `0x00000000`):

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `Magic` | `[4]byte` | ASCII `'E', 'O', 'N', '!'` (`0x214E4F45` in little-endian uint32) |
| `0x04` | 1 | `VersionMajor` | `uint8` | Major format version. Current standard: `0x00` |
| `0x05` | 1 | `VersionMinor` | `uint8` | Minor format revision. Current standard: `0x01` |
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
* `0x10`: `PROFILE_FAST_MOUNT` — Low-latency VFS mount profile (LZ4, 128 KiB micro-chunks).
* `0x20`: `PROFILE_ARCHIVE_MAX` — Maximum compression archival profile (ZSTD-19, 64 MiB chunks).
* `0xFF`: `PROFILE_CUSTOM` — Application-defined structure.

#### Table 3.1.2: Header Flags (`Flags`)
* `Bit 0` (`0x0001`): `FLAG_CENTRAL_DIRECTORY` — Central Directory is present at EOF.
* `Bit 1` (`0x0002`): `FLAG_RECOVERY_RECORD` — Reed-Solomon parity stream is present.
* `Bit 2` (`0x0004`): `FLAG_DIGITAL_SIGNATURE` — Directory includes cryptographic signature.
* `Bit 3` (`0x0008`): `FLAG_ENCRYPTED` — Container is protected with authenticated encryption (ENCR header, AEAD chunks, encrypted Central Directory).
* `Bit 4` (`0x0010`): `FLAG_STREAM_PIPE` — Archive written to non-seekable output.
* `Bit 5` (`0x0020`): `FLAG_ALIGNED_BLOCKS` — Chunks align to $(1 \ll \text{BlockSizeShift})$ bytes.
* `Bit 6` (`0x0040`): `FLAG_TIERED_INGEST` — Chunks ordered by ingest tier (Hot, Warm, Cold).
* `Bit 7` (`0x0080`): `FLAG_SELF_MOUNTING` — Executable stub precedes header (`ArchiveStartOffset` > 0).
* `Bit 8` (`0x0100`): `FLAG_CDC` — Content-Defined Chunking (FastCDC) active.
* `Bit 9` (`0x0200`): `FLAG_PROLLY_TREE` — Prolly Tree (Merkle B-Tree) index active.
* `Bit 10` (`0x0400`): `FLAG_DETACHED_CHUNKS` — Chunks stored as detached objects in S3/R2/GCS layout.

### 3.2. Chunk Header (64 Bytes Fixed)

Every compressed chunk begins with a 64-byte record:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `ChunkMagic` | `[4]byte` | ASCII `'C', 'H', 'N', 'K'` (`0x4B4E4843`) |
| `0x04` | 4 | `ChunkIndex` | `uint32` | Zero-based chunk sequence index |
| `0x08` | 1 | `CodecID` | `uint8` | Compression algorithm (see Section 4) |
| `0x09` | 1 | `CodecLevel` | `uint8` | Compressor configuration level |
| `0x0A` | 2 | `ChunkFlags` | `uint16` | Bit 0: Solid (`0x01`), Bit 1: Encrypted (`0x02`), Bit 2: MicroFrames (`0x04`), Bit 3: ByteShuffle (`0x08`), Bit 7: Pipeline (`0x0080`) |
| `0x0C` | 8 | `UncompressedSize`| `uint64`| Total uncompressed size of extent data within this chunk |
| `0x14` | 8 | `CompressedSize` | `uint64` | Byte length of compressed payload following header |
| `0x1C` | 4 | `ItemCount` | `uint32` | Number of distinct extents referencing this chunk |
| `0x20` | 32 | `UncompressedHash`| `[32]byte`| BLAKE3-256 hash of uncompressed chunk data |

#### 3.2.1. Sub-Chunk Micro-Framing
When `ChunkFlags` bit 2 (`0x0004`) is set, the compressed byte stream contains byte-aligned entropy decoder state resets at fixed 64 KiB uncompressed intervals while retaining the sliding LZ77 dictionary history. A decoder seeking an extent located at uncompressed offset $O$ initializes at checkpoint $\lfloor O / 65536 \rfloor \times 65536$, bounding maximum active memory to 64 KiB.

##### 3.2.1.1. Codec-Specific Implementations
1. **LZMA2 (`CodecID = 0x03`)**: Native chunking architecture. Each 64 KiB sub-chunk is emitted as a self-describing packet where the control byte specifies state reset (`mode 1`) and dictionary retention (`mode 0` or `mode 2`). Checkpoint seeking requires no supplemental table, as packet headers are self-describing.
2. **Zstandard (`CodecID = 0x01`)**: Implemented via the *Zstandard Seekable Format* (appends a 4-byte seek table footer to the chunk payload containing frame seek points every 64 KiB), or via independent 64 KiB Zstandard sub-frames referencing the preceding sliding window as an external prefix dictionary.
3. **Store / Raw (`CodecID = 0x00`)**: Checkpoint resolution is direct arithmetic calculation: $\text{DiskOffset} = \text{ChunkDiskOffset} + 64 + O$.

#### 3.2.2. Chunk Filter & Codec Pipeline Descriptor (`FLAG_PIPELINE`)
When `ChunkFlags` bit 7 (`0x0080`) is set, the chunk payload begins with a binary `PipelineDescriptor` header that specifies an ordered chain of reversible pre-filters applied to uncompressed extent data prior to compression:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `PipelineMagic` | `[4]byte` | ASCII `'P', 'I', 'P', 'E'` (`0x45504950` in little-endian uint32) |
| `0x04` | 1 | `StageCount` | `uint8` | Number of sequential filter stages ($N \in [1, 255]$) |

Following the header, each of the `StageCount` stages is serialized consecutively:
* `FilterID` (`uint8`): Filter type identifier (Table 3.2.2.1).
* `ParamLength` (`uint8`): Length of parameter payload ($P \in [0, 255]$).
* `Params` (`[ParamLength]byte`): Filter-specific configuration parameters.

Immediately following the last stage, the compressed payload produced by `ChunkHeader.CodecID` is serialized.

##### Table 3.2.2.1: Filter Identifiers (`FilterID`)
* `0x01`: `FILTER_SHUFFLE` — SIMD Byte-Shuffle for structured and multidimensional tensor data (stride in `Params[0]`, default: 4).
* `0x02`: `FILTER_BCJ_X86` — Branch-Call-Jump relative address translation for x86 / x86-64 machine code.
* `0x03`: `FILTER_BCJ_ARM64` — Branch relative address translation for AArch64 (ARM64) machine code.
* `0x04`: `FILTER_DELTA` — Byte distance prediction filter (distance in `Params[0]`, default: 1).
* `0x80..0xFF`: User-defined third-party application filters registered via `filter.Register()`.

During decompression, the pipeline descriptor is parsed, the payload is decompressed using `CodecID`, and each filter stage is executed in reverse order (**LIFO**: $\text{Stage}_N \to \dots \to \text{Stage}_1$).

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

#### 3.3.3.1. Multi-Chunk Extent Spanning Rules
When an extent's uncompressed payload exceeds remaining space in the current chunk or exceeds `ChunkTargetSize` (`ChunkCount > 1`):
1. **Initial Chunk (`ChunkIndex`)**: The extent data begins at byte offset `ChunkOffset` and extends up to the end of the uncompressed chunk:
   $$\text{BytesInInitialChunk} = \min(\text{PayloadSize}, \text{UncompressedSize}_{\text{ChunkIndex}} - \text{ChunkOffset})$$
2. **Intermediate Chunks (`ChunkIndex + 1` .. `ChunkIndex + ChunkCount - 2`)**: The extent occupies the entire uncompressed chunk from offset `0` through $\text{UncompressedSize}_{\text{chunk}}$.
3. **Final Chunk (`ChunkIndex + ChunkCount - 1`)**: The extent starts strictly at uncompressed byte offset `0`, reading the remaining payload:
   $$\text{RemainingBytes} = \text{PayloadSize} - \sum \text{BytesReadPrior}$$
4. Encoders MUST place spanning extents across strictly contiguous chunk sequence indices $[ \text{ChunkIndex} .. \text{ChunkIndex} + \text{ChunkCount} - 1 ]$.

#### 3.3.3.2. Canonical Extent Ordering & Binary Search Indexing
To ensure deterministic, reproducible archive creation across different operating systems and file traversal orders, and to enable decoders to execute random-access extent lookup in $O(\log N)$ time without allocating in-memory hash maps:
1. Encoders MUST serialize extent records in `INDX` sorted in strict canonical ascending order:
   - **Primary Key**: `Namespace` (`uint16`, unsigned numeric comparison: `0x0000` to `0xFFFF`).
   - **Secondary Key**: `Name` (lexicographical comparison of raw UTF-8 byte sequences).
2. For any two adjacent records $E_i$ and $E_{i+1}$ in `INDX`:
   $$(E_i.\text{Namespace} < E_{i+1}.\text{Namespace}) \lor (E_i.\text{Namespace} = E_{i+1}.\text{Namespace} \land E_i.\text{Name} < E_{i+1}.\text{Name})$$
3. Duplicate names within the same namespace SHALL NOT occur unless explicitly permitted by private enterprise profile extensions. Decoders MAY perform binary search (`bsearch`) directly over the extent catalog.

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
| `'CL'` | `0x434C` | Chunk List | Array of `uint32` discrete non-contiguous chunk indices |
| `'SS'` | `0x5353` | Snapshot ID | Unique snapshot identifier string |
| `'PS'` | `0x5053` | Parent Snapshot | Parent snapshot identifier string for Merkle history |

Implementations encountering unrecognized `Tag` identifiers SHALL skip `Length` bytes and continue catalog processing.

### 3.3.6. Central Directory Digital Signature Record ('DSIG')

When global archive flag `FLAG_DIGITAL_SIGNATURE` (`0x0004`) is enabled, the Central Directory concludes with a dedicated cryptographic signature record (`DSIG`) positioned immediately following the `INDX` catalog:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `SignatureMagic` | `[4]byte` | ASCII `'D', 'S', 'I', 'G'` (`0x47495344` in little-endian uint32) |
| `0x04` | 2 | `AlgorithmID` | `uint16` | Cryptographic signature algorithm (see Table 3.3.6.1) |
| `0x06` | 2 | `Reserved` | `uint16` | Reserved for alignment. MUST be `0x0000` |
| `0x08` | 2 | `KeyIDLength` | `uint16` | Byte length of `KeyID` string (0 if anonymous) |
| `0x0A` | 2 | `PublicKeyLength`| `uint16` | Byte length of embedded `PublicKey` bytes |
| `0x0C` | 2 | `SignatureLength`| `uint16` | Byte length of `Signature` payload |
| `0x0E` | `KeyIDLength` | `KeyID` | `[]byte` | UTF-8 key identifier (key fingerprint, email, or URI) |
| `...` | `PublicKeyLength` | `PublicKey` | `[]byte` | Raw public key bytes (e.g. 32 bytes for Ed25519) |
| `...` | `SignatureLength` | `Signature` | `[]byte` | Cryptographic signature over the signed payload digest |

#### Table 3.3.6.1: Signature Algorithms (`AlgorithmID`)
* `0x0001`: `SIG_ED25519` — PureEd25519 (RFC 8032) over SHA-256 or BLAKE3-256 digest.
* `0x0002`: `SIG_ECDSA_P256` — ECDSA over NIST P-256 with SHA-256.
* `0x0003`: `SIG_RSA_PSS` — RSA-PSS with 4096-bit modulus and SHA-256.
* `0x0004`: `SIG_ML_DSA_65` — Post-quantum lattice signature (FIPS 204 / ML-DSA-65).

#### 3.3.6.2. Signed Payload Digest
The cryptographic signature covers the exact, uncompressed, contiguous byte sequence of the Central Directory preceding `DSIG`:
$$\text{SignedDigest} = \text{SHA-256}(\text{Bytes}_{\text{CTOC}} \mathbin{\Vert} \text{Bytes}_{\text{INDX}})$$

Signing the entire Central Directory ensures holistic integrity: any unauthorized addition, modification, renaming, or deletion of extents, or manipulation of chunk offsets, invalidates the signature.

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

### 3.8. Encryption Subsystem & Key Slots Header ('ENCR')

When `FLAG_ENCRYPTED` (`0x0008`) is enabled, the container implements envelope authenticated encryption with multi-key slot management and complete metadata privacy. The Encryption Header (`ENCR`) is located immediately following the Archive Header (at byte offset `ArchiveStartOffset + 32`):

#### 3.8.1. Encryption Header Structure (24 Bytes Fixed)

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `Magic` | `[4]byte` | ASCII `'E', 'N', 'C', 'R'` (`0x52434E45` in little-endian uint32) |
| `0x04` | 1 | `CipherID` | `uint8` | Symmetric AEAD cipher identifier (see Table 3.8.1.1) |
| `0x05` | 1 | `SlotCount` | `uint8` | Number of active key slots (1 .. 16) |
| `0x06` | 2 | `Reserved` | `uint16` | Reserved for alignment. MUST be `0x0000` |
| `0x08` | 16 | `MasterIV` | `[16]byte` | Cryptographically secure random base salt/nonce seed |

##### Table 3.8.1.1: AEAD Cipher Identifiers (`CipherID`)
* `0x01`: `CIPHER_AES256_GCM` — AES-256 in Galois/Counter Mode (NIST SP 800-38D), 128-bit authentication tag.
* `0x02`: `CIPHER_CHACHA20_POLY1305` — ChaCha20-Poly1305 AEAD (RFC 8439), 128-bit authentication tag.

#### 3.8.2. Key Slot Record Structure (144 Bytes Fixed per Slot)

The `ENCR` header is followed immediately by an array of `SlotCount` contiguous 144-byte `KeySlot` records. Each slot binds a cryptographic credential to an isolated virtual vault descriptor:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 1 | `SlotIndex` | `uint8` | Slot index (`0` .. `SlotCount - 1`) |
| `0x01` | 1 | `SlotType` | `uint8` | Key derivation/unlock mechanism (see Table 3.8.2.1) |
| `0x02` | 2 | `SlotFlags` | `uint16` | Slot behavior flags (Bit 0: Hidden/Deniable, Bit 1: SharedChunks) |
| `0x04` | 32 | `Salt` | `[32]byte` | Cryptographically secure random salt for KDF |
| `0x24` | 4 | `KDFMemoryKiB` | `uint32` | Memory hardness in KiB (default: `65536` = 64 MiB for Argon2id) |
| `0x28` | 4 | `KDFIterations`| `uint32` | Iteration count/time cost (default: `3` for Argon2id, `600000` for PBKDF2) |
| `0x2C` | 1 | `KDFParallelism`| `uint8` | Degree of parallelism/threads (default: `4` for Argon2id) |
| `0x2D` | 3 | `ReservedPad` | `[3]byte` | Reserved for 8-byte alignment. MUST be `[0, 0, 0]` |
| `0x30` | 96 | `WrappedPayload`| `[96]byte` | Confidential `VaultDescriptor` (80B) encrypted with KEK via AEAD (16B tag) |

##### Table 3.8.2.1: Slot Types (`SlotType`)
* `0x01`: `SLOT_ARGON2ID` — Argon2id password-based key derivation (RFC 9106).
* `0x02`: `SLOT_PBKDF2_SHA256` — PBKDF2 with HMAC-SHA256 (RFC 8018) for constrained embedded runtimes.
* `0x03`: `SLOT_X25519` — Asymmetric key encapsulation via Curve25519 / HKDF.
* `0x04`: `SLOT_KMS` — Key Management Service envelope KEK.
* `0x05`: `SLOT_VAULT` — HashiCorp Vault Transit Secret Engine envelope KEK.

##### 3.8.2.2. Confidential Vault Descriptor (`VaultDescriptor`, 80 Bytes Plaintext)
The plaintext payload encapsulated within `WrappedPayload` is structured as follows:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 32 | `DEK` | `[32]byte` | Ephemeral 256-bit Data Encryption Key for this vault |
| `0x20` | 8 | `DirOffset` | `uint64` | Byte offset to this vault's encrypted Central Directory (`CTOC` + `INDX`) |
| `0x28` | 8 | `DirSize` | `uint64` | Byte length of this vault's encrypted Central Directory |
| `0x30` | 1 | `VaultID` | `uint8` | Virtual vault numeric identifier (`0` = default / root vault) |
| `0x31` | 1 | `VaultFlags` | `uint8` | Bit 0: Hidden/Deniable, Bit 1: SharedChunks |
| `0x32` | 22 | `VaultName` | `[22]byte` | Null-padded UTF-8 human-readable vault label (e.g. `"personal"`, `"work"`) |
| `0x48` | 8 | `Reserved` | `[8]byte` | Reserved for future expansion. MUST be zeroed |

Following the key slots array, padding bytes (zeros) align the stream to the block boundary $(1 \ll \text{BlockSizeShift})$, ensuring Chunk 0 commences at an aligned physical sector.

#### 3.8.3. Key Derivation & Key Hierarchy (Envelope Architecture)

1. **Master Data Encryption Key (DEK)**:
   - An ephemeral 256-bit symmetric key generated via high-entropy system CSPRNG (`crypto/rand`).
   - The raw DEK is NEVER stored in plaintext on storage media.
2. **Key Encryption Key (KEK)**:
   - Derived from the user passphrase $P$ and slot salt $S$:
     $$\text{KEK} = \text{Argon2id}(P, S, \text{time}=\text{KDFIterations}, \text{mem}=\text{KDFMemoryKiB}, \text{threads}=\text{KDFParallelism}, \text{keyLen}=32)$$
3. **Vault Descriptor Wrapping**:
   - The 80-byte `VaultDescriptor` is sealed into `WrappedPayload`:
     $$\text{WrappedPayload} = \text{AEAD-Encrypt}_{\text{KEK}}(\text{SlotNonce}, \text{VaultDescriptor}, \text{AAD}=\text{"eon-key-slot-v2-"}\mathbin{\Vert}\text{uint8}(\text{SlotIndex}))$$
     where $\text{SlotNonce} = \text{HKDF-Expand}(\text{Salt}, \text{"slot-nonce"}, 12)$.
4. **Sub-Key Derivation (Domain Separation)**:
   All operational encryption keys are derived from the unwrapped DEK via HKDF-Expand (RFC 5869) using SHA-256:
   - Chunk Payload Encryption: $K_{\text{chunk}} = \text{HKDF-Expand}(\text{DEK}, \text{"eon-chunk-payload-key"}, 32)$
   - Central Directory Encryption: $K_{\text{dir}} = \text{HKDF-Expand}(\text{DEK}, \text{"eon-central-directory-key"}, 32)$

#### 3.8.4. Chunk Authenticated Encryption

Each solid chunk $i$ ($0 \le i < N$) with `ChunkFlags` bit 1 (`ChunkFlagEncrypted = 0x0002`) is authenticated and encrypted:
1. **Deterministic Collision-Free Nonce**:
   $$\text{Nonce}_i = \text{HKDF-Expand}(MasterIV, \text{"chunk-"}\mathbin{\Vert}\text{uint32BE}(i), 12)$$
2. **Additional Authenticated Data (AAD)**:
   Cryptographically binds the encrypted ciphertext to the chunk sequence and metadata header:
   $$\text{AAD}_i = \text{MagicChunk (4B)} \mathbin{\Vert} \text{uint32BE}(i) \mathbin{\Vert} \text{CodecID (1B)} \mathbin{\Vert} \text{uint64BE}(\text{UncompressedSize})$$
3. **Sealing**:
   $$\text{Ciphertext}_i = \text{AEAD-Encrypt}_{K_{\text{chunk}}}(\text{Nonce}_i, \text{CompressedPayload}_i, \text{AAD}_i)$$
   The resulting payload is stored on disk with $\text{CompressedSize} = \text{len}(\text{CompressedPayload}) + 16$ bytes.

#### 3.8.5. Metadata Privacy (Encrypted Central Directory)

To guarantee zero metadata leakage (preventing observation of directory structure, file names, timestamps, or individual extent sizes):
1. The plaintext Central Directory payload consists of the concatenated serialized structures $[ \text{CTOC} \mathbin{\Vert} \text{INDX} \mathbin{\Vert} \text{DSIG} ]$.
2. **Directory Nonce**:
   $$\text{Nonce}_{\text{dir}} = \text{HKDF-Expand}(MasterIV, \text{"central-directory"}, 12)$$
3. **Directory AAD**:
   $$\text{AAD}_{\text{dir}} = \text{Bytes}_{\text{ArchiveHeader}[0..32]}$$
4. **Sealing**:
   $$\text{EncryptedDirectory} = \text{AEAD-Encrypt}_{K_{\text{dir}}}(\text{Nonce}_{\text{dir}}, \text{PlaintextDirectory}, \text{AAD}_{\text{dir}})$$
5. The 32-byte Archive Trailer at EOF points `CentralDirOffset` to the start of the default $\text{EncryptedDirectory}$.

#### 3.8.6. Compartmentalized Multi-Vault Containers & Plausible Deniability

EON containers support multiple isolated virtual storage vaults coexisting within a single physical `.eon` container:

1. **Vault Confidentiality & Isolation**:
   - Each vault $V$ possesses an independent `VaultDescriptor` sealed inside its designated `KeySlot`.
   - The directory offset `DirOffset`, directory byte length `DirSize`, and human-readable `VaultName` are stored strictly inside the AEAD ciphertext of `WrappedPayload`.
   - Without the specific passphrase for Vault $V$, an external observer cannot discern whether Vault $V$ exists, what its name is, or where its directory is located on disk.
2. **Independent Central Directories**:
   - Each vault maintains its own encrypted Central Directory $[ \text{CTOC}_V \mathbin{\Vert} \text{INDX}_V ]$ written at physical offset $\text{DirOffset}_V$.
   - Unlocking with Passphrase $A$ decrypts only $\text{INDX}_A$, rendering files belonging to Vault $B$ completely invisible to User $A$.
3. **Cross-Vault Chunk Deduplication ($A \cup B$)**:
   - If files across different vaults share identical binary content (e.g. shared runtime libraries, common documents), encoders store the data once within a shared solid chunk.
   - Both $\text{INDX}_A$ and $\text{INDX}_B$ emit extent records pointing to the same chunk coordinates $(ChunkIndex, ChunkOffset, PayloadSize)$ without duplicating data on disk.
4. **Multi-Key Union Resolution**:
   - When decoders are provided with multiple valid passphrases (e.g. $\{P_1, P_2, \dots, P_k\}$), the reader unlocks all matching vaults $\{V_1, V_2, \dots, V_k\}$ and synthesizes a unified virtual filesystem containing the union of all unlocked extents:
     $$\text{MergedCatalog} = \bigcup_{j=1}^k \text{INDX}_{V_j}$$
   - Duplicate entries across vaults are deduplicated by canonical `(Namespace, Name)` key.

## 4. Codec Identifiers

The 1-byte `CodecID` in the Chunk Header determines payload decompression:

| Codec ID | Name | Specification |
|---|---|---|
| `0x00` | STORE | Uncompressed byte pass-through |
| `0x01` | ZSTD | Zstandard (RFC 8878) |
| `0x02` | OPENZL | OpenZL transform graph and FSE |
| `0x03` | LZMA2 | 7-Zip LZMA2 dictionary coder |
| `0x04` | BROTLI | Brotli compression format (RFC 7932) |
| `0x05` | LZ4 | LZ4 framed streaming compression |
| `0x06` | PRZM | PRZM Unified Multi-Dimensional Manifold Compression (PRZM-SPEC-0.5) |

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

### 5.1. Container Compaction and Garbage Collection (Vacuum Protocol)

Repeated append-based delta updates cause superseded chunks and orphaned directory tables to remain inside the file as unreferenced dead space. Tooling MAY execute a vacuum compaction pass to reclaim dead space without modifying the logical directory structure:

1. **Identification Phase**:
   - The reader resolves the active Central Directory (`CTOC` and `INDX`) via the 32-byte trailer at physical EOF.
   - All actively referenced chunks are marked:
     $$\text{LiveChunks} = \{ c \in \mathbb{N} \mid \exists e \in \text{INDX}, e.\text{ChunkIndex} \le c < e.\text{ChunkIndex} + e.\text{ChunkCount} \}$$
2. **Relocation Phase**:
   - A compacted container file is created (or an in-place sliding compaction is executed).
   - Only `LiveChunks` are sequentially copied forward to contiguous LBA-aligned offsets.
   - An index remapping table $R[\text{OldChunkIndex}] \to \text{NewChunkIndex}$ tracks updated positions.
3. **Catalog Rewriting**:
   - `CTOC` entries are updated with the newly assigned contiguous `BlockIndex` coordinates.
   - In `INDX`, each extent's `ChunkIndex` is remapped: $e.\text{ChunkIndex} = R[e.\text{ChunkIndex}]$.
   - All `Name`, `Target`, `ExtraTLVs`, timestamps, and BLAKE3/SHA-256 hashes remain 100% bit-for-bit identical.
4. **Atomic Replacement**:
   - A revised `CTOC`, `INDX`, and Archive Trailer are written.
   - The compacted container is flushed to disk and atomically renamed over the source container.

## 6. Binary Reference Dump

### 6.1. Archive Header (Offset 0x0000)
```text
Offset  Hex Data                                        Interpretation
0x0000  45 4F 4E 21                                     Magic: 'EON!'
0x0004  00 01                                           Version 0.1
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
        Read fixed record prefix [58 bytes]
        Namespace    := prefix.Namespace
        ChunkIndex   := prefix.ChunkIndex
        ChunkCount   := prefix.ChunkCount
        ExtentFlags  := prefix.ExtentFlags
        Timestamp    := prefix.TimestampNano
        PayloadHash  := prefix.PayloadBLAKE3
        TargetLength := prefix.TargetLength
        NameLength   := prefix.NameLength
        ExtraLength  := prefix.ExtraLength
        ChunkOffset  := ReadULEB128(F)
        PayloadSize  := ReadULEB128(F)
        Target       := ReadBytes(F, TargetLength)
        Name         := ReadBytes(F, NameLength)
        ExtraTLVs    := ReadBytes(F, ExtraLength)
        M[Name] := {
            Namespace:   Namespace,
            ChunkIndex:  ChunkIndex,
            ChunkCount:  ChunkCount,
            ChunkOffset: ChunkOffset,
            PayloadSize: PayloadSize,
            Target:      Target,
            ExtraTLVs:   ExtraTLVs
        }

18. If (Trailer.FlagsCopy & 0x0004) != 0: // FLAG_DIGITAL_SIGNATURE
        Read DSIG Header [14 bytes]
        If DSIG.SignatureMagic != 0x47495344 ('DSIG'):
            Raise InvalidSignatureFormatError
        KeyID     := ReadBytes(F, DSIG.KeyIDLength)
        PublicKey := ReadBytes(F, DSIG.PublicKeyLength)
        Signature := ReadBytes(F, DSIG.SignatureLength)
        SigRecord := { Algorithm: DSIG.AlgorithmID, KeyID: KeyID, PublicKey: PublicKey, Signature: Signature }
    Else:
        SigRecord := nil

19. Return (M, T, S, SigRecord)
```

### 7.2. Algorithm 2: Extent Extraction (Single and Multi-Chunk)

```text
Input:  File handle F, Extent descriptor E, block table T, block shift S
Output: Extracted uncompressed byte sequence B

1. Allocate byte buffer B of length E.PayloadSize
2. ReadTotal := 0
3. CurChunk  := E.ChunkIndex
4. CurOffset := E.ChunkOffset

5. While ReadTotal < E.PayloadSize:
       PhysicalByteOffset := T[CurChunk].BlockIndex << S
       PayloadStream := InitializeDecompressor(F, PhysicalByteOffset + 64, T[CurChunk].CompressedSize)
       
       If CurChunk has micro-framing (ChunkFlags & 0x0004):
           TargetCheckpoint := floor(CurOffset / 65536) * 65536
           SeekStreamToCheckpoint(PayloadStream, TargetCheckpoint)
           LocalOffset := CurOffset - TargetCheckpoint
       Else:
           LocalOffset := CurOffset
           
       DiscardBytes(PayloadStream, LocalOffset)
       Avail  := T[CurChunk].UncompressedSize - CurOffset
       ToRead := min(E.PayloadSize - ReadTotal, Avail)
       ReadBytesInto(PayloadStream, B[ReadTotal : ReadTotal + ToRead])
       CloseDecompressor(PayloadStream)
       
       ReadTotal := ReadTotal + ToRead
       CurChunk  := CurChunk + 1
       CurOffset := 0

6. Return B
```

### 7.3. Algorithm 3: Central Directory Signature Verification

```text
Input:  File handle F, CentralDirOffset, DSIG Record SigRecord, Expected PublicKey K_exp
Output: Boolean (True if signature is valid, False otherwise)

1. If SigRecord == nil:
       Raise MissingSignatureError
2. If K_exp != nil and SigRecord.PublicKey != K_exp:
       Raise KeyMismatchError

3. DirectoryLength := (OffsetOf(SigRecord) - CentralDirOffset)
4. Seek to CentralDirOffset in F
5. DirectoryBytes := ReadBytes(F, DirectoryLength) // Exactly [CTOC || INDX]

6. Digest := SHA-256(DirectoryBytes)
7. Match SigRecord.Algorithm:
       Case 0x0001 (SIG_ED25519):
           IsValid := Ed25519Verify(SigRecord.PublicKey, Digest, SigRecord.Signature)
       Default:
           Raise UnsupportedAlgorithmError

8. Return IsValid
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
