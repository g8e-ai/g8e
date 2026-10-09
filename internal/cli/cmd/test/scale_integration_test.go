// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package testcmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// fakeScaleDeps records every child process and teardown call without
// starting a Gateway or Operator.
type fakeScaleDeps struct {
	calls     []scaleProcess
	failOn    string // fail the first call whose argv contains this substring
	stopped   []string
	portInUse int
}

func (f *fakeScaleDeps) deps() scaleDeps {
	return scaleDeps{
		run: func(_ context.Context, p scaleProcess) (int, error) {
			f.calls = append(f.calls, p)
			if f.failOn != "" && strings.Contains(strings.Join(p.Args, " "), f.failOn) {
				f.failOn = ""
				return 1, nil
			}
			return 0, nil
		},
		checkPort: func(port int) error {
			if port == f.portInUse {
				return constants.ErrScaleTestFailed
			}
			return nil
		},
		stopWorkers: func(_ context.Context, fleetDir string) (int, error) {
			f.stopped = append(f.stopped, fleetDir)
			return 0, nil
		},
		sample: func(ctx context.Context, _ scaleLayout, _ time.Duration) { <-ctx.Done() },
		stdout: io.Discard,
	}
}

func (f *fakeScaleDeps) argvs() []string {
	var out []string
	for _, c := range f.calls {
		out = append(out, strings.Join(c.Args, " "))
	}
	return out
}

func scaleTestConfig(t *testing.T) scaleConfig {
	t.Helper()
	t.Setenv("GOCACHE", "cache")
	t.Setenv("GOMODCACHE", "modcache")
	t.Setenv("GOPATH", "gopath")
	root := testutil.TempDir(t)
	bin := filepath.Join(root, "g8e")
	require.NoError(t, os.WriteFile(bin, nil, 0o600))
	cfg := scaleConfig{
		Count: 150, BatchSize: 100, Parallel: 25, Soak: time.Minute, FanOutConcurrency: "16",
		Rounds: 1, SampleInterval: time.Second, Root: filepath.Join(root, "run-root"), Binary: bin,
	}
	require.NoError(t, cfg.validate())
	return cfg
}

func TestRunScale_RunsPhasesInOrderAndTearsDown(t *testing.T) {
	cfg := scaleTestConfig(t)
	fake := &fakeScaleDeps{}

	require.NoError(t, runScale(context.Background(), cfg, fake.deps()))

	argvs := fake.argvs()
	require.Len(t, argvs, 7)
	assert.True(t, strings.HasPrefix(argvs[0], "gw start --cert-mode localhost --posture doctrine"))
	assert.Equal(t, "auth enroll user -e localhost --headless", argvs[1])
	assert.Contains(t, argvs[2], "operator deploy --local")
	assert.Contains(t, argvs[2], "--count 100 --start-index 1 ")
	assert.Contains(t, argvs[3], "--count 50 --start-index 101 ")
	assert.Contains(t, argvs[3], "--parallel 25")
	assert.Contains(t, argvs[4], "-run "+scaleFanOutScenario)
	assert.Contains(t, argvs[5], "-run "+scaleRestartScenario)
	assert.Equal(t, "gw stop", argvs[6])

	layout := newScaleLayout(cfg.Root)
	assert.Equal(t, []string{layout.Fleet}, fake.stopped)
	assert.Equal(t, "go", fake.calls[4].Name)
	assert.Contains(t, fake.calls[4].Env, scaleEnvRuntimeRoot+"="+layout.Run)
	assert.Contains(t, fake.calls[4].Env, "HOME="+layout.Home)
	assert.Contains(t, fake.calls[4].Env, "USERPROFILE="+layout.Home)
	assert.Contains(t, fake.calls[5].Env, scaleEnvFleetRestartReport+"="+filepath.Join(layout.Out, "restart-report.json"))
	assert.FileExists(t, filepath.Join(layout.Out, "scale-summary.json"))
}

func TestRunScale_FailedDeployStopsLaterPhasesButStillTearsDown(t *testing.T) {
	cfg := scaleTestConfig(t)
	fake := &fakeScaleDeps{failOn: "--start-index 101"}

	err := runScale(context.Background(), cfg, fake.deps())

	require.ErrorIs(t, err, constants.ErrScaleTestFailed)
	argvs := fake.argvs()
	require.Len(t, argvs, 5, "no scenario may run after a failed enrollment batch")
	assert.Equal(t, "gw stop", argvs[4])
	assert.Len(t, fake.stopped, 1)

	summary, readErr := os.ReadFile(filepath.Join(newScaleLayout(cfg.Root).Out, "scale-summary.json"))
	require.NoError(t, readErr)
	assert.Contains(t, string(summary), `"passed": false`)
}

func TestRunScale_SkipRestartOmitsRestartScenario(t *testing.T) {
	cfg := scaleTestConfig(t)
	cfg.SkipRestart = true
	fake := &fakeScaleDeps{}

	require.NoError(t, runScale(context.Background(), cfg, fake.deps()))
	for _, argv := range fake.argvs() {
		assert.NotContains(t, argv, scaleRestartScenario)
	}
}

func TestRunScale_RefusesOccupiedGatewayPortBeforeStartingAnything(t *testing.T) {
	cfg := scaleTestConfig(t)
	fake := &fakeScaleDeps{portInUse: constants.Ports.OperatorHttps}

	require.ErrorIs(t, runScale(context.Background(), cfg, fake.deps()), constants.ErrScaleTestFailed)
	assert.Empty(t, fake.calls)
	assert.Empty(t, fake.stopped)
	assert.NoDirExists(t, cfg.Root, "no scratch root is created while a port is occupied")
}

func TestPrepareScaleRoot_RefusesNonEmptyRootUnlessClean(t *testing.T) {
	root := testutil.TempDir(t)
	stale := filepath.Join(root, "stale")
	require.NoError(t, os.WriteFile(stale, []byte("x"), 0o600))

	_, err := prepareScaleRoot(root, false)
	require.ErrorIs(t, err, constants.ErrScaleTestInvalidInput)
	assert.FileExists(t, stale, "a refused root must not be modified")

	layout, err := prepareScaleRoot(root, true)
	require.NoError(t, err)
	assert.NoFileExists(t, stale)
	for _, dir := range []string{layout.Run, layout.Fleet, layout.Home, layout.Out} {
		assert.DirExists(t, dir)
	}
}

func TestScaleWorkerPIDs_ReadsOnlyRecordedFleetPIDs(t *testing.T) {
	fleet := testutil.TempDir(t)
	for name, content := range map[string]string{"op-00001": "101\n", "op-00002": "", "other": "999\n"} {
		require.NoError(t, os.MkdirAll(filepath.Join(fleet, name), 0o700))
		if content != "" {
			require.NoError(t, os.WriteFile(filepath.Join(fleet, name, constants.OperatorPIDFilename), []byte(content), 0o600))
		}
	}

	pids, err := scaleWorkerPIDs(fleet)
	require.NoError(t, err)
	assert.Equal(t, []int{101}, pids, "directories without a PID file and non-fleet directories are skipped")

	require.NoError(t, os.WriteFile(filepath.Join(fleet, "op-00002", constants.OperatorPIDFilename), []byte("nope"), 0o600))
	_, err = scaleWorkerPIDs(fleet)
	assert.Error(t, err, "a corrupt PID file must be reported, not ignored")
}
