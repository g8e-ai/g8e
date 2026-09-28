// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

const (
	defaultRunPollInterval = 2 * time.Second
	runLogFilenamePrefix   = "execution-"
)

// processControl is the process boundary a run lease depends on: who this
// process is, whether another is alive, and how to ask it to stop.
type processControl interface {
	PID() int
	Hostname() (string, error)
	Alive(pid int) bool
	Interrupt(pid int) error
}

// runControlDeps carries the process boundary and polling cadence used by
// commands that start, watch, or stop a run.
type runControlDeps struct {
	processFactory func(fs.RuntimeFileService) (processControl, error)
	// notifyInterrupts registers for interrupt signals and returns the channel
	// and the function that unregisters it.
	notifyInterrupts func() (<-chan os.Signal, func())
	pollInterval     time.Duration
}

// notifyProcessInterrupts registers for SIGINT and SIGTERM.
func notifyProcessInterrupts() (<-chan os.Signal, func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	return signals, func() { signal.Stop(signals) }
}

func defaultRunControlDeps() runControlDeps {
	return runControlDeps{
		processFactory:   newOSProcessControl,
		notifyInterrupts: notifyProcessInterrupts,
	}
}

func (d runControlDeps) interrupts() (<-chan os.Signal, func()) {
	if d.notifyInterrupts == nil {
		return nil, func() {}
	}
	return d.notifyInterrupts()
}

func (d runControlDeps) process(fileSvc fs.RuntimeFileService) (processControl, error) {
	if d.processFactory == nil {
		return nil, fmt.Errorf("evaluation: run control: %w", constants.ErrMissingRequiredField)
	}
	return d.processFactory(fileSvc)
}

func (d runControlDeps) interval() time.Duration {
	if d.pollInterval > 0 {
		return d.pollInterval
	}
	return defaultRunPollInterval
}

type osProcessControl struct {
	manager *platform.ProcessManager
}

func newOSProcessControl(fileSvc fs.RuntimeFileService) (processControl, error) {
	manager, err := platform.NewProcessManager(fileSvc)
	if err != nil {
		return nil, fmt.Errorf("evaluation: run control: %w", err)
	}
	return osProcessControl{manager: manager}, nil
}

func (p osProcessControl) PID() int { return os.Getpid() }

func (p osProcessControl) Hostname() (string, error) { return os.Hostname() }

func (p osProcessControl) Alive(pid int) bool { return p.manager.IsProcessRunning(pid) }

// Interrupt asks the process to stop after its current assignment. Platforms
// that cannot deliver the signal rely on the cancel request in the lease.
func (p osProcessControl) Interrupt(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(os.Interrupt)
}

// leaseLiveness reports a lease as live only when this host holds it and its
// process is running. A lease from another host, or whose process is gone, is
// stale.
func leaseLiveness(control processControl) evaluation.LeaseLiveness {
	host, hostErr := control.Hostname()
	return func(lease evaluation.RunLease) bool {
		if hostErr != nil || lease.Host != host {
			return false
		}
		return control.Alive(lease.PID)
	}
}

// runLogPath is the log file of one execution of a run.
func runLogPath(runID string, startedAt time.Time) (string, error) {
	dir := evaluation.QueueLogDir(runID)
	if dir == "" {
		return "", fmt.Errorf("evaluation: run log: invalid run ID %q", runID)
	}
	return path.Join(dir, fmt.Sprintf("%s%d%s", runLogFilenamePrefix, startedAt.Unix(), constants.FileExtText)), nil
}

// latestRunLogPath returns the newest execution log of a run, or "" when none
// exists.
func latestRunLogPath(ctx context.Context, fileSvc fs.RuntimeFileService, runID string) (string, error) {
	dir := evaluation.QueueLogDir(runID)
	if dir == "" {
		return "", fmt.Errorf("evaluation: run log: invalid run ID %q", runID)
	}
	entries, err := fileSvc.ReadDir(ctx, dir)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("evaluation: run log: %w", err)
	}
	latest := ""
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() > latest {
			latest = entry.Name()
		}
	}
	if latest == "" {
		return "", nil
	}
	return path.Join(dir, latest), nil
}

// runStopWatcher observes the two ways an executing run is asked to stop: a
// cancel request recorded on its lease and an interrupt signal. The first
// request lets the current assignment finish. A second signal cancels the
// running assignment through the context.
type runStopWatcher struct {
	requested atomic.Bool
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// startRunStopWatcher starts the watcher goroutine. hardCancel is called on a
// second interrupt. The caller owns the watcher and must call Stop.
func startRunStopWatcher(ctx context.Context, store *evaluation.Store, runID string, poll time.Duration, signals <-chan os.Signal, hardCancel context.CancelFunc) *runStopWatcher {
	watchCtx, cancel := context.WithCancel(ctx)
	watcher := &runStopWatcher{cancel: cancel}
	watcher.wg.Add(1)
	go func() {
		defer watcher.wg.Done()
		ticker := time.NewTicker(poll)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-signals:
				if watcher.requested.Swap(true) {
					hardCancel()
					return
				}
			case <-ticker.C:
				if lease, err := store.LoadRunLease(watchCtx, runID); err == nil && lease.CancelRequested() {
					watcher.requested.Store(true)
				}
			}
		}
	}()
	return watcher
}

// Requested reports whether the run was asked to stop.
func (w *runStopWatcher) Requested() bool { return w != nil && w.requested.Load() }

// Stop ends the watcher and waits for its goroutine.
func (w *runStopWatcher) Stop() {
	if w == nil {
		return
	}
	w.cancel()
	w.wg.Wait()
}

// runLog tees a command's output into the run's execution log.
type runLog struct {
	path    string
	file    *os.File
	restore func()
}

// openRunLog creates the run's execution log and redirects the command's
// output and error streams into it as well as their original destinations.
func openRunLog(cmd *cobra.Command, fileSvc fs.RuntimeFileService, runID string, startedAt time.Time) (*runLog, error) {
	logPath, err := runLogPath(runID, startedAt)
	if err != nil {
		return nil, err
	}
	file, err := fileSvc.OpenForAppend(cmd.Context(), logPath, constants.PermFilePrivate)
	if err != nil {
		return nil, fmt.Errorf("evaluation: open run log: %w", err)
	}
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	cmd.SetOut(io.MultiWriter(out, file))
	cmd.SetErr(io.MultiWriter(errOut, file))
	return &runLog{path: logPath, file: file, restore: func() {
		cmd.SetOut(out)
		cmd.SetErr(errOut)
	}}, nil
}

// close restores the command's original streams and closes the log.
func (l *runLog) close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.restore()
	return l.file.Close()
}
