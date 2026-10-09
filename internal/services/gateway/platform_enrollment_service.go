// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govpkg "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
	"github.com/g8e-ai/g8e/v2/protocol"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)

type platformEnrollmentApprovedUpdate struct {
	State                string     `json:"state"`
	IssuanceLeaseOwner   string     `json:"issuance_lease_owner"`
	IssuanceLeaseExpires *time.Time `json:"issuance_lease_expires_at"`
	LastTransitionAt     time.Time  `json:"last_transition_at"`
}

type platformEnrollmentExpiredUpdate struct {
	State            string    `json:"state"`
	LastTransitionAt time.Time `json:"last_transition_at"`
}

func marshalPlatformEnrollmentApprovedUpdate(now time.Time) (json.RawMessage, error) {
	data, err := json.Marshal(platformEnrollmentApprovedUpdate{
		State:                string(models.PlatformEnrollmentStateApproved),
		IssuanceLeaseExpires: nil,
		LastTransitionAt:     now,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal approved platform enrollment update: %w", err)
	}
	return data, nil
}

func marshalPlatformEnrollmentExpiredUpdate(now time.Time) (json.RawMessage, error) {
	data, err := json.Marshal(platformEnrollmentExpiredUpdate{
		State:            string(models.PlatformEnrollmentStateExpired),
		LastTransitionAt: now,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal expired platform enrollment update: %w", err)
	}
	return data, nil
}

// PlatformEnrollmentService owns the platform workload enrollment
// lifecycle: request creation/deduplication/quotas, owner decisions,
// issuance leases, idempotent component issuance, reconciliation of
// expired leases, and managed cleanup.
//
// All mutations route through the canonical governance gauntlet by
// submitting GovernanceEnvelope messages to the injected
// governance.EnvelopeProcessor (the gateway's in-process
// OperatorPubSubService). The single exception
// is the initial pending-request write, which is a non-mutation DocSet
// performed before the CREATE envelope is submitted for audit (CREATE is
// classified as non-mutation in IsMutation, so invariant 17 does not
// apply).
//
// The issuance lease is the recoverable saga boundary. The enrollment
// service acquires the lease (approved -> issuing with lease owner and
// lease expiry) before submitting the ISSUE envelope. The ISSUE handler
// signs the certificate and transitions issuing -> completed. If the
// process crashes between ISSUE and the downstream PERSIST_POLICY or
// CREATE_SESSION handlers, the next completion retry finds the request
// in the completed state with stored issued material and re-submits the
// downstream envelopes (which are idempotent). Reconciliation recovers
// an expired lease by transitioning issuing -> approved so a new
// completion attempt can re-acquire.
type PlatformEnrollmentService struct {
	db        *DocumentStoreService
	userSvc   *UserService
	envProc   governance.EnvelopeProcessor
	stateRoot governance.StateRootProvider
	posture   string
	approvals *ApprovalsChangePublisher
	logger    *slog.Logger

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.Mutex
}

// NewPlatformEnrollmentService creates a new PlatformEnrollmentService.
// The envProc and stateRoot are the gateway's in-process governance
// pipeline: envProc is the concrete OperatorPubSubService, stateRoot is the
// StateRootService. StartCleanup must be called to register the managed
// cleanup goroutine with the gateway lifecycle context.
func NewPlatformEnrollmentService(
	db *DocumentStoreService,
	userSvc *UserService,
	envProc governance.EnvelopeProcessor,
	stateRoot governance.StateRootProvider,
	posture string,
	approvals *ApprovalsChangePublisher,
	logger *slog.Logger,
) *PlatformEnrollmentService {
	return &PlatformEnrollmentService{
		db:        db,
		userSvc:   userSvc,
		envProc:   envProc,
		stateRoot: stateRoot,
		posture:   posture,
		approvals: approvals,
		logger:    logger,
	}
}

// StartCleanup registers the managed cleanup goroutine with the gateway
// lifecycle context. The goroutine periodically reconciles expired
// issuance leases and removes terminal request records past the
// retention window. StopCleanup cancels the goroutine and waits for it
// to exit. Calling StartCleanup more than once without StopCleanup
// between calls is a no-op.
func (s *PlatformEnrollmentService) StartCleanup(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	cleanupCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.running = true
	s.wg.Add(1)
	go s.runCleanup(cleanupCtx)
}

// StopCleanup cancels the managed cleanup goroutine and waits for it to
// exit. Safe to call when cleanup was never started or already stopped.
func (s *PlatformEnrollmentService) StopCleanup() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *PlatformEnrollmentService) runCleanup(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(constants.PlatformEnrollmentCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ReconcileExpiredLeases(ctx); err != nil {
				s.logger.Warn("platform enrollment: reconcile expired leases failed", "error", err)
			}
			if err := s.CleanupTerminalRequests(ctx); err != nil {
				s.logger.Warn("platform enrollment: cleanup terminal requests failed", "error", err)
			}
		}
	}
}

// CreateRequest validates bootstrap state and CSRs, deduplicates a live
// request for the same component kind, instance ID, and key fingerprint
// set, atomically reserves capacity and inserts the pending request (CREATE
// is classified as non-mutation), submits a PLATFORM_ENROLLMENT_CREATE
// envelope for audit, and returns the request ID, component name, fingerprints,
// approval URL, and expiry. The requester supplies token_hash; the Gateway
// stores only that hash and never receives or returns the raw token on creation.
func (s *PlatformEnrollmentService) CreateRequest(ctx context.Context, req models.PlatformEnrollmentCreateRequest, approvalURLBase string) (*models.PlatformEnrollmentCreateResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	// Invariant 1: a gateway with no users never issues a platform
	// certificate. Request creation requires a bootstrapped gateway.
	hasUsers, err := s.userSvc.HasAnyUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: check bootstrap: %w", err)
	}
	if !hasUsers {
		return nil, constants.ErrPlatformEnrollmentRequiresBootstrap
	}

	fingerprints, err := validatePlatformEnrollmentRequest(req)
	if err != nil {
		return nil, err
	}

	componentName, err := req.ComponentKind.CanonicalName(req.AppName)
	if err != nil {
		return nil, err
	}

	requestID, err := uuid.NewString()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(constants.PlatformEnrollmentRequestTTL)

	persistedReq := &models.PlatformEnrollmentRequest{
		ID:                requestID,
		TokenHash:         req.TokenHash,
		ComponentKind:     req.ComponentKind,
		ComponentName:     componentName,
		AppName:           req.AppName,
		InstanceID:        req.InstanceID,
		Hostname:          req.Hostname,
		SystemFingerprint: req.SystemFingerprint,
		App:               req.App,
		Operator:          req.Operator,
		Fingerprints:      fingerprints,
		State:             models.PlatformEnrollmentStatePending,
		CreatedAt:         now,
		ExpiresAt:         expiresAt,
		LastTransitionAt:  now,
	}

	// Reserve and insert the pending request document. CREATE is classified
	// as non-mutation in IsMutation, so invariant 17 (no direct DocSet
	// for mutations) does not apply to this initial write. The CSR PEM
	// is public material; the token hash is a stored credential. Neither
	// appears in the audited CREATE envelope payload.
	existing, err := s.createRequestRecord(ctx, persistedReq)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if req.DeploymentID != "" {
			s.approvals.EnrollmentRequested(ctx, existing.ID, req.DeploymentID)
		}
		return &models.PlatformEnrollmentCreateResponse{
			RequestID:     existing.ID,
			ComponentKind: existing.ComponentKind,
			ComponentName: existing.ComponentName,
			Fingerprints:  existing.Fingerprints,
			ApprovalURL:   buildApprovalURL(approvalURLBase, existing.ID),
			ExpiresAt:     existing.ExpiresAt,
		}, nil
	}

	// Submit the CREATE envelope for audit. The handler is audit-only:
	// it decodes the payload, returns a receipt summary, and writes
	// nothing to the doc store.
	_, err = s.submitEnvelope(ctx, constants.PlatformEnrollmentActionCreate, &commonv1.PlatformEnrollmentGovernancePayload{
		Action:        string(constants.PlatformEnrollmentActionCreate),
		Intent:        string(constants.PlatformEnrollmentIntentRequest),
		RequestId:     requestID,
		ComponentKind: payloadComponentKind(req.ComponentKind),
		InstanceId:    req.InstanceID,
		Fingerprints:  payloadFingerprints(fingerprints),
	})
	if err == nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		default:
		}
	}
	if err != nil {
		// The response was not returned, so the requester still holds its token
		// and can resubmit the same request after the failed audit.
		_, cleanupErr := s.db.db.ExecContext(context.WithoutCancel(ctx),
			`DELETE FROM documents WHERE collection = ? AND id = ? AND json_extract(data, '$.state') = ? AND json_extract(data, '$.token_hash') = ?`,
			platformEnrollmentCollectionName(), requestID, models.PlatformEnrollmentStatePending, req.TokenHash)
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("release failed enrollment reservation: %w", cleanupErr))
		}
		return nil, fmt.Errorf("platform enrollment: create envelope: %w", err)
	}

	s.logger.Info("platform enrollment request created",
		"request_id", requestID,
		"component_kind", string(req.ComponentKind),
		"instance_id", req.InstanceID,
		"expires_at", expiresAt)
	s.approvals.EnrollmentRequested(ctx, requestID, req.DeploymentID)

	return &models.PlatformEnrollmentCreateResponse{
		RequestID:     requestID,
		ComponentKind: req.ComponentKind,
		ComponentName: componentName,
		Fingerprints:  fingerprints,
		ApprovalURL:   buildApprovalURL(approvalURLBase, requestID),
		ExpiresAt:     expiresAt,
	}, nil
}

// GetStatus returns the requester-visible state and expiry for a request
// identified by its opaque token. The token is hashed and looked up by
// hash; the raw token is never stored. If the request has expired and
// is still in a non-terminal state, it is atomically transitioned to
// the expired state before returning.
func (s *PlatformEnrollmentService) GetStatus(ctx context.Context, token string) (*models.PlatformEnrollmentStatusResponse, error) {
	if token == "" {
		return nil, constants.ErrPlatformEnrollmentTokenRequired
	}
	req, err := s.loadByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if req.State.IsTerminal() {
		return s.statusResponse(req), nil
	}
	if time.Now().UTC().After(req.ExpiresAt) {
		s.expireRequest(ctx, req)
		return nil, constants.ErrPlatformEnrollmentRequestExpired
	}
	return s.statusResponse(req), nil
}

// WaitForDecision holds one status request while its enrollment is pending.
// Subscribe before reading the snapshot so a concurrent committed decision
// cannot fall between the read and the wait. Only decision events trigger a
// subsequent read; expiry and cancellation bound the wait.
func (s *PlatformEnrollmentService) WaitForDecision(ctx context.Context, token string) (*models.PlatformEnrollmentStatusResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.approvals.publisher.pubsub == nil {
		return nil, constants.ErrPlatformEnrollmentDepsRequired
	}
	changed := make(chan struct{}, 1)
	unregister := s.approvals.publisher.pubsub.RegisterHandler(string(constants.EventPlatformApprovalsChanged), func(_ string, _ []byte) {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	defer unregister()

	status, err := s.GetStatus(ctx, token)
	if err != nil || status.State != models.PlatformEnrollmentStatePending {
		return status, err
	}
	expiry := time.NewTimer(time.Until(status.ExpiresAt))
	defer expiry.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-expiry.C:
			return s.GetStatus(ctx, token)
		case <-changed:
			status, err = s.GetStatus(ctx, token)
			if err != nil || status.State != models.PlatformEnrollmentStatePending {
				return status, err
			}
		}
	}
}

// Decide authorizes an owner decision (approve or deny) on a pending
// request. The actorUserID is derived from authenticated context
// (web session or mTLS CLI) by the controller; it must be the active
// first user. The decision is submitted as a PLATFORM_ENROLLMENT_DECIDE
// envelope through the governance gauntlet. The handler performs the
// conditional pending -> approved|denied transition and stamps the
// approving user ID and envelope/receipt IDs.
func (s *PlatformEnrollmentService) Decide(ctx context.Context, actorUserID string, req models.PlatformEnrollmentDecisionRequest) (*models.PlatformEnrollmentDecisionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if actorUserID == "" {
		return nil, constants.ErrPlatformEnrollmentInvalidDecision
	}

	// Invariant 8: only the active first user may approve or deny.
	// The controller's requireActiveFirstUser enforces this at the
	// transport layer; the service enforces it independently so a
	// direct caller (e.g. a future internal admin path) cannot bypass
	// the active-owner check. A disabled first user fails closed with
	// the same typed authorization error as a non-owner.
	user, err := s.userSvc.GetByID(ctx, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: authorize decision: %w", err)
	}
	if user == nil || !user.IsActive() {
		return nil, constants.ErrPlatformEnrollmentInvalidDecision
	}
	isFirst, err := s.userSvc.IsFirstUser(ctx, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: authorize decision: %w", err)
	}
	if !isFirst {
		return nil, constants.ErrPlatformEnrollmentInvalidDecision
	}

	// Load the request to verify it exists and is pending before
	// submitting the envelope. The handler re-checks the state via
	// conditional update, but an early check gives a precise error
	// without consuming a governance receipt.
	existing, err := s.loadByID(ctx, req.RequestID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, constants.ErrPlatformEnrollmentRequestNotFound
	}
	if existing.State.IsTerminal() {
		return nil, s.terminalError(existing.State)
	}
	if existing.State != models.PlatformEnrollmentStatePending {
		return nil, constants.ErrPlatformEnrollmentAlreadyDecided
	}
	if time.Now().UTC().After(existing.ExpiresAt) {
		s.expireRequest(ctx, existing)
		return nil, constants.ErrPlatformEnrollmentRequestExpired
	}

	intent := constants.PlatformEnrollmentIntentApprove
	if req.Decision == models.PlatformEnrollmentDecisionDeny {
		intent = constants.PlatformEnrollmentIntentDeny
	}

	if _, err := s.submitEnvelope(ctx, constants.PlatformEnrollmentActionDecide, &commonv1.PlatformEnrollmentGovernancePayload{
		Action:        string(constants.PlatformEnrollmentActionDecide),
		Intent:        string(intent),
		RequestId:     req.RequestID,
		ComponentKind: payloadComponentKind(existing.ComponentKind),
		ActorUserId:   actorUserID,
		Decision:      payloadDecision(req.Decision),
		Reason:        req.Reason,
		Fingerprints:  payloadFingerprints(existing.Fingerprints),
	}); err != nil {
		return nil, fmt.Errorf("platform enrollment: decide envelope: %w", err)
	}

	// Reload to get the post-decision state.
	updated, err := s.loadByID(ctx, req.RequestID)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, constants.ErrPlatformEnrollmentRequestNotFound
	}

	s.logger.Info("platform enrollment decision recorded",
		"request_id", req.RequestID,
		"decision", string(req.Decision),
		"actor_user_id", actorUserID)
	s.approvals.EnrollmentsDecided(ctx)

	return &models.PlatformEnrollmentDecisionResponse{
		RequestID: req.RequestID,
		State:     updated.State,
	}, nil
}

// DecideBatch submits one governed decision over the owner's fixed pending
// snapshot. The executing document owner rechecks all members atomically.
func (s *PlatformEnrollmentService) DecideBatch(ctx context.Context, actorUserID string, req models.PlatformEnrollmentBatchDecisionRequest) (*models.PlatformEnrollmentBatchDecisionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	user, err := s.userSvc.GetByID(ctx, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: authorize batch: %w", err)
	}
	if user == nil || !user.IsActive() {
		return nil, constants.ErrPlatformEnrollmentInvalidDecision
	}
	first, err := s.userSvc.IsFirstUser(ctx, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: authorize batch owner: %w", err)
	}
	if !first {
		return nil, constants.ErrPlatformEnrollmentInvalidDecision
	}
	intent := constants.PlatformEnrollmentIntentApprove
	state := models.PlatformEnrollmentStateApproved
	if req.Decision == models.PlatformEnrollmentDecisionDeny {
		intent = constants.PlatformEnrollmentIntentDeny
		state = models.PlatformEnrollmentStateDenied
	}
	payload := &commonv1.PlatformEnrollmentGovernancePayload{
		Action: string(constants.PlatformEnrollmentActionDecide), Intent: string(intent),
		ActorUserId: actorUserID, Decision: payloadDecision(req.Decision), Reason: req.Reason,
	}
	for _, target := range req.Requests {
		payload.DecisionTargets = append(payload.DecisionTargets, &commonv1.PlatformEnrollmentDecisionTarget{
			RequestId: target.RequestID, Fingerprints: payloadFingerprints(target.Fingerprints),
		})
	}
	env, err := s.submitEnvelope(ctx, constants.PlatformEnrollmentActionDecide, payload)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: batch decision envelope: %w", err)
	}
	resp := &models.PlatformEnrollmentBatchDecisionResponse{ReceiptID: env.GetId(), Requests: make([]models.PlatformEnrollmentDecisionResponse, len(req.Requests))}
	for i, target := range req.Requests {
		resp.Requests[i] = models.PlatformEnrollmentDecisionResponse{RequestID: target.RequestID, State: state}
	}
	s.approvals.EnrollmentsDecided(ctx)
	return resp, nil
}

func (s *PlatformEnrollmentService) Revoke(ctx context.Context, actorUserID string, req models.PlatformEnrollmentRevokeRequest) (*models.PlatformEnrollmentRevokeResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	user, err := s.userSvc.GetByID(ctx, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: authorize revocation: %w", err)
	}
	if user == nil || !user.IsActive() {
		return nil, constants.ErrPlatformEnrollmentInvalidDecision
	}
	isFirst, err := s.userSvc.IsFirstUser(ctx, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: authorize revocation: %w", err)
	}
	if !isFirst {
		return nil, constants.ErrPlatformEnrollmentInvalidDecision
	}
	existing, err := s.loadByID(ctx, req.RequestID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, constants.ErrPlatformEnrollmentRequestNotFound
	}
	if existing.State == models.PlatformEnrollmentStateRevoked {
		return &models.PlatformEnrollmentRevokeResponse{RequestID: existing.ID, ComponentKind: existing.ComponentKind, State: existing.State}, nil
	}
	switch existing.State {
	case models.PlatformEnrollmentStatePending, models.PlatformEnrollmentStateApproved, models.PlatformEnrollmentStateIssuing:
		return nil, constants.ErrPlatformEnrollmentNotApproved
	case models.PlatformEnrollmentStateDenied:
		return nil, constants.ErrPlatformEnrollmentRequestDenied
	case models.PlatformEnrollmentStateExpired:
		return nil, constants.ErrPlatformEnrollmentRequestExpired
	case models.PlatformEnrollmentStateCompleted:
	default:
		return nil, constants.ErrPlatformEnrollmentInvalidState
	}
	targetDocumentID := ""
	if existing.ComponentKind == models.PlatformComponentDashboard || existing.ComponentKind == models.PlatformComponentEnsemble || existing.ComponentKind == models.PlatformComponentApplication {
		targetDocumentID = protocol.NewWorkloadIdentity().AppSPIFFEID(existing.ComponentName)
	}
	if _, err := s.submitEnvelope(ctx, constants.PlatformEnrollmentActionRevoke, &commonv1.PlatformEnrollmentGovernancePayload{
		Action:           string(constants.PlatformEnrollmentActionRevoke),
		Intent:           string(constants.PlatformEnrollmentIntentRevoke),
		RequestId:        existing.ID,
		ComponentKind:    payloadComponentKind(existing.ComponentKind),
		ActorUserId:      actorUserID,
		TargetDocumentId: targetDocumentID,
		Reason:           strings.TrimSpace(req.Reason),
	}); err != nil {
		return nil, fmt.Errorf("platform enrollment: revoke envelope: %w", err)
	}
	updated, err := s.loadByID(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	if updated == nil || updated.State != models.PlatformEnrollmentStateRevoked {
		return nil, constants.ErrPlatformEnrollmentInvalidState
	}
	return &models.PlatformEnrollmentRevokeResponse{RequestID: updated.ID, ComponentKind: updated.ComponentKind, State: updated.State}, nil
}

// ListEnrolled returns owner-visible metadata for completed and revoked
// platform enrollment requests. The response never includes token hashes,
// CSR PEM, certificates, or raw tokens. The caller must be authenticated as
// the active first user (enforced by the controller before calling this
// method).
func (s *PlatformEnrollmentService) ListEnrolled(ctx context.Context) (*models.PlatformEnrollmentEnrolledResponse, error) {
	docs, err := s.db.DocQuery(ctx, platformEnrollmentCollectionName(), nil, "created_at", 0)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: list enrolled: %w", err)
	}

	resp := &models.PlatformEnrollmentEnrolledResponse{Enrollments: []models.PlatformEnrollmentEnrolledRequest{}}
	for _, doc := range docs {
		req, err := decodePlatformEnrollmentRequest(doc)
		if err != nil {
			s.logger.Warn("platform enrollment: list enrolled: decode failed", "doc_id", doc.ID, "error", err)
			continue
		}
		switch req.State {
		case models.PlatformEnrollmentStateCompleted, models.PlatformEnrollmentStateRevoked:
			resp.Enrollments = append(resp.Enrollments, req.EnrolledMetadata())
		}
	}
	return resp, nil
}

// ListPending returns owner-visible metadata for all pending, non-expired
// platform enrollment requests. The response never includes token hashes,
// CSR PEM, certificates, or raw tokens. The caller must be authenticated as
// the active first user (enforced by the controller before calling this
// method).
func (s *PlatformEnrollmentService) ListPending(ctx context.Context) (*models.PlatformEnrollmentPendingResponse, error) {
	rows, err := s.db.db.QueryContext(ctx, `SELECT json_set(data, '$.id', id, '$.created_at', created_at)
		FROM documents WHERE collection = ? AND json_extract(data, '$.state') = ?
		AND julianday(json_extract(data, '$.expires_at')) >= julianday(?) ORDER BY created_at, id`,
		platformEnrollmentCollectionName(), models.PlatformEnrollmentStatePending, timesvc.NowTimestamp())
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: list pending: %w", err)
	}
	defer rows.Close()
	resp := &models.PlatformEnrollmentPendingResponse{Requests: []models.PlatformEnrollmentPendingRequest{}}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("platform enrollment: read pending: %w", err)
		}
		var req models.PlatformEnrollmentRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, fmt.Errorf("platform enrollment: decode pending: %w", err)
		}
		resp.Requests = append(resp.Requests, req.PendingMetadata())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("platform enrollment: iterate pending: %w", err)
	}
	return resp, nil
}

// Complete verifies the token, state, expiry, and proof-of-possession
// for every submitted key, then issues or resumes issuance. The
// issuance lease is the recoverable saga boundary:
//
//  1. If the request is completed, verify proofs and return the stored
//     response (idempotent). Downstream side effects (policy/session
//     creation) are re-submitted if they have not been applied, since
//     they are idempotent.
//  2. If the request is approved, acquire the issuance lease
//     (approved -> issuing with lease owner and lease expiry), submit
//     the ISSUE envelope, then submit the downstream PERSIST_POLICY
//     or CREATE_SESSION envelope.
//  3. If the request is issuing with a live lease, return a typed
//     retryable response. Reconciliation recovers an expired lease.
func (s *PlatformEnrollmentService) Complete(ctx context.Context, token string, proofs models.PlatformEnrollmentProofs) (*models.PlatformEnrollmentCompleteResponse, error) {
	if token == "" {
		return nil, constants.ErrPlatformEnrollmentTokenRequired
	}
	req, err := s.loadByToken(ctx, token)
	if err != nil {
		return nil, err
	}

	// Verify token freshness: the token hash matched, so the caller
	// possesses the correct token. Now check state and expiry.
	if time.Now().UTC().After(req.ExpiresAt) && !req.State.IsTerminal() {
		s.expireRequest(ctx, req)
		return nil, constants.ErrPlatformEnrollmentRequestExpired
	}

	switch req.State {
	case models.PlatformEnrollmentStateCompleted:
		// Idempotent retry: verify proofs and return the stored
		// response. Re-submit downstream side effects in case a crash
		// prevented them from running after ISSUE.
		if err := verifyPlatformEnrollmentProofs(req, proofs); err != nil {
			return nil, err
		}
		if err := s.submitDownstreamEnvelopes(ctx, req); err != nil {
			return nil, err
		}
		return req.Issued, nil

	case models.PlatformEnrollmentStateApproved:
		// First completion: verify proofs, acquire lease, issue.
		if err := verifyPlatformEnrollmentProofs(req, proofs); err != nil {
			return nil, err
		}
		// Once the lease is held, a disconnecting client must not abort a half-done
		// issuance. Its retry reads the stored result; the lease TTL bounds the work.
		sagaCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), constants.PlatformEnrollmentIssuanceLeaseTTL)
		defer cancel()
		return s.issueComponent(sagaCtx, req)

	case models.PlatformEnrollmentStateIssuing:
		// A live or expired lease. If expired, reconciliation will
		// recover it; the client should retry after a short delay.
		return nil, constants.ErrPlatformEnrollmentIssuanceInProgress

	case models.PlatformEnrollmentStatePending:
		return nil, constants.ErrPlatformEnrollmentNotApproved

	case models.PlatformEnrollmentStateDenied:
		return nil, constants.ErrPlatformEnrollmentRequestDenied

	case models.PlatformEnrollmentStateExpired:
		return nil, constants.ErrPlatformEnrollmentRequestExpired

	case models.PlatformEnrollmentStateRevoked:
		return nil, constants.ErrPlatformEnrollmentRevoked

	default:
		return nil, constants.ErrPlatformEnrollmentInvalidState
	}
}

// issueComponent acquires the issuance lease (approved -> issuing),
// submits the ISSUE envelope, reloads the request to get the issued
// material and generated IDs, submits the downstream PERSIST_POLICY
// or CREATE_SESSION envelope, and returns the issued response.
func (s *PlatformEnrollmentService) issueComponent(ctx context.Context, req *models.PlatformEnrollmentRequest) (*models.PlatformEnrollmentCompleteResponse, error) {
	leaseOwner, err := uuid.NewString()
	if err != nil {
		return nil, err
	}
	leaseExpiry := time.Now().UTC().Add(constants.PlatformEnrollmentIssuanceLeaseTTL)

	// Acquire the issuance lease: approved -> issuing with lease owner
	// and lease expiry. A concurrent completion attempt loses the
	// conditional update and fails closed.
	leaseUpdate, err := json.Marshal(struct {
		State                string    `json:"state"`
		IssuanceLeaseOwner   string    `json:"issuance_lease_owner"`
		IssuanceLeaseExpires time.Time `json:"issuance_lease_expires_at"`
		LastTransitionAt     time.Time `json:"last_transition_at"`
	}{
		State:                string(models.PlatformEnrollmentStateIssuing),
		IssuanceLeaseOwner:   leaseOwner,
		IssuanceLeaseExpires: leaseExpiry,
		LastTransitionAt:     time.Now().UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: marshal issuance lease: %w", err)
	}
	applied, err := s.db.DocConditionalUpdate(
		ctx, platformEnrollmentCollectionName(), req.ID,
		leaseUpdate,
		"state", string(models.PlatformEnrollmentStateApproved),
	)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: acquire lease: %w", err)
	}
	if !applied {
		return nil, constants.ErrPlatformEnrollmentIssuanceInProgress
	}

	// Submit the ISSUE envelope. The handler verifies the issuing
	// state, signs the certificate(s), stores the issued material, and
	// transitions issuing -> completed.
	if _, err := s.submitEnvelope(ctx, constants.PlatformEnrollmentActionIssue, &commonv1.PlatformEnrollmentGovernancePayload{
		Action:        string(constants.PlatformEnrollmentActionIssue),
		Intent:        string(constants.PlatformEnrollmentIntentIssue),
		RequestId:     req.ID,
		ComponentKind: payloadComponentKind(req.ComponentKind),
		ActorUserId:   req.ApprovedByUserID,
	}); err != nil {
		// Roll back the lease so a retry can re-acquire. A failed
		// ISSUE must not permanently consume approval.
		s.rollbackLease(ctx, req.ID)
		return nil, fmt.Errorf("platform enrollment: issue envelope: %w", err)
	}

	// Reload to get the issued material and generated IDs written by
	// the ISSUE handler.
	completed, err := s.loadByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if completed == nil {
		return nil, constants.ErrPlatformEnrollmentRequestNotFound
	}
	if completed.State != models.PlatformEnrollmentStateCompleted {
		// The handler rolled back to approved on a signing failure.
		// Return the issuance error so the client can retry.
		return nil, constants.ErrPlatformEnrollmentIssuanceFailed
	}

	// Submit downstream side effects (policy persistence for apps,
	// session creation for operator). These are idempotent.
	if err := s.submitDownstreamEnvelopes(ctx, completed); err != nil {
		return nil, err
	}

	s.logger.Info("platform enrollment completed",
		"request_id", req.ID,
		"component_kind", string(req.ComponentKind),
		"certificate_fingerprint", completed.CertificateFingerprint)

	return completed.Issued, nil
}

// submitDownstreamEnvelopes submits the PERSIST_POLICY envelope for
// dashboard/ensemble or the CREATE_SESSION envelope for operator. These
// handlers are idempotent: PERSIST_POLICY uses DocSet (overwrite), and
// CREATE_SESSION uses PersistCLISession/PersistOperatorSession
// (idempotent by session ID). Calling this on a completed request that
// already has downstream side effects is safe.
func (s *PlatformEnrollmentService) submitDownstreamEnvelopes(ctx context.Context, req *models.PlatformEnrollmentRequest) error {
	switch req.ComponentKind {
	case models.PlatformComponentDashboard, models.PlatformComponentEnsemble, models.PlatformComponentApplication:
		policyID := req.PolicyID
		if policyID == "" {
			id, err := uuid.NewString()
			if err != nil {
				return err
			}
			policyID = id
		}
		if _, err := s.submitEnvelope(ctx, constants.PlatformEnrollmentActionPersistPolicy, &commonv1.PlatformEnrollmentGovernancePayload{
			Action:                 string(constants.PlatformEnrollmentActionPersistPolicy),
			Intent:                 string(constants.PlatformEnrollmentIntentIssue),
			RequestId:              req.ID,
			ComponentKind:          payloadComponentKind(req.ComponentKind),
			ActorUserId:            req.ApprovedByUserID,
			TargetCollection:       marshaler.CollectionName(constants.CollectionAppPolicies),
			TargetDocumentId:       protocol.NewWorkloadIdentity().AppSPIFFEID(req.ComponentName),
			PolicyId:               policyID,
			CertificateSerial:      req.CertificateSerial,
			CertificateFingerprint: req.CertificateFingerprint,
			OwnerUserId:            req.ApprovedByUserID,
		}); err != nil {
			return fmt.Errorf("platform enrollment: persist policy envelope: %w", err)
		}

	case models.PlatformComponentOperator:
		if req.OperatorID == "" || req.OperatorSessionID == "" || req.CLISessionID == "" {
			return constants.ErrPlatformEnrollmentInvalidPayload
		}
		if _, err := s.submitEnvelope(ctx, constants.PlatformEnrollmentActionCreateSession, &commonv1.PlatformEnrollmentGovernancePayload{
			Action:                 string(constants.PlatformEnrollmentActionCreateSession),
			Intent:                 string(constants.PlatformEnrollmentIntentIssue),
			RequestId:              req.ID,
			ComponentKind:          payloadComponentKind(req.ComponentKind),
			ActorUserId:            req.ApprovedByUserID,
			OperatorId:             req.OperatorID,
			OperatorSessionId:      req.OperatorSessionID,
			CliSessionId:           req.CLISessionID,
			CertificateFingerprint: req.CertificateFingerprint,
			CertificateSerial:      req.CertificateSerial,
		}); err != nil {
			return fmt.Errorf("platform enrollment: create session envelope: %w", err)
		}
	}
	return nil
}

// ReconcileExpiredLeases finds requests in the issuing state with an
// expired issuance lease and transitions them back to the approved
// state so a new completion attempt can re-acquire the lease. This
// recovers from a crash between lease acquisition and ISSUE completion.
// Requests with a live lease are left in the issuing state.
func (s *PlatformEnrollmentService) ReconcileExpiredLeases(ctx context.Context) error {
	now := timesvc.NowTimestamp()
	result, err := s.db.db.ExecWithRetry(ctx, `UPDATE documents SET data = json_patch(data, json_object(
  'state', CASE WHEN julianday(json_extract(data, '$.expires_at')) < julianday(?) THEN ? ELSE ? END,
  'issuance_lease_owner', '', 'issuance_lease_expires_at', NULL, 'last_transition_at', ?)), updated_at = ?
  WHERE collection = ? AND json_extract(data, '$.state') = ?
  AND julianday(json_extract(data, '$.issuance_lease_expires_at')) <= julianday(?)`,
		now, models.PlatformEnrollmentStateExpired, models.PlatformEnrollmentStateApproved, now, now,
		platformEnrollmentCollectionName(), models.PlatformEnrollmentStateIssuing, now)
	if err != nil {
		return fmt.Errorf("platform enrollment: recover expired leases: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("platform enrollment: count recovered leases: %w", err)
	}
	if count > 0 {
		s.approvals.EnrollmentsChanged(ctx)
	}
	return nil
}

// CleanupTerminalRequests explicitly expires abandoned live requests, then
// removes denied/expired records past retention. Completed records retain the
// issued credentials needed for idempotent completion. Reads never do this work.
func (s *PlatformEnrollmentService) CleanupTerminalRequests(ctx context.Context) error {
	now := time.Now().UTC()
	stamp := timesvc.FormatTimestamp(now)
	result, err := s.db.db.ExecWithRetry(ctx, `UPDATE documents SET data = json_patch(data, json_object('state', ?, 'last_transition_at', ?)), updated_at = ?
  WHERE collection = ? AND json_extract(data, '$.state') IN ('pending', 'approved', 'issuing')
  AND julianday(json_extract(data, '$.expires_at')) < julianday(?)`,
		models.PlatformEnrollmentStateExpired, stamp, stamp, platformEnrollmentCollectionName(), stamp)
	if err != nil {
		return fmt.Errorf("platform enrollment: expire abandoned requests: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("platform enrollment: count expired requests: %w", err)
	}
	if count > 0 {
		s.approvals.EnrollmentsChanged(ctx)
	}
	_, err = s.db.db.ExecWithRetry(ctx, `DELETE FROM documents WHERE collection = ? AND json_extract(data, '$.state') IN ('denied', 'expired')
  AND julianday(json_extract(data, '$.last_transition_at')) < julianday(?)`,
		platformEnrollmentCollectionName(), timesvc.FormatTimestamp(now.Add(-constants.PlatformEnrollmentCleanupRetention)))
	if err != nil {
		return fmt.Errorf("platform enrollment: delete retained terminal requests: %w", err)
	}
	return nil
}

// platformEnrollmentStateRootMaxRetries bounds in-process resubmission when
// concurrent enrollment CREATE activity advances the bound root between fetch
// and verification. Each retry rebuilds the envelope with a fresh root, nonce,
// and transaction hash.
const platformEnrollmentStateRootMaxRetries = 3

func platformEnrollmentRequestEvent(action constants.PlatformEnrollmentGovernanceAction) constants.EventType {
	switch action {
	case constants.PlatformEnrollmentActionCreate:
		return constants.EventPlatformEnrollmentCreateRequested
	case constants.PlatformEnrollmentActionDecide:
		return constants.EventPlatformEnrollmentDecideRequested
	case constants.PlatformEnrollmentActionIssue:
		return constants.EventPlatformEnrollmentIssueRequested
	case constants.PlatformEnrollmentActionPersistPolicy:
		return constants.EventPlatformEnrollmentPersistPolicyRequested
	case constants.PlatformEnrollmentActionCreateSession:
		return constants.EventPlatformEnrollmentCreateSessionRequested
	case constants.PlatformEnrollmentActionRevoke:
		return constants.EventPlatformEnrollmentRevokeRequested
	default:
		return ""
	}
}

// submitEnvelope builds a GovernanceEnvelope with the given action type
// and PlatformEnrollmentGovernancePayload, marshals it as protojson,
// and calls the injected EnvelopeProcessor. The envelope carries the
// gateway's current state root, a unique nonce, and a near-future
// expiry. The payload is binary proto (the L4Warden decodes the same
// way).
func (s *PlatformEnrollmentService) submitEnvelope(ctx context.Context, action constants.PlatformEnrollmentGovernanceAction, payload *commonv1.PlatformEnrollmentGovernancePayload) (*commonv1.GovernanceEnvelope, error) {
	payloadBytes, err := marshalPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= platformEnrollmentStateRootMaxRetries; attempt++ {
		stateRoot, err := s.stateRoot.GetCurrentStateRoot(ctx)
		if err != nil {
			return nil, fmt.Errorf("get state root: %w", err)
		}

		nonce, err := generateNonce()
		if err != nil {
			return nil, fmt.Errorf("generate nonce: %w", err)
		}

		env := &commonv1.GovernanceEnvelope{
			ProtocolVersion: govpkg.GovernanceProtocolVersionV2,
			Timestamp:       timestamppb.Now(),
			ExpiresAt:       timestamppb.New(time.Now().Add(5 * time.Minute)),
			SourceComponent: commonv1.Component_COMPONENT_G8EO,
			OperatorId:      string(constants.DocIDEmbeddedOperator),
			ActionType:      string(action),
			EventType:       string(platformEnrollmentRequestEvent(action)),
			Payload:         payloadBytes,
			StateMerkleRoot: stateRoot,
			Nonce:           nonce,
			Posture:         s.posture,
			Governance: &commonv1.GovernanceMetadata{
				L1: &commonv1.L1Metadata{Validated: true},
			},
		}

		txHash, err := govpkg.GenerateMessageID(env)
		if err != nil {
			return nil, fmt.Errorf("generate message ID: %w", err)
		}
		env.Id = txHash
		env.TransactionHash = txHash

		wire, err := protojson.Marshal(env)
		if err != nil {
			return nil, fmt.Errorf("marshal envelope: %w", err)
		}

		submitCtx := ctx
		if stateRoot != "" {
			submitCtx = context.WithValue(ctx, constants.ContextKeyStateMerkleRoot, stateRoot)
		}

		receipt, err := s.envProc.ProcessEnvelope(submitCtx, wire)
		if err == nil {
			if receipt == nil {
				return nil, constants.ErrPlatformEnrollmentGovernanceRejected
			}
			return env, nil
		}
		lastErr = err
		if errors.Is(err, constants.ErrTxStateRootMismatch) && attempt < platformEnrollmentStateRootMaxRetries {
			continue
		}
		return nil, fmt.Errorf("%w: %w", constants.ErrPlatformEnrollmentGovernanceRejected, err)
	}
	return nil, fmt.Errorf("%w: %w", constants.ErrPlatformEnrollmentGovernanceRejected, lastErr)
}

// rollbackLease transitions a request from issuing back to approved so
// a retry can re-acquire the lease. Failures are logged; the caller
// has already decided to return an error.
func (s *PlatformEnrollmentService) rollbackLease(ctx context.Context, requestID string) {
	update, err := marshalPlatformEnrollmentApprovedUpdate(time.Now().UTC())
	if err != nil {
		s.logger.Error("platform enrollment: marshal rollback lease failed", "request_id", requestID, "error", err)
		return
	}
	applied, err := s.db.DocConditionalUpdate(
		ctx, platformEnrollmentCollectionName(), requestID, update,
		"state", string(models.PlatformEnrollmentStateIssuing),
	)
	if err != nil {
		s.logger.Error("platform enrollment: rollback lease failed", "request_id", requestID, "error", err)
		return
	}
	if !applied {
		s.logger.Warn("platform enrollment: rollback lease not applied", "request_id", requestID)
	}
}

// expireRequest attempts to atomically transition a non-terminal request
// to the expired state. Failures are logged; the caller has already
// decided to treat the request as expired.
func (s *PlatformEnrollmentService) expireRequest(ctx context.Context, req *models.PlatformEnrollmentRequest) {
	update, err := marshalPlatformEnrollmentExpiredUpdate(time.Now().UTC())
	if err != nil {
		s.logger.Warn("platform enrollment: marshal expire failed", "request_id", req.ID, "error", err)
		return
	}
	applied, err := s.db.DocConditionalUpdate(
		ctx, platformEnrollmentCollectionName(), req.ID, update,
		"state", string(req.State),
	)
	if err != nil {
		s.logger.Warn("platform enrollment: expire failed", "request_id", req.ID, "error", err)
		return
	}
	if applied {
		s.logger.Info("platform enrollment request expired", "request_id", req.ID)
		s.approvals.EnrollmentsChanged(ctx)
	}
}

// loadByToken resolves the opaque token through the indexed token hash.
func (s *PlatformEnrollmentService) loadByToken(ctx context.Context, token string) (*models.PlatformEnrollmentRequest, error) {
	var data []byte
	err := s.db.db.QueryRowWithRetry(ctx, `SELECT json_set(data, '$.id', id, '$.created_at', created_at)
		FROM documents WHERE collection = ? AND json_extract(data, '$.token_hash') = ? LIMIT 1`,
		platformEnrollmentCollectionName(), models.PlatformEnrollmentTokenHash(token)).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, constants.ErrPlatformEnrollmentRequestNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: load by token: %w", err)
	}
	var req models.PlatformEnrollmentRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("platform enrollment: decode request: %w", err)
	}
	return &req, nil
}

// loadByID loads a request by its request ID (the document ID).
func (s *PlatformEnrollmentService) loadByID(ctx context.Context, requestID string) (*models.PlatformEnrollmentRequest, error) {
	doc, err := s.db.DocGet(ctx, platformEnrollmentCollectionName(), requestID)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: load by id: %w", err)
	}
	if doc == nil {
		return nil, nil
	}
	return decodePlatformEnrollmentRequest(doc)
}

func (s *PlatformEnrollmentService) statusResponse(req *models.PlatformEnrollmentRequest) *models.PlatformEnrollmentStatusResponse {
	resp := &models.PlatformEnrollmentStatusResponse{
		RequestID:     req.ID,
		ComponentKind: req.ComponentKind,
		State:         req.State,
		ExpiresAt:     req.ExpiresAt,
	}
	if req.State == models.PlatformEnrollmentStateIssuing {
		resp.RetryAfter = constants.PlatformEnrollmentRetryAfterSeconds
	}
	if req.FailureReason != "" {
		resp.FailureReason = req.FailureReason
	}
	return resp
}

func (s *PlatformEnrollmentService) terminalError(state models.PlatformEnrollmentState) error {
	switch state {
	case models.PlatformEnrollmentStateCompleted:
		return constants.ErrPlatformEnrollmentAlreadyDecided
	case models.PlatformEnrollmentStateDenied:
		return constants.ErrPlatformEnrollmentRequestDenied
	case models.PlatformEnrollmentStateExpired:
		return constants.ErrPlatformEnrollmentRequestExpired
	case models.PlatformEnrollmentStateRevoked:
		return constants.ErrPlatformEnrollmentRevoked
	default:
		return constants.ErrPlatformEnrollmentInvalidState
	}
}

// platformEnrollmentCollectionName resolves the canonical collection
// name for persisted platform enrollment requests.
func platformEnrollmentCollectionName() string {
	return marshaler.CollectionName(constants.CollectionPlatformEnrollments)
}

// decodePlatformEnrollmentRequest deserializes a Document into a
// PlatformEnrollmentRequest.
func decodePlatformEnrollmentRequest(doc *models.Document) (*models.PlatformEnrollmentRequest, error) {
	dataBytes, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal document data: %w", err)
	}
	var req models.PlatformEnrollmentRequest
	if err := json.Unmarshal(dataBytes, &req); err != nil {
		return nil, fmt.Errorf("unmarshal platform enrollment request: %w", err)
	}
	req.CreatedAt = doc.CreatedAt
	return &req, nil
}

// fingerprintsMatch returns true if the two fingerprint sets are equal
// in constant time for each field.
func fingerprintsMatch(a, b models.PlatformEnrollmentCSRFingerprints) bool {
	return constantTimeEqual(a.App, b.App) &&
		constantTimeEqual(a.Operator, b.Operator) &&
		constantTimeEqual(a.CLI, b.CLI)
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}

// buildApprovalURL constructs the approval URL from the base URL and
// request ID. The URL fragment carries the request ID; the token is
// never in the URL.
func buildApprovalURL(base, requestID string) string {
	if base == "" {
		return ""
	}
	return base + "#platform-enrollment=" + requestID
}

// marshalPayload serializes a PlatformEnrollmentGovernancePayload as
// binary protobuf (the wire format inside GovernanceEnvelope.Payload).
func marshalPayload(payload *commonv1.PlatformEnrollmentGovernancePayload) ([]byte, error) {
	return proto.Marshal(payload)
}

// generateNonce returns 16 random bytes hex-encoded.
func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// payloadComponentKind maps the typed domain kind to the proto enum.
func payloadComponentKind(kind models.PlatformComponentKind) commonv1.PlatformComponentKind {
	switch kind {
	case models.PlatformComponentDashboard:
		return commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD
	case models.PlatformComponentEnsemble:
		return commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_ENSEMBLE
	case models.PlatformComponentOperator:
		return commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR
	case models.PlatformComponentApplication:
		return commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_APPLICATION
	default:
		return commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_UNSPECIFIED
	}
}

// payloadDecision maps the typed domain decision to the proto enum.
func payloadDecision(decision models.PlatformEnrollmentDecision) commonv1.PlatformEnrollmentDecision {
	switch decision {
	case models.PlatformEnrollmentDecisionApprove:
		return commonv1.PlatformEnrollmentDecision_PLATFORM_ENROLLMENT_DECISION_APPROVE
	case models.PlatformEnrollmentDecisionDeny:
		return commonv1.PlatformEnrollmentDecision_PLATFORM_ENROLLMENT_DECISION_DENY
	default:
		return commonv1.PlatformEnrollmentDecision_PLATFORM_ENROLLMENT_DECISION_UNSPECIFIED
	}
}

// payloadFingerprints maps the typed domain fingerprints to the proto
// message.
func payloadFingerprints(fp models.PlatformEnrollmentCSRFingerprints) *commonv1.PlatformEnrollmentFingerprints {
	return &commonv1.PlatformEnrollmentFingerprints{
		App:      fp.App,
		Operator: fp.Operator,
		Cli:      fp.CLI,
	}
}
