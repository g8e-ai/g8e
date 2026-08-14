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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/uuid"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

const (
	passkeyChallengeTTL = 5 * time.Minute
	challengeBytes      = 32
)

// MCPServiceProvider provides access to suspended transaction operations needed
// by the approval handlers. This interface is satisfied by *mcp.GatewayService.
type MCPServiceProvider interface {
	GetSuspendedTransaction(ctx context.Context, txHash string) (*models.SuspendedTransaction, bool, error)
	ResumeWithL3Proof(ctx context.Context, txHash, userID string, proof *commonv1.L3Proof) (*operatorv1.ActionReceipt, error)
}

// userStore defines the interface for user storage operations.
type userStore interface {
	GetUser(userID string) (*models.User, error)
	UpdateUser(userID string, user *models.User) error
	CreateUser() (*models.User, error)
	HasAnyUsers() (bool, error)
}

// sessionStore defines the interface for WebAuthn session storage.
type sessionStore interface {
	StoreSession(userID string, session *webauthn.SessionData) error
	GetSession(userID string) (*webauthn.SessionData, error)
	DeleteSession(userID string) error
}

// webauthnClient defines the interface for WebAuthn operations.
type webauthnClient interface {
	BeginRegistration(user webauthn.User) (*protocol.CredentialCreation, *webauthn.SessionData, error)
	FinishRegistration(user webauthn.User, session webauthn.SessionData, r *http.Request) (*webauthn.Credential, error)
	BeginLogin(user webauthn.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error)
	FinishLogin(user webauthn.User, session webauthn.SessionData, r *http.Request) (*webauthn.Credential, error)
	ValidateLogin(user webauthn.User, session webauthn.SessionData, parsedResponse *protocol.ParsedCredentialAssertionData) (*webauthn.Credential, error)
}

// PasskeyService handles L3 proof brokerage for passkey/WebAuthn operations.
// This moves the L3 authorization from client into g8eo as the sovereign authority.
type PasskeyService struct {
	userStore    userStore
	sessionStore sessionStore
	webauthn     webauthnClient
	logger       *slog.Logger
	rpID         string
	rpName       string
}

// PasskeyConfig holds configuration for passkey operations.
type PasskeyConfig struct {
	RpID      string
	RpName    string
	RpOrigins []string
	HTTPPort  int
	HTTPSPort int
}

// encodeCredID encodes a WebAuthn credential ID (raw bytes) as a base64 URL string.
// This is the canonical encoding used across all passkey transports.
func encodeCredID(id []byte) string {
	return base64.RawURLEncoding.EncodeToString(id)
}

// encodeChallenge encodes arbitrary challenge bytes (e.g. transaction hashes) as
// base64 URL. Semantically distinct from encodeCredID but uses the same encoding.
func encodeChallenge(challenge []byte) string {
	return base64.RawURLEncoding.EncodeToString(challenge)
}

// dbUserStore implements userStore using DocumentStoreService.
type dbUserStore struct {
	db *DocumentStoreService
}

func (s *dbUserStore) GetUser(userID string) (*models.User, error) {
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionUsers), userID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

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

func (s *dbUserStore) UpdateUser(userID string, user *models.User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}
	_, err = s.db.DocUpdate(marshaler.CollectionName(constants.CollectionUsers), userID, data)
	return err
}

func (s *dbUserStore) CreateUser() (*models.User, error) {
	userID := uuid.NewString()
	webAuthnUserID := uuid.NewString()

	u := &models.User{
		ID:                 userID,
		PasskeyCredentials: []models.PasskeyCredential{},
		Provider:           string(constants.AuthProviderPasskey),
		Status:             constants.UserStatusActive,
		WebAuthnUserID:     webAuthnUserID,
	}

	data, err := json.Marshal(u)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal user: %w", err)
	}
	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, data); err != nil {
		return nil, fmt.Errorf("failed to store user: %w", err)
	}
	return u, nil
}

func (s *dbUserStore) HasAnyUsers() (bool, error) {
	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionUsers), []models.DocFilter{}, "", 1)
	if err != nil {
		return false, err
	}
	return len(docs) > 0, nil
}

// dbSessionStore implements sessionStore using DocumentStoreService.
type dbSessionStore struct {
	db *DocumentStoreService
}

func (s *dbSessionStore) StoreSession(userID string, session *webauthn.SessionData) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.db.DocSet(marshaler.CollectionName(constants.CollectionPasskeyChallenges), userID, data)
}

func (s *dbSessionStore) GetSession(userID string) (*webauthn.SessionData, error) {
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionPasskeyChallenges), userID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, constants.ErrExpired
	}

	var session webauthn.SessionData
	b, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

func (s *dbSessionStore) DeleteSession(userID string) error {
	_, err := s.db.DocDelete(marshaler.CollectionName(constants.CollectionPasskeyChallenges), userID)
	return err
}

// realWebauthnClient implements webauthnClient using the actual webauthn library.
type realWebauthnClient struct {
	w *webauthn.WebAuthn
}

func (c *realWebauthnClient) BeginRegistration(user webauthn.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	return c.w.BeginRegistration(user)
}

func (c *realWebauthnClient) FinishRegistration(user webauthn.User, session webauthn.SessionData, r *http.Request) (*webauthn.Credential, error) {
	return c.w.FinishRegistration(user, session, r)
}

func (c *realWebauthnClient) BeginLogin(user webauthn.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	return c.w.BeginLogin(user)
}

func (c *realWebauthnClient) FinishLogin(user webauthn.User, session webauthn.SessionData, r *http.Request) (*webauthn.Credential, error) {
	return c.w.FinishLogin(user, session, r)
}

func (c *realWebauthnClient) ValidateLogin(user webauthn.User, session webauthn.SessionData, parsedResponse *protocol.ParsedCredentialAssertionData) (*webauthn.Credential, error) {
	return c.w.ValidateLogin(user, session, parsedResponse)
}

// buildRPOrigins constructs the WebAuthn RPOrigins list from PasskeyConfig.
// It is extracted as a pure function so tests can verify the origins list
// without constructing a full PasskeyService or depending on webauthn internals.
func buildRPOrigins(cfg *PasskeyConfig) []string {
	httpPort := cfg.HTTPPort
	if httpPort == 0 {
		httpPort = constants.Ports.OperatorHttp
	}
	httpsPort := cfg.HTTPSPort
	if httpsPort == 0 {
		httpsPort = constants.Ports.OperatorHttps
	}

	var rpOrigins []string
	if cfg.RpID == "localhost" || cfg.RpID == "127.0.0.1" {
		rpOrigins = []string{cfg.RpID,
			"http://localhost", fmt.Sprintf("http://localhost:%d", httpPort),
			"http://127.0.0.1", fmt.Sprintf("http://127.0.0.1:%d", httpPort),
			"https://localhost", fmt.Sprintf("https://localhost:%d", httpsPort),
			"https://127.0.0.1", fmt.Sprintf("https://127.0.0.1:%d", httpsPort),
		}
	} else {
		rpOrigins = []string{"https://" + cfg.RpID}
	}

	rpOrigins = append(rpOrigins, cfg.RpOrigins...)
	return rpOrigins
}

// NewPasskeyService creates a new PasskeyService with the given configuration.
func NewPasskeyService(docStore *DocumentStoreService, logger *slog.Logger, cfg *PasskeyConfig) (*PasskeyService, error) {
	rpName := cfg.RpName
	if rpName == "" {
		rpName = "g8e"
	}

	rpOrigins := buildRPOrigins(cfg)

	w, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RpID,
		RPDisplayName: rpName,
		RPOrigins:     rpOrigins,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize webauthn: %w", err)
	}

	return &PasskeyService{
		userStore:    &dbUserStore{db: docStore},
		sessionStore: &dbSessionStore{db: docStore},
		webauthn:     &realWebauthnClient{w: w},
		logger:       logger,
		rpID:         cfg.RpID,
		rpName:       rpName,
	}, nil
}

// PasskeyHandler handles HTTP endpoints for passkey registration, authentication,
// credential management, and OOB approval flows. It wraps a PasskeyService for
// domain logic and delegates business orchestration (MCP, suspended transactions,
// SSE, WebSocket) to PasskeyOrchestrator. HTTP-layer concerns only.
type PasskeyHandler struct {
	*PasskeyService
	webSessionSvc      *WebSessionService
	enrollmentTokenSvc *EnrollmentTokenService
	responder          *response.Writer
	maxPayload         int64
	orchestrator       *PasskeyOrchestrator
	crossOrigin        bool
}

// PasskeyHandlerDeps groups all dependencies for NewPasskeyHandler.
type PasskeyHandlerDeps struct {
	Service            *PasskeyService
	WebSessionSvc      *WebSessionService
	EnrollmentTokenSvc *EnrollmentTokenService
	Responder          *response.Writer
	MaxPayload         int64
	Orchestrator       *PasskeyOrchestrator
	CrossOrigin        bool
}

// NewPasskeyHandler creates a new PasskeyHandler with all dependencies wired.
func NewPasskeyHandler(deps PasskeyHandlerDeps) *PasskeyHandler {
	return &PasskeyHandler{
		PasskeyService:     deps.Service,
		webSessionSvc:      deps.WebSessionSvc,
		enrollmentTokenSvc: deps.EnrollmentTokenSvc,
		responder:          deps.Responder,
		maxPayload:         deps.MaxPayload,
		orchestrator:       deps.Orchestrator,
		crossOrigin:        deps.CrossOrigin,
	}
}

// ChallengeData stores a pending challenge for registration or authentication.
type ChallengeData struct {
	Challenge string `json:"challenge"`
	CreatedAt int64  `json:"created_at"`
	Purpose   string `json:"purpose"` // "register" or "auth"
}

// GenerateRegistrationChallenge creates a registration challenge for a user.
func (s *PasskeyService) GenerateRegistrationChallenge(userID, userName string) (*protocol.CredentialCreation, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, constants.ErrUserNotFound
	}

	options, session, err := s.webauthn.BeginRegistration(user)
	if err != nil {
		return nil, fmt.Errorf("failed to begin registration: %w", err)
	}

	// Store session data
	if err := s.storeWebAuthnSession(userID, session); err != nil {
		return nil, err
	}

	return options, nil
}

// VerifyRegistration verifies a registration response.
// It accepts the raw JSON of the WebAuthn response.
func (s *PasskeyService) VerifyRegistration(userID string, responseJSON []byte) (*models.PasskeyCredential, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, constants.ErrUserNotFound
	}

	session, err := s.getWebAuthnSession(userID)
	if err != nil {
		return nil, err
	}

	// The flat WebAuthnAttestationResponse JSON must be re-serialized into the
	// nested CredentialCreationResponse format expected by go-webauthn:
	//   {"id":"...","type":"public-key","rawId":"...","response":{"clientDataJSON":"...","attestationObject":"...","transports":[...]}}
	var att models.WebAuthnAttestationResponse
	if err := json.Unmarshal(responseJSON, &att); err != nil {
		return nil, fmt.Errorf("failed to parse attestation response: %w", err)
	}

	creationResponse := struct {
		ID       string                             `json:"id"`
		Type     string                             `json:"type"`
		RawID    string                             `json:"rawId"`
		Response models.WebAuthnAttestationResponse `json:"response"`
	}{
		ID:       att.ID,
		Type:     "public-key",
		RawID:    att.RawID,
		Response: att,
	}

	nestedJSON, err := json.Marshal(creationResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal creation response: %w", err)
	}

	r, err := http.NewRequest(http.MethodPost, "/", bytes.NewReader(nestedJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	r.Header.Set("Content-Type", "application/json")

	credential, err := s.webauthn.FinishRegistration(user, *session, r)
	if err != nil {
		return nil, fmt.Errorf("failed to finish registration: %w", err)
	}

	newCred := models.PasskeyCredential{
		ID:              credential.ID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		Transport:       credential.Transport,
		Authenticator: models.Authenticator{
			AAGUID:       credential.Authenticator.AAGUID,
			SignCount:    credential.Authenticator.SignCount,
			CloneWarning: credential.Authenticator.CloneWarning,
		},
		CreatedAtUnixMs: time.Now().UnixMilli(),
	}

	if err := s.addCredential(userID, newCred); err != nil {
		return nil, err
	}

	if delErr := s.deleteWebAuthnSession(userID); delErr != nil {
		s.logger.Warn("Failed to delete WebAuthn session after registration", "error", delErr, "userID", userID)
	}

	return &newCred, nil
}

// GenerateAuthenticationChallenge creates an authentication challenge.
func (s *PasskeyService) GenerateAuthenticationChallenge(userID string) (*protocol.CredentialAssertion, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, constants.ErrUserNotFound
	}

	if len(user.PasskeyCredentials) == 0 {
		return nil, constants.ErrNoPasskeysRegistered
	}

	options, session, err := s.webauthn.BeginLogin(user)
	if err != nil {
		return nil, fmt.Errorf("failed to begin login: %w", err)
	}

	if err := s.storeWebAuthnSession(userID, session); err != nil {
		return nil, err
	}

	return options, nil
}

// GenerateApprovalChallenge creates a WebAuthn assertion challenge bound to a transaction hash.
// This is used for Out-of-Band (OOB) approval of suspended transactions.
func (s *PasskeyService) GenerateApprovalChallenge(userID, transactionHash string) (*protocol.CredentialAssertion, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, constants.ErrUserNotFound
	}

	// We don't use BeginLogin here because we want to force the challenge to be the transaction hash.
	// We build the assertion options manually.
	var allowedCredentials []protocol.CredentialDescriptor
	for _, cred := range user.PasskeyCredentials {
		allowedCredentials = append(allowedCredentials, protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: cred.ID,
		})
	}

	options := &protocol.CredentialAssertion{
		Response: protocol.PublicKeyCredentialRequestOptions{
			Challenge:          protocol.URLEncodedBase64(encodeChallenge([]byte(transactionHash))),
			Timeout:            60000,
			RelyingPartyID:     s.rpID,
			AllowedCredentials: allowedCredentials,
			UserVerification:   protocol.VerificationPreferred,
		},
	}

	return options, nil
}

// VerifyAuthentication verifies an authentication assertion.
// It accepts the raw JSON of the WebAuthn response.
func (s *PasskeyService) VerifyAuthentication(userID string, responseJSON []byte) (*models.PasskeyCredential, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, constants.ErrUserNotFound
	}

	session, err := s.getWebAuthnSession(userID)
	if err != nil {
		return nil, err
	}

	// Reconstruct request with the response body
	r, err := http.NewRequest(http.MethodPost, "/", bytes.NewReader(responseJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	r.Header.Set("Content-Type", "application/json")

	credential, err := s.webauthn.FinishLogin(user, *session, r)
	if err != nil {
		return nil, fmt.Errorf("failed to finish login: %w", err)
	}

	// Update credential counter and last used
	var storedCred *models.PasskeyCredential
	for i := range user.PasskeyCredentials {
		if bytes.Equal(user.PasskeyCredentials[i].ID, credential.ID) {
			user.PasskeyCredentials[i].Authenticator.SignCount = credential.Authenticator.SignCount
			user.PasskeyCredentials[i].LastUsedAtUnixMs = time.Now().UnixMilli()
			storedCred = &user.PasskeyCredentials[i]
			break
		}
	}

	if err := s.updateUser(userID, user); err != nil {
		return nil, err
	}

	if delErr := s.deleteWebAuthnSession(userID); delErr != nil {
		s.logger.Warn("Failed to delete WebAuthn session after authentication", "error", delErr, "userID", userID)
	}

	return storedCred, nil
}

// listCredentials returns all passkey credentials for a user.
func (s *PasskeyService) listCredentials(userID string) ([]models.PasskeyCredential, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	return user.PasskeyCredentials, nil
}

// revokeCredential removes a passkey credential from a user.
func (s *PasskeyService) revokeCredential(userID, credentialID string) (found bool, remaining int, err error) {
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
		if encodeCredID(c.ID) != credentialID {
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

	credPrefix := credentialID
	if len(credentialID) > 12 {
		credPrefix = credentialID[:12]
	}
	s.logger.Info("Credential revoked", "userID", userID, "credentialID", credPrefix)
	return true, len(newCreds), nil
}

// VerifyL3Proof verifies a WebAuthn assertion against a registered passkey.
// The challenge is the transaction_hash.
// The cliSessionID parameter is ignored for web sessions (WebAuthn) but is required
// for interface compatibility with CLI mTLS-based L3 verification.
func (s *PasskeyService) VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	if userID == "" {
		return false, constants.ErrUserIDRequired
	}
	if transactionHash == "" {
		return false, constants.ErrCLIL3TransactionHashRequired
	}
	if proof == nil {
		return false, constants.ErrGatewayL3ProofRequired
	}
	if proof.CredentialId == "" {
		return false, constants.ErrMissingRequiredField
	}
	if proof.ClientDataJson == "" {
		return false, constants.ErrMissingRequiredField
	}
	if proof.AuthenticatorData == "" {
		return false, constants.ErrMissingRequiredField
	}
	if proof.Signature == "" {
		return false, constants.ErrMissingRequiredField
	}

	user, err := s.getUser(userID)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, constants.ErrUserNotFound
	}
	if len(user.PasskeyCredentials) == 0 {
		return false, constants.ErrNoPasskeysRegistered
	}

	allowedCredentialIDs := make([][]byte, 0, len(user.PasskeyCredentials))
	for _, credential := range user.PasskeyCredentials {
		allowedCredentialIDs = append(allowedCredentialIDs, credential.ID)
	}

	session := webauthn.SessionData{
		Challenge:            encodeChallenge([]byte(transactionHash)),
		RelyingPartyID:       s.rpID,
		UserID:               []byte(userID),
		AllowedCredentialIDs: allowedCredentialIDs,
		Expires:              time.Now().Add(passkeyChallengeTTL),
	}

	assertionResponse := models.ParsedAssertionResponse{
		ID:    proof.CredentialId,
		RawID: proof.CredentialId,
		Type:  "public-key",
		Response: models.ParsedAssertionResponseBody{
			ClientDataJSON:    proof.ClientDataJson,
			AuthenticatorData: proof.AuthenticatorData,
			Signature:         proof.Signature,
		},
	}

	body, err := json.Marshal(assertionResponse)
	if err != nil {
		return false, fmt.Errorf("failed to marshal assertion response: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	parsedResponse, err := protocol.ParseCredentialRequestResponse(req)
	if err != nil {
		return false, fmt.Errorf("failed to parse credential assertion: %w", err)
	}

	_, err = s.webauthn.ValidateLogin(user, session, parsedResponse)
	if err != nil {
		return false, fmt.Errorf("L3 WebAuthn verification failed: %w", err)
	}

	return true, nil
}

func (s *PasskeyService) getUser(userID string) (*models.User, error) {
	return s.userStore.GetUser(userID)
}

func (s *PasskeyService) addCredential(userID string, cred models.PasskeyCredential) error {
	if err := cred.Validate(); err != nil {
		return err
	}
	user, err := s.getUser(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return constants.ErrUserNotFound
	}

	user.PasskeyCredentials = append(user.PasskeyCredentials, cred)

	return s.updateUser(userID, user)
}

func (s *PasskeyService) setCredentials(userID string, creds []models.PasskeyCredential) error {
	user, err := s.getUser(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return constants.ErrUserNotFound
	}

	user.PasskeyCredentials = creds
	return s.updateUser(userID, user)
}

func (s *PasskeyService) updateUser(userID string, user *models.User) error {
	return s.userStore.UpdateUser(userID, user)
}

func (s *PasskeyService) storeWebAuthnSession(userID string, session *webauthn.SessionData) error {
	return s.sessionStore.StoreSession(userID, session)
}

func (s *PasskeyService) getWebAuthnSession(userID string) (*webauthn.SessionData, error) {
	return s.sessionStore.GetSession(userID)
}

func (s *PasskeyService) deleteWebAuthnSession(userID string) error {
	return s.sessionStore.DeleteSession(userID)
}
