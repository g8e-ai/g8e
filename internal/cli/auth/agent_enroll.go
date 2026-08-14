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
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/protocol"
)

// EnrollAgentApp enrolls an agent as an external app with the gateway using the delegated credential model.
// It requires an authenticated human CLI session (mTLS) and issues a short-lived cert that carries
// both the app SPIFFE ID and the requestor's user identity.
func EnrollAgentApp(fileSvc fs.RuntimeFileService, cfg *config.Config, agentName string) (appID, certFile, keyFile string, err error) {
	certFile = cfg.AppCertFile(agentName)
	keyFile = cfg.AppKeyFile(agentName)

	if existingAppID, ok := checkExistingAppCert(fileSvc, certFile, agentName); ok {
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

	httpClient, err := BuildMTLSClient(fileSvc, cfg, httpTimeout)
	if err != nil {
		return "", "", "", err
	}

	creds, err := LoadCredentials(fileSvc, cfg)
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

	certRel, err := fileSvc.RelFromAbs(certFile)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	keyRel, err := fileSvc.RelFromAbs(keyFile)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	if err := SaveCertAndKey(fileSvc, enrollResp.AppCert, enrollResp.CertChain, key, certRel, keyRel); err != nil {
		return "", "", "", fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	return enrollResp.AppID, certFile, keyFile, nil
}

func checkExistingAppCert(fileSvc fs.RuntimeFileService, certFile, agentName string) (string, bool) {
	certRel, err := fileSvc.RelFromAbs(certFile)
	if err != nil {
		return "", false
	}
	certBytes, err := fileSvc.ReadFile(context.Background(), certRel)
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
