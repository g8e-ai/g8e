// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestNewInferenceConfig_AppliesTypedDefaults(t *testing.T) {
	got := newInferenceConfig(LoadOptions{InferenceEnabled: true})

	assert.True(t, got.Enabled)
	assert.Equal(t, constants.InferenceDefaultKeepAlive, got.KeepAlive)
}

func TestNewInferenceConfig_ExplicitFlagsOverrideDefaults(t *testing.T) {
	got := newInferenceConfig(LoadOptions{
		InferenceEnabled:   true,
		InferenceKeepAlive: "5m",
	})

	assert.Equal(t, "5m", got.KeepAlive)
}
