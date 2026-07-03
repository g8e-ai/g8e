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

//go:build integration

package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestHandleBootstrapWithURL(t *testing.T) {
	t.Run("Success - Bootstrap with CSR", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		cliCsr := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleLocalBootstrapWithURL(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["operator_cert"])
		assert.NotEmpty(t, resp["operator_cert_chain"])
		assert.NotEmpty(t, resp["hub_trust_bundle"])
		assert.NotEmpty(t, resp["operator_session_id"])
		assert.NotEmpty(t, resp["cli_session_id"])
		assert.NotEqual(t, resp["operator_session_id"], resp["cli_session_id"],
			"cli_session_id MUST be a distinct identifier from operator_session_id")
	})

	t.Run("Success - Rotation for existing bootstrap user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateBootstrapUserWithOSUser(nil)
		require.NoError(t, err)
		require.NotNil(t, user)

		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		cliCsr := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "rotated-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleLocalBootstrapWithURL(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["operator_cert"])
	})

	t.Run("Failure - Rotation fails for disabled bootstrap user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, _ := c.userSvc.CreateBootstrapUserWithOSUser(nil)
		c.userSvc.Disable(user.ID, "retired", "actor", "op")

		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		cliCsr := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "fail-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleLocalBootstrapWithURL(rr, req)

		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.JSONEq(t, `{"error":"`+constants.ErrBootstrapUserDisabled.Error()+`"}`, rr.Body.String())
	})

	t.Run("Failure - Rejects bootstrap if ANY other users exist", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		c.userSvc.CreateUser()

		body := map[string]string{
			"name": "Superadmin",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleLocalBootstrapWithURL(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.JSONEq(t, `{"error":"`+constants.ErrBootstrapInitialSetupOnly.Error()+`"}`, rr.Body.String())
	})
}

func TestHandleBootstrapStatus(t *testing.T) {
	t.Run("Initially not bootstrapped", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
		rr := httptest.NewRecorder()
		c.handleBootstrapStatus(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp["bootstrapped"].(bool))
	})

	t.Run("Bootstrapped after creating a user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		_, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
		rr := httptest.NewRecorder()
		c.handleBootstrapStatus(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["bootstrapped"].(bool))
	})
}

func TestHandleCLIEnrollment(t *testing.T) {
	t.Run("Success - CLI enrollment after bootstrap", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUserWithOSUser(nil)
		require.NoError(t, err)
		require.NotNil(t, bootstrapUser)

		cliCSR := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"cli_csr_pem":        cliCSR,
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIEnrollment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["cli_cert"])
		assert.NotEmpty(t, resp["cli_cert_chain"])
		assert.NotEmpty(t, resp["cli_session_id"])
		assert.NotEmpty(t, resp["user_id"])
		assert.NotEmpty(t, resp["hub_trust_bundle"])
		assert.NotEmpty(t, resp["operator_session_id"])
		assert.NotEmpty(t, resp["operator_id"])
	})

	t.Run("Failure - Rejected when not bootstrapped", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)

		cliCSR := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"cli_csr_pem":        cliCSR,
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli/enroll", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleCLIEnrollment(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrCLIEnrollmentAfterBootstrap.Error())
	})

	t.Run("Failure - Rejected when bootstrap user disabled", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUserWithOSUser(nil)
		require.NoError(t, err)
		c.userSvc.Disable(bootstrapUser.ID, "retired", "actor", "op")

		cliCSR := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"cli_csr_pem":        cliCSR,
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli/enroll", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleCLIEnrollment(rr, req)

		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrBootstrapUserDisabledEnroll.Error())
	})

	t.Run("Failure - Missing cli_csr_pem", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUserWithOSUser(nil)
		require.NoError(t, err)
		require.NotNil(t, bootstrapUser)

		body := map[string]string{
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli/enroll", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleCLIEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrCLICSRRequired.Error())
	})

	t.Run("Failure - Method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUserWithOSUser(nil)
		require.NoError(t, err)
		require.NotNil(t, bootstrapUser)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli/enroll", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleCLIEnrollment(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestHandleDeviceEnrollment(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/device/enroll", nil)
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrInvalidJSONBody.Error())
	})

	t.Run("Failure - missing csr_pem", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrCSRRequired.Error())
	})

	t.Run("Failure - missing system_fingerprint", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"csr_pem":  testutil.GenerateTestCSRP256(t, "test-device"),
			"hostname": "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrSystemFingerprintRequired.Error())
	})

	t.Run("Failure - missing hostname", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"system_fingerprint": "fp-123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrHostnameRequired.Error())
	})

	t.Run("Failure - bootstrap user disabled", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUserWithOSUser(nil)
		require.NoError(t, err)
		c.userSvc.Disable(bootstrapUser.ID, "test", "actor", "op")

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrBootstrapUserDisabledEnroll.Error())
	})

	t.Run("Failure - device enrollment on non-empty system", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		// Create a regular user to make system non-empty
		c.userSvc.CreateUser()

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrDeviceEnrollmentInitialOnly.Error())
	})

	t.Run("Failure - missing cli_csr_pem", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrCLICSRMandatory.Error())
	})

	t.Run("Success - initial bootstrap", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)

		// Mock actuator key reader
		c.actuatorKeyReader = &mockActuatorKeyReader{
			keyID:     "act-123",
			publicKey: "pub-123",
		}

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"cli_csr_pem":        testutil.GenerateTestCSRP256(t, "test-cli"),
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp models.DeviceEnrollmentResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotEmpty(t, resp.OperatorCert)
		assert.NotEmpty(t, resp.CLICert)
		assert.Equal(t, "act-123", resp.ActuatorKeyID)
		assert.Equal(t, "pub-123", resp.ActuatorPubKey)

		// Verify user was created
		user, err := c.userSvc.GetByID(resp.UserID)
		require.NoError(t, err)
		assert.NotNil(t, user)
		assert.True(t, user.IsBootstrap)
	})

	t.Run("Success - existing bootstrap user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUserWithOSUser(nil)
		require.NoError(t, err)

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"cli_csr_pem":        testutil.GenerateTestCSRP256(t, "test-cli"),
			"system_fingerprint": "fp-124",
			"hostname":           "test-host-2",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp models.DeviceEnrollmentResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, bootstrapUser.ID, resp.UserID)
	})

	t.Run("Success - actuator key reader error (graceful degradation)", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)

		// Mock actuator key reader that returns an error
		c.actuatorKeyReader = &mockActuatorKeyReader{
			err: fmt.Errorf("failed to read actuator key: %w", constants.ErrPathNotFound),
		}

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"cli_csr_pem":        testutil.GenerateTestCSRP256(t, "test-cli"),
			"system_fingerprint": "fp-125",
			"hostname":           "test-host-3",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp models.DeviceEnrollmentResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Empty(t, resp.ActuatorKeyID)
		assert.Empty(t, resp.ActuatorPubKey)
	})
}
