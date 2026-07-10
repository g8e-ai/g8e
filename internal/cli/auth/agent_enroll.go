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
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/protocol"
)

// EnrollCLI performs idempotent first-time CLI session enrollment. It bootstraps
// the gateway if it has never been bootstrapped, or enrolls a new CLI identity if
// the gateway is already running. On Windows, it uses the Windows Certificate
// Store for key generation and imports the signed cert for Windows Hello native
// API access. It does NOT handle re-enrollment of existing credentials — that
// is the re-enrollment path's responsibility.
func EnrollCLI(cfg *config.Config, useTPM bool) error {
	bootstrapped, err := CheckBootstrapStatus(cfg, "")
	if err != nil {
		return fmt.Errorf("check bootstrap status: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrNetworkGetHostname, err)
	}

	var cliCSR string
	var cliKey *ecdsa.PrivateKey
	if runtime.GOOS == "windows" {
		cliCSR, cliKey, err = GenerateWindowsCSR(fmt.Sprintf("g8e-cli-%s", hostname), useTPM)
	} else {
		cliCSR, cliKey, err = GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	}
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	var regResp *RegistrationResponse
	if !bootstrapped {
		regResp, err = BootstrapWithURL(cfg, "", cliCSR, "", "")
	} else {
		regResp, err = CLIEnroll(cfg, cliCSR, "")
	}
	if err != nil {
		return err
	}
	if regResp.CLISessionID == "" || regResp.CLICert == "" {
		return constants.ErrMissingRequiredField
	}

	if err := SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	if runtime.GOOS == "windows" {
		if importErr := ImportCertificateToWindowsStore(regResp.CLICert); importErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to import CLI cert to Windows Certificate Store: %v\n", importErr)
		}
	}
	if regResp.HubTrustBundle != "" {
		trustPath := cfg.TrustBundlePath()
		if err := os.MkdirAll(filepath.Dir(trustPath), constants.PermDirStandard); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}
		if err := os.WriteFile(trustPath, []byte(regResp.HubTrustBundle), constants.PermFilePublic); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
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
// It requires an authenticated human CLI session (mTLS) and issues a short-lived cert that carries
// both the app SPIFFE ID and the requestor's user identity.
func EnrollAgentApp(cfg *config.Config, agentName string) (appID, certFile, keyFile string, err error) {
	certFile = cfg.AppCertFile(agentName)
	keyFile = cfg.AppKeyFile(agentName)

	if existingAppID, ok := checkExistingAppCert(certFile, agentName); ok {
		return existingAppID, certFile, keyFile, nil
	}

	csr, key, err := GenerateCSR(agentName)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	req := models.AppEnrollRequest{
		CSR:     csr,
		AppName: agentName,
		AppType: constants.AppTypeMCPClient,
	}
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %w", constants.ErrRequestMarshalFailed, err)
	}

	httpClient, err := BuildMTLSClient(cfg, httpTimeout)
	if err != nil {
		return "", "", "", err
	}

	creds, err := LoadCredentials(cfg)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}
	if creds == nil || creds.CLISessionID == "" {
		return "", "", "", constants.ErrNotAuthenticated
	}

	enrollURL := fmt.Sprintf("%s%s", cfg.OperatorHTTPURL(), constants.APIPaths.PKIAppsDelegated)
	httpReq, err := http.NewRequest(http.MethodPost, enrollURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}
	httpReq.Header.Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)
	httpReq.Header.Set(constants.HeaderCLISessionID, creds.CLISessionID)
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", "", "", fmt.Errorf("%w: status %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}

	var enrollResp models.AppEnrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&enrollResp); err != nil {
		return "", "", "", fmt.Errorf("%w: %w", constants.ErrResponseParseFailed, err)
	}

	if !enrollResp.Success {
		return "", "", "", fmt.Errorf("%w: %s", constants.ErrEnrollmentFailed, enrollResp.Error)
	}

	if err := SaveCertAndKey(enrollResp.AppCert, enrollResp.CertChain, key, certFile, keyFile); err != nil {
		return "", "", "", fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	return enrollResp.AppID, certFile, keyFile, nil
}

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
	if time.Until(cert.NotAfter) < constants.AppCertMinValidity {
		return "", false
	}
	expectedSPIFFE := protocol.NewWorkloadIdentity().AppSPIFFEID(agentName)
	for _, uri := range cert.URIs {
		if uri.String() == expectedSPIFFE {
			return expectedSPIFFE, true
		}
	}
	return "", false
}
