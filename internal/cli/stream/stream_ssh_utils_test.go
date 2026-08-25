// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package stream

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/pkg/ssh"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sshlib "golang.org/x/crypto/ssh"
)

func TestBuildHostKeyCallback(t *testing.T) {
	t.Run("known_hosts exists and is valid", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		khPath := filepath.Join(tempDir, "known_hosts")

		// Empty known_hosts is valid - knownhosts.New handles it. Any host attempt
		// against the returned callback will fail with an unknown-host error,
		// which is exactly the strict semantic we want.
		require.NoError(t, os.WriteFile(khPath, []byte(""), 0600))

		cb, err := ssh.BuildHostKeyCallback(khPath)
		require.NoError(t, err)
		assert.NotNil(t, cb)
	})

	t.Run("known_hosts missing, strict mode returns an error (no insecure fallback)", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		khPath := filepath.Join(tempDir, "known_hosts")

		cb, err := ssh.BuildHostKeyCallback(khPath)
		require.Error(t, err)
		assert.Nil(t, cb)
		assert.Error(t, err)
	})

	t.Run("G8E_KNOWN_HOSTS env var overrides home lookup", func(t *testing.T) {
		// G8E_KNOWN_HOSTS env var was removed - known_hosts path is now
		// resolved solely from ~/.ssh/known_hosts.
		// This test is removed as the feature no longer exists.
		t.Skip("G8E_KNOWN_HOSTS env var removed")
	})

	t.Run("strict callback rejects unknown host", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		khPath := filepath.Join(tempDir, "known_hosts")
		require.NoError(t, os.WriteFile(khPath, []byte(""), 0600))

		cb, err := ssh.BuildHostKeyCallback(khPath)
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

func TestSSHExitErrorDetection(t *testing.T) {
	t.Run("is an exit error", func(t *testing.T) {
		mockErr := &sshlib.ExitError{}
		var target *sshlib.ExitError
		result := errors.As(mockErr, &target)
		assert.True(t, result)
		assert.Equal(t, mockErr, target)
	})

	t.Run("is not an exit error", func(t *testing.T) {
		mockErr := fmt.Errorf("generic error")
		var target *sshlib.ExitError
		result := errors.As(mockErr, &target)
		assert.False(t, result)
		assert.Nil(t, target)
	})
}

// remove unused mockWaitMsg and its methods
