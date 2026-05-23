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
	"strings"

	"github.com/g8e-ai/g8e/services/g8eo/internal/cli/api"
	"github.com/g8e-ai/g8e/services/g8eo/internal/cli/config"
	"github.com/spf13/cobra"
)

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Operator struct {
	ID           string `json:"id"`
	CloudSubtype string `json:"cloud_subtype"`
	Status       string `json:"status"`
}

type DeviceLink struct {
	Token   string `json:"token"`
	Status  string `json:"status"`
	Uses    int    `json:"uses"`
	MaxUses int    `json:"max_uses"`
}

type QueryFilter struct {
	Field string      `json:"field"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
}

type QueryRequest struct {
	Filters []QueryFilter `json:"filters"`
}

type DeviceLinkCreateRequest struct {
	Email      string `json:"email"`
	MaxUses    int    `json:"max_uses"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type DeviceLinkCreateResponse struct {
	Token string `json:"token"`
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
		Short: "Administer the local substrate over mTLS",
		Long:  `Data management commands for users, operators, device-links, and settings.`,
	}

	cmd.AddCommand(
		dataUsersCmd(),
		dataOperatorsCmd(),
		dataDeviceLinksCmd(),
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
				cmd.Printf("  %s  %s  %s\n", user.ID, user.Email, user.Name)
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

func dataDeviceLinksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "device-links",
		Short: "Manage device-link tokens",
	}

	cmd.AddCommand(
		dataDeviceLinksListCmd(),
		dataDeviceLinksCreateCmd(),
		dataDeviceLinksDeleteCmd(),
	)

	return cmd
}

func dataDeviceLinksListCmd() *cobra.Command {
	var userID string
	var email string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List device-link tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			queryUserID := userID
			if queryUserID == "" && email == "" {
				queryUserID = os.Getenv("G8E_USER_ID")
			}

			if queryUserID == "" {
				return fmt.Errorf("provide --user-id, --email, or ensure G8E_USER_ID is set")
			}

			query := QueryRequest{
				Filters: []QueryFilter{
					{Field: "user_id", Op: "==", Value: queryUserID},
				},
			}

			resp, err := client.Post("/db/device_links/_query", query)
			if err != nil {
				return err
			}

			var links []DeviceLink
			if err := json.Unmarshal(resp, &links); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			cmd.Printf("Device Links for user %s (%d total)\n", queryUserID, len(links))
			cmd.Println(strings.Repeat("=", 110))
			for _, link := range links {
				cmd.Printf("  %s  status=%s  uses=%d/%d\n", link.Token, link.Status, link.Uses, link.MaxUses)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&userID, "user-id", "", "User ID")
	cmd.Flags().StringVar(&email, "email", "", "User email")

	return cmd
}

func dataDeviceLinksCreateCmd() *cobra.Command {
	var email string
	var count int
	var ttl int

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a device-link token",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if email == "" {
				return fmt.Errorf("--email is required")
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			req := DeviceLinkCreateRequest{
				Email:      email,
				MaxUses:    count,
				TTLSeconds: ttl,
			}

			resp, err := client.Post("/api/device-links", req)
			if err != nil {
				return err
			}

			var result DeviceLinkCreateResponse
			if err := json.Unmarshal(resp, &result); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Token == "" {
				return fmt.Errorf("response missing token field")
			}

			cmd.Printf("Device-link token created: %s\n", result.Token)
			cmd.Printf("Email: %s\n", email)
			cmd.Printf("Max uses: %d\n", count)
			cmd.Printf("TTL: %d seconds\n", ttl)

			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Email address (required)")
	cmd.Flags().IntVar(&count, "count", 1, "Number of uses")
	cmd.Flags().IntVar(&ttl, "ttl", 3600, "TTL in seconds")

	return cmd
}

func dataDeviceLinksDeleteCmd() *cobra.Command {
	var token string
	var userID string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a device-link token",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if token == "" {
				return fmt.Errorf("--token is required")
			}

			if userID == "" {
				userID = os.Getenv("G8E_USER_ID")
			}

			if userID == "" {
				return fmt.Errorf("--user-id is required or G8E_USER_ID must be set")
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/api/device-links/%s?user_id=%s", token, userID)
			_, err = client.Delete(path)
			if err != nil {
				return err
			}

			cmd.Printf("Device-link token deleted: %s\n", token)

			return nil
		},
	}

	cmd.Flags().StringVar(&token, "token", "", "Token to delete (required)")
	cmd.Flags().StringVar(&userID, "user-id", "", "User ID")

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

			resp, err := client.Get("/db/app_settings/app_settings")
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
	var operatorSessionID string
	var limit int

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Query audit vault",
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
				operatorSessionID = os.Getenv("G8E_OPERATOR_SESSION_ID")
			}

			if operatorSessionID == "" {
				return fmt.Errorf("--operator-session-id or G8E_OPERATOR_SESSION_ID is required")
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
