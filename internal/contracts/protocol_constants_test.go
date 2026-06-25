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
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var protocolConstantsDir string

func init() {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = constants.PathCurrentDir
	}
	current := cwd
	for {
		if _, err := os.Stat(filepath.Join(current, constants.TestProtocolDirname)); err == nil {
			protocolConstantsDir = filepath.Join(current, constants.TestProtocolDirname, constants.TestProtocolConstantsDirname)
			break
		}
		if _, err := os.Stat(filepath.Join(current, constants.GitDirname)); err == nil {
			protocolConstantsDir = filepath.Join(current, constants.TestProtocolDirname, constants.TestProtocolConstantsDirname)
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			protocolConstantsDir = filepath.Join(cwd, constants.TestProtocolDirname, constants.TestProtocolConstantsDirname)
			break
		}
		current = parent
	}
	if err := paths.InitWithBase(filepath.Dir(filepath.Dir(protocolConstantsDir))); err != nil {
		panic(fmt.Errorf("protocol_constants_test: init paths: %w", err))
	}
}

func loadProtocolFile(t *testing.T, filename string) []byte {
	t.Helper()
	path := filepath.Join(protocolConstantsDir, filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "protocol/constants/%s must be readable", filename)
	return data
}

type protocolLeaf struct {
	Value       string `json:"value"`
	GoConst     string `json:"_go_const"`
	PythonConst string `json:"_python_const"`
	GoName      string `json:"_go_name"`
	PythonName  string `json:"_python_name"`
}

type protocolEventsJSON struct {
	Events map[string]protocolLeaf `json:"events"`
}

type protocolStatusJSON struct {
	Status map[string]map[string]protocolLeaf `json:"status"`
}

type protocolChannelsJSON struct {
	Channels map[string]protocolLeaf `json:"channels"`
}

type protocolHeadersJSON struct {
	Headers map[string]protocolLeaf `json:"headers"`
}

type protocolCollectionsJSON struct {
	Collections map[string]protocolLeaf `json:"collections"`
}

type protocolEnvVarsJSON struct {
	EnvVars map[string]protocolLeaf `json:"env_vars"`
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

type protocolAPIPathsJSON struct {
	InternalPrefix string            `json:"internal_prefix"`
	OperatorPrefix string            `json:"operator_prefix"`
	Client         map[string]string `json:"client"`
}

func loadAPIPathsJSON(t *testing.T) protocolAPIPathsJSON {
	t.Helper()
	var ap protocolAPIPathsJSON
	require.NoError(t, json.Unmarshal(loadProtocolFile(t, "api_paths.json"), &ap), "api_paths.json must unmarshal into protocolAPIPathsJSON")
	return ap
}

// =============================================================================
// API Paths
// =============================================================================

func TestProtocolAPIPathsMatchGoConstants(t *testing.T) {
	ap := loadAPIPathsJSON(t)

	t.Run("prefixes", func(t *testing.T) {
		assert.Equal(t, ap.InternalPrefix, constants.APIPaths.InternalPrefix)
		assert.Equal(t, ap.OperatorPrefix, constants.APIPaths.OperatorPrefix)
	})

	t.Run("client.sse", func(t *testing.T) {
		assert.Equal(t, ap.Client["sse_events"], constants.APIPaths.Client["sse_events"])
		assert.Equal(t, ap.Client["sse_stream"], constants.APIPaths.Client["sse_stream"])
	})

	t.Run("client", func(t *testing.T) {
		for key, expected := range ap.Client {
			actual, ok := constants.APIPaths.Client[key]
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
		assert.Equal(t, events["OperatorHeartbeatSent"].Value, string(constants.EventOperatorHeartbeatSent))
	})

	t.Run("operator.command", func(t *testing.T) {
		assert.Equal(t, events["OperatorCommandRequested"].Value, string(constants.EventOperatorCommandRequested))
		assert.Equal(t, events["OperatorCommandCompleted"].Value, string(constants.EventOperatorCommandCompleted))
		assert.Equal(t, events["OperatorCommandFailed"].Value, string(constants.EventOperatorCommandFailed))
	})

	t.Run("operator.file.edit", func(t *testing.T) {
		assert.Equal(t, events["OperatorFileEditRequested"].Value, string(constants.EventOperatorFileEditRequested))
		assert.Equal(t, events["OperatorFileEditCompleted"].Value, string(constants.EventOperatorFileEditCompleted))
	})

	t.Run("operator.lifecycle", func(t *testing.T) {
		assert.Equal(t, events["OperatorBound"].Value, string(constants.EventOperatorBound))
		assert.Equal(t, events["OperatorUnbound"].Value, string(constants.EventOperatorUnbound))
	})
}

// =============================================================================
// Status
// =============================================================================

func TestProtocolStatusMatchesGoConstants(t *testing.T) {
	st := loadStatusJSON(t)

	t.Run("operator.status", func(t *testing.T) {
		assert.Equal(t, st.Status["operator_status"]["available"].Value, string(constants.OperatorStatusAvailable))
		assert.Equal(t, st.Status["operator_status"]["offline"].Value, string(constants.OperatorStatusOffline))
	})

	t.Run("execution.status", func(t *testing.T) {
		assert.Equal(t, st.Status["execution_status"]["pending"].Value, string(constants.ExecutionStatusPending))
		assert.Equal(t, st.Status["execution_status"]["executing"].Value, string(constants.ExecutionStatusExecuting))
		assert.Equal(t, st.Status["execution_status"]["completed"].Value, string(constants.ExecutionStatusCompleted))
		assert.Equal(t, st.Status["execution_status"]["failed"].Value, string(constants.ExecutionStatusFailed))
	})
}

// =============================================================================
// Channels
// =============================================================================

func TestProtocolChannelsMatchGoConstants(t *testing.T) {
	t.Run("channel prefixes used by CmdChannel/ResultsChannel/HeartbeatChannel", func(t *testing.T) {
		assert.Equal(t, "cmd:op1:s1", pubsub.CmdChannel("op1", "s1"))
		assert.Equal(t, "results:op1:s1", pubsub.ResultsChannel("op1", "s1"))
		assert.Equal(t, "heartbeat:op1:s1", pubsub.HeartbeatChannel("op1", "s1"))
	})
}

// =============================================================================
// Headers
// =============================================================================

func TestProtocolHeadersMatchGoConstants(t *testing.T) {
	h := loadHeadersJSON(t)

	t.Run("standard http headers", func(t *testing.T) {
		assert.Equal(t, h.Headers["Authorization"].Value, string(constants.HeaderAuthorization))
		assert.Equal(t, h.Headers["UserAgent"].Value, string(constants.HeaderUserAgent))
		assert.Equal(t, h.Headers["ContentType"].Value, string(constants.HeaderContentType))
		assert.Equal(t, h.Headers["ContentDisposition"].Value, string(constants.HeaderContentDisposition))
		assert.Equal(t, h.Headers["ContentLength"].Value, string(constants.HeaderContentLength))
		assert.Equal(t, h.Headers["XForwardedProto"].Value, string(constants.HeaderXForwardedProto))
		assert.Equal(t, h.Headers["XForwardedHost"].Value, string(constants.HeaderXForwardedHost))
		assert.Equal(t, h.Headers["XRequestTimestamp"].Value, string(constants.HeaderXRequestTimestamp))
	})
}

// =============================================================================
// PubSub wire protocol
// =============================================================================

func TestProtocolPubSubWireMatchesGoConstants(t *testing.T) {
	ch := loadChannelsJSON(t)

	t.Run("wire.actions", func(t *testing.T) {
		assert.Equal(t, ch.Channels["Subscribe"].Value, string(constants.PubSubActionSubscribe))
		assert.Equal(t, ch.Channels["PSubscribe"].Value, string(constants.PubSubActionPSubscribe))
		assert.Equal(t, ch.Channels["Unsubscribe"].Value, string(constants.PubSubActionUnsubscribe))
		assert.Equal(t, ch.Channels["Publish"].Value, string(constants.PubSubActionPublish))
	})

	t.Run("wire.event_types", func(t *testing.T) {
		assert.Equal(t, ch.Channels["Message"].Value, string(constants.PubSubEventMessage))
		assert.Equal(t, ch.Channels["PMessage"].Value, string(constants.PubSubEventPMessage))
		assert.Equal(t, ch.Channels["Subscribed"].Value, string(constants.PubSubEventSubscribed))
	})
}

// =============================================================================
// Execution status
// =============================================================================

func TestProtocolExecutionStatusMatchesGoConstants(t *testing.T) {
	st := loadStatusJSON(t)

	assert.Equal(t, st.Status["execution_status"]["pending"].Value, string(constants.ExecutionStatusPending))
	assert.Equal(t, st.Status["execution_status"]["executing"].Value, string(constants.ExecutionStatusExecuting))
	assert.Equal(t, st.Status["execution_status"]["completed"].Value, string(constants.ExecutionStatusCompleted))
	assert.Equal(t, st.Status["execution_status"]["failed"].Value, string(constants.ExecutionStatusFailed))
}

// =============================================================================
// Collections
// =============================================================================

func TestProtocolCollectionsMatchGoConstants(t *testing.T) {
	c := loadCollectionsJSON(t)

	goCollections := map[string]constants.CollectionName{
		"users":                   constants.CollectionUsers,
		"web_sessions":            constants.CollectionWebSessions,
		"operator_sessions":       constants.CollectionOperatorSessions,
		"cli_sessions":            constants.CollectionCLISessions,
		"login_audit":             constants.CollectionLoginAudit,
		"auth_admin_audit":        constants.CollectionAuthAdminAudit,
		"account_locks":           constants.CollectionAccountLocks,
		"organizations":           constants.CollectionOrganizations,
		"operators":               constants.CollectionOperators,
		"operator_usage":          constants.CollectionOperatorUsage,
		"cases":                   constants.CollectionCases,
		"investigations":          constants.CollectionInvestigations,
		"tasks":                   constants.CollectionTasks,
		"memories":                constants.CollectionMemories,
		"settings":                constants.CollectionSettings,
		"console_audit":           constants.CollectionConsoleAudit,
		"bound_sessions":          constants.CollectionBoundSessions,
		"passkey_challenges":      constants.CollectionPasskeyChallenges,
		"personas":                constants.CollectionPersonas,
		"agent_activity_metadata": constants.CollectionAgentActivityMetadata,
		"reputation_state":        constants.CollectionReputationState,
		"reputation_commitments":  constants.CollectionReputationCommitments,
		"stake_resolutions":       constants.CollectionStakeResolutions,
		"revoked_certificates":    constants.CollectionRevokedCertificates,
		"trusted_signers":         constants.CollectionTrustedSigners,
		"app_policies":            constants.CollectionAppPolicies,
		"tribunals":               constants.CollectionTribunals,
	}

	t.Run("collection names", func(t *testing.T) {
		for key, leaf := range c.Collections {
			goConst, ok := goCollections[key]
			assert.True(t, ok, "collection %s must have Go constant", key)
			if ok {
				assert.Equal(t, leaf.Value, string(goConst), "collection %s value mismatch", key)
			}
		}
	})
}

// =============================================================================
// Environment Variables
// =============================================================================

func TestProtocolEnvVarsMatchGoConstants(t *testing.T) {
	e := loadEnvVarsJSON(t)

	goEnvVars := map[string]string{
		"G8E_TRIBUNAL_ID":          string(constants.EnvVar.TribunalID),
		"G8E_TRIBUNAL_URL":         string(constants.EnvVar.TribunalURL),
		"G8E_VAULT_DIR":            string(constants.EnvVar.VaultDir),
		"G8E_VAULT_KEY":            string(constants.EnvVar.VaultKey),
		"G8E_VAULT_REQUIRE_UNLOCK": string(constants.EnvVar.VaultRequireUnlock),
	}

	t.Run("env var keys", func(t *testing.T) {
		for key, leaf := range e.EnvVars {
			goValue, ok := goEnvVars[leaf.Value]
			if ok {
				assert.Equal(t, leaf.Value, goValue, "env var %s value mismatch", key)
			} else {
				assert.NotEmpty(t, leaf.Value, "env var %s must have non-empty value", key)
			}
		}
	})
}
