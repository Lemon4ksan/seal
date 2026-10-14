// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/internal/sys"
	"github.com/lemon4ksan/seal/text/encoding/charmap"
)

func createTestZip(filename, comment string, flags uint16, extra []byte) []byte {
	buf := new(bytes.Buffer)

	// Local File Header
	lfh := LocalFileHeader{
		VersionNeededToExtract: 20,
		GeneralPurposeBitFlag:  flags,
		CompressionMethod:      0, // Store
		FilenameLength:         uint16(len(filename)),
		ExtraFieldLength:       uint16(len(extra)),
		Filename:               filename,
		ExtraField:             extra,
	}
	buf.Write(lfh.AppendBytes(nil))

	// Data payload
	data := []byte("test data")
	buf.Write(data)

	cdOffset := int64(buf.Len())

	// Central Directory Entry
	cd := CentralDirectory{
		VersionMadeBy:          (uint16(sys.HostSystemFAT) << 8) | 20,
		VersionNeededToExtract: 20,
		GeneralPurposeBitFlag:  flags,
		CompressionMethod:      0,
		FilenameLength:         uint16(len(filename)),
		ExtraFieldLength:       uint16(len(extra)),
		FileCommentLength:      uint16(len(comment)),
		Filename:               filename,
		ExtraField:             extra,
		Comment:                comment,
		LocalHeaderOffset:      0,
		UncompressedSize:       uint32(len(data)),
		CompressedSize:         uint32(len(data)),
	}
	cdBytes := cd.AppendBytes(nil)
	buf.Write(cdBytes)

	// EOCD
	eocd := EncodeEOCD(1, int64(len(cdBytes)), cdOffset, "")
	buf.Write(eocd)

	return buf.Bytes()
}

func TestZip_LegacyCP866_Decoding(t *testing.T) {
	// "Привет.txt" in CP866 (bit 11 is NOT set)
	cp866Name := "\x8f\xe0\xa8\xa2\xa5\xe2.txt"
	cp866Comment := "\x8e\xaf\xa8\xe1\xa0\xad\xa8\xa5" // "Описание" in CP866

	zipBytes := createTestZip(cp866Name, cp866Comment, 0, nil)

	// 1. Read with CP866 decoder
	cp866Decoder := func(s string) string {
		res, err := charmap.CodePage866.NewDecoder().String(s)
		if err != nil {
			return s
		}
		return res
	}

	r, err := NewReader(
		bytes.NewReader(zipBytes),
		int64(len(zipBytes)),
		"",
		codec.DefaultRegistry,
		WithTextDecoder(cp866Decoder),
	)
	require.NoError(t, err)

	entries := r.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "Привет.txt", entries[0].Name())
	assert.Equal(t, "Описание", entries[0].Comment())
}

func TestZip_InfoZipUnicodePath_Precedence(t *testing.T) {
	// Legacy short name without bit 11
	rawName := "SHORT.TXT"
	utf8Name := "ДлинноеИмя.txt"

	// Info-ZIP 0x7075 extra field
	extra := EncodeUnicodePath(utf8Name, rawName)
	zipBytes := createTestZip(rawName, "", 0, extra)

	r, err := NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)), "", codec.DefaultRegistry)
	require.NoError(t, err)

	entries := r.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, utf8Name, entries[0].Name())
}
