// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// sourcefiles prints portable Makefile dependencies from source directories.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/buildinfo"
)

func main() {
	os.Exit(runSourcefiles(os.Args[1:], os.Stdout, os.Stderr))
}

func runSourcefiles(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sourcefiles", flag.ContinueOnError)
	flags.SetOutput(stderr)
	base := flags.String("base", ".", "repository root the source entries are relative to")
	exclude := flags.String("exclude", "", "comma-separated path-component patterns to skip")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	entries := flags.Args()
	if len(entries) == 0 {
		fmt.Fprintln(stderr, "sourcefiles: at least one source entry is required")
		return 1
	}
	paths, err := buildinfo.ListSourceManifestFiles(*base, entries, splitSourceExcludes(*exclude))
	if err != nil {
		fmt.Fprintf(stderr, "sourcefiles: %v\n", err)
		return 1
	}
	for _, path := range paths {
		fmt.Fprintln(stdout, path)
	}
	return 0
}

func splitSourceExcludes(csv string) []string {
	if csv == "" {
		return nil
	}
	var excludes []string
	for _, value := range strings.Split(csv, ",") {
		if value = strings.TrimSpace(value); value != "" {
			excludes = append(excludes, value)
		}
	}
	return excludes
}
