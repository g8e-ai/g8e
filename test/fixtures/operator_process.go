// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration || e2e

package fixtures

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/g8e-ai/g8e/v2/protocol"
)

// operatorStopTimeout bounds how long cleanup waits for one Operator to stop.
const operatorStopTimeout = 30 * time.Second

// errOperatorStopped is the cause recorded when a test stops an Operator.
var errOperatorStopped = errors.New("operator process stopped")

// OperatorHost is one machine an Operator runs on: a runtime root and a master
// key provisioned outside it. Successive Operator processes on the same host
// share both, as a restarted Operator on a real machine would.
type OperatorHost struct {
	WorkingDir    string
	MasterKeyFile string
}

// NewOperatorHost provisions an isolated host.
func NewOperatorHost(t *testing.T) *OperatorHost {
	t.Helper()
	masterKeyFile, err := keystore.ProvisionMasterKeyFile(testutil.TempDir(t))
	require.NoError(t, err)
	return &OperatorHost{WorkingDir: testutil.TempDir(t), MasterKeyFile: masterKeyFile}
}

// OperatorProcessOptions tunes one Operator process.
type OperatorProcessOptions struct {
	// HeartbeatInterval defaults to one second.
	HeartbeatInterval time.Duration
}

// OperatorHeartbeat is one heartbeat the Gateway received from an Operator.
type OperatorHeartbeat struct {
	Payload []byte
	At      time.Time
}

// OperatorProcess is one in-process outbound Operator, started exactly as
// `g8e operator start` starts it, dialing the Gateway through a wire witness.
// Identity is the Operator's in-memory mTLS identity: the only place its
// certificate is held.
type OperatorProcess struct {
	Identity *certs.ClientIdentity

	gateway *GatewayFixture
	cancel  context.CancelFunc

	started  chan struct{}
	runtime  *serve.OperatorRuntime
	startErr error

	// alive ends, with a cause, when the service fails or the process stops,
	// so waits on this Operator end with the reason instead of a timeout.
	alive context.Context
	kill  context.CancelCauseFunc

	heartbeats *eventLog[OperatorHeartbeat]

	mu         sync.Mutex
	unregister func()
	stopOnce   sync.Once
	stopErr    error
}

// LaunchOperator starts an Operator on host that dials the Gateway through the
// witness. It returns immediately; AwaitStarted blocks through enrollment.
// The process is stopped when the test ends.
func (w *OperatorWireWitness) LaunchOperator(t *testing.T, host *OperatorHost, opts OperatorProcessOptions) *OperatorProcess {
	t.Helper()
	if opts.HeartbeatInterval == 0 {
		opts.HeartbeatInterval = time.Second
	}

	identity := certs.NewClientIdentity(tls.Certificate{})
	w.Present(identity)

	ctx, cancel := context.WithCancel(context.Background())
	alive, kill := context.WithCancelCause(context.Background())
	p := &OperatorProcess{
		Identity:   identity,
		gateway:    w.gateway,
		cancel:     cancel,
		started:    make(chan struct{}),
		alive:      alive,
		kill:       kill,
		heartbeats: newEventLog[OperatorHeartbeat](),
	}

	serveOpts := serve.ServeOperatorOptions{
		Endpoint:          constants.LocalhostIP,
		HTTPPort:          w.PlainPort(),
		HTTPSPort:         w.TLSPort(),
		WorkingDir:        host.WorkingDir,
		LaunchDir:         host.WorkingDir,
		ExecutionVault:    true,
		NoGit:             true,
		HeartbeatInterval: opts.HeartbeatInterval,
		MasterKeyFile:     host.MasterKeyFile,
	}
	deps := serve.OperatorRuntimeDeps{Logger: testutil.NewTestLogger(), ClientIdentity: identity}

	go func() {
		defer close(p.started)
		p.runtime, p.startErr = serve.StartOperator(ctx, serveOpts, serve.VersionInfo{Version: "test", BuildID: "test"}, deps)
		if p.startErr != nil {
			p.kill(fmt.Errorf("operator start failed: %w", p.startErr))
			return
		}
		go func() {
			select {
			case err := <-p.runtime.Failed():
				p.kill(fmt.Errorf("operator service failed: %w", err))
			case <-p.runtime.Done():
				p.kill(errors.New("operator service requested shutdown"))
			case <-alive.Done():
			}
		}()
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), operatorStopTimeout)
		defer cancel()
		if err := p.Stop(ctx); err != nil {
			t.Errorf("operator process: stop: %v", err)
		}
	})
	return p
}

// AwaitStarted blocks until the Operator has enrolled and its service is
// running, then begins recording its heartbeats.
func (p *OperatorProcess) AwaitStarted(ctx context.Context) error {
	select {
	case <-p.started:
	case <-ctx.Done():
		return fmt.Errorf("operator process: await start: %w", ctx.Err())
	}
	if p.startErr != nil {
		return fmt.Errorf("operator process: start: %w", p.startErr)
	}
	workload, err := p.Workload()
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unregister == nil {
		channel := pubsub.HeartbeatChannel(workload.OperatorID, workload.OperatorSessionID)
		p.unregister = p.gateway.Service.GetGatewayWebSocketHandler().RegisterHandler(channel, func(_ string, data []byte) {
			p.heartbeats.append(OperatorHeartbeat{Payload: append([]byte(nil), data...), At: time.Now()})
		})
	}
	return nil
}

// Leaf is the certificate the Operator currently holds in memory.
func (p *OperatorProcess) Leaf() (*x509.Certificate, error) {
	cert, ok := p.Identity.GetCertificate()
	if !ok {
		return nil, errors.New("operator process: no certificate in memory")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("operator process: parse leaf: %w", err)
	}
	return leaf, nil
}

// Workload is the Operator identity carried by the certificate in memory.
func (p *OperatorProcess) Workload() (protocol.OperatorWorkload, error) {
	leaf, err := p.Leaf()
	if err != nil {
		return protocol.OperatorWorkload{}, err
	}
	workload, ok := OperatorWorkloadOf(leaf)
	if !ok {
		return protocol.OperatorWorkload{}, fmt.Errorf("operator process: leaf %s carries no Operator identity", leaf.Subject)
	}
	return workload, nil
}

// HeartbeatMark returns a position in the heartbeat record for
// AwaitHeartbeatAfter.
func (p *OperatorProcess) HeartbeatMark() int {
	return p.heartbeats.len()
}

// AwaitHeartbeat blocks until the Gateway receives a heartbeat from this
// Operator. AwaitStarted must have returned first.
func (p *OperatorProcess) AwaitHeartbeat(ctx context.Context) (OperatorHeartbeat, error) {
	return p.AwaitHeartbeatAfter(ctx, 0)
}

// AwaitHeartbeatAfter blocks until the Gateway receives a heartbeat recorded
// at or after mark. It ends early with the cause if the Operator fails.
func (p *OperatorProcess) AwaitHeartbeatAfter(ctx context.Context, mark int) (OperatorHeartbeat, error) {
	heartbeat, _, err := p.awaitHeartbeat(ctx, mark)
	return heartbeat, err
}

func (p *OperatorProcess) awaitHeartbeat(ctx context.Context, mark int) (OperatorHeartbeat, int, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stop := context.AfterFunc(p.alive, func() { cancel(context.Cause(p.alive)) })
	defer stop()

	heartbeat, index, err := p.heartbeats.await(ctx, mark, func(OperatorHeartbeat) bool { return true })
	if err != nil {
		return OperatorHeartbeat{}, 0, fmt.Errorf("operator process: await heartbeat: %w", context.Cause(ctx))
	}
	return heartbeat, index, nil
}

// AwaitHeartbeatRecordedAfter blocks until the Gateway has recorded a
// heartbeat published at or after mark, and returns that heartbeat. The broker
// delivers a publish to in-process handlers, this fixture's included, before
// the Gateway records it, and processes one connection's publishes in order;
// the next heartbeat reaching this fixture therefore proves the previous one
// was recorded.
func (p *OperatorProcess) AwaitHeartbeatRecordedAfter(ctx context.Context, mark int) (OperatorHeartbeat, error) {
	recorded, index, err := p.awaitHeartbeat(ctx, mark)
	if err != nil {
		return OperatorHeartbeat{}, err
	}
	if _, _, err := p.awaitHeartbeat(ctx, index+1); err != nil {
		return OperatorHeartbeat{}, err
	}
	return recorded, nil
}

// Stop stops the Operator as a process exit would: its in-memory identity is
// gone with it. Stop is idempotent.
func (p *OperatorProcess) Stop(ctx context.Context) error {
	p.stopOnce.Do(func() {
		p.kill(errOperatorStopped)
		p.cancel()
		select {
		case <-p.started:
		case <-ctx.Done():
			p.stopErr = fmt.Errorf("start did not return: %w", ctx.Err())
			return
		}
		p.mu.Lock()
		if p.unregister != nil {
			p.unregister()
		}
		p.mu.Unlock()
		if p.runtime != nil {
			p.stopErr = p.runtime.Stop(ctx)
		}
	})
	return p.stopErr
}
