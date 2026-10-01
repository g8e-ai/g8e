// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func TestGenerate_EmitsEveryCatalogScenarioWithoutHintValues(t *testing.T) {
	out, err := generate()
	require.NoError(t, err)
	text := string(out)

	assert.Contains(t, text, "DO NOT EDIT")
	assert.Equal(t, evaluation.DefaultSuiteScenarioCount, strings.Count(text, `"trajectoryPolicy":`))
	assert.Contains(t, text, `SCENARIO_CATALOG_ID = "`+evaluation.DefaultSuiteID+`@`+evaluation.DefaultSuiteVersion+`"`)
	assert.Contains(t, text, `"promptHint":`)
	// Hint arguments publish where a value comes from, never the value itself.
	assert.NotContains(t, text, `"value":`)
	assert.Contains(t, text, `"source": "workspace"`)
}

func TestGenerate_KeepsLicenseHeader(t *testing.T) {
	out, err := generate()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(out), "// Copyright (c) 2026 Lateralus Labs, LLC.\n// Licensed under the Business Source License 1.1"))
}

func TestGenerate_IsDeterministic(t *testing.T) {
	first, err := generate()
	require.NoError(t, err)
	second, err := generate()
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestGenerate_UsesPublicToolArgumentsCategorySpelling(t *testing.T) {
	out, err := generate()
	require.NoError(t, err)
	assert.Contains(t, string(out), `"category": "tool_arguments"`)
	assert.NotContains(t, string(out), `"category": "tool_argument"`)
}
