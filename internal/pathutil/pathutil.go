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

package pathutil

import (
	"path/filepath"
	"runtime"
	"strings"
)

// SafeJoin safely joins path elements, handling the case where the second
// path is already absolute. On Windows, if the second path is absolute,
// it returns the second path instead of incorrectly concatenating them.
//
// This prevents issues like: filepath.Join("C:\\temp", "C:\\temp\\file.db")
// resulting in "C:\\temp\\C:\\temp\\file.db"
//
// Examples:
//   - SafeJoin("/tmp", "data.db") -> "/tmp/data.db"
//   - SafeJoin("C:\\temp", "data.db") -> "C:\\temp\\data.db"
//   - SafeJoin("C:\\temp", "C:\\temp\\data.db") -> "C:\\temp\\data.db" (Windows)
//   - SafeJoin("/tmp", "/tmp/data.db") -> "/tmp/data.db" (Unix)
func SafeJoin(base string, elem ...string) string {
	if len(elem) == 0 {
		return base
	}

	// If the first element is already absolute, use it as-is
	if filepath.IsAbs(elem[0]) {
		if len(elem) == 1 {
			return filepath.Clean(elem[0])
		}
		return filepath.Join(elem...)
	}

	// Otherwise, join all elements with the base
	return filepath.Join(append([]string{base}, elem...)...)
}

// ResolveDBPath resolves a database path relative to a data directory.
// If dbPath is already absolute, it returns dbPath as-is.
// Otherwise, it joins dataDir and dbPath.
//
// This is specifically designed to handle the common pattern where
// configuration may specify either:
//   - A relative path like "g8e.db" (should be joined with dataDir)
//   - An absolute path like "/var/lib/g8e/g8e.db" (should be used as-is)
func ResolveDBPath(dataDir, dbPath string) string {
	return SafeJoin(dataDir, dbPath)
}

// NormalizePath normalizes a path for the current OS.
// On Windows, it converts forward slashes to backslashes.
// On Unix, it converts backslashes to forward slashes.
// It also cleans the path to remove redundant separators.
func NormalizePath(path string) string {
	if path == "" {
		return path
	}

	// Clean the path first
	path = filepath.Clean(path)

	// On Windows, ensure backslashes
	if runtime.GOOS == "windows" {
		path = filepath.FromSlash(path)
	}

	return path
}

// IsWindowsAbsPath checks if a path is an absolute Windows path.
// This includes paths starting with drive letters (C:, D:, etc.)
// or UNC paths (\\server\share).
func IsWindowsAbsPath(path string) bool {
	if len(path) < 2 {
		return false
	}

	// Check for drive letter (C:, D:, etc.)
	if path[1] == ':' && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) {
		return true
	}

	// Check for UNC path (\\server\share or //server/share)
	if (path[0] == '\\' && path[1] == '\\') || (path[0] == '/' && path[1] == '/') {
		return true
	}

	return false
}

// ToSlash converts path separators to forward slashes.
// This is useful for logging and display purposes where
// forward slashes are more universally readable.
func ToSlash(path string) string {
	return filepath.ToSlash(path)
}

// FromSlash converts forward slashes to OS-specific separators.
// This is useful when reading paths from configuration files
// that use forward slashes.
func FromSlash(path string) string {
	return filepath.FromSlash(path)
}

// EnsureTrailingSeparator ensures the path ends with a separator.
// This is useful for directory paths that need to be clearly
// distinguished from file paths.
func EnsureTrailingSeparator(path string) string {
	if path == "" {
		return path
	}
	if !strings.HasSuffix(path, string(filepath.Separator)) {
		return path + string(filepath.Separator)
	}
	return path
}

// RemoveTrailingSeparator removes any trailing separator from the path.
func RemoveTrailingSeparator(path string) string {
	return strings.TrimSuffix(path, string(filepath.Separator))
}

// Made with Bob
