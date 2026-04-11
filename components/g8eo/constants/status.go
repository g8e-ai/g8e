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

// Status constants mirror shared/constants/status.json (the source of truth).
// g8eo duplicates these as compile-time Go constants (no runtime loading).
// Contract tests in contracts/shared_constants_test.go enforce alignment.

// ExecutionStatus is a typed string for command/file/fs execution terminal states.
// Values mirror shared/constants/status.json execution.status.
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusExecuting ExecutionStatus = "executing"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusTimeout   ExecutionStatus = "timeout"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
)

// StreamStatus is a typed string for the stream subcommand's per-host and summary JSON wire output.
type StreamStatus string

const (
	StreamStatusCompleted StreamStatus = "completed"
	StreamStatusFailed    StreamStatus = "failed"
	StreamStatusCancelled StreamStatus = "cancelled"
	StreamStatusExited    StreamStatus = "exited"
	StreamStatusSummary   StreamStatus = "summary"
)

// HeartbeatType is a typed string for the reason a heartbeat was sent.
// Values mirror shared/constants/status.json heartbeat.type.
type HeartbeatType string

const (
	HeartbeatTypeAutomatic HeartbeatType = "automatic"
	HeartbeatTypeBootstrap HeartbeatType = "bootstrap"
	HeartbeatTypeRequested HeartbeatType = "requested"
)

// authModeValues defines the supported operator authentication modes.
type authModeValues struct {
	APIKey          string
	OperatorSession string
}

// operatorStatusValues mirrors shared/constants/status.json g8e.status.
type operatorStatusValues struct {
	Available   string
	Unavailable string
	Offline     string
	Bound       string
	Stale       string
	Active      string
	Stopped     string
	Terminated  string
}

// operatorTypeValues mirrors shared/constants/status.json g8e.type.
type operatorTypeValues struct {
	System string
	Cloud  string
}

// cloudSubtypeValues mirrors shared/constants/status.json cloud.subtype.
type cloudSubtypeValues struct {
	AWS     string
	GCP     string
	Azure   string
	G8eNode string
}

// vaultModeValues mirrors shared/constants/status.json vault.mode.
type vaultModeValues struct {
	Raw      string
	Scrubbed string
}

// versionStabilityValues mirrors shared/constants/status.json version.stability.
type versionStabilityValues struct {
	Stable string
	Beta   string
	Dev    string
}

// componentNameValues mirrors shared/constants/status.json component.name.
type componentNameValues struct {
	G8EE string
	G8EO string
	G8ED string
}

// platformValues mirrors shared/constants/status.json platform.
type platformValues struct {
	Linux   string
	Windows string
	Darwin  string
}

// aiSourceValues mirrors shared/constants/status.json ai.source.
type aiSourceValues struct {
	Tool             string
	TerminalAnchored string
	TerminalDirect   string
}

// aiTaskIDValues mirrors shared/constants/status.json ai.task.id.
type aiTaskIDValues struct {
	Command          string
	DirectCommand    string
	FileEdit         string
	FsList           string
	FsRead           string
	PortCheck        string
	FetchLogs        string
	FetchHistory     string
	FetchFileHistory string
	RestoreFile      string
	FetchFileDiff    string
}

// listenModeValues holds HTTP response constants for the g8es listen-mode HTTP API.
type listenModeValues struct {
	StatusOK string
	Mode     string
}

// statusValues is the top-level namespace. Canonical values from shared/constants/status.json.
type statusValues struct {
	AuthMode         authModeValues
	ListenMode       listenModeValues
	OperatorStatus   operatorStatusValues
	OperatorType     operatorTypeValues
	CloudSubtype     cloudSubtypeValues
	VaultMode        vaultModeValues
	VersionStability versionStabilityValues
	ComponentName    componentNameValues
	Platform         platformValues
	AISource         aiSourceValues
	AITaskID         aiTaskIDValues
}

// Status is the package-level entry point for all status constants.
// Usage: constants.Status.OperatorStatus.Bound
var Status = statusValues{
	AuthMode: authModeValues{
		APIKey:          "api_key",
		OperatorSession: "operator_session",
	},
	ListenMode: listenModeValues{
		StatusOK: "ok",
		Mode:     "listen",
	},
	OperatorStatus: operatorStatusValues{
		Available:   "available",
		Unavailable: "unavailable",
		Offline:     "offline",
		Bound:       "bound",
		Stale:       "stale",
		Active:      "active",
		Stopped:     "stopped",
		Terminated:  "terminated",
	},
	OperatorType: operatorTypeValues{
		System: "system",
		Cloud:  "cloud",
	},
	CloudSubtype: cloudSubtypeValues{
		AWS:     "aws",
		GCP:     "gcp",
		Azure:   "azure",
		G8eNode: "g8e_pod",
	},
	VaultMode: vaultModeValues{
		Raw:      "raw",
		Scrubbed: "scrubbed",
	},
	VersionStability: versionStabilityValues{
		Stable: "stable",
		Beta:   "beta",
		Dev:    "dev",
	},
	ComponentName: componentNameValues{
		G8EE: "g8ee",
		G8EO: "g8eo",
		G8ED: "g8ed",
	},
	Platform: platformValues{
		Linux:   "linux",
		Windows: "windows",
		Darwin:  "darwin",
	},
	AISource: aiSourceValues{
		Tool:             "ai.tool.call",
		TerminalAnchored: "ai.terminal.anchored",
		TerminalDirect:   "ai.terminal.direct",
	},
	AITaskID: aiTaskIDValues{
		Command:          "ai.command",
		DirectCommand:    "ai.direct.command",
		FileEdit:         "ai.file.edit",
		FsList:           "ai.fs.list",
		FsRead:           "ai.fs.read",
		PortCheck:        "ai.port.check",
		FetchLogs:        "ai.fetch.logs",
		FetchHistory:     "ai.fetch.history",
		FetchFileHistory: "ai.fetch.file.history",
		RestoreFile:      "ai.restore.file",
		FetchFileDiff:    "ai.fetch.file.diff",
	},
}
