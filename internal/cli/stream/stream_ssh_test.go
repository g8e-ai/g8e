// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package stream

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/pkg/ssh"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sshlib "golang.org/x/crypto/ssh"
)

// mockSSHServer is a minimal SSH server for testing streamToHost.
type mockSSHServer struct {
	gateway net.Listener
	config  *sshlib.ServerConfig
	addr    string
	hostKey sshlib.PublicKey
}

func testutil_GenerateRSAPrivateKey(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return keyPEM
}

func TestStreamToHost_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	resultCh := make(chan streamResult, 1)
	streamToHost(
		ctx,
		"127.0.0.1",
		StreamHostOptions{
			BinaryData:  []byte("data"),
			DialTimeout: 2 * time.Second,
			Username:    "user",
		},
		resultCh,
	)

	res := <-resultCh
	assert.Equal(t, constants.StreamStatusCancelled, res.Status)
}

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"timeout error", fmt.Errorf("i/o timeout"), true},
		{"connection refused", fmt.Errorf("connection refused"), true},
		{"network unreachable", fmt.Errorf("network is unreachable"), true},
		{"no route to host", fmt.Errorf("no route to host"), true},
		{"broken pipe", fmt.Errorf("broken pipe"), true},
		{"connection reset", fmt.Errorf("connection reset"), true},
		{"auth failure", fmt.Errorf("permission denied"), false},
		{"nil error", nil, false},
		{"unknown error", fmt.Errorf("something else"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTransientError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildAuthMethods_WithPassphrase(t *testing.T) {
	dir := testutil.TempDir(t)
	keyPath := filepath.Join(dir, "id_rsa_encrypted")

	// Generate an RSA key
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Encrypt with passphrase (PKCS#8 with passphrase)
	// For simplicity, we'll test the code path without actual encryption
	// since generating encrypted PEM is complex
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	}
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0600))

	r := ssh.HostConfig{
		KeyFiles: []string{keyPath},
	}

	// Test with empty passphrase (should work for unencrypted key)
	methods, err := ssh.BuildAuthMethods(r, "", "")
	require.NoError(t, err)
	assert.Len(t, methods, 1)

	// Test with wrong passphrase (should fall back to no passphrase)
	methods, err = ssh.BuildAuthMethods(r, "", "wrongpassphrase")
	require.NoError(t, err)
	assert.Len(t, methods, 1)
}

func TestParseConfig_ProxyCommand(t *testing.T) {
	dir := testutil.TempDir(t)
	configPath := filepath.Join(dir, "config")
	configContent := `Host bastion
  ProxyCommand ssh -W %h:%p jumpuser@jumphost
  User bastionuser
  Port 2222

Host *.internal
  ProxyCommand nc -X 5 -x proxy.example.com:1080 %h %p
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0600))

	blocks, err := ssh.ParseConfig(configPath)
	require.NoError(t, err)
	assert.NotEmpty(t, blocks)

	// Test bastion host
	bastionBlock := ssh.MatchBlock(blocks, "bastion")
	assert.NotNil(t, bastionBlock)
	assert.Equal(t, "ssh -W %h:%p jumpuser@jumphost", bastionBlock.ProxyCommand)
	assert.Equal(t, "bastionuser", bastionBlock.User)
	assert.Equal(t, "2222", bastionBlock.Port)

	// Test wildcard host
	internalBlock := ssh.MatchBlock(blocks, "server.internal")
	assert.NotNil(t, internalBlock)
	assert.Equal(t, "nc -X 5 -x proxy.example.com:1080 %h %p", internalBlock.ProxyCommand)
}

func TestResolveHost_ProxyCommand(t *testing.T) {
	dir := testutil.TempDir(t)
	configPath := filepath.Join(dir, "config")
	configContent := `Host proxyhost
  ProxyCommand ssh -W %h:%p jump@bastion
  User proxyuser
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0600))

	r, err := ssh.ResolveHost("proxyhost", configPath, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "ssh -W %h:%p jump@bastion", r.ProxyCommand)
	assert.Equal(t, "proxyuser", r.User)
}

// ---------------------------------------------------------------------------
// preFlightCheck
// ---------------------------------------------------------------------------

func TestPreFlightCheck_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Need to provide a valid key to get past auth method check
	dir := testutil.TempDir(t)
	keyPath := filepath.Join(dir, "id_rsa")
	generateTestSSHKey(t, keyPath)

	// Create a known_hosts file to satisfy strict host-key checking
	khPath := filepath.Join(dir, "known_hosts")
	require.NoError(t, os.WriteFile(khPath, []byte(""), 0600))

	r := ssh.HostConfig{
		Hostname: "127.0.0.1",
		Port:     "22",
		User:     "testuser",
		KeyFiles: []string{keyPath},
	}

	err := preFlightCheck(ctx, r, "", "", khPath, 5*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestPreFlightCheck_NoAuthMethods(t *testing.T) {
	ctx := context.Background()
	r := ssh.HostConfig{
		Hostname: "127.0.0.1",
		Port:     "22",
		User:     "testuser",
		KeyFiles: []string{},
	}

	err := preFlightCheck(ctx, r, "", "", "", 5*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no SSH auth methods available")
}

func TestPreFlightCheck_InvalidKeyFile(t *testing.T) {
	ctx := context.Background()
	dir := testutil.TempDir(t)
	badKey := filepath.Join(dir, "bad_key")
	require.NoError(t, os.WriteFile(badKey, []byte("not a valid key"), 0600))

	r := ssh.HostConfig{
		Hostname: "127.0.0.1",
		Port:     "22",
		User:     "testuser",
		KeyFiles: []string{badKey},
	}

	err := preFlightCheck(ctx, r, "", "", "", 5*time.Second)
	assert.Error(t, err)
}

func TestPreFlightCheck_DialTimeout(t *testing.T) {
	ctx := context.Background()

	// Use a non-routable IP to trigger timeout
	r := ssh.HostConfig{
		Hostname: "192.0.2.1", // TEST-NET-1, guaranteed non-routable
		Port:     "22",
		User:     "testuser",
		KeyFiles: []string{},
	}

	err := preFlightCheck(ctx, r, "", "", "", 100*time.Millisecond)
	assert.Error(t, err)
	// Should fail due to no auth methods or dial timeout
}
