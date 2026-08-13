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
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
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
	assert.Contains(t, output, "permanently destroyed")
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
	assert.Contains(t, output, "This command will:")
	assert.Contains(t, output, "Stop all running g8e services")
	assert.Contains(t, output, "Wipe the SQLite databases")
	assert.Contains(t, output, "Preserve your existing TLS/PKI")
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
	fileSvc, cfg := newCmdTestEnv(t)
	mock := &mockTrustCleaner{
		anchors: []platform.StaleAnchor{
			{Fingerprint: "stale-fp-1", CommonName: "g8e Root CA", Handle: "/path/stale1"},
			{Fingerprint: "stale-fp-2", CommonName: "g8e Root CA", Handle: "/path/stale2"},
		},
	}
	cmd := gatewayCleanCmdWithConfig(
		configLoaderFor(cfg),
		fileSvcFactoryFor(fileSvc),
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
	fileSvc, cfg := newCmdTestEnv(t)
	mock := &mockTrustCleaner{anchors: nil}
	cmd := gatewayCleanCmdWithConfig(
		configLoaderFor(cfg),
		fileSvcFactoryFor(fileSvc),
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
	fileSvc, cfg := newCmdTestEnv(t)
	mock := &mockTrustCleaner{listErr: constants.ErrSystemTrustUnsupported}
	cmd := gatewayCleanCmdWithConfig(
		configLoaderFor(cfg),
		fileSvcFactoryFor(fileSvc),
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
	fileSvc, cfg := newCmdTestEnv(t)
	mock := &mockTrustCleaner{listErr: errFactory}
	cmd := gatewayCleanCmdWithConfig(
		configLoaderFor(cfg),
		fileSvcFactoryFor(fileSvc),
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
