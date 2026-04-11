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

package pubsub

import (
	"context"
	"log/slog"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/require"
)

// pubsubFixture is the standard test fixture for PubSubCommandService unit tests.
// All tests must construct services via helpers in this file — no raw struct literals
// accessing internal sub-service fields.
type pubsubFixture struct {
	DB      *MockG8esPubSubClient
	Cfg     *config.Config
	Logger  *slog.Logger
	Svc     *PubSubCommandService
	Results *PubSubResultsService
}

// newPubsubFixture creates a fully wired PubSubCommandService for unit tests.
func newPubsubFixture(t *testing.T) *pubsubFixture {
	t.Helper()
	db := NewMockG8esPubSubClient()
	t.Cleanup(func() { db.Close() })

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	execSvc := execution.NewExecutionService(cfg, logger)
	fileEditSvc := execution.NewFileEditService(cfg, logger)

	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:         cfg,
		Logger:         logger,
		Execution:      execSvc,
		FileEdit:       fileEditSvc,
		PubSubClient:   db,
		ResultsService: resultsSvc,
	})
	require.NoError(t, err)

	svc.ctx = context.Background()

	return &pubsubFixture{
		DB:      db,
		Cfg:     cfg,
		Logger:  logger,
		Svc:     svc,
		Results: resultsSvc,
	}
}

// newPubsubFixtureWithAuditVault creates a fixture with a real, enabled AuditVaultService.
func newPubsubFixtureWithAuditVault(t *testing.T) (*pubsubFixture, *storage.AuditVaultService) {
	t.Helper()
	db := NewMockG8esPubSubClient()
	t.Cleanup(func() { db.Close() })

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	execSvc := execution.NewExecutionService(cfg, logger)
	fileEditSvc := execution.NewFileEditService(cfg, logger)

	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	tempDir := t.TempDir()
	avConfig := storage.AuditVaultConfig{
		Enabled:                   true,
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             30,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
	}
	avs, err := storage.NewAuditVaultService(&avConfig, logger)
	require.NoError(t, err)
	t.Cleanup(func() { avs.Close() })

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:         cfg,
		Logger:         logger,
		Execution:      execSvc,
		FileEdit:       fileEditSvc,
		PubSubClient:   db,
		ResultsService: resultsSvc,
		AuditVault:     avs,
	})
	require.NoError(t, err)

	svc.ctx = context.Background()

	f := &pubsubFixture{
		DB:      db,
		Cfg:     cfg,
		Logger:  logger,
		Svc:     svc,
		Results: resultsSvc,
	}

	return f, avs
}
