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

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Operator struct {
	ID           string `json:"id"`
	CloudSubtype string `json:"cloud_subtype"`
	Status       string `json:"status"`
}

type QueryFilter struct {
	Field string      `json:"field"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
}

type QueryRequest struct {
	Filters []QueryFilter `json:"filters"`
}

type SettingsResponse struct {
	Settings map[string]interface{} `json:"settings"`
}

type QueryRequestWithLimit struct {
	Filters []QueryFilter `json:"filters"`
	Limit   int           `json:"limit,omitempty"`
}

func dataCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: "Administer the local platform over mTLS",
		Long:  `Data management commands for users, operators, settings, and audit.`,
	}

	cmd.AddCommand(
		dataUsersCmd(),
		dataOperatorsCmd(),
		dataSettingsCmd(),
		dataStoreCmd(),
		dataAuditCmd(),
	)

	return cmd
}

func dataUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Manage user accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			resp, err := client.Get("/api/users")
			if err != nil {
				return err
			}

			var users []User
			if err := json.Unmarshal(resp, &users); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			cmd.Printf("Users (%d total)\n", len(users))
			cmd.Println(strings.Repeat("=", 110))
			for _, user := range users {
				cmd.Printf("  %s  %s\n", user.ID, user.Name)
			}

			return nil
		},
	}
	return cmd
}

func dataOperatorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operators",
		Short: "Manage operator instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			resp, err := client.Get("/api/operators")
			if err != nil {
				return err
			}

			var operators []Operator
			if err := json.Unmarshal(resp, &operators); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			cmd.Printf("Operators (%d total)\n", len(operators))
			cmd.Println(strings.Repeat("=", 110))
			for _, op := range operators {
				cmd.Printf("  %s  %s  %s\n", op.ID, op.CloudSubtype, op.Status)
			}

			return nil
		},
	}
	return cmd
}

func dataSettingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Manage Gateway settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			resp, err := client.Get("/db/settings/platform_settings")
			if err != nil {
				return err
			}

			var settings SettingsResponse
			if err := json.Unmarshal(resp, &settings); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			cmd.Println("Platform Settings")
			cmd.Println(strings.Repeat("=", 110))

			for key, value := range settings.Settings {
				cmd.Printf("  %s: %v\n", key, value)
			}

			return nil
		},
	}
	return cmd
}

func dataStoreCmd() *cobra.Command {
	var collection string
	var documentID string

	cmd := &cobra.Command{
		Use:   "store",
		Short: "Manage document storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			if collection == "" {
				return fmt.Errorf("--collection is required")
			}

			if documentID == "" {
				// List documents in collection
				query := QueryRequest{
					Filters: []QueryFilter{},
				}
				resp, err := client.Post(fmt.Sprintf("/db/%s/_query", collection), query)
				if err != nil {
					return err
				}

				cmd.Printf("Documents in collection '%s':\n", collection)
				cmd.Println(string(resp))
			} else {
				// Get specific document
				resp, err := client.Get(fmt.Sprintf("/db/%s/%s", collection, documentID))
				if err != nil {
					return err
				}

				cmd.Printf("Document %s/%s:\n", collection, documentID)
				cmd.Println(string(resp))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&collection, "collection", "", "Collection name")
	cmd.Flags().StringVar(&documentID, "document-id", "", "Document ID (omit to list collection)")

	return cmd
}

func dataAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Query audit vault",
	}

	cmd.AddCommand(
		dataAuditListCmd(),
		dataAuditSummaryCmd(),
	)

	return cmd
}

func dataAuditListCmd() *cobra.Command {
	var operatorSessionID string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List audit events for a session",
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
				return fmt.Errorf("--operator-session-id is required")
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

			cmd.Printf("Audit events for session %s:\n", operatorSessionID)
			cmd.Println(string(resp))

			return nil
		},
	}

	cmd.Flags().StringVar(&operatorSessionID, "operator-session-id", "", "Operator session ID")
	cmd.Flags().IntVar(&limit, "limit", 100, "Limit number of events")

	return cmd
}

func dataAuditSummaryCmd() *cobra.Command {
	var operatorSessionID string

	cmd := &cobra.Command{
		Use:   string(constants.StreamStatusSummary),
		Short: "Show audit event summary by type",
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

	cmd.Flags().StringVar(&operatorSessionID, "operator-session-id", "", "Filter by operator session ID")

	return cmd
}

func sqlDBQuery(dbPath, query string, args ...interface{}) (*sql.Rows, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	return db.Query(query, args...)
}
