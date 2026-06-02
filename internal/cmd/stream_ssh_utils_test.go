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
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/pkg/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sshlib "golang.org/x/crypto/ssh"
)

func TestBuildHostKeyCallback(t *testing.T) {
	t.Run("known_hosts exists and is valid", func(t *testing.T) {
		tempDir := t.TempDir()
		// Mock home directory by setting HOME env var (os.UserHomeDir respects it on Linux)
		t.Setenv("HOME", tempDir)
		// No env var to unset - ~/.ssh/known_hosts is the only path

		sshDir := filepath.Join(tempDir, ".ssh")
		require.NoError(t, os.MkdirAll(sshDir, 0700))
		khPath := filepath.Join(sshDir, "known_hosts")

		// Empty known_hosts is valid - knownhosts.New handles it. Any host attempt
		// against the returned callback will fail with an unknown-host error,
		// which is exactly the strict semantic we want.
		require.NoError(t, os.WriteFile(khPath, []byte(""), 0600))

		cb, err := ssh.BuildHostKeyCallback()
		require.NoError(t, err)
		assert.NotNil(t, cb)
	})

	t.Run("known_hosts missing, strict mode returns an error (no insecure fallback)", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("HOME", tempDir)
		// No env var to unset - ~/.ssh/known_hosts is the only path

		cb, err := ssh.BuildHostKeyCallback()
		assert.Nil(t, cb)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "known_hosts not found")
	})

	t.Run("G8E_KNOWN_HOSTS env var overrides home lookup", func(t *testing.T) {
		// G8E_KNOWN_HOSTS env var was removed - known_hosts path is now
		// resolved solely from ~/.ssh/known_hosts.
		// This test is removed as the feature no longer exists.
		t.Skip("G8E_KNOWN_HOSTS env var removed")
	})

	t.Run("strict callback rejects unknown host", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("HOME", tempDir)
		// No env var to unset - ~/.ssh/known_hosts is the only path
		sshDir := filepath.Join(tempDir, ".ssh")
		require.NoError(t, os.MkdirAll(sshDir, 0700))
		khPath := filepath.Join(sshDir, "known_hosts")
		require.NoError(t, os.WriteFile(khPath, []byte(""), 0600))

		cb, err := ssh.BuildHostKeyCallback()
		require.NoError(t, err)

		// Build a real RSA public key so the callback actually invokes its
		// host-key matching logic instead of bailing on a nil key.
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		pub, err := sshlib.NewPublicKey(&priv.PublicKey)
		require.NoError(t, err)

		err = cb("localhost:22", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}, pub)
		require.Error(t, err, "strict callback must reject hosts not in known_hosts")
	})
}

func TestIsSSHExitError(t *testing.T) {
	t.Run("is an exit error", func(t *testing.T) {
		mockErr := &sshlib.ExitError{}
		var target *sshlib.ExitError
		result := isSSHExitError(mockErr, &target)
		assert.True(t, result)
		assert.Equal(t, mockErr, target)
	})

	t.Run("is not an exit error", func(t *testing.T) {
		mockErr := fmt.Errorf("generic error")
		var target *sshlib.ExitError
		result := isSSHExitError(mockErr, &target)
		assert.False(t, result)
		assert.Nil(t, target)
	})
}

// remove unused mockWaitMsg and its methods
