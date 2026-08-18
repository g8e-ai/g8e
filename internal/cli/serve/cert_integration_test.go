// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/fs"
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
func writeExpiringCertPair(t *testing.T, fileSvc fs.RuntimeFileService, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) (certPath, keyPath string) {
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

	certRel := filepath.Join(constants.PkiDirname, constants.TestClientCrtFilename)
	keyRel := filepath.Join(constants.PkiDirname, constants.TestClientKeyFilename)
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, certPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, keyPEM, constants.PermFilePrivate))
	certPath = fileSvc.Resolve(certRel)
	keyPath = fileSvc.Resolve(keyRel)
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
			fileSvc := newTestFileSvc(t)

			caPEM, caKey, caCert := generateTestCAIntegration(t)
			srv := enrollmentTestServer(t, caPEM, caCert, caKey, tt.trustStatus, tt.enrollStatus, tt.enrollBody)

			constants.Ports.OperatorHttp = getServerPort(t, srv)

			_, _, err := PerformAutomaticEnrollment(context.Background(), "127.0.0.1", fileSvc, testLogger())

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)

			if tt.checkSuccess {
				opKeyRel := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorKey)
				opCertRel := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorCert)
				caBundleRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
				exists, err := fileSvc.FileExists(context.Background(), opKeyRel)
				assert.NoError(t, err)
				assert.True(t, exists, "operator key should be saved")
				exists, err = fileSvc.FileExists(context.Background(), opCertRel)
				assert.NoError(t, err)
				assert.True(t, exists, "operator cert should be saved")
				exists, err = fileSvc.FileExists(context.Background(), caBundleRel)
				assert.NoError(t, err)
				assert.True(t, exists, "CA bundle should be saved")
			}
		})
	}
}

func TestPerformAutomaticEnrollment_ActuatorPublicKeySaved(t *testing.T) {
	restorePorts(t)
	fileSvc := newTestFileSvc(t)

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

	sessionID, _, err := PerformAutomaticEnrollment(context.Background(), "127.0.0.1", fileSvc, testLogger())
	require.NoError(t, err)
	assert.Equal(t, "sess-001", sessionID)

	signerRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrustedSigners, "act-001"+constants.PublicKeySuffix)
	exists, err := fileSvc.FileExists(context.Background(), signerRel)
	assert.NoError(t, err)
	assert.True(t, exists, "actuator public key should be saved to trusted_signers")
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
			fileSvc := newTestFileSvc(t)

			caPEM, caKey, caCert := generateTestCAIntegration(t)
			certPath, keyPath := writeExpiringCertPair(t, fileSvc, caCert, caKey)

			srv := renewalTestServer(t, caPEM, caCert, caKey, tt.trustStatus, tt.enrollStatus, tt.trustBody, tt.enrollBody)

			cfg := &config.Config{Endpoint: srv.URL}
			ci := certs.NewClientIdentity(tls.Certificate{})

			err := RenewOperatorCertificate(context.Background(), cfg, fileSvc, certPath, keyPath, ci)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)

			if tt.checkRenewed {
				certRel, err := fileSvc.Rel(certPath)
				require.NoError(t, err)
				savedCert, err := fileSvc.ReadFile(context.Background(), certRel)
				require.NoError(t, err)
				assert.Contains(t, string(savedCert), "BEGIN CERTIFICATE")

				keyRel, err := fileSvc.Rel(keyPath)
				require.NoError(t, err)
				savedKey, err := fileSvc.ReadFile(context.Background(), keyRel)
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
	fileSvc := newTestFileSvc(t)

	caPEM, caKey, caCert := generateTestCAIntegration(t)
	certPath, keyPath := writeExpiringCertPair(t, fileSvc, caCert, caKey)

	srv := renewalTestServer(t, caPEM, caCert, caKey, http.StatusOK, http.StatusOK, nil, nil)

	cfg := &config.Config{Endpoint: srv.URL}
	ci := certs.NewClientIdentity(tls.Certificate{})
	logger := testLogger()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		RunClientCertRenewalLoop(ctx, cfg, fileSvc, certPath, keyPath, logger, ci)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunClientCertRenewalLoop did not stop after context cancellation")
	}

	certRel, err := fileSvc.Rel(certPath)
	require.NoError(t, err)
	savedCert, err := fileSvc.ReadFile(context.Background(), certRel)
	require.NoError(t, err)
	assert.Contains(t, string(savedCert), "BEGIN CERTIFICATE",
		"cert should have been renewed on startup before context cancellation")
}

// ---------------------------------------------------------------------------
// fetchAndSaveTrustBundle (direct tests)
// ---------------------------------------------------------------------------

func TestFetchAndSaveTrustBundle(t *testing.T) {
	caPEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))
	caBundleRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)

	t.Run("Success_SavesBodyAndReturnsPEM", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(caPEM)
		}))
		t.Cleanup(srv.Close)

		body, err := fetchAndSaveTrustBundle(context.Background(), srv.URL, fileSvc)
		require.NoError(t, err)
		assert.Equal(t, caPEM, body)

		saved, err := fileSvc.ReadFile(context.Background(), caBundleRel)
		require.NoError(t, err)
		assert.Equal(t, caPEM, saved)
	})

	t.Run("HTTPError_ReturnsHTTPStatusError", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		_, err := fetchAndSaveTrustBundle(context.Background(), srv.URL, fileSvc)
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrHTTPStatusError))
	})

	t.Run("EmptyBody_ReturnsEmptyTrustBundle", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		_, err := fetchAndSaveTrustBundle(context.Background(), srv.URL, fileSvc)
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrEmptyTrustBundle))
	})

	t.Run("UnreachableServer_ReturnsFailedToReadTrustBundle", func(t *testing.T) {
		fileSvc := newTestFileSvc(t)
		_, err := fetchAndSaveTrustBundle(context.Background(), "http://127.0.0.1:0", fileSvc)
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrFailedToReadTrustBundle))
	})
}

// ---------------------------------------------------------------------------
// submitRenewal (direct tests)
// ---------------------------------------------------------------------------

func TestSubmitRenewal(t *testing.T) {
	caPEM, caKey, caCert := generateTestCAIntegration(t)

	t.Run("Success_ReturnsParsedResponse", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var req models.DeviceEnrollRequest
			require.NoError(t, json.Unmarshal(body, &req))
			opCert := signCSRIntegration(t, []byte(req.CSR), caCert, caKey, "test-operator", time.Now().Add(365*24*time.Hour))
			cliCert := signCSRIntegration(t, []byte(req.CLICSR), caCert, caKey, "test-cli", time.Now().Add(365*24*time.Hour))
			resp := models.OperatorRegistrationResponse{
				OperatorCert:      string(opCert),
				OperatorCertChain: string(caPEM),
				CLICert:           string(cliCert),
			}
			respBytes, err := json.Marshal(resp)
			require.NoError(t, err)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(respBytes)
		}))
		t.Cleanup(srv.Close)

		opCSR, _, err := GenerateCSR("test-operator")
		require.NoError(t, err)
		cliCSR, _, err := GenerateCSR("test-cli")
		require.NoError(t, err)

		client := &http.Client{}
		regResp, err := submitRenewal(context.Background(), client, srv.URL, opCSR, cliCSR, "test-host")
		require.NoError(t, err)
		require.NotNil(t, regResp)
		assert.NotEmpty(t, regResp.OperatorCert)
		assert.NotEmpty(t, regResp.CLICert)
	})

	t.Run("HTTPError_ReturnsHTTPStatusError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server error"))
		}))
		t.Cleanup(srv.Close)

		opCSR, _, err := GenerateCSR("test-operator")
		require.NoError(t, err)
		cliCSR, _, err := GenerateCSR("test-cli")
		require.NoError(t, err)

		client := &http.Client{}
		_, err = submitRenewal(context.Background(), client, srv.URL, opCSR, cliCSR, "test-host")
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrHTTPStatusError))
	})

	t.Run("ErrorFieldInResponse_ReturnsEnrollmentFailed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshal(t, models.OperatorRegistrationResponse{Error: "renewal denied"}))
		}))
		t.Cleanup(srv.Close)

		opCSR, _, err := GenerateCSR("test-operator")
		require.NoError(t, err)
		cliCSR, _, err := GenerateCSR("test-cli")
		require.NoError(t, err)

		client := &http.Client{}
		_, err = submitRenewal(context.Background(), client, srv.URL, opCSR, cliCSR, "test-host")
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrEnrollmentFailed))
	})

	t.Run("MissingCerts_ReturnsMissingRequiredField", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(mustMarshal(t, models.OperatorRegistrationResponse{
				OperatorCert: "",
				CLICert:      "",
			}))
		}))
		t.Cleanup(srv.Close)

		opCSR, _, err := GenerateCSR("test-operator")
		require.NoError(t, err)
		cliCSR, _, err := GenerateCSR("test-cli")
		require.NoError(t, err)

		client := &http.Client{}
		_, err = submitRenewal(context.Background(), client, srv.URL, opCSR, cliCSR, "test-host")
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrMissingRequiredField))
	})

	t.Run("MalformedJSON_ReturnsResponseParseFailed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{not valid json"))
		}))
		t.Cleanup(srv.Close)

		opCSR, _, err := GenerateCSR("test-operator")
		require.NoError(t, err)
		cliCSR, _, err := GenerateCSR("test-cli")
		require.NoError(t, err)

		client := &http.Client{}
		_, err = submitRenewal(context.Background(), client, srv.URL, opCSR, cliCSR, "test-host")
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrResponseParseFailed))
	})

	t.Run("UnreachableServer_ReturnsEnrollmentFailed", func(t *testing.T) {
		opCSR, _, err := GenerateCSR("test-operator")
		require.NoError(t, err)
		cliCSR, _, err := GenerateCSR("test-cli")
		require.NoError(t, err)

		client := &http.Client{}
		_, err = submitRenewal(context.Background(), client, "http://127.0.0.1:0", opCSR, cliCSR, "test-host")
		require.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrEnrollmentFailed))
	})
}

// ---------------------------------------------------------------------------
// PerformAutomaticEnrollment (additional edge cases)
// ---------------------------------------------------------------------------

func TestPerformAutomaticEnrollment_HubTrustBundleOverwritesCAFile(t *testing.T) {
	restorePorts(t)
	fileSvc := newTestFileSvc(t)

	caPEM, caKey, caCert := generateTestCAIntegration(t)
	hubBundlePEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))

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
				HubTrustBundle:    string(hubBundlePEM),
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

	_, _, err := PerformAutomaticEnrollment(context.Background(), "127.0.0.1", fileSvc, testLogger())
	require.NoError(t, err)

	caBundleRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
	savedCA, err := fileSvc.ReadFile(context.Background(), caBundleRel)
	require.NoError(t, err)
	assert.Equal(t, string(hubBundlePEM), string(savedCA),
		"CA file should be overwritten with HubTrustBundle when provided")
}

func TestPerformAutomaticEnrollment_CertChainAppendedToOperatorCert(t *testing.T) {
	restorePorts(t)
	fileSvc := newTestFileSvc(t)

	caPEM, caKey, caCert := generateTestCAIntegration(t)
	intermediatePEM := generateTestCertPEM(t, time.Now(), time.Now().Add(365*24*time.Hour))

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
				OperatorCertChain: string(intermediatePEM),
				OperatorID:        "op-001",
				OperatorSessionID: "sess-001",
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

	_, _, err := PerformAutomaticEnrollment(context.Background(), "127.0.0.1", fileSvc, testLogger())
	require.NoError(t, err)

	opCertRel := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorCert)
	savedCert, err := fileSvc.ReadFile(context.Background(), opCertRel)
	require.NoError(t, err)
	savedStr := string(savedCert)
	assert.Contains(t, savedStr, "BEGIN CERTIFICATE")

	block, rest := pem.Decode(savedCert)
	require.NotNil(t, block, "first PEM block (operator cert) should parse")
	assert.Equal(t, "CERTIFICATE", block.Type)

	chainBlock, _ := pem.Decode(rest)
	require.NotNil(t, chainBlock, "second PEM block (cert chain) should be present")
	assert.Equal(t, "CERTIFICATE", chainBlock.Type)
}

func TestPerformAutomaticEnrollment_MalformedJSONResponse(t *testing.T) {
	restorePorts(t)
	fileSvc := newTestFileSvc(t)

	caPEM, _, _ := generateTestCAIntegration(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case constants.WellKnownPKICABundle:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(caPEM)
		case constants.APIPathAuthDeviceEnroll:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{not valid json"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	constants.Ports.OperatorHttp = getServerPort(t, srv)

	_, _, err := PerformAutomaticEnrollment(context.Background(), "127.0.0.1", fileSvc, testLogger())
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrResponseParseFailed))
}
