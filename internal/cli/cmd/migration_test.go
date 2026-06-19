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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMigrationCmd(t *testing.T) {
	cmd := migrationCmd()

	if cmd == nil {
		t.Fatal("migrationCmd returned nil")
	}

	if cmd.Use != "migration" {
		t.Errorf("expected Use 'migration', got %q", cmd.Use)
	}

	if cmd.Short != "Manage governed data migrations" {
		t.Errorf("expected Short 'Manage governed data migrations', got %q", cmd.Short)
	}

	expectedSubcommands := []string{"manifest", "connector", "report"}
	for _, name := range expectedSubcommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestMigrationManifestCmd(t *testing.T) {
	cmd := migrationManifestCmd()

	if cmd == nil {
		t.Fatal("migrationManifestCmd returned nil")
	}

	if cmd.Use != "manifest" {
		t.Errorf("expected Use 'manifest', got %q", cmd.Use)
	}

	if cmd.Short != "Manage migration manifests" {
		t.Errorf("expected Short 'Manage migration manifests', got %q", cmd.Short)
	}

	expectedSubcommands := []string{"sign"}
	for _, name := range expectedSubcommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestMigrationManifestSignCmd(t *testing.T) {
	cmd := migrationManifestSignCmd()

	if cmd == nil {
		t.Fatal("migrationManifestSignCmd returned nil")
	}

	if cmd.Use != "sign" {
		t.Errorf("expected Use 'sign', got %q", cmd.Use)
	}

	if cmd.Short != "Sign a migration manifest" {
		t.Errorf("expected Short 'Sign a migration manifest', got %q", cmd.Short)
	}

	// Test flag definitions
	flags := cmd.Flags()
	if flags == nil {
		t.Fatal("flags is nil")
	}

	if flags.Lookup("manifest") == nil {
		t.Error("missing --manifest flag")
	}

	if flags.Lookup("out") == nil {
		t.Error("missing --out flag")
	}
}

func TestMigrationManifestSignCmd_Execution(t *testing.T) {
	tmpDir := t.TempDir()

	manifestPath := filepath.Join(tmpDir, "manifest.json")
	manifestContent := `{"version": "1.0", "migration_id": "TEST-MIG-001", "items": []}`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to create test manifest: %v", err)
	}

	var buf bytes.Buffer
	cmd := migrationManifestSignCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--manifest", manifestPath, "--out", filepath.Join(tmpDir, "signed.json")})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Signing manifest") {
		t.Errorf("expected 'Signing manifest' in output, got: %s", output)
	}

	if !strings.Contains(output, "TEST-MIG-001") {
		t.Errorf("expected migration ID 'TEST-MIG-001' in output, got: %s", output)
	}

	if !strings.Contains(output, "Signed manifest written to") {
		t.Errorf("expected 'Signed manifest written to' in output, got: %s", output)
	}

	outPath := filepath.Join(tmpDir, "signed.json")
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Errorf("output file was not created: %s", outPath)
	}
}

func TestMigrationManifestSignCmd_MigrationIDFallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Manifest without a migration_id — should fall back to filename stem.
	manifestPath := filepath.Join(tmpDir, "my-migration.json")
	if err := os.WriteFile(manifestPath, []byte(`{"version": "1.0"}`), 0644); err != nil {
		t.Fatalf("failed to create test manifest: %v", err)
	}

	var buf bytes.Buffer
	cmd := migrationManifestSignCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--manifest", manifestPath})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	if !strings.Contains(buf.String(), "my-migration") {
		t.Errorf("expected filename-derived migration ID 'my-migration' in output, got: %s", buf.String())
	}
}

func TestMigrationManifestSignCmd_MissingManifest(t *testing.T) {
	cmd := migrationManifestSignCmd()
	cmd.SetArgs([]string{})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when --manifest is missing")
	}

	if !strings.Contains(err.Error(), "--manifest is required") {
		t.Errorf("expected '--manifest is required' error, got: %v", err)
	}
}

func TestMigrationManifestSignCmd_AutoOutPath(t *testing.T) {
	tmpDir := t.TempDir()

	manifestPath := filepath.Join(tmpDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"version": "1.0"}`), 0644); err != nil {
		t.Fatalf("failed to create test manifest: %v", err)
	}

	var buf bytes.Buffer
	cmd := migrationManifestSignCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--manifest", manifestPath})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	expectedOutPath := filepath.Join(tmpDir, "manifest.signed.json")
	if _, err := os.Stat(expectedOutPath); os.IsNotExist(err) {
		t.Errorf("auto-generated output file was not created: %s", expectedOutPath)
	}
}

func TestMigrationManifestSignCmd_InvalidManifestPath(t *testing.T) {
	cmd := migrationManifestSignCmd()
	cmd.SetArgs([]string{"--manifest", "/nonexistent/path/manifest.json"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid manifest path")
	}

	if !strings.Contains(err.Error(), "failed to read manifest") {
		t.Errorf("expected 'failed to read manifest' error, got: %v", err)
	}
}

func TestMigrationConnectorCmd(t *testing.T) {
	cmd := migrationConnectorCmd()

	if cmd == nil {
		t.Fatal("migrationConnectorCmd returned nil")
	}

	if cmd.Use != "connector" {
		t.Errorf("expected Use 'connector', got %q", cmd.Use)
	}

	if cmd.Short != "Manage migration connectors" {
		t.Errorf("expected Short 'Manage migration connectors', got %q", cmd.Short)
	}

	expectedSubcommands := []string{"rclone", "sharepoint"}
	for _, name := range expectedSubcommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestMigrationConnectorRcloneCmd(t *testing.T) {
	cmd := migrationConnectorRcloneCmd()

	if cmd == nil {
		t.Fatal("migrationConnectorRcloneCmd returned nil")
	}

	if cmd.Use != "rclone" {
		t.Errorf("expected Use 'rclone', got %q", cmd.Use)
	}

	if cmd.Short != "rclone connector (S3, Azure, Google Cloud, SMB, SFTP)" {
		t.Errorf("expected Short 'rclone connector (S3, Azure, Google Cloud, SMB, SFTP)', got %q", cmd.Short)
	}

	expectedSubcommands := []string{"configure", "plan", "run"}
	for _, name := range expectedSubcommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestMigrationConnectorRcloneConfigureCmd(t *testing.T) {
	cmd := migrationConnectorRcloneConfigureCmd()

	if cmd == nil {
		t.Fatal("migrationConnectorRcloneConfigureCmd returned nil")
	}

	if cmd.Use != "configure" {
		t.Errorf("expected Use 'configure', got %q", cmd.Use)
	}

	if cmd.Short != "Configure rclone connector remotes" {
		t.Errorf("expected Short 'Configure rclone connector remotes', got %q", cmd.Short)
	}

	flags := cmd.Flags()
	if flags == nil {
		t.Fatal("flags is nil")
	}

	if flags.Lookup("source") == nil {
		t.Error("missing --source flag")
	}

	if flags.Lookup("destination") == nil {
		t.Error("missing --destination flag")
	}

	if flags.Lookup("name") == nil {
		t.Error("missing --name flag")
	}
}

func TestMigrationConnectorRcloneConfigureCmd_Execution(t *testing.T) {
	var buf bytes.Buffer
	cmd := migrationConnectorRcloneConfigureCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{
		"--source", "s3:bucket",
		"--destination", "azure:container",
		"--name", "test-connector",
	})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Configuring rclone connector 'test-connector'") {
		t.Errorf("expected connector name in output, got: %s", output)
	}

	if !strings.Contains(output, "Source:      s3:bucket") {
		t.Errorf("expected source in output, got: %s", output)
	}

	if !strings.Contains(output, "Destination: azure:container") {
		t.Errorf("expected destination in output, got: %s", output)
	}
}

func TestMigrationConnectorRclonePlanCmd(t *testing.T) {
	cmd := migrationConnectorRclonePlanCmd()

	if cmd == nil {
		t.Fatal("migrationConnectorRclonePlanCmd returned nil")
	}

	if cmd.Use != "plan" {
		t.Errorf("expected Use 'plan', got %q", cmd.Use)
	}

	if cmd.Short != "Enumerate source tree and build migration manifest" {
		t.Errorf("expected Short 'Enumerate source tree and build migration manifest', got %q", cmd.Short)
	}

	flags := cmd.Flags()
	if flags == nil {
		t.Fatal("flags is nil")
	}

	if flags.Lookup("name") == nil {
		t.Error("missing --name flag")
	}

	if flags.Lookup("out") == nil {
		t.Error("missing --out flag")
	}

	outFlag := flags.Lookup("out")
	if outFlag == nil {
		t.Fatal("--out flag not found")
	}
	if outFlag.DefValue != "migration-manifest.json" {
		t.Errorf("expected default --out value 'migration-manifest.json', got %q", outFlag.DefValue)
	}
}

func TestMigrationConnectorRclonePlanCmd_Execution(t *testing.T) {
	var buf bytes.Buffer
	cmd := migrationConnectorRclonePlanCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{
		"--name", "test-connector",
		"--out", "/tmp/test-manifest.json",
	})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Planning migration for connector 'test-connector'") {
		t.Errorf("expected connector name in output, got: %s", output)
	}

	if !strings.Contains(output, "Manifest written to: /tmp/test-manifest.json") {
		t.Errorf("expected manifest path in output, got: %s", output)
	}
}

func TestMigrationConnectorRcloneRunCmd(t *testing.T) {
	cmd := migrationConnectorRcloneRunCmd()

	if cmd == nil {
		t.Fatal("migrationConnectorRcloneRunCmd returned nil")
	}

	if cmd.Use != "run" {
		t.Errorf("expected Use 'run', got %q", cmd.Use)
	}

	if cmd.Short != "Execute governed migration from manifest" {
		t.Errorf("expected Short 'Execute governed migration from manifest', got %q", cmd.Short)
	}

	flags := cmd.Flags()
	if flags == nil {
		t.Fatal("flags is nil")
	}

	if flags.Lookup("manifest") == nil {
		t.Error("missing --manifest flag")
	}
}

func TestMigrationConnectorRcloneRunCmd_Execution(t *testing.T) {
	var buf bytes.Buffer
	cmd := migrationConnectorRcloneRunCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--manifest", "/tmp/manifest.json"})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	if !strings.Contains(buf.String(), "Running governed migration from manifest: /tmp/manifest.json") {
		t.Errorf("expected manifest path in output, got: %s", buf.String())
	}
}

func TestMigrationConnectorSharepointCmd(t *testing.T) {
	cmd := migrationConnectorSharepointCmd()

	if cmd == nil {
		t.Fatal("migrationConnectorSharepointCmd returned nil")
	}

	if cmd.Use != "sharepoint" {
		t.Errorf("expected Use 'sharepoint', got %q", cmd.Use)
	}

	if cmd.Short != "SharePoint connector (On-Prem to Online, S3, Azure)" {
		t.Errorf("expected Short 'SharePoint connector (On-Prem to Online, S3, Azure)', got %q", cmd.Short)
	}

	expectedSubcommands := []string{"configure", "plan", "run", "enroll"}
	for _, name := range expectedSubcommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestMigrationConnectorSharepointConfigureCmd(t *testing.T) {
	cmd := migrationConnectorSharepointConfigureCmd()

	if cmd == nil {
		t.Fatal("migrationConnectorSharepointConfigureCmd returned nil")
	}

	if cmd.Use != "configure" {
		t.Errorf("expected Use 'configure', got %q", cmd.Use)
	}

	if cmd.Short != "Configure SharePoint connector remotes" {
		t.Errorf("expected Short 'Configure SharePoint connector remotes', got %q", cmd.Short)
	}

	flags := cmd.Flags()
	if flags == nil {
		t.Fatal("flags is nil")
	}

	expectedFlags := []string{"tenant", "source", "destination", "name"}
	for _, flagName := range expectedFlags {
		if flags.Lookup(flagName) == nil {
			t.Errorf("missing --%s flag", flagName)
		}
	}
}

func TestMigrationConnectorSharepointConfigureCmd_Execution(t *testing.T) {
	var buf bytes.Buffer
	cmd := migrationConnectorSharepointConfigureCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{
		"--tenant", "contoso.onmicrosoft.com",
		"--source", "https://contoso.sharepoint.com/sites/source",
		"--destination", "https://contoso.sharepoint.com/sites/dest",
		"--name", "sp-connector",
	})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Configuring SharePoint connector 'sp-connector'") {
		t.Errorf("expected connector name in output, got: %s", output)
	}

	if !strings.Contains(output, "Tenant:      contoso.onmicrosoft.com") {
		t.Errorf("expected tenant in output, got: %s", output)
	}
}

func TestMigrationConnectorSharepointPlanCmd(t *testing.T) {
	cmd := migrationConnectorSharepointPlanCmd()

	if cmd == nil {
		t.Fatal("migrationConnectorSharepointPlanCmd returned nil")
	}

	if cmd.Use != "plan" {
		t.Errorf("expected Use 'plan', got %q", cmd.Use)
	}

	if cmd.Short != "Enumerate SharePoint library and build migration manifest" {
		t.Errorf("expected Short 'Enumerate SharePoint library and build migration manifest', got %q", cmd.Short)
	}

	flags := cmd.Flags()
	if flags == nil {
		t.Fatal("flags is nil")
	}

	if flags.Lookup("name") == nil {
		t.Error("missing --name flag")
	}

	if flags.Lookup("out") == nil {
		t.Error("missing --out flag")
	}
}

func TestMigrationConnectorSharepointPlanCmd_Execution(t *testing.T) {
	var buf bytes.Buffer
	cmd := migrationConnectorSharepointPlanCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{
		"--name", "sp-connector",
		"--out", "/tmp/sp-manifest.json",
	})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Planning SharePoint migration for connector 'sp-connector'") {
		t.Errorf("expected connector name in output, got: %s", output)
	}

	if !strings.Contains(output, "Manifest written to: /tmp/sp-manifest.json") {
		t.Errorf("expected manifest path in output, got: %s", output)
	}
}

func TestMigrationConnectorSharepointRunCmd(t *testing.T) {
	cmd := migrationConnectorSharepointRunCmd()

	if cmd == nil {
		t.Fatal("migrationConnectorSharepointRunCmd returned nil")
	}

	if cmd.Use != "run" {
		t.Errorf("expected Use 'run', got %q", cmd.Use)
	}

	if cmd.Short != "Execute governed SharePoint migration" {
		t.Errorf("expected Short 'Execute governed SharePoint migration', got %q", cmd.Short)
	}

	flags := cmd.Flags()
	if flags == nil {
		t.Fatal("flags is nil")
	}

	if flags.Lookup("manifest") == nil {
		t.Error("missing --manifest flag")
	}

	if flags.Lookup("posture") == nil {
		t.Error("missing --posture flag")
	}

	postureFlag := flags.Lookup("posture")
	if postureFlag == nil {
		t.Fatal("--posture flag not found")
	}
	if postureFlag.DefValue != "notary" {
		t.Errorf("expected default --posture value 'notary', got %q", postureFlag.DefValue)
	}
}

func TestMigrationConnectorSharepointRunCmd_Execution(t *testing.T) {
	var buf bytes.Buffer
	cmd := migrationConnectorSharepointRunCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{
		"--manifest", "/tmp/sp-manifest.json",
		"--posture", "consensus",
	})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Running governed SharePoint migration from manifest: /tmp/sp-manifest.json") {
		t.Errorf("expected manifest path in output, got: %s", output)
	}

	if !strings.Contains(output, "Posture: consensus") {
		t.Errorf("expected posture in output, got: %s", output)
	}
}

func TestMigrationConnectorSharepointEnrollCmd(t *testing.T) {
	cmd := migrationConnectorSharepointEnrollCmd()

	if cmd == nil {
		t.Fatal("migrationConnectorSharepointEnrollCmd returned nil")
	}

	if cmd.Use != "enroll" {
		t.Errorf("expected Use 'enroll', got %q", cmd.Use)
	}

	if cmd.Short != "Enroll SharePoint connector with a Gateway" {
		t.Errorf("expected Short 'Enroll SharePoint connector with a Gateway', got %q", cmd.Short)
	}

	flags := cmd.Flags()
	if flags == nil {
		t.Fatal("flags is nil")
	}

	if flags.Lookup("gateway") == nil {
		t.Error("missing --gateway flag")
	}

	if flags.Lookup("name") == nil {
		t.Error("missing --name flag")
	}

	nameFlag := flags.Lookup("name")
	if nameFlag.DefValue != "sharepoint-connector" {
		t.Errorf("expected default --name value 'sharepoint-connector', got %q", nameFlag.DefValue)
	}
}

func TestMigrationConnectorSharepointEnrollCmd_Execution(t *testing.T) {
	var buf bytes.Buffer
	cmd := migrationConnectorSharepointEnrollCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--gateway", "https://gateway.example.com"})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Enrolling SharePoint connector with gateway: https://gateway.example.com") {
		t.Errorf("expected gateway URL in output, got: %s", output)
	}

	if !strings.Contains(output, "spiffe://g8e.local/app/sharepoint-connector") {
		t.Errorf("expected SPIFFE identity in output, got: %s", output)
	}
}

func TestMigrationConnectorSharepointEnrollCmd_CustomName(t *testing.T) {
	var buf bytes.Buffer
	cmd := migrationConnectorSharepointEnrollCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{
		"--gateway", "https://gateway.example.com",
		"--name", "contoso-sp",
	})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	if !strings.Contains(buf.String(), "spiffe://g8e.local/app/contoso-sp") {
		t.Errorf("expected custom SPIFFE identity in output, got: %s", buf.String())
	}
}

func TestMigrationReportCmd(t *testing.T) {
	cmd := migrationReportCmd()

	if cmd == nil {
		t.Fatal("migrationReportCmd returned nil")
	}

	if cmd.Use != "report" {
		t.Errorf("expected Use 'report', got %q", cmd.Use)
	}

	if cmd.Short != "Generate a combined chain-of-custody report" {
		t.Errorf("expected Short 'Generate a combined chain-of-custody report', got %q", cmd.Short)
	}

	flags := cmd.Flags()
	if flags == nil {
		t.Fatal("flags is nil")
	}

	if flags.Lookup("migration-id") == nil {
		t.Error("missing --migration-id flag")
	}

	if flags.Lookup("out") == nil {
		t.Error("missing --out flag")
	}

	outFlag := flags.Lookup("out")
	if outFlag == nil {
		t.Fatal("--out flag not found")
	}
	if outFlag.DefValue != "./migration-report/" {
		t.Errorf("expected default --out value './migration-report/', got %q", outFlag.DefValue)
	}
}

func TestMigrationReportCmd_Execution(t *testing.T) {
	var buf bytes.Buffer
	cmd := migrationReportCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{
		"--migration-id", "MIG-2026-001",
		"--out", "/tmp/migration-report/",
	})

	if err := cmd.Execute(); err != nil {
		t.Errorf("execution failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Generating migration report for: MIG-2026-001") {
		t.Errorf("expected migration ID in output, got: %s", output)
	}

	if !strings.Contains(output, "Report written to: /tmp/migration-report/") {
		t.Errorf("expected report path in output, got: %s", output)
	}
}

func TestMigrationCommandStructure(t *testing.T) {
	rootCmd := migrationCmd()

	manifestCmd := findSubcommand(rootCmd, "manifest")
	if manifestCmd == nil {
		t.Fatal("manifest subcommand not found")
	}
	signCmd := findSubcommand(manifestCmd, "sign")
	if signCmd == nil {
		t.Fatal("sign subcommand not found")
	}

	connectorCmd := findSubcommand(rootCmd, "connector")
	if connectorCmd == nil {
		t.Fatal("connector subcommand not found")
	}
	rcloneCmd := findSubcommand(connectorCmd, "rclone")
	if rcloneCmd == nil {
		t.Fatal("rclone subcommand not found")
	}
	rcloneSubcommands := []string{"configure", "plan", "run"}
	for _, name := range rcloneSubcommands {
		if findSubcommand(rcloneCmd, name) == nil {
			t.Errorf("rclone subcommand %q not found", name)
		}
	}

	sharepointCmd := findSubcommand(connectorCmd, "sharepoint")
	if sharepointCmd == nil {
		t.Fatal("sharepoint subcommand not found")
	}
	sharepointSubcommands := []string{"configure", "plan", "run", "enroll"}
	for _, name := range sharepointSubcommands {
		if findSubcommand(sharepointCmd, name) == nil {
			t.Errorf("sharepoint subcommand %q not found", name)
		}
	}

	reportCmd := findSubcommand(rootCmd, "report")
	if reportCmd == nil {
		t.Fatal("report subcommand not found")
	}
}

// Helper function to find a subcommand by name
func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}
