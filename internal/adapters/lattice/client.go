// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package lattice

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	entitymanagerv1 "github.com/g8e-ai/g8e/v2/internal/adapters/lattice/gen/anduril/entitymanager/v1"
	taskmanagerv1 "github.com/g8e-ai/g8e/v2/internal/adapters/lattice/gen/anduril/taskmanager/v1"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// HeartbeatRegistrar is the interface for registering periodic heartbeat
// sinks. pubsub.HeartbeatService satisfies this interface.
type HeartbeatRegistrar interface {
	RegisterSink(sink func(ctx context.Context)) int64
	UnregisterSink(id int64)
}

// TaskHandler processes a Lattice task assignment.
type TaskHandler func(ctx context.Context, task *taskmanagerv1.Task) error

// PostureProvider returns the current active governance posture.
type PostureProvider func() string

// Adapter is the Lattice gRPC adapter client. It manages the gRPC connection,
// entity presence publishing, and task stream subscription lifecycle.
type Adapter struct {
	config    *LatticeConfig
	logger    *slog.Logger
	fileSvc   fs.RuntimeFileService
	tlsConfig *tls.Config

	heartbeatSvc    HeartbeatRegistrar
	heartbeatSinkID int64

	conn      *grpc.ClientConn
	entityMgr entitymanagerv1.EntityManagerAPIClient
	taskMgr   taskmanagerv1.TaskManagerAPIClient

	entityID        string
	taskHandler     TaskHandler
	postureProvider PostureProvider

	running bool
	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewAdapter validates the configuration, dials the Lattice gRPC endpoint,
// and returns a ready-to-configure Adapter. The caller must invoke
// SetTaskHandler and SetPostureProvider before calling Start.
func NewAdapter(cfg *LatticeConfig, fileSvc fs.RuntimeFileService, tlsConfig *tls.Config, logger *slog.Logger) (*Adapter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	a := &Adapter{
		config:    cfg,
		logger:    logger,
		fileSvc:   fileSvc,
		tlsConfig: tlsConfig,
	}

	conn, entityMgr, taskMgr, err := a.dialLattice()
	if err != nil {
		return nil, err
	}

	a.conn = conn
	a.entityMgr = entityMgr
	a.taskMgr = taskMgr
	return a, nil
}

// SetHeartbeatService injects the heartbeat service for periodic presence
// republishing. Must be called before Start.
func (a *Adapter) SetHeartbeatService(hs HeartbeatRegistrar) {
	a.heartbeatSvc = hs
}

// SetTaskHandler registers the task handler called when a task is received.
// Must be called before Start. Start returns ErrLatticeNotInitialized if
// taskHandler is nil (fail-closed).
func (a *Adapter) SetTaskHandler(handler TaskHandler) {
	a.taskHandler = handler
}

// SetPostureProvider injects the posture provider. Must be called before
// Start. Start returns ErrLatticeNotInitialized if postureProvider is nil
// (fail-closed).
func (a *Adapter) SetPostureProvider(provider PostureProvider) {
	a.postureProvider = provider
}

// Start loads or creates the entity ID, publishes initial entity presence,
// registers a heartbeat sink for periodic republishing, and starts the task
// stream subscription goroutine.
func (a *Adapter) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return constants.ErrLatticeAlreadyRunning
	}
	if a.taskHandler == nil || a.postureProvider == nil {
		a.mu.Unlock()
		return constants.ErrLatticeNotInitialized
	}
	a.mu.Unlock()

	entityID, err := a.loadOrCreateEntityID(ctx)
	if err != nil {
		return err
	}

	adapterCtx, cancel := context.WithCancel(ctx)

	a.mu.Lock()
	a.entityID = entityID
	a.cancel = cancel
	a.running = true
	a.mu.Unlock()

	if err := a.PublishPresence(adapterCtx); err != nil {
		a.logger.Warn("Lattice: initial presence publish failed",
			"entity_id", a.entityID,
			"error", err)
	}

	if a.heartbeatSvc != nil {
		a.heartbeatSinkID = a.heartbeatSvc.RegisterSink(func(sinkCtx context.Context) {
			if err := a.PublishPresence(sinkCtx); err != nil {
				a.logger.Warn("Lattice: heartbeat presence publish failed",
					"entity_id", a.entityID,
					"error", err)
			}
		})
	} else {
		a.logger.Warn("Lattice: heartbeat service not set; presence will not be republished periodically")
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.subscribeToTasks(adapterCtx)
	}()

	a.logger.Info("Lattice adapter started",
		"endpoint", a.config.Endpoint,
		"entity_id", a.entityID)
	return nil
}

// Stop unregisters the heartbeat sink, cancels the adapter context, waits
// for goroutines to finish, and closes the gRPC connection.
func (a *Adapter) Stop(ctx context.Context) error {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return nil
	}
	a.running = false

	if a.heartbeatSvc != nil && a.heartbeatSinkID != 0 {
		a.heartbeatSvc.UnregisterSink(a.heartbeatSinkID)
		a.heartbeatSinkID = 0
	}

	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()

	a.wg.Wait()

	if a.conn != nil {
		if err := a.conn.Close(); err != nil {
			return fmt.Errorf("lattice: close gRPC connection: %w", err)
		}
	}

	a.logger.Info("Lattice adapter stopped")
	return nil
}

// IsRunning returns whether the adapter is currently running.
func (a *Adapter) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// dialLattice creates the gRPC connection to the Lattice endpoint and
// instantiates the EntityManager and TaskManager client stubs.
func (a *Adapter) dialLattice() (*grpc.ClientConn, entitymanagerv1.EntityManagerAPIClient, taskmanagerv1.TaskManagerAPIClient, error) {
	rpcCreds := NewClientCredentialsAuth(
		a.config.ClientID,
		a.config.ClientSecret,
		a.config.SandboxesToken,
		a.config.Endpoint+"/api/v1/oauth/token",
	)

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(a.tlsConfig)),
		grpc.WithPerRPCCredentials(rpcCreds),
		grpc.WithUnaryInterceptor(unaryRetryInterceptor(rpcCreds)),
	}

	conn, err := grpc.NewClient(a.config.Endpoint, dialOpts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %w", constants.ErrLatticeDialFailed, err)
	}

	entityMgr := entitymanagerv1.NewEntityManagerAPIClient(conn)
	taskMgr := taskmanagerv1.NewTaskManagerAPIClient(conn)
	return conn, entityMgr, taskMgr, nil
}

// loadOrCreateEntityID reads the persisted entity ID from the runtime
// directory. If no ID exists, it generates a new UUID and persists it.
func (a *Adapter) loadOrCreateEntityID(ctx context.Context) (string, error) {
	data, err := a.fileSvc.ReadFile(ctx, constants.LatticeEntityIDFilename)
	if err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	}

	if err != nil && !errors.Is(err, constants.ErrNotFound) {
		return "", fmt.Errorf("%w: %w", constants.ErrLatticeEntityIDReadFailed, err)
	}

	id := uuid.NewString()
	if err := a.fileSvc.WriteFile(ctx, constants.LatticeEntityIDFilename, []byte(id), constants.PermFilePrivate); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrLatticeEntityIDPersistFailed, err)
	}

	a.logger.Info("Lattice: generated new entity ID", "entity_id", id)
	return id, nil
}
