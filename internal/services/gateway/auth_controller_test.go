// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/mcp"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func setupTestBootstrapController(t *testing.T) (*BootstrapController, *config.Config) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)
	return newBootstrapController(BootstrapControllerDeps{
		Cfg:                infra.Cfg,
		Logger:             infra.Logger,
		DocStore:           infra.DocStore,
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
	enrollmentTokenSvc := NewEnrollmentTokenService(infra.DocStore, infra.Logger)
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
		DocStore:    infra.DocStore,
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
	db, err := openTestDB(t, dbDir, fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	userSvc := NewUserService(db.GetDocStore(), logger)
	resp := response.NewWriter(logger)
	webSessionSvc := NewWebSessionService(db.GetDocStore(), logger)
	passkey, err := NewPasskeyService(db.GetDocStore(), logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
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
		AuditStore:       mcp.NoopAuditEventRecorder{},
	})
	require.NoError(t, err)

	orchestrator, err := NewPasskeyOrchestrator(mcpGateway, suspendedTxService, db.GetSSEStore(), NewGatewayWebSocketHandler(logger), logger)
	require.NoError(t, err)
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
