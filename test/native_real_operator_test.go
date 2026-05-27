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

package tests

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	pubsubv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/pubsub/v1"
)

// TestNativeRealOperator_EndToEnd is a live-operator integration test that validates
// the native g8e protocol (GovernanceEnvelope) against a running ./g8e platform.
//
// This test demonstrates the complete native protocol flow:
// 1. Bootstrap against running Gateway with CSR
// 2. Submit native GovernanceEnvelope via /api/governance/envelope
// 3. Receive execution results via WebSocket Pub/Sub
// 4. Verify real file operations through actual Operator
// 5. Verify L1 doctrine enforcement through actual Operator
//
// Skip conditions:
//   - Operator not reachable at $OPERATOR_URL (default from paths.json)
//   - Trust bundle not present at $G8E_PKI_DIR_HOST/trust/hub-bundle.pem
//   - Platform already bootstrapped (403) and no rotation context available
func TestNativeRealOperator_EndToEnd(t *testing.T) {
	operatorURL := os.Getenv("OPERATOR_URL")
	if operatorURL == "" {
		operatorURL = fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps)
	}

	insecureClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	if resp, err := insecureClient.Get(operatorURL + "/health"); err != nil {
		t.Skipf("Operator not reachable at %s: %v. Run './g8e platform start' to enable.", operatorURL, err)
	} else {
		resp.Body.Close()
	}

	cwd, err := os.Getwd()
	require.NoError(t, err)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(cwd)))
	pkiDir := filepath.Join(repoRoot, ".g8e", "pki")
	if override := os.Getenv("G8E_PKI_DIR_HOST"); override != "" {
		pkiDir = override
	}

	trustBundlePath := filepath.Join(pkiDir, "trust", "hub-bundle.pem")
	trustPEM, err := os.ReadFile(trustBundlePath)
	if err != nil {
		t.Skipf("Trust bundle not found at %s: %v. Run './g8e platform clean && ./g8e platform start'.", trustBundlePath, err)
	}
	rootPool := x509.NewCertPool()
	require.True(t, rootPool.AppendCertsFromPEM(trustPEM), "failed to parse trust bundle")

	_, opPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	opCsrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "g8e-native-op"}}, opPriv)
	require.NoError(t, err)
	opCsrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: opCsrDER})

	_, cliPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cliCsrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "g8e-native-cli"}}, cliPriv)
	require.NoError(t, err)
	cliCsrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: cliCsrDER})

	fpHash := sha256.Sum256([]byte("g8e-native-protocol-test"))
	fingerprint := hex.EncodeToString(fpHash[:])

	reqBody, _ := json.Marshal(map[string]string{
		"csr_pem":            string(opCsrPEM),
		"cli_csr_pem":        string(cliCsrPEM),
		"system_fingerprint": fingerprint,
	})

	trustClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: rootPool}},
	}

	httpReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:%d/api/auth/bootstrap", constants.Ports.OperatorBootstrapHttps), bytes.NewReader(reqBody))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	bootstrapResp, err := trustClient.Do(httpReq)
	require.NoError(t, err)
	defer bootstrapResp.Body.Close()

	respBytes, _ := io.ReadAll(bootstrapResp.Body)
	if bootstrapResp.StatusCode == http.StatusForbidden || bootstrapResp.StatusCode == http.StatusConflict {
		t.Skipf("Platform already bootstrapped (status %d): %s. Run './g8e platform clean && ./g8e platform start' to reset.", bootstrapResp.StatusCode, string(respBytes))
	}
	require.Equal(t, http.StatusCreated, bootstrapResp.StatusCode, "bootstrap failed: %s", string(respBytes))

	var regResp models.OperatorRegistrationResponse
	require.NoError(t, json.Unmarshal(respBytes, &regResp))
	require.NotEmpty(t, regResp.CLICert, "bootstrap did not return CLI cert")
	require.NotEmpty(t, regResp.OperatorSessionID)
	require.NotEmpty(t, regResp.CLISessionID)

	cliPrivBytes, err := x509.MarshalPKCS8PrivateKey(cliPriv)
	require.NoError(t, err)
	cliKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: cliPrivBytes})
	cliCert, err := tls.X509KeyPair([]byte(regResp.CLICert), cliKeyPEM)
	require.NoError(t, err)

	mtlsClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cliCert},
			},
		},
	}

	// Fetch current state root
	healthResp, err := mtlsClient.Get(operatorURL + "/health")
	require.NoError(t, err)
	defer healthResp.Body.Close()
	require.Equal(t, http.StatusOK, healthResp.StatusCode)

	var health models.HealthResponse
	err = json.NewDecoder(healthResp.Body).Decode(&health)
	require.NoError(t, err)
	stateRoot := health.StateMerkleRoot

	// Add trusted signer for L2 consensus
	signerName := "native-test-signer"
	signerPub, signerPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signerReqBody, _ := json.Marshal(map[string]interface{}{
		"id":         signerName,
		"public_key": hex.EncodeToString(signerPub),
		"enabled":    true,
	})
	signerReq, _ := http.NewRequest(http.MethodPost, operatorURL+"/db/trusted_signers/"+signerName, bytes.NewReader(signerReqBody))
	signerReq.Header.Set("Content-Type", "application/json")
	signerReq.Header.Set("Authorization", "Bearer "+regResp.OperatorSessionID)
	signerReq.Header.Set("X-G8E-CLI-Session-ID", regResp.CLISessionID)
	signerReq.Header.Set("X-G8E-Source-Component", "client")

	signerResp, err := mtlsClient.Do(signerReq)
	require.NoError(t, err)
	signerResp.Body.Close()

	// Add auto-approval policy for benign commands
	autoApprovePolicy := map[string]interface{}{
		"pattern": "^echo\\s+.*",
		"enabled": true,
		"reason":  "test auto-approval for echo command",
		"posture": "notary",
	}
	policyBody, _ := json.Marshal(autoApprovePolicy)
	policyReq, _ := http.NewRequest(http.MethodPost, operatorURL+"/db/auto_approved/native-test", bytes.NewReader(policyBody))
	policyReq.Header.Set("Content-Type", "application/json")
	policyReq.Header.Set("Authorization", "Bearer "+regResp.OperatorSessionID)
	policyReq.Header.Set("X-G8E-CLI-Session-ID", regResp.CLISessionID)
	policyReq.Header.Set("X-G8E-Source-Component", "client")

	policyResp, err := mtlsClient.Do(policyReq)
	require.NoError(t, err)
	policyResp.Body.Close()

	// Connect to WebSocket Pub/Sub for results
	wssURL := fmt.Sprintf("wss://localhost:%d/ws/pubsub", constants.Ports.OperatorHttps)
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			RootCAs:      rootPool,
			Certificates: []tls.Certificate{cliCert},
		},
	}
	wsHeader := http.Header{}
	wsHeader.Set("Authorization", "Bearer "+regResp.OperatorSessionID)
	wsHeader.Set("X-G8E-CLI-Session-ID", regResp.CLISessionID)
	wsHeader.Set("X-G8E-Source-Component", "client")

	wsConn, wsResp, err := dialer.Dial(wssURL, wsHeader)
	if err != nil {
		if wsResp != nil {
			wsResp.Body.Close()
		}
		require.NoError(t, err)
	}
	defer wsConn.Close()

	resultsChannel := constants.ResultsChannel(regResp.OperatorID, regResp.OperatorSessionID)
	subMsg := &pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionSubscribe,
		Channel: resultsChannel,
	}
	subBytes, _ := proto.Marshal(subMsg)
	err = wsConn.WriteMessage(websocket.BinaryMessage, subBytes)
	require.NoError(t, err)

	_, ackMsg, err := wsConn.ReadMessage()
	require.NoError(t, err)
	var ackEvent pubsubv1.PubSubEvent
	err = proto.Unmarshal(ackMsg, &ackEvent)
	require.NoError(t, err)
	require.Equal(t, constants.PubSubEventSubscribed, ackEvent.Type)

	t.Run("native envelope shell command", func(t *testing.T) {
		cmdReq := &operatorv1.CommandRequested{
			Command:      "echo hello native protocol",
			ExecutionId:  "exec-native-1",
			Intent:       "test native protocol",
			SentinelMode: "audit",
		}
		cmdPayload, _ := proto.Marshal(cmdReq)

		nonce := "nonce-native-1"
		envelope := &commonv1.GovernanceEnvelope{
			Timestamp:         timestamppb.Now(),
			ExpiresAt:         timestamppb.New(time.Now().Add(5 * time.Minute)),
			SourceComponent:   commonv1.Component_COMPONENT_G8EE,
			OperatorId:        regResp.OperatorID,
			OperatorSessionId: regResp.OperatorSessionID,
			EventType:         string(constants.EventOperatorCommandRequested),
			ActionType:        string(constants.ActionTypeExecuteBash),
			Payload:           cmdPayload,
			StateMerkleRoot:   stateRoot,
			Nonce:             nonce,
			ProtocolVersion:   "1.0",
		}

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
			L3: &commonv1.L3Metadata{AutoApproved: true},
		}

		dataJSON, err := protojson.Marshal(envelope)
		require.NoError(t, err)

		envReq, err := http.NewRequest(http.MethodPost, operatorURL+"/api/governance/envelope", bytes.NewReader(dataJSON))
		require.NoError(t, err)
		envReq.Header.Set("Content-Type", "application/json")
		envReq.Header.Set("Authorization", "Bearer "+regResp.OperatorSessionID)
		envReq.Header.Set("X-G8E-CLI-Session-ID", regResp.CLISessionID)
		envReq.Header.Set("X-G8E-Source-Component", "client")

		envResp, err := mtlsClient.Do(envReq)
		require.NoError(t, err)
		defer envResp.Body.Close()
		require.Equal(t, http.StatusOK, envResp.StatusCode)

		var receipt operatorv1.ActionReceipt
		err = json.NewDecoder(envResp.Body).Decode(&receipt)
		require.NoError(t, err)
		require.Equal(t, envelope.Id, receipt.TransactionId)

		// Receive result via WebSocket
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

		var result operatorv1.CommandResult
		err = proto.Unmarshal(receivedEnv.Payload, &result)
		require.NoError(t, err)
		require.Equal(t, "exec-native-1", result.ExecutionId)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
		require.Contains(t, result.Stdout, "hello native protocol")
		require.Equal(t, int32(0), result.ReturnCode)
	})

	t.Run("L1 doctrine enforcement - sudo rejected", func(t *testing.T) {
		cmdReq := &operatorv1.CommandRequested{
			Command:      "sudo rm -rf /",
			ExecutionId:  "exec-sudo-native",
			Intent:       "malicious intent",
			SentinelMode: "audit",
		}
		cmdPayload, _ := proto.Marshal(cmdReq)

		nonce := "nonce-sudo-native"
		envelope := &commonv1.GovernanceEnvelope{
			Timestamp:         timestamppb.Now(),
			ExpiresAt:         timestamppb.New(time.Now().Add(5 * time.Minute)),
			SourceComponent:   commonv1.Component_COMPONENT_G8EE,
			OperatorId:        regResp.OperatorID,
			OperatorSessionId: regResp.OperatorSessionID,
			EventType:         string(constants.EventOperatorCommandRequested),
			ActionType:        string(constants.ActionTypeExecuteBash),
			Payload:           cmdPayload,
			StateMerkleRoot:   stateRoot,
			Nonce:             nonce,
			ProtocolVersion:   "1.0",
		}

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
			L3: &commonv1.L3Metadata{AutoApproved: true},
		}

		dataJSON, err := protojson.Marshal(envelope)
		require.NoError(t, err)

		envReq, err := http.NewRequest(http.MethodPost, operatorURL+"/api/governance/envelope", bytes.NewReader(dataJSON))
		require.NoError(t, err)
		envReq.Header.Set("Content-Type", "application/json")
		envReq.Header.Set("Authorization", "Bearer "+regResp.OperatorSessionID)
		envReq.Header.Set("X-G8E-CLI-Session-ID", regResp.CLISessionID)
		envReq.Header.Set("X-G8E-Source-Component", "client")

		envResp, err := mtlsClient.Do(envReq)
		require.NoError(t, err)
		defer envResp.Body.Close()

		require.Equal(t, http.StatusBadRequest, envResp.StatusCode, "L1 forbidden pattern should be rejected")
	})

	t.Run("native envelope file operations", func(t *testing.T) {
		testFile := filepath.Join(repoRoot, ".g8e", "test-native-file.txt")
		initialContent := "native protocol test content\n"

		// Write file
		fileEditReq := &operatorv1.FileEditRequested{
			FilePath:        testFile,
			Operation:       "write",
			ExecutionId:     "exec-write-native",
			Justification:   "test native file write",
			Content:         initialContent,
			CreateIfMissing: true,
			SentinelMode:    "audit",
		}
		fileEditPayload, _ := proto.Marshal(fileEditReq)

		nonce := "nonce-write-native"
		envelope := &commonv1.GovernanceEnvelope{
			Timestamp:         timestamppb.Now(),
			ExpiresAt:         timestamppb.New(time.Now().Add(5 * time.Minute)),
			SourceComponent:   commonv1.Component_COMPONENT_G8EE,
			OperatorId:        regResp.OperatorID,
			OperatorSessionId: regResp.OperatorSessionID,
			EventType:         string(constants.EventOperatorFileEditRequested),
			ActionType:        string(constants.ActionTypeFileEdit),
			Payload:           fileEditPayload,
			StateMerkleRoot:   stateRoot,
			Nonce:             nonce,
			ProtocolVersion:   "1.0",
		}

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
			L3: &commonv1.L3Metadata{AutoApproved: true},
		}

		dataJSON, err := protojson.Marshal(envelope)
		require.NoError(t, err)

		envReq, err := http.NewRequest(http.MethodPost, operatorURL+"/api/governance/envelope", bytes.NewReader(dataJSON))
		require.NoError(t, err)
		envReq.Header.Set("Content-Type", "application/json")
		envReq.Header.Set("Authorization", "Bearer "+regResp.OperatorSessionID)
		envReq.Header.Set("X-G8E-CLI-Session-ID", regResp.CLISessionID)
		envReq.Header.Set("X-G8E-Source-Component", "client")

		envResp, err := mtlsClient.Do(envReq)
		require.NoError(t, err)
		defer envResp.Body.Close()
		require.Equal(t, http.StatusOK, envResp.StatusCode)

		var receipt operatorv1.ActionReceipt
		err = json.NewDecoder(envResp.Body).Decode(&receipt)
		require.NoError(t, err)
		require.Equal(t, envelope.Id, receipt.TransactionId)

		// Receive result via WebSocket
		_, wsMsg, err := wsConn.ReadMessage()
		require.NoError(t, err)

		var pubsubEvent pubsubv1.PubSubEvent
		err = proto.Unmarshal(wsMsg, &pubsubEvent)
		require.NoError(t, err)
		require.Equal(t, constants.PubSubEventMessage, pubsubEvent.Type)

		// Verify file was actually written
		actualContent, err := os.ReadFile(testFile)
		require.NoError(t, err)
		require.Equal(t, initialContent, string(actualContent))

		// Clean up
		os.Remove(testFile)
	})

	t.Logf("Test completed. Querying live operator audit vault at %s", filepath.Join(repoRoot, ".g8e", "audit_vault.db"))
	vaultPath := filepath.Join(repoRoot, ".g8e", "audit_vault.db")
	if _, statErr := os.Stat(vaultPath); statErr == nil {
		t.Logf("Live audit vault found at: %s", vaultPath)
		t.Logf("To inspect vault: sqlite3 %s", vaultPath)
	} else {
		t.Logf("No audit vault found at %s", vaultPath)
	}
}
