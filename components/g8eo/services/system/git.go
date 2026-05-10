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
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResolveGitBinary finds the git binary using the following resolution order:
// 1. Sibling bin/git relative to the running operator binary
// 2. System PATH
// Returns the absolute path to git, or empty string if not found.
func ResolveGitBinary(logger *slog.Logger) string {
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		siblingGit := filepath.Join(execDir, "bin", "git")
		if isExecutable(siblingGit) {
			absPath, _ := filepath.Abs(siblingGit)
			logger.Info("Git binary found (bundled)", "path", absPath)
			return absPath
		}
	}

	if systemGit, err := exec.LookPath("git"); err == nil {
		absPath, _ := filepath.Abs(systemGit)
		logger.Info("Git binary found (system)", "path", absPath)
		return absPath
	}

	logger.Warn("Git binary not found — ledger (git-backed file versioning) will be disabled")
	return ""
}

// ValidateGitBinary verifies the resolved git binary is functional and returns the version string.
func ValidateGitBinary(gitPath string) (string, error) {
	if gitPath == "" {
		return "", fmt.Errorf("no git binary path provided")
	}

	cmd := exec.Command(gitPath, "version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git binary at %s is not functional: %w", gitPath, err)
	}

	version := strings.TrimSpace(string(out))
	return version, nil
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Mode()&0111 != 0
}

func truncateHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}
