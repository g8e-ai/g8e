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

	taskmanagerv1 "github.com/g8e-ai/g8e/internal/adapters/lattice/gen/anduril/taskmanager/v1"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validTestConfig() *LatticeConfig {
	return &LatticeConfig{
		Enabled:      true,
		Endpoint:     "https://lattice.example.com",
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		Entity:       EntityConfig{Name: "g8e-operator", PlatformType: "g8e-operator"},
		PostureFloor: "consensus",
	}
}

func TestAdapter_Start_ReturnsErrLatticeNotInitializedWhenTaskHandlerNil(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(validTestConfig(), newMockFileSvc())
	a.SetPostureProvider(func() string { return "consensus" })

	err := a.Start(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLatticeNotInitialized)
}

func TestAdapter_Start_ReturnsErrLatticeNotInitializedWhenPostureProviderNil(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(validTestConfig(), newMockFileSvc())
	a.SetTaskHandler(func(ctx context.Context, task *taskmanagerv1.Task) error { return nil })

	err := a.Start(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLatticeNotInitialized)
}

func TestAdapter_Start_ReturnsErrLatticeAlreadyRunningWhenStartedTwice(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(validTestConfig(), newMockFileSvc())
	a.SetTaskHandler(func(ctx context.Context, task *taskmanagerv1.Task) error { return nil })
	a.SetPostureProvider(func() string { return "consensus" })

	require.NoError(t, a.Start(context.Background()))
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	err := a.Start(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLatticeAlreadyRunning)
}

func TestAdapter_Start_GeneratesNewEntityIDWhenFileMissing(t *testing.T) {
	t.Parallel()

	fileSvc := newMockFileSvc()
	a := newTestAdapter(validTestConfig(), fileSvc)
	a.SetTaskHandler(func(ctx context.Context, task *taskmanagerv1.Task) error { return nil })
	a.SetPostureProvider(func() string { return "consensus" })

	require.NoError(t, a.Start(context.Background()))
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	assert.NotEmpty(t, a.entityID)
	persisted, ok := fileSvc.files[constants.LatticeEntityIDFilename]
	require.True(t, ok)
	assert.Equal(t, a.entityID, string(persisted))
}

func TestAdapter_Start_LoadsExistingEntityIDFromFile(t *testing.T) {
	t.Parallel()

	fileSvc := newMockFileSvc()
	existingID := "pre-existing-uuid-1234"
	fileSvc.files[constants.LatticeEntityIDFilename] = []byte(existingID)

	a := newTestAdapter(validTestConfig(), fileSvc)
	a.SetTaskHandler(func(ctx context.Context, task *taskmanagerv1.Task) error { return nil })
	a.SetPostureProvider(func() string { return "consensus" })

	require.NoError(t, a.Start(context.Background()))
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	assert.Equal(t, existingID, a.entityID)
}

func TestAdapter_Start_TrimsWhitespaceFromLoadedEntityID(t *testing.T) {
	t.Parallel()

	fileSvc := newMockFileSvc()
	rawID := "  \n\tuuid-with-whitespace\t\n  "
	fileSvc.files[constants.LatticeEntityIDFilename] = []byte(rawID)

	a := newTestAdapter(validTestConfig(), fileSvc)
	a.SetTaskHandler(func(ctx context.Context, task *taskmanagerv1.Task) error { return nil })
	a.SetPostureProvider(func() string { return "consensus" })

	require.NoError(t, a.Start(context.Background()))
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	assert.Equal(t, "uuid-with-whitespace", a.entityID)
}

func TestAdapter_Start_GeneratesNewIDWhenFileContainsOnlyWhitespace(t *testing.T) {
	t.Parallel()

	fileSvc := newMockFileSvc()
	fileSvc.files[constants.LatticeEntityIDFilename] = []byte("   \n\t  ")

	a := newTestAdapter(validTestConfig(), fileSvc)
	a.SetTaskHandler(func(ctx context.Context, task *taskmanagerv1.Task) error { return nil })
	a.SetPostureProvider(func() string { return "consensus" })

	require.NoError(t, a.Start(context.Background()))
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	assert.NotEqual(t, "   \n\t  ", a.entityID)
	assert.NotEmpty(t, a.entityID)
}

func TestAdapter_Stop_ReturnsNilWhenNotRunning(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(validTestConfig(), newMockFileSvc())
	err := a.Stop(context.Background())
	require.NoError(t, err)
}

func TestAdapter_Stop_UnregistersHeartbeatSink(t *testing.T) {
	t.Parallel()

	hs := newMockHeartbeatRegistrar()
	a := newTestAdapter(validTestConfig(), newMockFileSvc())
	a.SetHeartbeatService(hs)
	a.SetTaskHandler(func(ctx context.Context, task *taskmanagerv1.Task) error { return nil })
	a.SetPostureProvider(func() string { return "consensus" })

	require.NoError(t, a.Start(context.Background()))
	assert.Equal(t, 1, hs.registered)

	require.NoError(t, a.Stop(context.Background()))
	assert.Equal(t, 1, hs.unregistered)
}

func TestAdapter_IsRunning_ReturnsFalseBeforeStart(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(validTestConfig(), newMockFileSvc())
	assert.False(t, a.IsRunning())
}

func TestAdapter_IsRunning_ReturnsTrueAfterStart(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(validTestConfig(), newMockFileSvc())
	a.SetTaskHandler(func(ctx context.Context, task *taskmanagerv1.Task) error { return nil })
	a.SetPostureProvider(func() string { return "consensus" })

	require.NoError(t, a.Start(context.Background()))
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	assert.True(t, a.IsRunning())
}

func TestAdapter_IsRunning_ReturnsFalseAfterStop(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(validTestConfig(), newMockFileSvc())
	a.SetTaskHandler(func(ctx context.Context, task *taskmanagerv1.Task) error { return nil })
	a.SetPostureProvider(func() string { return "consensus" })

	require.NoError(t, a.Start(context.Background()))
	require.NoError(t, a.Stop(context.Background()))

	assert.False(t, a.IsRunning())
}

func TestAdapter_HeartbeatRegistrar_InterfaceSatisfied(t *testing.T) {
	t.Parallel()

	var _ HeartbeatRegistrar = (*mockHeartbeatRegistrar)(nil)
}

func TestAdapter_Start_HeartbeatSinkRepublishesPresence(t *testing.T) {
	t.Parallel()

	hs := newMockHeartbeatRegistrar()
	entityMgr := &mockEntityManagerAPIClient{}
	fileSvc := newMockFileSvc()

	a := &Adapter{
		config:    validTestConfig(),
		logger:    newTestLogger(),
		fileSvc:   fileSvc,
		entityMgr: entityMgr,
		taskMgr:   &mockTaskManagerAPIClient{},
	}
	a.SetHeartbeatService(hs)
	a.SetTaskHandler(func(ctx context.Context, task *taskmanagerv1.Task) error { return nil })
	a.SetPostureProvider(func() string { return "consensus" })

	require.NoError(t, a.Start(context.Background()))
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	initialCalls := entityMgr.publishEntityCalls

	for id, sink := range hs.sinks {
		sink(context.Background())
		_ = id
	}

	assert.Greater(t, entityMgr.publishEntityCalls, initialCalls)
}

func TestAdapter_Start_ContinuesWhenPresencePublishFails(t *testing.T) {
	t.Parallel()

	entityMgr := &mockEntityManagerAPIClient{
		publishEntityErr: errors.New("network down"),
	}
	a := &Adapter{
		config:    validTestConfig(),
		logger:    newTestLogger(),
		fileSvc:   newMockFileSvc(),
		entityMgr: entityMgr,
		taskMgr:   &mockTaskManagerAPIClient{},
	}
	a.SetTaskHandler(func(ctx context.Context, task *taskmanagerv1.Task) error { return nil })
	a.SetPostureProvider(func() string { return "consensus" })

	err := a.Start(context.Background())
	require.NoError(t, err)
	assert.True(t, a.IsRunning())
	_ = a.Stop(context.Background())
}
