// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package eval

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestModelsDiff_RejectsSeveralInferenceOperators(t *testing.T) {
	root := testutil.TempDir(t)
	seedModelScopes(t, root)

	// A second inference session makes the provider ambiguous.
	deps := newGatewayTestDeps(t, root, append(campaignOrchestrateOperators(), inferenceOperatorFixture("infer-op-2", "infer-session-2")))

	command := evalCmdWithConfig(deps)
	var out bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&out)
	command.SetArgs([]string{"models", "diff", "--project-root", root})
	err := command.Execute()
	require.ErrorIs(t, err, constants.ErrInferenceOperatorAmbiguous)
}
