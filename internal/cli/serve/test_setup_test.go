// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"context"
	"testing"

	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/require"
)

// newTestFileSvc creates a RuntimeFileService backed by a temp directory
// with the full .g8e runtime tree created. All serve tests should use this
// helper to obtain a fileSvc.
func newTestFileSvc(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	baseDir := testutil.TempDir(t)
	svc, err := fs.NewRuntimeFileService(baseDir, testLogger())
	require.NoError(t, err)
	require.NoError(t, svc.CreateRuntimeTree(context.Background()))
	return svc
}
