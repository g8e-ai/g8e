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
Shared Constants Contract Tests

Verifies that g8eo Go constants in constants/events.go and constants/status.go
exactly match the canonical values in shared/constants/*.json.

g8eo duplicates shared JSON values as compile-time Go constants (no //go:embed,
no runtime loading). These tests are the enforcement mechanism that detects
drift between the JSON source of truth and the Go consumer.
*/
package contracts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sharedConstantsDir string

func init() {
	// shared/constants/ is two levels up from g8eoRoot: g8eo -> components -> repo root
	dir, err := filepath.Abs(filepath.Join(g8eoRoot, "../../shared/constants"))
	if err != nil {
		panic(fmt.Sprintf("failed to resolve shared constants dir: %v", err))
	}
	sharedConstantsDir = dir
}

func loadSharedFile(t *testing.T, filename string) []byte {
	t.Helper()
	path := filepath.Join(sharedConstantsDir, filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "shared/constants/%s must be readable", filename)
	return data
}

// ---------------------------------------------------------------------------
// Typed structs mirroring shared/constants/events.json
// ---------------------------------------------------------------------------

type sharedCommandOutputEvents struct {
	Received string `json:"received"`
}

type sharedCommandCancelEvents struct {
	Requested    string `json:"requested"`
	Acknowledged string `json:"acknowledged"`
	Failed       string `json:"failed"`
}

type sharedCommandApprovalEvents struct {
	Requested string `json:"requested"`
	Granted   string `json:"granted"`
	Rejected  string `json:"rejected"`
}

type sharedOperatorCommandEvents struct {
	Requested string                      `json:"requested"`
	Started   string                      `json:"started"`
	Completed string                      `json:"completed"`
	Failed    string                      `json:"failed"`
	Cancelled string                      `json:"cancelled"`
	Output    sharedCommandOutputEvents   `json:"output"`
	Cancel    sharedCommandCancelEvents   `json:"cancel"`
	Approval  sharedCommandApprovalEvents `json:"approval"`
}

type sharedFileEditApprovalEvents struct {
	Requested string `json:"requested"`
	Granted   string `json:"granted"`
	Rejected  string `json:"rejected"`
}

type sharedOperatorFileEditEvents struct {
	Requested string                       `json:"requested"`
	Started   string                       `json:"started"`
	Completed string                       `json:"completed"`
	Failed    string                       `json:"failed"`
	Approval  sharedFileEditApprovalEvents `json:"approval"`
}

type sharedIntentApprovalEvents struct {
	Requested string `json:"requested"`
	Granted   string `json:"granted"`
	Rejected  string `json:"rejected"`
}

type sharedOperatorIntentEvents struct {
	Granted  string                     `json:"granted"`
	Denied   string                     `json:"denied"`
	Revoked  string                     `json:"revoked"`
	Approval sharedIntentApprovalEvents `json:"approval"`
}

type sharedFetchLeaf struct {
	Requested string `json:"requested"`
	Completed string `json:"completed"`
	Failed    string `json:"failed"`
}

type sharedOperatorNetworkPortCheck struct {
	Requested string `json:"requested"`
	Completed string `json:"completed"`
	Failed    string `json:"failed"`
}

type sharedOperatorNetworkPort struct {
	Check sharedOperatorNetworkPortCheck `json:"check"`
}

type sharedOperatorNetwork struct {
	Port sharedOperatorNetworkPort `json:"port"`
}

type sharedOperatorFilesystem struct {
	List sharedFetchLeaf `json:"list"`
	Read sharedFetchLeaf `json:"read"`
}

type sharedOperatorFileHistory struct {
	Fetch sharedFetchLeaf `json:"fetch"`
}

type sharedOperatorFileDiff struct {
	Fetch sharedFetchLeaf `json:"fetch"`
}

type sharedOperatorFileRestore struct {
	Requested string `json:"requested"`
	Completed string `json:"completed"`
	Failed    string `json:"failed"`
}

type sharedOperatorFileEditApproval struct {
	Requested string `json:"requested"`
	Granted   string `json:"granted"`
	Rejected  string `json:"rejected"`
}

type sharedOperatorFileEdit struct {
	Requested string                         `json:"requested"`
	Started   string                         `json:"started"`
	Completed string                         `json:"completed"`
	Failed    string                         `json:"failed"`
	Approval  sharedOperatorFileEditApproval `json:"approval"`
}

type sharedOperatorFileEvents struct {
	Edit    sharedOperatorFileEdit    `json:"edit"`
	History sharedOperatorFileHistory `json:"history"`
	Diff    sharedOperatorFileDiff    `json:"diff"`
	Restore sharedOperatorFileRestore `json:"restore"`
}

type sharedOperatorAuditUserRecorded struct {
	Recorded string `json:"recorded"`
}

type sharedOperatorAuditAIRecorded struct {
	Recorded string `json:"recorded"`
}

type sharedOperatorAuditDirectCommandResult struct {
	Recorded string `json:"recorded"`
}

type sharedOperatorAuditDirectCommandEvents struct {
	Recorded string                                 `json:"recorded"`
	Result   sharedOperatorAuditDirectCommandResult `json:"result"`
}

type sharedOperatorAuditDirectEvents struct {
	Command sharedOperatorAuditDirectCommandEvents `json:"command"`
}

type sharedOperatorAuditEvents struct {
	User   sharedOperatorAuditUserRecorded `json:"user"`
	AI     sharedOperatorAuditAIRecorded   `json:"ai"`
	Direct sharedOperatorAuditDirectEvents `json:"direct"`
}

type sharedOperatorHeartbeat struct {
	Sent      string `json:"sent"`
	Requested string `json:"requested"`
	Received  string `json:"received"`
	Missed    string `json:"missed"`
}

type sharedOperatorShutdown struct {
	Requested    string `json:"requested"`
	Acknowledged string `json:"acknowledged"`
}

type sharedOperatorAPIKey struct {
	Refreshed string `json:"refreshed"`
}

type sharedOperatorAPI struct {
	Key sharedOperatorAPIKey `json:"key"`
}

type sharedOperatorLogs struct {
	Fetch sharedFetchLeaf `json:"fetch"`
}

type sharedOperatorHistory struct {
	Fetch sharedFetchLeaf `json:"fetch"`
}

type sharedOperatorEvents struct {
	Heartbeat  sharedOperatorHeartbeat     `json:"heartbeat"`
	Shutdown   sharedOperatorShutdown      `json:"shutdown"`
	API        sharedOperatorAPI           `json:"api"`
	Command    sharedOperatorCommandEvents `json:"command"`
	Intent     sharedOperatorIntentEvents  `json:"intent"`
	Filesystem sharedOperatorFilesystem    `json:"filesystem"`
	Logs       sharedOperatorLogs          `json:"logs"`
	History    sharedOperatorHistory       `json:"history"`
	File       sharedOperatorFileEvents    `json:"file"`
	Network    sharedOperatorNetwork       `json:"network"`
	Audit      sharedOperatorAuditEvents   `json:"audit"`
}

type sharedEventsJSON struct {
	Operator sharedOperatorEvents `json:"operator"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring shared/constants/status.json
// ---------------------------------------------------------------------------

type sharedOperatorStatusValues struct {
	Available   string `json:"available"`
	Unavailable string `json:"unavailable"`
	Offline     string `json:"offline"`
	Bound       string `json:"bound"`
	Stale       string `json:"stale"`
	Active      string `json:"active"`
	Stopped     string `json:"stopped"`
	Terminated  string `json:"terminated"`
}

type sharedOperatorTypeValues struct {
	System string `json:"system"`
	Cloud  string `json:"cloud"`
}

type sharedCloudSubtypeValues struct {
	AWS     string `json:"aws"`
	GCP     string `json:"gcp"`
	Azure   string `json:"azure"`
	G8eNode string `json:"g8ep"`
}

type sharedVaultModeValues struct {
	Raw      string `json:"raw"`
	Scrubbed string `json:"scrubbed"`
}

type sharedVersionStabilityValues struct {
	Stable string `json:"stable"`
	Beta   string `json:"beta"`
	Dev    string `json:"dev"`
}

type sharedComponentNameValues struct {
	G8EE string `json:"g8ee"`
	G8EO string `json:"g8eo"`
	G8ED string `json:"g8ed"`
}

type sharedPlatformValues struct {
	Linux   string `json:"linux"`
	Windows string `json:"windows"`
	Darwin  string `json:"darwin"`
}

type sharedAISourceValues struct {
	Tool             string `json:"tool.call"`
	TerminalAnchored string `json:"terminal.anchored"`
	TerminalDirect   string `json:"terminal.direct"`
}

type sharedAITaskIDValues struct {
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

type sharedAuthModeValues struct {
	APIKey string `json:"api_key"`
}

type sharedHeartbeatTypeValues struct {
	Automatic string `json:"automatic"`
	Bootstrap string `json:"bootstrap"`
	Requested string `json:"requested"`
}

type sharedExecutionStatusValues struct {
	Pending   string `json:"pending"`
	Executing string `json:"executing"`
	Completed string `json:"completed"`
	Failed    string `json:"failed"`
	Timeout   string `json:"timeout"`
	Cancelled string `json:"cancelled"`
}

type sharedStatusJSON struct {
	OperatorStatus   sharedOperatorStatusValues   `json:"g8e.status"`
	OperatorType     sharedOperatorTypeValues     `json:"g8e.type"`
	CloudSubtype     sharedCloudSubtypeValues     `json:"cloud.subtype"`
	VaultMode        sharedVaultModeValues        `json:"vault.mode"`
	VersionStability sharedVersionStabilityValues `json:"version.stability"`
	ComponentName    sharedComponentNameValues    `json:"component.name"`
	Platform         sharedPlatformValues         `json:"platform"`
	AISource         sharedAISourceValues         `json:"ai.source"`
	AITaskID         sharedAITaskIDValues         `json:"ai.task.id"`
	AuthMode         sharedAuthModeValues         `json:"auth.mode"`
	HeartbeatType    sharedHeartbeatTypeValues    `json:"heartbeat.type"`
	ExecutionStatus  sharedExecutionStatusValues  `json:"execution.status"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring shared/constants/channels.json
// ---------------------------------------------------------------------------

type sharedChannelPrefixes struct {
	Cmd       string `json:"cmd"`
	Results   string `json:"results"`
	Heartbeat string `json:"heartbeat"`
}

type sharedPubSubChannels struct {
	Prefixes sharedChannelPrefixes `json:"prefixes"`
}

type sharedChannelsJSON struct {
	PubSub sharedPubSubChannels `json:"pubsub"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring shared/constants/pubsub.json
// ---------------------------------------------------------------------------

type sharedPubSubWireActions struct {
	Subscribe   string `json:"subscribe"`
	PSubscribe  string `json:"psubscribe"`
	Unsubscribe string `json:"unsubscribe"`
	Publish     string `json:"publish"`
}

type sharedPubSubWireEventTypes struct {
	Message    string `json:"message"`
	PMessage   string `json:"pmessage"`
	Subscribed string `json:"subscribed"`
}

type sharedPubSubWireFields struct {
	Action  string `json:"action"`
	Channel string `json:"channel"`
	Data    string `json:"data"`
	Message string `json:"message"`
	Pattern string `json:"pattern"`
	Type    string `json:"type"`
	Sender  string `json:"sender"`
}

type sharedPubSubWire struct {
	Actions    sharedPubSubWireActions    `json:"actions"`
	EventTypes sharedPubSubWireEventTypes `json:"event_types"`
	Fields     sharedPubSubWireFields     `json:"fields"`
}

type sharedPubSubJSON struct {
	Wire sharedPubSubWire `json:"wire"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring shared/constants/headers.json
// ---------------------------------------------------------------------------

type sharedHeadersJSON struct {
	G8eSessionID       string `json:"x-g8e.session-id"`
	G8eUserID          string `json:"x-g8e.user-id"`
	G8eOrganizationID  string `json:"x-g8e.organization-id"`
	G8eCaseID          string `json:"x-g8e.case-id"`
	G8eInvestigationID string `json:"x-g8e.investigation-id"`
	G8eTaskID          string `json:"x-g8e.task-id"`
	G8eSourceComponent string `json:"x-g8e.source-component"`
	G8eBoundOperators  string `json:"x-g8e.bound-operators"`
	G8eRequestID       string `json:"x-g8e.execution-id"`
	G8eService         string `json:"x-g8e.service"`
	G8eClient          string `json:"x-g8e.client"`
	G8eOperatorStatus  string `json:"x-g8e.operator-status"`
	Authorization      string `json:"http.authorization"`
	UserAgent          string `json:"http.user-agent"`
	ContentType        string `json:"http.content-type"`
	ContentDisposition string `json:"http.content-disposition"`
	ContentLength      string `json:"http.content-length"`
	XForwardedProto    string `json:"http.x-forwarded-proto"`
	XForwardedHost     string `json:"http.x-forwarded-host"`
	XRequestTimestamp  string `json:"http.x-request-timestamp"`
}

func loadEventsJSON(t *testing.T) sharedEventsJSON {
	t.Helper()
	var ev sharedEventsJSON
	require.NoError(t, json.Unmarshal(loadSharedFile(t, "events.json"), &ev), "events.json must unmarshal into sharedEventsJSON")
	return ev
}

func loadStatusJSON(t *testing.T) sharedStatusJSON {
	t.Helper()
	var st sharedStatusJSON
	require.NoError(t, json.Unmarshal(loadSharedFile(t, "status.json"), &st), "status.json must unmarshal into sharedStatusJSON")
	return st
}

func loadChannelsJSON(t *testing.T) sharedChannelsJSON {
	t.Helper()
	var ch sharedChannelsJSON
	require.NoError(t, json.Unmarshal(loadSharedFile(t, "channels.json"), &ch), "channels.json must unmarshal into sharedChannelsJSON")
	return ch
}

func loadPubSubJSON(t *testing.T) sharedPubSubJSON {
	t.Helper()
	var ps sharedPubSubJSON
	require.NoError(t, json.Unmarshal(loadSharedFile(t, "pubsub.json"), &ps), "pubsub.json must unmarshal into sharedPubSubJSON")
	return ps
}

func loadHeadersJSON(t *testing.T) sharedHeadersJSON {
	t.Helper()
	var h sharedHeadersJSON
	require.NoError(t, json.Unmarshal(loadSharedFile(t, "headers.json"), &h), "headers.json must unmarshal into sharedHeadersJSON")
	return h
}

// =============================================================================
// Events
// =============================================================================

func TestSharedEventsMatchGoConstants(t *testing.T) {
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

func TestSharedStatusMatchesGoConstants(t *testing.T) {
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
		assert.Equal(t, st.CloudSubtype.G8eNode, constants.Status.CloudSubtype.G8eNode)
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
		assert.Equal(t, st.ComponentName.G8ED, constants.Status.ComponentName.G8ED)
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

	t.Run("auth.mode", func(t *testing.T) {
		assert.Equal(t, st.AuthMode.APIKey, constants.Status.AuthMode.APIKey)
	})
}

// =============================================================================
// Channels
// =============================================================================

func TestSharedChannelsMatchGoConstants(t *testing.T) {
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

func TestSharedHeartbeatTypeMatchesGoConstants(t *testing.T) {
	st := loadStatusJSON(t)

	assert.Equal(t, st.HeartbeatType.Automatic, string(constants.HeartbeatTypeAutomatic))
	assert.Equal(t, st.HeartbeatType.Bootstrap, string(constants.HeartbeatTypeBootstrap))
	assert.Equal(t, st.HeartbeatType.Requested, string(constants.HeartbeatTypeRequested))
}

// =============================================================================
// Headers
// =============================================================================

func TestSharedHeadersMatchGoConstants(t *testing.T) {
	h := loadHeadersJSON(t)

	t.Run("x-g8e headers", func(t *testing.T) {
		assert.Equal(t, h.G8eSessionID, constants.HeaderG8eWebSessionID)
		assert.Equal(t, h.G8eUserID, constants.HeaderG8eUserID)
		assert.Equal(t, h.G8eOrganizationID, constants.HeaderG8eOrganizationID)
		assert.Equal(t, h.G8eCaseID, constants.HeaderG8eCaseID)
		assert.Equal(t, h.G8eInvestigationID, constants.HeaderG8eInvestigationID)
		assert.Equal(t, h.G8eTaskID, constants.HeaderG8eTaskID)
		assert.Equal(t, h.G8eSourceComponent, constants.HeaderG8eSourceComponent)
		assert.Equal(t, h.G8eBoundOperators, constants.HeaderG8eBoundOperators)
		assert.Equal(t, h.G8eRequestID, constants.HeaderG8eRequestID)
		assert.Equal(t, h.G8eService, constants.HeaderG8eService)
		assert.Equal(t, h.G8eClient, constants.HeaderG8eClient)
		assert.Equal(t, h.G8eOperatorStatus, constants.HeaderG8eOperatorStatus)
	})

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

func TestSharedPubSubWireMatchesGoConstants(t *testing.T) {
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

func TestSharedExecutionStatusMatchesGoConstants(t *testing.T) {
	st := loadStatusJSON(t)

	assert.Equal(t, st.ExecutionStatus.Pending, string(constants.ExecutionStatusPending))
	assert.Equal(t, st.ExecutionStatus.Executing, string(constants.ExecutionStatusExecuting))
	assert.Equal(t, st.ExecutionStatus.Completed, string(constants.ExecutionStatusCompleted))
	assert.Equal(t, st.ExecutionStatus.Failed, string(constants.ExecutionStatusFailed))
	assert.Equal(t, st.ExecutionStatus.Timeout, string(constants.ExecutionStatusTimeout))
	assert.Equal(t, st.ExecutionStatus.Cancelled, string(constants.ExecutionStatusCancelled))
}
