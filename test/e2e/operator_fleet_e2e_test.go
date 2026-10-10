// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/operator"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// Environment contract for the fleet scenarios. `g8e test scale`
// (internal/cli/cmd/test/scale.go) deploys the fleet and sets these; the
// scenarios are opt-in (not part of defaultE2ERunRegexp) and fail closed when
// their inputs are missing.
const (
	fleetSizeEnv          = "G8E_E2E_FLEET_SIZE"           // required: remote Operators the script deployed
	fleetBinEnv           = "G8E_E2E_FLEET_BIN"            // required: g8e binary used for `operator run`
	fleetDirEnv           = "G8E_E2E_FLEET_DIR"            // required for restart: local op-NNNNN roots
	fleetSoakEnv          = "G8E_E2E_FLEET_SOAK"           // optional: idle soak before fan-out (Go duration)
	fleetConcurrencyEnv   = "G8E_E2E_FLEET_CONCURRENCY"    // optional: comma-separated fan-out concurrencies
	fleetRoundsEnv        = "G8E_E2E_FLEET_ROUNDS"         // optional: fan-outs per concurrency level
	fleetReportEnv        = "G8E_E2E_FLEET_REPORT"         // optional: path for the JSON report
	fleetRestartReportEnv = "G8E_E2E_FLEET_RESTART_REPORT" // optional: path for the restart JSON report

	defaultFleetSoak        = 75 * time.Second // longer than OperatorHeartbeatStaleAfter, so a missed heartbeat shows up
	defaultFleetConcurrency = "16,64"
	defaultFleetRounds      = 1
	fleetSettleDuration     = 40 * time.Second // one heartbeat interval plus margin, after the last fan-out
	fleetSampleInterval     = 10 * time.Second
	fleetEnrollmentWait     = 90 * time.Second
	fleetFanOutTimeoutSecs  = 60
	fleetRestartTimeout     = 2 * time.Minute
	fleetRecoveryWait       = 3 * time.Minute // several heartbeat intervals plus reconnect backoff
)

// fleetFanOutCommand prints the host name on every supported Operator platform.
func fleetFanOutCommand() string {
	if runtime.GOOS == "windows" {
		return "hostname"
	}
	return "uname -n"
}

// fleetSample is one observation of the remote Operator registry.
type fleetSample struct {
	Phase           string    `json:"phase"`
	At              time.Time `json:"at"`
	Active          int       `json:"active"`
	NotActive       int       `json:"not_active"`
	Stale           int       `json:"stale"`
	MaxHeartbeatAge float64   `json:"max_heartbeat_age_seconds"`
}

// fleetReport is the machine-readable result of one scenario run. It carries
// no identifiers, keys, or tokens.
type fleetReport struct {
	Size        int                   `json:"size"`
	TargetScope string                `json:"target_scope"`
	EnrolledIn  float64               `json:"enrolled_wait_seconds"`
	Samples     []fleetSample         `json:"samples"`
	FanOut      []operator.RunSummary `json:"fan_out"`
	GeneratedAt time.Time             `json:"generated_at"`
}

// fleetRunOutput mirrors the --json document written by `g8e operator run`.
type fleetRunOutput struct {
	Results []operator.RunResult `json:"results"`
	Summary operator.RunSummary  `json:"summary"`
}

// TestOperatorFleet_HoldsUnderFanOut proves that a fleet of N real remote
// Operators, deployed as separate processes by `g8e operator deploy`, enrolls
// completely, keeps heartbeating without any Operator going stale, answers a
// governed command fanned out to all of them at several concurrency levels,
// and is still healthy afterwards. Latency is reported, not asserted: shared
// runners make absolute latency thresholds flaky, while correctness (every
// dispatch succeeds, nobody goes stale) does not depend on machine speed.
func TestOperatorFleet_HoldsUnderFanOut(t *testing.T) {
	size := requireFleetSize(t)
	bin := requireFleetEnv(t, fleetBinEnv)
	sessions, err := parseFleetSessions(os.Getenv(string(constants.EnvVar.E2EFleetSessions)))
	require.NoError(t, err)
	if len(sessions) > 0 {
		require.Len(t, sessions, size, "cohort must contain exactly the requested fleet size")
	}
	soak := fleetDurationEnv(t, fleetSoakEnv, defaultFleetSoak)
	concurrencies := fleetConcurrencies(t)
	rounds := fleetIntEnv(t, fleetRoundsEnv, defaultFleetRounds)

	// The scale Gateway binds IPv4 loopback. Both in-process API calls and
	// child CLI dispatches must use that address without resolving localhost.
	httpPort, httpsPort, err := e2eGatewayPorts()
	require.NoError(t, err)
	if httpPort != 0 {
		require.Equal(t, fmt.Sprintf("http://%s:%d", constants.LocalhostIP, httpPort), e2eCfg.gatewayHTTPURL)
		require.Equal(t, fmt.Sprintf("https://%s:%d", constants.LocalhostIP, httpsPort), e2eCfg.gatewayHTTPSURL)
		endpointArgs, err := e2eGatewayEndpointArgs()
		require.NoError(t, err)
		require.Equal(t, []string{"-e", fmt.Sprintf("%s:%d", constants.LocalhostIP, httpPort), "-p", strconv.Itoa(httpsPort)}, endpointArgs)
	}

	report := fleetReport{Size: size, TargetScope: "all-active including embedded"}
	if len(sessions) > 0 {
		report.TargetScope = "explicit remote session cohort; embedded excluded"
	}
	t.Cleanup(func() { writeFleetReport(t, &report) })

	t.Run("every Operator enrolls and heartbeats", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), fleetEnrollmentWait)
		defer cancel()
		started := time.Now()
		require.Eventually(t, func() bool {
			sample, err := sampleFleet(ctx, "enroll", sessions)
			if err != nil {
				t.Logf("enrollment poll: %v", err)
			}
			return err == nil && sample.Active == size && sample.NotActive == 0 && fleetHeartbeating(ctx, t, sessions)
		}, fleetEnrollmentWait, time.Second, "all %d remote Operators must be active and heartbeating", size)
		report.EnrolledIn = time.Since(started).Seconds()
	})

	if t.Failed() {
		t.FailNow()
	}
	t.Run("soak keeps every Operator active", func(t *testing.T) {
		watchFleet(t, &report, "soak", size, soak, sessions)
	})

	if t.Failed() {
		t.FailNow()
	}
	for _, concurrency := range concurrencies {
		for round := 1; round <= rounds; round++ {
			t.Run(fmt.Sprintf("fan-out concurrency %d round %d", concurrency, round), func(t *testing.T) {
				out := runFleetFanOut(t, bin, concurrency, size, sessions)
				report.FanOut = append(report.FanOut, out.Summary)
				t.Logf("fan-out concurrency=%d targets=%d ok=%d failed=%d wall=%.0fms p50=%.0fms p95=%.0fms p99=%.0fms max=%.0fms",
					out.Summary.Concurrency, out.Summary.Targets, out.Summary.Succeeded, out.Summary.Failed,
					out.Summary.WallMs, out.Summary.P50Ms, out.Summary.P95Ms, out.Summary.P99Ms, out.Summary.MaxMs)
			})
			if t.Failed() {
				t.FailNow()
			}
		}
	}

	t.Run("fan-out leaves every Operator active", func(t *testing.T) {
		watchFleet(t, &report, "settle", size, fleetSettleDuration, sessions)
	})
}

// fleetRestartReport is the machine-readable result of the restart scenario.
// It carries no identifiers, keys, or tokens.
type fleetRestartReport struct {
	Size               int                 `json:"size"`
	RestartSeconds     float64             `json:"gateway_restart_seconds"`
	RecoveredIn        float64             `json:"recovered_after_restart_seconds"`
	SessionsRotated    int                 `json:"sessions_rotated"`
	FanOutAfterRecover operator.RunSummary `json:"fan_out_after_recovery"`
	GeneratedAt        time.Time           `json:"generated_at"`
}

// TestOperatorFleet_RecoversAfterGatewayRestart proves that a deployed fleet
// survives a Gateway restart without re-enrollment: after `g8e gw restart`,
// every remote Operator identity that was active before is active again with
// a heartbeat newer than the restart, no new Operator identity appears, and a
// governed fan-out reaches the whole fleet. Recovery time is reported, not
// asserted beyond the fleetRecoveryWait correctness bound.
func TestOperatorFleet_RecoversAfterGatewayRestart(t *testing.T) {
	size := requireFleetSize(t)
	bin := requireFleetEnv(t, fleetBinEnv)
	fleetDir := requireFleetEnv(t, fleetDirEnv)
	require.Empty(t, strings.TrimSpace(os.Getenv(string(constants.EnvVar.E2EFleetSessions))),
		"the restart scenario runs against a dedicated Gateway and does not take a session cohort")
	root, err := resolveRuntimeRoot()
	require.NoError(t, err)

	report := fleetRestartReport{Size: size}
	t.Cleanup(func() {
		report.GeneratedAt = time.Now().UTC()
		writeFleetJSON(t, fleetRestartReportEnv, &report)
	})

	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	before, err := remoteFleetOperators(ctx, nil)
	cancel()
	require.NoError(t, err)
	identities := make(map[string]string, len(before))
	for _, op := range before {
		require.Equal(t, string(constants.OperatorStatusActive), op.Status, "every remote Operator must be active before the restart")
		identities[op.Id] = op.OperatorSessionId
	}
	require.Len(t, identities, size, "the registry must hold exactly the deployed fleet before the restart")

	restartAt := time.Now()
	restartCtx, restartCancel := context.WithTimeout(context.Background(), fleetRestartTimeout)
	restart := exec.CommandContext(restartCtx, bin, "gw", "restart")
	restart.Dir = root
	output, err := restart.CombinedOutput()
	restartCancel()
	require.NoError(t, err, "gw restart failed: %s", output)
	report.RestartSeconds = time.Since(restartAt).Seconds()

	var recovered []*operatorv1.OperatorDocument
	require.Eventually(t, func() bool {
		pollCtx, pollCancel := context.WithTimeout(context.Background(), defaultClientTimeout)
		defer pollCancel()
		operators, err := remoteFleetOperators(pollCtx, nil)
		if err != nil {
			t.Logf("recovery poll: %v", err)
			return false
		}
		recovered = recovered[:0]
		for _, op := range operators {
			if op.Status != string(constants.OperatorStatusActive) {
				continue
			}
			if op.LastHeartbeatAt == nil || !op.LastHeartbeatAt.AsTime().After(restartAt) {
				return false
			}
			recovered = append(recovered, op)
		}
		if len(recovered) != size {
			return false
		}
		ready, err := fleetCommandsReady(pollCtx, fleetDir, recovered, restartAt)
		if err != nil {
			t.Logf("command subscription recovery poll: %v", err)
		}
		return err == nil && ready
	}, fleetRecoveryWait, 2*time.Second, "all %d remote Operators must heartbeat and re-establish command subscriptions after the Gateway restart", size)
	report.RecoveredIn = time.Since(restartAt).Seconds()

	for _, op := range recovered {
		session, known := identities[op.Id]
		assert.True(t, known, "Operator %s appeared after the restart: the fleet must reconnect, not re-enroll", op.Id)
		if known && session != op.OperatorSessionId {
			report.SessionsRotated++
		}
	}
	t.Logf("gateway restart: restart=%.1fs recovered=%.1fs sessions_rotated=%d", report.RestartSeconds, report.RecoveredIn, report.SessionsRotated)

	// One wave across the fleet plus the embedded Operator.
	out := runFleetFanOut(t, bin, size+1, size, nil)
	report.FanOutAfterRecover = out.Summary
}

// fleetCommandsReady checks transport observations written by the real local
// workers after their command subscription ACK. A publish-socket heartbeat can
// arrive while the separate command socket is still reconnecting.
func fleetCommandsReady(ctx context.Context, fleetDir string, operators []*operatorv1.OperatorDocument, after time.Time) (bool, error) {
	sessions := make(map[string]bool, len(operators))
	for _, op := range operators {
		sessions[op.OperatorSessionId] = true
	}
	entries, err := os.ReadDir(fleetDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read fleet dir %s: %w", fleetDir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "op-") {
			continue
		}
		opDir := filepath.Join(fleetDir, entry.Name())
		fileSvc, err := fs.NewRuntimeFileService(opDir, nil)
		if err != nil {
			return false, fmt.Errorf("worker %s file service: %w", entry.Name(), err)
		}
		data, err := fileSvc.ReadFile(ctx, filepath.Join(constants.DeploymentDirname, constants.DeploymentStateFileOperator))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			return false, fmt.Errorf("worker %s deployment state: %w", entry.Name(), err)
		}
		var state models.OperatorDeploymentState
		if err := json.Unmarshal(data, &state); err != nil {
			return false, fmt.Errorf("worker %s decode deployment state: %w", entry.Name(), err)
		}
		if state.Phase != models.OperatorDeploymentPhaseReady || !state.UpdatedAt.After(after) || !sessions[state.OperatorSessionID] {
			return false, nil
		}
		delete(sessions, state.OperatorSessionID)
	}
	return true, nil
}

// sampleFleet reads the registry once and summarises the remote Operators.
func sampleFleet(ctx context.Context, phase string, sessions []string) (fleetSample, error) {
	operators, err := remoteFleetOperators(ctx, sessions)
	if err != nil {
		return fleetSample{}, err
	}
	sample := fleetSample{Phase: phase, At: time.Now()}
	for _, op := range operators {
		switch constants.OperatorStatus(op.Status) {
		case constants.OperatorStatusActive:
			sample.Active++
		case constants.OperatorStatusStale:
			sample.Stale++
			sample.NotActive++
		default:
			sample.NotActive++
		}
		if op.LastHeartbeatAt != nil {
			if age := sample.At.Sub(op.LastHeartbeatAt.AsTime()).Seconds(); age > sample.MaxHeartbeatAge {
				sample.MaxHeartbeatAge = age
			}
		}
	}
	return sample, nil
}

// remoteFleetOperators lists the owner's remote Operators. The embedded
// Operator is excluded: it is the Gateway's own substrate, not part of the
// fleet under test. An explicit session cohort excludes historical and unrelated
// registry entries without changing their lifecycle state.
func remoteFleetOperators(ctx context.Context, sessions []string) ([]*operatorv1.OperatorDocument, error) {
	listed, err := e2eClient.ListOperators(ctx)
	if err != nil {
		return nil, err
	}
	var remote []*operatorv1.OperatorDocument
	for _, op := range listed.Operators {
		if op.OperatorType == string(constants.OperatorTypeRemote) {
			remote = append(remote, op)
		}
	}
	return selectFleetOperators(remote, sessions)
}

// fleetHeartbeating reports whether every remote Operator has delivered at
// least one heartbeat.
func fleetHeartbeating(ctx context.Context, t *testing.T, sessions []string) bool {
	t.Helper()
	operators, err := remoteFleetOperators(ctx, sessions)
	if err != nil {
		return false
	}
	for _, op := range operators {
		if op.LastHeartbeatAt == nil {
			return false
		}
	}
	return true
}

// watchFleet samples the registry for the given duration and fails if any
// Operator leaves the active state or goes longer without a heartbeat than the
// Gateway's staleness window.
func watchFleet(t *testing.T, report *fleetReport, phase string, size int, duration time.Duration, sessions []string) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
		sample, err := sampleFleet(ctx, phase, sessions)
		cancel()
		require.NoError(t, err, "operator list must succeed during %s", phase)
		report.Samples = append(report.Samples, sample)

		assert.Equal(t, size, sample.Active, "%s: every remote Operator must stay active", phase)
		assert.Zero(t, sample.Stale, "%s: no Operator may be marked stale", phase)
		assert.Less(t, sample.MaxHeartbeatAge, constants.OperatorHeartbeatStaleAfter.Seconds(),
			"%s: heartbeat age must stay inside the staleness window", phase)
		remaining := time.Until(deadline)
		if t.Failed() || remaining <= 0 {
			return
		}
		time.Sleep(min(fleetSampleInterval, remaining))
	}
}

// runFleetFanOut runs the shipped `g8e operator run --all-active` CLI against
// the Gateway under test and decodes its typed JSON result. Using the real CLI
// keeps the scenario on the same dispatch path operators use.
func runFleetFanOut(t *testing.T, bin string, concurrency, size int, sessions []string) fleetRunOutput {
	t.Helper()
	root, err := resolveRuntimeRoot()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), (fleetFanOutTimeoutSecs+30)*time.Second)
	defer cancel()
	endpointArgs, err := e2eGatewayEndpointArgs()
	require.NoError(t, err)
	args := append([]string{"operator", "run"}, endpointArgs...)
	if len(sessions) == 0 {
		args = append(args, "--all-active")
	} else {
		args = append(args, sessions...)
	}
	args = append(args,
		"--concurrency", strconv.Itoa(concurrency),
		"--cmd", fleetFanOutCommand(),
		"--timeout", strconv.Itoa(fleetFanOutTimeoutSecs),
		"--json")
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(t, cmd.Run(), "operator run failed: %s", stderr.String())

	var out fleetRunOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out), "decode operator run --json output")
	assert.Equal(t, concurrency, out.Summary.Concurrency, "run must use the requested concurrency")
	assert.GreaterOrEqual(t, out.Summary.Targets, size, "fan-out must reach every fleet Operator (the embedded Operator may add one)")
	if len(sessions) > 0 {
		assert.Equal(t, size, out.Summary.Targets)
		actual := make([]string, 0, len(out.Results))
		for _, result := range out.Results {
			actual = append(actual, result.OperatorSessionID)
		}
		assert.ElementsMatch(t, sessions, actual, "dispatch must reach exactly the selected cohort")
	}
	assert.Zero(t, out.Summary.Failed, "every dispatch must succeed")
	for _, result := range out.Results {
		assert.True(t, result.Success, "dispatch to %s failed: %s", result.OperatorID, result.Error)
		assert.Zero(t, result.ExitCode, "dispatch to %s exited non-zero", result.OperatorID)
		assert.NotEmpty(t, strings.TrimSpace(result.Stdout), "dispatch to %s returned no output", result.OperatorID)
	}
	return out
}

func writeFleetReport(t *testing.T, report *fleetReport) {
	t.Helper()
	report.GeneratedAt = time.Now().UTC()
	writeFleetJSON(t, fleetReportEnv, report)
}

// writeFleetJSON writes report to the path named by env, if set.
func writeFleetJSON(t *testing.T, env string, report any) {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(env))
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Errorf("encode fleet report: %v", err)
		return
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Errorf("write fleet report %s: %v", path, err)
	}
}

func requireFleetEnv(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	require.NotEmpty(t, value, "%s must be set by g8e test scale", name)
	return value
}

func requireFleetSize(t *testing.T) int {
	t.Helper()
	size, err := strconv.Atoi(requireFleetEnv(t, fleetSizeEnv))
	require.NoError(t, err, "%s must be an integer", fleetSizeEnv)
	require.Positive(t, size, "%s must be positive", fleetSizeEnv)
	return size
}

func fleetIntEnv(t *testing.T, name string, fallback int) int {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	require.NoError(t, err, "%s must be an integer", name)
	require.Positive(t, value, "%s must be positive", name)
	return value
}

func fleetDurationEnv(t *testing.T, name string, fallback time.Duration) time.Duration {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	require.NoError(t, err, "%s must be a Go duration", name)
	return value
}

func fleetConcurrencies(t *testing.T) []int {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(fleetConcurrencyEnv))
	if raw == "" {
		raw = defaultFleetConcurrency
	}
	var levels []int
	for _, part := range strings.Split(raw, ",") {
		level, err := strconv.Atoi(strings.TrimSpace(part))
		require.NoError(t, err, "%s entries must be integers", fleetConcurrencyEnv)
		require.Positive(t, level, "%s entries must be positive", fleetConcurrencyEnv)
		levels = append(levels, level)
	}
	return levels
}

// parseFleetSessions rejects ambiguous or malformed target selections before dispatch.
func parseFleetSessions(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var sessions []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		session := strings.TrimSpace(part)
		if _, err := uuid.Parse(session); err != nil {
			return nil, fmt.Errorf("invalid fleet session %q: %w", session, err)
		}
		if seen[session] {
			return nil, fmt.Errorf("duplicate fleet session %q", session)
		}
		seen[session] = true
		sessions = append(sessions, session)
	}
	return sessions, nil
}

// selectFleetOperators fails closed if a requested session is absent or duplicated.
func selectFleetOperators(operators []*operatorv1.OperatorDocument, sessions []string) ([]*operatorv1.OperatorDocument, error) {
	if len(sessions) == 0 {
		return operators, nil
	}
	var selected []*operatorv1.OperatorDocument
	for _, session := range sessions {
		var match *operatorv1.OperatorDocument
		for _, op := range operators {
			if op.OperatorSessionId != session {
				continue
			}
			if match != nil {
				return nil, fmt.Errorf("multiple operators for fleet session %q", session)
			}
			match = op
		}
		if match == nil {
			return nil, fmt.Errorf("missing fleet session %q", session)
		}
		selected = append(selected, match)
	}
	return selected, nil
}
