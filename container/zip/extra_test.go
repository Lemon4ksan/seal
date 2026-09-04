// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/container/zip"
)

func TestExtendedTimestamp_Roundtrip(t *testing.T) {
	mtime := time.Unix(1700000000, 0).UTC()
	atime := time.Unix(1700000100, 0).UTC()
	ctime := time.Unix(1700000200, 0).UTC()

	// Local header encoding (all 3 timestamps)
	encLocal := zip.EncodeExtendedTimestamp(mtime, atime, ctime, false)
	// Skip 4-byte header ID and length
	parsedMtime, parsedAtime, parsedCtime, ok := zip.ParseExtendedTimestamp(encLocal[4:])
	require.True(t, ok)
	assert.Equal(t, mtime, parsedMtime)
	assert.Equal(t, atime, parsedAtime)
	assert.Equal(t, ctime, parsedCtime)

	// Central directory encoding (only mtime)
	encCD := zip.EncodeExtendedTimestamp(mtime, atime, ctime, true)
	parsedMtimeCD, parsedAtimeCD, _, ok := zip.ParseExtendedTimestamp(encCD[4:])
	require.True(t, ok)
	assert.Equal(t, mtime, parsedMtimeCD)
	assert.True(t, parsedAtimeCD.IsZero())
}

func TestUnicodePath_Roundtrip(t *testing.T) {
	utf8Name := "тест/документ_привет.txt"
	rawName := "test/dok_privet.txt"

	enc := zip.EncodeUnicodePath(utf8Name, rawName)
	parsedName, ok := zip.ParseUnicodePath(enc[4:], rawName)
	require.True(t, ok)
	assert.Equal(t, utf8Name, parsedName)

	// If header name changed, validation must fail
	_, okTampered := zip.ParseUnicodePath(enc[4:], "tampered_name.txt")
	assert.False(t, okTampered, "expected CRC mismatch on tampered header name, but succeeded")
}

func TestUnicodeComment_Roundtrip(t *testing.T) {
	utf8Comment := "Архивный комментарий на русском языке"
	rawComment := "Archive comment"

	enc := zip.EncodeUnicodeComment(utf8Comment, rawComment)
	parsedComment, ok := zip.ParseUnicodeComment(enc[4:], rawComment)
	require.True(t, ok)
	assert.Equal(t, utf8Comment, parsedComment)
}

func TestInfoZipUIDGID_Roundtrip(t *testing.T) {
	uid := uint32(1001)
	gid := uint32(1002)

	enc := zip.EncodeInfoZipUIDGID(uid, gid)
	parsedUID, parsedGID, ok := zip.ParseInfoZipUIDGID(enc[4:])
	require.True(t, ok)
	assert.Equal(t, uid, parsedUID)
	assert.Equal(t, gid, parsedGID)
}

func TestDataStreamAlignment_Encoding(t *testing.T) {
	enc := zip.EncodeDataStreamAlignment(4096, 12)
	assert.Len(t, enc, 4+2+12)
}
