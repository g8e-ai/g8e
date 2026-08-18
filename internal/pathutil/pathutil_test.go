// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pathutil

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeJoin(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		elem     []string
		expected string
		osCheck  string // "windows", "unix", or "" for all
	}{
		{
			name:     "relative path on unix",
			base:     "/tmp",
			elem:     []string{"data.db"},
			expected: "/tmp/data.db",
			osCheck:  "unix",
		},
		{
			name:     "relative path on windows",
			base:     "C:\\temp",
			elem:     []string{"data.db"},
			expected: "C:\\temp\\data.db",
			osCheck:  "windows",
		},
		{
			name:     "absolute path on unix should use absolute",
			base:     "/tmp",
			elem:     []string{"/var/lib/data.db"},
			expected: "/var/lib/data.db",
			osCheck:  "unix",
		},
		{
			name:     "absolute path on windows should use absolute",
			base:     "C:\\temp",
			elem:     []string{"C:\\var\\lib\\data.db"},
			expected: "C:\\var\\lib\\data.db",
			osCheck:  "windows",
		},
		{
			name:     "multiple relative elements",
			base:     "/tmp",
			elem:     []string{"subdir", "data.db"},
			expected: "/tmp/subdir/data.db",
			osCheck:  "unix",
		},
		{
			name:     "empty elements",
			base:     "/tmp",
			elem:     []string{},
			expected: "/tmp",
			osCheck:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip OS-specific tests
			if tt.osCheck == "windows" && runtime.GOOS != "windows" {
				t.Skip("Windows-only test")
			}
			if tt.osCheck == "unix" && runtime.GOOS == "windows" {
				t.Skip("Unix-only test")
			}

			result := SafeJoin(tt.base, tt.elem...)
			assert.Equal(t, filepath.Clean(tt.expected), filepath.Clean(result))
		})
	}
}

func TestResolveDBPath(t *testing.T) {
	tests := []struct {
		name     string
		dataDir  string
		dbPath   string
		expected string
		osCheck  string
	}{
		{
			name:     "relative db path on unix",
			dataDir:  "/var/lib/g8e",
			dbPath:   "g8e.db",
			expected: "/var/lib/g8e/g8e.db",
			osCheck:  "unix",
		},
		{
			name:     "relative db path on windows",
			dataDir:  "C:\\ProgramData\\g8e",
			dbPath:   "g8e.db",
			expected: "C:\\ProgramData\\g8e\\g8e.db",
			osCheck:  "windows",
		},
		{
			name:     "absolute db path on unix",
			dataDir:  "/var/lib/g8e",
			dbPath:   "/opt/g8e/g8e.db",
			expected: "/opt/g8e/g8e.db",
			osCheck:  "unix",
		},
		{
			name:     "absolute db path on windows",
			dataDir:  "C:\\ProgramData\\g8e",
			dbPath:   "D:\\databases\\g8e.db",
			expected: "D:\\databases\\g8e.db",
			osCheck:  "windows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.osCheck == "windows" && runtime.GOOS != "windows" {
				t.Skip("Windows-only test")
			}
			if tt.osCheck == "unix" && runtime.GOOS == "windows" {
				t.Skip("Unix-only test")
			}

			result := ResolveDBPath(tt.dataDir, tt.dbPath)
			assert.Equal(t, filepath.Clean(tt.expected), filepath.Clean(result))
		})
	}
}

func TestToSlash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Run("windows conversions", func(t *testing.T) {
			path := "C:\\temp\\data"
			slashed := ToSlash(path)
			assert.Equal(t, "C:/temp/data", slashed)
		})
	}
}

// Made with Bob
