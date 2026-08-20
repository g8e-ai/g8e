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
