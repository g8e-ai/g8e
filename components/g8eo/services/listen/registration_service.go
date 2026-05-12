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

package listen

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
)

const (
	deviceLinkKeyPrefix            = "g8e:device-link:"
	deviceLinkFingerprintSetPrefix = "g8e:device-link-fingerprints:"
	deviceLinkLockPrefix           = "g8e:device-link-lock:"
	defaultDeviceLinkTTL           = 24 * time.Hour
	minDeviceLinkTTL               = 5 * time.Minute
	maxDeviceLinkTTL               = 7 * 24 * time.Hour
	defaultDeviceLinkMaxUses       = 1
	maxDeviceLinkMaxUses           = 1000
	deviceLinkStatusActive         = "active"
	deviceLinkStatusPending        = "pending"
	deviceLinkStatusRevoked        = "revoked"
	deviceLinkStatusExpired        = "expired"
	deviceLinkStatusExhausted      = "exhausted"
	lockTTL                        = 10 * time.Second
	lockMaxRetries                 = 30
	lockRetryDelay                 = 50 * time.Millisecond

	// Session binding KV prefixes
	sessionWebBindPrefix      = "g8e:session:web:"
	sessionOperatorBindPrefix = "g8e:session:operator:"
	sessionBindSuffix         = ":bind"
)

// RegistrationService handles substrate-native device enrollment.
// Ported from g8ed/DeviceLinkService and g8ee/OperatorAuthService.
type RegistrationService struct {
	db     *ListenDBService
	pki    *PKIAuthority
	logger *slog.Logger
}

// NewRegistrationService creates a new RegistrationService.
func NewRegistrationService(db *ListenDBService, pki *PKIAuthority, logger *slog.Logger) *RegistrationService {
	return &RegistrationService{
		db:     db,
		pki:    pki,
		logger: logger,
	}
}

func deviceLinkKey(token string) string {
	return deviceLinkKeyPrefix + token
}

func deviceLinkFingerprintSetKey(token string) string {
	return deviceLinkFingerprintSetPrefix + token
}

func deviceLinkLockKey(token string) string {
	return deviceLinkLockPrefix + token
}

func sessionWebBindKey(webSessionID string) string {
	return sessionWebBindPrefix + webSessionID + sessionBindSuffix
}

func sessionOperatorBindKey(operatorSessionID string) string {
	return sessionOperatorBindPrefix + operatorSessionID + sessionBindSuffix
}

func isValidDeviceLinkToken(token string) bool {
	return strings.HasPrefix(token, "dlk_") && len(token) >= 20 && !strings.ContainsAny(token, `/\`)
}

func newDeviceLinkToken() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "dlk_" + base64.RawURLEncoding.EncodeToString(raw), nil
}

// fingerprintSetAdd adds a fingerprint to the set atomically.
// Returns true if added, false if already present.
func (s *RegistrationService) fingerprintSetAdd(token, fingerprint string) (bool, error) {
	setKey := deviceLinkFingerprintSetKey(token)
	raw, found := s.db.KVGet(setKey)
	if !found {
		// New set, create with this fingerprint
		fingerprints := []string{fingerprint}
		body, err := json.Marshal(fingerprints)
		if err != nil {
			return false, err
		}
		if err := s.db.KVSet(setKey, string(body), 0); err != nil {
			return false, err
		}
		return true, nil
	}

	var fingerprints []string
	if err := json.Unmarshal([]byte(raw), &fingerprints); err != nil {
		return false, err
	}

	// Check if fingerprint already exists
	for _, fp := range fingerprints {
		if fp == fingerprint {
			return false, nil
		}
	}

	// Add fingerprint
	fingerprints = append(fingerprints, fingerprint)
	body, err := json.Marshal(fingerprints)
	if err != nil {
		return false, err
	}
	if err := s.db.KVSet(setKey, string(body), 0); err != nil {
		return false, err
	}
	return true, nil
}

// fingerprintSetRemove removes a fingerprint from the set.
func (s *RegistrationService) fingerprintSetRemove(token, fingerprint string) error {
	setKey := deviceLinkFingerprintSetKey(token)
	raw, found := s.db.KVGet(setKey)
	if !found {
		return nil
	}

	var fingerprints []string
	if err := json.Unmarshal([]byte(raw), &fingerprints); err != nil {
		return err
	}

	// Remove fingerprint
	var newFingerprints []string
	for _, fp := range fingerprints {
		if fp != fingerprint {
			newFingerprints = append(newFingerprints, fp)
		}
	}

	body, err := json.Marshal(newFingerprints)
	if err != nil {
		return err
	}
	return s.db.KVSet(setKey, string(body), 0)
}

// fingerprintSetCount returns the number of fingerprints in the set.
func (s *RegistrationService) fingerprintSetCount(token string) (int, error) {
	setKey := deviceLinkFingerprintSetKey(token)
	raw, found := s.db.KVGet(setKey)
	if !found {
		return 0, nil
	}

	var fingerprints []string
	if err := json.Unmarshal([]byte(raw), &fingerprints); err != nil {
		return 0, err
	}
	return len(fingerprints), nil
}

// acquireLock attempts to acquire a distributed lock with retry.
func (s *RegistrationService) acquireLock(lockKey string) (bool, error) {
	lockValue := uuid.NewString()
	for attempt := 0; attempt < lockMaxRetries; attempt++ {
		// Try to set the lock key if it doesn't exist
		_, found := s.db.KVGet(lockKey)
		if !found {
			if err := s.db.KVSet(lockKey, lockValue, int(lockTTL.Seconds())); err == nil {
				return true, nil
			}
		}
		// Wait with exponential backoff
		backoff := lockRetryDelay * time.Duration(attempt+1)
		time.Sleep(backoff)
	}
	return false, nil
}

// releaseLock releases a distributed lock.
func (s *RegistrationService) releaseLock(lockKey, lockValue string) error {
	current, found := s.db.KVGet(lockKey)
	if !found {
		return nil
	}
	if current == lockValue {
		return s.db.KVDelete(lockKey)
	}
	return nil
}

func (s *RegistrationService) CreateDeviceLink(req models.CreateDeviceLinkRequest) (*models.DeviceLinkResponse, error) {
	if req.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	maxUses := req.MaxUses
	if maxUses == 0 {
		maxUses = defaultDeviceLinkMaxUses
	}
	if maxUses < 1 || maxUses > maxDeviceLinkMaxUses {
		return nil, fmt.Errorf("max_uses must be between 1 and %d", maxDeviceLinkMaxUses)
	}
	ttl := defaultDeviceLinkTTL
	if req.TTLSeconds != 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	if ttl < minDeviceLinkTTL || ttl > maxDeviceLinkTTL {
		return nil, fmt.Errorf("ttl_seconds must be between %d and %d", int(minDeviceLinkTTL.Seconds()), int(maxDeviceLinkTTL.Seconds()))
	}
	if req.OperatorID != "" {
		doc, err := s.db.DocGet(string(constants.CollectionOperators), req.OperatorID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch operator: %w", err)
		}
		if doc == nil {
			return nil, fmt.Errorf("operator slot %s not found", req.OperatorID)
		}
		operator, err := s.toOperatorDoc(doc)
		if err != nil {
			return nil, err
		}
		if operator.UserID != req.UserID {
			return nil, fmt.Errorf("operator slot does not belong to user")
		}
	}
	token, err := newDeviceLinkToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate device link token: %w", err)
	}
	now := time.Now().UTC()
	status := deviceLinkStatusActive
	if req.OperatorID != "" {
		status = deviceLinkStatusPending
	}
	link := models.DeviceLinkData{
		Token:          token,
		UserID:         req.UserID,
		OrganizationID: req.OrganizationID,
		OperatorID:     req.OperatorID,
		WebSessionID:   req.WebSessionID,
		Name:           strings.TrimSpace(req.Name),
		MaxUses:        maxUses,
		Uses:           0,
		Status:         status,
		CreatedAt:      now,
		ExpiresAt:      now.Add(ttl),
		Claims:         []models.DeviceLinkClaim{},
	}
	body, err := json.Marshal(link)
	if err != nil {
		return nil, err
	}
	if err := s.db.KVSet(deviceLinkKey(token), string(body), int(ttl.Seconds())); err != nil {
		return nil, err
	}
	return deviceLinkResponse(link), nil
}

func (s *RegistrationService) ListDeviceLinks(userID string) ([]models.DeviceLinkListItem, error) {
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	keys, err := s.db.KVKeys(deviceLinkKeyPrefix + "*")
	if err != nil {
		return nil, err
	}
	links := make([]models.DeviceLinkListItem, 0)
	for _, key := range keys {
		raw, found := s.db.KVGet(key)
		if !found {
			continue
		}
		var link models.DeviceLinkData
		if err := json.Unmarshal([]byte(raw), &link); err != nil {
			return nil, fmt.Errorf("failed to parse device link: %w", err)
		}
		if link.UserID != userID {
			continue
		}
		status := link.Status
		if link.ExpiresAt.Before(time.Now()) {
			status = deviceLinkStatusExpired
		}
		links = append(links, models.DeviceLinkListItem{
			Token:     link.Token,
			Name:      link.Name,
			MaxUses:   link.MaxUses,
			Uses:      link.Uses,
			Status:    status,
			CreatedAt: link.CreatedAt,
			ExpiresAt: link.ExpiresAt,
		})
	}
	return links, nil
}

func (s *RegistrationService) DeleteDeviceLink(token, userID string) error {
	if !isValidDeviceLinkToken(token) {
		return fmt.Errorf("invalid device link token")
	}
	raw, found := s.db.KVGet(deviceLinkKey(token))
	if !found {
		return nil
	}
	var link models.DeviceLinkData
	if err := json.Unmarshal([]byte(raw), &link); err != nil {
		return fmt.Errorf("failed to parse device link: %w", err)
	}
	if link.UserID != userID {
		return fmt.Errorf("device link does not belong to user")
	}
	if link.Status == deviceLinkStatusActive && link.ExpiresAt.After(time.Now()) {
		link.Status = deviceLinkStatusRevoked
		now := time.Now().UTC()
		link.RevokedAt = &now
		body, err := json.Marshal(link)
		if err != nil {
			return err
		}
		return s.db.KVSet(deviceLinkKey(token), string(body), int(minDeviceLinkTTL.Seconds()))
	}
	return s.db.KVDelete(deviceLinkKey(token))
}

func (s *RegistrationService) ListOperatorSlots(userID string) ([]models.OperatorDocumentGo, error) {
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	filters := []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
		{Field: "is_slot", Op: "==", Value: json.RawMessage("true")},
	}
	docs, err := s.db.DocQuery(string(constants.CollectionOperators), filters, "slot_number", 0)
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

func (s *RegistrationService) RotateOperatorAPIKey(operatorID, userID string) (string, error) {
	if operatorID == "" {
		return "", fmt.Errorf("operator_id is required")
	}
	doc, err := s.db.DocGet(string(constants.CollectionOperators), operatorID)
	if err != nil {
		return "", err
	}
	if doc == nil {
		return "", fmt.Errorf("operator not found")
	}
	op, err := s.toOperatorDoc(doc)
	if err != nil {
		return "", err
	}
	if op.UserID != userID {
		return "", fmt.Errorf("operator does not belong to user")
	}

	prefix := operatorID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	newKey := fmt.Sprintf("g8e_%s_%s", prefix, uuid.NewString())
	update := map[string]interface{}{
		"operator_api_key": newKey,
		"updated_at":       time.Now().UTC(),
	}
	updateBytes, _ := json.Marshal(update)
	if _, err := s.db.DocUpdate(string(constants.CollectionOperators), operatorID, updateBytes); err != nil {
		return "", err
	}

	return newKey, nil
}

func (s *RegistrationService) TerminateOperator(operatorID, userID, reason string) error {
	if operatorID == "" {
		return fmt.Errorf("operator_id is required")
	}
	if userID == "" {
		return fmt.Errorf("user_id is required")
	}

	doc, err := s.db.DocGet(string(constants.CollectionOperators), operatorID)
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

	if op.Status == constants.Status.OperatorStatus.Terminated {
		return nil // Already terminated
	}

	// Update operator to terminated status
	update := map[string]interface{}{
		"status":     constants.Status.OperatorStatus.Terminated,
		"updated_at": time.Now().UTC(),
	}
	if reason != "" {
		update["termination_reason"] = reason
	}
	updateBytes, _ := json.Marshal(update)
	if _, err := s.db.DocUpdate(string(constants.CollectionOperators), operatorID, updateBytes); err != nil {
		return fmt.Errorf("failed to update operator status: %w", err)
	}

	s.logger.Info("[REGISTRATION] Operator terminated",
		"operator_id", operatorID,
		"user_id", userID,
		"reason", reason)

	return nil
}

func deviceLinkResponse(link models.DeviceLinkData) *models.DeviceLinkResponse {
	return &models.DeviceLinkResponse{
		Success:         true,
		Token:           link.Token,
		OperatorCommand: "g8e.operator --device-token " + link.Token,
		Name:            link.Name,
		MaxUses:         link.MaxUses,
		ExpiresAt:       link.ExpiresAt,
	}
}

// RegisterDevice handles the registration request from an enrolling operator binary.
func (s *RegistrationService) RegisterDevice(token string, req models.OperatorRegistrationRequest) (*models.OperatorRegistrationResponse, error) {
	s.logger.Info("[REGISTRATION] Registering device", "token", token, "hostname", req.Hostname)

	if req.SystemFingerprint == "" {
		return nil, fmt.Errorf("system_fingerprint is required")
	}

	// Sanitize fingerprint (remove non-hex characters, lowercase)
	sanitizedFingerprint := strings.ToLower(strings.Trim(req.SystemFingerprint, " \t\n\r"))
	if sanitizedFingerprint == "" {
		return nil, fmt.Errorf("invalid system_fingerprint")
	}

	// 1. Fetch and validate device link
	linkKey := deviceLinkKey(token)
	storedLink, found := s.db.KVGet(linkKey)
	if !found {
		return nil, fmt.Errorf("device link not found or expired")
	}

	var linkData models.DeviceLinkData
	if err := json.Unmarshal([]byte(storedLink), &linkData); err != nil {
		return nil, fmt.Errorf("failed to parse device link: %w", err)
	}

	if linkData.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("device link expired")
	}

	if linkData.Status == deviceLinkStatusRevoked {
		return nil, fmt.Errorf("device link revoked")
	}

	if linkData.Status == deviceLinkStatusExhausted {
		return nil, fmt.Errorf("device link exhausted")
	}

	// 2. Handle single-operator link (pending status)
	if linkData.Status == deviceLinkStatusPending {
		if linkData.OperatorID == "" {
			return nil, fmt.Errorf("pending link missing operator_id")
		}
		doc, err := s.db.DocGet(string(constants.CollectionOperators), linkData.OperatorID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch operator: %w", err)
		}
		if doc == nil {
			return nil, fmt.Errorf("operator slot %s not found", linkData.OperatorID)
		}
		operator, err := s.toOperatorDoc(doc)
		if err != nil {
			return nil, err
		}

		// Mark as used
		linkData.Status = deviceLinkStatusActive
		linkData.Uses = 1
		now := time.Now().UTC()
		linkData.UsedAt = &now
		linkData.DeviceInfo = &models.DeviceLinkInfo{
			SystemFingerprint: sanitizedFingerprint,
			Hostname:          req.Hostname,
			OS:                req.OS,
			Arch:              req.Arch,
			Username:          req.Username,
		}
		body, _ := json.Marshal(linkData)
		s.db.KVSet(linkKey, string(body), 0)

		return s.completeRegistration(operator, &linkData, req, sanitizedFingerprint)
	}

	// 3. Multi-use link: check if device already claimed via this link
	for _, claim := range linkData.Claims {
		if claim.SystemFingerprint == sanitizedFingerprint {
			// Device already claimed, reuse the same operator
			doc, err := s.db.DocGet(string(constants.CollectionOperators), claim.OperatorID)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch operator: %w", err)
			}
			if doc == nil {
				return nil, fmt.Errorf("operator slot %s not found", claim.OperatorID)
			}
			operator, err := s.toOperatorDoc(doc)
			if err != nil {
				return nil, err
			}
			return s.completeRegistration(operator, &linkData, req, sanitizedFingerprint)
		}
	}

	// 4. Fingerprint deduplication - prevent same device from registering twice on this link
	added, err := s.fingerprintSetAdd(token, sanitizedFingerprint)
	if err != nil {
		return nil, fmt.Errorf("fingerprint dedup failed: %w", err)
	}
	if !added {
		// Fingerprint already in set - poll for claim to appear
		for i := 0; i < 10; i++ {
			freshLinkRaw, found := s.db.KVGet(linkKey)
			if found {
				var freshLink models.DeviceLinkData
				if json.Unmarshal([]byte(freshLinkRaw), &freshLink) == nil {
					for _, claim := range freshLink.Claims {
						if claim.SystemFingerprint == sanitizedFingerprint {
							doc, err := s.db.DocGet(string(constants.CollectionOperators), claim.OperatorID)
							if err == nil && doc != nil {
								operator, _ := s.toOperatorDoc(doc)
								if operator != nil {
									return s.completeRegistration(operator, &freshLink, req, sanitizedFingerprint)
								}
							}
						}
					}
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
		fpPrefix := sanitizedFingerprint
		if len(fpPrefix) > 16 {
			fpPrefix = fpPrefix[:16]
		}
		s.logger.Error("[REGISTRATION] Device already registered - claim not found after polling",
			"token", token, "fingerprint", fpPrefix)
		return nil, fmt.Errorf("device already registered on this link")
	}

	// 5. Usage check - enforce max_uses
	currentUsage, err := s.fingerprintSetCount(token)
	if err != nil {
		s.fingerprintSetRemove(token, sanitizedFingerprint)
		return nil, fmt.Errorf("usage check failed: %w", err)
	}
	if currentUsage > linkData.MaxUses {
		s.fingerprintSetRemove(token, sanitizedFingerprint)
		s.logger.Error("[REGISTRATION] Link exhausted",
			"token", token, "current_usage", currentUsage, "max_uses", linkData.MaxUses)
		return nil, fmt.Errorf("device link exhausted")
	}

	// 6. Resolve operator slot
	var operator *models.OperatorDocumentGo

	// Try fingerprint match first
	filters := []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", linkData.UserID))},
		{Field: "system_fingerprint", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", sanitizedFingerprint))},
	}
	docs, err := s.db.DocQuery(string(constants.CollectionOperators), filters, "", 1)
	if err == nil && len(docs) > 0 {
		operator, _ = s.toOperatorDoc(docs[0])
	}

	// Try offline slot
	if operator == nil {
		filters = []models.DocFilter{
			{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", linkData.UserID))},
			{Field: "status", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", constants.Status.OperatorStatus.Offline))},
		}
		docs, err = s.db.DocQuery(string(constants.CollectionOperators), filters, "", 1)
		if err == nil && len(docs) > 0 {
			operator, _ = s.toOperatorDoc(docs[0])
		}
	}

	// Create new slot
	if operator == nil {
		operator, err = s.createSlot(linkData.UserID, linkData.OrganizationID)
		if err != nil {
			s.fingerprintSetRemove(token, sanitizedFingerprint)
			return nil, fmt.Errorf("failed to create operator slot: %w", err)
		}
	}

	if operator == nil {
		s.fingerprintSetRemove(token, sanitizedFingerprint)
		return nil, fmt.Errorf("failed to resolve operator slot")
	}

	// 7. Complete registration
	resp, err := s.completeRegistration(operator, &linkData, req, sanitizedFingerprint)
	if err != nil {
		s.fingerprintSetRemove(token, sanitizedFingerprint)
		return nil, err
	}

	// 8. Acquire lock to update linkData with claim
	lockKey := deviceLinkLockKey(token)
	lockValue := uuid.NewString()
	lockAcquired, err := s.acquireLock(lockKey)
	if !lockAcquired {
		s.logger.Error("[REGISTRATION] Failed to acquire registration lock", "token", token)
		return resp, nil // Registration succeeded, but claim update failed - device can retry
	}
	defer s.releaseLock(lockKey, lockValue)

	// 9. Add claim to linkData
	freshLinkRaw, found := s.db.KVGet(linkKey)
	if !found {
		return resp, nil
	}
	var freshLink models.DeviceLinkData
	if err := json.Unmarshal([]byte(freshLinkRaw), &freshLink); err != nil {
		return resp, nil
	}

	// Check if claim already added by concurrent request
	for _, claim := range freshLink.Claims {
		if claim.SystemFingerprint == sanitizedFingerprint {
			return resp, nil
		}
	}

	// Add claim
	freshLink.Claims = append(freshLink.Claims, models.DeviceLinkClaim{
		SystemFingerprint: sanitizedFingerprint,
		Hostname:          req.Hostname,
		OperatorID:        operator.ID,
		ClaimedAt:         time.Now().UTC(),
	})
	freshLink.Uses = len(freshLink.Claims)

	// Mark as exhausted if max uses reached
	if freshLink.Uses >= freshLink.MaxUses {
		freshLink.Status = deviceLinkStatusExhausted
	}

	body, _ := json.Marshal(freshLink)
	s.db.KVSet(linkKey, string(body), 0)

	s.logger.Info("[REGISTRATION] Device registered and link updated",
		"token", token, "operator_id", operator.ID, "uses", freshLink.Uses, "max_uses", freshLink.MaxUses)

	return resp, nil
}

// completeRegistration performs the common registration logic after operator slot is resolved.
func (s *RegistrationService) completeRegistration(operator *models.OperatorDocumentGo, linkData *models.DeviceLinkData, req models.OperatorRegistrationRequest, sanitizedFingerprint string) (*models.OperatorRegistrationResponse, error) {
	// Create session
	sessionID := uuid.NewString()
	session := &models.SessionSummary{
		ID:        sessionID,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}

	// Update operator document
	update := map[string]interface{}{
		"status":              constants.Status.OperatorStatus.Active,
		"operator_session_id": sessionID,
		"system_fingerprint":  sanitizedFingerprint,
		"claimed":             true,
		"claimed_at":          time.Now().UTC(),
	}

	// CSR-based enrollment
	if req.CSR != "" {
		certPEM, chainPEM, err := s.pki.SignCSR(req.CSR, constants.LeafTypeOperator, linkData.OrganizationID, operator.ID, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to sign operator CSR: %w", err)
		}
		update["operator_cert"] = certPEM
		update["operator_cert_chain"] = chainPEM
		update["operator_cert_serial"] = ""
	} else {
		return nil, fmt.Errorf("CSR required for device registration")
	}

	updateBytes, _ := json.Marshal(update)
	_, err := s.db.DocUpdate(string(constants.CollectionOperators), operator.ID, updateBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to update operator status: %w", err)
	}

	// Generate credentials
	apiKey := operator.OperatorAPIKey
	if apiKey == "" {
		apiKey = fmt.Sprintf("g8e_%s_%s", operator.ID[:8], uuid.NewString())
		s.db.DocUpdate(string(constants.CollectionOperators), operator.ID, json.RawMessage(fmt.Sprintf(`{"operator_api_key": %q}`, apiKey)))
	}

	// Fetch trust bundle
	hubBundle, _ := s.pki.HubTrustBundle()

	// Fetch operator cert and chain from updated doc
	finalCertPEM := update["operator_cert"].(string)
	finalChainPEM := update["operator_cert_chain"].(string)

	return &models.OperatorRegistrationResponse{
		Success:           true,
		OperatorID:        operator.ID,
		OperatorSessionID: sessionID,
		APIKey:            apiKey,
		OperatorCert:      finalCertPEM,
		OperatorCertChain: finalChainPEM,
		HubTrustBundle:    string(hubBundle),
		Session:           session,
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
	docs, err := s.db.DocQuery(string(constants.CollectionOperators), filters, "", 0)
	if err == nil {
		slotNumber = len(docs) + 1
	}

	op := &models.OperatorDocumentGo{
		ID:             id,
		UserID:         userID,
		OrganizationID: orgID,
		Component:      constants.Status.ComponentName.G8EO,
		Name:           fmt.Sprintf("operator-%d", slotNumber),
		Status:         constants.Status.OperatorStatus.Offline,
		SlotNumber:     slotNumber,
		IsSlot:         true,
		OperatorType:   constants.Status.OperatorType.System,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	b, _ := json.Marshal(op)
	if err := s.db.DocSet(string(constants.CollectionOperators), id, b); err != nil {
		return nil, err
	}

	return op, nil
}

// BindOperators binds one or more operators to a session.
func (s *RegistrationService) BindOperators(req models.BindOperatorsRequest) (*models.BindOperatorsResponse, error) {
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
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
		doc, err := s.db.DocGet(string(constants.CollectionOperators), opID)
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
		if err := s.db.KVSet(sessionOperatorBindKey(op.OperatorSessionID), req.SessionID, 0); err != nil {
			failed = append(failed, opID)
			lastErr = err
			continue
		}

		// sessionWebBind(webSessionId) -> operatorSessionId (SET)
		// We use a JSON array for the SET since our KV store is simple
		webBindKey := sessionWebBindKey(req.SessionID)
		raw, found := s.db.KVGet(webBindKey)
		var sessionIDs []string
		if found {
			json.Unmarshal([]byte(raw), &sessionIDs)
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
			s.db.KVSet(webBindKey, string(body), 0)
		}

		// 2. Update durability document
		docID := req.SessionID
		existingDoc, _ := s.db.DocGet(string(constants.CollectionBoundSessions), docID)
		if existingDoc == nil {
			newDoc := models.BoundSessionsDocumentGo{
				ID:                 docID,
				WebSessionID:       req.SessionID,
				UserID:             req.UserID,
				OperatorSessionIDs: []string{op.OperatorSessionID},
				OperatorIDs:        []string{opID},
				BoundAt:            time.Now().UTC(),
				LastUpdatedAt:      time.Now().UTC(),
				Status:             constants.Status.OperatorStatus.Active,
			}
			body, _ := json.Marshal(newDoc)
			s.db.DocSet(string(constants.CollectionBoundSessions), docID, body)
		} else {
			var bDoc models.BoundSessionsDocumentGo
			b, _ := json.Marshal(existingDoc.ForWire())
			json.Unmarshal(b, &bDoc)

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
				bDoc.Status = constants.Status.OperatorStatus.Active
				body, _ := json.Marshal(bDoc)
				s.db.DocUpdate(string(constants.CollectionBoundSessions), docID, body)
			}
		}

		// 3. Update operator document itself (for UI)
		s.db.DocUpdate(string(constants.CollectionOperators), opID, []byte(fmt.Sprintf(`{"bound_web_session_id": %q}`, req.SessionID)))

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
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	unbound := []string{}
	failed := []string{}
	var lastErr error

	for _, opID := range req.OperatorIDs {
		doc, err := s.db.DocGet(string(constants.CollectionOperators), opID)
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
			s.db.KVDelete(sessionOperatorBindKey(op.OperatorSessionID))

			webBindKey := sessionWebBindKey(req.SessionID)
			raw, found := s.db.KVGet(webBindKey)
			if found {
				var sessionIDs []string
				json.Unmarshal([]byte(raw), &sessionIDs)
				newSessionIDs := []string{}
				for _, sid := range sessionIDs {
					if sid != op.OperatorSessionID {
						newSessionIDs = append(newSessionIDs, sid)
					}
				}
				if len(newSessionIDs) == 0 {
					s.db.KVDelete(webBindKey)
				} else {
					body, _ := json.Marshal(newSessionIDs)
					s.db.KVSet(webBindKey, string(body), 0)
				}
			}
		}

		// 2. Update durability document
		docID := req.SessionID
		existingDoc, _ := s.db.DocGet(string(constants.CollectionBoundSessions), docID)
		if existingDoc != nil {
			var bDoc models.BoundSessionsDocumentGo
			b, _ := json.Marshal(existingDoc.ForWire())
			json.Unmarshal(b, &bDoc)

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
				bDoc.Status = "ended"
			}
			body, _ := json.Marshal(bDoc)
			s.db.DocUpdate(string(constants.CollectionBoundSessions), docID, body)
		}

		// 3. Update operator document itself
		s.db.DocUpdate(string(constants.CollectionOperators), opID, []byte(`{"bound_web_session_id": ""}`))

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

// SetTargetContext sets the active target operator for a session.
func (s *RegistrationService) SetTargetContext(req models.SetTargetContextRequest) (*models.SetTargetContextResponse, error) {
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	// For now, "target context" is just making sure the operator is bound to the session.
	// In the future, this might set a specific "active" flag in the session state.

	doc, err := s.db.DocGet(string(constants.CollectionOperators), req.OperatorID)
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

	if op.BoundWebSessionID != req.SessionID {
		// Not bound, so bind it first
		bindRes, err := s.BindOperators(models.BindOperatorsRequest{
			OperatorIDs: []string{req.OperatorID},
			UserID:      req.UserID,
			SessionID:   req.SessionID,
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
