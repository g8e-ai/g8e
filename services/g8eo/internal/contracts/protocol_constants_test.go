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

/*
Protocol Constants Contract Tests

Verifies that g8eo Go constants in constants/events.go and constants/status.go
exactly match the canonical values in protocol/constants/*.json.

g8eo duplicates protocol JSON values as compile-time Go constants (no //go:embed,
no runtime loading). These tests are the enforcement mechanism that detects
drift between the JSON source of truth and the Go consumer.
*/
package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var protocolConstantsDir string

func init() {
	protocolConstantsDir = filepath.Join(system.ResolveProjectRoot(), "protocol/constants")
}

func loadProtocolFile(t *testing.T, filename string) []byte {
	t.Helper()
	path := filepath.Join(protocolConstantsDir, filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "protocol/constants/%s must be readable", filename)
	return data
}

// ---------------------------------------------------------------------------
// Typed structs mirroring protocol/constants/events.json
// ---------------------------------------------------------------------------

type protocolCommandOutputEvents struct {
	Received string `json:"received"`
}

type protocolCommandCancelEvents struct {
	Requested    string `json:"requested"`
	Acknowledged string `json:"acknowledged"`
	Failed       string `json:"failed"`
}

type protocolCommandApprovalEvents struct {
	Requested string `json:"requested"`
	Granted   string `json:"granted"`
	Rejected  string `json:"rejected"`
}

type protocolOperatorCommandEvents struct {
	Requested string                        `json:"requested"`
	Started   string                        `json:"started"`
	Completed string                        `json:"completed"`
	Failed    string                        `json:"failed"`
	Cancelled string                        `json:"cancelled"`
	Output    protocolCommandOutputEvents   `json:"output"`
	Cancel    protocolCommandCancelEvents   `json:"cancel"`
	Approval  protocolCommandApprovalEvents `json:"approval"`
}

type protocolFileEditApprovalEvents struct {
	Requested string `json:"requested"`
	Granted   string `json:"granted"`
	Rejected  string `json:"rejected"`
}

type protocolOperatorFileEditEvents struct {
	Requested string                         `json:"requested"`
	Started   string                         `json:"started"`
	Completed string                         `json:"completed"`
	Failed    string                         `json:"failed"`
	Approval  protocolFileEditApprovalEvents `json:"approval"`
}

type protocolIntentApprovalEvents struct {
	Requested string `json:"requested"`
	Granted   string `json:"granted"`
	Rejected  string `json:"rejected"`
}

type protocolOperatorIntentEvents struct {
	Granted  string                       `json:"granted"`
	Denied   string                       `json:"denied"`
	Revoked  string                       `json:"revoked"`
	Approval protocolIntentApprovalEvents `json:"approval"`
}

type protocolFetchLeaf struct {
	Requested string `json:"requested"`
	Completed string `json:"completed"`
	Failed    string `json:"failed"`
}

type protocolOperatorNetworkPortCheck struct {
	Requested string `json:"requested"`
	Completed string `json:"completed"`
	Failed    string `json:"failed"`
}

type protocolOperatorNetworkPort struct {
	Check protocolOperatorNetworkPortCheck `json:"check"`
}

type protocolOperatorNetwork struct {
	Port protocolOperatorNetworkPort `json:"port"`
}

type protocolOperatorFilesystem struct {
	List protocolFetchLeaf `json:"list"`
	Read protocolFetchLeaf `json:"read"`
}

type protocolOperatorFileHistory struct {
	Fetch protocolFetchLeaf `json:"fetch"`
}

type protocolOperatorFileDiff struct {
	Fetch protocolFetchLeaf `json:"fetch"`
}

type protocolOperatorFileRestore struct {
	Requested string `json:"requested"`
	Completed string `json:"completed"`
	Failed    string `json:"failed"`
}

type protocolOperatorFileEditApproval struct {
	Requested string `json:"requested"`
	Granted   string `json:"granted"`
	Rejected  string `json:"rejected"`
}

type protocolOperatorFileEdit struct {
	Requested string                           `json:"requested"`
	Started   string                           `json:"started"`
	Completed string                           `json:"completed"`
	Failed    string                           `json:"failed"`
	Approval  protocolOperatorFileEditApproval `json:"approval"`
}

type protocolOperatorFileEvents struct {
	Edit    protocolOperatorFileEdit    `json:"edit"`
	History protocolOperatorFileHistory `json:"history"`
	Diff    protocolOperatorFileDiff    `json:"diff"`
	Restore protocolOperatorFileRestore `json:"restore"`
}

type protocolOperatorAuditUserRecorded struct {
	Recorded string `json:"recorded"`
}

type protocolOperatorAuditAIRecorded struct {
	Recorded string `json:"recorded"`
}

type protocolOperatorAuditDirectCommandResult struct {
	Recorded string `json:"recorded"`
}

type protocolOperatorAuditDirectCommandEvents struct {
	Recorded string                                   `json:"recorded"`
	Result   protocolOperatorAuditDirectCommandResult `json:"result"`
}

type protocolOperatorAuditDirectEvents struct {
	Command protocolOperatorAuditDirectCommandEvents `json:"command"`
}

type protocolOperatorAuditEvents struct {
	User   protocolOperatorAuditUserRecorded `json:"user"`
	AI     protocolOperatorAuditAIRecorded   `json:"ai"`
	Direct protocolOperatorAuditDirectEvents `json:"direct"`
}

type protocolOperatorHeartbeat struct {
	Sent      string `json:"sent"`
	Requested string `json:"requested"`
	Received  string `json:"received"`
	Missed    string `json:"missed"`
}

type protocolOperatorShutdown struct {
	Requested    string `json:"requested"`
	Acknowledged string `json:"acknowledged"`
}

type protocolOperatorAPIKey struct {
	Refreshed string `json:"refreshed"`
}

type protocolOperatorAPI struct {
	Key protocolOperatorAPIKey `json:"key"`
}

type protocolOperatorLogs struct {
	Fetch protocolFetchLeaf `json:"fetch"`
}

type protocolOperatorHistory struct {
	Fetch protocolFetchLeaf `json:"fetch"`
}

type protocolOperatorEvents struct {
	Heartbeat  protocolOperatorHeartbeat     `json:"heartbeat"`
	Shutdown   protocolOperatorShutdown      `json:"shutdown"`
	API        protocolOperatorAPI           `json:"api"`
	Command    protocolOperatorCommandEvents `json:"command"`
	Intent     protocolOperatorIntentEvents  `json:"intent"`
	Filesystem protocolOperatorFilesystem    `json:"filesystem"`
	Logs       protocolOperatorLogs          `json:"logs"`
	History    protocolOperatorHistory       `json:"history"`
	File       protocolOperatorFileEvents    `json:"file"`
	Network    protocolOperatorNetwork       `json:"network"`
	Audit      protocolOperatorAuditEvents   `json:"audit"`
}

type protocolEventsJSON struct {
	Operator protocolOperatorEvents `json:"operator"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring protocol/constants/status.json
// ---------------------------------------------------------------------------

type protocolOperatorStatusValues struct {
	Available   string `json:"available"`
	Unavailable string `json:"unavailable"`
	Offline     string `json:"offline"`
	Bound       string `json:"bound"`
	Stale       string `json:"stale"`
	Active      string `json:"active"`
	Stopped     string `json:"stopped"`
	Terminated  string `json:"terminated"`
}

type protocolOperatorTypeValues struct {
	System string `json:"system"`
	Cloud  string `json:"cloud"`
}

type protocolCloudSubtypeValues struct {
	AWS   string `json:"aws"`
	GCP   string `json:"gcp"`
	Azure string `json:"azure"`
}

type protocolVaultModeValues struct {
	Raw      string `json:"raw"`
	Scrubbed string `json:"scrubbed"`
}

type protocolVersionStabilityValues struct {
	Stable string `json:"stable"`
	Beta   string `json:"beta"`
	Dev    string `json:"dev"`
}

type protocolComponentNameValues struct {
	G8EE   string `json:"g8ee"`
	G8EO   string `json:"g8eo"`
	CLIENT string `json:"client"`
}

type protocolPlatformValues struct {
	Linux   string `json:"linux"`
	Windows string `json:"windows"`
	Darwin  string `json:"darwin"`
}

type protocolAISourceValues struct {
	Tool             string `json:"tool.call"`
	TerminalAnchored string `json:"terminal.anchored"`
	TerminalDirect   string `json:"terminal.direct"`
}

type protocolAITaskIDValues struct {
	Command          string `json:"command"`
	DirectCommand    string `json:"direct.command"`
	FileEdit         string `json:"file.edit"`
	FsList           string `json:"fs.list"`
	FsRead           string `json:"fs.read"`
	PortCheck        string `json:"port.check"`
	FetchLogs        string `json:"fetch.logs"`
	FetchHistory     string `json:"fetch.history"`
	FetchFileHistory string `json:"fetch.file.history"`
	RestoreFile      string `json:"restore.file"`
	FetchFileDiff    string `json:"fetch.file.diff"`
}

type protocolHeartbeatTypeValues struct {
	Automatic string `json:"automatic"`
	Bootstrap string `json:"bootstrap"`
	Requested string `json:"requested"`
}

type protocolExecutionStatusValues struct {
	Pending   string `json:"pending"`
	Executing string `json:"executing"`
	Completed string `json:"completed"`
	Failed    string `json:"failed"`
	Timeout   string `json:"timeout"`
	Cancelled string `json:"cancelled"`
}

type protocolStatusJSON struct {
	OperatorStatus   protocolOperatorStatusValues   `json:"g8e.status"`
	OperatorType     protocolOperatorTypeValues     `json:"g8e.type"`
	CloudSubtype     protocolCloudSubtypeValues     `json:"cloud.subtype"`
	VaultMode        protocolVaultModeValues        `json:"vault.mode"`
	VersionStability protocolVersionStabilityValues `json:"version.stability"`
	ComponentName    protocolComponentNameValues    `json:"component.name"`
	Platform         protocolPlatformValues         `json:"platform"`
	AISource         protocolAISourceValues         `json:"ai.source"`
	AITaskID         protocolAITaskIDValues         `json:"ai.task.id"`
	HeartbeatType    protocolHeartbeatTypeValues    `json:"heartbeat.type"`
	ExecutionStatus  protocolExecutionStatusValues  `json:"execution.status"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring protocol/constants/channels.json
// ---------------------------------------------------------------------------

type protocolChannelPrefixes struct {
	Cmd       string `json:"cmd"`
	Results   string `json:"results"`
	Heartbeat string `json:"heartbeat"`
}

type protocolPubSubChannels struct {
	Prefixes protocolChannelPrefixes `json:"prefixes"`
}

type protocolChannelsJSON struct {
	PubSub protocolPubSubChannels `json:"pubsub"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring protocol/constants/pubsub.json
// ---------------------------------------------------------------------------

type protocolPubSubWireActions struct {
	Subscribe   string `json:"subscribe"`
	PSubscribe  string `json:"psubscribe"`
	Unsubscribe string `json:"unsubscribe"`
	Publish     string `json:"publish"`
}

type protocolPubSubWireEventTypes struct {
	Message    string `json:"message"`
	PMessage   string `json:"pmessage"`
	Subscribed string `json:"subscribed"`
}

type protocolPubSubWireFields struct {
	Action  string `json:"action"`
	Channel string `json:"channel"`
	Data    string `json:"data"`
	Message string `json:"message"`
	Pattern string `json:"pattern"`
	Type    string `json:"type"`
	Sender  string `json:"sender"`
}

type protocolPubSubWire struct {
	Actions    protocolPubSubWireActions    `json:"actions"`
	EventTypes protocolPubSubWireEventTypes `json:"event_types"`
	Fields     protocolPubSubWireFields     `json:"fields"`
}

type protocolPubSubJSON struct {
	Wire protocolPubSubWire `json:"wire"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring protocol/constants/headers.json
// ---------------------------------------------------------------------------

type protocolHeadersJSON struct {
	OperatorSessionID  string `json:"x-g8e.operator-session-id"`
	DeviceToken        string `json:"x-g8e.device-token"`
	Authorization      string `json:"http.authorization"`
	UserAgent          string `json:"http.user-agent"`
	ContentType        string `json:"http.content-type"`
	ContentDisposition string `json:"http.content-disposition"`
	ContentLength      string `json:"http.content-length"`
	XForwardedProto    string `json:"http.x-forwarded-proto"`
	XForwardedHost     string `json:"http.x-forwarded-host"`
	XRequestTimestamp  string `json:"http.x-request-timestamp"`
}

func loadEventsJSON(t *testing.T) protocolEventsJSON {
	t.Helper()
	var ev protocolEventsJSON
	require.NoError(t, json.Unmarshal(loadProtocolFile(t, "events.json"), &ev), "events.json must unmarshal into protocolEventsJSON")
	return ev
}

func loadStatusJSON(t *testing.T) protocolStatusJSON {
	t.Helper()
	var st protocolStatusJSON
	require.NoError(t, json.Unmarshal(loadProtocolFile(t, "status.json"), &st), "status.json must unmarshal into protocolStatusJSON")
	return st
}

func loadChannelsJSON(t *testing.T) protocolChannelsJSON {
	t.Helper()
	var ch protocolChannelsJSON
	require.NoError(t, json.Unmarshal(loadProtocolFile(t, "channels.json"), &ch), "channels.json must unmarshal into protocolChannelsJSON")
	return ch
}

func loadPubSubJSON(t *testing.T) protocolPubSubJSON {
	t.Helper()
	var ps protocolPubSubJSON
	require.NoError(t, json.Unmarshal(loadProtocolFile(t, "pubsub.json"), &ps), "pubsub.json must unmarshal into protocolPubSubJSON")
	return ps
}

func loadHeadersJSON(t *testing.T) protocolHeadersJSON {
	t.Helper()
	var h protocolHeadersJSON
	require.NoError(t, json.Unmarshal(loadProtocolFile(t, "headers.json"), &h), "headers.json must unmarshal into protocolHeadersJSON")
	return h
}

// =============================================================================
// Events
// =============================================================================

func TestProtocolEventsMatchGoConstants(t *testing.T) {
	ev := loadEventsJSON(t)
	op := ev.Operator

	t.Run("operator.heartbeat", func(t *testing.T) {
		assert.Equal(t, op.Heartbeat.Sent, constants.Event.Operator.Heartbeat)
		assert.Equal(t, op.Heartbeat.Requested, constants.Event.Operator.HeartbeatRequested)
	})

	t.Run("operator.shutdown", func(t *testing.T) {
		assert.Equal(t, op.Shutdown.Requested, constants.Event.Operator.ShutdownRequested)
		assert.Equal(t, op.Shutdown.Acknowledged, constants.Event.Operator.ShutdownAcknowledged)
	})

	t.Run("operator.api.key", func(t *testing.T) {
		assert.Equal(t, op.API.Key.Refreshed, constants.Event.Operator.APIKeyRefreshed)
	})

	t.Run("operator.command", func(t *testing.T) {
		assert.Equal(t, op.Command.Requested, constants.Event.Operator.Command.Requested)
		assert.Equal(t, op.Command.Completed, constants.Event.Operator.Command.Completed)
		assert.Equal(t, op.Command.Failed, constants.Event.Operator.Command.Failed)
		assert.Equal(t, op.Command.Cancelled, constants.Event.Operator.Command.Cancelled)
		assert.Equal(t, op.Command.Cancel.Requested, constants.Event.Operator.Command.CancelRequested)
		assert.Equal(t, op.Command.Approval.Requested, constants.Event.Operator.Command.ApprovalRequested)
	})

	t.Run("operator.file.edit", func(t *testing.T) {
		assert.Equal(t, op.File.Edit.Requested, constants.Event.Operator.FileEdit.Requested)
		assert.Equal(t, op.File.Edit.Completed, constants.Event.Operator.FileEdit.Completed)
		assert.Equal(t, op.File.Edit.Failed, constants.Event.Operator.FileEdit.Failed)
		assert.Equal(t, op.File.Edit.Approval.Requested, constants.Event.Operator.FileEdit.ApprovalRequested)
	})

	t.Run("operator.intent", func(t *testing.T) {
		assert.Equal(t, op.Intent.Approval.Requested, constants.Event.Operator.Intent.ApprovalRequested)
		assert.Equal(t, op.Intent.Granted, constants.Event.Operator.Intent.Granted)
		assert.Equal(t, op.Intent.Denied, constants.Event.Operator.Intent.Denied)
		assert.Equal(t, op.Intent.Revoked, constants.Event.Operator.Intent.Revoked)
	})

	t.Run("operator.network.port.check", func(t *testing.T) {
		assert.Equal(t, op.Network.Port.Check.Requested, constants.Event.Operator.PortCheck.Requested)
		assert.Equal(t, op.Network.Port.Check.Completed, constants.Event.Operator.PortCheck.Completed)
		assert.Equal(t, op.Network.Port.Check.Failed, constants.Event.Operator.PortCheck.Failed)
	})

	t.Run("operator.filesystem.list", func(t *testing.T) {
		assert.Equal(t, op.Filesystem.List.Requested, constants.Event.Operator.FsList.Requested)
		assert.Equal(t, op.Filesystem.List.Completed, constants.Event.Operator.FsList.Completed)
		assert.Equal(t, op.Filesystem.List.Failed, constants.Event.Operator.FsList.Failed)
	})

	t.Run("operator.filesystem.read", func(t *testing.T) {
		assert.Equal(t, op.Filesystem.Read.Requested, constants.Event.Operator.FsRead.Requested)
		assert.Equal(t, op.Filesystem.Read.Completed, constants.Event.Operator.FsRead.Completed)
		assert.Equal(t, op.Filesystem.Read.Failed, constants.Event.Operator.FsRead.Failed)
	})

	t.Run("operator.logs.fetch", func(t *testing.T) {
		assert.Equal(t, op.Logs.Fetch.Requested, constants.Event.Operator.FetchLogs.Requested)
		assert.Equal(t, op.Logs.Fetch.Completed, constants.Event.Operator.FetchLogs.Completed)
		assert.Equal(t, op.Logs.Fetch.Failed, constants.Event.Operator.FetchLogs.Failed)
	})

	t.Run("operator.history.fetch", func(t *testing.T) {
		assert.Equal(t, op.History.Fetch.Requested, constants.Event.Operator.FetchHistory.Requested)
		assert.Equal(t, op.History.Fetch.Completed, constants.Event.Operator.FetchHistory.Completed)
		assert.Equal(t, op.History.Fetch.Failed, constants.Event.Operator.FetchHistory.Failed)
	})

	t.Run("operator.file.history.fetch", func(t *testing.T) {
		assert.Equal(t, op.File.History.Fetch.Requested, constants.Event.Operator.FetchFileHistory.Requested)
		assert.Equal(t, op.File.History.Fetch.Completed, constants.Event.Operator.FetchFileHistory.Completed)
		assert.Equal(t, op.File.History.Fetch.Failed, constants.Event.Operator.FetchFileHistory.Failed)
	})

	t.Run("operator.file.restore", func(t *testing.T) {
		assert.Equal(t, op.File.Restore.Requested, constants.Event.Operator.RestoreFile.Requested)
		assert.Equal(t, op.File.Restore.Completed, constants.Event.Operator.RestoreFile.Completed)
		assert.Equal(t, op.File.Restore.Failed, constants.Event.Operator.RestoreFile.Failed)
	})

	t.Run("operator.file.diff.fetch", func(t *testing.T) {
		assert.Equal(t, op.File.Diff.Fetch.Requested, constants.Event.Operator.FetchFileDiff.Requested)
		assert.Equal(t, op.File.Diff.Fetch.Completed, constants.Event.Operator.FetchFileDiff.Completed)
		assert.Equal(t, op.File.Diff.Fetch.Failed, constants.Event.Operator.FetchFileDiff.Failed)
	})

	t.Run("operator.audit", func(t *testing.T) {
		assert.Equal(t, op.Audit.User.Recorded, constants.Event.Operator.Audit.UserMsg)
		assert.Equal(t, op.Audit.AI.Recorded, constants.Event.Operator.Audit.AIMsg)
		assert.Equal(t, op.Audit.Direct.Command.Recorded, constants.Event.Operator.Audit.DirectCmd)
		assert.Equal(t, op.Audit.Direct.Command.Result.Recorded, constants.Event.Operator.Audit.DirectCmdResult)
	})
}

// =============================================================================
// Status
// =============================================================================

func TestProtocolStatusMatchesGoConstants(t *testing.T) {
	st := loadStatusJSON(t)

	t.Run("operator.status", func(t *testing.T) {
		assert.Equal(t, st.OperatorStatus.Available, constants.Status.OperatorStatus.Available)
		assert.Equal(t, st.OperatorStatus.Unavailable, constants.Status.OperatorStatus.Unavailable)
		assert.Equal(t, st.OperatorStatus.Offline, constants.Status.OperatorStatus.Offline)
		assert.Equal(t, st.OperatorStatus.Bound, constants.Status.OperatorStatus.Bound)
		assert.Equal(t, st.OperatorStatus.Stale, constants.Status.OperatorStatus.Stale)
		assert.Equal(t, st.OperatorStatus.Active, constants.Status.OperatorStatus.Active)
		assert.Equal(t, st.OperatorStatus.Stopped, constants.Status.OperatorStatus.Stopped)
		assert.Equal(t, st.OperatorStatus.Terminated, constants.Status.OperatorStatus.Terminated)
	})

	t.Run("operator.type", func(t *testing.T) {
		assert.Equal(t, st.OperatorType.System, constants.Status.OperatorType.System)
		assert.Equal(t, st.OperatorType.Cloud, constants.Status.OperatorType.Cloud)
	})

	t.Run("cloud.subtype", func(t *testing.T) {
		assert.Equal(t, st.CloudSubtype.AWS, constants.Status.CloudSubtype.AWS)
		assert.Equal(t, st.CloudSubtype.GCP, constants.Status.CloudSubtype.GCP)
		assert.Equal(t, st.CloudSubtype.Azure, constants.Status.CloudSubtype.Azure)
	})

	t.Run("vault.mode", func(t *testing.T) {
		assert.Equal(t, st.VaultMode.Raw, constants.Status.VaultMode.Raw)
		assert.Equal(t, st.VaultMode.Scrubbed, constants.Status.VaultMode.Scrubbed)
	})

	t.Run("version.stability", func(t *testing.T) {
		assert.Equal(t, st.VersionStability.Stable, constants.Status.VersionStability.Stable)
		assert.Equal(t, st.VersionStability.Beta, constants.Status.VersionStability.Beta)
		assert.Equal(t, st.VersionStability.Dev, constants.Status.VersionStability.Dev)
	})

	t.Run("component.name", func(t *testing.T) {
		assert.Equal(t, st.ComponentName.G8EE, constants.Status.ComponentName.G8EE)
		assert.Equal(t, st.ComponentName.G8EO, constants.Status.ComponentName.G8EO)
		assert.Equal(t, st.ComponentName.CLIENT, constants.Status.ComponentName.CLIENT)
	})

	t.Run("platform", func(t *testing.T) {
		assert.Equal(t, st.Platform.Linux, constants.Status.Platform.Linux)
		assert.Equal(t, st.Platform.Windows, constants.Status.Platform.Windows)
		assert.Equal(t, st.Platform.Darwin, constants.Status.Platform.Darwin)
	})

	t.Run("ai.source", func(t *testing.T) {
		assert.Equal(t, st.AISource.Tool, constants.Status.AISource.Tool)
		assert.Equal(t, st.AISource.TerminalAnchored, constants.Status.AISource.TerminalAnchored)
		assert.Equal(t, st.AISource.TerminalDirect, constants.Status.AISource.TerminalDirect)
	})

	t.Run("ai.task.id", func(t *testing.T) {
		assert.Equal(t, st.AITaskID.Command, constants.Status.AITaskID.Command)
		assert.Equal(t, st.AITaskID.DirectCommand, constants.Status.AITaskID.DirectCommand)
		assert.Equal(t, st.AITaskID.FileEdit, constants.Status.AITaskID.FileEdit)
		assert.Equal(t, st.AITaskID.FsList, constants.Status.AITaskID.FsList)
		assert.Equal(t, st.AITaskID.FsRead, constants.Status.AITaskID.FsRead)
		assert.Equal(t, st.AITaskID.PortCheck, constants.Status.AITaskID.PortCheck)
		assert.Equal(t, st.AITaskID.FetchLogs, constants.Status.AITaskID.FetchLogs)
		assert.Equal(t, st.AITaskID.FetchHistory, constants.Status.AITaskID.FetchHistory)
		assert.Equal(t, st.AITaskID.FetchFileHistory, constants.Status.AITaskID.FetchFileHistory)
		assert.Equal(t, st.AITaskID.RestoreFile, constants.Status.AITaskID.RestoreFile)
		assert.Equal(t, st.AITaskID.FetchFileDiff, constants.Status.AITaskID.FetchFileDiff)
	})
}

// =============================================================================
// Channels
// =============================================================================

func TestProtocolChannelsMatchGoConstants(t *testing.T) {
	ch := loadChannelsJSON(t)

	t.Run("channel prefixes used by CmdChannel/ResultsChannel/HeartbeatChannel", func(t *testing.T) {
		assert.Equal(t, "cmd", ch.PubSub.Prefixes.Cmd)
		assert.Equal(t, "results", ch.PubSub.Prefixes.Results)
		assert.Equal(t, "heartbeat", ch.PubSub.Prefixes.Heartbeat)

		assert.Equal(t, constants.CmdChannel("op1", "s1"), "cmd:op1:s1")
		assert.Equal(t, constants.ResultsChannel("op1", "s1"), "results:op1:s1")
		assert.Equal(t, constants.HeartbeatChannel("op1", "s1"), "heartbeat:op1:s1")
	})
}

// =============================================================================
// Heartbeat type
// =============================================================================

func TestProtocolHeartbeatTypeMatchesGoConstants(t *testing.T) {
	st := loadStatusJSON(t)

	assert.Equal(t, st.HeartbeatType.Automatic, string(constants.HeartbeatTypeAutomatic))
	assert.Equal(t, st.HeartbeatType.Bootstrap, string(constants.HeartbeatTypeBootstrap))
	assert.Equal(t, st.HeartbeatType.Requested, string(constants.HeartbeatTypeRequested))
}

// =============================================================================
// Headers
// =============================================================================

func TestProtocolHeadersMatchGoConstants(t *testing.T) {
	h := loadHeadersJSON(t)

	t.Run("standard http headers", func(t *testing.T) {
		assert.Equal(t, h.Authorization, constants.HeaderAuthorization)
		assert.Equal(t, h.UserAgent, constants.HeaderUserAgent)
		assert.Equal(t, h.ContentType, constants.HeaderContentType)
		assert.Equal(t, h.ContentDisposition, constants.HeaderContentDisposition)
		assert.Equal(t, h.ContentLength, constants.HeaderContentLength)
		assert.Equal(t, h.XForwardedProto, constants.HeaderXForwardedProto)
		assert.Equal(t, h.XForwardedHost, constants.HeaderXForwardedHost)
		assert.Equal(t, h.XRequestTimestamp, constants.HeaderXRequestTimestamp)
	})
}

// =============================================================================
// PubSub wire protocol
// =============================================================================

func TestProtocolPubSubWireMatchesGoConstants(t *testing.T) {
	ps := loadPubSubJSON(t)

	t.Run("wire.actions", func(t *testing.T) {
		assert.Equal(t, ps.Wire.Actions.Subscribe, constants.PubSubActionSubscribe)
		assert.Equal(t, ps.Wire.Actions.PSubscribe, constants.PubSubActionPSubscribe)
		assert.Equal(t, ps.Wire.Actions.Unsubscribe, constants.PubSubActionUnsubscribe)
		assert.Equal(t, ps.Wire.Actions.Publish, constants.PubSubActionPublish)
	})

	t.Run("wire.event_types", func(t *testing.T) {
		assert.Equal(t, ps.Wire.EventTypes.Message, constants.PubSubEventMessage)
		assert.Equal(t, ps.Wire.EventTypes.PMessage, constants.PubSubEventPMessage)
		assert.Equal(t, ps.Wire.EventTypes.Subscribed, constants.PubSubEventSubscribed)
	})
}

// =============================================================================
// Execution status
// =============================================================================

func TestProtocolExecutionStatusMatchesGoConstants(t *testing.T) {
	st := loadStatusJSON(t)

	assert.Equal(t, st.ExecutionStatus.Pending, string(constants.ExecutionStatusPending))
	assert.Equal(t, st.ExecutionStatus.Executing, string(constants.ExecutionStatusExecuting))
	assert.Equal(t, st.ExecutionStatus.Completed, string(constants.ExecutionStatusCompleted))
	assert.Equal(t, st.ExecutionStatus.Failed, string(constants.ExecutionStatusFailed))
	assert.Equal(t, st.ExecutionStatus.Timeout, string(constants.ExecutionStatusTimeout))
	assert.Equal(t, st.ExecutionStatus.Cancelled, string(constants.ExecutionStatusCancelled))
}
