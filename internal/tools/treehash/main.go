// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// treehash prints the canonical source-tree state hash used for build-time
// provenance stamping. The Makefile stamps its output into the binary as
// main.sourceTreeHash; `g8e version --json` surfaces it and the evals
// provenance bridge consumes it.
//
// Modes:
//
//	auto     (default) tracked-source hashing when -base is inside a git
//	         work tree, manifest hashing otherwise
//	git      tracked-source hashing only (git ls-files over the work tree)
//	manifest walk the positional entries relative to -base
//
// In manifest mode, -exclude is a comma-separated list of filepath.Match
// patterns matched against each path component (e.g. ".venv,node_modules,
// __pycache__,*.egg-info").
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/buildinfo"
)

func main() {
	base := flag.String("base", ".", "repository root the manifest entries are relative to")
	exclude := flag.String("exclude", "", "comma-separated path-component patterns skipped in manifest mode")
	mode := flag.String("mode", "auto", "collection mode: auto, git, or manifest")
	flag.Parse()

	excludes := splitCSV(*exclude)
	entries := flag.Args()

	var hash string
	var err error
	switch *mode {
	case "auto":
		hash, err = buildinfo.SourceTreeHash(*base, entries, excludes)
	case "git":
		hash, err = buildinfo.ComputeTrackedSourceHash(*base)
	case "manifest":
		if len(entries) == 0 {
			err = fmt.Errorf("manifest mode requires at least one source entry")
		} else {
			hash, err = buildinfo.ComputeSourceManifestHash(*base, entries, excludes)
		}
	default:
		err = fmt.Errorf("unknown mode %q (want auto, git, or manifest)", *mode)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "treehash: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(hash)
}

func splitCSV(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := parts[:0]
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
