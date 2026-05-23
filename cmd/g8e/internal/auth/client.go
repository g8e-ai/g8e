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
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/g8e-ai/g8e/cmd/g8e/internal/config"
)

type DeviceLinkRequest struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	MaxUses    int    `json:"max_uses"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type DeviceLinkResponse struct {
	Token  string `json:"token"`
	UserID string `json:"user_id"`
	Error  string `json:"error,omitempty"`
}

type RegistrationRequest struct {
	SystemFingerprint string `json:"system_fingerprint"`
	Hostname          string `json:"hostname"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	Username          string `json:"username"`
	CSRPEM            string `json:"csr_pem"`
	CLICSRPEM         string `json:"cli_csr_pem"`
}

type RegistrationResponse struct {
	OperatorSessionID string `json:"operator_session_id"`
	CLISessionID      string `json:"cli_session_id"`
	OperatorID        string `json:"operator_id"`
	OperatorCert      string `json:"operator_cert"`
	OperatorCertChain string `json:"operator_cert_chain,omitempty"`
	CLICert           string `json:"cli_cert"`
	CLICertChain      string `json:"cli_cert_chain,omitempty"`
	HubTrustBundle    string `json:"hub_trust_bundle,omitempty"`
	Error             string `json:"error,omitempty"`
}

type Credentials struct {
	OperatorSessionID string `json:"operator_session_id"`
	UserID            string `json:"user_id"`
	OperatorID        string `json:"operator_id"`
	CLISessionID      string `json:"cli_session_id"`
}

func GenerateCSR(commonName string) (string, *ecdsa.PrivateKey, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate ECDSA key: %w", err)
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e"},
		},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create CSR: %w", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEM), privKey, nil
}

func RequestDeviceLink(cfg *config.Config, email string, count int, ttl int) (*DeviceLinkResponse, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	req := DeviceLinkRequest{
		Email:      email,
		Name:       fmt.Sprintf("cli-%s", hostname),
		MaxUses:    count,
		TTLSeconds: ttl,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/auth/device-link/request", cfg.OperatorBootstrapURL())
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to request device-link: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var dlResp DeviceLinkResponse
	if err := json.Unmarshal(respBody, &dlResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if dlResp.Error != "" {
		return nil, fmt.Errorf("device-link request failed: %s", dlResp.Error)
	}

	if dlResp.Token == "" {
		return nil, fmt.Errorf("device-link response missing token")
	}

	return &dlResp, nil
}

func RegisterDeviceLink(cfg *config.Config, token string, operatorCSR, cliCSR string) (*RegistrationResponse, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("LOGNAME")
	}

	req := RegistrationRequest{
		SystemFingerprint: fmt.Sprintf("g8e-cli-%s-%s", hostname, username),
		Hostname:          hostname,
		OS:                "linux",
		Arch:              runtime.GOARCH,
		Username:          username,
		CSRPEM:            operatorCSR,
		CLICSRPEM:         cliCSR,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", fmt.Sprintf("%s/api/auth/device-link/register", cfg.OperatorBootstrapURL()), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Device-Token", token)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to register: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var regResp RegistrationResponse
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if regResp.Error != "" {
		return nil, fmt.Errorf("registration failed: %s", regResp.Error)
	}

	return &regResp, nil
}

func SaveCredentials(cfg *config.Config, creds *Credentials) error {
	if err := os.MkdirAll(cfg.CredentialsDir, 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	credsFile := cfg.CredentialsFile()
	credsData, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	if err := os.WriteFile(credsFile, credsData, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

func LoadCredentials(cfg *config.Config) (*Credentials, error) {
	credsFile := cfg.CredentialsFile()
	credsData, err := os.ReadFile(credsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(credsData, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return &creds, nil
}

func DeleteCredentials(cfg *config.Config) error {
	credsFile := cfg.CredentialsFile()
	if err := os.Remove(credsFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete credentials file: %w", err)
	}

	certFiles := []string{
		cfg.CLICertFile(),
		cfg.CLIKeyFile(),
		cfg.OperatorCertFile(),
		cfg.OperatorKeyFile(),
		filepath.Join(cfg.CredentialsDir, "hub-bundle.pem"),
	}

	for _, file := range certFiles {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete %s: %w", file, err)
		}
	}

	return nil
}

func SaveCertAndKey(certPEM, chainPEM string, key *ecdsa.PrivateKey, certFile, keyFile string) error {
	if err := os.MkdirAll(filepath.Dir(certFile), 0700); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	certContent := certPEM
	if chainPEM != "" {
		certContent += "\n" + chainPEM
	}

	if err := os.WriteFile(certFile, []byte(certContent), 0600); err != nil {
		return fmt.Errorf("failed to write cert file: %w", err)
	}

	return nil
}

func EnsureOperatorRunning(cfg *config.Config) error {
	pidFile := filepath.Join(cfg.RuntimeDir, "pids", "operator.pid")
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("operator is not running - start the platform first: ./g8e platform start")
		}
		return fmt.Errorf("failed to read pid file: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
		return fmt.Errorf("failed to parse pid: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("operator process not running: %w", err)
	}

	return nil
}
