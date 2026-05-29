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

	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

const (
	// Session binding KV prefixes
	sessionWebBindPrefix      = "g8e:session:web:"
	sessionOperatorBindPrefix = "g8e:session:operator:"
	sessionCLIBindPrefix      = "g8e:session:cli:"
	sessionBindSuffix         = ":bind"
)

// RegistrationService handles Gateway-native device enrollment via CSR-based authentication.
type RegistrationService struct {
	db         *GatewayDBService
	pki        *PKIAuthority
	logger     *slog.Logger
	userSvc    *UserService
	sessionSvc *SessionService
	cfg        *config.GatewayConfig
}

// NewRegistrationService creates a new RegistrationService.
func NewRegistrationService(db *GatewayDBService, pki *PKIAuthority, logger *slog.Logger, userSvc *UserService, sessionSvc *SessionService, cfg *config.GatewayConfig) *RegistrationService {
	return &RegistrationService{
		db:         db,
		pki:        pki,
		logger:     logger,
		userSvc:    userSvc,
		sessionSvc: sessionSvc,
		cfg:        cfg,
	}
}

// sessionWebBindKey returns the KV key for binding a web session to operator sessions.
func sessionWebBindKey(webSessionID string) string {
	return sessionWebBindPrefix + webSessionID + sessionBindSuffix
}

// sessionOperatorBindKey returns the KV key for binding an operator session to a web session.
func sessionOperatorBindKey(operatorSessionID string) string {
	return sessionOperatorBindPrefix + operatorSessionID + sessionBindSuffix
}

func (s *RegistrationService) ListOperatorSlots(userID string) ([]models.OperatorDocumentGo, error) {
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	filters := []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
		{Field: "is_slot", Op: "==", Value: json.RawMessage("true")},
	}
	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "slot_number", 0)
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
		return fmt.Errorf("operator_id is required")
	}
	if userID == "" {
		return fmt.Errorf("user_id is required")
	}

	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionOperators), operatorID)
	if err != nil {
		return fmt.Errorf("failed to fetch operator: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("operator not found")
	}

	op, err := s.toOperatorDoc(doc)
	if err != nil {
		return err
	}

	if op.UserID != userID {
		return fmt.Errorf("operator does not belong to user")
	}

	if op.Status == constants.OperatorStatus(marshaler.OperatorStatus(constants.OperatorStatusTerminated)) {
		return nil // Already terminated
	}

	// Update operator to terminated status
	update := map[string]interface{}{
		"status":     constants.OperatorStatusTerminated,
		"updated_at": time.Now().UTC(),
	}
	if reason != "" {
		update["termination_reason"] = reason
	}
	updateBytes, _ := json.Marshal(update)
	if _, err := s.db.DocUpdate(marshaler.CollectionName(constants.CollectionOperators), operatorID, updateBytes); err != nil {
		return fmt.Errorf("failed to update operator status: %w", err)
	}

	s.logger.Info("[REGISTRATION] Operator terminated",
		"operator_id", operatorID,
		"user_id", userID,
		"reason", reason)

	return nil
}

// RegisterDeviceCSR handles CSR-based enrollment.
// Clients must present a valid client certificate (mTLS) and provide CSRs
// for operator and CLI certificates. The user_id is extracted from the
// client certificate's SPIFFE URI SAN.
func (s *RegistrationService) RegisterDeviceCSR(userID, organizationID string, req models.OperatorRegistrationRequest) (*models.OperatorRegistrationResponse, error) {
	s.logger.Info("[REGISTRATION] CSR-based enrollment", "hostname", req.Hostname, "user_id", userID)

	if req.SystemFingerprint == "" {
		return nil, fmt.Errorf("system_fingerprint is required")
	}
	if userID == "" {
		return nil, fmt.Errorf("user_id is required (extracted from client certificate)")
	}
	if req.CSR == "" {
		return nil, fmt.Errorf("operator CSR is required")
	}

	// Sanitize fingerprint
	sanitizedFingerprint := strings.ToLower(strings.Trim(req.SystemFingerprint, " \t\n\r"))
	if sanitizedFingerprint == "" {
		return nil, fmt.Errorf("invalid system_fingerprint")
	}

	// Resolve or create operator slot
	var operator *models.OperatorDocumentGo
	var err error

	// Try fingerprint match first
	filters := []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
		{Field: "system_fingerprint", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", sanitizedFingerprint))},
	}
	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 1)
	if err == nil && len(docs) > 0 {
		operator, _ = s.toOperatorDoc(docs[0])
	}

	// Try offline slot
	if operator == nil {
		filters = []models.DocFilter{
			{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
			{Field: "status", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", constants.OperatorStatusOffline))},
		}
		docs, err = s.db.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 1)
		if err == nil && len(docs) > 0 {
			operator, _ = s.toOperatorDoc(docs[0])
		}
	}

	// Create new slot
	if operator == nil {
		operator, err = s.createSlot(userID, organizationID)
		if err != nil {
			return nil, fmt.Errorf("failed to create operator slot: %w", err)
		}
	}

	if operator == nil {
		return nil, fmt.Errorf("failed to resolve operator slot")
	}

	// Complete registration with CSR
	linkData := &models.DeviceLinkData{
		UserID:         userID,
		OrganizationID: organizationID,
	}
	resp, err := s.completeRegistration(operator, linkData, req, sanitizedFingerprint)
	if err != nil {
		return nil, err
	}

	// Retire bootstrap user if this is a real login
	if s.userSvc != nil && userID != "" {
		bootstrapUser, err := s.userSvc.FindBootstrapUser()
		if err != nil {
			s.logger.Error("[REGISTRATION] Failed to check for bootstrap user", string(constants.ConnectionStateError), err)
		} else if bootstrapUser != nil && bootstrapUser.ID != userID {
			s.logger.Info("[REGISTRATION] Retiring bootstrap user on real login",
				"bootstrap_user_id", bootstrapUser.ID,
				"new_user_id", userID,
				"operator_id", operator.ID)
			if err := s.userSvc.Disable(bootstrapUser.ID, "retired_by_real_login", userID, operator.ID); err != nil {
				s.logger.Error("[REGISTRATION] Failed to retire bootstrap user", string(constants.ConnectionStateError), err)
				return nil, fmt.Errorf("registration failed: bootstrap retirement failed: %w", err)
			}
		}
	}

	s.logger.Info("[REGISTRATION] CSR-based enrollment complete",
		"operator_id", operator.ID, "user_id", userID)

	return resp, nil
}

// completeRegistration performs the common registration logic after operator slot is resolved.
func (s *RegistrationService) completeRegistration(operator *models.OperatorDocumentGo, linkData *models.DeviceLinkData, req models.OperatorRegistrationRequest, sanitizedFingerprint string) (*models.OperatorRegistrationResponse, error) {
	// Create operator session
	operatorSessionID := uuid.NewString()
	operatorSessionSummary := &models.SessionSummary{
		OperatorSessionID: operatorSessionID,
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(1 * time.Hour),
	}

	// Update operator document
	update := map[string]interface{}{
		"status":              constants.OperatorStatusActive,
		"operator_session_id": operatorSessionID,
		"system_fingerprint":  sanitizedFingerprint,
		string(constants.HistoryEventTypeClaimed): true,
		"claimed_at": time.Now().UTC(),
	}

	// Mint a strictly-disjoint cli_session_id alongside the operator session.
	// See OperatorRegistrationResponse doc: the two session types must never
	// share an identifier.
	cliSessionID := uuid.NewString()

	// CSR-based enrollment
	if req.CSR != "" {
		// Basic CSR validation
		block, _ := pem.Decode([]byte(req.CSR))
		if block == nil || block.Type != "CERTIFICATE REQUEST" {
			return nil, fmt.Errorf("invalid CSR PEM format")
		}

		// Use operator.OrganizationID instead of linkData.OrganizationID to ensure
		// the certificate SPIFFE ID matches the operator document in the database
		orgID := operator.OrganizationID
		if orgID == "" {
			orgID = linkData.OrganizationID
		}
		certPEM, chainPEM, err := s.pki.SignCSR(req.CSR, constants.LeafTypeOperator, orgID, operator.ID, "", operatorSessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to sign operator CSR: %w", err)
		}
		update["operator_cert"] = certPEM
		update["operator_cert_chain"] = chainPEM
		update["operator_cert_serial"] = ""
	} else {
		return nil, fmt.Errorf("CSR required for device registration")
	}

	// CLI certificate generation (optional for backwards compatibility)
	// If the client provides a CLI CSR, generate a CLI certificate with distinct SPIFFE identity
	var cliCertPEM, cliCertChainPEM, cliCertFingerprint, cliCertSerial string
	if req.CLICSR != "" {
		block, _ := pem.Decode([]byte(req.CLICSR))
		if block == nil || block.Type != "CERTIFICATE REQUEST" {
			return nil, fmt.Errorf("invalid CLI CSR PEM format")
		}

		var err error
		cliCertPEM, cliCertChainPEM, err = s.pki.SignCSR(req.CLICSR, constants.LeafTypeCLI, "", "", linkData.UserID, cliSessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to sign CLI CSR: %w", err)
		}
		// Calculate fingerprint and serial from the issued CLI certificate
		cliCertFingerprint = calculateFingerprintFromPEM(cliCertPEM)
		cliCertSerial = calculateSerialFromPEM(cliCertPEM)
	} else {
		// [SPIFFE-DRIFT] Fallback: If no CLI CSR provided, the CLI cert returned MUST be
		// the operator cert for backwards compatibility with older binaries, even though
		// they will fail modern /cli/ path checks.
		// NOTE: New protocol requires CLI CSR for distinct /cli/ SPIFFE ID.
		cliCertPEM = update["operator_cert"].(string)
		cliCertChainPEM = update["operator_cert_chain"].(string)
		cliCertFingerprint = calculateFingerprintFromPEM(cliCertPEM)
		cliCertSerial = calculateSerialFromPEM(cliCertPEM)
	}

	updateBytes, _ := json.Marshal(update)
	_, err := s.db.DocUpdate(marshaler.CollectionName(constants.CollectionOperators), operator.ID, updateBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to update operator status: %w", err)
	}

	// Fetch trust bundle
	hubBundle, _ := s.pki.HubTrustBundle()

	// Resolve operator cert and chain from updated doc
	finalCertPEM := update["operator_cert"].(string)
	finalChainPEM := update["operator_cert_chain"].(string)

	err = s.sessionSvc.PersistSessions(
		cliSessionID,
		operatorSessionID,
		linkData.UserID,
		linkData.OrganizationID,
		operator.ID,
		sanitizedFingerprint,
		cliCertFingerprint,
		cliCertSerial,
		"csr",
	)
	if err != nil {
		return nil, err
	}

	return &models.OperatorRegistrationResponse{
		Success:                true,
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
	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionOperators), filters, "", 0)
	if err == nil {
		slotNumber = len(docs) + 1
	}

	op := &models.OperatorDocumentGo{
		ID:             id,
		UserID:         userID,
		OrganizationID: orgID,
		Component:      constants.ComponentName(marshaler.Status(constants.ComponentNameG8EO)),
		Name:           fmt.Sprintf("operator-%d", slotNumber),
		Status:         constants.OperatorStatus(marshaler.OperatorStatus(constants.OperatorStatusOffline)),
		SlotNumber:     slotNumber,
		IsSlot:         true,
		OperatorType:   constants.OperatorTypeSystem,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	b, _ := json.Marshal(op)
	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionOperators), id, b); err != nil {
		return nil, err
	}

	return op, nil
}

// BindOperators binds one or more operators to a session.
func (s *RegistrationService) BindOperators(req models.BindOperatorsRequest) (*models.BindOperatorsResponse, error) {
	if req.WebSessionID == "" {
		return nil, fmt.Errorf("web_session_id is required")
	}
	if req.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	if len(req.OperatorIDs) == 0 {
		return nil, fmt.Errorf("operator_ids required")
	}

	bound := []string{}
	failed := []string{}
	var lastErr error

	for _, opID := range req.OperatorIDs {
		doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionOperators), opID)
		if err != nil {
			failed = append(failed, opID)
			lastErr = err
			continue
		}
		if doc == nil {
			failed = append(failed, opID)
			lastErr = fmt.Errorf("operator %s not found", opID)
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
			lastErr = fmt.Errorf("operator %s does not belong to user", opID)
			continue
		}
		if op.OperatorSessionID == "" {
			failed = append(failed, opID)
			lastErr = fmt.Errorf("operator %s has no active session", opID)
			continue
		}

		// 1. Update KV binding
		// sessionBindOperators(operatorSessionId) -> webSessionId
		if err := s.db.KVSet(sessionOperatorBindKey(op.OperatorSessionID), req.WebSessionID, 0); err != nil {
			failed = append(failed, opID)
			lastErr = err
			continue
		}

		// sessionWebBind(webSessionId) -> operatorSessionId (SET)
		// We use a JSON array for the SET since our KV store is simple
		webBindKey := sessionWebBindKey(req.WebSessionID)
		raw, found := s.db.KVGet(webBindKey)
		var sessionIDs []string
		if found {
			_ = json.Unmarshal([]byte(raw), &sessionIDs)
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
			body, _ := json.Marshal(sessionIDs)
			_ = s.db.KVSet(webBindKey, string(body), 0)
		}

		// 2. Update durability document
		docID := req.WebSessionID
		existingDoc, _ := s.db.DocGet(marshaler.CollectionName(constants.CollectionBoundSessions), docID)
		if existingDoc == nil {
			newDoc := models.BoundSessionsDocumentGo{
				ID:                 docID,
				WebSessionID:       req.WebSessionID,
				UserID:             req.UserID,
				OperatorSessionIDs: []string{op.OperatorSessionID},
				OperatorIDs:        []string{opID},
				BoundAt:            time.Now().UTC(),
				LastUpdatedAt:      time.Now().UTC(),
				Status:             constants.OperatorStatus(marshaler.OperatorStatus(constants.OperatorStatusActive)),
			}
			body, _ := json.Marshal(newDoc)
			_ = s.db.DocSet(marshaler.CollectionName(constants.CollectionBoundSessions), docID, body)
		} else {
			var bDoc models.BoundSessionsDocumentGo
			b, _ := json.Marshal(existingDoc.ForWire())
			_ = json.Unmarshal(b, &bDoc)

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
				bDoc.Status = constants.OperatorStatus(marshaler.OperatorStatus(constants.OperatorStatusActive))
				body, _ := json.Marshal(bDoc)
				_, _ = s.db.DocUpdate(marshaler.CollectionName(constants.CollectionBoundSessions), docID, body)
			}
		}

		// 3. Update operator document itself (for UI)
		_, _ = s.db.DocUpdate(marshaler.CollectionName(constants.CollectionOperators), opID, []byte(fmt.Sprintf(`{"bound_web_session_id": %q}`, req.WebSessionID)))

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
		return nil, fmt.Errorf("web_session_id is required")
	}
	if req.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	unbound := []string{}
	failed := []string{}
	var lastErr error

	for _, opID := range req.OperatorIDs {
		doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionOperators), opID)
		if err != nil {
			failed = append(failed, opID)
			lastErr = err
			continue
		}
		if doc == nil {
			failed = append(failed, opID)
			lastErr = fmt.Errorf("operator %s not found", opID)
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
			lastErr = fmt.Errorf("operator %s does not belong to user", opID)
			continue
		}

		// 1. Update KV binding
		if op.OperatorSessionID != "" {
			_ = s.db.KVDelete(sessionOperatorBindKey(op.OperatorSessionID))

			webBindKey := sessionWebBindKey(req.WebSessionID)
			raw, found := s.db.KVGet(webBindKey)
			if found {
				var sessionIDs []string
				_ = json.Unmarshal([]byte(raw), &sessionIDs)
				newSessionIDs := []string{}
				for _, sid := range sessionIDs {
					if sid != op.OperatorSessionID {
						newSessionIDs = append(newSessionIDs, sid)
					}
				}
				if len(newSessionIDs) == 0 {
					_ = s.db.KVDelete(webBindKey)
				} else {
					body, _ := json.Marshal(newSessionIDs)
					_ = s.db.KVSet(webBindKey, string(body), 0)
				}
			}
		}

		// 2. Update durability document
		docID := req.WebSessionID
		existingDoc, _ := s.db.DocGet(marshaler.CollectionName(constants.CollectionBoundSessions), docID)
		if existingDoc != nil {
			var bDoc models.BoundSessionsDocumentGo
			b, _ := json.Marshal(existingDoc.ForWire())
			_ = json.Unmarshal(b, &bDoc)

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
				bDoc.Status = constants.OperatorStatus(marshaler.OperatorStatus(constants.OperatorStatusTerminated))
			}
			body, _ := json.Marshal(bDoc)
			_, _ = s.db.DocUpdate(marshaler.CollectionName(constants.CollectionBoundSessions), docID, body)
		}

		// 3. Update operator document itself
		_, _ = s.db.DocUpdate(marshaler.CollectionName(constants.CollectionOperators), opID, []byte(`{"bound_web_session_id": ""}`))

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

// SetTargetContext sets the active target operator for a web session.
func (s *RegistrationService) SetTargetContext(req models.SetTargetContextRequest) (*models.SetTargetContextResponse, error) {
	if req.WebSessionID == "" {
		return nil, fmt.Errorf("web_session_id is required")
	}
	if req.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	// For now, "target context" is just making sure the operator is bound to the operator session.
	// In the future, this might set a specific "active" flag in the operator session state.

	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionOperators), req.OperatorID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("operator %s not found", req.OperatorID)
	}
	op, err := s.toOperatorDoc(doc)
	if err != nil {
		return nil, err
	}
	if op.UserID != req.UserID {
		return nil, fmt.Errorf("operator does not belong to user")
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
			return nil, fmt.Errorf("failed to bind operator for target context: %s", bindRes.Error)
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
