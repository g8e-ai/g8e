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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

// EnrollCLI performs idempotent first-time CLI enrollment. It bootstraps the
// gateway if it has never been bootstrapped, or enrolls a new CLI identity if
// the gateway is already running. It does NOT handle re-enrollment of existing
// credentials — that is the login command's responsibility.
func EnrollCLI(cfg *config.Config) error {
	bootstrapped, err := CheckBootstrapStatus(cfg)
	if err != nil {
		return fmt.Errorf("check bootstrap status: %w", err)
	}

	hostname, _ := os.Hostname()
	cliCSR, cliKey, err := GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	if err != nil {
		return fmt.Errorf("generate CSR: %w", err)
	}

	var regResp *RegistrationResponse
	if !bootstrapped {
		regResp, err = Bootstrap(cfg, "", cliCSR, "")
	} else {
		regResp, err = CLIEnroll(cfg, cliCSR)
	}
	if err != nil {
		return err
	}
	if regResp.CLISessionID == "" || regResp.CLICert == "" {
		return fmt.Errorf("unexpected enrollment response (missing required fields)")
	}

	if err := SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
		return fmt.Errorf("save cert: %w", err)
	}
	if regResp.HubTrustBundle != "" {
		if err := os.WriteFile(cfg.TrustBundleFile(), []byte(regResp.HubTrustBundle), 0644); err != nil {
			return fmt.Errorf("save trust bundle: %w", err)
		}
	}
	return SaveCredentials(cfg, &Credentials{
		OperatorSessionID: regResp.OperatorSessionID,
		UserID:            regResp.UserID,
		OperatorID:        regResp.OperatorID,
		CLISessionID:      regResp.CLISessionID,
	})
}

// EnrollAgentApp enrolls an agent as an external app with the gateway using the delegated credential model.
// It requires an authenticated human CLI session (mTLS) and issues a short-lived cert (1 hour) that carries
// both the app SPIFFE ID and the requestor's user identity.
// It returns the app SPIFFE ID, cert file path, key file path, and an error if any.
func EnrollAgentApp(cfg *config.Config, agentName string) (appID, certFile, keyFile string, err error) {
	certFile = cfg.AppCertFile(agentName)
	keyFile = cfg.AppKeyFile(agentName)

	// Idempotency: reuse existing cert if still valid (>7 days) with correct SPIFFE URI SAN
	if existingAppID, ok := checkExistingAppCert(certFile, agentName); ok {
		return existingAppID, certFile, keyFile, nil
	}

	// Generate CSR for the agent app
	csr, key, err := GenerateCSR(agentName)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate CSR: %w", err)
	}

	// Build delegated credential request
	req := struct {
		CSR     string `json:"csr_pem"`
		AppName string `json:"app_name"`
		AppType string `json:"app_type"`
	}{
		CSR:     csr,
		AppName: agentName,
		AppType: "mcp-client",
	}
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to marshal enrollment request: %w", err)
	}

	// Load CLI mTLS certificate for authentication
	cliCert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return "", "", "", fmt.Errorf("failed to load CLI certificate: %w", err)
	}

	// Load CA bundle for server verification
	caBundleBytes, err := os.ReadFile(cfg.TrustBundlePath())
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read CA bundle: %w", err)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caBundleBytes)

	// Load credentials to get CLI session ID
	creds, err := LoadCredentials(cfg)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to load credentials: %w", err)
	}
	if creds == nil || creds.CLISessionID == "" {
		return "", "", "", fmt.Errorf("no CLI session found; run 'g8e auth enroll' first")
	}

	// Create mTLS HTTP client
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cliCert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}

	// POST to the delegated credential endpoint (requires mTLS)
	enrollURL := cfg.OperatorHTTPURL() + constants.APIPaths.PKIAppsDelegated
	httpReq, err := http.NewRequest("POST", enrollURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(constants.HeaderCLISessionID, creds.CLISessionID)
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to POST delegated credential request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", "", "", fmt.Errorf("delegated credential enrollment failed with status %d", resp.StatusCode)
	}

	var enrollResp struct {
		Success     bool   `json:"success"`
		AppCert     string `json:"app_cert"`
		CertChain   string `json:"cert_chain"`
		TrustBundle string `json:"trust_bundle"`
		AppID       string `json:"app_id"`
		Error       string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&enrollResp); err != nil {
		return "", "", "", fmt.Errorf("failed to decode enrollment response: %w", err)
	}

	if !enrollResp.Success {
		return "", "", "", fmt.Errorf("delegated credential enrollment failed: %s", enrollResp.Error)
	}

	// Save cert and key to disk
	if err := SaveCertAndKey(enrollResp.AppCert, enrollResp.CertChain, key, certFile, keyFile); err != nil {
		return "", "", "", fmt.Errorf("failed to save cert and key: %w", err)
	}

	return enrollResp.AppID, certFile, keyFile, nil
}

// checkExistingAppCert checks if an existing app cert is still valid (>7 days remaining)
// and carries the expected SPIFFE URI SAN for the given agent name.
func checkExistingAppCert(certFile, agentName string) (string, bool) {
	certBytes, err := os.ReadFile(certFile)
	if err != nil {
		return "", false
	}
	block, _ := pem.Decode(certBytes)
	if block == nil {
		return "", false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", false
	}
	if time.Until(cert.NotAfter) < 7*24*time.Hour {
		return "", false
	}
	expectedSPIFFE := fmt.Sprintf("spiffe://g8e.local/app/%s", agentName)
	for _, uri := range cert.URIs {
		if uri.String() == expectedSPIFFE {
			return expectedSPIFFE, true
		}
	}
	return "", false
}
