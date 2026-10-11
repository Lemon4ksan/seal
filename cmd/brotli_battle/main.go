// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lemon4ksan/przm"
	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/codec/compress/brotli"
)

type WebDataSet struct {
	Name        string
	Category    string
	Description string
	Data        []byte
}

type BenchResult struct {
	DatasetName    string
	Category       string
	Algorithm      string
	LevelDesc      string
	OriginalSize   int
	CompressedSize int
	Ratio          float64
	CompSpeedMBs   float64
	DecompSpeedMBs float64
	Passed         bool
}

var allResults []BenchResult

func main() {
	codec.RegisterBrotliWebDictionary()
	brotliRawDict := codec.GetBrotliWebDictionaryData()

	fmt.Println("====================================================================================================")
	fmt.Println("              PRZM 2.0 vs GOOGLE BROTLI: THE COMPREHENSIVE WEB COMPRESSION ARENA                    ")
	fmt.Println("====================================================================================================")
	fmt.Println("Brotli uses a pre-baked 122KB static dictionary tailored for Web text (RFC 7932).                   ")
	fmt.Println("PRZM injects external prefix dictionaries agnostically via DictID (Zero Engine Pollution).          ")
	fmt.Println("====================================================================================================")

	datasets := []WebDataSet{
		generateMicroJSONPayload(16 * 1024),
		generateSemanticHTML5(128 * 1024),
		generateMinifiedJSBundle(256 * 1024),
		generateTailwindCSS(128 * 1024),
		generateTechnicalDoc(256 * 1024),
		generateServerLogStream(1024 * 1024),
	}

	for _, ds := range datasets {
		fmt.Printf("\n>>> ARENA: [%s] %s (%.1f KiB)\n", ds.Category, ds.Name, float64(len(ds.Data))/1024.0)
		fmt.Printf("    Description: %s\n", ds.Description)
		fmt.Printf("    %-24s | %-12s | %-10s | %-8s | %-13s | %-14s | %-6s\n",
			"Algorithm", "Role / Level", "Size (KiB)", "Ratio", "Comp (MB/s)", "Decomp (MB/s)", "Exact")
		fmt.Println("    -------------------------+--------------+------------+----------+---------------+----------------+-------")

		// 1. Classic Web Baseline: Gzip (Deflate) Level 6
		runGzip(ds, 6)

		// 2. Google Brotli Level 4 (CDN Stream Default)
		runBrotli(ds, "Brotli (L4 Dynamic)", "CDN Stream", 4)

		// 3. Google Brotli Level 9 (Static Pre-compressed)
		runBrotli(ds, "Brotli (L9 Static)", "High Static", 9)

		// 4. Google Brotli Level 11 (Max Compression)
		runBrotli(ds, "Brotli (L11 Ultra)", "Max Density", 11)

		// 5. PRZM Level 1 (Real-Time Geometric, Cold Zero-Dict)
		runPRZM(ds, "PRZM (L1 Cold Zero-Dict)", "Realtime", 1, nil, 0, 4, false)

		// 6. PRZM Level 5 (High Density, Cold Zero-Dict)
		runPRZM(ds, "PRZM (L5 Cold Zero-Dict)", "High Density", 5, nil, 0, 4, false)

		// 7. PRZM Level 1 + Injected Brotli Web Dictionary (122KB RFC 7932)
		runPRZM(ds, "PRZM (L1 + Brotli Dict)", "Injected L1", 1, brotliRawDict, codec.DictIDBrotliWeb, 4, false)

		// 8. PRZM Level 5 + Injected Brotli Web Dictionary (122KB RFC 7932)
		runPRZM(ds, "PRZM (L5 + Brotli Dict)", "Injected L5", 5, brotliRawDict, codec.DictIDBrotliWeb, 4, false)

		// 9. PRZM Level 5 + MinMatch=3
		runPRZM(ds, "PRZM (L5 + MM=3)", "MinMatch 3", 5, brotliRawDict, codec.DictIDBrotliWeb, 3, false)

		// 10. PRZM Level 5 + CompactDict
		runPRZM(ds, "PRZM (L5 + CompactDict)", "CompactDict", 5, brotliRawDict, codec.DictIDBrotliWeb, 4, true)

		// 11. PRZM Level 6 (Deep Hensel-Ultrametric Multi-Tier Search)
		runPRZM(ds, "PRZM (L6 Hensel-Ultra)", "Level 6 Ultra", 6, brotliRawDict, codec.DictIDBrotliWeb, 3, false)
	}

	printSummaryReport()
}

func recordAndPrint(r BenchResult) {
	allResults = append(allResults, r)
	status := "PASS"
	if !r.Passed {
		status = "FAIL"
	}
	fmt.Printf("    %-24s | %-12s | %8.2f KB | %7.2f%% | %10.1f MB/s | %10.1f MB/s | %-6s\n",
		r.Algorithm, r.LevelDesc, float64(r.CompressedSize)/1024.0, r.Ratio, r.CompSpeedMBs, r.DecompSpeedMBs, status)
}

func runGzip(ds WebDataSet, level int) {
	data := ds.Data
	var compBytes []byte
	cIters := 0
	start := time.Now()
	for {
		var buf bytes.Buffer
		gw, _ := gzip.NewWriterLevel(&buf, level)
		_, _ = gw.Write(data)
		_ = gw.Close()
		compBytes = buf.Bytes()
		cIters++
		if time.Since(start) >= 20*time.Millisecond || cIters >= 50 {
			break
		}
	}
	compDuration := time.Since(start)
	compSpeed := float64(len(data)*cIters) / compDuration.Seconds() / (1024 * 1024)

	dIters := 0
	start = time.Now()
	passed := false
	for {
		gr, err := gzip.NewReader(bytes.NewReader(compBytes))
		if err == nil {
			var out bytes.Buffer
			_, _ = io.Copy(&out, gr)
			_ = gr.Close()
			if dIters == 0 {
				passed = bytes.Equal(data, out.Bytes())
			}
		}
		dIters++
		if time.Since(start) >= 20*time.Millisecond || dIters >= 100 {
			break
		}
	}
	decompDuration := time.Since(start)
	decompSpeed := float64(len(data)*dIters) / decompDuration.Seconds() / (1024 * 1024)

	recordAndPrint(BenchResult{
		DatasetName:    ds.Name,
		Category:       ds.Category,
		Algorithm:      "Gzip (RFC 1952)",
		LevelDesc:      fmt.Sprintf("Level %d", level),
		OriginalSize:   len(data),
		CompressedSize: len(compBytes),
		Ratio:          float64(len(compBytes)) / float64(len(data)) * 100,
		CompSpeedMBs:   compSpeed,
		DecompSpeedMBs: decompSpeed,
		Passed:         passed,
	})
}

func runBrotli(ds WebDataSet, name, role string, level int) {
	data := ds.Data
	var compBytes []byte
	cIters := 0
	start := time.Now()
	for {
		var buf bytes.Buffer
		bw := brotli.NewWriterLevel(&buf, level)
		_, _ = bw.Write(data)
		_ = bw.Close()
		compBytes = buf.Bytes()
		cIters++
		if time.Since(start) >= 20*time.Millisecond || cIters >= 50 {
			break
		}
	}
	compDuration := time.Since(start)
	compSpeed := float64(len(data)*cIters) / compDuration.Seconds() / (1024 * 1024)

	dIters := 0
	start = time.Now()
	passed := false
	for {
		br := brotli.NewReader(bytes.NewReader(compBytes))
		var out bytes.Buffer
		_, _ = io.Copy(&out, br)
		if dIters == 0 {
			passed = bytes.Equal(data, out.Bytes())
		}
		dIters++
		if time.Since(start) >= 20*time.Millisecond || dIters >= 100 {
			break
		}
	}
	decompDuration := time.Since(start)
	decompSpeed := float64(len(data)*dIters) / decompDuration.Seconds() / (1024 * 1024)

	recordAndPrint(BenchResult{
		DatasetName:    ds.Name,
		Category:       ds.Category,
		Algorithm:      name,
		LevelDesc:      role,
		OriginalSize:   len(data),
		CompressedSize: len(compBytes),
		Ratio:          float64(len(compBytes)) / float64(len(data)) * 100,
		CompSpeedMBs:   compSpeed,
		DecompSpeedMBs: decompSpeed,
		Passed:         passed,
	})
}

func runPRZM(ds WebDataSet, name, role string, level int, rawDict []byte, dictID uint32, minMatch int, compactDict bool) {
	data := ds.Data
	var compBytes []byte
	var err error
	cIters := 0
	start := time.Now()
	for {
		if len(rawDict) > 0 {
			opts := przm.CompressOptions{
				Level:       level,
				RawDict:     rawDict,
				DictID:      dictID,
				MinMatch:    minMatch,
				CompactDict: compactDict,
			}
			compBytes, err = przm.CompressWithOptions(data, opts)
		} else {
			opts := przm.CompressOptions{
				Level:    level,
				MinMatch: minMatch,
			}
			compBytes, err = przm.CompressWithOptions(data, opts)
		}
		cIters++
		if time.Since(start) >= 20*time.Millisecond || cIters >= 50 {
			break
		}
	}
	compDuration := time.Since(start)
	if err != nil {
		fmt.Printf("    PRZM ERROR: %v\n", err)
		return
	}
	compSpeed := float64(len(data)*cIters) / compDuration.Seconds() / (1024 * 1024)

	dIters := 0
	start = time.Now()
	passed := false
	for {
		decompBytes, err := przm.Decompress(compBytes)
		if dIters == 0 {
			passed = err == nil && bytes.Equal(data, decompBytes)
		}
		dIters++
		if time.Since(start) >= 20*time.Millisecond || dIters >= 100 {
			break
		}
	}
	decompDuration := time.Since(start)
	decompSpeed := float64(len(data)*dIters) / decompDuration.Seconds() / (1024 * 1024)

	recordAndPrint(BenchResult{
		DatasetName:    ds.Name,
		Category:       ds.Category,
		Algorithm:      name,
		LevelDesc:      role,
		OriginalSize:   len(data),
		CompressedSize: len(compBytes),
		Ratio:          float64(len(compBytes)) / float64(len(data)) * 100,
		CompSpeedMBs:   compSpeed,
		DecompSpeedMBs: decompSpeed,
		Passed:         passed,
	})
}

func printSummaryReport() {
	fmt.Println("\n====================================================================================================")
	fmt.Println("                                EXECUTIVE SUMMARY: KEY BENCHMARK METRICS                            ")
	fmt.Println("====================================================================================================")
	fmt.Println("| Dataset | Algorithm | Compressed Size | Ratio | Comp Speed | Decomp Speed |")
	fmt.Println("| :--- | :--- | :---: | :---: | :---: | :---: |")
	for _, r := range allResults {
		fmt.Printf("| %s | %s (%s) | %.2f KB | %.2f%% | %.1f MB/s | %.1f MB/s |\n",
			r.DatasetName, r.Algorithm, r.LevelDesc, float64(r.CompressedSize)/1024.0, r.Ratio, r.CompSpeedMBs, r.DecompSpeedMBs)
	}
	fmt.Println("====================================================================================================")
}

func generateMicroJSONPayload(targetSize int) WebDataSet {
	var b strings.Builder
	b.WriteString(`{"status":"ok","code":200,"server":"edge-router-04","region":"us-east-1","timestamp":"2026-10-11T01:15:30.124Z","data":{"session_id":"sess_98f4a1c0d2","user":{"id":84192,"username":"alex_developer","email":"alex@engineering.corp","roles":["admin","billing_manager"],"profile":{"avatar":"https://cdn.corp/avatars/84192.png","organization":"Acme Systems International LLC","tier":"enterprise_plus","mfa_enabled":true,"last_login_ip":"192.168.1.105"}},"permissions":[{"scope":"read:all","expires_at":1798761600},{"scope":"write:deployments","expires_at":1798761600},{"scope":"admin:billing","expires_at":1798761600}]},"items":[`)
	for i := 0; b.Len() < targetSize-64; i++ {
		fmt.Fprintf(&b, `{"item_id":"it_%04x","sku":"SKU-%d-PRO","price":%.2f,"in_stock":true,"tags":["cloud","saas","premium"]},`, i, i, 19.99+float64(i%10))
	}
	b.WriteString(`{"item_id":"it_final","sku":"SKU-END","price":0.00,"in_stock":false}]}`)
	return WebDataSet{
		Name:        "Micro JSON API Payload (16 KiB)",
		Category:    "Micro Web API",
		Description: "Typical edge HTTP micro-response with auth tokens, user session metadata, and product array",
		Data:        []byte(b.String()[:targetSize]),
	}
}

func generateSemanticHTML5(targetSize int) WebDataSet {
	snippet := `<!DOCTYPE html><html lang="en" class="h-full bg-slate-50"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>CloudScale Infrastructure Control Plane</title><link rel="stylesheet" href="/assets/css/app.min.css"><script src="/assets/js/runtime.min.js" defer></script></head><body class="h-full font-sans antialiased text-slate-800"><div class="min-h-full flex flex-col"><header class="sticky top-0 z-40 bg-white/95 backdrop-blur border-b border-slate-200"><div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between"><div class="flex items-center space-x-6"><a href="/" class="flex items-center space-x-2 text-indigo-600 font-bold text-xl"><svg class="w-8 h-8" fill="currentColor" viewBox="0 0 24 24"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg><span>CloudScale</span></a><nav class="hidden md:flex space-x-4"><a href="/overview" class="text-slate-600 hover:text-indigo-600 px-3 py-2 text-sm font-medium">Overview</a><a href="/clusters" class="text-slate-600 hover:text-indigo-600 px-3 py-2 text-sm font-medium">Kubernetes</a><a href="/edge" class="text-slate-600 hover:text-indigo-600 px-3 py-2 text-sm font-medium">Edge Nodes</a><a href="/metrics" class="text-slate-600 hover:text-indigo-600 px-3 py-2 text-sm font-medium">Observability</a></nav></div><div class="flex items-center space-x-4"><button class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-indigo-600 hover:bg-indigo-700">Deploy Service</button></div></div></header><main class="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8"><div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8"><div class="bg-white overflow-hidden shadow rounded-lg p-5"><dt class="text-sm font-medium text-slate-500 truncate">Total Ingress Throughput</dt><dd class="mt-1 text-3xl font-semibold text-slate-900">48.2 Gbps</dd></div><div class="bg-white overflow-hidden shadow rounded-lg p-5"><dt class="text-sm font-medium text-slate-500 truncate">Mean Edge Latency (p99)</dt><dd class="mt-1 text-3xl font-semibold text-slate-900">1.82 ms</dd></div><div class="bg-white overflow-hidden shadow rounded-lg p-5"><dt class="text-sm font-medium text-slate-500 truncate">Active Global Replicas</dt><dd class="mt-1 text-3xl font-semibold text-slate-900">1,420 nodes</dd></div></div></main></div></body></html>`
	var b strings.Builder
	for b.Len() < targetSize {
		b.WriteString(snippet)
	}
	return WebDataSet{
		Name:        "Semantic HTML5 Web Application (128 KiB)",
		Category:    "Web DOM",
		Description: "Production responsive dashboard template with navigation, cards, SVG icons, and Tailwind utility classes",
		Data:        []byte(b.String()[:targetSize]),
	}
}

func generateMinifiedJSBundle(targetSize int) WebDataSet {
	chunk := `!function(e,t){"object"==typeof exports&&"object"==typeof module?module.exports=t():"function"==typeof define&&define.amd?define([],t):"object"==typeof exports?exports.App=t():e.App=t()}(self,(function(){return function(e){var t={};function n(r){if(t[r])return t[r].exports;var o=t[r]={i:r,l:!1,exports:{}};return e[r].call(o.exports,o,o.exports,n),o.l=!0,o.exports}return n.m=e,n.c=t,n.d=function(e,t,r){n.o(e,t)||Object.defineProperty(e,t,{enumerable:!0,get:r})},n.r=function(e){"undefined"!=typeof Symbol&&Symbol.toStringTag&&Object.defineProperty(e,Symbol.toStringTag,{value:"Module"}),Object.defineProperty(e,"__esModule",{value:!0})},n.t=function(e,t){if(1&t&&(e=n(e)),8&t)return e;if(4&t&&"object"==typeof e&&e&&e.__esModule)return e;var r=Object.create(null);if(n.r(r),Object.defineProperty(r,"default",{enumerable:!0,value:e}),2&t&&"string"!=typeof e)for(var o in e)n.d(r,o,function(t){return e[t]}.bind(null,o));return r},n.n=function(e){var t=e&&e.__esModule?function(){return e.default}:function(){return e};return n.d(t,"a",t),t},n.o=function(e,t){return Object.prototype.hasOwnProperty.call(e,t)},n.p="",n(n.s=102)}([function(e,t,n){"use strict";var r=n(1);Object.defineProperty(t,"__esModule",{value:!0}),t.StateController=function(e,t){return{dispatch:function(n){e.setState((function(e){return Object.assign({},e,n)}))},subscribe:function(e){return t.listeners.push(e),function(){t.listeners=t.listeners.filter((function(t){return t!==e}))}}}}]);`
	var b strings.Builder
	for b.Len() < targetSize {
		b.WriteString(chunk)
	}
	return WebDataSet{
		Name:        "Minified Webpack/React Bundle (256 KiB)",
		Category:    "JavaScript Runtime",
		Description: "Optimized and mangled ECMAScript 2022 bundle with module loaders, closures, and state dispatcher",
		Data:        []byte(b.String()[:targetSize]),
	}
}

func generateTailwindCSS(targetSize int) WebDataSet {
	snippet := `.flex{display:flex}.inline-flex{display:inline-flex}.grid{display:grid}.hidden{display:none}.h-full{height:100%}.w-full{width:100%}.max-w-7xl{max-width:80rem}.items-center{align-items:center}.justify-between{justify-content:space-between}.rounded-lg{border-radius:0.5rem}.rounded-xl{border-radius:0.75rem}.border{border-width:1px}.border-slate-200{border-color:#e2e8f0}.bg-white{background-color:#ffffff}.bg-slate-50{background-color:#f8fafc}.bg-indigo-600{background-color:#4f46e5}.text-white{color:#ffffff}.text-slate-900{color:#0f172a}.text-slate-600{color:#475569}.shadow-sm{box-shadow:0 1px 2px 0 rgba(0,0,0,0.05)}.shadow{box-shadow:0 1px 3px 0 rgba(0,0,0,0.1),0 1px 2px -1px rgba(0,0,0,0.1)}.hover\:bg-indigo-700:hover{background-color:#4338ca}.hover\:text-indigo-600:hover{color:#4f46e5}@media (min-width: 768px){.md\:flex{display:flex}}@media (min-width: 1024px){.lg\:px-8{padding-left:2rem;padding-right:2rem}}`
	var b strings.Builder
	for b.Len() < targetSize {
		b.WriteString(snippet)
	}
	return WebDataSet{
		Name:        "Tailwind CSS Stylesheet (128 KiB)",
		Category:    "CSS Framework",
		Description: "Production minified atomic utility stylesheet with responsive media queries, variables, and color classes",
		Data:        []byte(b.String()[:targetSize]),
	}
}

func generateTechnicalDoc(targetSize int) WebDataSet {
	snippet := `# RFC Specification: Hyper-Scale Distributed Event Streaming Engine

## Abstract
This document specifies the protocol semantics, framing layers, and consensus mechanisms for ultra-low latency distributed stream replication across geo-replicated heterogeneous storage clusters. The architecture guarantees strict linearizability under Raft quorum while achieving sub-millisecond p99 latency across cross-datacenter fiber interlinks.

## 1. Architectural Overview
The protocol utilizes zero-copy vectored ring buffers with hardware timestamping at the network interface card (NIC) layer. Each partition is partitioned across a deterministic topological sheaf over simplicial complexes:

1. **Section 1 (Control Plane):** Epoch synchronization and leader leases.
2. **Section 2 (Log Replication):** Multi-Paxos pipeline with pipelined commit indices.
3. **Section 3 (Storage Tiering):** Autonomous tiered compaction to append-only log segments.

### Performance Bounds
Under maximum saturation, each broker node sustains 12.8 million input/output operations per second (IOPS) with memory footprint bounded by O(log N) where N represents active TCP/QUIC stream sessions.
`
	var b strings.Builder
	for b.Len() < targetSize {
		b.WriteString(snippet)
	}
	return WebDataSet{
		Name:        "Technical Specification Markdown (256 KiB)",
		Category:    "Technical Prose",
		Description: "RFC-style engineering specification with markdown headings, lists, code tokens, and architectural text",
		Data:        []byte(b.String()[:targetSize]),
	}
}

func generateServerLogStream(targetSize int) WebDataSet {
	var b strings.Builder
	for i := 1000; b.Len() < targetSize-128; i++ {
		fmt.Fprintf(&b, `{"timestamp":"2026-10-11T01:15:%02d.%03dZ","level":"INFO","trace_id":"trc_%08x","client_ip":"192.168.1.%d","http":{"method":"POST","uri":"/api/v2/ingest/telemetry","status":200,"duration_ms":%.2f,"bytes_sent":%d},"user_agent":"CloudScale-Agent/2.4.1 (Linux x86_64) Go-http-client/2.0","service":"edge-ingest-router","region":"eu-central-1"}`+"\n",
			(i/1000)%60, i%1000, i*31, i%254+1, 1.2+float64(i%50)/10.0, 480+(i%200))
	}
	return WebDataSet{
		Name:        "Server JSON Access Logs (1 MiB)",
		Category:    "Server Telemetry",
		Description: "High-throughput cloud server access logs with repetitive schemas, IPs, timestamps, and URIs",
		Data:        []byte(b.String()[:targetSize]),
	}
}
