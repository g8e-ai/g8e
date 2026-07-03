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
	"strings"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/paths"
)

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
	return dataUsersCmdWithConfig(loadConfig, defaultAPIClientFactory)
}

func dataUsersCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Manage user accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			client, err := clientFactory(cfg)
			if err != nil {
				return fmt.Errorf("data: create API client: %w", err)
			}

			resp, err := client.Get("/api/users")
			if err != nil {
				return fmt.Errorf("data: fetch users: %w", err)
			}

			var users []models.User
			if err := json.Unmarshal(resp, &users); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
			}

			cmd.Printf("Users (%d total)\n", len(users))
			cmd.Println(strings.Repeat("=", 110))
			for _, user := range users {
				cmd.Printf("  %s\n", user.ID)
			}

			return nil
		},
	}
	return cmd
}

func dataOperatorsCmd() *cobra.Command {
	return dataOperatorsCmdWithConfig(loadConfig, defaultAPIClientFactory)
}

func dataOperatorsCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operators",
		Short: "Manage Operator instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			client, err := clientFactory(cfg)
			if err != nil {
				return fmt.Errorf("data: create API client: %w", err)
			}

			resp, err := client.Get("/api/operators")
			if err != nil {
				return fmt.Errorf("data: fetch operators: %w", err)
			}

			var operators []models.OperatorDocumentGo
			if err := json.Unmarshal(resp, &operators); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
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
	return dataSettingsCmdWithConfig(loadConfig, defaultAPIClientFactory)
}

func dataSettingsCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Manage Gateway settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			client, err := clientFactory(cfg)
			if err != nil {
				return fmt.Errorf("data: create API client: %w", err)
			}

			resp, err := client.Get("/db/settings/platform_settings")
			if err != nil {
				return fmt.Errorf("data: fetch settings: %w", err)
			}

			var settings models.SettingsDocument
			if err := json.Unmarshal(resp, &settings); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
			}

			cmd.Println("Platform Settings")
			cmd.Println(strings.Repeat("=", 110))

			if settings.Settings != nil {
				data, err := json.MarshalIndent(settings.Settings, "", "  ")
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
				}
				cmd.Println(string(data))
			}

			return nil
		},
	}
	return cmd
}

func dataStoreCmd() *cobra.Command {
	return dataStoreCmdWithConfig(loadConfig, defaultAPIClientFactory)
}

func dataStoreCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory) *cobra.Command {
	var collection string
	var documentID string

	cmd := &cobra.Command{
		Use:   "store",
		Short: "Manage document storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			client, err := clientFactory(cfg)
			if err != nil {
				return fmt.Errorf("data: create API client: %w", err)
			}

			if collection == "" {
				return constants.ErrCollectionRequired
			}

			if documentID == "" {
				// List documents in collection
				query := models.DocQueryRequest{
					Filters: []models.DocFilter{},
				}
				resp, err := client.Post(fmt.Sprintf("/db/%s/_query", collection), query)
				if err != nil {
					return fmt.Errorf("data: query collection: %w", err)
				}

				cmd.Printf("Documents in collection '%s':\n", collection)
				cmd.Println(string(resp))
			} else {
				// Get specific document
				resp, err := client.Get(fmt.Sprintf("/db/%s/%s", collection, documentID))
				if err != nil {
					return fmt.Errorf("data: fetch document: %w", err)
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
	return dataAuditListCmdWithConfig(loadConfig, defaultAPIClientFactory)
}

func dataAuditListCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory) *cobra.Command {
	var operatorSessionID string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List audit events for a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			client, err := clientFactory(cfg)
			if err != nil {
				return fmt.Errorf("data: create API client: %w", err)
			}

			if operatorSessionID == "" {
				return constants.ErrOperatorSessionIDRequired
			}

			query := models.DocQueryRequest{
				Filters: []models.DocFilter{
					{Field: "operator_session_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", operatorSessionID))},
				},
				Limit: limit,
			}

			resp, err := client.Post("/db/audit_events/_query", query)
			if err != nil {
				return fmt.Errorf("data: query audit events: %w", err)
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
			dbPath := paths.Infra.DbPath
			if _, err := os.Stat(dbPath); os.IsNotExist(err) {
				return fmt.Errorf("%w: %s", constants.ErrAuditVaultDatabaseNotFound, dbPath)
			}

			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrSQLDatabaseOpenFailed, err)
			}
			defer db.Close()

			query := "SELECT type, COUNT(*) as count FROM events"
			if operatorSessionID != "" {
				query += " WHERE operator_session_id = ?"
			}
			query += " GROUP BY type"

			var rows *sql.Rows
			if operatorSessionID != "" {
				rows, err = db.Query(query, operatorSessionID)
			} else {
				rows, err = db.Query(query)
			}
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrAuditQueryFailed, err)
			}
			defer rows.Close()

			summary := make(map[string]int)
			total := 0
			for rows.Next() {
				var eventType string
				var count int
				if err := rows.Scan(&eventType, &count); err != nil {
					return fmt.Errorf("%w: %w", constants.ErrAuditScanFailed, err)
				}
				summary[eventType] = count
				total += count
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrAuditScanFailed, err)
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

	cmd.Flags().StringVar(&operatorSessionID, "operator-session-id", "", "Filter by Operator session ID")

	return cmd
}
