// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/g8e-ai/g8e/v2/internal/cli/sse"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// deploymentEvents is the deploying CLI's one SSE subscription to the Gateway.
// A worker launched by this deploy presents its launch ID when it creates its
// enrollment request and when it establishes its command subscription; the
// Gateway announces both to the owner's CLI sessions keyed by that ID, so
// staging and readiness are learned from events and nothing polls.
type deploymentEvents struct {
	mu      sync.Mutex
	watches map[string]*deploymentWatch
}

// deploymentWatch holds what the Gateway has announced for one launch.
type deploymentWatch struct {
	mu            sync.Mutex
	requestID     string
	sessionID     string
	status        constants.OperatorStatus
	requestSeen   chan struct{}
	readinessSeen chan struct{}
}

func newDeploymentEvents() *deploymentEvents {
	return &deploymentEvents{watches: make(map[string]*deploymentWatch)}
}

// watch registers launchID. It must run before the worker starts, because the
// stream delivers only live events.
func (e *deploymentEvents) watch(launchID string) *deploymentWatch {
	w := &deploymentWatch{requestSeen: make(chan struct{}), readinessSeen: make(chan struct{})}
	e.mu.Lock()
	e.watches[launchID] = w
	e.mu.Unlock()
	return w
}

// deploymentEventEnvelope is the nested event object of an SSE push payload.
type deploymentEventEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// handle routes one SSE frame to the watch of the launch it names. Events for
// other launches, and every other event type, are ignored.
func (e *deploymentEvents) handle(eventType, data string) {
	var push models.SSEPushPayload
	if err := json.Unmarshal([]byte(data), &push); err != nil {
		return
	}
	var event deploymentEventEnvelope
	if err := json.Unmarshal(push.Event, &event); err != nil {
		return
	}
	if event.Type == "" {
		event.Type = eventType
	}
	switch event.Type {
	case string(constants.EventPlatformApprovalsChanged):
		var payload models.ApprovalsChangedPayload
		if json.Unmarshal(event.Data, &payload) != nil || payload.DeploymentID == "" || payload.RequestID == "" {
			return
		}
		if w := e.lookup(payload.DeploymentID); w != nil {
			w.setRequest(payload.RequestID)
		}
	case string(constants.EventOperatorStatusUpdatedActive),
		string(constants.EventOperatorStatusUpdatedStale),
		string(constants.EventOperatorStatusUpdatedStopped),
		string(constants.EventOperatorStatusUpdatedTerminated):
		var payload models.OperatorStatusUpdatedPayload
		if json.Unmarshal(event.Data, &payload) != nil || payload.DeploymentID == "" {
			return
		}
		if w := e.lookup(payload.DeploymentID); w != nil {
			w.setReadiness(payload.OperatorSessionID, payload.Status)
		}
	}
}

func (e *deploymentEvents) lookup(launchID string) *deploymentWatch {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.watches[launchID]
}

func (w *deploymentWatch) setRequest(requestID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.requestID == "" {
		w.requestID = requestID
		close(w.requestSeen)
	}
}

func (w *deploymentWatch) setReadiness(sessionID string, status constants.OperatorStatus) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == constants.OperatorStatusActive {
		return
	}
	first := w.status == ""
	w.sessionID, w.status = sessionID, status
	if first {
		close(w.readinessSeen)
	}
}

// awaitStaged returns the request ID the worker's enrollment request was
// announced with. It returns "" when the worker became ready first, which is a
// worker whose retained credentials were already enrolled.
func (w *deploymentWatch) awaitStaged(ctx context.Context) (string, error) {
	select {
	case <-w.requestSeen:
	case <-w.readinessSeen:
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.requestID, nil
	case <-ctx.Done():
		return "", fmt.Errorf("%w: %w", constants.ErrOperatorDeployFailed, ctx.Err())
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.requestID, nil
}

// awaitReady returns the session ID of the worker's command subscription once
// the Gateway announces it. A subscription announced for an Operator that is
// not active is a failure, not readiness.
func (w *deploymentWatch) awaitReady(ctx context.Context) (string, error) {
	select {
	case <-w.readinessSeen:
	case <-ctx.Done():
		return "", fmt.Errorf("%w: %w", constants.ErrOperatorDeployFailed, ctx.Err())
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status != constants.OperatorStatusActive {
		return "", fmt.Errorf("%w: operator established its command subscription as %s", constants.ErrOperatorDeployFailed, w.status)
	}
	return w.sessionID, nil
}

// connectDeploymentEvents opens the owner's CLI-session SSE stream and returns
// once it is established, so no event for a worker started afterwards is
// missed. The returned stop ends the stream.
func connectDeploymentEvents(ctx context.Context, httpClient *http.Client, baseURL, cliSessionID string) (*deploymentEvents, func(), error) {
	events := newDeploymentEvents()
	client := sse.NewClient(baseURL+constants.APIPaths.SSEStream+"?since_id=0", httpClient)
	client.SetHeader(constants.HeaderCLISessionID, cliSessionID)
	connected := make(chan struct{})
	var once sync.Once
	client.SetOnConnect(func() { once.Do(func() { close(connected) }) })

	streamCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		client.Run(streamCtx, events.handle)
	}()
	stop := func() {
		cancel()
		<-done
	}

	select {
	case <-connected:
		return events, stop, nil
	case <-done:
		cancel()
		return nil, nil, fmt.Errorf("%w: deployment event stream closed before it connected", constants.ErrOperatorDeployFailed)
	case <-ctx.Done():
		stop()
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrOperatorDeployFailed, ctx.Err())
	}
}
