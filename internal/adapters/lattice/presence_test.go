// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
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
