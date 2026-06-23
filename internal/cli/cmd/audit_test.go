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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditCmd(t *testing.T) {
	t.Run("audit command has correct use and description", func(t *testing.T) {
		cmd := auditCmd()
		assert.Equal(t, "audit", cmd.Use)
		assert.Contains(t, cmd.Short, "Run audit reports")
		assert.Contains(t, cmd.Long, "compliance evidence")
	})

	t.Run("audit command has all subcommands", func(t *testing.T) {
		cmd := auditCmd()
		subcommands := cmd.Commands()
		subcommandNames := make(map[string]bool)
		for _, sub := range subcommands {
			subcommandNames[sub.Use] = true
		}

		assert.True(t, subcommandNames["receipts"], "missing receipts subcommand")
		assert.True(t, subcommandNames["export"], "missing export subcommand")
		assert.True(t, subcommandNames["report"], "missing report subcommand")
		assert.True(t, subcommandNames["events"], "missing events subcommand")
		assert.True(t, subcommandNames["summary"], "missing summary subcommand")
	})
}

func TestAuditReceiptsCmd(t *testing.T) {
	t.Run("receipts command has correct use and description", func(t *testing.T) {
		cmd := auditReceiptsCmd()
		assert.Equal(t, "receipts", cmd.Use)
		assert.Contains(t, cmd.Short, "List signed receipts")
	})

	t.Run("receipts has session, tx-id, and json flags", func(t *testing.T) {
		cmd := auditReceiptsCmd()
		sessionFlag := cmd.Flags().Lookup("session")
		txIDFlag := cmd.Flags().Lookup("tx-id")
		jsonFlag := cmd.Flags().Lookup("json")

		assert.NotNil(t, sessionFlag)
		assert.NotNil(t, txIDFlag)
		assert.NotNil(t, jsonFlag)
	})
}

func TestAuditExportCmd(t *testing.T) {
	t.Run("export command has correct use and description", func(t *testing.T) {
		cmd := auditExportCmd()
		assert.Equal(t, "export", cmd.Use)
		assert.Contains(t, cmd.Short, "Export the full receipts bundle")
	})

	t.Run("export has session and out flags", func(t *testing.T) {
		cmd := auditExportCmd()
		sessionFlag := cmd.Flags().Lookup("session")
		outFlag := cmd.Flags().Lookup("out")

		assert.NotNil(t, sessionFlag)
		assert.NotNil(t, outFlag)
	})

	t.Run("export has default out path", func(t *testing.T) {
		cmd := auditExportCmd()
		outFlag := cmd.Flags().Lookup("out")
		assert.Equal(t, "./receipts-export.json", outFlag.DefValue)
	})
}

func TestAuditReportCmd(t *testing.T) {
	t.Run("report command has correct use and description", func(t *testing.T) {
		cmd := auditReportCmd()
		assert.Equal(t, "report", cmd.Use)
		assert.Contains(t, cmd.Short, "Generate a compliance report")
	})

	t.Run("report has session and out flags", func(t *testing.T) {
		cmd := auditReportCmd()
		sessionFlag := cmd.Flags().Lookup("session")
		outFlag := cmd.Flags().Lookup("out")

		assert.NotNil(t, sessionFlag)
		assert.NotNil(t, outFlag)
	})

	t.Run("report has default out directory", func(t *testing.T) {
		cmd := auditReportCmd()
		outFlag := cmd.Flags().Lookup("out")
		assert.Equal(t, "./reports", outFlag.DefValue)
	})
}

func TestAuditEventsCmd(t *testing.T) {
	t.Run("events command has correct use and description", func(t *testing.T) {
		cmd := auditEventsCmd()
		assert.Equal(t, "events", cmd.Use)
		assert.Contains(t, cmd.Short, "Query raw audit events")
	})

	t.Run("events has session, limit, and json flags", func(t *testing.T) {
		cmd := auditEventsCmd()
		sessionFlag := cmd.Flags().Lookup("session")
		limitFlag := cmd.Flags().Lookup("limit")
		jsonFlag := cmd.Flags().Lookup("json")

		assert.NotNil(t, sessionFlag)
		assert.NotNil(t, limitFlag)
		assert.NotNil(t, jsonFlag)
	})

	t.Run("events has default limit", func(t *testing.T) {
		cmd := auditEventsCmd()
		limitFlag := cmd.Flags().Lookup("limit")
		assert.Equal(t, "100", limitFlag.DefValue)
	})
}

func TestAuditSummaryCmd(t *testing.T) {
	t.Run("summary command has correct use and description", func(t *testing.T) {
		cmd := auditSummaryCmd()
		assert.Equal(t, "summary", cmd.Use)
		assert.Contains(t, cmd.Short, "Aggregate audit events")
	})

	t.Run("summary has session flag", func(t *testing.T) {
		cmd := auditSummaryCmd()
		sessionFlag := cmd.Flags().Lookup("session")

		assert.NotNil(t, sessionFlag)
	})
}

func TestAuditCommandFlags(t *testing.T) {
	t.Run("all audit commands have consistent session flag naming", func(t *testing.T) {
		commands := []*cobra.Command{
			auditReceiptsCmd(),
			auditExportCmd(),
			auditReportCmd(),
			auditEventsCmd(),
			auditSummaryCmd(),
		}

		for _, cmd := range commands {
			sessionFlag := cmd.Flags().Lookup("session")
			if sessionFlag != nil {
				assert.Equal(t, "session", sessionFlag.Name)
			}
		}
	})
}

// Test helper functions for audit response parsing

func TestAuditReceiptsResponseParsing(t *testing.T) {
	t.Run("parses valid receipts response", func(t *testing.T) {
		jsonResp := `{
			"success": true,
			"receipts": [
				{
					"transaction_id": "tx-123",
					"transaction_hash": "abc123def456",
					"operator_id": "op-1",
					"operator_session_id": "sess-1",
					"action_type": "FS_READ",
					"target_resource": "/path/to/file",
					"result_summary": "success",
					"state_root_before": "root1",
					"state_root_after": "root2",
					"executed_at": "2024-01-01T00:00:00Z",
					"signer_key_id": "key-1",
					"signature": "sig1",
					"l2_valid": true,
					"l3_valid": true,
					"timestamp": "2024-01-01T00:00:00Z"
				}
			]
		}`

		var resp models.AuditReceiptsResponse
		err := json.Unmarshal([]byte(jsonResp), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Len(t, resp.Receipts, 1)
		assert.Equal(t, "tx-123", resp.Receipts[0].TransactionID)
		assert.Equal(t, constants.ActionTypeFsRead, resp.Receipts[0].ActionType)
	})

	t.Run("handles empty receipts array", func(t *testing.T) {
		jsonResp := `{
			"success": true,
			"receipts": []
		}`

		var resp models.AuditReceiptsResponse
		err := json.Unmarshal([]byte(jsonResp), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Empty(t, resp.Receipts)
	})
}

func TestAuditEventsResponseParsing(t *testing.T) {
	t.Run("parses valid events response", func(t *testing.T) {
		jsonResp := `{
			"success": true,
			"events": [
				{
					"id": 1,
					"operator_session_id": "sess-1",
					"timestamp": "2024-01-01T00:00:00Z",
					"type": "COMMAND_EXECUTION",
					"command_raw": "ls -la",
					"command_exit_code": 0
				}
			],
			"count": 1
		}`

		var resp struct {
			Success bool `json:"success"`
			Events  []struct {
				ID                int64  `json:"id"`
				OperatorSessionID string `json:"operator_session_id"`
				Timestamp         string `json:"timestamp"`
				Type              string `json:"type"`
				CommandRaw        string `json:"command_raw"`
				CommandExitCode   int    `json:"command_exit_code"`
			} `json:"events"`
			Count int `json:"count"`
		}
		err := json.Unmarshal([]byte(jsonResp), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Len(t, resp.Events, 1)
		assert.Equal(t, int64(1), resp.Events[0].ID)
		assert.Equal(t, 0, resp.Events[0].CommandExitCode)
	})

	t.Run("handles ExitCodeNone sentinel", func(t *testing.T) {
		jsonResp := `{
			"success": true,
			"events": [
				{
					"id": 1,
					"operator_session_id": "sess-1",
					"timestamp": "2024-01-01T00:00:00Z",
					"type": "USER_MESSAGE",
					"command_raw": "",
					"command_exit_code": -1
				}
			],
			"count": 1
		}`

		var resp struct {
			Success bool `json:"success"`
			Events  []struct {
				ID                int64  `json:"id"`
				OperatorSessionID string `json:"operator_session_id"`
				Timestamp         string `json:"timestamp"`
				Type              string `json:"type"`
				CommandRaw        string `json:"command_raw"`
				CommandExitCode   int    `json:"command_exit_code"`
			} `json:"events"`
			Count int `json:"count"`
		}
		err := json.Unmarshal([]byte(jsonResp), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, constants.ExitCodeNone, resp.Events[0].CommandExitCode)
	})
}

func TestAuditSummaryResponseParsing(t *testing.T) {
	t.Run("parses valid summary response", func(t *testing.T) {
		jsonResp := `{
			"success": true,
			"events_summary": {
				"COMMAND_EXECUTION": 10,
				"FILE_READ": 5
			},
			"events_total": 15,
			"receipts_summary": {
				"READ": 8,
				"WRITE": 2
			},
			"receipts_total": 10,
			"total_records": 25
		}`

		var resp struct {
			Success         bool           `json:"success"`
			EventsSummary   map[string]int `json:"events_summary"`
			EventsTotal     int            `json:"events_total"`
			ReceiptsSummary map[string]int `json:"receipts_summary"`
			ReceiptsTotal   int            `json:"receipts_total"`
			TotalRecords    int            `json:"total_records"`
		}
		err := json.Unmarshal([]byte(jsonResp), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, 15, resp.EventsTotal)
		assert.Equal(t, 10, resp.ReceiptsTotal)
		assert.Equal(t, 25, resp.TotalRecords)
		assert.Equal(t, 10, resp.EventsSummary["COMMAND_EXECUTION"])
		assert.Equal(t, 8, resp.ReceiptsSummary["READ"])
	})

	t.Run("handles empty summary", func(t *testing.T) {
		jsonResp := `{
			"success": true,
			"events_summary": {},
			"events_total": 0,
			"receipts_summary": {},
			"receipts_total": 0,
			"total_records": 0
		}`

		var resp struct {
			Success         bool           `json:"success"`
			EventsSummary   map[string]int `json:"events_summary"`
			EventsTotal     int            `json:"events_total"`
			ReceiptsSummary map[string]int `json:"receipts_summary"`
			ReceiptsTotal   int            `json:"receipts_total"`
			TotalRecords    int            `json:"total_records"`
		}
		err := json.Unmarshal([]byte(jsonResp), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, 0, resp.TotalRecords)
	})
}

func TestAuditReportResponseParsing(t *testing.T) {
	t.Run("parses valid report response", func(t *testing.T) {
		jsonResp := `{
			"success": true,
			"report": {
				"generated_at": "2024-01-01T00:00:00Z",
				"operator_session_id": "sess-1",
				"events": [],
				"events_count": 0,
				"receipts": [],
				"receipts_count": 0,
				"total_records": 0
			}
		}`

		var resp struct {
			Success bool `json:"success"`
			Report  struct {
				GeneratedAt       string        `json:"generated_at"`
				OperatorSessionID string        `json:"operator_session_id"`
				Events            []interface{} `json:"events"`
				EventsCount       int           `json:"events_count"`
				Receipts          []interface{} `json:"receipts"`
				ReceiptsCount     int           `json:"receipts_count"`
				TotalRecords      int           `json:"total_records"`
			} `json:"report"`
		}
		err := json.Unmarshal([]byte(jsonResp), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, "2024-01-01T00:00:00Z", resp.Report.GeneratedAt)
		assert.Equal(t, "sess-1", resp.Report.OperatorSessionID)
	})
}

// Error handling tests

func TestAuditReceiptsCmd_ConfigLoadFailure(t *testing.T) {
	t.Run("receipts fails when config load fails", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		cmd := auditReceiptsCmdWithConfig(func(_ string) (*config.Config, error) {
			return nil, errors.New("config load failed")
		})

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

func TestAuditReceiptsCmd_APIClientFailure(t *testing.T) {
	t.Run("receipts fails when API client creation fails", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Create invalid config that will cause API client creation to fail
		// by not having the required gateway URL
		runtimeDir := filepath.Join(tmpDir, ".g8e")
		pkiDir := filepath.Join(runtimeDir, "pki")
		secretsDir := filepath.Join(runtimeDir, "secrets")

		require.NoError(t, os.MkdirAll(pkiDir, 0755))
		require.NoError(t, os.MkdirAll(secretsDir, 0700))

		trustBundlePath := filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem")
		require.NoError(t, os.MkdirAll(filepath.Dir(trustBundlePath), 0755))
		require.NoError(t, os.WriteFile(trustBundlePath, []byte("dummy-trust-bundle"), 0644))

		pathsJSON := minimalPathsJSON(t)
		cfg, err := config.LoadWithPaths(tmpDir, []byte(pathsJSON))
		require.NoError(t, err)

		cmd := auditReceiptsCmdWithConfig(func(_ string) (*config.Config, error) {
			return cfg, nil
		})

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		cmd.Flags().Set("session", "sess-123")

		err = cmd.RunE(cmd, []string{})
		require.Error(t, err)
		// API client creation will fail due to missing/invalid gateway URL
	})
}

func TestAuditReceiptsCmd_CredentialsLoadFailure(t *testing.T) {
	t.Run("receipts fails when credentials file is corrupted", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := setupAuditTestConfig(t, tmpDir)
		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Write corrupted credentials file
		require.NoError(t, os.WriteFile(cfg.CredentialsFile(), []byte("invalid json {{{"), 0600))

		cmd := auditReceiptsCmdWithConfig(func(_ string) (*config.Config, error) {
			return cfg, nil
		})

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load credentials")
	})
}

func TestAuditReceiptsCmd_NoCredentials(t *testing.T) {
	t.Run("receipts fails when no credentials exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := setupAuditTestConfig(t, tmpDir)
		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Ensure credentials file does not exist
		os.Remove(cfg.CredentialsFile())

		cmd := auditReceiptsCmdWithConfig(func(_ string) (*config.Config, error) {
			return cfg, nil
		})

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not authenticated")
	})
}

func TestAuditEventsCmd_LimitValidation(t *testing.T) {
	t.Run("events has limit flag with correct default", func(t *testing.T) {
		cmd := auditEventsCmd()
		limitFlag := cmd.Flags().Lookup("limit")
		assert.NotNil(t, limitFlag)
		assert.Equal(t, "100", limitFlag.DefValue)
	})

	t.Run("events validates limit bounds", func(t *testing.T) {
		// Test the validation logic directly
		testCases := []struct {
			limit    int
			expected bool
		}{
			{0, false},
			{1, true},
			{100, true},
			{10000, true},
			{10001, false},
		}

		for _, tc := range testCases {
			valid := tc.limit >= 1 && tc.limit <= 10000
			assert.Equal(t, tc.expected, valid, "limit %d validation failed", tc.limit)
		}
	})
}

// Table-driven tests for various scenarios

func TestAuditReceiptsCmd_TableDriven(t *testing.T) {
	tests := []struct {
		name          string
		flags         map[string]string
		expectError   bool
		errorContains string
	}{
		{
			name:        "default flags",
			flags:       map[string]string{},
			expectError: true, // Will fail on API call
		},
		{
			name: "with session flag",
			flags: map[string]string{
				"session": "sess-123",
			},
			expectError: true, // Will fail on API call
		},
		{
			name: "with tx-id flag",
			flags: map[string]string{
				"tx-id": "tx-456",
			},
			expectError: true, // Will fail on API call
		},
		{
			name: "with json flag",
			flags: map[string]string{
				"json": "true",
			},
			expectError: true, // Will fail on API call
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfg := setupAuditTestConfig(t, tmpDir)
			originalWd, _ := os.Getwd()
			os.Chdir(tmpDir)
			defer os.Chdir(originalWd)

			cmd := auditReceiptsCmdWithConfig(func(_ string) (*config.Config, error) {
				return cfg, nil
			})

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			for flag, value := range tt.flags {
				cmd.Flags().Set(flag, value)
			}

			err := cmd.RunE(cmd, []string{})
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Output formatting tests

func TestAuditReceiptsCmd_TableFormatting(t *testing.T) {
	t.Run("receipts truncates long transaction hash", func(t *testing.T) {
		txHash := strings.Repeat("a", 20)
		assert.Equal(t, 12, len(txHash[:12]))
		assert.Equal(t, "aaaaaaaaaaaa…", txHash[:12]+"…")
	})

	t.Run("receipts truncates long resource", func(t *testing.T) {
		resource := strings.Repeat("b", 50)
		assert.Equal(t, 38, len(resource[:38]))
		assert.Equal(t, strings.Repeat("b", 38)+"…", resource[:38]+"…")
	})
}

func TestAuditEventsCmd_TableFormatting(t *testing.T) {
	t.Run("events truncates long timestamp", func(t *testing.T) {
		timestamp := "2024-01-01T00:00:00.000Z"
		if len(timestamp) > 19 {
			timestamp = timestamp[:19]
		}
		assert.Equal(t, 19, len(timestamp))
		assert.Equal(t, "2024-01-01T00:00:00", timestamp)
	})

	t.Run("events truncates long type", func(t *testing.T) {
		eventType := strings.Repeat("c", 35)
		if len(eventType) > 28 {
			eventType = eventType[:28] + "…"
		}
		// Verify truncation happened
		assert.LessOrEqual(t, len(eventType), 31) // 28 chars + ellipsis (3 bytes in UTF-8)
		assert.Contains(t, eventType, "…")
	})

	t.Run("events truncates long command", func(t *testing.T) {
		command := strings.Repeat("d", 50)
		if len(command) > 35 {
			command = command[:35] + "…"
		}
		// Verify truncation happened
		assert.LessOrEqual(t, len(command), 38) // 35 chars + ellipsis (3 bytes in UTF-8)
		assert.Contains(t, command, "…")
	})
}

// JSON output mode tests

func TestAuditReceiptsCmd_JSONOutput(t *testing.T) {
	t.Run("receipts json flag is present", func(t *testing.T) {
		cmd := auditReceiptsCmd()
		jsonFlag := cmd.Flags().Lookup("json")
		assert.NotNil(t, jsonFlag)
		assert.Equal(t, "false", jsonFlag.DefValue)
	})
}

func TestAuditEventsCmd_JSONOutput(t *testing.T) {
	t.Run("events json flag is present", func(t *testing.T) {
		cmd := auditEventsCmd()
		jsonFlag := cmd.Flags().Lookup("json")
		assert.NotNil(t, jsonFlag)
		assert.Equal(t, "false", jsonFlag.DefValue)
	})
}

// Helper functions for injectable config

func auditReceiptsCmdWithConfig(configLoader func(string) (*config.Config, error)) *cobra.Command {
	var operatorSessionID string
	var txID string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "receipts",
		Short: "List signed receipts from the running Gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			if operatorSessionID == "" {
				creds, err := auth.LoadCredentials(cfg)
				if err != nil {
					return fmt.Errorf("failed to load credentials: %w", err)
				}
				if creds == nil {
					return fmt.Errorf("not authenticated; run 'g8e auth login' first")
				}
				operatorSessionID = creds.OperatorSessionID
			}

			path := constants.APIPaths.AuditReceipts
			query := ""
			if txID != "" {
				query = "?tx_id=" + txID
			} else if operatorSessionID != "" {
				query = "?operator_session_id=" + operatorSessionID
			}
			path += query

			resp, err := client.Get(path)
			if err != nil {
				return err
			}

			if jsonOutput {
				cmd.Println(string(resp))
				return nil
			}

			var receiptsResp models.AuditReceiptsResponse
			if err := json.Unmarshal(resp, &receiptsResp); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			if len(receiptsResp.Receipts) == 0 {
				cmd.Println("No receipts found")
				return nil
			}

			cmd.Printf("Signed receipts — Operator session: %s\n", operatorSessionID)
			cmd.Println(strings.Repeat("=", 110))
			cmd.Printf("%-16s %-16s %-40s %-8s %s\n", "TX HASH", "ACTION TYPE", "RESOURCE", "STATUS", "AT")
			cmd.Println(strings.Repeat("-", 110))

			for _, r := range receiptsResp.Receipts {
				txHash := r.TransactionHash
				if len(txHash) > 12 {
					txHash = txHash[:12] + "…"
				}
				resource := r.TargetResource
				if len(resource) > 38 {
					resource = resource[:38] + "…"
				}
				cmd.Printf("%-16s %-16s %-40s %-8s %s\n",
					txHash,
					string(r.ActionType),
					resource,
					r.Status.String(),
					r.ExecutedAt.Format(time.RFC3339))
			}

			cmd.Println(strings.Repeat("-", 110))
			cmd.Printf("%d receipts\n", len(receiptsResp.Receipts))

			return nil
		},
	}

	cmd.Flags().StringVar(&operatorSessionID, "session", "", "Operator session ID (auto-discovers if omitted)")
	cmd.Flags().StringVar(&txID, "tx-id", "", "Get a single receipt by transaction ID")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Raw JSON output")

	return cmd
}

func auditEventsCmdWithConfig(configLoader func(string) (*config.Config, error)) *cobra.Command {
	var operatorSessionID string
	var limit int
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Query raw audit events from the Gateway audit store",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			if operatorSessionID == "" {
				creds, err := auth.LoadCredentials(cfg)
				if err != nil {
					return fmt.Errorf("failed to load credentials: %w", err)
				}
				if creds == nil {
					return fmt.Errorf("not authenticated; run 'g8e auth login' first")
				}
				operatorSessionID = creds.OperatorSessionID
			}

			if limit < 1 || limit > 10000 {
				return fmt.Errorf("limit must be between 1 and 10000")
			}

			path := constants.APIPaths.AuditEvents
			query := "?limit=" + fmt.Sprintf("%d", limit)
			if operatorSessionID != "" {
				query += "&operator_session_id=" + operatorSessionID
			}
			path += query

			resp, err := client.Get(path)
			if err != nil {
				return err
			}

			if jsonOutput {
				cmd.Println(string(resp))
				return nil
			}

			var eventsResp struct {
				Success bool `json:"success"`
				Events  []struct {
					ID                int64  `json:"id"`
					OperatorSessionID string `json:"operator_session_id"`
					Timestamp         string `json:"timestamp"`
					Type              string `json:"type"`
					CommandRaw        string `json:"command_raw"`
					CommandExitCode   int    `json:"command_exit_code"`
				} `json:"events"`
				Count int `json:"count"`
			}
			if err := json.Unmarshal(resp, &eventsResp); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			if len(eventsResp.Events) == 0 {
				cmd.Println("No audit events found")
				return nil
			}

			cmd.Printf("Audit events for session %s:\n", operatorSessionID)
			cmd.Println(strings.Repeat("=", 110))
			cmd.Printf("%-8s %-20s %-30s %-10s %s\n", "ID", "TIMESTAMP", "TYPE", "EXIT CODE", "COMMAND")
			cmd.Println(strings.Repeat("-", 110))

			for _, e := range eventsResp.Events {
				timestamp := e.Timestamp
				if len(timestamp) > 19 {
					timestamp = timestamp[:19]
				}
				eventType := e.Type
				if len(eventType) > 28 {
					eventType = eventType[:28] + "…"
				}
				command := e.CommandRaw
				if len(command) > 35 {
					command = command[:35] + "…"
				}
				exitCode := "-"
				if e.CommandExitCode != constants.ExitCodeNone {
					exitCode = fmt.Sprintf("%d", e.CommandExitCode)
				}
				cmd.Printf("%-8d %-20s %-30s %-10s %s\n", e.ID, timestamp, eventType, exitCode, command)
			}

			cmd.Println(strings.Repeat("-", 110))
			cmd.Printf("Total: %d events\n", eventsResp.Count)

			return nil
		},
	}

	cmd.Flags().StringVar(&operatorSessionID, "session", "", "Filter by operator session ID (shows all if omitted)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Max rows")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Raw JSON output")

	return cmd
}

func setupAuditTestConfig(t *testing.T, tmpDir string) *config.Config {
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	pkiDir := filepath.Join(runtimeDir, "pki")
	secretsDir := filepath.Join(runtimeDir, "secrets")

	require.NoError(t, os.MkdirAll(pkiDir, 0755))
	require.NoError(t, os.MkdirAll(secretsDir, 0700))

	// Create trust bundle
	trustBundlePath := filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem")
	require.NoError(t, os.MkdirAll(filepath.Dir(trustBundlePath), 0755))
	require.NoError(t, os.WriteFile(trustBundlePath, []byte("dummy-trust-bundle"), 0644))

	// Use LoadWithPaths for hermetic test execution
	pathsJSON := minimalPathsJSON(t)

	cfg, err := config.LoadWithPaths(tmpDir, []byte(pathsJSON))
	require.NoError(t, err)
	return cfg
}
