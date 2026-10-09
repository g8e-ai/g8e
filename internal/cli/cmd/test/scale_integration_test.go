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
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/netutil"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// fakeScaleDeps records every child process and teardown call without
// starting a Gateway or Operator.
type fakeScaleDeps struct {
	calls       []scaleProcess
	failOn      string // fail the first call whose argv contains this substring
	stopped     []string
	portsErr    error
	verifyErr   error
	verifyCalls []string
}

// fakeScalePorts are deliberately not the default Gateway ports.
var fakeScalePorts = scalePorts{HTTP: 18080, HTTPS: 18443}

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
		reservePorts: func() (scalePorts, error) {
			if f.portsErr != nil {
				return scalePorts{}, f.portsErr
			}
			return fakeScalePorts, nil
		},
		verifyGateway: func(runDir string, ports scalePorts) error {
			f.verifyCalls = append(f.verifyCalls, runDir)
			return f.verifyErr
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
	assert.Contains(t, argvs[0], "--http-port 18080 --https-port 18443")
	assert.NotContains(t, argvs[0], "--quiet", "gw start output must be logged for diagnostics")
	assert.Equal(t, "auth enroll user -e localhost:18080 -p 18443 --headless", argvs[1])
	assert.Contains(t, argvs[2], "operator deploy --local")
	assert.Contains(t, argvs[2], "-e localhost --gateway-http-port 18080 --gateway-https-port 18443")
	assert.Contains(t, argvs[2], "--count 100 --start-index 1 ")
	assert.Contains(t, argvs[3], "--count 50 --start-index 101 ")
	assert.Contains(t, argvs[3], "--parallel 25")
	assert.Contains(t, argvs[4], "-run "+scaleFanOutScenario)
	assert.Contains(t, argvs[5], "-run "+scaleRestartScenario)
	assert.Equal(t, "gw stop", argvs[6])

	layout := newScaleLayout(cfg.Root)
	assert.Equal(t, []string{layout.Run}, fake.verifyCalls, "gateway ownership must be verified in layout.Run before enrollment")
	assert.Equal(t, []string{layout.Fleet}, fake.stopped)
	assert.Equal(t, "go", fake.calls[4].Name)
	assert.Contains(t, fake.calls[4].Env, scaleEnvRuntimeRoot+"="+layout.Run)
	assert.Contains(t, fake.calls[4].Env, "HOME="+layout.Home)
	assert.Contains(t, fake.calls[4].Env, "USERPROFILE="+layout.Home)
	assert.Contains(t, fake.calls[4].Env, scaleEnvGatewayHTTPPort+"=18080")
	assert.Contains(t, fake.calls[4].Env, scaleEnvGatewayHTTPSPort+"=18443")
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

func TestRunScale_FailedPortReservationStartsNothing(t *testing.T) {
	cfg := scaleTestConfig(t)
	fake := &fakeScaleDeps{portsErr: constants.ErrPortUnavailable}

	err := runScale(context.Background(), cfg, fake.deps())
	require.ErrorIs(t, err, constants.ErrScaleTestFailed)
	require.ErrorIs(t, err, constants.ErrPortUnavailable)
	assert.Empty(t, fake.calls)
	assert.Empty(t, fake.stopped)
	assert.NoDirExists(t, cfg.Root, "no scratch root is created without Gateway ports")
}

func TestReserveScalePorts_ReturnsDistinctNonDefaultPorts(t *testing.T) {
	ports, err := reserveScalePorts()
	require.NoError(t, err)
	assert.NotEqual(t, ports.HTTP, ports.HTTPS)
	for _, port := range []int{ports.HTTP, ports.HTTPS} {
		assert.Positive(t, port)
		assert.NotContains(t, constants.GatewayReservedLoopbackPorts, port)
		require.NoError(t, netutil.CheckTCPPortAvailable(port), "reserved ports are released before return")
	}
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

func TestRunScale_GatewayOwnershipFailureAbortsBeforeAuthEnrollAndDoesNotStopForeignGateway(t *testing.T) {
	cfg := scaleTestConfig(t)
	fake := &fakeScaleDeps{verifyErr: errors.New("foreign gateway detected")}

	err := runScale(context.Background(), cfg, fake.deps())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify gateway ownership: foreign gateway detected")

	argvs := fake.argvs()
	require.Len(t, argvs, 1, "gw start was called, but failure aborted before auth enroll or scenarios")
	assert.True(t, strings.HasPrefix(argvs[0], "gw start"))
	for _, argv := range argvs {
		assert.NotEqual(t, "gw stop", argv, "teardown must not stop a gateway whose ownership was not verified")
	}
}

func TestVerifyScaleGatewayOwnership(t *testing.T) {
	root := testutil.TempDir(t)
	runDir := filepath.Join(root, "run")
	require.NoError(t, os.MkdirAll(runDir, 0o700))

	ports := scalePorts{HTTP: 18080, HTTPS: 18443}

	// Case 1: no runtime files -> fails on missing PID
	err := verifyScaleGatewayOwnership(runDir, ports)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read gateway pid")

	// Case 2: valid PID file with current process PID, but missing launch profile
	fileSvc, err := fs.NewRuntimeFileService(runDir, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	pm, err := platform.NewProcessManager(fileSvc)
	require.NoError(t, err)
	require.NoError(t, pm.WritePIDFile(constants.OperatorPIDFilename, os.Getpid()))

	err = verifyScaleGatewayOwnership(runDir, ports)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read gateway launch profile")

	// Case 3: launch profile with mismatched ports
	mismatchedCfg := serve.GatewayConfig{
		HTTPPort:  19080,
		HTTPSPort: 19443,
		Posture:   constants.PostureDoctrine,
	}
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, mismatchedCfg))

	err = verifyScaleGatewayOwnership(runDir, ports)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match reserved port")

	// Case 4: launch profile matching reserved ports
	matchingCfg := serve.GatewayConfig{
		HTTPPort:  ports.HTTP,
		HTTPSPort: ports.HTTPS,
		Posture:   constants.PostureDoctrine,
	}
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, matchingCfg))

	err = verifyScaleGatewayOwnership(runDir, ports)
	require.NoError(t, err)
}

