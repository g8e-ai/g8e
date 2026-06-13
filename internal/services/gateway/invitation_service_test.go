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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvitationService(t *testing.T) {
	t.Parallel()
	infra := setupTestInfrastructure(t, true)
	svc := NewInvitationService(infra.DB, infra.Logger)

	orgID := "org-1"
	sub := "user-1"
	createdBy := "admin"

	// Create
	inv, err := svc.CreateInvitation(orgID, sub, createdBy, []string{"admin"}, time.Hour)
	require.NoError(t, err)
	require.NotNil(t, inv)
	assert.Equal(t, orgID, inv.OrganizationID)
	assert.Equal(t, sub, inv.Sub)

	// Find
	found, err := svc.FindActiveInvitationBySub(sub)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, inv.ID, found.ID)

	// Consume
	err = svc.ConsumeInvitation(inv.ID)
	require.NoError(t, err)

	// Find again (should be nil because consumed)
	foundAfter, err := svc.FindActiveInvitationBySub(sub)
	require.NoError(t, err)
	assert.Nil(t, foundAfter)
}
