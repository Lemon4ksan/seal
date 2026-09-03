# SEAL Container Format Specification (v1.0)
**Document Status**: Standard Reference Specification  
**Classification**: Public Domain / Universal Open Standard  
**Target File Extensions**: `.seal`  
**MIME Type**: `application/x-seal-archive`  
**Byte Order**: Little-Endian (all multibyte numeric values unless explicitly noted)  

## 1. Abstract & Architectural Philosophy

The **SEAL Container Format** is an open, high-performance, self-healing archive and container layer standard. It is engineered specifically to eliminate the half-century-old architectural bottlenecks of `tar.gz` and `zip` in modern cloud-native infrastructures, edge devices, and serverless runtimes.

### Core Design Guarantees:
1. **Instant Cold Starts ($O(1)$ Seeking)**: Central Directory located at a fixed offset from the end of the file allows container runtimes (such as `niko`) to fetch the catalog via a single HTTP Range Request and mount filesystems in under **3 milliseconds** without full image extraction.
2. **Deterministic Memory Ceiling ($\le 32$ MB RAM)**: Payload data is partitioned into independent Solid Chunks of at most 32 MB uncompressed bytes. No decompression operation in SEAL can exceed this limit, eliminating `OOMKilled` crashes under strict Kubernetes/cgroup quotas.
3. **Resilience to Bit-Rot (Self-Healing)**: An optional Cauchy Reed-Solomon erasure coding parity layer over $GF(2^8)$ mathematically reconstructs damaged or corrupted blocks on decaying storage media.
4. **Decoupled Architecture**: Transport packaging (directory trees, chunk index, xattrs, permissions, Reed-Solomon parity, Ed25519 signatures) is strictly segregated from the compression codec (Zstandard, OpenZL Graph VM, Store, LZMA2).
5. **Zero-Dependency Implementability**: Any competent engineer can implement a fully compliant, self-contained SEAL reader in an evening in ~200 lines of C, Go, Rust, or Python using only standard libraries.

## 2. File Organization Overview

A compliant `.seal` archive exhibits strict binary symmetry with fixed 32-byte boundaries at both file extremities:

```text
+-------------------------------------------------------------------------------+
| 1. GLOBAL ARCHIVE HEADER (32 Bytes Fixed at Offset 0x00)                     |
|    Magic 'SEAL', Version, Container Kind, OS, Arch, Flags, Chunk Target Size  |
+-------------------------------------------------------------------------------+
| 2. CHUNK SEQUENCE (Contiguous Solid Chunks 0 .. N-1)                          |
|    +------------------------------------------------------------------------+ |
|    | Chunk 0: Header (64 B) + Compressed Payload (<= 32 MB) + Pad (0..3 B)  | |
|    +------------------------------------------------------------------------+ |
|    | Chunk 1: Header (64 B) + Compressed Payload (<= 32 MB) + Pad (0..3 B)  | |
|    +------------------------------------------------------------------------+ |
|    | ...                                                                    | |
|    +------------------------------------------------------------------------+ |
|    | Chunk N-1: Header (64 B) + Compressed Payload (<= 32 MB) + Pad (0..3 B)| |
|    +------------------------------------------------------------------------+ |
+-------------------------------------------------------------------------------+
| 3. REED-SOLOMON RECOVERY RECORD (Optional Parity Stream, 1% - 50% Size)       |
|    Magic 'RECV', Galois Field GF(2^8) Parity Blocks                           |
+-------------------------------------------------------------------------------+
| 4. CENTRAL DIRECTORY (Variable Size)                                          |
|    Magic 'INDX', File Records, Modes, UID/GID, Nano MTime, BLAKE3, Extra TLVs |
+-------------------------------------------------------------------------------+
| 5. END-OF-ARCHIVE FOOTER (32 Bytes Fixed at Exact [EOF - 32B])               |
|    Central Directory Offset & Size, Recovery Offset, Flags, Magic '!LAES'     |
+-------------------------------------------------------------------------------+
```

## 3. Structural Byte-Level Specifications

### 3.1. Global Archive Header (32 Bytes Fixed)

Located at byte offset `0x00000000` of the file:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `Magic` | `[4]byte` | ASCII `'S', 'E', 'A', 'L'` (`0x4C414553` Little-Endian) |
| `0x04` | 1 | `VersionMajor` | `uint8` | Format major version. Standard = `0x01` |
| `0x05` | 1 | `VersionMinor` | `uint8` | Format minor revision. Standard = `0x00` |
| `0x06` | 1 | `ContainerKind` | `uint8` | Semantic payload classification (see Table 3.1.1) |
| `0x07` | 1 | `TargetOS` | `uint8` | Target operating system (see Table 3.1.2) |
| `0x08` | 1 | `TargetArch` | `uint8` | Target machine architecture (see Table 3.1.3) |
| `0x09` | 1 | `Reserved1` | `uint8` | Reserved byte. Must be `0x00` |
| `0x0A` | 2 | `Flags` | `uint16` | Global bitmask flags (see Table 3.1.4) |
| `0x0C` | 4 | `ChunkTargetSize`| `uint32` | Max uncompressed chunk size in bytes (`0x02000000` = 32 MB) |
| `0x10` | 8 | `CreatedTimeNano`| `int64` | Creation timestamp in nanoseconds since Unix epoch |
| `0x18` | 4 | `HeaderCRC32` | `uint32` | IEEE 802.3 CRC32 of bytes `0x00..0x17` |
| `0x1C` | 4 | `Reserved2` | `uint32` | Reserved for alignment. Must be `0x00000000` |

#### Table 3.1.1: Container Kind (`ContainerKind`)
* `0x00`: `KIND_GENERIC` — Standard multi-file archive.
* `0x01`: `KIND_OCI_LAYER` — Container image rootfs layer (optimized for `niko` / containerd).
* `0x02`: `KIND_ROOTFS` — Complete bootable OS root filesystem image.
* `0x03`: `KIND_DB_SNAPSHOT` — Database snapshot / WAL dump (e.g. `dawn` DB).
* `0x04`: `KIND_APP_BUNDLE` — Self-contained runnable application bundle.

#### Table 3.1.2: Target Operating System (`TargetOS`)
* `0x00`: Universal / OS-Agnostic
* `0x01`: Linux
* `0x02`: Windows
* `0x03`: Darwin (macOS / iOS)
* `0x04`: FreeBSD

#### Table 3.1.3: Target Architecture (`TargetArch`)
* `0x00`: Universal / Architecture-Agnostic
* `0x01`: amd64 (x86_64)
* `0x02`: arm64 (aarch64)
* `0x03`: riscv64
* `0x04`: WebAssembly (Wasm)

#### Table 3.1.4: Header Flags (`Flags`)
* `Bit 0` (`0x0001`): `HAS_CENTRAL_DIRECTORY` — Archive includes `INDX` table at EOF.
* `Bit 1` (`0x0002`): `HAS_RECOVERY_RECORD` — Archive includes `RECV` Reed-Solomon parity.
* `Bit 2` (`0x0004`): `HAS_DIGITAL_SIGNATURE` — Extra Fields contain verified Ed25519 signature.
* `Bit 3` (`0x0008`): `IS_ENCRYPTED` — Solid chunk payloads are encrypted with AEAD.
* `Bit 4` (`0x0010`): `IS_STREAM_ONLY` — Generated on a sequential non-seekable pipe (no footer).

### 3.2. Chunk Header (64 Bytes Fixed)

Files are partitioned into discrete solid chunks of up to `ChunkTargetSize` uncompressed bytes. Each chunk begins with a 64-byte aligned header:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `ChunkMagic` | `[4]byte` | ASCII `'C', 'H', 'N', 'K'` (`0x4B4E4843`) |
| `0x04` | 4 | `ChunkIndex` | `uint32` | Zero-based sequence number (`0`, `1`, `2`, ...) |
| `0x08` | 1 | `CodecID` | `uint8` | Compression codec identifier (see Section 4) |
| `0x09` | 1 | `CodecLevel` | `uint8` | Codec compression level (e.g. 1..22 for Zstd) |
| `0x0A` | 2 | `ChunkFlags` | `uint16` | Bit 0: IsSolid (`0x01`), Bit 1: IsEncrypted (`0x02`) |
| `0x0C` | 8 | `UncompressedSize`| `uint64`| Uncompressed size of all file payloads in this chunk |
| `0x14` | 8 | `CompressedSize` | `uint64` | Byte length of compressed payload immediately following |
| `0x1C` | 4 | `FileCount` | `uint32` | Number of files packaged within this chunk |
| `0x20` | 32 | `UncompressedHash`| `[32]byte`| BLAKE3-256 cryptographic hash of uncompressed data |

Immediately following byte `0x3F`, exactly `CompressedSize` bytes of payload are stored.  
If `CompressedSize % 4 != 0`, zero-padding is appended to align the next header to a 4-byte boundary.

### 3.3. Central Directory Section

Located towards the tail of the archive. Begins with an 8-byte index header followed by variable-length file entry records.

#### 3.3.1. Index Header (8 Bytes)
| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `IndexMagic` | `[4]byte` | ASCII `'I', 'N', 'D', 'X'` (`0x58444E49`) |
| `0x04` | 4 | `TotalFiles` | `uint32` | Total count of file and directory records in index |

#### 3.3.2. File Record Structure (Variable Length)
Each entry in the Central Directory is structured as follows:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 2 | `RecordLength` | `uint16` | Total size of this record in bytes (including variable fields) |
| `0x02` | 4 | `ChunkIndex` | `uint32` | ID of the solid chunk containing this file's data |
| `0x06` | 8 | `ChunkOffset` | `uint64` | Byte offset of file payload inside uncompressed chunk |
| `0x0E` | 8 | `UncompressedSize`| `uint64`| Exact uncompressed file size in bytes |
| `0x16` | 4 | `FileMode` | `uint32` | Unix permissions and file type bitmask (see Table 3.3.2) |
| `0x1A` | 4 | `UID` | `uint32` | POSIX User ID |
| `0x1E` | 4 | `GID` | `uint32` | POSIX Group ID |
| `0x22` | 8 | `ModTimeNano` | `int64` | Modification timestamp in nanoseconds since Unix epoch |
| `0x2A` | 32 | `FileBLAKE3` | `[32]byte`| BLAKE3 hash of uncompressed file content |
| `0x4A` | 2 | `LinkTargetLen` | `uint16` | Length of symlink target path (0 if not a link) |
| `0x4C` | 2 | `NameLength` | `uint16` | Length of UTF-8 filename in bytes |
| `0x4E` | 2 | `ExtraLength` | `uint16` | Length of Extra Fields section |
| `0x50` | `LinkTargetLen` | `LinkTarget` | `[]byte` | Symlink destination string |
| `...` | `NameLength` | `Name` | `[]byte` | Normalized UTF-8 filename (no leading `/`, forward slashes) |
| `...` | `ExtraLength` | `ExtraFields` | `[]byte` | TLV-encoded extension attributes |

#### Table 3.3.2: FileMode Bitmask Layout (32 Bits)
```
Bits  0..8  : Standard POSIX permissions (0o777: rwxrwxrwx)
Bit   9     : SUID (Set User ID)
Bit   10    : SGID (Set Group ID)
Bit   11    : Sticky Bit
Bits  12..15: File Type:
              0x1: Regular File (S_IFREG)
              0x2: Directory (S_IFDIR)
              0x3: Symbolic Link (S_IFLNK)
              0x4: Hard Link
              0x5: FIFO / Named Pipe (S_IFIFO)
              0x6: Unix Domain Socket (S_IFSOCK)
              0x7: Character Device (S_IFCHR)
              0x8: Block Device (S_IFBLK)
Bits  16..31: Reserved (must be 0)
```

#### 3.3.3. Extra Field TLV Registry (Type-Length-Value)
Extra fields allow arbitrary first-party and cloud-native metadata without breaking format compatibility:
* `Tag` (`uint16`): 2-byte identifier.
* `Length` (`uint16`): Byte length of payload.
* `Data` (`[Length]byte`): Serialized payload.

| Tag | Hex | Name | Payload Description |
|---|---|---|---|
| `'SI'` | `0x5349` | **Digital Signature** | Ed25519 public key (32B) + signature (64B) + Signer ID |
| `'PV'` | `0x5056` | **Build Provenance** | Git commit SHA (20B/32B), branch, tag, repository URL |
| `'EP'` | `0x4550` | **Container Entrypoint** | Command line string to execute upon container launch |
| `'EV'` | `0x4556` | **Environment Variables**| Null-separated `KEY=VALUE\0` list for container runtimes |
| `'XT'` | `0x5854` | **Extended Attributes** | Security labels (e.g. `security.selinux`), POSIX ACLs |

### 3.4. Reed-Solomon Recovery Record Section

SEAL incorporates native self-healing using Cauchy Reed-Solomon erasure coding over the finite field $GF(2^8)$.

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 4 | `RecoveryMagic` | `[4]byte` | ASCII `'R', 'E', 'C', 'V'` (`0x56434552`) |
| `0x04` | 1 | `FieldPolynomial` | `uint8` | Primitive poly: `0x01` = $x^8 + x^4 + x^3 + x^2 + 1$ ($0x11D$) |
| `0x05` | 1 | `ParityPercentage` | `uint8` | Parity ratio (e.g. `3` = 3% redundant parity data) |
| `0x06` | 2 | `ShardSizeKB` | `uint16` | Size of each parity shard in kilobytes (default `64` KB) |
| `0x08` | 8 | `ProtectedOffset` | `uint64` | Starting byte offset of protected range (usually `0x00`) |
| `0x10` | 8 | `ProtectedLength` | `uint64` | Number of bytes covered by recovery coding |
| `0x18` | 4 | `TotalDataShards` | `uint32` | Number of input data shards ($K$) |
| `0x1C` | 4 | `TotalParityShards`| `uint32` | Number of generated parity shards ($M$) |
| `0x20` | `M * ShardSize` | `ParityData` | `[]byte` | Contiguous Reed-Solomon parity shards |

#### Mathematical Repair Guarantee:
Given $K$ data shards and $M$ parity shards, **any combination of up to $M$ damaged or missing shards** can be reconstructed with 100% mathematical precision using Cauchy matrix inversion over $GF(2^8)$.

### 3.5. End-of-Archive Footer (32 Bytes Fixed)

The Footer is **always located at the exact final 32 bytes** of the file (`file_size - 32` to `file_size`). This permits $O(1)$ discovery of archive indices without reading earlier contents:

| Offset | Length | Field Name | Type | Description |
|---|---|---|---|---|
| `0x00` | 8 | `CentralDirOffset` | `uint64` | Absolute byte offset to start of `INDX` section |
| `0x08` | 8 | `CentralDirLength` | `uint64` | Total byte length of `INDX` section |
| `0x10` | 8 | `RecoveryOffset` | `uint64` | Absolute byte offset to `RECV` section (`0` if absent) |
| `0x18` | 4 | `FlagsCopy` | `uint32` | Copy of global archive flags for instant inspection |
| `0x1C` | 4 | `TrailerMagic` | `[4]byte` | ASCII `'!', 'L', 'A', 'E', 'S'` (`0x53414521`, `'!EAS'`) |

## 4. Codec Registry

SEAL defines standard 1-byte codec identifiers in the Chunk Header (`CodecID`):

| Codec ID | Name | Description | Standard Reference |
|---|---|---|---|
| `0x00` | **STORE** | Raw uncompressed byte transfer (zero-copy) | None |
| `0x01` | **ZSTD** | Zstandard (Default general-purpose compressor) | IETF RFC 8878 |
| `0x02` | **OPENZL** | Meta OpenZL Format-Aware Graph VM | DAG Transform + FSE |
| `0x03` | **LZMA2** | High-ratio dictionary coder | 7-Zip LZMA2 |
| `0x04` | **BROTLI** | Web & text-optimized LZ77 | IETF RFC 7932 |

## 5. Concrete Byte Walkthrough & Hex Dumps

### 5.1. Example Global Header (32 Bytes)
```text
Offset  Bytes                                           Description
0x0000  53 45 41 4C                                     Magic: 'SEAL'
0x0004  01 00                                           Version 1.0
0x0006  01                                              ContainerKind: 0x01 (OCI Container Layer)
0x0007  01                                              TargetOS: 0x01 (Linux)
0x0008  01                                              TargetArch: 0x01 (amd64)
0x0009  00                                              Reserved1: 0x00
0x000A  07 00                                           Flags: HAS_INDEX | HAS_RECOVERY | HAS_SIG
0x000C  00 00 00 02                                     ChunkTargetSize: 32 MB (0x02000000 LE)
0x0010  00 68 39 4F 35 01 00 00                         CreatedTimeNano: Unix Timestamp
0x0018  B4 82 1A F2                                     HeaderCRC32: 0xF21A82B4
0x001C  00 00 00 00                                     Reserved2: 0x00000000
```

### 5.2. Example Footer (32 Bytes at EOF)
```text
Offset  Bytes                                           Description
EOF-32  00 10 20 00 00 00 00 00                         CentralDirOffset: 0x00201000 (2.1 MB)
EOF-24  A0 04 00 00 00 00 00 00                         CentralDirLength: 1,184 Bytes
EOF-16  00 00 20 00 00 00 00 00                         RecoveryOffset: 0x00200000 (2.0 MB)
EOF-08  07 00 00 00                                     FlagsCopy: 0x00000007
EOF-04  21 4C 41 45                                     TrailerMagic: '!LAE' (0x53454121)
```

## 6. How to Implement a Reader in an Evening

A minimal, fully functioning reader can be assembled in under 100 lines of code following this exact logic:

```go
// Step 1: Read the 32-byte Footer at the very end of the file
file.Seek(-32, io.SeekEnd)
footerBytes := readExact(32)
if string(footerBytes[28:32]) != "!LAE" {
    panic("not a valid SEAL archive")
}
centralDirOffset := binary.LittleEndian.Uint64(footerBytes[0:8])
centralDirLength := binary.LittleEndian.Uint64(footerBytes[8:16])

// Step 2: Read Central Directory (INDX)
file.Seek(centralDirOffset, io.SeekStart)
indxHeader := readExact(8)
if string(indxHeader[0:4]) != "INDX" {
    panic("corrupted central directory")
}
totalFiles := binary.LittleEndian.Uint32(indxHeader[4:8])

// Step 3: Iterate through file records
for i := 0; i < totalFiles; i++ {
    recLen := readUint16()
    chunkIndex := readUint32()
    chunkOffset := readUint64()
    fileSize := readUint64()
    fileMode := readUint32()
    uid, gid := readUint32(), readUint32()
    modTime := readInt64()
    fileHash := readExact(32)
    linkLen := readUint16()
    nameLen := readUint16()
    extraLen := readUint16()
    linkTarget := readString(linkLen)
    fileName := readString(nameLen)
    extraFields := readExact(extraLen)
    
    // Register file in map: fileName -> (chunkIndex, chunkOffset, fileSize)
}

// Step 4: Extracting any file on demand
func Extract(name string) []byte {
    meta := fileMap[name]
    // Seek to Chunk Header
    file.Seek(chunkOffsetOnDisk[meta.chunkIndex], io.SeekStart)
    chunkHdr := readExact(64) // CHNK
    compressedPayload := readExact(chunkHdr.CompressedSize)
    
    // Decompress chunk (<= 32 MB uncompressed)
    uncompressedChunk := zstd.Decompress(compressedPayload)
    
    // Return file slice
    return uncompressedChunk[meta.chunkOffset : meta.chunkOffset + meta.fileSize]
}
```

## 7. Mathematical Foundations: Galois Field $GF(2^8)$

SEAL specifies a fixed, deterministic Galois Field representation:
* **Primitive Polynomial**:
  $$P(x) = x^8 + x^4 + x^3 + x^2 + 1 \quad (0x11D = 285)$$
* **Field Generator**:
  $$\alpha = 2 \quad (0x02)$$
* **Multiplication**:
  Carried out via log/antilog exponent tables (`gf_exp` and `gf_log` of 256 bytes each), enabling constant-time, SIMD-vectorized (AVX2 / ARM NEON) byte manipulation without timing attack surfaces.

## 8. Security & Cryptographic Verifications

* **Tamper Proofing**: Individual files, chunk solid blocks, and the Central Directory are hashed using **BLAKE3-256**, guaranteeing collision resistance and tree-verifiable integrity.
* **Authenticity**: When `HAS_DIGITAL_SIGNATURE` is set, an **Ed25519** signature of the Central Directory hash is embedded in an Extra Field, preventing supply-chain substitution attacks.
* **AEAD Encryption**: When `IS_ENCRYPTED` is active, each solid chunk is encrypted independently using **AES-256-GCM** or **XChaCha20-Poly1305** with a deterministic 96-bit nonce derived from `(ArchiveSalt, ChunkIndex)`.

## 9. Conformance & Compatibility Rules

1. **Future Field Ignorance**: Readers encountering unknown flags in the Central Directory or unknown Extra Field tags **MUST** ignore them and proceed normally.
2. **Chunk Budget Guarantee**: Encoders **MUST NOT** emit uncompressed solid chunks exceeding 64 MB (standard: 32 MB) to preserve the universal memory ceiling.
3. **Trailing Null Padding**: Encoders **MUST** align all records to 4-byte boundaries using zero bytes. Readers **MUST** tolerate zero padding between sections.
*SEAL Container Specification — Copyright (c) 2026 Lemon4ksan and The Seal Archival Initiative. Dedicated to the Public Domain under Creative Commons CC0 1.0 Universal.*
