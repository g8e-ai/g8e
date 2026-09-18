// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

type capturingPublicationStateClient struct {
	putPath string
	putBody interface{}
}

func (c *capturingPublicationStateClient) Get(string) ([]byte, error) {
	return nil, fmt.Errorf("unexpected get")
}

func (c *capturingPublicationStateClient) Post(string, interface{}) ([]byte, error) {
	return nil, fmt.Errorf("unexpected post")
}

func (c *capturingPublicationStateClient) Put(path string, body interface{}) ([]byte, error) {
	c.putPath = path
	c.putBody = body
	return []byte(`{}`), nil
}

func (c *capturingPublicationStateClient) Delete(string) ([]byte, error) {
	return nil, fmt.Errorf("unexpected delete")
}

func TestGatewayCampaignPublicationStateStoreSaveSendsTypedStruct(t *testing.T) {
	t.Parallel()
	client := &capturingPublicationStateClient{}
	store := newGatewayCampaignPublicationStateStore(client)
	state := &evaluation.CampaignPublicationState{
		SchemaVersion:         "eval-campaign-publication-state/v1",
		RunID:                 "eval-init-gemma3-270m-1789739892",
		PublishedIdempotency:  []string{"eval-init-gemma3-270m-1789739892:assign-1:result"},
		LastPublishedSequence: 162,
	}
	require.NoError(t, store.Save(context.Background(), state))

	expectedPath := constants.APIPaths.EvalCampaignPublicationStateByRun + state.RunID + "/publication-state"
	assert.Equal(t, expectedPath, client.putPath)

	remote, ok := client.putBody.(models.EvalCampaignPublicationState)
	require.True(t, ok, "put body must be typed struct, got %T", client.putBody)
	assert.Equal(t, state.SchemaVersion, remote.SchemaVersion)
	assert.Equal(t, state.RunID, remote.RunID)
	assert.Equal(t, state.PublishedIdempotency, remote.PublishedIdempotency)
	assert.Equal(t, state.LastPublishedSequence, remote.LastPublishedSequence)

	bodyBytes, err := json.Marshal(client.putBody)
	require.NoError(t, err)
	var decoded models.EvalCampaignPublicationState
	require.NoError(t, json.Unmarshal(bodyBytes, &decoded))
	assert.Equal(t, remote, decoded)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(bodyBytes, &raw))
	assert.Contains(t, raw, "run_id")
	assert.Contains(t, raw, "published_idempotency_keys")
	assert.NotContains(t, raw, "SchemaVersion")
}

func TestGatewayCampaignPublicationStateStoreLoadMapsRemoteState(t *testing.T) {
	t.Parallel()
	remote := models.EvalCampaignPublicationState{
		SchemaVersion:         "eval-campaign-publication-state/v1",
		RunID:                 "eval-init-gemma2-9b-1789729690",
		PublishedIdempotency:  []string{"key-a", "key-b"},
		LastPublishedSequence: 99,
	}
	body, err := json.Marshal(remote)
	require.NoError(t, err)

	client := &mockAPIClient{getResp: body}
	store := newGatewayCampaignPublicationStateStore(client)
	loaded, err := store.Load(context.Background(), remote.RunID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, remote.SchemaVersion, loaded.SchemaVersion)
	assert.Equal(t, remote.RunID, loaded.RunID)
	assert.Equal(t, remote.PublishedIdempotency, loaded.PublishedIdempotency)
	assert.Equal(t, remote.LastPublishedSequence, loaded.LastPublishedSequence)
	assert.Equal(t, []string{constants.APIPaths.EvalCampaignPublicationStateByRun + remote.RunID + "/publication-state"}, client.getCalls)
}
