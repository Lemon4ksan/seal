// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HTTPReaderAt implements io.ReaderAt and io.Closer for partial reading over HTTP(S) Range requests.
type HTTPReaderAt struct {
	url        string
	client     *http.Client
	size       int64
	cache      map[string][]byte
	cacheMu    sync.RWMutex
	cacheLimit int
}

// NewHTTPReaderAt probes the remote URL for size and Range support, returning a streaming ReaderAt.
func NewHTTPReaderAt(rawURL string, client *http.Client) (*HTTPReaderAt, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	// 1. Try HEAD request to get Content-Length and Accept-Ranges
	req, err := http.NewRequestWithContext(context.Background(), http.MethodHead, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create head request: %w", err)
	}

	resp, err := client.Do(req)
	var size int64 = -1
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			size = resp.ContentLength
		}
	}

	// 2. If HEAD didn't yield size, try GET with Range: bytes=0-0 to probe range support & size
	if size <= 0 {
		getReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create range probe: %w", err)
		}
		getReq.Header.Set("Range", "bytes=0-0")
		getResp, err := client.Do(getReq)
		if err != nil {
			return nil, fmt.Errorf("probe range request: %w", err)
		}
		defer getResp.Body.Close()

		if getResp.StatusCode == http.StatusPartialContent {
			cr := getResp.Header.Get("Content-Range")
			if parts := strings.Split(cr, "/"); len(parts) == 2 {
				if total, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
					size = total
				}
			}
		} else if getResp.StatusCode == http.StatusOK {
			return nil, fmt.Errorf("remote server does not support HTTP Range requests (status %d)", getResp.StatusCode)
		} else {
			return nil, fmt.Errorf("probe failed with status: %d", getResp.StatusCode)
		}
	}

	if size <= 0 {
		return nil, fmt.Errorf("unable to determine remote file size for %s", rawURL)
	}

	return &HTTPReaderAt{
		url:        rawURL,
		client:     client,
		size:       size,
		cache:      make(map[string][]byte),
		cacheLimit: 256,
	}, nil
}

// Size returns the total size of the remote file in bytes.
func (h *HTTPReaderAt) Size() int64 {
	return h.size
}

// ReadAt reads len(p) bytes from the remote file starting at byte offset off via HTTP Range.
func (h *HTTPReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= h.size {
		return 0, io.EOF
	}

	needed := int64(len(p))
	if off+needed > h.size {
		needed = h.size - off
	}
	if needed == 0 {
		return 0, nil
	}

	cacheKey := fmt.Sprintf("%d-%d", off, off+needed)
	h.cacheMu.RLock()
	cached, ok := h.cache[cacheKey]
	h.cacheMu.RUnlock()
	if ok {
		copy(p, cached)
		if needed < int64(len(p)) {
			return int(needed), io.EOF
		}
		return int(needed), nil
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, h.url, nil)
	if err != nil {
		return 0, fmt.Errorf("create range request: %w", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+needed-1))

	resp, err := h.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http range error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("http range request failed with status %d", resp.StatusCode)
	}

	n, err := io.ReadFull(resp.Body, p[:needed])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return n, err
	}

	h.cacheMu.Lock()
	if len(h.cache) >= h.cacheLimit {
		clear(h.cache)
	}
	bufCopy := make([]byte, n)
	copy(bufCopy, p[:n])
	h.cache[cacheKey] = bufCopy
	h.cacheMu.Unlock()

	if needed < int64(len(p)) {
		return n, io.EOF
	}
	return n, nil
}

// Close releases any resources held by the reader.
func (h *HTTPReaderAt) Close() error {
	h.cacheMu.Lock()
	h.cache = nil
	h.cacheMu.Unlock()
	return nil
}

// OpenURL opens an EON container hosted remotely over HTTP(S) using partial Range requests.
func OpenURL(rawURL string, opts ...ReaderOption) (*Reader, io.Closer, error) {
	hr, err := NewHTTPReaderAt(rawURL, nil)
	if err != nil {
		return nil, nil, err
	}

	r, err := OpenReader(hr, hr.Size(), opts...)
	if err != nil {
		_ = hr.Close()
		return nil, nil, err
	}

	return r, hr, nil
}
