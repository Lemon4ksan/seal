// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package server provides a zero-copy HTTP Range CDN edge gateway for EON containers.
// It serves raw content-addressed chunks with immutable cache headers and streams
// virtual file extents with full HTTP 206 Range support.
package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"mime"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/engine"
)

// Config configures the CDN edge server.
type Config struct {
	Addr       string
	Port       int
	CacheDir   string
	AuthToken  string
	EnableCORS bool
}

// Server provides HTTP file and chunk serving backed by an EON container Reader.
type Server struct {
	reader   *engine.Reader
	cfg      Config
	mux      *http.ServeMux
	fileMap  map[string]catalog.Extent
	chunkMap map[string]uint32 // hexHash -> chunkIdx
}

// New creates a new CDN Edge Server for reader.
func New(reader *engine.Reader, cfg Config) *Server {
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1"
	}

	s := &Server{
		reader:   reader,
		cfg:      cfg,
		mux:      http.NewServeMux(),
		fileMap:  make(map[string]catalog.Extent),
		chunkMap: make(map[string]uint32),
	}

	// Index extents
	for _, ext := range reader.Extents() {
		clean := strings.TrimPrefix(path.Clean("/"+ext.Name), "/")
		s.fileMap[clean] = ext
	}

	// Index chunks if hashes are available
	ctoc := reader.CTOC()
	if ctoc != nil {
		for i := range ctoc.Entries {
			idx := uint32(i)
			if h, err := reader.ChunkHash(idx); err == nil {
				hexH := hex.EncodeToString(h[:])
				s.chunkMap[hexH] = idx
			}
		}
	}

	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/v1/info", s.handleInfo)
	s.mux.HandleFunc("/chunks/", s.handleChunks)
	s.mux.HandleFunc("/index.eon", s.handleIndexEON)
	s.mux.HandleFunc("/", s.handleFiles)
}

// ServeHTTP handles HTTP requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.cfg.EnableCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Range, Authorization, Content-Type")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges, ETag")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	if s.cfg.AuthToken != "" {
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + s.cfg.AuthToken
		if auth != expected && r.URL.Query().Get("token") != s.cfg.AuthToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP CDN edge server.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Addr, s.cfg.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	defer listener.Close()

	srv := &http.Server{
		Handler:      s,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	return srv.Serve(listener)
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	extents := s.reader.Extents()
	var totalUncompressed uint64
	for _, ext := range extents {
		totalUncompressed += ext.PayloadSize
	}

	snaps := s.reader.Snapshots()
	snapList := make([]map[string]any, len(snaps))
	for i, sn := range snaps {
		snapList[i] = map[string]any{
			"id":        sn.ID,
			"message":   sn.Message,
			"timestamp": sn.Timestamp,
			"files":     sn.FileCount,
		}
	}

	info := map[string]any{
		"version":           engine.Version,
		"total_files":       len(extents),
		"total_chunks":      s.reader.TotalChunks(),
		"uncompressed_size": totalUncompressed,
		"encrypted":         s.reader.IsEncrypted(),
		"has_signature":     s.reader.HasSignature(),
		"snapshots":         snapList,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(info)
}

func (s *Server) handleChunks(w http.ResponseWriter, r *http.Request) {
	// e.g. /chunks/ab/cd/abcdef123... or /chunks/abcdef123...
	reqPath := strings.TrimPrefix(r.URL.Path, "/chunks/")
	parts := strings.Split(reqPath, "/")
	rawHash := parts[len(parts)-1]
	rawHash = strings.TrimSuffix(rawHash, ".chnk")

	chunkIdx, ok := s.chunkMap[rawHash]
	if !ok {
		http.Error(w, "Chunk not found", http.StatusNotFound)
		return
	}

	rawChunk, err := s.reader.RawChunkBytes(chunkIdx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving chunk: %v", err), http.StatusInternalServerError)
		return
	}

	// Immutable CDN Cache Headers!
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, rawHash))
	w.Header().Set("Content-Type", "application/octet-stream")

	http.ServeContent(w, r, rawHash, time.Time{}, bytes.NewReader(rawChunk))
}

func (s *Server) handleIndexEON(w http.ResponseWriter, r *http.Request) {
	indexBytes, err := s.reader.IndexBytes()
	if err != nil {
		http.Error(w, fmt.Sprintf("Index not available: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=60") // Re-validate index frequently
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, "index.eon", time.Time{}, bytes.NewReader(indexBytes))
}

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if cleanPath == "" || cleanPath == "." {
		// Directory listing at root
		s.serveDirectory(w, r, "")
		return
	}

	ext, ok := s.fileMap[cleanPath]
	if !ok {
		// Check if it's a directory
		prefix := cleanPath + "/"
		var isDir bool
		for f := range s.fileMap {
			if strings.HasPrefix(f, prefix) {
				isDir = true
				break
			}
		}
		if isDir {
			s.serveDirectory(w, r, cleanPath)
			return
		}

		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	data, err := s.reader.ReadExtent(ext.Name)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading file: %v", err), http.StatusInternalServerError)
		return
	}

	// Set content type
	ctype := mime.TypeByExtension(filepath.Ext(ext.Name))
	if ctype == "" {
		ctype = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Accept-Ranges", "bytes")

	modTime := time.Unix(0, ext.Timestamp)
	http.ServeContent(w, r, filepath.Base(ext.Name), modTime, bytes.NewReader(data))
}

func (s *Server) serveDirectory(w http.ResponseWriter, r *http.Request, dir string) {
	prefix := dir
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	entries := make(map[string]bool)
	for f := range s.fileMap {
		if strings.HasPrefix(f, prefix) {
			rel := strings.TrimPrefix(f, prefix)
			part := strings.Split(rel, "/")[0]
			entries[part] = strings.Contains(rel, "/")
		}
	}

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		items := make([]map[string]any, 0, len(entries))
		for name, isSubDir := range entries {
			items = append(items, map[string]any{
				"name":   name,
				"is_dir": isSubDir,
			})
		}
		_ = json.NewEncoder(w).Encode(items)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	escapedDir := html.EscapeString(dir)
	fmt.Fprintf(w, "<!DOCTYPE html><html><head><title>Index of /%s</title>", escapedDir)
	fmt.Fprintf(
		w,
		"<style>body{font-family:monospace;padding:20px;} a{text-decoration:none;} a:hover{text-decoration:underline;}</style></head><body>",
	)
	fmt.Fprintf(w, "<h2>Aeon CDN Gateway: /%s</h2><hr/><ul>", escapedDir)
	if dir != "" {
		parent := path.Dir(strings.TrimSuffix(dir, "/"))
		if parent == "." {
			parent = ""
		}
		escapedParent := html.EscapeString(parent)
		fmt.Fprintf(w, "<li><a href=\"/%s\">📁 ../</a></li>", escapedParent)
	}
	for name, isSubDir := range entries {
		icon := "📄"
		target := path.Join("/", dir, name)
		if isSubDir {
			icon = "📁"
			name += "/"
		}
		escapedTarget := html.EscapeString(target)
		escapedName := html.EscapeString(name)
		fmt.Fprintf(w, "<li>%s <a href=\"%s\">%s</a></li>", icon, escapedTarget, escapedName)
	}
	fmt.Fprintf(w, "</ul><hr/><small>Served by Aeon Edge CDN v%s</small></body></html>", engine.Version)
}
