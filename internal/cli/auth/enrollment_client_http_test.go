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
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

const testSystemFingerprint = "fp-test-host"

var errTestFingerprint = errors.New("fingerprint unavailable")

func fixedFingerprint() (string, error) { return testSystemFingerprint, nil }

func writeJSONResponse(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	require.NoError(t, json.NewEncoder(w).Encode(v))
}

// discoveryStub serves the plain-HTTP discovery surface the unauthenticated
// enrollment calls use. A nil handler for a path answers 404.
type discoveryStub struct {
	server *httptest.Server
	mu     sync.Mutex
	hits   map[string]int
}

func (s *discoveryStub) hitCount(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

func newDiscoveryClient(t *testing.T) (*EnrollmentClient, *config.Config) {
	t.Helper()
	_, cfg := newAuthTestEnv(t)
	return NewEnrollmentClient(cfg, fixedFingerprint), cfg
}

func bootstrapResponseFrom(a EnrollmentArtifacts) models.BootstrapResponse {
	return models.BootstrapResponse{
		Success:           true,
		CLISessionID:      a.CLISessionID,
		UserID:            a.UserID,
		OperatorSessionID: "op-session-1",
		OperatorID:        "op-1",
		CLICert:           a.CLICertPEM,
		CLICertChain:      a.CLICertChainPEM,
		HubTrustBundle:    a.TrustBundlePEM,
	}
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNewEnrollmentClient_DefaultsFingerprintGeneratorWhenNotInjected(t *testing.T) {
	_, cfg := newAuthTestEnv(t)

	defaulted := NewEnrollmentClient(cfg, nil)
	injected := NewEnrollmentClient(cfg, fixedFingerprint)

	assert.NotNil(t, defaulted.systemFingerprint, "a nil generator must fall back to the production generator")
	fp, err := injected.systemFingerprint()
	require.NoError(t, err)
	assert.Equal(t, testSystemFingerprint, fp)
	assert.Equal(t, httpTimeout, injected.httpClient.Timeout)
}

// ---------------------------------------------------------------------------
// Bootstrap
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Recovery
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Bootstrap status and CA discovery
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Local helpers
// ---------------------------------------------------------------------------

func TestSignRecoveryProof_RejectsMissingInputsAndProducesVerifiableSignature(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	sig, err := signRecoveryProof(nil, "rec-1")
	require.ErrorIs(t, err, constants.ErrCLIRecoveryProofInvalid)
	assert.Empty(t, sig)

	sig, err = signRecoveryProof(key, "")
	require.ErrorIs(t, err, constants.ErrCLIRecoveryProofInvalid)
	assert.Empty(t, sig)

	sig, err = signRecoveryProof(key, "rec-1")
	require.NoError(t, err)
	raw, err := base64.StdEncoding.DecodeString(sig)
	require.NoError(t, err)
	assert.True(t, ecdsa.VerifyASN1(&key.PublicKey, []byte("rec-1"), raw))
	assert.False(t, ecdsa.VerifyASN1(&key.PublicKey, []byte("rec-2"), raw), "the signature is bound to this request id")
}

func TestParseCertPEMBytes_RejectsNonCertificates(t *testing.T) {
	valid := testutil.GenerateTestCA(t, "parse-test")

	cert, err := parseCertPEMBytes([]byte(valid))
	require.NoError(t, err)
	assert.Equal(t, "parse-test", cert.Subject.CommonName)

	_, err = parseCertPEMBytes([]byte("not pem"))
	require.ErrorIs(t, err, constants.ErrPEMDecodeFailed)

	_, err = parseCertPEMBytes(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")}))
	require.ErrorIs(t, err, constants.ErrInvalidPEMType)

	_, err = parseCertPEMBytes(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("junk")}))
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// mTLS surface
// ---------------------------------------------------------------------------

type mtlsRequest struct {
	Method, Path, CLISession, PeerCN string
	Body                             []byte
}

type mtlsGateway struct {
	client   *EnrollmentClient
	fileSvc  fs.RuntimeFileService
	cfg      *config.Config
	server   *httptest.Server
	requests chan mtlsRequest
}

func (g *mtlsGateway) lastRequest(t *testing.T) mtlsRequest {
	t.Helper()
	select {
	case r := <-g.requests:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("gateway received no request")
		return mtlsRequest{}
	}
}

func (g *mtlsGateway) requireNoRequest(t *testing.T) {
	t.Helper()
	select {
	case r := <-g.requests:
		t.Fatalf("unexpected request %s %s", r.Method, r.Path)
	default:
	}
}

// noIdentityClient returns a client whose runtime tree holds no CLI identity,
// so building the mTLS client must fail closed.
func noIdentityClient(t *testing.T) (*EnrollmentClient, fs.RuntimeFileService) {
	t.Helper()
	fileSvc, cfg := newAuthTestEnv(t)
	return NewEnrollmentClient(cfg, fixedFingerprint), fileSvc
}

func TestEnrollmentClient_MTLSOperations_FailClosedWithoutLocalIdentity(t *testing.T) {
	ops := map[string]func(c *EnrollmentClient, fileSvc fs.RuntimeFileService) error{
		"Rotate": func(c *EnrollmentClient, f fs.RuntimeFileService) error {
			_, err := c.Rotate(t.Context(), f, "csr", nil, "")
			return err
		},
		"Refresh": func(c *EnrollmentClient, f fs.RuntimeFileService) error {
			_, err := c.Refresh(t.Context(), f)
			return err
		},
		"Bind": func(c *EnrollmentClient, f fs.RuntimeFileService) error {
			_, err := c.Bind(t.Context(), f, []string{"s1"})
			return err
		},
		"Unbind": func(c *EnrollmentClient, f fs.RuntimeFileService) error {
			_, err := c.Unbind(t.Context(), f)
			return err
		},
		"Logout": func(c *EnrollmentClient, f fs.RuntimeFileService) error {
			_, err := c.Logout(t.Context(), f, constants.LogoutScopeAll)
			return err
		},
		"SessionInfo": func(c *EnrollmentClient, f fs.RuntimeFileService) error {
			_, err := c.SessionInfo(t.Context(), f)
			return err
		},
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			client, fileSvc := noIdentityClient(t)

			require.ErrorIs(t, op(client, fileSvc), constants.ErrFailedToLoadClientCertificate)
		})
	}

	t.Run("ProbeCLISession", func(t *testing.T) {
		client, fileSvc := noIdentityClient(t)

		require.ErrorIs(t, client.ProbeCLISession(t.Context(), fileSvc), constants.ErrHTTPRequestExecuteFailed)
	})
}
