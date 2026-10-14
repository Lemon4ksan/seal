// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package forensics

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lemon4ksan/ctrl/argkit"
	"golang.org/x/net/webdav"

	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	aeon "github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/vfs"
)

// MountCmd implements mounting container as a virtual file system.
type MountCmd struct{}

func (c *MountCmd) Name() string      { return "mount" }
func (c *MountCmd) Aliases() []string { return []string{"m"} }
func (c *MountCmd) Synopsis() string {
	return "Mount container as a virtual read-only file system"
}

func (c *MountCmd) Usage() string {
	return "aeon mount [flags] <archive.eon> [drive_or_mountpoint]"
}

func (c *MountCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("mount", flag.ContinueOnError)
	fset.SetOutput(stderr)

	addr := fset.String("addr", "127.0.0.1:0", "Listen address for HTTP/WebDAV mount server")
	drive := fset.String("drive", "", "Drive letter to mount on Windows (e.g. Z: or Z)")
	passwords, shares, kmsKey, vaultAddr, vaultToken := util.SetupUnlockFlags(fset)

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(remaining) < 1 {
		return errors.New(
			"usage: aeon mount [-p password] [--shares s1,...] [--kms keyID] [-drive Z:] [-addr :8080] <archive.eon> [drive_or_mountpoint]",
		)
	}

	archivePath := remaining[0]
	targetMount := *drive
	if targetMount == "" && len(remaining) > 1 {
		targetMount = remaining[1]
	}

	rOpts, err := util.BuildUnlockOptions(*passwords, *shares, *kmsKey, *vaultAddr, *vaultToken)
	if err != nil {
		return fmt.Errorf("configuring unlock credentials: %w", err)
	}

	r, f, err := aeon.OpenFile(archivePath, rOpts...)
	if err != nil {
		return fmt.Errorf("opening container: %w", err)
	}
	defer f.Close()

	// Initialize VFS filesystem
	filesystem := vfs.New(r)
	webdavFS := vfs.NewWebDAV(filesystem)
	handler := &webdav.Handler{
		FileSystem: webdavFS,
		LockSystem: webdav.NewMemLS(),
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("starting mount server: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	localURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	fmt.Fprintln(stdout, "=== EON Virtual Filesystem Mount ===")
	fmt.Fprintf(stdout, "Container:  %s (%d extents)\n", filepath.Base(archivePath), len(r.Extents()))
	if len(r.UnlockedVaults()) > 0 {
		fmt.Fprintf(stdout, "Vault:      %s\n", r.ActiveVault())
	}
	fmt.Fprintf(stdout, "Mount URL:  %s\n", localURL)

	var mountedDrive string
	if runtime.GOOS == "windows" && targetMount != "" {
		d := strings.ToUpper(strings.TrimSuffix(targetMount, ":"))
		if len(d) == 1 && d[0] >= 'A' && d[0] <= 'Z' {
			mountedDrive = d + ":"
			fmt.Fprintf(stdout, "Attaching Windows Drive: %s ...\n", mountedDrive)
			_ = exec.Command("net", "use", mountedDrive, "/delete", "/yes").Run()
			cmd := exec.Command("net", "use", mountedDrive, localURL, "/persistent:no")
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Fprintf(
					stdout,
					"Notice: Windows native drive mapping requires WebClient service (%v: %s)\n",
					err,
					strings.TrimSpace(string(out)),
				)
				fmt.Fprintf(stdout, "        WebDAV server is ACTIVE at: %s\n", localURL)
				fmt.Fprintf(stdout, "        To access: In Windows Explorer -> 'Map network drive' -> %s\n", localURL)
				mountedDrive = ""
			} else {
				fmt.Fprintf(stdout, "✓ Drive %s successfully mounted! Accessible in Windows Explorer.\n", mountedDrive)
			}
		}
	} else if runtime.GOOS != "windows" && targetMount != "" {
		fmt.Fprintf(stdout, "Mount point specified: %s\n", targetMount)
		fmt.Fprintf(
			stdout,
			"Mount command: 'mount -t davfs %s %s' or macOS Finder 'Connect to Server'\n",
			localURL,
			targetMount,
		)
	}

	server := &http.Server{
		Handler: handler,
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "mount server error: %v\n", err)
		}
	}()

	fmt.Fprintln(stdout, "\nVirtual file system is ACTIVE (Streaming on demand).")
	fmt.Fprintln(stdout, "Press Ctrl+C or cancel context to unmount.")

	<-ctx.Done()

	fmt.Fprintln(stdout, "\nUnmounting...")
	if mountedDrive != "" {
		_ = exec.Command("net", "use", mountedDrive, "/delete", "/yes").Run()
		fmt.Fprintf(stdout, "✓ Detached %s\n", mountedDrive)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	fmt.Fprintln(stdout, "Cleanly unmounted.")
	return nil
}
