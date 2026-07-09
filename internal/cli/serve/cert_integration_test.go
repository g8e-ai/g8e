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

//go:build integration

package serve

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Shared helpers (used by both enrollment and renewal integration tests)
// ---------------------------------------------------------------------------

// generateTestCAIntegration creates a self-signed CA certificate and key pair.
// Returns (caPEM, caKey, caCert).
func generateTestCAIntegration(t *testing.T) ([]byte, *ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	caCert, err := x509.ParseCertificate(derBytes)
	require.NoError(t, err)

	return caPEM, caKey, caCert
}

// signCSRIntegration parses a CSR PEM, signs it with the given CA, and returns the PEM-encoded certificate.
func signCSRIntegration(t *testing.T, csrPEM []byte, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, commonName string, notAfter time.Time) []byte {
	t.Helper()
	block, _ := pem.Decode(csrPEM)
	require.NotNil(t, block)
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, csr.PublicKey, caKey)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
}

// writeExpiringCertPair creates a cert+key pair where the cert expires within 24h,
// writes them to temp files, and returns the paths. The cert is signed by the given CA.
func writeExpiringCertPair(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) (certPath, keyPath string) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "expiring-operator"},
		NotBefore:    time.Now().Add(-23 * time.Hour),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privKey.PublicKey, caKey)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	dir := t.TempDir()
	certPath = filepath.Join(dir, constants.TestClientCrtFilename)
	keyPath = filepath.Join(dir, constants.TestClientKeyFilename)
	require.NoError(t, os.WriteFile(certPath, certPEM, 0600))
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0600))
	return certPath, keyPath
}

// mustMarshal marshals v to JSON, failing the test on error.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// ---------------------------------------------------------------------------
// PerformAutomaticEnrollment
// ---------------------------------------------------------------------------

// enrollmentTestServer creates an httptest.Server that handles the trust bundle
// and device enrollment endpoints. The trustStatus and enrollStatus control the
// HTTP status codes returned. The enrollBody is returned as-is for the enrollment
// endpoint. If enrollBody is nil, a valid success response is generated using the
// CA to sign the CSR from the request. Teardown is registered via t.Cleanup.
func enrollmentTestServer(t *testing.T, caPEM []byte, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, trustStatus, enrollStatus int, enrollBody []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case constants.WellKnownPKICABundle:
			w.WriteHeader(trustStatus)
			if trustStatus == http.StatusOK {
				_, _ = w.Write(caPEM)
			}
		case constants.APIPathAuthDeviceEnroll:
			if enrollStatus == http.StatusOK || enrollStatus == http.StatusCreated {
				if enrollBody != nil {
					w.WriteHeader(enrollStatus)
					_, _ = w.Write(enrollBody)
					return
				}
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var req models.DeviceEnrollRequest
				require.NoError(t, json.Unmarshal(body, &req))
				opCert := signCSRIntegration(t, []byte(req.CSR), caCert, caKey, "test-operator", time.Now().Add(365*24*time.Hour))
				resp := models.DeviceEnrollmentResponse{
					OperatorCert:      string(opCert),
					OperatorID:        "op-001",
					OperatorSessionID: "sess-001",
				}
				respBytes, err := json.Marshal(resp)
				require.NoError(t, err)
				w.WriteHeader(enrollStatus)
				_, _ = w.Write(respBytes)
			} else {
				w.WriteHeader(enrollStatus)
				_, _ = w.Write([]byte(`{"error":"server error"}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPerformAutomaticEnrollment(t *testing.T) {
	tests := []struct {
		name         string
		trustStatus  int
		enrollStatus int
		enrollBody   []byte
		wantErr      error
		checkSuccess bool
	}{
		{
			name:         "Success",
			trustStatus:  http.StatusOK,
			enrollStatus: http.StatusOK,
			wantErr:      nil,
			checkSuccess: true,
		},
		{
			name:         "TrustBundleFetchFailure",
			trustStatus:  http.StatusInternalServerError,
			enrollStatus: http.StatusOK,
			wantErr:      constants.ErrFailedToReadTrustBundle,
		},
		{
			name:         "EnrollmentHTTPError",
			trustStatus:  http.StatusOK,
			enrollStatus: http.StatusInternalServerError,
			wantErr:      constants.ErrHTTPStatusError,
		},
		{
			name:         "ErrorFieldInResponse",
			trustStatus:  http.StatusOK,
			enrollStatus: http.StatusOK,
			enrollBody:   mustMarshal(t, models.DeviceEnrollmentResponse{Error: "enrollment denied"}),
			wantErr:      constants.ErrEnrollmentFailed,
		},
		{
			name:         "MissingOperatorCert",
			trustStatus:  http.StatusOK,
			enrollStatus: http.StatusOK,
			enrollBody:   mustMarshal(t, models.DeviceEnrollmentResponse{OperatorID: "op-001", OperatorSessionID: "sess-001"}),
			wantErr:      constants.ErrMissingCertificate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restorePorts(t)
			require.NoError(t, paths.InitWithBase(t.TempDir()))

			caPEM, caKey, caCert := generateTestCAIntegration(t)
			srv := enrollmentTestServer(t, caPEM, caCert, caKey, tt.trustStatus, tt.enrollStatus, tt.enrollBody)

			constants.Ports.OperatorHttp = getServerPort(t, srv)

			_, err := PerformAutomaticEnrollment(context.Background(), "127.0.0.1", CertPaths{
				PkiTrustDir:       paths.Infra.PkiTrustDir,
				OperatorKeyPath:   paths.Infra.OperatorKeyPath,
				OperatorCertPath:  paths.Infra.OperatorCertPath,
				CaCertPath:        paths.Infra.CaCertPath,
				TrustedSignersDir: paths.Infra.TrustedSignersDir,
			}, testLogger())

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)

			if tt.checkSuccess {
				_, err = os.Stat(paths.Infra.OperatorKeyPath)
				assert.NoError(t, err, "operator key should be saved")
				_, err = os.Stat(paths.Infra.OperatorCertPath)
				assert.NoError(t, err, "operator cert should be saved")
				_, err = os.Stat(paths.Infra.CaCertPath)
				assert.NoError(t, err, "CA bundle should be saved")
			}
		})
	}
}

func TestPerformAutomaticEnrollment_ActuatorPublicKeySaved(t *testing.T) {
	restorePorts(t)
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	caPEM, caKey, caCert := generateTestCAIntegration(t)
	actuatorPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	actuatorPubB64 := hex.EncodeToString(actuatorPub)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case constants.WellKnownPKICABundle:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(caPEM)
		case constants.APIPathAuthDeviceEnroll:
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var req models.DeviceEnrollRequest
			require.NoError(t, json.Unmarshal(body, &req))
			opCert := signCSRIntegration(t, []byte(req.CSR), caCert, caKey, "test-operator", time.Now().Add(365*24*time.Hour))
			resp := models.DeviceEnrollmentResponse{
				OperatorCert:      string(opCert),
				OperatorID:        "op-001",
				OperatorSessionID: "sess-001",
				ActuatorKeyID:     "act-001",
				ActuatorPubKey:    actuatorPubB64,
			}
			respBytes, err := json.Marshal(resp)
			require.NoError(t, err)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(respBytes)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	constants.Ports.OperatorHttp = getServerPort(t, srv)

	sessionID, err := PerformAutomaticEnrollment(context.Background(), "127.0.0.1", CertPaths{
		PkiTrustDir:       paths.Infra.PkiTrustDir,
		OperatorKeyPath:   paths.Infra.OperatorKeyPath,
		OperatorCertPath:  paths.Infra.OperatorCertPath,
		CaCertPath:        paths.Infra.CaCertPath,
		TrustedSignersDir: paths.Infra.TrustedSignersDir,
	}, testLogger())
	require.NoError(t, err)
	assert.Equal(t, "sess-001", sessionID)

	signerPath := filepath.Join(paths.Infra.TrustedSignersDir, "act-001"+constants.PublicKeySuffix)
	_, err = os.Stat(signerPath)
	assert.NoError(t, err, "actuator public key should be saved to trusted_signers")
}

// ---------------------------------------------------------------------------
// RenewOperatorCertificate (network-based tests)
// ---------------------------------------------------------------------------

// renewalTestServer creates an httptest.Server that handles the trust bundle
// and PKI device enrollment endpoints for RenewOperatorCertificate tests.
// The trustStatus and enrollStatus control HTTP status codes.
// enrollBody overrides the enrollment response; if nil, a valid response is generated.
// trustBody overrides the trust bundle response; if nil, caPEM is used.
// Teardown is registered via t.Cleanup.
func renewalTestServer(t *testing.T, caPEM []byte, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, trustStatus, enrollStatus int, trustBody, enrollBody []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case constants.WellKnownPKICABundle:
			w.WriteHeader(trustStatus)
			if trustStatus == http.StatusOK {
				if trustBody != nil {
					_, _ = w.Write(trustBody)
				} else {
					_, _ = w.Write(caPEM)
				}
			}
		case constants.APIPathPKIDevicesEnroll:
			if enrollStatus >= http.StatusOK && enrollStatus < http.StatusMultipleChoices {
				if enrollBody != nil {
					w.WriteHeader(enrollStatus)
					_, _ = w.Write(enrollBody)
					return
				}
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var req models.DeviceEnrollRequest
				require.NoError(t, json.Unmarshal(body, &req))
				opCert := signCSRIntegration(t, []byte(req.CSR), caCert, caKey, "test-operator", time.Now().Add(365*24*time.Hour))
				cliCert := signCSRIntegration(t, []byte(req.CSR), caCert, caKey, "test-cli", time.Now().Add(365*24*time.Hour))
				resp := models.OperatorRegistrationResponse{
					OperatorCert: string(opCert),
					CLICert:      string(cliCert),
				}
				respBytes, err := json.Marshal(resp)
				require.NoError(t, err)
				w.WriteHeader(enrollStatus)
				_, _ = w.Write(respBytes)
			} else {
				w.WriteHeader(enrollStatus)
				_, _ = w.Write([]byte(`{"error":"server error"}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRenewOperatorCertificate_Network(t *testing.T) {
	tests := []struct {
		name         string
		trustStatus  int
		enrollStatus int
		trustBody    []byte
		enrollBody   []byte
		wantErr      error
		checkRenewed bool
	}{
		{
			name:         "Success",
			trustStatus:  http.StatusOK,
			enrollStatus: http.StatusOK,
			wantErr:      nil,
			checkRenewed: true,
		},
		{
			name:         "TrustBundleHTTPError",
			trustStatus:  http.StatusInternalServerError,
			enrollStatus: http.StatusOK,
			wantErr:      constants.ErrHTTPStatusError,
		},
		{
			name:         "EnrollmentHTTPError",
			trustStatus:  http.StatusOK,
			enrollStatus: http.StatusInternalServerError,
			wantErr:      constants.ErrHTTPStatusError,
		},
		{
			name:         "ErrorFieldInResponse",
			trustStatus:  http.StatusOK,
			enrollStatus: http.StatusOK,
			enrollBody:   mustMarshal(t, models.OperatorRegistrationResponse{Error: "renewal denied"}),
			wantErr:      constants.ErrEnrollmentFailed,
		},
		{
			name:         "MissingCertsInResponse",
			trustStatus:  http.StatusOK,
			enrollStatus: http.StatusOK,
			enrollBody:   mustMarshal(t, models.OperatorRegistrationResponse{OperatorCert: "", CLICert: ""}),
			wantErr:      constants.ErrMissingRequiredField,
		},
		{
			name:         "EmptyTrustBundle",
			trustStatus:  http.StatusOK,
			enrollStatus: http.StatusOK,
			trustBody:    []byte{},
			wantErr:      constants.ErrEmptyTrustBundle,
		},
		{
			name:         "InvalidTrustBundlePEM",
			trustStatus:  http.StatusOK,
			enrollStatus: http.StatusOK,
			trustBody:    []byte("not a valid PEM"),
			wantErr:      constants.ErrCAParseFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restorePorts(t)
			require.NoError(t, paths.InitWithBase(t.TempDir()))
			if tt.trustStatus == http.StatusOK && (tt.trustBody == nil || len(tt.trustBody) > 0) {
				require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.CaCertPath), 0700))
			}

			caPEM, caKey, caCert := generateTestCAIntegration(t)
			certPath, keyPath := writeExpiringCertPair(t, caCert, caKey)

			srv := renewalTestServer(t, caPEM, caCert, caKey, tt.trustStatus, tt.enrollStatus, tt.trustBody, tt.enrollBody)

			cfg := &config.Config{Endpoint: srv.URL}
			ci := certs.NewClientIdentity(tls.Certificate{})

			err := RenewOperatorCertificate(context.Background(), cfg, certPath, keyPath, ci)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)

			if tt.checkRenewed {
				savedCert, err := os.ReadFile(certPath)
				require.NoError(t, err)
				assert.Contains(t, string(savedCert), "BEGIN CERTIFICATE")

				savedKey, err := os.ReadFile(keyPath)
				require.NoError(t, err)
				assert.Contains(t, string(savedKey), "EC PRIVATE KEY")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RunClientCertRenewalLoop (renewal trigger)
// ---------------------------------------------------------------------------

func TestRunClientCertRenewalLoop_RenewalTriggerWithExpiringCert(t *testing.T) {
	restorePorts(t)
	require.NoError(t, paths.InitWithBase(t.TempDir()))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.CaCertPath), 0700))

	caPEM, caKey, caCert := generateTestCAIntegration(t)
	certPath, keyPath := writeExpiringCertPair(t, caCert, caKey)

	srv := renewalTestServer(t, caPEM, caCert, caKey, http.StatusOK, http.StatusOK, nil, nil)

	cfg := &config.Config{Endpoint: srv.URL}
	ci := certs.NewClientIdentity(tls.Certificate{})
	logger := testLogger()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		RunClientCertRenewalLoop(ctx, cfg, certPath, keyPath, logger, ci)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunClientCertRenewalLoop did not stop after context cancellation")
	}

	savedCert, err := os.ReadFile(certPath)
	require.NoError(t, err)
	assert.Contains(t, string(savedCert), "BEGIN CERTIFICATE",
		"cert should have been renewed on startup before context cancellation")
}
