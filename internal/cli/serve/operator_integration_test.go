// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package serve

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
)

// ---------------------------------------------------------------------------
// classifyConfigLoadError — integration with real config.Load
// ---------------------------------------------------------------------------

// TestClassifyConfigLoadError_RealConfigLoadNoPosture_Succeeds verifies that
// the operator no longer requires config posture to start. config.Load with
// an empty posture succeeds (the operator reads posture per-envelope from
// GovernanceEnvelope.Posture at L4 verification time). classifyConfigLoadError
// is never reached on this path.
func TestClassifyConfigLoadError_RealConfigLoadNoPosture_Succeeds(t *testing.T) {
	opts := ServeOperatorOptions{
		Endpoint:   "127.0.0.1",
		WorkingDir: t.TempDir(),
	}
	loadOpts := buildOperatorLoadOptions(opts, resolveOperatorEndpoint(opts.Endpoint), opts.WorkingDir)
	assert.Empty(t, loadOpts.Posture, "precondition: no posture supplied (operator reads posture per-envelope)")

	cfg, err := config.Load(loadOpts)
	require.NoError(t, err, "config.Load with empty posture should succeed (operator reads posture per-envelope)")
	require.NotNil(t, cfg)
	assert.Empty(t, cfg.Posture, "cfg.Posture should be empty when no posture was supplied")
}

// TestClassifyConfigLoadError_RealConfigLoadWithPosture_Succeeds verifies
// the positive half: when a posture IS supplied (informational), config.Load
// succeeds and the posture is preserved on the config.
func TestClassifyConfigLoadError_RealConfigLoadWithPosture_Succeeds(t *testing.T) {
	opts := ServeOperatorOptions{
		Endpoint:   "127.0.0.1",
		WorkingDir: t.TempDir(),
		Posture:    "doctrine",
	}
	loadOpts := buildOperatorLoadOptions(opts, resolveOperatorEndpoint(opts.Endpoint), opts.WorkingDir)
	assert.NotEmpty(t, loadOpts.Posture, "precondition: posture supplied (informational)")

	cfg, err := config.Load(loadOpts)
	require.NoError(t, err, "config.Load with a posture should succeed")
	require.NotNil(t, cfg)
	assert.Equal(t, config.GatewayPosture("doctrine"), cfg.Posture)
}
