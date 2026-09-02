// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package evidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestDemoDirectoryProvenanceSourceArtifactsReadsSortedCanonicalFiles(t *testing.T) {
	projectRoot := testutil.TempDir(t)
	demoRoot := filepath.Join(projectRoot, constants.DemosDirname, constants.DemosOrgHealthcare)
	for _, directory := range []string{constants.DemosDoctrineDir, constants.DemosTargetDataDir, constants.DemoConfigDirname} {
		require.NoError(t, os.MkdirAll(filepath.Join(demoRoot, directory), constants.PermDirPrivate))
	}
	files := map[string][]byte{
		constants.DemosComposeFile: []byte("services: {}"),
		filepath.Join(constants.DemosDoctrineDir, constants.DemosHIPAADoctrineFile): []byte(`{"version":"1.0.0"}`),
		filepath.Join(constants.DemosTargetDataDir, constants.DemosPARequestsFile):  []byte(`[]`),
	}
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(demoRoot, name), body, constants.PermFilePrivate))
	}

	artifacts, err := NewDemoDirectoryProvenanceSource(projectRoot).Artifacts(context.Background(), constants.DemosOrgHealthcare)

	require.NoError(t, err)
	require.Len(t, artifacts, len(files))
	for index, name := range []string{
		constants.DemosComposeFile,
		filepath.Join(constants.DemosDoctrineDir, constants.DemosHIPAADoctrineFile),
		filepath.Join(constants.DemosTargetDataDir, constants.DemosPARequestsFile),
	} {
		assert.Equal(t, name, artifacts[index].Name)
		assert.Equal(t, files[name], artifacts[index].Body)
	}
}

func TestDemoDirectoryProvenanceSourceArtifactsRejectsInvalidInputs(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name   string
		source *DemoDirectoryProvenanceSource
		ctx    context.Context
		demoID string
	}{
		{name: "cancelled context", source: NewDemoDirectoryProvenanceSource(testutil.TempDir(t)), ctx: cancelled, demoID: constants.DemosOrgHealthcare},
		{name: "nil source", ctx: context.Background(), demoID: constants.DemosOrgHealthcare},
		{name: "empty project root", source: NewDemoDirectoryProvenanceSource(""), ctx: context.Background(), demoID: constants.DemosOrgHealthcare},
		{name: "unsafe demo ID", source: NewDemoDirectoryProvenanceSource(testutil.TempDir(t)), ctx: context.Background(), demoID: constants.PathParentDir},
		{name: "missing compose file", source: NewDemoDirectoryProvenanceSource(testutil.TempDir(t)), ctx: context.Background(), demoID: constants.DemosOrgHealthcare},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifacts, err := tt.source.Artifacts(tt.ctx, tt.demoID)

			require.Error(t, err)
			assert.Nil(t, artifacts)
		})
	}
}
