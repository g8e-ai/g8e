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

package cmd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestBuildHostKeyCallback(t *testing.T) {
	t.Run("known_hosts exists and is valid", func(t *testing.T) {
		tempDir := t.TempDir()
		// Mock home directory by setting HOME env var (os.UserHomeDir respects it on Linux)
		oldHome := os.Getenv("HOME")
		os.Setenv("HOME", tempDir)
		defer os.Setenv("HOME", oldHome)

		sshDir := filepath.Join(tempDir, ".ssh")
		require.NoError(t, os.MkdirAll(sshDir, 0700))
		khPath := filepath.Join(sshDir, "known_hosts")

		// Use a dummy valid known_hosts content or empty (knownhosts.New handles empty)
		require.NoError(t, os.WriteFile(khPath, []byte(""), 0600))

		cb := buildHostKeyCallback("")
		assert.NotNil(t, cb)
	})

	t.Run("known_hosts does not exist, returns insecure callback", func(t *testing.T) {
		tempDir := t.TempDir()
		oldHome := os.Getenv("HOME")
		os.Setenv("HOME", tempDir)
		defer os.Setenv("HOME", oldHome)

		cb := buildHostKeyCallback("")
		assert.NotNil(t, cb)

		// Verify it returns nil (accepts)
		err := cb("localhost:22", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}, nil)
		assert.NoError(t, err)
	})
}

func TestIsSSHExitError(t *testing.T) {
	t.Run("is an exit error", func(t *testing.T) {
		mockErr := &ssh.ExitError{}
		var target *ssh.ExitError
		result := isSSHExitError(mockErr, &target)
		assert.True(t, result)
		assert.Equal(t, mockErr, target)
	})

	t.Run("is not an exit error", func(t *testing.T) {
		mockErr := fmt.Errorf("generic error")
		var target *ssh.ExitError
		result := isSSHExitError(mockErr, &target)
		assert.False(t, result)
		assert.Nil(t, target)
	})
}

// remove unused mockWaitMsg and its methods
