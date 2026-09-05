// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"flag"
	"strings"

	aeon "github.com/lemon4ksan/seal/container/eon/engine"
)

// MultiStringFlag implements flag.Value for repeatable string flags.
type MultiStringFlag []string

func (m *MultiStringFlag) String() string {
	return strings.Join(*m, ", ")
}

func (m *MultiStringFlag) Set(val string) error {
	*m = append(*m, val)
	return nil
}

// SetupUnlockFlags registers credentials flags (-p, --share, --kms, etc.) on the given FlagSet.
func SetupUnlockFlags(fset *flag.FlagSet) (passwords *MultiStringFlag, shares *MultiStringFlag, kmsKey, vaultAddr, vaultToken *string) {
	passwords = &MultiStringFlag{}
	shares = &MultiStringFlag{}
	fset.Var(passwords, "p", "Passphrase(s) to unlock encrypted container / vaults (repeatable)")
	fset.Var(passwords, "password", "Passphrase(s) to unlock encrypted container / vaults (repeatable)")
	fset.Var(shares, "share", "Shamir secret share string (repeatable)")
	fset.Var(shares, "shares", "Shamir secret share string (repeatable or comma-separated)")
	kmsKey = fset.String("kms", "", "KMS Key identifier (vault://transit/keys/<name>, mock://<name>)")
	vaultAddr = fset.String("vault-addr", "", "HashiCorp Vault server address")
	vaultToken = fset.String("vault-token", "", "HashiCorp Vault authentication token")
	return
}

// BuildUnlockOptions converts parsed credentials flags into aeon ReaderOptions.
func BuildUnlockOptions(passwords []string, shares []string, kmsKey, vaultAddr, vaultToken string) ([]aeon.ReaderOption, error) {
	var opts []aeon.ReaderOption
	if len(passwords) > 0 {
		opts = append(opts, aeon.WithReaderPasswords(passwords...))
	}
	if len(shares) > 0 {
		for _, sh := range shares {
			for _, part := range strings.Split(sh, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					opts = append(opts, aeon.WithShamirShares(part))
				}
			}
		}
	}
	if kmsKey != "" {
		client, _, err := CreateKMSClient(kmsKey, vaultAddr, vaultToken)
		if err != nil {
			return nil, err
		}
		opts = append(opts, aeon.WithKMS(client))
	}
	return opts, nil
}
