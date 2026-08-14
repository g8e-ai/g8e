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

package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock implementations ---

// mockGateway records every call and returns configurable responses.
type mockGateway struct {
	mu                       sync.Mutex
	bootstrapStatus          bool
	bootstrapStatusErr       error
	bootstrapArtifacts       EnrollmentArtifacts
	bootstrapErr             error
	bootstrapCalls           int
	recoveryRequestID        string
	recoveryToken            string
	recoveryApprovalURL      string
	recoveryRequestErr       error
	recoveryRequestCalls     int
	recoveryStates           []models.CLIRecoveryState
	recoveryStateIdx         int
	recoveryStateErr         error
	recoveryCompleteArtifact EnrollmentArtifacts
	recoveryCompleteErr      error
	recoveryCompleteCalls    int
	rotateArtifact           EnrollmentArtifacts
	rotateErr                error
	rotateCalls              int

	// DiscoverGatewayCA mock fields.
	discoveryBundlePEM   []byte
	discoveryFingerprint string
	discoveryErr         error
	discoveryCalls       int
}

func (m *mockGateway) CheckBootstrapStatus(ctx context.Context, baseURL string) (bool, error) {
	return m.bootstrapStatus, m.bootstrapStatusErr
}

func (m *mockGateway) Bootstrap(ctx context.Context, cliCSR string, cliKey *ecdsa.PrivateKey, operatorCSR, caFingerprint, baseURL string) (EnrollmentArtifacts, error) {
	m.mu.Lock()
	m.bootstrapCalls++
	m.mu.Unlock()
	return m.bootstrapArtifacts, m.bootstrapErr
}

func (m *mockGateway) CreateRecoveryRequest(ctx context.Context, cliCSR, baseURL string) (string, string, string, time.Time, error) {
	m.mu.Lock()
	m.recoveryRequestCalls++
	m.mu.Unlock()
	if m.recoveryRequestErr != nil {
		return "", "", "", time.Time{}, m.recoveryRequestErr
	}
	return m.recoveryRequestID, m.recoveryToken, m.recoveryApprovalURL, time.Now().Add(5 * time.Minute), nil
}

func (m *mockGateway) RecoveryStatus(ctx context.Context, token, baseURL string) (models.CLIRecoveryState, error) {
	if m.recoveryStateErr != nil {
		return "", m.recoveryStateErr
	}
	if m.recoveryStateIdx < len(m.recoveryStates) {
		s := m.recoveryStates[m.recoveryStateIdx]
		m.recoveryStateIdx++
		return s, nil
	}
	return models.CLIRecoveryStatePending, nil
}

func (m *mockGateway) CompleteRecovery(ctx context.Context, requestID, token string, cliCSR string, cliKey *ecdsa.PrivateKey, caFingerprint, baseURL string) (EnrollmentArtifacts, error) {
	m.mu.Lock()
	m.recoveryCompleteCalls++
	m.mu.Unlock()
	return m.recoveryCompleteArtifact, m.recoveryCompleteErr
}

func (m *mockGateway) Rotate(ctx context.Context, fileSvc fs.RuntimeFileService, cliCSR string, cliKey *ecdsa.PrivateKey, caFingerprint string) (EnrollmentArtifacts, error) {
	m.mu.Lock()
	m.rotateCalls++
	m.mu.Unlock()
	return m.rotateArtifact, m.rotateErr
}

// DiscoverGatewayCA returns the configured mock discovery response. By
// default (zero-value mockGateway), discoveryErr is nil and
// discoveryFingerprint is empty, which the coordinator treats as
// "discovery unreachable" (discoveryReachable = false). Tests that want
// to simulate a reachable gateway with a specific live fingerprint must
// set both discoveryBundlePEM and discoveryFingerprint.
func (m *mockGateway) DiscoverGatewayCA(ctx context.Context) ([]byte, string, error) {
	m.mu.Lock()
	m.discoveryCalls++
	m.mu.Unlock()
	if m.discoveryErr != nil {
		return nil, "", m.discoveryErr
	}
	return m.discoveryBundlePEM, m.discoveryFingerprint, nil
}

// mockKeyProvider returns a pre-generated CSR + key.
type mockKeyProvider struct {
	csr string
	key *ecdsa.PrivateKey
	err error
}

func (p *mockKeyProvider) GenerateCLIKeyAndCSR(ctx context.Context, commonName string) (string, *ecdsa.PrivateKey, error) {
	return p.csr, p.key, p.err
}

// mockTrustInstaller records calls and returns configurable results.
type mockTrustInstaller struct {
	mu                   sync.Mutex
	isTrusted            bool
	isTrustedErr         error
	isTrustedCalls       int
	lastTrustedFP        string
	installErr           error
	installCalls         int
	lastInstallRoot      *x509.Certificate
	lastInstallFP        string
	staleAnchors         []platform.StaleAnchor
	staleErr             error
	staleListCalls       int
	lastStaleFingerprint string
	removeErr            error
	staleRemoveCalls     int
}

func (m *mockTrustInstaller) IsTrusted(ctx context.Context, fingerprint string) (bool, error) {
	m.mu.Lock()
	m.isTrustedCalls++
	m.lastTrustedFP = fingerprint
	m.mu.Unlock()
	return m.isTrusted, m.isTrustedErr
}

func (m *mockTrustInstaller) InstallRoot(ctx context.Context, root *x509.Certificate, fingerprint string) error {
	m.mu.Lock()
	m.installCalls++
	m.lastInstallRoot = root
	m.lastInstallFP = fingerprint
	m.mu.Unlock()
	return m.installErr
}

func (m *mockTrustInstaller) ListStaleAnchors(ctx context.Context, currentFingerprint string) ([]platform.StaleAnchor, error) {
	m.mu.Lock()
	m.staleListCalls++
	m.lastStaleFingerprint = currentFingerprint
	m.mu.Unlock()
	return m.staleAnchors, m.staleErr
}

func (m *mockTrustInstaller) RemoveStaleAnchors(ctx context.Context, anchors []platform.StaleAnchor) error {
	m.mu.Lock()
	m.staleRemoveCalls++
	m.mu.Unlock()
	return m.removeErr
}

// mockBrowserOpener records calls and returns a configurable error.
type mockBrowserOpener struct {
	mu    sync.Mutex
	calls int
	urls  []string
	err   error
}

func (m *mockBrowserOpener) Open(url string) error {
	m.mu.Lock()
	m.calls++
	m.urls = append(m.urls, url)
	m.mu.Unlock()
	return m.err
}

// mockPasskeyRegistrar records calls and returns a configurable error.
type mockPasskeyRegistrar struct {
	mu            sync.Mutex
	calls         int
	lastUserID    string
	lastSessionID string
	err           error
}

func (m *mockPasskeyRegistrar) Register(ctx context.Context, userID, cliSessionID string) error {
	m.mu.Lock()
	m.calls++
	m.lastUserID = userID
	m.lastSessionID = cliSessionID
	m.mu.Unlock()
	return m.err
}

// mockConfirm records the prompt and returns a configurable bool.
type mockConfirm struct {
	mu         sync.Mutex
	calls      int
	lastPrompt string
	result     bool
}

func (m *mockConfirm) confirm(prompt string) bool {
	m.mu.Lock()
	m.calls++
	m.lastPrompt = prompt
	m.mu.Unlock()
	return m.result
}

// outputRecorder captures coordinator progress output for assertion.
type outputRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *outputRecorder) out(format string, args ...any) {
	r.mu.Lock()
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
	r.mu.Unlock()
}

func (r *outputRecorder) contains(substr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range r.lines {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

// --- Test fixture helpers ---

// buildTestArtifacts generates a valid EnrollmentArtifacts set: a root CA,
// a CLI cert signed by that CA with a known key, and a trust bundle
// containing the root CA + a leaf cert (so verifyRootUsable passes).
func buildTestArtifacts(t *testing.T, source EnrollmentSource) EnrollmentArtifacts {
	t.Helper()
	// Build the CA manually so we have the key for signing leaf certs.
	// Use caCert.Raw (the original DER) for the bundle PEM so the
	// fingerprint is stable — re-encoding via CreateCertificate would
	// produce a different DER (ECDSA signatures are non-deterministic).
	caKey, caCert := generateTestCAWithKey(t, "test-root-ca")
	caPEM := pemEncode("CERTIFICATE", caCert.Raw)

	// CLI cert signed by the root CA.
	cliCertPEM, cliKey := testutil.GenerateTestSignedCert(t, "g8e-cli-test", caCert, caKey)

	// Leaf cert for the bundle (so verifyRootUsable finds a chaining cert).
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "test-leaf", caCert, caKey)

	bundlePEM := caPEM + leafPEM

	return EnrollmentArtifacts{
		Source:         source,
		CLISessionID:   "cli-session-123",
		UserID:         "user-456",
		CLICertPEM:     cliCertPEM,
		CLIKey:         cliKey,
		TrustBundlePEM: bundlePEM,
	}
}

// generateTestCAWithKey generates a self-signed CA and returns both the key
// and the parsed cert. Unlike testutil.GenerateTestCA, this exposes the key
// so callers can sign leaf certs with it.
func generateTestCAWithKey(t *testing.T, cn string) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	return generateTestCAWithKeyAndExpiry(t, cn, time.Now().Add(365*24*time.Hour))
}

func generateTestCAWithKeyAndExpiry(t *testing.T, cn string, notAfter time.Time) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              notAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return key, cert
}

// writeCompleteIdentity writes a complete, valid CLI identity to disk so
// Inspect classifies it as LocalStateComplete. Returns the identity's
// session/user IDs for assertion.
func writeCompleteIdentity(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config) (userID, cliSessionID string) {
	t.Helper()
	return writeCompleteIdentityWithExpiry(t, fileSvc, cfg, time.Now().Add(365*24*time.Hour))
}

func writeCompleteIdentityWithExpiry(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config, certNotAfter time.Time) (userID, cliSessionID string) {
	t.Helper()
	caKey, caCert := generateTestCAWithKeyAndExpiry(t, "test-root-ca", time.Now().Add(365*24*time.Hour))
	cliCertPEM, cliKey := testutil.GenerateTestSignedCertWithExpiry(t, "g8e-cli-test", caCert, caKey, certNotAfter)
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "test-leaf", caCert, caKey)
	// Use caCert.Raw (the original DER) for the bundle PEM so the root
	// anchor matches the CA that signed the leaf — re-encoding via
	// GenerateTestCA would produce a different CA (non-deterministic
	// ECDSA signature) and verifyRootUsable would reject the bundle.
	bundlePEM := pemEncode("CERTIFICATE", caCert.Raw) + leafPEM

	// Write CLI cert + key.
	certRel, err := fileSvc.RelFromAbs(cfg.CLICertFile())
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, []byte(cliCertPEM), constants.PermFilePrivate))
	keyDER, err := x509.MarshalECPrivateKey(cliKey)
	require.NoError(t, err)
	keyPEM := pemEncode("EC PRIVATE KEY", keyDER)
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, []byte(keyPEM), constants.PermFilePrivate))

	// Write trust bundle.
	require.NoError(t, WriteTrustBundleFS(fileSvc, cfg, []byte(bundlePEM), constants.PermFilePublic))

	// Write credentials JSON LAST.
	userID = "user-existing"
	cliSessionID = "cli-session-existing"
	creds := &Credentials{UserID: userID, CLISessionID: cliSessionID}
	require.NoError(t, SaveCredentials(fileSvc, cfg, creds))
	return userID, cliSessionID
}

// writeCompleteIdentityWithBundleFP writes a complete, valid CLI identity
// to disk (like writeCompleteIdentityWithExpiry) but uses the SAME CA for
// both the trust bundle and the CLI cert signing, and returns the bundle's
// primary root fingerprint and PEM. This lets tests inject a matching
// (healthy) or mismatching (stale) live fingerprint via
// mockGateway.DiscoverGatewayCA, and use the valid PEM as the live
// discovery bundle (installSystemTrust parses the live bundle via
// ExtractRootAnchors, so it must be valid PEM, not a placeholder string).
func writeCompleteIdentityWithBundleFP(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config, certNotAfter time.Time) (userID, cliSessionID, bundleRootFP, bundlePEM string) {
	t.Helper()
	caKey, caCert := generateTestCAWithKeyAndExpiry(t, "test-root-ca", time.Now().Add(365*24*time.Hour))
	cliCertPEM, cliKey := testutil.GenerateTestSignedCertWithExpiry(t, "g8e-cli-test", caCert, caKey, certNotAfter)
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "test-leaf", caCert, caKey)

	// Encode the CA cert as PEM for the bundle — uses the SAME CA that
	// signed the CLI cert, so the fingerprint is consistent. Use caCert.Raw
	// (the original DER) rather than re-encoding via CreateCertificate,
	// which would produce a different DER (ECDSA signatures are
	// non-deterministic) and thus a different fingerprint.
	caPEM := pemEncode("CERTIFICATE", caCert.Raw)
	bundlePEM = caPEM + leafPEM
	bundleRootFP = platform.CertFingerprint(caCert)

	// Write CLI cert + key.
	certRel, err := fileSvc.RelFromAbs(cfg.CLICertFile())
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, []byte(cliCertPEM), constants.PermFilePrivate))
	keyDER, err := x509.MarshalECPrivateKey(cliKey)
	require.NoError(t, err)
	keyPEM := pemEncode("EC PRIVATE KEY", keyDER)
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, []byte(keyPEM), constants.PermFilePrivate))

	// Write trust bundle.
	require.NoError(t, WriteTrustBundleFS(fileSvc, cfg, []byte(bundlePEM), constants.PermFilePublic))

	// Write credentials JSON LAST.
	userID = "user-existing"
	cliSessionID = "cli-session-existing"
	creds := &Credentials{UserID: userID, CLISessionID: cliSessionID}
	require.NoError(t, SaveCredentials(fileSvc, cfg, creds))
	return userID, cliSessionID, bundleRootFP, bundlePEM
}

// setupCoordinatorTest builds a coordinator with all mocks injected and a
// fresh temp-rooted fileSvc/cfg. Returns the coordinator, the mocks, and
// the output recorder so individual tests can configure responses.
func setupCoordinatorTest(t *testing.T) (*EnrollmentCoordinator, *mockGateway, *mockKeyProvider, *mockTrustInstaller, *mockBrowserOpener, *mockPasskeyRegistrar, *outputRecorder, fs.RuntimeFileService, *config.Config) {
	t.Helper()
	fileSvc, cfg := newAuthTestEnv(t)

	gw := &mockGateway{}
	keys := &mockKeyProvider{}
	trust := &mockTrustInstaller{}
	browser := &mockBrowserOpener{}
	passkey := &mockPasskeyRegistrar{}
	recorder := &outputRecorder{}

	coord := NewEnrollmentCoordinator(EnrollmentCoordinatorDeps{
		Gateway: gw,
		Store:   NewCredentialStore(fileSvc, cfg),
		Keys:    keys,
		Trust:   trust,
		Browser: browser,
		Passkey: passkey,
		Confirm: func(string) bool { return true },
		FileSvc: fileSvc,
		Cfg:     cfg,
		Clock:   time.Now,
		Out:     recorder.out,
	})
	return coord, gw, keys, trust, browser, passkey, recorder, fileSvc, cfg
}

// --- State classification tests ---

func TestEnroll_AbsentUnbootstrapped_PerformsBootstrap(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceBootstrap, result.Source)
	assert.False(t, result.Reused)
	assert.Equal(t, "user-456", result.UserID)
	assert.Equal(t, "cli-session-123", result.CLISessionID)

	assert.Equal(t, 1, gw.bootstrapCalls, "bootstrap should be called exactly once")
	assert.Equal(t, 0, gw.recoveryRequestCalls, "recovery should not be called")
	assert.Equal(t, 0, gw.rotateCalls, "rotation should not be called")

	// System trust should be installed (IsTrusted returns false, InstallRoot runs).
	assert.Equal(t, 1, trust.isTrustedCalls)
	assert.Equal(t, 1, trust.installCalls)
	assert.True(t, result.SystemTrustInstalled)

	// Passkey ceremony should run.
	assert.Equal(t, 1, passkey.calls)
	assert.Equal(t, "user-456", passkey.lastUserID)

	// Credentials should be committed to disk.
	creds, err := LoadCredentials(fileSvc, cfg)
	require.NoError(t, err)
	assert.Equal(t, "cli-session-123", creds.CLISessionID)

	// Output should mention bootstrap.
	assert.True(t, recorder.contains("bootstrap"))
}

func TestEnroll_AbsentBootstrapped_PerformsRecovery(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, browser, passkey, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceRecovery)
	gw.bootstrapStatus = true
	gw.recoveryRequestID = "req-123"
	gw.recoveryToken = "token-abc"
	gw.recoveryApprovalURL = "https://example.com/console#recovery=token-abc"
	gw.recoveryStates = []models.CLIRecoveryState{models.CLIRecoveryStateApproved}
	gw.recoveryCompleteArtifact = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceRecovery, result.Source)
	assert.False(t, result.Reused)
	assert.Equal(t, "user-456", result.UserID)

	assert.Equal(t, 0, gw.bootstrapCalls)
	assert.Equal(t, 1, gw.recoveryRequestCalls)
	assert.Equal(t, 1, gw.recoveryCompleteCalls)
	assert.Equal(t, 0, gw.rotateCalls)

	// Browser should open for approval.
	assert.Equal(t, 1, browser.calls)
	assert.Contains(t, browser.urls[0], "recovery")

	// System trust + passkey.
	assert.Equal(t, 1, trust.isTrustedCalls)
	assert.Equal(t, 1, trust.installCalls)
	assert.Equal(t, 1, passkey.calls)

	// Credentials committed.
	creds, err := LoadCredentials(fileSvc, cfg)
	require.NoError(t, err)
	assert.Equal(t, "cli-session-123", creds.CLISessionID)

	assert.True(t, recorder.contains("recovery"))
}

func TestEnroll_CompleteHealthy_ReusesIdentity(t *testing.T) {
	t.Parallel()
	coord, gw, _, trust, browser, passkey, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	userID, cliSessionID, bundleFP, liveBundlePEM := writeCompleteIdentityWithBundleFP(t, fileSvc, cfg, time.Now().Add(365*24*time.Hour))
	// Inject a matching live fingerprint so the bundle is NOT stale.
	// Use the valid bundle PEM — installSystemTrust parses the live bundle
	// via ExtractRootAnchors, so it must be valid PEM.
	gw.discoveryBundlePEM = []byte(liveBundlePEM)
	gw.discoveryFingerprint = bundleFP

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.True(t, result.Reused)
	assert.Equal(t, userID, result.UserID)
	assert.Equal(t, cliSessionID, result.CLISessionID)

	// No enrollment calls.
	assert.Equal(t, 0, gw.bootstrapCalls)
	assert.Equal(t, 0, gw.recoveryRequestCalls)
	assert.Equal(t, 0, gw.rotateCalls)

	// Discovery should have been called once.
	assert.Equal(t, 1, gw.discoveryCalls, "DiscoverGatewayCA should be called once")

	// System trust should still be checked (from live bundle).
	assert.Equal(t, 1, trust.isTrustedCalls)
	assert.Equal(t, 1, trust.installCalls)

	// Passkey should run.
	assert.Equal(t, 1, passkey.calls)
	assert.Equal(t, userID, passkey.lastUserID)

	// No browser open for reuse (passkey registrar manages its own browser).
	assert.Equal(t, 0, browser.calls)

	assert.True(t, recorder.contains("Reusing"))
}

func TestEnroll_CompleteExpiring_RotatesOnce(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	// Write a complete identity with a cert expiring within rotationThreshold.
	_, _, bundleFP, _ := writeCompleteIdentityWithBundleFP(t, fileSvc, cfg, time.Now().Add(1*time.Hour))
	gw.discoveryFingerprint = bundleFP

	artifacts := buildTestArtifacts(t, EnrollmentSourceRotation)
	gw.rotateArtifact = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceRotation, result.Source)
	assert.False(t, result.Reused)

	assert.Equal(t, 0, gw.bootstrapCalls)
	assert.Equal(t, 0, gw.recoveryRequestCalls)
	assert.Equal(t, 1, gw.rotateCalls, "rotation should be called exactly once")

	// System trust + passkey.
	assert.Equal(t, 1, trust.isTrustedCalls)
	assert.Equal(t, 1, trust.installCalls)
	assert.Equal(t, 1, passkey.calls)

	// New credentials committed.
	creds, err := LoadCredentials(fileSvc, cfg)
	require.NoError(t, err)
	assert.Equal(t, "cli-session-123", creds.CLISessionID)

	assert.True(t, recorder.contains("Rotating"))
}

func TestEnroll_CompleteExplicitRotate_RotatesOnce(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, _, fileSvc, cfg := setupCoordinatorTest(t)

	// Write a healthy, non-expiring identity.
	_, _, bundleFP, _ := writeCompleteIdentityWithBundleFP(t, fileSvc, cfg, time.Now().Add(365*24*time.Hour))
	gw.discoveryFingerprint = bundleFP

	artifacts := buildTestArtifacts(t, EnrollmentSourceRotation)
	gw.rotateArtifact = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{RotateCLI: true})
	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceRotation, result.Source)
	assert.False(t, result.Reused)
	assert.Equal(t, 1, gw.rotateCalls)
}

func TestEnroll_CompleteExpired_UsesRecoveryNotRotation(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, browser, _, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	// Write a complete identity with an already-expired cert.
	_, _, bundleFP, _ := writeCompleteIdentityWithBundleFP(t, fileSvc, cfg, time.Now().Add(-time.Hour))
	gw.discoveryFingerprint = bundleFP

	artifacts := buildTestArtifacts(t, EnrollmentSourceRecovery)
	gw.recoveryRequestID = "req-exp"
	gw.recoveryToken = "token-exp"
	gw.recoveryApprovalURL = "https://example.com/console#recovery=token-exp"
	gw.recoveryStates = []models.CLIRecoveryState{models.CLIRecoveryStateApproved}
	gw.recoveryCompleteArtifact = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceRecovery, result.Source)

	// Rotation must NOT be called — expired certs can't authenticate via mTLS.
	assert.Equal(t, 0, gw.rotateCalls)
	assert.Equal(t, 1, gw.recoveryRequestCalls)
	assert.Equal(t, 1, browser.calls)
	assert.True(t, recorder.contains("expired"))
}

func TestEnroll_PartialState_UsesRecovery(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	// Write only the credentials JSON (no cert/key/bundle) → partial.
	creds := &Credentials{UserID: "user-partial", CLISessionID: "cli-partial"}
	require.NoError(t, SaveCredentials(fileSvc, cfg, creds))

	gw.bootstrapStatus = true
	artifacts := buildTestArtifacts(t, EnrollmentSourceRecovery)
	gw.recoveryRequestID = "req-p"
	gw.recoveryToken = "token-p"
	gw.recoveryApprovalURL = "https://example.com/console#recovery=token-p"
	gw.recoveryStates = []models.CLIRecoveryState{models.CLIRecoveryStateApproved}
	gw.recoveryCompleteArtifact = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceRecovery, result.Source)
	assert.Equal(t, 1, gw.recoveryRequestCalls)
	assert.Equal(t, 0, gw.bootstrapCalls)
	assert.True(t, recorder.contains("partial"))
}

// TestEnroll_PartialState_Unbootstrapped_PerformsBootstrap verifies that
// a partial local identity on an unbootstrapped gateway bootstraps rather
// than attempting recovery (which the gateway would reject with 403). This
// is the defense-in-depth companion to the classifier fix: even if partial
// state reaches the coordinator, it must not bypass the bootstrap check.
func TestEnroll_PartialState_Unbootstrapped_PerformsBootstrap(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	// Write only the credentials JSON (no cert/key/bundle) → partial.
	creds := &Credentials{UserID: "user-partial", CLISessionID: "cli-partial"}
	require.NoError(t, SaveCredentials(fileSvc, cfg, creds))

	gw.bootstrapStatus = false
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceBootstrap, result.Source)
	assert.Equal(t, 1, gw.bootstrapCalls, "bootstrap should be called on unbootstrapped gateway")
	assert.Equal(t, 0, gw.recoveryRequestCalls, "recovery must not be attempted on unbootstrapped gateway")
	assert.True(t, recorder.contains("bootstrap"))
}

func TestEnroll_CorruptState_UsesRecovery(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	// Write credentials JSON + cert but corrupt the key (unparseable).
	caKey, caCert := generateTestCAWithKey(t, "test-root-ca")
	cliCertPEM, _ := testutil.GenerateTestSignedCert(t, "g8e-cli-test", caCert, caKey)
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "test-leaf", caCert, caKey)
	bundlePEM := testutil.GenerateTestCA(t, "test-root-ca") + leafPEM

	certRel, err := fileSvc.RelFromAbs(cfg.CLICertFile())
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, []byte(cliCertPEM), constants.PermFilePrivate))
	keyRel, err := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, []byte("not-a-valid-key"), constants.PermFilePrivate))
	require.NoError(t, WriteTrustBundleFS(fileSvc, cfg, []byte(bundlePEM), constants.PermFilePublic))
	require.NoError(t, SaveCredentials(fileSvc, cfg, &Credentials{UserID: "u", CLISessionID: "s"}))

	gw.bootstrapStatus = true
	artifacts := buildTestArtifacts(t, EnrollmentSourceRecovery)
	gw.recoveryRequestID = "req-c"
	gw.recoveryToken = "token-c"
	gw.recoveryApprovalURL = "https://example.com/console#recovery=token-c"
	gw.recoveryStates = []models.CLIRecoveryState{models.CLIRecoveryStateApproved}
	gw.recoveryCompleteArtifact = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceRecovery, result.Source)
	assert.True(t, recorder.contains("corrupt"))
}

// --- Recovery polling tests ---

func TestEnroll_RecoveryDenied_ReturnsTypedError(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, browser, _, _, _, _ := setupCoordinatorTest(t)

	gw.bootstrapStatus = true
	gw.recoveryRequestID = "req-d"
	gw.recoveryToken = "token-d"
	gw.recoveryApprovalURL = "https://example.com/console#recovery=token-d"
	gw.recoveryStates = []models.CLIRecoveryState{models.CLIRecoveryStateDenied}
	keys.csr, keys.key = "test-csr", nil

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIRecoveryRequestDenied)
	assert.Equal(t, 1, browser.calls)
}

func TestEnroll_RecoveryExpired_ReturnsTypedError(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, _, _, _ := setupCoordinatorTest(t)

	gw.bootstrapStatus = true
	gw.recoveryRequestID = "req-e"
	gw.recoveryToken = "token-e"
	gw.recoveryApprovalURL = "https://example.com/console#recovery=token-e"
	gw.recoveryStates = []models.CLIRecoveryState{models.CLIRecoveryStateExpired}
	keys.csr, keys.key = "test-csr", nil

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIRecoveryRequestExpired)
}

func TestEnroll_RecoveryPollsUntilApproved(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, _, _, _ := setupCoordinatorTest(t)

	gw.bootstrapStatus = true
	gw.recoveryRequestID = "req-poll"
	gw.recoveryToken = "token-poll"
	gw.recoveryApprovalURL = "https://example.com/console#recovery=token-poll"
	// Pending twice, then approved.
	gw.recoveryStates = []models.CLIRecoveryState{
		models.CLIRecoveryStatePending,
		models.CLIRecoveryStatePending,
		models.CLIRecoveryStateApproved,
	}
	artifacts := buildTestArtifacts(t, EnrollmentSourceRecovery)
	gw.recoveryCompleteArtifact = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceRecovery, result.Source)
	assert.Equal(t, 1, gw.recoveryCompleteCalls)
}

// --- Cancellation tests ---

func TestEnroll_ContextCancelled_AbortsBeforeEnrollment(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, _, _, _ := setupCoordinatorTest(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	gw.bootstrapStatus = false
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	_, err := coord.Enroll(ctx, EnrollmentOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestEnroll_RecoveryContextCancelled_AbortsPolling(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, browser, _, _, _, _ := setupCoordinatorTest(t)

	gw.bootstrapStatus = true
	gw.recoveryRequestID = "req-cancel"
	gw.recoveryToken = "token-cancel"
	gw.recoveryApprovalURL = "https://example.com/console#recovery=token-cancel"
	// Always pending — never terminates on its own.
	gw.recoveryStates = nil // defaults to pending
	keys.csr, keys.key = "test-csr", nil

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay to let polling start.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := coord.Enroll(ctx, EnrollmentOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, browser.calls, "browser should open before cancellation")
}

// --- --no-system-trust tests ---

func TestEnroll_NoSystemTrust_SkipsInstaller(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, recorder, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{NoSystemTrust: true})
	require.NoError(t, err)
	assert.False(t, result.SystemTrustInstalled)
	// C4: stale detection still runs under --no-system-trust; only
	// installation (IsTrusted + InstallRoot) is skipped.
	assert.Equal(t, 0, trust.isTrustedCalls, "IsTrusted should not be called")
	assert.Equal(t, 0, trust.installCalls, "InstallRoot should not be called")
	assert.Equal(t, 1, trust.staleListCalls, "ListStaleAnchors should still run under --no-system-trust")
	assert.Equal(t, 1, passkey.calls, "passkey should still run")
	assert.True(t, recorder.contains("--no-system-trust"))
}

func TestEnroll_NoSystemTrust_ReusedIdentity_SkipsInstaller(t *testing.T) {
	t.Parallel()
	coord, gw, _, trust, _, passkey, _, fileSvc, cfg := setupCoordinatorTest(t)

	_, _, bundleFP, _ := writeCompleteIdentityWithBundleFP(t, fileSvc, cfg, time.Now().Add(365*24*time.Hour))
	gw.discoveryFingerprint = bundleFP

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{NoSystemTrust: true})
	require.NoError(t, err)
	assert.True(t, result.Reused)
	// C4: stale detection still runs under --no-system-trust; only
	// installation (IsTrusted + InstallRoot) is skipped.
	assert.Equal(t, 0, trust.isTrustedCalls, "IsTrusted should not be called")
	assert.Equal(t, 0, trust.installCalls, "InstallRoot should not be called")
	assert.Equal(t, 1, trust.staleListCalls, "ListStaleAnchors should still run under --no-system-trust")
	assert.Equal(t, 1, passkey.calls)
}

func TestEnroll_SystemTrustFailure_AbortsBeforePasskey(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, _, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.installErr = constants.ErrSystemTrustInstallFailed

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSystemTrustInstallFailed)
	assert.Equal(t, 0, passkey.calls, "passkey must not run after trust failure")
}

func TestEnroll_SystemTrustAlreadyTrusted_NoPrivilegePrompt(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, recorder, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.isTrusted = true

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.False(t, result.SystemTrustInstalled)
	assert.Equal(t, 1, passkey.calls)
	assert.True(t, recorder.contains("already trusted"))
}

func TestEnroll_SystemTrustInstalled_PrintsBrowserCloseNote(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, recorder, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	// IsTrusted returns false (default), InstallRoot succeeds (default) → installed.

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.True(t, result.SystemTrustInstalled)
	assert.True(t, recorder.contains("close all open browser windows"))
}

// --- SkipPasskey tests ---

func TestEnroll_SkipPasskey_SuppressesCeremony(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, passkey, _, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{SkipPasskey: true})
	require.NoError(t, err)
	assert.Equal(t, 0, passkey.calls, "passkey should be suppressed")
}

// --- Error propagation tests ---

func TestEnroll_BootstrapStatusError_ReturnsError(t *testing.T) {
	t.Parallel()
	coord, gw, _, _, _, _, _, _, _ := setupCoordinatorTest(t)

	gw.bootstrapStatusErr = constants.ErrServiceUnavailable

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.Error(t, err)
}

func TestEnroll_BootstrapError_ReturnsError(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, _, _, _ := setupCoordinatorTest(t)

	gw.bootstrapStatus = false
	gw.bootstrapErr = constants.ErrEnrollmentFailed
	keys.csr, keys.key = "test-csr", nil

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.Error(t, err)
}

func TestEnroll_RecoveryRequestError_ReturnsTypedError(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, _, _, _ := setupCoordinatorTest(t)

	gw.bootstrapStatus = true
	gw.recoveryRequestErr = constants.ErrCLIRecoveryRequestFailed
	keys.csr, keys.key = "test-csr", nil

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIRecoveryRequestFailed)
}

func TestEnroll_RotationError_ReturnsError(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, _, fileSvc, cfg := setupCoordinatorTest(t)

	_, _, bundleFP, _ := writeCompleteIdentityWithBundleFP(t, fileSvc, cfg, time.Now().Add(1*time.Hour))
	gw.discoveryFingerprint = bundleFP
	gw.rotateErr = constants.ErrEnrollmentFailed
	keys.csr, keys.key = "test-csr", nil

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.Error(t, err)
}

func TestEnroll_BrowserOpenFailure_ContinuesRecovery(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, browser, _, recorder, _, _ := setupCoordinatorTest(t)

	gw.bootstrapStatus = true
	gw.recoveryRequestID = "req-bf"
	gw.recoveryToken = "token-bf"
	gw.recoveryApprovalURL = "https://example.com/console#recovery=token-bf"
	gw.recoveryStates = []models.CLIRecoveryState{models.CLIRecoveryStateApproved}
	artifacts := buildTestArtifacts(t, EnrollmentSourceRecovery)
	gw.recoveryCompleteArtifact = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	browser.err = constants.ErrBrowserURLEmpty

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err, "browser failure should not abort recovery")
	assert.Equal(t, EnrollmentSourceRecovery, result.Source)
	assert.True(t, recorder.contains("manually"))
}

func TestEnroll_PasskeyFailure_ReturnsTypedError(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, passkey, _, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	passkey.err = constants.ErrPasskeyRegistrationTimedOut

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPasskeyRegistrationFailed)
}

// --- No-hidden-writes test ---

func TestEnroll_TransportMethods_NoHiddenFileWrites(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, _, fileSvc, cfg := setupCoordinatorTest(t)

	// Use a spying file service wrapper to detect unexpected writes.
	// The mock gateway performs no file I/O, so the only writes should
	// come from CredentialStore.Commit (after Stage). We verify by
	// checking that no files are written before the gateway is called.
	gw.bootstrapStatus = false
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.isTrusted = true
	passkey.err = nil

	// Snapshot file state before enrollment.
	credsRel, _ := fileSvc.RelFromAbs(cfg.CredentialsFile())
	certRel, _ := fileSvc.RelFromAbs(cfg.CLICertFile())
	keyRel, _ := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	bundleRel := cfg.DefaultTrustBundleRelPath()

	// Before enrollment, no files should exist.
	for _, rel := range []string{credsRel, certRel, keyRel, bundleRel} {
		exists, err := fileSvc.FileExists(context.Background(), rel)
		require.NoError(t, err)
		assert.False(t, exists, "file %s should not exist before enrollment", rel)
	}

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{SkipPasskey: true})
	require.NoError(t, err)

	// After enrollment, all managed files should exist (written by Commit).
	for _, rel := range []string{credsRel, certRel, keyRel, bundleRel} {
		exists, err := fileSvc.FileExists(context.Background(), rel)
		require.NoError(t, err)
		assert.True(t, exists, "file %s should exist after enrollment", rel)
	}
}

// --- Staging failure rollback test ---

func TestEnroll_StageValidationFailure_RollsBackAndReturnsError(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, _, _, _, fileSvc, cfg := setupCoordinatorTest(t)

	gw.bootstrapStatus = false
	// Artifacts with mismatched cert/key → Stage validation fails.
	caKey, caCert := generateTestCAWithKey(t, "test-root-ca")
	cliCertPEM, _ := testutil.GenerateTestSignedCert(t, "g8e-cli-test", caCert, caKey)
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "test-leaf", caCert, caKey)
	bundlePEM := testutil.GenerateTestCA(t, "test-root-ca") + leafPEM

	// Different key than the one the cert was signed with.
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	gw.bootstrapArtifacts = EnrollmentArtifacts{
		Source:         EnrollmentSourceBootstrap,
		CLISessionID:   "s",
		UserID:         "u",
		CLICertPEM:     cliCertPEM,
		CLIKey:         otherKey,
		TrustBundlePEM: bundlePEM,
	}
	keys.csr, keys.key = "test-csr", otherKey

	_, err = coord.Enroll(context.Background(), EnrollmentOptions{})
	require.Error(t, err)

	// No credentials should be committed.
	creds, err := LoadCredentials(fileSvc, cfg)
	require.NoError(t, err)
	assert.Nil(t, creds, "no credentials should be written after Stage failure")
}

// --- Stale trust anchor tests ---

// TestEnroll_StaleAnchors_Confirmed_RemovesAndInstalls verifies that when
// stale g8e root anchors are found, the coordinator prompts the user, removes
// them on confirmation, then installs the new root and prints the browser-
// close directive.
func TestEnroll_StaleAnchors_Confirmed_RemovesAndInstalls(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, recorder, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.staleAnchors = []platform.StaleAnchor{
		{Fingerprint: "stale-fp-1", CommonName: constants.RootCACommonName, Handle: "/path/stale1"},
		{Fingerprint: "stale-fp-2", CommonName: constants.RootCACommonName, Handle: "/path/stale2"},
	}
	// IsTrusted returns false (default), InstallRoot succeeds (default) → installed.

	confirmCalled := false
	coord.confirm = func(prompt string) bool {
		confirmCalled = true
		assert.Contains(t, prompt, "2 stale")
		return true
	}

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.True(t, result.SystemTrustInstalled)
	assert.True(t, confirmCalled, "confirm should be called when stale anchors exist")
	assert.Equal(t, 1, trust.staleListCalls, "ListStaleAnchors should be called once")
	assert.Equal(t, 1, trust.staleRemoveCalls, "RemoveStaleAnchors should be called once")
	assert.Equal(t, 1, passkey.calls, "passkey should run after stale removal + install")
	assert.True(t, recorder.contains("stale"))
	assert.True(t, recorder.contains("close all open browser windows"))
}

// TestEnroll_StaleAnchors_Declined_AbortsBeforeInstall verifies that when
// the user declines stale anchor removal, enrollment aborts with
// ErrSystemTrustStaleRemovalDenied before installing the new root or
// launching the browser.
func TestEnroll_StaleAnchors_Declined_AbortsBeforeInstall(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, _, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.staleAnchors = []platform.StaleAnchor{
		{Fingerprint: "stale-fp-1", CommonName: constants.RootCACommonName, Handle: "/path/stale1"},
	}

	coord.confirm = func(prompt string) bool { return false }

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSystemTrustStaleRemovalDenied)
	assert.Equal(t, 0, trust.staleRemoveCalls, "RemoveStaleAnchors must not be called when declined")
	assert.Equal(t, 0, passkey.calls, "passkey must not run when user declines stale removal")
}

// TestEnroll_StaleAnchors_None_NoPrompt verifies that when no stale anchors
// are found, the confirm function is NOT called and enrollment proceeds
// normally.
func TestEnroll_StaleAnchors_None_NoPrompt(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, _, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.staleAnchors = nil
	// IsTrusted returns false (default), InstallRoot succeeds (default) → installed.

	confirmCalled := false
	coord.confirm = func(prompt string) bool {
		confirmCalled = true
		return true
	}

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.False(t, confirmCalled, "confirm should not be called when no stale anchors")
	assert.Equal(t, 0, trust.staleRemoveCalls, "RemoveStaleAnchors should not be called")
	assert.Equal(t, 1, passkey.calls, "passkey should run normally")
}

// TestEnroll_StaleAnchors_RemovalError_Aborts verifies that a removal error
// aborts enrollment with ErrSystemTrustInstallFailed before the passkey
// ceremony.
func TestEnroll_StaleAnchors_RemovalError_Aborts(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, _, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.staleAnchors = []platform.StaleAnchor{
		{Fingerprint: "stale-fp-1", CommonName: constants.RootCACommonName, Handle: "/path/stale1"},
	}
	trust.removeErr = constants.ErrSystemTrustInstallFailed

	coord.confirm = func(prompt string) bool { return true }

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrSystemTrustInstallFailed)
	assert.Equal(t, 0, passkey.calls, "passkey must not run after stale removal failure")
}

// TestEnroll_StaleAnchors_ListError_ProceedsAsBestEffort verifies that when
// ListStaleAnchors returns a non-unsupported error, enrollment proceeds
// (best-effort) rather than aborting — stale detection is a safety improvement,
// not a gate.
func TestEnroll_StaleAnchors_ListError_ProceedsAsBestEffort(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, recorder, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.staleErr = errors.New("enumeration failed")
	// IsTrusted returns false (default), InstallRoot succeeds (default) → installed.

	confirmCalled := false
	coord.confirm = func(prompt string) bool {
		confirmCalled = true
		return true
	}

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.False(t, confirmCalled, "confirm should not be called when list errored")
	assert.True(t, recorder.contains("could not check for stale"))
	assert.Equal(t, 1, passkey.calls, "passkey should run despite stale list error")
}

// TestEnroll_StaleAnchors_Unsupported_SkipsDetection verifies that on
// platforms where stale detection is unsupported (stub returns
// ErrSystemTrustUnsupported), enrollment proceeds without warning or
// prompting.
func TestEnroll_StaleAnchors_Unsupported_SkipsDetection(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, recorder, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.staleErr = constants.ErrSystemTrustUnsupported
	// IsTrusted returns false (default), InstallRoot succeeds (default) → installed.

	confirmCalled := false
	coord.confirm = func(prompt string) bool {
		confirmCalled = true
		return true
	}

	_, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.False(t, confirmCalled, "confirm should not be called on unsupported platform")
	assert.False(t, recorder.contains("could not check for stale"), "no warning on unsupported platform")
	assert.Equal(t, 1, passkey.calls)
}

// TestInstallSystemTrust_UnsupportedPlatformWarns verifies that when
// IsTrusted and InstallRoot return ErrSystemTrustUnsupported (stub
// platform), installSystemTrust prints a warning and proceeds without
// failing enrollment. The passkey ceremony still runs.
func TestInstallSystemTrust_UnsupportedPlatformWarns(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, recorder, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.isTrustedErr = constants.ErrSystemTrustUnsupported
	trust.installErr = constants.ErrSystemTrustUnsupported

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err, "unsupported platform must not fail enrollment")
	assert.False(t, result.SystemTrustInstalled, "nothing installed on unsupported platform")
	assert.Equal(t, 1, trust.isTrustedCalls, "IsTrusted should be called once")
	assert.Equal(t, 0, trust.installCalls, "InstallRoot should NOT be called when IsTrusted returns unsupported")
	assert.True(t, recorder.contains("unsupported"), "should print unsupported warning")
	assert.Equal(t, 1, passkey.calls, "passkey should still run")
}

// TestInstallSystemTrust_InstallRootUnsupportedWarns verifies that when
// IsTrusted succeeds (returns false) but InstallRoot returns
// ErrSystemTrustUnsupported, installSystemTrust prints a warning and
// proceeds without failing enrollment.
func TestInstallSystemTrust_InstallRootUnsupportedWarns(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, recorder, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.isTrusted = false
	trust.installErr = constants.ErrSystemTrustUnsupported

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err, "unsupported InstallRoot must not fail enrollment")
	assert.False(t, result.SystemTrustInstalled)
	assert.Equal(t, 1, trust.isTrustedCalls)
	assert.Equal(t, 1, trust.installCalls, "InstallRoot should be called (IsTrusted succeeded)")
	assert.True(t, recorder.contains("unsupported"))
	assert.Equal(t, 1, passkey.calls)
}

// --- Stale trust bundle on reused identity tests (R1/R2/R3/R5) ---

// TestEnroll_ReusedIdentity_StaleBundle_RoutesToRecovery is the failing
// test that confirms the root cause: a complete local identity whose
// trust bundle root fingerprint does NOT match the live gateway root CA
// must NOT take the reuse branch. Instead, it routes through recovery
// (human-approved, plain-HTTP, token-scoped), which issues a fresh cert
// signed by the new CA. This fails on the pre-fix code (which takes the
// reuse branch and never calls discovery).
func TestEnroll_ReusedIdentity_StaleBundle_RoutesToRecovery(t *testing.T) {
	t.Parallel()
	coord, gw, keys, _, browser, _, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	// Write a complete identity with a known bundle root fingerprint.
	writeCompleteIdentityWithBundleFP(t, fileSvc, cfg, time.Now().Add(365*24*time.Hour))

	// Inject a MISMATCHING live fingerprint — simulates `gw clean`
	// regenerating the gateway PKI.
	gw.discoveryFingerprint = "different-live-fp"
	gw.discoveryBundlePEM = []byte("live-bundle")

	// Configure recovery flow.
	artifacts := buildTestArtifacts(t, EnrollmentSourceRecovery)
	gw.recoveryRequestID = "req-stale"
	gw.recoveryToken = "token-stale"
	gw.recoveryApprovalURL = "https://example.com/console#recovery=token-stale"
	gw.recoveryStates = []models.CLIRecoveryState{models.CLIRecoveryStateApproved}
	gw.recoveryCompleteArtifact = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceRecovery, result.Source)
	assert.False(t, result.Reused, "must NOT reuse when bundle is stale")

	// Discovery should have been called.
	assert.Equal(t, 1, gw.discoveryCalls)
	// Recovery should be called, not rotation or bootstrap.
	assert.Equal(t, 1, gw.recoveryRequestCalls)
	assert.Equal(t, 1, gw.recoveryCompleteCalls)
	assert.Equal(t, 0, gw.rotateCalls, "rotation must not be called — stale cert can't authenticate via mTLS")
	assert.Equal(t, 0, gw.bootstrapCalls)

	// Browser should open for recovery approval.
	assert.Equal(t, 1, browser.calls)

	assert.True(t, recorder.contains("does not match the live gateway"))
	assert.True(t, recorder.contains("recovery"))
}

// TestEnroll_ReusedIdentity_HealthyBundle_ReusesWithLiveFingerprint
// verifies that when the live fingerprint matches the local bundle, the
// reuse branch is taken AND installSystemTrust uses the LIVE fingerprint
// (not the local bundle's fingerprint) for stale-anchor detection and
// IsTrusted/InstallRoot.
func TestEnroll_ReusedIdentity_HealthyBundle_ReusesWithLiveFingerprint(t *testing.T) {
	t.Parallel()
	coord, gw, _, trust, _, passkey, _, fileSvc, cfg := setupCoordinatorTest(t)

	_, _, bundleFP, liveBundlePEM := writeCompleteIdentityWithBundleFP(t, fileSvc, cfg, time.Now().Add(365*24*time.Hour))
	gw.discoveryFingerprint = bundleFP
	// Use the valid bundle PEM — installSystemTrust parses the live bundle
	// via ExtractRootAnchors, so it must be valid PEM.
	gw.discoveryBundlePEM = []byte(liveBundlePEM)

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.True(t, result.Reused)

	// ListStaleAnchors must be called with the LIVE fingerprint.
	assert.Equal(t, 1, trust.staleListCalls)
	assert.Equal(t, bundleFP, trust.lastStaleFingerprint, "ListStaleAnchors should receive the live fingerprint")

	// IsTrusted + InstallRoot should be called with the LIVE fingerprint
	// (not the local bundle's fingerprint). The live bundle was parsed by
	// ExtractRootAnchors to obtain the root cert, and keepFingerprint was
	// set to the live fingerprint from discovery.
	assert.Equal(t, 1, trust.isTrustedCalls, "IsTrusted should be called once")
	assert.Equal(t, 1, trust.installCalls, "InstallRoot should be called once")
	assert.Equal(t, bundleFP, trust.lastTrustedFP, "IsTrusted should receive the live fingerprint")
	assert.Equal(t, bundleFP, trust.lastInstallFP, "InstallRoot should receive the live fingerprint")

	assert.Equal(t, 1, passkey.calls)
}

// TestEnroll_ReusedIdentity_DiscoveryUnreachable_WarnsAndProceeds
// verifies that when DiscoverGatewayCA returns a network error, the
// coordinator prints a diagnostic warning and proceeds to reuse (does
// NOT abort). The user may be intentionally offline.
func TestEnroll_ReusedIdentity_DiscoveryUnreachable_WarnsAndProceeds(t *testing.T) {
	t.Parallel()
	coord, gw, _, _, _, passkey, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	writeCompleteIdentity(t, fileSvc, cfg)
	gw.discoveryErr = constants.ErrServiceUnavailable

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.True(t, result.Reused, "should proceed with reuse when discovery is unreachable")

	// Should print the diagnostic warning.
	assert.True(t, recorder.contains("could not reach the gateway discovery endpoint"),
		"should print discovery-unreachable warning")

	// Passkey should still run.
	assert.Equal(t, 1, passkey.calls)
}

// TestEnroll_StaleAnchorNotDetectedWhenDiscoveryFails is the R5 regression
// guard. On the pre-fix code, when discovery was unreachable on the reuse
// path, ensureSystemTrust fell back to the local (possibly stale) trust
// bundle fingerprint for ListStaleAnchors, so the old OS anchor matched
// the "keep" fingerprint and was NOT reported as stale. EnsureSystemTrust
// then reported "already trusted" with the stale fingerprint, and the
// passkey ceremony proceeded against a root the browser did not trust.
//
// After R2, installSystemTrust surfaces a diagnosable R5 warning when
// discovery is unreachable AND reuse is attempted, BEFORE bundle
// resolution. This test asserts the warning IS printed — it would have
// failed on the old ensureSystemTrust which did not print a warning in
// the unreachable-reuse path.
func TestEnroll_StaleAnchorNotDetectedWhenDiscoveryFails(t *testing.T) {
	t.Parallel()
	coord, gw, _, trust, _, passkey, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	_, _, localFP, _ := writeCompleteIdentityWithBundleFP(t, fileSvc, cfg, time.Now().Add(365*24*time.Hour))
	// Discovery is unreachable — the coordinator cannot obtain the live
	// fingerprint, so it falls back to the local bundle.
	gw.discoveryErr = constants.ErrServiceUnavailable

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.True(t, result.Reused, "should proceed with reuse when discovery is unreachable")

	// R5 warning MUST be printed — this is the regression assertion.
	assert.True(t, recorder.contains("could not reach the gateway discovery endpoint"),
		"R5 warning must be printed when discovery is unreachable on the reuse path")

	// ListStaleAnchors is called with the LOCAL fingerprint (the only one
	// available when discovery fails). This means a stale OS anchor
	// matching the local fingerprint would NOT be detected — the R5
	// warning is the user's only signal that something may be wrong.
	assert.Equal(t, 1, trust.staleListCalls)
	assert.Equal(t, localFP, trust.lastStaleFingerprint,
		"ListStaleAnchors should receive the local fingerprint when discovery is unreachable")

	// Passkey should still run (best-effort, user may be intentionally offline).
	assert.Equal(t, 1, passkey.calls)
}

// TestEnroll_ReusedIdentity_StaleBundle_DetectsStaleOSAnchors verifies
// that when the local bundle is stale (OLD_FP) and the live fingerprint is
// NEW_FP, ListStaleAnchors is called with NEW_FP (not OLD_FP), the stale
// anchor OLD_FP is listed, the user is prompted, and on confirmation
// RemoveStaleAnchors is called before recovery runs.
func TestEnroll_ReusedIdentity_StaleBundle_DetectsStaleOSAnchors(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, browser, _, recorder, fileSvc, cfg := setupCoordinatorTest(t)

	_, _, oldFP, _ := writeCompleteIdentityWithBundleFP(t, fileSvc, cfg, time.Now().Add(365*24*time.Hour))

	// Live fingerprint is different — bundle is stale.
	newFP := "new-live-fp"
	gw.discoveryFingerprint = newFP
	gw.discoveryBundlePEM = []byte("live-bundle-new-fp")

	// The OS store has the OLD root anchor — it should be detected as stale.
	trust.staleAnchors = []platform.StaleAnchor{
		{Fingerprint: oldFP, CommonName: constants.RootCACommonName, Handle: "/path/old"},
	}
	// IsTrusted returns false (default), InstallRoot succeeds (default) → installed.

	// Configure recovery flow.
	artifacts := buildTestArtifacts(t, EnrollmentSourceRecovery)
	gw.recoveryRequestID = "req-stale-os"
	gw.recoveryToken = "token-stale-os"
	gw.recoveryApprovalURL = "https://example.com/console#recovery=token-stale-os"
	gw.recoveryStates = []models.CLIRecoveryState{models.CLIRecoveryStateApproved}
	gw.recoveryCompleteArtifact = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	confirmCalled := false
	coord.confirm = func(prompt string) bool {
		confirmCalled = true
		assert.Contains(t, prompt, "1 stale")
		return true
	}

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceRecovery, result.Source)

	// ListStaleAnchors must be called with the LIVE fingerprint (NEW_FP).
	assert.Equal(t, 1, trust.staleListCalls)
	assert.Equal(t, newFP, trust.lastStaleFingerprint,
		"ListStaleAnchors should receive the LIVE fingerprint, not the stale local one")

	// The stale anchor should be removed.
	assert.True(t, confirmCalled, "user should be prompted about stale anchors")
	assert.Equal(t, 1, trust.staleRemoveCalls, "RemoveStaleAnchors should be called")

	// Recovery should run.
	assert.Equal(t, 1, gw.recoveryRequestCalls)
	assert.Equal(t, 1, browser.calls)

	assert.True(t, recorder.contains("does not match the live gateway"))
}

// TestEnroll_NoSystemTrust_StillRunsStaleDetection verifies that
// --no-system-trust still runs stale-anchor detection (ListStaleAnchors +
// RemoveStaleAnchors) but skips IsTrusted + InstallRoot. This is the C4
// behavior change: the user may have stale anchors that break the browser
// even if the CLI skips installation.
func TestEnroll_NoSystemTrust_StillRunsStaleDetection(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, recorder, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.staleAnchors = []platform.StaleAnchor{
		{Fingerprint: "stale-fp-1", CommonName: constants.RootCACommonName, Handle: "/path/stale1"},
	}

	confirmCalled := false
	coord.confirm = func(prompt string) bool {
		confirmCalled = true
		return true
	}

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{NoSystemTrust: true})
	require.NoError(t, err)
	assert.False(t, result.SystemTrustInstalled)

	// Stale detection should run.
	assert.Equal(t, 1, trust.staleListCalls, "ListStaleAnchors should run under --no-system-trust")
	assert.True(t, confirmCalled, "user should be prompted about stale anchors")
	assert.Equal(t, 1, trust.staleRemoveCalls, "RemoveStaleAnchors should run under --no-system-trust")

	// Installation should be skipped.
	assert.Equal(t, 0, trust.isTrustedCalls, "IsTrusted should NOT be called under --no-system-trust")
	assert.Equal(t, 0, trust.installCalls, "InstallRoot should NOT be called under --no-system-trust")

	// Passkey should still run.
	assert.Equal(t, 1, passkey.calls)
	assert.True(t, recorder.contains("--no-system-trust"))
}

// TestEnroll_Bootstrap_DiscoveryMatchesArtifacts is a regression guard
// for the new-enrollment paths: on bootstrap, ListStaleAnchors should use
// the live fingerprint (from the artifacts, which equal the discovery
// bundle). The bootstrap path receives a fresh bundle from the gateway, so
// the stale-anchor detection is already correct — this test ensures it
// stays correct after the R3 refactor.
func TestEnroll_Bootstrap_DiscoveryMatchesArtifacts(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, _, _, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey

	// Compute the fingerprint of the artifacts' bundle root.
	bundle, err := ParseTrustBundle([]byte(artifacts.TrustBundlePEM), time.Now())
	require.NoError(t, err)
	require.NotEmpty(t, bundle.PrimaryRootFingerprint)

	// Inject a matching live fingerprint.
	gw.discoveryFingerprint = bundle.PrimaryRootFingerprint
	gw.discoveryBundlePEM = []byte(artifacts.TrustBundlePEM)

	_, err = coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)

	// ListStaleAnchors should use the live fingerprint (which equals the
	// artifacts' bundle root fingerprint on the bootstrap path).
	assert.Equal(t, 1, trust.staleListCalls)
	assert.Equal(t, bundle.PrimaryRootFingerprint, trust.lastStaleFingerprint,
		"ListStaleAnchors should use the live fingerprint on bootstrap")
}

// --- Helpers ---

func pemEncode(blockType string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}))
}

// Suppress unused import errors for packages used only in helpers.
var _ = errors.Is
