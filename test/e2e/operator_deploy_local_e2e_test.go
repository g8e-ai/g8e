// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e && linux

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type deploymentLifecycleReport struct {
	Size         int                `json:"size"`
	BinarySHA256 string             `json:"binary_sha256"`
	DeploySecs   map[string]float64 `json:"deploy_seconds"`
	Passed       bool               `json:"passed"`
}

// This opt-in scenario owns real CLI-launched processes. Require an explicitly
// selected Gateway runtime and binary; never silently use a developer's stack.
// It runs at small size in CI and accepts the existing fleet size input for a
// separately scheduled scale qualification. Cleanup also covers unregistered
// workers left behind by cancellation, and never uses a global operator stop.
func TestOperatorDeploy_LocalLifecycle(t *testing.T) {
	require.NotEmpty(t, os.Getenv(e2eRuntimeRootEnv), "select an isolated Gateway runtime explicitly")
	n := requireFleetSize(t)
	require.LessOrEqual(t, n, 4997, "reserve directories for append, denial, and cancellation cases")
	bin := requireFleetEnv(t, fleetBinEnv)
	bin, err := filepath.Abs(bin)
	require.NoError(t, err)
	binary, err := os.ReadFile(bin)
	require.NoError(t, err)
	report := deploymentLifecycleReport{Size: n, BinarySHA256: fmt.Sprintf("%x", sha256.Sum256(binary)), DeploySecs: map[string]float64{}}
	t.Cleanup(func() {
		report.Passed = !t.Failed()
		if path := os.Getenv(fleetReportEnv); path != "" {
			data, err := json.MarshalIndent(report, "", "  ")
			if assert.NoError(t, err) {
				assert.NoError(t, os.WriteFile(path, append(data, '\n'), 0o600))
			}
		}
	})
	root := testutil.TempDir(t)
	home := testutil.TempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { stopDeploymentProcesses(t, root) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	before, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err)
	decisions := deploymentDecisionCount(t)

	deploy := func(phase string, count, start int, approve bool) []models.OperatorDeploymentState {
		t.Helper()
		args := []string{"operator", "deploy", "--local", "--dest-dir", root,
			"--count", strconv.Itoa(count), "--start-index", strconv.Itoa(start),
			"--parallel", strconv.Itoa(min(count, 100)), "--background", "--roles", "data",
			"--endpoint", e2eCfg.cfg.Paths.Host}
		if approve {
			args = append(args, "--approve")
		}
		started := time.Now()
		output, err := deploymentCLI(ctx, bin, args...)
		report.DeploySecs[phase] = time.Since(started).Seconds()
		t.Logf("%s: count=%d wall=%.3fs", phase, count, report.DeploySecs[phase])
		require.NoError(t, err, "%s: %s", phase, output)
		states := make([]models.OperatorDeploymentState, count)
		launches := map[string]bool{}
		for i := range states {
			dir := filepath.Join(root, fmt.Sprintf("op-%05d", start+i))
			states[i] = deploymentState(t, dir)
			require.NotEmpty(t, states[i].LaunchID)
			require.False(t, launches[states[i].LaunchID], "launch IDs must be unique")
			launches[states[i].LaunchID] = true
			if approve {
				require.Equal(t, models.OperatorDeploymentPhaseReady, states[i].Phase, "CLI completion requires command-subscription readiness")
				require.NotEmpty(t, states[i].OperatorSessionID)
			} else {
				require.Equal(t, models.OperatorDeploymentPhasePendingApproval, states[i].Phase)
			}
		}
		return states
	}

	cold := deploy("cold", n, 1, true)
	require.Equal(t, decisions+1, deploymentDecisionCount(t), "cold deployment must submit exactly one governed batch decision")
	sessions := make([]string, n)
	requests := map[string]bool{}
	for i, state := range cold {
		require.NotEmpty(t, state.RequestID)
		require.False(t, requests[state.RequestID], "cold workers must stage distinct requests")
		requests[state.RequestID] = true
		sessions[i] = state.OperatorSessionID
	}
	require.Len(t, localDeploymentPIDs(t, root), n, "one live process per cold worker")
	verifyDeploymentFanOut(t, bin, root, sessions)

	warm := deploy("retained_identity", n, 1, true)
	require.Equal(t, decisions+1, deploymentDecisionCount(t), "retained identities must not request another approval")
	for i, state := range warm {
		assert.Equal(t, cold[i].OperatorSessionID, state.OperatorSessionID)
		assert.NotEqual(t, cold[i].LaunchID, state.LaunchID, "readiness must belong to this invocation")
	}
	require.Len(t, localDeploymentPIDs(t, root), n, "redeploy must replace its workers without duplicates")
	verifyDeploymentFanOut(t, bin, root, sessions)

	added := deploy("append", 1, n+1, true)
	require.Equal(t, decisions+2, deploymentDecisionCount(t))
	for i := range warm {
		assert.Equal(t, warm[i], deploymentState(t, filepath.Join(root, fmt.Sprintf("op-%05d", i+1))), "append must preserve earlier deployment state")
	}
	sessions = append(sessions, added[0].OperatorSessionID)
	verifyDeploymentFanOut(t, bin, root, sessions)

	denied := deploy("pending_then_denied", 1, n+2, false)
	require.Empty(t, denied[0].OperatorSessionID, "pending enrollment cannot claim a ready session")
	require.NoError(t, e2eClient.DenyEnrollment(ctx, denied[0].RequestID))
	deniedDir := filepath.Join(root, fmt.Sprintf("op-%05d", n+2))
	require.Eventually(t, func() bool {
		return deploymentState(t, deniedDir).Phase == models.OperatorDeploymentPhaseFailed
	}, 15*time.Second, 50*time.Millisecond, "denial must be recorded as failed, never ready")
	require.NotEmpty(t, deploymentState(t, deniedDir).Error)
	stopOutput, err := deploymentCLI(ctx, bin, "operator", "stop", added[0].OperatorSessionID,
		"--reason", "deployment lifecycle E2E teardown", "--json")
	require.NoError(t, err, "%s", stopOutput)
	var stopped struct {
		models.StopOperatorResponse
		PID    int    `json:"pid"`
		Method string `json:"method"`
	}
	require.NoError(t, json.Unmarshal([]byte(stopOutput), &stopped))
	require.True(t, stopped.Success)
	require.Equal(t, added[0].OperatorSessionID, stopped.OperatorSessionID)
	require.NotEmpty(t, stopped.TransactionID, "targeted stop must carry a governed shutdown receipt")

	// A CLI cancellation can leave a worker that has never reached the registry.
	// Exercise teardown of that exact failure, alongside the healthy cohort.
	canceledCtx, cancelDeploy := context.WithTimeout(ctx, 2*time.Second)
	_, err = deploymentCLI(canceledCtx, bin, "operator", "deploy", "--local", "--dest-dir", root,
		"--count", "1", "--start-index", strconv.Itoa(n+3), "--background", "--approve",
		"--operator-endpoint", "127.0.0.1:1", "--endpoint", e2eCfg.cfg.Paths.Host)
	cancelDeploy()
	require.Error(t, err)
	require.ErrorIs(t, canceledCtx.Err(), context.DeadlineExceeded, "cancel while the worker cannot stage")
	stopDeploymentProcesses(t, root)
	require.Empty(t, localDeploymentPIDs(t, root), "canceled and denied workers must also be gone")

	// Unrelated registry entries retain their identities and lifecycle status.
	after, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err)
	for _, original := range before.Operators {
		var found *operatorv1.OperatorDocument
		for _, op := range after.Operators {
			if op.Id == original.Id {
				found = op
			}
		}
		require.NotNil(t, found)
		assert.Equal(t, original.OperatorSessionId, found.OperatorSessionId)
		assert.Equal(t, original.Status, found.Status, "scoped cleanup cannot stop unrelated Operators")
	}
	require.NoError(t, preflightHealthCheck(ctx, e2eCfg), "Gateway must remain healthy after the lifecycle")
}

func deploymentCLI(ctx context.Context, bin string, args ...string) (string, error) {
	root, err := resolveRuntimeRoot()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = root
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err = cmd.Run()
	return output.String(), err
}

func deploymentState(t *testing.T, dir string) models.OperatorDeploymentState {
	t.Helper()
	fileSvc, err := fs.NewRuntimeFileService(dir, testutil.NewTestLogger())
	require.NoError(t, err)
	data, err := fileSvc.ReadFile(context.Background(), filepath.Join(constants.DeploymentDirname, constants.DeploymentStateFileOperator))
	require.NoError(t, err)
	var state models.OperatorDeploymentState
	require.NoError(t, json.Unmarshal(data, &state))
	return state
}

func deploymentAuditDB(t *testing.T, fileSvc fs.RuntimeFileService) *sql.DB {
	t.Helper()
	u := url.URL{Scheme: "file", Path: fileSvc.Resolve(constants.CanonicalDBRelPath)}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { assert.NoError(t, db.Close()) })
	return db
}

func deploymentDecisionCount(t *testing.T) int {
	t.Helper()
	db := deploymentAuditDB(t, e2eCfg.fileSvc)
	var count int
	require.NoError(t, db.QueryRow("SELECT count(*) FROM receipts WHERE action_type = ?", string(constants.ActionTypePlatformEnrollmentDecide)).Scan(&count))
	return count
}

func verifyDeploymentFanOut(t *testing.T, bin, root string, sessions []string) {
	t.Helper()
	out := runFleetFanOut(t, bin, min(len(sessions), 100), len(sessions), sessions)
	// Command readiness and observed heartbeat telemetry are separate signals.
	// Wait for one real heartbeat per cohort member, with a bounded deadline,
	// rather than treating the subscription acknowledgement as heartbeat data.
	heartbeatCtx, cancelHeartbeat := context.WithTimeout(context.Background(), fleetEnrollmentWait)
	defer cancelHeartbeat()
	require.Eventually(t, func() bool {
		ops, err := remoteFleetOperators(heartbeatCtx, sessions)
		if err != nil || len(ops) != len(sessions) {
			return false
		}
		for _, op := range ops {
			if op.LastHeartbeatAt == nil || time.Since(op.LastHeartbeatAt.AsTime()) >= constants.OperatorHeartbeatStaleAfter {
				return false
			}
		}
		return true
	}, fleetEnrollmentWait, 100*time.Millisecond, "every cohort member must deliver a real heartbeat")
	for _, result := range out.Results {
		op, err := e2eClient.GetOperatorBySession(context.Background(), result.OperatorSessionID)
		require.NoError(t, err)
		require.NotNil(t, op.Operator)
		require.Equal(t, string(constants.OperatorStatusActive), op.Operator.Status)
		require.NotNil(t, op.Operator.LastHeartbeatAt, "each ready worker must send its initial heartbeat")
		require.WithinDuration(t, time.Now(), op.Operator.LastHeartbeatAt.AsTime(), constants.OperatorHeartbeatStaleAfter)
		rel, err := filepath.Rel(root, op.Operator.LocalDir)
		require.NoError(t, err)
		require.False(t, filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)), "receipt evidence must belong to this fleet")
		fileSvc, err := fs.NewRuntimeFileService(op.Operator.LocalDir, testutil.NewTestLogger())
		require.NoError(t, err)
		db := deploymentAuditDB(t, fileSvc)
		var receiptJSON string
		var operatorID, sessionID string
		require.NoError(t, db.QueryRow("SELECT receipt_json, operator_id, operator_session_id FROM receipts WHERE transaction_id = ?", result.TransactionID).Scan(&receiptJSON, &operatorID, &sessionID), "the Operator's local receipt is authoritative")
		var receipt operatorv1.ActionReceipt
		require.NoError(t, compliancev1.UnmarshalCanonical([]byte(receiptJSON), &receipt))
		require.Equal(t, result.TransactionID, receipt.TransactionId)
		require.Equal(t, result.OperatorID, operatorID)
		require.Equal(t, result.OperatorSessionID, sessionID)
		require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)
		key, err := governance.SignerPublicKey(receipt.SignerKeyId)
		require.NoError(t, err)
		require.NoError(t, governance.VerifyActionReceiptSignature(&receipt, key))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		require.Eventually(t, func() bool {
			projected, err := e2eClient.GetAuditReceipts(ctx, result.TransactionID)
			if err != nil {
				return false
			}
			for _, record := range projected.Receipts {
				if record.TransactionID == result.TransactionID && record.OperatorSessionID == result.OperatorSessionID && record.ActionReceipt != nil {
					return record.ActionReceipt.Signature == receipt.Signature
				}
			}
			return false
		}, 10*time.Second, 50*time.Millisecond, "Gateway must project the same signed local receipt")
		cancel()
	}
}

// Read host process metadata, never runtime state, and match both the worker
// command and its exact working-directory ancestry before signaling a PID.
func localDeploymentPIDs(t *testing.T, root string) []int {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	require.NoError(t, err)
	var pids []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(string(data), "\x00")
		if len(args) < 3 || filepath.Base(args[0]) != "g8e" || args[1] != "operator" || args[2] != "start" {
			continue
		}
		for i := 3; i+1 < len(args); i++ {
			if args[i] == "--working-dir" {
				rel, err := filepath.Rel(root, args[i+1])
				if err == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					pids = append(pids, pid)
				}
				break
			}
		}
	}
	return pids
}

func stopDeploymentProcesses(t *testing.T, root string) {
	t.Helper()
	for _, signal := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		for _, pid := range localDeploymentPIDs(t, root) {
			if err := syscall.Kill(pid, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
				t.Errorf("stop fleet PID %d: %v", pid, err)
			}
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if len(localDeploymentPIDs(t, root)) == 0 {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	assert.Empty(t, localDeploymentPIDs(t, root), "test cleanup must leave no workers")
}
