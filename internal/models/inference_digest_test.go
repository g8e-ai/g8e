// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestNormalizeProviderModelDigest_AcceptsOllamaPrefix(t *testing.T) {
	t.Parallel()
	hexDigest := strings.Repeat("b", 64)
	normalized, err := NormalizeProviderModelDigest("sha256:" + hexDigest)
	require.NoError(t, err)
	assert.Equal(t, hexDigest, normalized)
}

func TestNormalizeProviderModelDigest_RejectsMalformedDigest(t *testing.T) {
	t.Parallel()
	_, err := NormalizeProviderModelDigest("sha256:not-a-digest")
	require.ErrorIs(t, err, constants.ErrInferenceModelRegistryInvalid)
}
