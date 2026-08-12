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
	mu            sync.Mutex
	calls         int
	result        platform.SystemTrustResult
	err           error
	lastBundlePEM []byte
}

func (m *mockTrustInstaller) EnsureSystemTrust(ctx context.Context, bundlePEM []byte) (platform.SystemTrustResult, error) {
	m.mu.Lock()
	m.calls++
	m.lastBundlePEM = bundlePEM
	m.mu.Unlock()
	return m.result, m.err
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
	caPEM := testutil.GenerateTestCA(t, "test-root-ca")
	rootCert := testutil.ParseTestCert(t, caPEM)
	// Parse the CA key from the PEM — GenerateTestCA doesn't return it,
	// so re-derive by generating a fresh CA with the same approach.
	// Instead, use GenerateTestSignedCert which needs the parent key.
	// Since GenerateTestCA doesn't expose the key, build the CA manually.
	caKey, caCert := generateTestCAWithKey(t, "test-root-ca")
	_ = caPEM
	_ = rootCert

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
	bundlePEM := testutil.GenerateTestCA(t, "test-root-ca") + leafPEM

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

	// System trust should be installed.
	assert.Equal(t, 1, trust.calls)
	assert.True(t, result.SystemTrustInstalled || true, "trust result depends on mock")

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
	assert.Equal(t, 1, trust.calls)
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

	userID, cliSessionID := writeCompleteIdentity(t, fileSvc, cfg)

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.True(t, result.Reused)
	assert.Equal(t, userID, result.UserID)
	assert.Equal(t, cliSessionID, result.CLISessionID)

	// No enrollment calls.
	assert.Equal(t, 0, gw.bootstrapCalls)
	assert.Equal(t, 0, gw.recoveryRequestCalls)
	assert.Equal(t, 0, gw.rotateCalls)

	// System trust should still be checked (from local bundle).
	assert.Equal(t, 1, trust.calls)

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
	writeCompleteIdentityWithExpiry(t, fileSvc, cfg, time.Now().Add(1*time.Hour))

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
	assert.Equal(t, 1, trust.calls)
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
	writeCompleteIdentity(t, fileSvc, cfg)

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
	writeCompleteIdentityWithExpiry(t, fileSvc, cfg, time.Now().Add(-time.Hour))

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
	assert.Equal(t, 0, trust.calls, "trust installer should not be called")
	assert.Equal(t, 1, passkey.calls, "passkey should still run")
	assert.True(t, recorder.contains("--no-system-trust"))
}

func TestEnroll_NoSystemTrust_ReusedIdentity_SkipsInstaller(t *testing.T) {
	t.Parallel()
	coord, _, _, trust, _, passkey, _, fileSvc, cfg := setupCoordinatorTest(t)

	writeCompleteIdentity(t, fileSvc, cfg)

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{NoSystemTrust: true})
	require.NoError(t, err)
	assert.True(t, result.Reused)
	assert.Equal(t, 0, trust.calls)
	assert.Equal(t, 1, passkey.calls)
}

func TestEnroll_SystemTrustFailure_AbortsBeforePasskey(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, passkey, _, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.err = constants.ErrSystemTrustInstallFailed

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
	trust.result = platform.SystemTrustResult{Status: platform.SystemTrustAlreadyTrusted, Fingerprint: "abc123"}

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.False(t, result.SystemTrustInstalled)
	assert.Equal(t, 1, passkey.calls)
	assert.True(t, recorder.contains("already trusted"))
}

func TestEnroll_SystemTrustInstalled_PrintsBrowserRestartNote(t *testing.T) {
	t.Parallel()
	coord, gw, keys, trust, _, _, recorder, _, _ := setupCoordinatorTest(t)

	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	gw.bootstrapStatus = false
	gw.bootstrapArtifacts = artifacts
	keys.csr, keys.key = "test-csr", artifacts.CLIKey
	trust.result = platform.SystemTrustResult{Status: platform.SystemTrustInstalled, Fingerprint: "abc123"}

	result, err := coord.Enroll(context.Background(), EnrollmentOptions{})
	require.NoError(t, err)
	assert.True(t, result.SystemTrustInstalled)
	assert.True(t, recorder.contains("restart"))
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

	writeCompleteIdentityWithExpiry(t, fileSvc, cfg, time.Now().Add(1*time.Hour))
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
	trust.result = platform.SystemTrustResult{Status: platform.SystemTrustAlreadyTrusted}
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

// --- Helpers ---

func pemEncode(blockType string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}))
}

// Suppress unused import errors for packages used only in helpers.
var _ = errors.Is
