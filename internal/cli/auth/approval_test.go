// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestApprovalPageURL(t *testing.T) {
	cfg := &config.Config{}
	got := ApprovalPageURL(cfg, "tx-abc")
	assert.True(t, strings.HasPrefix(got, cfg.OperatorPublicURL()))
	assert.True(t, strings.HasSuffix(got, constants.APIPaths.ApprovePagePrefix+"tx-abc"))
}

func TestApprovalStatusPath(t *testing.T) {
	assert.Equal(t, constants.APIPaths.ApprovalsCLIStatus+"tx-abc", ApprovalStatusPath("tx-abc"))
}

func TestVerifyApprovalStatus(t *testing.T) {
	t.Run("approved", func(t *testing.T) {
		status, err := VerifyApprovalStatus("tx-1", []byte(`{"status":"approved","tool_name":"run_command"}`))
		require.NoError(t, err)
		assert.Equal(t, "run_command", status.ToolName)
	})

	t.Run("expired or not found", func(t *testing.T) {
		_, err := VerifyApprovalStatus("tx-1", []byte(`{"status":"expired_or_not_found"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tx-1 expired or not found")
	})

	t.Run("pending is not approved", func(t *testing.T) {
		_, err := VerifyApprovalStatus("tx-1", []byte(`{"status":"pending"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status")
	})

	t.Run("malformed body", func(t *testing.T) {
		_, err := VerifyApprovalStatus("tx-1", []byte(`not json`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse status response")
	})
}
