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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
)

func makeTask(taskID, specURL string) *taskmanagerv1.Task {
	task := &taskmanagerv1.Task{
		Version: &taskmanagerv1.TaskVersion{
			TaskId:            taskID,
			DefinitionVersion: 1,
			StatusVersion:     1,
		},
	}
	if specURL != "" {
		task.Specification = &anypb.Any{TypeUrl: specURL}
	}
	return task
}

func TestIsTaskAccepted_AcceptsAllWhenCatalogEmpty(t *testing.T) {
	t.Parallel()

	a := &Adapter{config: &LatticeConfig{TaskCatalog: nil}}
	assert.True(t, a.isTaskAccepted(makeTask("t1", "type.googleapis.com/Foo")))
}

func TestIsTaskAccepted_AcceptsTaskWhenSpecURLInCatalog(t *testing.T) {
	t.Parallel()

	a := &Adapter{config: &LatticeConfig{
		TaskCatalog: []string{"type.googleapis.com/Foo", "type.googleapis.com/Bar"},
	}}
	assert.True(t, a.isTaskAccepted(makeTask("t1", "type.googleapis.com/Foo")))
	assert.True(t, a.isTaskAccepted(makeTask("t2", "type.googleapis.com/Bar")))
}

func TestIsTaskAccepted_RejectsTaskWhenSpecURLNotInCatalog(t *testing.T) {
	t.Parallel()

	a := &Adapter{config: &LatticeConfig{
		TaskCatalog: []string{"type.googleapis.com/Foo"},
	}}
	assert.False(t, a.isTaskAccepted(makeTask("t1", "type.googleapis.com/Baz")))
}

func TestIsTaskAccepted_RejectsTaskWhenSpecIsNil(t *testing.T) {
	t.Parallel()

	a := &Adapter{config: &LatticeConfig{
		TaskCatalog: []string{"type.googleapis.com/Foo"},
	}}
	assert.False(t, a.isTaskAccepted(makeTask("t1", "")))
}

func TestPostureRank_ReturnsExpectedOrdering(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, postureRank("doctrine"))
	assert.Equal(t, 1, postureRank("consensus"))
	assert.Equal(t, 2, postureRank("notary"))
	assert.Equal(t, 0, postureRank("unknown"))
	assert.Equal(t, 0, postureRank(""))
}

func TestCheckPostureFloor_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		active      string
		floor       string
		expectError bool
	}{
		{"doctrine active, consensus floor -> rejected", "doctrine", "consensus", true},
		{"consensus active, consensus floor -> accepted", "consensus", "consensus", false},
		{"notary active, consensus floor -> accepted", "notary", "consensus", false},
		{"doctrine active, doctrine floor -> accepted", "doctrine", "doctrine", false},
		{"notary active, notary floor -> accepted", "notary", "notary", false},
		{"consensus active, notary floor -> rejected", "consensus", "notary", true},
		{"empty active, empty floor -> defaults to consensus floor, doctrine active -> rejected (fail-closed)", "", "", true},
		{"unknown active, consensus floor -> rejected (fail-closed)", "unknown", "consensus", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := &Adapter{
				config:          &LatticeConfig{PostureFloor: tt.floor},
				postureProvider: func() string { return tt.active },
			}

			err := a.checkPostureFloor()
			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, constants.ErrLatticePostureFloorViolated)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestReportTaskStatus_CallsUpdateStatusRPC(t *testing.T) {
	t.Parallel()

	taskMgr := &mockTaskManagerAPIClient{}
	a := &Adapter{
		config:    validTestConfig(),
		logger:    newTestLogger(),
		fileSvc:   newMockFileSvc(),
		entityMgr: &mockEntityManagerAPIClient{},
		taskMgr:   taskMgr,
		entityID:  "ent-1",
	}

	version := &taskmanagerv1.TaskVersion{TaskId: "task-abc", StatusVersion: 1}
	err := a.reportTaskStatus(context.Background(), version,
		taskmanagerv1.Status_STATUS_DONE_NOT_OK,
		taskmanagerv1.ErrorCode_ERROR_CODE_REJECTED,
		"posture floor violated")

	require.NoError(t, err)
	assert.Equal(t, 1, taskMgr.updateStatusCalls)
}

func TestReportTaskStatus_ReturnsErrLatticeStatusReportFailedOnRPCError(t *testing.T) {
	t.Parallel()

	taskMgr := &mockTaskManagerAPIClient{
		updateStatusErr: status.Error(codes.Unavailable, "lattice down"),
	}
	a := &Adapter{
		config:    validTestConfig(),
		logger:    newTestLogger(),
		fileSvc:   newMockFileSvc(),
		entityMgr: &mockEntityManagerAPIClient{},
		taskMgr:   taskMgr,
		entityID:  "ent-1",
	}

	version := &taskmanagerv1.TaskVersion{TaskId: "task-xyz", StatusVersion: 1}
	err := a.reportTaskStatus(context.Background(), version,
		taskmanagerv1.Status_STATUS_DONE_NOT_OK,
		taskmanagerv1.ErrorCode_ERROR_CODE_FAILED,
		"handler error")

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLatticeStatusReportFailed)
	assert.Equal(t, 1, taskMgr.updateStatusCalls)
}

func TestReportTaskStatus_ReturnsErrLatticeStatusReportFailedOnNonGRPCError(t *testing.T) {
	t.Parallel()

	taskMgr := &mockTaskManagerAPIClient{
		updateStatusErr: errors.New("plain error"),
	}
	a := &Adapter{
		config:    validTestConfig(),
		logger:    newTestLogger(),
		fileSvc:   newMockFileSvc(),
		entityMgr: &mockEntityManagerAPIClient{},
		taskMgr:   taskMgr,
		entityID:  "ent-1",
	}

	version := &taskmanagerv1.TaskVersion{TaskId: "task-plain", StatusVersion: 1}
	err := a.reportTaskStatus(context.Background(), version,
		taskmanagerv1.Status_STATUS_DONE_NOT_OK,
		taskmanagerv1.ErrorCode_ERROR_CODE_FAILED,
		"plain")

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLatticeStatusReportFailed)
}
