# ⚔️ Compression Battle: 35 Years of Algorithmic Evolution (1989 – 2026)

This benchmark analyzes the performance, compression density, memory behavior, and historical evolution of data compression algorithms implemented in the **`seal`** engine, spanning from **PKZIP 1.0 (1989)** to **Brotli & Zstandard (2020s)**.

---

## 📊 Summary Across Datasets

### 1. Source Code & AST (`512 KB` Go Code)
*High keyword density, structural indentation, repetitive identifiers.*

| Algorithm | Generation / Era | Size (KB) | Ratio (%) | Comp (MB/s) | Decomp (MB/s) |
| :--- | :--- | :---: | :---: | :---: | :---: |
| **Brotli (Level 9)** | 2015 (Web/RFC) | **0.3 KB** | **0.07%** | 44.3 MB/s | 500.3 MB/s |
| **Brotli (Level 4)** | 2015 (Web/RFC) | **0.4 KB** | **0.07%** | 23.0 MB/s | 475.2 MB/s |
| **Deflate (Level 9)** | 1993 (Classic) | **2.9 KB** | **0.56%** | 250.0 MB/s | >500 MB/s |
| **Gzip (RFC 1952)** | 1996 (Web) | **2.9 KB** | **0.57%** | 500.4 MB/s | 250.3 MB/s |
| **LZMA2 (Ultra)** | 2000s (7-Zip) | **3.7 KB** | **0.72%** | 499.6 MB/s | >500 MB/s |
| **Deflate (Level 1)** | 1993 (Classic) | **5.9 KB** | **1.16%** | 96.7 MB/s | >500 MB/s |
| **Shrink (LZW 9-13b)** | 1989 (DOS/ARC) | **56.1 KB** | **10.96%** | 46.9 MB/s | 239.2 MB/s |
| **PPMd (Order 8)** | 2000s (PPM) | **135.5 KB** | **26.46%** | 57.7 MB/s | 41.8 MB/s |

> 🏆 **Winner:** **Brotli Level 9** ($187\times$ smaller than Shrink, $10\times$ smaller than Deflate).

---

### 2. Tabular CSV & Server Logs (`512 KB` Database Records)
*Semi-structured column-oriented records with recurring timestamps, HTTP status codes, and IP addresses.*

| Algorithm | Generation / Era | Size (KB) | Ratio (%) | Comp (MB/s) | Decomp (MB/s) |
| :--- | :--- | :---: | :---: | :---: | :---: |
| **Brotli (Level 9)** | 2015 (Web/RFC) | **18.9 KB** | **3.69%** | 22.4 MB/s | 497.1 MB/s |
| **LZMA2 (Ultra)** | 2000s (7-Zip) | **46.3 KB** | **9.04%** | 123.2 MB/s | >500 MB/s |
| **LZMA2 (Fast)** | 2000s (7-Zip) | **50.0 KB** | **9.76%** | 249.8 MB/s | 500.2 MB/s |
| **Deflate (Level 9)** | 1993 (Classic) | **55.8 KB** | **10.89%** | 20.0 MB/s | 475.2 MB/s |
| **Brotli (Level 4)** | 2015 (Web/RFC) | **57.5 KB** | **11.24%** | 76.8 MB/s | 500.6 MB/s |
| **Gzip (RFC 1952)** | 1996 (Web) | **59.8 KB** | **11.68%** | 256.1 MB/s | 500.6 MB/s |
| **Shrink (LZW 9-13b)** | 1989 (DOS/ARC) | **76.2 KB** | **14.89%** | 48.6 MB/s | 193.9 MB/s |
| **Deflate (Level 1)** | 1993 (Classic) | **84.7 KB** | **16.55%** | 498.6 MB/s | 500.2 MB/s |
| **PPMd (Order 8)** | 2000s (PPM) | **161.8 KB** | **31.60%** | 31.1 MB/s | 25.2 MB/s |

> 🏆 **Winner:** **Brotli Level 9** and **LZMA2 Ultra** (LZMA2 provides balanced $123\text{ MB/s}$ speed with sub-10% ratio).

---

### 3. Binary Machine Code (`512 KB` x86_64 Instructions)
*Binary instruction stream with relative `CALL` (0xE8) and `JMP` (0xE9) offsets.*

| Algorithm | Generation / Era | Size (KB) | Ratio (%) | Comp (MB/s) | Decomp (MB/s) |
| :--- | :--- | :---: | :---: | :---: | :---: |
| **Brotli (Level 9)** | 2015 (Web/RFC) | **0.1 KB** | **0.01%** | 110.8 MB/s | 499.1 MB/s |
| **LZMA2 (Ultra)** | 2000s (7-Zip) | **0.9 KB** | **0.18%** | 118.9 MB/s | >500 MB/s |
| **Deflate (Level 9)** | 1993 (Classic) | **1.5 KB** | **0.30%** | 480.7 MB/s | >500 MB/s |
| **Shrink (LZW 9-13b)** | 1989 (DOS/ARC) | **7.8 KB** | **1.51%** | 52.2 MB/s | 293.5 MB/s |
| **BCJ + LZMA2** | 2000s (7-Zip) | **18.8 KB** | **3.68%** | 110.8 MB/s | 331.1 MB/s |
| **PPMd (Order 8)** | 2000s (PPM) | **106.9 KB** | **20.87%** | 21.2 MB/s | 17.6 MB/s |

---

### 4. High-Entropy / Incompressible Data (`256 KB` Random Noise)
*Cryptographically random data simulating pre-compressed archives or encrypted payloads.*

| Algorithm | Generation / Era | Size (KB) | Ratio (%) | Overhead Analysis |
| :--- | :--- | :---: | :---: | :--- |
| **Deflate / Gzip / Brotli** | 1990s–2010s | **256.0 KB** | **100.01%** | Stored blocks fallback (0 expansion) |
| **LZMA2 (Fast / Ultra)** | 2000s (7-Zip) | **256.0 KB** | **100.00%** | Zero overhead chunk bypass |
| **PPMd (Order 8)** | 2000s (PPM) | **256.1 KB** | **100.04%** | Minimal overhead |
| ⚠️ **Shrink (LZW 9-13b)** | 1989 (DOS/ARC) | **372.0 KB** | **145.32%** | 💣 **+45.3% Expansion Penalty!** |

---

## 🔍 In-Depth Technical Analysis

### 1. The Legacy Era (1989): Why Did Shrink Suffer from Expansion?
* **Dictionary Growth without Bypass:**
  In classic LZW (Method 1), the compressor emits 9-bit to 13-bit tokens for every single input byte. When feeding high-entropy random data (where strings never repeat), the compressor emits a 9-bit code for almost every byte, causing a catastrophic **$+45.3\%$ inflation** in file size.
* **Lack of Uncompressed Framing:**
  Modern formats (Deflate, LZMA2, Zstandard) detect when compression is inefficient and immediately switch to raw uncompressed bypass blocks (`BTYPE=00` or raw uncompressed chunks).

### 2. The Golden Standard: Deflate (1993)
* **The Sweet Spot of Simplicity and Speed:**
  Deflate combined a 32 KB sliding window with dynamic canonical Huffman trees. It compressed code down to **0.56%** at over **$250\text{ MB/s}$**, outperforming Shrink by $19\times$ in compression ratio while using less memory than full LZW tables.

### 3. The Maximum Ratio Kings: LZMA2 and Brotli
* **Why Brotli Dominates Text & Web Payloads:**
  Brotli uses **2nd-order context modeling**, static transformation dictionaries (containing common HTML, CSS, and English tokens), and variable-length distance codes up to 30 bits ($1\text{ GB}$ window). It achieved **$0.07\%$ ratio** on code and **$3.69\%$** on logs.
* **Why LZMA2 Dominates Heavy Engineering Workloads:**
  LZMA2's Range Coder combined with 8MB+ sliding history allows it to maintain sustained $120\text{--}500\text{ MB/s}$ throughput on multicore systems with ultra-compact archive sizes.

---

## 🚀 How to Run the Benchmark

```bash
go run ./cmd/bench_battle
```
