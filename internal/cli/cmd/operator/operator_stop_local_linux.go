//go:build linux

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"golang.org/x/sys/unix"
)

// Hold pidfds before inspecting identity so PID reuse cannot redirect a signal.
// Inspect argv tokens, never a substring of a shell command or a parent PID.
func discoverLocalOperators() ([]localOperatorProcess, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("operator stop: discover local workers: %w", err)
	}
	var processes []localOperatorProcess
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 || pid == os.Getpid() {
			continue
		}
		root := filepath.Join("/proc", entry.Name())
		info, err := os.Stat(root)
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != os.Geteuid() {
			continue
		}
		// Avoid requiring pidfd support for unrelated processes.
		raw, err := os.ReadFile(filepath.Join(root, "cmdline"))
		if err != nil || !isLocalOperatorArgv(strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")) {
			continue
		}
		fd, err := unix.PidfdOpen(pid, 0)
		if errors.Is(err, unix.ESRCH) {
			continue
		}
		if err != nil {
			for _, p := range processes {
				p.close()
			}
			return nil, fmt.Errorf("operator stop: open PID %d: %w", pid, err)
		}
		raw, err = os.ReadFile(filepath.Join(root, "cmdline"))
		args := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		executable, exeErr := os.Readlink(filepath.Join(root, "exe"))
		if err != nil || exeErr != nil || !isLocalOperatorArgv(args) || filepath.Base(strings.TrimSuffix(executable, " (deleted)")) != "g8e" {
			unix.Close(fd)
			continue
		}
		cwd, _ := os.Readlink(filepath.Join(root, "cwd"))
		dir := operatorArgValue(args, "--working-dir")
		if dir == "" {
			dir = cwd
		} else if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
		sessionID := ""
		env, _ := os.ReadFile(filepath.Join(root, "environ"))
		for _, value := range strings.Split(string(env), "\x00") {
			if v, ok := strings.CutPrefix(value, string(constants.EnvVar.OperatorSessionID)+"="); ok {
				sessionID = v
			}
		}
		processes = append(processes, localOperatorProcess{
			pid: pid, dir: filepath.Clean(dir), sessionID: sessionID,
			close: func() { unix.Close(fd) },
			signal: func(force bool) error {
				sig := unix.SIGTERM
				if force {
					sig = unix.SIGKILL
				}
				err := unix.PidfdSendSignal(fd, sig, nil, 0)
				if errors.Is(err, unix.ESRCH) {
					return nil
				}
				return err
			},
			wait: func(timeout time.Duration) (bool, error) {
				deadline := time.Now().Add(timeout)
				for {
					remaining := time.Until(deadline)
					ms := 0
					if remaining > 0 {
						ms = int((remaining + time.Millisecond - 1) / time.Millisecond)
					}
					fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
					n, err := unix.Poll(fds, ms)
					if errors.Is(err, unix.EINTR) {
						continue
					}
					if err != nil {
						return false, err
					}
					return n > 0, nil
				}
			},
		})
	}
	return processes, nil
}

func isLocalOperatorArgv(args []string) bool {
	return len(args) >= 3 && filepath.Base(args[0]) == "g8e" && (args[1] == "operator" || args[1] == "operators") && args[2] == "start"
}

func operatorArgValue(args []string, flag string) string {
	for i := 3; i < len(args); i++ {
		if args[i] == flag && i+1 < len(args) {
			return args[i+1]
		}
		if value, ok := strings.CutPrefix(args[i], flag+"="); ok {
			return value
		}
	}
	return ""
}
