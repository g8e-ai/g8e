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

// ToSlash converts path separators to forward slashes.
// This is useful for logging and display purposes where
// forward slashes are more universally readable.
func ToSlash(path string) string {
	return filepath.ToSlash(path)
}

// Made with Bob
