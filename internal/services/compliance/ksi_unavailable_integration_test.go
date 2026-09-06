// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package compliance

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestUnavailableIntervalStoreIntegration_PersistsBoundFailuresAcrossStoreRestart(t *testing.T) {
	ctx := context.Background()
	baseDir := testutil.TempDir(t)
	fileSvc, err := fs.NewRuntimeFileService(baseDir, testutil.NewVerboseTestLogger(t))
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(ctx))
	dir := filepath.Join(constants.DataDirname, constants.ComplianceDirname)
	resultSet := unavailableTestResultSet()

	writer := NewUnavailableIntervalStore(fileSvc, dir)
	require.NoError(t, writer.AppendFromResultSet(ctx, resultSet))

	reader := NewUnavailableIntervalStore(fileSvc, dir)
	intervals, err := reader.ListIntervals(ctx)
	require.NoError(t, err)
	require.Len(t, intervals, 1)
	assert.Equal(t, resultSet.Binding.ScopeID, intervals[0].ScopeID)
	assert.Equal(t, resultSet.Binding.RunID, intervals[0].RunID)
	assert.Equal(t, resultSet.Binding.WindowStartUnixMs, intervals[0].StartUnixMs)
	assert.Equal(t, resultSet.Binding.WindowEndUnixMs, intervals[0].EndUnixMs)
	assert.Equal(t, resultSet.Binding, intervals[0].Binding)
	assert.Equal(t, KSIOutcomeInvalidEvidence, intervals[0].Outcome)
}
