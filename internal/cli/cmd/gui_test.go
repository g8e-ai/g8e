// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestGuiCmdRegistration(t *testing.T) {
	cmd := guiCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "gui", cmd.Use)
	assert.Contains(t, cmd.Short, "frontend")
	assert.Len(t, cmd.Commands(), 4)
}

func TestValidateOrigin(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		wantErr bool
	}{
		{"valid https", "https://my-app.lovable.app", false},
		{"valid http", "http://localhost:3003", false},
		{"valid http with path", "http://localhost:3003/app", false},
		{"empty string", "", true},
		{"ftp scheme", "ftp://example.com", true},
		{"no scheme", "example.com", true},
		{"no host", "https:///", true},
		{"ws scheme", "ws://localhost:3003", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrigin(tt.origin)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, constants.ErrValidationFailed)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLoadGUIEnrollment_FileNotFound(t *testing.T) {
	enrollment, err := loadGUIEnrollment("/nonexistent/path/gui_enrollments.json")
	require.NoError(t, err)
	assert.NotNil(t, enrollment)
	assert.Empty(t, enrollment.Origins)
}

func TestLoadGUIEnrollment_ValidFile(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	path := filepath.Join(tmpDir, GUIEnrollmentFile)
	require.NoError(t, os.WriteFile(path, []byte(`{"origins":["https://app1.example.com","http://localhost:3003"]}`), 0600))

	enrollment, err := loadGUIEnrollment(path)
	require.NoError(t, err)
	assert.Len(t, enrollment.Origins, 2)
	assert.Equal(t, "https://app1.example.com", enrollment.Origins[0])
	assert.Equal(t, "http://localhost:3003", enrollment.Origins[1])
}

func TestLoadGUIEnrollment_InvalidJSON(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	path := filepath.Join(tmpDir, GUIEnrollmentFile)
	require.NoError(t, os.WriteFile(path, []byte(`{invalid json`), 0600))

	_, err := loadGUIEnrollment(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse enrollment file")
}

func TestSaveGUIEnrollment_RoundTrip(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	path := filepath.Join(tmpDir, "subdir", GUIEnrollmentFile)

	enrollment := &GUIEnrollment{
		Origins: []string{"https://app1.example.com", "http://localhost:3003"},
	}
	require.NoError(t, saveGUIEnrollment(path, enrollment))

	loaded, err := loadGUIEnrollment(path)
	require.NoError(t, err)
	assert.Equal(t, enrollment.Origins, loaded.Origins)
}

func TestSaveGUIEnrollment_CreatesParentDir(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	path := filepath.Join(tmpDir, "nested", "deep", GUIEnrollmentFile)

	require.NoError(t, saveGUIEnrollment(path, &GUIEnrollment{Origins: []string{}}))
	_, err := os.Stat(path)
	require.NoError(t, err)
}

func TestGuiEnrollCmdWithDeps_CreatesEnrollment(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	require.NoError(t, os.MkdirAll(runtimeDir, 0700))

	cfg := &config.Config{
		ProjectRoot: tmpDir,
		RuntimeDir:  runtimeDir,
	}
	loader := func(string) (*config.Config, error) { return cfg, nil }
	corsChecker := func(*config.Config, string) error { return nil }

	cmd := guiEnrollCmdWithDeps(loader, corsChecker)
	cmd.SetArgs([]string{"--origin", "https://my-app.lovable.app"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())

	enrollmentPath := filepath.Join(runtimeDir, GUIEnrollmentFile)
	loaded, err := loadGUIEnrollment(enrollmentPath)
	require.NoError(t, err)
	assert.Contains(t, loaded.Origins, "https://my-app.lovable.app")
}

func TestGuiEnrollCmdWithDeps_DuplicateOriginIsIdempotent(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	require.NoError(t, os.MkdirAll(runtimeDir, 0700))

	enrollmentPath := filepath.Join(runtimeDir, GUIEnrollmentFile)
	require.NoError(t, saveGUIEnrollment(enrollmentPath, &GUIEnrollment{
		Origins: []string{"https://my-app.lovable.app"},
	}))

	cfg := &config.Config{
		ProjectRoot: tmpDir,
		RuntimeDir:  runtimeDir,
	}
	loader := func(string) (*config.Config, error) { return cfg, nil }
	corsChecker := func(*config.Config, string) error { return nil }

	cmd := guiEnrollCmdWithDeps(loader, corsChecker)
	cmd.SetArgs([]string{"--origin", "https://my-app.lovable.app"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())

	loaded, err := loadGUIEnrollment(enrollmentPath)
	require.NoError(t, err)
	assert.Len(t, loaded.Origins, 1, "duplicate origin should not be added twice")
}

func TestGuiEnrollCmdWithDeps_MissingOriginReturnsError(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: filepath.Join(tmpDir, ".g8e")}
	loader := func(string) (*config.Config, error) { return cfg, nil }
	corsChecker := func(*config.Config, string) error { return nil }

	cmd := guiEnrollCmdWithDeps(loader, corsChecker)
	cmd.SetArgs([]string{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestGuiEnrollCmdWithDeps_InvalidOriginReturnsError(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: filepath.Join(tmpDir, ".g8e")}
	loader := func(string) (*config.Config, error) { return cfg, nil }
	corsChecker := func(*config.Config, string) error { return nil }

	cmd := guiEnrollCmdWithDeps(loader, corsChecker)
	cmd.SetArgs([]string{"--origin", "ftp://bad-origin"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestGuiEnrollCmdWithDeps_ConfigLoadError(t *testing.T) {
	failLoader := func(string) (*config.Config, error) {
		return nil, errors.New("config load error")
	}
	corsChecker := func(*config.Config, string) error { return nil }

	cmd := guiEnrollCmdWithDeps(failLoader, corsChecker)
	cmd.SetArgs([]string{"--origin", "https://example.com"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}

func TestGuiShowCmdWithDeps_NoEnrollments(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: filepath.Join(tmpDir, ".g8e")}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiShowCmdWithDeps(loader)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.RunE(cmd, nil))
	output := buf.String()
	assert.Contains(t, output, "No frontend applications enrolled")
}

func TestGuiShowCmdWithDeps_ListsEnrolledOrigins(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	require.NoError(t, os.MkdirAll(runtimeDir, 0700))

	enrollmentPath := filepath.Join(runtimeDir, GUIEnrollmentFile)
	require.NoError(t, saveGUIEnrollment(enrollmentPath, &GUIEnrollment{
		Origins: []string{"https://app1.lovable.app", "http://localhost:3003"},
	}))

	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: runtimeDir}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiShowCmdWithDeps(loader)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.RunE(cmd, nil))
	output := buf.String()
	assert.Contains(t, output, "https://app1.lovable.app")
	assert.Contains(t, output, "http://localhost:3003")
	assert.Contains(t, output, "Enrolled Frontend Applications")
}

func TestGuiVerifyCmdWithDeps_PrintsVerificationChecklist(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	require.NoError(t, os.MkdirAll(runtimeDir, 0700))

	enrollmentPath := filepath.Join(runtimeDir, GUIEnrollmentFile)
	require.NoError(t, saveGUIEnrollment(enrollmentPath, &GUIEnrollment{
		Origins: []string{"http://localhost:3003"},
	}))

	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: runtimeDir}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiVerifyCmdWithDeps(loader)
	cmd.SetArgs([]string{"--origin", "http://localhost:3003"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())
	output := buf.String()
	assert.Contains(t, output, "Verification Results")
	assert.Contains(t, output, "http://localhost:3003")
	assert.Contains(t, output, "Enrolled:   true")
	assert.Contains(t, output, "CORS headers present")
	assert.Contains(t, output, "SSE stream connects")
}

func TestGuiVerifyCmdWithDeps_UnenrolledOriginWarns(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: filepath.Join(tmpDir, ".g8e")}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiVerifyCmdWithDeps(loader)
	cmd.SetArgs([]string{"--origin", "http://localhost:3003"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())
	output := buf.String()
	assert.Contains(t, output, "not in the enrollment list")
}

func TestGuiVerifyCmdWithDeps_MissingOriginReturnsError(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: filepath.Join(tmpDir, ".g8e")}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiVerifyCmdWithDeps(loader)
	cmd.SetArgs([]string{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestGuiRemoveCmdWithDeps_RemovesEnrolledOrigin(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	require.NoError(t, os.MkdirAll(runtimeDir, 0700))

	enrollmentPath := filepath.Join(runtimeDir, GUIEnrollmentFile)
	require.NoError(t, saveGUIEnrollment(enrollmentPath, &GUIEnrollment{
		Origins: []string{"https://app1.lovable.app", "http://localhost:3003"},
	}))

	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: runtimeDir}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiRemoveCmdWithDeps(loader)
	cmd.SetArgs([]string{"--origin", "https://app1.lovable.app"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())

	loaded, err := loadGUIEnrollment(enrollmentPath)
	require.NoError(t, err)
	assert.Len(t, loaded.Origins, 1)
	assert.Equal(t, "http://localhost:3003", loaded.Origins[0])
}

func TestGuiRemoveCmdWithDeps_RemovesLastOrigin(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	require.NoError(t, os.MkdirAll(runtimeDir, 0700))

	enrollmentPath := filepath.Join(runtimeDir, GUIEnrollmentFile)
	require.NoError(t, saveGUIEnrollment(enrollmentPath, &GUIEnrollment{
		Origins: []string{"https://app1.lovable.app"},
	}))

	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: runtimeDir}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiRemoveCmdWithDeps(loader)
	cmd.SetArgs([]string{"--origin", "https://app1.lovable.app"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())

	loaded, err := loadGUIEnrollment(enrollmentPath)
	require.NoError(t, err)
	assert.Empty(t, loaded.Origins)
}

func TestGuiRemoveCmdWithDeps_NotEnrolledReturnsNotFound(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	require.NoError(t, os.MkdirAll(runtimeDir, 0700))

	enrollmentPath := filepath.Join(runtimeDir, GUIEnrollmentFile)
	require.NoError(t, saveGUIEnrollment(enrollmentPath, &GUIEnrollment{
		Origins: []string{"https://app1.lovable.app"},
	}))

	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: runtimeDir}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiRemoveCmdWithDeps(loader)
	cmd.SetArgs([]string{"--origin", "http://localhost:9999"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestGuiRemoveCmdWithDeps_MissingOriginReturnsError(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: filepath.Join(tmpDir, ".g8e")}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiRemoveCmdWithDeps(loader)
	cmd.SetArgs([]string{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestGuiRemoveCmdWithDeps_InvalidOriginReturnsError(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: filepath.Join(tmpDir, ".g8e")}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiRemoveCmdWithDeps(loader)
	cmd.SetArgs([]string{"--origin", "ftp://bad-origin"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestGuiRemoveCmdWithDeps_ConfigLoadError(t *testing.T) {
	failLoader := func(string) (*config.Config, error) {
		return nil, errors.New("config load error")
	}

	cmd := guiRemoveCmdWithDeps(failLoader)
	cmd.SetArgs([]string{"--origin", "https://example.com"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}

func TestGuiShowCmdWithDeps_JSONOutput(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	require.NoError(t, os.MkdirAll(runtimeDir, 0700))

	enrollmentPath := filepath.Join(runtimeDir, GUIEnrollmentFile)
	require.NoError(t, saveGUIEnrollment(enrollmentPath, &GUIEnrollment{
		Origins: []string{"https://app1.lovable.app", "http://localhost:3003"},
	}))

	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: runtimeDir}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiShowCmdWithDeps(loader)
	cmd.SetArgs([]string{"--json"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())
	output := buf.String()
	assert.Contains(t, output, `"origins"`)
	assert.Contains(t, output, "https://app1.lovable.app")
	assert.Contains(t, output, "http://localhost:3003")
	assert.NotContains(t, output, "Enrolled Frontend Applications")
}

func TestGuiShowCmdWithDeps_JSONOutputEmpty(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: filepath.Join(tmpDir, ".g8e")}
	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := guiShowCmdWithDeps(loader)
	cmd.SetArgs([]string{"--json"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())
	output := buf.String()
	assert.Contains(t, output, `"origins"`)
	assert.Contains(t, output, "[]")
}

func TestGuiEnrollCmdWithDeps_CORSCheckFailsHard(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	require.NoError(t, os.MkdirAll(runtimeDir, 0700))

	cfg := &config.Config{ProjectRoot: tmpDir, RuntimeDir: runtimeDir}
	loader := func(string) (*config.Config, error) { return cfg, nil }
	corsChecker := func(*config.Config, string) error {
		return fmt.Errorf("gui: gateway is running but does not have origin in its CORS allowed origins")
	}

	cmd := guiEnrollCmdWithDeps(loader, corsChecker)
	cmd.SetArgs([]string{"--origin", "https://my-app.lovable.app"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CORS allowed origins")

	enrollmentPath := filepath.Join(runtimeDir, GUIEnrollmentFile)
	loaded, err := loadGUIEnrollment(enrollmentPath)
	require.NoError(t, err)
	assert.Empty(t, loaded.Origins, "origin should not be saved when CORS check fails")
}

func TestGuiShowCmdHasListAlias(t *testing.T) {
	cmd := guiShowCmd()
	assert.Contains(t, cmd.Aliases, "list")
}
