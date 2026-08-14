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

//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// dispatchRequestJSON is the typed JSON body for POST /api/v1/operators/commands.
// Mirrors gateway.OperatorCommandRequest. Defined locally to keep the E2E
// package decoupled from internal gateway types.
type dispatchRequestJSON struct {
	TargetOperatorSessionID string `json:"target_operator_session_id"`
	ActionType              string `json:"action_type"`
	Payload                 []byte `json:"payload"`
	TargetResource          string `json:"target_resource,omitempty"`
}

// dispatchResponseJSON mirrors gateway.DispatchResponse.
type dispatchResponseJSON struct {
	Success       bool   `json:"success"`
	TransactionID string `json:"transaction_id"`
	EventType     string `json:"event_type,omitempty"`
	ActionType    string `json:"action_type,omitempty"`
	ResultPayload []byte `json:"result_payload,omitempty"`
	Error         string `json:"error,omitempty"`
}

// TestDockerGateway_CommandRoundtrip_FsRead proves the full gateway → operator
// command round-trip end to end in Docker: the gateway constructs a signed
// GovernanceEnvelope carrying an FS_READ action, publishes it to the operator's
// cmd channel over the WS pub/sub broker, the operator verifies it through the
// L4Warden gauntlet (using the gateway's state root and posture propagated at
// enrollment), executes the fs.read via the L5Actuator, and publishes the
// FsReadResult on its results channel. The gateway correlates the result by
// transaction ID and returns it in the HTTP response. The test asserts the
// result payload carries the contents of /etc/hostname inside the operator
// container.
//
// This is the capstone of the gateway-operator command round-trip plan: it
// exercises D5 (remote state root), D6 (posture propagation), D1 (dispatch
// service), and D2 (HTTP route) together against the real Docker stack.
func TestDockerGateway_CommandRoundtrip_FsRead(t *testing.T) {
	if sharedFixture == nil {
		t.Skip("Docker E2E fixture not available")
	}
	f := sharedFixture

	// Ensure the operator has completed bootstrap authentication and is
	// subscribed to its cmd channel.
	f.CheckOperatorContainer(t)

	// Resolve the operator's session ID from its bootstrap logs. The dispatch
	// route targets the operator by session ID.
	sessionID := f.GetOperatorSessionID(t)
	require.NotEmpty(t, sessionID, "Operator session ID should not be empty")

	// The dispatch route is mTLS-protected (RouteAuthMTLS, the fail-closed
	// default). The operator's enrolled cert authenticates the requestor;
	// the auth middleware's handleOperatorAuth path stamps ContextKeyUserID
	// from the operator document. The target operator is specified by session
	// ID in the request body — here the operator dispatches to itself, which
	// is sufficient to prove the round-trip.
	tlsConfig := f.operatorMTLSConfig(t)

	client := &http.Client{
		Timeout:   60 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}

	// Build the FS_READ payload (proto-marshaled FsReadRequested). The path
	// is /etc/hostname, a stable file present in the operator container. The
	// path constant is the canonical SSOT — no hardcoded filepath strings.
	fsReadReq := &operatorv1.FsReadRequested{Path: constants.PathEtcHostname}
	payload, err := proto.Marshal(fsReadReq)
	require.NoError(t, err, "Failed to marshal FsReadRequested payload")

	reqBody := dispatchRequestJSON{
		TargetOperatorSessionID: sessionID,
		ActionType:              string(constants.ActionTypeFsRead),
		Payload:                 payload,
		TargetResource:          constants.PathEtcHostname,
	}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err, "Failed to marshal dispatch request body")

	reqURL := f.GatewayHTTPSURL + constants.APIPaths.OperatorsCommands
	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	// The round-trip includes in-process publish, WS delivery to the operator,
	// L4/L5 verification and execution, WS publish back, and in-process handler
	// delivery. Use a generous polling window rather than a single shot — the
	// operator may still be settling its WS subscription after bootstrap.
	var resp dispatchResponseJSON
	require.Eventually(t, func() bool {
		resp = dispatchResponseJSON{}
		httpReq, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return false
		}
		httpReq.Header.Set("Content-Type", "application/json")

		httpResp, err := client.Do(httpReq)
		if err != nil {
			t.Logf("dispatch attempt error: %v", err)
			return false
		}
		defer httpResp.Body.Close()
		body, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			t.Logf("dispatch attempt read error: %v", readErr)
			return false
		}
		if httpResp.StatusCode != http.StatusOK {
			t.Logf("dispatch attempt status=%d body=%s", httpResp.StatusCode, string(body))
			return false
		}
		if jsonErr := json.Unmarshal(body, &resp); jsonErr != nil {
			t.Logf("dispatch attempt unmarshal error: %v body=%s", jsonErr, string(body))
			return false
		}
		return resp.Success
	}, 90*time.Second, 2*time.Second, "dispatch did not succeed within 90s; last response: %+v", resp)

	// The response must carry the transaction ID and the result event type.
	// The result envelope's ActionType is the *result* action type
	// (MapEventTypeToResultActionType appends _RESULT), and its EventType is
	// the completed event — both prove the operator processed the command
	// through the full L4/L5 chain and published a typed result.
	assert.NotEmpty(t, resp.TransactionID, "response must carry the transaction ID")
	assert.Equal(t, string(constants.Event.Operator.FsRead.Completed), resp.EventType, "response event type must be the fs.read completed event")
	assert.NotEmpty(t, resp.ResultPayload, "response must carry the operator result payload")

	// Decode the result payload as FsReadResult and assert the operator read
	// /etc/hostname successfully. The content is the file's bytes inside the
	// operator container — proving the command executed on the operator, not
	// in the gateway process.
	var fsReadResult operatorv1.FsReadResult
	require.NoError(t, proto.Unmarshal(resp.ResultPayload, &fsReadResult), "Failed to unmarshal FsReadResult from response payload")
	assert.Equal(t, constants.PathEtcHostname, fsReadResult.Path, "result path must match the requested path")
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, fsReadResult.Status, "fs.read must complete successfully")
	assert.NotEmpty(t, fsReadResult.Content, "fs.read result content must not be empty")
	assert.Greater(t, fsReadResult.SizeBytes, int64(0), "fs.read result size must be positive")
}
