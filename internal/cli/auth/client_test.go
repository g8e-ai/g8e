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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
)

// extractPortFromURL extracts the port number from a httptest server URL
func extractPortFromURL(url string) int {
	// httptest URLs are like "http://127.0.0.1:12345"
	// Split by "://" first to get the host:port part
	parts := strings.Split(url, "://")
	if len(parts) < 2 {
		return 0
	}
	// Then split by ":" to get the port
	hostPort := parts[1]
	portParts := strings.Split(hostPort, ":")
	if len(portParts) < 2 {
		return 0
	}
	var port int
	fmt.Sscanf(portParts[1], "%d", &port)
	return port
}

func TestGenerateCSR(t *testing.T) {
	tests := []struct {
		name       string
		commonName string
		wantErr    bool
	}{
		{
			name:       "valid CSR generation",
			commonName: "test-operator",
			wantErr:    false,
		},
		{
			name:       "CSR with special characters",
			commonName: "test-operator-123",
			wantErr:    false,
		},
		{
			name:       "CSR with domain-style name",
			commonName: "operator.example.com",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			csrPEM, privKey, err := GenerateCSR(tt.commonName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateCSR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if csrPEM == "" {
					t.Error("GenerateCSR() returned empty CSR PEM")
				}

				if privKey == nil {
					t.Error("GenerateCSR() returned nil private key")
					return
				}

				// Verify CSR PEM format
				block, _ := pem.Decode([]byte(csrPEM))
				if block == nil {
					t.Error("Generated CSR is not valid PEM")
					return
				}
				if block.Type != "CERTIFICATE REQUEST" {
					t.Errorf("CSR PEM block type is %s, want CERTIFICATE REQUEST", block.Type)
				}

				// Verify private key is ECDSA P-256
				if privKey.Curve != elliptic.P256() {
					t.Errorf("Private key curve is %v, want P-256", privKey.Curve)
				}
			}
		})
	}
}

func TestVerifyCAFingerprint(t *testing.T) {
	// Generate a test certificate
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber:          bigIntFromInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		t.Fatalf("Failed to create test certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Compute the actual fingerprint
	fingerprint := computeSHA256Fingerprint(certDER)

	tests := []struct {
		name                string
		caPEM               []byte
		expectedFingerprint string
		wantErr             bool
		errMsg              string
	}{
		{
			name:                "valid fingerprint match",
			caPEM:               certPEM,
			expectedFingerprint: fingerprint,
			wantErr:             false,
		},
		{
			name:                "empty fingerprint (skip verification)",
			caPEM:               certPEM,
			expectedFingerprint: "",
			wantErr:             false,
		},
		{
			name:                "fingerprint mismatch",
			caPEM:               certPEM,
			expectedFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			wantErr:             true,
			errMsg:              "CA fingerprint mismatch",
		},
		{
			name:                "invalid PEM",
			caPEM:               []byte("not valid PEM"),
			expectedFingerprint: fingerprint,
			wantErr:             true,
			errMsg:              "failed to decode CA PEM",
		},
		{
			name:                "wrong PEM block type",
			caPEM:               pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: certDER}),
			expectedFingerprint: fingerprint,
			wantErr:             true,
			errMsg:              "PEM block is not a certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyCAFingerprint(tt.caPEM, tt.expectedFingerprint)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyCAFingerprint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("VerifyCAFingerprint() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

func TestIsCertificateVerificationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "generic error",
			err:  fmt.Errorf("some error"),
			want: false,
		},
		{
			name: "UnknownAuthorityError",
			err:  x509.UnknownAuthorityError{},
			want: true,
		},
		{
			name: "HostnameError",
			err:  x509.HostnameError{},
			want: true,
		},
		{
			name: "CertificateInvalidError",
			err:  x509.CertificateInvalidError{},
			want: true,
		},
		{
			name: "wrapped UnknownAuthorityError",
			err:  fmt.Errorf("wrapped: %w", x509.UnknownAuthorityError{}),
			want: true,
		},
		{
			name: "double wrapped error",
			err:  fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", x509.HostnameError{})),
			want: true,
		},
		{
			name: "wrapped generic error",
			err:  fmt.Errorf("wrapped: %w", fmt.Errorf("generic")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCertificateVerificationError(tt.err)
			if got != tt.want {
				t.Errorf("isCertificateVerificationError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveCredentials(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &config.Config{}
	cfg.CredentialsDir = tempDir

	creds := &Credentials{
		OperatorSessionID: "test-session-id",
		UserID:            "test-user-id",
		OperatorID:        "test-operator-id",
		CLISessionID:      "test-cli-session-id",
	}

	err := SaveCredentials(cfg, creds)
	if err != nil {
		t.Fatalf("SaveCredentials() failed: %v", err)
	}

	// Verify file was created using the config's CredentialsFile method
	credsFile := cfg.CredentialsFile()
	data, err := os.ReadFile(credsFile)
	if err != nil {
		t.Fatalf("Failed to read credentials file: %v", err)
	}

	// Verify file permissions (should be 0600)
	info, err := os.Stat(credsFile)
	if err != nil {
		t.Fatalf("Failed to stat credentials file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Credentials file permissions = %v, want 0600", info.Mode().Perm())
	}

	// Verify content
	var loadedCreds Credentials
	if err := json.Unmarshal(data, &loadedCreds); err != nil {
		t.Fatalf("Failed to unmarshal credentials: %v", err)
	}

	if loadedCreds.OperatorSessionID != creds.OperatorSessionID {
		t.Errorf("OperatorSessionID = %v, want %v", loadedCreds.OperatorSessionID, creds.OperatorSessionID)
	}
	if loadedCreds.UserID != creds.UserID {
		t.Errorf("UserID = %v, want %v", loadedCreds.UserID, creds.UserID)
	}
}

func TestLoadCredentials(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &config.Config{}
	cfg.CredentialsDir = tempDir

	t.Run("non-existent file returns nil", func(t *testing.T) {
		creds, err := LoadCredentials(cfg)
		if err != nil {
			t.Errorf("LoadCredentials() error = %v, want nil", err)
		}
		if creds != nil {
			t.Error("LoadCredentials() returned non-nil for non-existent file")
		}
	})

	t.Run("load existing credentials", func(t *testing.T) {
		creds := &Credentials{
			OperatorSessionID: "test-session-id",
			UserID:            "test-user-id",
			OperatorID:        "test-operator-id",
			CLISessionID:      "test-cli-session-id",
		}

		if err := SaveCredentials(cfg, creds); err != nil {
			t.Fatalf("Failed to save credentials: %v", err)
		}

		loaded, err := LoadCredentials(cfg)
		if err != nil {
			t.Fatalf("LoadCredentials() failed: %v", err)
		}

		if loaded.OperatorSessionID != creds.OperatorSessionID {
			t.Errorf("OperatorSessionID = %v, want %v", loaded.OperatorSessionID, creds.OperatorSessionID)
		}
		if loaded.UserID != creds.UserID {
			t.Errorf("UserID = %v, want %v", loaded.UserID, creds.UserID)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		credsFile := cfg.CredentialsFile()
		if err := os.WriteFile(credsFile, []byte("invalid json"), 0600); err != nil {
			t.Fatalf("Failed to write invalid JSON: %v", err)
		}

		_, err := LoadCredentials(cfg)
		if err == nil {
			t.Error("LoadCredentials() should return error for invalid JSON")
		}
	})
}

func TestDeleteCredentials(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &config.Config{}
	cfg.CredentialsDir = tempDir
	cfg.Paths = &config.PathsConfig{}
	cfg.Paths.Infra.CACertPath = filepath.Join(tempDir, "trust-bundle.pem")

	t.Run("delete existing credentials", func(t *testing.T) {
		// Create test files using config methods
		credsFile := cfg.CredentialsFile()
		certFile := cfg.CLICertFile()
		keyFile := cfg.CLIKeyFile()
		trustFile := cfg.TrustBundlePath()

		for _, f := range []string{credsFile, certFile, keyFile, trustFile} {
			if err := os.WriteFile(f, []byte("test"), 0600); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		if err := DeleteCredentials(cfg); err != nil {
			t.Fatalf("DeleteCredentials() failed: %v", err)
		}

		// Verify files are deleted
		for _, f := range []string{credsFile, certFile, keyFile, trustFile} {
			if _, err := os.Stat(f); !os.IsNotExist(err) {
				t.Errorf("File %s still exists after deletion", f)
			}
		}
	})

	t.Run("delete non-existent files succeeds", func(t *testing.T) {
		// Don't create any files
		if err := DeleteCredentials(cfg); err != nil {
			t.Errorf("DeleteCredentials() with non-existent files should succeed, got error: %v", err)
		}
	})
}

func TestSaveCertAndKey(t *testing.T) {
	tempDir := t.TempDir()

	certFile := filepath.Join(tempDir, "cert.pem")
	keyFile := filepath.Join(tempDir, "key.pem")

	// Generate test key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	certPEM := "-----BEGIN CERTIFICATE-----\ntest cert data\n-----END CERTIFICATE-----"
	chainPEM := "-----BEGIN CERTIFICATE-----\ntest chain data\n-----END CERTIFICATE-----"

	t.Run("save cert and key without chain", func(t *testing.T) {
		err := SaveCertAndKey(certPEM, "", privKey, certFile, keyFile)
		if err != nil {
			t.Fatalf("SaveCertAndKey() failed: %v", err)
		}

		// Verify cert file
		certData, err := os.ReadFile(certFile)
		if err != nil {
			t.Fatalf("Failed to read cert file: %v", err)
		}
		if string(certData) != certPEM {
			t.Errorf("Cert file content mismatch")
		}

		// Verify key file
		keyData, err := os.ReadFile(keyFile)
		if err != nil {
			t.Fatalf("Failed to read key file: %v", err)
		}
		block, _ := pem.Decode(keyData)
		if block == nil {
			t.Error("Key file is not valid PEM")
			return
		}
		if block.Type != "EC PRIVATE KEY" {
			t.Errorf("Key PEM block type is %s, want EC PRIVATE KEY", block.Type)
		}

		// Verify file permissions
		info, err := os.Stat(keyFile)
		if err != nil {
			t.Fatalf("Failed to stat key file: %v", err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("Key file permissions = %v, want 0600", info.Mode().Perm())
		}
	})

	t.Run("save cert and key with chain", func(t *testing.T) {
		err := SaveCertAndKey(certPEM, chainPEM, privKey, certFile, keyFile)
		if err != nil {
			t.Fatalf("SaveCertAndKey() failed: %v", err)
		}

		certData, err := os.ReadFile(certFile)
		if err != nil {
			t.Fatalf("Failed to read cert file: %v", err)
		}
		expected := certPEM + "\n" + chainPEM
		if string(certData) != expected {
			t.Errorf("Cert file content mismatch, got %q, want %q", string(certData), expected)
		}
	})
}

func TestCheckOperatorRunningAtURL(t *testing.T) {
	t.Run("invalid URL format", func(t *testing.T) {
		err := CheckOperatorRunningAtURL("invalid-url")
		if err == nil {
			t.Error("CheckOperatorRunningAtURL() should return error for invalid URL")
		}
		if !strings.Contains(err.Error(), "invalid Operator URL") {
			t.Errorf("Error message should contain 'invalid Operator URL', got: %v", err)
		}
	})

	t.Run("localhost to IPv4 conversion", func(t *testing.T) {
		// Start a test listener
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to create test listener: %v", err)
		}
		defer listener.Close()

		addr := listener.Addr().String()
		url := fmt.Sprintf("http://localhost:%s", strings.Split(addr, ":")[1])

		err = CheckOperatorRunningAtURL(url)
		if err != nil {
			t.Errorf("CheckOperatorRunningAtURL() failed for running server: %v", err)
		}
	})

	t.Run("server not running", func(t *testing.T) {
		// Use a port that's unlikely to be in use
		url := "http://127.0.0.1:59999"
		err := CheckOperatorRunningAtURL(url)
		if err == nil {
			t.Error("CheckOperatorRunningAtURL() should return error when server not running")
		}
		if !strings.Contains(err.Error(), "not running or not responding") {
			t.Errorf("Error message should contain 'not running or not responding', got: %v", err)
		}
	})
}

func TestParseCertPEM(t *testing.T) {
	// Generate a test certificate
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: bigIntFromInt(1),
		Subject:      pkix.Name{CommonName: "Test Cert"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		t.Fatalf("Failed to create test certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	tempDir := t.TempDir()
	certFile := filepath.Join(tempDir, "cert.pem")

	t.Run("valid certificate", func(t *testing.T) {
		if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
			t.Fatalf("Failed to write cert file: %v", err)
		}

		cert, err := parseCertPEM(certFile)
		if err != nil {
			t.Fatalf("parseCertPEM() failed: %v", err)
		}

		if cert.Subject.CommonName != "Test Cert" {
			t.Errorf("CommonName = %v, want Test Cert", cert.Subject.CommonName)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := parseCertPEM(filepath.Join(tempDir, "nonexistent.pem"))
		if err == nil {
			t.Error("parseCertPEM() should return error for non-existent file")
		}
	})

	t.Run("invalid PEM", func(t *testing.T) {
		if err := os.WriteFile(certFile, []byte("invalid pem"), 0600); err != nil {
			t.Fatalf("Failed to write invalid PEM: %v", err)
		}

		_, err := parseCertPEM(certFile)
		if err == nil {
			t.Error("parseCertPEM() should return error for invalid PEM")
		}
	})

	t.Run("wrong PEM block type", func(t *testing.T) {
		keyPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: certDER,
		})
		if err := os.WriteFile(certFile, keyPEM, 0600); err != nil {
			t.Fatalf("Failed to write key PEM: %v", err)
		}

		_, err := parseCertPEM(certFile)
		if err == nil {
			t.Error("parseCertPEM() should return error for wrong PEM type")
		}
	})
}

func TestIsCertExpiringSoon(t *testing.T) {
	tests := []struct {
		name         string
		notAfter     time.Time
		wantExpiring bool
	}{
		{
			name:         "cert expiring in 1 hour",
			notAfter:     time.Now().Add(1 * time.Hour),
			wantExpiring: true,
		},
		{
			name:         "cert expiring in 23 hours",
			notAfter:     time.Now().Add(23 * time.Hour),
			wantExpiring: true,
		},
		{
			name:         "cert expiring in 25 hours",
			notAfter:     time.Now().Add(25 * time.Hour),
			wantExpiring: false,
		},
		{
			name:         "cert expiring in 7 days",
			notAfter:     time.Now().Add(7 * 24 * time.Hour),
			wantExpiring: false,
		},
		{
			name:         "already expired",
			notAfter:     time.Now().Add(-1 * time.Hour),
			wantExpiring: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := &x509.Certificate{
				NotAfter: tt.notAfter,
			}
			got := isCertExpiringSoon(cert)
			if got != tt.wantExpiring {
				t.Errorf("isCertExpiringSoon() = %v, want %v", got, tt.wantExpiring)
			}
		})
	}
}

func TestCheckCertExpiry(t *testing.T) {
	// Generate a test certificate
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: bigIntFromInt(1),
		Subject:      pkix.Name{CommonName: "Test Cert"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour), // Expiring soon
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		t.Fatalf("Failed to create test certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	tempDir := t.TempDir()
	certFile := filepath.Join(tempDir, "cert.pem")

	t.Run("expiring certificate", func(t *testing.T) {
		if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
			t.Fatalf("Failed to write cert file: %v", err)
		}

		expiring, err := CheckCertExpiry(certFile)
		if err != nil {
			t.Fatalf("CheckCertExpiry() failed: %v", err)
		}
		if !expiring {
			t.Error("CheckCertExpiry() should return true for expiring certificate")
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, err := CheckCertExpiry(filepath.Join(tempDir, "nonexistent.pem"))
		if err == nil {
			t.Error("CheckCertExpiry() should return error for non-existent file")
		}
	})
}

// Helper functions

func bigIntFromInt(n int64) *big.Int {
	return big.NewInt(n)
}

func computeSHA256Fingerprint(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
