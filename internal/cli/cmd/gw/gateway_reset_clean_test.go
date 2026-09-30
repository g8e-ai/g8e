// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayCleanCmd_ConfirmYesProceedsPastPrompt(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("y\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Clean complete")
}

func TestGatewayCleanCmd_ConfirmYesUppercaseProceeds(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("Y\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Clean complete")
}

func TestGatewayCleanCmd_AbortsOnNViaCmdSetIn(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("n\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
	assert.NotContains(t, buf.String(), "Clean complete")
}

func TestGatewayCleanCmd_AbortsOnEmptyResponseViaCmdSetIn(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
}

func TestGatewayResetCmd_AbortsOnNViaCmdSetIn(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("n\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
}

func TestGatewayCleanCmd_ForceFlagSkipsPromptViaCmdSetIn(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Clean complete")
}

func TestGatewayCleanCmd_PromptTextContainsWarning(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayCleanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("n\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "WARNING")
	assert.Contains(t, output, "Rename the runtime directory (.g8e) aside to .g8e-<MMDDHHMM>")
	assert.Contains(t, output, "Remove g8e root CA anchors from the OS trust store")
	assert.Contains(t, output, "Continue? [y/N]")
}

func TestGatewayResetCmd_PromptTextContainsResetSteps(t *testing.T) {
	setupGatewayTestEnv(t)

	cmd := gatewayResetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader("n\n"))

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "destructive operation")
	assert.Contains(t, output, "Stop all running g8e services")
	assert.Contains(t, output, "Rename the runtime directory (.g8e) aside to .g8e-<MMDDHHMM>")
	assert.Contains(t, output, "Start a fresh gateway with a new trust domain")
	assert.Contains(t, output, "Continue? [y/N]")
}

// mockTrustCleaner is a test systemTrustCleaner that records calls and
// returns configurable results. It never touches the real OS trust store.
type mockTrustCleaner struct {
	mu              sync.Mutex
	listCalls       int
	lastFingerprint string
	anchors         []platform.StaleAnchor
	listErr         error
	removeCalls     int
	lastRemoved     []platform.StaleAnchor
	removeErr       error
}

func (m *mockTrustCleaner) ListStaleAnchors(_ context.Context, currentFingerprint string) ([]platform.StaleAnchor, error) {
	m.mu.Lock()
	m.listCalls++
	m.lastFingerprint = currentFingerprint
	m.mu.Unlock()
	return m.anchors, m.listErr
}

func (m *mockTrustCleaner) RemoveStaleAnchors(_ context.Context, anchors []platform.StaleAnchor) error {
	m.mu.Lock()
	m.removeCalls++
	m.lastRemoved = anchors
	m.mu.Unlock()
	return m.removeErr
}

// TestGatewayCleanCmd_RemovesOSTrustAnchors verifies that `gw clean`
// calls ListStaleAnchors with an empty keep-fingerprint (list ALL g8e
// anchors) and RemoveStaleAnchors for each, BEFORE pm.Clean() wipes the
// runtime directory.
func TestGatewayCleanCmd_RemovesOSTrustAnchors(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	mock := &mockTrustCleaner{
		anchors: []platform.StaleAnchor{
			{Fingerprint: "stale-fp-1", CommonName: constants.RootCACommonName, Handle: "/path/stale1"},
			{Fingerprint: "stale-fp-2", CommonName: constants.RootCACommonName, Handle: "/path/stale2"},
		},
	}
	cmd := gatewayCleanCmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		cmdtest.FileSvcFactoryFor(fileSvc),
		func() (systemTrustCleaner, error) { return mock, nil },
	)
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	// ListStaleAnchors must be called with an empty fingerprint (list all).
	assert.Equal(t, 1, mock.listCalls, "ListStaleAnchors should be called once")
	assert.Equal(t, "", mock.lastFingerprint, "ListStaleAnchors should receive an empty keep-fingerprint")

	// RemoveStaleAnchors must be called with the listed anchors.
	assert.Equal(t, 1, mock.removeCalls, "RemoveStaleAnchors should be called once")
	require.Len(t, mock.lastRemoved, 2, "RemoveStaleAnchors should receive both stale anchors")

	// Output should mention the OS trust anchor removal.
	output := buf.String()
	assert.Contains(t, output, "Removing 2 g8e root CA anchor(s)")
	assert.Contains(t, output, "OS trust anchors removed")
	assert.Contains(t, output, "Clean complete")
}

// TestGatewayCleanCmd_NoOSTrustAnchors_NoRemoveCall verifies that when
// ListStaleAnchors returns no anchors, RemoveStaleAnchors is NOT called
// and the runtime wipe proceeds normally.
func TestGatewayCleanCmd_NoOSTrustAnchors_NoRemoveCall(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	mock := &mockTrustCleaner{anchors: nil}
	cmd := gatewayCleanCmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		cmdtest.FileSvcFactoryFor(fileSvc),
		func() (systemTrustCleaner, error) { return mock, nil },
	)
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, mock.listCalls)
	assert.Equal(t, 0, mock.removeCalls, "RemoveStaleAnchors should not be called when no anchors found")
	assert.Contains(t, buf.String(), "Clean complete")
}

// TestGatewayCleanCmd_OSTrustUnsupported_ProceedsWithWipe verifies that
// when ListStaleAnchors returns ErrSystemTrustUnsupported (stub platform),
// `gw clean` does NOT print a warning and proceeds with the runtime wipe.
func TestGatewayCleanCmd_OSTrustUnsupported_ProceedsWithWipe(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	mock := &mockTrustCleaner{listErr: constants.ErrSystemTrustUnsupported}
	cmd := gatewayCleanCmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		cmdtest.FileSvcFactoryFor(fileSvc),
		func() (systemTrustCleaner, error) { return mock, nil },
	)
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, mock.removeCalls, "RemoveStaleAnchors should not be called on unsupported platform")
	assert.NotContains(t, buf.String(), "could not enumerate", "no warning on unsupported platform")
	assert.Contains(t, buf.String(), "Clean complete")
}

// TestGatewayCleanCmd_OSTrustListError_ProceedsWithWarning verifies that
// when ListStaleAnchors returns a non-unsupported error, `gw clean` prints
// a warning and proceeds with the runtime wipe (best-effort).
func TestGatewayCleanCmd_OSTrustListError_ProceedsWithWarning(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	mock := &mockTrustCleaner{listErr: errFactory}
	cmd := gatewayCleanCmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		cmdtest.FileSvcFactoryFor(fileSvc),
		func() (systemTrustCleaner, error) { return mock, nil },
	)
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err, "list error is best-effort and must not abort the runtime wipe")
	assert.Contains(t, buf.String(), "could not enumerate OS trust anchors")
	assert.Contains(t, buf.String(), "Clean complete")
}

// TestGatewayCleanCmd_ArchivesRuntimeInsteadOfDeleting verifies `gw clean`
// renames .g8e to .g8e-<MMDDHHMM> so the prior state stays recoverable.
func TestGatewayCleanCmd_ArchivesRuntimeInsteadOfDeleting(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	require.NoError(t, fileSvc.WriteFile(context.Background(), "data/marker.txt", []byte("keep me"), constants.PermFilePrivate))
	runtimeDir := fileSvc.Resolve("")
	cmd := gatewayCleanCmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		cmdtest.FileSvcFactoryFor(fileSvc),
		func() (systemTrustCleaner, error) { return &mockTrustCleaner{}, nil },
	)
	require.NoError(t, cmd.Flags().Set("force", "true"))
	require.NoError(t, cmd.Flags().Set("skip-backup", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.RunE(cmd, nil))

	assert.NoDirExists(t, runtimeDir)
	archives, err := filepath.Glob(runtimeDir + "-*")
	require.NoError(t, err)
	require.Len(t, archives, 1)
	assert.FileExists(t, filepath.Join(archives[0], "data", "marker.txt"))
	assert.Contains(t, buf.String(), archives[0])
}

// TestGatewayCleanCmd_OffersBackupBeforeArchiving verifies the default flow
// snapshots evaluation evidence before the runtime is moved aside, and that
// --skip-backup leaves no snapshot behind.
func TestGatewayCleanCmd_OffersBackupBeforeArchiving(t *testing.T) {
	for _, tc := range []struct {
		name       string
		skipBackup bool
		wantBackup bool
	}{
		{name: "backup taken by default", wantBackup: true},
		{name: "skip-backup opts out", skipBackup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
			evidence := constants.EvaluationDataPath + "/" + constants.EvaluationRunsDirname + "/run-1/report.json"
			require.NoError(t, fileSvc.WriteFile(context.Background(), evidence, []byte(`{"ok":true}`), constants.PermFilePrivate))
			cmd := gatewayCleanCmdWithConfig(
				cmdtest.ConfigLoaderFor(cfg),
				cmdtest.FileSvcFactoryFor(fileSvc),
				func() (systemTrustCleaner, error) { return &mockTrustCleaner{}, nil },
			)
			require.NoError(t, cmd.Flags().Set("force", "true"))
			if tc.skipBackup {
				require.NoError(t, cmd.Flags().Set("skip-backup", "true"))
			}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			require.NoError(t, cmd.RunE(cmd, nil))

			backupDir := filepath.Join(cfg.ProjectRoot, filepath.FromSlash(constants.EvaluationBackupDefaultDir))
			snapshots, _ := filepath.Glob(filepath.Join(backupDir, constants.EvaluationBackupDirPrefix+"*"))
			if tc.wantBackup {
				require.Len(t, snapshots, 1)
				assert.FileExists(t, filepath.Join(snapshots[0], filepath.FromSlash(evidence)))
			} else {
				assert.Empty(t, snapshots)
			}
		})
	}
}
