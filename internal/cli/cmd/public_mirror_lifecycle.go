// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

const (
	defaultPublicMirrorPrivateListen = "127.0.0.1:8081"
	defaultPublicMirrorPublicListen  = "127.0.0.1:8082"
)

func newPublicMirrorProcessManager(fileSvc fs.RuntimeFileService) (*platform.ProcessManager, error) {
	return platform.NewProcessManager(fileSvc)
}

func stopPublicMirrorProcess(out io.Writer, pm *platform.ProcessManager) error {
	pid, err := pm.ReadPIDFile(constants.PublicMirrorPIDFilename)
	if err != nil {
		return err
	}
	if pid == 0 || !pm.IsProcessRunning(pid) {
		_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
		_, err = fmt.Fprintln(out, "Public mirror is not running.")
		return err
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
		return fmt.Errorf("public mirror: find pid %d: %w", pid, err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
		return fmt.Errorf("public mirror: stop pid %d: %w", pid, err)
	}
	deadline := time.Now().Add(platform.ShutdownTimeout)
	for time.Now().Before(deadline) {
		if !pm.IsProcessRunning(pid) {
			_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
			_, err = fmt.Fprintf(out, "Stopped public mirror (pid %d)\n", pid)
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := process.Kill(); err != nil {
		return fmt.Errorf("public mirror: kill pid %d: %w", pid, err)
	}
	_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
	_, err = fmt.Fprintf(out, "Stopped public mirror (pid %d)\n", pid)
	return err
}

func startPublicMirrorDaemon(listenAddress, publicListenAddress string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("public mirror: resolve executable: %w", err)
	}
	args := []string{
		"public", "mirror", "run",
		"--listen", listenAddress,
		"--public-listen", publicListenAddress,
	}
	command := exec.Command(executable, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return 0, fmt.Errorf("public mirror: start daemon: %w", err)
	}
	return command.Process.Pid, nil
}

func restartPublicMirrorDaemon(ctx context.Context, out io.Writer, pm *platform.ProcessManager, fileSvc fs.RuntimeFileService, listenAddress, publicListenAddress string) (int, error) {
	if err := stopPublicMirrorProcess(out, pm); err != nil {
		return 0, err
	}
	if publicMirrorBootstrapHealthy(publicListenAddress) {
		_, err := fmt.Fprintf(out, "Public mirror already available at %s (gateway-owned or existing listener)\n", publicListenAddress)
		return 0, err
	}
	if strings.TrimSpace(listenAddress) == "" {
		listenAddress = defaultPublicMirrorPrivateListen
	}
	if strings.TrimSpace(publicListenAddress) == "" {
		publicListenAddress = defaultPublicMirrorPublicListen
	}
	if err := fileSvc.CreateRuntimeTree(ctx); err != nil {
		return 0, err
	}
	pid, err := startPublicMirrorDaemon(listenAddress, publicListenAddress)
	if err != nil {
		return 0, err
	}
	if err := pm.WritePIDFile(constants.PublicMirrorPIDFilename, pid); err != nil {
		return 0, err
	}
	_, err = fmt.Fprintf(out, "Public mirror started on %s (private) and %s (public) with pid %d\n", listenAddress, publicListenAddress, pid)
	return pid, err
}
