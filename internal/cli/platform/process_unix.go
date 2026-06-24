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
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !windows
// +build !windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// processFinder is an interface for finding processes
type processFinder interface {
	FindProcess(pid int) (process, error)
}

// process is an interface for process operations
type process interface {
	Signal(sig syscall.Signal) error
}

// osProcess wraps os.Process to implement the process interface
type osProcess struct {
	*os.Process
}

func (p *osProcess) Signal(sig syscall.Signal) error {
	return p.Process.Signal(sig)
}

// osProcessFinder wraps os.FindProcess to implement processFinder
type osProcessFinder struct{}

func (f osProcessFinder) FindProcess(pid int) (process, error) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}
	return &osProcess{p}, nil
}

// realCommandExecutor implements CommandExecutor using actual exec package
type realCommandExecutor struct{}

func (c realCommandExecutor) Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func (c realCommandExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

func (c realCommandExecutor) Run(cmd *exec.Cmd) error {
	return cmd.Run()
}

// sleeper is an interface for sleep operations (for testing)
type sleeper interface {
	Sleep(d time.Duration)
}

// timeSleeper wraps time.Sleep to implement sleeper
type timeSleeper struct{}

func (s timeSleeper) Sleep(d time.Duration) {
	time.Sleep(d)
}

// ticker is an interface for ticker operations (for testing)
type ticker interface {
	C() <-chan time.Time
	Stop()
}

// timeTicker wraps time.Ticker to implement ticker
type timeTicker struct {
	*time.Ticker
}

func (t *timeTicker) C() <-chan time.Time {
	return t.Ticker.C
}

// tickerFactory is an interface for creating tickers
type tickerFactory interface {
	NewTicker(d time.Duration) ticker
}

// timeTickerFactory wraps time.NewTicker to implement tickerFactory
type timeTickerFactory struct{}

func (f timeTickerFactory) NewTicker(d time.Duration) ticker {
	return &timeTicker{time.NewTicker(d)}
}

// setProcessGroup sets the process group for Unix systems
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}

// isProcessRunning checks if a process with the given PID is running on Unix systems.
// It uses syscall.Signal(0) which doesn't actually send a signal but checks if the process exists.
func (pm *ProcessManager) isProcessRunning(pid int) bool {
	return pm.isProcessRunningWithFinder(pid, osProcessFinder{})
}

// isProcessRunningWithFinder checks if a process is running using a provided processFinder (for testing)
func (pm *ProcessManager) isProcessRunningWithFinder(pid int, finder processFinder) bool {
	if pid == 0 {
		return false
	}

	process, err := finder.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// findProcessOnPort finds the PID of the process listening on the given port on Unix systems.
// It uses lsof to find the process ID.
func (pm *ProcessManager) findProcessOnPort(port int) int {
	return pm.findProcessOnPortWithFactory(port, realCommandExecutor{})
}

// findProcessOnPortWithFactory finds the PID using a provided commandFactory (for testing)
func (pm *ProcessManager) findProcessOnPortWithFactory(port int, executor CommandExecutor) int {
	if executor == nil {
		executor = realCommandExecutor{}
	}
	cmd := executor.Command("lsof", "-ti", fmt.Sprintf(":%d", port))
	output, err := executor.Output(cmd)
	if err != nil {
		return 0
	}

	var pid int
	if _, err := fmt.Sscanf(string(output), "%d", &pid); err != nil {
		return 0
	}

	return pid
}

// findOperatorProcess finds the PID of the running g8e operator process using pgrep.
// This is used as a fallback when the PID file is missing or stale.
func (pm *ProcessManager) findOperatorProcess() int {
	return pm.findOperatorProcessWithExecutor(realCommandExecutor{})
}

// findOperatorProcessWithExecutor finds the PID using a provided CommandExecutor (for testing)
func (pm *ProcessManager) findOperatorProcessWithExecutor(executor CommandExecutor) int {
	cmd := executor.Command("pgrep", "-f", "g8e gateway serve")
	output, err := executor.Output(cmd)
	if err != nil {
		return 0
	}

	var pid int
	if _, err := fmt.Sscanf(string(output), "%d", &pid); err != nil {
		return 0
	}

	return pid
}

// stopProcess stops a process with the given PID on Unix systems.
// It sends SIGTERM first, then SIGKILL if the process doesn't exit within the timeout.
func (pm *ProcessManager) stopProcess(pid int, name string) error {
	return pm.stopProcessWithDeps(pid, name, osProcessFinder{}, timeSleeper{}, timeTickerFactory{})
}

// stopProcessWithDeps stops a process using injected dependencies (for testing)
func (pm *ProcessManager) stopProcessWithDeps(pid int, name string, finder processFinder, sleep sleeper, tickerFactory tickerFactory) error {
	if pid == 0 {
		return nil
	}

	if !pm.isProcessRunningWithFinder(pid, finder) {
		return nil
	}

	process, err := finder.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	timeout := time.After(10 * time.Second)
	ticker := tickerFactory.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			if err := process.Signal(syscall.SIGKILL); err != nil {
				return fmt.Errorf("failed to send SIGKILL: %w", err)
			}
			// Wait for process to actually exit after SIGKILL
			for i := 0; i < 20; i++ {
				sleep.Sleep(100 * time.Millisecond)
				if !pm.isProcessRunningWithFinder(pid, finder) {
					return nil
				}
			}
			return fmt.Errorf("process %d did not exit after SIGKILL", pid)
		case <-ticker.C():
			if !pm.isProcessRunningWithFinder(pid, finder) {
				return nil
			}
		}
	}
}
