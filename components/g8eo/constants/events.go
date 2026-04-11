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

// operatorCommandStatusUpdatedEvents mirrors shared/constants/events.json operator.command.status.updated.*
type operatorCommandStatusUpdatedEvents struct {
	Queued    string
	Running   string
	Completed string
	Failed    string
	Cancelled string
}

// operatorCommandEvents mirrors shared/constants/events.json operator.command.*
type operatorCommandEvents struct {
	Requested         string
	Started           string
	OutputReceived    string
	StatusUpdated     operatorCommandStatusUpdatedEvents
	Completed         string
	Failed            string
	Cancelled         string
	CancelRequested   string
	ApprovalRequested string
}

// operatorFileEditEvents mirrors shared/constants/events.json operator.file.edit.*
type operatorFileEditEvents struct {
	Requested         string
	Started           string
	Completed         string
	Failed            string
	ApprovalRequested string
}

// operatorIntentEvents mirrors shared/constants/events.json g8e.intent.*
type operatorIntentEvents struct {
	ApprovalRequested string
	Granted           string
	Denied            string
	Revoked           string
}

// operatorPortCheckEvents mirrors shared/constants/events.json g8e.port.check.*
type operatorPortCheckEvents struct {
	Requested string
	Completed string
	Failed    string
}

// operatorFsListEvents mirrors shared/constants/events.json g8e.fs.list.*
type operatorFsListEvents struct {
	Requested string
	Completed string
	Failed    string
}

// operatorFsReadEvents mirrors shared/constants/events.json g8e.fs.read.*
type operatorFsReadEvents struct {
	Requested string
	Completed string
	Failed    string
}

// operatorFetchLogsEvents mirrors shared/constants/events.json g8e.fetch.logs.*
type operatorFetchLogsEvents struct {
	Requested string
	Completed string
	Failed    string
}

// operatorFetchHistoryEvents mirrors shared/constants/events.json g8e.fetch.history.*
type operatorFetchHistoryEvents struct {
	Requested string
	Completed string
	Failed    string
}

// operatorFetchFileHistoryEvents mirrors shared/constants/events.json g8e.fetch.file.history.*
type operatorFetchFileHistoryEvents struct {
	Requested string
	Completed string
	Failed    string
}

// operatorRestoreFileEvents mirrors shared/constants/events.json g8e.restore.file.*
type operatorRestoreFileEvents struct {
	Requested string
	Completed string
	Failed    string
}

// operatorMcpEvents mirrors shared/constants/events.json g8e.mcp.*
type operatorMcpEvents struct {
	ToolsCall       string
	ToolsResult     string
	ResourcesList   string
	ResourcesRead   string
	ResourcesResult string
}

// operatorFetchFileDiffEvents mirrors shared/constants/events.json g8e.fetch.file.diff.*
type operatorFetchFileDiffEvents struct {
	Requested string
	Completed string
	Failed    string
}

// operatorAuditEvents mirrors shared/constants/events.json g8e.audit.*
type operatorAuditEvents struct {
	UserMsg         string
	AIMsg           string
	DirectCmd       string
	DirectCmdResult string
}

// operatorEvents mirrors shared/constants/events.json g8e.*
type operatorEvents struct {
	Heartbeat            string
	HeartbeatRequested   string
	Ping                 string
	Stale                string
	StateChange          string
	ListUpdated          string
	StatusUpdated        string
	APIKeyRefreshed      string
	ContextChange        string
	ShutdownRequested    string
	ShutdownAcknowledged string

	Command          operatorCommandEvents
	FileEdit         operatorFileEditEvents
	Intent           operatorIntentEvents
	PortCheck        operatorPortCheckEvents
	FsList           operatorFsListEvents
	FsRead           operatorFsReadEvents
	FetchLogs        operatorFetchLogsEvents
	FetchHistory     operatorFetchHistoryEvents
	FetchFileHistory operatorFetchFileHistoryEvents
	RestoreFile      operatorRestoreFileEvents
	FetchFileDiff    operatorFetchFileDiffEvents
	Audit            operatorAuditEvents
	MCP              operatorMcpEvents
}

// events is the top-level namespace. Canonical values from shared/constants/events.json.
type events struct {
	Operator operatorEvents
}

// Event is the package-level entry point for all event type constants.
// Usage: constants.Event.Operator.Command.Requested
var Event = events{
	Operator: operatorEvents{
		Heartbeat:            "g8e.v1.operator.heartbeat.sent",
		HeartbeatRequested:   "g8e.v1.operator.heartbeat.requested",
		Ping:                 "g8e.v1.operator.network.ping.requested",
		Stale:                "g8e.v1.operator.heartbeat.missed",
		StateChange:          "g8e.v1.operator.status.updated",
		ListUpdated:          "g8e.v1.operator.panel.list.updated",
		StatusUpdated:        "g8e.v1.operator.status.updated",
		APIKeyRefreshed:      "g8e.v1.operator.api.key.refreshed",
		ContextChange:        "g8e.v1.operator.bound",
		ShutdownRequested:    "g8e.v1.operator.shutdown.requested",
		ShutdownAcknowledged: "g8e.v1.operator.shutdown.acknowledged",

		Command: operatorCommandEvents{
			Requested:      "g8e.v1.operator.command.requested",
			Started:        "g8e.v1.operator.command.started",
			OutputReceived: "g8e.v1.operator.command.output.received",
			StatusUpdated: operatorCommandStatusUpdatedEvents{
				Queued:    "g8e.v1.operator.command.status.updated.queued",
				Running:   "g8e.v1.operator.command.status.updated.running",
				Completed: "g8e.v1.operator.command.status.updated.completed",
				Failed:    "g8e.v1.operator.command.status.updated.failed",
				Cancelled: "g8e.v1.operator.command.status.updated.cancelled",
			},
			Completed:         "g8e.v1.operator.command.completed",
			Failed:            "g8e.v1.operator.command.failed",
			Cancelled:         "g8e.v1.operator.command.cancelled",
			CancelRequested:   "g8e.v1.operator.command.cancel.requested",
			ApprovalRequested: "g8e.v1.operator.command.approval.requested",
		},
		FileEdit: operatorFileEditEvents{
			Requested:         "g8e.v1.operator.file.edit.requested",
			Started:           "g8e.v1.operator.file.edit.started",
			Completed:         "g8e.v1.operator.file.edit.completed",
			Failed:            "g8e.v1.operator.file.edit.failed",
			ApprovalRequested: "g8e.v1.operator.file.edit.approval.requested",
		},
		Intent: operatorIntentEvents{
			ApprovalRequested: "g8e.v1.operator.intent.approval.requested",
			Granted:           "g8e.v1.operator.intent.granted",
			Denied:            "g8e.v1.operator.intent.denied",
			Revoked:           "g8e.v1.operator.intent.revoked",
		},
		PortCheck: operatorPortCheckEvents{
			Requested: "g8e.v1.operator.network.port.check.requested",
			Completed: "g8e.v1.operator.network.port.check.completed",
			Failed:    "g8e.v1.operator.network.port.check.failed",
		},
		FsList: operatorFsListEvents{
			Requested: "g8e.v1.operator.filesystem.list.requested",
			Completed: "g8e.v1.operator.filesystem.list.completed",
			Failed:    "g8e.v1.operator.filesystem.list.failed",
		},
		FsRead: operatorFsReadEvents{
			Requested: "g8e.v1.operator.filesystem.read.requested",
			Completed: "g8e.v1.operator.filesystem.read.completed",
			Failed:    "g8e.v1.operator.filesystem.read.failed",
		},
		FetchLogs: operatorFetchLogsEvents{
			Requested: "g8e.v1.operator.logs.fetch.requested",
			Completed: "g8e.v1.operator.logs.fetch.completed",
			Failed:    "g8e.v1.operator.logs.fetch.failed",
		},
		FetchHistory: operatorFetchHistoryEvents{
			Requested: "g8e.v1.operator.history.fetch.requested",
			Completed: "g8e.v1.operator.history.fetch.completed",
			Failed:    "g8e.v1.operator.history.fetch.failed",
		},
		FetchFileHistory: operatorFetchFileHistoryEvents{
			Requested: "g8e.v1.operator.file.history.fetch.requested",
			Completed: "g8e.v1.operator.file.history.fetch.completed",
			Failed:    "g8e.v1.operator.file.history.fetch.failed",
		},
		RestoreFile: operatorRestoreFileEvents{
			Requested: "g8e.v1.operator.file.restore.requested",
			Completed: "g8e.v1.operator.file.restore.completed",
			Failed:    "g8e.v1.operator.file.restore.failed",
		},
		FetchFileDiff: operatorFetchFileDiffEvents{
			Requested: "g8e.v1.operator.file.diff.fetch.requested",
			Completed: "g8e.v1.operator.file.diff.fetch.completed",
			Failed:    "g8e.v1.operator.file.diff.fetch.failed",
		},
		Audit: operatorAuditEvents{
			UserMsg:         "g8e.v1.operator.audit.user.recorded",
			AIMsg:           "g8e.v1.operator.audit.ai.recorded",
			DirectCmd:       "g8e.v1.operator.audit.direct.command.recorded",
			DirectCmdResult: "g8e.v1.operator.audit.direct.command.result.recorded",
		},
		MCP: operatorMcpEvents{
			ToolsCall:       "g8e.v1.operator.mcp.tools.call",
			ToolsResult:     "g8e.v1.operator.mcp.tools.result",
			ResourcesList:   "g8e.v1.operator.mcp.resources.list",
			ResourcesRead:   "g8e.v1.operator.mcp.resources.read",
			ResourcesResult: "g8e.v1.operator.mcp.resources.result",
		},
	},
}
