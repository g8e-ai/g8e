// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/tools/chaos"
)

func TestRunChaos_ConfigPopulationFromFlags(t *testing.T) {
	originalCount := chaosCount
	originalDataDir := chaosDataDir
	originalPKIDir := chaosPKIDir
	t.Cleanup(func() {
		chaosCount = originalCount
		chaosDataDir = originalDataDir
		chaosPKIDir = originalPKIDir
	})

	chaosCount = 42
	chaosDataDir = "/tmp/chaos-data"
	chaosPKIDir = "/tmp/chaos-pki"

	cfg := chaos.Config{
		Count:   chaosCount,
		DataDir: chaosDataDir,
		PKIDir:  chaosPKIDir,
	}

	assert.Equal(t, 42, cfg.Count)
	assert.Equal(t, "/tmp/chaos-data", cfg.DataDir)
	assert.Equal(t, "/tmp/chaos-pki", cfg.PKIDir)
}

func TestRunChaos_ZeroCountConfigConstruction(t *testing.T) {
	originalCount := chaosCount
	t.Cleanup(func() { chaosCount = originalCount })

	cmd := chaosCmd()
	require.NotNil(t, cmd)

	chaosCount = 0

	cfg := chaos.Config{
		Count:   chaosCount,
		DataDir: chaosDataDir,
		PKIDir:  chaosPKIDir,
	}

	assert.Equal(t, 0, cfg.Count)
}

func TestRunChaos_DefaultCountFromFlagIs100(t *testing.T) {
	cmd := chaosCmd()
	require.NotNil(t, cmd)

	flag := cmd.Flags().Lookup("count")
	require.NotNil(t, flag)
	assert.Equal(t, "100", flag.DefValue)
}
