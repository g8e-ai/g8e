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
	"encoding/json"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/stretchr/testify/require"
)

func TestAppPolicyStoreService_GetAppPolicy(t *testing.T) {
	ts := setupTestInfrastructure(t, true)
	svc := NewAppPolicyStoreService(ts.DB.db, ts.Logger, ts.DB.DocStore)

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
