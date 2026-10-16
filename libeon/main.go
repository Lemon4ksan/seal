// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
    char     name[512];
    uint64_t size;
    uint16_t namespace_id;
    int64_t  mod_time_unix;
    uint32_t chunk_index;
    uint8_t  is_dir;
} eon_file_stat_t;
*/
import "C"

import (
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	aeon "github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

type containerEntry struct {
	r     *aeon.Reader
	f     io.Closer
	files []catalog.Extent
}

var (
	handlesMu sync.RWMutex
	handles   = make(map[uint64]*containerEntry)
	nextID    uint64

	lastErrorMu sync.RWMutex
	lastError   string
)

func setLastError(err error) {
	lastErrorMu.Lock()
	defer lastErrorMu.Unlock()
	if err == nil {
		lastError = ""
	} else {
		lastError = err.Error()
	}
}

//export eon_version
func eon_version() *C.char {
	return C.CString(aeon.Version)
}

//export eon_get_last_error
func eon_get_last_error() *C.char {
	lastErrorMu.RLock()
	defer lastErrorMu.RUnlock()
	return C.CString(lastError)
}

//export eon_open
func eon_open(cPath, cPassword *C.char, outErr *C.int) C.uint64_t {
	if cPath == nil {
		if outErr != nil {
			*outErr = C.int(-1) // EON_ERR_IO
		}
		setLastError(errors.New("null path"))
		return 0
	}

	path := C.GoString(cPath)
	var password string
	if cPassword != nil {
		password = C.GoString(cPassword)
	}

	var opts []aeon.ReaderOption
	if password != "" {
		opts = append(opts, aeon.WithReaderPassword(password))
	}

	r, f, err := aeon.OpenFile(path, opts...)
	if err != nil {
		setLastError(err)
		if outErr != nil {
			switch {
			case errors.Is(err, spec.ErrPasswordRequired):
				*outErr = C.int(-4) // EON_ERR_PASSWORD_REQUIRED
			case errors.Is(err, spec.ErrInvalidPassword):
				*outErr = C.int(-5) // EON_ERR_INVALID_PASSWORD
			case errors.Is(err, spec.ErrInvalidHeader):
				*outErr = C.int(-2) // EON_ERR_INVALID_HEADER
			case errors.Is(err, spec.ErrInvalidTrailer):
				*outErr = C.int(-3) // EON_ERR_INVALID_TRAILER
			default:
				*outErr = C.int(-1) // EON_ERR_IO
			}
		}
		return 0
	}

	id := atomic.AddUint64(&nextID, 1)
	entry := &containerEntry{
		r:     r,
		f:     f,
		files: r.Extents(),
	}

	handlesMu.Lock()
	handles[id] = entry
	handlesMu.Unlock()

	setLastError(nil)
	if outErr != nil {
		*outErr = 0 // EON_OK
	}
	return C.uint64_t(id)
}

//export eon_close
func eon_close(handle C.uint64_t) {
	id := uint64(handle)
	handlesMu.Lock()
	entry, ok := handles[id]
	if ok {
		delete(handles, id)
	}
	handlesMu.Unlock()

	if ok && entry.f != nil {
		entry.f.Close()
	}
}

//export eon_file_count
func eon_file_count(handle C.uint64_t) C.int64_t {
	handlesMu.RLock()
	entry, ok := handles[uint64(handle)]
	handlesMu.RUnlock()
	if !ok {
		setLastError(errors.New("invalid handle"))
		return -8 // EON_ERR_INVALID_HANDLE
	}
	return C.int64_t(len(entry.files))
}

//export eon_stat_index
func eon_stat_index(handle C.uint64_t, index C.int64_t, outStat *C.eon_file_stat_t) C.int {
	if outStat == nil {
		return -1
	}

	handlesMu.RLock()
	entry, ok := handles[uint64(handle)]
	handlesMu.RUnlock()
	if !ok {
		setLastError(errors.New("invalid handle"))
		return -8
	}

	idx := int(index)
	if idx < 0 || idx >= len(entry.files) {
		setLastError(errors.New("index out of range"))
		return -6 // EON_ERR_NOT_FOUND
	}

	ext := entry.files[idx]
	fillStat(outStat, ext)
	return 0 // EON_OK
}

//export eon_stat_name
func eon_stat_name(handle C.uint64_t, cName *C.char, outStat *C.eon_file_stat_t) C.int {
	if cName == nil || outStat == nil {
		return -1
	}

	name := C.GoString(cName)
	handlesMu.RLock()
	entry, ok := handles[uint64(handle)]
	handlesMu.RUnlock()
	if !ok {
		setLastError(errors.New("invalid handle"))
		return -8
	}

	ext, found := entry.r.FindExtent(spec.NamespaceVFS, name)
	if !found {
		setLastError(errors.New("file not found"))
		return -6 // EON_ERR_NOT_FOUND
	}

	fillStat(outStat, ext)
	return 0 // EON_OK
}

func fillStat(outStat *C.eon_file_stat_t, ext catalog.Extent) {
	cName := C.CString(ext.Name)
	defer C.free(unsafe.Pointer(cName))

	C.strncpy(&outStat.name[0], cName, 511)
	outStat.name[511] = 0
	outStat.size = C.uint64_t(ext.PayloadSize)
	ts := ext.Timestamp / int64(time.Second)
	if ts < 0 {
		ts = 0
	}
	outStat.mod_time_unix = C.int64_t(ts)
	outStat.chunk_index = C.uint32_t(ext.ChunkIndex)

	if strings.HasSuffix(ext.Name, "/") {
		outStat.is_dir = 1
	} else {
		outStat.is_dir = 0
	}
}

//export eon_read_file
func eon_read_file(handle C.uint64_t, cName *C.char, outBuf unsafe.Pointer, bufLen C.size_t) C.int64_t {
	if cName == nil || outBuf == nil {
		return -1
	}

	name := C.GoString(cName)
	handlesMu.RLock()
	entry, ok := handles[uint64(handle)]
	handlesMu.RUnlock()
	if !ok {
		setLastError(errors.New("invalid handle"))
		return -8
	}

	rc, err := entry.r.Open(name)
	if err != nil {
		setLastError(err)
		return -6
	}
	defer rc.Close()

	goBuf := unsafe.Slice((*byte)(outBuf), int(bufLen))
	n, err := io.ReadFull(rc, goBuf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		setLastError(err)
		return -1
	}

	setLastError(nil)
	return C.int64_t(n)
}

//export eon_read_index
func eon_read_index(handle C.uint64_t, index C.int64_t, outBuf unsafe.Pointer, bufLen C.size_t) C.int64_t {
	if outBuf == nil {
		return -1
	}

	handlesMu.RLock()
	entry, ok := handles[uint64(handle)]
	handlesMu.RUnlock()
	if !ok {
		setLastError(errors.New("invalid handle"))
		return -8
	}

	idx := int(index)
	if idx < 0 || idx >= len(entry.files) {
		setLastError(errors.New("index out of range"))
		return -6
	}

	ext := entry.files[idx]
	rc, err := entry.r.Open(ext.Name)
	if err != nil {
		setLastError(err)
		return -6
	}
	defer rc.Close()

	goBuf := unsafe.Slice((*byte)(outBuf), int(bufLen))
	n, err := io.ReadFull(rc, goBuf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		setLastError(err)
		return -1
	}

	setLastError(nil)
	return C.int64_t(n)
}

func main() {}
