// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/auth"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// enrollStub is a gateway stand-in whose three enrollment endpoints are
// supplied per test. A nil route answers 404. Hit counters let tests assert
// that the client did (or did not) talk to the gateway.
type enrollStub struct {
	server      *httptest.Server
	requestHits atomic.Int32
	statusHits  atomic.Int32
	completeHit atomic.Int32
}

type enrollRoutes struct {
	request  http.HandlerFunc
	status   http.HandlerFunc
	complete http.HandlerFunc
}

func newEnrollClient(t *testing.T, gatewayURL string) (*OperatorPlatformEnrollmentClient, fs.RuntimeFileService) {
	t.Helper()
	fileSvc := newTestFileSvc(t)
	client, err := NewOperatorPlatformEnrollmentClient(gatewayURL, "inst-1", "host-1", fileSvc, testLogger())
	require.NoError(t, err)
	return client, fileSvc
}

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func statusReply(state models.PlatformEnrollmentState) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, models.PlatformEnrollmentStatusResponse{
			RequestID: "req-1", ComponentKind: models.PlatformComponentOperator, State: state,
		})
	}
}

func createdReply(requestID, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusCreated, models.PlatformEnrollmentCreateResponse{
			RequestID: requestID, Token: token, ComponentKind: models.PlatformComponentOperator,
			ExpiresAt: time.Now().Add(30 * time.Minute).UTC(),
		})
	}
}

func shortContext(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNewOperatorPlatformEnrollmentClient_RejectsMissingIdentity(t *testing.T) {
	tests := []struct {
		name                      string
		url, instanceID, hostname string
	}{
		{"missing gateway URL", "", "inst", "host"},
		{"missing instance id", "http://gw:8080", "", "host"},
		{"missing hostname", "http://gw:8080", "inst", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewOperatorPlatformEnrollmentClient(tt.url, tt.instanceID, tt.hostname, newTestFileSvc(t), testLogger())

			require.ErrorIs(t, err, constants.ErrInternal)
			assert.Nil(t, client)
		})
	}
}

func TestNewOperatorPlatformEnrollmentClient_NormalizesTrailingSlashes(t *testing.T) {
	client, err := NewOperatorPlatformEnrollmentClient("http://gw:8080///", "inst", "host", newTestFileSvc(t), testLogger())

	require.NoError(t, err)
	assert.Equal(t, "http://gw:8080", client.gatewayHTTPURL)
}

func TestOperatorPlatformEnrollmentClient_SetFingerprintOptions_ReplacesPriorOptions(t *testing.T) {
	client, _ := newEnrollClient(t, "http://gw:8080")
	first := auth.FingerprintOptions{LocalDir: "/srv/a", Account: "alice", Port: 8443, Role: "data"}
	second := auth.FingerprintOptions{LocalDir: "/srv/b", Role: "inference"}

	client.SetFingerprintOptions(first)
	assert.Equal(t, first, client.fingerprintOpts)
	client.SetFingerprintOptions(second)
	assert.Equal(t, second, client.fingerprintOpts)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestParseRetryAfter_AcceptsOnlyPositiveSeconds(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"1", time.Second},
		{"30", 30 * time.Second},
		{"0", 0},
		{"-5", 0},
		{"soon", 0},
		{"Wed, 21 Oct 2015 07:28:00 GMT", 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, parseRetryAfter(tt.in))
		})
	}
}

func TestTrimTrailingSlash(t *testing.T) {
	tests := map[string]string{
		"":               "",
		"http://gw":      "http://gw",
		"http://gw/":     "http://gw",
		"http://gw///":   "http://gw",
		"http://gw/a/b/": "http://gw/a/b",
		"/":              "",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, trimTrailingSlash(in))
		})
	}
}

func TestParseECPrivateKeyPEM_RoundTripsEveryAcceptedEncoding(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	t.Run("SEC1 EC PRIVATE KEY", func(t *testing.T) {
		encoded, err := encodeECPrivateKeyPEM(key)
		require.NoError(t, err)

		parsed, err := parseECPrivateKeyPEM(encoded)

		require.NoError(t, err)
		assert.True(t, key.Equal(parsed))
	})

	t.Run("PKCS8 PRIVATE KEY", func(t *testing.T) {
		der, err := x509.MarshalPKCS8PrivateKey(key)
		require.NoError(t, err)

		parsed, err := parseECPrivateKeyPEM(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))

		require.NoError(t, err)
		assert.True(t, key.Equal(parsed))
	})
}

func TestParseECPrivateKeyPEM_RejectsAnythingThatIsNotAnECKey(t *testing.T) {
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	edDER, err := x509.MarshalPKCS8PrivateKey(edKey)
	require.NoError(t, err)

	tests := []struct {
		name    string
		in      string
		wantMsg string
	}{
		{"no PEM block", "not pem at all", "no PEM block found"},
		{"unexpected PEM type", string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("x")})), `unexpected PEM type "RSA PRIVATE KEY"`},
		{"PKCS8 holding a non-EC key", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: edDER})), "not an EC key"},
		{"PKCS8 with undecodable body", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("junk")})), "parse PKCS8 private key"},
		{"SEC1 with undecodable body", string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("junk")})), "parse EC private key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := parseECPrivateKeyPEM(tt.in)

			require.Error(t, err)
			assert.Nil(t, key)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestCsrFingerprint_RejectsMalformedRequests(t *testing.T) {
	validCSR, _, err := generateTestCSR(t, "fp-test")
	require.NoError(t, err)
	block, _ := pem.Decode([]byte(validCSR))
	require.NotNil(t, block)
	tampered := append([]byte(nil), block.Bytes...)
	tampered[len(tampered)-1] ^= 0xff

	tests := []struct {
		name string
		in   string
	}{
		{"not PEM", "garbage"},
		{"wrong PEM block type", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: block.Bytes}))},
		{"undecodable DER", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: []byte("junk")}))},
		{"tampered signature", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: tampered}))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp, err := csrFingerprint(tt.in)

			require.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidCSR)
			assert.Empty(t, fp)
		})
	}
}

// ---------------------------------------------------------------------------
// submitRequest
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// pollUntilApproved
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// submitCompletion
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// writeCredentials
// ---------------------------------------------------------------------------

func readRuntimeFile(t *testing.T, fileSvc fs.RuntimeFileService, rel string) (string, os.FileMode) {
	t.Helper()
	data, err := fileSvc.ReadFile(t.Context(), rel)
	require.NoError(t, err)
	info, err := fileSvc.Stat(t.Context(), rel)
	require.NoError(t, err)
	return string(data), info.Mode().Perm()
}

func TestWriteCredentials_PersistsChainsKeysTrustBundleAndActuatorSigner(t *testing.T) {
	client, fileSvc := newEnrollClient(t, "http://gw:8080")
	creds := &models.PlatformEnrollmentOperatorCredentials{
		OperatorCert:      "OPERATOR-CERT",
		OperatorCertChain: "OPERATOR-CHAIN",
		CLICert:           "CLI-CERT",
		CLICertChain:      "CLI-CHAIN",
		HubTrustBundle:    "TRUST-BUNDLE",
		ActuatorKeyID:     "actuator-key-1",
		ActuatorPubKey:    "ACTUATOR-PUB",
	}

	require.NoError(t, client.writeCredentials(creds, "OPERATOR-KEY", "CLI-KEY"))

	content, perm := readRuntimeFile(t, fileSvc, client.operatorCertPath())
	assert.Equal(t, "OPERATOR-CERT\nOPERATOR-CHAIN", content, "the chain is appended to the leaf certificate")
	assert.Equal(t, testutil.FileMode(constants.PermFilePrivate, false), perm)

	content, perm = readRuntimeFile(t, fileSvc, client.operatorKeyPath())
	assert.Equal(t, "OPERATOR-KEY", content)
	assert.Equal(t, testutil.FileMode(constants.PermFilePrivate, false), perm, "private keys must be 0600")

	content, perm = readRuntimeFile(t, fileSvc, client.cliCertPath())
	assert.Equal(t, "CLI-CERT\nCLI-CHAIN", content)
	assert.Equal(t, testutil.FileMode(constants.PermFilePrivate, false), perm)

	content, perm = readRuntimeFile(t, fileSvc, client.cliKeyPath())
	assert.Equal(t, "CLI-KEY", content)
	assert.Equal(t, testutil.FileMode(constants.PermFilePrivate, false), perm)

	content, perm = readRuntimeFile(t, fileSvc, client.trustBundlePath())
	assert.Equal(t, "TRUST-BUNDLE", content)
	assert.Equal(t, testutil.FileMode(constants.PermFilePublic, false), perm, "the trust bundle is public")

	signerPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrustedSigners, "actuator-key-1"+constants.PublicKeySuffix)
	content, perm = readRuntimeFile(t, fileSvc, signerPath)
	assert.Equal(t, "ACTUATOR-PUB", content)
	assert.Equal(t, testutil.FileMode(constants.PermFilePrivate, false), perm)
}

func TestWriteCredentialsRetainsPinnedRecoveryTrust(t *testing.T) {
	client, fileSvc := newEnrollClient(t, "http://gw:8080")
	require.NoError(t, client.atomicWrite(t.Context(), client.trustBundlePath(), []byte("PINNED-LOCAL-CA"), constants.PermFilePublic))
	creds := &models.PlatformEnrollmentOperatorCredentials{OperatorCert: "OPERATOR-CERT", CLICert: "CLI-CERT", HubTrustBundle: "UNEXPECTED-CA"}
	require.NoError(t, client.writeCredentials(creds, "OPERATOR-KEY", "CLI-KEY"))
	content, _ := readRuntimeFile(t, fileSvc, client.trustBundlePath())
	assert.Equal(t, "PINNED-LOCAL-CA", content)
}

func TestWriteCredentials_OmitsOptionalMaterialThatWasNotIssued(t *testing.T) {
	tests := []struct {
		name  string
		creds *models.PlatformEnrollmentOperatorCredentials
	}{
		{"no trust bundle and no actuator key", &models.PlatformEnrollmentOperatorCredentials{OperatorCert: "OC", CLICert: "CC"}},
		{"actuator key id without key material", &models.PlatformEnrollmentOperatorCredentials{OperatorCert: "OC", CLICert: "CC", ActuatorKeyID: "k1"}},
		{"actuator key material without key id", &models.PlatformEnrollmentOperatorCredentials{OperatorCert: "OC", CLICert: "CC", ActuatorPubKey: "PUB"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, fileSvc := newEnrollClient(t, "http://gw:8080")

			require.NoError(t, client.writeCredentials(tt.creds, "OK", "CK"))

			content, _ := readRuntimeFile(t, fileSvc, client.operatorCertPath())
			assert.Equal(t, "OC", content, "no chain means the leaf certificate alone")
			assert.NoFileExists(t, fileSvc.Resolve(client.trustBundlePath()))
			signers := fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirTrustedSigners, "k1"+constants.PublicKeySuffix))
			assert.NoFileExists(t, signers)
		})
	}
}

// ---------------------------------------------------------------------------
// Enroll validation and failure branches
// ---------------------------------------------------------------------------

// csrPublicKey extracts the ECDSA public key a CSR PEM commits to.
func csrPublicKey(t *testing.T, csrPEM string) *ecdsa.PublicKey {
	t.Helper()
	block, _ := pem.Decode([]byte(csrPEM))
	require.NotNil(t, block)
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	key, ok := csr.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok, "CSR must carry an ECDSA key")
	return key
}
