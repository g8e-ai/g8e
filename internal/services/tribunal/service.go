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

package tribunal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	govsvc "github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// TribunalService is the enrolled agentic application that deliberates on
// governance envelopes and produces L2 consensus votes.
//
// Each member is a distinct enrolled principal with its own Ed25519 key.
// For single-binary deployments, members run in-process but never share
// the gateway identity key.
type TribunalService struct {
	tribunalID string
	members    []TribunalMember
	doctrine   *govsvc.L1Doctrine
	logger     *slog.Logger
	responder  *response.Writer
}

// NewTribunalService creates a new Tribunal service.
//
// tribunalID is the TribunalPolicy.ID this service represents.
// members must contain at least one member with a non-nil PrivateKey.
// doctrine is the L1Doctrine used for deterministic evaluation by all members.
func NewTribunalService(tribunalID string, members []TribunalMember, doctrine *govsvc.L1Doctrine, logger *slog.Logger, responder *response.Writer) *TribunalService {
	return &TribunalService{
		tribunalID: tribunalID,
		members:    members,
		doctrine:   doctrine,
		logger:     logger,
		responder:  responder,
	}
}

// TribunalID returns the tribunal policy ID this service represents.
func (s *TribunalService) TribunalID() string {
	return s.tribunalID
}

// DeliberateResult is the output of a deliberation: the envelope with L2
// votes populated, ready for the caller to submit to the gateway.
type DeliberateResult struct {
	Envelope *governance.GovernanceEnvelope
}

// Deliberate processes a GovernanceEnvelope through all tribunal members.
// Each member independently evaluates the payload and signs a vote.
// The envelope is returned with L2 metadata populated (tribunal_id + votes).
//
// Fail-closed: if the envelope id does not match the recomputed hash,
// the envelope is rejected with ErrTribunalHashMismatch.
func (s *TribunalService) Deliberate(env *governance.GovernanceEnvelope) (*DeliberateResult, error) {
	expectedHash, err := governance.GenerateMessageID(env)
	if err != nil {
		return nil, fmt.Errorf("tribunal: deliberate: %w", err)
	}
	if env.Id != expectedHash {
		return nil, constants.ErrTribunalHashMismatch
	}

	cmdData, intent, err := extractCommandData(env)
	if err != nil {
		return nil, fmt.Errorf("tribunal: deliberate: %w", err)
	}

	votes := make([]*commonv1.L2Vote, 0, len(s.members))
	for _, member := range s.members {
		isSafe := s.evaluateSafety(s.doctrine, env.TargetResource, cmdData, intent)

		sig, err := signDecision(member.PrivateKey, env.Id, isSafe)
		if err != nil {
			return nil, fmt.Errorf("tribunal: deliberate: member %s: %w", member.AppID, err)
		}

		votes = append(votes, &commonv1.L2Vote{
			SignerKeyId:        member.AppID,
			ConsensusSignature: sig,
			Decision:           isSafe,
		})
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
	env.Governance.L2.TribunalId = s.tribunalID
	env.Governance.L2.Votes = votes

	return &DeliberateResult{Envelope: env}, nil
}

// HandleDeliberate is the mTLS-guarded HTTP handler for POST /tribunal/v1/deliberate.
// It accepts a canonical-JSON GovernanceEnvelope and returns the envelope with
// L2 consensus votes populated.
func (s *TribunalService) HandleDeliberate(w http.ResponseWriter, r *http.Request) {
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
		if errors.Is(err, constants.ErrTribunalHashMismatch) {
			s.responder.Error(w, http.StatusBadRequest, constants.ErrTribunalHashMismatch.Error())
			return
		}
		s.logger.Error("tribunal: deliberate failed", "error", err)
		s.responder.Error(w, http.StatusInternalServerError, "deliberation failed")
		return
	}

	signedBytes, err := protojson.Marshal(result.Envelope)
	if err != nil {
		s.logger.Error("tribunal: marshal signed envelope", "error", err)
		s.responder.Error(w, http.StatusInternalServerError, "failed to marshal response")
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)
	w.Header().Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(signedBytes); err != nil {
		s.logger.Error("tribunal: write response", "error", err)
	}
}

// LocalDeliberator is an in-process adapter that satisfies the
// mcp.TribunalDeliberator interface by calling TribunalService.Deliberate
// directly, without an HTTP round-trip. This is used when the Tribunal runs
// in the same process as the gateway (single-binary deployment).
type LocalDeliberator struct {
	service *TribunalService
}

// NewLocalDeliberator creates a local (in-process) Tribunal deliberator.
func NewLocalDeliberator(s *TribunalService) *LocalDeliberator {
	return &LocalDeliberator{service: s}
}

// Deliberate unmarshals the envelope bytes, runs deliberation, and returns
// the marshaled envelope with L2 votes populated.
func (d *LocalDeliberator) Deliberate(_ context.Context, envelopeBytes []byte) ([]byte, error) {
	var env governance.GovernanceEnvelope
	if err := protojson.Unmarshal(envelopeBytes, &env); err != nil {
		return nil, fmt.Errorf("tribunal local deliberator: unmarshal: %w", err)
	}
	result, err := d.service.Deliberate(&env)
	if err != nil {
		return nil, fmt.Errorf("tribunal local deliberator: %w", err)
	}
	out, err := (protojson.MarshalOptions{Multiline: false}).Marshal(result.Envelope)
	if err != nil {
		return nil, fmt.Errorf("tribunal local deliberator: marshal: %w", err)
	}
	return out, nil
}
