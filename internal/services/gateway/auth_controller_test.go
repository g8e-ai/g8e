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
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func setupTestBootstrapController(t *testing.T) (*BootstrapController, *config.Config) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)
	return newBootstrapController(BootstrapControllerDeps{
		Cfg:                infra.Cfg,
		Logger:             infra.Logger,
		DocStore:           infra.Stores.DocStore,
		UserSvc:            infra.UserSvc,
		PKI:                infra.PKI,
		CLISessionSvc:      infra.CLISessionSvc,
		OperatorSessionSvc: infra.OperatorSessionSvc,
		Responder:          infra.Responder,
		ActuatorKeyReader:  nil,
	}), infra.Cfg
}

func setupTestEnrollmentTokenController(t *testing.T) (*EnrollmentTokenController, *config.Config) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)
	enrollmentTokenSvc := NewEnrollmentTokenService(infra.Stores.DocStore, infra.Logger)
	return newEnrollmentTokenController(EnrollmentTokenControllerDeps{
		Cfg:                infra.Cfg,
		Logger:             infra.Logger,
		EnrollmentTokenSvc: enrollmentTokenSvc,
		Responder:          infra.Responder,
	}), infra.Cfg
}

func setupTestUserController(t *testing.T) (*UserController, *config.Config) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)
	return newUserController(UserControllerDeps{
		Cfg:       infra.Cfg,
		Logger:    infra.Logger,
		UserSvc:   infra.UserSvc,
		Responder: infra.Responder,
	}), infra.Cfg
}

func setupTestSessionController(t *testing.T) (*SessionController, *config.Config) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)
	return newSessionController(SessionControllerDeps{
		Logger:      infra.Logger,
		DocStore:    infra.Stores.DocStore,
		Responder:   infra.Responder,
		CrossOrigin: false,
	}), infra.Cfg
}

// setupTestPasskeyService creates a PasskeyHandler with approval dependencies for testing.
func setupTestPasskeyService(t *testing.T) (*PasskeyHandler, *UserService, storage.SuspendedTransactionStore) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	userSvc := NewUserService(stores.DocStore, logger)
	resp := response.NewWriter(logger)
	webSessionSvc := NewWebSessionService(stores.DocStore, logger)
	passkey, err := NewPasskeyService(stores.DocStore, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
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
		ThreatScanner:    governance.NewL1Doctrine(),
		MaxPayloadBytes:  cfg.Gateway.MaxPayloadBytes,
		Posture:          string(cfg.Gateway.Posture),
	})
	require.NoError(t, err)

	orchestrator := NewPasskeyOrchestrator(mcpGateway, suspendedTxService, nil, nil, logger)
	passkeyHandler := NewPasskeyHandler(PasskeyHandlerDeps{
		Service:       passkey,
		WebSessionSvc: webSessionSvc,
		Responder:     resp,
		MaxPayload:    cfg.Gateway.MaxPayloadBytes,
		Orchestrator:  orchestrator,
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

func TestReadRequestBody(t *testing.T) {
	t.Run("Success - reads valid JSON body", func(t *testing.T) {
		body := `{"test":"data"}`
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))

		data, err := readRequestBody(req, 1024)
		require.NoError(t, err)
		assert.Equal(t, []byte(body), data)
	})

	t.Run("Success - reads empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(""))

		data, err := readRequestBody(req, 1024)
		require.NoError(t, err)
		assert.Equal(t, []byte{}, data)
	})

	t.Run("Failure - body exceeds max payload bytes", func(t *testing.T) {
		largeBody := strings.Repeat("a", 200)
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(largeBody))

		_, err := readRequestBody(req, 100)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "payload exceeds maximum size limit")
	})
}
