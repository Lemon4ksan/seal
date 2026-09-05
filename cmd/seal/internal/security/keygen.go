// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package security

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/lemon4ksan/foundation/argkit"
)

// KeygenCmd generates Ed25519 keypairs for digital signing.
type KeygenCmd struct{}

func (c *KeygenCmd) Name() string      { return "keygen" }
func (c *KeygenCmd) Aliases() []string { return nil }
func (c *KeygenCmd) Synopsis() string {
	return "Generate an Ed25519 keypair for digital container signing"
}
func (c *KeygenCmd) Usage() string {
	return "aeon keygen [flags] [-o aeon_ed25519.key]"
}

func (c *KeygenCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("keygen", flag.ContinueOnError)
	fset.SetOutput(stderr)

	outKey := fset.String("o", "aeon_ed25519.key", "Output private key file path")

	if _, err := argkit.ParseInterspersedFlags(fset, args); err != nil {
		return err
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generating key: %w", err)
	}

	if err := os.WriteFile(*outKey, priv, 0600); err != nil {
		return fmt.Errorf("saving private key: %w", err)
	}

	pubPath := *outKey + ".pub"
	if err := os.WriteFile(pubPath, pub, 0644); err != nil {
		return fmt.Errorf("saving public key: %w", err)
	}

	fmt.Fprintf(stdout, "Generated Ed25519 keypair:\n  Private Key: %s\n  Public Key:  %s (%x)\n",
		*outKey, pubPath, hex.EncodeToString(pub))
	return nil
}
