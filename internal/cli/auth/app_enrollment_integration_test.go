// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package auth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func startTLSEnrollServer(t *testing.T, cfg *config.Config, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	// Generate a test CA.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	// Generate server cert signed by CA, valid for localhost / 127.0.0.1.
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serverSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	serverTemplate := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	require.NoError(t, err)

	serverTLSCert := tls.Certificate{Certificate: [][]byte{serverDER}, PrivateKey: serverKey}

	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{serverTLSCert}}
	server.StartTLS()
	t.Cleanup(server.Close)

	// Write CA cert as trust bundle so callers can verify the server.
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	caPath := filepath.Join(cfg.RuntimeDir, "test-ca.pem")
	require.NoError(t, os.WriteFile(caPath, caPEM, constants.PermFilePrivate))
	cfg.Paths.Infra.CACertPath = caPath // absolute — CustomTrustBundlePath() returns it directly
	cfg.Paths.Host = server.URL         // full URL — OperatorHTTPURL() returns it directly

	return server
}

func TestAppPlatformEnrollmentClient_FullFlow(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	appName := "test-app"

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	var (
		requestReceived  int32
		statusCount      int32
		completeReceived int32
		issuedCertPEM    string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case constants.APIPaths.AuthPlatformEnrollmentRequest:
			atomic.AddInt32(&requestReceived, 1)
			assert.Equal(t, http.MethodPost, r.Method)
			var createReq models.PlatformEnrollmentCreateRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&createReq))
			assert.Equal(t, models.PlatformComponentApplication, createReq.ComponentKind)
			assert.Equal(t, appName, createReq.AppName)
			assert.NotEmpty(t, createReq.App.CSRPEM)

			block, _ := pem.Decode([]byte(createReq.App.CSRPEM))
			require.NotNil(t, block)
			csr, err := x509.ParseCertificateRequest(block.Bytes)
			require.NoError(t, err)

			uri, err := url.Parse("spiffe://g8e.local/app/" + appName)
			require.NoError(t, err)
			leafTemplate := x509.Certificate{
				SerialNumber: big.NewInt(2),
				Subject:      pkix.Name{CommonName: "g8e-app-" + appName},
				NotBefore:    time.Now().Add(-time.Hour),
				NotAfter:     time.Now().Add(24 * time.Hour),
				KeyUsage:     x509.KeyUsageDigitalSignature,
				ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
				URIs:         []*url.URL{uri},
			}
			leafDER, err := x509.CreateCertificate(rand.Reader, &leafTemplate, caCert, csr.PublicKey, caKey)
			require.NoError(t, err)
			issuedCertPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}))

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(models.PlatformEnrollmentCreateResponse{
				RequestID:     "req-12345",
				Token:         "tok-secret-abc",
				ComponentKind: models.PlatformComponentApplication,
				ComponentName: "g8e-app-" + appName,
				ApprovalURL:   "http://localhost/approve/req-12345",
				ExpiresAt:     time.Now().Add(10 * time.Minute),
			})

		case constants.APIPaths.AuthPlatformEnrollmentStatus:
			count := atomic.AddInt32(&statusCount, 1)
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "tok-secret-abc", r.URL.Query().Get("token"))

			state := models.PlatformEnrollmentStatePending
			if count >= 2 {
				state = models.PlatformEnrollmentStateApproved
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(models.PlatformEnrollmentStatusResponse{
				RequestID:     "req-12345",
				ComponentKind: models.PlatformComponentApplication,
				State:         state,
				ExpiresAt:     time.Now().Add(10 * time.Minute),
			})

		case constants.APIPaths.AuthPlatformEnrollmentComplete:
			atomic.AddInt32(&completeReceived, 1)
			assert.Equal(t, http.MethodPost, r.Method)
			var compReq models.PlatformEnrollmentCompleteRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&compReq))
			assert.Equal(t, "tok-secret-abc", compReq.Token)
			assert.NotEmpty(t, compReq.Proofs.App)

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(models.PlatformEnrollmentCompleteResponse{
				RequestID:     "req-12345",
				ComponentKind: models.PlatformComponentApplication,
				App: &models.PlatformEnrollmentAppCredentials{
					AppID:     "spiffe://g8e.local/app/" + appName,
					AppCert:   issuedCertPEM,
					PolicyID:  "pol-default",
					ExpiresAt: time.Now().Add(24 * time.Hour),
				},
			})

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewAppPlatformEnrollmentClient(appName, fileSvc, cfg, nil, AppEnrollmentOptions{
		GatewayHTTPURL: server.URL,
	})
	require.NoError(t, err)

	outBuf := &bytes.Buffer{}
	resp, err := client.Enroll(context.Background(), outBuf)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "spiffe://g8e.local/app/"+appName, resp.App.AppID)

	assert.Equal(t, int32(1), atomic.LoadInt32(&requestReceived))
	assert.GreaterOrEqual(t, atomic.LoadInt32(&statusCount), int32(2))
	assert.Equal(t, int32(1), atomic.LoadInt32(&completeReceived))

	// Verify credentials written to disk
	assert.True(t, client.HasValidIdentity())
	cert, err := client.LoadIdentity()
	require.NoError(t, err)
	assert.NotNil(t, cert)

	// Verify pending state was cleared
	pendingExists, _ := fileSvc.FileExists(context.Background(), client.pendingStatePath())
	assert.False(t, pendingExists)
}
