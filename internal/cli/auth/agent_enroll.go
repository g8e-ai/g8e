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

// EnrollAgentApp enrolls an agent as an external app with the gateway.
// It returns the app SPIFFE ID, cert file path, key file path, and an error if any.
// The function is idempotent: if a valid cert exists and is not near expiry, it reuses it.
func EnrollAgentApp(cfg *config.Config, agentName string) (appID, certFile, keyFile string, err error) {
	certFile = cfg.AppCertFile(agentName)
	keyFile = cfg.AppKeyFile(agentName)

	// Idempotency check: if cert exists and is valid (more than 7 days from expiry), reuse it
	if certData, err := os.ReadFile(certFile); err == nil {
		block, _ := pem.Decode(certData)
		if block != nil && block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				// Check if cert is valid for more than 7 days
				if time.Until(cert.NotAfter) > 7*24*time.Hour {
					// Extract app SPIFFE ID from cert's URI SAN
					for _, uri := range cert.URIs {
						if uri.Scheme == "spiffe" && uri.Host == "g8e.local" {
							appID = uri.String()
							return appID, certFile, keyFile, nil
						}
					}
					// If no URI SAN found, fall through to re-enroll
				}
			}
		}
	}

	// Generate CSR for the agent app
	csr, key, err := GenerateCSR(agentName)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate CSR: %w", err)
	}

	// Build enrollment request
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

	// POST to the enrollment endpoint (plain HTTP, no mTLS yet)
	enrollURL := cfg.OperatorDiscoveryURL() + constants.APIPaths.PKIAppsEnroll
	httpClient := &http.Client{}
	resp, err := httpClient.Post(enrollURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to POST enrollment request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", "", "", fmt.Errorf("enrollment failed with status %d", resp.StatusCode)
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
		return "", "", "", fmt.Errorf("enrollment failed: %s", enrollResp.Error)
	}

	// Save cert and key to disk
	if err := SaveCertAndKey(enrollResp.AppCert, enrollResp.CertChain, key, certFile, keyFile); err != nil {
		return "", "", "", fmt.Errorf("failed to save cert and key: %w", err)
	}

	return enrollResp.AppID, certFile, keyFile, nil
}
