// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore/keystoretest"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestInfrastructure holds common test setup components shared across gateway tests.
type TestInfrastructure struct {
	Cfg                *config.Config
	Logger             *slog.Logger
	DB                 *CanonicalDBService
	Stores             *Stores
	Pubsub             *GatewayWebSocketHandler
	SecretMgr          *SecretManager
	PKI                *PKIAuthority
	UserSvc            *UserService
	PersonaSvc         *PersonaService
	Responder          *response.Writer
	Auth               *AuthService
	CLISessionSvc      *CLISessionService
	OperatorSessionSvc *OperatorSessionService
	WebSessionSvc      *WebSessionService
	Reg                *RegistrationService
	Passkey            *PasskeyHandler
	SuspendedStore     storage.SuspendedTransactionStore
	DBDir              string
	PKIDir             string
	SecretsDir         string
}

// newTestFileSvc creates a RuntimeFileService backed by a temp directory
// with the full .g8e runtime tree created. All gateway integration tests
// should use this helper to obtain a fileSvc.
func newTestFileSvc(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	baseDir := testutil.TempDir(t)
	svc, err := fs.NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, svc.CreateRuntimeTree(context.Background()))
	return svc
}

// newTestKeystore creates an initialized keystore using an in-memory keyring.
func newTestKeystore(tb testing.TB, fileSvc fs.RuntimeFileService, logger *slog.Logger) *keystore.Keystore {
	tb.Helper()
	keyring := keystoretest.NewMemoryKeyring()
	ks, err := keystore.NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(tb, err)
	require.NoError(tb, ks.Initialize())
	require.NoError(tb, ks.EnforcePermissions())
	return ks
}

// openTestDB wraps OpenCanonicalDBService for tests, creating a keystore
// with an in-memory keyring so callers don't need to manage a keystore.
// Vault auto-initializes on first open; the keystore is for secret operations.
func openTestDB(t *testing.T, dataDir string, fileSvc fs.RuntimeFileService, logger *slog.Logger) (*CanonicalDBService, *Stores, error) {
	t.Helper()
	ks := newTestKeystore(t, fileSvc, logger)
	vaultDir := fileSvc.Resolve(constants.VaultDirname)
	return OpenCanonicalDBService(dataDir, vaultDir, logger, "", ks, fileSvc)
}

// setupTestInfrastructure creates common test infrastructure for gateway tests.
// It initializes DB, PKI, auth services, and other shared components.
func setupTestInfrastructure(t *testing.T, resetKeystoreStorage bool) *TestInfrastructure {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	fileSvc := newTestFileSvc(t)
	dbDir := testutil.TempDir(t)
	pkiDir := testutil.TempDir(t)
	secretsDir := fileSvc.Resolve(constants.SecretsDirname)

	ks := newTestKeystore(t, fileSvc, logger)

	db, stores, err := OpenCanonicalDBService(dbDir, fileSvc.Resolve(constants.VaultDirname), logger, "", ks, fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	sm := db.GetSecretManager()

	pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(stores.DocStore, logger)
	personaSvc := NewPersonaService(stores.DocStore, logger)
	resp := response.NewWriter(logger)
	auth := NewAuthService(stores.DocStore, pki, logger, userSvc, personaSvc, resp, nil, "", "", "")
	// Wire up auth service to user service for cache invalidation
	userSvc.SetAuthService(auth)
	cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
	operatorSessionSvc := NewOperatorSessionService(stores.DocStore, logger)
	webSessionSvc := NewWebSessionService(stores.DocStore, logger)
	reg := NewRegistrationService(stores.DocStore, stores.KVStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, &cfg.Gateway)
	passkey, _ := NewPasskeyService(stores.DocStore, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})

	// Initialize suspended transaction service for tests
	suspendedTxConfig := &storage.SuspendedTransactionConfig{
		DBPath:               filepath.Join(dbDir, constants.SuspendedTxFilename),
		MaxDBSizeMB:          256,
		RetentionDays:        7,
		PruneIntervalMinutes: 30,
	}
	suspendedTxService, err := storage.NewSuspendedTransactionService(suspendedTxConfig, logger)
	require.NoError(t, err)
	t.Cleanup(func() { suspendedTxService.Close() })

	passkeyOrchestrator, err := NewPasskeyOrchestrator(nil, suspendedTxService, stores.SSEStore, pubsub, logger)
	require.NoError(t, err)
	passkeyHandler := NewPasskeyHandler(PasskeyHandlerDeps{
		Service:       passkey,
		WebSessionSvc: webSessionSvc,
		Responder:     resp,
		MaxPayload:    cfg.Gateway.MaxPayloadBytes,
		Orchestrator:  passkeyOrchestrator,
	})

	return &TestInfrastructure{
		Cfg:                cfg,
		Logger:             logger,
		DB:                 db,
		Stores:             stores,
		Pubsub:             pubsub,
		SecretMgr:          sm,
		PKI:                pki,
		UserSvc:            userSvc,
		PersonaSvc:         personaSvc,
		Responder:          resp,
		Auth:               auth,
		CLISessionSvc:      cliSessionSvc,
		OperatorSessionSvc: operatorSessionSvc,
		WebSessionSvc:      webSessionSvc,
		Reg:                reg,
		Passkey:            passkeyHandler,
		SuspendedStore:     suspendedTxService,
		DBDir:              dbDir,
		PKIDir:             pkiDir,
		SecretsDir:         secretsDir,
	}
}

// setupTestHTTPHandlerLightweight creates a minimal HTTPHandler for testing
// handler methods that don't require full infrastructure (e.g., readBody, pathTraversalGuard).
// This is significantly faster than setupTestHTTPHandler for unit testing simple handler logic.
func setupTestHTTPHandlerLightweight(t *testing.T) *HTTPHandler {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	h := &HTTPHandler{
		cfg:    cfg,
		logger: logger,
	}

	return h
}
