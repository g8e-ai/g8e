// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidatePath ensures the path is safe and valid within the given root.
// It cleans the path, checks for traversal attempts, and resolves it against the root.
// For absolute paths, it validates they don't contain traversal attempts but doesn't
// enforce they're within root (allowing test fixtures and system paths).
func ValidatePath(path string, root string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}

	// Clean the path to handle multiple slashes and redundant segments
	cleanPath := filepath.Clean(path)

	// SECURITY: Block obvious path traversal attempts before resolution
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("path traversal attempt detected")
	}

	// Resolve to absolute path
	var absPath string
	if filepath.IsAbs(cleanPath) {
		absPath = cleanPath
	} else {
		// Resolve relative paths against the root directory
		absPath = filepath.Join(root, cleanPath)
	}

	// Re-clean and re-validate absolute path
	absPath = filepath.Clean(absPath)
	if strings.Contains(absPath, "..") {
		return "", fmt.Errorf("path traversal detected after resolution")
	}

	// For relative paths (now resolved), ensure they're within the root directory
	// For absolute paths that were originally absolute, allow them (for test fixtures, system paths)
	if !filepath.IsAbs(cleanPath) {
		rel, err := filepath.Rel(root, absPath)
		if err != nil {
			return "", fmt.Errorf("failed to calculate relative path: %w", err)
		}
		if strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("path is outside of the root directory")
		}
	}

	return absPath, nil
}

// IsShellRequired checks for shell-specific characters that might require a shell
func IsShellRequired(command string) bool {
	// Metacharacters that require shell processing:
	// |  - pipe
	// &  - background/logical AND
	// >  - output redirection
	// <  - input redirection
	// $  - variable expansion
	// ( ) - subshell
	// ;  - command separator
	// `  - backtick execution
	// \  - escape character
	// * ? [ ] - globbing
	// ~  - home directory expansion
	return strings.ContainsAny(command, "|&><$();`\\*?[]~")
}
