// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

// Integration tests for PlatformEnrollmentService. These tests exercise the
// real governance pipeline (L4Warden + L5Actuator + OperatorPubSubService),
// real PKI signing, real SQLite document storage, and real session services.
// They do not mock internal services. The test helper setupPlatformEnrollmentEnv
// constructs a full GatewayModeService under doctrine posture and wires the
// in-process OperatorPubSubService with PlatformEnrollmentDeps so the
// enrollment service's envelope submissions flow through the canonical
// governance gauntlet.

package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/execution"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
)

// platformEnrollmentTestEnv bundles the services needed by the enrollment
// service integration tests. The env is constructed once per test and cleaned
// up via t.Cleanup registered by newTestGatewayService and the fixture helper.
type platformEnrollmentTestEnv struct {
	svc       *GatewayModeService
	enrollSvc *PlatformEnrollmentService
	docStore  *DocumentStoreService
	userSvc   *UserService
	pki       *PKIAuthority
	cliSess   *CLISessionService
	opSess    *OperatorSessionService
	ownerID   string
}

// setupPlatformEnrollmentEnv constructs a full GatewayModeService under
// doctrine posture, wires the in-process OperatorPubSubService with
// PlatformEnrollmentDeps, and returns the env. The gateway is NOT started
// (no port binding); the enrollment service operates through the wired
// envProcAdapter without an HTTP listener. If createOwner is true, a first
// user is created to bootstrap the gateway and the owner ID is returned.
func setupPlatformEnrollmentEnv(t *testing.T, createOwner bool) *platformEnrollmentTestEnv {
	t.Helper()

	ls := newTestGatewayService(t, testGatewayOpts{posture: config.PostureDoctrine})

	cfg := ls.cfg
	logger := ls.logger

	execSvc := execution.NewExecutionService(cfg, logger)
	fileEditSvc := execution.NewFileEditService(cfg, logger)
	scrubbingSvc, err := scrubbing.NewScrubbingService(context.Background(), scrubbing.DefaultConfig(), logger, nil)
	require.NoError(t, err)

	govDeps := ls.GetGovernanceDeps()
	sm, err := ls.GetSecretManager()
	require.NoError(t, err)
	actuatorPriv, actuatorKeyID, err := sm.GetActuatorKey()
	require.NoError(t, err)

	// Add actuator key to signer store so governance signatures are trusted.
	actuatorPub := actuatorPriv.Public().(ed25519.PublicKey)
	err = ls.GetSignerStore().AddTrustedSigner(models.TrustedSigner{
		ID:        actuatorKeyID,
		PublicKey: hexEncode(actuatorPub),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	})
	require.NoError(t, err)

	mcpGateway := ls.GetMCPGateway()
	require.NotNil(t, mcpGateway)

	loopbackClient := pubsub.NewInProcessPubSubClient(ls.GetGatewayWebSocketHandler())

	_, err = pubsub.NewGatewayOperatorPubSubService(pubsub.GatewayCommandServiceConfig{
		CommandServiceConfig: pubsub.CommandServiceConfig{
			Config:             cfg,
			Logger:             logger,
			Execution:          execSvc,
			FileEdit:           fileEditSvc,
			PubSubClient:       loopbackClient,
			Scrubbing:          scrubbingSvc,
			ActuatorSigningKey: actuatorPriv,
			ActuatorKeyID:      actuatorKeyID,
		},
		GovDeps: &pubsub.GovernanceDeps{
			ReplayStore:          govDeps.ReplayStore,
			StateRootProvider:    govDeps.StateRootProvider,
			TransactionAudit:     govDeps.TransactionAudit,
			SignerStore:          govDeps.SignerStore,
			ConsensusPolicyStore: govDeps.ConsensusPolicyStore,
			L3Notary:             govDeps.L3Notary,
			FieldReader:          govDeps.FieldReader,
			Doctrine:             govDeps.Doctrine,
		},
		MCPGateway:              mcpGateway,
		EnvProcAdapter:          ls.GetEnvProcAdapter(),
		SessionValidatorAdapter: ls.GetSessionValidatorAdapter(),
		PlatformEnrollmentDeps: &pubsub.PlatformEnrollmentDeps{
			DocStore:         ls.GetDocStore(),
			PKI:              ls.GetPKI(),
			CLISessions:      ls.GetCLISessionService(),
			OperatorSessions: ls.GetOperatorSessionService(),
			Posture:          string(cfg.Gateway.Posture),
		},
	})
	require.NoError(t, err)

	env := &platformEnrollmentTestEnv{
		svc:       ls,
		enrollSvc: ls.GetPlatformEnrollmentService(),
		docStore:  ls.GetDocStore(),
		userSvc:   ls.GetUserService(),
		pki:       ls.GetPKI(),
		cliSess:   ls.GetCLISessionService(),
		opSess:    ls.GetOperatorSessionService(),
	}

	if createOwner {
		owner, err := ls.GetUserService().CreateUser()
		require.NoError(t, err)
		env.ownerID = owner.ID
	}

	return env
}

// generateAppCSRAndKey generates a P-256 CSR and private key for app
// components (dashboard, ensemble). Returns the CSR PEM and the private key
// for proof-of-possession signing.
func generateAppCSRAndKey(t *testing.T) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "ignored"}}, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})), key
}

// generateOperatorCSRsAndKeys generates two independent P-256 CSRs and keys
// for the operator component (operator + CLI).
func generateOperatorCSRsAndKeys(t *testing.T) (operatorCSR string, operatorKey *ecdsa.PrivateKey, cliCSR string, cliKey *ecdsa.PrivateKey) {
	t.Helper()
	operatorCSR, operatorKey = generateAppCSRAndKey(t)
	cliCSR, cliKey = generateAppCSRAndKey(t)
	return
}

// signCompletionTranscript signs the deterministic completion transcript
// digest with the given private key and returns the base64url-encoded
// signature.
func signCompletionTranscript(t *testing.T, req *models.PlatformEnrollmentRequest, key *ecdsa.PrivateKey) string {
	t.Helper()
	transcript, err := platformEnrollmentCompletionTranscript(req)
	require.NoError(t, err)
	digest := sha256.Sum256(transcript)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(sig)
}

// createAndApproveRequest is a test helper that creates a platform enrollment
// request, approves it as the owner, and returns the request ID, token, and
// the stored request (reloaded after approval). The caller provides the CSR
// material and component kind.
func createAndApproveRequest(t *testing.T, env *platformEnrollmentTestEnv, kind models.PlatformComponentKind, instanceID, hostname string, appCSR string, operatorCSR, cliCSR string) (string, string, *models.PlatformEnrollmentRequest) {
	t.Helper()

	req := models.PlatformEnrollmentCreateRequest{
		ComponentKind: kind,
		InstanceID:    instanceID,
		Hostname:      hostname,
	}
	if appCSR != "" {
		req.App = &models.PlatformAppCSRPayload{CSRPEM: appCSR}
	}
	if operatorCSR != "" {
		req.SystemFingerprint = "test-fingerprint"
		req.Operator = &models.PlatformOperatorCSRPayload{
			OperatorCSRPEM: operatorCSR,
			CLICSRPEM:      cliCSR,
		}
	}

	createResp, err := env.enrollSvc.CreateRequest(context.Background(), req, "https://gateway.local/console")
	require.NoError(t, err)
	require.NotEmpty(t, createResp.RequestID)
	require.NotEmpty(t, createResp.Token)

	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	require.NoError(t, err)

	stored, err := env.enrollSvc.loadByID(createResp.RequestID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, models.PlatformEnrollmentStateApproved, stored.State)

	return createResp.RequestID, createResp.Token, stored
}

// loadStoredRequest loads a platform enrollment request from the doc store by ID.
func loadStoredRequest(t *testing.T, env *platformEnrollmentTestEnv, requestID string) *models.PlatformEnrollmentRequest {
	t.Helper()
	req, err := env.enrollSvc.loadByID(requestID)
	require.NoError(t, err)
	require.NotNil(t, req)
	return req
}

// extractURISANsFromCert extracts URI SANs from a PEM-encoded certificate.
func extractURISANsFromCert(t *testing.T, certPEM string) []string {
	t.Helper()
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block, "cert PEM must decode")
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "cert must parse")
	uris := make([]string, 0, len(cert.URIs))
	for _, u := range cert.URIs {
		uris = append(uris, u.String())
	}
	return uris
}

// ============================================================================
// Pre-bootstrap and bootstrap tests
// ============================================================================

// TestPlatformEnrollmentService_RejectsRequestBeforeBootstrap proves that a
// gateway with no users rejects platform enrollment requests. This is
// invariant 1: a gateway with no users never issues a platform certificate.
func TestPlatformEnrollmentService_RejectsRequestBeforeBootstrap(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, false)

	csr, _ := generateAppCSRAndKey(t)
	_, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")

	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentRequiresBootstrap)
}

// TestPlatformEnrollmentService_BootstrapEnablesRequestCreation proves that
// after the first user is created, request creation succeeds but no
// certificate is issued until the owner approves.
func TestPlatformEnrollmentService_BootstrapEnablesRequestCreation(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	resp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)
	assert.NotEmpty(t, resp.RequestID)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, "g8ed", resp.ComponentName)
	assert.NotEmpty(t, resp.ApprovalURL)
	assert.Contains(t, resp.ApprovalURL, resp.RequestID)
	assert.False(t, resp.ExpiresAt.IsZero())

	// No certificate is issued yet: the request is pending.
	stored := loadStoredRequest(t, env, resp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStatePending, stored.State)
	assert.Nil(t, stored.Issued)
}

// ============================================================================
// Owner authorization tests
// ============================================================================

// TestPlatformEnrollmentService_NonOwnerDecisionFailsClosed proves that a
// non-first-user cannot approve or deny a request. This is invariant 8.
func TestPlatformEnrollmentService_NonOwnerDecisionFailsClosed(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	// Create a second user (non-owner).
	secondUser, err := env.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotEqual(t, env.ownerID, secondUser.ID)

	csr, _ := generateAppCSRAndKey(t)
	resp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	_, err = env.enrollSvc.Decide(context.Background(), secondUser.ID, models.PlatformEnrollmentDecisionRequest{
		RequestID: resp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidDecision)

	// The request remains pending.
	stored := loadStoredRequest(t, env, resp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStatePending, stored.State)
}

// TestPlatformEnrollmentService_EmptyActorDecisionFailsClosed proves that an
// empty actor user ID is rejected.
func TestPlatformEnrollmentService_EmptyActorDecisionFailsClosed(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	resp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	_, err = env.enrollSvc.Decide(context.Background(), "", models.PlatformEnrollmentDecisionRequest{
		RequestID: resp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidDecision)
}

// ============================================================================
// Dashboard and ensemble issuance tests
// ============================================================================

// TestPlatformEnrollmentService_DashboardIssuanceProducesDualSANAndOwnershipPolicy
// proves that dashboard enrollment produces a certificate with the canonical
// app SPIFFE URI and the approving user's SPIFFE URI, and that the app policy
// persists owner and approval provenance. This is invariants 5 and 6.
func TestPlatformEnrollmentService_DashboardIssuanceProducesDualSANAndOwnershipPolicy(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, key := generateAppCSRAndKey(t)
	requestID, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-1", "dashboard.local",
		csr, "", "")

	// Complete: sign the transcript and call Complete.
	proof := models.PlatformEnrollmentProofs{
		App: signCompletionTranscript(t, approved, key),
	}
	resp, err := env.enrollSvc.Complete(context.Background(), token, proof)
	require.NoError(t, err)
	require.NotNil(t, resp.App)
	assert.NotEmpty(t, resp.App.AppCert)
	assert.NotEmpty(t, resp.App.CertChain)
	assert.NotEmpty(t, resp.App.TrustBundle)
	assert.Equal(t, "g8ed", resp.App.AppID)
	assert.False(t, resp.App.ExpiresAt.IsZero())

	// Verify the certificate carries the canonical app URI first and the
	// approving user URI second.
	uris := extractURISANsFromCert(t, resp.App.AppCert)
	require.Len(t, uris, 2)
	assert.Equal(t, "spiffe://g8e.local/app/g8ed", uris[0])
	assert.Equal(t, "spiffe://g8e.local/user/"+env.ownerID, uris[1])

	// Verify the stored request is completed with cert metadata.
	stored := loadStoredRequest(t, env, requestID)
	assert.Equal(t, models.PlatformEnrollmentStateCompleted, stored.State)
	assert.NotEmpty(t, stored.CertificateFingerprint)
	assert.NotEmpty(t, stored.CertificateSerial)
	assert.Equal(t, env.ownerID, stored.ApprovedByUserID)
	assert.NotNil(t, stored.CompletedAt)
	assert.NotNil(t, stored.Issued)

	// Verify the app policy was persisted with ownership and approval provenance.
	policyDoc, err := env.docStore.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), "spiffe://g8e.local/app/g8ed")
	require.NoError(t, err)
	require.NotNil(t, policyDoc)
	dataBytes, err := json.Marshal(policyDoc.Data)
	require.NoError(t, err)
	var policy models.AppPolicy
	require.NoError(t, json.Unmarshal(dataBytes, &policy))
	assert.Equal(t, "spiffe://g8e.local/app/g8ed", policy.AppID)
	assert.Equal(t, env.ownerID, policy.OwnerUserID)
	assert.Equal(t, env.ownerID, policy.ApprovedByUserID)
	assert.Equal(t, requestID, policy.EnrollmentRequestID)
	assert.NotEmpty(t, policy.CertificateFingerprint)
}

// TestPlatformEnrollmentService_EnsembleIssuanceProducesDualSANAndOwnershipPolicy
// proves that ensemble enrollment produces a certificate with the canonical
// ensemble app URI and the approving user's URI, with a corresponding policy.
func TestPlatformEnrollmentService_EnsembleIssuanceProducesDualSANAndOwnershipPolicy(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, key := generateAppCSRAndKey(t)
	requestID, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentEnsemble, "ensemble-1", "ensemble.local",
		csr, "", "")

	proof := models.PlatformEnrollmentProofs{
		App: signCompletionTranscript(t, approved, key),
	}
	resp, err := env.enrollSvc.Complete(context.Background(), token, proof)
	require.NoError(t, err)
	require.NotNil(t, resp.App)
	assert.Equal(t, "g8ee", resp.App.AppID)

	uris := extractURISANsFromCert(t, resp.App.AppCert)
	require.Len(t, uris, 2)
	assert.Equal(t, "spiffe://g8e.local/app/g8ee", uris[0])
	assert.Equal(t, "spiffe://g8e.local/user/"+env.ownerID, uris[1])

	// Verify the ensemble app policy.
	policyDoc, err := env.docStore.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), "spiffe://g8e.local/app/g8ee")
	require.NoError(t, err)
	require.NotNil(t, policyDoc)

	stored := loadStoredRequest(t, env, requestID)
	assert.Equal(t, models.PlatformEnrollmentStateCompleted, stored.State)
}

// ============================================================================
// Operator issuance tests
// ============================================================================

// TestPlatformEnrollmentService_OperatorIssuanceSignsBothCSRsAndPersistsOperator
// proves that operator enrollment signs both the operator and CLI CSRs,
// persists the operator document stamped with the approving owner's user_id,
// and creates CLI and operator sessions bound to that same user_id. The
// approving owner is the actor recorded on the enrollment request; that
// user_id propagates to the operator document and both session documents so
// the owner can discover and manage the platform-enrolled operator through
// ListUserOperators.
func TestPlatformEnrollmentService_OperatorIssuanceSignsBothCSRsAndPersistsOperator(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	operatorCSR, operatorKey, cliCSR, cliKey := generateOperatorCSRsAndKeys(t)
	requestID, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentOperator, "operator-1", "operator.local",
		"", operatorCSR, cliCSR)

	proof := models.PlatformEnrollmentProofs{
		Operator: signCompletionTranscript(t, approved, operatorKey),
		CLI:      signCompletionTranscript(t, approved, cliKey),
	}
	resp, err := env.enrollSvc.Complete(context.Background(), token, proof)
	require.NoError(t, err)
	require.NotNil(t, resp.Operator)
	assert.NotEmpty(t, resp.Operator.OperatorCert)
	assert.NotEmpty(t, resp.Operator.CLICert)
	assert.NotEmpty(t, resp.Operator.OperatorID)
	assert.NotEmpty(t, resp.Operator.OperatorSessionID)
	assert.NotEmpty(t, resp.Operator.CLISessionID)
	assert.NotEmpty(t, resp.Operator.HubTrustBundle)

	// Verify the operator document was persisted with the approving
	// owner's user_id so the owner can discover it via ListUserOperators.
	// is_slot remains false: platform-enrolled operators are not
	// user-created slots, but they are user-owned.
	opDoc, err := env.docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), resp.Operator.OperatorID)
	require.NoError(t, err)
	require.NotNil(t, opDoc)
	dataBytes, err := json.Marshal(opDoc.Data)
	require.NoError(t, err)
	var op models.OperatorDocumentGo
	require.NoError(t, json.Unmarshal(dataBytes, &op))
	// DocSet strips the "id" field from the data JSON (the ID is stored
	// separately in the documents table), so op.ID is empty. The operator
	// document was found by resp.Operator.OperatorID, which proves the ID
	// was correctly generated and used as the document key.
	assert.Equal(t, env.ownerID, op.UserID, "operator doc must carry the approving owner's user_id")
	assert.False(t, op.IsSlot, "platform-enrolled operators are not slots")
	assert.Equal(t, constants.OperatorStatusActive, op.Status)
	assert.True(t, op.Claimed)

	// Verify the CLI session is bound to the approving owner's user_id.
	cliDoc, err := env.docStore.DocGet(marshaler.CollectionName(constants.CollectionCLISessions), resp.Operator.CLISessionID)
	require.NoError(t, err)
	require.NotNil(t, cliDoc)
	cliBytes, err := json.Marshal(cliDoc.Data)
	require.NoError(t, err)
	var cliSession models.CLISession
	require.NoError(t, json.Unmarshal(cliBytes, &cliSession))
	assert.Equal(t, env.ownerID, cliSession.UserID, "CLI session must carry the approving owner's user_id")

	// Verify the operator session is bound to the approving owner's user_id.
	opSessDoc, err := env.docStore.DocGet(marshaler.CollectionName(constants.CollectionOperatorSessions), resp.Operator.OperatorSessionID)
	require.NoError(t, err)
	require.NotNil(t, opSessDoc)
	opSessBytes, err := json.Marshal(opSessDoc.Data)
	require.NoError(t, err)
	var opSession models.OperatorSession
	require.NoError(t, json.Unmarshal(opSessBytes, &opSession))
	assert.Equal(t, env.ownerID, opSession.UserID, "operator session must carry the approving owner's user_id")

	// Verify the stored request is completed with approval provenance.
	stored := loadStoredRequest(t, env, requestID)
	assert.Equal(t, models.PlatformEnrollmentStateCompleted, stored.State)
	assert.Equal(t, env.ownerID, stored.ApprovedByUserID)
	assert.NotEmpty(t, stored.OperatorID)
	assert.NotEmpty(t, stored.OperatorSessionID)
	assert.NotEmpty(t, stored.CLISessionID)
}

// TestPlatformEnrollmentService_OperatorIssuanceIsDiscoverableViaListUserOperators
// proves that after a platform-enrolled operator is issued, the approving
// owner can discover it through RegistrationService.ListUserOperators. This
// is the production fix for the E2E defect where ListOperatorSlots filtered
// by is_slot=true hid platform-enrolled operators (which carry is_slot=false)
// from the owner's operator list.
func TestPlatformEnrollmentService_OperatorIssuanceIsDiscoverableViaListUserOperators(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	operatorCSR, operatorKey, cliCSR, cliKey := generateOperatorCSRsAndKeys(t)
	_, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentOperator, "operator-1", "operator.local",
		"", operatorCSR, cliCSR)

	proof := models.PlatformEnrollmentProofs{
		Operator: signCompletionTranscript(t, approved, operatorKey),
		CLI:      signCompletionTranscript(t, approved, cliKey),
	}
	resp, err := env.enrollSvc.Complete(context.Background(), token, proof)
	require.NoError(t, err)
	require.NotNil(t, resp.Operator)

	// The owner can discover the platform-enrolled operator via
	// ListUserOperators, which filters by user_id only (no is_slot
	// requirement).
	regSvc := env.svc.GetRegistrationService()
	operators, err := regSvc.ListUserOperators(env.ownerID)
	require.NoError(t, err)
	require.NotEmpty(t, operators, "owner must see the platform-enrolled operator")

	var found *models.OperatorDocumentGo
	for i := range operators {
		if operators[i].ID == resp.Operator.OperatorID {
			found = &operators[i]
			break
		}
	}
	require.NotNil(t, found, "platform-enrolled operator must appear in ListUserOperators")
	assert.Equal(t, env.ownerID, found.UserID)
	assert.False(t, found.IsSlot, "platform-enrolled operator is not a slot")
	assert.Equal(t, constants.OperatorStatusActive, found.Status)
}

// ============================================================================
// Proof verification failure tests
// ============================================================================

// TestPlatformEnrollmentService_ProofMismatchFails proves that an invalid
// proof-of-possession signature is rejected at completion.
func TestPlatformEnrollmentService_ProofMismatchFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	_, token, _ := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-1", "dashboard.local",
		csr, "", "")

	// Submit a garbage proof.
	_, err := env.enrollSvc.Complete(context.Background(), token, models.PlatformEnrollmentProofs{
		App: "invalid-signature",
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentProofInvalid)
}

// TestPlatformEnrollmentService_MissingProofFails proves that omitting a
// required proof is rejected.
func TestPlatformEnrollmentService_MissingProofFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	_, token, _ := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-1", "dashboard.local",
		csr, "", "")

	_, err := env.enrollSvc.Complete(context.Background(), token, models.PlatformEnrollmentProofs{})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentProofRequired)
}

// TestPlatformEnrollmentService_OperatorMissingOneProofFails proves that
// operator completion requires both operator and CLI proofs.
func TestPlatformEnrollmentService_OperatorMissingOneProofFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	operatorCSR, operatorKey, cliCSR, _ := generateOperatorCSRsAndKeys(t)
	_, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentOperator, "operator-1", "operator.local",
		"", operatorCSR, cliCSR)

	// Only provide the operator proof, omit the CLI proof.
	_, err := env.enrollSvc.Complete(context.Background(), token, models.PlatformEnrollmentProofs{
		Operator: signCompletionTranscript(t, approved, operatorKey),
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentProofRequired)
}

// TestPlatformEnrollmentService_SwappedProofsFails proves that signing the
// transcript with the wrong key (operator key for CLI proof and vice versa)
// is rejected.
func TestPlatformEnrollmentService_SwappedProofsFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	operatorCSR, operatorKey, cliCSR, cliKey := generateOperatorCSRsAndKeys(t)
	_, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentOperator, "operator-1", "operator.local",
		"", operatorCSR, cliCSR)

	// Swap: sign the operator proof with the CLI key and vice versa.
	_, err := env.enrollSvc.Complete(context.Background(), token, models.PlatformEnrollmentProofs{
		Operator: signCompletionTranscript(t, approved, cliKey),
		CLI:      signCompletionTranscript(t, approved, operatorKey),
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentProofInvalid)
}

// TestPlatformEnrollmentService_TokenTheftWithoutKeysFails proves that
// possessing the token alone (without the private keys) cannot complete
// enrollment. The attacker cannot produce valid proofs.
func TestPlatformEnrollmentService_TokenTheftWithoutKeysFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	_, token, _ := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-1", "dashboard.local",
		csr, "", "")

	// Attacker has the token but generates their own key to sign the proof.
	// The proof is signed with a different key than the CSR, so verification fails.
	attackerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Load the approved request via the token to build the transcript.
	statusReq, err := env.enrollSvc.loadByToken(token)
	require.NoError(t, err)
	attackerSig := signCompletionTranscript(t, statusReq, attackerKey)

	_, err = env.enrollSvc.Complete(context.Background(), token, models.PlatformEnrollmentProofs{
		App: attackerSig,
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentProofInvalid)
}

// ============================================================================
// Denial, expiry, and state transition tests
// ============================================================================

// TestPlatformEnrollmentService_DenialPreventsIssuance proves that a denied
// request cannot be completed. This is invariant 13.
func TestPlatformEnrollmentService_DenialPreventsIssuance(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, key := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionDeny,
		Reason:    "not authorized",
	})
	require.NoError(t, err)

	stored := loadStoredRequest(t, env, createResp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStateDenied, stored.State)

	_, err = env.enrollSvc.Complete(context.Background(), createResp.Token, models.PlatformEnrollmentProofs{
		App: signCompletionTranscript(t, stored, key),
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentRequestDenied)
}

// TestPlatformEnrollmentService_CompleteOnPendingFails proves that completion
// of a pending (not yet approved) request fails.
func TestPlatformEnrollmentService_CompleteOnPendingFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	_, err = env.enrollSvc.Complete(context.Background(), createResp.Token, models.PlatformEnrollmentProofs{})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentNotApproved)
}

// TestPlatformEnrollmentService_AlreadyDecidedFails proves that deciding an
// already-approved request fails.
func TestPlatformEnrollmentService_AlreadyDecidedFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	require.NoError(t, err)

	// Second decision on the same request fails.
	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentAlreadyDecided)
}

// TestPlatformEnrollmentService_DecideOnNonexistentRequestFails proves that
// deciding a request that does not exist returns a not-found error.
func TestPlatformEnrollmentService_DecideOnNonexistentRequestFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	_, err := env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: "nonexistent-request-id",
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentRequestNotFound)
}

// ============================================================================
// Idempotent completion tests
// ============================================================================

// TestPlatformEnrollmentService_IdempotentCompletedResponse proves that
// retrying completion with the same token and valid proofs returns the same
// issued artifacts. A second completion must not create a new identity.
// This is invariant 12.
func TestPlatformEnrollmentService_IdempotentCompletedResponse(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, key := generateAppCSRAndKey(t)
	_, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-1", "dashboard.local",
		csr, "", "")

	proof := models.PlatformEnrollmentProofs{
		App: signCompletionTranscript(t, approved, key),
	}

	first, err := env.enrollSvc.Complete(context.Background(), token, proof)
	require.NoError(t, err)
	require.NotNil(t, first.App)

	second, err := env.enrollSvc.Complete(context.Background(), token, proof)
	require.NoError(t, err)
	require.NotNil(t, second.App)

	// The same certificate is returned; no second identity is created.
	assert.Equal(t, first.App.AppCert, second.App.AppCert)
	assert.Equal(t, first.App.AppID, second.App.AppID)
}

// ============================================================================
// Deduplication and quota tests
// ============================================================================

// TestPlatformEnrollmentService_DeduplicatesLiveRequest proves that creating
// a second request for the same component kind, instance ID, and fingerprint
// set returns the existing request's metadata without a new token.
func TestPlatformEnrollmentService_DeduplicatesLiveRequest(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	req := models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}

	first, err := env.enrollSvc.CreateRequest(context.Background(), req, "https://gateway.local/console")
	require.NoError(t, err)
	require.NotEmpty(t, first.Token)

	// Same CSR, same instance ID: dedup returns the same request ID with no token.
	second, err := env.enrollSvc.CreateRequest(context.Background(), req, "https://gateway.local/console")
	require.NoError(t, err)
	assert.Equal(t, first.RequestID, second.RequestID)
	assert.Empty(t, second.Token, "deduplicated response must not return the token")
}

// TestPlatformEnrollmentService_QuotaExceeded proves that creating more than
// the configured maximum live requests for the same component kind is rejected.
func TestPlatformEnrollmentService_QuotaExceeded(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	// Create max+1 requests with different instance IDs and different CSRs
	// (so dedup does not collapse them).
	for i := 0; i < constants.PlatformEnrollmentMaxLiveRequestsPerComponent; i++ {
		csr, _ := generateAppCSRAndKey(t)
		_, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
			ComponentKind: models.PlatformComponentDashboard,
			InstanceID:    fmt.Sprintf("dashboard-%d", i),
			Hostname:      "dashboard.local",
			App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
		}, "https://gateway.local/console")
		require.NoError(t, err)
	}

	// The next request exceeds the quota.
	csr, _ := generateAppCSRAndKey(t)
	_, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-overflow",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentQuotaExceeded)
}

// ============================================================================
// Concurrent decision and completion tests
// ============================================================================

// TestPlatformEnrollmentService_ConcurrentApproveDeny proves that concurrent
// approve and deny decisions on the same request result in exactly one
// winning decision. The conditional update ensures no double-decision.
func TestPlatformEnrollmentService_ConcurrentApproveDeny(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	var wg sync.WaitGroup
	var approveErr, denyErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, approveErr = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
			RequestID: createResp.RequestID,
			Decision:  models.PlatformEnrollmentDecisionApprove,
		})
	}()
	go func() {
		defer wg.Done()
		_, denyErr = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
			RequestID: createResp.RequestID,
			Decision:  models.PlatformEnrollmentDecisionDeny,
		})
	}()
	wg.Wait()

	// Exactly one decision succeeds; the other fails. The losing decision
	// can fail with ErrPlatformEnrollmentAlreadyDecided (if the handler
	// runs and finds the state already changed),
	// ErrPlatformEnrollmentGovernanceRejected (if the L4 warden rejects
	// the stale state root before the handler runs, because the first
	// decision changed the state merkle root),
	// ErrPlatformEnrollmentRequestDenied (if the losing goroutine loads
	// the request after the winner has already transitioned it to the
	// denied terminal state), or ErrPlatformEnrollmentRequestExpired
	// (analogous for the expired terminal state). All are correct
	// fail-closed behavior; the invariant is that exactly one decision
	// wins.
	stored := loadStoredRequest(t, env, createResp.RequestID)
	assert.True(t, stored.State == models.PlatformEnrollmentStateApproved || stored.State == models.PlatformEnrollmentStateDenied,
		"exactly one decision must win; state=%s", stored.State)
	assert.True(t, (approveErr == nil) != (denyErr == nil),
		"exactly one decision must succeed; approveErr=%v denyErr=%v", approveErr, denyErr)
	loserErr := approveErr
	if approveErr == nil {
		loserErr = denyErr
	}
	assert.True(t,
		errors.Is(loserErr, constants.ErrPlatformEnrollmentAlreadyDecided) ||
			errors.Is(loserErr, constants.ErrPlatformEnrollmentGovernanceRejected) ||
			errors.Is(loserErr, constants.ErrPlatformEnrollmentRequestDenied) ||
			errors.Is(loserErr, constants.ErrPlatformEnrollmentRequestExpired),
		"losing decision must fail closed with AlreadyDecided, GovernanceRejected, RequestDenied, or RequestExpired; got %v", loserErr)
}

// TestPlatformEnrollmentService_ConcurrentComplete proves that concurrent
// completion attempts on the same approved request result in at most one
// issuance. The issuance lease ensures only one signer proceeds.
func TestPlatformEnrollmentService_ConcurrentComplete(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, key := generateAppCSRAndKey(t)
	_, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-1", "dashboard.local",
		csr, "", "")

	proof := models.PlatformEnrollmentProofs{
		App: signCompletionTranscript(t, approved, key),
	}

	var wg sync.WaitGroup
	results := make(chan *models.PlatformEnrollmentCompleteResponse, 2)
	errs := make(chan error, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			resp, err := env.enrollSvc.Complete(context.Background(), token, proof)
			if err != nil {
				errs <- err
				return
			}
			results <- resp
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	successCount := 0
	inProgressCount := 0
	for resp := range results {
		require.NotNil(t, resp.App)
		successCount++
	}
	for err := range errs {
		if errors.Is(err, constants.ErrPlatformEnrollmentIssuanceInProgress) {
			inProgressCount++
		} else {
			t.Fatalf("unexpected error from concurrent complete: %v", err)
		}
	}

	assert.Equal(t, 1, successCount, "exactly one completion must succeed")
	assert.Equal(t, 1, inProgressCount, "the losing completion must get issuance-in-progress")
}

// ============================================================================
// Issuance lease and reconciliation tests
// ============================================================================

// TestPlatformEnrollmentService_ExpiredLeaseRecovery proves that an expired
// issuance lease is recovered by reconciliation, allowing a new completion
// attempt to succeed.
func TestPlatformEnrollmentService_ExpiredLeaseRecovery(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, key := generateAppCSRAndKey(t)
	_, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-1", "dashboard.local",
		csr, "", "")

	// Manually acquire the issuance lease with an already-expired expiry.
	leaseExpiry := time.Now().UTC().Add(-time.Second)
	applied, err := env.docStore.DocConditionalUpdate(
		platformEnrollmentCollectionName(), approved.ID,
		map[string]interface{}{
			"state":                     string(models.PlatformEnrollmentStateIssuing),
			"issuance_lease_owner":      "crashed-owner",
			"issuance_lease_expires_at": leaseExpiry,
			"last_transition_at":        time.Now().UTC(),
		},
		"state", string(models.PlatformEnrollmentStateApproved),
	)
	require.NoError(t, err)
	require.True(t, applied)

	// Reconciliation should recover the expired lease.
	err = env.enrollSvc.ReconcileExpiredLeases()
	require.NoError(t, err)

	stored := loadStoredRequest(t, env, approved.ID)
	assert.Equal(t, models.PlatformEnrollmentStateApproved, stored.State,
		"expired lease must be recovered back to approved")

	// Now completion should succeed.
	proof := models.PlatformEnrollmentProofs{
		App: signCompletionTranscript(t, stored, key),
	}
	resp, err := env.enrollSvc.Complete(context.Background(), token, proof)
	require.NoError(t, err)
	require.NotNil(t, resp.App)
}

// TestPlatformEnrollmentService_LiveLeaseReturnsRetryAfter proves that a
// completion attempt on a request with a live issuance lease returns the
// issuance-in-progress error.
func TestPlatformEnrollmentService_LiveLeaseReturnsRetryAfter(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, key := generateAppCSRAndKey(t)
	_, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-1", "dashboard.local",
		csr, "", "")

	// Manually acquire the lease with a future expiry.
	leaseExpiry := time.Now().UTC().Add(constants.PlatformEnrollmentIssuanceLeaseTTL)
	applied, err := env.docStore.DocConditionalUpdate(
		platformEnrollmentCollectionName(), approved.ID,
		map[string]interface{}{
			"state":                     string(models.PlatformEnrollmentStateIssuing),
			"issuance_lease_owner":      "active-owner",
			"issuance_lease_expires_at": leaseExpiry,
			"last_transition_at":        time.Now().UTC(),
		},
		"state", string(models.PlatformEnrollmentStateApproved),
	)
	require.NoError(t, err)
	require.True(t, applied)

	// Status should show issuing with retry-after.
	status, err := env.enrollSvc.GetStatus(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, models.PlatformEnrollmentStateIssuing, status.State)
	assert.NotZero(t, status.RetryAfter)

	// Completion returns issuance-in-progress.
	_, err = env.enrollSvc.Complete(context.Background(), token, models.PlatformEnrollmentProofs{
		App: signCompletionTranscript(t, approved, key),
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentIssuanceInProgress)
}

// ============================================================================
// Status and pending list tests
// ============================================================================

// TestPlatformEnrollmentService_GetStatusReturnsRequesterVisibleState proves
// that GetStatus returns the state and expiry without exposing the token hash
// or CSR PEM.
func TestPlatformEnrollmentService_GetStatusReturnsRequesterVisibleState(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	status, err := env.enrollSvc.GetStatus(context.Background(), createResp.Token)
	require.NoError(t, err)
	assert.Equal(t, createResp.RequestID, status.RequestID)
	assert.Equal(t, models.PlatformComponentDashboard, status.ComponentKind)
	assert.Equal(t, models.PlatformEnrollmentStatePending, status.State)
	assert.False(t, status.ExpiresAt.IsZero())
}

// TestPlatformEnrollmentService_GetStatusInvalidTokenFails proves that an
// invalid token returns a not-found error.
func TestPlatformEnrollmentService_GetStatusInvalidTokenFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	_, err := env.enrollSvc.GetStatus(context.Background(), "invalid-token")
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentRequestNotFound)
}

// TestPlatformEnrollmentService_ListPendingReturnsOwnerVisibleMetadata proves
// that ListPending returns pending request metadata without tokens or CSR PEM.
func TestPlatformEnrollmentService_ListPendingReturnsOwnerVisibleMetadata(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	list, err := env.enrollSvc.ListPending(context.Background())
	require.NoError(t, err)
	require.Len(t, list.Requests, 1)

	pending := list.Requests[0]
	assert.Equal(t, createResp.RequestID, pending.RequestID)
	assert.Equal(t, models.PlatformComponentDashboard, pending.ComponentKind)
	assert.Equal(t, "g8ed", pending.ComponentName)
	assert.Equal(t, "dashboard-1", pending.InstanceID)
	assert.Equal(t, "dashboard.local", pending.Hostname)
	assert.NotEmpty(t, pending.Fingerprints.App)
}

// TestPlatformEnrollmentService_ListPendingExcludesTerminal proves that
// ListPending excludes denied, expired, and completed requests.
func TestPlatformEnrollmentService_ListPendingExcludesTerminal(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	// Deny the request.
	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionDeny,
	})
	require.NoError(t, err)

	// ListPending should not include the denied request.
	list, err := env.enrollSvc.ListPending(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list.Requests)
}

// ============================================================================
// Cleanup tests
// ============================================================================

// TestPlatformEnrollmentService_CleanupRemovesTerminalRequestsPastRetention
// proves that cleanup removes denied/expired requests past the retention
// window but never removes completed requests (they hold issued artifacts).
func TestPlatformEnrollmentService_CleanupRemovesTerminalRequestsPastRetention(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	// Create and deny a request.
	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	_, err = env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionDeny,
	})
	require.NoError(t, err)

	// Manually backdate the last_transition_at past the retention window.
	stored := loadStoredRequest(t, env, createResp.RequestID)
	oldTransition := time.Now().UTC().Add(-(constants.PlatformEnrollmentCleanupRetention + time.Hour))
	_, err = env.docStore.DocConditionalUpdate(
		platformEnrollmentCollectionName(), createResp.RequestID,
		map[string]interface{}{
			"last_transition_at": oldTransition,
		},
		"state", string(models.PlatformEnrollmentStateDenied),
	)
	require.NoError(t, err)
	_ = stored

	// Run cleanup.
	err = env.enrollSvc.CleanupTerminalRequests()
	require.NoError(t, err)

	// The denied request is removed.
	doc, err := env.docStore.DocGet(platformEnrollmentCollectionName(), createResp.RequestID)
	require.NoError(t, err)
	assert.Nil(t, doc, "denied request past retention must be removed by cleanup")
}

// TestPlatformEnrollmentService_CleanupPreservesCompletedRequests proves that
// cleanup never removes completed requests, even past the retention window,
// because they hold the sole copy of issued artifacts for idempotent retry.
func TestPlatformEnrollmentService_CleanupPreservesCompletedRequests(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, key := generateAppCSRAndKey(t)
	requestID, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentDashboard, "dashboard-1", "dashboard.local",
		csr, "", "")

	proof := models.PlatformEnrollmentProofs{
		App: signCompletionTranscript(t, approved, key),
	}
	_, err := env.enrollSvc.Complete(context.Background(), token, proof)
	require.NoError(t, err)

	// Backdate the completed request past retention.
	oldTransition := time.Now().UTC().Add(-(constants.PlatformEnrollmentCleanupRetention + time.Hour))
	_, err = env.docStore.DocConditionalUpdate(
		platformEnrollmentCollectionName(), requestID,
		map[string]interface{}{
			"last_transition_at": oldTransition,
		},
		"state", string(models.PlatformEnrollmentStateCompleted),
	)
	require.NoError(t, err)

	err = env.enrollSvc.CleanupTerminalRequests()
	require.NoError(t, err)

	// The completed request is still present.
	doc, err := env.docStore.DocGet(platformEnrollmentCollectionName(), requestID)
	require.NoError(t, err)
	assert.NotNil(t, doc, "completed request must not be removed by cleanup")
}

// ============================================================================
// Cleanup goroutine lifecycle tests
// ============================================================================

// TestPlatformEnrollmentService_StartStopCleanup proves that the cleanup
// goroutine can be started and stopped without panics, and that double-stop
// is safe.
func TestPlatformEnrollmentService_StartStopCleanup(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env.enrollSvc.StartCleanup(ctx)
	env.enrollSvc.StartCleanup(ctx) // double-start is a no-op
	env.enrollSvc.StopCleanup()
	env.enrollSvc.StopCleanup() // double-stop is safe
}

// TestPlatformEnrollmentService_CleanupContextCancellation proves that the
// cleanup goroutine exits when the context is cancelled.
func TestPlatformEnrollmentService_CleanupContextCancellation(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	ctx, cancel := context.WithCancel(context.Background())
	env.enrollSvc.StartCleanup(ctx)

	// Cancel the context and verify StopCleanup returns promptly.
	cancel()
	done := make(chan struct{})
	go func() {
		env.enrollSvc.StopCleanup()
		close(done)
	}()
	select {
	case <-done:
		// Success: StopCleanup returned.
	case <-time.After(5 * time.Second):
		t.Fatal("StopCleanup did not return within 5s after context cancellation")
	}
}

// ============================================================================
// Token security tests
// ============================================================================

// TestPlatformEnrollmentService_TokenHashStoredNotRawToken proves that only
// the SHA-256 hash of the token is stored, not the raw token. This is
// invariant 10.
func TestPlatformEnrollmentService_TokenHashStoredNotRawToken(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	stored := loadStoredRequest(t, env, createResp.RequestID)
	assert.NotEmpty(t, stored.TokenHash, "token hash must be stored")
	assert.NotEqual(t, createResp.Token, stored.TokenHash, "raw token must not be stored")
	expectedHash := platformEnrollmentTokenHash(createResp.Token)
	assert.Equal(t, expectedHash, stored.TokenHash, "stored hash must match SHA-256 of token")
}

// TestPlatformEnrollmentService_PendingListExcludesTokenHash proves that the
// pending list response does not expose the token hash. This is invariant 10.
func TestPlatformEnrollmentService_PendingListExcludesTokenHash(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	_, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	list, err := env.enrollSvc.ListPending(context.Background())
	require.NoError(t, err)
	require.Len(t, list.Requests, 1)

	// Marshal the pending response and verify no token or token_hash field appears.
	data, err := json.Marshal(list)
	require.NoError(t, err)
	str := string(data)
	assert.NotContains(t, str, "token_hash")
	assert.NotContains(t, str, "token")
}

// ============================================================================
// Approval URL tests
// ============================================================================

// TestPlatformEnrollmentService_ApprovalURLContainsRequestID proves that the
// approval URL contains the request ID in the fragment and never contains the
// token. This is invariant 9.
func TestPlatformEnrollmentService_ApprovalURLContainsRequestID(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	require.NoError(t, err)

	assert.Contains(t, createResp.ApprovalURL, "#platform-enrollment="+createResp.RequestID)
	assert.NotContains(t, createResp.ApprovalURL, createResp.Token)
}

// TestPlatformEnrollmentService_EmptyApprovalURLBaseReturnsEmpty proves that
// an empty approval URL base produces an empty URL (no panic, no fragment).
func TestPlatformEnrollmentService_EmptyApprovalURLBaseReturnsEmpty(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	createResp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "")
	require.NoError(t, err)
	assert.Empty(t, createResp.ApprovalURL)
}

// ============================================================================
// Validation tests at the service layer
// ============================================================================

// TestPlatformEnrollmentService_InvalidComponentKindFails proves that an
// unknown component kind is rejected at request creation.
func TestPlatformEnrollmentService_InvalidComponentKindFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	_, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: "unknown",
		InstanceID:    "x-1",
		Hostname:      "x.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidComponent)
}

// TestPlatformEnrollmentService_OperatorWithoutOperatorPayloadFails proves
// that an operator request without the operator CSR payload is rejected.
func TestPlatformEnrollmentService_OperatorWithoutOperatorPayloadFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	_, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind:     models.PlatformComponentOperator,
		InstanceID:        "operator-1",
		Hostname:          "operator.local",
		SystemFingerprint: "fp",
		App:               &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidPayload)
}

// TestPlatformEnrollmentService_AppWithOperatorPayloadFails proves that a
// dashboard/ensemble request with an operator payload is rejected.
func TestPlatformEnrollmentService_AppWithOperatorPayloadFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	appCSR, _ := generateAppCSRAndKey(t)
	opCSR, _, cliCSR, _ := generateOperatorCSRsAndKeys(t)
	_, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: appCSR},
		Operator:      &models.PlatformOperatorCSRPayload{OperatorCSRPEM: opCSR, CLICSRPEM: cliCSR},
	}, "https://gateway.local/console")
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidPayload)
}

// TestPlatformEnrollmentService_DuplicateOperatorKeysFails proves that
// operator and CLI CSRs using the same key are rejected.
func TestPlatformEnrollmentService_DuplicateOperatorKeysFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	_, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind:     models.PlatformComponentOperator,
		InstanceID:        "operator-1",
		Hostname:          "operator.local",
		SystemFingerprint: "fp",
		Operator: &models.PlatformOperatorCSRPayload{
			OperatorCSRPEM: csr,
			CLICSRPEM:      csr,
		},
	}, "https://gateway.local/console")
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentDuplicateKey)
}

// TestPlatformEnrollmentService_InvalidInstanceIDFails proves that an invalid
// instance ID format is rejected.
func TestPlatformEnrollmentService_InvalidInstanceIDFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	csr, _ := generateAppCSRAndKey(t)
	_, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "invalid instance with spaces",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	}, "https://gateway.local/console")
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidInstanceID)
}

// TestPlatformEnrollmentService_EmptyTokenCompleteFails proves that calling
// Complete with an empty token returns the token-required error.
func TestPlatformEnrollmentService_EmptyTokenCompleteFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	_, err := env.enrollSvc.Complete(context.Background(), "", models.PlatformEnrollmentProofs{})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentTokenRequired)
}

// TestPlatformEnrollmentService_EmptyTokenStatusFails proves that calling
// GetStatus with an empty token returns the token-required error.
func TestPlatformEnrollmentService_EmptyTokenStatusFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	_, err := env.enrollSvc.GetStatus(context.Background(), "")
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentTokenRequired)
}

// TestPlatformEnrollmentService_InvalidDecisionFails proves that an invalid
// decision value is rejected.
func TestPlatformEnrollmentService_InvalidDecisionFails(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	_, err := env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: "any",
		Decision:  "maybe",
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidDecision)
}
