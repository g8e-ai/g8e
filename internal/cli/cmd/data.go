// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/fs"
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
	return dataUsersCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func dataUsersCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Manage user accounts",
		Long:  `List all user accounts registered on the running Gateway, retrieved over mTLS.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			client, err := clientFactory(fileSvc, cfg)
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
	return dataOperatorsCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func dataOperatorsCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operators",
		Short: "Manage Operator instances",
		Long:  `List all Operator instances registered on the running Gateway, retrieved over mTLS.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			client, err := clientFactory(fileSvc, cfg)
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
	return dataSettingsCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func dataSettingsCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Manage Gateway settings",
		Long:  `Fetch and display the current platform settings from the running Gateway over mTLS.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			client, err := clientFactory(fileSvc, cfg)
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
	return dataStoreCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func dataStoreCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var collection string
	var documentID string

	cmd := &cobra.Command{
		Use:   "store",
		Short: "Manage document storage",
		Long: `Query the Gateway document store over mTLS. Use --collection to specify a
collection name. Omit --document-id to list all documents in the collection,
or provide --document-id to fetch a specific document.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			client, err := clientFactory(fileSvc, cfg)
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
		Long: `Query the local audit vault for events and summaries. Subcommands provide
listing and aggregation of audit events by operator session.`,
	}

	cmd.AddCommand(
		dataAuditListCmd(),
		dataAuditSummaryCmd(),
	)

	return cmd
}

func dataAuditListCmd() *cobra.Command {
	return dataAuditListCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func dataAuditListCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var operatorSessionID string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List audit events for a session",
		Long: `List audit events for a specific operator session by querying the Gateway
audit store over mTLS. Use --operator-session-id to filter events and --limit
to control the number of results.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			client, err := clientFactory(fileSvc, cfg)
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
		Long: `Show an aggregated summary of audit events grouped by type, queried directly
from the local audit vault SQLite database. Use --operator-session-id to filter
by a specific session.`,
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
