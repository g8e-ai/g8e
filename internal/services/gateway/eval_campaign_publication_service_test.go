// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestEvalCampaignPublicationService_PutAndGet(t *testing.T) {
	logger := testutil.NewTestLogger()
	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(":memory:"), logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)

	service := NewEvalCampaignPublicationService(NewDocumentStoreService(db, logger))
	state := models.EvalCampaignPublicationState{
		SchemaVersion:         evalCampaignPublicationStateSchemaVersion,
		RunID:                 "eval-init-gemma2-9b-1789729690",
		PublishedIdempotency:  []string{"run-1:key-a", "run-1:key-b"},
		LastPublishedSequence: 42,
	}
	require.NoError(t, service.Put(state))

	loaded, err := service.Get(state.RunID)
	require.NoError(t, err)
	assert.Equal(t, state.RunID, loaded.RunID)
	assert.Equal(t, state.LastPublishedSequence, loaded.LastPublishedSequence)
	assert.Equal(t, state.PublishedIdempotency, loaded.PublishedIdempotency)
}
