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

package system

import (
	"os"
	"path/filepath"
)

// ResolveProjectRoot returns the absolute path to the project root.
// Walks up from current working directory until it detects the repository root.
func ResolveProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "." // Fallback to current working directory
	}

	// Try to find the root by looking for protocol or .git
	current := cwd
	for {
		// Check for markers of the repository root
		// protocol/ is the canonical marker for the g8e repository
		// .git is the standard git repository marker
		_, protocolErr := os.Stat(filepath.Join(current, "protocol"))
		_, gitErr := os.Stat(filepath.Join(current, ".git"))

		if protocolErr == nil || gitErr == nil {
			// Either marker found - this is the repository root
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding markers
			// Fall back to CWD
			return cwd
		}
		current = parent
	}
}
