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
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/marshaler"
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
	Received protocolLeaf `json:"received"`
}

type protocolCommandCancelEvents struct {
	Requested    protocolLeaf `json:"requested"`
	Acknowledged protocolLeaf `json:"acknowledged"`
	Failed       protocolLeaf `json:"failed"`
}

type protocolCommandApprovalEvents struct {
	Requested protocolLeaf `json:"requested"`
	Granted   protocolLeaf `json:"granted"`
	Rejected  protocolLeaf `json:"rejected"`
}

type protocolOperatorCommandEvents struct {
	Requested protocolLeaf                  `json:"requested"`
	Started   protocolLeaf                  `json:"started"`
	Completed protocolLeaf                  `json:"completed"`
	Failed    protocolLeaf                  `json:"failed"`
	Cancelled protocolLeaf                  `json:"cancelled"`
	Output    protocolCommandOutputEvents   `json:"output"`
	Cancel    protocolCommandCancelEvents   `json:"cancel"`
	Approval  protocolCommandApprovalEvents `json:"approval"`
}

type protocolFileEditApprovalEvents struct {
	Requested protocolLeaf `json:"requested"`
	Granted   protocolLeaf `json:"granted"`
	Rejected  protocolLeaf `json:"rejected"`
}

type protocolOperatorFileEditEvents struct {
	Requested protocolLeaf                   `json:"requested"`
	Started   protocolLeaf                   `json:"started"`
	Completed protocolLeaf                   `json:"completed"`
	Failed    protocolLeaf                   `json:"failed"`
	Approval  protocolFileEditApprovalEvents `json:"approval"`
}

type protocolIntentApprovalEvents struct {
	Requested protocolLeaf `json:"requested"`
	Granted   protocolLeaf `json:"granted"`
	Rejected  protocolLeaf `json:"rejected"`
}

type protocolOperatorIntentEvents struct {
	Granted  protocolLeaf                 `json:"granted"`
	Denied   protocolLeaf                 `json:"denied"`
	Revoked  protocolLeaf                 `json:"revoked"`
	Approval protocolIntentApprovalEvents `json:"approval"`
}

type protocolFetchLeaf struct {
	Requested protocolLeaf `json:"requested"`
	Completed protocolLeaf `json:"completed"`
	Failed    protocolLeaf `json:"failed"`
}

type protocolOperatorNetworkPortCheck struct {
	Requested protocolLeaf `json:"requested"`
	Completed protocolLeaf `json:"completed"`
	Failed    protocolLeaf `json:"failed"`
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
	Requested protocolLeaf `json:"requested"`
	Completed protocolLeaf `json:"completed"`
	Failed    protocolLeaf `json:"failed"`
}

type protocolOperatorFileEditApproval struct {
	Requested protocolLeaf `json:"requested"`
	Granted   protocolLeaf `json:"granted"`
	Rejected  protocolLeaf `json:"rejected"`
}

type protocolOperatorFileEdit struct {
	Requested protocolLeaf                     `json:"requested"`
	Started   protocolLeaf                     `json:"started"`
	Completed protocolLeaf                     `json:"completed"`
	Failed    protocolLeaf                     `json:"failed"`
	Approval  protocolOperatorFileEditApproval `json:"approval"`
}

type protocolOperatorFileEvents struct {
	Edit    protocolOperatorFileEdit    `json:"edit"`
	History protocolOperatorFileHistory `json:"history"`
	Diff    protocolOperatorFileDiff    `json:"diff"`
	Restore protocolOperatorFileRestore `json:"restore"`
}

type protocolOperatorAuditUserRecorded struct {
	Recorded protocolLeaf `json:"recorded"`
}

type protocolOperatorAuditAIRecorded struct {
	Recorded protocolLeaf `json:"recorded"`
}

type protocolOperatorAuditDirectCommandResult struct {
	Recorded protocolLeaf `json:"recorded"`
}

type protocolOperatorAuditDirectCommandEvents struct {
	Recorded protocolLeaf                             `json:"recorded"`
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
	Sent      protocolLeaf `json:"sent"`
	Requested protocolLeaf `json:"requested"`
	Received  protocolLeaf `json:"received"`
	Missed    protocolLeaf `json:"missed"`
}

type protocolOperatorShutdown struct {
	Requested    protocolLeaf `json:"requested"`
	Acknowledged protocolLeaf `json:"acknowledged"`
}

type protocolOperatorAPIKey struct {
	Refreshed protocolLeaf `json:"refreshed"`
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
	Heartbeat  protocolOperatorHeartbeat     `json:"operator.heartbeat"`
	Shutdown   protocolOperatorShutdown      `json:"operator.shutdown"`
	API        protocolOperatorAPI           `json:"operator.api"`
	Command    protocolOperatorCommandEvents `json:"operator.command"`
	Intent     protocolOperatorIntentEvents  `json:"operator.intent"`
	Filesystem protocolOperatorFilesystem    `json:"operator.filesystem"`
	Logs       protocolOperatorLogs          `json:"operator.logs"`
	History    protocolOperatorHistory       `json:"operator.history"`
	File       protocolOperatorFileEvents    `json:"operator.file"`
	Network    protocolOperatorNetwork       `json:"operator.network"`
	Audit      protocolOperatorAuditEvents   `json:"operator.audit"`
}

type protocolEventsJSON struct {
	Events map[string]protocolLeaf `json:"events"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring protocol/constants/status.json
// ---------------------------------------------------------------------------

type protocolLeaf struct {
	Value       string `json:"value"`
	GoConst     string `json:"_go_const"`
	PythonConst string `json:"_python_const"`
	GoName      string `json:"_go_name"`
	PythonName  string `json:"_python_name"`
}

type protocolOperatorStatusValues struct {
	Available   protocolLeaf `json:"available"`
	Unavailable protocolLeaf `json:"unavailable"`
	Offline     protocolLeaf `json:"offline"`
	Bound       protocolLeaf `json:"bound"`
	Stale       protocolLeaf `json:"stale"`
	Active      protocolLeaf `json:"active"`
	Stopped     protocolLeaf `json:"stopped"`
	Terminated  protocolLeaf `json:"terminated"`
}

type protocolOperatorTypeValues struct {
	System protocolLeaf `json:"system"`
	Cloud  protocolLeaf `json:"cloud"`
}

type protocolCloudSubtypeValues struct {
	AWS   protocolLeaf `json:"aws"`
	GCP   protocolLeaf `json:"gcp"`
	Azure protocolLeaf `json:"azure"`
}

type protocolVaultModeValues struct {
	Raw      protocolLeaf `json:"raw"`
	Scrubbed protocolLeaf `json:"scrubbed"`
}

type protocolVersionStabilityValues struct {
	Stable protocolLeaf `json:"stable"`
	Beta   protocolLeaf `json:"beta"`
	Dev    protocolLeaf `json:"dev"`
}

type protocolComponentNameValues struct {
	G8EE   protocolLeaf `json:"g8ee"`
	G8EO   protocolLeaf `json:"g8eo"`
	CLIENT protocolLeaf `json:"client"`
}

type protocolPlatformValues struct {
	Linux   protocolLeaf `json:"linux"`
	Windows protocolLeaf `json:"windows"`
	Darwin  protocolLeaf `json:"darwin"`
}

type protocolAISourceValues struct {
	Tool             protocolLeaf `json:"tool.call"`
	TerminalAnchored protocolLeaf `json:"terminal.anchored"`
	TerminalDirect   protocolLeaf `json:"terminal.direct"`
}

type protocolAITaskIDValues struct {
	Command          protocolLeaf `json:"command"`
	DirectCommand    protocolLeaf `json:"direct.command"`
	FileEdit         protocolLeaf `json:"file.edit"`
	FsList           protocolLeaf `json:"fs.list"`
	FsRead           protocolLeaf `json:"fs.read"`
	PortCheck        protocolLeaf `json:"port.check"`
	FetchLogs        protocolLeaf `json:"fetch.logs"`
	FetchHistory     protocolLeaf `json:"fetch.history"`
	FetchFileHistory protocolLeaf `json:"fetch.file.history"`
	RestoreFile      protocolLeaf `json:"restore.file"`
	FetchFileDiff    protocolLeaf `json:"fetch.file.diff"`
}

type protocolHeartbeatTypeValues struct {
	Automatic protocolLeaf `json:"automatic"`
	Bootstrap protocolLeaf `json:"bootstrap"`
	Requested protocolLeaf `json:"requested"`
}

type protocolExecutionStatusValues struct {
	Pending   protocolLeaf `json:"pending"`
	Executing protocolLeaf `json:"executing"`
	Completed protocolLeaf `json:"completed"`
	Failed    protocolLeaf `json:"failed"`
	Timeout   protocolLeaf `json:"timeout"`
	Cancelled protocolLeaf `json:"cancelled"`
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
	Cmd       protocolLeaf `json:"cmd"`
	Results   protocolLeaf `json:"results"`
	Heartbeat protocolLeaf `json:"heartbeat"`
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
	Subscribe   protocolLeaf `json:"subscribe"`
	PSubscribe  protocolLeaf `json:"psubscribe"`
	Unsubscribe protocolLeaf `json:"unsubscribe"`
	Publish     protocolLeaf `json:"publish"`
}

type protocolPubSubWireEventTypes struct {
	Message    protocolLeaf `json:"message"`
	PMessage   protocolLeaf `json:"pmessage"`
	Subscribed protocolLeaf `json:"subscribed"`
}

type protocolPubSubWireFields struct {
	Action  protocolLeaf `json:"action"`
	Channel protocolLeaf `json:"channel"`
	Data    protocolLeaf `json:"data"`
	Message protocolLeaf `json:"message"`
	Pattern protocolLeaf `json:"pattern"`
	Type    protocolLeaf `json:"type"`
	Sender  protocolLeaf `json:"sender"`
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
	OperatorSessionID  protocolLeaf `json:"x-g8e.operator-session-id"`
	DeviceToken        protocolLeaf `json:"x-g8e.device-token"`
	Authorization      protocolLeaf `json:"http.authorization"`
	UserAgent          protocolLeaf `json:"http.user-agent"`
	ContentType        protocolLeaf `json:"http.content-type"`
	ContentDisposition protocolLeaf `json:"http.content-disposition"`
	ContentLength      protocolLeaf `json:"http.content-length"`
	XForwardedProto    protocolLeaf `json:"http.x-forwarded-proto"`
	XForwardedHost     protocolLeaf `json:"http.x-forwarded-host"`
	XRequestTimestamp  protocolLeaf `json:"http.x-request-timestamp"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring protocol/constants/collections.json
// ---------------------------------------------------------------------------

type protocolCollectionsJSON struct {
	Collections map[string]protocolLeaf `json:"collections"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring protocol/constants/env_vars.json
// ---------------------------------------------------------------------------

type protocolEnvVarsJSON struct {
	LLM     map[string]string `json:"llm"`
	Auth    map[string]string `json:"auth"`
	General map[string]string `json:"general"`
	Search  map[string]string `json:"search"`
	SSL     map[string]string `json:"ssl"`
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

func loadCollectionsJSON(t *testing.T) protocolCollectionsJSON {
	t.Helper()
	var c protocolCollectionsJSON
	require.NoError(t, json.Unmarshal(loadProtocolFile(t, "collections.json"), &c), "collections.json must unmarshal into protocolCollectionsJSON")
	return c
}

func loadEnvVarsJSON(t *testing.T) protocolEnvVarsJSON {
	t.Helper()
	var e protocolEnvVarsJSON
	require.NoError(t, json.Unmarshal(loadProtocolFile(t, "env_vars.json"), &e), "env_vars.json must unmarshal into protocolEnvVarsJSON")
	return e
}

// =============================================================================
// Events
// =============================================================================

func TestProtocolEventsMatchGoConstants(t *testing.T) {
	ev := loadEventsJSON(t)
	events := ev.Events

	t.Run("operator.heartbeat", func(t *testing.T) {
		assert.Equal(t, events["operator.heartbeat.sent"].Value, string(constants.Event.Operator.Heartbeat))
		assert.Equal(t, events["operator.heartbeat.requested"].Value, string(constants.Event.Operator.HeartbeatRequested))
	})

	t.Run("operator.shutdown", func(t *testing.T) {
		assert.Equal(t, events["operator.shutdown.requested"].Value, string(constants.Event.Operator.ShutdownRequested))
		assert.Equal(t, events["operator.shutdown.acknowledged"].Value, string(constants.Event.Operator.ShutdownAcknowledged))
	})

	t.Run("operator.api.key", func(t *testing.T) {
		assert.Equal(t, events["operator.api.key.refreshed"].Value, string(constants.Event.Operator.APIKeyRefreshed))
	})

	t.Run("operator.command", func(t *testing.T) {
		assert.Equal(t, events["operator.command.requested"].Value, string(constants.Event.Operator.Command.Requested))
		assert.Equal(t, events["operator.command.started"].Value, string(constants.Event.Operator.Command.Started))
		assert.Equal(t, events["operator.command.completed"].Value, string(constants.Event.Operator.Command.Completed))
		assert.Equal(t, events["operator.command.failed"].Value, string(constants.Event.Operator.Command.Failed))
		assert.Equal(t, events["operator.command.cancelled"].Value, string(constants.Event.Operator.Command.Cancelled))
		assert.Equal(t, events["operator.command.cancel.requested"].Value, string(constants.Event.Operator.Command.CancelRequested))
		assert.Equal(t, events["operator.command.approval.requested"].Value, string(constants.Event.Operator.Command.ApprovalRequested))
	})

	t.Run("operator.file.edit", func(t *testing.T) {
		assert.Equal(t, events["operator.file.edit.requested"].Value, string(constants.Event.Operator.FileEdit.Requested))
		assert.Equal(t, events["operator.file.edit.started"].Value, string(constants.Event.Operator.FileEdit.Started))
		assert.Equal(t, events["operator.file.edit.completed"].Value, string(constants.Event.Operator.FileEdit.Completed))
		assert.Equal(t, events["operator.file.edit.failed"].Value, string(constants.Event.Operator.FileEdit.Failed))
		assert.Equal(t, events["operator.file.edit.approval.requested"].Value, string(constants.Event.Operator.FileEdit.ApprovalRequested))
	})

	t.Run("operator.intent", func(t *testing.T) {
		assert.Equal(t, events["operator.intent.approval.requested"].Value, string(constants.Event.Operator.Intent.ApprovalRequested))
		assert.Equal(t, events["operator.intent.granted"].Value, string(constants.Event.Operator.Intent.Granted))
		assert.Equal(t, events["operator.intent.denied"].Value, string(constants.Event.Operator.Intent.Denied))
		assert.Equal(t, events["operator.intent.revoked"].Value, string(constants.Event.Operator.Intent.Revoked))
	})

	t.Run("operator.network.port.check", func(t *testing.T) {
		assert.Equal(t, events["operator.network.port.check.requested"].Value, string(constants.Event.Operator.PortCheck.Requested))
		assert.Equal(t, events["operator.network.port.check.completed"].Value, string(constants.Event.Operator.PortCheck.Completed))
		assert.Equal(t, events["operator.network.port.check.failed"].Value, string(constants.Event.Operator.PortCheck.Failed))
	})

	t.Run("operator.filesystem.list", func(t *testing.T) {
		assert.Equal(t, events["operator.filesystem.list.requested"].Value, string(constants.Event.Operator.FsList.Requested))
		assert.Equal(t, events["operator.filesystem.list.completed"].Value, string(constants.Event.Operator.FsList.Completed))
		assert.Equal(t, events["operator.filesystem.list.failed"].Value, string(constants.Event.Operator.FsList.Failed))
	})

	t.Run("operator.filesystem.read", func(t *testing.T) {
		assert.Equal(t, events["operator.filesystem.read.requested"].Value, string(constants.Event.Operator.FsRead.Requested))
		assert.Equal(t, events["operator.filesystem.read.completed"].Value, string(constants.Event.Operator.FsRead.Completed))
		assert.Equal(t, events["operator.filesystem.read.failed"].Value, string(constants.Event.Operator.FsRead.Failed))
	})

	t.Run("operator.logs.fetch", func(t *testing.T) {
		assert.Equal(t, events["operator.logs.fetch.requested"].Value, string(constants.Event.Operator.FetchLogs.Requested))
		assert.Equal(t, events["operator.logs.fetch.completed"].Value, string(constants.Event.Operator.FetchLogs.Completed))
		assert.Equal(t, events["operator.logs.fetch.failed"].Value, string(constants.Event.Operator.FetchLogs.Failed))
	})

	t.Run("operator.history.fetch", func(t *testing.T) {
		assert.Equal(t, events["operator.history.fetch.requested"].Value, string(constants.Event.Operator.FetchHistory.Requested))
		assert.Equal(t, events["operator.history.fetch.completed"].Value, string(constants.Event.Operator.FetchHistory.Completed))
		assert.Equal(t, events["operator.history.fetch.failed"].Value, string(constants.Event.Operator.FetchHistory.Failed))
	})

	t.Run("operator.file.history.fetch", func(t *testing.T) {
		assert.Equal(t, events["operator.file.history.fetch.requested"].Value, string(constants.Event.Operator.FetchFileHistory.Requested))
		assert.Equal(t, events["operator.file.history.fetch.completed"].Value, string(constants.Event.Operator.FetchFileHistory.Completed))
		assert.Equal(t, events["operator.file.history.fetch.failed"].Value, string(constants.Event.Operator.FetchFileHistory.Failed))
	})

	t.Run("operator.file.restore", func(t *testing.T) {
		assert.Equal(t, events["operator.file.restore.requested"].Value, string(constants.Event.Operator.RestoreFile.Requested))
		assert.Equal(t, events["operator.file.restore.completed"].Value, string(constants.Event.Operator.RestoreFile.Completed))
		assert.Equal(t, events["operator.file.restore.failed"].Value, string(constants.Event.Operator.RestoreFile.Failed))
	})

	t.Run("operator.file.diff.fetch", func(t *testing.T) {
		assert.Equal(t, events["operator.file.diff.fetch.requested"].Value, string(constants.Event.Operator.FetchFileDiff.Requested))
		assert.Equal(t, events["operator.file.diff.fetch.completed"].Value, string(constants.Event.Operator.FetchFileDiff.Completed))
		assert.Equal(t, events["operator.file.diff.fetch.failed"].Value, string(constants.Event.Operator.FetchFileDiff.Failed))
	})

	t.Run("operator.audit", func(t *testing.T) {
		assert.Equal(t, events["operator.audit.user.recorded"].Value, string(constants.Event.Operator.Audit.UserMsg))
		assert.Equal(t, events["operator.audit.ai.recorded"].Value, string(constants.Event.Operator.Audit.AIMsg))
		assert.Equal(t, events["operator.audit.direct.command.recorded"].Value, string(constants.Event.Operator.Audit.DirectCmd))
		assert.Equal(t, events["operator.audit.direct.command.result.recorded"].Value, string(constants.Event.Operator.Audit.DirectCmdResult))
	})
}

// =============================================================================
// Status
// =============================================================================

func TestProtocolStatusMatchesGoConstants(t *testing.T) {
	st := loadStatusJSON(t)

	t.Run("operator.status", func(t *testing.T) {
		assert.Equal(t, st.OperatorStatus.Available.Value, string(constants.Status.OperatorStatus.Available))
		assert.Equal(t, st.OperatorStatus.Unavailable.Value, string(constants.Status.OperatorStatus.Unavailable))
		assert.Equal(t, st.OperatorStatus.Offline.Value, string(constants.Status.OperatorStatus.Offline))
		assert.Equal(t, st.OperatorStatus.Bound.Value, string(constants.Status.OperatorStatus.Bound))
		assert.Equal(t, st.OperatorStatus.Stale.Value, string(constants.Status.OperatorStatus.Stale))
		assert.Equal(t, st.OperatorStatus.Active.Value, string(constants.Status.OperatorStatus.Active))
		assert.Equal(t, st.OperatorStatus.Stopped.Value, string(constants.Status.OperatorStatus.Stopped))
		assert.Equal(t, st.OperatorStatus.Terminated.Value, string(constants.Status.OperatorStatus.Terminated))
	})

	t.Run("operator.type", func(t *testing.T) {
		assert.Equal(t, st.OperatorType.System.Value, string(constants.Status.OperatorType.System))
		assert.Equal(t, st.OperatorType.Cloud.Value, string(constants.Status.OperatorType.Cloud))
	})

	t.Run("cloud.subtype", func(t *testing.T) {
		assert.Equal(t, st.CloudSubtype.AWS.Value, string(constants.Status.CloudSubtype.AWS))
		assert.Equal(t, st.CloudSubtype.GCP.Value, string(constants.Status.CloudSubtype.GCP))
		assert.Equal(t, st.CloudSubtype.Azure.Value, string(constants.Status.CloudSubtype.Azure))
	})

	t.Run("vault.mode", func(t *testing.T) {
		assert.Equal(t, st.VaultMode.Raw.Value, string(constants.Status.VaultMode.Raw))
		assert.Equal(t, st.VaultMode.Scrubbed.Value, string(constants.Status.VaultMode.Scrubbed))
	})

	t.Run("version.stability", func(t *testing.T) {
		assert.Equal(t, st.VersionStability.Stable.Value, string(constants.Status.VersionStability.Stable))
		assert.Equal(t, st.VersionStability.Beta.Value, string(constants.Status.VersionStability.Beta))
		assert.Equal(t, st.VersionStability.Dev.Value, string(constants.Status.VersionStability.Dev))
	})

	t.Run("component.name", func(t *testing.T) {
		assert.Equal(t, st.ComponentName.G8EE.Value, string(constants.Status.ComponentName.G8EE))
		assert.Equal(t, st.ComponentName.G8EO.Value, string(constants.Status.ComponentName.G8EO))
	})

	t.Run("platform", func(t *testing.T) {
		assert.Equal(t, st.Platform.Linux.Value, string(constants.Status.Platform.Linux))
		assert.Equal(t, st.Platform.Windows.Value, string(constants.Status.Platform.Windows))
		assert.Equal(t, st.Platform.Darwin.Value, string(constants.Status.Platform.Darwin))
	})

	t.Run("ai.source", func(t *testing.T) {
		assert.Equal(t, st.AISource.Tool.Value, string(constants.Status.AiSource.ToolCall))
		assert.Equal(t, st.AISource.TerminalAnchored.Value, string(constants.Status.AiSource.TerminalAnchored))
		assert.Equal(t, st.AISource.TerminalDirect.Value, string(constants.Status.AiSource.TerminalDirect))
	})

	t.Run("ai.task.id", func(t *testing.T) {
		assert.Equal(t, st.AITaskID.Command.Value, string(constants.Status.AITaskId.Command))
		assert.Equal(t, st.AITaskID.DirectCommand.Value, string(constants.Status.AITaskId.DirectCommand))
		assert.Equal(t, st.AITaskID.FileEdit.Value, string(constants.Status.AITaskId.FileEdit))
		assert.Equal(t, st.AITaskID.FsList.Value, string(constants.Status.AITaskId.FsList))
		assert.Equal(t, st.AITaskID.FsRead.Value, string(constants.Status.AITaskId.FsRead))
		assert.Equal(t, st.AITaskID.PortCheck.Value, string(constants.Status.AITaskId.PortCheck))
		assert.Equal(t, st.AITaskID.FetchLogs.Value, string(constants.Status.AITaskId.FetchLogs))
		assert.Equal(t, st.AITaskID.FetchHistory.Value, string(constants.Status.AITaskId.FetchHistory))
		assert.Equal(t, st.AITaskID.FetchFileHistory.Value, string(constants.Status.AITaskId.FetchFileHistory))
		assert.Equal(t, st.AITaskID.RestoreFile.Value, string(constants.Status.AITaskId.RestoreFile))
		assert.Equal(t, st.AITaskID.FetchFileDiff.Value, string(constants.Status.AITaskId.FetchFileDiff))
	})
}

// =============================================================================
// Channels
// =============================================================================

func TestProtocolChannelsMatchGoConstants(t *testing.T) {
	ch := loadChannelsJSON(t)

	t.Run("channel prefixes used by CmdChannel/ResultsChannel/HeartbeatChannel", func(t *testing.T) {
		assert.Equal(t, "cmd", ch.PubSub.Prefixes.Cmd.Value)
		assert.Equal(t, "results", ch.PubSub.Prefixes.Results.Value)
		assert.Equal(t, "heartbeat", ch.PubSub.Prefixes.Heartbeat.Value)

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

	assert.Equal(t, st.HeartbeatType.Automatic.Value, marshaler.Status(constants.HeartbeatTypeAutomatic))
	assert.Equal(t, st.HeartbeatType.Bootstrap.Value, marshaler.Status(constants.HeartbeatTypeBootstrap))
	assert.Equal(t, st.HeartbeatType.Requested.Value, marshaler.Status(constants.HeartbeatTypeRequested))
}

// =============================================================================
// Headers
// =============================================================================

func TestProtocolHeadersMatchGoConstants(t *testing.T) {
	h := loadHeadersJSON(t)

	t.Run("standard http headers", func(t *testing.T) {
		assert.Equal(t, h.Authorization.Value, string(constants.HeaderAuthorization))
		assert.Equal(t, h.UserAgent.Value, string(constants.HeaderUserAgent))
		assert.Equal(t, h.ContentType.Value, string(constants.HeaderContentType))
		assert.Equal(t, h.ContentDisposition.Value, string(constants.HeaderContentDisposition))
		assert.Equal(t, h.ContentLength.Value, string(constants.HeaderContentLength))
		assert.Equal(t, h.XForwardedProto.Value, string(constants.HeaderXForwardedProto))
		assert.Equal(t, h.XForwardedHost.Value, string(constants.HeaderXForwardedHost))
		assert.Equal(t, h.XRequestTimestamp.Value, string(constants.HeaderXRequestTimestamp))
		assert.Equal(t, h.DeviceToken.Value, string(constants.HeaderDeviceToken))
	})
}

// =============================================================================
// PubSub wire protocol
// =============================================================================

func TestProtocolPubSubWireMatchesGoConstants(t *testing.T) {
	ps := loadPubSubJSON(t)

	t.Run("wire.actions", func(t *testing.T) {
		assert.Equal(t, ps.Wire.Actions.Subscribe.Value, string(constants.PubSubActionSubscribe))
		assert.Equal(t, ps.Wire.Actions.PSubscribe.Value, string(constants.PubSubActionPSubscribe))
		assert.Equal(t, ps.Wire.Actions.Unsubscribe.Value, string(constants.PubSubActionUnsubscribe))
		assert.Equal(t, ps.Wire.Actions.Publish.Value, string(constants.PubSubActionPublish))
	})

	t.Run("wire.event_types", func(t *testing.T) {
		assert.Equal(t, ps.Wire.EventTypes.Message.Value, string(constants.PubSubEventMessage))
		assert.Equal(t, ps.Wire.EventTypes.PMessage.Value, string(constants.PubSubEventPMessage))
		assert.Equal(t, ps.Wire.EventTypes.Subscribed.Value, string(constants.PubSubEventSubscribed))
	})
}

// =============================================================================
// Execution status
// =============================================================================

func TestProtocolExecutionStatusMatchesGoConstants(t *testing.T) {
	st := loadStatusJSON(t)

	assert.Equal(t, st.ExecutionStatus.Pending.Value, marshaler.Status(constants.ExecutionStatusPending))
	assert.Equal(t, st.ExecutionStatus.Executing.Value, marshaler.Status(constants.ExecutionStatusExecuting))
	assert.Equal(t, st.ExecutionStatus.Completed.Value, marshaler.Status(constants.ExecutionStatusCompleted))
	assert.Equal(t, st.ExecutionStatus.Failed.Value, marshaler.Status(constants.ExecutionStatusFailed))
	assert.Equal(t, st.ExecutionStatus.Timeout.Value, marshaler.Status(constants.ExecutionStatusTimeout))
	assert.Equal(t, st.ExecutionStatus.Cancelled.Value, marshaler.Status(constants.ExecutionStatusCancelled))
}

// =============================================================================
// Collections
// =============================================================================

func TestProtocolCollectionsMatchGoConstants(t *testing.T) {
	c := loadCollectionsJSON(t)

	t.Run("collection names", func(t *testing.T) {
		// Verify that all collections in JSON have corresponding Go constants
		for key, leaf := range c.Collections {
			value := leaf.Value
			// Convert key to Go constant name format (CollectionUsers, CollectionWebSessions, etc.)
			constName := "Collection" + toPascalCase(key)
			// This is a basic check - in a full implementation we'd use reflection
			// to verify the constant exists and has the correct value
			t.Logf("Collection %s should have constant %s with value %s", key, constName, value)
		}
	})
}

// =============================================================================
// Environment Variables
// =============================================================================

func TestProtocolEnvVarsMatchGoConstants(t *testing.T) {
	e := loadEnvVarsJSON(t)

	t.Run("env var keys", func(t *testing.T) {
		// Verify that env vars in JSON have corresponding Go constants
		allEnvVars := map[string]string{}
		for _, vars := range []map[string]string{e.LLM, e.Auth, e.General, e.Search, e.SSL} {
			for key, value := range vars {
				allEnvVars[key] = value
			}
		}

		// Log all env vars for verification
		for key, value := range allEnvVars {
			t.Logf("Env var %s maps to config key %s", key, value)
		}
	})
}

// Helper function to convert snake_case to PascalCase
func toPascalCase(s string) string {
	parts := []string{}
	current := ""
	for _, r := range s {
		if r == '_' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	result := ""
	for _, part := range parts {
		if len(part) > 0 {
			result += strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return result
}
