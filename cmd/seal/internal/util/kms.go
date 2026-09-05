// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"fmt"
	"strings"

	"github.com/lemon4ksan/seal/container/eon/crypto"
)

// CreateKMSClient instantiates a KMSClient for the specified key URI.
func CreateKMSClient(keyID, vaultAddr, vaultToken string) (crypto.KMSClient, string, error) {
	if strings.HasPrefix(keyID, "mock://") {
		return crypto.NewMockKMSClient(), keyID, nil
	}
	if strings.HasPrefix(keyID, "vault://") {
		client := crypto.NewVaultClient(vaultAddr, vaultToken)
		return client, keyID, nil
	}
	return nil, "", fmt.Errorf("unsupported KMS provider scheme for key %q (supported: vault://, mock://)", keyID)
}
