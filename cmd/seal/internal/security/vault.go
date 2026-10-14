// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package security

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lemon4ksan/ctrl/argkit"
	"github.com/lemon4ksan/foundation/fskit"

	"github.com/lemon4ksan/seal/cmd/seal/internal/archive"
	"github.com/lemon4ksan/seal/cmd/seal/internal/forensics"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/container/eon/crypto"
	aeon "github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

// VaultCmd manages isolated compartments (multi-vault) and plausible deniability.
type VaultCmd struct{}

func (c *VaultCmd) Name() string      { return "vault" }
func (c *VaultCmd) Aliases() []string { return nil }
func (c *VaultCmd) Synopsis() string {
	return "Manage isolated compartments (multi-vault) and plausible deniability"
}

func (c *VaultCmd) Usage() string {
	return "aeon vault <create|list|extract|test|info> [arguments]"
}

func (c *VaultCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		c.printUsage(stdout)
		return errors.New("subcommand required: create, list, extract, test, or info")
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "create":
		return c.runCreate(ctx, subArgs, stdout, stderr)
	case "list":
		return (&archive.ListCmd{}).Run(ctx, subArgs, stdout, stderr)
	case "extract":
		return (&archive.ExtractCmd{}).Run(ctx, subArgs, stdout, stderr)
	case "test":
		return (&forensics.TestCmd{}).Run(ctx, subArgs, stdout, stderr)
	case "info":
		return c.runInfo(ctx, subArgs, stdout, stderr)
	case "help", "-h", "--help":
		c.printUsage(stdout)
		return nil
	default:
		c.printUsage(stderr)
		return fmt.Errorf("unknown vault subcommand: %s", sub)
	}
}

func (c *VaultCmd) printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: seal vault <subcommand> [arguments]")
	fmt.Fprintln(w, "\nSubcommands:")
	fmt.Fprintln(w, "  create   Create a multi-vault container with isolated compartments")
	fmt.Fprintln(w, "  list     List files in unlocked vault(s) (supports multi-key union view)")
	fmt.Fprintln(w, "  extract  Extract files from unlocked vault(s)")
	fmt.Fprintln(w, "  info     Display Key Slot parameters and decrypted vault descriptors")
	fmt.Fprintln(w, "  test     Verify chunk & extent integrity for unlocked vault(s)")
	fmt.Fprintln(w, "\nExamples:")
	fmt.Fprintln(
		w,
		"  seal vault create -o secure.eon -vault \"personal:Pass1!:./personal\" -vault \"work:Pass2!:./work\"",
	)
	fmt.Fprintln(w, "  seal vault list -p Pass1! secure.eon")
	fmt.Fprintln(w, "  seal vault list -p Pass1! -p Pass2! secure.eon  # Union view (Pass1 + Pass2)")
	fmt.Fprintln(w, "  seal vault extract -p Pass1! -o ./restored secure.eon")
	fmt.Fprintln(w, "  seal vault info secure.eon")
	fmt.Fprintln(w, "  seal vault info -p Pass1! secure.eon")
}

func (c *VaultCmd) runCreate(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("vault create", flag.ContinueOnError)
	fset.SetOutput(stderr)

	outPath := fset.String("o", "vaults.eon", "Output .eon file path")
	codecName := fset.String("c", "zstd", "Compression codec: zstd, lzma2, brotli, store")
	levelStr := fset.String("l", "default", "Compression level: fast, default, max, ultra, or 1..9")
	cipherStr := fset.String("cipher", "aes", "AEAD cipher algorithm: aes (AES-256-GCM) or chacha (ChaCha20-Poly1305)")
	var vaults util.MultiStringFlag
	fset.Var(&vaults, "vault", "Define a vault: 'name:password:source_path' (repeatable, at least 1 required)")

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(vaults) == 0 {
		return errors.New(
			"specify at least one -vault 'name:password:path'\nexample: aeon vault create -o secure.eon -vault 'personal:Secret123!:./personal' -vault 'work:CorpPass456!:./work'",
		)
	}

	if *outPath == "vaults.eon" && len(remaining) > 0 {
		*outPath = remaining[0]
	}

	outFile, err := os.Create(*outPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer outFile.Close()

	codecID := spec.CodecZSTD
	switch strings.ToLower(*codecName) {
	case "lz4":
		codecID = spec.CodecLZ4
	case "lzma2", "lzma":
		codecID = spec.CodecLZMA2
	case "brotli", "br":
		codecID = spec.CodecBrotli
	case "przm", "prism":
		codecID = spec.CodecPRZM
	case "store", "copy", "raw":
		codecID = spec.CodecStore
	default:
		codecID = spec.CodecZSTD
	}

	var codecLevel uint8 = 3
	switch strings.ToLower(*levelStr) {
	case "fast", "fastest", "1":
		codecLevel = 1
	case "default", "normal", "3":
		codecLevel = 3
	case "max", "maximum", "7":
		codecLevel = 7
	case "ultra", "best", "9":
		codecLevel = 9
	default:
		var n int
		if _, err := fmt.Sscanf(*levelStr, "%d", &n); err == nil && n >= 1 && n <= 9 {
			codecLevel = uint8(n)
		}
	}

	cipherID := spec.CipherAES256GCM
	switch strings.ToLower(*cipherStr) {
	case "chacha", "chacha20", "chacha20poly1305":
		cipherID = spec.CipherChaCha20Poly1305
	}

	w := aeon.NewWriter(outFile,
		aeon.WithCodec(codecID),
		aeon.WithCodecLevel(codecLevel),
		aeon.WithCipher(cipherID),
	)

	type vaultSpec struct {
		name     string
		password string
		path     string
	}
	var vspecs []vaultSpec

	for _, vstr := range vaults {
		parts := strings.SplitN(vstr, ":", 3)
		if len(parts) != 3 {
			return fmt.Errorf("invalid -vault syntax %q, expected 'name:password:path'", vstr)
		}
		vspecs = append(vspecs, vaultSpec{name: parts[0], password: parts[1], path: parts[2]})
		if err := w.AddVault(parts[0], parts[1]); err != nil {
			return fmt.Errorf("registering vault %q: %w", parts[0], err)
		}
	}

	totalCount := 0
	var totalBytes int64
	startTime := time.Now()

	for _, vs := range vspecs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fmt.Fprintf(stdout, "Packing vault %q from %s...\n", vs.name, vs.path)
		info, err := os.Stat(vs.path)
		if err != nil {
			fmt.Fprintf(stderr, "error stating %s: %v\n", vs.path, err)
			continue
		}

		vaultFiles := 0
		var vaultBytes int64

		if info.IsDir() {
			ch := make(chan fskit.Entry, 2048)
			go fskit.Walk(vs.path, ch)

			for entry := range ch {
				if entry.IsDir {
					_ = w.AddVaultFileStream(vs.name, entry.RelPath, 0, time.Time{}, nil)
					continue
				}

				f, err := os.Open(entry.Path)
				if err != nil {
					continue
				}
				err = w.AddVaultFileStream(vs.name, entry.RelPath, entry.Size, entry.ModTime, f)
				f.Close()
				if err != nil {
					continue
				}
				vaultFiles++
				vaultBytes += entry.Size
			}
		} else {
			f, err := os.Open(vs.path)
			if err != nil {
				continue
			}
			err = w.AddVaultFileStream(vs.name, filepath.Base(vs.path), info.Size(), info.ModTime(), f)
			f.Close()
			if err == nil {
				vaultFiles++
				vaultBytes += info.Size()
			}
		}
		totalCount += vaultFiles
		totalBytes += vaultBytes
		fmt.Fprintf(
			stdout,
			"  ✓ Vault %q: %d file(s), %s\n",
			vs.name,
			vaultFiles,
			util.FormatHumanBytes(uint64(vaultBytes)),
		)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("closing container: %w", err)
	}

	stat, _ := outFile.Stat()
	eonSize := uint64(stat.Size())
	ratio := 0.0
	if totalBytes > 0 {
		ratio = float64(eonSize) / float64(totalBytes) * 100
	}
	fmt.Fprintf(
		stdout,
		"\n✓ Successfully created multi-vault container with %d vault(s), %d item(s) in %.2fs (Raw: %s ➔ EON: %s, %.1f%%)\n",
		len(vspecs),
		totalCount,
		time.Since(startTime).Seconds(),
		util.FormatHumanBytes(uint64(totalBytes)),
		util.FormatHumanBytes(eonSize),
		ratio,
	)
	return nil
}

func (c *VaultCmd) runInfo(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("vault info", flag.ContinueOnError)
	fset.SetOutput(stderr)

	var passwords util.MultiStringFlag
	fset.Var(&passwords, "p", "Passphrase(s) to inspect unlocked vault descriptors (repeatable)")
	fset.Var(&passwords, "password", "Passphrase(s) to inspect unlocked vault descriptors (repeatable)")

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(remaining) < 1 {
		return errors.New("usage: aeon vault info [-p password ...] <archive.eon>")
	}

	archivePath := remaining[0]
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening container: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stating container: %w", err)
	}

	if stat.Size() < spec.HeaderSize+spec.TrailerSize {
		return errors.New("invalid container size")
	}

	var hdr [spec.HeaderSize]byte
	if _, err := f.ReadAt(hdr[:], 0); err != nil {
		return fmt.Errorf("reading header: %w", err)
	}

	flags := binary.LittleEndian.Uint16(hdr[10:12])
	if (flags & spec.FlagEncrypted) == 0 {
		fmt.Fprintln(stdout, "Container is not encrypted; no vault compartments found.")
		return nil
	}

	encr, err := crypto.ReadENCR(io.NewSectionReader(f, spec.HeaderSize, stat.Size()-spec.TrailerSize-spec.HeaderSize))
	if err != nil {
		return fmt.Errorf("reading ENCR header: %w", err)
	}

	cipherName := "AES-256-GCM"
	if encr.CipherID == spec.CipherChaCha20Poly1305 {
		cipherName = "ChaCha20-Poly1305"
	}

	fmt.Fprintln(stdout, "=== EON Vault Container Information ===")
	fmt.Fprintf(stdout, "File:            %s\n", filepath.Base(archivePath))
	fmt.Fprintf(stdout, "Size:            %s (%d bytes)\n", util.FormatHumanBytes(uint64(stat.Size())), stat.Size())
	fmt.Fprintf(stdout, "AEAD Cipher:     %s (0x%02X)\n", cipherName, encr.CipherID)
	fmt.Fprintf(stdout, "Master IV:       %x\n", encr.MasterIV)
	fmt.Fprintf(stdout, "Total Key Slots: %d\n\n", len(encr.Slots))

	fmt.Fprintln(stdout, "--- Key Slots (Plausible Deniability) ---")
	for i, slot := range encr.Slots {
		kdfName := "Argon2id"
		if slot.SlotType == spec.SlotTypePBKDF2SHA256 {
			kdfName = "PBKDF2-SHA256"
		}
		fmt.Fprintf(stdout, "  Slot #%d: Type=0x%02X, KDF=%s, Mem=%d KiB, Iter=%d, Threads=%d, Salt=%x\n",
			i, slot.SlotType, kdfName, slot.KDFMemoryKiB, slot.KDFIterations, slot.KDFParallelism, slot.Salt)
	}
	fmt.Fprintln(stdout)

	if len(passwords) == 0 {
		fmt.Fprintln(stdout, "[i] Compartmentalized Vaults: Vault descriptors and directories are confidential.")
		fmt.Fprintln(stdout, "    Provide -p <passphrase> to unlock and inspect specific vault descriptors.")
		return nil
	}

	unlockedCount := 0
	for _, pw := range passwords {
		found := false
		for _, slot := range encr.Slots {
			vd, err := slot.Unlock(pw, encr.CipherID, encr.MasterIV)
			if err == nil {
				found = true
				unlockedCount++
				fmt.Fprintf(stdout, "[✓] Password %q unlocked Slot #%d:\n", pw, slot.SlotIndex)
				fmt.Fprintf(stdout, "    Vault Name:        %s\n", vd.VaultName)
				fmt.Fprintf(stdout, "    Vault ID:          %x\n", vd.VaultID)
				fmt.Fprintf(stdout, "    Directory Offset:  0x%X (%d bytes)\n", vd.DirOffset, vd.DirOffset)
				fmt.Fprintf(stdout, "    Directory Size:    %d bytes\n", vd.DirSize)
				flagDesc := "None"
				if (vd.VaultFlags & spec.VaultFlagSharedChunks) != 0 {
					flagDesc = "SharedChunks (cross-vault deduplication)"
				}
				if (vd.VaultFlags & spec.VaultFlagHidden) != 0 {
					flagDesc += " | Hidden"
				}
				fmt.Fprintf(stdout, "    Vault Flags:       0x%04X (%s)\n\n", vd.VaultFlags, flagDesc)
				break
			}
		}
		if !found {
			fmt.Fprintf(stdout, "[✗] Password %q: FAILED (no matching key slot)\n", pw)
		}
	}

	if unlockedCount > 0 {
		var rOpts []aeon.ReaderOption
		rOpts = append(rOpts, aeon.WithReaderPasswords(passwords...))
		if r, rFile, err := aeon.OpenFile(archivePath, rOpts...); err == nil {
			defer rFile.Close()
			fmt.Fprintf(stdout, "Extents visible under provided keys: %d (Active Vault: %s)\n",
				len(r.Extents()), r.ActiveVault())
		}
	}

	return nil
}
