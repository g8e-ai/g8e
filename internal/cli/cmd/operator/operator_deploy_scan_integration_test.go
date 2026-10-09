// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration && !windows

package operatorcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestOperatorDeployScansOnlyPreviousDeployments(t *testing.T) {
	for _, local := range []bool{true, false} {
		name := "ssh"
		if local {
			name = "local"
		}
		t.Run(name, func(t *testing.T) {
			for _, name := range []string{"fresh", "pid", "log only", "dangling pid"} {
				t.Run(name, func(t *testing.T) {
					dir := testutil.TempDir(t)
					bin := testutil.TempDir(t)
					marker := filepath.Join(bin, "scanned")
					// Run the actual local/SSH launch scripts, substituting only the
					// external transport and process scanners. Never signal host workers.
					for _, tool := range []string{"pkill", "pgrep"} {
						require.NoError(t, os.WriteFile(filepath.Join(bin, tool), []byte("#!/bin/sh\necho scan >> "+operatorDeployShellQuote(marker)+"\nexit 1\n"), 0o700))
					}
					require.NoError(t, os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\nshift\nexec sh -c \"$1\"\n"), 0o700))
					require.NoError(t, os.WriteFile(filepath.Join(dir, "g8e"), []byte("#!/bin/sh\nexit 0\n"), 0o700))
					switch name {
					case "pid":
						require.NoError(t, os.WriteFile(filepath.Join(dir, "operator.pid"), []byte("123\n"), 0o600))
					case "log only":
						// A failed/interrupted launch may create its log before its PID.
						require.NoError(t, os.WriteFile(filepath.Join(dir, "start.log"), nil, 0o600))
					case "dangling pid":
						require.NoError(t, os.Symlink(filepath.Join(dir, "missing-pid"), filepath.Join(dir, "operator.pid")))
					}
					t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
					host := deploySSH{local: local, host: "fixture"}
					require.NoError(t, host.startOperator(t.Context(), dir, "localhost"))
					_, err := os.Stat(marker)
					if name != "fresh" {
						require.NoError(t, err, "redeployments must still check for their previous worker")
					} else {
						require.ErrorIs(t, err, os.ErrNotExist, "fresh workers must not scan the host process table")
					}
				})
			}
		})
	}
}
