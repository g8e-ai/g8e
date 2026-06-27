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
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestInfrastructure holds common test setup components shared across gateway tests.
type TestInfrastructure struct {
	Cfg                *config.Config
	Logger             *slog.Logger
	DB                 *CanonicalDBService
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
	Passkey            *PasskeyService
	SuspendedStore     storage.SuspendedTransactionStore
	DBDir              string
	PKIDir             string
	SecretsDir         string
}

// setupTestInfrastructure creates common test infrastructure for gateway tests.
// It initializes DB, PKI, auth services, and other shared components.
func setupTestInfrastructure(t *testing.T, resetKeystoreStorage bool) *TestInfrastructure {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()

	var ks *keystore.Keystore
	if resetKeystoreStorage {
		keyring, err := keystore.NewMemoryKeyring()
		require.NoError(t, err)
		ks, err = keystore.NewWithKeyring(secretsDir, logger, keyring)
		require.NoError(t, err)
		require.NoError(t, ks.Initialize())
		require.NoError(t, ks.EnforcePermissions())
	} else {
		// Reuse the keystore from shared test storage
		keyring, err := keystore.NewMemoryKeyring()
		require.NoError(t, err)
		ks, err = keystore.NewWithKeyring(secretsDir, logger, keyring)
		require.NoError(t, err)
		// Initialize will retrieve the existing master key from shared test storage
		require.NoError(t, ks.Initialize())
		require.NoError(t, ks.EnforcePermissions())
	}

	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, constants.VaultDirname), logger, true, "", false, ks)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

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
	// Wire up auth service to user service for cache invalidation
	userSvc.SetAuthService(auth)
	cliSessionSvc := NewCLISessionService(db, logger)
	operatorSessionSvc := NewOperatorSessionService(db, logger)
	webSessionSvc := NewWebSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, &cfg.Gateway)
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"}, webSessionSvc, resp, cfg.Gateway.MaxPayloadBytes)

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

	return &TestInfrastructure{
		Cfg:                cfg,
		Logger:             logger,
		DB:                 db,
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
		Passkey:            passkey,
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
