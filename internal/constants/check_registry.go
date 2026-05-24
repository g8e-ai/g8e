// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ConstantInfo holds information about a constant declaration
type ConstantInfo struct {
	Name       string
	SourceFile string
}

func main() {
	constantsDir := "."
	if len(os.Args) > 1 {
		constantsDir = os.Args[1]
	}

	// Only check files that should be exported to JSON/Python
	// Internal-only files (status.go, platform.go, agents.go, timestamp.go, kv_keys.go) are excluded
	// as they contain Go-specific enums, platform details, persona data, or internal KV schemas not needed downstream
	// Generated files (registry.go, status_generated.go, headers_generated.go) are the source of truth
	trackedFiles := map[string]bool{
		"collections.go":       true,
		"events.go":            true,
		"headers_generated.go": true,
		"channels.go":          true,
		"intents.go":           true,
		"document_ids.go":      true,
		"senders.go":           true,
		"prompts.go":           true,
		"pubsub.go":            true,
	}

	// Parse tracked constant files
	constants, err := parseTrackedConstants(constantsDir, trackedFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing constants: %v\n", err)
		os.Exit(1)
	}

	// Parse registry.go to get registered constants
	registry, err := parseRegistry(constantsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing registry: %v\n", err)
		os.Exit(1)
	}

	// Check for missing constants
	missing := findMissingConstants(constants, registry)
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "ERROR: The following constants are missing from registry.go:\n")
		for _, m := range missing {
			fmt.Fprintf(os.Stderr, "  - %s (from %s)\n", m.Name, m.SourceFile)
		}
		fmt.Fprintf(os.Stderr, "\nTotal missing: %d constants\n", len(missing))
		fmt.Fprintf(os.Stderr, "Please add these constants to registry.go to keep the registry in sync.\n")
		fmt.Fprintf(os.Stderr, "Run: go run ./internal/constants/check_registry.go\n")
		os.Exit(1)
	}

	fmt.Printf("Registry validation passed: all %d tracked constants are registered.\n", len(constants))
}

func parseTrackedConstants(dir string, trackedFiles map[string]bool) ([]ConstantInfo, error) {
	fset := token.NewFileSet()
	var constants []ConstantInfo

	for fileName := range trackedFiles {
		filePath := filepath.Join(dir, fileName)
		src, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading file %s: %w", filePath, err)
		}

		fileConstants, err := parseFileConstants(fset, filePath, src)
		if err != nil {
			return nil, fmt.Errorf("parsing file %s: %w", filePath, err)
		}

		// Add source file info to each constant
		for i := range fileConstants {
			fileConstants[i].SourceFile = fileName
		}

		constants = append(constants, fileConstants...)
	}

	return constants, nil
}

func parseFileConstants(fset *token.FileSet, filename string, src []byte) ([]ConstantInfo, error) {
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var constants []ConstantInfo

	ast.Inspect(file, func(n ast.Node) bool {
		genDecl, ok := n.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			return true
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for _, name := range valueSpec.Names {
				// Skip function-like constants (e.g., CmdChannel, ResultsChannel)
				if strings.HasPrefix(name.Name, "CmdChannel") || strings.HasPrefix(name.Name, "ResultsChannel") || strings.HasPrefix(name.Name, "HeartbeatChannel") {
					continue
				}
				// Skip template keys with braces
				if strings.Contains(name.Name, "{") {
					continue
				}
				// Skip string variants (e.g., OperatorTypeSystemStr)
				if strings.HasSuffix(name.Name, "Str") {
					continue
				}

				constants = append(constants, ConstantInfo{
					Name: name.Name,
				})
			}
		}

		return true
	})

	return constants, nil
}

func parseRegistry(dir string) (map[string]bool, error) {
	fset := token.NewFileSet()
	registryPath := filepath.Join(dir, "registry.go")
	src, err := os.ReadFile(registryPath)
	if err != nil {
		return nil, fmt.Errorf("reading registry.go: %w", err)
	}

	file, err := parser.ParseFile(fset, registryPath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing registry.go: %w", err)
	}

	registered := make(map[string]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for struct field assignments like {Value: string(CollectionUsers), GoConst: "CollectionUsers", ...}
		compositeLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		for _, elt := range compositeLit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}

			// Look for GoConst field
			if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == "GoConst" {
				if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					// Extract the constant name from the string literal
					constName := strings.Trim(lit.Value, `"`)
					registered[constName] = true
				}
			}
		}

		return true
	})

	return registered, nil
}

func findMissingConstants(constants []ConstantInfo, registry map[string]bool) []ConstantInfo {
	var missing []ConstantInfo

	for _, c := range constants {
		if !registry[c.Name] {
			missing = append(missing, c)
		}
	}

	// Sort for consistent output
	sort.Slice(missing, func(i, j int) bool {
		return missing[i].Name < missing[j].Name
	})

	return missing
}
