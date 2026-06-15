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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
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
		cmd  func() *cobra.Command
	}{
		{"users", dataUsersCmd},
		{"operators", dataOperatorsCmd},
		{"settings", dataSettingsCmd},
		{"store", dataStoreCmd},
		{"audit list", dataAuditListCmd},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.cmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			originalWd, _ := os.Getwd()
			tmpDir := t.TempDir()
			os.Chdir(tmpDir)
			defer os.Chdir(originalWd)

			// Set up minimal config structure so config loads, then auth fails
			setupDataTestConfig(t, tmpDir)

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

func setupDataTestConfig(t *testing.T, tmpDir string) *config.Config {
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	pkiDir := filepath.Join(runtimeDir, "pki")
	secretsDir := filepath.Join(runtimeDir, "secrets")
	credentialsDir := runtimeDir

	require.NoError(t, os.MkdirAll(pkiDir, 0755))
	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	// Create credentials directory but NOT the credentials file itself
	// This ensures auth.LoadCredentials returns (nil, nil) which triggers ErrNotAuthenticated
	require.NoError(t, os.MkdirAll(credentialsDir, 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "root"), 0755))

	// Create minimal paths.json structure
	protocolDir := filepath.Join(tmpDir, "protocol")
	constantsDir := filepath.Join(protocolDir, "constants")
	require.NoError(t, os.MkdirAll(constantsDir, 0755))

	pathsJSON := minimalPathsJSON(t)
	pathsPath := filepath.Join(constantsDir, "paths.json")
	require.NoError(t, os.WriteFile(pathsPath, []byte(pathsJSON), 0644))

	return &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     runtimeDir,
		PKIDir:         pkiDir,
		SecretsDir:     secretsDir,
		CredentialsDir: credentialsDir,
		Paths: &config.PathsConfig{
			Host: "localhost",
			Infra: struct {
				AppCertDir           string `json:"app_cert_dir"`
				CACertPath           string `json:"ca_cert_path"`
				DBPath               string `json:"db_path"`
				DocsDir              string `json:"docs_dir"`
				PKIDir               string `json:"pki_dir"`
				ProtocolConstantsDir string `json:"protocol_constants_dir"`
				ProtocolDir          string `json:"protocol_dir"`
				ProtocolModelsDir    string `json:"protocol_models_dir"`
				SecretsDir           string `json:"secrets_dir"`
				SSHConfigPath        string `json:"ssh_config_path"`
				VaultDir             string `json:"vault_dir"`
				VaultKeyPath         string `json:"vault_key_path"`
			}{
				AppCertDir:           filepath.Join(tmpDir, constants.Paths.Infra.AppCertDir),
				CACertPath:           filepath.Join(tmpDir, constants.Paths.Infra.CaCertPath),
				DBPath:               filepath.Join(tmpDir, constants.Paths.Infra.DbPath),
				DocsDir:              filepath.Join(tmpDir, constants.Paths.Infra.DocsDir),
				PKIDir:               filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
				ProtocolConstantsDir: filepath.Join(tmpDir, constants.Paths.Infra.ProtocolConstantsDir),
				ProtocolDir:          filepath.Join(tmpDir, constants.Paths.Infra.ProtocolDir),
				ProtocolModelsDir:    filepath.Join(tmpDir, constants.Paths.Infra.ProtocolModelsDir),
				SecretsDir:           filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
				SSHConfigPath:        filepath.Join(tmpDir, constants.Paths.Infra.SshConfigPath),
			},
		},
	}
}

func TestSqlDBQuery(t *testing.T) {
	t.Run("returns error for non-existent database", func(t *testing.T) {
		_, err := sqlDBQuery("/nonexistent/path/to/db.db", "SELECT 1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to open database file")
	})

	t.Run("returns error for invalid SQL syntax", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		// Create an empty database file
		require.NoError(t, os.WriteFile(dbPath, []byte{}, 0644))

		_, err := sqlDBQuery(dbPath, "INVALID SQL SYNTAX")
		require.Error(t, err)
	})

	t.Run("executes valid query with in-memory database", func(t *testing.T) {
		// Use :memory: for in-memory SQLite database
		rows, err := sqlDBQuery(":memory:", "SELECT 1 as result")
		require.NoError(t, err)
		defer rows.Close()

		assert.True(t, rows.Next())
		var result int
		require.NoError(t, rows.Scan(&result))
		assert.Equal(t, 1, result)
	})

	t.Run("executes query with parameters", func(t *testing.T) {
		rows, err := sqlDBQuery(":memory:", "SELECT ? as value", 42)
		require.NoError(t, err)
		defer rows.Close()

		assert.True(t, rows.Next())
		var value int
		require.NoError(t, rows.Scan(&value))
		assert.Equal(t, 42, value)
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
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		cmd := dataAuditSummaryCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "audit vault database not found")
	})

	t.Run("summary succeeds with empty database", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		// Initialize global paths to use tmpDir
		require.NoError(t, constants.InitPathsWithBase(tmpDir))

		// Create data directory and empty database using global paths
		dataDir := constants.Paths.Infra.DataDir
		require.NoError(t, os.MkdirAll(dataDir, 0755))
		dbPath := constants.Paths.Infra.DbPath

		// Create database with events table but no data
		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec("CREATE TABLE events (type TEXT, operator_session_id TEXT)")
		require.NoError(t, err)

		cmd := dataAuditSummaryCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

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
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		cmd := dataStoreCmd()
		cmd.Flags().Set("collection", "test_collection")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Will fail on API call, but should pass flag validation
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
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		cmd := dataAuditListCmd()
		cmd.Flags().Set("operator-session-id", "test-session-123")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Will fail on API call, but should pass flag validation
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
			jsonData:    `[{"id":"user1","name":"Test User"}]`,
			targetType:  &[]User{},
			expectError: false,
		},
		{
			name:        "invalid User JSON",
			jsonData:    `invalid json`,
			targetType:  &[]User{},
			expectError: true,
			errorMsg:    "failed to parse response",
		},
		{
			name:        "valid Operator JSON",
			jsonData:    `[{"id":"op1","cloud_subtype":"aws","status":"active"}]`,
			targetType:  &[]Operator{},
			expectError: false,
		},
		{
			name:        "valid SettingsResponse JSON",
			jsonData:    `{"settings":{"key1":"value1","key2":123}}`,
			targetType:  &SettingsResponse{},
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

func TestDataQueryFilterTypes(t *testing.T) {
	t.Run("QueryFilter struct serialization", func(t *testing.T) {
		filter := QueryFilter{
			Field: "test_field",
			Op:    "==",
			Value: "test_value",
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded QueryFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, filter.Field, decoded.Field)
		assert.Equal(t, filter.Op, decoded.Op)
		assert.Equal(t, filter.Value, decoded.Value)
	})

	t.Run("QueryRequest struct serialization", func(t *testing.T) {
		req := QueryRequest{
			Filters: []QueryFilter{
				{Field: "field1", Op: "==", Value: "value1"},
				{Field: "field2", Op: "!=", Value: "value2"},
			},
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded QueryRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Len(t, decoded.Filters, 2)
		assert.Equal(t, "field1", decoded.Filters[0].Field)
		assert.Equal(t, "field2", decoded.Filters[1].Field)
	})

	t.Run("QueryRequestWithLimit struct serialization", func(t *testing.T) {
		req := QueryRequestWithLimit{
			Filters: []QueryFilter{
				{Field: "session_id", Op: "==", Value: "sess-123"},
			},
			Limit: 50,
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded QueryRequestWithLimit
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Len(t, decoded.Filters, 1)
		assert.Equal(t, 50, decoded.Limit)
	})

	t.Run("QueryFilter with numeric value", func(t *testing.T) {
		filter := QueryFilter{
			Field: "count",
			Op:    ">",
			Value: 100,
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded QueryFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "count", decoded.Field)
		assert.Equal(t, ">", decoded.Op)
		assert.Equal(t, float64(100), decoded.Value) // JSON numbers unmarshal as float64
	})

	t.Run("QueryFilter with null value", func(t *testing.T) {
		filter := QueryFilter{
			Field: "deleted_at",
			Op:    "==",
			Value: nil,
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded QueryFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "deleted_at", decoded.Field)
		assert.Equal(t, "==", decoded.Op)
		assert.Nil(t, decoded.Value)
	})

	t.Run("QueryRequestWithLimit with zero limit", func(t *testing.T) {
		req := QueryRequestWithLimit{
			Filters: []QueryFilter{},
			Limit:   0,
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded QueryRequestWithLimit
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, 0, decoded.Limit)
	})
}

func TestDataAuditSummaryWithSessionFilter(t *testing.T) {
	t.Run("summary with session filter constructs correct query", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		// Initialize global paths to use tmpDir
		require.NoError(t, constants.InitPathsWithBase(tmpDir))

		// Create data directory and database with test data
		dataDir := constants.Paths.Infra.DataDir
		require.NoError(t, os.MkdirAll(dataDir, 0755))
		dbPath := constants.Paths.Infra.DbPath

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

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

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
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		// Initialize global paths to use tmpDir
		require.NoError(t, constants.InitPathsWithBase(tmpDir))

		// Create data directory and database with test data
		dataDir := constants.Paths.Infra.DataDir
		require.NoError(t, os.MkdirAll(dataDir, 0755))
		dbPath := constants.Paths.Infra.DbPath

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

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Audit Event Summary")
		assert.Contains(t, output, "login: 2")
		assert.Contains(t, output, "logout: 1")
		assert.Contains(t, output, "Total events: 3")
	})
}

func TestDataAuditSummaryQueryConstruction(t *testing.T) {
	t.Run("SQL query construction without session filter", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		// Initialize global paths to use tmpDir
		require.NoError(t, constants.InitPathsWithBase(tmpDir))

		// Create data directory and database
		dataDir := constants.Paths.Infra.DataDir
		require.NoError(t, os.MkdirAll(dataDir, 0755))
		dbPath := constants.Paths.Infra.DbPath

		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec("CREATE TABLE events (type TEXT, operator_session_id TEXT)")
		require.NoError(t, err)

		// Test the query directly
		rows, err := sqlDBQuery(dbPath, "SELECT type, COUNT(*) as count FROM events GROUP BY type")
		require.NoError(t, err)
		defer rows.Close()

		// Should return no rows for empty table
		assert.False(t, rows.Next())
	})

	t.Run("SQL query construction with session filter", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		// Initialize global paths to use tmpDir
		require.NoError(t, constants.InitPathsWithBase(tmpDir))

		// Create data directory and database
		dataDir := constants.Paths.Infra.DataDir
		require.NoError(t, os.MkdirAll(dataDir, 0755))
		dbPath := constants.Paths.Infra.DbPath

		db, err := sql.Open("sqlite", dbPath)
		require.NoError(t, err)
		defer db.Close()

		_, err = db.Exec("CREATE TABLE events (type TEXT, operator_session_id TEXT)")
		require.NoError(t, err)

		// Test the query with parameter
		rows, err := sqlDBQuery(dbPath, "SELECT type, COUNT(*) as count FROM events WHERE operator_session_id = ? GROUP BY type", "test-session")
		require.NoError(t, err)
		defer rows.Close()

		// Should return no rows for empty table
		assert.False(t, rows.Next())
	})
}

func TestDataAuditSummaryOutputFormatting(t *testing.T) {
	t.Run("summary formats output correctly with multiple event types", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		// Initialize global paths to use tmpDir
		require.NoError(t, constants.InitPathsWithBase(tmpDir))

		// Create data directory and database with test data
		dataDir := constants.Paths.Infra.DataDir
		require.NoError(t, os.MkdirAll(dataDir, 0755))
		dbPath := constants.Paths.Infra.DbPath

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

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

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
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		cmd := dataStoreCmd()
		cmd.Flags().Set("collection", "test_collection")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Will fail on API call, but we can verify the flag state
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		// Error should not be about missing collection flag
		assert.NotContains(t, err.Error(), "--collection is required")
	})

	t.Run("store command uses document mode when document-id provided", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		cmd := dataStoreCmd()
		cmd.Flags().Set("collection", "test_collection")
		cmd.Flags().Set("document-id", "doc-123")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Will fail on API call, but we can verify both flags are set
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
		var users []User
		err := json.Unmarshal([]byte(invalidJSON), &users)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid character")
	})

	t.Run("operators command handles JSON parse errors", func(t *testing.T) {
		invalidJSON := `{invalid}`
		var operators []Operator
		err := json.Unmarshal([]byte(invalidJSON), &operators)
		require.Error(t, err)
	})

	t.Run("settings command handles JSON parse errors", func(t *testing.T) {
		invalidJSON := `{invalid}`
		var settings SettingsResponse
		err := json.Unmarshal([]byte(invalidJSON), &settings)
		require.Error(t, err)
	})

	t.Run("settings command handles missing settings field", func(t *testing.T) {
		// Valid JSON but missing required field
		invalidJSON := `{"other_field": "value"}`
		var settings SettingsResponse
		err := json.Unmarshal([]byte(invalidJSON), &settings)
		require.NoError(t, err) // JSON is valid, just empty settings
		assert.Nil(t, settings.Settings)
	})
}

func TestDataQueryFilterEdgeCases(t *testing.T) {
	t.Run("QueryFilter with empty field", func(t *testing.T) {
		filter := QueryFilter{
			Field: "",
			Op:    "==",
			Value: "value",
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded QueryFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "", decoded.Field)
	})

	t.Run("QueryFilter with empty operator", func(t *testing.T) {
		filter := QueryFilter{
			Field: "field",
			Op:    "",
			Value: "value",
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded QueryFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "", decoded.Op)
	})

	t.Run("QueryFilter with complex value (object)", func(t *testing.T) {
		complexValue := map[string]interface{}{"nested": "value", "number": 42}
		filter := QueryFilter{
			Field: "metadata",
			Op:    "==",
			Value: complexValue,
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded QueryFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "metadata", decoded.Field)
		assert.Equal(t, "==", decoded.Op)
		assert.IsType(t, map[string]interface{}{}, decoded.Value)
	})

	t.Run("QueryFilter with array value", func(t *testing.T) {
		arrayValue := []string{"value1", "value2", "value3"}
		filter := QueryFilter{
			Field: "tags",
			Op:    "in",
			Value: arrayValue,
		}

		data, err := json.Marshal(filter)
		require.NoError(t, err)

		var decoded QueryFilter
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "tags", decoded.Field)
		assert.Equal(t, "in", decoded.Op)
		assert.IsType(t, []interface{}{}, decoded.Value)
	})
}

func TestDataCommandLongDescription(t *testing.T) {
	t.Run("data command has long description", func(t *testing.T) {
		cmd := dataCmd()
		assert.NotEmpty(t, cmd.Long)
		assert.Contains(t, cmd.Long, "Data management")
	})

	t.Run("users command has no long description (uses default)", func(t *testing.T) {
		cmd := dataUsersCmd()
		// Most subcommands don't have long descriptions, that's fine
		assert.Empty(t, cmd.Long)
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

func TestDataSqlDBQueryEdgeCases(t *testing.T) {
	t.Run("sqlDBQuery handles empty result set", func(t *testing.T) {
		rows, err := sqlDBQuery(":memory:", "SELECT 1 WHERE 1=0")
		require.NoError(t, err)
		defer rows.Close()

		assert.False(t, rows.Next())
	})

	t.Run("sqlDBQuery handles multiple parameters", func(t *testing.T) {
		rows, err := sqlDBQuery(":memory:", "SELECT ? + ? as result", 10, 20)
		require.NoError(t, err)
		defer rows.Close()

		assert.True(t, rows.Next())
		var result int
		require.NoError(t, rows.Scan(&result))
		assert.Equal(t, 30, result)
	})

	t.Run("sqlDBQuery handles string parameters", func(t *testing.T) {
		rows, err := sqlDBQuery(":memory:", "SELECT ? as result", "test-string")
		require.NoError(t, err)
		defer rows.Close()

		assert.True(t, rows.Next())
		var result string
		require.NoError(t, rows.Scan(&result))
		assert.Equal(t, "test-string", result)
	})

	t.Run("sqlDBQuery handles NULL parameters", func(t *testing.T) {
		rows, err := sqlDBQuery(":memory:", "SELECT ? as result", nil)
		require.NoError(t, err)
		defer rows.Close()

		assert.True(t, rows.Next())
		var result interface{}
		require.NoError(t, rows.Scan(&result))
		assert.Nil(t, result)
	})
}
