// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testcmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestTestCmd_RegistersScale(t *testing.T) {
	cmd, _, err := Cmd().Find([]string{"scale"})
	require.NoError(t, err)
	assert.Equal(t, "scale", cmd.Name())
	for _, flag := range []string{"count", "batch-size", "parallel", "soak", "fan-out-concurrency", "rounds", "timeout", "sample-interval", "root", "binary", "clean", "skip-restart", "hosts", "operator-endpoint"} {
		assert.NotNil(t, cmd.Flags().Lookup(flag), "missing --%s", flag)
	}
}

func validScaleConfig() scaleConfig {
	return scaleConfig{Count: 100, BatchSize: 100, Parallel: 25, Soak: time.Minute, FanOutConcurrency: "16,64", Rounds: 1, SampleInterval: 10 * time.Second}
}

func TestScaleConfigValidate_RejectsOutOfRangeInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*scaleConfig)
	}{
		{"zero count", func(c *scaleConfig) { c.Count = 0 }},
		{"count above deploy ceiling", func(c *scaleConfig) { c.Count = scaleMaxCount + 1 }},
		{"negative batch size", func(c *scaleConfig) { c.BatchSize = -1 }},
		{"parallel above deploy ceiling", func(c *scaleConfig) { c.Parallel = scaleMaxParallel + 1 }},
		{"zero parallel", func(c *scaleConfig) { c.Parallel = 0 }},
		{"zero rounds", func(c *scaleConfig) { c.Rounds = 0 }},
		{"negative soak", func(c *scaleConfig) { c.Soak = -time.Second }},
		{"sub-second sample interval", func(c *scaleConfig) { c.SampleInterval = time.Millisecond }},
		{"non-numeric fan-out level", func(c *scaleConfig) { c.FanOutConcurrency = "16,x" }},
		{"zero fan-out level", func(c *scaleConfig) { c.FanOutConcurrency = "0" }},
		{"unknown fan-out token", func(c *scaleConfig) { c.FanOutConcurrency = "most" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validScaleConfig()
			tt.mutate(&cfg)
			assert.ErrorIs(t, cfg.validate(), constants.ErrScaleTestInvalidInput)
		})
	}
}

func TestScaleConfigValidate_RemoteHostsValidation(t *testing.T) {
	t.Run("empty hosts defaults to local", func(t *testing.T) {
		cfg := validScaleConfig()
		cfg.Hosts = ""
		require.NoError(t, cfg.validate())
		assert.Equal(t, "local", cfg.Hosts)
	})

	t.Run("remote host requires operator-endpoint", func(t *testing.T) {
		cfg := validScaleConfig()
		cfg.Hosts = "livingroom"
		cfg.OperatorEndpoint = ""
		err := cfg.validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrScaleTestInvalidInput)
		assert.ErrorIs(t, err, constants.ErrOperatorEndpointInvalid)
	})

	t.Run("remote host rejects loopback endpoint", func(t *testing.T) {
		for _, loopback := range []string{"127.0.0.1", "localhost", "127.0.0.1:8080", "http://127.0.0.1:8080", "https://localhost:8443", "[::1]", "0.0.0.0"} {
			cfg := validScaleConfig()
			cfg.Hosts = "remote-node"
			cfg.OperatorEndpoint = loopback
			err := cfg.validate()
			require.Error(t, err, "expected error for loopback endpoint %s", loopback)
			assert.ErrorIs(t, err, constants.ErrScaleTestInvalidInput)
			assert.ErrorIs(t, err, constants.ErrOperatorEndpointInvalid)
		}
	})

	t.Run("mixed local and remote host requires valid endpoint", func(t *testing.T) {
		cfg := validScaleConfig()
		cfg.Hosts = "local,livingroom"
		cfg.OperatorEndpoint = "127.0.0.1"
		err := cfg.validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrScaleTestInvalidInput)
		assert.ErrorIs(t, err, constants.ErrOperatorEndpointInvalid)
	})

	t.Run("remote host with valid endpoint succeeds", func(t *testing.T) {
		cfg := validScaleConfig()
		cfg.Hosts = "livingroom"
		cfg.OperatorEndpoint = "192.168.1.53"
		require.NoError(t, cfg.validate())
	})
}

func TestScaleConfigValidate_DefaultsScenarioTimeoutFromSoak(t *testing.T) {
	cfg := validScaleConfig()
	require.NoError(t, cfg.validate())
	assert.Equal(t, cfg.Soak+20*time.Minute, cfg.ScenarioTimeout)

	cfg = validScaleConfig()
	cfg.ScenarioTimeout = time.Hour
	require.NoError(t, cfg.validate())
	assert.Equal(t, time.Hour, cfg.ScenarioTimeout, "an explicit --timeout is kept")
}

func TestScaleConfigValidate_DefaultsToWholeFleetAtOnce(t *testing.T) {
	cfg := validScaleConfig()
	cfg.BatchSize = 0
	cfg.FanOutConcurrency = "16, all"
	require.NoError(t, cfg.validate())
	assert.Equal(t, cfg.Count, cfg.BatchSize, "the default deploys the whole fleet in one invocation")
	assert.Equal(t, "16,101", cfg.FanOutConcurrency, `"all" covers the fleet plus the embedded Operator`)

	cmd := scaleCmd()
	parallel, err := cmd.Flags().GetInt("parallel")
	require.NoError(t, err)
	assert.Equal(t, constants.PlatformEnrollmentMaxLiveOperatorRequests, parallel, "staging is not throttled below the Gateway quota")
	fanOut, err := cmd.Flags().GetString("fan-out-concurrency")
	require.NoError(t, err)
	assert.Equal(t, scaleFanOutAll, fanOut)
}

func TestScaleBatches_SplitsCountIntoAppendingStartIndexes(t *testing.T) {
	tests := []struct {
		name        string
		count, size int
		want        []scaleBatch
	}{
		{"single batch", 100, 100, []scaleBatch{{StartIndex: 1, Count: 100}}},
		{"batch larger than count", 3, 100, []scaleBatch{{StartIndex: 1, Count: 3}}},
		{"remainder batch", 250, 100, []scaleBatch{{StartIndex: 1, Count: 100}, {StartIndex: 101, Count: 100}, {StartIndex: 201, Count: 50}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, scaleBatches(tt.count, tt.size))
		})
	}
}

func TestScaleScenarioEnv_SetsFleetContract(t *testing.T) {
	cfg := validScaleConfig()
	layout := newScaleLayout("root")
	env := scaleScenarioEnv([]string{"A=1"}, layout, cfg, "bin", scalePorts{HTTP: 18080, HTTPS: 18443})
	assert.Equal(t, []string{
		"A=1",
		scaleEnvRuntimeRoot + "=" + layout.Run,
		scaleEnvFleetSize + "=100",
		scaleEnvFleetBin + "=bin",
		scaleEnvFleetDir + "=" + layout.Fleet,
		scaleEnvFleetSoak + "=1m0s",
		scaleEnvFleetConcurrency + "=16,64",
		scaleEnvFleetRounds + "=1",
		scaleEnvGatewayHTTPPort + "=18080",
		scaleEnvGatewayHTTPSPort + "=18443",
	}, env)
}

func TestDefaultScaleRoot_IsUnderLocalDevNeverTemp(t *testing.T) {
	now := time.Date(2026, 10, 9, 14, 5, 6, 0, time.FixedZone("x", 3600))
	root := defaultScaleRoot(now)
	assert.Equal(t, filepath.Join(".local.dev", "scale", "2026-10-09T13-05-06Z"), root)
	assert.False(t, filepath.IsAbs(root), "the default is relative to the working directory, not the OS temp directory")
}
