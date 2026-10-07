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
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/operator"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// Environment contract for the fleet scenario. scripts/ci/operator-fleet-smoke.sh
// deploys the fleet and sets these; the scenario is opt-in (it is not part of
// defaultE2ERunRegexp) and fails closed when its inputs are missing.
const (
	fleetSizeEnv        = "G8E_E2E_FLEET_SIZE"        // required: remote Operators the script deployed
	fleetBinEnv         = "G8E_E2E_FLEET_BIN"         // required: g8e binary used for `operator run`
	fleetSoakEnv        = "G8E_E2E_FLEET_SOAK"        // optional: idle soak before fan-out (Go duration)
	fleetConcurrencyEnv = "G8E_E2E_FLEET_CONCURRENCY" // optional: comma-separated fan-out concurrencies
	fleetRoundsEnv      = "G8E_E2E_FLEET_ROUNDS"      // optional: fan-outs per concurrency level
	fleetReportEnv      = "G8E_E2E_FLEET_REPORT"      // optional: path for the JSON report

	defaultFleetSoak        = 75 * time.Second // longer than OperatorHeartbeatStaleAfter, so a missed heartbeat shows up
	defaultFleetConcurrency = "16,64"
	defaultFleetRounds      = 1
	fleetSettleDuration     = 40 * time.Second // one heartbeat interval plus margin, after the last fan-out
	fleetSampleInterval     = 10 * time.Second
	fleetEnrollmentWait     = 90 * time.Second
	fleetFanOutTimeoutSecs  = 60
	fleetFanOutCommand      = "uname -n"
)

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
	soak := fleetDurationEnv(t, fleetSoakEnv, defaultFleetSoak)
	concurrencies := fleetConcurrencies(t)
	rounds := fleetIntEnv(t, fleetRoundsEnv, defaultFleetRounds)

	report := fleetReport{Size: size}
	t.Cleanup(func() { writeFleetReport(t, &report) })

	t.Run("every Operator enrolls and heartbeats", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), fleetEnrollmentWait)
		defer cancel()
		started := time.Now()
		require.Eventually(t, func() bool {
			sample, err := sampleFleet(ctx, "enroll")
			return err == nil && sample.Active == size && sample.NotActive == 0 && fleetHeartbeating(ctx, t)
		}, fleetEnrollmentWait, time.Second, "all %d remote Operators must be active and heartbeating", size)
		report.EnrolledIn = time.Since(started).Seconds()
	})

	t.Run("soak keeps every Operator active", func(t *testing.T) {
		watchFleet(t, &report, "soak", size, soak)
	})

	for _, concurrency := range concurrencies {
		for round := 1; round <= rounds; round++ {
			t.Run(fmt.Sprintf("fan-out concurrency %d round %d", concurrency, round), func(t *testing.T) {
				out := runFleetFanOut(t, bin, concurrency, size)
				report.FanOut = append(report.FanOut, out.Summary)
				t.Logf("fan-out concurrency=%d targets=%d ok=%d failed=%d wall=%.0fms p50=%.0fms p95=%.0fms p99=%.0fms max=%.0fms",
					out.Summary.Concurrency, out.Summary.Targets, out.Summary.Succeeded, out.Summary.Failed,
					out.Summary.WallMs, out.Summary.P50Ms, out.Summary.P95Ms, out.Summary.P99Ms, out.Summary.MaxMs)
			})
		}
	}

	t.Run("fan-out leaves every Operator active", func(t *testing.T) {
		watchFleet(t, &report, "settle", size, fleetSettleDuration)
	})
}

// sampleFleet reads the registry once and summarises the remote Operators.
func sampleFleet(ctx context.Context, phase string) (fleetSample, error) {
	operators, err := remoteFleetOperators(ctx)
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
// fleet under test.
func remoteFleetOperators(ctx context.Context) ([]*operatorv1.OperatorDocument, error) {
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
	return remote, nil
}

// fleetHeartbeating reports whether every remote Operator has delivered at
// least one heartbeat.
func fleetHeartbeating(ctx context.Context, t *testing.T) bool {
	t.Helper()
	operators, err := remoteFleetOperators(ctx)
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
func watchFleet(t *testing.T, report *fleetReport, phase string, size int, duration time.Duration) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
		sample, err := sampleFleet(ctx, phase)
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
func runFleetFanOut(t *testing.T, bin string, concurrency, size int) fleetRunOutput {
	t.Helper()
	root, err := resolveRuntimeRoot()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), (fleetFanOutTimeoutSecs+30)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "operator", "run", "--all-active",
		"--concurrency", strconv.Itoa(concurrency),
		"--cmd", fleetFanOutCommand,
		"--timeout", strconv.Itoa(fleetFanOutTimeoutSecs),
		"--json")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(t, cmd.Run(), "operator run failed: %s", stderr.String())

	var out fleetRunOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out), "decode operator run --json output")
	assert.Equal(t, concurrency, out.Summary.Concurrency, "run must use the requested concurrency")
	assert.GreaterOrEqual(t, out.Summary.Targets, size, "fan-out must reach every fleet Operator (the embedded Operator may add one)")
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
	path := strings.TrimSpace(os.Getenv(fleetReportEnv))
	if path == "" {
		return
	}
	report.GeneratedAt = time.Now().UTC()
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
	require.NotEmpty(t, value, "%s must be set by scripts/ci/operator-fleet-smoke.sh", name)
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
