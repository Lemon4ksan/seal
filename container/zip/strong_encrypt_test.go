// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/container/zip"
)

func TestArchiveExtraData_Roundtrip(t *testing.T) {
	extraPayload := []byte{0x17, 0x00, 0x04, 0x00, 0xAA, 0xBB, 0xCC, 0xDD}
	encoded := zip.EncodeArchiveExtraData(extraPayload)

	// Read skipping 4-byte signature
	r := bytes.NewReader(encoded[4:])
	parsed, err := zip.ReadArchiveExtraData(r)
	require.NoError(t, err)

	assert.Equal(t, uint32(len(extraPayload)), parsed.ExtraFieldLength)
	assert.Equal(t, extraPayload, parsed.ExtraField)
}

func TestStrongEncryptionHeader_Roundtrip(t *testing.T) {
	erd := []byte("16-bytes-erd-key")
	vdata := []byte("validation-bytes")
	vcrc := uint32(0xCAFEBABE)

	encoded := zip.EncodeStrongEncryptionHeader(zip.AlgAES256, 256, erd, vdata, vcrc)
	parsed, ok := zip.ParseStrongEncryptionHeader(encoded)
	require.True(t, ok)

	assert.Equal(t, uint16(zip.AlgAES256), parsed.AlgId)
	assert.Equal(t, uint16(256), parsed.BitLen)
	assert.Equal(t, erd, parsed.ErdData)
	assert.Equal(t, vdata, parsed.Validation)
	assert.Equal(t, vcrc, parsed.ValidationCRC)
}

func TestCentralDirectoryEncryption_Roundtrip(t *testing.T) {
	rawCD := []byte("This is the raw Central Directory records table containing secret filenames")
	password := "SecretPassw0rd!"

	// 1. Plain (uncompressed) encryption
	encBytes, method, uncompSize, compSize, err := zip.EncryptCentralDirectory(rawCD, password, false)
	require.NoError(t, err)
	assert.Equal(t, zip.MethodStore, method)
	assert.Equal(t, int64(len(rawCD)), uncompSize)
	assert.Greater(t, compSize, int64(0))

	decrypted, err := zip.DecryptCentralDirectory(encBytes, password, method)
	require.NoError(t, err)
	assert.Equal(t, rawCD, decrypted)

	// 2. Deflate compressed encryption
	encDeflate, methodDef, _, _, err := zip.EncryptCentralDirectory(rawCD, password, true)
	require.NoError(t, err)
	assert.Equal(t, zip.MethodDeflate, methodDef)

	decryptedDef, err := zip.DecryptCentralDirectory(encDeflate, password, methodDef)
	require.NoError(t, err)
	assert.Equal(t, rawCD, decryptedDef)

	// 3. Wrong password verification
	_, errWrongPwd := zip.DecryptCentralDirectory(encBytes, "WrongPassword!", method)
	require.Error(t, errWrongPwd)
}
