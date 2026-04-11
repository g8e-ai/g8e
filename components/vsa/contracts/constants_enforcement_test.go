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

/*
Constants Enforcement Tests

Scans VSA source files for raw string literals that should use constants
from constants/events.go, constants/status.go, models/base.go, or models/file_edit.go.

This test uses Go's AST parser to extract constant values from source-of-truth
files and flags any raw usage in services, models, config, and root-level files.

Exclusions:
  - The constants definition files themselves (constants/ package)
  - Test files (*_test.go)
  - Test utilities (testutil/ package)
  - End-to-end tests (e2e/ package)
  - String values inside const blocks (where constants are defined)
  - String values that are too generic to enforce (allowlisted)
*/
package contracts

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var vsaRoot string

func init() {
	// Resolve VSA root relative to this test file's package (contracts/)
	var err error
	vsaRoot, err = filepath.Abs(filepath.Join(".."))
	if err != nil {
		panic(fmt.Sprintf("failed to resolve VSA root: %v", err))
	}
}

// scanDirs are the directories to scan for violations (relative to VSA root)
var scanDirs = []string{"services", "models", "config"}

// scanRootFiles are individual root-level files to scan
var scanRootFiles = []string{"main.go", "terminal_darwin.go", "terminal_linux.go"}

// excludePatterns are path substrings that exclude a file from scanning
var excludePatterns = []string{
	"_test.go",
	"/testutil/",
	"/e2e/",
	"/contracts/",
	"/constants/",
}

// constantSourceFiles are the files that define constants (relative to VSA root).
// Map key is the display name, value is the relative path.
var constantSourceFiles = map[string]string{
	"events.go":    "constants/events.go",
	"status.go":    "constants/status.go",
	"file_edit.go": "models/file_edit.go",
}

// allowlistedValues are constant values too generic/short to enforce.
// These appear in many unrelated contexts and would cause false positives.
var allowlistedValues = map[string]bool{
	// ExecutionStatus values - very common single-word strings
	"pending":   true,
	"executing": true,
	"completed": true,
	"failed":    true,
	"timeout":   true,
	"cancelled": true,

	// FileEditOperation values - very common single-word strings
	"read":    true,
	"write":   true,
	"replace": true,
	"insert":  true,
	"delete":  true,
	"patch":   true,

	// VaultMode values - short and used in comments/other contexts
	"raw":      true,
	"scrubbed": true,

	// AuthMode values - also used as CLI flag names, slog key names, and sentinel pattern strings
	"api_key":          true,
	"operator_session": true,
	"listen":           true,
}

// constantInfo tracks where a constant value is defined
type constantInfo struct {
	ConstantName string // e.g. "EventTypeOperatorHeartbeat"
	SourceFile   string // e.g. "events.go"
}

// violation tracks a raw string literal that should use a constant
type violation struct {
	File         string
	Line         int
	Value        string
	ConstantName string
	SourceFile   string
}

// extractStringLit strips quotes from a Go string literal token value.
func extractStringLit(lit string) string {
	return strings.Trim(lit, "`"+`"'`)
}

// extractConstantsFromFile parses a Go file and extracts all string values
// from const blocks AND from var struct composite literals (nested key:value
// pairs where the value is a string literal).
func extractConstantsFromFile(filePath string, displayName string) (map[string]constantInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filePath, err)
	}

	result := make(map[string]constantInfo)

	ast.Inspect(node, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.GenDecl:
			if decl.Tok == token.CONST {
				for _, spec := range decl.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					constName := ""
					if len(valueSpec.Names) > 0 {
						constName = valueSpec.Names[0].Name
					}
					for _, value := range valueSpec.Values {
						basicLit, ok := value.(*ast.BasicLit)
						if !ok || basicLit.Kind != token.STRING {
							continue
						}
						strVal := extractStringLit(basicLit.Value)
						if strVal != "" {
							result[strVal] = constantInfo{ConstantName: constName, SourceFile: displayName}
						}
					}
				}
			} else if decl.Tok == token.VAR {
				for _, spec := range decl.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					varName := ""
					if len(valueSpec.Names) > 0 {
						varName = valueSpec.Names[0].Name
					}
					for _, val := range valueSpec.Values {
						extractCompositeLitStrings(val, varName, displayName, result)
					}
				}
			}
		}
		return true
	})

	return result, nil
}

// extractCompositeLitStrings recursively walks a composite literal expression
// and collects all string literal values into result.
func extractCompositeLitStrings(expr ast.Expr, ownerName string, displayName string, result map[string]constantInfo) {
	switch e := expr.(type) {
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			switch kv := elt.(type) {
			case *ast.KeyValueExpr:
				extractCompositeLitStrings(kv.Value, ownerName, displayName, result)
			default:
				extractCompositeLitStrings(elt, ownerName, displayName, result)
			}
		}
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			strVal := extractStringLit(e.Value)
			if strVal != "" {
				result[strVal] = constantInfo{ConstantName: ownerName, SourceFile: displayName}
			}
		}
	}
}

// buildEnforcedValues collects all constant values from source-of-truth files,
// filters out allowlisted values, and returns the enforced set.
func buildEnforcedValues() (map[string]constantInfo, map[string]map[string]constantInfo, error) {
	allConstants := make(map[string]map[string]constantInfo) // file -> {value -> info}
	enforced := make(map[string]constantInfo)                // value -> info

	for displayName, relPath := range constantSourceFiles {
		fullPath := filepath.Join(vsaRoot, relPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		constants, err := extractConstantsFromFile(fullPath, displayName)
		if err != nil {
			return nil, nil, err
		}

		allConstants[displayName] = constants

		for value, info := range constants {
			if allowlistedValues[value] {
				continue
			}
			enforced[value] = info
		}
	}

	return enforced, allConstants, nil
}

// getFilesToScan returns all .go source files in scan directories,
// excluding test files and other excluded patterns.
func getFilesToScan() ([]string, error) {
	var files []string

	// Scan directories
	for _, dir := range scanDirs {
		dirPath := filepath.Join(vsaRoot, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			for _, pattern := range excludePatterns {
				if strings.Contains(path, pattern) {
					return nil
				}
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Scan root-level files
	for _, fileName := range scanRootFiles {
		fullPath := filepath.Join(vsaRoot, fileName)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}
		files = append(files, fullPath)
	}

	return files, nil
}

// findViolationsInFile parses a Go source file and finds raw string literals
// that match enforced constant values.
func findViolationsInFile(filePath string, enforced map[string]constantInfo) ([]violation, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filePath, err)
	}

	var violations []violation

	// Track positions of const blocks to skip string literals inside them
	constRanges := make([][2]token.Pos, 0)
	ast.Inspect(node, func(n ast.Node) bool {
		genDecl, ok := n.(*ast.GenDecl)
		if ok && genDecl.Tok == token.CONST {
			constRanges = append(constRanges, [2]token.Pos{genDecl.Pos(), genDecl.End()})
		}
		return true
	})

	isInConstBlock := func(pos token.Pos) bool {
		for _, r := range constRanges {
			if pos >= r[0] && pos <= r[1] {
				return true
			}
		}
		return false
	}

	ast.Inspect(node, func(n ast.Node) bool {
		basicLit, ok := n.(*ast.BasicLit)
		if !ok || basicLit.Kind != token.STRING {
			return true
		}

		// Skip strings inside const blocks (the definitions themselves)
		if isInConstBlock(basicLit.Pos()) {
			return true
		}

		// Strip quotes
		strVal := strings.Trim(basicLit.Value, `"'`+"`")

		info, found := enforced[strVal]
		if !found {
			return true
		}

		relPath, _ := filepath.Rel(vsaRoot, filePath)
		pos := fset.Position(basicLit.Pos())

		violations = append(violations, violation{
			File:         relPath,
			Line:         pos.Line,
			Value:        strVal,
			ConstantName: info.ConstantName,
			SourceFile:   info.SourceFile,
		})

		return true
	})

	return violations, nil
}

// =============================================================================
// TESTS
// =============================================================================

func TestExtractConstantsFromEvents(t *testing.T) {
	fullPath := filepath.Join(vsaRoot, "constants/events.go")
	constants, err := extractConstantsFromFile(fullPath, "events.go")
	require.NoError(t, err)
	require.NotEmpty(t, constants, "should extract constants from events.go")

	t.Logf("Extracted %d constants from events.go", len(constants))

	// Verify key constants exist
	_, hasHeartbeat := constants["g8e.v1.operator.heartbeat.sent"]
	assert.True(t, hasHeartbeat, "should contain g8e.v1.operator.heartbeat.sent")

	_, hasCommandReq := constants["g8e.v1.operator.command.requested"]
	assert.True(t, hasCommandReq, "should contain g8e.v1.operator.command.requested")
}

func TestExtractConstantsFromVault(t *testing.T) {
	fullPath := filepath.Join(vsaRoot, "constants/status.go")
	constants, err := extractConstantsFromFile(fullPath, "status.go")
	require.NoError(t, err)
	require.NotEmpty(t, constants, "should extract constants from status.go")

	t.Logf("Extracted %d constants from status.go", len(constants))

	_, hasRaw := constants["raw"]
	assert.True(t, hasRaw, "should contain raw (VaultMode.Raw)")

	_, hasScrubbed := constants["scrubbed"]
	assert.True(t, hasScrubbed, "should contain scrubbed (VaultMode.Scrubbed)")
}

func TestExtractConstantsFromModels(t *testing.T) {
	t.Run("base.go execution statuses", func(t *testing.T) {
		fullPath := filepath.Join(vsaRoot, "models/base.go")
		_, err := extractConstantsFromFile(fullPath, "base.go")
		require.NoError(t, err, "base.go must parse without error")
	})

	t.Run("file_edit.go operations", func(t *testing.T) {
		fullPath := filepath.Join(vsaRoot, "models/file_edit.go")
		constants, err := extractConstantsFromFile(fullPath, "file_edit.go")
		require.NoError(t, err)
		require.NotEmpty(t, constants, "should extract constants from file_edit.go")

		t.Logf("Extracted %d constants from file_edit.go", len(constants))
	})
}

func TestEnforcedValuesAfterAllowlist(t *testing.T) {
	enforced, _, err := buildEnforcedValues()
	require.NoError(t, err)
	require.NotEmpty(t, enforced, "should have enforced values after filtering allowlist")

	t.Logf("Enforcing %d constant values (%d allowlisted)", len(enforced), len(allowlistedValues))

	values := make([]string, 0, len(enforced))
	for v := range enforced {
		values = append(values, v)
	}
	sort.Strings(values)
	t.Logf("Enforced values: %s", strings.Join(values, ", "))
}

func TestNoRawStringLiteralsWhereConstantsExist(t *testing.T) {
	enforced, _, err := buildEnforcedValues()
	require.NoError(t, err)

	files, err := getFilesToScan()
	require.NoError(t, err)
	require.NotEmpty(t, files, "should find source files to scan")

	var allViolations []violation

	for _, filePath := range files {
		fileViolations, err := findViolationsInFile(filePath, enforced)
		require.NoError(t, err, "failed to scan %s", filePath)
		allViolations = append(allViolations, fileViolations...)
	}

	if len(allViolations) > 0 {
		// Group by file for readable output
		byFile := make(map[string][]violation)
		for _, v := range allViolations {
			byFile[v.File] = append(byFile[v.File], v)
		}

		var report strings.Builder
		report.WriteString(fmt.Sprintf("\nFound %d raw string literal(s) that should use constants:\n", len(allViolations)))

		fileKeys := make([]string, 0, len(byFile))
		for k := range byFile {
			fileKeys = append(fileKeys, k)
		}
		sort.Strings(fileKeys)

		for _, file := range fileKeys {
			vList := byFile[file]
			report.WriteString(fmt.Sprintf("\n  %s:\n", file))
			for _, v := range vList {
				report.WriteString(fmt.Sprintf("    Line %d: %q -> use %s from %s\n",
					v.Line, v.Value, v.ConstantName, v.SourceFile))
			}
		}

		assert.Empty(t, allViolations, report.String())
	}
}

func TestScansMeaningfulNumberOfSourceFiles(t *testing.T) {
	files, err := getFilesToScan()
	require.NoError(t, err)

	t.Logf("Scanned %d source files across: %s + root files", len(files), strings.Join(scanDirs, ", "))
	assert.Greater(t, len(files), 10, "should scan a meaningful number of source files")
}
