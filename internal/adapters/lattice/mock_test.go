// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package lattice

import (
	"context"
	"log/slog"
	"os"

	entitymanagerv1 "github.com/g8e-ai/g8e/v2/internal/adapters/lattice/gen/anduril/entitymanager/v1"
	taskmanagerv1 "github.com/g8e-ai/g8e/v2/internal/adapters/lattice/gen/anduril/taskmanager/v1"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mockEntityManagerAPIClient is a stub EntityManagerAPIClient for unit tests.
type mockEntityManagerAPIClient struct {
	publishEntityResp  *entitymanagerv1.PublishEntityResponse
	publishEntityErr   error
	publishEntityCalls int
}

func (m *mockEntityManagerAPIClient) PublishEntity(ctx context.Context, in *entitymanagerv1.PublishEntityRequest, opts ...grpc.CallOption) (*entitymanagerv1.PublishEntityResponse, error) {
	m.publishEntityCalls++
	if m.publishEntityErr != nil {
		return nil, m.publishEntityErr
	}
	if m.publishEntityResp != nil {
		return m.publishEntityResp, nil
	}
	return &entitymanagerv1.PublishEntityResponse{}, nil
}

func (m *mockEntityManagerAPIClient) PublishEntities(ctx context.Context, opts ...grpc.CallOption) (grpc.ClientStreamingClient[entitymanagerv1.PublishEntitiesRequest, entitymanagerv1.PublishEntitiesResponse], error) {
	return nil, nil
}

func (m *mockEntityManagerAPIClient) GetEntity(ctx context.Context, in *entitymanagerv1.GetEntityRequest, opts ...grpc.CallOption) (*entitymanagerv1.GetEntityResponse, error) {
	return nil, nil
}

func (m *mockEntityManagerAPIClient) OverrideEntity(ctx context.Context, in *entitymanagerv1.OverrideEntityRequest, opts ...grpc.CallOption) (*entitymanagerv1.OverrideEntityResponse, error) {
	return nil, nil
}

func (m *mockEntityManagerAPIClient) RemoveEntityOverride(ctx context.Context, in *entitymanagerv1.RemoveEntityOverrideRequest, opts ...grpc.CallOption) (*entitymanagerv1.RemoveEntityOverrideResponse, error) {
	return nil, nil
}

func (m *mockEntityManagerAPIClient) StreamEntityComponents(ctx context.Context, in *entitymanagerv1.StreamEntityComponentsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[entitymanagerv1.StreamEntityComponentsResponse], error) {
	return nil, nil
}

// mockTaskManagerAPIClient is a stub TaskManagerAPIClient for unit tests.
type mockTaskManagerAPIClient struct {
	listenAsAgentStream grpc.ServerStreamingClient[taskmanagerv1.ListenAsAgentResponse]
	listenAsAgentErr    error
	updateStatusResp    *taskmanagerv1.UpdateStatusResponse
	updateStatusErr     error
	updateStatusCalls   int
}

func (m *mockTaskManagerAPIClient) CreateTask(ctx context.Context, in *taskmanagerv1.CreateTaskRequest, opts ...grpc.CallOption) (*taskmanagerv1.CreateTaskResponse, error) {
	return nil, nil
}

func (m *mockTaskManagerAPIClient) GetTask(ctx context.Context, in *taskmanagerv1.GetTaskRequest, opts ...grpc.CallOption) (*taskmanagerv1.GetTaskResponse, error) {
	return nil, nil
}

func (m *mockTaskManagerAPIClient) QueryTasks(ctx context.Context, in *taskmanagerv1.QueryTasksRequest, opts ...grpc.CallOption) (*taskmanagerv1.QueryTasksResponse, error) {
	return nil, nil
}

func (m *mockTaskManagerAPIClient) UpdateStatus(ctx context.Context, in *taskmanagerv1.UpdateStatusRequest, opts ...grpc.CallOption) (*taskmanagerv1.UpdateStatusResponse, error) {
	m.updateStatusCalls++
	if m.updateStatusErr != nil {
		return nil, m.updateStatusErr
	}
	if m.updateStatusResp != nil {
		return m.updateStatusResp, nil
	}
	return &taskmanagerv1.UpdateStatusResponse{}, nil
}

func (m *mockTaskManagerAPIClient) CancelTask(ctx context.Context, in *taskmanagerv1.CancelTaskRequest, opts ...grpc.CallOption) (*taskmanagerv1.CancelTaskResponse, error) {
	return nil, nil
}

func (m *mockTaskManagerAPIClient) ListenAsAgent(ctx context.Context, in *taskmanagerv1.ListenAsAgentRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[taskmanagerv1.ListenAsAgentResponse], error) {
	if m.listenAsAgentErr != nil {
		return nil, m.listenAsAgentErr
	}
	if m.listenAsAgentStream == nil {
		return nil, status.Error(codes.Unavailable, "no mock stream configured")
	}
	return m.listenAsAgentStream, nil
}

func (m *mockTaskManagerAPIClient) ListenForManualControlFrames(ctx context.Context, in *taskmanagerv1.ListenForManualControlFramesRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[taskmanagerv1.ListenForManualControlFramesResponse], error) {
	return nil, nil
}

func (m *mockTaskManagerAPIClient) StreamTasks(ctx context.Context, in *taskmanagerv1.StreamTasksRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[taskmanagerv1.StreamTasksResponse], error) {
	return nil, nil
}

// mockStream is a stub ServerStreamingClient for testing stream behavior.
type mockStream struct {
	msgs []*taskmanagerv1.ListenAsAgentResponse
	errs []error
	idx  int
}

func (s *mockStream) Recv() (*taskmanagerv1.ListenAsAgentResponse, error) {
	if s.idx < len(s.msgs) {
		msg := s.msgs[s.idx]
		var err error
		if s.idx < len(s.errs) {
			err = s.errs[s.idx]
		}
		s.idx++
		return msg, err
	}
	return nil, context.Canceled
}

func (s *mockStream) Header() (metadata.MD, error) { return nil, nil }
func (s *mockStream) Trailer() metadata.MD         { return nil }
func (s *mockStream) CloseSend() error             { return nil }
func (s *mockStream) Context() context.Context     { return context.Background() }
func (s *mockStream) SendMsg(any) error            { return nil }
func (s *mockStream) RecvMsg(any) error            { return nil }

// mockHeartbeatRegistrar is a stub HeartbeatRegistrar for unit tests.
type mockHeartbeatRegistrar struct {
	sinks        map[int64]func(ctx context.Context)
	nextID       int64
	registered   int
	unregistered int
}

func newMockHeartbeatRegistrar() *mockHeartbeatRegistrar {
	return &mockHeartbeatRegistrar{sinks: make(map[int64]func(ctx context.Context))}
}

func (m *mockHeartbeatRegistrar) RegisterSink(sink func(ctx context.Context)) int64 {
	m.nextID++
	m.sinks[m.nextID] = sink
	m.registered++
	return m.nextID
}

func (m *mockHeartbeatRegistrar) UnregisterSink(id int64) {
	delete(m.sinks, id)
	m.unregistered++
}

// mockFileSvc is a minimal RuntimeFileService stub for adapter unit tests.
// It only implements ReadFile, WriteFile, and Resolve — the methods used by
// the adapter. All other methods panic to catch unexpected usage.
type mockFileSvc struct {
	files map[string][]byte
}

func newMockFileSvc() *mockFileSvc {
	return &mockFileSvc{files: make(map[string][]byte)}
}

func (m *mockFileSvc) ReadFile(ctx context.Context, relPath string) ([]byte, error) {
	data, ok := m.files[relPath]
	if !ok {
		return nil, constants.ErrNotFound
	}
	return data, nil
}

func (m *mockFileSvc) WriteFile(ctx context.Context, relPath string, data []byte, mode os.FileMode) error {
	m.files[relPath] = data
	return nil
}

func (m *mockFileSvc) Resolve(relPath string) string { return "/tmp/" + relPath }

func (m *mockFileSvc) MkdirAll(ctx context.Context, relPath string, mode os.FileMode) error {
	return nil
}
func (m *mockFileSvc) CreateRuntimeTree(ctx context.Context) error { return nil }
func (m *mockFileSvc) FileExists(ctx context.Context, relPath string) (bool, error) {
	return false, nil
}
func (m *mockFileSvc) Stat(ctx context.Context, relPath string) (os.FileInfo, error) { return nil, nil }
func (m *mockFileSvc) Remove(ctx context.Context, relPath string) error              { return nil }
func (m *mockFileSvc) RemoveAll(ctx context.Context, relPath string) error           { return nil }
func (m *mockFileSvc) ReadDir(ctx context.Context, relPath string) ([]os.DirEntry, error) {
	return nil, nil
}
func (m *mockFileSvc) Rename(ctx context.Context, oldPath, newPath string) error { return nil }
func (m *mockFileSvc) EnforceDirPermissions(ctx context.Context, relPath string, mode os.FileMode) error {
	return nil
}
func (m *mockFileSvc) EnforceFilePermissions(ctx context.Context, relPath string, mode os.FileMode) error {
	return nil
}
func (m *mockFileSvc) Rel(absPath string) (string, error)        { return absPath, nil }
func (m *mockFileSvc) RelFromAbs(absPath string) (string, error) { return absPath, nil }
func (m *mockFileSvc) OpenForAppend(ctx context.Context, relPath string, mode os.FileMode) (*os.File, error) {
	return nil, constants.ErrFileOpenFailed
}
func (m *mockFileSvc) OpenForRead(ctx context.Context, relPath string) (*os.File, error) {
	return nil, constants.ErrNotFound
}

// newTestAdapter creates an Adapter with mock gRPC clients and file service,
// bypassing the real dialLattice. The caller sets taskHandler and
// postureProvider before calling Start.
func newTestAdapter(cfg *LatticeConfig, fileSvc *mockFileSvc) *Adapter {
	return &Adapter{
		config:    cfg,
		logger:    newTestLogger(),
		fileSvc:   fileSvc,
		tlsConfig: nil,
		entityMgr: &mockEntityManagerAPIClient{},
		taskMgr:   &mockTaskManagerAPIClient{},
	}
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
