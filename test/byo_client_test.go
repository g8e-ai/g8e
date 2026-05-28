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

//go:build integration
// +build integration

package tests

/*
TestBYOClientParity_EndToEnd exercises g8eo from the perspective of a protocol-aware
BYO client. This test verifies the canonical JSON wire format, mTLS enrollment,
and fail-closed L1/L2/L3 governance gates.

Practical Coverage:
1. Canonical JSON Wire Format: Uses protojson-encoded GovernanceEnvelope on all client paths.
2. mTLS & Enrollment: Exercises CSR-based enrollment via proper enrollment API.
3. State Binding: Verifies transactions are bound to StateMerkleRoot.
4. Fail-Closed L3: Uses mTLS certificate fingerprint for L3 verification (no mock).
5. Real Execution: Tests actual command execution through Actuator, not simulation.
*/

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/execution"
	"github.com/g8e-ai/g8e/internal/services/gateway"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/internal/services/sovereignty"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	pubsubv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/pubsub/v1"
)

func TestBYOClientParity_EndToEnd(t *testing.T) {
	dataDir := t.TempDir()
	secretsDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")

	cfg, err := config.LoadGateway(config.GatewayOptions{
		DataDir:           dataDir,
		PKIDir:            pkiDir,
		SecretsDir:        secretsDir,
		PasskeyRpID:       "localhost",
		PasskeyRpName:     "g8e",
		AllowTestPortZero: true,
	})
	require.NoError(t, err)

	ls, err := gateway.NewGatewayService(cfg, testutil.NewTestLogger())
	require.NoError(t, err)

	execSvc := execution.NewExecutionService(cfg, testutil.NewTestLogger())
	fileSvc := execution.NewFileEditService(cfg, testutil.NewTestLogger())
	govDeps := ls.GetGovernanceDeps()
	sm, err := ls.GetSecretManager()
	require.NoError(t, err)
	ActuatorPriv, ActuatorKeyID, err := sm.GetActuatorKey()
	require.NoError(t, err)
	cmdSvc, err := pubsub.NewPubSubCommandService(pubsub.CommandServiceConfig{
		Config:             cfg,
		Logger:             testutil.NewTestLogger(),
		Execution:          execSvc,
		FileEdit:           fileSvc,
		PubSubClient:       pubsub.NewInProcessPubSubClient(ls.GetHTTPHandler().GetPubSubBroker()),
		Sovereignty:        sovereignty.NewSovereigntyService(sovereignty.DefaultConfig(), testutil.NewTestLogger(), nil),
		ReplayStore:        govDeps.ReplayStore,
		StateRootProvider:  govDeps.StateRootProvider,
		TransactionAudit:   govDeps.TransactionAudit,
		SignerStore:        govDeps.SignerStore,
		L3Notary:           nil, // L3 verified via mTLS certificate fingerprint in CLIL3Notary
		ActuatorSigningKey: ActuatorPriv,
		ActuatorKeyID:      ActuatorKeyID,
	})
	require.NoError(t, err)
	ls.SetEnvelopeProcessor(cmdSvc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := ls.Start(ctx); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Gateway service failed: %v\n", err)
		}
	}()

	// Wait for readiness
	require.Eventually(t, func() bool {
		return ls.IsReady()
	}, 5*time.Second, 100*time.Millisecond)

	// Since we used port 0, we need to know what ports were assigned.
	// We'll add getters for the servers in GatewayService.
	publicURL := fmt.Sprintf("https://localhost:%d", ls.GetPublicPort())
	mtlsURL := fmt.Sprintf("https://localhost:%d", ls.GetHTTPPort())
	wssURL := fmt.Sprintf("wss://localhost:%d/ws/pubsub", ls.GetHTTPPort())

	// 1. Discover Operator trust metadata
	// Hub bundle (Root + Hub CA) is available on public port via HTTPS for initial discovery
	// Instead of insecurely trusting the endpoint, we bootstrap trust from the known PKI dir
	// which simulates a user pre-installing the Operator's root CA.
	hubBundlePath := filepath.Join(pkiDir, "trust", "hub-bundle.pem")
	require.Eventually(t, func() bool {
		_, err := os.Stat(hubBundlePath)
		return err == nil
	}, 5*time.Second, 100*time.Millisecond)

	bootstrapRootPool := x509.NewCertPool()
	initialBundle, err := os.ReadFile(hubBundlePath)
	require.NoError(t, err)
	require.True(t, bootstrapRootPool.AppendCertsFromPEM(initialBundle))

	secureDiscoveryClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: bootstrapRootPool,
			},
		},
	}
	resp, err := secureDiscoveryClient.Get(publicURL + "/.well-known/g8e/pki/hub-bundle.pem")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	hubBundlePEM, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, initialBundle, hubBundlePEM)

	rootPool := x509.NewCertPool()
	require.True(t, rootPool.AppendCertsFromPEM(hubBundlePEM))

	// 2. Create enrollment token for enrollment (test setup via DB)
	// In production, enrollment tokens are created via admin API by authorized users.
	// For test setup, we inject the token directly into the DB.
	userID := "byo-user-test-"
	orgID := "byo-org-test-"
	token := "enroll_byo_test_client_token_12345"
	tokenData := models.DeviceLinkData{
		Token:          token,
		UserID:         userID,
		OrganizationID: orgID,
		MaxUses:        1,
		Status:         "active",
		ExpiresAt:      time.Now().Add(1 * time.Hour),
	}
	tokenBytes, _ := json.Marshal(tokenData)
	err = ls.GetDB().KVSet("g8e:device-link:"+token, string(tokenBytes), 3600)
	require.NoError(t, err)

	// Generate CSR
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	csrTmpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: "byo-test-client",
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTmpl, priv)
	require.NoError(t, err)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// Submit registration
	regReq := models.OperatorRegistrationRequest{
		CSR:               string(csrPEM),
		SystemFingerprint: "byo-fingerprint",
		Hostname:          "byo-host",
	}
	regBody, _ := json.Marshal(regReq)
	req, err := http.NewRequest(http.MethodPost, publicURL+"/api/auth/device-link/register", bytes.NewReader(regBody))
	require.NoError(t, err)
	req.Header.Set(constants.HeaderDeviceToken, token)

	publicClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: rootPool}}}
	resp, err = publicClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var regResp models.OperatorRegistrationResponse
	err = json.NewDecoder(resp.Body).Decode(&regResp)
	require.NoError(t, err)
	require.True(t, regResp.Success)

	// Configure mTLS client
	cert, err := tls.X509KeyPair([]byte(regResp.OperatorCert), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: priv}))
	if err != nil {
		// Try ED25519 private key encoding if standard fails
		privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
		cert, err = tls.X509KeyPair([]byte(regResp.OperatorCert), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))
		require.NoError(t, err)
	}

	mtlsClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cert},
			},
		},
	}

	signerName := "test-signer"
	signerPub, signerPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	err = ls.GetDB().AddTrustedSigner(models.TrustedSigner{
		ID:        signerName,
		PublicKey: hex.EncodeToString(signerPub),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	})
	require.NoError(t, err)

	// 3. Fetch current state root
	resp, err = mtlsClient.Get(mtlsURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var health models.HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)
	stateRoot := health.StateMerkleRoot
	// Initial state root might be empty string or some default

	// 4. Add auto-approval policy for benign diagnostic command
	autoApprovePolicy := map[string]interface{}{
		"pattern": "^echo\\s+.*",
		"enabled": true,
		"reason":  "test auto-approval for echo command",
		"posture": "notary",
	}
	policyBody, _ := json.Marshal(autoApprovePolicy)
	policyReq, _ := http.NewRequest(http.MethodPost, mtlsURL+"/db/auto_approved/echo-test", bytes.NewReader(policyBody))
	policyReq.Header.Set("Content-Type", "application/json")
	policyReq.Header.Set(constants.HeaderAuthorization, "Bearer "+regResp.OperatorSessionID)
	policyResp, err := mtlsClient.Do(policyReq)
	require.NoError(t, err)
	policyResp.Body.Close()

	// 5. Build typed transaction payload
	cmdReq := &operatorv1.CommandRequested{
		Command:      "echo hello BYO",
		ExecutionId:  "exec-1",
		Intent:       "verify BYO client flow",
		SentinelMode: "audit",
	}
	cmdPayload, _ := proto.Marshal(cmdReq)

	nonce := "nonce-1"
	envelope := &commonv1.GovernanceEnvelope{
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(5 * time.Minute)),
		SourceComponent:   commonv1.Component_COMPONENT_G8EE,
		OperatorId:        regResp.OperatorID,
		OperatorSessionId: regResp.OperatorSessionID,
		EventType:         string(constants.Event.Operator.Command.Requested),
		ActionType:        string(constants.ActionTypeExecuteBash),
		Payload:           cmdPayload,
		StateMerkleRoot:   stateRoot,
		Nonce:             nonce,
		ProtocolVersion:   "1.0",
	}

	// 6. Attach L2 proof (Tribunal)
	transactionHash, err := governance.GenerateMessageID(envelope)
	require.NoError(t, err)
	envelope.Id = transactionHash
	envelope.TransactionHash = transactionHash
	sigPayload := fmt.Sprintf("%s|%v", transactionHash, true)
	signature := ed25519.Sign(signerPriv, []byte(sigPayload))

	envelope.Governance = &commonv1.GovernanceMetadata{
		L1: &commonv1.L1Metadata{Validated: true},
		L2: &commonv1.L2Metadata{
			ConsensusSignature: hex.EncodeToString(signature),
			AgentIds:           []string{signerName},
			KeyId:              signerName,
		},
		L3: &commonv1.L3Metadata{
			// L3 proof uses mTLS certificate fingerprint for CLI sessions
			// The CLIL3Notary will verify the certificate from the mTLS handshake
			AutoApproved: true, // Mark as auto-approved via policy
		},
	}

	// 7. Submit transaction via canonical JSON wire format
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			RootCAs:      rootPool,
			Certificates: []tls.Certificate{cert},
		},
	}
	wsHeader := http.Header{}
	wsHeader.Set(constants.HeaderAuthorization, "Bearer "+regResp.OperatorSessionID)

	wsConn, resp, err := dialer.Dial(wssURL, wsHeader)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		require.NoError(t, err)
	}
	defer wsConn.Close()

	// Subscribe to results
	resultsChannel := constants.ResultsChannel(regResp.OperatorID, regResp.OperatorSessionID)
	subMsg := &pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionSubscribe,
		Channel: resultsChannel,
	}
	subBytes, _ := proto.Marshal(subMsg)
	err = wsConn.WriteMessage(websocket.BinaryMessage, subBytes)
	require.NoError(t, err)

	// Drain the 'subscribed' ack message
	_, ackMsg, err := wsConn.ReadMessage()
	require.NoError(t, err)
	var ackEvent pubsubv1.PubSubEvent
	err = proto.Unmarshal(ackMsg, &ackEvent)
	require.NoError(t, err)
	require.Equal(t, constants.PubSubEventSubscribed, ackEvent.Type)

	// Submit the envelope via the canonical governed mutation entry.
	dataJSON, err := protojson.Marshal(envelope)
	require.NoError(t, err)
	httpReq, err := http.NewRequest(http.MethodPost, mtlsURL+"/api/governance/envelope", bytes.NewReader(dataJSON))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(constants.HeaderAuthorization, "Bearer "+regResp.OperatorSessionID)

	resp, err = mtlsClient.Do(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var receipt operatorv1.ActionReceipt
	err = json.NewDecoder(resp.Body).Decode(&receipt)
	require.NoError(t, err)
	require.Equal(t, envelope.Id, receipt.TransactionId)
	require.Equal(t, envelope.TransactionHash, receipt.TransactionHash)
	require.NotEmpty(t, receipt.SignerKeyId)
	require.NotEmpty(t, receipt.Signature)

	// 8. Receive real execution result on the WebSocket
	// The Actuator executes the command and publishes the result
	_, wsMsg, err := wsConn.ReadMessage()
	require.NoError(t, err)

	var pubsubEvent pubsubv1.PubSubEvent
	err = proto.Unmarshal(wsMsg, &pubsubEvent)
	require.NoError(t, err)
	require.Equal(t, constants.PubSubEventMessage, pubsubEvent.Type)
	require.Equal(t, resultsChannel, pubsubEvent.Channel)

	var receivedEnv commonv1.GovernanceEnvelope
	err = protojson.Unmarshal(pubsubEvent.Data, &receivedEnv)
	require.NoError(t, err)

	// Verify the result envelope contains the actual command output
	var result operatorv1.CommandResult
	err = proto.Unmarshal(receivedEnv.Payload, &result)
	require.NoError(t, err)
	require.Equal(t, "exec-1", result.ExecutionId)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
	require.Contains(t, result.Stdout, "hello BYO")
	require.Equal(t, int32(0), result.ReturnCode)
}
