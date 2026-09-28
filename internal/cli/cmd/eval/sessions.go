// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

const (
	flagInferenceSession = "inference-session"
	flagDataSession      = "data-session"
)

type operatorRole string

const (
	operatorRoleInference operatorRole = "inference"
	operatorRoleData      operatorRole = "data"
)

// operatorCandidate is one active operator session that can serve a role.
type operatorCandidate struct {
	OperatorID string
	SessionID  string
}

// operatorSessions is the pair of operator sessions a dispatching command
// binds to. Fields for roles a command did not request are empty.
type operatorSessions struct {
	InferenceSessionID string
	DataSessionID      string
	DataOperatorID     string
}

// bindSessionFlags registers the operator-session pins once on the eval group.
// Subcommands read them through sessionPinsFromFlags.
func bindSessionFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String(flagInferenceSession, "", "Pin the inference Operator session (default: the single active one)")
	cmd.PersistentFlags().String(flagDataSession, "", "Pin the data Operator session (default: the single active one)")
}

func sessionPinsFromFlags(cmd *cobra.Command) (inference string, data string, err error) {
	inference, err = cmd.Flags().GetString(flagInferenceSession)
	if err != nil {
		return "", "", fmt.Errorf("evaluation: read --%s: %w", flagInferenceSession, err)
	}
	data, err = cmd.Flags().GetString(flagDataSession)
	if err != nil {
		return "", "", fmt.Errorf("evaluation: read --%s: %w", flagDataSession, err)
	}
	return strings.TrimSpace(inference), strings.TrimSpace(data), nil
}

// pickOperatorSession resolves one session for a role. A pinned session must be
// among the candidates. Otherwise exactly one candidate must exist.
func pickOperatorSession(role operatorRole, pinned string, candidates []operatorCandidate) (operatorCandidate, error) {
	if pinned != "" {
		for _, candidate := range candidates {
			if candidate.SessionID == pinned {
				return candidate, nil
			}
		}
		return operatorCandidate{}, fmt.Errorf("%w: no active %s operator session %q", constants.ErrOperatorSessionNotFound, role, pinned)
	}
	switch len(candidates) {
	case 0:
		return operatorCandidate{}, fmt.Errorf("%w: no active %s operator session", constants.ErrOperatorSessionNotFound, role)
	case 1:
		return candidates[0], nil
	default:
		ids := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			ids = append(ids, candidate.SessionID)
		}
		sort.Strings(ids)
		return operatorCandidate{}, fmt.Errorf("%w: %d active %s operator sessions (%s); pass --%s", constants.ErrOperatorSessionAmbiguous, len(ids), role, strings.Join(ids, ", "), flagForRole(role))
	}
}

func flagForRole(role operatorRole) string {
	if role == operatorRoleInference {
		return flagInferenceSession
	}
	return flagDataSession
}

func inferenceCandidates(operators []models.OperatorDocumentGo) []operatorCandidate {
	active := evaluation.ActiveInferenceOperators(operators)
	candidates := make([]operatorCandidate, 0, len(active))
	for _, op := range active {
		candidates = append(candidates, operatorCandidate{OperatorID: op.OperatorID, SessionID: op.OperatorSessionID})
	}
	return candidates
}

func dataCandidates(operators []models.OperatorDocumentGo) []operatorCandidate {
	active := evaluation.ActiveCampaignDataOperators(operators)
	candidates := make([]operatorCandidate, 0, len(active))
	for _, op := range active {
		candidates = append(candidates, operatorCandidate{OperatorID: op.OperatorID, SessionID: op.OperatorSessionID})
	}
	return candidates
}

// resolveOperatorSessions asks the Gateway for active operator sessions and
// resolves each requested role from the persistent --inference-session and
// --data-session pins. Only commands that dispatch work call it.
func resolveOperatorSessions(cmd *cobra.Command, deps nativeEvalDeps, roles ...operatorRole) (operatorSessions, error) {
	inferencePin, dataPin, err := sessionPinsFromFlags(cmd)
	if err != nil {
		return operatorSessions{}, err
	}
	chatDeps := deps.chatDeps()
	cfg, _, authContext, err := chatEvalEnvironment(cmd, chatDeps)
	if err != nil {
		return operatorSessions{}, err
	}
	operators, err := chatEvalListOperators(cmd, chatDeps, cfg, authContext)
	if err != nil {
		return operatorSessions{}, err
	}
	return resolveOperatorSessionsFrom(operators, inferencePin, dataPin, roles...)
}

func resolveOperatorSessionsFrom(operators []models.OperatorDocumentGo, inferencePin, dataPin string, roles ...operatorRole) (operatorSessions, error) {
	var sessions operatorSessions
	for _, role := range roles {
		switch role {
		case operatorRoleInference:
			picked, err := pickOperatorSession(role, inferencePin, inferenceCandidates(operators))
			if err != nil {
				return operatorSessions{}, err
			}
			sessions.InferenceSessionID = picked.SessionID
		case operatorRoleData:
			picked, err := pickOperatorSession(role, dataPin, dataCandidates(operators))
			if err != nil {
				return operatorSessions{}, err
			}
			sessions.DataSessionID = picked.SessionID
			sessions.DataOperatorID = picked.OperatorID
		default:
			return operatorSessions{}, fmt.Errorf("evaluation: unknown operator role %q", role)
		}
	}
	return sessions, nil
}
