// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// mockActuatorKeyReader is a test implementation of actuatorKeyReader.
type mockActuatorKeyReader struct {
	keyID     string
	publicKey string
	err       error
}

func (m *mockActuatorKeyReader) ReadActuatorPublicKey() (keyID, publicKey string, err error) {
	return m.keyID, m.publicKey, m.err
}

func TestFileActuatorKeyReader(t *testing.T) {
	t.Run("Success - reads valid actuator key file", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		keyFile := filepath.Join(tmpDir, constants.ActuatorPubJSONFilename)
		keyData := `{"key_id":"test-key-id","public_key":"test-public-key"}`
		require.NoError(t, os.WriteFile(keyFile, []byte(keyData), 0644))

		reader := &fileActuatorKeyReader{path: keyFile}
		keyID, publicKey, err := reader.ReadActuatorPublicKey()

		require.NoError(t, err)
		assert.Equal(t, "test-key-id", keyID)
		assert.Equal(t, "test-public-key", publicKey)
	})

	t.Run("Failure - file does not exist", func(t *testing.T) {
		reader := &fileActuatorKeyReader{path: "/nonexistent/path/" + constants.ActuatorPubJSONFilename}
		_, _, err := reader.ReadActuatorPublicKey()

		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("Failure - invalid JSON in file", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		keyFile := filepath.Join(tmpDir, constants.ActuatorPubJSONFilename)
		require.NoError(t, os.WriteFile(keyFile, []byte("{invalid json"), 0644))

		reader := &fileActuatorKeyReader{path: keyFile}
		_, _, err := reader.ReadActuatorPublicKey()

		assert.Error(t, err)
	})

	t.Run("Success - missing required fields returns empty values", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		keyFile := filepath.Join(tmpDir, constants.ActuatorPubJSONFilename)
		require.NoError(t, os.WriteFile(keyFile, []byte(`{"key_id":"test-id"}`), 0644))

		reader := &fileActuatorKeyReader{path: keyFile}
		keyID, publicKey, err := reader.ReadActuatorPublicKey()

		require.NoError(t, err)
		assert.Equal(t, "test-id", keyID)
		assert.Empty(t, publicKey)
	})
}

func TestHandleBootstrapWithURL(t *testing.T) {
	t.Run("Success - Bootstrap with CSR creates the first real user", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
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
		// Bootstrap creates exactly one real user (the first human enrollee
		// and gateway admin). There is no ephemeral bootstrap-user concept.
		hasUsers, err := c.userSvc.HasAnyUsers()
		require.NoError(t, err)
		assert.True(t, hasUsers, "bootstrap must create the first real user")
	})

	t.Run("Failure - Rejects bootstrap if ANY other users exist", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
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
	t.Run("Fresh gateway is bootstrapped but not activated", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
		rr := httptest.NewRecorder()
		c.handleBootstrapStatus(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.BootstrapStatusResponse
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		// bootstrapped is always true when the listener responds; the
		// endpoint existing IS the proof of infrastructure being up.
		assert.True(t, resp.Bootstrapped, "bootstrapped is always true when the endpoint responds")
		// activated is false until a human enrolls as the first user.
		assert.False(t, resp.Activated, "activated is false on a fresh gateway with no users")
	})

	t.Run("Activated after creating the first user", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
		_, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
		rr := httptest.NewRecorder()
		c.handleBootstrapStatus(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.BootstrapStatusResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Bootstrapped)
		assert.True(t, resp.Activated, "activated flips to true once the first user exists")
	})
}

func TestHandleOperatorEnrollment(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/operator/enroll", nil)
		rr := httptest.NewRecorder()

		c.handleOperatorEnrollment(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/operator/enroll", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handleOperatorEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrInvalidJSONBody.Error())
	})

	t.Run("Failure - missing csr_pem", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
		body := map[string]string{
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/operator/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleOperatorEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrCSRRequired.Error())
	})

	t.Run("Failure - missing system_fingerprint", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
		body := map[string]string{
			"csr_pem":  testutil.GenerateTestCSRP256(t, "test-operator"),
			"hostname": "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/operator/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleOperatorEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrSystemFingerprintRequired.Error())
	})

	t.Run("Failure - missing hostname", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-operator"),
			"system_fingerprint": "fp-123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/operator/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleOperatorEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrHostnameRequired.Error())
	})

	t.Run("Failure - rejects before activation with no users", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
		// Fresh gateway: no users exist, so the gateway is not activated.
		// Operator enrollment must be refused until a human runs
		// `auth enroll user` to create the first user.
		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-operator"),
			"cli_csr_pem":        testutil.GenerateTestCSRP256(t, "test-cli"),
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/operator/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleOperatorEnrollment(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrOperatorEnrollmentRequiresActivation.Error())
	})

	t.Run("Failure - missing cli_csr_pem on activated gateway", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
		// The CLI CSR check runs after the activation gate, so the gateway
		// must be activated (a user must exist) for this 400 to surface
		// instead of the 403 activation gate.
		_, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-operator"),
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/operator/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleOperatorEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrCLICSRRequired.Error())
	})

	t.Run("Success - after activation creates no user and binds empty user_id", func(t *testing.T) {
		c, cfg := setupTestBootstrapController(t)
		// Activate the gateway by creating the first real user. Operator
		// enrollment is only available once a human has bootstrapped the
		// gateway via `auth enroll user`.
		firstUser, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		c.actuatorKeyReader = &mockActuatorKeyReader{
			keyID:     "act-123",
			publicKey: "pub-123",
		}

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-operator"),
			"cli_csr_pem":        testutil.GenerateTestCSRP256(t, "test-cli"),
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/operator/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleOperatorEnrollment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp models.OperatorEnrollmentResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotEmpty(t, resp.OperatorCert)
		assert.NotEmpty(t, resp.CLICert)
		assert.NotEmpty(t, resp.OperatorSessionID)
		assert.NotEmpty(t, resp.CLISessionID)
		assert.NotEqual(t, resp.OperatorSessionID, resp.CLISessionID,
			"cli_session_id MUST be a distinct identifier from operator_session_id")
		assert.Equal(t, "act-123", resp.ActuatorKeyID)
		assert.Equal(t, "pub-123", resp.ActuatorPubKey)
		// The gateway owns the posture and must send it to the operator at
		// enrollment so the operator can run L4 posture-gated checks against
		// the gateway's posture rather than inventing its own.
		assert.Equal(t, string(cfg.Gateway.Posture), resp.Posture)

		// Operator enrollment creates NO users: the first (and only) user is
		// still the one created for activation, and it remains the first user
		// (admin). No new user was created by the enrollment.
		isFirst, err := c.userSvc.IsFirstUser(firstUser.ID)
		require.NoError(t, err)
		assert.True(t, isFirst, "operator enrollment must not create a new user; the first user remains the sole user")

		// The persisted Operator document binds an empty user_id: operator
		// identity is certificate-based (SPIFFE URI SAN), not user-bound.
		opDoc, err := c.docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), resp.OperatorID)
		require.NoError(t, err)
		require.NotNil(t, opDoc)
		dataBytes, err := json.Marshal(opDoc.Data)
		require.NoError(t, err)
		var op models.OperatorDocumentGo
		require.NoError(t, json.Unmarshal(dataBytes, &op))
		assert.Empty(t, op.UserID, "operator document must bind an empty user_id (operator identity is certificate-based)")
		assert.Empty(t, op.OrganizationID, "operator document must bind an empty organization_id")

		// The persisted CLI session binds an empty user_id: user binding is a
		// separate human-only action performed later via `auth enroll user`.
		cliSession, err := c.cliSessionSvc.loadCLISession(resp.CLISessionID)
		require.NoError(t, err)
		assert.Empty(t, cliSession.UserID, "CLI session must bind an empty user_id (operator enrollment is not user-bound)")
	})

	t.Run("Success - actuator key reader error (graceful degradation)", func(t *testing.T) {
		c, _ := setupTestBootstrapController(t)
		// Activate the gateway so the enrollment reaches the actuator key
		// read path instead of the activation gate.
		_, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		c.actuatorKeyReader = &mockActuatorKeyReader{
			err: fmt.Errorf("failed to read actuator key: %w", constants.ErrPathNotFound),
		}

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-operator"),
			"cli_csr_pem":        testutil.GenerateTestCSRP256(t, "test-cli"),
			"system_fingerprint": "fp-125",
			"hostname":           "test-host-3",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/operator/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleOperatorEnrollment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp models.OperatorEnrollmentResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Empty(t, resp.ActuatorKeyID)
		assert.Empty(t, resp.ActuatorPubKey)
	})
}
