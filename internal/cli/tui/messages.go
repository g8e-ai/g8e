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

package tui

import (
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// PipelineStage identifies a layer in the 5-layer verification gauntlet.
type PipelineStage int

const (
	StageL1 PipelineStage = iota
	StageL2
	StageL3
	StageL4
	StageL5
)

func (s PipelineStage) String() string {
	switch s {
	case StageL1:
		return "L1: Technical Bedrock"
	case StageL2:
		return "L2: Consensus Consensus"
	case StageL3:
		return "L3: Notary"
	case StageL4:
		return "L4: Warden"
	case StageL5:
		return "L5: Actuator"
	default:
		return "Unknown"
	}
}

// PipelineStatus represents the visual state of a pipeline stage.
type PipelineStatus int

const (
	StatusIdle PipelineStatus = iota
	StatusActive
	StatusWaiting
	StatusPassed
	StatusFailed
)

func (s PipelineStatus) String() string {
	switch s {
	case StatusIdle:
		return "Pending"
	case StatusActive:
		return "Processing"
	case StatusWaiting:
		return "AWAITING FIDO2 KEY..."
	case StatusPassed:
		return "Passed"
	case StatusFailed:
		return "BLOCKED"
	default:
		return "Unknown"
	}
}

// LedgerLevel controls the color and tag of a ledger entry.
type LedgerLevel int

const (
	LevelInfo LedgerLevel = iota
	LevelWarn
	LevelCritical
)

func (l LedgerLevel) Tag() string {
	switch l {
	case LevelInfo:
		return "[INFO]"
	case LevelWarn:
		return "[WARN]"
	case LevelCritical:
		return "[CRIT]"
	default:
		return "[UNKNOWN]"
	}
}

// ConsensusResult represents the outcome of a consensus deliberation.
type ConsensusResult int

const (
	ConsensusPending ConsensusResult = iota
	ConsensusReached
	ConsensusRejected
)

func (c ConsensusResult) String() string {
	switch c {
	case ConsensusPending:
		return "PENDING"
	case ConsensusReached:
		return "CONSENSUS REACHED"
	case ConsensusRejected:
		return "CONSENSUS REJECTED"
	default:
		return "UNKNOWN"
	}
}

// PipelineMsg advances or updates a pipeline stage in the TUI.
type PipelineMsg struct {
	Stage  PipelineStage
	Status PipelineStatus
	TxID   string
	Detail string
}

// LedgerMsg appends a line to the Sovereign Audit Ledger pane.
type LedgerMsg struct {
	Level   LedgerLevel
	Message string
	Time    time.Time
}

// ConsensusMsg updates the consensus voting status.
type ConsensusMsg struct {
	Member   constants.ConsensusMember
	Decision bool
	Signed   bool
	Quorum   int
	Total    int
	Result   ConsensusResult
	Hash     string
}

// ConnStatus represents the SSE adapter connection state.
type ConnStatus int

const (
	ConnIdle ConnStatus = iota
	ConnConnecting
	ConnConnected
	ConnReconnecting
	ConnFailed
)

func (c ConnStatus) String() string {
	switch c {
	case ConnIdle:
		return "IDLE"
	case ConnConnecting:
		return "CONNECTING"
	case ConnConnected:
		return "CONNECTED"
	case ConnReconnecting:
		return "RECONNECTING"
	case ConnFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// ConnStatusMsg updates the TUI's connection status indicator.
type ConnStatusMsg struct {
	Status ConnStatus
	Detail string
}

// TickMsg drives the blink/pulse animation for waiting and failed states.
type TickMsg struct{}

// QuitMsg signals the TUI to exit cleanly.
type QuitMsg struct{}

// pipelineStageState is the per-stage state held in the Model.
type pipelineStageState struct {
	status PipelineStatus
	detail string
}

// ledgerEntry is a single line in the Sovereign Audit Ledger.
type ledgerEntry struct {
	level   LedgerLevel
	message string
	time    time.Time
}

// consensusMemberState is the per-member state in the consensus pane.
type consensusMemberState struct {
	name     constants.ConsensusMember
	decision bool
	signed   bool
}
