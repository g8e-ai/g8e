// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func TestExecuteDeployHostPrepare(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	ctx := context.Background()

	t.Run("creates and resolves dirs including home expansion", func(t *testing.T) {
		tempDir := t.TempDir()
		relDir := filepath.Join(tempDir, "relative", "sub")
		homeDir := "~/g8e-test-deploy"

		resolved, err := ExecuteDeployHostPrepare(ctx, []string{relDir, homeDir})
		require.NoError(t, err)
		require.Len(t, resolved, 2)

		// Check first dir
		info1, err := os.Stat(resolved[0])
		require.NoError(t, err)
		require.True(t, info1.IsDir())
		require.Equal(t, os.FileMode(constants.PermDirPrivate), info1.Mode().Perm())

		// Check home dir
		expectedHomeSub := filepath.Join(tempHome, "g8e-test-deploy")
		evalHomeSub, err := filepath.EvalSymlinks(expectedHomeSub)
		require.NoError(t, err)
		require.Equal(t, evalHomeSub, resolved[1])

		info2, err := os.Stat(resolved[1])
		require.NoError(t, err)
		require.True(t, info2.IsDir())
		require.Equal(t, os.FileMode(constants.PermDirPrivate), info2.Mode().Perm())
	})

	t.Run("empty dirs error", func(t *testing.T) {
		_, err := ExecuteDeployHostPrepare(ctx, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "dirs list must not be empty")

		_, err = ExecuteDeployHostPrepare(ctx, []string{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "dirs list must not be empty")
	})

	t.Run("context cancelled", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := ExecuteDeployHostPrepare(cancelCtx, []string{t.TempDir()})
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestExecuteDeployHostInstall(t *testing.T) {
	ctx := context.Background()

	t.Run("copies binary atomically and sets executable permissions", func(t *testing.T) {
		srcDir := t.TempDir()
		destDir := t.TempDir()

		srcFile := filepath.Join(srcDir, "source-bin")
		content := []byte("#!/bin/sh\necho test\n")
		require.NoError(t, os.WriteFile(srcFile, content, 0o600))

		targetFile := filepath.Join(destDir, "bin", "target-bin")
		err := ExecuteDeployHostInstall(ctx, srcFile, targetFile)
		require.NoError(t, err)

		targetContent, err := os.ReadFile(targetFile)
		require.NoError(t, err)
		require.Equal(t, content, targetContent)

		info, err := os.Stat(targetFile)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(constants.PermFileExecutable), info.Mode().Perm())
	})

	t.Run("missing source", func(t *testing.T) {
		destDir := t.TempDir()
		err := ExecuteDeployHostInstall(ctx, "", filepath.Join(destDir, "bin"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "source path required")
	})

	t.Run("missing target", func(t *testing.T) {
		srcDir := t.TempDir()
		err := ExecuteDeployHostInstall(ctx, filepath.Join(srcDir, "bin"), "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "target path required")
	})

	t.Run("source file does not exist", func(t *testing.T) {
		tempDir := t.TempDir()
		err := ExecuteDeployHostInstall(ctx, filepath.Join(tempDir, "nonexistent"), filepath.Join(tempDir, "target"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "open source binary")
	})

	t.Run("context cancelled", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		err := ExecuteDeployHostInstall(cancelCtx, "src", "target")
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestExecuteDeployHostLink(t *testing.T) {
	ctx := context.Background()

	t.Run("creates hard link via staging", func(t *testing.T) {
		tempDir := t.TempDir()
		binDir := filepath.Join(tempDir, "bin")
		destDir := filepath.Join(tempDir, "dest")
		require.NoError(t, os.MkdirAll(binDir, constants.PermDirPrivate))

		srcBin := filepath.Join(binDir, "g8e")
		require.NoError(t, os.WriteFile(srcBin, []byte("binary-payload"), constants.PermFileExecutable))

		err := ExecuteDeployHostLink(ctx, binDir, destDir, "g8e")
		require.NoError(t, err)

		destBin := filepath.Join(destDir, "g8e")
		srcInfo, err := os.Stat(srcBin)
		require.NoError(t, err)
		destInfo, err := os.Stat(destBin)
		require.NoError(t, err)
		require.True(t, os.SameFile(srcInfo, destInfo))
	})

	t.Run("default binary name g8e", func(t *testing.T) {
		tempDir := t.TempDir()
		binDir := filepath.Join(tempDir, "bin")
		destDir := filepath.Join(tempDir, "dest")
		require.NoError(t, os.MkdirAll(binDir, constants.PermDirPrivate))

		srcBin := filepath.Join(binDir, "g8e")
		require.NoError(t, os.WriteFile(srcBin, []byte("binary-payload"), constants.PermFileExecutable))

		err := ExecuteDeployHostLink(ctx, binDir, destDir, "")
		require.NoError(t, err)

		destBin := filepath.Join(destDir, "g8e")
		srcInfo, err := os.Stat(srcBin)
		require.NoError(t, err)
		destInfo, err := os.Stat(destBin)
		require.NoError(t, err)
		require.True(t, os.SameFile(srcInfo, destInfo))
	})

	t.Run("missing binary_dir", func(t *testing.T) {
		err := ExecuteDeployHostLink(ctx, "", "dest", "g8e")
		require.Error(t, err)
		require.Contains(t, err.Error(), "binary_dir required")
	})

	t.Run("missing dest_dir", func(t *testing.T) {
		err := ExecuteDeployHostLink(ctx, "bin", "", "g8e")
		require.Error(t, err)
		require.Contains(t, err.Error(), "dest_dir required")
	})

	t.Run("source binary not found", func(t *testing.T) {
		tempDir := t.TempDir()
		err := ExecuteDeployHostLink(ctx, tempDir, filepath.Join(tempDir, "dest"), "g8e")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("context cancelled", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		err := ExecuteDeployHostLink(cancelCtx, "bin", "dest", "g8e")
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestExecuteDeployHostStart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock worker script uses POSIX shell")
	}

	ctx := context.Background()

	t.Run("starts worker detached and writes operator.pid and start.log", func(t *testing.T) {
		workingDir := t.TempDir()
		mockWorker := filepath.Join(workingDir, "g8e")
		script := "#!/bin/sh\necho \"worker running\"\nsleep 30\n"
		require.NoError(t, os.WriteFile(mockWorker, []byte(script), constants.PermFileExecutable))

		pid, err := ExecuteDeployHostStart(ctx, workingDir, "g8e", []string{"--dummy-arg"})
		require.NoError(t, err)
		require.Greater(t, pid, 1)

		t.Cleanup(func() {
			_, _ = ExecuteDeployHostStop(context.Background(), "", workingDir)
		})

		// Verify pid file exists and matches
		pidFile := filepath.Join(workingDir, constants.OperatorPIDFilename)
		pidData, err := os.ReadFile(pidFile)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("%d\n", pid), string(pidData))

		// Verify start.log exists
		logPath := filepath.Join(workingDir, constants.DeployStartLogFilename)
		_, err = os.Stat(logPath)
		require.NoError(t, err)

		// Calling start again in same workingDir stops previous worker first
		pid2, err := ExecuteDeployHostStart(ctx, workingDir, "g8e", []string{"--dummy-arg-2"})
		require.NoError(t, err)
		require.Greater(t, pid2, 1)
		require.NotEqual(t, pid, pid2)

		// Previous pid should no longer be running
		prevProc, err := os.FindProcess(pid)
		require.NoError(t, err)
		_ = prevProc.Release()
	})

	t.Run("missing working_dir", func(t *testing.T) {
		_, err := ExecuteDeployHostStart(ctx, "", "g8e", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "working_dir required")
	})

	t.Run("binary not found", func(t *testing.T) {
		tempDir := t.TempDir()
		_, err := ExecuteDeployHostStart(ctx, tempDir, "nonexistent-bin", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("context cancelled", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := ExecuteDeployHostStart(cancelCtx, t.TempDir(), "g8e", nil)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestExecuteDeployHostState(t *testing.T) {
	ctx := context.Background()

	t.Run("reads deployment state successfully", func(t *testing.T) {
		fileSvc, _ := cmdtest.NewCmdTestEnv(t)
		now := time.Now().UTC().Truncate(time.Second)
		stateJSON := fmt.Sprintf(`{
			"launch_id": "launch-123",
			"phase": "ready",
			"operator_session_id": "session-456",
			"request_id": "req-789",
			"updated_at": %q
		}`, now.Format(time.RFC3339))
		statePath := filepath.Join(constants.DeploymentDirname, constants.DeploymentStateFileOperator)
		require.NoError(t, fileSvc.WriteFile(ctx, statePath, []byte(stateJSON), constants.PermFilePrivate))

		state, err := ExecuteDeployHostState(ctx, fileSvc)
		require.NoError(t, err)
		require.NotNil(t, state)
		require.Equal(t, "launch-123", state.LaunchID)
		require.Equal(t, models.OperatorDeploymentPhaseReady, state.Phase)
		require.Equal(t, "session-456", state.OperatorSessionID)
		require.Equal(t, "req-789", state.RequestID)
		require.Equal(t, now, state.UpdatedAt)
	})

	t.Run("file not found returns nil without error", func(t *testing.T) {
		fileSvc, _ := cmdtest.NewCmdTestEnv(t)
		state, err := ExecuteDeployHostState(ctx, fileSvc)
		require.NoError(t, err)
		require.Nil(t, state)
	})

	t.Run("invalid state json returns error", func(t *testing.T) {
		fileSvc, _ := cmdtest.NewCmdTestEnv(t)
		statePath := filepath.Join(constants.DeploymentDirname, constants.DeploymentStateFileOperator)
		require.NoError(t, fileSvc.WriteFile(ctx, statePath, []byte("invalid-json"), constants.PermFilePrivate))

		state, err := ExecuteDeployHostState(ctx, fileSvc)
		require.ErrorIs(t, err, constants.ErrOperatorDeployFailed)
		require.Nil(t, state)
	})
}

func TestExecuteDeployHostPreflight(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid host fails validation", func(t *testing.T) {
		err := ExecuteDeployHostPreflight(ctx, []string{"invalid$$host"})
		require.Error(t, err)
		require.ErrorIs(t, err, constants.ErrPathValidation)
	})

	t.Run("no args fails validation", func(t *testing.T) {
		err := ExecuteDeployHostPreflight(ctx, []string{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "accepts 1 arg(s)")
	})
}

func TestExecuteDeployHostStop(t *testing.T) {
	ctx := context.Background()

	t.Run("discovery mock handles matches and escalates TERM to KILL", func(t *testing.T) {
		destDir := t.TempDir()
		workingDir := filepath.Join(destDir, "worker-1")
		otherDir := t.TempDir()
		require.NoError(t, os.MkdirAll(workingDir, constants.PermDirPrivate))
		require.NoError(t, os.MkdirAll(otherDir, constants.PermDirPrivate))

		// Create PID files
		pid1File := filepath.Join(workingDir, constants.OperatorPIDFilename)
		require.NoError(t, os.WriteFile(pid1File, []byte("101\n"), constants.PermFilePrivate))

		var signals1, signals2 []bool
		closed3 := false

		p1 := localOperatorProcess{
			pid: 101,
			dir: workingDir,
			wait: func(time.Duration) (bool, error) {
				return true, nil
			},
			signal: func(force bool) error {
				signals1 = append(signals1, force)
				return nil
			},
		}

		p2 := localOperatorProcess{
			pid: 102,
			dir: destDir,
			wait: func(time.Duration) (bool, error) {
				// First check (TERM) returns false; second check (KILL) returns true
				return len(signals2) > 1, nil
			},
			signal: func(force bool) error {
				signals2 = append(signals2, force)
				return nil
			},
		}

		p3 := localOperatorProcess{
			pid: 103,
			dir: otherDir,
			close: func() {
				closed3 = true
			},
		}

		origDiscoverer := defaultLocalOperatorDiscoverer
		defaultLocalOperatorDiscoverer = func() ([]localOperatorProcess, error) {
			return []localOperatorProcess{p1, p2, p3}, nil
		}
		t.Cleanup(func() {
			defaultLocalOperatorDiscoverer = origDiscoverer
		})

		pids, err := ExecuteDeployHostStop(ctx, destDir, "")
		require.NoError(t, err)
		require.Equal(t, []int{101, 102}, pids)
		require.Equal(t, []bool{false}, signals1)       // TERM was sufficient
		require.Equal(t, []bool{false, true}, signals2) // TERM then KILL
		require.True(t, closed3)                        // Non-matching process closed

		// PID file in workingDir should have been cleaned up
		_, err = os.Stat(pid1File)
		require.True(t, errors.Is(err, os.ErrNotExist))
	})

	t.Run("pid file fallback terminates process and removes pid file", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skipping process exec on windows")
		}

		origDiscoverer := defaultLocalOperatorDiscoverer
		defaultLocalOperatorDiscoverer = func() ([]localOperatorProcess, error) {
			return nil, errors.New("discovery not available")
		}
		t.Cleanup(func() {
			defaultLocalOperatorDiscoverer = origDiscoverer
		})

		tempDir := t.TempDir()
		workerDir := filepath.Join(tempDir, "worker")
		require.NoError(t, os.MkdirAll(workerDir, constants.PermDirPrivate))

		// Start a real dummy process
		cmd := exec.Command("sleep", "30")
		require.NoError(t, cmd.Start())
		pid := cmd.Process.Pid
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})

		pidFile := filepath.Join(workerDir, constants.OperatorPIDFilename)
		require.NoError(t, os.WriteFile(pidFile, []byte(strconv.Itoa(pid)+"\n"), constants.PermFilePrivate))

		stopped, err := ExecuteDeployHostStop(ctx, tempDir, "")
		require.NoError(t, err)
		require.Contains(t, stopped, pid)

		// PID file should be removed
		_, err = os.Stat(pidFile)
		require.True(t, errors.Is(err, os.ErrNotExist))
	})

	t.Run("missing both dest_dir and working_dir returns error", func(t *testing.T) {
		_, err := ExecuteDeployHostStop(ctx, "", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "dest_dir or working_dir required")
	})
}

func TestExecuteDeployHost_Router(t *testing.T) {
	ctx := context.Background()

	t.Run("dispatches prepare action", func(t *testing.T) {
		tempDir := t.TempDir()
		req := models.DeployHostRequest{
			Action: models.DeployHostActionPrepare,
			Dirs:   []string{filepath.Join(tempDir, "dir1")},
		}
		resp, err := ExecuteDeployHost(ctx, req, nil)
		require.NoError(t, err)
		require.Len(t, resp.ResolvedDirs, 1)
	})

	t.Run("dispatches install action with target", func(t *testing.T) {
		tempDir := t.TempDir()
		src := filepath.Join(tempDir, "src")
		target := filepath.Join(tempDir, "target")
		require.NoError(t, os.WriteFile(src, []byte("data"), constants.PermFileExecutable))

		req := models.DeployHostRequest{
			Action: models.DeployHostActionInstall,
			Source: src,
			Target: target,
		}
		_, err := ExecuteDeployHost(ctx, req, nil)
		require.NoError(t, err)

		_, err = os.Stat(target)
		require.NoError(t, err)
	})

	t.Run("dispatches install action with DestDir fallback", func(t *testing.T) {
		tempDir := t.TempDir()
		src := filepath.Join(tempDir, "src")
		destDir := filepath.Join(tempDir, "dest")
		require.NoError(t, os.WriteFile(src, []byte("data"), constants.PermFileExecutable))

		req := models.DeployHostRequest{
			Action:  models.DeployHostActionInstall,
			Source:  src,
			DestDir: destDir,
		}
		_, err := ExecuteDeployHost(ctx, req, nil)
		require.NoError(t, err)

		expectedTarget := filepath.Join(destDir, constants.DeployBinDirname, "g8e")
		_, err = os.Stat(expectedTarget)
		require.NoError(t, err)
	})

	t.Run("dispatches link action", func(t *testing.T) {
		tempDir := t.TempDir()
		binDir := filepath.Join(tempDir, "bin")
		destDir := filepath.Join(tempDir, "dest")
		require.NoError(t, os.MkdirAll(binDir, constants.PermDirPrivate))
		srcBin := filepath.Join(binDir, "g8e")
		require.NoError(t, os.WriteFile(srcBin, []byte("data"), constants.PermFileExecutable))

		req := models.DeployHostRequest{
			Action:    models.DeployHostActionLink,
			BinaryDir: binDir,
			DestDir:   destDir,
			Binary:    "g8e",
		}
		_, err := ExecuteDeployHost(ctx, req, nil)
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(destDir, "g8e"))
		require.NoError(t, err)
	})

	t.Run("dispatches state action", func(t *testing.T) {
		fileSvc, _ := cmdtest.NewCmdTestEnv(t)
		now := time.Now().UTC().Truncate(time.Second)
		stateJSON := fmt.Sprintf(`{
			"launch_id": "launch-123",
			"phase": "ready",
			"operator_session_id": "session-456",
			"updated_at": %q
		}`, now.Format(time.RFC3339))
		statePath := filepath.Join(constants.DeploymentDirname, constants.DeploymentStateFileOperator)
		require.NoError(t, fileSvc.WriteFile(ctx, statePath, []byte(stateJSON), constants.PermFilePrivate))

		req := models.DeployHostRequest{
			Action:     models.DeployHostActionState,
			WorkingDir: "work",
		}
		factory := func(string, *slog.Logger) (fs.RuntimeFileService, error) {
			return fileSvc, nil
		}
		resp, err := ExecuteDeployHost(ctx, req, factory)
		require.NoError(t, err)
		require.NotNil(t, resp.State)
		require.Equal(t, models.OperatorDeploymentPhaseReady, resp.State.Phase)
	})

	t.Run("dispatches stop action", func(t *testing.T) {
		origDiscoverer := defaultLocalOperatorDiscoverer
		defaultLocalOperatorDiscoverer = func() ([]localOperatorProcess, error) {
			return nil, nil
		}
		t.Cleanup(func() {
			defaultLocalOperatorDiscoverer = origDiscoverer
		})

		req := models.DeployHostRequest{
			Action:     models.DeployHostActionStop,
			WorkingDir: t.TempDir(),
		}
		resp, err := ExecuteDeployHost(ctx, req, nil)
		require.NoError(t, err)
		require.Equal(t, 0, resp.StoppedCount)
	})

	t.Run("unknown action returns error", func(t *testing.T) {
		req := models.DeployHostRequest{
			Action: "invalid-action",
		}
		_, err := ExecuteDeployHost(ctx, req, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown deploy-host action")
	})
}

func TestOperatorDeployHostCmd_Protocol(t *testing.T) {
	t.Run("valid json request on stdin returns json response on stdout", func(t *testing.T) {
		tempDir := t.TempDir()
		req := models.DeployHostRequest{
			Action: models.DeployHostActionPrepare,
			Dirs:   []string{filepath.Join(tempDir, "sub")},
		}
		reqBytes, err := json.Marshal(req)
		require.NoError(t, err)

		cmd := operatorDeployHostCmd()
		var stdout bytes.Buffer
		cmd.SetIn(bytes.NewReader(reqBytes))
		cmd.SetOut(&stdout)
		cmd.SetErr(io.Discard)

		err = cmd.Execute()
		require.NoError(t, err)

		var resp models.DeployHostResponse
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &resp))
		require.True(t, resp.Success)
		require.Len(t, resp.ResolvedDirs, 1)
	})

	t.Run("invalid json on stdin returns error response", func(t *testing.T) {
		cmd := operatorDeployHostCmd()
		var stdout bytes.Buffer
		cmd.SetIn(strings.NewReader("not-valid-json"))
		cmd.SetOut(&stdout)
		cmd.SetErr(io.Discard)

		err := cmd.Execute()
		require.Error(t, err)

		var resp models.DeployHostResponse
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &resp))
		require.False(t, resp.Success)
		require.Contains(t, resp.Error, "decode deploy-host request")
	})

	t.Run("action failure returns error response and command error", func(t *testing.T) {
		req := models.DeployHostRequest{
			Action: "unknown-action",
		}
		reqBytes, err := json.Marshal(req)
		require.NoError(t, err)

		cmd := operatorDeployHostCmd()
		var stdout bytes.Buffer
		cmd.SetIn(bytes.NewReader(reqBytes))
		cmd.SetOut(&stdout)
		cmd.SetErr(io.Discard)

		err = cmd.Execute()
		require.Error(t, err)

		var resp models.DeployHostResponse
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &resp))
		require.False(t, resp.Success)
		require.Contains(t, resp.Error, "unknown deploy-host action")
	})
}
