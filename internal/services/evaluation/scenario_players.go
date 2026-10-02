// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// PlayerID is the g8ee persona id a trace's player step carries
// (ensemble/app/models/personas). A scored turn runs the real g8ee chain, and
// every player that does its job in it is graded on that job.
type PlayerID string

const (
	PlayerTriage         PlayerID = "triage"
	PlayerSage           PlayerID = "sage"
	PlayerDash           PlayerID = "dash"
	PlayerTribunal       PlayerID = "tribunal" // the deterministic vote over the five seats' candidates
	PlayerMarshalCommand PlayerID = "marshal_command"
	PlayerAuditor        PlayerID = "auditor"
	PlayerCodex          PlayerID = "codex"

	// PlayerGradePrefix starts the criterion id of every player grade.
	PlayerGradePrefix = "player:"
)

// The five Tribunal seats are the ConsensusMember personas.
const (
	SeatAxiom    = PlayerID(constants.ConsensusMemberAxiom)
	SeatConcord  = PlayerID(constants.ConsensusMemberConcord)
	SeatVariance = PlayerID(constants.ConsensusMemberVariance)
	SeatPragma   = PlayerID(constants.ConsensusMemberPragma)
	SeatNemesis  = PlayerID(constants.ConsensusMemberNemesis)
)

// tribunalSeats are the five independent generation passes, in seat order.
// Nemesis is the calibrated adversary: it may introduce a plausible flaw, so
// it is held to safety and never to the scenario's required command terms.
var tribunalSeats = []PlayerID{SeatAxiom, SeatConcord, SeatVariance, SeatPragma, SeatNemesis}

// MarshalRisk is the risk level Marshal assigns a command (g8ee RiskLevel).
type MarshalRisk string

const (
	MarshalRiskLow    MarshalRisk = "LOW"
	MarshalRiskMedium MarshalRisk = "MEDIUM"
	MarshalRiskHigh   MarshalRisk = "HIGH"
)

// PlayerGradeCriterionID is the criterion id of one player's grade.
func PlayerGradeCriterionID(player PlayerID) string {
	return PlayerGradePrefix + string(player)
}

// PlayerFromCriterionID returns the player a criterion id grades, or false when
// the id is not a player grade.
func PlayerFromCriterionID(criterionID string) (PlayerID, bool) {
	player, ok := strings.CutPrefix(criterionID, PlayerGradePrefix)
	return PlayerID(player), ok && player != ""
}

// PlayerTier is the model tier a player's calls resolve from in g8ee: Sage and
// the Auditor run on the primary slot, Dash on the assistant slot, and every
// other player (Triage, the Tribunal seats and vote, Marshal, Codex) on the
// lite slot. A tier's result is the pass rate over the players of that tier.
func PlayerTier(player PlayerID) FormationRole {
	switch player {
	case PlayerSage, PlayerAuditor:
		return FormationRolePrimary
	case PlayerDash:
		return FormationRoleAssistant
	default:
		return FormationRoleLite
	}
}

// PlayerForDesignatedRole is the reasoning persona a designated role activates
// in g8ee: the primary tier is Sage, every other tier runs Dash.
func PlayerForDesignatedRole(role FormationRole) PlayerID {
	if role == FormationRolePrimary {
		return PlayerSage
	}
	return PlayerDash
}

// TriageExpectation is what Triage must classify for the scenario's user
// message. A field left empty is not graded; a listed value is accepted.
type TriageExpectation struct {
	Complexity []constants.TriageComplexity `json:"complexity,omitempty"`
	Intent     []constants.TriageIntent     `json:"intent,omitempty"`
	Posture    []constants.TriagePosture    `json:"posture,omitempty"`
}

// MarshalExpectation is the risk Marshal may assign the command the Tribunal
// chose, when that command is the one the scenario anticipates. A command that
// breaks the scenario's command expectation is judged against MEDIUM or HIGH
// instead: a Marshal that waves a dangerous command through is wrong whatever
// the scenario says about the intended one.
type MarshalExpectation struct {
	Risk []MarshalRisk `json:"risk"`
}

// ScenarioPlayerExpectations is the per-player gold for a scenario that runs
// the real chain. A nil expectation leaves that player ungraded for the
// scenario; a player the chain never reached (no host command reached the
// Tribunal, say) is not graded either, and the persona's own grade says why.
type ScenarioPlayerExpectations struct {
	// Triage is what Triage must classify.
	Triage *TriageExpectation `json:"triage,omitempty"`
	// Command is the contract the Tribunal's commands must meet: required and
	// forbidden terms, matched case-insensitively against each command. It
	// applies to the four honest seats, the vote's winner, and the command the
	// Auditor lets through. Nemesis is held only to its forbidden terms.
	Command *ScenarioContentCheck `json:"command,omitempty"`
	// Marshal is the risk band for an acceptable command; see MarshalExpectation.
	Marshal *MarshalExpectation `json:"marshal,omitempty"`
	// Codex is the contract Codex's memory text must meet, in addition to the
	// built-in redaction check (no IP addresses or credentials).
	Codex *ScenarioContentCheck `json:"codex,omitempty"`
}
