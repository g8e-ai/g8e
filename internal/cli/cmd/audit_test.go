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
	"encoding/json"
	"testing"

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
					"gateway_signed": true,
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
		assert.Len(t, resp.Receipts, 0)
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
				CommandExitCode   *int   `json:"command_exit_code"`
			} `json:"events"`
			Count int `json:"count"`
		}
		err := json.Unmarshal([]byte(jsonResp), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Len(t, resp.Events, 1)
		assert.Equal(t, int64(1), resp.Events[0].ID)
		assert.NotNil(t, resp.Events[0].CommandExitCode)
		assert.Equal(t, 0, *resp.Events[0].CommandExitCode)
	})

	t.Run("handles nil exit code", func(t *testing.T) {
		jsonResp := `{
			"success": true,
			"events": [
				{
					"id": 1,
					"operator_session_id": "sess-1",
					"timestamp": "2024-01-01T00:00:00Z",
					"type": "COMMAND_EXECUTION",
					"command_raw": "ls -la"
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
				CommandExitCode   *int   `json:"command_exit_code"`
			} `json:"events"`
			Count int `json:"count"`
		}
		err := json.Unmarshal([]byte(jsonResp), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Nil(t, resp.Events[0].CommandExitCode)
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

// Integration test placeholder for when gateway is running
func TestAuditReceiptsCmd_Integration(t *testing.T) {
	t.Run("receipts with running gateway", func(t *testing.T) {
		t.Skip("Integration test requiring running gateway - test with ./g8e test e2e")
	})
}

func TestAuditExportCmd_Integration(t *testing.T) {
	t.Run("export with running gateway", func(t *testing.T) {
		t.Skip("Integration test requiring running gateway - test with ./g8e test e2e")
	})
}

func TestAuditReportCmd_Integration(t *testing.T) {
	t.Run("report with running gateway", func(t *testing.T) {
		t.Skip("Integration test requiring running gateway - test with ./g8e test e2e")
	})
}

func TestAuditEventsCmd_Integration(t *testing.T) {
	t.Run("events with running gateway", func(t *testing.T) {
		t.Skip("Integration test requiring running gateway - test with ./g8e test e2e")
	})
}

func TestAuditSummaryCmd_Integration(t *testing.T) {
	t.Run("summary with running gateway", func(t *testing.T) {
		t.Skip("Integration test requiring running gateway - test with ./g8e test e2e")
	})
}
