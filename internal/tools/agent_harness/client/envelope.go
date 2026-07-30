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
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	// REAL g8e packages — not forks. This is what guarantees the transaction
	// hash and wire format match the verifier. GovernanceEnvelope is the alias for
	// g8e.common.v1.GovernanceEnvelope; GenerateMessageID is the canonical hasher.

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// Canonical GovernanceEnvelope action types. SCREAMING_SNAKE per protocol/constants. Pinned
// here so Agent Harness has no internal/ import; verify against MapEventTypeToActionType.
const (
	ActionMcpCall   = "MCP_CALL"
	ActionA2aCall   = "A2A_CALL"
	ProtocolVersion = "1.0"
)

// Ensemble is Agent Harness's L2 consensus voter pool: N agents that each
// vote on a transaction hash. When memberKeys is populated (per-member seed
// mode), each vote is signed with the member's own Ed25519 private key and
// the corresponding public key is registered as a trusted signer for that
// member. When memberKeys is nil (single-key mode), all votes share one
// signature from the ensemble key.
type Ensemble struct {
	KeyID        string
	ConsensusID  string
	MemberKeyIDs []string // when set, Vote produces one vote per member key ID
	priv         ed25519.PrivateKey
	pub          ed25519.PublicKey
	agents       []string
	memberKeys   map[string]ed25519.PrivateKey // per-member private keys (optional)
	memberPubs   map[string]ed25519.PublicKey  // per-member public keys (optional)
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
	return &Ensemble{KeyID: keyID, ConsensusID: "test-consensus", priv: priv, pub: pub, agents: agents}, nil
}

// NewEnsembleFromSeed constructs an Ensemble from a hex-encoded Ed25519 seed.
// This enables deterministic key generation for demo consensus bootstrap: the
// gateway seeds the trusted signer from the same seed file, and the harness
// reconstructs the same private key to sign L2 votes that verify against it.
func NewEnsembleFromSeed(keyID string, n int, seedHex string) (*Ensemble, error) {
	seed, err := hex.DecodeString(strings.TrimSpace(seedHex))
	if err != nil {
		return nil, fmt.Errorf("ensemble from seed: decode hex: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("ensemble from seed: %w: got %d, expected %d", constants.ErrInvalidSeedLength, len(seed), ed25519.SeedSize)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	agents := make([]string, n)
	for i := range agents {
		agents[i] = fmt.Sprintf("%s-agent-%d", keyID, i+1)
	}
	return &Ensemble{KeyID: keyID, ConsensusID: "test-consensus", priv: priv, pub: pub, agents: agents}, nil
}

// NewEnsembleFromMemberSeeds constructs an Ensemble where each member has its
// own Ed25519 key pair derived from a distinct seed. This makes RequireDistinct
// and quorum cryptographically meaningful: a single key cannot forge multiple
// votes. memberSeeds maps member app ID to hex-encoded Ed25519 seed.
func NewEnsembleFromMemberSeeds(keyID, consensusID string, memberSeeds map[string]string) (*Ensemble, error) {
	memberKeys := make(map[string]ed25519.PrivateKey, len(memberSeeds))
	memberPubs := make(map[string]ed25519.PublicKey, len(memberSeeds))
	memberKeyIDs := make([]string, 0, len(memberSeeds))

	for appID, seedHex := range memberSeeds {
		seed, err := hex.DecodeString(strings.TrimSpace(seedHex))
		if err != nil {
			return nil, fmt.Errorf("ensemble from member seeds: decode hex for %s: %w", appID, err)
		}
		if len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("ensemble from member seeds: %w for %s: got %d, expected %d", constants.ErrInvalidSeedLength, appID, len(seed), ed25519.SeedSize)
		}
		priv := ed25519.NewKeyFromSeed(seed)
		pub := priv.Public().(ed25519.PublicKey)
		memberKeys[appID] = priv
		memberPubs[appID] = pub
		memberKeyIDs = append(memberKeyIDs, appID)
	}

	return &Ensemble{
		KeyID:        keyID,
		ConsensusID:  consensusID,
		MemberKeyIDs: memberKeyIDs,
		memberKeys:   memberKeys,
		memberPubs:   memberPubs,
	}, nil
}

// MemberPubHex returns the hex-encoded public key for a specific member.
// Returns empty string if the member is not found or per-member keys are not
// in use.
func (e *Ensemble) MemberPubHex(appID string) string {
	if e.memberPubs == nil {
		return ""
	}
	pub, ok := e.memberPubs[appID]
	if !ok {
		return ""
	}
	return hex.EncodeToString(pub)
}

// HasPerMemberKeys returns true when the ensemble uses distinct per-member
// keys rather than a single shared key.
func (e *Ensemble) HasPerMemberKeys() bool {
	return len(e.memberKeys) > 0
}

// PubHex is the consensus public key for trusted-signer registration.
func (e *Ensemble) PubHex() string { return hex.EncodeToString(e.pub) }

// SeedHex returns the hex-encoded Ed25519 seed for persistence (demo bootstrap).
func (e *Ensemble) SeedHex() string { return hex.EncodeToString(e.priv.Seed()) }

// AgentCount is the number of mock voters in the ensemble.
func (e *Ensemble) AgentCount() int { return len(e.agents) }

// Vote produces the L2 metadata for a decision over the transaction hash.
// decision==true means "the ensemble agreed this mutation is safe."
// When per-member keys are in use, each vote is signed with the member's own
// private key. Otherwise, all votes share the ensemble's single signature.
func (e *Ensemble) Vote(txHash string, decision bool) *commonv1.L2Metadata {
	basis := fmt.Sprintf("%s|%v", txHash, decision) // matches l2_consensus.SignDecision

	var votes []*commonv1.L2Vote
	if len(e.MemberKeyIDs) > 0 {
		votes = make([]*commonv1.L2Vote, 0, len(e.MemberKeyIDs))
		for _, keyID := range e.MemberKeyIDs {
			var sigHex string
			if e.memberKeys != nil {
				if mk, ok := e.memberKeys[keyID]; ok {
					sig := ed25519.Sign(mk, []byte(basis))
					sigHex = hex.EncodeToString(sig)
				}
			} else {
				sig := ed25519.Sign(e.priv, []byte(basis))
				sigHex = hex.EncodeToString(sig)
			}
			votes = append(votes, &commonv1.L2Vote{
				SignerKeyId:        keyID,
				ConsensusSignature: sigHex,
				Decision:           decision,
			})
		}
	} else {
		sig := ed25519.Sign(e.priv, []byte(basis))
		sigHex := hex.EncodeToString(sig)
		votes = []*commonv1.L2Vote{{
			SignerKeyId:        e.KeyID,
			ConsensusSignature: sigHex,
			Decision:           decision,
		}}
	}

	return &commonv1.L2Metadata{
		ConsensusSetId: e.ConsensusID,
		Votes:          votes,
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
	OperatorID        string
	OperatorSessionID string
	ToolName          string
	ArgumentsJSON     string
	TargetResource    string
	StateRoot         string
	Ensemble          *Ensemble  // attach L2 when non-nil
	Principal         *Principal // attach mock L3 when non-nil ("mock" mode)
	Decision          *bool      // nil or *true = affirmative; *false = veto
	TTL               time.Duration
}

// SubmitEnvelope submits a pre-built GovernanceEnvelope directly to the admission API.
// This is used when the caller has already constructed the envelope (e.g., from fixtures).
func (c *Client) SubmitEnvelope(ctx context.Context, p Persona, envelope *commonv1.GovernanceEnvelope) (status int, body []byte, err error) {
	wire, err := protojson.Marshal(envelope)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %w", constants.ErrPubSubMarshalEnvelope, err)
	}

	status, body, err = c.do(ctx, p, http.MethodPost, c.cfg.MTLSBaseURL+constants.APIPaths.GovernanceEnvelopes, wire)
	return status, body, err
}

// SubmitMaximal builds a real GovernanceEnvelope wrapping an MCP_CALL, computes the
// canonical transaction hash with g8e's own hasher, attaches L2 (and optionally
// mock L3), marshals it as protojson, and POSTs it to the admission API.
// Returns the computed hash so callers can drive the OOB approve flow.
func (c *Client) SubmitMaximal(ctx context.Context, p Persona, m MaximalEnvelope) (txHash string, status int, body []byte, err error) {
	// 1. Typed protobuf payload — the "what". The Operator decodes this exact type.
	call := &operatorv1.McpCallRequested{
		ToolName:      m.ToolName,
		ArgumentsJson: m.ArgumentsJSON,
		ExecutionId:   fmt.Sprintf("agent-harness-%d", time.Now().UnixNano()),
	}
	payloadBytes, err := proto.Marshal(call)
	if err != nil {
		return "", 0, nil, fmt.Errorf("%w: %w", constants.ErrRequestMarshalFailed, err)
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
		ProtocolVersion:   ProtocolVersion,
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(ttl)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        m.OperatorID,
		OperatorSessionId: m.OperatorSessionID,
		ActionType:        ActionMcpCall,
		TargetResource:    m.TargetResource,
		EventType:         "operator.mcp.call.requested",
		Payload:           payloadBytes,
		IntentData:        intent,
		StateMerkleRoot:   m.StateRoot,
		Nonce:             hex.EncodeToString(nonce),
	}

	// 3. Canonical hash — REAL hasher. id == transaction_hash == SHA256(canonical).
	txHash, err = governance.GenerateMessageID(env)
	if err != nil {
		return "", 0, nil, fmt.Errorf("%w: %w", constants.ErrTxTransactionHashMissing, err)
	}
	env.Id = txHash
	env.TransactionHash = txHash

	// 4. Governance proofs over the hash.
	env.Governance = &commonv1.GovernanceMetadata{L1: &commonv1.L1Metadata{Validated: true}}
	if m.Ensemble != nil {
		decision := true
		if m.Decision != nil {
			decision = *m.Decision
		}
		env.Governance.L2 = m.Ensemble.Vote(txHash, decision)
	}
	if m.Principal != nil { // "mock" L3 mode
		env.Governance.L3 = m.Principal.Sign(txHash)
	}

	// 5. protojson is the canonical client-facing wire format (NOT encoding/json).
	wire, err := protojson.Marshal(env)
	if err != nil {
		return txHash, 0, nil, fmt.Errorf("%w: %w", constants.ErrPubSubMarshalEnvelope, err)
	}

	status, body, err = c.do(ctx, p, http.MethodPost, c.cfg.MTLSBaseURL+constants.APIPaths.GovernanceEnvelopes, wire)
	return txHash, status, body, err
}
