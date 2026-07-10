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
// See the License for the specific language and governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestRunDemosStatus_WithValidDemoDirButNoDocker(t *testing.T) {
	tmpDir := chdirTemp(t)

	orgDir := filepath.Join(tmpDir, constants.DemosDirname, "myorg")
	require.NoError(t, os.MkdirAll(orgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(orgDir, constants.DemosComposeFile), []byte("version: '3'\n"), 0o644))

	cmd := demosStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"myorg"})
	require.Error(t, err)
}

func TestRunDemosRebuild_WithValidDemoDirButNoDocker(t *testing.T) {
	tmpDir := chdirTemp(t)

	orgDir := filepath.Join(tmpDir, constants.DemosDirname, "myorg")
	require.NoError(t, os.MkdirAll(orgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(orgDir, constants.DemosComposeFile), []byte("version: '3'\n"), 0o644))

	cmd := demosRebuildCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"myorg"})
	require.Error(t, err)
}

func TestRunDemosStart_WithValidDemoDirButNoDocker(t *testing.T) {
	tmpDir := chdirTemp(t)

	orgDir := filepath.Join(tmpDir, constants.DemosDirname, "myorg")
	require.NoError(t, os.MkdirAll(orgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(orgDir, constants.DemosComposeFile), []byte("version: '3'\n"), 0o644))

	cmd := demosStartCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"myorg"})
	require.Error(t, err)
}

func TestRunDemosStop_WithValidDemoDirButNoDocker(t *testing.T) {
	tmpDir := chdirTemp(t)

	orgDir := filepath.Join(tmpDir, constants.DemosDirname, "myorg")
	require.NoError(t, os.MkdirAll(orgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(orgDir, constants.DemosComposeFile), []byte("version: '3'\n"), 0o644))

	cmd := demosStopCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"myorg"})
	require.Error(t, err)
}

func TestRunDemosClean_AllDemosWithDirsButNoDocker(t *testing.T) {
	tmpDir := chdirTemp(t)

	demosDir := filepath.Join(tmpDir, constants.DemosDirname)
	require.NoError(t, os.MkdirAll(demosDir, 0o755))

	orgDir := filepath.Join(demosDir, "myorg")
	require.NoError(t, os.MkdirAll(orgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(orgDir, constants.DemosComposeFile), []byte("version: '3'\n"), 0o644))

	cmd := demosCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	assert.NoError(t, err, "clean all demos should succeed when Docker is available (no-op on stopped containers)")
}

func TestRunDemosClean_SingleOrgWithValidDirButNoDocker(t *testing.T) {
	tmpDir := chdirTemp(t)

	orgDir := filepath.Join(tmpDir, constants.DemosDirname, "myorg")
	require.NoError(t, os.MkdirAll(orgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(orgDir, constants.DemosComposeFile), []byte("version: '3'\n"), 0o644))

	cmd := demosCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"myorg"})
	assert.NoError(t, err, "clean single demo should succeed when Docker is available (no-op on stopped containers)")
}

func TestRunDemosReset_WithValidDemoDirButNoDocker(t *testing.T) {
	tmpDir := chdirTemp(t)

	orgDir := filepath.Join(tmpDir, constants.DemosDirname, "myorg")
	require.NoError(t, os.MkdirAll(orgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(orgDir, constants.DemosComposeFile), []byte("version: '3'\n"), 0o644))

	cmd := demosResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"myorg"})
	require.Error(t, err)
}

func TestRunDemosRun_WithValidDemoDirButNoDocker(t *testing.T) {
	tmpDir := chdirTemp(t)

	orgDir := filepath.Join(tmpDir, constants.DemosDirname, "myorg")
	require.NoError(t, os.MkdirAll(orgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(orgDir, constants.DemosComposeFile), []byte("version: '3'\n"), 0o644))

	cmd := demosRunCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"myorg"})
	require.Error(t, err)
}

func TestRunDemosPull_WithValidManifestButNoDocker(t *testing.T) {
	tmpDir := chdirTemp(t)

	demosDir := filepath.Join(tmpDir, constants.DemosDirname)
	require.NoError(t, os.MkdirAll(demosDir, 0o755))
	manifest := `[{"image":"alpine","tag":"latest","digest":"sha256:abc","demos":["gov"]}]`
	require.NoError(t, os.WriteFile(filepath.Join(demosDir, constants.DemosImagesManifestFile), []byte(manifest), 0o644))

	cmd := demosPullCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestRunDemosExport_WithValidManifestWithEntriesCreatesDir(t *testing.T) {
	tmpDir := chdirTemp(t)

	demosDir := filepath.Join(tmpDir, constants.DemosDirname)
	require.NoError(t, os.MkdirAll(demosDir, 0o755))
	manifest := `[{"image":"alpine","tag":"latest","digest":"sha256:abc","demos":["gov"]}]`
	require.NoError(t, os.WriteFile(filepath.Join(demosDir, constants.DemosImagesManifestFile), []byte(manifest), 0o644))

	cmd := demosExportCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err, "docker save should fail for non-existent image")

	exportDir := filepath.Join(demosDir, "images-export")
	info, statErr := os.Stat(exportDir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}
