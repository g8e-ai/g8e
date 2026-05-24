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
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/marshaler"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				_, _ = db.DocDelete(marshaler.CollectionName(constants.CollectionTrustedSigners), appID)
				if pki.secretManager != nil {
					_ = pki.secretManager.DeleteServicePrivateKey(appID)
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
				tt.req.CSR = testutil.GenerateTestCSR(t, tt.req.AppName)
			} else if tt.req.CSR == "" && !tt.wantSuccess && tt.wantError != "csr_pem is required" {
				// For negative tests that need a CSR to validate other fields
				tt.req.CSR = testutil.GenerateTestCSR(t, tt.req.AppName)
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

func TestAppEnrollmentService_RollbackOnFailure(t *testing.T) {
	// Not running in parallel because setupTestGatewayService resets global keystore state
	gateway, _ := setupTestGatewayService(t)
	db := gateway.db
	logger := gateway.logger
	pki := gateway.pki
	_ = NewAppEnrollmentService(db, pki, logger)

	t.Run("rollback deletes signer and private key on CSR signing failure", func(t *testing.T) {
		// Not running in parallel

		// This test requires mocking PKI.SignCSR to fail
		// For now, we'll skip this as it requires more complex test setup
		t.Skip("requires PKI.SignCSR failure mock")
	})
}

func TestHandleAppPolicySigner(t *testing.T) {
	// Not running in parallel because setupTestGatewayService resets global keystore state
	gateway, _ := setupTestGatewayService(t)
	db := gateway.db
	handler := gateway.handler
	userSvc := gateway.userSvc

	// Create a bootstrap user for admin authorization
	bootstrapUser, err := userSvc.CreateBootstrapUser()
	require.NoError(t, err)
	require.NotNil(t, bootstrapUser)
	t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionUsers), bootstrapUser.ID) })

	// Create a non-bootstrap user for testing unauthorized access
	regularUser, err := userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionUsers), regularUser.ID) })

	t.Run("reject signer registration without admin authorization", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-no-auth"
		pubKeyHex := "a" + strings.Repeat("0", 63)

		// Create AppPolicy
		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionAppPolicies), appID) })

		// Create request with regular user context (non-bootstrap)
		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/app-policies/" + appID + "/signer"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, regularUser.ID))

		w := httptest.NewRecorder()
		handler.handleAppPolicySigner(w, req)

		// Verify 403 Forbidden due to non-bootstrap user
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "admin-only")
	})

	t.Run("reject signer registration without user context", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-no-context"
		pubKeyHex := "a" + strings.Repeat("0", 63)

		// Create AppPolicy
		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionAppPolicies), appID) })

		// Create request without user context
		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/app-policies/" + appID + "/signer"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}

		w := httptest.NewRecorder()
		handler.handleAppPolicySigner(w, req)

		// Verify 401 Unauthorized
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "unauthorized")
	})

	t.Run("reject signer registration without AppPolicy", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-no-policy"
		pubKeyHex := "a" + strings.Repeat("0", 63) // 64 hex chars = 32 bytes

		// Create request with bootstrap user context
		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/app-policies/" + appID + "/signer"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		// Create response recorder
		w := httptest.NewRecorder()

		// Call handler
		handler.handleAppPolicySigner(w, req)

		// Verify 403 Forbidden
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "app policy not found")
	})

	t.Run("reject signer registration with invalid public key size", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-invalid-key"

		// Create AppPolicy first
		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionAppPolicies), appID) })

		// Invalid public key (odd number of hex chars = invalid hex)
		pubKeyHex := "a" + strings.Repeat("0", 30) // 31 hex chars = invalid hex

		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/app-policies/" + appID + "/signer"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		w := httptest.NewRecorder()
		handler.handleAppPolicySigner(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid hex")
	})

	t.Run("successfully register signer with valid AppPolicy", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-valid-signer"

		// Create AppPolicy
		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionAppPolicies), appID) })

		// Valid Ed25519 public key (64 hex chars = 32 bytes)
		pubKeyHex := strings.Repeat("0", 64)

		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/app-policies/" + appID + "/signer"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		w := httptest.NewRecorder()
		handler.handleAppPolicySigner(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		// Verify signer was registered
		signerDoc, err := db.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), appID)
		require.NoError(t, err)
		require.NotNil(t, signerDoc)
		assert.Equal(t, appID, signerDoc.ID)

		var signer models.TrustedSigner
		signerData, _ := json.Marshal(signerDoc.Data)
		err = json.Unmarshal(signerData, &signer)
		require.NoError(t, err)
		assert.Equal(t, pubKeyHex, signer.PublicKey)
		assert.True(t, signer.Enabled)

		t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionTrustedSigners), appID) })
	})

	t.Run("successfully register signer with SPIFFE ID containing colons", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-mcp-client"

		// Create AppPolicy
		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionAppPolicies), appID) })

		// Valid Ed25519 public key (64 hex chars = 32 bytes)
		pubKeyHex := strings.Repeat("a", 64)

		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/app-policies/" + appID + "/signer"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		w := httptest.NewRecorder()
		handler.handleAppPolicySigner(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		// Verify signer was registered with full SPIFFE ID
		signerDoc, err := db.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), appID)
		require.NoError(t, err)
		require.NotNil(t, signerDoc)
		assert.Equal(t, appID, signerDoc.ID)

		var signer models.TrustedSigner
		signerData, _ := json.Marshal(signerDoc.Data)
		err = json.Unmarshal(signerData, &signer)
		require.NoError(t, err)
		assert.Equal(t, pubKeyHex, signer.PublicKey)
		assert.True(t, signer.Enabled)

		t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionTrustedSigners), appID) })
	})
}

func TestHandleRevokeApp(t *testing.T) {
	// Not running in parallel because setupTestGatewayService resets global keystore state
	gateway, _ := setupTestGatewayService(t)
	db := gateway.db
	handler := gateway.handler
	userSvc := gateway.userSvc

	// Create a bootstrap user for admin authorization
	bootstrapUser, err := userSvc.CreateBootstrapUser()
	require.NoError(t, err)
	require.NotNil(t, bootstrapUser)
	t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionUsers), bootstrapUser.ID) })

	// Create a non-bootstrap user for testing unauthorized access
	regularUser, err := userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionUsers), regularUser.ID) })

	t.Run("reject app revocation without admin authorization", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-no-auth"

		// Create AppPolicy
		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionAppPolicies), appID) })

		// Create request with regular user context (non-bootstrap)
		reqBody := map[string]string{"app_id": appID}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/revoke-app"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, regularUser.ID))

		w := httptest.NewRecorder()
		handler.handleRevokeApp(w, req)

		// Verify 403 Forbidden due to non-bootstrap user
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "admin-only")
	})

	t.Run("reject app revocation without user context", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-no-context"

		// Create AppPolicy
		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete(marshaler.CollectionName(constants.CollectionAppPolicies), appID) })

		// Create request without user context
		reqBody := map[string]string{"app_id": appID}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/revoke-app"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}

		w := httptest.NewRecorder()
		handler.handleRevokeApp(w, req)

		// Verify 401 Unauthorized
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "unauthorized")
	})

	t.Run("reject app revocation with missing app_id", func(t *testing.T) {
		// Create request with bootstrap user context but missing app_id
		reqBody := map[string]string{}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/revoke-app"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		w := httptest.NewRecorder()
		handler.handleRevokeApp(w, req)

		// Verify 400 Bad Request
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "app_id required")
	})

	t.Run("successfully revoke app with policy only", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-revoke-policy-only"

		// Create AppPolicy
		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes)
		require.NoError(t, err)

		// Verify policy exists
		policyDoc, err := db.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
		require.NoError(t, err)
		require.NotNil(t, policyDoc)

		// Create request with bootstrap user context
		reqBody := map[string]string{"app_id": appID}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/revoke-app"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		w := httptest.NewRecorder()
		handler.handleRevokeApp(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify policy was deleted
		policyDoc, err = db.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
		require.NoError(t, err)
		assert.Nil(t, policyDoc)
	})

	t.Run("successfully revoke app with policy and signer", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-revoke-with-signer"

		// Create AppPolicy
		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes)
		require.NoError(t, err)

		// Create TrustedSigner
		signer := models.TrustedSigner{
			ID:        appID,
			PublicKey: strings.Repeat("a", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		signerBytes, _ := json.Marshal(signer)
		err = db.DocSet(marshaler.CollectionName(constants.CollectionTrustedSigners), appID, signerBytes)
		require.NoError(t, err)

		// Verify both exist
		policyDoc, err := db.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
		require.NoError(t, err)
		require.NotNil(t, policyDoc)

		signerDoc, err := db.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), appID)
		require.NoError(t, err)
		require.NotNil(t, signerDoc)

		// Create request with bootstrap user context
		reqBody := map[string]string{"app_id": appID}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/revoke-app"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		w := httptest.NewRecorder()
		handler.handleRevokeApp(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify both were deleted
		policyDoc, err = db.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
		require.NoError(t, err)
		assert.Nil(t, policyDoc)

		signerDoc, err = db.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), appID)
		require.NoError(t, err)
		assert.Nil(t, signerDoc)
	})

	t.Run("successfully revoke app with SPIFFE ID containing colons", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-mcp-client"

		// Create AppPolicy
		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes)
		require.NoError(t, err)

		// Create TrustedSigner
		signer := models.TrustedSigner{
			ID:        appID,
			PublicKey: strings.Repeat("b", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		signerBytes, _ := json.Marshal(signer)
		err = db.DocSet(marshaler.CollectionName(constants.CollectionTrustedSigners), appID, signerBytes)
		require.NoError(t, err)

		// Create request with bootstrap user context
		reqBody := map[string]string{"app_id": appID}
		bodyBytes, _ := json.Marshal(reqBody)
		req := &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: "/api/admin/revoke-app"},
			Body:   io.NopCloser(bytes.NewReader(bodyBytes)),
		}
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		w := httptest.NewRecorder()
		handler.handleRevokeApp(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify both were deleted with full SPIFFE ID
		policyDoc, err := db.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
		require.NoError(t, err)
		assert.Nil(t, policyDoc)

		signerDoc, err := db.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), appID)
		require.NoError(t, err)
		assert.Nil(t, signerDoc)
	})
}
