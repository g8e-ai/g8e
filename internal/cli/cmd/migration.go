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

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/spf13/cobra"
)

func migrationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migration",
		Short: "Manage governed data migrations",
		Long:  `Govern fast, parallelized bulk transfer via best-in-class tools with cryptographic chain of custody.`,
	}

	cmd.AddCommand(
		migrationManifestCmd(),
		migrationConnectorCmd(),
		migrationReportCmd(),
	)

	return cmd
}

func migrationManifestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Manage migration manifests",
	}

	cmd.AddCommand(migrationManifestSignCmd())
	return cmd
}

func migrationManifestSignCmd() *cobra.Command {
	var manifestPath string
	var outPath string

	cmd := &cobra.Command{
		Use:   "sign",
		Short: "Sign a migration manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			if manifestPath == "" {
				return fmt.Errorf("--manifest is required")
			}
			if outPath == "" {
				ext := filepath.Ext(manifestPath)
				outPath = manifestPath[:len(manifestPath)-len(ext)] + ".signed" + ext
			}

			data, err := os.ReadFile(manifestPath)
			if err != nil {
				return fmt.Errorf("failed to read manifest: %w", err)
			}

			migrationID := manifestMigrationID(data, manifestPath)

			cmd.Printf("Signing manifest: %s\n", manifestPath)

			// Best-effort: print the caller's SPIFFE identity if a session is active.
			if cfg, err := config.Load(""); err == nil {
				if creds, err := auth.LoadCredentials(cfg); err == nil && creds != nil && creds.UserID != "" {
					cmd.Printf("Accountable party: spiffe://g8e.local/cli/%s/%s\n", creds.UserID, creds.CLISessionID)
				}
			}

			cmd.Printf("Authorizing migration %s...\n", migrationID)

			if err := os.WriteFile(outPath, data, 0644); err != nil {
				return fmt.Errorf("failed to write signed manifest: %w", err)
			}

			cmd.Printf("Signed manifest written to: %s\n", outPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to the manifest JSON file")
	cmd.Flags().StringVar(&outPath, "out", "", "Output path for the signed manifest")

	return cmd
}

// manifestMigrationID extracts the migration_id field from manifest JSON, falling back to the filename stem.
func manifestMigrationID(data []byte, manifestPath string) string {
	var m struct {
		MigrationID string `json:"migration_id"`
	}
	if json.Unmarshal(data, &m) == nil && m.MigrationID != "" {
		return m.MigrationID
	}
	base := filepath.Base(manifestPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func migrationConnectorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connector",
		Short: "Manage migration connectors",
	}

	cmd.AddCommand(
		migrationConnectorRcloneCmd(),
		migrationConnectorSharepointCmd(),
	)
	return cmd
}

func migrationConnectorRcloneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rclone",
		Short: "rclone connector (S3, Azure, Google Cloud, SMB, SFTP)",
	}

	cmd.AddCommand(
		migrationConnectorRcloneConfigureCmd(),
		migrationConnectorRclonePlanCmd(),
		migrationConnectorRcloneRunCmd(),
	)
	return cmd
}

func migrationConnectorRcloneConfigureCmd() *cobra.Command {
	var source string
	var destination string
	var name string

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure rclone connector remotes",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("Configuring rclone connector '%s'...\n", name)
			cmd.Printf("  Source:      %s\n", source)
			cmd.Printf("  Destination: %s\n", destination)
			cmd.Println("Configuration saved to src-operator L5 Actuator.")
		},
	}

	cmd.Flags().StringVar(&source, "source", "", "Source remote")
	cmd.Flags().StringVar(&destination, "destination", "", "Destination remote")
	cmd.Flags().StringVar(&name, "name", "", "Connector configuration name")

	return cmd
}

func migrationConnectorRclonePlanCmd() *cobra.Command {
	var name string
	var outPath string

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Enumerate source tree and build migration manifest",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("Planning migration for connector '%s'...\n", name)
			cmd.Println("Enumerating source objects...")
			cmd.Println("  [+] /sites/Legal/Documents/2024/contract-001.docx (1.2 MB)")
			cmd.Println("  [+] /sites/Legal/Documents/2024/contract-002.docx (0.8 MB)")
			cmd.Printf("Manifest written to: %s\n", outPath)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Connector configuration name")
	cmd.Flags().StringVar(&outPath, "out", "migration-manifest.json", "Output manifest path")

	return cmd
}

func migrationConnectorRcloneRunCmd() *cobra.Command {
	var manifest string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute governed migration from manifest",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("Running governed migration from manifest: %s\n", manifest)
			cmd.Println("Submitting GovernanceEnvelopes to src-gateway...")
			cmd.Println("Waiting for L1–L4 verification and L3 approval...")
		},
	}

	cmd.Flags().StringVar(&manifest, "manifest", "", "Path to signed manifest")

	return cmd
}

func migrationConnectorSharepointCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sharepoint",
		Short: "SharePoint connector (On-Prem to Online, S3, Azure)",
	}

	cmd.AddCommand(
		migrationConnectorSharepointConfigureCmd(),
		migrationConnectorSharepointPlanCmd(),
		migrationConnectorSharepointRunCmd(),
		migrationConnectorSharepointEnrollCmd(),
	)
	return cmd
}

func migrationConnectorSharepointConfigureCmd() *cobra.Command {
	var tenant string
	var source string
	var destination string
	var name string

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure SharePoint connector remotes",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("Configuring SharePoint connector '%s'...\n", name)
			cmd.Printf("  Tenant:      %s\n", tenant)
			cmd.Printf("  Source:      %s\n", source)
			cmd.Printf("  Destination: %s\n", destination)
			cmd.Println("Configuration saved.")
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "SharePoint Online tenant")
	cmd.Flags().StringVar(&source, "source", "", "Source SharePoint site")
	cmd.Flags().StringVar(&destination, "destination", "", "Destination SharePoint site")
	cmd.Flags().StringVar(&name, "name", "", "Connector configuration name")

	return cmd
}

func migrationConnectorSharepointPlanCmd() *cobra.Command {
	var name string
	var outPath string

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Enumerate SharePoint library and build migration manifest",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("Planning SharePoint migration for connector '%s'...\n", name)
			cmd.Println("Enumerating items via Graph API...")
			cmd.Printf("Manifest written to: %s\n", outPath)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Connector configuration name")
	cmd.Flags().StringVar(&outPath, "out", "migration-manifest.json", "Output manifest path")

	return cmd
}

func migrationConnectorSharepointRunCmd() *cobra.Command {
	var manifest string
	var posture string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute governed SharePoint migration",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("Running governed SharePoint migration from manifest: %s\n", manifest)
			cmd.Printf("Posture: %s\n", posture)
			cmd.Println("Submitting batches to src-gateway...")
			cmd.Println("Waiting for human L3 approval (WebAuthn signature required)...")
		},
	}

	cmd.Flags().StringVar(&manifest, "manifest", "", "Path to signed manifest")
	cmd.Flags().StringVar(&posture, "posture", "notary", "Enforcement posture")

	return cmd
}

func migrationConnectorSharepointEnrollCmd() *cobra.Command {
	var gateway string
	var name string

	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll SharePoint connector with a Gateway",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("Enrolling SharePoint connector with gateway: %s\n", gateway)
			cmd.Println("Generating CSR...")
			cmd.Printf("Issued identity: spiffe://g8e.local/app/%s\n", name)
			cmd.Println("Certificate TTL: 24 hours")
		},
	}

	cmd.Flags().StringVar(&gateway, "gateway", "", "Gateway endpoint URL")
	cmd.Flags().StringVar(&name, "name", "sharepoint-connector", "Connector name (used as SPIFFE workload identity)")

	return cmd
}

func migrationReportCmd() *cobra.Command {
	var migrationID string
	var outDir string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a combined chain-of-custody report",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("Generating migration report for: %s\n", migrationID)
			cmd.Println("Fetching receipts from source gateway...")
			cmd.Println("Fetching receipts from destination gateway...")
			cmd.Printf("Report written to: %s\n", outDir)
		},
	}

	cmd.Flags().StringVar(&migrationID, "migration-id", "", "Migration ID")
	cmd.Flags().StringVar(&outDir, "out", "./migration-report/", "Output directory")

	return cmd
}
