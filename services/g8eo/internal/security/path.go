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

package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidatePath ensures the path is safe and valid within the given root.
// It cleans the path, checks for traversal attempts, and resolves it against the root.
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

	// Ensure the path is within the root directory
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return "", fmt.Errorf("failed to calculate relative path: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path is outside of the root directory")
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
