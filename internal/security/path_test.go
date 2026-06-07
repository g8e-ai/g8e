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
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidatePath(t *testing.T) {
	root := "/safe/root"

	tests := []struct {
		name    string
		path    string
		root    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty path",
			path:    "",
			root:    root,
			wantErr: true,
			errMsg:  "empty path",
		},
		{
			name:    "simple relative path within root",
			path:    "file.txt",
			root:    root,
			wantErr: false,
		},
		{
			name:    "relative path with subdirectory",
			path:    "subdir/file.txt",
			root:    root,
			wantErr: false,
		},
		{
			name:    "relative path with multiple slashes",
			path:    "subdir//file.txt",
			root:    root,
			wantErr: false,
		},
		{
			name:    "relative path with trailing slash",
			path:    "subdir/",
			root:    root,
			wantErr: false,
		},
		{
			name:    "relative path with dot segments",
			path:    "./subdir/file.txt",
			root:    root,
			wantErr: false,
		},
		{
			name:    "path traversal attempt with ..",
			path:    "../etc/passwd",
			root:    root,
			wantErr: true,
			errMsg:  "path traversal attempt detected",
		},
		{
			name:    "path traversal attempt with .. in middle",
			path:    "subdir/../../etc/passwd",
			root:    root,
			wantErr: true,
			errMsg:  "path traversal attempt detected",
		},
		{
			name:    "path traversal attempt with .. at end (cleaned to .)",
			path:    "subdir/..",
			root:    root,
			wantErr: false,
		},
		{
			name:    "path with dot segments that resolve safely",
			path:    "subdir/./../etc/passwd",
			root:    root,
			wantErr: false,
		},
		{
			name:    "relative path outside root via ..",
			path:    "../../../etc/passwd",
			root:    root,
			wantErr: true,
			errMsg:  "path traversal attempt detected",
		},
		{
			name:    "absolute path allowed for system paths",
			path:    "/etc/passwd",
			root:    root,
			wantErr: false,
		},
		{
			name:    "absolute path with .. (cleaned by filepath.Clean)",
			path:    "/etc/../passwd",
			root:    root,
			wantErr: false,
		},
		{
			name:    "absolute path with multiple slashes",
			path:    "/etc//passwd",
			root:    root,
			wantErr: false,
		},
		{
			name:    "absolute path with dot segments",
			path:    "/etc/./passwd",
			root:    root,
			wantErr: false,
		},
		{
			name:    "deep relative path within root",
			path:    "a/b/c/d/e/f/g/h/i/j/file.txt",
			root:    root,
			wantErr: false,
		},
		{
			name:    "relative path escaping root after resolution",
			path:    "subdir/../../../etc/passwd",
			root:    root,
			wantErr: true,
			errMsg:  "path traversal attempt detected",
		},
		{
			name:    "Windows-style backslash path",
			path:    "subdir\\file.txt",
			root:    root,
			wantErr: false,
		},
		{
			name:    "path with only dots",
			path:    "...",
			root:    root,
			wantErr: true,
			errMsg:  "path traversal attempt detected",
		},
		{
			name:    "path with double dot prefix",
			path:    "..file.txt",
			root:    root,
			wantErr: true,
			errMsg:  "path traversal attempt detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidatePath(tt.path, tt.root)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if err.Error() != tt.errMsg {
					t.Errorf("ValidatePath() error message = %q, want %q", err.Error(), tt.errMsg)
				}
			}
			if !tt.wantErr && got == "" {
				t.Errorf("ValidatePath() returned empty path on success")
			}
		})
	}
}

func TestValidatePathResolution(t *testing.T) {
	// Test that the returned path is absolute and cleaned
	root := "/safe/root"
	if runtime.GOOS == "windows" {
		root = `C:\safe\root`
	}

	t.Run("relative path becomes absolute", func(t *testing.T) {
		got, err := ValidatePath("file.txt", root)
		if err != nil {
			t.Fatalf("ValidatePath() unexpected error: %v", err)
		}
		if !filepath.IsAbs(got) {
			t.Errorf("ValidatePath() returned non-absolute path: %s", got)
		}
		expected := filepath.Join(root, "file.txt")
		if got != expected {
			t.Errorf("ValidatePath() = %s, want %s", got, expected)
		}
	})

	t.Run("absolute path remains absolute", func(t *testing.T) {
		absPath := "/etc/passwd"
		if runtime.GOOS == "windows" {
			absPath = "C:\\etc\\passwd"
		}
		got, err := ValidatePath(absPath, root)
		if err != nil {
			t.Fatalf("ValidatePath() unexpected error: %v", err)
		}
		if !filepath.IsAbs(got) {
			t.Errorf("ValidatePath() returned non-absolute path: %s", got)
		}
		if got != absPath {
			t.Errorf("ValidatePath() = %s, want %s", got, absPath)
		}
	})

	t.Run("path is cleaned", func(t *testing.T) {
		got, err := ValidatePath("subdir//./file.txt", root)
		if err != nil {
			t.Fatalf("ValidatePath() unexpected error: %v", err)
		}
		expected := filepath.Join(root, "subdir", "file.txt")
		if got != expected {
			t.Errorf("ValidatePath() = %s, want %s", got, expected)
		}
	})
}

func TestValidatePathCrossPlatform(t *testing.T) {
	root := "/safe/root"

	t.Run("relative path stays within root", func(t *testing.T) {
		got, err := ValidatePath("subdir/file.txt", root)
		if err != nil {
			t.Fatalf("ValidatePath() unexpected error: %v", err)
		}

		rel, err := filepath.Rel(root, got)
		if err != nil {
			t.Fatalf("filepath.Rel() failed: %v", err)
		}

		if rel == ".." || len(rel) >= 3 && rel[0:3] == ".." {
			t.Errorf("ValidatePath() allowed path outside root: %s (relative: %s)", got, rel)
		}
	})

	t.Run("absolute path not constrained to root", func(t *testing.T) {
		// Absolute paths should be allowed for test fixtures and system paths
		absPath := "/tmp/test.txt"
		if runtime.GOOS == "windows" {
			absPath = "C:\\temp\\test.txt"
		}

		got, err := ValidatePath(absPath, root)
		if err != nil {
			t.Fatalf("ValidatePath() unexpected error for absolute path: %v", err)
		}

		// Absolute paths should be returned as-is (cleaned)
		if !filepath.IsAbs(got) {
			t.Errorf("ValidatePath() returned non-absolute path for absolute input: %s", got)
		}
	})
}

func TestIsShellRequired(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{
			name:     "simple command without metacharacters",
			command:  "ls -la",
			expected: false,
		},
		{
			name:     "command with pipe",
			command:  "ls | grep test",
			expected: true,
		},
		{
			name:     "command with background operator",
			command:  "sleep 10 &",
			expected: true,
		},
		{
			name:     "command with output redirection",
			command:  "echo test > file.txt",
			expected: true,
		},
		{
			name:     "command with input redirection",
			command:  "cat < file.txt",
			expected: true,
		},
		{
			name:     "command with variable expansion",
			command:  "echo $HOME",
			expected: true,
		},
		{
			name:     "command with subshell",
			command:  "$(echo test)",
			expected: true,
		},
		{
			name:     "command with command separator",
			command:  "ls; pwd",
			expected: true,
		},
		{
			name:     "command with backtick execution",
			command:  "`echo test`",
			expected: true,
		},
		{
			name:     "command with escape character",
			command:  "echo \\n",
			expected: true,
		},
		{
			name:     "command with glob asterisk",
			command:  "ls *.txt",
			expected: true,
		},
		{
			name:     "command with glob question mark",
			command:  "ls file?.txt",
			expected: true,
		},
		{
			name:     "command with glob brackets",
			command:  "ls file[0-9].txt",
			expected: true,
		},
		{
			name:     "command with home directory expansion",
			command:  "ls ~/Documents",
			expected: true,
		},
		{
			name:     "command with logical AND",
			command:  "ls && pwd",
			expected: true,
		},
		{
			name:     "empty command",
			command:  "",
			expected: false,
		},
		{
			name:     "whitespace only",
			command:  "   ",
			expected: false,
		},
		{
			name:     "command with path only",
			command:  "/usr/bin/ls",
			expected: false,
		},
		{
			name:     "command with single dash",
			command:  "ls -",
			expected: false,
		},
		{
			name:     "command with equals sign",
			command:  "VAR=value command",
			expected: false,
		},
		{
			name:     "command with quotes",
			command:  "echo 'test'",
			expected: false,
		},
		{
			name:     "command with double quotes",
			command:  "echo \"test\"",
			expected: false,
		},
		{
			name:     "command with newline (escaped)",
			command:  "echo line1\\nline2",
			expected: true,
		},
		{
			name:     "command with tab (escaped)",
			command:  "echo col1\\tcol2",
			expected: true,
		},
		{
			name:     "command with curly braces",
			command:  "echo {a,b,c}",
			expected: false,
		},
		{
			name:     "command with exclamation mark",
			command:  "echo test!",
			expected: false,
		},
		{
			name:     "command with at sign",
			command:  "echo test@",
			expected: false,
		},
		{
			name:     "command with hash",
			command:  "echo test#",
			expected: false,
		},
		{
			name:     "command with percent",
			command:  "echo test%",
			expected: false,
		},
		{
			name:     "command with caret",
			command:  "echo test^",
			expected: false,
		},
		{
			name:     "command with underscore",
			command:  "echo test_",
			expected: false,
		},
		{
			name:     "command with plus",
			command:  "echo test+",
			expected: false,
		},
		{
			name:     "command with equals",
			command:  "echo test=",
			expected: false,
		},
		{
			name:     "command with pipe in quotes",
			command:  "echo '|'",
			expected: true,
		},
		{
			name:     "command with multiple metacharacters",
			command:  "ls | grep test > output.txt &",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsShellRequired(tt.command)
			if got != tt.expected {
				t.Errorf("IsShellRequired(%q) = %v, want %v", tt.command, got, tt.expected)
			}
		})
	}
}

func TestIsShellRequiredAllMetacharacters(t *testing.T) {
	// Test each metacharacter individually to ensure detection
	metacharacters := []string{
		"|", "&", ">", "<", "$", "(", ")", ";", "`", "\\", "*", "?", "[", "]", "~",
	}

	for _, mc := range metacharacters {
		t.Run("metacharacter_"+mc, func(t *testing.T) {
			command := "echo test " + mc
			if !IsShellRequired(command) {
				t.Errorf("IsShellRequired(%q) = false, want true (metacharacter %s)", command, mc)
			}
		})
	}
}
