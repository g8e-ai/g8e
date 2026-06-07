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

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestAppEnrollmentService_EnrollApp(t *testing.T) {
	// Not running in parallel because setupTestGatewayService resets global keystore state

	// Setup test infrastructure (PKI is now initialized by setupTestGatewayService)
	gateway, _ := setupTestGatewayService(t)
	db := gateway.db
	logger := gateway.logger
	pki := gateway.pki

	// Create AppEnrollmentService
	appEnrollment := NewAppEnrollmentService(db, pki, logger)

	// Test cases
	tests := []struct {
		name        string
		req         AppEnrollRequest
		setup       func() // Setup function to prepare test state
		wantSuccess bool
		wantError   string
		teardown    func() // Teardown function to clean up test state
	}{
		{
			name: "successful enrollment with valid CSR",
			req: AppEnrollRequest{
				AppName:        "test-mcp-client",
				AppType:        "mcp-client",
				OrganizationID: "test-org",
			},
			setup: func() {
				// CSR will be generated in the test runner
			},
			wantSuccess: true,
			teardown: func() {
				// Clean up the enrolled app (identity-only, no signer to delete)
				appID := "spiffe://g8e.local/app/test-mcp-client"
				if _, err := db.DocDelete(marshaler.CollectionName(constants.CollectionTrustedSigners), appID); err != nil {
					t.Logf("Failed to delete signer document: %v", err)
				}
				if pki.secretManager != nil {
					if err := pki.secretManager.DeleteServicePrivateKey(appID); err != nil {
						t.Logf("Failed to delete service private key: %v", err)
					}
				}
			},
		},
		{
			name: "reject enrollment with missing CSR",
			req: AppEnrollRequest{
				AppName:        "test-app",
				AppType:        "mcp-client",
				OrganizationID: "test-org",
			},
			wantSuccess: false,
			wantError:   "csr_pem is required",
		},
		{
			name: "reject enrollment with missing app name",
			req: AppEnrollRequest{
				CSR:            testutil.GenerateTestCSR(t, "test-app"),
				AppType:        "mcp-client",
				OrganizationID: "test-org",
			},
			wantSuccess: false,
			wantError:   "app_name is required",
		},
		{
			name: "reject enrollment with missing app type",
			req: AppEnrollRequest{
				CSR:            testutil.GenerateTestCSR(t, "test-app"),
				AppName:        "test-app",
				OrganizationID: "test-org",
			},
			wantSuccess: false,
			wantError:   "app_type is required",
		},
		{
			name: "reject enrollment with invalid app type",
			req: AppEnrollRequest{
				CSR:            testutil.GenerateTestCSR(t, "test-app"),
				AppName:        "test-app",
				AppType:        "invalid-type",
				OrganizationID: "test-org",
			},
			wantSuccess: false,
			wantError:   "invalid app_type",
		},
		{
			name: "reject enrollment with invalid app name (special chars)",
			req: AppEnrollRequest{
				CSR:            testutil.GenerateTestCSR(t, "test@app"),
				AppName:        "test@app",
				AppType:        "mcp-client",
				OrganizationID: "test-org",
			},
			wantSuccess: false,
			wantError:   "app_name must contain only alphanumeric characters",
		},
		{
			name: "reject enrollment with invalid app name (spaces)",
			req: AppEnrollRequest{
				CSR:            testutil.GenerateTestCSR(t, "test app"),
				AppName:        "test app",
				AppType:        "mcp-client",
				OrganizationID: "test-org",
			},
			wantSuccess: false,
			wantError:   "app_name must contain only alphanumeric characters",
		},
		{
			name: "reject enrollment with invalid CSR PEM format",
			req: AppEnrollRequest{
				CSR:            "invalid-pem-data",
				AppName:        "test-app",
				AppType:        "mcp-client",
				OrganizationID: "test-org",
			},
			wantSuccess: false,
			wantError:   "invalid CSR PEM format",
		},
		{
			name: "reject enrollment with malformed CSR",
			req: AppEnrollRequest{
				CSR:            "-----BEGIN CERTIFICATE REQUEST-----\nMIICZzCCAT8CAQAwFjEUMBIGA1UEAwwLdGVzdC1hcHAwggEiMA0GCSqGSIb3DQEB\n-----END CERTIFICATE REQUEST-----",
				AppName:        "test-app",
				AppType:        "mcp-client",
				OrganizationID: "test-org",
			},
			wantSuccess: false,
			wantError:   "failed to parse CSR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Not running in parallel since test cases share gateway DB state

			// Run setup if provided
			if tt.setup != nil {
				tt.setup()
			}

			// If the test requires a CSR, generate it
			if tt.req.CSR == "" && tt.wantSuccess {
				tt.req.CSR = testutil.GenerateTestCSRP256(t, tt.req.AppName)
			} else if tt.req.CSR == "" && !tt.wantSuccess && tt.wantError != "csr_pem is required" {
				// For negative tests that need a CSR to validate other fields
				tt.req.CSR = testutil.GenerateTestCSRP256(t, tt.req.AppName)
			}

			// Execute enrollment
			resp, err := appEnrollment.EnrollApp(tt.req)

			// Verify response
			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if !tt.wantSuccess {
				assert.Contains(t, resp.Error, tt.wantError)
			} else {
				// Log the error if enrollment failed unexpectedly
				if !resp.Success && resp.Error != "" {
					t.Logf("Enrollment failed with error: %s", resp.Error)
				}
				assert.NotEmpty(t, resp.AppCert)
				assert.NotEmpty(t, resp.CertChain)
				assert.NotEmpty(t, resp.AppID)

				// Verify the L2 signer was NOT registered automatically (identity-only enrollment)
				signerDoc, err := db.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), resp.AppID)
				require.NoError(t, err)
				require.Nil(t, signerDoc)

				// Verify no L2 private key was stored
				_, err = pki.secretManager.GetServicePrivateKey(resp.AppID)
				require.Error(t, err)
			}

			// Run teardown if provided
			if tt.teardown != nil {
				tt.teardown()
			}
		})
	}
}

func TestIsValidAppName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid alphanumeric",
			input: "testapp123",
			want:  true,
		},
		{
			name:  "valid with hyphens",
			input: "test-app-name",
			want:  true,
		},
		{
			name:  "valid with underscores",
			input: "test_app_name",
			want:  true,
		},
		{
			name:  "valid mixed",
			input: "Test-App_123",
			want:  true,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "contains space",
			input: "test app",
			want:  false,
		},
		{
			name:  "contains special char",
			input: "test@app",
			want:  false,
		},
		{
			name:  "contains slash",
			input: "test/app",
			want:  false,
		},
		{
			name:  "contains dot",
			input: "test.app",
			want:  false,
		},
		{
			name:  "starts with hyphen",
			input: "-testapp",
			want:  true,
		},
		{
			name:  "ends with hyphen",
			input: "testapp-",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isValidAppName(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

