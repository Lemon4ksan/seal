// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/seal/container/zip"
)

func TestArchiveExtraData_Roundtrip(t *testing.T) {
	extraPayload := []byte{0x17, 0x00, 0x04, 0x00, 0xAA, 0xBB, 0xCC, 0xDD}
	encoded := zip.EncodeArchiveExtraData(extraPayload)

	// Read skipping 4-byte signature
	r := bytes.NewReader(encoded[4:])
	parsed, err := zip.ReadArchiveExtraData(r)
	if err != nil {
		t.Fatalf("ReadArchiveExtraData failed: %v", err)
	}

	if parsed.ExtraFieldLength != uint32(len(extraPayload)) {
		t.Errorf("length mismatch: got %d, want %d", parsed.ExtraFieldLength, len(extraPayload))
	}
	if !bytes.Equal(parsed.ExtraField, extraPayload) {
		t.Errorf("payload mismatch: got %x, want %x", parsed.ExtraField, extraPayload)
	}
}

func TestStrongEncryptionHeader_Roundtrip(t *testing.T) {
	erd := []byte("16-bytes-erd-key")
	vdata := []byte("validation-bytes")
	vcrc := uint32(0xCAFEBABE)

	encoded := zip.EncodeStrongEncryptionHeader(zip.AlgAES256, 256, erd, vdata, vcrc)
	parsed, ok := zip.ParseStrongEncryptionHeader(encoded)
	if !ok {
		t.Fatalf("ParseStrongEncryptionHeader failed")
	}

	if parsed.AlgId != zip.AlgAES256 {
		t.Errorf("AlgId mismatch: got %x, want %x", parsed.AlgId, zip.AlgAES256)
	}
	if parsed.BitLen != 256 {
		t.Errorf("BitLen mismatch: got %d, want 256", parsed.BitLen)
	}
	if !bytes.Equal(parsed.ErdData, erd) {
		t.Errorf("ErdData mismatch: got %q, want %q", string(parsed.ErdData), string(erd))
	}
	if !bytes.Equal(parsed.Validation, vdata) {
		t.Errorf("Validation mismatch: got %q, want %q", string(parsed.Validation), string(vdata))
	}
	if parsed.ValidationCRC != vcrc {
		t.Errorf("ValidationCRC mismatch: got %x, want %x", parsed.ValidationCRC, vcrc)
	}
}

func TestCentralDirectoryEncryption_Roundtrip(t *testing.T) {
	rawCD := []byte("This is the raw Central Directory records table containing secret filenames")
	password := "SecretPassw0rd!"

	// 1. Plain (uncompressed) encryption
	encBytes, method, uncompSize, compSize, err := zip.EncryptCentralDirectory(rawCD, password, false)
	if err != nil {
		t.Fatalf("EncryptCentralDirectory failed: %v", err)
	}
	if method != zip.MethodStore {
		t.Errorf("expected MethodStore, got %d", method)
	}
	if uncompSize != int64(len(rawCD)) {
		t.Errorf("uncompSize mismatch: got %d, want %d", uncompSize, len(rawCD))
	}
	if compSize <= 0 {
		t.Errorf("compSize invalid: %d", compSize)
	}

	decrypted, err := zip.DecryptCentralDirectory(encBytes, password, method)
	if err != nil {
		t.Fatalf("DecryptCentralDirectory failed: %v", err)
	}
	if !bytes.Equal(decrypted, rawCD) {
		t.Fatalf("decrypted CD mismatch: got %q, want %q", string(decrypted), string(rawCD))
	}

	// 2. Deflate compressed encryption
	encDeflate, methodDef, _, _, err := zip.EncryptCentralDirectory(rawCD, password, true)
	if err != nil {
		t.Fatalf("EncryptCentralDirectory (deflate) failed: %v", err)
	}
	if methodDef != zip.MethodDeflate {
		t.Errorf("expected MethodDeflate, got %d", methodDef)
	}

	decryptedDef, err := zip.DecryptCentralDirectory(encDeflate, password, methodDef)
	if err != nil {
		t.Fatalf("DecryptCentralDirectory (deflate) failed: %v", err)
	}
	if !bytes.Equal(decryptedDef, rawCD) {
		t.Fatalf("decrypted deflate CD mismatch: got %q, want %q", string(decryptedDef), string(rawCD))
	}

	// 3. Wrong password verification
	_, errWrongPwd := zip.DecryptCentralDirectory(encBytes, "WrongPassword!", method)
	if errWrongPwd == nil {
		t.Errorf("expected error on wrong password, got nil")
	}
}
