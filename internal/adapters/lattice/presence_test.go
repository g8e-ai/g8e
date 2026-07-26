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

package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPublishPresence_CallsPublishEntityWithCorrectEntityID(t *testing.T) {
	t.Parallel()

	entityMgr := &mockEntityManagerAPIClient{}
	a := &Adapter{
		config:    validTestConfig(),
		logger:    newTestLogger(),
		fileSvc:   newMockFileSvc(),
		entityMgr: entityMgr,
		taskMgr:   &mockTaskManagerAPIClient{},
		entityID:  "test-entity-123",
	}

	err := a.PublishPresence(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, entityMgr.publishEntityCalls)
}

func TestPublishPresence_ReturnsErrLatticePresencePublishFailedOnRPCError(t *testing.T) {
	t.Parallel()

	entityMgr := &mockEntityManagerAPIClient{
		publishEntityErr: status.Error(codes.Unavailable, "lattice down"),
	}
	a := &Adapter{
		config:    validTestConfig(),
		logger:    newTestLogger(),
		fileSvc:   newMockFileSvc(),
		entityMgr: entityMgr,
		taskMgr:   &mockTaskManagerAPIClient{},
		entityID:  "test-entity-456",
	}

	err := a.PublishPresence(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLatticePresencePublishFailed)
}

func TestPublishPresence_ReturnsErrLatticePresencePublishFailedOnNonGRPCError(t *testing.T) {
	t.Parallel()

	entityMgr := &mockEntityManagerAPIClient{
		publishEntityErr: errors.New("plain network error"),
	}
	a := &Adapter{
		config:    validTestConfig(),
		logger:    newTestLogger(),
		fileSvc:   newMockFileSvc(),
		entityMgr: entityMgr,
		taskMgr:   &mockTaskManagerAPIClient{},
		entityID:  "test-entity-789",
	}

	err := a.PublishPresence(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLatticePresencePublishFailed)
}

func TestPublishPresence_SucceedsWithNilResponse(t *testing.T) {
	t.Parallel()

	entityMgr := &mockEntityManagerAPIClient{}
	a := &Adapter{
		config:    validTestConfig(),
		logger:    newTestLogger(),
		fileSvc:   newMockFileSvc(),
		entityMgr: entityMgr,
		taskMgr:   &mockTaskManagerAPIClient{},
		entityID:  "entity-success",
	}

	err := a.PublishPresence(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, entityMgr.publishEntityCalls)
}
