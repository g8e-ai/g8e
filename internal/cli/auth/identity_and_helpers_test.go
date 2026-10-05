// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// ---------------------------------------------------------------------------
// LoadClientAuthContext
// ---------------------------------------------------------------------------

func TestLoadClientAuthContext_ReturnsIdentityOnlyWhenCredentialsAndKeyMaterialAreComplete(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	userID, cliSessionID := writeCompleteIdentity(t, fileSvc, cfg)
	require.NoError(t, SaveCredentials(fileSvc, cfg, &Credentials{
		UserID: userID, CLISessionID: cliSessionID, OperatorSessionID: "op-sess", OperatorID: "op-1",
	}))

	got, err := LoadClientAuthContext(fileSvc, cfg)

	require.NoError(t, err)
	assert.Equal(t, &ClientAuthContext{
		OperatorSessionID: "op-sess", CLISessionID: cliSessionID, UserID: userID, OperatorID: "op-1",
		ClientCert: cfg.CLICertFile(), ClientKey: cfg.CLIKeyFile(),
	}, got)
}

func TestLoadClientAuthContext_FailsClosedWhenIdentityIsAbsentOrIncomplete(t *testing.T) {
	t.Run("no credentials on disk", func(t *testing.T) {
		fileSvc, cfg := newAuthTestEnv(t)

		got, err := LoadClientAuthContext(fileSvc, cfg)

		require.ErrorIs(t, err, constants.ErrNotAuthenticated)
		assert.Contains(t, err.Error(), "credentials are absent")
		assert.Nil(t, got)
	})

	t.Run("credentials without a CLI session", func(t *testing.T) {
		fileSvc, cfg := newAuthTestEnv(t)
		require.NoError(t, SaveCredentials(fileSvc, cfg, &Credentials{UserID: "u"}))

		_, err := LoadClientAuthContext(fileSvc, cfg)

		require.ErrorIs(t, err, constants.ErrNotAuthenticated)
		assert.Contains(t, err.Error(), "cli_session_id")
		assert.NotContains(t, err.Error(), "user_id", "only the missing fields are named")
	})

	t.Run("credentials without a user", func(t *testing.T) {
		fileSvc, cfg := newAuthTestEnv(t)
		require.NoError(t, SaveCredentials(fileSvc, cfg, &Credentials{CLISessionID: "c"}))

		_, err := LoadClientAuthContext(fileSvc, cfg)

		require.ErrorIs(t, err, constants.ErrNotAuthenticated)
		assert.Contains(t, err.Error(), "user_id")
	})

	t.Run("credentials missing both identifiers names both", func(t *testing.T) {
		fileSvc, cfg := newAuthTestEnv(t)
		require.NoError(t, SaveCredentials(fileSvc, cfg, &Credentials{OperatorID: "op"}))

		_, err := LoadClientAuthContext(fileSvc, cfg)

		require.ErrorIs(t, err, constants.ErrNotAuthenticated)
		assert.Contains(t, err.Error(), "cli_session_id, user_id")
	})

	for name, path := range map[string]func(cfg *config.Config) string{
		"CLI certificate missing": func(cfg *config.Config) string { return cfg.CLICertFile() },
		"CLI key missing":         func(cfg *config.Config) string { return cfg.CLIKeyFile() },
	} {
		t.Run(name, func(t *testing.T) {
			fileSvc, cfg := newAuthTestEnv(t)
			writeCompleteIdentity(t, fileSvc, cfg)
			rel, err := fileSvc.RelFromAbs(path(cfg))
			require.NoError(t, err)
			require.NoError(t, fileSvc.Remove(t.Context(), rel))

			got, err := LoadClientAuthContext(fileSvc, cfg)

			require.ErrorIs(t, err, constants.ErrNotAuthenticated)
			assert.Contains(t, err.Error(), "incomplete local CLI identity")
			assert.Nil(t, got)
		})
	}

	t.Run("corrupt credentials file", func(t *testing.T) {
		fileSvc, cfg := newAuthTestEnv(t)
		rel, err := fileSvc.RelFromAbs(cfg.CredentialsFile())
		require.NoError(t, err)
		require.NoError(t, fileSvc.MkdirAll(t.Context(), filepath.Dir(rel), constants.PermDirPrivate))
		require.NoError(t, fileSvc.WriteFile(t.Context(), rel, []byte("{not json"), constants.PermFilePrivate))

		_, err = LoadClientAuthContext(fileSvc, cfg)

		require.ErrorIs(t, err, constants.ErrInvalidJSONBody)
	})
}

// ---------------------------------------------------------------------------
// CheckOperatorRunning
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// CredentialStore path resolution
// ---------------------------------------------------------------------------

func TestCredentialStore_ResolvedPathsMatchTheConfiguredLocations(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	store := NewCredentialStore(fileSvc, cfg)

	assert.Equal(t, cfg.CLICertFile(), store.ResolveCLICertPath())
	assert.Equal(t, cfg.CLIKeyFile(), store.ResolveCLIKeyPath())
	assert.Equal(t, cfg.ResolvedTrustBundlePath(), store.ResolveTrustBundlePath())
	assert.NotEqual(t, store.ResolveCLICertPath(), store.ResolveCLIKeyPath(), "cert and key live in different files")
}

// ---------------------------------------------------------------------------
// LocalIdentity.NeedsRecovery / NeedsRotation
// ---------------------------------------------------------------------------

func TestLocalIdentity_RecoveryAndRotationDecisions(t *testing.T) {
	tests := []struct {
		name         string
		identity     LocalIdentity
		wantRecovery bool
		wantRotation bool
	}{
		{"absent identity bootstraps rather than recovers", LocalIdentity{State: LocalStateAbsent}, false, false},
		{"partial identity must recover", LocalIdentity{State: LocalStatePartial}, true, false},
		{"corrupt identity must recover", LocalIdentity{State: LocalStateCorrupt}, true, false},
		{"healthy complete identity needs nothing", LocalIdentity{State: LocalStateComplete}, false, false},
		{"expiring complete identity rotates", LocalIdentity{State: LocalStateComplete, CertExpiring: true}, false, true},
		{"expired complete identity recovers because it cannot authenticate", LocalIdentity{State: LocalStateComplete, CertExpiring: true, CertExpired: true}, true, false},
		{"unknown state is not recoverable by this path", LocalIdentity{State: LocalEnrollmentState(99)}, false, false},
		{"expiry flags on a partial identity do not trigger rotation", LocalIdentity{State: LocalStatePartial, CertExpiring: true}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantRecovery, tt.identity.NeedsRecovery(), "NeedsRecovery")
			assert.Equal(t, tt.wantRotation, tt.identity.NeedsRotation(), "NeedsRotation")
		})
	}
}

// ---------------------------------------------------------------------------
// TrustBundle pinning and disk helpers
// ---------------------------------------------------------------------------

func TestTrustBundle_FingerprintPinning(t *testing.T) {
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	bundle, err := ParseTrustBundle([]byte(artifacts.TrustBundlePEM), time.Now())
	require.NoError(t, err)
	rootFP := bundle.PrimaryRootFingerprint
	require.NotEmpty(t, rootFP)
	// Flip the first hex digit so the value is guaranteed to differ from the real fingerprint.
	otherFP := "0" + rootFP[1:]
	if rootFP[0] == '0' {
		otherFP = "1" + rootFP[1:]
	}

	t.Run("ContainsFingerprint matches any certificate in the bundle", func(t *testing.T) {
		assert.True(t, bundle.ContainsFingerprint(rootFP))
		assert.False(t, bundle.ContainsFingerprint(otherFP), "a different fingerprint must not match")
		assert.False(t, bundle.ContainsFingerprint(""), "an empty fingerprint never matches")
		var nilBundle *TrustBundle
		assert.False(t, nilBundle.ContainsFingerprint(rootFP), "a nil bundle contains nothing")
	})

	t.Run("VerifyFingerprintPin", func(t *testing.T) {
		require.NoError(t, bundle.VerifyFingerprintPin(""), "no pin configured is a no-op")
		require.NoError(t, bundle.VerifyFingerprintPin(rootFP))
		require.ErrorIs(t, bundle.VerifyFingerprintPin(otherFP), constants.ErrValidationFailed)
		var nilBundle *TrustBundle
		require.NoError(t, nilBundle.VerifyFingerprintPin(""), "no pin means nothing to verify even for a nil bundle")
		require.ErrorIs(t, nilBundle.VerifyFingerprintPin(rootFP), constants.ErrValidationFailed, "a pin can never be satisfied by a missing bundle")
	})
}

func TestTrustBundleDiskHelpers_WriteAndRemove(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	bundlePEM := []byte(buildTestArtifacts(t, EnrollmentSourceBootstrap).TrustBundlePEM)

	t.Run("an empty bundle is never written", func(t *testing.T) {
		require.ErrorIs(t, WriteTrustBundleToDisk(t.Context(), fileSvc, cfg, nil), constants.ErrEmptyTrustBundle)
		parsed, err := ReadTrustBundleFromDisk(t.Context(), fileSvc, cfg)
		require.NoError(t, err)
		assert.Nil(t, parsed)
	})

	t.Run("a written bundle reads back and is removed idempotently", func(t *testing.T) {
		require.NoError(t, WriteTrustBundleToDisk(t.Context(), fileSvc, cfg, bundlePEM))
		parsed, err := ReadTrustBundleFromDisk(t.Context(), fileSvc, cfg)
		require.NoError(t, err)
		require.NotNil(t, parsed)
		assert.Equal(t, bundlePEM, parsed.PEM)

		require.NoError(t, RemoveTrustBundleFromDisk(t.Context(), fileSvc, cfg))
		parsed, err = ReadTrustBundleFromDisk(t.Context(), fileSvc, cfg)
		require.NoError(t, err)
		assert.Nil(t, parsed, "after removal the bundle reads as absent")

		require.NoError(t, RemoveTrustBundleFromDisk(t.Context(), fileSvc, cfg), "removing an absent bundle is a no-op")
	})
}

// ---------------------------------------------------------------------------
// FileKeyProvider
// ---------------------------------------------------------------------------

func TestFileKeyProvider_GeneratesP256KeyAndMatchingSignedCSR(t *testing.T) {
	csrPEM, key, err := FileKeyProvider{}.GenerateCLIKeyAndCSR(t.Context(), "g8e-cli-test-user")

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, elliptic.P256(), key.Curve)
	block, _ := pem.Decode([]byte(csrPEM))
	require.NotNil(t, block)
	assert.Equal(t, "CERTIFICATE REQUEST", block.Type)
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	require.NoError(t, csr.CheckSignature(), "the CSR must be self-signed by the generated key")
	assert.Equal(t, "g8e-cli-test-user", csr.Subject.CommonName)
	pub, ok := csr.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok)
	assert.True(t, key.PublicKey.Equal(pub), "the CSR commits to the returned key")

	_, key2, err := FileKeyProvider{}.GenerateCLIKeyAndCSR(t.Context(), "g8e-cli-test-user")
	require.NoError(t, err)
	assert.False(t, key.Equal(key2), "every call generates a fresh key")
}

// ---------------------------------------------------------------------------
// App enrollment helpers
// ---------------------------------------------------------------------------

func TestAppEnrollment_ParseECPrivateKeyPEM(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	t.Run("round-trips SEC1 and PKCS8 encodings", func(t *testing.T) {
		sec1, err := encodeECPrivateKeyPEM(key)
		require.NoError(t, err)
		parsed, err := parseECPrivateKeyPEM(sec1)
		require.NoError(t, err)
		assert.True(t, key.Equal(parsed))

		der, err := x509.MarshalPKCS8PrivateKey(key)
		require.NoError(t, err)
		parsed, err = parseECPrivateKeyPEM(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
		require.NoError(t, err)
		assert.True(t, key.Equal(parsed))
	})

	t.Run("rejects anything else", func(t *testing.T) {
		rsaLike := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("x")}))
		cases := map[string]string{
			"no PEM":              "garbage",
			"unexpected PEM type": rsaLike,
			"PKCS8 with bad body": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("junk")})),
			"SEC1 with bad body":  string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("junk")})),
		}
		for name, in := range cases {
			got, err := parseECPrivateKeyPEM(in)
			require.Errorf(t, err, "%s must be rejected", name)
			assert.Nil(t, got)
		}
	})
}

func TestAppEnrollment_ParseRetryAfter(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", 0}, {"2", 2 * time.Second}, {"45", 45 * time.Second}, {"0", 0}, {"-1", 0}, {"later", 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, parseRetryAfter(tt.in))
		})
	}
}

func TestAppEnrollment_ResolveGatewayHTTPURL(t *testing.T) {
	defaultURL := fmt.Sprintf("http://127.0.0.1:%d", constants.Ports.OperatorHttp)
	tests := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{"nil config uses the loopback default", nil, defaultURL},
		{"config without paths uses the loopback default", &config.Config{}, defaultURL},
		{"empty host uses the loopback default", &config.Config{Paths: &config.PathsConfig{}}, defaultURL},
		{"full URL is used verbatim minus trailing slashes", &config.Config{Paths: &config.PathsConfig{Host: "http://gw.example:9000//"}}, "http://gw.example:9000"},
		{"host:port gains the http scheme", &config.Config{Paths: &config.PathsConfig{Host: "gw.example:9000"}}, "http://gw.example:9000"},
		{"bare host gains the default discovery port", &config.Config{Paths: &config.PathsConfig{Host: "gw.example"}}, fmt.Sprintf("http://gw.example:%d", constants.Ports.OperatorHttp)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveGatewayHTTPURL(tt.cfg))
		})
	}
}

func TestAppEnrollment_PendingStateLoading(t *testing.T) {
	newClient := func(t *testing.T) *AppPlatformEnrollmentClient {
		t.Helper()
		fileSvc, cfg := newAuthTestEnv(t)
		client, err := NewAppPlatformEnrollmentClient("pending-app", fileSvc, cfg, testutil.NewTestLogger(), AppEnrollmentOptions{GatewayHTTPURL: "http://gw:8080"})
		require.NoError(t, err)
		return client
	}

	t.Run("absent state loads as nothing to resume", func(t *testing.T) {
		client := newClient(t)

		state, err := client.loadPendingState(client.pendingStatePath())

		require.NoError(t, err)
		assert.Nil(t, state)
	})

	t.Run("persisted state round-trips with private permissions", func(t *testing.T) {
		client := newClient(t)
		want := &appPendingState{
			RequestID: "req-1", Token: "tok", AppFingerprint: "fp", KeyPEM: "KEY",
			ExpiresAt: time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			AppName:   "pending-app", InstanceID: "inst", Hostname: "host",
		}
		path := client.pendingStatePath()
		require.NoError(t, client.fileSvc.MkdirAll(t.Context(), filepath.Dir(path), constants.PermDirPrivate))
		require.NoError(t, client.persistPendingState(path, want))

		got, err := client.loadPendingState(path)

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.True(t, want.ExpiresAt.Equal(got.ExpiresAt))
		got.ExpiresAt = want.ExpiresAt
		assert.Equal(t, want, got)
		info, err := client.fileSvc.Stat(t.Context(), path)
		require.NoError(t, err)
		assert.Equal(t, constants.PermFilePrivate, int(info.Mode().Perm()), "the pending state holds a private key and requester token")

		require.NoError(t, client.removePendingState(path))
		gone, err := client.loadPendingState(path)
		require.NoError(t, err)
		assert.Nil(t, gone)
	})

	t.Run("corrupt state is an error, not an empty resume", func(t *testing.T) {
		client := newClient(t)
		path := client.pendingStatePath()
		require.NoError(t, client.fileSvc.MkdirAll(t.Context(), filepath.Dir(path), constants.PermDirPrivate))
		require.NoError(t, client.fileSvc.WriteFile(t.Context(), path, []byte("{not json"), constants.PermFilePrivate))

		state, err := client.loadPendingState(path)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse pending state")
		assert.Nil(t, state)
	})
}
