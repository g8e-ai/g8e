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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/require"
)

var (
	tempDirCounters = make(map[string]int)
	tempDirMu       sync.Mutex
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
	SuspendedStore     *storage.SuspendedTransactionService
	DBDir              string
	PKIDir             string
	SecretsDir         string
}

// tempDir creates a temporary directory in the current working directory.
// This avoids Windows %TEMP% permission issues and temp dir cleanup problems.
func tempDir(tb testing.TB) string {
	tb.Helper()
	// Sanitize test name for use as directory name
	safeName := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, tb.Name())

	tempDirMu.Lock()
	tempDirCounters[safeName]++
	count := tempDirCounters[safeName]
	tempDirMu.Unlock()

	dir := filepath.Join(constants.ProjectRootFromCurrentDir, "test-temp", fmt.Sprintf("%s_%d", safeName, count))
	err := os.MkdirAll(dir, 0755)
	require.NoError(tb, err, "failed to create temp dir in cwd")
	tb.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// setupTestInfrastructure creates common test infrastructure for gateway tests.
// It initializes DB, PKI, auth services, and other shared components.
func setupTestInfrastructure(t *testing.T, resetKeystoreStorage bool) *TestInfrastructure {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := tempDir(t)
	pkiDir := tempDir(t)
	secretsDir := tempDir(t)
	// Ensure directories are clean to avoid stale state from previous test runs
	os.RemoveAll(dbDir)
	require.NoError(t, os.MkdirAll(dbDir, 0755))
	os.RemoveAll(pkiDir)
	require.NoError(t, os.MkdirAll(pkiDir, 0755))
	os.RemoveAll(secretsDir)
	require.NoError(t, os.MkdirAll(secretsDir, 0755))
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	var sm *SecretManager
	if resetKeystoreStorage {
		keystore.ResetTestStorage()
		backend, err := keystore.NewTestBackend()
		require.NoError(t, err)
		ks, err := keystore.NewWithBackend(secretsDir, logger, backend)
		require.NoError(t, err)
		require.NoError(t, ks.Initialize())
		require.NoError(t, ks.EnforcePermissions())
		sm = &SecretManager{
			db:         db.db,
			secretsDir: secretsDir,
			logger:     logger,
			keystore:   ks,
		}
	} else {
		// Reuse the keystore from DB initialization - create a new SecretManager instance
		// that points to the same secretsDir and uses the shared test backend
		backend, err := keystore.NewTestBackend()
		require.NoError(t, err)
		ks, err := keystore.NewWithBackend(secretsDir, logger, backend)
		require.NoError(t, err)
		// Initialize will retrieve the existing master key from shared test storage
		require.NoError(t, ks.Initialize())
		require.NoError(t, ks.EnforcePermissions())
		sm = &SecretManager{
			db:         db.db,
			secretsDir: secretsDir,
			logger:     logger,
			keystore:   ks,
		}
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
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})

	// Initialize suspended transaction service for tests
	suspendedTxConfig := &storage.SuspendedTransactionConfig{
		DBPath:               filepath.Join(dbDir, "suspended_transactions.db"),
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
