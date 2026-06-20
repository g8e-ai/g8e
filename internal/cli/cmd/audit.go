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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/spf13/cobra"
)

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Run audit reports for compliance",
		Long:  `Audit commands for compliance evidence, signed receipts, and event logs.`,
	}

	cmd.AddCommand(
		auditReceiptsCmd(),
		auditExportCmd(),
		auditReportCmd(),
		auditEventsCmd(),
		auditSummaryCmd(),
	)

	return cmd
}

func auditReceiptsCmd() *cobra.Command {
	var operatorSessionID string
	var txID string
	var migrationID string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "receipts",
		Short: "List signed receipts from the running Gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			// Auto-discover session ID if not provided
			if operatorSessionID == "" {
				creds, err := auth.LoadCredentials(cfg)
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
				}
				if creds == nil {
					return constants.ErrNotAuthenticated
				}
				operatorSessionID = creds.OperatorSessionID
			}

			// Build query path
			path := constants.APIPaths.AuditReceipts
			query := ""
			if txID != "" {
				query = "?tx_id=" + txID
			} else if migrationID != "" {
				query = "?migration_id=" + migrationID
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
				return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
			}

			if len(receiptsResp.Receipts) == 0 {
				cmd.Println("No receipts found")
				return nil
			}

			// Display table output
			sessionDisplay := operatorSessionID
			if sessionDisplay == "" {
				sessionDisplay = "(all)"
			}
			cmd.Printf("Signed receipts — Operator session: %s\n", sessionDisplay)
			cmd.Println(strings.Repeat("=", 110))
			cmd.Printf("%-16s %-16s %-40s %-8s %s\n", "TX HASH", "ACTION TYPE", "RESOURCE", "STATUS", "AT")
			cmd.Println(strings.Repeat("-", 110))

			l2Valid := 0
			l3Valid := 0
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
				if r.L2Valid {
					l2Valid++
				}
				if r.L3Valid {
					l3Valid++
				}
			}

			cmd.Println(strings.Repeat("-", 110))
			cmd.Printf("%d receipts  |  L2 valid: %d  |  L3 valid: %d\n",
				len(receiptsResp.Receipts), l2Valid, l3Valid)

			return nil
		},
	}

	cmd.Flags().StringVar(&operatorSessionID, "session", "", "Operator session ID (auto-discovers if omitted)")
	cmd.Flags().StringVar(&txID, "tx-id", "", "Get a single receipt by transaction ID")
	cmd.Flags().StringVar(&migrationID, "migration-id", "", "Filter by migration ID")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Raw JSON output")

	return cmd
}

func auditExportCmd() *cobra.Command {
	var operatorSessionID string
	var outPath string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the full receipts bundle for archival",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			// Auto-discover session ID if not provided
			if operatorSessionID == "" {
				creds, err := auth.LoadCredentials(cfg)
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
				}
				if creds == nil {
					return constants.ErrNotAuthenticated
				}
				operatorSessionID = creds.OperatorSessionID
			}

			// Build query path
			path := constants.APIPaths.AuditReceiptsExport
			if operatorSessionID != "" {
				path += "?operator_session_id=" + operatorSessionID
			}

			resp, err := client.Get(path)
			if err != nil {
				return err
			}

			// Write to file
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
			}
			if err := os.WriteFile(outPath, resp, 0644); err != nil {
				return fmt.Errorf("%w: failed to write export file: %w", constants.ErrInternal, err)
			}

			cmd.Printf("Receipts export written to: %s (%d bytes)\n", outPath, len(resp))
			return nil
		},
	}

	cmd.Flags().StringVar(&operatorSessionID, "session", "", "Operator session ID")
	cmd.Flags().StringVar(&outPath, "out", "./receipts-export.json", "Output file path")

	return cmd
}

func auditReportCmd() *cobra.Command {
	var operatorSessionID string
	var outDir string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a compliance report (JSON + Markdown)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			// Auto-discover session ID if not provided
			if operatorSessionID == "" {
				creds, err := auth.LoadCredentials(cfg)
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
				}
				if creds == nil {
					return constants.ErrNotAuthenticated
				}
				operatorSessionID = creds.OperatorSessionID
			}

			// Fetch comprehensive report from Gateway
			path := constants.APIPaths.AuditReport
			if operatorSessionID != "" {
				path += "?operator_session_id=" + operatorSessionID
			}

			resp, err := client.Get(path)
			if err != nil {
				return err
			}

			var reportResp struct {
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
			if err := json.Unmarshal(resp, &reportResp); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
			}

			// Write report to file
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
			}

			jsonPath := filepath.Join(outDir, constants.ComplianceReportFilename)
			if err := os.WriteFile(jsonPath, resp, 0644); err != nil {
				return fmt.Errorf("%w: failed to write JSON report: %w", constants.ErrInternal, err)
			}

			cmd.Println("Compliance report written:")
			cmd.Printf("  JSON:     %s\n", jsonPath)
			cmd.Printf("  Events:   %d\n", reportResp.Report.EventsCount)
			cmd.Printf("  Receipts: %d\n", reportResp.Report.ReceiptsCount)
			cmd.Printf("  Total:    %d records\n", reportResp.Report.TotalRecords)

			return nil
		},
	}

	cmd.Flags().StringVar(&operatorSessionID, "session", "", "Operator session ID")
	cmd.Flags().StringVar(&outDir, "out", "./reports", "Output directory")

	return cmd
}

func auditEventsCmd() *cobra.Command {
	var operatorSessionID string
	var limit int
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Query raw audit events from the Gateway audit store",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			// Auto-discover session ID if not provided
			if operatorSessionID == "" {
				creds, err := auth.LoadCredentials(cfg)
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
				}
				if creds == nil {
					return constants.ErrNotAuthenticated
				}
				operatorSessionID = creds.OperatorSessionID
			}

			// Validate limit
			if limit < 1 || limit > 10000 {
				return fmt.Errorf("%w: limit must be between 1 and 10000", constants.ErrValidationFailed)
			}

			// Build query path
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
					CommandExitCode   *int   `json:"command_exit_code"`
				} `json:"events"`
				Count int `json:"count"`
			}
			if err := json.Unmarshal(resp, &eventsResp); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
			}

			if len(eventsResp.Events) == 0 {
				cmd.Println("No audit events found")
				return nil
			}

			sessionDisplay := operatorSessionID
			if sessionDisplay == "" {
				sessionDisplay = "(all)"
			}
			cmd.Printf("Audit events for session %s:\n", sessionDisplay)
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
				if e.CommandExitCode != nil {
					exitCode = fmt.Sprintf("%d", *e.CommandExitCode)
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

func auditSummaryCmd() *cobra.Command {
	var operatorSessionID string

	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Aggregate audit events and receipts by type",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			// Auto-discover session ID if not provided
			if operatorSessionID == "" {
				creds, err := auth.LoadCredentials(cfg)
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
				}
				if creds == nil {
					return constants.ErrNotAuthenticated
				}
				operatorSessionID = creds.OperatorSessionID
			}

			// Build query path
			path := constants.APIPaths.AuditSummary
			if operatorSessionID != "" {
				path += "?operator_session_id=" + operatorSessionID
			}

			resp, err := client.Get(path)
			if err != nil {
				return err
			}

			var summaryResp struct {
				Success         bool           `json:"success"`
				EventsSummary   map[string]int `json:"events_summary"`
				EventsTotal     int            `json:"events_total"`
				ReceiptsSummary map[string]int `json:"receipts_summary"`
				ReceiptsTotal   int            `json:"receipts_total"`
				TotalRecords    int            `json:"total_records"`
			}
			if err := json.Unmarshal(resp, &summaryResp); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
			}

			if summaryResp.TotalRecords == 0 {
				cmd.Println("No audit records found")
				return nil
			}

			cmd.Println("Audit Summary")
			cmd.Println(strings.Repeat("=", 110))

			if summaryResp.EventsTotal > 0 {
				cmd.Printf("\nEvents (%d total):\n", summaryResp.EventsTotal)
				for eventType, count := range summaryResp.EventsSummary {
					cmd.Printf("  %-50s %d\n", eventType, count)
				}
			}

			if summaryResp.ReceiptsTotal > 0 {
				cmd.Printf("\nGoverned Receipts (%d total):\n", summaryResp.ReceiptsTotal)
				for keyStr, count := range summaryResp.ReceiptsSummary {
					cmd.Printf("  %-40s %d\n", keyStr, count)
				}
			}

			cmd.Printf("\nTotal records: %d\n", summaryResp.TotalRecords)

			return nil
		},
	}

	cmd.Flags().StringVar(&operatorSessionID, "session", "", "Filter by Operator session ID")

	return cmd
}
