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
// Manifest mode walks the positional entries relative to -base. The -exclude
// value is a comma-separated list of filepath.Match patterns matched against
// each path component.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/buildinfo"
)

type manifestHashFunc func(string, []string, []string) (string, error)

func main() {
	os.Exit(runTreehash(os.Args[1:], os.Stdout, os.Stderr, buildinfo.ComputeSourceManifestHash))
}

func runTreehash(args []string, stdout, stderr io.Writer, hashManifest manifestHashFunc) int {
	flags := flag.NewFlagSet("treehash", flag.ContinueOnError)
	flags.SetOutput(stderr)
	base := flags.String("base", ".", "repository root the manifest entries are relative to")
	exclude := flags.String("exclude", "", "comma-separated path-component patterns skipped in manifest mode")
	mode := flags.String("mode", "manifest", "collection mode: manifest")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *mode != "manifest" {
		fmt.Fprintf(stderr, "treehash: unknown mode %q (want manifest)\n", *mode)
		return 1
	}
	entries := flags.Args()
	if len(entries) == 0 {
		fmt.Fprintln(stderr, "treehash: manifest mode requires at least one source entry")
		return 1
	}
	hash, err := hashManifest(*base, entries, splitCSV(*exclude))
	if err != nil {
		fmt.Fprintf(stderr, "treehash: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, hash)
	return 0
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
