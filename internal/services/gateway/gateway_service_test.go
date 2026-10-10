// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore/keystoretest"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// testGatewayOpts configures optional parameters for newTestGatewayService.
type testGatewayOpts struct {
	posture  config.GatewayPosture
	httpPort int
}

// newTestGatewayService creates a GatewayModeService via NewGatewayModeService
// with test-appropriate configuration. Cleanup is registered via t.Cleanup.
func newTestGatewayService(t *testing.T, opts testGatewayOpts) *GatewayModeService {
	t.Helper()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	fileSvc := newTestFileSvc(t)

	cfg.Gateway.DataDir = testutil.TempDir(t)
	cfg.Gateway.PKIDir = testutil.TempDir(t)
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.VaultDir = fileSvc.Resolve(constants.VaultDirname)
	cfg.Gateway.HTTPPort = opts.httpPort
	cfg.Gateway.Posture = opts.posture

	db, err := openTestDB(t, fileSvc, logger)
	require.NoError(t, err)

	ls, err := NewGatewayModeServiceWithDB(cfg, fileSvc, logger, db, nil, nil)
	require.NoError(t, err)
	// Pub/sub callbacks run without a request context and use serviceCtx.
	ls.serviceCtx = context.Background()
	t.Cleanup(func() { ls.Stop(context.Background()) })
	return ls
}

func TestNewGatewayModeService(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	assert.NotNil(t, ls)
	assert.NotNil(t, ls.server)
	assert.NotNil(t, ls.pki)
	assert.False(t, ls.running)
}

func TestGatewayModeService_StateManagement(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	t.Run("Initial state", func(t *testing.T) {
		assert.False(t, ls.running)
		assert.False(t, ls.IsReady())
	})

	t.Run("IsReady returns false when not ready", func(t *testing.T) {
		assert.False(t, ls.IsReady())
	})
}

func TestDetectBasicNonLoopbackIPv4Addresses(t *testing.T) {
	ips := detectBasicNonLoopbackIPv4Addresses()
	assert.NotNil(t, ips)
}

func TestGateway_NoPlaintextOnDiskAfterBoot(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	fileSvc := newTestFileSvc(t)

	cfg.Gateway.DataDir = testutil.TempDir(t)
	cfg.Gateway.PKIDir = testutil.TempDir(t)
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.VaultDir = fileSvc.Resolve(constants.VaultDirname)
	cfg.Gateway.HTTPPort = 0
	cfg.Gateway.Posture = config.PostureDoctrine

	ks, err := keystore.NewWithKeyringAndFS(logger, keystoretest.NewMemoryKeyring(), fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())

	db, err := OpenCanonicalDBService(logger, ks, fileSvc)
	require.NoError(t, err)

	ls, err := NewGatewayModeServiceWithDB(cfg, fileSvc, logger, db, nil, nil)
	require.NoError(t, err)
	ls.serviceCtx = context.Background()
	t.Cleanup(func() { ls.Stop(context.Background()) })

	sm, err := ls.GetSecretManager()
	require.NoError(t, err)
	require.NotNil(t, sm)

	actuatorPriv, _, err := sm.GetActuatorKey()
	require.NoError(t, err)
	actuatorSeed := actuatorPriv.Seed()

	auditorPriv, _, err := sm.GetAuditorKey()
	require.NoError(t, err)
	auditorSeed := auditorPriv.Seed()

	sessionKeyHex, err := sm.GetSessionEncryptionKey()
	require.NoError(t, err)
	require.NotEmpty(t, sessionKeyHex)
	sessionKeyBytes, err := hex.DecodeString(sessionKeyHex)
	require.NoError(t, err)

	vaultKey, err := ks.LoadVaultKey()
	require.NoError(t, err)
	defer vault.SecureZero(vaultKey)

	type keyCheck struct {
		name string
		raw  []byte
	}
	keys := []keyCheck{
		{"vault key", vaultKey},
		{"actuator seed", actuatorSeed},
		{"auditor seed", auditorSeed},
		{"session encryption key", sessionKeyBytes},
	}

	var forbiddenRaw [][]byte
	var forbiddenStrings []string

	for _, k := range keys {
		require.NotEmpty(t, k.raw, "key %s must not be empty", k.name)
		forbiddenRaw = append(forbiddenRaw, k.raw)
		forbiddenStrings = append(forbiddenStrings,
			hex.EncodeToString(k.raw),
			base64.StdEncoding.EncodeToString(k.raw),
			base64.RawStdEncoding.EncodeToString(k.raw),
			base64.URLEncoding.EncodeToString(k.raw),
			base64.RawURLEncoding.EncodeToString(k.raw),
		)
	}
	forbiddenStrings = append(forbiddenStrings, sessionKeyHex)

	dirsToScan := []string{constants.SecretsDirname, constants.VaultDirname}
	scannedFiles := 0

	var walkDir func(relDir string)
	walkDir = func(relDir string) {
		entries, err := fileSvc.ReadDir(context.Background(), relDir)
		require.NoError(t, err)
		for _, entry := range entries {
			entryRel := filepath.Join(relDir, entry.Name())
			if entry.IsDir() {
				walkDir(entryRel)
				continue
			}
			data, err := fileSvc.ReadFile(context.Background(), entryRel)
			require.NoError(t, err)
			scannedFiles++

			for _, raw := range forbiddenRaw {
				assert.False(t, bytes.Contains(data, raw), "file %s contains raw key material", entryRel)
			}
			contentStr := string(data)
			for _, s := range forbiddenStrings {
				assert.False(t, strings.Contains(contentStr, s), "file %s contains encoded key material %q", entryRel, s)
			}
		}
	}

	for _, d := range dirsToScan {
		walkDir(d)
	}

	assert.GreaterOrEqual(t, scannedFiles, 4, "expected to scan secrets and vault files")
}
