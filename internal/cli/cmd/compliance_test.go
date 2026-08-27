// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
)

// writeTestKSICatalog creates and writes a valid test KSI catalog via the given file service.
func writeTestKSICatalog(t *testing.T, fileSvc fs.RuntimeFileService, relPath string) string {
	t.Helper()
	cat := &compliance.KSICatalog{
		Version: "CR26-TEST",
		Source:  "test-source",
		KSIs: []compliance.KSI{
			{
				ID:          "KSI-CMT-01",
				Title:       "Git-backed infrastructure change ledger",
				Category:    compliance.KSICategoryCMT,
				Description: "All infra changes tracked in git ledger",
				ControlRefs: []string{"CM-3", "CM-8"},
				ApplicableClasses: []compliance.CertificationClass{
					compliance.ClassA, compliance.ClassB, compliance.ClassC, compliance.ClassD,
				},
				ValidationCycle: compliance.ValidationCycleMachine,
				Status:          compliance.KSIStatusSatisfied,
			},
			{
				ID:          "KSI-MLA-03",
				Title:       "Append-only cryptographic audit logging",
				Category:    compliance.KSICategoryMLA,
				Description: "Audit records cryptographically signed and chained",
				ControlRefs: []string{"AU-2", "AU-9"},
				ApplicableClasses: []compliance.CertificationClass{
					compliance.ClassA, compliance.ClassB, compliance.ClassC, compliance.ClassD,
				},
				ValidationCycle: compliance.ValidationCycleMachine,
				Status:          compliance.KSIStatusSatisfied,
			},
		},
	}
	data, err := json.MarshalIndent(cat, "", "  ")
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, data, constants.PermFilePublic))
	return fileSvc.Resolve(relPath)
}

// setupTestVaultWithKey initializes a test vault header and writes its private key to secrets.
func setupTestVaultWithKey(t *testing.T, fileSvc fs.RuntimeFileService, privKey []byte) {
	t.Helper()
	vaultDir := fileSvc.Resolve(constants.VaultDirname)
	require.NoError(t, os.MkdirAll(vaultDir, constants.PermDirPrivate))
	header, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(vaultDir))

	vaultKeyRel := constants.SecretsDirname + "/" + constants.VaultKeyFilename
	hexKey := hex.EncodeToString(privKey)
	require.NoError(t, fileSvc.WriteFile(context.Background(), vaultKeyRel, []byte(hexKey), constants.PermFilePrivate))
}

// writeTestOverlays creates and writes an overlay catalog JSON file in the given directory.
func writeTestOverlays(t *testing.T, dir string, overlayCat *compliance.OverlayCatalog) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, constants.PermDirStandard))
	data, err := json.MarshalIndent(overlayCat, "", "  ")
	require.NoError(t, err)
	overlayFile := filepath.Join(dir, constants.COSAiSOverlaysFilename)
	require.NoError(t, os.WriteFile(overlayFile, data, constants.PermFilePublic))
	return dir
}

// TestComplianceCmd_Structure asserts that complianceCmd() is non-nil and has
// the expected "export", "ksi", "ksi-history", and "overlay" subcommands.
func TestComplianceCmd_Structure(t *testing.T) {
	cmd := complianceCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "compliance", cmd.Use)

	subcommands := cmd.Commands()
	assert.Len(t, subcommands, 4)

	names := make(map[string]bool, 4)
	for _, sub := range subcommands {
		names[sub.Name()] = true
	}
	assert.True(t, names["export"], "compliance should have 'export' subcommand")
	assert.True(t, names["ksi"], "compliance should have 'ksi' subcommand")
	assert.True(t, names["ksi-history"], "compliance should have 'ksi-history' subcommand")
	assert.True(t, names["overlay"], "compliance should have 'overlay' subcommand")
}

// TestComplianceExportCmd_FileSvcFactoryError asserts that the export subcommand
// wraps fileSvcFactory errors with ErrFileServiceInit.
func TestComplianceExportCmd_FileSvcFactoryError(t *testing.T) {
	cmd := complianceExportCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// TestComplianceKSICmd_FileSvcFactoryError asserts that the ksi subcommand
// wraps fileSvcFactory errors with ErrFileServiceInit.
func TestComplianceKSICmd_FileSvcFactoryError(t *testing.T) {
	cmd := complianceKSICmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// TestComplianceKSIHistoryCmd_FileSvcFactoryError asserts that the ksi-history
// subcommand wraps fileSvcFactory errors with ErrFileServiceInit.
func TestComplianceKSIHistoryCmd_FileSvcFactoryError(t *testing.T) {
	cmd := complianceKSIHistoryCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// TestComplianceExportCmd_InvalidFormat asserts that a non-"oscal" format
// returns a validation error before touching fileSvcFactory.
func TestComplianceExportCmd_InvalidFormat(t *testing.T) {
	cmd := complianceExportCmdWithConfig(failingFileSvcFactory(errFactory))
	cmd.Flags().Set("format", "xml")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Contains(t, err.Error(), "xml")
}

// TestComplianceOverlayCmd_FileSvcFactoryError asserts that the overlay subcommand
// wraps fileSvcFactory errors with ErrFileServiceInit.
func TestComplianceOverlayCmd_FileSvcFactoryError(t *testing.T) {
	cmd := complianceOverlayCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// TestValidateCertClass asserts valid and invalid certification class inputs.
func TestValidateCertClass(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "class A", input: "A", wantErr: false},
		{name: "class B", input: "B", wantErr: false},
		{name: "class C", input: "C", wantErr: false},
		{name: "class D", input: "D", wantErr: false},
		{name: "lowercase c", input: "c", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "two chars", input: "AB", wantErr: true},
		{name: "invalid char", input: "X", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class, err := validateCertClass(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, constants.ErrValidationFailed)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.input, string(class))
			}
		})
	}
}

// TestComplianceExportCmd_ValidationErrors tests flag and input validation for export.
func TestComplianceExportCmd_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, fileSvc fs.RuntimeFileService) (catalogPath, certClass string)
		expectedErr error
		errContains string
	}{
		{
			name: "invalid certification class",
			setup: func(t *testing.T, fileSvc fs.RuntimeFileService) (string, string) {
				catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)
				return catPath, "INVALID"
			},
			expectedErr: constants.ErrValidationFailed,
		},
		{
			name: "catalog file not found",
			setup: func(t *testing.T, fileSvc fs.RuntimeFileService) (string, string) {
				return constants.TestPathNonexistentCatalog, "C"
			},
			errContains: "load KSI catalog",
		},
		{
			name: "invalid catalog missing version",
			setup: func(t *testing.T, fileSvc fs.RuntimeFileService) (string, string) {
				invalidCat := &compliance.KSICatalog{
					Source: "test",
					KSIs:   []compliance.KSI{{ID: "KSI-CMT-01", Title: "Test", Category: compliance.KSICategoryCMT}},
				}
				data, err := json.Marshal(invalidCat)
				require.NoError(t, err)
				require.NoError(t, fileSvc.WriteFile(context.Background(), constants.TestInvalidJSONFilename, data, constants.PermFilePublic))
				return fileSvc.Resolve(constants.TestInvalidJSONFilename), "C"
			},
			expectedErr: constants.ErrKSICatalogInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSvc, _ := newCmdTestEnv(t)
			catPath, certClass := tt.setup(t, fileSvc)

			cmd := complianceExportCmdWithConfig(fileSvcFactoryFor(fileSvc))
			require.NoError(t, cmd.Flags().Set("catalog", catPath))
			require.NoError(t, cmd.Flags().Set("class", certClass))

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			err := cmd.RunE(cmd, nil)
			require.Error(t, err)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			}
			if tt.errContains != "" {
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

// TestComplianceExportCmd_StoresUnavailable asserts that export fails gracefully
// with ErrReportStoreUnavailable when backend evaluation stores cannot be opened.
func TestComplianceExportCmd_StoresUnavailable(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)

	// Block data directory with a plain file so SQLite / audit store cannot open.
	dataDir := fileSvc.Resolve(constants.DataDirname)
	require.NoError(t, os.RemoveAll(dataDir))
	require.NoError(t, os.WriteFile(dataDir, []byte("blocking-file"), constants.PermFileReadOnly))

	cmd := complianceExportCmdWithConfig(fileSvcFactoryFor(fileSvc))
	require.NoError(t, cmd.Flags().Set("catalog", catPath))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	runErr := cmd.RunE(cmd, nil)
	require.Error(t, runErr)
	assert.ErrorIs(t, runErr, constants.ErrReportStoreUnavailable)
}

// TestComplianceExportCmd_Success_DefaultOutDir asserts that compliance export
// generates both OSCAL artifacts, writes them to the default output directory,
// saves a KSI history snapshot, and prints execution summary.
func TestComplianceExportCmd_Success_DefaultOutDir(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	setupTestVaultWithKey(t, fileSvc, privKey)

	catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)

	cmd := complianceExportCmdWithConfig(fileSvcFactoryFor(fileSvc))
	require.NoError(t, cmd.Flags().Set("catalog", catPath))
	require.NoError(t, cmd.Flags().Set("class", "C"))

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, nil)
	require.NoError(t, err)

	outStr := buf.String()
	assert.Contains(t, outStr, "OSCAL artifacts written to:")
	assert.Contains(t, outStr, constants.OSCALComponentDefFilename)
	assert.Contains(t, outStr, constants.OSCALAssessmentResultsFilename)
	assert.Contains(t, outStr, "KSI evaluation: 0 satisfied, 2 not satisfied (Class C)")

	// Verify OSCAL component-definition artifact exists and parses.
	ctx := context.Background()
	compDefRel := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.OSCALComponentDefFilename)
	exists, err := fileSvc.FileExists(ctx, compDefRel)
	require.NoError(t, err)
	assert.True(t, exists)

	compDefData, err := fileSvc.ReadFile(ctx, compDefRel)
	require.NoError(t, err)
	var compDef compliance.OSCALComponentDefinition
	require.NoError(t, json.Unmarshal(compDefData, &compDef))
	assert.NotEmpty(t, compDef.UUID)
	assert.NotEmpty(t, compDef.Components)

	// Verify OSCAL assessment-results artifact exists and parses.
	assessRel := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.OSCALAssessmentResultsFilename)
	exists, err = fileSvc.FileExists(ctx, assessRel)
	require.NoError(t, err)
	assert.True(t, exists)

	assessData, err := fileSvc.ReadFile(ctx, assessRel)
	require.NoError(t, err)
	var assessResults compliance.OSCALAssessmentResults
	require.NoError(t, json.Unmarshal(assessData, &assessResults))
	assert.NotEmpty(t, assessResults.UUID)
	assert.NotEmpty(t, assessResults.Results)

	// Verify KSI history store snapshot was written.
	historyStore := newKSIHistoryStore(fileSvc)
	snapshots, err := historyStore.ListSnapshots(ctx)
	require.NoError(t, err)
	assert.Len(t, snapshots, 1)
}

// TestComplianceExportCmd_Success_CustomOutDir asserts that compliance export
// writes artifacts to the directory specified by the --out flag.
func TestComplianceExportCmd_Success_CustomOutDir(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)

	cmd := complianceExportCmdWithConfig(fileSvcFactoryFor(fileSvc))
	require.NoError(t, cmd.Flags().Set("catalog", catPath))
	require.NoError(t, cmd.Flags().Set("out", constants.TestCustomComplianceOutDir))

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	ctx := context.Background()
	compDefRel := filepath.Join(constants.TestCustomComplianceOutDir, constants.OSCALComponentDefFilename)
	exists, err := fileSvc.FileExists(ctx, compDefRel)
	require.NoError(t, err)
	assert.True(t, exists)

	assessRel := filepath.Join(constants.TestCustomComplianceOutDir, constants.OSCALAssessmentResultsFilename)
	exists, err = fileSvc.FileExists(ctx, assessRel)
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestComplianceExportCmd_CertClasses asserts export succeeds across all certification classes.
func TestComplianceExportCmd_CertClasses(t *testing.T) {
	classes := []string{"A", "B", "C", "D"}
	for _, certClass := range classes {
		t.Run("class "+certClass, func(t *testing.T) {
			fileSvc, _ := newCmdTestEnv(t)
			catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)

			cmd := complianceExportCmdWithConfig(fileSvcFactoryFor(fileSvc))
			require.NoError(t, cmd.Flags().Set("catalog", catPath))
			require.NoError(t, cmd.Flags().Set("class", certClass))

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			err := cmd.RunE(cmd, nil)
			require.NoError(t, err)
			assert.Contains(t, buf.String(), "Class "+certClass)
		})
	}
}

// TestComplianceKSICmd_ValidationErrors asserts validation failures for the ksi subcommand.
func TestComplianceKSICmd_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, fileSvc fs.RuntimeFileService) (catalogPath, certClass string)
		expectedErr error
		errContains string
	}{
		{
			name: "invalid certification class",
			setup: func(t *testing.T, fileSvc fs.RuntimeFileService) (string, string) {
				catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)
				return catPath, "INVALID"
			},
			expectedErr: constants.ErrValidationFailed,
		},
		{
			name: "catalog file not found",
			setup: func(t *testing.T, fileSvc fs.RuntimeFileService) (string, string) {
				return constants.TestPathNonexistentCatalog, "C"
			},
			errContains: "load KSI catalog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSvc, _ := newCmdTestEnv(t)
			catPath, certClass := tt.setup(t, fileSvc)

			cmd := complianceKSICmdWithConfig(fileSvcFactoryFor(fileSvc))
			require.NoError(t, cmd.Flags().Set("catalog", catPath))
			require.NoError(t, cmd.Flags().Set("class", certClass))

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			err := cmd.RunE(cmd, nil)
			require.Error(t, err)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			}
			if tt.errContains != "" {
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

// TestComplianceKSICmd_StoresUnavailable asserts that ksi subcommand fails gracefully
// when storage dependencies are unavailable.
func TestComplianceKSICmd_StoresUnavailable(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)

	// Block data directory with a plain file so SQLite / audit store cannot open.
	dataDir := fileSvc.Resolve(constants.DataDirname)
	require.NoError(t, os.RemoveAll(dataDir))
	require.NoError(t, os.WriteFile(dataDir, []byte("blocking-file"), constants.PermFileReadOnly))

	cmd := complianceKSICmdWithConfig(fileSvcFactoryFor(fileSvc))
	require.NoError(t, cmd.Flags().Set("catalog", catPath))

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	runErr := cmd.RunE(cmd, nil)
	require.Error(t, runErr)
	assert.ErrorIs(t, runErr, constants.ErrReportStoreUnavailable)
}

// TestComplianceKSICmd_Success_OutputsJSON asserts that compliance ksi evaluates
// the live state, outputs a valid KSIResultSet JSON, and saves history.
func TestComplianceKSICmd_Success_OutputsJSON(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)

	cmd := complianceKSICmdWithConfig(fileSvcFactoryFor(fileSvc))
	require.NoError(t, cmd.Flags().Set("catalog", catPath))
	require.NoError(t, cmd.Flags().Set("class", "C"))

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	var resultSet compliance.KSIResultSet
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resultSet))
	assert.Equal(t, compliance.ClassC, resultSet.Class)
	assert.NotEmpty(t, resultSet.Results)
	assert.Equal(t, 2, resultSet.NotSatisfiedCount())

	// Verify history snapshot saved.
	historyStore := newKSIHistoryStore(fileSvc)
	snapshots, err := historyStore.ListSnapshots(context.Background())
	require.NoError(t, err)
	assert.Len(t, snapshots, 1)
}

// TestComplianceKSIHistoryCmd_EmptyHistory asserts that reading history when no
// snapshots exist prints an empty JSON array.
func TestComplianceKSIHistoryCmd_EmptyHistory(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)

	cmd := complianceKSIHistoryCmdWithConfig(fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, "[]\n", buf.String())
}

// TestComplianceKSIHistoryCmd_ListSnapshots asserts that all persisted history
// snapshots are listed in chronological order.
func TestComplianceKSIHistoryCmd_ListSnapshots(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()
	historyStore := newKSIHistoryStore(fileSvc)

	rs1 := &compliance.KSIResultSet{
		Class:         compliance.ClassC,
		EvaluatedAtMs: time.Now().Add(-1 * time.Hour).UnixMilli(),
		Results: []compliance.KSIResult{
			{ID: "KSI-CMT-01", Status: compliance.KSIStatusSatisfied},
		},
	}
	require.NoError(t, saveKSIHistorySnapshot(ctx, historyStore, rs1))

	rs2 := &compliance.KSIResultSet{
		Class:         compliance.ClassC,
		EvaluatedAtMs: time.Now().UnixMilli(),
		Results: []compliance.KSIResult{
			{ID: "KSI-CMT-01", Status: compliance.KSIStatusSatisfied},
			{ID: "KSI-MLA-03", Status: compliance.KSIStatusSatisfied},
		},
	}
	require.NoError(t, saveKSIHistorySnapshot(ctx, historyStore, rs2))

	cmd := complianceKSIHistoryCmdWithConfig(fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	var snapshots []compliance.KSIResultSet
	require.NoError(t, json.Unmarshal(buf.Bytes(), &snapshots))
	assert.Len(t, snapshots, 2)
}

// TestComplianceKSIHistoryCmd_FilterByKSI asserts filtering history by a specific KSI ID.
func TestComplianceKSIHistoryCmd_FilterByKSI(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()
	historyStore := newKSIHistoryStore(fileSvc)

	rs := &compliance.KSIResultSet{
		Class:         compliance.ClassC,
		EvaluatedAtMs: time.Now().UnixMilli(),
		Results: []compliance.KSIResult{
			{ID: "KSI-CMT-01", Status: compliance.KSIStatusSatisfied, MethodCount: 2},
		},
	}
	require.NoError(t, saveKSIHistorySnapshot(ctx, historyStore, rs))

	t.Run("existing KSI ID", func(t *testing.T) {
		cmd := complianceKSIHistoryCmdWithConfig(fileSvcFactoryFor(fileSvc))
		require.NoError(t, cmd.Flags().Set("ksi", "KSI-CMT-01"))

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, nil)
		require.NoError(t, err)

		var results []compliance.KSIResult
		require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
		require.Len(t, results, 1)
		assert.Equal(t, "KSI-CMT-01", results[0].ID)
	})

	t.Run("nonexistent KSI ID returns error", func(t *testing.T) {
		cmd := complianceKSIHistoryCmdWithConfig(fileSvcFactoryFor(fileSvc))
		require.NoError(t, cmd.Flags().Set("ksi", "KSI-NONEXISTENT"))

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrKSIHistoryEmpty)
	})
}

// TestComplianceOverlayCmd_ValidationAndErrors asserts error handling for overlay commands.
func TestComplianceOverlayCmd_ValidationAndErrors(t *testing.T) {
	t.Run("nonexistent overlay directory", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)
		cmd := complianceOverlayCmdWithConfig(fileSvcFactoryFor(fileSvc))
		require.NoError(t, cmd.Flags().Set("overlay-dir", constants.TestPathNonexistentOverlayDir))

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "load overlays")
	})

	t.Run("invalid catalog path with valid overlays", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)
		overlayDir := fileSvc.Resolve(constants.TestOverlaysDirname)
		writeTestOverlays(t, overlayDir, &compliance.OverlayCatalog{
			Version: "1.0",
			Source:  "test",
			Overlays: []compliance.Overlay{
				{ID: "COSAiS-TEST-01", Title: "Test Overlay", UseCase: "test_use_case", Status: compliance.OverlayStatusDraft, ControlRefs: []string{"CM-3"}},
			},
		})

		cmd := complianceOverlayCmdWithConfig(fileSvcFactoryFor(fileSvc))
		require.NoError(t, cmd.Flags().Set("overlay-dir", overlayDir))
		require.NoError(t, cmd.Flags().Set("catalog", constants.TestPathNonexistentCatalog))

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "load KSI catalog")
	})
}

// TestComplianceOverlayCmd_EmptyOverlayDir asserts that an empty overlay directory outputs "[]".
func TestComplianceOverlayCmd_EmptyOverlayDir(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	emptyDir := fileSvc.Resolve(constants.TestOverlaysDirname)
	require.NoError(t, os.MkdirAll(emptyDir, constants.PermDirStandard))

	cmd := complianceOverlayCmdWithConfig(fileSvcFactoryFor(fileSvc))
	require.NoError(t, cmd.Flags().Set("overlay-dir", emptyDir))

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, "[]\n", buf.String())
}

// TestComplianceOverlayCmd_Success_NoDanglingRefs asserts that overlay validation passes
// without warnings when all overlay control references match KSIs in the catalog.
func TestComplianceOverlayCmd_Success_NoDanglingRefs(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)
	overlayDir := fileSvc.Resolve(constants.TestOverlaysDirname)

	writeTestOverlays(t, overlayDir, &compliance.OverlayCatalog{
		Version: "1.0",
		Source:  "test",
		Overlays: []compliance.Overlay{
			{ID: "COSAiS-TEST-01", Title: "Test Overlay", UseCase: "test_use_case", Status: compliance.OverlayStatusDraft, ControlRefs: []string{"CM-3"}},
		},
	})

	cmd := complianceOverlayCmdWithConfig(fileSvcFactoryFor(fileSvc))
	require.NoError(t, cmd.Flags().Set("overlay-dir", overlayDir))
	require.NoError(t, cmd.Flags().Set("catalog", catPath))

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	outStr := buf.String()
	assert.NotContains(t, outStr, "WARNING")

	var cat compliance.OverlayCatalog
	require.NoError(t, json.Unmarshal(buf.Bytes(), &cat))
	assert.Equal(t, "1.0", cat.Version)
	require.Len(t, cat.Overlays, 1)
}

// TestComplianceOverlayCmd_Success_WithDanglingRefs asserts that dangling overlay references
// are reported as warnings while still outputting the overlay catalog JSON.
func TestComplianceOverlayCmd_Success_WithDanglingRefs(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)

	// Write KSI catalog with a dangling overlay ref.
	danglingKSICat := &compliance.KSICatalog{
		Version: "CR26-TEST",
		Source:  "test-source",
		KSIs: []compliance.KSI{
			{
				ID:          "KSI-CMT-01",
				Title:       "Test KSI with Dangling Overlay",
				Category:    compliance.KSICategoryCMT,
				OverlayRefs: []string{"COSAiS-NONEXISTENT-99"},
			},
		},
	}
	catData, err := json.MarshalIndent(danglingKSICat, "", "  ")
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), constants.KSICatalogFilename, catData, constants.PermFilePublic))
	catPath := fileSvc.Resolve(constants.KSICatalogFilename)

	overlayDir := fileSvc.Resolve(constants.TestOverlaysDirname)
	writeTestOverlays(t, overlayDir, &compliance.OverlayCatalog{
		Version: "1.0",
		Source:  "test",
		Overlays: []compliance.Overlay{
			{ID: "COSAiS-TEST-01", Title: "Valid Overlay", UseCase: "test_use_case", Status: compliance.OverlayStatusDraft, ControlRefs: []string{"CM-3"}},
		},
	})

	cmd := complianceOverlayCmdWithConfig(fileSvcFactoryFor(fileSvc))
	require.NoError(t, cmd.Flags().Set("overlay-dir", overlayDir))
	require.NoError(t, cmd.Flags().Set("catalog", catPath))

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, nil)
	require.NoError(t, err)

	outStr := buf.String()
	assert.Contains(t, outStr, "WARNING: 1 dangling overlay reference(s):")
	assert.Contains(t, outStr, "KSI-CMT-01 -> COSAiS-NONEXISTENT-99")
}

// TestOpenVault_Scenarios tests openVault behavior across various key states.
func TestOpenVault_Scenarios(t *testing.T) {
	t.Run("missing vault key returns locked vault", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)
		v, cleanup := openVault(context.Background(), fileSvc)
		defer cleanup()
		require.NotNil(t, v)
		assert.False(t, v.IsUnlocked())
	})

	t.Run("invalid hex key returns locked vault", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)
		vaultKeyRel := constants.SecretsDirname + "/" + constants.VaultKeyFilename
		require.NoError(t, fileSvc.WriteFile(context.Background(), vaultKeyRel, []byte("not-valid-hex!"), constants.PermFilePrivate))

		v, cleanup := openVault(context.Background(), fileSvc)
		defer cleanup()
		require.NotNil(t, v)
		assert.False(t, v.IsUnlocked())
	})

	t.Run("valid key and header unlocks vault", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)
		_, privKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		setupTestVaultWithKey(t, fileSvc, privKey)

		v, cleanup := openVault(context.Background(), fileSvc)
		defer cleanup()
		require.NotNil(t, v)
		assert.True(t, v.IsUnlocked())
	})

	t.Run("key mismatch with vault header returns locked vault", func(t *testing.T) {
		fileSvc, _ := newCmdTestEnv(t)
		_, privKey1, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		_, privKey2, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		// Setup vault header with privKey1, but write privKey2 to secrets.
		setupTestVaultWithKey(t, fileSvc, privKey1)
		vaultKeyRel := constants.SecretsDirname + "/" + constants.VaultKeyFilename
		hexKey2 := hex.EncodeToString(privKey2)
		require.NoError(t, fileSvc.WriteFile(context.Background(), vaultKeyRel, []byte(hexKey2), constants.PermFilePrivate))

		v, cleanup := openVault(context.Background(), fileSvc)
		defer cleanup()
		require.NotNil(t, v)
		assert.False(t, v.IsUnlocked())
	})
}

// TestRunCleanups_LifoOrder asserts that cleanups run in last-in, first-out order.
func TestRunCleanups_LifoOrder(t *testing.T) {
	var executionOrder []int

	cleanups := []func(){
		func() { executionOrder = append(executionOrder, 1) },
		func() { executionOrder = append(executionOrder, 2) },
		func() { executionOrder = append(executionOrder, 3) },
	}

	runCleanups(cleanups)

	assert.Equal(t, []int{3, 2, 1}, executionOrder)
}

// TestSaveKSIHistorySnapshot_Pruning asserts that saving a snapshot triggers pruning.
func TestSaveKSIHistorySnapshot_Pruning(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()
	historyStore := newKSIHistoryStore(fileSvc)

	rs := &compliance.KSIResultSet{
		Class:         compliance.ClassC,
		EvaluatedAtMs: time.Now().UnixMilli(),
		Results: []compliance.KSIResult{
			{ID: "KSI-CMT-01", Status: compliance.KSIStatusSatisfied},
		},
	}

	err := saveKSIHistorySnapshot(ctx, historyStore, rs)
	require.NoError(t, err)

	snapshots, err := historyStore.ListSnapshots(ctx)
	require.NoError(t, err)
	assert.Len(t, snapshots, 1)
}

// TestEvaluateKSIs_NilContextHandling asserts that evaluateKSIs handles nil context safely.
func TestEvaluateKSIs_NilContextHandling(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	catPath := writeTestKSICatalog(t, fileSvc, constants.KSICatalogFilename)
	cat, err := compliance.LoadKSICatalog(catPath)
	require.NoError(t, err)

	// evaluateKSIs with nil context should not panic.
	var nilCtx context.Context
	resultSet := evaluateKSIs(nilCtx, fileSvc, cat, compliance.ClassC)
	require.NotNil(t, resultSet)
	assert.Equal(t, compliance.ClassC, resultSet.Class)
	assert.Equal(t, 2, resultSet.NotSatisfiedCount())
}

// TestSaveKSIHistorySnapshot_NilResultSetReturnsError asserts error when saving nil result set.
func TestSaveKSIHistorySnapshot_NilResultSetReturnsError(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	historyStore := newKSIHistoryStore(fileSvc)

	err := saveKSIHistorySnapshot(context.Background(), historyStore, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKSIHistoryWriteFailed)
}

// TestOpenEvaluatorDeps_DatabaseError asserts that openEvaluatorDeps fails and cleans up
// when data directory cannot be initialized.
func TestOpenEvaluatorDeps_DatabaseError(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	dataDir := fileSvc.Resolve(constants.DataDirname)
	require.NoError(t, os.RemoveAll(dataDir))
	require.NoError(t, os.WriteFile(dataDir, []byte("blocking-file"), constants.PermFileReadOnly))

	deps, cleanup, ok := openEvaluatorDeps(context.Background(), fileSvc)
	assert.False(t, ok)
	assert.Nil(t, cleanup)
	assert.Nil(t, deps.Audit)
}
