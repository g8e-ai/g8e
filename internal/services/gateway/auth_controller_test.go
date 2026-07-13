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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/storage"
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

func setupTestAuthController(t *testing.T) (*AuthController, *config.Config) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	pkiDir := testutil.TempDir(t)
	secretsDir := testutil.TempDir(t)
	ks := newTestKeystore(t, secretsDir, logger)
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, constants.VaultDirname), logger, true, "", false, ks)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	sm := &SecretManager{
		db:         db.db,
		secretsDir: secretsDir,
		logger:     logger,
		keystore:   ks,
	}

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	resp := response.NewWriter(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, resp, secretsDir, nil, "", "", "")
	cliSessionSvc := NewCLISessionService(db, logger)
	operatorSessionSvc := NewOperatorSessionService(db, logger)
	webSessionSvc := NewWebSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, &cfg.Gateway)
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})

	// Initialize suspended transaction service for tests
	suspendedTxConfig := &storage.SuspendedTransactionConfig{
		DBPath:               filepath.Join(dbDir, constants.SuspendedTxFilename),
		MaxDBSizeMB:          256,
		RetentionDays:        7,
		PruneIntervalMinutes: 30,
	}
	suspendedTxService, err := storage.NewSuspendedTransactionService(suspendedTxConfig, logger)
	if err != nil {
		t.Fatalf("failed to create suspended transaction service: %v", err)
	}
	t.Cleanup(func() { suspendedTxService.Close() })

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:           logger,
		Responder:        resp,
		SuspendedStore:   suspendedTxService,
		ScrubbingService: nil,
		MaxPayloadBytes:  cfg.Gateway.MaxPayloadBytes,
		Posture:          string(cfg.Gateway.Posture),
	})
	if err != nil {
		t.Fatalf("failed to create MCP gateway: %v", err)
	}

	passkeyHandler := NewPasskeyHandler(PasskeyHandlerDeps{
		Service:        passkey,
		WebSessionSvc:  webSessionSvc,
		Responder:      resp,
		MaxPayload:     cfg.Gateway.MaxPayloadBytes,
		MCPSvc:         mcpGateway,
		SuspendedStore: suspendedTxService,
	})

	enrollmentTokenSvc := NewEnrollmentTokenService(db, logger)
	authController := newAuthController(AuthControllerDeps{
		Cfg:                cfg,
		Logger:             logger,
		DB:                 db,
		Auth:               auth,
		Passkey:            passkeyHandler,
		UserSvc:            userSvc,
		Reg:                reg,
		PKI:                pki,
		WebSessionSvc:      webSessionSvc,
		CLISessionSvc:      cliSessionSvc,
		OperatorSessionSvc: operatorSessionSvc,
		EnrollmentTokenSvc: enrollmentTokenSvc,
		Responder:          resp,
		ActuatorKeyReader:  nil,
		CrossOrigin:        false,
	})
	return authController, cfg
}

// setupTestPasskeyService creates a PasskeyHandler with approval dependencies for testing.
func setupTestPasskeyService(t *testing.T) (*PasskeyHandler, *UserService, storage.SuspendedTransactionStore) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	db, err := openTestDB(t, dbDir, testutil.TempDir(t), filepath.Join(dbDir, constants.VaultDirname), logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	userSvc := NewUserService(db, logger)
	resp := response.NewWriter(logger)
	webSessionSvc := NewWebSessionService(db, logger)
	passkey, err := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
	require.NoError(t, err)

	suspendedTxConfig := &storage.SuspendedTransactionConfig{
		DBPath:               filepath.Join(dbDir, constants.SuspendedTxFilename),
		MaxDBSizeMB:          256,
		RetentionDays:        7,
		PruneIntervalMinutes: 30,
	}
	suspendedTxService, err := storage.NewSuspendedTransactionService(suspendedTxConfig, logger)
	require.NoError(t, err)
	t.Cleanup(func() { suspendedTxService.Close() })

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:           logger,
		Responder:        resp,
		SuspendedStore:   suspendedTxService,
		ScrubbingService: nil,
		MaxPayloadBytes:  cfg.Gateway.MaxPayloadBytes,
		Posture:          string(cfg.Gateway.Posture),
	})
	require.NoError(t, err)

	passkeyHandler := NewPasskeyHandler(PasskeyHandlerDeps{
		Service:        passkey,
		WebSessionSvc:  webSessionSvc,
		Responder:      resp,
		MaxPayload:     cfg.Gateway.MaxPayloadBytes,
		MCPSvc:         mcpGateway,
		SuspendedStore: suspendedTxService,
	})
	return passkeyHandler, userSvc, suspendedTxService
}

// Test Helpers

func testMethodNotAllowed(t *testing.T, handler http.HandlerFunc, method, url string) {
	t.Helper()
	req := httptest.NewRequest(method, url, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func testInvalidJSON(t *testing.T, handler http.HandlerFunc, method, url string) {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader("{invalid}"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func testMissingUserID(t *testing.T, handler http.HandlerFunc, method, url string) {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(`{"not_user_id":"data"}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "user_id required")
}

func TestAuthControllerReadBody(t *testing.T) {
	t.Run("Success - reads valid JSON body", func(t *testing.T) {
		c, _ := setupTestAuthController(t)
		body := `{"test":"data"}`
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		rr := httptest.NewRecorder()

		data, err := c.readBody(rr, req)
		require.NoError(t, err)
		assert.Equal(t, []byte(body), data)
	})

	t.Run("Success - reads empty body", func(t *testing.T) {
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(""))
		rr := httptest.NewRecorder()

		data, err := c.readBody(rr, req)
		require.NoError(t, err)
		assert.Equal(t, []byte{}, data)
	})

	t.Run("Failure - body exceeds max payload bytes", func(t *testing.T) {
		c, _ := setupTestAuthController(t)
		// Set a small max payload for testing
		c.cfg.Gateway.MaxPayloadBytes = 100
		largeBody := strings.Repeat("a", 200)
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(largeBody))
		rr := httptest.NewRecorder()

		_, err := c.readBody(rr, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "http: request body too large")
	})
}

func TestFileActuatorKeyReader(t *testing.T) {
	t.Run("Success - reads valid actuator key file", func(t *testing.T) {
		// Create a temporary file with valid actuator key data
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
		assert.Empty(t, publicKey) // Missing field returns empty string
	})
}
