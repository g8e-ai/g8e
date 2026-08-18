// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystoretest

import (
	"context"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/require"
)

// NewTestFileService creates a RuntimeFileService backed by a temp directory
// with the full .g8e runtime tree created. Returns the file service and the
// absolute path to the secrets directory.
func NewTestFileService(t *testing.T) (fs.RuntimeFileService, string) {
	t.Helper()
	baseDir := testutil.TempDir(t)
	svc, err := fs.NewRuntimeFileService(baseDir, testutil.NewVerboseTestLogger(t))
	require.NoError(t, err)
	require.NoError(t, svc.CreateRuntimeTree(context.Background()))
	return svc, svc.Resolve(constants.SecretsDirname)
}
