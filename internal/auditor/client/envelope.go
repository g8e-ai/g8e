// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package client

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	// REAL g8e packages — not forks. This is what guarantees the transaction
	// hash and wire format match the verifier. GovernanceEnvelope is the alias for
	// g8e.common.v1.GovernanceEnvelope; GenerateMessageID is the canonical hasher.

	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// Canonical UAP action types. SCREAMING_SNAKE per protocol/constants. Pinned
// here so Phantom has no internal/ import; verify against MapEventTypeToActionType.
const (
	ActionMcpCall   = "MCP_CALL"
	ActionA2aCall   = "A2A_CALL"
	ProtocolVersion = "1.0"
)

// Ensemble is Phantom's mock L2 consensus tribunal: N agents that each "vote"
// on a transaction hash. The envelope carries a single aggregate Ed25519
// signature from the registered consensus key over "<hash>|<decision>", plus
// one AgentID per voter — exactly what L4Warden.verifyL2Signature checks.
type Ensemble struct {
	KeyID  string
	priv   ed25519.PrivateKey
	pub    ed25519.PublicKey
	agents []string
}

// NewEnsemble mints a fresh consensus key and n agent identities.
func NewEnsemble(keyID string, n int) (*Ensemble, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	agents := make([]string, n)
	for i := range agents {
		agents[i] = fmt.Sprintf("%s-agent-%d", keyID, i+1)
	}
	return &Ensemble{KeyID: keyID, priv: priv, pub: pub, agents: agents}, nil
}

// PubHex is the consensus public key for trusted-signer registration.
func (e *Ensemble) PubHex() string { return hex.EncodeToString(e.pub) }

// AgentCount is the number of mock voters in the ensemble.
func (e *Ensemble) AgentCount() int { return len(e.agents) }

// Vote produces the L2 metadata for a decision over the transaction hash.
// decision==true means "the ensemble agreed this mutation is safe."
func (e *Ensemble) Vote(txHash string, decision bool) *commonv1.L2Metadata {
	basis := fmt.Sprintf("%s|%v", txHash, decision) // matches l2_consensus.SignDecision
	sig := ed25519.Sign(e.priv, []byte(basis))
	return &commonv1.L2Metadata{
		ConsensusSignature: hex.EncodeToString(sig),
		AgentIds:           append([]string(nil), e.agents...),
		KeyId:              e.KeyID,
	}
}

// Principal is the mock L3 notary: a single "human" key that authorizes the
// exact transaction hash. In a real notary deployment this is replaced by a
// WebAuthn/passkey assertion (web) or an mTLS cert fingerprint (CLI) — see the
// "suspend" L3 mode for the genuine OOB flow.
type Principal struct {
	KeyID string
	priv  ed25519.PrivateKey
	pub   ed25519.PublicKey
}

func NewPrincipal(keyID string) (*Principal, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Principal{KeyID: keyID, priv: priv, pub: pub}, nil
}

func (p *Principal) PubHex() string { return hex.EncodeToString(p.pub) }

// Sign returns an L3 proof: a principal signature over the transaction hash.
func (p *Principal) Sign(txHash string) *commonv1.L3Metadata {
	sig := ed25519.Sign(p.priv, []byte(txHash))
	return &commonv1.L3Metadata{
		Proof: &commonv1.L3Proof{Signature: hex.EncodeToString(sig)},
	}
}

// MaximalEnvelope is the input for an official governance envelope submission.
type MaximalEnvelope struct {
	OperatorID     string
	ToolName       string
	ArgumentsJSON  string
	TargetResource string
	StateRoot      string
	Ensemble       *Ensemble  // attach L2 when non-nil
	Principal      *Principal // attach mock L3 when non-nil ("mock" mode)
	TTL            time.Duration
}

// SubmitMaximal builds a real UAPEnvelope wrapping an MCP_CALL, computes the
// canonical transaction hash with g8e's own hasher, attaches L2 (and optionally
// mock L3), marshals it as protojson, and POSTs it to the admission API.
// Returns the computed hash so callers can drive the OOB approve flow.
func (c *Client) SubmitMaximal(ctx context.Context, p Persona, m MaximalEnvelope) (txHash string, status int, body []byte, err error) {
	// 1. Typed protobuf payload — the "what". The Operator decodes this exact type.
	call := &operatorv1.McpCallRequested{
		ToolName:      m.ToolName,
		ArgumentsJson: m.ArgumentsJSON,
		ExecutionId:   fmt.Sprintf("phantom-%d", time.Now().UnixNano()),
	}
	payloadBytes, err := proto.Marshal(call)
	if err != nil {
		return "", 0, nil, fmt.Errorf("marshal payload: %w", err)
	}

	// JSON-first mirror of intent for consumers that read intent_data.
	intent, _ := structpb.NewStruct(map[string]any{
		"payload_type": "McpCallRequested",
		"tool_name":    m.ToolName,
		"arguments":    m.ArgumentsJSON,
	})

	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)

	ttl := m.TTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	// 2. The envelope — the "how". GovernanceEnvelope is the canonical type.
	env := &governance.GovernanceEnvelope{
		ProtocolVersion: ProtocolVersion,
		Timestamp:       timestamppb.Now(),
		ExpiresAt:       timestamppb.New(time.Now().Add(ttl)),
		SourceComponent: commonv1.Component_COMPONENT_CLIENT,
		OperatorId:      m.OperatorID,
		ActionType:      ActionMcpCall,
		TargetResource:  m.TargetResource,
		EventType:       "operator.mcp.call.requested",
		Payload:         payloadBytes,
		IntentData:      intent,
		StateMerkleRoot: m.StateRoot,
		Nonce:           hex.EncodeToString(nonce),
	}

	// 3. Canonical hash — REAL hasher. id == transaction_hash == SHA256(canonical).
	txHash, err = governance.GenerateMessageID(env)
	if err != nil {
		return "", 0, nil, fmt.Errorf("generate message id: %w", err)
	}
	env.Id = txHash
	env.TransactionHash = txHash

	// 4. Governance proofs over the hash.
	env.Governance = &commonv1.GovernanceMetadata{L1: &commonv1.L1Metadata{Validated: true}}
	if m.Ensemble != nil {
		env.Governance.L2 = m.Ensemble.Vote(txHash, true)
	}
	if m.Principal != nil { // "mock" L3 mode
		env.Governance.L3 = m.Principal.Sign(txHash)
	}

	// 5. protojson is the canonical client-facing wire format (NOT encoding/json).
	wire, err := protojson.Marshal(env)
	if err != nil {
		return txHash, 0, nil, fmt.Errorf("protojson marshal: %w", err)
	}

	status, body, err = c.do(ctx, p, http.MethodPost, c.cfg.MTLSBaseURL+"/api/governance/envelope", wire)
	return txHash, status, body, err
}
