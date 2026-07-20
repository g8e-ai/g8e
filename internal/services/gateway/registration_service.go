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
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/uuid"
)

const (
	// Session binding KV prefixes
	sessionWebBindPrefix      = "g8e:sessions:web:"
	sessionOperatorBindPrefix = "g8e:sessions:operator:"
	sessionCLIBindPrefix      = "g8e:sessions:cli:"
	sessionBindSuffix         = ":bind"
)

// RegistrationService handles Gateway-native device enrollment via CSR-based authentication.
type RegistrationService struct {
	docStore           *DocumentStoreService
	kvStore            *KVStoreService
	pki                *PKIAuthority
	logger             *slog.Logger
	userSvc            *UserService
	cliSessionSvc      *CLISessionService
	operatorSessionSvc *OperatorSessionService
	cfg                *config.GatewayConfig
}

// NewRegistrationService creates a new RegistrationService.
func NewRegistrationService(docStore *DocumentStoreService, kvStore *KVStoreService, pki *PKIAuthority, logger *slog.Logger, userSvc *UserService, cliSessionSvc *CLISessionService, operatorSessionSvc *OperatorSessionService, cfg *config.GatewayConfig) *RegistrationService {
	return &RegistrationService{
		docStore:           docStore,
		kvStore:            kvStore,
		pki:                pki,
		logger:             logger,
		userSvc:            userSvc,
		cliSessionSvc:      cliSessionSvc,
		operatorSessionSvc: operatorSessionSvc,
		cfg:                cfg,
	}
}

// sessionWebBindKey returns the KV key for binding a web session to Operator sessions.
func sessionWebBindKey(webSessionID string) string {
	return sessionWebBindPrefix + webSessionID + sessionBindSuffix
}

// sessionOperatorBindKey returns the KV key for binding an Operator session to a web session.
func sessionOperatorBindKey(operatorSessionID string) string {
	return sessionOperatorBindPrefix + operatorSessionID + sessionBindSuffix
}

func (s *RegistrationService) ListOperatorSlots(userID string) ([]models.OperatorDocumentGo, error) {
	if userID == "" {
		return nil, constants.ErrRegistrationUserIDRequired
	}
	filters := []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
		{Field: "is_slot", Op: "==", Value: json.RawMessage("true")},
	}
	docs, err := s.docStore.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "slot_number", 0)
	if err != nil {
		return nil, err
	}
	slots := make([]models.OperatorDocumentGo, 0, len(docs))
	for _, doc := range docs {
		slot, err := s.toOperatorDoc(doc)
		if err != nil {
			continue
		}
		slots = append(slots, *slot)
	}
	return slots, nil
}

func (s *RegistrationService) TerminateOperator(operatorID, userID, reason string) error {
	if operatorID == "" {
		return constants.ErrRegistrationOperatorIDRequired
	}
	if userID == "" {
		return constants.ErrRegistrationUserIDRequired
	}

	doc, err := s.docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), operatorID)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrRegistrationOperatorNotFound, err)
	}
	if doc == nil {
		return constants.ErrRegistrationOperatorNotFound
	}

	op, err := s.toOperatorDoc(doc)
	if err != nil {
		return err
	}

	if op.UserID != userID {
		return constants.ErrRegistrationOperatorNotBelongToUser
	}

	if op.Status == constants.OperatorStatusTerminated {
		return nil // Already terminated
	}

	// Update Operator to terminated status
	type operatorTerminationUpdate struct {
		Status            string    `json:"status"`
		UpdatedAt         time.Time `json:"updated_at"`
		TerminationReason string    `json:"termination_reason,omitempty"`
	}
	update := operatorTerminationUpdate{
		Status:    string(constants.OperatorStatusTerminated),
		UpdatedAt: time.Now().UTC(),
	}
	if reason != "" {
		update.TerminationReason = reason
	}
	updateBytes, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}
	if _, err := s.docStore.DocUpdate(marshaler.CollectionName(constants.CollectionOperators), operatorID, updateBytes); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}

	s.logger.Info("[REGISTRATION] Operator terminated",
		"operator_id", operatorID,
		"user_id", userID,
		"reason", reason)

	return nil
}

// RegisterDeviceCSR handles CSR-based enrollment.
// Clients must present a valid client certificate (mTLS) and provide CSRs
// for Operator and CLI certificates. The user_id is extracted from the
// client certificate's SPIFFE URI SAN.
func (s *RegistrationService) RegisterDeviceCSR(userID, organizationID string, req models.OperatorRegistrationRequest) (*models.OperatorRegistrationResponse, error) {
	s.logger.Info("[REGISTRATION] CSR-based enrollment", "hostname", req.Hostname, "user_id", userID)

	if req.SystemFingerprint == "" {
		return nil, constants.ErrRegistrationSystemFingerprintRequired
	}
	if userID == "" {
		return nil, constants.ErrRegistrationUserIDRequired
	}
	if req.CSR == "" {
		return nil, constants.ErrRegistrationOperatorCSRRequired
	}
	// CLI CSR is optional for operator-only enrollment

	// Sanitize fingerprint
	sanitizedFingerprint := strings.ToLower(strings.Trim(req.SystemFingerprint, " \t\n\r"))
	if sanitizedFingerprint == "" {
		return nil, constants.ErrRegistrationInvalidSystemFingerprint
	}

	// Resolve or create Operator slot
	var operator *models.OperatorDocumentGo
	var err error

	// Try fingerprint match first
	filters := []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
		{Field: "system_fingerprint", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", sanitizedFingerprint))},
	}
	docs, err := s.docStore.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 1)
	if err == nil && len(docs) > 0 {
		operator, _ = s.toOperatorDoc(docs[0])
	}

	// Try offline slot
	if operator == nil {
		filters = []models.DocFilter{
			{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
			{Field: "status", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", constants.OperatorStatusOffline))},
		}
		docs, err = s.docStore.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 1)
		if err == nil && len(docs) > 0 {
			operator, _ = s.toOperatorDoc(docs[0])
		}
	}

	// Create new slot
	if operator == nil {
		operator, err = s.createSlot(userID, organizationID)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrRegistrationFailedToCreateSlot, err)
		}
	}

	if operator == nil {
		return nil, constants.ErrRegistrationFailedToResolveSlot
	}

	// Complete registration with CSR
	resp, err := s.completeRegistration(operator, userID, organizationID, req, sanitizedFingerprint)
	if err != nil {
		return nil, err
	}

	// Retire bootstrap user if this is a real login
	if s.userSvc != nil && userID != "" {
		bootstrapUser, err := s.userSvc.FindBootstrapUser()
		if err != nil {
			s.logger.Error("[REGISTRATION] Failed to check for bootstrap user", "error", err)
		} else if bootstrapUser != nil && bootstrapUser.ID != userID {
			s.logger.Info("[REGISTRATION] Retiring bootstrap user on real login",
				"bootstrap_user_id", bootstrapUser.ID,
				"new_user_id", userID,
				"operator_id", operator.ID)
			if err := s.userSvc.Disable(bootstrapUser.ID, "retired_by_real_login", userID, operator.ID); err != nil {
				s.logger.Error("[REGISTRATION] Failed to retire bootstrap user", "error", err)
				return nil, fmt.Errorf("%w: %w", constants.ErrRegistrationBootstrapRetirementFailed, err)
			}
		}
	}

	s.logger.Info("[REGISTRATION] CSR-based enrollment complete",
		"operator_id", operator.ID, "user_id", userID)

	return resp, nil
}

// completeRegistration performs the common registration logic after Operator slot is resolved.
func (s *RegistrationService) completeRegistration(operator *models.OperatorDocumentGo, userID, organizationID string, req models.OperatorRegistrationRequest, sanitizedFingerprint string) (*models.OperatorRegistrationResponse, error) {
	// Create Operator session
	operatorSessionID := uuid.NewString()
	operatorSessionSummary := &models.SessionSummary{
		OperatorSessionID: operatorSessionID,
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(1 * time.Hour),
	}

	// Update Operator document
	type operatorClaimUpdate struct {
		Status             string    `json:"status"`
		OperatorSessionID  string    `json:"operator_session_id"`
		SystemFingerprint  string    `json:"system_fingerprint"`
		Claimed            bool      `json:"claimed"`
		ClaimedAt          time.Time `json:"claimed_at"`
		OperatorCert       string    `json:"operator_cert,omitempty"`
		OperatorCertChain  string    `json:"operator_cert_chain,omitempty"`
		OperatorCertSerial string    `json:"operator_cert_serial,omitempty"`
	}
	update := operatorClaimUpdate{
		Status:            string(constants.OperatorStatusActive),
		OperatorSessionID: operatorSessionID,
		SystemFingerprint: sanitizedFingerprint,
		Claimed:           true,
		ClaimedAt:         time.Now().UTC(),
	}

	// Mint a strictly-disjoint cli_session_id alongside the Operator session.
	// See OperatorRegistrationResponse doc: the two session types must never
	// share an identifier.
	// Only generate CLI session ID if CLI CSR is provided
	var cliSessionID string
	if req.CLICSR != "" {
		cliSessionID = uuid.NewString()
	}

	// CSR-based enrollment
	if req.CSR != "" {
		// Basic CSR validation
		block, _ := pem.Decode([]byte(req.CSR))
		if block == nil {
			return nil, constants.ErrRegistrationInvalidCSRPEMFormat
		}
		if block.Type != "CERTIFICATE REQUEST" {
			return nil, constants.ErrRegistrationCSRParsingFailed
		}

		// Use operator.OrganizationID, fallback to provided organizationID
		orgID := operator.OrganizationID
		if orgID == "" {
			orgID = organizationID
		}
		certPEM, chainPEM, signErr := s.pki.SignCSR(req.CSR, constants.LeafTypeOperator, orgID, operator.ID, "", operatorSessionID, "")
		if signErr != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrRegistrationCSRSignFailed, signErr)
		}
		update.OperatorCert = certPEM
		update.OperatorCertChain = chainPEM
		update.OperatorCertSerial = calculateSerialFromPEM(certPEM)
	} else {
		return nil, constants.ErrRegistrationCSRRequired
	}

	// CLI certificate generation - CLI CSR is optional for operator-only enrollment
	var cliCertPEM, cliCertChainPEM, cliCertFingerprint, cliCertSerial string
	if req.CLICSR != "" {
		block, _ := pem.Decode([]byte(req.CLICSR))
		if block == nil {
			return nil, constants.ErrRegistrationInvalidCSRPEMFormat
		}
		if block.Type != "CERTIFICATE REQUEST" {
			return nil, constants.ErrRegistrationCSRParsingFailed
		}

		var signErr error
		cliCertPEM, cliCertChainPEM, signErr = s.pki.SignCSR(req.CLICSR, constants.LeafTypeCLI, "", "", userID, cliSessionID, "")
		if signErr != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrRegistrationCSRSignFailed, signErr)
		}
		// Calculate fingerprint and serial from the issued CLI certificate
		cliCertFingerprint = calculateFingerprintFromPEM(cliCertPEM)
		cliCertSerial = calculateSerialFromPEM(cliCertPEM)
	}

	updateBytes, err := json.Marshal(update)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}
	_, updateErr := s.docStore.DocUpdate(marshaler.CollectionName(constants.CollectionOperators), operator.ID, updateBytes)
	if updateErr != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, updateErr)
	}

	// Fetch trust bundle
	hubBundle, err := s.pki.GatewayTrustBundle()
	if err != nil {
		s.logger.Warn("[REGISTRATION] Failed to fetch trust bundle", "error", err)
	}

	// Resolve Operator cert and chain from updated doc
	finalCertPEM := update.OperatorCert
	finalChainPEM := update.OperatorCertChain

	// Persist CLI session if CLI CSR was provided
	if cliSessionID != "" {
		persistErr := s.cliSessionSvc.PersistCLISession(
			cliSessionID,
			operatorSessionID,
			userID,
			sanitizedFingerprint,
			cliCertFingerprint,
			cliCertSerial,
			"csr",
		)
		if persistErr != nil {
			return nil, persistErr
		}
	}

	// Persist operator session
	if persistErr := s.operatorSessionSvc.PersistOperatorSession(
		operatorSessionID,
		userID,
		organizationID,
		operator.ID,
		"csr",
	); persistErr != nil {
		return nil, persistErr
	}
	return &models.OperatorRegistrationResponse{
		Success:                true,
		UserID:                 userID,
		OperatorID:             operator.ID,
		OperatorSessionID:      operatorSessionID,
		CLISessionID:           cliSessionID,
		OperatorCert:           finalCertPEM,
		OperatorCertChain:      finalChainPEM,
		CLICert:                cliCertPEM,
		CLICertChain:           cliCertChainPEM,
		HubTrustBundle:         string(hubBundle),
		OperatorSessionSummary: operatorSessionSummary,
	}, nil
}

func (s *RegistrationService) toOperatorDoc(doc *models.Document) (*models.OperatorDocumentGo, error) {
	b, err := json.Marshal(doc.ForWire())
	if err != nil {
		return nil, err
	}
	var op models.OperatorDocumentGo
	if err := json.Unmarshal(b, &op); err != nil {
		return nil, err
	}
	return &op, nil
}

func (s *RegistrationService) createSlot(userID, orgID string) (*models.OperatorDocumentGo, error) {
	id := uuid.NewString()
	if orgID == "" {
		orgID = userID
	}

	// Simple slot counter logic
	slotNumber := 1
	filters := []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
	}
	docs, err := s.docStore.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 0)
	if err == nil {
		slotNumber = len(docs) + 1
	}

	op := &models.OperatorDocumentGo{
		ID:             id,
		UserID:         userID,
		OrganizationID: orgID,
		Component:      constants.ComponentName(marshaler.Status(constants.ComponentNameG8EO)),
		Name:           fmt.Sprintf("operator-%d", slotNumber),
		Status:         constants.OperatorStatusOffline,
		SlotNumber:     slotNumber,
		IsSlot:         true,
		OperatorType:   constants.OperatorTypeSystem,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	b, err := json.Marshal(op)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}
	if err := s.docStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), id, b); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}

	return op, nil
}

// BindOperators binds one or more operators to a session.
func (s *RegistrationService) BindOperators(req models.BindOperatorsRequest) (*models.BindOperatorsResponse, error) {
	if req.WebSessionID == "" {
		return nil, constants.ErrRegistrationWebSessionIDRequired
	}
	if req.UserID == "" {
		return nil, constants.ErrRegistrationUserIDRequired
	}
	if len(req.OperatorIDs) == 0 {
		return nil, constants.ErrRegistrationOperatorIDsRequired
	}

	bound := []string{}
	failed := []string{}
	var lastErr error

	for _, opID := range req.OperatorIDs {
		doc, err := s.docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), opID)
		if err != nil {
			failed = append(failed, opID)
			lastErr = err
			continue
		}
		if doc == nil {
			failed = append(failed, opID)
			lastErr = constants.ErrRegistrationOperatorNotFound
			continue
		}
		op, err := s.toOperatorDoc(doc)
		if err != nil {
			failed = append(failed, opID)
			lastErr = err
			continue
		}
		if op.UserID != req.UserID {
			failed = append(failed, opID)
			lastErr = constants.ErrRegistrationOperatorNotBelongToUser
			continue
		}
		if op.OperatorSessionID == "" {
			failed = append(failed, opID)
			lastErr = constants.ErrRegistrationOperatorNoActiveSession
			continue
		}

		// 1. Update KV binding
		// sessionBindOperators(operatorSessionId) -> webSessionId
		if err := s.kvStore.KVSet(sessionOperatorBindKey(op.OperatorSessionID), req.WebSessionID, 0); err != nil {
			failed = append(failed, opID)
			lastErr = err
			continue
		}

		// sessionWebBind(webSessionId) -> operatorSessionId (SET)
		// We use a JSON array for the SET since our KV store is simple
		webBindKey := sessionWebBindKey(req.WebSessionID)
		raw, kvFound := s.kvStore.KVGet(webBindKey)
		var sessionIDs []string
		if kvFound {
			if err := json.Unmarshal([]byte(raw), &sessionIDs); err != nil {
				s.logger.Warn("[REGISTRATION] Failed to unmarshal session IDs", "error", err)
			}
		}
		exists := false
		for _, sid := range sessionIDs {
			if sid == op.OperatorSessionID {
				exists = true
				break
			}
		}
		if !exists {
			sessionIDs = append(sessionIDs, op.OperatorSessionID)
			body, err := json.Marshal(sessionIDs)
			if err != nil {
				failed = append(failed, opID)
				lastErr = fmt.Errorf("%w: %w", constants.ErrRegistrationFailedToMarshalSessionIDs, err)
				continue
			}
			if err := s.kvStore.KVSet(webBindKey, string(body), 0); err != nil {
				failed = append(failed, opID)
				lastErr = fmt.Errorf("%w: %w", constants.ErrRegistrationFailedToSetKVBinding, err)
				continue
			}
		}

		// 2. Update durability document
		docID := req.WebSessionID
		existingDoc, err := s.docStore.DocGet(marshaler.CollectionName(constants.CollectionBoundSessions), docID)
		if err != nil {
			failed = append(failed, opID)
			lastErr = fmt.Errorf("%w: %w", constants.ErrRegistrationFailedToGetBoundSessions, err)
			continue
		}
		if existingDoc == nil {
			newDoc := models.BoundSessionsDocumentGo{
				ID:                 docID,
				WebSessionID:       req.WebSessionID,
				UserID:             req.UserID,
				OperatorSessionIDs: []string{op.OperatorSessionID},
				OperatorIDs:        []string{opID},
				BoundAt:            time.Now().UTC(),
				LastUpdatedAt:      time.Now().UTC(),
				Status:             constants.OperatorStatusActive,
			}
			body, err := json.Marshal(newDoc)
			if err != nil {
				failed = append(failed, opID)
				lastErr = fmt.Errorf("%w: %w", constants.ErrRegistrationFailedToMarshalBoundSessions, err)
				continue
			}
			if err := s.docStore.DocSet(marshaler.CollectionName(constants.CollectionBoundSessions), docID, body); err != nil {
				failed = append(failed, opID)
				lastErr = fmt.Errorf("%w: %w", constants.ErrRegistrationFailedToSetBoundSessions, err)
				continue
			}
		} else {
			var bDoc models.BoundSessionsDocumentGo
			b, err := json.Marshal(existingDoc.ForWire())
			if err != nil {
				failed = append(failed, opID)
				lastErr = fmt.Errorf("%w: %w", constants.ErrRegistrationFailedToMarshalExistingDocument, err)
				continue
			}
			if err := json.Unmarshal(b, &bDoc); err != nil {
				failed = append(failed, opID)
				lastErr = fmt.Errorf("%w: %w", constants.ErrRegistrationFailedToUnmarshalBoundSessions, err)
				continue
			}

			opExists := false
			for _, id := range bDoc.OperatorIDs {
				if id == opID {
					opExists = true
					break
				}
			}
			if !opExists {
				bDoc.OperatorIDs = append(bDoc.OperatorIDs, opID)
				bDoc.OperatorSessionIDs = append(bDoc.OperatorSessionIDs, op.OperatorSessionID)
				bDoc.LastUpdatedAt = time.Now().UTC()
				bDoc.Status = constants.OperatorStatusActive
				body, err := json.Marshal(bDoc)
				if err != nil {
					failed = append(failed, opID)
					lastErr = fmt.Errorf("%w: %w", constants.ErrRegistrationFailedToMarshalBoundSessions, err)
					continue
				}
				if _, err := s.docStore.DocUpdate(marshaler.CollectionName(constants.CollectionBoundSessions), docID, body); err != nil {
					failed = append(failed, opID)
					lastErr = fmt.Errorf("%w: %w", constants.ErrRegistrationFailedToUpdateBoundSessions, err)
					continue
				}
			}
		}

		// 3. Update Operator document itself (for UI)
		if _, err := s.docStore.DocUpdate(marshaler.CollectionName(constants.CollectionOperators), opID, []byte(fmt.Sprintf(`{"bound_web_session_id": %q}`, req.WebSessionID))); err != nil {
			s.logger.Warn("[REGISTRATION] Failed to update operator bound session", "error", err, "operator_id", opID)
		}

		bound = append(bound, opID)
	}

	res := &models.BindOperatorsResponse{
		Success:           len(bound) > 0,
		BoundCount:        len(bound),
		FailedCount:       len(failed),
		BoundOperatorIDs:  bound,
		FailedOperatorIDs: failed,
	}
	if lastErr != nil && len(bound) == 0 {
		res.Error = lastErr.Error()
	}
	return res, nil
}

// UnbindOperators unbinds one or more operators from a session.
func (s *RegistrationService) UnbindOperators(req models.UnbindOperatorsRequest) (*models.UnbindOperatorsResponse, error) {
	if req.WebSessionID == "" {
		return nil, constants.ErrRegistrationWebSessionIDRequired
	}
	if req.UserID == "" {
		return nil, constants.ErrRegistrationUserIDRequired
	}

	unbound := []string{}
	failed := []string{}
	var lastErr error

	for _, opID := range req.OperatorIDs {
		doc, err := s.docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), opID)
		if err != nil {
			failed = append(failed, opID)
			lastErr = err
			continue
		}
		if doc == nil {
			failed = append(failed, opID)
			lastErr = constants.ErrRegistrationOperatorNotFound
			continue
		}
		op, err := s.toOperatorDoc(doc)
		if err != nil {
			failed = append(failed, opID)
			lastErr = err
			continue
		}
		if op.UserID != req.UserID {
			failed = append(failed, opID)
			lastErr = constants.ErrRegistrationOperatorNotBelongToUser
			continue
		}

		// 1. Update KV binding
		if op.OperatorSessionID != "" {
			if err := s.kvStore.KVDelete(sessionOperatorBindKey(op.OperatorSessionID)); err != nil {
				s.logger.Warn("[REGISTRATION] Failed to delete operator session binding", "error", err, "operator_session_id", op.OperatorSessionID)
			}

			webBindKey := sessionWebBindKey(req.WebSessionID)
			raw, kvFound := s.kvStore.KVGet(webBindKey)
			if kvFound {
				var sessionIDs []string
				if err := json.Unmarshal([]byte(raw), &sessionIDs); err != nil {
					s.logger.Warn("[REGISTRATION] Failed to unmarshal session IDs", "error", err)
					continue
				}
				newSessionIDs := []string{}
				for _, sid := range sessionIDs {
					if sid != op.OperatorSessionID {
						newSessionIDs = append(newSessionIDs, sid)
					}
				}
				if len(newSessionIDs) == 0 {
					if err := s.kvStore.KVDelete(webBindKey); err != nil {
						s.logger.Warn("[REGISTRATION] Failed to delete web session binding", "error", err)
					}
				} else {
					body, err := json.Marshal(newSessionIDs)
					if err != nil {
						s.logger.Warn("[REGISTRATION] Failed to marshal session IDs", "error", err)
						continue
					}
					if err := s.kvStore.KVSet(webBindKey, string(body), 0); err != nil {
						s.logger.Warn("[REGISTRATION] Failed to set session IDs", "error", err)
					}
				}
			}
		}

		// 2. Update durability document
		docID := req.WebSessionID
		existingDoc, err := s.docStore.DocGet(marshaler.CollectionName(constants.CollectionBoundSessions), docID)
		if err != nil {
			s.logger.Warn("[REGISTRATION] Failed to get bound sessions document", "error", err)
			continue
		}
		if existingDoc != nil {
			var bDoc models.BoundSessionsDocumentGo
			b, err := json.Marshal(existingDoc.ForWire())
			if err != nil {
				s.logger.Warn("[REGISTRATION] Failed to marshal existing document", "error", err)
				continue
			}
			if err := json.Unmarshal(b, &bDoc); err != nil {
				s.logger.Warn("[REGISTRATION] Failed to unmarshal bound sessions document", "error", err)
				continue
			}

			newOpIDs := []string{}
			newSessIDs := []string{}
			for i, id := range bDoc.OperatorIDs {
				if id != opID {
					newOpIDs = append(newOpIDs, id)
					newSessIDs = append(newSessIDs, bDoc.OperatorSessionIDs[i])
				}
			}
			bDoc.OperatorIDs = newOpIDs
			bDoc.OperatorSessionIDs = newSessIDs
			bDoc.LastUpdatedAt = time.Now().UTC()
			if len(newOpIDs) == 0 {
				bDoc.Status = constants.OperatorStatusTerminated
			}
			body, err := json.Marshal(bDoc)
			if err != nil {
				s.logger.Warn("[REGISTRATION] Failed to marshal updated bound sessions document", "error", err)
				continue
			}
			if _, err := s.docStore.DocUpdate(marshaler.CollectionName(constants.CollectionBoundSessions), docID, body); err != nil {
				s.logger.Warn("[REGISTRATION] Failed to update bound sessions document", "error", err)
			}
		}

		// 3. Update Operator document itself
		if _, err := s.docStore.DocUpdate(marshaler.CollectionName(constants.CollectionOperators), opID, []byte(`{"bound_web_session_id": ""}`)); err != nil {
			s.logger.Warn("[REGISTRATION] Failed to update operator bound session", "error", err, "operator_id", opID)
		}

		unbound = append(unbound, opID)
	}

	res := &models.UnbindOperatorsResponse{
		Success:            len(unbound) > 0 || len(req.OperatorIDs) == 0,
		UnboundCount:       len(unbound),
		FailedCount:        len(failed),
		UnboundOperatorIDs: unbound,
		FailedOperatorIDs:  failed,
	}
	if lastErr != nil && len(unbound) == 0 {
		res.Error = lastErr.Error()
	}
	return res, nil
}

// SetTargetContext sets the active target Operator for a web session.
func (s *RegistrationService) SetTargetContext(req models.SetTargetContextRequest) (*models.SetTargetContextResponse, error) {
	if req.WebSessionID == "" {
		return nil, constants.ErrRegistrationWebSessionIDRequired
	}
	if req.UserID == "" {
		return nil, constants.ErrRegistrationUserIDRequired
	}

	// For now, "target context" is just making sure the Operator is bound to the Operator session.
	// In the future, this might set a specific "active" flag in the Operator session state.

	doc, err := s.docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), req.OperatorID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, constants.ErrRegistrationOperatorNotFound
	}
	op, err := s.toOperatorDoc(doc)
	if err != nil {
		return nil, err
	}
	if op.UserID != req.UserID {
		return nil, constants.ErrRegistrationOperatorNotBelongToUser
	}

	if op.BoundWebSessionID != req.WebSessionID {
		// Not bound, so bind it first
		bindRes, err := s.BindOperators(models.BindOperatorsRequest{
			OperatorIDs:  []string{req.OperatorID},
			UserID:       req.UserID,
			WebSessionID: req.WebSessionID,
		})
		if err != nil {
			return nil, err
		}
		if !bindRes.Success {
			return nil, fmt.Errorf("%w: %s", constants.ErrRegistrationFailedToBindOperator, bindRes.Error)
		}
	}

	return &models.SetTargetContextResponse{
		Success:    true,
		OperatorID: req.OperatorID,
	}, nil
}

// calculateFingerprintFromPEM computes the SHA-256 fingerprint of a PEM-encoded certificate.
func calculateFingerprintFromPEM(certPEM string) string {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:])
}

// calculateSerialFromPEM extracts the serial number from a PEM-encoded certificate.
func calculateSerialFromPEM(certPEM string) string {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	return cert.SerialNumber.String()
}
