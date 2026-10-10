// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
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
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

var defaultLocalOperatorDiscoverer = discoverLocalOperators

func operatorDeployHostCmd() *cobra.Command {
	return operatorDeployHostCmdWithFactory(shared.NewFileSvc)
}

func operatorDeployHostCmdWithFactory(factory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "deploy-host",
		Short:        "Remote host agent for operator deployment and management",
		Hidden:       true,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var req models.DeployHostRequest
			decoder := json.NewDecoder(cmd.InOrStdin())
			if err := decoder.Decode(&req); err != nil {
				resp := models.DeployHostResponse{
					Success: false,
					Error:   fmt.Sprintf("decode deploy-host request: %v", err),
				}
				_ = json.NewEncoder(cmd.OutOrStdout()).Encode(resp)
				return fmt.Errorf("decode deploy-host request: %w", err)
			}

			resp, err := ExecuteDeployHost(cmd.Context(), req, factory)
			if err != nil {
				resp.Success = false
				resp.Error = err.Error()
				_ = json.NewEncoder(cmd.OutOrStdout()).Encode(resp)
				return err
			}

			resp.Success = true
			return json.NewEncoder(cmd.OutOrStdout()).Encode(resp)
		},
	}
	return cmd
}

// ExecuteDeployHost executes a typed DeployHostRequest and returns a typed DeployHostResponse.
func ExecuteDeployHost(ctx context.Context, req models.DeployHostRequest, factory func(string, *slog.Logger) (fs.RuntimeFileService, error)) (models.DeployHostResponse, error) {
	if factory == nil {
		factory = shared.NewFileSvc
	}
	var resp models.DeployHostResponse

	switch req.Action {
	case models.DeployHostActionPrepare:
		dirs, err := ExecuteDeployHostPrepare(ctx, req.Dirs)
		if err != nil {
			return resp, err
		}
		resp.ResolvedDirs = dirs
		return resp, nil

	case models.DeployHostActionInstall:
		target := req.Target
		if target == "" && req.DestDir != "" {
			target = filepath.Join(req.DestDir, constants.DeployBinDirname, "g8e")
		}
		if err := ExecuteDeployHostInstall(ctx, req.Source, target); err != nil {
			return resp, err
		}
		return resp, nil

	case models.DeployHostActionLink:
		binaryDir := req.BinaryDir
		if binaryDir == "" && req.Source != "" {
			binaryDir = req.Source
		}
		destDir := req.DestDir
		if destDir == "" && req.Target != "" {
			destDir = req.Target
		}
		if destDir == "" && req.WorkingDir != "" {
			destDir = req.WorkingDir
		}
		if err := ExecuteDeployHostLink(ctx, binaryDir, destDir, req.Binary); err != nil {
			return resp, err
		}
		return resp, nil

	case models.DeployHostActionStart:
		workingDir := req.WorkingDir
		if workingDir == "" {
			workingDir = req.DestDir
		}
		pid, err := ExecuteDeployHostStart(ctx, workingDir, req.Binary, req.Args)
		if err != nil {
			return resp, err
		}
		resp.PID = pid
		return resp, nil

	case models.DeployHostActionState:
		workingDir := req.WorkingDir
		if workingDir == "" {
			workingDir = req.DestDir
		}
		fileSvc, err := factory(workingDir, slog.Default())
		if err != nil {
			return resp, fmt.Errorf("operator deploy host: %w: %w", constants.ErrFileServiceInit, err)
		}
		state, err := ExecuteDeployHostState(ctx, fileSvc)
		if err != nil {
			return resp, err
		}
		resp.State = state
		return resp, nil

	case models.DeployHostActionPreflight:
		args := req.PreflightArgs
		if len(args) == 0 {
			args = req.Args
		}
		if err := ExecuteDeployHostPreflight(ctx, args); err != nil {
			return resp, err
		}
		return resp, nil

	case models.DeployHostActionStop:
		pids, err := ExecuteDeployHostStop(ctx, req.DestDir, req.WorkingDir)
		if err != nil {
			return resp, err
		}
		resp.StoppedPIDs = pids
		resp.StoppedCount = len(pids)
		return resp, nil

	default:
		return resp, fmt.Errorf("unknown deploy-host action: %q", req.Action)
	}
}

func resolveHomePath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir for %s: %w", p, err)
		}
		if p == "~" {
			return home, nil
		} else if strings.HasPrefix(p, "~/") {
			return filepath.Join(home, strings.TrimPrefix(p, "~/")), nil
		}
		return filepath.Join(home, strings.TrimPrefix(p, `~\`)), nil
	}
	return p, nil
}

// ExecuteDeployHostPrepare resolves `~` with os.UserHomeDir, creates directories with PermDirPrivate,
// and returns their absolute resolved paths.
func ExecuteDeployHostPrepare(ctx context.Context, dirs []string) ([]string, error) {
	if len(dirs) == 0 {
		return nil, errors.New("operator deploy-host prepare: dirs list must not be empty")
	}
	var resolvedDirs []string
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		clean, err := resolveHomePath(dir)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(clean, constants.PermDirPrivate); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", clean, err)
		}
		absDir, err := filepath.Abs(clean)
		if err != nil {
			return nil, fmt.Errorf("resolve abs dir for %s: %w", clean, err)
		}
		resolved, err := filepath.EvalSymlinks(absDir)
		if err != nil {
			return nil, fmt.Errorf("eval symlinks for %s: %w", absDir, err)
		}
		resolvedDirs = append(resolvedDirs, resolved)
	}
	return resolvedDirs, nil
}

// ExecuteDeployHostInstall copies a binary from source to target atomically and ETXTBSY-safe.
func ExecuteDeployHostInstall(ctx context.Context, source, target string) error {
	if source == "" {
		return errors.New("operator deploy-host install: source path required")
	}
	if target == "" {
		return errors.New("operator deploy-host install: target path required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var err error
	source, err = resolveHomePath(source)
	if err != nil {
		return err
	}
	target, err = resolveHomePath(target)
	if err != nil {
		return err
	}

	targetDir := filepath.Dir(target)
	if err := os.MkdirAll(targetDir, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("create target dir %s: %w", targetDir, err)
	}

	srcFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source binary %s: %w", source, err)
	}
	defer srcFile.Close()

	tmp, err := os.CreateTemp(targetDir, ".g8e-install-*")
	if err != nil {
		return fmt.Errorf("create install staging in %s: %w", targetDir, err)
	}
	staging := tmp.Name()
	defer func() { _ = os.Remove(staging) }()

	if _, err := io.Copy(tmp, srcFile); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("copy binary to staging %s: %w", staging, err)
	}
	if err := tmp.Chmod(constants.PermFileExecutable); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod executable staging %s: %w", staging, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close staging file %s: %w", staging, err)
	}
	if err := os.Rename(staging, target); err != nil {
		return fmt.Errorf("rename %s to %s: %w", staging, target, err)
	}
	return nil
}

// ExecuteDeployHostLink hard-links a binary from binaryDir into destDir/binary via a staging name.
func ExecuteDeployHostLink(ctx context.Context, binaryDir, destDir, binary string) error {
	if binary == "" {
		binary = "g8e"
	}
	if binaryDir == "" {
		return errors.New("operator deploy-host link: binary_dir required")
	}
	if destDir == "" {
		return errors.New("operator deploy-host link: dest_dir required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var err error
	binaryDir, err = resolveHomePath(binaryDir)
	if err != nil {
		return err
	}
	destDir, err = resolveHomePath(destDir)
	if err != nil {
		return err
	}

	sourceBinary := binaryDir
	if filepath.Base(binaryDir) != binary {
		sourceBinary = filepath.Join(binaryDir, binary)
	}
	if _, err := os.Stat(sourceBinary); err != nil {
		return fmt.Errorf("source binary %s not found: %w", sourceBinary, err)
	}

	if err := os.MkdirAll(destDir, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("create dest dir %s: %w", destDir, err)
	}

	tmp, err := os.CreateTemp(destDir, ".g8e-link-*")
	if err != nil {
		return fmt.Errorf("create temp link staging in %s: %w", destDir, err)
	}
	staging := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(staging) }()
	_ = os.Remove(staging)

	if err := os.Link(sourceBinary, staging); err != nil {
		return fmt.Errorf("hard link %s to %s: %w", sourceBinary, staging, err)
	}
	destBinary := filepath.Join(destDir, binary)
	if err := os.Rename(staging, destBinary); err != nil {
		return fmt.Errorf("rename link %s to %s: %w", staging, destBinary, err)
	}
	return nil
}

// ExecuteDeployHostStart stops any previous worker in workingDir, starts a new worker detached,
// writes operator.pid, and returns the new PID.
func ExecuteDeployHostStart(ctx context.Context, workingDir, binary string, args []string) (int, error) {
	if workingDir == "" {
		return 0, errors.New("operator deploy-host start: working_dir required")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	resolvedWorkingDir, err := resolveHomePath(workingDir)
	if err != nil {
		return 0, err
	}
	absWorkingDir, err := filepath.Abs(resolvedWorkingDir)
	if err != nil {
		return 0, fmt.Errorf("resolve abs working dir for %s: %w", workingDir, err)
	}
	workingDir = filepath.Clean(absWorkingDir)

	if _, err := ExecuteDeployHostStop(ctx, "", workingDir); err != nil {
		return 0, fmt.Errorf("stop previous worker in %s: %w", workingDir, err)
	}

	logPath := filepath.Join(workingDir, constants.DeployStartLogFilename)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, constants.PermFilePrivate)
	if err != nil {
		return 0, fmt.Errorf("open start log %s: %w", logPath, err)
	}
	defer logFile.Close()

	binName := "g8e"
	if runtime.GOOS == "windows" {
		binName = "g8e.exe"
	}
	binPath := binary
	if binPath == "" {
		binPath = filepath.Join(workingDir, binName)
	} else if !filepath.IsAbs(binPath) {
		binPath = filepath.Join(workingDir, binPath)
	}
	if _, err := os.Stat(binPath); err != nil {
		return 0, fmt.Errorf("worker binary %s not found: %w", binPath, err)
	}

	var cmdArgs []string
	if len(args) >= 2 && (args[0] == "operator" || args[0] == "operators") && args[1] == "start" {
		cmdArgs = append(cmdArgs, args...)
	} else if len(args) >= 1 && (args[0] == "operator" || args[0] == "operators") {
		cmdArgs = append([]string{args[0], "start"}, args[1:]...)
	} else {
		cmdArgs = append([]string{"operator", "start"}, args...)
	}

	hasWorkingDir := false
	for _, a := range cmdArgs {
		if a == "--working-dir" || strings.HasPrefix(a, "--working-dir=") {
			hasWorkingDir = true
			break
		}
	}
	if !hasWorkingDir {
		cmdArgs = append(cmdArgs, "--working-dir", workingDir)
	}

	worker := exec.Command(binPath, cmdArgs...)
	if runtime.GOOS == "windows" {
		worker.Args[0] = `.\` + filepath.Base(binPath)
	} else {
		worker.Args[0] = "./" + filepath.Base(binPath)
	}
	worker.Dir = workingDir
	worker.Stdout = logFile
	worker.Stderr = logFile
	detachDeployedOperator(worker)

	if err := worker.Start(); err != nil {
		return 0, fmt.Errorf("start worker process: %w", err)
	}

	pid := worker.Process.Pid
	pidFile := filepath.Join(workingDir, constants.OperatorPIDFilename)
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", pid)), constants.PermFilePrivate); err != nil {
		_ = worker.Process.Kill()
		_ = worker.Wait()
		return 0, fmt.Errorf("write pid file %s: %w", pidFile, err)
	}
	_ = worker.Process.Release()
	return pid, nil
}

// ExecuteDeployHostState reads the OperatorDeploymentState using RuntimeFileService.
func ExecuteDeployHostState(ctx context.Context, fileSvc fs.RuntimeFileService) (*models.OperatorDeploymentState, error) {
	return readOperatorDeploymentState(ctx, fileSvc)
}

// ExecuteDeployHostPreflight executes the gateway preflight check.
func ExecuteDeployHostPreflight(ctx context.Context, preflightArgs []string) error {
	cmd := operatorGatewayPreflightCmd()
	cmd.SetArgs(preflightArgs)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.ExecuteContext(ctx)
}

// ExecuteDeployHostStop stops every worker under destDir or matching workingDir concurrently.
func ExecuteDeployHostStop(ctx context.Context, destDir, workingDir string) ([]int, error) {
	if destDir == "" && workingDir == "" {
		return nil, errors.New("operator deploy-host stop: dest_dir or working_dir required")
	}

	var cleanDest, cleanWork string
	if destDir != "" {
		if r, err := resolveHomePath(destDir); err == nil {
			destDir = r
		}
		abs, err := filepath.Abs(destDir)
		if err == nil {
			cleanDest = filepath.Clean(abs)
		} else {
			cleanDest = filepath.Clean(destDir)
		}
	}
	if workingDir != "" {
		if r, err := resolveHomePath(workingDir); err == nil {
			workingDir = r
		}
		abs, err := filepath.Abs(workingDir)
		if err == nil {
			cleanWork = filepath.Clean(abs)
		} else {
			cleanWork = filepath.Clean(workingDir)
		}
	}

	matchesDir := func(dir string) bool {
		c := filepath.Clean(dir)
		if cleanWork != "" && c == cleanWork {
			return true
		}
		if cleanDest != "" && (c == cleanDest || strings.HasPrefix(c, cleanDest+string(filepath.Separator))) {
			return true
		}
		return false
	}

	var stoppedPIDs []int
	var mu sync.Mutex

	// 1. Try discovery (on Linux via pidfd).
	locals, err := defaultLocalOperatorDiscoverer()
	if err == nil {
		var matched []localOperatorProcess
		for _, p := range locals {
			if matchesDir(p.dir) {
				matched = append(matched, p)
			} else if p.close != nil {
				p.close()
			}
		}

		if len(matched) > 0 {
			var wg sync.WaitGroup
			sem := make(chan struct{}, 64)
			for _, p := range matched {
				wg.Add(1)
				go func(proc localOperatorProcess) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					_ = proc.signal(false)
					exited, _ := proc.wait(2 * time.Second)
					if !exited {
						_ = proc.signal(true)
						_, _ = proc.wait(2 * time.Second)
					}
					if proc.close != nil {
						proc.close()
					}
					_ = os.Remove(filepath.Join(proc.dir, constants.OperatorPIDFilename))

					mu.Lock()
					stoppedPIDs = append(stoppedPIDs, proc.pid)
					mu.Unlock()
				}(p)
			}
			wg.Wait()
		}
	}

	// 2. Also check recorded PID files under destDir and/or workingDir (for non-Linux or script/fallback workers).
	fallbackPIDs := stopWorkersFromPIDFiles(ctx, cleanDest, cleanWork, matchesDir)
	for _, pid := range fallbackPIDs {
		if !slices.Contains(stoppedPIDs, pid) {
			stoppedPIDs = append(stoppedPIDs, pid)
		}
	}

	sort.Ints(stoppedPIDs)
	return stoppedPIDs, nil
}

func stopWorkersFromPIDFiles(ctx context.Context, destDir, workingDir string, matchesDir func(string) bool) []int {
	var pidFiles []string
	if workingDir != "" {
		p := filepath.Join(workingDir, constants.OperatorPIDFilename)
		if _, err := os.Stat(p); err == nil {
			pidFiles = append(pidFiles, p)
		}
	}
	if destDir != "" {
		_ = filepath.WalkDir(destDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && d.Name() == constants.OperatorPIDFilename {
				dir := filepath.Dir(path)
				if matchesDir(dir) {
					pidFiles = append(pidFiles, path)
				}
			}
			return nil
		})
	}

	var stopped []int
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 64)

	for _, pf := range pidFiles {
		data, err := os.ReadFile(pf)
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || pid <= 1 || pid == os.Getpid() {
			_ = os.Remove(pf)
			continue
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			_ = os.Remove(pf)
			continue
		}

		wg.Add(1)
		go func(p *os.Process, pid int, file string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			defer func() { _ = p.Release() }()

			if runtime.GOOS == "windows" {
				_ = p.Kill()
				_ = os.Remove(file)
				mu.Lock()
				stopped = append(stopped, pid)
				mu.Unlock()
				return
			}

			if err := p.Signal(syscall.Signal(0)); err != nil {
				_ = os.Remove(file)
				return
			}

			_ = p.Signal(syscall.SIGTERM)
			deadline := time.Now().Add(2 * time.Second)
			exited := false
			for time.Now().Before(deadline) && ctx.Err() == nil {
				if err := p.Signal(syscall.Signal(0)); err != nil {
					exited = true
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			if !exited {
				_ = p.Kill()
			}
			_ = os.Remove(file)

			mu.Lock()
			stopped = append(stopped, pid)
			mu.Unlock()
		}(proc, pid, pf)
	}
	wg.Wait()
	return stopped
}
