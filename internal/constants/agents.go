// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// TriageComplexity is a typed string for triage complexity levels.
type TriageComplexity string

const (
	TriageComplexitySimple  TriageComplexity = "simple"
	TriageComplexityComplex TriageComplexity = "complex"
)

// TriageConfidence is a typed string for triage confidence levels.
type TriageConfidence string

const (
	TriageConfidenceHigh TriageConfidence = "high"
	TriageConfidenceLow  TriageConfidence = "low"
)

// TriageIntent is a typed string for triage intent classifications.
type TriageIntent string

const (
	TriageIntentInformation TriageIntent = "information"
	TriageIntentAction      TriageIntent = "action"
	TriageIntentUnknown     TriageIntent = "unknown"
)

// TriagePosture is a typed string for triage posture classifications.
type TriagePosture string

const (
	TriagePostureNormal      TriagePosture = "normal"
	TriagePostureEscalated   TriagePosture = "escalated"
	TriagePostureAdversarial TriagePosture = "adversarial"
	TriagePostureConfused    TriagePosture = "confused"
)

// AgentName is a typed string for agent persona identifiers.
type AgentName string

const (
	AgentNameSage AgentName = "sage"
	AgentNameDash AgentName = "dash"
)

// AgentBinary is a typed string for external AI agent binary identifiers.
type AgentBinary string

const (
	AgentBinaryClaude AgentBinary = "claude"
	AgentBinaryCodex  AgentBinary = "codex"
	AgentBinaryDevin  AgentBinary = "devin"
	AgentBinaryGemini AgentBinary = "gemini"
	AgentBinaryGoose  AgentBinary = "goose"
)
