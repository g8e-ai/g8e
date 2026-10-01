// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

// formationHandoffTruncationMarker ends a handoff turn whose role output did
// not fit the seed text bound.
const formationHandoffTruncationMarker = "\n[output truncated]"

// formationHandoffRoles is the order roles run in, and so the order handoff
// turns are seeded.
var formationHandoffRoles = []FormationRole{FormationRoleLite, FormationRoleAssistant, FormationRolePrimary}

// formationHandoffTurnPrefix opens the seeded conversation turn that carries
// one completed role's designated output to the roles after it.
func formationHandoffTurnPrefix(role FormationRole) string {
	return "[" + string(role) + " output]\n"
}

// formationHandoffTurnContent renders one completed role's designated output as
// the text of a seeded assistant turn. Output beyond the seed text bound is cut
// and marked, so a verbose model cannot turn the next role's request into a
// seed rejection. Grading recomputes this text from the role's recorded output
// to verify the handoff, so it must stay a pure function of (role, output).
func formationHandoffTurnContent(role FormationRole, output string) string {
	prefix := formationHandoffTurnPrefix(role)
	budget := harnessclient.EnsembleSeedMaxText - utf8.RuneCountInString(prefix)
	if utf8.RuneCountInString(output) > budget {
		keep := budget - utf8.RuneCountInString(formationHandoffTruncationMarker)
		output = string([]rune(output)[:keep]) + formationHandoffTruncationMarker
	}
	return prefix + output
}

// formationRoleSeed builds the seed for one role's request: the scenario's
// frozen seed rendered through the workspace, followed by one assistant turn
// per already-completed role. Prior outputs live in the investigation the role
// runs in, not in the user message, so the scored turn is the same prompt every
// role sees. A scenario without a seed cannot carry a handoff.
func formationRoleSeed(input ScenarioInputFixture, ws *ScenarioWorkspace, prior []formationRoleOutput) (*harnessclient.EnsembleInvestigationSeed, error) {
	seed := buildHarnessInvestigationSeed(&input.Seed, ws)
	if seed == nil {
		if len(prior) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: scenario %s has no investigation seed to carry the role handoff", constants.ErrEvaluationScenarioContractInvalid, input.ScenarioID)
	}
	for _, output := range prior {
		seed.Turns = append(seed.Turns, harnessclient.EnsembleSeedTurn{
			Sender:  harnessclient.EnsembleSeedSenderAssistant,
			Content: formationHandoffTurnContent(output.Role, output.Output),
		})
	}
	return seed, nil
}

// traceHandoffTurnContents returns, in order, the seeded turns of a role trace
// that carry another role's output.
func traceHandoffTurnContents(trace EvaluationTrace) ([]string, error) {
	seed, err := decodeTraceSeed(trace)
	if err != nil {
		return nil, err
	}
	var handoffs []string
	for _, turn := range seed.Turns {
		if turn.Sender != harnessclient.EnsembleSeedSenderAssistant {
			continue
		}
		for _, role := range formationHandoffRoles {
			if strings.HasPrefix(turn.Content, formationHandoffTurnPrefix(role)) {
				handoffs = append(handoffs, turn.Content)
				break
			}
		}
	}
	return handoffs, nil
}
