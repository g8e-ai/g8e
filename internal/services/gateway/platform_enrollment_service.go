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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/constants"
	govpkg "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/timesvc"
	"github.com/g8e-ai/g8e/internal/uuid"
	"github.com/g8e-ai/g8e/protocol"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// PlatformEnrollmentService owns the platform workload enrollment
// lifecycle: request creation/deduplication/quotas, owner decisions,
// issuance leases, idempotent component issuance, reconciliation of
// expired leases, and managed cleanup.
//
// All mutations route through the canonical governance gauntlet by
// submitting GovernanceEnvelope messages to the injected
// governance.EnvelopeProcessor (the gateway's in-process
// OperatorPubSubService via GatewayEnvProcAdapter). The single exception
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
	logger    *slog.Logger

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.Mutex
}

// NewPlatformEnrollmentService creates a new PlatformEnrollmentService.
// The envProc and stateRoot are the gateway's in-process governance
// pipeline: envProc is the GatewayEnvProcAdapter, stateRoot is the
// StateRootService. StartCleanup must be called to register the managed
// cleanup goroutine with the gateway lifecycle context.
func NewPlatformEnrollmentService(
	db *DocumentStoreService,
	userSvc *UserService,
	envProc governance.EnvelopeProcessor,
	stateRoot governance.StateRootProvider,
	logger *slog.Logger,
) *PlatformEnrollmentService {
	return &PlatformEnrollmentService{
		db:        db,
		userSvc:   userSvc,
		envProc:   envProc,
		stateRoot: stateRoot,
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
			if err := s.ReconcileExpiredLeases(); err != nil {
				s.logger.Warn("platform enrollment: reconcile expired leases failed", "error", err)
			}
			if err := s.CleanupTerminalRequests(); err != nil {
				s.logger.Warn("platform enrollment: cleanup terminal requests failed", "error", err)
			}
		}
	}
}

// CreateRequest validates activation and CSRs, deduplicates a live
// request for the same component kind, instance ID, and key fingerprint
// set, writes the pending request document directly via DocSet (CREATE
// is classified as non-mutation), submits a PLATFORM_ENROLLMENT_CREATE
// envelope for audit, and returns the request ID, requester token,
// component name, fingerprints, approval URL, and expiry. The raw token
// is returned once and never persisted; only its SHA-256 hash is stored.
func (s *PlatformEnrollmentService) CreateRequest(ctx context.Context, req models.PlatformEnrollmentCreateRequest, approvalURLBase string) (*models.PlatformEnrollmentCreateResponse, error) {
	// Invariant 1: a gateway with no users never issues a platform
	// certificate. Request creation requires an activated gateway.
	hasUsers, err := s.userSvc.HasAnyUsers()
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: check activation: %w", err)
	}
	if !hasUsers {
		return nil, constants.ErrPlatformEnrollmentRequiresActivation
	}

	fingerprints, err := validatePlatformEnrollmentRequest(req)
	if err != nil {
		return nil, err
	}

	componentName, err := req.ComponentKind.CanonicalName()
	if err != nil {
		return nil, err
	}

	// Deduplicate: if a live (non-terminal, non-expired) request exists
	// for the same component kind, instance ID, and fingerprint set,
	// return it instead of creating a new one. The requester resumes
	// the existing request with the original token. Since the raw token
	// is not stored, a deduplicated response cannot return the token;
	// the requester must have persisted it from the original creation.
	// If no matching live request exists, proceed with creation.
	existing, err := s.findLiveRequest(req.ComponentKind, req.InstanceID, fingerprints)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: dedup query: %w", err)
	}
	if existing != nil {
		// A live request exists. The requester must resume with the
		// original token. Return the public metadata so the requester
		// can display the approval URL and fingerprints. The token is
		// intentionally absent from the deduplicated response.
		return &models.PlatformEnrollmentCreateResponse{
			RequestID:     existing.ID,
			Token:         "",
			ComponentKind: existing.ComponentKind,
			ComponentName: existing.ComponentName,
			Fingerprints:  existing.Fingerprints,
			ApprovalURL:   buildApprovalURL(approvalURLBase, existing.ID),
			ExpiresAt:     existing.ExpiresAt,
		}, nil
	}

	// Quota: bound live requests per component kind to prevent
	// unbounded pending request creation.
	if err := s.checkQuota(req.ComponentKind); err != nil {
		return nil, err
	}

	token, err := newPlatformEnrollmentToken()
	if err != nil {
		return nil, err
	}
	tokenHash := platformEnrollmentTokenHash(token)
	requestID := uuid.NewString()
	now := time.Now().UTC()
	expiresAt := now.Add(constants.PlatformEnrollmentRequestTTL)

	persistedReq := &models.PlatformEnrollmentRequest{
		ID:                requestID,
		TokenHash:         tokenHash,
		ComponentKind:     req.ComponentKind,
		ComponentName:     componentName,
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

	// Write the pending request document directly. CREATE is classified
	// as non-mutation in IsMutation, so invariant 17 (no direct DocSet
	// for mutations) does not apply to this initial write. The CSR PEM
	// is public material; the token hash is a stored credential. Neither
	// appears in the audited CREATE envelope payload.
	data, err := json.Marshal(persistedReq)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: marshal request: %w", err)
	}
	if err := s.db.DocSet(platformEnrollmentCollectionName(), requestID, data); err != nil {
		return nil, fmt.Errorf("platform enrollment: persist request: %w", err)
	}

	// Submit the CREATE envelope for audit. The handler is audit-only:
	// it decodes the payload, returns a receipt summary, and writes
	// nothing to the doc store.
	if _, err := s.submitEnvelope(ctx, constants.PlatformEnrollmentActionCreate, constants.PlatformEnrollmentIntentRequest, &commonv1.PlatformEnrollmentGovernancePayload{
		Action:        string(constants.PlatformEnrollmentActionCreate),
		Intent:        string(constants.PlatformEnrollmentIntentRequest),
		RequestId:     requestID,
		ComponentKind: payloadComponentKind(req.ComponentKind),
		InstanceId:    req.InstanceID,
		Fingerprints:  payloadFingerprints(fingerprints),
	}); err != nil {
		return nil, fmt.Errorf("platform enrollment: create envelope: %w", err)
	}

	s.logger.Info("platform enrollment request created",
		"request_id", requestID,
		"component_kind", string(req.ComponentKind),
		"instance_id", req.InstanceID,
		"expires_at", expiresAt)

	return &models.PlatformEnrollmentCreateResponse{
		RequestID:     requestID,
		Token:         token,
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
	_ = ctx
	if token == "" {
		return nil, constants.ErrPlatformEnrollmentTokenRequired
	}
	req, err := s.loadByToken(token)
	if err != nil {
		return nil, err
	}
	if req.State.IsTerminal() {
		return s.statusResponse(req), nil
	}
	if time.Now().UTC().After(req.ExpiresAt) {
		s.expireRequest(req)
		return nil, constants.ErrPlatformEnrollmentRequestExpired
	}
	return s.statusResponse(req), nil
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
	user, err := s.userSvc.GetByID(actorUserID)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: authorize decision: %w", err)
	}
	if user == nil || !user.IsActive() {
		return nil, constants.ErrPlatformEnrollmentInvalidDecision
	}
	isFirst, err := s.userSvc.IsFirstUser(actorUserID)
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
	existing, err := s.loadByID(req.RequestID)
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
		s.expireRequest(existing)
		return nil, constants.ErrPlatformEnrollmentRequestExpired
	}

	intent := constants.PlatformEnrollmentIntentApprove
	if req.Decision == models.PlatformEnrollmentDecisionDeny {
		intent = constants.PlatformEnrollmentIntentDeny
	}

	if _, err := s.submitEnvelope(ctx, constants.PlatformEnrollmentActionDecide, intent, &commonv1.PlatformEnrollmentGovernancePayload{
		Action:        string(constants.PlatformEnrollmentActionDecide),
		Intent:        string(intent),
		RequestId:     req.RequestID,
		ComponentKind: payloadComponentKind(existing.ComponentKind),
		ActorUserId:   actorUserID,
		Decision:      payloadDecision(req.Decision),
	}); err != nil {
		return nil, fmt.Errorf("platform enrollment: decide envelope: %w", err)
	}

	// Reload to get the post-decision state.
	updated, err := s.loadByID(req.RequestID)
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

	return &models.PlatformEnrollmentDecisionResponse{
		RequestID: req.RequestID,
		State:     updated.State,
	}, nil
}

// ListPending returns owner-visible metadata for all non-terminal
// requests. The response never includes token hashes, CSR PEM,
// certificates, or raw tokens. The caller must be authenticated as the
// active first user (enforced by the controller before calling this
// method).
func (s *PlatformEnrollmentService) ListPending(ctx context.Context) (*models.PlatformEnrollmentPendingResponse, error) {
	_ = ctx
	docs, err := s.db.DocQuery(platformEnrollmentCollectionName(), nil, "created_at", 0)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: list pending: %w", err)
	}

	resp := &models.PlatformEnrollmentPendingResponse{Requests: []models.PlatformEnrollmentPendingRequest{}}
	now := time.Now().UTC()
	for _, doc := range docs {
		req, err := decodePlatformEnrollmentRequest(doc)
		if err != nil {
			s.logger.Warn("platform enrollment: list pending: decode failed", "doc_id", doc.ID, "error", err)
			continue
		}
		if req.State.IsTerminal() {
			continue
		}
		if now.After(req.ExpiresAt) {
			s.expireRequest(req)
			continue
		}
		resp.Requests = append(resp.Requests, req.PendingMetadata())
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
	req, err := s.loadByToken(token)
	if err != nil {
		return nil, err
	}

	// Verify token freshness: the token hash matched, so the caller
	// possesses the correct token. Now check state and expiry.
	if time.Now().UTC().After(req.ExpiresAt) && !req.State.IsTerminal() {
		s.expireRequest(req)
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
		return s.issueComponent(ctx, req)

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

	default:
		return nil, constants.ErrPlatformEnrollmentInvalidState
	}
}

// issueComponent acquires the issuance lease (approved -> issuing),
// submits the ISSUE envelope, reloads the request to get the issued
// material and generated IDs, submits the downstream PERSIST_POLICY
// or CREATE_SESSION envelope, and returns the issued response.
func (s *PlatformEnrollmentService) issueComponent(ctx context.Context, req *models.PlatformEnrollmentRequest) (*models.PlatformEnrollmentCompleteResponse, error) {
	leaseOwner := uuid.NewString()
	leaseExpiry := time.Now().UTC().Add(constants.PlatformEnrollmentIssuanceLeaseTTL)

	// Acquire the issuance lease: approved -> issuing with lease owner
	// and lease expiry. A concurrent completion attempt loses the
	// conditional update and fails closed.
	applied, err := s.db.DocConditionalUpdate(
		platformEnrollmentCollectionName(), req.ID,
		map[string]interface{}{
			"state":                     string(models.PlatformEnrollmentStateIssuing),
			"issuance_lease_owner":      leaseOwner,
			"issuance_lease_expires_at": leaseExpiry,
			"last_transition_at":        time.Now().UTC(),
		},
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
	if _, err := s.submitEnvelope(ctx, constants.PlatformEnrollmentActionIssue, constants.PlatformEnrollmentIntentIssue, &commonv1.PlatformEnrollmentGovernancePayload{
		Action:        string(constants.PlatformEnrollmentActionIssue),
		Intent:        string(constants.PlatformEnrollmentIntentIssue),
		RequestId:     req.ID,
		ComponentKind: payloadComponentKind(req.ComponentKind),
		ActorUserId:   req.ApprovedByUserID,
	}); err != nil {
		// Roll back the lease so a retry can re-acquire. A failed
		// ISSUE must not permanently consume approval.
		s.rollbackLease(req.ID)
		return nil, fmt.Errorf("platform enrollment: issue envelope: %w", err)
	}

	// Reload to get the issued material and generated IDs written by
	// the ISSUE handler.
	completed, err := s.loadByID(req.ID)
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
	case models.PlatformComponentDashboard, models.PlatformComponentEnsemble:
		policyID := req.PolicyID
		if policyID == "" {
			policyID = uuid.NewString()
		}
		if _, err := s.submitEnvelope(ctx, constants.PlatformEnrollmentActionPersistPolicy, constants.PlatformEnrollmentIntentIssue, &commonv1.PlatformEnrollmentGovernancePayload{
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
		if _, err := s.submitEnvelope(ctx, constants.PlatformEnrollmentActionCreateSession, constants.PlatformEnrollmentIntentIssue, &commonv1.PlatformEnrollmentGovernancePayload{
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
func (s *PlatformEnrollmentService) ReconcileExpiredLeases() error {
	docs, err := s.db.DocQuery(
		platformEnrollmentCollectionName(),
		[]models.DocFilter{{Field: "state", Op: "==", Value: json.RawMessage(`"issuing"`)}},
		"", 0,
	)
	if err != nil {
		return fmt.Errorf("platform enrollment: reconcile: query: %w", err)
	}

	now := time.Now().UTC()
	var recovered int
	for _, doc := range docs {
		req, err := decodePlatformEnrollmentRequest(doc)
		if err != nil {
			s.logger.Warn("platform enrollment: reconcile: decode failed", "doc_id", doc.ID, "error", err)
			continue
		}
		if req.IssuanceLeaseExpiresAt == nil || now.Before(*req.IssuanceLeaseExpiresAt) {
			continue
		}
		// Also check request-level expiry: if the request itself has
		// expired, transition to expired instead of approved.
		if now.After(req.ExpiresAt) {
			s.expireRequest(req)
			continue
		}
		applied, err := s.db.DocConditionalUpdate(
			platformEnrollmentCollectionName(), req.ID,
			map[string]interface{}{
				"state":                     string(models.PlatformEnrollmentStateApproved),
				"issuance_lease_owner":      "",
				"issuance_lease_expires_at": nil,
				"last_transition_at":        now,
			},
			"state", string(models.PlatformEnrollmentStateIssuing),
		)
		if err != nil {
			s.logger.Warn("platform enrollment: reconcile: rollback failed", "request_id", req.ID, "error", err)
			continue
		}
		if applied {
			recovered++
			s.logger.Info("platform enrollment: expired lease recovered", "request_id", req.ID)
		}
	}
	if recovered > 0 {
		s.logger.Info("platform enrollment: reconciled expired leases", "count", recovered)
	}
	return nil
}

// CleanupTerminalRequests removes terminal request records (denied or
// expired) that are past the retention window. Completed requests are
// never removed by cleanup because they hold the sole copy of issued
// artifacts needed for idempotent retry. Denied and expired requests
// carry no issued artifacts and are safe to remove after retention.
func (s *PlatformEnrollmentService) CleanupTerminalRequests() error {
	cutoff := time.Now().UTC().Add(-constants.PlatformEnrollmentCleanupRetention)
	cutoffStr := timesvc.FormatTimestamp(cutoff)

	filters := []models.DocFilter{
		{Field: "state", Op: "==", Value: json.RawMessage(`"denied"`)},
		{Field: "last_transition_at", Op: "<", Value: json.RawMessage(`"` + cutoffStr + `"`)},
	}
	if err := s.cleanupFiltered(filters); err != nil {
		return err
	}

	filters = []models.DocFilter{
		{Field: "state", Op: "==", Value: json.RawMessage(`"expired"`)},
		{Field: "last_transition_at", Op: "<", Value: json.RawMessage(`"` + cutoffStr + `"`)},
	}
	return s.cleanupFiltered(filters)
}

func (s *PlatformEnrollmentService) cleanupFiltered(filters []models.DocFilter) error {
	docs, err := s.db.DocQuery(platformEnrollmentCollectionName(), filters, "", 0)
	if err != nil {
		return fmt.Errorf("platform enrollment: cleanup: query: %w", err)
	}
	var deleted int
	for _, doc := range docs {
		_, err := s.db.DocDelete(platformEnrollmentCollectionName(), doc.ID)
		if err != nil {
			s.logger.Warn("platform enrollment: cleanup: delete failed", "doc_id", doc.ID, "error", err)
			continue
		}
		deleted++
	}
	if deleted > 0 {
		s.logger.Info("platform enrollment: cleaned up terminal requests", "count", deleted)
	}
	return nil
}

// findLiveRequest searches for a non-terminal, non-expired request
// matching the same component kind, instance ID, and fingerprint set.
// Returns nil if no match is found.
func (s *PlatformEnrollmentService) findLiveRequest(kind models.PlatformComponentKind, instanceID string, fingerprints models.PlatformEnrollmentCSRFingerprints) (*models.PlatformEnrollmentRequest, error) {
	docs, err := s.db.DocQuery(
		platformEnrollmentCollectionName(),
		[]models.DocFilter{
			{Field: "component_kind", Op: "==", Value: json.RawMessage(`"` + string(kind) + `"`)},
			{Field: "instance_id", Op: "==", Value: json.RawMessage(`"` + instanceID + `"`)},
		},
		"created_at", 0,
	)
	if err != nil {
		return nil, fmt.Errorf("query live requests: %w", err)
	}
	now := time.Now().UTC()
	for _, doc := range docs {
		req, err := decodePlatformEnrollmentRequest(doc)
		if err != nil {
			continue
		}
		if req.State.IsTerminal() {
			continue
		}
		if now.After(req.ExpiresAt) {
			s.expireRequest(req)
			continue
		}
		if fingerprintsMatch(req.Fingerprints, fingerprints) {
			return req, nil
		}
	}
	return nil, nil
}

// checkQuota rejects creation if the number of live requests for the
// given component kind exceeds the configured maximum.
func (s *PlatformEnrollmentService) checkQuota(kind models.PlatformComponentKind) error {
	docs, err := s.db.DocQuery(
		platformEnrollmentCollectionName(),
		[]models.DocFilter{{Field: "component_kind", Op: "==", Value: json.RawMessage(`"` + string(kind) + `"`)}},
		"", 0,
	)
	if err != nil {
		return fmt.Errorf("platform enrollment: quota query: %w", err)
	}
	now := time.Now().UTC()
	var live int
	for _, doc := range docs {
		req, err := decodePlatformEnrollmentRequest(doc)
		if err != nil {
			continue
		}
		if req.State.IsTerminal() {
			continue
		}
		if now.After(req.ExpiresAt) {
			s.expireRequest(req)
			continue
		}
		live++
	}
	if live >= constants.PlatformEnrollmentMaxLiveRequestsPerComponent {
		return constants.ErrPlatformEnrollmentQuotaExceeded
	}
	return nil
}

// submitEnvelope builds a GovernanceEnvelope with the given action type
// and PlatformEnrollmentGovernancePayload, marshals it as protojson,
// and calls the injected EnvelopeProcessor. The envelope carries the
// gateway's current state root, a unique nonce, and a near-future
// expiry. The payload is binary proto (the L4Warden decodes the same
// way).
func (s *PlatformEnrollmentService) submitEnvelope(ctx context.Context, action constants.PlatformEnrollmentGovernanceAction, intent constants.PlatformEnrollmentGovernanceIntent, payload *commonv1.PlatformEnrollmentGovernancePayload) (*commonv1.GovernanceEnvelope, error) {
	payloadBytes, err := marshalPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	stateRoot, err := s.stateRoot.GetCurrentStateRoot()
	if err != nil {
		return nil, fmt.Errorf("get state root: %w", err)
	}

	nonce, err := generateNonce()
	if err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	env := &commonv1.GovernanceEnvelope{
		ProtocolVersion: "1.0",
		Timestamp:       timestamppb.Now(),
		ExpiresAt:       timestamppb.New(time.Now().Add(5 * time.Minute)),
		SourceComponent: commonv1.Component_COMPONENT_G8EO,
		ActionType:      string(action),
		EventType:       string(constants.MapActionTypeToEventType(constants.ActionType(action))),
		Payload:         payloadBytes,
		StateMerkleRoot: stateRoot,
		Nonce:           nonce,
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

	receipt, err := s.envProc.ProcessEnvelope(ctx, wire)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrPlatformEnrollmentGovernanceRejected, err)
	}
	if receipt == nil {
		return nil, constants.ErrPlatformEnrollmentGovernanceRejected
	}
	_ = receipt
	return env, nil
}

// rollbackLease transitions a request from issuing back to approved so
// a retry can re-acquire the lease. Failures are logged; the caller
// has already decided to return an error.
func (s *PlatformEnrollmentService) rollbackLease(requestID string) {
	applied, err := s.db.DocConditionalUpdate(
		platformEnrollmentCollectionName(), requestID,
		map[string]interface{}{
			"state":                     string(models.PlatformEnrollmentStateApproved),
			"issuance_lease_owner":      "",
			"issuance_lease_expires_at": nil,
			"last_transition_at":        time.Now().UTC(),
		},
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
func (s *PlatformEnrollmentService) expireRequest(req *models.PlatformEnrollmentRequest) {
	applied, err := s.db.DocConditionalUpdate(
		platformEnrollmentCollectionName(), req.ID,
		map[string]interface{}{
			"state":              string(models.PlatformEnrollmentStateExpired),
			"last_transition_at": time.Now().UTC(),
		},
		"state", string(req.State),
	)
	if err != nil {
		s.logger.Warn("platform enrollment: expire failed", "request_id", req.ID, "error", err)
		return
	}
	if applied {
		s.logger.Info("platform enrollment request expired", "request_id", req.ID)
	}
}

// loadByToken loads a request by its opaque token. The token is hashed
// and the hash is used as the document ID for O(1) lookup. Wait — the
// document ID is the request ID (UUID), not the token hash. So we need
// to query by token_hash field.
func (s *PlatformEnrollmentService) loadByToken(token string) (*models.PlatformEnrollmentRequest, error) {
	tokenHash := platformEnrollmentTokenHash(token)
	docs, err := s.db.DocQuery(
		platformEnrollmentCollectionName(),
		[]models.DocFilter{{Field: "token_hash", Op: "==", Value: json.RawMessage(`"` + tokenHash + `"`)}},
		"", 1,
	)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: load by token: %w", err)
	}
	if len(docs) == 0 {
		return nil, constants.ErrPlatformEnrollmentRequestNotFound
	}
	req, err := decodePlatformEnrollmentRequest(docs[0])
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: decode request: %w", err)
	}
	return req, nil
}

// loadByID loads a request by its request ID (the document ID).
func (s *PlatformEnrollmentService) loadByID(requestID string) (*models.PlatformEnrollmentRequest, error) {
	doc, err := s.db.DocGet(platformEnrollmentCollectionName(), requestID)
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
