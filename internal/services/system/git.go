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
	"log/slog"
	"os"
	"runtime"

	"github.com/g8e-ai/g8e/internal/constants"
)

const GitEmbedded = "embedded"

// ResolveGitBinary is a stub for native go-git migration.
func ResolveGitBinary(logger *slog.Logger) string {
	return GitEmbedded
}

// ValidateGitBinary is a stub for native go-git migration.
func ValidateGitBinary(gitPath string) (string, error) {
	if gitPath == "" {
		return "", constants.ErrMCPGitOpsBinaryPathRequired
	}
	return "go-git v5 (embedded)", nil
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true // On Windows, if it exists and is not a dir, we consider it executable for these purposes
	}
	return info.Mode()&0111 != 0
}

func truncateHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}
