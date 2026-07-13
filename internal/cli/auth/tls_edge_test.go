package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestBuildMTLSClient_MissingCertFile(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newAuthTestFileSvc(t)
	cfg := &config.Config{
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{Host: "localhost"},
	}

	_, err := BuildMTLSClient(fileSvc, cfg, 30*time.Second)
	if err == nil {
		t.Error("expected error for missing cert file")
	}
	if !errors.Is(err, constants.ErrFailedToLoadClientCertificate) {
		t.Errorf("expected ErrFailedToLoadClientCertificate, got: %v", err)
	}
}

func TestBuildMTLSClient_InvalidCertPair(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newAuthTestFileSvc(t)
	cfg := &config.Config{
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{Host: "localhost"},
	}

	certFile := cfg.CLICertFile()
	keyFile := cfg.CLIKeyFile()
	if err := os.MkdirAll(filepath.Dir(certFile), constants.PermDirPrivate); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(certFile, []byte("not a cert"), constants.PermFilePrivate); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte("not a key"), constants.PermFilePrivate); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err := BuildMTLSClient(fileSvc, cfg, 30*time.Second)
	if err == nil {
		t.Error("expected error for invalid cert pair")
	}
	if !errors.Is(err, constants.ErrFailedToLoadClientCertificate) {
		t.Errorf("expected ErrFailedToLoadClientCertificate, got: %v", err)
	}
}

func TestBuildMTLSClient_MissingTrustBundle(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newAuthTestFileSvc(t)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		t.Fatalf("CreateCertificate failed: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey failed: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cfg := &config.Config{
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{Host: "localhost"},
	}
	cfg.Paths.Infra.CACertPath = filepath.Join(tmpDir, "ca-bundle.pem")

	certFile := cfg.CLICertFile()
	keyFile := cfg.CLIKeyFile()
	if err := os.MkdirAll(filepath.Dir(certFile), constants.PermDirPrivate); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(certFile, certPEM, constants.PermFilePrivate); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, constants.PermFilePrivate); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err = BuildMTLSClient(fileSvc, cfg, 30*time.Second)
	if err == nil {
		t.Error("expected error for missing trust bundle")
	}
	if !errors.Is(err, constants.ErrFailedToReadTrustBundle) {
		t.Errorf("expected ErrFailedToReadTrustBundle, got: %v", err)
	}
}

func TestBuildMTLSClient_InvalidTrustBundle(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newAuthTestFileSvc(t)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		t.Fatalf("CreateCertificate failed: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey failed: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cfg := &config.Config{
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{Host: "localhost"},
	}
	caFile := filepath.Join(tmpDir, "ca-bundle.pem")
	cfg.Paths.Infra.CACertPath = caFile

	certFile := cfg.CLICertFile()
	keyFile := cfg.CLIKeyFile()
	if err := os.MkdirAll(filepath.Dir(certFile), constants.PermDirPrivate); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(certFile, certPEM, constants.PermFilePrivate); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, constants.PermFilePrivate); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(caFile, []byte("not a cert"), constants.PermFilePublic); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err = BuildMTLSClient(fileSvc, cfg, 30*time.Second)
	if err == nil {
		t.Error("expected error for invalid trust bundle")
	}
	if !errors.Is(err, constants.ErrCAParseFailed) {
		t.Errorf("expected ErrCAParseFailed, got: %v", err)
	}
}

func TestCheckOperatorRunning_Running(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	err = CheckOperatorRunningAtURL("http://127.0.0.1:" + strconv.Itoa(port))
	if err != nil {
		t.Errorf("expected no error for running operator, got: %v", err)
	}
}

func TestCheckOperatorRunning_LocalhostReplacement(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	err = CheckOperatorRunningAtURL("http://localhost:" + strconv.Itoa(port))
	if err != nil {
		t.Errorf("expected no error for localhost URL, got: %v", err)
	}
}

func TestAutoRenewCertificate_InvalidCertType(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newAuthTestFileSvc(t)
	cfg := &config.Config{
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{Host: "localhost"},
	}

	err := AutoRenewCertificate(fileSvc, cfg, "invalid-type", "")
	if err == nil {
		t.Error("expected error for invalid cert type")
	}
	if !errors.Is(err, constants.ErrValidationFailed) {
		t.Errorf("expected ErrValidationFailed, got: %v", err)
	}
}

func TestAutoRenewCertificate_NonExpiringCert(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newAuthTestFileSvc(t)

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		t.Fatalf("CreateCertificate failed: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	cfg := &config.Config{
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{Host: "localhost"},
	}

	certFile := cfg.CLICertFile()
	if err := os.MkdirAll(filepath.Dir(certFile), constants.PermDirPrivate); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(certFile, certPEM, constants.PermFilePrivate); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err = AutoRenewCertificate(fileSvc, cfg, "cli", "")
	if err != nil {
		t.Errorf("expected no error for non-expiring cert, got: %v", err)
	}
}

func TestLoadCredentials_NonExistentFile(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newAuthTestFileSvc(t)
	cfg := &config.Config{
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{Host: "localhost"},
	}

	creds, err := LoadCredentials(fileSvc, cfg)
	if err != nil {
		t.Errorf("expected no error for non-existent file, got: %v", err)
	}
	if creds != nil {
		t.Errorf("expected nil creds for non-existent file, got: %v", creds)
	}
}

func TestDeleteCredentials_NonExistent(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newAuthTestFileSvc(t)
	cfg := &config.Config{
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{Host: "localhost"},
	}

	err := DeleteCredentials(fileSvc, cfg)
	if err != nil {
		t.Errorf("expected no error for non-existent file, got: %v", err)
	}
}

func TestCheckBootstrapStatus_InvalidURL(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{Host: "localhost"},
	}

	_, err := CheckBootstrapStatus(cfg, "://invalid")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestCheckBootstrapStatus_NotRunning(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{Host: "localhost"},
	}

	_, err := CheckBootstrapStatus(cfg, "http://127.0.0.1:1")
	if err == nil {
		t.Error("expected error for non-running server")
	}
	if !errors.Is(err, constants.ErrServiceUnavailable) {
		t.Errorf("expected ErrServiceUnavailable, got: %v", err)
	}
}

func TestCheckBootstrapStatus_ValidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"bootstrapped": true})
	}))
	defer srv.Close()

	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{Host: "localhost"},
	}

	bootstrapped, err := CheckBootstrapStatus(cfg, srv.URL)
	if err != nil {
		t.Fatalf("CheckBootstrapStatus failed: %v", err)
	}
	if !bootstrapped {
		t.Error("expected bootstrapped=true")
	}
}
