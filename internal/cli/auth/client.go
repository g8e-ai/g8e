// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/pkg/certutil"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// httpTimeout is the default timeout for all HTTP clients in the auth package.
const httpTimeout = 30 * time.Second

// ReadTrustBundle reads the trust bundle using fileSvc for the default runtime
// path, or os.ReadFile for a custom external path.
func ReadTrustBundle(fileSvc fs.RuntimeFileService, cfg *config.Config) ([]byte, error) {
	if custom := cfg.CustomTrustBundlePath(); custom != "" {
		return os.ReadFile(custom)
	}
	return fileSvc.ReadFile(context.Background(), cfg.DefaultTrustBundleRelPath())
}

// WriteTrustBundleFS writes the trust bundle using fileSvc for the default runtime
// path, or os.* for a custom external path.
func WriteTrustBundleFS(fileSvc fs.RuntimeFileService, cfg *config.Config, data []byte, mode os.FileMode) error {
	if custom := cfg.CustomTrustBundlePath(); custom != "" {
		if err := os.MkdirAll(filepath.Dir(custom), constants.PermDirPrivate); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if err := os.WriteFile(custom, data, mode); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
		}
		return nil
	}
	return fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), data, mode)
}

// RemoveTrustBundleFS removes the trust bundle using fileSvc for the default
// runtime path, or os.Remove for a custom external path. No-op if the file
// does not exist.
func RemoveTrustBundleFS(fileSvc fs.RuntimeFileService, cfg *config.Config) error {
	if custom := cfg.CustomTrustBundlePath(); custom != "" {
		if err := os.Remove(custom); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
		}
		return nil
	}
	return fileSvc.Remove(context.Background(), cfg.DefaultTrustBundleRelPath())
}

// isNotFound checks if an error indicates the file does not exist.
func isNotFound(err error) bool {
	return errors.Is(err, constants.ErrNotFound) || errors.Is(err, os.ErrNotExist)
}

type Credentials struct {
	OperatorSessionID string `json:"operator_session_id"`
	UserID            string `json:"user_id"`
	OperatorID        string `json:"operator_id"`
	CLISessionID      string `json:"cli_session_id"`
}

type ClientAuthContext struct {
	OperatorSessionID string `json:"operator_session_id"`
	CLISessionID      string `json:"cli_session_id"`
	UserID            string `json:"user_id"`
	OperatorID        string `json:"operator_id"`
	ClientCert        string `json:"client_cert"`
	ClientKey         string `json:"client_key"`
}

func LoadClientAuthContext(fileSvc fs.RuntimeFileService, cfg *config.Config) (*ClientAuthContext, error) {
	creds, err := LoadCredentials(fileSvc, cfg)
	if err != nil {
		return nil, err
	}
	if creds == nil {
		return nil, fmt.Errorf("%w: local CLI credentials are absent; run './g8e auth enroll user'", constants.ErrNotAuthenticated)
	}
	missing := make([]string, 0, 2)
	if creds.CLISessionID == "" {
		missing = append(missing, "cli_session_id")
	}
	if creds.UserID == "" {
		missing = append(missing, "user_id")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: local CLI credentials are missing %s; run './g8e auth enroll user' or './g8e auth refresh'", constants.ErrNotAuthenticated, strings.Join(missing, ", "))
	}
	paths := []string{cfg.CLICertFile(), cfg.CLIKeyFile()}
	for _, path := range paths {
		relPath, err := fileSvc.RelFromAbs(path)
		if err != nil {
			return nil, fmt.Errorf("auth: resolve client identity path: %w", err)
		}
		exists, err := fileSvc.FileExists(context.Background(), relPath)
		if err != nil {
			return nil, fmt.Errorf("auth: inspect client identity path: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("%w: incomplete local CLI identity; run './g8e auth enroll user'", constants.ErrNotAuthenticated)
		}
	}
	return &ClientAuthContext{
		OperatorSessionID: creds.OperatorSessionID,
		CLISessionID:      creds.CLISessionID,
		UserID:            creds.UserID,
		OperatorID:        creds.OperatorID,
		ClientCert:        cfg.CLICertFile(),
		ClientKey:         cfg.CLIKeyFile(),
	}, nil
}

// getLocalOSUser retrieves the current OS user information.
func getLocalOSUser() *models.LocalOSUser {
	currentUser, err := user.Current()
	if err != nil {
		return nil
	}

	var domain, username string
	parts := strings.SplitN(currentUser.Username, "\\", 2)
	if len(parts) == 2 {
		domain = parts[0]
		username = parts[1]
	} else {
		username = currentUser.Username
	}

	var sid string
	if runtime.GOOS == "windows" {
		sid = currentUser.Uid
	}

	return &models.LocalOSUser{
		Domain:   domain,
		Username: username,
		UID:      currentUser.Uid,
		GID:      currentUser.Gid,
		SID:      sid,
	}
}

func GenerateCSR(commonName string) (csrPEM string, privKey *ecdsa.PrivateKey, err error) {
	privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e"},
		},
		DNSNames: []string{"localhost", "g8e.local"},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	csrPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEMBytes), privKey, nil
}

// VerifyCAFingerprint verifies that a PEM-encoded CA bundle matches the expected fingerprint.
// The fingerprint should be a hex-encoded SHA-256 hash (64 characters).
func VerifyCAFingerprint(caPEM []byte, expectedFingerprint string) error {
	if expectedFingerprint == "" {
		return nil
	}

	// Parse the PEM to extract the DER-encoded certificate
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return constants.ErrPEMDecodeFailed
	}

	if block.Type != "CERTIFICATE" {
		return constants.ErrInvalidPEMType
	}

	// Compute SHA-256 hash of the DER-encoded certificate
	hash := sha256.Sum256(block.Bytes)
	actualFP := hex.EncodeToString(hash[:])

	if actualFP != expectedFingerprint {
		return constants.ErrValidationFailed
	}

	return nil
}

// isCertificateVerificationError checks if an error is a TLS certificate verification error
// without using string matching on error messages.
func isCertificateVerificationError(err error) bool {
	if err == nil {
		return false
	}

	// Check for x509 certificate errors by unwrapping the error chain
	for {
		// Check for common x509 error types
		switch err.(type) {
		case x509.UnknownAuthorityError:
			return true
		case x509.HostnameError:
			return true
		case x509.CertificateInvalidError:
			return true
		}

		// Unwrap to check wrapped errors
		if unwrapped := errors.Unwrap(err); unwrapped != nil {
			err = unwrapped
			continue
		}
		break
	}

	return false
}

func SaveCredentials(fileSvc fs.RuntimeFileService, cfg *config.Config, creds *Credentials) error {
	credsData, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONBody, err)
	}

	relPath, err := fileSvc.RelFromAbs(cfg.CredentialsFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}
	if err := fileSvc.WriteFile(context.Background(), relPath, credsData, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}

	return nil
}

func LoadCredentials(fileSvc fs.RuntimeFileService, cfg *config.Config) (*Credentials, error) {
	relPath, err := fileSvc.RelFromAbs(cfg.CredentialsFile())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}
	credsData, err := fileSvc.ReadFile(context.Background(), relPath)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}

	var creds Credentials
	if err := json.Unmarshal(credsData, &creds); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONBody, err)
	}

	return &creds, nil
}

func DeleteCredentials(fileSvc fs.RuntimeFileService, cfg *config.Config) error {
	credsRel, err := fileSvc.RelFromAbs(cfg.CredentialsFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}
	if err := fileSvc.Remove(context.Background(), credsRel); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}

	cliCertRel, err := fileSvc.RelFromAbs(cfg.CLICertFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}
	if err := fileSvc.Remove(context.Background(), cliCertRel); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}

	cliKeyRel, err := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}
	if err := fileSvc.Remove(context.Background(), cliKeyRel); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}

	if err := RemoveTrustBundleFS(fileSvc, cfg); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}

	return nil
}

// SaveCertAndKey writes a certificate and its private key to the runtime directory via fileSvc.
// certRelPath and keyRelPath must be relative to the .g8e/ runtime directory root.
func SaveCertAndKey(fileSvc fs.RuntimeFileService, certPEM, chainPEM string, key *ecdsa.PrivateKey, certRelPath, keyRelPath string) error {
	certBytes, keyBytes, err := certutil.EncodeCertAndKey(certPEM, chainPEM, key)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	if err := fileSvc.WriteFile(context.Background(), keyRelPath, keyBytes, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	if err := fileSvc.WriteFile(context.Background(), certRelPath, certBytes, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	return nil
}

func CheckOperatorRunning(cfg *config.Config) error {
	return CheckOperatorRunningAtURL(cfg.OperatorDiscoveryURL())
}

func CheckOperatorRunningAtURL(operatorURL string) error {
	// Parse the URL to extract host:port
	parsedURL, err := url.Parse(operatorURL)
	if err != nil {
		return fmt.Errorf("%w: %s", constants.ErrGatewayURLRequired, operatorURL)
	}

	hostPort := parsedURL.Host
	if hostPort == "" {
		return fmt.Errorf("%w: %s", constants.ErrGatewayURLRequired, operatorURL)
	}
	// Force IPv4 by replacing localhost with 127.0.0.1 to prevent IPv6 resolution
	if strings.HasPrefix(hostPort, "localhost:") {
		hostPort = "127.0.0.1" + hostPort[9:]
	}
	// Try to connect to the port
	conn, err := net.DialTimeout(string(constants.NetworkProtocolTCP), hostPort, 5*time.Second)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrServiceUnavailable, err)
	}
	conn.Close()

	return nil
}

// parseCertPEM parses a PEM-encoded certificate file from the runtime directory and returns the x509 certificate.
func parseCertPEM(fileSvc fs.RuntimeFileService, certRelPath string) (*x509.Certificate, error) {
	certPEM, err := fileSvc.ReadFile(context.Background(), certRelPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrCertReadFailed, err)
	}

	return certutil.ParseCertFromPEM(certPEM)
}

// isCertExpiringSoon checks if a certificate is expiring within the renewal threshold.
// The threshold is set to 24 hours before expiry to allow ample time for renewal.
func isCertExpiringSoon(cert *x509.Certificate) bool {
	renewalThreshold := 24 * time.Hour
	timeUntilExpiry := time.Until(cert.NotAfter)
	return timeUntilExpiry <= renewalThreshold
}

// CheckCertExpiry checks if the local CLI or Operator certificate is expiring soon.
// Returns true if the certificate is expiring within the renewal threshold.
// certRelPath must be relative to the .g8e/ runtime directory root.
func CheckCertExpiry(fileSvc fs.RuntimeFileService, certRelPath string) (bool, error) {
	cert, err := parseCertPEM(fileSvc, certRelPath)
	if err != nil {
		return false, err
	}

	return isCertExpiringSoon(cert), nil
}
