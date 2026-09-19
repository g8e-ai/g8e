// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopConsensusPolicyStore_GetConsensusPolicy_ReturnsNilNoError(t *testing.T) {
	t.Parallel()
	store := NoopConsensusPolicyStore{}
	policy, err := store.GetConsensusPolicy("any-consensus-id")
	require.NoError(t, err, "NoopConsensusPolicyStore must never return an error")
	assert.Nil(t, policy, "NoopConsensusPolicyStore must return nil policy")
}

func TestNoopConsensusPolicyStore_SatisfiesInterface(t *testing.T) {
	t.Parallel()
	var _ L2ConsensusPolicyStore = NoopConsensusPolicyStore{}
}
