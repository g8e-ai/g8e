// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package consensus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/response"
	govsvc "github.com/g8e-ai/g8e/v2/internal/services/governance"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// ConsensusService is the enrolled agentic application that deliberates on
// governance envelopes and produces L2 consensus votes.
//
// Each member is a distinct enrolled principal with its own Ed25519 key.
// For single-binary deployments, members run in-process but never share
// the gateway identity key.
type ConsensusService struct {
	consensusID string
	members     []ConsensusMember
	doctrine    *govsvc.L1Doctrine
	logger      *slog.Logger
	responder   *response.Writer
}

// NewConsensusService creates a new Consensus service.
//
// consensusID is the ConsensusPolicy.ID this service represents.
// members must contain at least one member with a non-nil PrivateKey.
// doctrine is the L1Doctrine used for deterministic evaluation by all members.
func NewConsensusService(consensusID string, members []ConsensusMember, doctrine *govsvc.L1Doctrine, logger *slog.Logger, responder *response.Writer) *ConsensusService {
	return &ConsensusService{
		consensusID: consensusID,
		members:     members,
		doctrine:    doctrine,
		logger:      logger,
		responder:   responder,
	}
}

// ConsensusID returns the consensus policy ID this service represents.
func (s *ConsensusService) ConsensusID() string {
	return s.consensusID
}

// DeliberateResult is the output of a deliberation: the envelope with L2
// votes populated, ready for the caller to submit to the gateway.
type DeliberateResult struct {
	Envelope *governance.GovernanceEnvelope
}

// Deliberate processes a GovernanceEnvelope through all consensus members.
// Each member independently evaluates the payload and signs a vote.
// The envelope is returned with L2 metadata populated (consensus_id + votes).
//
// Fail-closed: if the envelope id does not match the recomputed hash,
// the envelope is rejected with ErrConsensusHashMismatch.
func (s *ConsensusService) Deliberate(env *governance.GovernanceEnvelope) (*DeliberateResult, error) {
	expectedHash, err := governance.GenerateMessageID(env)
	if err != nil {
		return nil, fmt.Errorf("consensus: deliberate: %w", err)
	}
	if env.Id != expectedHash {
		return nil, constants.ErrConsensusHashMismatch
	}

	cmdData, err := extractCommandData(env)
	if err != nil {
		return nil, fmt.Errorf("consensus: deliberate: %w", err)
	}

	votes := make([]*commonv1.L2Vote, 0, len(s.members))
	for _, member := range s.members {
		if member.PrivateKey == nil {
			continue
		}

		isSafe := s.evaluateSafety(s.doctrine, cmdData)

		sig, err := signDecision(member.PrivateKey, env.Id, isSafe)
		if err != nil {
			return nil, fmt.Errorf("consensus: deliberate: member %s: %w", member.AppID, err)
		}

		votes = append(votes, &commonv1.L2Vote{
			SignerKeyId:        member.AppID,
			ConsensusSignature: sig,
			Decision:           isSafe,
		})
	}

	if len(votes) == 0 {
		return nil, constants.ErrConsensusNoSigningMembers
	}

	if env.Governance == nil {
		env.Governance = &commonv1.GovernanceMetadata{
			L1: &commonv1.L1Metadata{},
			L2: &commonv1.L2Metadata{},
			L3: &commonv1.L3Metadata{},
		}
	}
	if env.Governance.L2 == nil {
		env.Governance.L2 = &commonv1.L2Metadata{}
	}
	env.Governance.L2.ConsensusSetId = s.consensusID
	env.Governance.L2.Votes = votes

	return &DeliberateResult{Envelope: env}, nil
}

// HandleDeliberate is the mTLS-guarded HTTP handler for POST /consensus/v1/deliberate.
// It accepts a canonical-JSON GovernanceEnvelope and returns the envelope with
// L2 consensus votes populated.
func (s *ConsensusService) HandleDeliberate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		s.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var env governance.GovernanceEnvelope
	if err := protojson.Unmarshal(body, &env); err != nil {
		s.responder.Error(w, http.StatusBadRequest, "invalid envelope JSON")
		return
	}

	result, err := s.Deliberate(&env)
	if err != nil {
		if errors.Is(err, constants.ErrConsensusHashMismatch) {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrConsensusHashMismatch.Error())
			return
		}
		s.logger.Error("consensus: deliberate failed", "error", err)
		s.responder.Error(w, http.StatusInternalServerError, "deliberation failed")
		return
	}

	signedBytes, err := protojson.Marshal(result.Envelope)
	if err != nil {
		s.logger.Error("consensus: marshal signed envelope", "error", err)
		s.responder.Error(w, http.StatusInternalServerError, "failed to marshal response")
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(signedBytes); err != nil {
		s.logger.Error("consensus: write response", "error", err)
	}
}

// LocalDeliberator is an in-process adapter that satisfies the
// mcp.L2ConsensusDeliberator interface by calling ConsensusService.Deliberate
// directly, without an HTTP round-trip. This is used when the Consensus runs
// in the same process as the gateway (single-binary deployment).
type LocalDeliberator struct {
	service *ConsensusService
}

// NewLocalDeliberator creates a local (in-process) Consensus deliberator.
func NewLocalDeliberator(s *ConsensusService) *LocalDeliberator {
	return &LocalDeliberator{service: s}
}

// Deliberate unmarshals the envelope bytes, runs deliberation, and returns
// the marshaled envelope with L2 votes populated.
func (d *LocalDeliberator) Deliberate(_ context.Context, envelopeBytes []byte) ([]byte, error) {
	var env governance.GovernanceEnvelope
	if err := protojson.Unmarshal(envelopeBytes, &env); err != nil {
		return nil, fmt.Errorf("consensus local deliberator: unmarshal: %w", err)
	}
	result, err := d.service.Deliberate(&env)
	if err != nil {
		return nil, fmt.Errorf("consensus local deliberator: %w", err)
	}
	out, err := (protojson.MarshalOptions{Multiline: false}).Marshal(result.Envelope)
	if err != nil {
		return nil, fmt.Errorf("consensus local deliberator: marshal: %w", err)
	}
	return out, nil
}
