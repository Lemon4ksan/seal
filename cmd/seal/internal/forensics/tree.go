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
	"os"
	"sort"
	"strings"

	"github.com/lemon4ksan/foundation/argkit"
	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/container"
)

// TreeCmd displays a visual ASCII tree of an archive with depth limiting and file filtering.
type TreeCmd struct{}

func (c *TreeCmd) Name() string      { return "tree" }
func (c *TreeCmd) Aliases() []string { return nil }
func (c *TreeCmd) Synopsis() string {
	return "Display visual ASCII directory tree of the archive with depth & filters"
}
func (c *TreeCmd) Usage() string {
	return "seal tree [flags] <archive-path> [patterns...]"
}

type treeNode struct {
	Name     string
	IsDir    bool
	Size     int64
	Children map[string]*treeNode
}

func newTreeNode(name string, isDir bool) *treeNode {
	return &treeNode{
		Name:     name,
		IsDir:    isDir,
		Children: make(map[string]*treeNode),
	}
}

func (c *TreeCmd) Run(_ context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("tree", flag.ContinueOnError)
	depth := fs.Int("L", 0, "Max display depth of the directory tree (0 = unlimited)")
	fs.IntVar(depth, "depth", 0, "Max display depth of the directory tree (0 = unlimited)")
	dirsOnly := fs.Bool("d", false, "List directories only")
	fs.BoolVar(dirsOnly, "dirs-only", false, "List directories only")
	showSizes := fs.Bool("s", false, "Print the size of each file")
	fs.BoolVar(showSizes, "sizes", false, "Print the size of each file")
	pwd := fs.String("p", "", "Decryption password")

	posArgs, err := argkit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(posArgs) < 1 {
		return errors.New("archive file not specified (usage: seal tree [flags] <archive-path> [patterns...])")
	}

	archivePath := posArgs[0]
	patterns := posArgs[1:]

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	var header [512]byte
	n, _ := f.ReadAt(header[:], 0)
	cType := container.SniffHeader(header[:n])
	if cType == container.Unknown {
		cType = util.DetectContainerType(archivePath)
	}

	engine, err := seal.DefaultContainer(cType)
	if err != nil {
		return fmt.Errorf("unsupported archive format %s: %w", cType, err)
	}

	archiveInstance := seal.NewArchive(
		seal.WithContainer(cType),
		seal.WithPassword(*pwd),
	)

	files, err := archiveInstance.Load(f, stat.Size(), engine)
	if err != nil {
		return err
	}

	// Build tree hierarchy
	root := newTreeNode(".", true)

	for _, entry := range files {
		cleanName := strings.TrimPrefix(strings.ReplaceAll(entry.Name(), "\\", "/"), "/")
		if cleanName == "" {
			continue
		}

		if !entry.IsDir() {
			if *dirsOnly {
				continue
			}
			if len(patterns) > 0 && !util.MatchFilters(cleanName, false, patterns, nil, true) {
				continue
			}
		}

		parts := strings.Split(cleanName, "/")
		current := root

		for i, part := range parts {
			if part == "" {
				continue
			}

			isLast := (i == len(parts)-1)
			isDir := !isLast || entry.IsDir()

			child, exists := current.Children[part]
			if !exists {
				child = newTreeNode(part, isDir)
				current.Children[part] = child
			} else if isDir {
				child.IsDir = true
			}

			if isLast && !entry.IsDir() {
				child.Size = entry.UncompressedSize()
			}
			current = child
		}
	}

	var dirCount, fileCount, totalBytes int64
	var sb strings.Builder
	sb.WriteString("\n" + tuikit.Bold(archivePath) + "\n")
	renderTreeNode(root, "", 0, *depth, *showSizes, *dirsOnly, &sb, &dirCount, &fileCount, &totalBytes)

	var summary string
	if *dirsOnly {
		summary = fmt.Sprintf("\n%s\n", tuikit.Cyan(fmt.Sprintf("%d directories", dirCount)))
	} else {
		summary = fmt.Sprintf("\n%s", tuikit.Cyan(fmt.Sprintf("%d directories, %d files", dirCount, fileCount)))
		if totalBytes > 0 {
			summary += fmt.Sprintf(" (%s uncompressed)", util.FormatBytes(totalBytes))
		}
		summary += "\n"
	}
	sb.WriteString(summary)

	fmt.Fprintln(stdout, sb.String())
	return nil
}

func renderTreeNode(
	node *treeNode,
	prefix string,
	currentDepth, maxDepth int,
	showSizes, dirsOnly bool,
	sb *strings.Builder,
	dirCount, fileCount, totalBytes *int64,
) {
	if maxDepth > 0 && currentDepth >= maxDepth {
		return
	}

	keys := make([]string, 0, len(node.Children))
	for k := range node.Children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, name := range keys {
		child := node.Children[name]
		isLast := i == len(keys)-1

		connector := "├── "
		nextIndent := "│   "
		if isLast {
			connector = "└── "
			nextIndent = "    "
		}

		if child.IsDir {
			*dirCount++
			sb.WriteString(prefix)
			sb.WriteString(connector)
			sb.WriteString(tuikit.Bold(tuikit.Cyan(name)))
			sb.WriteString("/\n")
			renderTreeNode(child, prefix+nextIndent, currentDepth+1, maxDepth, showSizes, dirsOnly, sb, dirCount, fileCount, totalBytes)
		} else if !dirsOnly {
			*fileCount++
			*totalBytes += child.Size
			sb.WriteString(prefix)
			sb.WriteString(connector)
			sb.WriteString(name)
			if showSizes {
				sb.WriteString("  ")
				sb.WriteString(tuikit.Magenta("[" + util.FormatBytes(child.Size) + "]"))
			}
			sb.WriteString("\n")
		}
	}
}
