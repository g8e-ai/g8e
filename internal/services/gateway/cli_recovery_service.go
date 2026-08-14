// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/timesvc"
	"github.com/g8e-ai/g8e/internal/uuid"
)

const (
	// cliRecoveryRequestTTL is the lifetime of a CLI recovery request (10 minutes).
	// The request must survive the time it takes for the user to open the browser,
	// authenticate with a passkey, and explicitly approve the new CLI.
	cliRecoveryRequestTTL = 10 * time.Minute
	// cliRecoveryTokenBytes is the number of random bytes in the opaque recovery token.
	cliRecoveryTokenBytes = 32
)

// CLIRecoveryService manages the lifecycle of human-approved CLI recovery
// requests. A recovery request lets a new CLI obtain credentials on an
// already-bootstrapped gateway by requiring an existing user to approve the
// request through the browser console.
//
// Security properties:
//   - The opaque token is never stored in cleartext; only its SHA-256 hash is
//     persisted.
//   - State transitions (pending → approved → completed, or pending → denied,
//     or any → expired) are atomic via DocConditionalUpdate, preventing
//     TOCTOU races where two completion callers could both obtain a
//     certificate.
//   - The request is bound to the CSR public-key fingerprint, which is used at
//     completion to verify proof-of-possession of the CSR private key.
//   - Only safe token/request prefixes are logged; raw tokens and certificate
//     material are never logged.
type CLIRecoveryService struct {
	db     *DocumentStoreService
	logger *slog.Logger
}

// NewCLIRecoveryService creates a new CLIRecoveryService.
func NewCLIRecoveryService(docStore *DocumentStoreService, logger *slog.Logger) *CLIRecoveryService {
	return &CLIRecoveryService{
		db:     docStore,
		logger: logger,
	}
}

// CreateRequest creates a new pending CLI recovery request. The caller supplies
// a CSR (containing the public key for the new CLI identity) and optional
// system metadata. The service generates an opaque one-time token, stores only
// its hash, and returns the raw token along with the request ID and expiration.
// The raw token is only returned once and must be conveyed to the user
// out-of-band (via the browser URL fragment).
func (s *CLIRecoveryService) CreateRequest(cliCSRPEM, systemFingerprint string, localOSUser *models.LocalOSUser) (requestID, token string, expiresAt time.Time, err error) {
	// Compute the CSR public-key fingerprint for proof-of-possession binding.
	csrFingerprint, err := csrPublicKeyFingerprint(cliCSRPEM)
	if err != nil {
		s.logger.Error("CLI recovery: failed to compute CSR fingerprint", "error", err)
		return "", "", time.Time{}, fmt.Errorf("cli recovery: CSR fingerprint: %w", err)
	}

	// Generate the opaque one-time token.
	tokenBytes := make([]byte, cliRecoveryTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		s.logger.Error("CLI recovery: failed to generate token", "error", err)
		return "", "", time.Time{}, constants.ErrCLIRecoveryRequestFailed
	}
	token = hex.EncodeToString(tokenBytes)
	tokenHash := hashToken(token)

	requestID = uuid.NewString()
	now := time.Now().UTC()
	expiresAt = now.Add(cliRecoveryRequestTTL)

	req := &models.CLIRecoveryRequest{
		ID:                requestID,
		TokenHash:         tokenHash,
		CLICSRPEM:         cliCSRPEM,
		CSRFingerprint:    csrFingerprint,
		SystemFingerprint: systemFingerprint,
		LocalOSUser:       localOSUser,
		State:             models.CLIRecoveryStatePending,
		CreatedAt:         now,
		ExpiresAt:         expiresAt,
	}

	data, err := json.Marshal(req)
	if err != nil {
		s.logger.Error("CLI recovery: failed to marshal request", "error", err)
		return "", "", time.Time{}, constants.ErrCLIRecoveryRequestFailed
	}

	// Use the token hash as the document ID for direct O(1) lookup by token.
	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash, data); err != nil {
		s.logger.Error("CLI recovery: failed to persist request", "error", err)
		return "", "", time.Time{}, constants.ErrCLIRecoveryRequestFailed
	}

	s.logger.Info("CLI recovery request created",
		"request_id", requestID,
		"token_prefix", safePrefix(token),
		"csr_fingerprint_prefix", safePrefix(csrFingerprint),
		"expires_at", expiresAt,
	)
	return requestID, token, expiresAt, nil
}

// GetByToken retrieves a CLI recovery request by its opaque token. The token is
// hashed and looked up in the document store. If the request has expired, it is
// atomically transitioned to the expired state and ErrCLIRecoveryRequestExpired
// is returned. If no request exists for the token, ErrCLIRecoveryRequestNotFound
// is returned.
func (s *CLIRecoveryService) GetByToken(token string) (*models.CLIRecoveryRequest, error) {
	tokenHash := hashToken(token)
	prefix := safePrefix(token)

	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash)
	if err != nil {
		s.logger.Error("CLI recovery: failed to look up request", "error", err, "token_prefix", prefix)
		return nil, constants.ErrCLIRecoveryRequestNotFound
	}
	if doc == nil {
		s.logger.Warn("CLI recovery: request not found", "token_prefix", prefix)
		return nil, constants.ErrCLIRecoveryRequestNotFound
	}

	req, err := decodeCLIRecoveryRequest(doc)
	if err != nil {
		s.logger.Error("CLI recovery: failed to decode request", "error", err, "token_prefix", prefix)
		return nil, constants.ErrCLIRecoveryRequestNotFound
	}

	// Auto-expire if the deadline has passed and the request is still in a
	// non-terminal state. Terminal states (completed, denied, expired) are
	// returned as-is so callers see the correct typed error.
	if isTerminalState(req.State) {
		return req, nil
	}
	if time.Now().UTC().After(req.ExpiresAt) {
		s.expireRequest(req, tokenHash, prefix)
		return nil, constants.ErrCLIRecoveryRequestExpired
	}

	return req, nil
}

// GetStatus returns the current lifecycle state of a CLI recovery request. If
// the request has expired, it is atomically transitioned to the expired state
// before returning.
func (s *CLIRecoveryService) GetStatus(token string) (models.CLIRecoveryState, error) {
	req, err := s.GetByToken(token)
	if err != nil {
		return "", err
	}
	return req.State, nil
}

// Approve atomically transitions a pending recovery request to the approved
// state, binding it to the approving user. Only a pending request can be
// approved. If the request has already expired, been denied, or been completed,
// the transition fails with the appropriate typed error.
func (s *CLIRecoveryService) Approve(token, approvingUserID string) error {
	return s.transitionOnApproval(token, approvingUserID, true)
}

// Deny atomically transitions a pending recovery request to the denied state,
// binding it to the denying user. Only a pending request can be denied.
func (s *CLIRecoveryService) Deny(token, denyingUserID string) error {
	return s.transitionOnApproval(token, denyingUserID, false)
}

func (s *CLIRecoveryService) transitionOnApproval(token, userID string, approve bool) error {
	tokenHash := hashToken(token)
	prefix := safePrefix(token)

	req, err := s.fetchByHash(tokenHash, prefix)
	if err != nil {
		return err
	}

	// Check terminal states first to give precise errors.
	if req.State == models.CLIRecoveryStateCompleted {
		return constants.ErrCLIRecoveryRequestConsumed
	}
	if req.State == models.CLIRecoveryStateApproved {
		return constants.ErrCLIRecoveryRequestConsumed
	}
	if req.State == models.CLIRecoveryStateDenied {
		return constants.ErrCLIRecoveryRequestDenied
	}
	if req.State == models.CLIRecoveryStateExpired {
		return constants.ErrCLIRecoveryRequestExpired
	}
	if req.State != models.CLIRecoveryStatePending {
		return constants.ErrCLIRecoveryRequestFailed
	}

	// Check expiry before transitioning.
	if time.Now().UTC().After(req.ExpiresAt) {
		s.expireRequest(req, tokenHash, prefix)
		return constants.ErrCLIRecoveryRequestExpired
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)

	targetState := models.CLIRecoveryStateApproved
	if !approve {
		targetState = models.CLIRecoveryStateDenied
	}

	setFields := map[string]interface{}{
		"state":             string(targetState),
		"approving_user_id": userID,
	}
	if approve {
		setFields["approved_at"] = nowStr
	} else {
		setFields["denied_at"] = nowStr
	}

	applied, err := s.db.DocConditionalUpdate(
		marshaler.CollectionName(constants.CollectionCLIRecoveryRequests),
		tokenHash,
		setFields,
		"state", string(models.CLIRecoveryStatePending),
	)
	if err != nil {
		s.logger.Error("CLI recovery: failed to transition state", "error", err, "token_prefix", prefix, "target_state", targetState)
		return constants.ErrCLIRecoveryRequestFailed
	}
	if !applied {
		// Another caller transitioned the request between our read and the
		// conditional update. Re-read to determine the actual state.
		current, err := s.fetchByHash(tokenHash, prefix)
		if err != nil {
			return err
		}
		switch current.State {
		case models.CLIRecoveryStateApproved:
			return constants.ErrCLIRecoveryRequestConsumed
		case models.CLIRecoveryStateDenied:
			return constants.ErrCLIRecoveryRequestDenied
		case models.CLIRecoveryStateExpired:
			return constants.ErrCLIRecoveryRequestExpired
		case models.CLIRecoveryStateCompleted:
			return constants.ErrCLIRecoveryRequestConsumed
		default:
			return constants.ErrCLIRecoveryRequestFailed
		}
	}

	s.logger.Info("CLI recovery request transitioned",
		"request_id", req.ID,
		"token_prefix", prefix,
		"new_state", targetState,
		"approving_user_id", userID,
	)
	return nil
}

// Complete atomically transitions an approved recovery request to the completed
// state and returns the full request for certificate issuance. Only an approved
// request can be completed. The caller (controller) is responsible for verifying
// proof-of-possession of the CSR private key before calling this method; the
// atomic transition ensures that only one concurrent caller can complete the
// request even if multiple callers possess a valid token.
func (s *CLIRecoveryService) Complete(token string) (*models.CLIRecoveryRequest, error) {
	tokenHash := hashToken(token)
	prefix := safePrefix(token)

	req, err := s.fetchByHash(tokenHash, prefix)
	if err != nil {
		return nil, err
	}

	// Check terminal/non-approved states first to give precise errors.
	if req.State == models.CLIRecoveryStatePending {
		return nil, constants.ErrCLIRecoveryNotApproved
	}
	if req.State == models.CLIRecoveryStateCompleted {
		return nil, constants.ErrCLIRecoveryRequestConsumed
	}
	if req.State == models.CLIRecoveryStateDenied {
		return nil, constants.ErrCLIRecoveryRequestDenied
	}
	if req.State == models.CLIRecoveryStateExpired {
		return nil, constants.ErrCLIRecoveryRequestExpired
	}
	if req.State != models.CLIRecoveryStateApproved {
		return nil, constants.ErrCLIRecoveryRequestFailed
	}

	// Check expiry before transitioning.
	if time.Now().UTC().After(req.ExpiresAt) {
		s.expireRequest(req, tokenHash, prefix)
		return nil, constants.ErrCLIRecoveryRequestExpired
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)

	applied, err := s.db.DocConditionalUpdate(
		marshaler.CollectionName(constants.CollectionCLIRecoveryRequests),
		tokenHash,
		map[string]interface{}{
			"state":        string(models.CLIRecoveryStateCompleted),
			"completed_at": nowStr,
		},
		"state", string(models.CLIRecoveryStateApproved),
	)
	if err != nil {
		s.logger.Error("CLI recovery: failed to complete request", "error", err, "token_prefix", prefix)
		return nil, constants.ErrCLIRecoveryRequestFailed
	}
	if !applied {
		// Another caller completed or transitioned the request between our read
		// and the conditional update. Re-read to determine the actual state.
		current, err := s.fetchByHash(tokenHash, prefix)
		if err != nil {
			return nil, err
		}
		switch current.State {
		case models.CLIRecoveryStateCompleted:
			return nil, constants.ErrCLIRecoveryRequestConsumed
		case models.CLIRecoveryStateDenied:
			return nil, constants.ErrCLIRecoveryRequestDenied
		case models.CLIRecoveryStateExpired:
			return nil, constants.ErrCLIRecoveryRequestExpired
		case models.CLIRecoveryStatePending:
			return nil, constants.ErrCLIRecoveryNotApproved
		default:
			return nil, constants.ErrCLIRecoveryRequestFailed
		}
	}

	req.State = models.CLIRecoveryStateCompleted
	req.CompletedAt = &now

	s.logger.Info("CLI recovery request completed",
		"request_id", req.ID,
		"token_prefix", prefix,
		"approving_user_id", req.ApprovingUserID,
	)
	return req, nil
}

// CleanupExpired removes recovery requests that have expired from the database.
// This should be called periodically to prevent unbounded growth of the
// cli_recovery_requests collection.
func (s *CLIRecoveryService) CleanupExpired() error {
	now := timesvc.NowTimestamp()
	filters := []models.DocFilter{
		{Field: "expires_at", Op: "<", Value: json.RawMessage(`"` + now + `"`)},
	}
	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), filters, "", 0)
	if err != nil {
		return fmt.Errorf("cli recovery: cleanup: query: %w", err)
	}

	var deleted int
	for _, doc := range docs {
		_, err := s.db.DocDelete(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), doc.ID)
		if err != nil {
			s.logger.Warn("Failed to delete expired CLI recovery request", "doc_id", doc.ID, "error", err)
			continue
		}
		deleted++
	}
	if deleted > 0 {
		s.logger.Info("Cleaned up expired CLI recovery requests", "count", deleted)
	}
	return nil
}

// fetchByHash retrieves a request by its token hash and returns precise errors
// for not-found and decode failures.
func (s *CLIRecoveryService) fetchByHash(tokenHash, prefix string) (*models.CLIRecoveryRequest, error) {
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash)
	if err != nil {
		s.logger.Error("CLI recovery: failed to look up request", "error", err, "token_prefix", prefix)
		return nil, constants.ErrCLIRecoveryRequestNotFound
	}
	if doc == nil {
		s.logger.Warn("CLI recovery: request not found", "token_prefix", prefix)
		return nil, constants.ErrCLIRecoveryRequestNotFound
	}
	req, err := decodeCLIRecoveryRequest(doc)
	if err != nil {
		s.logger.Error("CLI recovery: failed to decode request", "error", err, "token_prefix", prefix)
		return nil, constants.ErrCLIRecoveryRequestNotFound
	}
	return req, nil
}

// expireRequest attempts to atomically transition a non-terminal request to the
// expired state. Failures are logged but not returned; the caller has already
// decided to treat the request as expired.
func (s *CLIRecoveryService) expireRequest(req *models.CLIRecoveryRequest, tokenHash, prefix string) {
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	applied, err := s.db.DocConditionalUpdate(
		marshaler.CollectionName(constants.CollectionCLIRecoveryRequests),
		tokenHash,
		map[string]interface{}{
			"state": string(models.CLIRecoveryStateExpired),
		},
		"state", string(req.State),
	)
	if err != nil {
		s.logger.Warn("CLI recovery: failed to mark request as expired", "error", err, "token_prefix", prefix)
		return
	}
	if applied {
		s.logger.Info("CLI recovery request expired",
			"request_id", req.ID,
			"token_prefix", prefix,
			"expired_at", nowStr,
		)
	}
}

// hashToken computes the SHA-256 hash of the opaque token and returns it as a
// hex-encoded string. Only the hash is persisted; the raw token is never stored.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// safePrefix returns the first 8 characters of a sensitive string for logging.
// If the string is shorter than 8 characters, the entire string is returned.
func safePrefix(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// isTerminalState returns true if the state is completed, denied, or expired.
// Terminal-state requests are returned as-is by GetByToken without
// auto-expiry checks.
func isTerminalState(state models.CLIRecoveryState) bool {
	return state == models.CLIRecoveryStateCompleted ||
		state == models.CLIRecoveryStateDenied ||
		state == models.CLIRecoveryStateExpired
}

// decodeCLIRecoveryRequest deserializes a Document into a CLIRecoveryRequest.
func decodeCLIRecoveryRequest(doc *models.Document) (*models.CLIRecoveryRequest, error) {
	dataBytes, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal document data: %w", err)
	}
	var req models.CLIRecoveryRequest
	if err := json.Unmarshal(dataBytes, &req); err != nil {
		return nil, fmt.Errorf("unmarshal recovery request: %w", err)
	}
	return &req, nil
}

// csrPublicKeyFingerprint computes the SHA-256 fingerprint of the public key
// embedded in a PEM-encoded CSR. The fingerprint is the SHA-256 hash of the
// DER-encoded SubjectPublicKeyInfo, returned as a hex-encoded string. This
// fingerprint binds the recovery request to the CSR's key pair, enabling
// proof-of-possession verification at completion.
func csrPublicKeyFingerprint(csrPEM string) (string, error) {
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return "", fmt.Errorf("%w: invalid CSR PEM", constants.ErrPEMDecodeFailed)
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse CSR: %w", err)
	}
	// Verify the CSR signature integrity before trusting the embedded
	// public key. A CSR with an invalid signature must not be used to
	// bind a recovery request or compute a proof-of-possession fingerprint.
	if err := csr.CheckSignature(); err != nil {
		return "", fmt.Errorf("CSR signature verification failed: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(csr.PublicKey)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	h := sha256.Sum256(pubDER)
	return hex.EncodeToString(h[:]), nil
}

// VerifyProofOfPossession verifies that the caller possesses the private key
// corresponding to the CSR public key stored in the recovery request. The
// signature must be over the request ID, produced with the CSR's private key.
// This is called by the controller before Complete to ensure that stealing the
// opaque token alone is not sufficient to complete the recovery.
func (s *CLIRecoveryService) VerifyProofOfPossession(req *models.CLIRecoveryRequest, signature []byte) error {
	block, _ := pem.Decode([]byte(req.CLICSRPEM))
	if block == nil {
		return constants.ErrCLIRecoveryProofInvalid
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return constants.ErrCLIRecoveryProofInvalid
	}
	pubKey, ok := csr.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return constants.ErrCLIRecoveryProofInvalid
	}
	// Verify the ECDSA signature over the SHA-256 hash of the request ID.
	msgHash := sha256.Sum256([]byte(req.ID))
	if !ecdsa.VerifyASN1(pubKey, msgHash[:], signature) {
		return constants.ErrCLIRecoveryProofInvalid
	}
	return nil
}

// VerifyTokenCSRMatch verifies that the CSR in the completion request matches
// the CSR originally submitted in the recovery request. This prevents a
// different key from being certified under the same approved token.
func (s *CLIRecoveryService) VerifyTokenCSRMatch(req *models.CLIRecoveryRequest, csrPEM string) error {
	fingerprint, err := csrPublicKeyFingerprint(csrPEM)
	if err != nil {
		return constants.ErrCLIRecoveryCSRMismatch
	}
	if subtle.ConstantTimeCompare([]byte(fingerprint), []byte(req.CSRFingerprint)) != 1 {
		return constants.ErrCLIRecoveryCSRMismatch
	}
	return nil
}
