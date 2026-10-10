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
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
)

// OwnerPolicy is how the owner answers enrollment requests.
type OwnerPolicy int

const (
	// OwnerApprovesEach approves every enrollment request as it is announced.
	OwnerApprovesEach OwnerPolicy = iota
	// OwnerHoldsRequests records requests and leaves the decision to the test.
	OwnerHoldsRequests
)

// OwnerEvent is one SSE event the Gateway delivered to the owner's session.
type OwnerEvent struct {
	Type string
	Data json.RawMessage
	At   time.Time
}

// OwnerConsole is the platform owner with a CLI session whose SSE stream is
// open: the first user, receiving exactly the events the Gateway addresses to
// the owner. It answers enrollment requests through the real enrollment
// service, as `g8e operator approve` does.
type OwnerConsole struct {
	UserID       string
	CLISessionID string

	gateway *GatewayFixture
	policy  OwnerPolicy
	events  *eventLog[OwnerEvent]
	faults  *eventLog[error]

	mu        sync.Mutex
	requested map[string]struct{}
	decisions sync.WaitGroup
	closed    bool
}

// NewOwnerConsole creates the owner and opens their session stream. It must
// be created before any other user so it is the owner.
func NewOwnerConsole(t *testing.T, f *GatewayFixture, policy OwnerPolicy) *OwnerConsole {
	t.Helper()
	ctx := t.Context()

	user, err := f.Service.GetUserService().CreateUser(ctx)
	require.NoError(t, err)
	ownerID, err := f.Service.GetUserService().FirstUserID(ctx)
	require.NoError(t, err)
	require.Equal(t, user.ID, ownerID, "owner console must be the first user")

	now := time.Now().UTC()
	session := models.CLISession{
		ID:                uuid.NewString(),
		UserID:            user.ID,
		CreatedAt:         now,
		ExpiresAt:         now.Add(time.Hour),
		AbsoluteExpiresAt: now.Add(time.Hour),
		IdleExpiresAt:     now.Add(time.Hour),
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       "mTLS",
	}
	sessionBytes, err := json.Marshal(session)
	require.NoError(t, err)
	require.NoError(t, f.Service.GetDocStore().DocSet(ctx, marshaler.CollectionName(constants.CollectionCLISessions), session.ID, sessionBytes))

	c := &OwnerConsole{
		UserID:       user.ID,
		CLISessionID: session.ID,
		gateway:      f,
		policy:       policy,
		events:       newEventLog[OwnerEvent](),
		faults:       newEventLog[error](),
		requested:    make(map[string]struct{}),
	}
	unregister := f.Service.GetGatewayWebSocketHandler().RegisterHandler(pubsub.SSECLIChannel(session.ID), c.receive)

	t.Cleanup(func() {
		unregister()
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		c.decisions.Wait()
		for _, fault := range c.faults.snapshot() {
			t.Errorf("owner console: %v", fault)
		}
	})
	return c
}

// receive runs on the publisher's goroutine under the broker's handler lock:
// it records the event and hands any decision to its own goroutine.
func (c *OwnerConsole) receive(_ string, data []byte) {
	var published models.SSEPublishedEvent
	if err := json.Unmarshal(data, &published); err != nil {
		c.faults.append(fmt.Errorf("decode published event: %w", err))
		return
	}
	var push models.SSEPushPayload
	if err := json.Unmarshal(published.Payload, &push); err != nil {
		c.faults.append(fmt.Errorf("decode push payload: %w", err))
		return
	}
	var envelope struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(push.Event, &envelope); err != nil {
		c.faults.append(fmt.Errorf("decode event envelope: %w", err))
		return
	}
	event := OwnerEvent{Type: envelope.Type, Data: envelope.Data, At: time.Now()}
	c.events.append(event)

	requestID, ok := enrollmentRequestID(event)
	if !ok {
		return
	}
	c.mu.Lock()
	_, seen := c.requested[requestID]
	c.requested[requestID] = struct{}{}
	decide := !seen && !c.closed && c.policy == OwnerApprovesEach
	if decide {
		c.decisions.Add(1)
	}
	c.mu.Unlock()
	if decide {
		go func() {
			defer c.decisions.Done()
			_, err := c.gateway.Service.GetPlatformEnrollmentService().Decide(context.Background(), c.UserID, models.PlatformEnrollmentDecisionRequest{
				RequestID: requestID,
				Decision:  models.PlatformEnrollmentDecisionApprove,
			})
			if err != nil {
				c.faults.append(fmt.Errorf("approve %s: %w", requestID, err))
			}
		}()
	}
}

// enrollmentRequestID returns the request an approvals event announces.
func enrollmentRequestID(event OwnerEvent) (string, bool) {
	if event.Type != string(constants.EventPlatformApprovalsChanged) {
		return "", false
	}
	var payload models.ApprovalsChangedPayload
	if err := json.Unmarshal(event.Data, &payload); err != nil {
		return "", false
	}
	if payload.Subject != models.ApprovalsChangedEnrollments || payload.RequestID == "" {
		return "", false
	}
	return payload.RequestID, true
}

// Events returns every event delivered to the owner so far.
func (c *OwnerConsole) Events() []OwnerEvent {
	return c.events.snapshot()
}

// Mark returns a position in the event record for EventsAfter and
// AwaitEventAfter.
func (c *OwnerConsole) Mark() int {
	return c.events.len()
}

// EventsAfter returns the events recorded at or after mark.
func (c *OwnerConsole) EventsAfter(mark int) []OwnerEvent {
	events := c.events.snapshot()
	if mark > len(events) {
		return nil
	}
	return events[mark:]
}

// AwaitEventAfter blocks until an event recorded at or after mark satisfies
// match.
func (c *OwnerConsole) AwaitEventAfter(ctx context.Context, mark int, match func(OwnerEvent) bool) (OwnerEvent, error) {
	event, _, err := c.events.await(ctx, mark, match)
	if err != nil {
		return OwnerEvent{}, fmt.Errorf("owner console: await event: %w", err)
	}
	return event, nil
}

// EnrollmentRequests returns the distinct enrollment request IDs announced to
// the owner, in announcement order.
func (c *OwnerConsole) EnrollmentRequests() []string {
	return distinctEnrollmentRequests(c.events.snapshot())
}

// AwaitEnrollmentRequests blocks until at least n distinct enrollment requests
// were announced, and returns them.
func (c *OwnerConsole) AwaitEnrollmentRequests(ctx context.Context, n int) ([]string, error) {
	events, err := c.events.awaitAll(ctx, func(events []OwnerEvent) bool {
		return len(distinctEnrollmentRequests(events)) >= n
	})
	ids := distinctEnrollmentRequests(events)
	if err != nil {
		return ids, fmt.Errorf("owner console: await %d enrollment requests (have %d): %w", n, len(ids), err)
	}
	return ids, nil
}

func distinctEnrollmentRequests(events []OwnerEvent) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, event := range events {
		id, ok := enrollmentRequestID(event)
		if !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

// ApproveBatch approves requestIDs with the fingerprints the owner sees in the
// pending list, in decision batches no larger than the Gateway accepts.
func (c *OwnerConsole) ApproveBatch(ctx context.Context, requestIDs []string) error {
	enrollSvc := c.gateway.Service.GetPlatformEnrollmentService()
	pending, err := enrollSvc.ListPending(ctx)
	if err != nil {
		return fmt.Errorf("owner console: list pending: %w", err)
	}
	byID := make(map[string]models.PlatformEnrollmentPendingRequest, len(pending.Requests))
	for _, request := range pending.Requests {
		byID[request.RequestID] = request
	}
	targets := make([]models.PlatformEnrollmentDecisionTarget, 0, len(requestIDs))
	var missing []string
	for _, id := range requestIDs {
		request, ok := byID[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		targets = append(targets, models.PlatformEnrollmentDecisionTarget{RequestID: id, Fingerprints: request.Fingerprints})
	}
	if len(missing) > 0 {
		return fmt.Errorf("owner console: %d requests not pending: %s", len(missing), strings.Join(missing, ", "))
	}
	for start := 0; start < len(targets); start += constants.PlatformEnrollmentMaxDecisionBatch {
		end := min(start+constants.PlatformEnrollmentMaxDecisionBatch, len(targets))
		_, err := enrollSvc.DecideBatch(ctx, c.UserID, models.PlatformEnrollmentBatchDecisionRequest{
			Requests: targets[start:end],
			Decision: models.PlatformEnrollmentDecisionApprove,
		})
		if err != nil {
			return fmt.Errorf("owner console: approve batch [%d,%d): %w", start, end, err)
		}
	}
	return nil
}

// StoredRequest reads an enrollment request as the Gateway committed it.
func (c *OwnerConsole) StoredRequest(ctx context.Context, requestID string) (models.PlatformEnrollmentRequest, error) {
	doc, err := c.gateway.Service.GetDocStore().DocGet(ctx, marshaler.CollectionName(constants.CollectionPlatformEnrollments), requestID)
	if err != nil {
		return models.PlatformEnrollmentRequest{}, fmt.Errorf("owner console: load request %s: %w", requestID, err)
	}
	if doc == nil {
		return models.PlatformEnrollmentRequest{}, fmt.Errorf("owner console: request %s: %w", requestID, constants.ErrPlatformEnrollmentRequestNotFound)
	}
	wire, err := json.Marshal(doc.ForWire())
	if err != nil {
		return models.PlatformEnrollmentRequest{}, fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}
	var request models.PlatformEnrollmentRequest
	if err := json.Unmarshal(wire, &request); err != nil {
		return models.PlatformEnrollmentRequest{}, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalDocument, err)
	}
	request.ID = requestID
	return request, nil
}

// CommittedOperatorCertificate returns the Operator identity certificate the
// Gateway issued for an enrollment request. The request's CertificateSerial
// names the companion CLI certificate, not the Operator's identity.
func (c *OwnerConsole) CommittedOperatorCertificate(ctx context.Context, requestID string) (*x509.Certificate, error) {
	request, err := c.StoredRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if request.Issued == nil || request.Issued.Operator == nil {
		return nil, fmt.Errorf("owner console: request %s committed no Operator credentials", requestID)
	}
	block, _ := pem.Decode([]byte(request.Issued.Operator.OperatorCert))
	if block == nil {
		return nil, fmt.Errorf("owner console: request %s: Operator certificate is not PEM", requestID)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("owner console: request %s: parse Operator certificate: %w", requestID, err)
	}
	return cert, nil
}

// IsOperatorStatusEvent reports whether event is a status.updated event of
// eventType for operatorID.
func IsOperatorStatusEvent(event OwnerEvent, operatorID string, eventType constants.EventType) bool {
	if event.Type != string(eventType) {
		return false
	}
	var payload models.OperatorStatusUpdatedPayload
	if err := json.Unmarshal(event.Data, &payload); err != nil {
		return false
	}
	return payload.OperatorID == operatorID
}
