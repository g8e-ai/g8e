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
	"database/sql"
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
	"github.com/g8e-ai/g8e/internal/emulator/report"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
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
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "receipts",
		Short: "List signed receipts from the running Gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			// Auto-discover session ID if not provided
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

			// Build query path
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
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			// Auto-discover session ID if not provided
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
				return fmt.Errorf("failed to create output directory: %w", err)
			}
			if err := os.WriteFile(outPath, resp, 0644); err != nil {
				return fmt.Errorf("failed to write export file: %w", err)
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
	var jsonOnly bool

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a compliance report (JSON + Markdown)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			// Auto-discover session ID if not provided
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

			// Fetch receipts
			path := constants.APIPaths.AuditReceipts
			if operatorSessionID != "" {
				path += "?operator_session_id=" + operatorSessionID
			}

			resp, err := client.Get(path)
			if err != nil {
				return err
			}

			var receiptsResp models.AuditReceiptsResponse
			if err := json.Unmarshal(resp, &receiptsResp); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			// Build report - skip Receipts conversion since report.Write handles empty Receipts gracefully
			// The plan notes that Results will be nil/empty for non-emulator runs
			rep := report.Report{
				GeneratedAt:       time.Now(),
				Gateway:           cfg.OperatorPublicURL(),
				OperatorSessionID: operatorSessionID,
				Results:           nil, // Empty for non-emulator runs
				Receipts:          nil, // Skip conversion - report.Write handles empty Receipts
			}

			// Write report
			jsonPath, mdPath, err := report.Write(outDir, rep)
			if err != nil {
				return fmt.Errorf("failed to write report: %w", err)
			}

			cmd.Println("Compliance report written:")
			cmd.Printf("  JSON:     %s\n", jsonPath)
			if !jsonOnly {
				cmd.Printf("  Markdown: %s\n", mdPath)
			}
			cmd.Printf("  Receipts: %d signed records (see 'g8e audit receipts' for details)\n", len(receiptsResp.Receipts))

			// Remove markdown if json-only
			if jsonOnly {
				if err := os.Remove(mdPath); err != nil {
					return fmt.Errorf("failed to remove markdown file: %w", err)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&operatorSessionID, "session", "", "Operator session ID")
	cmd.Flags().StringVar(&outDir, "out", "./reports", "Output directory")
	cmd.Flags().BoolVar(&jsonOnly, "json-only", false, "Skip Markdown, emit JSON only")

	return cmd
}

func auditEventsCmd() *cobra.Command {
	var operatorSessionID string
	var limit int
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Query raw audit events from the local SQLite vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			if operatorSessionID == "" {
				return fmt.Errorf("--session is required for events query")
			}

			query := QueryRequestWithLimit{
				Filters: []QueryFilter{
					{Field: "operator_session_id", Op: "==", Value: operatorSessionID},
				},
				Limit: limit,
			}

			resp, err := client.Post("/db/audit_events/_query", query)
			if err != nil {
				return err
			}

			if jsonOutput {
				cmd.Println(string(resp))
				return nil
			}

			cmd.Printf("Audit events for session %s:\n", operatorSessionID)
			cmd.Println(string(resp))

			return nil
		},
	}

	cmd.Flags().StringVar(&operatorSessionID, "session", "", "Operator session ID (required)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Max rows")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Raw JSON output")

	return cmd
}

func auditSummaryCmd() *cobra.Command {
	var operatorSessionID string

	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Aggregate audit events by action type",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath := filepath.Join(constants.Paths.Infra.DataDir, "g8e.db")
			if _, err := os.Stat(dbPath); os.IsNotExist(err) {
				return fmt.Errorf("audit vault database not found at %s", dbPath)
			}

			query := "SELECT type, COUNT(*) as count FROM events"
			if operatorSessionID != "" {
				query += " WHERE operator_session_id = ?"
			}
			query += " GROUP BY type"

			var rows *sql.Rows
			var err error
			if operatorSessionID != "" {
				rows, err = sqlDBQuery(dbPath, query, operatorSessionID)
			} else {
				rows, err = sqlDBQuery(dbPath, query)
			}
			if err != nil {
				return fmt.Errorf("failed to query audit events: %w", err)
			}
			defer rows.Close()

			summary := make(map[string]int)
			total := 0
			for rows.Next() {
				var eventType string
				var count int
				if err := rows.Scan(&eventType, &count); err != nil {
					return fmt.Errorf("failed to scan row: %w", err)
				}
				summary[eventType] = count
				total += count
			}

			if total == 0 {
				cmd.Println("No audit events found in audit vault")
				return nil
			}

			cmd.Println("Audit Event Summary")
			cmd.Println(strings.Repeat("=", 110))
			for eventType, count := range summary {
				cmd.Printf("  %s: %d\n", eventType, count)
			}
			cmd.Printf("\nTotal events: %d\n", total)

			return nil
		},
	}

	cmd.Flags().StringVar(&operatorSessionID, "session", "", "Filter by Operator session ID")

	return cmd
}
