// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"encoding/json"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/require"
)

func TestAppPolicyStoreService_GetAppPolicy(t *testing.T) {
	ts := setupTestInfrastructure(t, true)
	svc := NewAppPolicyStoreService(ts.DB.db, ts.Logger, ts.Stores.DocStore)

	// Test non-existent policy
	policy, err := svc.GetAppPolicy("non-existent")
	require.NoError(t, err)
	require.Nil(t, policy)

	// Test existing policy
	appID := "test-app"
	expectedPolicy := &models.AppPolicy{
		AppID: appID,
	}

	// Convert expectedPolicy to json.RawMessage
	data, err := json.Marshal(expectedPolicy)
	require.NoError(t, err)

	err = svc.docSvc.DocSet(marshaler.CollectionName(constants.CollectionAppPolicies), appID, data)
	require.NoError(t, err)

	policy, err = svc.GetAppPolicy(appID)
	require.NoError(t, err)
	require.NotNil(t, policy)
	require.Equal(t, appID, policy.AppID)
}
