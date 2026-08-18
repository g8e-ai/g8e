// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// os.Chdir is used for source-tree demo discovery (finding ./demos/ directories,
// compose.yml, doctrine/, target-data/), not .g8e/ runtime state. This is a
// legitimate cwd usage — demo commands resolve paths relative to the working
// directory, not through RuntimeFileService.

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// chdirTemp creates a temp dir, chdirs into it, and restores the original
// working directory on cleanup.
//
// WARNING: This function mutates process-global state via os.Chdir.
// Tests using chdirTemp must NOT call t.Parallel(), and no other test in
// this package should chdir concurrently. The functions under test rely on
// os.Getwd(), so chdir is unavoidable.
func chdirTemp(t *testing.T) string {
	t.Helper()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := testutil.TempDir(t)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { os.Chdir(originalWd) })
	return tmpDir
}

func TestRunDemosList_MissingDemosDirReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := demosListCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDirectoryRead)
}

func TestRunDemosList_WithValidDemosDir(t *testing.T) {
	tmpDir := chdirTemp(t)

	demosDir := filepath.Join(tmpDir, constants.DemosDirname)
	require.NoError(t, os.MkdirAll(demosDir, 0o755))

	for _, org := range []string{"org-a", "org-b"} {
		orgDir := filepath.Join(demosDir, org)
		require.NoError(t, os.MkdirAll(orgDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(orgDir, constants.DemosComposeFile), []byte("version: '3'\n"), 0o644))
	}

	binDir := filepath.Join(demosDir, constants.DemosBinDirname)
	require.NoError(t, os.MkdirAll(binDir, 0o755))

	var buf bytes.Buffer
	cmd := demosListCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)

	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "org-a")
	assert.Contains(t, output, "org-b")
	assert.NotContains(t, output, constants.DemosBinDirname)
}

func TestRunDemosPull_MissingManifestReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := demosPullCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "images manifest")
}

func TestRunDemosPull_InvalidJSONReturnsError(t *testing.T) {
	tmpDir := chdirTemp(t)

	demosDir := filepath.Join(tmpDir, constants.DemosDirname)
	require.NoError(t, os.MkdirAll(demosDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(demosDir, constants.DemosImagesManifestFile), []byte("{invalid json"), 0o644))

	cmd := demosPullCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing images manifest")
}

func TestRunDemosExport_MissingManifestReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := demosExportCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "images manifest")
}

func TestRunDemosExport_InvalidJSONReturnsError(t *testing.T) {
	tmpDir := chdirTemp(t)

	demosDir := filepath.Join(tmpDir, constants.DemosDirname)
	require.NoError(t, os.MkdirAll(demosDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(demosDir, constants.DemosImagesManifestFile), []byte("not json"), 0o644))

	cmd := demosExportCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing images manifest")
}

func TestRunDemosExport_CreatesOutputDir(t *testing.T) {
	tmpDir := chdirTemp(t)

	demosDir := filepath.Join(tmpDir, constants.DemosDirname)
	require.NoError(t, os.MkdirAll(demosDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(demosDir, constants.DemosImagesManifestFile), []byte("[]"), 0o644))

	cmd := demosExportCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	exportDir := filepath.Join(demosDir, "images-export")
	info, err := os.Stat(exportDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestRunDemosImport_MissingDirReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := demosImportCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"/nonexistent/import/dir"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPathNotFound)
}

func TestRunDemosImport_EmptyDirReturnsNotFound(t *testing.T) {
	tmpDir := chdirTemp(t)

	importDir := filepath.Join(tmpDir, "empty-import")
	require.NoError(t, os.MkdirAll(importDir, 0o755))

	cmd := demosImportCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{importDir})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
	assert.Contains(t, err.Error(), "no .tar files")
}

func TestRunDemosImages_MissingManifestReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := demosImagesCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "images manifest")
}

func TestRunDemosImages_ValidManifestListsImages(t *testing.T) {
	tmpDir := chdirTemp(t)

	demosDir := filepath.Join(tmpDir, constants.DemosDirname)
	require.NoError(t, os.MkdirAll(demosDir, 0o755))
	manifest := `[{"image":"alpine","tag":"latest","digest":"sha256:abc","demos":["finance","dhs"]}]`
	require.NoError(t, os.WriteFile(filepath.Join(demosDir, constants.DemosImagesManifestFile), []byte(manifest), 0o644))

	cmd := demosImagesCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "alpine@sha256:abc")
	assert.Contains(t, output, "finance, dhs")
}

func TestRunDemosStart_MissingDemoDirReturnsNotFound(t *testing.T) {
	chdirTemp(t)

	cmd := demosStartCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"nonexistent-org"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
	assert.Contains(t, err.Error(), "nonexistent-org")
}

func TestRunDemosStart_MissingComposeFileReturnsNotFound(t *testing.T) {
	tmpDir := chdirTemp(t)

	orgDir := filepath.Join(tmpDir, constants.DemosDirname, "myorg")
	require.NoError(t, os.MkdirAll(orgDir, 0o755))

	cmd := demosStartCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"myorg"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
	assert.Contains(t, err.Error(), "compose.yml")
}

func TestRunDemosStop_MissingDemoDirReturnsNotFound(t *testing.T) {
	chdirTemp(t)

	cmd := demosStopCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"nonexistent-org"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestRunDemosStop_MissingComposeFileReturnsNotFound(t *testing.T) {
	tmpDir := chdirTemp(t)

	orgDir := filepath.Join(tmpDir, constants.DemosDirname, "myorg")
	require.NoError(t, os.MkdirAll(orgDir, 0o755))

	cmd := demosStopCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"myorg"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
	assert.Contains(t, err.Error(), "compose.yml")
}

func TestRunDemosStatus_MissingDemoDirReturnsNotFound(t *testing.T) {
	chdirTemp(t)

	cmd := demosStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"nonexistent-org"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestRunDemosRebuild_MissingDemoDirReturnsNotFound(t *testing.T) {
	chdirTemp(t)

	cmd := demosRebuildCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"nonexistent-org"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestRunDemosClean_SingleOrgMissingDirReturnsNotFound(t *testing.T) {
	chdirTemp(t)

	cmd := demosCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"nonexistent-org"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestRunDemosClean_AllDemosNoDirsFound(t *testing.T) {
	tmpDir := chdirTemp(t)

	demosDir := filepath.Join(tmpDir, constants.DemosDirname)
	require.NoError(t, os.MkdirAll(demosDir, 0o755))

	cmd := demosCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No demo environments found")
}

func TestRunDemosReset_MissingDemoDirReturnsNotFound(t *testing.T) {
	chdirTemp(t)

	cmd := demosResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"nonexistent-org"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestRunDemosRun_MissingDemoDirReturnsNotFound(t *testing.T) {
	chdirTemp(t)

	cmd := demosRunCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"nonexistent-org"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestRunDemosRun_MissingComposeFileReturnsNotFound(t *testing.T) {
	tmpDir := chdirTemp(t)

	orgDir := filepath.Join(tmpDir, constants.DemosDirname, "myorg")
	require.NoError(t, os.MkdirAll(orgDir, 0o755))

	cmd := demosRunCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"myorg"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
	assert.Contains(t, err.Error(), "compose.yml")
}
