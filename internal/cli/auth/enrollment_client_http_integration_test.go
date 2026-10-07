// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func newDiscoveryStub(t *testing.T, routes map[string]http.HandlerFunc) *discoveryStub {
	t.Helper()
	stub := &discoveryStub{hits: map[string]int{}}
	mux := http.NewServeMux()
	for path, handler := range routes {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			stub.mu.Lock()
			stub.hits[r.URL.Path]++
			stub.mu.Unlock()
			handler(w, r)
		})
	}
	stub.server = httptest.NewServer(mux)
	t.Cleanup(stub.server.Close)
	return stub
}

func TestEnrollmentClient_Bootstrap_ReturnsValidatedArtifactsAndSendsTypedRequest(t *testing.T) {
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	var got models.BootstrapRequest
	var gotContentType string
	stub := newDiscoveryStub(t, map[string]http.HandlerFunc{
		constants.APIPaths.AuthBootstrap: func(w http.ResponseWriter, r *http.Request) {
			gotContentType = r.Header.Get(constants.HeaderContentType)
			require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
			writeJSONResponse(t, w, http.StatusOK, bootstrapResponseFrom(artifacts))
		},
	})
	client, _ := newDiscoveryClient(t)

	result, err := client.Bootstrap(t.Context(), "cli-csr-pem", artifacts.CLIKey, "", stub.server.URL)

	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceBootstrap, result.Source)
	assert.Equal(t, artifacts.CLISessionID, result.CLISessionID)
	assert.Equal(t, artifacts.UserID, result.UserID)
	assert.Equal(t, "op-session-1", result.OperatorSessionID)
	assert.Equal(t, "op-1", result.OperatorID)
	assert.Equal(t, artifacts.CLICertPEM, result.CLICertPEM)
	assert.Same(t, artifacts.CLIKey, result.CLIKey, "the caller's staged key is carried into the artifacts")
	assert.Equal(t, constants.HeaderValueApplicationJSON, gotContentType)
	assert.Equal(t, "cli-csr-pem", got.CLICSR)
	assert.Equal(t, testSystemFingerprint, got.SystemFingerprint)
}

func TestEnrollmentClient_Bootstrap_UsesConfiguredDiscoveryURLWhenNoOverrideIsGiven(t *testing.T) {
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	stub := newDiscoveryStub(t, map[string]http.HandlerFunc{
		constants.APIPaths.AuthBootstrap: func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, bootstrapResponseFrom(artifacts))
		},
	})
	client, cfg := newDiscoveryClient(t)
	cfg.Paths.Host = stub.server.URL

	_, err := client.Bootstrap(t.Context(), "csr", artifacts.CLIKey, "", "")

	require.NoError(t, err)
	assert.Equal(t, 1, stub.hitCount(constants.APIPaths.AuthBootstrap))
}

func TestEnrollmentClient_Bootstrap_RejectsInvalidGatewayAnswers(t *testing.T) {
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	good := bootstrapResponseFrom(artifacts)
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tests := []struct {
		name          string
		handler       http.HandlerFunc
		key           *ecdsa.PrivateKey
		caFingerprint string
		wantErr       error
	}{
		{
			name:    "server error",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			key:     artifacts.CLIKey,
			wantErr: constants.ErrHTTPStatusError,
		},
		{
			name:    "malformed JSON",
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "{nope") },
			key:     artifacts.CLIKey,
			wantErr: constants.ErrInvalidJSONResponse,
		},
		{
			name: "gateway reports failure",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSONResponse(t, w, http.StatusOK, models.BootstrapResponse{Success: false})
			},
			key:     artifacts.CLIKey,
			wantErr: constants.ErrEnrollmentFailed,
		},
		{
			name: "response without an operator binding",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				r := good
				r.OperatorSessionID, r.OperatorID = "", ""
				writeJSONResponse(t, w, http.StatusOK, r)
			},
			key:     artifacts.CLIKey,
			wantErr: constants.ErrMissingRequiredField,
		},
		{
			name: "response without a trust bundle",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				r := good
				r.HubTrustBundle = ""
				writeJSONResponse(t, w, http.StatusOK, r)
			},
			key:     artifacts.CLIKey,
			wantErr: constants.ErrEmptyTrustBundle,
		},
		{
			name: "issued certificate does not match the staged key",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSONResponse(t, w, http.StatusOK, good)
			},
			key:     otherKey,
			wantErr: constants.ErrValidationFailed,
		},
		{
			name: "trust bundle fails the caller's fingerprint pin",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSONResponse(t, w, http.StatusOK, good)
			},
			key:           artifacts.CLIKey,
			caFingerprint: "00" + "11223344556677889900112233445566778899001122334455667788990011",
			wantErr:       constants.ErrValidationFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newDiscoveryStub(t, map[string]http.HandlerFunc{constants.APIPaths.AuthBootstrap: tt.handler})
			client, _ := newDiscoveryClient(t)

			result, err := client.Bootstrap(t.Context(), "csr", tt.key, tt.caFingerprint, stub.server.URL)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, EnrollmentArtifacts{}, result, "a failed bootstrap must return no artifacts")
		})
	}
}

func TestEnrollmentClient_Bootstrap_FailsBeforeNetworkWhenFingerprintUnavailable(t *testing.T) {
	stub := newDiscoveryStub(t, map[string]http.HandlerFunc{
		constants.APIPaths.AuthBootstrap: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
	})
	_, cfg := newAuthTestEnv(t)
	client := NewEnrollmentClient(cfg, func() (string, error) { return "", errTestFingerprint })

	_, err := client.Bootstrap(t.Context(), "csr", nil, "", stub.server.URL)

	require.ErrorIs(t, err, errTestFingerprint)
	assert.Zero(t, stub.hitCount(constants.APIPaths.AuthBootstrap), "no request may be sent without a system fingerprint")
}

func TestEnrollmentClient_Bootstrap_ReportsUnreachableGateway(t *testing.T) {
	stub := newDiscoveryStub(t, nil)
	url := stub.server.URL
	stub.server.Close()
	client, _ := newDiscoveryClient(t)

	_, err := client.Bootstrap(t.Context(), "csr", nil, "", url)

	require.ErrorIs(t, err, constants.ErrHTTPRequestExecuteFailed)
}

func TestEnrollmentClient_CreateRecoveryRequest_ReturnsTokenAndApprovalDetails(t *testing.T) {
	expires := time.Now().Add(5 * time.Minute).UTC().Truncate(time.Second)
	var got models.CLIRecoveryRequestRequest
	stub := newDiscoveryStub(t, map[string]http.HandlerFunc{
		constants.APIPaths.AuthCLIRecoveryRequest: func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
			writeJSONResponse(t, w, http.StatusOK, models.CLIRecoveryRequestResponse{
				Success: true, RequestID: "rec-1", Token: "tok-1", ApprovalURL: "https://gw/console/#rec", ExpiresAt: expires,
			})
		},
	})
	client, _ := newDiscoveryClient(t)

	requestID, token, approvalURL, expiresAt, err := client.CreateRecoveryRequest(t.Context(), "cli-csr", stub.server.URL)

	require.NoError(t, err)
	assert.Equal(t, "rec-1", requestID)
	assert.Equal(t, "tok-1", token)
	assert.Equal(t, "https://gw/console/#rec", approvalURL)
	assert.True(t, expires.Equal(expiresAt))
	assert.Equal(t, "cli-csr", got.CLICSRPEM)
	assert.Equal(t, testSystemFingerprint, got.SystemFingerprint)
}

func TestEnrollmentClient_CreateRecoveryRequest_RejectsInvalidAnswers(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{"gateway error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }, constants.ErrHTTPStatusError},
		{"gateway reports failure", func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLIRecoveryRequestResponse{Success: false})
		}, constants.ErrEnrollmentFailed},
		{"missing token", func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLIRecoveryRequestResponse{Success: true, RequestID: "r"})
		}, constants.ErrMissingRequiredField},
		{"missing request id", func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLIRecoveryRequestResponse{Success: true, Token: "t"})
		}, constants.ErrMissingRequiredField},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newDiscoveryStub(t, map[string]http.HandlerFunc{constants.APIPaths.AuthCLIRecoveryRequest: tt.handler})
			client, _ := newDiscoveryClient(t)

			requestID, token, approvalURL, expiresAt, err := client.CreateRecoveryRequest(t.Context(), "csr", stub.server.URL)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, requestID)
			assert.Empty(t, token)
			assert.Empty(t, approvalURL)
			assert.True(t, expiresAt.IsZero())
		})
	}

	t.Run("fingerprint unavailable", func(t *testing.T) {
		_, cfg := newAuthTestEnv(t)
		client := NewEnrollmentClient(cfg, func() (string, error) { return "", errTestFingerprint })

		_, _, _, _, err := client.CreateRecoveryRequest(t.Context(), "csr", "http://unused")

		require.ErrorIs(t, err, errTestFingerprint)
	})
}

func TestEnrollmentClient_RecoveryStatus(t *testing.T) {
	t.Run("returns the lifecycle state and sends the token", func(t *testing.T) {
		var gotToken string
		stub := newDiscoveryStub(t, map[string]http.HandlerFunc{
			constants.APIPaths.AuthCLIRecoveryStatus: func(w http.ResponseWriter, r *http.Request) {
				gotToken = r.URL.Query().Get("token")
				writeJSONResponse(t, w, http.StatusOK, models.CLIRecoveryStatusResponse{Success: true, State: models.CLIRecoveryStateApproved})
			},
		})
		client, _ := newDiscoveryClient(t)

		state, err := client.RecoveryStatus(t.Context(), "tok-abc_123-xyz", stub.server.URL)

		require.NoError(t, err)
		assert.Equal(t, models.CLIRecoveryStateApproved, state)
		assert.Equal(t, "tok-abc_123-xyz", gotToken)
	})

	failures := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{"unknown token", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, constants.ErrHTTPStatusError},
		{"malformed body", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "{nope") }, constants.ErrInvalidJSONResponse},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			stub := newDiscoveryStub(t, map[string]http.HandlerFunc{constants.APIPaths.AuthCLIRecoveryStatus: tt.handler})
			client, _ := newDiscoveryClient(t)

			state, err := client.RecoveryStatus(t.Context(), "tok", stub.server.URL)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, state)
		})
	}

	t.Run("unreachable gateway", func(t *testing.T) {
		stub := newDiscoveryStub(t, nil)
		url := stub.server.URL
		stub.server.Close()
		client, _ := newDiscoveryClient(t)

		_, err := client.RecoveryStatus(t.Context(), "tok", url)

		require.ErrorIs(t, err, constants.ErrHTTPRequestExecuteFailed)
	})
}

func TestEnrollmentClient_CompleteRecovery_ProvesPossessionOfTheCSRKey(t *testing.T) {
	artifacts := buildTestArtifacts(t, EnrollmentSourceRecovery)
	var got models.CLIRecoveryCompleteRequest
	stub := newDiscoveryStub(t, map[string]http.HandlerFunc{
		constants.APIPaths.AuthCLIRecoveryComplete: func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
			writeJSONResponse(t, w, http.StatusOK, models.CLIRecoveryCompleteResponse{
				Success: true, CLISessionID: artifacts.CLISessionID, UserID: artifacts.UserID,
				OperatorSessionID: "op-session-2", OperatorID: "op-2",
				CLICert: artifacts.CLICertPEM, HubTrustBundle: artifacts.TrustBundlePEM,
			})
		},
	})
	client, _ := newDiscoveryClient(t)

	result, err := client.CompleteRecovery(t.Context(), "rec-77", "tok-77", "csr", artifacts.CLIKey, "", stub.server.URL)

	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceRecovery, result.Source)
	assert.Equal(t, "op-session-2", result.OperatorSessionID)
	assert.Same(t, artifacts.CLIKey, result.CLIKey)
	assert.Equal(t, "tok-77", got.Token)
	sig, err := base64.StdEncoding.DecodeString(got.Signature)
	require.NoError(t, err)
	assert.True(t, ecdsa.VerifyASN1(&artifacts.CLIKey.PublicKey, []byte("rec-77"), sig),
		"the proof must be a signature over the request id by the key that produced the CSR")
}

func TestEnrollmentClient_CompleteRecovery_RejectsMissingProofInputsWithoutContactingGateway(t *testing.T) {
	artifacts := buildTestArtifacts(t, EnrollmentSourceRecovery)
	tests := []struct {
		name      string
		requestID string
		key       *ecdsa.PrivateKey
	}{
		{"no key", "rec-1", nil},
		{"no request id", "", artifacts.CLIKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newDiscoveryStub(t, map[string]http.HandlerFunc{
				constants.APIPaths.AuthCLIRecoveryComplete: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
			})
			client, _ := newDiscoveryClient(t)

			_, err := client.CompleteRecovery(t.Context(), tt.requestID, "tok", "csr", tt.key, "", stub.server.URL)

			require.ErrorIs(t, err, constants.ErrCLIRecoveryProofInvalid)
			assert.Zero(t, stub.hitCount(constants.APIPaths.AuthCLIRecoveryComplete))
		})
	}
}

func TestEnrollmentClient_CompleteRecovery_RejectsInvalidGatewayAnswers(t *testing.T) {
	artifacts := buildTestArtifacts(t, EnrollmentSourceRecovery)
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{"gateway error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }, constants.ErrHTTPStatusError},
		{"gateway reports failure", func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLIRecoveryCompleteResponse{Success: false})
		}, constants.ErrEnrollmentFailed},
		{"response without operator binding", func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLIRecoveryCompleteResponse{
				Success: true, CLISessionID: artifacts.CLISessionID, UserID: artifacts.UserID,
				CLICert: artifacts.CLICertPEM, HubTrustBundle: artifacts.TrustBundlePEM,
			})
		}, constants.ErrMissingRequiredField},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newDiscoveryStub(t, map[string]http.HandlerFunc{constants.APIPaths.AuthCLIRecoveryComplete: tt.handler})
			client, _ := newDiscoveryClient(t)

			result, err := client.CompleteRecovery(t.Context(), "rec", "tok", "csr", artifacts.CLIKey, "", stub.server.URL)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, EnrollmentArtifacts{}, result)
		})
	}
}

func TestEnrollmentClient_CheckBootstrapStatus(t *testing.T) {
	for _, bootstrapped := range []bool{true, false} {
		t.Run(map[bool]string{true: "bootstrapped", false: "not bootstrapped"}[bootstrapped], func(t *testing.T) {
			stub := newDiscoveryStub(t, map[string]http.HandlerFunc{
				constants.APIPaths.AuthBootstrapStatus: func(w http.ResponseWriter, _ *http.Request) {
					writeJSONResponse(t, w, http.StatusOK, models.BootstrapStatusResponse{Bootstrapped: bootstrapped})
				},
			})
			client, _ := newDiscoveryClient(t)

			got, err := client.CheckBootstrapStatus(t.Context(), stub.server.URL)

			require.NoError(t, err)
			assert.Equal(t, bootstrapped, got)
		})
	}

	failures := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{"gateway error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }, constants.ErrHTTPStatusError},
		{"malformed body", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "{nope") }, constants.ErrInvalidJSONResponse},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			stub := newDiscoveryStub(t, map[string]http.HandlerFunc{constants.APIPaths.AuthBootstrapStatus: tt.handler})
			client, _ := newDiscoveryClient(t)

			got, err := client.CheckBootstrapStatus(t.Context(), stub.server.URL)

			require.ErrorIs(t, err, tt.wantErr)
			assert.False(t, got, "an unreadable answer must never be reported as bootstrapped")
		})
	}

	t.Run("unreachable gateway reads as service unavailable", func(t *testing.T) {
		stub := newDiscoveryStub(t, nil)
		url := stub.server.URL
		stub.server.Close()
		client, _ := newDiscoveryClient(t)

		got, err := client.CheckBootstrapStatus(t.Context(), url)

		require.ErrorIs(t, err, constants.ErrServiceUnavailable)
		assert.False(t, got)
	})
}

func TestEnrollmentClient_DiscoverGatewayCA(t *testing.T) {
	artifacts := buildTestArtifacts(t, EnrollmentSourceBootstrap)
	block, _ := pem.Decode([]byte(artifacts.TrustBundlePEM))
	require.NotNil(t, block)
	root, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	t.Run("returns the live bundle and the primary root fingerprint", func(t *testing.T) {
		stub := newDiscoveryStub(t, map[string]http.HandlerFunc{
			constants.WellKnownPKICABundle: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, artifacts.TrustBundlePEM)
			},
		})
		client, cfg := newDiscoveryClient(t)
		cfg.Paths.Host = stub.server.URL

		bundle, fingerprint, err := client.DiscoverGatewayCA(t.Context())

		require.NoError(t, err)
		assert.Equal(t, artifacts.TrustBundlePEM, string(bundle))
		assert.Equal(t, platform.CertFingerprint(root), fingerprint)
	})

	t.Run("an unavailable gateway yields no bundle", func(t *testing.T) {
		stub := newDiscoveryStub(t, nil)
		client, cfg := newDiscoveryClient(t)
		cfg.Paths.Host = stub.server.URL

		bundle, fingerprint, err := client.DiscoverGatewayCA(t.Context())

		require.Error(t, err)
		assert.Nil(t, bundle)
		assert.Empty(t, fingerprint)
	})

	t.Run("a bundle that is not a usable trust anchor is rejected", func(t *testing.T) {
		stub := newDiscoveryStub(t, map[string]http.HandlerFunc{
			constants.WellKnownPKICABundle: func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "not a bundle") },
		})
		client, cfg := newDiscoveryClient(t)
		cfg.Paths.Host = stub.server.URL

		bundle, fingerprint, err := client.DiscoverGatewayCA(t.Context())

		require.Error(t, err)
		assert.Nil(t, bundle)
		assert.Empty(t, fingerprint)
	})
}

// newMTLSGateway starts a TLS server that requires a client certificate
// issued by the same CA the CLI trusts, and writes a complete local CLI
// identity (cert, key, trust bundle, credentials) for the client to use.
func newMTLSGateway(t *testing.T, respond http.HandlerFunc) *mtlsGateway {
	t.Helper()
	fileSvc, cfg := newAuthTestEnv(t)
	caKey, caCert := generateTestCAWithKey(t, "mtls-test-ca")

	cliCertPEM, cliKey := testutil.GenerateTestSignedCert(t, "g8e-cli-mtls", caCert, caKey)
	certRel, err := fileSvc.RelFromAbs(cfg.CLICertFile())
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(cliKey)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(t.Context(), certRel, []byte(cliCertPEM), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(t.Context(), keyRel, []byte(pemEncode("EC PRIVATE KEY", keyDER)), constants.PermFilePrivate))
	require.NoError(t, WriteTrustBundleFS(fileSvc, cfg, []byte(pemEncode("CERTIFICATE", caCert.Raw)), constants.PermFilePublic))
	require.NoError(t, SaveCredentials(fileSvc, cfg, &Credentials{UserID: "user-1", CLISessionID: "cli-sess-1"}))

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 64))
	require.NoError(t, err)
	serverDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "mtls-test-server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}, caCert, &serverKey.PublicKey, caKey)
	require.NoError(t, err)

	clientCAs := x509.NewCertPool()
	clientCAs.AddCert(caCert)
	gw := &mtlsGateway{fileSvc: fileSvc, cfg: cfg, requests: make(chan mtlsRequest, 8)}
	gw.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec := mtlsRequest{Method: r.Method, Path: r.URL.Path, CLISession: r.Header.Get(constants.HeaderCLISessionID), Body: body}
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			rec.PeerCN = r.TLS.PeerCertificates[0].Subject.CommonName
		}
		gw.requests <- rec
		respond(w, r)
	}))
	gw.server.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{serverDER}, PrivateKey: serverKey}},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
		MinVersion:   tls.VersionTLS13,
	}
	gw.server.StartTLS()
	t.Cleanup(gw.server.Close)

	cfg.Paths.Host = gw.server.URL
	gw.client = NewEnrollmentClient(cfg, fixedFingerprint)
	return gw
}

func TestEnrollmentClient_Rotate_PresentsClientIdentityAndValidatesTheNewCertificate(t *testing.T) {
	artifacts := buildTestArtifacts(t, EnrollmentSourceRotation)
	gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSONResponse(t, w, http.StatusOK, models.CLIRotationResponse{
			Success: true, CLISessionID: artifacts.CLISessionID, UserID: artifacts.UserID,
			CLICert: artifacts.CLICertPEM, HubTrustBundle: artifacts.TrustBundlePEM,
		})
	})

	result, err := gw.client.Rotate(t.Context(), gw.fileSvc, "new-csr", artifacts.CLIKey, "")

	require.NoError(t, err)
	assert.Equal(t, EnrollmentSourceRotation, result.Source)
	assert.Equal(t, artifacts.CLISessionID, result.CLISessionID)
	assert.Same(t, artifacts.CLIKey, result.CLIKey)
	req := gw.lastRequest(t)
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Equal(t, constants.APIPaths.AuthCLIRotate, req.Path)
	assert.Equal(t, "g8e-cli-mtls", req.PeerCN, "the gateway must see the local CLI certificate")
	assert.Equal(t, "cli-sess-1", req.CLISession, "the persisted CLI session id travels as a header")
	var body models.CLIRotationRequest
	require.NoError(t, json.Unmarshal(req.Body, &body))
	assert.Equal(t, "new-csr", body.CLICSRPEM)
}

func TestEnrollmentClient_Rotate_RejectsInvalidAnswers(t *testing.T) {
	artifacts := buildTestArtifacts(t, EnrollmentSourceRotation)
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tests := []struct {
		name    string
		handler http.HandlerFunc
		key     *ecdsa.PrivateKey
		wantErr error
	}{
		{"gateway error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }, artifacts.CLIKey, constants.ErrHTTPStatusError},
		{"gateway reports failure", func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLIRotationResponse{Success: false})
		}, artifacts.CLIKey, constants.ErrEnrollmentFailed},
		{"rotated certificate does not match the staged key", func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLIRotationResponse{
				Success: true, CLISessionID: artifacts.CLISessionID, UserID: artifacts.UserID,
				CLICert: artifacts.CLICertPEM, HubTrustBundle: artifacts.TrustBundlePEM,
			})
		}, otherKey, constants.ErrValidationFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := newMTLSGateway(t, tt.handler)

			result, err := gw.client.Rotate(t.Context(), gw.fileSvc, "csr", tt.key, "")

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, EnrollmentArtifacts{}, result)
		})
	}
}

func TestEnrollmentClient_Refresh(t *testing.T) {
	t.Run("returns the authoritative session and operator binding", func(t *testing.T) {
		gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLIRefreshResponse{
				Success: true, CLISessionID: "cli-new", UserID: "user-1", OperatorSessionID: "op-sess", OperatorID: "op-1",
			})
		})

		got, err := gw.client.Refresh(t.Context(), gw.fileSvc)

		require.NoError(t, err)
		assert.Equal(t, CLISessionRefresh{CLISessionID: "cli-new", UserID: "user-1", OperatorSessionID: "op-sess", OperatorID: "op-1"}, got)
		req := gw.lastRequest(t)
		assert.Equal(t, constants.APIPaths.AuthCLIRefresh, req.Path)
		assert.Equal(t, "g8e-cli-mtls", req.PeerCN)
		assert.Equal(t, "cli-sess-1", req.CLISession)
	})

	failures := []struct {
		name    string
		resp    any
		status  int
		wantErr error
	}{
		{"gateway error", nil, http.StatusInternalServerError, constants.ErrHTTPStatusError},
		{"gateway reports failure", models.CLIRefreshResponse{Success: false}, http.StatusOK, constants.ErrCLIRefreshFailed},
		{"missing session id", models.CLIRefreshResponse{Success: true, UserID: "u", OperatorSessionID: "s", OperatorID: "o"}, http.StatusOK, constants.ErrMissingRequiredField},
		{"missing user id", models.CLIRefreshResponse{Success: true, CLISessionID: "c", OperatorSessionID: "s", OperatorID: "o"}, http.StatusOK, constants.ErrMissingRequiredField},
		{"missing operator session", models.CLIRefreshResponse{Success: true, CLISessionID: "c", UserID: "u", OperatorID: "o"}, http.StatusOK, constants.ErrMissingRequiredField},
		{"missing operator id", models.CLIRefreshResponse{Success: true, CLISessionID: "c", UserID: "u", OperatorSessionID: "s"}, http.StatusOK, constants.ErrMissingRequiredField},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) {
				if tt.resp == nil {
					w.WriteHeader(tt.status)
					return
				}
				writeJSONResponse(t, w, tt.status, tt.resp)
			})

			got, err := gw.client.Refresh(t.Context(), gw.fileSvc)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, CLISessionRefresh{}, got)
		})
	}
}

func TestEnrollmentClient_Bind(t *testing.T) {
	t.Run("binds every requested operator session in one call", func(t *testing.T) {
		bound := []models.CLIBoundOperator{{OperatorSessionID: "s1", OperatorID: "o1"}, {OperatorSessionID: "s2", OperatorID: "o2"}}
		gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLIBindResponse{
				Success: true, CLISessionID: "cli-new", UserID: "user-1", OperatorSessionID: "s1", OperatorID: "o1",
				AlreadyBound: true, Bound: bound,
			})
		})

		got, err := gw.client.Bind(t.Context(), gw.fileSvc, []string{"s1", "s2"})

		require.NoError(t, err)
		assert.Equal(t, CLISessionBind{
			CLISessionID: "cli-new", UserID: "user-1", OperatorSessionID: "s1", OperatorID: "o1", AlreadyBound: true, Bound: bound,
		}, got)
		req := gw.lastRequest(t)
		assert.Equal(t, constants.APIPaths.AuthCLIBind, req.Path)
		assert.Equal(t, "cli-sess-1", req.CLISession)
		var body models.CLIBindRequest
		require.NoError(t, json.Unmarshal(req.Body, &body))
		assert.Equal(t, []string{"s1", "s2"}, body.OperatorSessionIDs)
	})

	t.Run("invalid arguments are rejected before any connection", func(t *testing.T) {
		tests := map[string][]string{"no sessions": nil, "empty session id": {"s1", ""}}
		for name, ids := range tests {
			t.Run(name, func(t *testing.T) {
				gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

				got, err := gw.client.Bind(t.Context(), gw.fileSvc, ids)

				require.ErrorIs(t, err, constants.ErrGatewayOperatorSessionIDRequired)
				assert.Equal(t, CLISessionBind{}, got)
				gw.requireNoRequest(t)
			})
		}
	})

	failures := []struct {
		name    string
		resp    any
		status  int
		wantErr error
	}{
		{"gateway error", nil, http.StatusForbidden, constants.ErrHTTPStatusError},
		{"gateway reports failure", models.CLIBindResponse{Success: false}, http.StatusOK, constants.ErrCLIRefreshFailed},
		{"missing primary binding", models.CLIBindResponse{Success: true, CLISessionID: "c", UserID: "u"}, http.StatusOK, constants.ErrMissingRequiredField},
		{"gateway bound fewer sessions than requested", models.CLIBindResponse{
			Success: true, CLISessionID: "c", UserID: "u", OperatorSessionID: "s1", OperatorID: "o1",
			Bound: []models.CLIBoundOperator{{OperatorSessionID: "s1", OperatorID: "o1"}},
		}, http.StatusOK, constants.ErrCLIRefreshFailed},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) {
				if tt.resp == nil {
					w.WriteHeader(tt.status)
					return
				}
				writeJSONResponse(t, w, tt.status, tt.resp)
			})

			got, err := gw.client.Bind(t.Context(), gw.fileSvc, []string{"s1", "s2"})

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, CLISessionBind{}, got)
		})
	}
}

func TestEnrollmentClient_Unbind(t *testing.T) {
	t.Run("reports the replacement session and whether a binding existed", func(t *testing.T) {
		for _, already := range []bool{false, true} {
			t.Run(map[bool]string{false: "binding cleared", true: "already unbound"}[already], func(t *testing.T) {
				gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) {
					writeJSONResponse(t, w, http.StatusOK, models.CLIUnbindResponse{
						Success: true, CLISessionID: "cli-new", UserID: "user-1", AlreadyUnbound: already,
					})
				})

				got, err := gw.client.Unbind(t.Context(), gw.fileSvc)

				require.NoError(t, err)
				assert.Equal(t, CLISessionUnbind{CLISessionID: "cli-new", UserID: "user-1", AlreadyUnbound: already}, got)
				req := gw.lastRequest(t)
				assert.Equal(t, constants.APIPaths.AuthCLIUnbind, req.Path)
				assert.Equal(t, "cli-sess-1", req.CLISession)
			})
		}
	})

	failures := []struct {
		name    string
		resp    any
		status  int
		wantErr error
	}{
		{"gateway error", nil, http.StatusInternalServerError, constants.ErrHTTPStatusError},
		{"gateway reports failure", models.CLIUnbindResponse{Success: false}, http.StatusOK, constants.ErrCLIRefreshFailed},
		{"missing session id", models.CLIUnbindResponse{Success: true, UserID: "u"}, http.StatusOK, constants.ErrMissingRequiredField},
		{"missing user id", models.CLIUnbindResponse{Success: true, CLISessionID: "c"}, http.StatusOK, constants.ErrMissingRequiredField},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) {
				if tt.resp == nil {
					w.WriteHeader(tt.status)
					return
				}
				writeJSONResponse(t, w, tt.status, tt.resp)
			})

			got, err := gw.client.Unbind(t.Context(), gw.fileSvc)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, CLISessionUnbind{}, got)
		})
	}
}

func TestEnrollmentClient_SessionInfo(t *testing.T) {
	t.Run("returns the persisted identity binding", func(t *testing.T) {
		gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLISessionInfoResponse{
				Success: true, CLISessionID: "cli-1", UserID: "user-1", OperatorSessionID: "s1", OperatorID: "o1",
				BoundOperatorSessionIDs: []string{"s1", "s2"},
			})
		})

		got, err := gw.client.SessionInfo(t.Context(), gw.fileSvc)

		require.NoError(t, err)
		assert.Equal(t, CLISessionInfo{
			CLISessionID: "cli-1", UserID: "user-1", OperatorSessionID: "s1", OperatorID: "o1",
			BoundOperatorSessionIDs: []string{"s1", "s2"},
		}, got)
		req := gw.lastRequest(t)
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, constants.APIPaths.AuthCLISession, req.Path)
		assert.Equal(t, "g8e-cli-mtls", req.PeerCN)
		assert.Equal(t, "cli-sess-1", req.CLISession)
	})

	t.Run("an unbound session is still a valid answer", func(t *testing.T) {
		gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLISessionInfoResponse{Success: true, CLISessionID: "cli-1", UserID: "user-1"})
		})

		got, err := gw.client.SessionInfo(t.Context(), gw.fileSvc)

		require.NoError(t, err)
		assert.Empty(t, got.OperatorSessionID)
		assert.Empty(t, got.OperatorID)
	})

	failures := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
		wantMsg string
	}{
		{"gateway error carries the response body", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "session expired")
		}, constants.ErrHTTPStatusError, "session expired"},
		{"malformed body", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "{nope") }, constants.ErrInvalidJSONResponse, ""},
		{"unsuccessful answer", func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLISessionInfoResponse{Success: false, CLISessionID: "c", UserID: "u"})
		}, constants.ErrMissingRequiredField, ""},
		{"missing session id", func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLISessionInfoResponse{Success: true, UserID: "u"})
		}, constants.ErrMissingRequiredField, ""},
		{"missing user id", func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, models.CLISessionInfoResponse{Success: true, CLISessionID: "c"})
		}, constants.ErrMissingRequiredField, ""},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			gw := newMTLSGateway(t, tt.handler)

			got, err := gw.client.SessionInfo(t.Context(), gw.fileSvc)

			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantMsg != "" {
				assert.Contains(t, err.Error(), tt.wantMsg)
			}
			assert.Equal(t, CLISessionInfo{}, got)
		})
	}
}

func TestEnrollmentClient_ProbeCLISession(t *testing.T) {
	t.Run("a healthy session returns nil and presents the client identity", func(t *testing.T) {
		gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

		require.NoError(t, gw.client.ProbeCLISession(t.Context(), gw.fileSvc))

		req := gw.lastRequest(t)
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, constants.APIPaths.ApprovalsCLIList, req.Path)
		assert.Equal(t, "g8e-cli-mtls", req.PeerCN)
		assert.Equal(t, "cli-sess-1", req.CLISession)
	})

	rejections := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{"expired session", http.StatusUnauthorized, constants.ErrCLISessionExpired.Error(), constants.ErrCLISessionExpired},
		{"invalidated session", http.StatusUnauthorized, constants.ErrCLISessionInvalid.Error(), constants.ErrCLISessionInvalid},
		{"unauthorized for an unrecognised reason is treated as invalid", http.StatusUnauthorized, "nope", constants.ErrCLISessionInvalid},
		{"unexpected status", http.StatusInternalServerError, "", constants.ErrHTTPStatusError},
	}
	for _, tt := range rejections {
		t.Run(tt.name, func(t *testing.T) {
			gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})

			require.ErrorIs(t, gw.client.ProbeCLISession(t.Context(), gw.fileSvc), tt.wantErr)
		})
	}
}

func TestEnrollmentClient_MTLSOperations_ReportUnreachableGateway(t *testing.T) {
	gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	gw.server.Close()

	_, err := gw.client.Refresh(t.Context(), gw.fileSvc)
	require.ErrorIs(t, err, constants.ErrHTTPRequestExecuteFailed)

	_, err = gw.client.SessionInfo(t.Context(), gw.fileSvc)
	require.ErrorIs(t, err, constants.ErrHTTPRequestExecuteFailed)

	require.ErrorIs(t, gw.client.ProbeCLISession(t.Context(), gw.fileSvc), constants.ErrHTTPRequestExecuteFailed)
}

func TestEnrollmentClient_MTLSOperations_RejectServerNotSignedByTheLocalTrustBundle(t *testing.T) {
	gw := newMTLSGateway(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	// Replace the trust bundle with an unrelated CA: the server's certificate
	// no longer chains to anything the CLI trusts.
	otherCA := testutil.GenerateTestCA(t, "unrelated-ca")
	require.NoError(t, WriteTrustBundleFS(gw.fileSvc, gw.cfg, []byte(otherCA), constants.PermFilePublic))

	_, err := gw.client.Refresh(t.Context(), gw.fileSvc)

	require.ErrorIs(t, err, constants.ErrHTTPRequestExecuteFailed, "the CLI must not talk to a gateway its trust bundle does not vouch for")
	gw.requireNoRequest(t)
}
