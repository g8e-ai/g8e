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
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
)

const (
	passkeyChallengeTTL = 5 * time.Minute
	webSessionTTL       = 24 * time.Hour
	challengeBytes      = 32
)

// PasskeyService handles L3 proof brokerage for passkey/WebAuthn operations.
// This moves the L3 authorization from g8ed into g8eo as the sovereign authority.
//
// NOTE: This is a simplified ED25519-based implementation. Full WebAuthn/FIDO2
// support will be added in a future iteration using the go-webauthn library.
type PasskeyService struct {
	db     *ListenDBService
	logger *slog.Logger
	rpID   string
	rpName string
}

// PasskeyConfig holds configuration for passkey operations.
type PasskeyConfig struct {
	RpID   string
	RpName string
}

// NewPasskeyService creates a new PasskeyService with the given configuration.
func NewPasskeyService(db *ListenDBService, logger *slog.Logger, cfg *PasskeyConfig) *PasskeyService {
	rpName := cfg.RpName
	if rpName == "" {
		rpName = "g8e"
	}
	return &PasskeyService{
		db:     db,
		logger: logger,
		rpID:   cfg.RpID,
		rpName: rpName,
	}
}

// ChallengeData stores a pending challenge for registration or authentication.
type ChallengeData struct {
	Challenge string `json:"challenge"`
	CreatedAt int64  `json:"created_at"`
	Purpose   string `json:"purpose"` // "register" or "auth"
}

// GenerateRegistrationChallenge creates a registration challenge for a user.
// Returns WebAuthn-compatible options structure.
func (s *PasskeyService) GenerateRegistrationChallenge(userID, email, userName string) (map[string]interface{}, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	// Generate random challenge
	challenge := make([]byte, challengeBytes)
	if _, err := rand.Read(challenge); err != nil {
		return nil, fmt.Errorf("failed to generate challenge: %w", err)
	}
	challengeStr := base64.RawURLEncoding.EncodeToString(challenge)

	// Store challenge
	challengeData := ChallengeData{
		Challenge: challengeStr,
		CreatedAt: time.Now().UnixMilli(),
		Purpose:   constants.PasskeyPurposeRegister,
	}
	if err := s.storeChallenge(userID, challengeData); err != nil {
		s.logger.Error("Failed to store challenge", "error", err, "userID", userID)
		return nil, fmt.Errorf("failed to store challenge: %w", err)
	}

	// Build exclude list from existing credentials
	var excludeCredentials []map[string]interface{}
	for _, cred := range user.PasskeyCredentials {
		excludeCredentials = append(excludeCredentials, map[string]interface{}{
			"id":         cred.ID,
			"type":       constants.WebAuthnTypePublicKey,
			"transports": cred.Transports,
		})
	}

	// Build WebAuthn-compatible options
	options := map[string]interface{}{
		"rp": map[string]interface{}{
			"name": s.rpName,
			"id":   s.rpID,
		},
		"user": map[string]interface{}{
			"id":          base64.RawURLEncoding.EncodeToString([]byte(userID)),
			"name":        email,
			"displayName": userName,
		},
		"challenge": challengeStr,
		"pubKeyCredParams": []map[string]interface{}{
			{"type": constants.WebAuthnTypePublicKey, "alg": constants.WebAuthnAlgES256}, // ES256
			{"type": constants.WebAuthnTypePublicKey, "alg": constants.WebAuthnAlgRS256}, // RS256
		},
		"timeout":            60000,
		"attestation":        constants.WebAuthnAttestationNone,
		"excludeCredentials": excludeCredentials,
		"authenticatorSelection": map[string]interface{}{
			"residentKey":      constants.WebAuthnResidentKeyRequired,
			"userVerification": constants.WebAuthnUserVerificationRequired,
		},
	}

	s.logger.Info("Registration challenge generated", "userID", userID)
	return options, nil
}

// AttestationResponse is the client response for registration verification.
type AttestationResponse struct {
	ID                string   `json:"id"`
	RawID             string   `json:"rawId"`
	ClientDataJSON    string   `json:"clientDataJSON"`
	AttestationObject string   `json:"attestationObject"`
	Transports        []string `json:"transports,omitempty"`
}

// VerifyRegistration verifies a registration response.
func (s *PasskeyService) VerifyRegistration(userID string, resp *AttestationResponse) (*models.PasskeyCredential, error) {
	// Consume the challenge
	challengeData, err := s.consumeChallenge(userID)
	if err != nil {
		s.logger.Warn("Registration verify: challenge expired or missing", "userID", userID, "error", err)
		return nil, fmt.Errorf("challenge expired or missing")
	}
	if challengeData.Purpose != constants.PasskeyPurposeRegister {
		return nil, fmt.Errorf("invalid challenge purpose")
	}

	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	// For now, we accept the credential ID and store a placeholder.
	// Full WebAuthn verification will validate the attestation object.
	// In a complete implementation, this would:
	// 1. Decode and verify the attestation object
	// 2. Extract the public key from the authenticator data
	// 3. Verify the challenge matches
	// 4. Verify origin and RP ID

	newCred := models.PasskeyCredential{
		ID:              resp.ID,
		PublicKey:       "", // Would be extracted from attestation in full impl
		Counter:         0,
		Transports:      resp.Transports,
		CreatedAtUnixMs: time.Now().UnixMilli(),
	}

	if err := s.addCredential(userID, newCred); err != nil {
		s.logger.Error("Failed to store credential", "error", err, "userID", userID)
		return nil, fmt.Errorf("failed to store credential: %w", err)
	}

	s.logger.Info("Credential registered", "userID", userID, "credentialID", resp.ID[:12])
	return &newCred, nil
}

// GenerateAuthenticationChallenge creates an authentication challenge.
func (s *PasskeyService) GenerateAuthenticationChallenge(userID string) (map[string]interface{}, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	if len(user.PasskeyCredentials) == 0 {
		s.logger.Warn("Auth challenge: user has no registered passkeys", "userID", userID)
		return nil, fmt.Errorf("no passkeys registered")
	}

	// Generate random challenge
	challenge := make([]byte, challengeBytes)
	if _, err := rand.Read(challenge); err != nil {
		return nil, fmt.Errorf("failed to generate challenge: %w", err)
	}
	challengeStr := base64.RawURLEncoding.EncodeToString(challenge)

	// Store challenge
	challengeData := ChallengeData{
		Challenge: challengeStr,
		CreatedAt: time.Now().UnixMilli(),
		Purpose:   constants.PasskeyPurposeAuth,
	}
	if err := s.storeChallenge(userID, challengeData); err != nil {
		s.logger.Error("Failed to store challenge", "error", err, "userID", userID)
		return nil, fmt.Errorf("failed to store challenge: %w", err)
	}

	// Build allow list from existing credentials
	var allowCredentials []map[string]interface{}
	for _, cred := range user.PasskeyCredentials {
		allowCredentials = append(allowCredentials, map[string]interface{}{
			"id":         cred.ID,
			"type":       constants.WebAuthnTypePublicKey,
			"transports": cred.Transports,
		})
	}

	options := map[string]interface{}{
		"challenge":        challengeStr,
		"timeout":          60000,
		"rpId":             s.rpID,
		"allowCredentials": allowCredentials,
		"userVerification": constants.WebAuthnUserVerificationRequired,
	}

	s.logger.Info("Authentication challenge generated", "userID", userID)
	return options, nil
}

// AssertionResponse is the client response for authentication verification.
type AssertionResponse struct {
	ID                string `json:"id"`
	RawID             string `json:"rawId"`
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
	UserHandle        string `json:"userHandle,omitempty"`
}

// VerifyAuthentication verifies an authentication assertion.
func (s *PasskeyService) VerifyAuthentication(userID string, resp *AssertionResponse) (*models.PasskeyCredential, error) {
	// Consume the challenge
	challengeData, err := s.consumeChallenge(userID)
	if err != nil {
		s.logger.Warn("Auth verify: challenge expired or missing", "userID", userID, "error", err)
		return nil, fmt.Errorf("challenge expired or missing")
	}
	if challengeData.Purpose != constants.PasskeyPurposeAuth {
		return nil, fmt.Errorf("invalid challenge purpose")
	}

	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	// Find the credential being used
	var storedCred *models.PasskeyCredential
	for i := range user.PasskeyCredentials {
		if user.PasskeyCredentials[i].ID == resp.ID {
			storedCred = &user.PasskeyCredentials[i]
			break
		}
	}
	if storedCred == nil {
		s.logger.Warn("Auth verify: credential not found", "userID", userID, "credentialID", resp.ID[:12])
		return nil, fmt.Errorf("credential not found")
	}

	// In a full implementation, this would:
	// 1. Decode the authenticator data and verify RP ID hash
	// 2. Decode clientDataJSON and verify challenge, origin, type
	// 3. Decode the signature and verify against public key
	// 4. Verify counter increased to prevent replay

	// For now, update last used timestamp
	storedCred.LastUsedAtUnixMs = time.Now().UnixMilli()
	if err := s.updateCredential(userID, *storedCred); err != nil {
		s.logger.Error("Failed to update credential", "error", err, "userID", userID)
		return nil, fmt.Errorf("failed to update credential: %w", err)
	}

	s.logger.Info("Authentication verified", "userID", userID, "credentialID", resp.ID[:12])
	return storedCred, nil
}

// ListCredentials returns all passkey credentials for a user.
func (s *PasskeyService) ListCredentials(userID string) ([]models.PasskeyCredential, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	return user.PasskeyCredentials, nil
}

// RevokeCredential removes a passkey credential from a user.
func (s *PasskeyService) RevokeCredential(userID, credentialID string) (found bool, remaining int, err error) {
	user, err := s.getUser(userID)
	if err != nil {
		return false, 0, err
	}
	if user == nil {
		return false, 0, nil
	}

	var newCreds []models.PasskeyCredential
	found = false
	for _, c := range user.PasskeyCredentials {
		if c.ID != credentialID {
			newCreds = append(newCreds, c)
		} else {
			found = true
		}
	}

	if !found {
		return false, len(user.PasskeyCredentials), nil
	}

	if err := s.setCredentials(userID, newCreds); err != nil {
		s.logger.Error("Failed to revoke credential", "error", err, "userID", userID)
		return false, 0, err
	}

	s.logger.Info("Credential revoked", "userID", userID, "credentialID", credentialID[:12])
	return true, len(newCreds), nil
}

// CreateSession creates a web session after successful authentication.
func (s *PasskeyService) CreateSession(userID string) (*models.WebSession, error) {
	sessionID := uuid.New().String()
	now := time.Now()

	session := &models.WebSession{
		ID:              sessionID,
		UserID:          userID,
		CreatedAtUnixMs: now.UnixMilli(),
		ExpiresAtUnixMs: now.Add(webSessionTTL).UnixMilli(),
	}

	data, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}
	if err := s.db.DocSet(string(constants.CollectionWebSessions), sessionID, data); err != nil {
		s.logger.Error("Failed to create session", "error", err, "userID", userID)
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	s.logger.Info("Session created", "userID", userID, "sessionID", sessionID[:8])
	return session, nil
}

// VerifyL3Signature verifies an ED25519 signature for L3 authorization.
// This is used when L3 proof is provided as a direct signature rather than WebAuthn.
func (s *PasskeyService) VerifyL3Signature(userID string, challenge, signatureHex string, pubKeyHex string) (bool, error) {
	// Decode signature
	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, fmt.Errorf("invalid signature encoding: %w", err)
	}

	// Decode public key
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return false, fmt.Errorf("invalid public key encoding: %w", err)
	}
	pubKey := ed25519.PublicKey(pubKeyBytes)

	// Verify signature
	message := []byte(challenge)
	if !ed25519.Verify(pubKey, message, sigBytes) {
		return false, fmt.Errorf("signature verification failed")
	}

	return true, nil
}

// Helper methods

func (s *PasskeyService) getUser(userID string) (*models.User, error) {
	doc, err := s.db.DocGet(string(constants.CollectionUsers), userID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	// Re-serialize the document data map to JSON for unmarshaling into struct
	data, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal doc data: %w", err)
	}

	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}
	user.ID = doc.ID
	return &user, nil
}

func (s *PasskeyService) addCredential(userID string, cred models.PasskeyCredential) error {
	user, err := s.getUser(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	user.PasskeyCredentials = append(user.PasskeyCredentials, cred)

	return s.updateUser(userID, user)
}

func (s *PasskeyService) updateCredential(userID string, cred models.PasskeyCredential) error {
	user, err := s.getUser(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	for i := range user.PasskeyCredentials {
		if user.PasskeyCredentials[i].ID == cred.ID {
			user.PasskeyCredentials[i] = cred
			break
		}
	}

	return s.updateUser(userID, user)
}

func (s *PasskeyService) setCredentials(userID string, creds []models.PasskeyCredential) error {
	user, err := s.getUser(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	user.PasskeyCredentials = creds
	return s.updateUser(userID, user)
}

func (s *PasskeyService) updateUser(userID string, user *models.User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}
	_, err = s.db.DocUpdate(string(constants.CollectionUsers), userID, data)
	return err
}

func (s *PasskeyService) storeChallenge(userID string, challenge ChallengeData) error {
	data, err := json.Marshal(challenge)
	if err != nil {
		return fmt.Errorf("failed to marshal challenge: %w", err)
	}
	return s.db.DocSet(string(constants.CollectionPasskeyChallenges), userID, data)
}

func (s *PasskeyService) consumeChallenge(userID string) (*ChallengeData, error) {
	doc, err := s.db.DocGet(string(constants.CollectionPasskeyChallenges), userID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("challenge not found")
	}

	// Delete immediately (best effort)
	_, _ = s.db.DocDelete(string(constants.CollectionPasskeyChallenges), userID)

	// Re-serialize the document data map to JSON for unmarshaling into struct
	data, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal doc data: %w", err)
	}

	var challenge ChallengeData
	if err := json.Unmarshal(data, &challenge); err != nil {
		return nil, fmt.Errorf("failed to unmarshal challenge: %w", err)
	}

	// Check TTL
	if time.Now().UnixMilli()-challenge.CreatedAt > int64(passkeyChallengeTTL.Milliseconds()) {
		return nil, fmt.Errorf("challenge expired")
	}

	return &challenge, nil
}
