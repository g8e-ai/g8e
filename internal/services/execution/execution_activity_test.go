// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build !windows

package execution

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const (
	envFIFOReady = "G8E_TEST_FIFO_READY"
	envFIFOData  = "G8E_TEST_FIFO_DATA"

	// scriptBlockOnFIFO opens the read side of the data FIFO, announces readiness on
	// the ready FIFO, then blocks reading the data FIFO until its writers close.
	// The test holds the write side of both FIFOs, so every open succeeds without a
	// rendezvous and the readiness byte is the only synchronization point.
	scriptBlockOnFIFO = `exec 3< "$` + envFIFOData + `"; echo ready > "$` + envFIFOReady + `"; cat <&3`
)

func newTestExecutionService(t *testing.T) *ExecutionService {
	t.Helper()
	svc := NewExecutionService(testutil.NewTestConfig(t), testutil.NewTestLogger())
	t.Cleanup(svc.Stop)
	return svc
}

func shellRequest(executionID, script string) *models.ExecutionRequestPayload {
	return &models.ExecutionRequestPayload{
		ExecutionID:    executionID,
		CaseID:         "case-" + executionID,
		Command:        "sh",
		Args:           []string{"-c", script},
		TimeoutSeconds: 30,
	}
}

// fifoPair is a ready/data FIFO pair whose write sides are held open by the test.
type fifoPair struct {
	ready *os.File
	data  *os.File
	env   map[string]string
}

// newFIFOPair creates the FIFOs and opens them O_RDWR, which never blocks. The
// descriptors are closed on t.Cleanup, which also unblocks any pending read.
func newFIFOPair(t *testing.T) *fifoPair {
	t.Helper()
	dir := testutil.TempDir(t)

	open := func(name string) (string, *os.File) {
		path := filepath.Join(dir, name)
		require.NoError(t, syscall.Mkfifo(path, constants.PermFilePrivate))
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		require.NoError(t, err)
		t.Cleanup(func() { _ = f.Close() })
		return path, f
	}

	readyPath, ready := open("ready")
	dataPath, data := open("data")
	return &fifoPair{
		ready: ready,
		data:  data,
		env: map[string]string{
			envFIFOReady: readyPath,
			envFIFOData:  dataPath,
		},
	}
}

// executeAsync runs the request on its own goroutine and delivers the outcome on a
// channel. The goroutine is joined on t.Cleanup.
func executeAsync(t *testing.T, ctx context.Context, svc *ExecutionService, req *models.ExecutionRequestPayload) <-chan executeOutcome {
	t.Helper()
	out := make(chan executeOutcome, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, err := svc.ExecuteCommand(ctx, req)
		out <- executeOutcome{result: result, err: err}
	}()
	t.Cleanup(wg.Wait)
	return out
}

type executeOutcome struct {
	result *models.ExecutionResult
	err    error
}

// awaitReady blocks until the command writes its readiness byte, or fails the test
// if the command finishes first (for example because it failed to start).
func (p *fifoPair) awaitReady(t *testing.T, done <-chan executeOutcome) {
	t.Helper()
	signalled := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := p.ready.Read(make([]byte, 1))
		signalled <- err
	}()
	t.Cleanup(func() {
		_ = p.ready.Close()
		wg.Wait()
	})

	select {
	case err := <-signalled:
		require.NoError(t, err, "command did not signal readiness")
	case outcome := <-done:
		require.FailNow(t, "command finished before signalling readiness",
			"result=%+v err=%v", outcome.result, outcome.err)
	}
}

func TestExecutionService_ExecuteCommand_ShellSemantics(t *testing.T) {
	t.Parallel()
	svc := newTestExecutionService(t)

	tests := []struct {
		name       string
		id         string
		script     string
		environ    map[string]string
		setup      func(t *testing.T, dir string)
		wantStdout func(dir string) string
	}{
		{
			name:       "expands shell variables from the request environment",
			id:         "shell-semantics-variable",
			script:     `echo "$G8E_TEST_GREETING"`,
			environ:    map[string]string{"G8E_TEST_GREETING": "hello"},
			wantStdout: func(string) string { return "hello\n" },
		},
		{
			name:       "expands tilde to the request HOME",
			id:         "shell-semantics-tilde",
			script:     `echo ~`,
			wantStdout: func(dir string) string { return dir + "\n" },
		},
		{
			name:       "connects commands with pipes",
			id:         "shell-semantics-pipe",
			script:     `echo hello world | wc -w | tr -d ' '`,
			wantStdout: func(string) string { return "2\n" },
		},
		{
			name:   "expands globs against the working directory",
			id:     "shell-semantics-glob",
			script: `echo *.conf`,
			setup: func(t *testing.T, dir string) {
				for _, name := range []string{"a.conf", "b.conf", "c.txt"} {
					require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, constants.PermFilePrivate))
				}
			},
			wantStdout: func(string) string { return "a.conf b.conf\n" },
		},
		{
			name:       "gives the command a closed stdin so reads fail fast",
			id:         "shell-semantics-stdin",
			script:     `read input || echo stdin-eof`,
			wantStdout: func(string) string { return "stdin-eof\n" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := testutil.TempDir(t)
			if tt.setup != nil {
				tt.setup(t, dir)
			}

			req := shellRequest(tt.id, tt.script)
			req.WorkingDirectory = &dir
			req.Environment = map[string]string{"HOME": dir}
			for k, v := range tt.environ {
				req.Environment[k] = v
			}

			result, err := svc.ExecuteCommand(t.Context(), req)

			require.NoError(t, err)
			assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status)
			assert.Equal(t, constants.ExitCodeSuccess, result.ReturnCode)
			assert.Equal(t, tt.wantStdout(dir), result.Stdout)
		})
	}
}

func TestExecutionService_ExecuteCommand_DeliversOutputProducedAfterInitialSilence(t *testing.T) {
	t.Parallel()
	svc := newTestExecutionService(t)
	fifos := newFIFOPair(t)

	req := shellRequest("output-after-silence", scriptBlockOnFIFO)
	req.Environment = fifos.env
	done := executeAsync(t, t.Context(), svc, req)

	fifos.awaitReady(t, done)
	_, err := fifos.data.WriteString("Done waiting\n")
	require.NoError(t, err)
	require.NoError(t, fifos.data.Close())

	outcome := <-done
	require.NoError(t, outcome.err)
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, outcome.result.Status)
	assert.Equal(t, "Done waiting\n", outcome.result.Stdout)
}

func TestExecutionService_ExecuteCommand_TimesOutCommandBlockedOnInput(t *testing.T) {
	t.Parallel()
	svc := newTestExecutionService(t)
	fifos := newFIFOPair(t)

	// The data FIFO's writer stays open, so the command can only end by being killed.
	req := shellRequest("blocked-command-timeout", scriptBlockOnFIFO)
	req.Environment = fifos.env
	req.TimeoutSeconds = 1

	result, err := svc.ExecuteCommand(t.Context(), req)

	require.NoError(t, err)
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT, result.Status)
	assert.Equal(t, constants.ExitCodeTimeout, result.ReturnCode)
	assert.Equal(t, constants.ErrMessageExecutionTimeout, result.ErrorMessage)
}

func TestExecutionService_ExecuteCommand_StopsBlockedCommandWhenContextCancelled(t *testing.T) {
	t.Parallel()
	svc := newTestExecutionService(t)
	fifos := newFIFOPair(t)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	req := shellRequest("blocked-command-cancel", scriptBlockOnFIFO)
	req.Environment = fifos.env
	done := executeAsync(t, ctx, svc, req)

	fifos.awaitReady(t, done)
	cancel()

	outcome := <-done
	require.NoError(t, outcome.err)
	assert.NotEqual(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT, outcome.result.Status,
		"cancellation is not a deadline expiry")
	assert.NotEqual(t, constants.ExitCodeSuccess, outcome.result.ReturnCode,
		"a killed command must not report success")
}
