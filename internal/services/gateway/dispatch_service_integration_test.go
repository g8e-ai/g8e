// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.com/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build integration

package gateway

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/protocol"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// seedOperatorForDispatch registers an active user and an active operator
// document in the doc store so that AuthService.ValidateOperatorSession
// resolves the session. Returns the operator ID, session ID, and user ID.
func seedOperatorForDispatch(t *testing.T, infra *TestInfrastructure) (operatorID, operatorSessionID, userID string) {
	t.Helper()
	operatorID = "op-dispatch-int"
	operatorSessionID = "sess-dispatch-int"
	userID = "user-dispatch-int"

	userDoc := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, infra.Stores.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	opDoc := &models.OperatorDocumentGo{
		ID:                operatorID,
		UserID:            userID,
		OrganizationID:    "org-dispatch-int",
		Status:            constants.OperatorStatusActive,
		OperatorSessionID: operatorSessionID,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, infra.Stores.DocStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes))

	return operatorID, operatorSessionID, userID
}

// seedCLISessionForDispatch registers a CLI session for the given user so the
// auth middleware's handleCLIAuth path accepts the mTLS cert. Returns the CLI
// session ID and a self-signed cert with a matching CLI SPIFFE URI SAN.
func seedCLISessionForDispatch(t *testing.T, infra *TestInfrastructure, userID string) (cliSessionID string, cert *x509.Certificate) {
	t.Helper()
	cliSessionID = "cli-dispatch-int"

	cliDoc := &models.CLISession{
		ID:        cliSessionID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, infra.Stores.DocStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliBytes))

	wid := protocol.NewWorkloadIdentity()
	cliURI, err := wid.CLISPIFFEURL(userID, cliSessionID)
	require.NoError(t, err)

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "test-cli-dispatch"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{cliURI},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)
	cert, err = x509.ParseCertificate(certDER)
	require.NoError(t, err)

	return cliSessionID, cert
}

// TestDispatchController_HandleDispatch_RoundTrip verifies the full HTTP
// round-trip through the built public router: POST /api/v1/operators/commands
// publishes a signed GovernanceEnvelope to the target operator's cmd channel,
// a simulated operator handler receives it, publishes a result on the results
// channel, and the correlated result is returned in the HTTP response. This
// exercises the real in-process WS broker, the real AuthService
// ValidateOperatorSession path, the real StateRootService, and the real auth
// middleware (mTLS CLI session).
func TestDispatchController_HandleDispatch_RoundTrip(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)

	operatorID, operatorSessionID, requestorUserID := seedOperatorForDispatch(t, infra)
	cliSessionID, cliCert := seedCLISessionForDispatch(t, infra, requestorUserID)

	// Register a simulated operator on the cmd channel. The operator
	// unmarshals the command, asserts the envelope carries the gateway's
	// state root and targets the right session, then publishes a result
	// envelope on the results channel with Id == command TransactionHash.
	broker := infra.Pubsub
	cmdChannel := pubsub.CmdChannel(operatorID, operatorSessionID)
	resultsChannel := pubsub.ResultsChannel(operatorID, operatorSessionID)

	operatorDone := make(chan struct{})
	var receivedCmd *commonv1.GovernanceEnvelope
	unregisterOperator := broker.RegisterHandler(cmdChannel, func(channel string, data []byte) {
		defer close(operatorDone)
		cmdEnv := &commonv1.GovernanceEnvelope{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, cmdEnv); err != nil {
			t.Errorf("operator: unmarshal command: %v", err)
			return
		}
		receivedCmd = cmdEnv
		// The envelope must carry the gateway's current state root.
		assert.NotEmpty(t, cmdEnv.StateMerkleRoot, "dispatched envelope must carry the gateway state root")
		assert.Equal(t, operatorID, cmdEnv.OperatorId, "envelope must target the correct operator")
		assert.Equal(t, operatorSessionID, cmdEnv.OperatorSessionId, "envelope must target the correct session")
		assert.NotEmpty(t, cmdEnv.Nonce, "envelope must carry a nonce")
		assert.Equal(t, cmdEnv.Id, cmdEnv.TransactionHash, "Id must equal TransactionHash")
		assert.Equal(t, string(constants.ActionTypeFsRead), cmdEnv.ActionType, "action type must be FS_READ")
		assert.Equal(t, requestorUserID, cmdEnv.RequestorUserId, "envelope must carry the requestor user ID from mTLS context")

		// Publish a correlated result envelope on the results channel.
		resultEnv := &commonv1.GovernanceEnvelope{
			Id:         cmdEnv.Id,
			EventType:  cmdEnv.EventType,
			ActionType: cmdEnv.ActionType,
			Timestamp:  timestamppb.Now(),
		}
		resultWire, err := protojson.Marshal(resultEnv)
		require.NoError(t, err)
		broker.Publish(resultsChannel, resultWire)
	})
	t.Cleanup(unregisterOperator)

	// Build the FS_READ payload (proto-marshaled FsReadRequested).
	fsReadReq := &operatorv1.FsReadRequested{Path: "/etc/hostname"}
	payload, err := proto.Marshal(fsReadReq)
	require.NoError(t, err)

	reqBody := OperatorCommandRequest{
		TargetOperatorSessionID: operatorSessionID,
		ActionType:              string(constants.ActionTypeFsRead),
		Payload:                 payload,
		TargetResource:          "/etc/hostname",
	}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// The dispatch route is mTLS-protected. Stamp the request with a CLI
	// mTLS cert + CLI session header so the auth middleware's handleCLIAuth
	// path validates the session and stamps ContextKeyUserID.
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsCommands, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cliCert},
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "dispatch HTTP response body: %s", rr.Body.String())

	var resp DispatchResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp), "response body: %s", rr.Body.String())
	assert.True(t, resp.Success, "dispatch must succeed")
	assert.NotEmpty(t, resp.TransactionID, "response must carry the transaction ID")
	assert.Equal(t, string(constants.ActionTypeFsRead), resp.ActionType, "response action type must echo FS_READ")

	// The simulated operator must have received the command.
	select {
	case <-operatorDone:
	case <-time.After(5 * time.Second):
		t.Fatal("simulated operator did not receive the command on the cmd channel")
	}
	require.NotNil(t, receivedCmd)
	assert.Equal(t, resp.TransactionID, receivedCmd.Id, "response transaction ID must match the dispatched envelope Id")
}

// TestDispatchController_HandleDispatch_UnknownSession verifies that
// dispatching to an unregistered operator session fails closed with a 500
// error (the dispatch service wraps the auth error) and does not publish to
// any cmd channel.
func TestDispatchController_HandleDispatch_UnknownSession(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)

	// Seed a CLI session for mTLS auth, but target a nonexistent operator.
	_, _, requestorUserID := seedOperatorForDispatch(t, infra)
	cliSessionID, cliCert := seedCLISessionForDispatch(t, infra, requestorUserID)

	fsReadReq := &operatorv1.FsReadRequested{Path: "/etc/hostname"}
	payload, err := proto.Marshal(fsReadReq)
	require.NoError(t, err)

	reqBody := OperatorCommandRequest{
		TargetOperatorSessionID: "nonexistent-session-int",
		ActionType:              string(constants.ActionTypeFsRead),
		Payload:                 payload,
	}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsCommands, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cliCert},
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code, "unknown operator session must fail closed")
	assert.Contains(t, rr.Body.String(), "validate operator session")
}
