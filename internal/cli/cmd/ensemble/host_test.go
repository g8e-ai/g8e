// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package ensemble

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestHostStatusReadiness(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"ready", `{"status":"ok"}`, "ready ·", 200},
		{"starting", `{"status":"ok"}`, "running; not ready", 503},
		{"invalid health", `not JSON`, "running; not ready", 200},
		{"unhealthy", `{"status":"error"}`, "running; not ready", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "full.pid"), []byte(fmt.Sprint(os.Getpid())), 0600))
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			require.NoError(t, printHostStatus(cmd, dir, server.URL))
			require.Contains(t, buf.String(), tc.want)
		})
	}
}

func TestHostStatusMissingAndInvalidPID(t *testing.T) {
	dir := t.TempDir()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	require.NoError(t, printHostStatus(cmd, dir, "http://127.0.0.1:1"))
	require.Contains(t, buf.String(), "stopped (host)")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "full.pid"), []byte("-1"), 0600))
	require.ErrorContains(t, printHostStatus(cmd, dir, "http://127.0.0.1:1"), "invalid g8ee host PID")
}

func TestEnsembleDefaultsToHost(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(hostRuntime, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(hostRuntime, "full.log"), []byte("local Python log\n"), 0600))
	for _, tc := range []struct{ name, want string }{
		{"status", "stopped (host)"},
		{"logs", "local Python log"},
	} {
		cmd := Cmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{tc.name})
		require.NoError(t, cmd.Execute())
		require.Contains(t, buf.String(), tc.want)
	}
}

func TestLocalPythonPrefersActiveEnvironment(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0700))
	python := filepath.Join(dir, "bin", "python")
	require.NoError(t, os.WriteFile(python, []byte("#!/bin/sh\n"), 0700))
	t.Setenv("VIRTUAL_ENV", dir)
	got, err := localPython()
	require.NoError(t, err)
	require.Equal(t, python, got)
}
