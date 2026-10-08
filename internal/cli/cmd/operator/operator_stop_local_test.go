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
	"testing"
	"testing/synctest"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestStopLocalOperatorEscalation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		exits   []bool
		signals []bool
		method  string
	}{
		{"governed exit", []bool{true}, nil, "exited"},
		{"TERM exit", []bool{false, true}, []bool{false}, "TERM"},
		{"KILL exit", []bool{false, false, true}, []bool{false, true}, "KILL"},
		{"KILL timeout", []bool{false, false, false}, []bool{false, true}, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var signals []bool
			checks := 0
			p := localOperatorProcess{pid: 123, wait: func(time.Duration) (bool, error) { v := tc.exits[checks]; checks++; return v, nil }, signal: func(force bool) error { signals = append(signals, force); return nil }}
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.SetErr(&bytes.Buffer{})
			result := stopLocalOperator(cmd, p, models.StopOperatorResponse{}, nil, time.Millisecond)
			require.Equal(t, tc.signals, signals)
			require.Equal(t, tc.method, result.Method)
			require.Equal(t, tc.method != "failed", result.Success)
		})
	}
}

func TestOperatorStopLocalWithoutGateway(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")
	client := &cmdtest.MockAPIClient{GetErr: errors.New("gateway offline")}
	var signals []bool
	checks := 0
	closed := false
	p := localOperatorProcess{pid: 123, wait: func(time.Duration) (bool, error) { checks++; return checks == 3, nil }, signal: func(force bool) error { signals = append(signals, force); return nil }, close: func() { closed = true }}
	cmd := operatorStopCmdWithLocal(cmdtest.ConfigLoaderFor(cfg), authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc), func() ([]localOperatorProcess, error) { return []localOperatorProcess{p}, nil })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--grace=0s"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, []bool{false, true}, signals)
	require.True(t, closed)
}

func TestOperatorStopLocal_BoundedConcurrencyAndAccounting(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")
	client := &cmdtest.MockAPIClient{GetErr: errors.New("gateway offline")}
	synctest.Test(t, func(t *testing.T) {
		const count = 70
		started := make(chan int, count)
		release := make(chan struct{})
		locals := make([]localOperatorProcess, count)
		for i := range locals {
			locals[i] = localOperatorProcess{pid: i + 1, close: func() {}, wait: func(time.Duration) (bool, error) {
				started <- i
				<-release
				return true, nil
			}}
		}
		cmd := operatorStopCmdWithLocal(cmdtest.ConfigLoaderFor(cfg), authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc), func() ([]localOperatorProcess, error) { return locals, nil })
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(nil)
		done := make(chan error, 1)
		go func() { done <- cmd.Execute() }()
		synctest.Wait()
		inFlight := len(started)
		close(release)
		require.NoError(t, <-done)
		require.Equal(t, 64, inFlight, "bulk stop must overlap waits with a bounded worker count")
		require.Len(t, started, count, "every discovered worker must be accounted for")
		for i := range locals {
			require.Contains(t, out.String(), fmt.Sprintf("Local operator PID %d: exited", i+1))
		}
	})
}

func TestOperatorStopLocal_CancellationAccountsForQueuedWorkers(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")
	client := &cmdtest.MockAPIClient{GetErr: errors.New("gateway offline")}
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		started := make(chan int, 70)
		release := make(chan struct{})
		locals := make([]localOperatorProcess, 70)
		for i := range locals {
			locals[i] = localOperatorProcess{pid: i + 1, close: func() {}, wait: func(time.Duration) (bool, error) {
				started <- i
				<-release
				return false, nil
			}, signal: func(bool) error { panic("cancellation must prevent signals") }}
		}
		cmd := operatorStopCmdWithLocal(cmdtest.ConfigLoaderFor(cfg), authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc), func() ([]localOperatorProcess, error) { return locals, nil })
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.PersistentFlags().Bool("json", true, "")
		cmd.SetArgs(nil)
		done := make(chan error, 1)
		go func() { done <- cmd.ExecuteContext(ctx) }()
		synctest.Wait()
		cancel()
		close(release)
		require.Error(t, <-done)
		require.Len(t, started, 64, "queued workers must not start after cancellation")
		var results []operatorStopResult
		require.NoError(t, json.Unmarshal(out.Bytes(), &results))
		require.Len(t, results, len(locals))
		for i, result := range results {
			require.Equal(t, i+1, result.PID)
			require.False(t, result.Success)
			require.Equal(t, "failed", result.Method)
			require.NotEmpty(t, result.Error)
		}
	})
}

func TestStopLocalOperatorSignalFailure(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	result := stopLocalOperator(cmd, localOperatorProcess{pid: 123, wait: func(time.Duration) (bool, error) { return false, nil }, signal: func(bool) error { return errors.New("permission denied") }}, models.StopOperatorResponse{}, nil, 0)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "permission denied")
}

func TestTargetedStopFallsBackAfterShutdownFailure(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: []*operatorv1.OperatorDocument{{Id: "op-a", OperatorSessionId: "session-a", OperatorType: string(constants.OperatorTypeRemote)}}})
	require.NoError(t, err)
	client := &cmdtest.MockAPIClient{GetResp: body, PostErr: errors.New("shutdown channel disconnected")}
	checks := 0
	var signals []bool
	p := localOperatorProcess{pid: 123, sessionID: "session-a", wait: func(time.Duration) (bool, error) { checks++; return checks == 2, nil }, signal: func(force bool) error { signals = append(signals, force); return nil }, close: func() {}}
	cmd := operatorStopCmdWithLocal(cmdtest.ConfigLoaderFor(cfg), authcmd.MockClientFactory(client), cmdtest.FileSvcFactoryFor(fileSvc), func() ([]localOperatorProcess, error) { return []localOperatorProcess{p}, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"session-a", "--grace=0s"})
	require.NoError(t, cmd.Execute())
	require.Len(t, client.PostCalls, 1)
	require.Equal(t, []bool{false}, signals)
	require.Contains(t, out.String(), "Stopped local operator PID 123 (TERM)")
}
