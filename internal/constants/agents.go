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
	AgentBinaryGemini AgentBinary = "gemini"
	AgentBinaryGoose  AgentBinary = "goose"
)
