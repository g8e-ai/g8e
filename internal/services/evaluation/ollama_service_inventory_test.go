// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type inventoryTestDispatcher struct {
	request OllamaModelCommandDispatchRequest
}

func (d *inventoryTestDispatcher) DispatchOllamaModelCommand(_ context.Context, request OllamaModelCommandDispatchRequest) (*OllamaModelCommandDispatchResult, error) {
	d.request = request
	return &OllamaModelCommandDispatchResult{
		Status:  200,
		Success: true,
		ActionType: constants.ActionTypeOllamaModelInventory,
		InventoryResult: &operatorv1.OllamaModelInventoryResult{
			Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			Entries: inference.ProviderModelInventoryEntriesToProto([]inference.ProviderModelInventoryEntry{
				{
					ProviderClass:  "ollama",
					ServedModelTag: "qwen3:4b",
					ModelDigest:    repeatHex('a', 64),
				},
			}),
		},
	}, nil
}

func TestListOllamaProviderInventory_UsesTypedInventoryAction(t *testing.T) {
	t.Parallel()
	dispatcher := &inventoryTestDispatcher{}
	maintenance := OllamaModelMaintenanceContext{
		TargetOperatorSessionID: "infer-session",
		NewID:                   func(prefix string) string { return prefix },
	}
	entries, err := ListOllamaProviderInventory(context.Background(), dispatcher, maintenance)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "qwen3:4b", entries[0].ServedModelTag)
	assert.Equal(t, constants.ActionTypeOllamaModelInventory, dispatcher.request.ActionType)
	assert.NotEmpty(t, dispatcher.request.Payload)
}
