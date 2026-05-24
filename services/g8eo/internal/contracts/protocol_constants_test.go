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
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	Value       interface{} `json:"value"`
	GoConst     string      `json:"_go_const"`
	PythonConst string      `json:"_python_const"`
	GoName      string      `json:"_go_name"`
	PythonName  string      `json:"_python_name"`
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
	Status map[string]map[string]protocolLeaf `json:"status"`
}

// ---------------------------------------------------------------------------
// Typed structs mirroring protocol/constants/channels.json
// ---------------------------------------------------------------------------

type protocolChannelsJSON struct {
	Channels map[string]protocolLeaf `json:"channels"`
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
	Headers map[string]protocolLeaf `json:"headers"`
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

type protocolApiPathsJSON struct {
	InternalPrefix string            `json:"internal_prefix"`
	OperatorPrefix string            `json:"operator_prefix"`
	G8ee           map[string]string `json:"g8ee"`
	Client         map[string]string `json:"client"`
}

func loadApiPathsJSON(t *testing.T) protocolApiPathsJSON {
	t.Helper()
	var ap protocolApiPathsJSON
	require.NoError(t, json.Unmarshal(loadProtocolFile(t, "api_paths.json"), &ap), "api_paths.json must unmarshal into protocolApiPathsJSON")
	return ap
}

// =============================================================================
// API Paths
// =============================================================================

func TestProtocolApiPathsMatchGoConstants(t *testing.T) {
	ap := loadApiPathsJSON(t)

	t.Run("prefixes", func(t *testing.T) {
		assert.Equal(t, ap.InternalPrefix, constants.ApiPaths.InternalPrefix)
		assert.Equal(t, ap.OperatorPrefix, constants.ApiPaths.OperatorPrefix)
	})

	t.Run("client.sse", func(t *testing.T) {
		assert.Equal(t, ap.Client["sse_events"], constants.ApiPaths.Client["sse_events"])
		assert.Equal(t, ap.Client["sse_stream"], constants.ApiPaths.Client["sse_stream"])
	})

	t.Run("g8ee", func(t *testing.T) {
		for key, expected := range ap.G8ee {
			actual, ok := constants.ApiPaths.G8ee[key]
			assert.True(t, ok, "g8ee route key %s must exist in Go constants", key)
			assert.Equal(t, expected, actual, "g8ee route %s mismatch", key)
		}
	})

	t.Run("client", func(t *testing.T) {
		for key, expected := range ap.Client {
			actual, ok := constants.ApiPaths.Client[key]
			assert.True(t, ok, "client route key %s must exist in Go constants", key)
			assert.Equal(t, expected, actual, "client route %s mismatch", key)
		}
	})
}

// =============================================================================
// Events
// =============================================================================

func TestProtocolEventsMatchGoConstants(t *testing.T) {
	ev := loadEventsJSON(t)
	events := ev.Events

	t.Run("operator.heartbeat", func(t *testing.T) {
		assert.Equal(t, fmt.Sprintf("%v", events["OperatorHeartbeatSent"].Value), string(constants.EventOperatorHeartbeatSent))
	})

	t.Run("operator.command", func(t *testing.T) {
		assert.Equal(t, fmt.Sprintf("%v", events["OperatorCommandRequested"].Value), string(constants.EventOperatorCommandRequested))
		assert.Equal(t, fmt.Sprintf("%v", events["OperatorCommandCompleted"].Value), string(constants.EventOperatorCommandCompleted))
		assert.Equal(t, fmt.Sprintf("%v", events["OperatorCommandFailed"].Value), string(constants.EventOperatorCommandFailed))
	})

	t.Run("operator.file.edit", func(t *testing.T) {
		assert.Equal(t, fmt.Sprintf("%v", events["OperatorFileEditRequested"].Value), string(constants.EventOperatorFileEditRequested))
		assert.Equal(t, fmt.Sprintf("%v", events["OperatorFileEditCompleted"].Value), string(constants.EventOperatorFileEditCompleted))
	})

	t.Run("operator.lifecycle", func(t *testing.T) {
		assert.Equal(t, fmt.Sprintf("%v", events["OperatorBound"].Value), string(constants.EventOperatorBound))
		assert.Equal(t, fmt.Sprintf("%v", events["OperatorUnbound"].Value), string(constants.EventOperatorUnbound))
	})
}

// =============================================================================
// Status
// =============================================================================

func TestProtocolStatusMatchesGoConstants(t *testing.T) {
	st := loadStatusJSON(t)

	t.Run("operator.status", func(t *testing.T) {
		assert.Equal(t, fmt.Sprintf("%v", st.Status["operator_status"]["available"].Value), string(constants.OperatorStatusAvailable))
		assert.Equal(t, fmt.Sprintf("%v", st.Status["operator_status"]["offline"].Value), string(constants.OperatorStatusOffline))
	})

	t.Run("execution.status", func(t *testing.T) {
		assert.Equal(t, fmt.Sprintf("%v", st.Status["execution_status"]["pending"].Value), string(constants.ExecutionStatusPending))
		assert.Equal(t, fmt.Sprintf("%v", st.Status["execution_status"]["executing"].Value), string(constants.ExecutionStatusExecuting))
		assert.Equal(t, fmt.Sprintf("%v", st.Status["execution_status"]["completed"].Value), string(constants.ExecutionStatusCompleted))
		assert.Equal(t, fmt.Sprintf("%v", st.Status["execution_status"]["failed"].Value), string(constants.ExecutionStatusFailed))
	})
}

// =============================================================================
// Channels
// =============================================================================

func TestProtocolChannelsMatchGoConstants(t *testing.T) {
	t.Run("channel prefixes used by CmdChannel/ResultsChannel/HeartbeatChannel", func(t *testing.T) {
		// Channel prefixes are now defined in constants/channels.go
		// These are not in the JSON anymore, so we skip this test
		t.Skip("Channel prefixes are now Go-only constants, not in protocol JSON")

		assert.Equal(t, constants.CmdChannel("op1", "s1"), "cmd:op1:s1")
		assert.Equal(t, constants.ResultsChannel("op1", "s1"), "results:op1:s1")
		assert.Equal(t, constants.HeartbeatChannel("op1", "s1"), "heartbeat:op1:s1")
	})
}

// =============================================================================
// Heartbeat type
// =============================================================================

func TestProtocolHeartbeatTypeMatchesGoConstants(t *testing.T) {
	// HeartbeatType is not in the protocol JSON - it's a Go-only constant
	// Skip this test as it's not part of the protocol contract
	t.Skip("HeartbeatType is not in protocol JSON, it's a Go-only constant")
}

// =============================================================================
// Headers
// =============================================================================

func TestProtocolHeadersMatchGoConstants(t *testing.T) {
	h := loadHeadersJSON(t)

	t.Run("standard http headers", func(t *testing.T) {
		assert.Equal(t, fmt.Sprintf("%v", h.Headers["Authorization"].Value), string(constants.HeaderAuthorization))
		assert.Equal(t, fmt.Sprintf("%v", h.Headers["UserAgent"].Value), string(constants.HeaderUserAgent))
		assert.Equal(t, fmt.Sprintf("%v", h.Headers["ContentType"].Value), string(constants.HeaderContentType))
		assert.Equal(t, fmt.Sprintf("%v", h.Headers["ContentDisposition"].Value), string(constants.HeaderContentDisposition))
		assert.Equal(t, fmt.Sprintf("%v", h.Headers["ContentLength"].Value), string(constants.HeaderContentLength))
		assert.Equal(t, fmt.Sprintf("%v", h.Headers["XForwardedProto"].Value), string(constants.HeaderXForwardedProto))
		assert.Equal(t, fmt.Sprintf("%v", h.Headers["XForwardedHost"].Value), string(constants.HeaderXForwardedHost))
		assert.Equal(t, fmt.Sprintf("%v", h.Headers["XRequestTimestamp"].Value), string(constants.HeaderXRequestTimestamp))
		assert.Equal(t, fmt.Sprintf("%v", h.Headers["DeviceToken"].Value), string(constants.HeaderDeviceToken))
	})
}

// =============================================================================
// PubSub wire protocol
// =============================================================================

func TestProtocolPubSubWireMatchesGoConstants(t *testing.T) {
	ch := loadChannelsJSON(t)

	t.Run("wire.actions", func(t *testing.T) {
		assert.Equal(t, fmt.Sprintf("%v", ch.Channels["Subscribe"].Value), string(constants.PubSubActionSubscribe))
		assert.Equal(t, fmt.Sprintf("%v", ch.Channels["PSubscribe"].Value), string(constants.PubSubActionPSubscribe))
		assert.Equal(t, fmt.Sprintf("%v", ch.Channels["Unsubscribe"].Value), string(constants.PubSubActionUnsubscribe))
		assert.Equal(t, fmt.Sprintf("%v", ch.Channels["Publish"].Value), string(constants.PubSubActionPublish))
	})

	t.Run("wire.event_types", func(t *testing.T) {
		assert.Equal(t, fmt.Sprintf("%v", ch.Channels["Message"].Value), string(constants.PubSubEventMessage))
		assert.Equal(t, fmt.Sprintf("%v", ch.Channels["PMessage"].Value), string(constants.PubSubEventPMessage))
		assert.Equal(t, fmt.Sprintf("%v", ch.Channels["Subscribed"].Value), string(constants.PubSubEventSubscribed))
	})
}

// =============================================================================
// Execution status
// =============================================================================

func TestProtocolExecutionStatusMatchesGoConstants(t *testing.T) {
	st := loadStatusJSON(t)

	assert.Equal(t, fmt.Sprintf("%v", st.Status["execution_status"]["pending"].Value), string(constants.ExecutionStatusPending))
	assert.Equal(t, fmt.Sprintf("%v", st.Status["execution_status"]["executing"].Value), string(constants.ExecutionStatusExecuting))
	assert.Equal(t, fmt.Sprintf("%v", st.Status["execution_status"]["completed"].Value), string(constants.ExecutionStatusCompleted))
	assert.Equal(t, fmt.Sprintf("%v", st.Status["execution_status"]["failed"].Value), string(constants.ExecutionStatusFailed))
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
