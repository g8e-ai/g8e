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
	"strings"
)

// ResolveProjectRoot returns the absolute path to the project root.
// Priority:
// 1. G8E_PROJECT_ROOT environment variable
// 2. Fallback: walks up from current working directory until it detects the repository root.
func ResolveProjectRoot() string {
	if root := os.Getenv("G8E_PROJECT_ROOT"); root != "" {
		abs, err := filepath.Abs(root)
		if err == nil {
			return abs
		}
		return root
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "/home/g8e" // Absolute fallback for container/standard environments
	}

	// Try to find the root by looking for services/g8eo or .git
	current := cwd
	for {
		// Check for markers of the repository root
		if _, err := os.Stat(filepath.Join(current, "components")); err == nil {
			if _, err := os.Stat(filepath.Join(current, "g8e")); err == nil {
				return current
			}
		}
		
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// Fallback to relative path from services/g8eo if we are likely there
	if strings.Contains(cwd, filepath.Join("components", "g8eo")) {
		// We are inside g8eo, likely <root>/services/g8eo
		// Walk up until we are above 'components'
		current = cwd
		for {
			if filepath.Base(current) == "g8eo" && filepath.Base(filepath.Dir(current)) == "components" {
				return filepath.Dir(filepath.Dir(current))
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	return cwd
}
