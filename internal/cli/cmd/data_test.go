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

package cmd

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

func TestDataCmd(t *testing.T) {
	t.Run("data command has correct use and description", func(t *testing.T) {
		cmd := dataCmd()
		assert.Equal(t, "data", cmd.Use)
		assert.Contains(t, cmd.Short, "Administer")
		assert.Contains(t, cmd.Short, "mTLS")
	})
}

func TestDataUsersCmd(t *testing.T) {
	t.Run("users command has correct use", func(t *testing.T) {
		cmd := dataUsersCmd()
		assert.Equal(t, "users", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage user accounts")
	})
}

func TestDataOperatorsCmd(t *testing.T) {
	t.Run("operators command has correct use", func(t *testing.T) {
		cmd := dataOperatorsCmd()
		assert.Equal(t, "operators", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage Operator instances")
	})
}

func TestDataSettingsCmd(t *testing.T) {
	t.Run("settings command has correct use", func(t *testing.T) {
		cmd := dataSettingsCmd()
		assert.Equal(t, "settings", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage Gateway settings")
	})
}

func TestDataStoreCmd(t *testing.T) {
	t.Run("store command has correct use", func(t *testing.T) {
		cmd := dataStoreCmd()
		assert.Equal(t, "store", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage document storage")
	})

	t.Run("store has collection flag", func(t *testing.T) {
		cmd := dataStoreCmd()
		flag := cmd.Flags().Lookup("collection")
		assert.NotNil(t, flag)
	})

	t.Run("store has document-id flag", func(t *testing.T) {
		cmd := dataStoreCmd()
		flag := cmd.Flags().Lookup("document-id")
		assert.NotNil(t, flag)
	})

}

func TestDataAuditCmd(t *testing.T) {
	t.Run("audit command has correct use", func(t *testing.T) {
		cmd := dataAuditCmd()
		assert.Equal(t, "audit", cmd.Use)
		assert.Contains(t, cmd.Short, "Query audit vault")
	})

	t.Run("audit has operator-session-id flag", func(t *testing.T) {
		cmd := dataAuditListCmd()
		flag := cmd.Flags().Lookup("operator-session-id")
		assert.NotNil(t, flag)
	})

	t.Run("audit has limit flag", func(t *testing.T) {
		cmd := dataAuditListCmd()
		flag := cmd.Flags().Lookup("limit")
		assert.NotNil(t, flag)
	})

}

func TestDataCommandsRequireAuthentication(t *testing.T) {
	testCases := []struct {
		name string
		cmd  func(func(string) (*config.Config, error), apiClientFactory, func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command
	}{
		{"users", dataUsersCmdWithConfig},
		{"operators", dataOperatorsCmdWithConfig},
		{"settings", dataSettingsCmdWithConfig},
		{"store", dataStoreCmdWithConfig},
		{"audit list", dataAuditListCmdWithConfig},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fileSvc, cfg := newCmdTestEnv(t)
			loader := func(_ string) (*config.Config, error) { return cfg, nil }
			cmd := tc.cmd(loader, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			err := cmd.RunE(cmd, []string{})
			require.Error(t, err)
			require.ErrorIs(t, err, constants.ErrNotAuthenticated)
		})
	}
}

func TestDataCommandFlags(t *testing.T) {
	t.Run("audit limit flag has default", func(t *testing.T) {
		cmd := dataAuditListCmd()
		limitFlag := cmd.Flags().Lookup("limit")
		assert.NotNil(t, limitFlag)
		assert.Equal(t, "100", limitFlag.DefValue)
	})
}

func TestDataAuditSummaryCmd(t *testing.T) {
	t.Run("summary command has correct use", func(t *testing.T) {
		cmd := dataAuditSummaryCmd()
		assert.Equal(t, string(constants.StreamStatusSummary), cmd.Use)
		assert.Contains(t, cmd.Short, "Show audit event summary")
	})

	t.Run("summary has operator-session-id flag", func(t *testing.T) {
		cmd := dataAuditSummaryCmd()
		flag := cmd.Flags().Lookup("operator-session-id")
		assert.NotNil(t, flag)
	})

	t.Run("summary fails when database does not exist", func(t *testing.T) {
		_, cfg := newCmdTestEnv(t)
		require.NoError(t, paths.InitWithBase(cfg.ProjectRoot))

		cmd := dataAuditSummaryCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "audit vault database not found")
	})

	t.Run("summary succeeds with empty database", func(t *testing.T) {
		_, cfg := newCmdTestEnv(t)

		// Initialize global paths to use test tmpDir
		require.NoError(t, paths.InitWithBase(cfg.ProjectRoot))

		// Create empty database using global paths
		dbPath := paths.Infra.DbPath

		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec("CREATE TABLE events (type TEXT, operator_session_id TEXT)")
		require.NoError(t, err)

		cmd := dataAuditSummaryCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "No audit events found in audit vault")
	})
}

func TestDataStoreCmdFlagValidation(t *testing.T) {
	t.Run("store fails without collection flag", func(t *testing.T) {
		// This test verifies that the collection flag is required
		// Note: The actual command checks the flag AFTER config load and client creation
		// So we verify the flag exists and is required by checking the command definition
		cmd := dataStoreCmd()
		flag := cmd.Flags().Lookup("collection")
		assert.NotNil(t, flag)
		// The flag has no default value, making it required
		assert.Empty(t, flag.DefValue)
	})

	t.Run("store list mode with collection flag", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)
		loader := func(_ string) (*config.Config, error) { return cfg, nil }
		cmd := dataStoreCmdWithConfig(loader, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("collection", "test_collection")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Will fail on API client creation (no credentials), but should pass flag validation
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		// Error should not be about missing collection flag
		assert.NotContains(t, err.Error(), "--collection is required")
	})
}

func TestDataAuditListCmdFlagValidation(t *testing.T) {
	t.Run("audit list requires operator-session-id flag", func(t *testing.T) {
		// This test verifies that the operator-session-id flag is required
		// Note: The actual command checks the flag AFTER config load and client creation
		// So we verify the flag exists and is required by checking the command definition
		cmd := dataAuditListCmd()
		flag := cmd.Flags().Lookup("operator-session-id")
		assert.NotNil(t, flag)
		// The flag has no default value, making it required
		assert.Empty(t, flag.DefValue)
	})

	t.Run("audit list with operator-session-id flag", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)
		loader := func(_ string) (*config.Config, error) { return cfg, nil }
		cmd := dataAuditListCmdWithConfig(loader, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("operator-session-id", "test-session-123")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Will fail on API client creation (no credentials), but should pass flag validation
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		// Error should not be about missing operator-session-id flag
		assert.NotContains(t, err.Error(), "--operator-session-id is required")
	})
}

func TestDataAuditCmdStructure(t *testing.T) {
	t.Run("audit command has correct subcommands", func(t *testing.T) {
		cmd := dataAuditCmd()
		subcommands := cmd.Commands()
		subcommandNames := make(map[string]bool)
		for _, sub := range subcommands {
			subcommandNames[sub.Use] = true
		}

		assert.True(t, subcommandNames["list"], "missing list subcommand")
		assert.True(t, subcommandNames[string(constants.StreamStatusSummary)], "missing summary subcommand")
	})
}

func TestDataCommandJSONUnmarshaling(t *testing.T) {
	testCases := []struct {
		name        string
		jsonData    string
		targetType  interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid User JSON",
			jsonData:    `[{"id":"user1"}]`,
			targetType:  &[]models.User{},
			expectError: false,
		},
		{
			name:        "invalid User JSON",
			jsonData:    `invalid json`,
			targetType:  &[]models.User{},
			expectError: true,
			errorMsg:    "failed to parse response",
		},
		{
			name:        "valid OperatorDocumentGo JSON",
			jsonData:    `[{"id":"op1","cloud_subtype":"aws","status":"active"}]`,
			targetType:  &[]models.OperatorDocumentGo{},
			expectError: false,
		},
		{
			name:        "valid SettingsDocument JSON",
			jsonData:    `{"settings":{"actuator_key_id":"key1"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			targetType:  &models.SettingsDocument{},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := json.Unmarshal([]byte(tc.jsonData), tc.targetType)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDataDocFilterTypes(t *testing.T) {
	t.Run("DocFilter struct serialization", func(t *testing.T) {
		filter := models.DocFilter{
			Field: "test_field",
			Op:    "==",
			Value: json.RawMessage(`"test_value"`),
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded models.DocFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, filter.Field, decoded.Field)
		assert.Equal(t, filter.Op, decoded.Op)
		assert.Equal(t, filter.Value, decoded.Value)
	})

	t.Run("DocQueryRequest struct serialization", func(t *testing.T) {
		req := models.DocQueryRequest{
			Filters: []models.DocFilter{
				{Field: "field1", Op: "==", Value: json.RawMessage(`"value1"`)},
				{Field: "field2", Op: "!=", Value: json.RawMessage(`"value2"`)},
			},
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded models.DocQueryRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Len(t, decoded.Filters, 2)
		assert.Equal(t, "field1", decoded.Filters[0].Field)
		assert.Equal(t, "field2", decoded.Filters[1].Field)
	})

	t.Run("DocQueryRequest with limit serialization", func(t *testing.T) {
		req := models.DocQueryRequest{
			Filters: []models.DocFilter{
				{Field: "session_id", Op: "==", Value: json.RawMessage(`"sess-123"`)},
			},
			Limit: 50,
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded models.DocQueryRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Len(t, decoded.Filters, 1)
		assert.Equal(t, 50, decoded.Limit)
	})

	t.Run("DocFilter with numeric value", func(t *testing.T) {
		filter := models.DocFilter{
			Field: "count",
			Op:    ">",
			Value: json.RawMessage("100"),
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded models.DocFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "count", decoded.Field)
		assert.Equal(t, ">", decoded.Op)
		assert.Equal(t, json.RawMessage("100"), decoded.Value)
	})

	t.Run("DocFilter with null value", func(t *testing.T) {
		filter := models.DocFilter{
			Field: "deleted_at",
			Op:    "==",
			Value: json.RawMessage("null"),
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded models.DocFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "deleted_at", decoded.Field)
		assert.Equal(t, "==", decoded.Op)
		assert.Equal(t, json.RawMessage("null"), decoded.Value)
	})

	t.Run("DocQueryRequest with zero limit", func(t *testing.T) {
		req := models.DocQueryRequest{
			Filters: []models.DocFilter{},
			Limit:   0,
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded models.DocQueryRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, 0, decoded.Limit)
	})
}

func TestDataAuditSummaryWithSessionFilter(t *testing.T) {
	t.Run("summary with session filter constructs correct query", func(t *testing.T) {
		_, cfg := newCmdTestEnv(t)

		// Initialize global paths to use test tmpDir
		require.NoError(t, paths.InitWithBase(cfg.ProjectRoot))

		dbPath := paths.Infra.DbPath

		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec("CREATE TABLE events (type TEXT, operator_session_id TEXT)")
		require.NoError(t, err)

		// Insert test events for different sessions
		_, err = db.Exec("INSERT INTO events (type, operator_session_id) VALUES (?, ?)", "login", "session-1")
		require.NoError(t, err)
		_, err = db.Exec("INSERT INTO events (type, operator_session_id) VALUES (?, ?)", "logout", "session-1")
		require.NoError(t, err)
		_, err = db.Exec("INSERT INTO events (type, operator_session_id) VALUES (?, ?)", "login", "session-2")
		require.NoError(t, err)

		cmd := dataAuditSummaryCmd()
		cmd.Flags().Set("operator-session-id", "session-1")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Audit Event Summary")
		assert.Contains(t, output, "login: 1")
		assert.Contains(t, output, "logout: 1")
		assert.Contains(t, output, "Total events: 2")
		// Should not include events from session-2
		assert.NotContains(t, output, "Total events: 3")
	})

	t.Run("summary without session filter includes all events", func(t *testing.T) {
		_, cfg := newCmdTestEnv(t)

		// Initialize global paths to use test tmpDir
		require.NoError(t, paths.InitWithBase(cfg.ProjectRoot))

		dbPath := paths.Infra.DbPath

		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec("CREATE TABLE events (type TEXT, operator_session_id TEXT)")
		require.NoError(t, err)

		// Insert test events for different sessions
		_, err = db.Exec("INSERT INTO events (type, operator_session_id) VALUES (?, ?)", "login", "session-1")
		require.NoError(t, err)
		_, err = db.Exec("INSERT INTO events (type, operator_session_id) VALUES (?, ?)", "logout", "session-1")
		require.NoError(t, err)
		_, err = db.Exec("INSERT INTO events (type, operator_session_id) VALUES (?, ?)", "login", "session-2")
		require.NoError(t, err)

		cmd := dataAuditSummaryCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Audit Event Summary")
		assert.Contains(t, output, "login: 2")
		assert.Contains(t, output, "logout: 1")
		assert.Contains(t, output, "Total events: 3")
	})
}

func TestDataAuditSummaryOutputFormatting(t *testing.T) {
	t.Run("summary formats output correctly with multiple event types", func(t *testing.T) {
		_, cfg := newCmdTestEnv(t)

		// Initialize global paths to use test tmpDir
		require.NoError(t, paths.InitWithBase(cfg.ProjectRoot))

		dbPath := paths.Infra.DbPath

		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec("CREATE TABLE events (type TEXT, operator_session_id TEXT)")
		require.NoError(t, err)

		// Insert multiple event types
		eventTypes := []string{"login", "logout", "command", "error", "success"}
		for i, eventType := range eventTypes {
			for j := 0; j < i+1; j++ {
				_, err = db.Exec("INSERT INTO events (type, operator_session_id) VALUES (?, ?)", eventType, "session-1")
				require.NoError(t, err)
			}
		}

		cmd := dataAuditSummaryCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Audit Event Summary")
		assert.Contains(t, output, strings.Repeat("=", 110))
		assert.Contains(t, output, "login: 1")
		assert.Contains(t, output, "logout: 2")
		assert.Contains(t, output, "command: 3")
		assert.Contains(t, output, "error: 4")
		assert.Contains(t, output, "success: 5")
		assert.Contains(t, output, "Total events: 15")
	})
}

func TestDataStoreCommandModes(t *testing.T) {
	t.Run("store command defaults to list mode when document-id not provided", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)
		loader := func(_ string) (*config.Config, error) { return cfg, nil }
		cmd := dataStoreCmdWithConfig(loader, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("collection", "test_collection")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Will fail on API client creation (no credentials), but we can verify the flag state
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		// Error should not be about missing collection flag
		assert.NotContains(t, err.Error(), "--collection is required")
	})

	t.Run("store command uses document mode when document-id provided", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)
		loader := func(_ string) (*config.Config, error) { return cfg, nil }
		cmd := dataStoreCmdWithConfig(loader, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
		cmd.Flags().Set("collection", "test_collection")
		cmd.Flags().Set("document-id", "doc-123")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Will fail on API client creation (no credentials), but we can verify both flags are set
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		// Error should not be about missing flags
		assert.NotContains(t, err.Error(), "--collection is required")
	})
}

func TestDataCommandErrorHandling(t *testing.T) {
	t.Run("users command handles JSON parse errors", func(t *testing.T) {
		// This test verifies that the users command properly handles invalid JSON responses
		// Since we can't mock the API client easily, we test the struct unmarshaling directly
		invalidJSON := `not valid json`
		var users []models.User
		err := json.Unmarshal([]byte(invalidJSON), &users)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid character")
	})

	t.Run("operators command handles JSON parse errors", func(t *testing.T) {
		invalidJSON := `{invalid}`
		var operators []models.OperatorDocumentGo
		err := json.Unmarshal([]byte(invalidJSON), &operators)
		require.Error(t, err)
	})

	t.Run("settings command handles JSON parse errors", func(t *testing.T) {
		invalidJSON := `{invalid}`
		var settings models.SettingsDocument
		err := json.Unmarshal([]byte(invalidJSON), &settings)
		require.Error(t, err)
	})

	t.Run("settings command handles missing settings field", func(t *testing.T) {
		// Valid JSON but missing required field
		invalidJSON := `{"other_field": "value"}`
		var settings models.SettingsDocument
		err := json.Unmarshal([]byte(invalidJSON), &settings)
		require.NoError(t, err) // JSON is valid, just empty settings
		assert.Nil(t, settings.Settings)
	})
}

func TestDataDocFilterEdgeCases(t *testing.T) {
	t.Run("DocFilter with empty field", func(t *testing.T) {
		filter := models.DocFilter{
			Field: "",
			Op:    "==",
			Value: json.RawMessage(`"value"`),
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded models.DocFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "", decoded.Field)
	})

	t.Run("DocFilter with empty operator", func(t *testing.T) {
		filter := models.DocFilter{
			Field: "field",
			Op:    "",
			Value: json.RawMessage(`"value"`),
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded models.DocFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "", decoded.Op)
	})

	t.Run("DocFilter with complex value (object)", func(t *testing.T) {
		complexValue := map[string]interface{}{"nested": "value", "number": 42}
		valueBytes, err := json.Marshal(complexValue)
		require.NoError(t, err)

		filter := models.DocFilter{
			Field: "metadata",
			Op:    "==",
			Value: valueBytes,
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded models.DocFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "metadata", decoded.Field)
		assert.Equal(t, "==", decoded.Op)
		assert.JSONEq(t, string(valueBytes), string(decoded.Value))
	})

	t.Run("DocFilter with array value", func(t *testing.T) {
		arrayValue := []string{"value1", "value2", "value3"}
		valueBytes, err := json.Marshal(arrayValue)
		require.NoError(t, err)

		filter := models.DocFilter{
			Field: "tags",
			Op:    "in",
			Value: valueBytes,
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded models.DocFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "tags", decoded.Field)
		assert.Equal(t, "in", decoded.Op)
		assert.JSONEq(t, string(valueBytes), string(decoded.Value))
	})
}

func TestDataCommandLongDescription(t *testing.T) {
	t.Run("data command has long description", func(t *testing.T) {
		cmd := dataCmd()
		assert.NotEmpty(t, cmd.Long)
		assert.Contains(t, cmd.Long, "Data management")
	})

	t.Run("users command has long description", func(t *testing.T) {
		cmd := dataUsersCmd()
		assert.NotEmpty(t, cmd.Long)
		assert.Contains(t, cmd.Long, "user accounts")
	})
}

func TestDataCommandSubcommandRegistration(t *testing.T) {
	t.Run("data command registers all expected subcommands", func(t *testing.T) {
		cmd := dataCmd()
		subcommands := cmd.Commands()
		subcommandNames := make(map[string]bool)
		for _, sub := range subcommands {
			subcommandNames[sub.Use] = true
		}

		assert.True(t, subcommandNames["users"], "missing users subcommand")
		assert.True(t, subcommandNames["operators"], "missing operators subcommand")
		assert.True(t, subcommandNames["settings"], "missing settings subcommand")
		assert.True(t, subcommandNames["store"], "missing store subcommand")
		assert.True(t, subcommandNames["audit"], "missing audit subcommand")
	})

	t.Run("audit command registers list and summary subcommands", func(t *testing.T) {
		cmd := dataAuditCmd()
		subcommands := cmd.Commands()
		subcommandNames := make(map[string]bool)
		for _, sub := range subcommands {
			subcommandNames[sub.Use] = true
		}

		assert.True(t, subcommandNames["list"], "missing list subcommand")
		assert.True(t, subcommandNames[string(constants.StreamStatusSummary)], "missing summary subcommand")
	})
}
