// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

// writeSuiteFile renders a custom suite carved from the default suite to a
// definition file and returns its path.
func writeSuiteFile(t *testing.T, id, version string, scenarioIDs ...string) string {
	t.Helper()
	custom := evaluation.ScenarioSuite{SchemaVersion: evaluation.SuiteSchemaVersion, ID: id, Version: version, Description: "cli test suite"}
	for _, scenario := range evaluation.DefaultScenarioSuite().Scenarios {
		for _, want := range scenarioIDs {
			if scenario.ScenarioID == want {
				custom.Scenarios = append(custom.Scenarios, scenario)
			}
		}
	}
	require.Len(t, custom.Scenarios, len(scenarioIDs))
	body, err := evaluation.EncodeScenarioSuite(custom)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), id+".json")
	require.NoError(t, os.WriteFile(path, body, 0o600))
	return path
}
