// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

// Cross-gateway enrollment integration tests. A gateway process (which
// includes an in-process operator substrate) can enroll as an operator of
// another gateway. The platform enrollment protocol has no requester-identity
// screening — the request endpoint is RouteAuthNone, validation only checks
// CSR key material and format, and the owner's manual approval is the sole
// gate. These tests prove that gateway-originated operator CSRs (subject CNs
// and instance IDs that mimic a gateway's RunOperator path) are accepted,
// issued, and managed identically to standalone operator enrollment. The
// current permissive behavior is intentional and documented here, not
// changed.

package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// gatewayHostname is the hostname used by the secondary (enrolling) gateway
// in these tests, mimicking the container hostname of a gateway running in
// operator mode against a primary gateway.
const gatewayHostname = "gateway-secondary"

// generateGatewayCSRAndKey generates a P-256 CSR and private key with a
// gateway-style subject CN (e.g. "g8e-operator-gateway-secondary"), mimicking
// the gateway's RunOperator path which uses
// fmt.Sprintf("g8e-operator-%s", c.hostname). The enrollment service does not
// inspect the CSR subject CN — it only validates the key material and format —
// so a gateway-originated CSR is indistinguishable from a standalone operator
// CSR at the protocol level.
func generateGatewayCSRAndKey(t *testing.T, cn string) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: cn}}, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})), key
}

// generateGatewayOperatorCSRsAndKeys generates two independent P-256 CSRs and
// keys for a gateway-as-operator enrollment, with subject CNs that mimic the
// gateway's RunOperator path: "g8e-operator-<hostname>" and
// "g8e-cli-<hostname>".
func generateGatewayOperatorCSRsAndKeys(t *testing.T) (operatorCSR string, operatorKey *ecdsa.PrivateKey, cliCSR string, cliKey *ecdsa.PrivateKey) {
	t.Helper()
	operatorCSR, operatorKey = generateGatewayCSRAndKey(t, "g8e-operator-"+gatewayHostname)
	cliCSR, cliKey = generateGatewayCSRAndKey(t, "g8e-cli-"+gatewayHostname)
	return
}

// submitGatewayOriginEnrollment submits a platform enrollment request with
// gateway-style instance ID, hostname, and CSRs, returning the create
// response. The caller provides the operator and CLI CSR material.
func submitGatewayOriginEnrollment(t *testing.T, env *platformEnrollmentTestEnv, operatorCSR, cliCSR string) *models.PlatformEnrollmentCreateResponse {
	t.Helper()
	resp, err := env.enrollSvc.CreateRequest(context.Background(), models.PlatformEnrollmentCreateRequest{
		ComponentKind:     models.PlatformComponentOperator,
		InstanceID:        "operator-" + gatewayHostname,
		Hostname:          gatewayHostname,
		SystemFingerprint: "gateway-fingerprint",
		Operator: &models.PlatformOperatorCSRPayload{
			OperatorCSRPEM: operatorCSR,
			CLICSRPEM:      cliCSR,
		},
	}, "https://gateway.local/console")
	require.NoError(t, err)
	return resp
}

// ============================================================================
// Test 1A: CreateRequest accepts gateway-origin CSR
// ============================================================================

// TestPlatformEnrollment_CreateRequest_AcceptsGatewayOriginCSR proves that
// CreateRequest does not inspect or reject CSRs whose subject CN indicates a
// gateway origin (e.g. g8e-operator-gateway-secondary). The enrollment
// service treats a gateway-originated operator CSR identically to a
// standalone operator CSR. The request is accepted, a non-empty request ID
// and token are returned, the component name is the operator component name
// (g8eo), and the persisted request is in the pending state.
func TestPlatformEnrollment_CreateRequest_AcceptsGatewayOriginCSR(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	operatorCSR, _, cliCSR, _ := generateGatewayOperatorCSRsAndKeys(t)
	resp := submitGatewayOriginEnrollment(t, env, operatorCSR, cliCSR)

	assert.NotEmpty(t, resp.RequestID, "gateway-origin enrollment must return a non-empty request ID")
	assert.NotEmpty(t, resp.Token, "gateway-origin enrollment must return a non-empty token")
	assert.Equal(t, models.PlatformOperatorName, resp.ComponentName,
		"gateway-origin enrollment component name must be %s", models.PlatformOperatorName)

	stored := loadStoredRequest(t, env, resp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStatePending, stored.State,
		"gateway-origin enrollment request must be in the pending state")
}

// ============================================================================
// Test 1B: Approve and issue — gateway-origin becomes active operator
// ============================================================================

// TestPlatformEnrollment_ApproveAndIssue_GatewayOriginBecomesActiveOperator
// proves that approving a gateway-originated enrollment request and
// completing it issues valid operator + CLI certificates and creates an
// active operator document — identical to the standalone operator path. The
// operator document is owner-owned (user_id = approving owner), the operator
// type is system, the operator is claimed, and the operator cert carries a
// SPIFFE URI SAN in the canonical operator format.
func TestPlatformEnrollment_ApproveAndIssue_GatewayOriginBecomesActiveOperator(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	operatorCSR, operatorKey, cliCSR, cliKey := generateGatewayOperatorCSRsAndKeys(t)
	createResp := submitGatewayOriginEnrollment(t, env, operatorCSR, cliCSR)

	// Approve as the first (owner) user.
	_, err := env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	require.NoError(t, err)

	approved := loadStoredRequest(t, env, createResp.RequestID)
	require.Equal(t, models.PlatformEnrollmentStateApproved, approved.State)

	// Complete with valid proof-of-possession signatures.
	proof := models.PlatformEnrollmentProofs{
		Operator: signCompletionTranscript(t, approved, operatorKey),
		CLI:      signCompletionTranscript(t, approved, cliKey),
	}
	completionResp, err := env.enrollSvc.Complete(context.Background(), createResp.Token, proof)
	require.NoError(t, err)
	require.NotNil(t, completionResp.Operator)
	assert.Contains(t, completionResp.Operator.OperatorCert, "BEGIN CERTIFICATE",
		"operator cert must be a PEM-encoded certificate")
	assert.Contains(t, completionResp.Operator.CLICert, "BEGIN CERTIFICATE",
		"CLI cert must be a PEM-encoded certificate")
	assert.NotEmpty(t, completionResp.Operator.OperatorID, "operator ID must be non-empty")
	assert.NotEmpty(t, completionResp.Operator.OperatorSessionID, "operator session ID must be non-empty")
	assert.NotEmpty(t, completionResp.Operator.CLISessionID, "CLI session ID must be non-empty")

	// Verify the operator document was persisted with the approving owner's
	// user_id, active status, system operator type, and claimed flag —
	// identical to the standalone operator path.
	opDoc, err := env.docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), completionResp.Operator.OperatorID)
	require.NoError(t, err)
	require.NotNil(t, opDoc)
	dataBytes, err := json.Marshal(opDoc.Data)
	require.NoError(t, err)
	var op models.OperatorDocumentGo
	require.NoError(t, json.Unmarshal(dataBytes, &op))
	assert.Equal(t, env.ownerID, op.UserID,
		"gateway-as-operator doc must carry the approving owner's user_id")
	assert.Equal(t, constants.OperatorStatusActive, op.Status,
		"gateway-as-operator must be active after issuance")
	assert.Equal(t, constants.OperatorTypeRemote, op.OperatorType,
		"gateway-as-operator type must be remote")
	assert.True(t, op.Claimed, "gateway-as-operator must be claimed")
	assert.Equal(t, gatewayHostname, op.Name,
		"gateway-as-operator name must match the enrollment hostname")

	// Verify the operator cert carries a SPIFFE URI SAN in the canonical
	// operator format: spiffe://g8e.local/operator/<org>/<operatorID>/<sessionID>.
	// The issuance handler passes an empty organization_id, so the org
	// segment is empty. The assertion checks the prefix and that the
	// operator_id and session_id appear in the URI.
	uris := extractURISANsFromCert(t, completionResp.Operator.OperatorCert)
	require.NotEmpty(t, uris, "operator cert must carry at least one URI SAN")
	var operatorURI string
	for _, u := range uris {
		if len(u) > len("spiffe://g8e.local/operator/") && u[:len("spiffe://g8e.local/operator/")] == "spiffe://g8e.local/operator/" {
			operatorURI = u
			break
		}
	}
	require.NotEmpty(t, operatorURI, "operator cert must carry a SPIFFE operator URI SAN")
	assert.Contains(t, operatorURI, completionResp.Operator.OperatorID,
		"operator SPIFFE URI must contain the operator ID")
	assert.Contains(t, operatorURI, completionResp.Operator.OperatorSessionID,
		"operator SPIFFE URI must contain the operator session ID")
}

// ============================================================================
// Test 1C: Deny — gateway-origin terminal, no active operator
// ============================================================================

// TestPlatformEnrollment_Deny_GatewayOriginTerminalNoActiveOperator proves
// that denying a gateway-originated enrollment request transitions it to the
// terminal denied state and produces no active operator — identical to the
// standalone denial path. The gateway's operator registry remains empty
// (no active operators from the denied request).
func TestPlatformEnrollment_Deny_GatewayOriginTerminalNoActiveOperator(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	operatorCSR, _, cliCSR, _ := generateGatewayOperatorCSRsAndKeys(t)
	createResp := submitGatewayOriginEnrollment(t, env, operatorCSR, cliCSR)

	// Deny as the first (owner) user.
	_, err := env.enrollSvc.Decide(context.Background(), env.ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionDeny,
		Reason:    "gateway not authorized",
	})
	require.NoError(t, err)

	// The request must be in the terminal denied state.
	stored := loadStoredRequest(t, env, createResp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStateDenied, stored.State,
		"gateway-origin enrollment must be in the denied state after denial")

	// No operator document must exist for the denied request. The operator
	// ID is only generated during issuance (signOperatorComponent), which
	// never runs for a denied request. Assert the operators collection has
	// no active operators.
	regSvc := env.svc.GetRegistrationService()
	operators, err := regSvc.ListUserOperators(env.ownerID)
	require.NoError(t, err)
	for _, op := range operators {
		assert.NotEqual(t, constants.OperatorStatusActive, op.Status,
			"no operator must be active after its gateway-origin enrollment was denied (id=%s)", op.ID)
	}
}

// ============================================================================
// Test 1D: Non-first user cannot approve gateway-origin request
// ============================================================================

// TestPlatformEnrollment_NonFirstUserCannotApproveGatewayOriginRequest
// proves that the active-first-user gate applies identically to
// gateway-originated requests. A non-first user cannot approve a
// gateway-as-operator enrollment. The decision fails closed with
// ErrPlatformEnrollmentInvalidDecision and the request remains pending.
func TestPlatformEnrollment_NonFirstUserCannotApproveGatewayOriginRequest(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	// Create a second user (non-owner).
	secondUser, err := env.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotEqual(t, env.ownerID, secondUser.ID)

	operatorCSR, _, cliCSR, _ := generateGatewayOperatorCSRsAndKeys(t)
	createResp := submitGatewayOriginEnrollment(t, env, operatorCSR, cliCSR)

	// The second user attempts to approve — must fail closed.
	_, err = env.enrollSvc.Decide(context.Background(), secondUser.ID, models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidDecision,
		"non-first-user approval of a gateway-origin enrollment must fail with ErrPlatformEnrollmentInvalidDecision")

	// The request must remain pending (unchanged).
	stored := loadStoredRequest(t, env, createResp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStatePending, stored.State,
		"gateway-origin enrollment must remain pending after a non-owner approval attempt")
}
