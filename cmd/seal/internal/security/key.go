// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package security

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/lemon4ksan/foundation/argkit"
	"github.com/lemon4ksan/foundation/cryptokit/shamir"

	aeon "github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

// KeyCmd manages encryption keys, Shamir Secret Sharing split, and combine operations.
type KeyCmd struct{}

func (c *KeyCmd) Name() string      { return "key" }
func (c *KeyCmd) Aliases() []string { return nil }
func (c *KeyCmd) Synopsis() string {
	return "Manage encryption keys (Shamir Secret Sharing split/combine, KMS)"
}
func (c *KeyCmd) Usage() string {
	return "aeon key <split|combine> [arguments]"
}

func (c *KeyCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		fmt.Fprintln(stdout, "Usage: aeon key <command> [arguments]")
		fmt.Fprintln(stdout, "\nKey Management Commands:")
		fmt.Fprintln(stdout, "  split     Split container DEK into M-of-N Shamir secret shares")
		fmt.Fprintln(stdout, "  combine   Combine Shamir secret shares to reconstruct master DEK")
		return errors.New("subcommand required: split or combine")
	}

	cmd := args[0]
	switch cmd {
	case "split":
		return c.runSplit(ctx, args[1:], stdout, stderr)
	case "combine":
		return c.runCombine(ctx, args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, "Usage: aeon key <command> [arguments]")
		fmt.Fprintln(stdout, "\nKey Management Commands:")
		fmt.Fprintln(stdout, "  split     Split container DEK into M-of-N Shamir secret shares")
		fmt.Fprintln(stdout, "  combine   Combine Shamir secret shares to reconstruct master DEK")
		return nil
	default:
		return fmt.Errorf("unknown key command: %s", cmd)
	}
}

func (c *KeyCmd) runSplit(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("key split", flag.ContinueOnError)
	fset.SetOutput(stderr)

	threshold := fset.Int("m", 3, "Threshold number of shares required to reconstruct (M)")
	total := fset.Int("n", 5, "Total number of shares to generate (N)")
	passwords, shares, kmsKey, vaultAddr, vaultToken := util.SetupUnlockFlags(fset)

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(remaining) < 1 {
		return errors.New("usage: aeon key split -m <threshold> -n <total> [-p password] [--kms keyID] <archive.eon>")
	}

	if *threshold < 2 || *threshold > 255 || *total < *threshold || *total > 255 {
		return errors.New("invalid threshold/total: must satisfy 2 <= M <= N <= 255")
	}

	archivePath := remaining[0]
	opts, err := util.BuildUnlockOptions(*passwords, *shares, *kmsKey, *vaultAddr, *vaultToken)
	if err != nil {
		return fmt.Errorf("configuring unlock credentials: %w", err)
	}

	r, f, err := aeon.OpenFile(archivePath, opts...)
	if err != nil {
		if errors.Is(err, spec.ErrPasswordRequired) {
			return errors.New("container is encrypted; provide passphrase with -p or KMS with --kms")
		}
		if errors.Is(err, spec.ErrInvalidPassword) {
			return errors.New("invalid credentials for encrypted container")
		}
		return fmt.Errorf("opening container: %w", err)
	}
	defer f.Close()

	if !r.IsEncrypted() {
		return fmt.Errorf("container %s is not encrypted; no DEK to split", filepath.Base(archivePath))
	}

	dek := r.DEK()
	if len(dek) == 0 {
		return errors.New("failed to retrieve decrypted DEK")
	}

	rawShares, err := shamir.Split(dek, *threshold, *total)
	if err != nil {
		return fmt.Errorf("splitting DEK: %w", err)
	}

	fmt.Fprintf(stdout, "\n✓ Split Master DEK into %d-of-%d Shamir Secret Shares for %s:\n", *threshold, *total, filepath.Base(archivePath))
	fmt.Fprintln(stdout, strings.Repeat("-", 72))
	for i, s := range rawShares {
		formatted := shamir.FormatShare(s, *threshold, *total)
		fmt.Fprintf(stdout, "Share #%d: %s\n", i+1, formatted)
	}
	fmt.Fprintln(stdout, strings.Repeat("-", 72))
	fmt.Fprintf(stdout, "Any %d share(s) can unlock this container without a password:\n", *threshold)
	fmt.Fprintf(stdout, "  aeon extract --share <share1> --share <share2> %s\n\n", filepath.Base(archivePath))
	return nil
}

func (c *KeyCmd) runCombine(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: aeon key combine <share1> <share2> ...")
	}

	var parsedShares []shamir.Share
	for _, arg := range args {
		s, _, _, err := shamir.ParseShare(arg)
		if err != nil {
			return fmt.Errorf("parsing share %q: %w", arg, err)
		}
		parsedShares = append(parsedShares, s)
	}

	dek, err := shamir.Combine(parsedShares)
	if err != nil {
		return fmt.Errorf("combining shares: %w", err)
	}

	fmt.Fprintf(stdout, "\n✓ Reconstructed Master DEK (%d bytes):\n", len(dek))
	fmt.Fprintf(stdout, "Hex: %x\n\n", dek)
	return nil
}
