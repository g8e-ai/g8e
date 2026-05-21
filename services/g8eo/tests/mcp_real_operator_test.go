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

package tests

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
)

// TestMCPRealOperator_Smoke is a live-operator smoke test that validates
// the bootstrap + MCP tools/list flow against a running ./g8e platform.
//
// This test is intentionally narrow: it does NOT exercise tools/call,
// which requires a downstream MCP server and OOB WebAuthn approval -
// those flows are covered hermetically by TestMCPGateway_EndToEnd in
// mcp_gateway_test.go.
//
// Skip conditions:
//   - Operator not reachable at $OPERATOR_URL (default from paths.json)
//   - Trust bundle not present at $G8E_PKI_DIR_HOST/trust/hub-bundle.pem
//   - Platform already bootstrapped (403) and no rotation context available
func TestMCPRealOperator_Smoke(t *testing.T) {
	operatorURL := os.Getenv("OPERATOR_URL")
	if operatorURL == "" {
		operatorURL = fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttp)
	}

	insecureClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	if resp, err := insecureClient.Get(operatorURL + "/health"); err != nil {
		t.Skipf("Operator not reachable at %s: %v. Run './g8e platform start' to enable.", operatorURL, err)
	} else {
		resp.Body.Close()
	}

	cwd, err := os.Getwd()
	require.NoError(t, err)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(cwd)))
	pkiDir := filepath.Join(repoRoot, ".g8e", "pki")
	if override := os.Getenv("G8E_PKI_DIR_HOST"); override != "" {
		pkiDir = override
	}

	trustBundlePath := filepath.Join(pkiDir, "trust", "hub-bundle.pem")
	trustPEM, err := os.ReadFile(trustBundlePath)
	if err != nil {
		t.Skipf("Trust bundle not found at %s: %v. Run './g8e platform clean && ./g8e platform start'.", trustBundlePath, err)
	}
	rootPool := x509.NewCertPool()
	require.True(t, rootPool.AppendCertsFromPEM(trustPEM), "failed to parse trust bundle")

	_, opPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	opCsrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "g8e-op-smoke"}}, opPriv)
	require.NoError(t, err)
	opCsrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: opCsrDER})

	_, cliPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cliCsrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "g8e-cli-smoke"}}, cliPriv)
	require.NoError(t, err)
	cliCsrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: cliCsrDER})

	fpHash := sha256.Sum256([]byte("g8e-mcp-smoke-test"))
	fingerprint := hex.EncodeToString(fpHash[:])

	reqBody, _ := json.Marshal(map[string]string{
		"email":              "mcp-smoke@test.local",
		"name":               "MCP Smoke Tester",
		"csr_pem":            string(opCsrPEM),
		"cli_csr_pem":        string(cliCsrPEM),
		"system_fingerprint": fingerprint,
	})

	trustClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: rootPool}},
	}

	httpReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:%d/api/auth/bootstrap", constants.Ports.OperatorBootstrap), bytes.NewReader(reqBody))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	bootstrapResp, err := trustClient.Do(httpReq)
	require.NoError(t, err)
	defer bootstrapResp.Body.Close()

	respBytes, _ := io.ReadAll(bootstrapResp.Body)
	if bootstrapResp.StatusCode == http.StatusForbidden || bootstrapResp.StatusCode == http.StatusConflict {
		t.Skipf("Platform already bootstrapped (status %d): %s. Run './g8e platform clean && ./g8e platform start' to reset.", bootstrapResp.StatusCode, string(respBytes))
	}
	require.Equal(t, http.StatusCreated, bootstrapResp.StatusCode, "bootstrap failed: %s", string(respBytes))

	var regResp models.OperatorRegistrationResponse
	require.NoError(t, json.Unmarshal(respBytes, &regResp))
	require.NotEmpty(t, regResp.CLICert, "bootstrap did not return CLI cert")
	require.NotEmpty(t, regResp.OperatorSessionID)
	require.NotEmpty(t, regResp.CLISessionID)

	cliPrivBytes, err := x509.MarshalPKCS8PrivateKey(cliPriv)
	require.NoError(t, err)
	cliKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: cliPrivBytes})
	cliCert, err := tls.X509KeyPair([]byte(regResp.CLICert), cliKeyPEM)
	require.NoError(t, err)

	mtlsClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cliCert},
			},
		},
	}

	listReq, err := http.NewRequest(http.MethodGet, operatorURL+"/api/mcp/v1/tools/list", nil)
	require.NoError(t, err)
	listReq.Header.Set("Authorization", "Bearer "+regResp.OperatorSessionID)
	listReq.Header.Set("X-G8E-CLI-Session-ID", regResp.CLISessionID)
	listReq.Header.Set("X-G8E-Source-Component", "client")

	listResp, err := mtlsClient.Do(listReq)
	require.NoError(t, err)
	defer listResp.Body.Close()

	listBody, _ := io.ReadAll(listResp.Body)
	require.Equal(t, http.StatusOK, listResp.StatusCode, "tools/list failed: %s", string(listBody))
	require.Contains(t, string(listBody), "jsonrpc")
	require.Contains(t, string(listBody), "tools")
}
