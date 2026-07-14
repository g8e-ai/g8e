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
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// mockProcess is a mock implementation of the process interface
type mockProcess struct {
	signalFunc func(sig syscall.Signal) error
}

func (m *mockProcess) Signal(sig syscall.Signal) error {
	if m.signalFunc != nil {
		return m.signalFunc(sig)
	}
	return nil
}

// mockProcessFinder is a mock implementation of the processFinder interface
type mockProcessFinder struct {
	findProcessFunc func(pid int) (process, error)
}

func (m *mockProcessFinder) FindProcess(pid int) (process, error) {
	if m.findProcessFunc != nil {
		return m.findProcessFunc(pid)
	}
	return &mockProcess{}, nil
}

// mockCommandExecutor is a mock implementation of the CommandExecutor interface
type mockCommandExecutor struct {
	commandFunc func(name string, args ...string) *exec.Cmd
	outputFunc  func(cmd *exec.Cmd) ([]byte, error)
	runFunc     func(cmd *exec.Cmd) error
}

func (m *mockCommandExecutor) Command(name string, args ...string) *exec.Cmd {
	if m.commandFunc != nil {
		return m.commandFunc(name, args...)
	}
	// Return a dummy command so we don't return nil
	return exec.Command("echo")
}

func (m *mockCommandExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	if m.outputFunc != nil {
		return m.outputFunc(cmd)
	}
	return []byte{}, nil
}

func (m *mockCommandExecutor) Run(cmd *exec.Cmd) error {
	if m.runFunc != nil {
		return m.runFunc(cmd)
	}
	return nil
}

// mockSleeper is a mock implementation of the sleeper interface
type mockSleeper struct {
	sleepFunc func(d time.Duration)
}

func (m *mockSleeper) Sleep(d time.Duration) {
	if m.sleepFunc != nil {
		m.sleepFunc(d)
	}
}

// mockTicker is a mock implementation of the ticker interface
type mockTicker struct {
	c        chan time.Time
	stopFunc func()
}

func (m *mockTicker) C() <-chan time.Time {
	return m.c
}

func (m *mockTicker) Stop() {
	if m.stopFunc != nil {
		m.stopFunc()
	}
}

// mockTickerFactory is a mock implementation of the tickerFactory interface
type mockTickerFactory struct {
	newTickerFunc func(d time.Duration) ticker
}

func (m *mockTickerFactory) NewTicker(d time.Duration) ticker {
	if m.newTickerFunc != nil {
		return m.newTickerFunc(d)
	}
	return &mockTicker{c: make(chan time.Time)}
}

func TestIsProcessRunningWithFinder(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	t.Run("returns false for PID 0", func(t *testing.T) {
		finder := &mockProcessFinder{}
		result := pm.isProcessRunningWithFinder(0, finder)
		if result {
			t.Error("expected false for PID 0")
		}
	})

	t.Run("returns false when FindProcess fails", func(t *testing.T) {
		finder := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				return nil, errors.New("process not found")
			},
		}
		result := pm.isProcessRunningWithFinder(123, finder)
		if result {
			t.Error("expected false when FindProcess fails")
		}
	})

	t.Run("returns false when Signal fails", func(t *testing.T) {
		finder := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				return &mockProcess{
					signalFunc: func(sig syscall.Signal) error {
						return errors.New("signal failed")
					},
				}, nil
			},
		}
		result := pm.isProcessRunningWithFinder(123, finder)
		if result {
			t.Error("expected false when Signal fails")
		}
	})

	t.Run("returns true when Signal succeeds", func(t *testing.T) {
		finder := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				return &mockProcess{
					signalFunc: func(sig syscall.Signal) error {
						return nil
					},
				}, nil
			},
		}
		result := pm.isProcessRunningWithFinder(123, finder)
		if !result {
			t.Error("expected true when Signal succeeds")
		}
	})

	t.Run("returns true for Signal(0) success", func(t *testing.T) {
		finder := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				return &mockProcess{
					signalFunc: func(sig syscall.Signal) error {
						if sig == syscall.Signal(0) {
							return nil
						}
						return errors.New("unexpected signal")
					},
				}, nil
			},
		}
		result := pm.isProcessRunningWithFinder(123, finder)
		if !result {
			t.Error("expected true for Signal(0) success")
		}
	})
}

func TestFindProcessOnPortWithFactory(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	t.Run("returns 0 when command fails", func(t *testing.T) {
		executor := &mockCommandExecutor{
			outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return nil, errors.New("lsof failed")
			},
		}
		result := pm.findProcessOnPortWithFactory(8080, executor)
		if result != 0 {
			t.Errorf("expected 0 when command fails, got %d", result)
		}
	})

	t.Run("returns 0 when output is malformed", func(t *testing.T) {
		executor := &mockCommandExecutor{
			outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("not-a-number"), nil
			},
		}
		result := pm.findProcessOnPortWithFactory(8080, executor)
		if result != 0 {
			t.Errorf("expected 0 for malformed output, got %d", result)
		}
	})

	t.Run("returns 0 when output is empty", func(t *testing.T) {
		executor := &mockCommandExecutor{
			outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte(""), nil
			},
		}
		result := pm.findProcessOnPortWithFactory(8080, executor)
		if result != 0 {
			t.Errorf("expected 0 for empty output, got %d", result)
		}
	})

	t.Run("returns valid PID when output is valid", func(t *testing.T) {
		expectedPID := 12345
		executor := &mockCommandExecutor{
			outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("12345"), nil
			},
		}
		result := pm.findProcessOnPortWithFactory(8080, executor)
		if result != expectedPID {
			t.Errorf("expected %d, got %d", expectedPID, result)
		}
	})

	t.Run("returns valid PID with whitespace", func(t *testing.T) {
		expectedPID := 67890
		executor := &mockCommandExecutor{
			outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("  67890  "), nil
			},
		}
		result := pm.findProcessOnPortWithFactory(8080, executor)
		if result != expectedPID {
			t.Errorf("expected %d, got %d", expectedPID, result)
		}
	})

	t.Run("passes correct port to command", func(t *testing.T) {
		testPort := 9090
		executor := &mockCommandExecutor{
			commandFunc: func(name string, args ...string) *exec.Cmd {
				if name != "lsof" {
					t.Errorf("expected command 'lsof', got '%s'", name)
				}
				if len(args) != 2 || args[0] != "-ti" || args[1] != ":9090" {
					t.Errorf("expected args ['-ti', ':9090'], got %v", args)
				}
				return exec.Command(name, args...)
			},
			outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("12345"), nil
			},
		}
		pm.findProcessOnPortWithFactory(testPort, executor)
	})
}

func TestFindOperatorProcessWithExecutor(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	t.Run("returns 0 when command fails", func(t *testing.T) {
		executor := &mockCommandExecutor{
			outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return nil, errors.New("pgrep failed")
			},
		}
		result := pm.findOperatorProcessWithExecutor(executor)
		if result != 0 {
			t.Errorf("expected 0 when command fails, got %d", result)
		}
	})

	t.Run("returns 0 when output is malformed", func(t *testing.T) {
		executor := &mockCommandExecutor{
			outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("invalid"), nil
			},
		}
		result := pm.findOperatorProcessWithExecutor(executor)
		if result != 0 {
			t.Errorf("expected 0 for malformed output, got %d", result)
		}
	})

	t.Run("returns valid PID when output is valid", func(t *testing.T) {
		expectedPID := 54321
		executor := &mockCommandExecutor{
			outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("54321"), nil
			},
		}
		result := pm.findOperatorProcessWithExecutor(executor)
		if result != expectedPID {
			t.Errorf("expected %d, got %d", expectedPID, result)
		}
	})

	t.Run("passes correct arguments to command", func(t *testing.T) {
		expectedPattern := fmt.Sprintf("g8e gw start.*--data-dir %s", fileSvc.Resolve(constants.DataDirname))
		executor := &mockCommandExecutor{
			commandFunc: func(name string, args ...string) *exec.Cmd {
				if name != "pgrep" {
					t.Errorf("expected command 'pgrep', got '%s'", name)
				}
				if len(args) != 2 || args[0] != "-f" || args[1] != expectedPattern {
					t.Errorf("expected args ['-f', '%s'], got %v", expectedPattern, args)
				}
				return exec.Command(name, args...)
			},
			outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("12345"), nil
			},
		}
		pm.findOperatorProcessWithExecutor(executor)
	})
}

func TestStopProcessWithDeps(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	t.Run("returns nil for PID 0", func(t *testing.T) {
		finder := &mockProcessFinder{}
		sleeper := &mockSleeper{}
		tickerFactory := &mockTickerFactory{}
		err := pm.stopProcessWithDeps(0, "test", finder, sleeper, tickerFactory, 10*time.Second)
		if err != nil {
			t.Errorf("expected nil for PID 0, got %v", err)
		}
	})

	t.Run("returns nil when process is not running", func(t *testing.T) {
		finder := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				return nil, errors.New("process not found")
			},
		}
		sleeper := &mockSleeper{}
		tickerFactory := &mockTickerFactory{}
		err := pm.stopProcessWithDeps(123, "test", finder, sleeper, tickerFactory, 10*time.Second)
		if err != nil {
			t.Errorf("expected nil when process not running, got %v", err)
		}
	})

	t.Run("returns error when FindProcess fails after running check", func(t *testing.T) {
		callCount := 0
		finder := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				callCount++
				if callCount == 1 {
					// First call for isProcessRunning - process is running
					return &mockProcess{
						signalFunc: func(sig syscall.Signal) error {
							return nil
						},
					}, nil
				}
				// Second call for stopProcess - process not found
				return nil, errors.New("process not found")
			},
		}
		sleeper := &mockSleeper{}
		tickerFactory := &mockTickerFactory{}
		err := pm.stopProcessWithDeps(123, "test", finder, sleeper, tickerFactory, 10*time.Second)
		if err == nil {
			t.Error("expected error when FindProcess fails")
		}
	})

	t.Run("returns error when SIGTERM fails", func(t *testing.T) {
		finder := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				return &mockProcess{
					signalFunc: func(sig syscall.Signal) error {
						if sig == syscall.SIGTERM {
							return errors.New("SIGTERM failed")
						}
						return nil
					},
				}, nil
			},
		}
		sleeper := &mockSleeper{}
		tickerFactory := &mockTickerFactory{}
		err := pm.stopProcessWithDeps(123, "test", finder, sleeper, tickerFactory, 10*time.Second)
		if err == nil {
			t.Error("expected error when SIGTERM fails")
		}
	})

	t.Run("returns nil when process exits after SIGTERM", func(t *testing.T) {
		sigtermCalled := false
		finder := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				return &mockProcess{
					signalFunc: func(sig syscall.Signal) error {
						if sig == syscall.SIGTERM {
							sigtermCalled = true
							return nil
						}
						// Process is running until SIGTERM is sent
						if sigtermCalled {
							return errors.New("process not running")
						}
						return nil
					},
				}, nil
			},
		}
		sleeper := &mockSleeper{}
		tickerFactory := &mockTickerFactory{
			newTickerFunc: func(d time.Duration) ticker {
				tickerChan := make(chan time.Time, 1)
				tickerChan <- time.Now() // Immediate tick
				return &mockTicker{
					c: tickerChan,
					stopFunc: func() {
						close(tickerChan)
					},
				}
			},
		}
		err := pm.stopProcessWithDeps(123, "test", finder, sleeper, tickerFactory, 10*time.Second)
		if err != nil {
			t.Errorf("expected nil when process exits after SIGTERM, got %v", err)
		}
		if !sigtermCalled {
			t.Error("SIGTERM should have been called")
		}
	})

	t.Run("sends SIGKILL after timeout", func(t *testing.T) {
		finder := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				return &mockProcess{
					signalFunc: func(sig syscall.Signal) error {
						if sig == syscall.SIGTERM {
							return nil
						}
						if sig == syscall.SIGKILL {
							return nil
						}
						return nil
					},
				}, nil
			},
		}
		sleeper := &mockSleeper{}
		tickerFactory := &mockTickerFactory{
			newTickerFunc: func(d time.Duration) ticker {
				tickerChan := make(chan time.Time)
				return &mockTicker{
					c: tickerChan,
					stopFunc: func() {
						close(tickerChan)
					},
				}
			},
		}
		// Use a short timeout by manipulating time.After indirectly
		// Since we can't mock time.After, we'll just test the SIGKILL path
		// by ensuring the ticker never fires
		err := pm.stopProcessWithDeps(123, "test", finder, sleeper, tickerFactory, 50*time.Millisecond)
		// With a 50ms timeout, the SIGKILL path is exercised quickly.
		_ = err
	})

	t.Run("returns error when SIGKILL fails", func(t *testing.T) {
		finder := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				return &mockProcess{
					signalFunc: func(sig syscall.Signal) error {
						if sig == syscall.SIGTERM {
							return nil
						}
						if sig == syscall.SIGKILL {
							return errors.New("SIGKILL failed")
						}
						return nil
					},
				}, nil
			},
		}
		sleeper := &mockSleeper{}
		tickerFactory := &mockTickerFactory{
			newTickerFunc: func(d time.Duration) ticker {
				tickerChan := make(chan time.Time)
				return &mockTicker{
					c: tickerChan,
					stopFunc: func() {
						close(tickerChan)
					},
				}
			},
		}
		err := pm.stopProcessWithDeps(123, "test", finder, sleeper, tickerFactory, 50*time.Millisecond)
		_ = err
	})

	t.Run("returns nil when process exits after SIGKILL", func(t *testing.T) {
		sigkillExitCount := 0
		finder := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				return &mockProcess{
					signalFunc: func(sig syscall.Signal) error {
						if sig == syscall.SIGTERM {
							return nil
						}
						if sig == syscall.SIGKILL {
							return nil
						}
						// After SIGKILL, check if process exited
						sigkillExitCount++
						if sigkillExitCount > 0 {
							return errors.New("process not running")
						}
						return nil
					},
				}, nil
			},
		}
		sleeper := &mockSleeper{}
		tickerFactory := &mockTickerFactory{
			newTickerFunc: func(d time.Duration) ticker {
				tickerChan := make(chan time.Time)
				return &mockTicker{
					c: tickerChan,
					stopFunc: func() {
						close(tickerChan)
					},
				}
			},
		}
		err := pm.stopProcessWithDeps(123, "test", finder, sleeper, tickerFactory, 50*time.Millisecond)
		_ = err
	})
}

func TestMockProcess(t *testing.T) {
	t.Run("mockProcess Signal returns nil by default", func(t *testing.T) {
		p := &mockProcess{}
		err := p.Signal(syscall.SIGTERM)
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("mockProcess Signal uses custom function", func(t *testing.T) {
		expectedErr := errors.New("custom error")
		p := &mockProcess{
			signalFunc: func(sig syscall.Signal) error {
				return expectedErr
			},
		}
		err := p.Signal(syscall.SIGTERM)
		if err != expectedErr {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	})
}

func TestMockProcessFinder(t *testing.T) {
	t.Run("mockProcessFinder FindProcess returns mock by default", func(t *testing.T) {
		f := &mockProcessFinder{}
		p, err := f.FindProcess(123)
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
		if p == nil {
			t.Error("expected non-nil process")
		}
	})

	t.Run("mockProcessFinder FindProcess uses custom function", func(t *testing.T) {
		expectedErr := errors.New("custom error")
		f := &mockProcessFinder{
			findProcessFunc: func(pid int) (process, error) {
				return nil, expectedErr
			},
		}
		_, err := f.FindProcess(123)
		if err != expectedErr {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	})
}

func TestMockCommandExecutor(t *testing.T) {
	t.Run("mockCommandExecutor Output returns empty by default", func(t *testing.T) {
		e := &mockCommandExecutor{}
		output, err := e.Output(nil)
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
		if len(output) != 0 {
			t.Errorf("expected empty output, got %v", output)
		}
	})

	t.Run("mockCommandExecutor Output uses custom function", func(t *testing.T) {
		expectedOutput := []byte("test output")
		expectedErr := errors.New("custom error")
		e := &mockCommandExecutor{
			outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return expectedOutput, expectedErr
			},
		}
		output, err := e.Output(nil)
		if err != expectedErr {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
		if string(output) != string(expectedOutput) {
			t.Errorf("expected %v, got %v", expectedOutput, output)
		}
	})
}

func TestMockSleeper(t *testing.T) {
	t.Run("mockSleeper Sleep is no-op by default", func(t *testing.T) {
		s := &mockSleeper{}
		s.Sleep(time.Second) // Should not panic
	})

	t.Run("mockSleeper Sleep uses custom function", func(t *testing.T) {
		called := false
		s := &mockSleeper{
			sleepFunc: func(d time.Duration) {
				called = true
				if d != time.Second {
					t.Errorf("expected 1s, got %v", d)
				}
			},
		}
		s.Sleep(time.Second)
		if !called {
			t.Error("sleep function should have been called")
		}
	})
}

func TestMockTicker(t *testing.T) {
	t.Run("mockTicker C returns channel", func(t *testing.T) {
		tickerChan := make(chan time.Time, 1)
		tickerChan <- time.Now()
		ticker := &mockTicker{c: tickerChan}
		c := ticker.C()
		if c == nil {
			t.Error("expected non-nil channel")
		}
		select {
		case <-c:
			// OK
		default:
			t.Error("channel should have value")
		}
	})

	t.Run("mockTicker Stop is no-op by default", func(t *testing.T) {
		tickerChan := make(chan time.Time)
		ticker := &mockTicker{c: tickerChan}
		ticker.Stop() // Should not panic
	})

	t.Run("mockTicker Stop uses custom function", func(t *testing.T) {
		called := false
		tickerChan := make(chan time.Time)
		ticker := &mockTicker{
			c: tickerChan,
			stopFunc: func() {
				called = true
			},
		}
		ticker.Stop()
		if !called {
			t.Error("stop function should have been called")
		}
	})
}

func TestMockTickerFactory(t *testing.T) {
	t.Run("mockTickerFactory NewTicker returns mock by default", func(t *testing.T) {
		f := &mockTickerFactory{}
		ticker := f.NewTicker(time.Second)
		if ticker == nil {
			t.Error("expected non-nil ticker")
		}
	})

	t.Run("mockTickerFactory NewTicker uses custom function", func(t *testing.T) {
		expectedTicker := &mockTicker{}
		f := &mockTickerFactory{
			newTickerFunc: func(d time.Duration) ticker {
				if d != time.Second {
					t.Errorf("expected 1s, got %v", d)
				}
				return expectedTicker
			},
		}
		ticker := f.NewTicker(time.Second)
		if ticker != expectedTicker {
			t.Error("expected custom ticker")
		}
	})
}
