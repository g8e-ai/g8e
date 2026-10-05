// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentResponse_GetString_AbsentField(t *testing.T) {
	doc := DocumentResponse{"case_title": json.RawMessage(`"hello"`)}
	assert.Equal(t, "hello", doc.GetString("case_title"))
	assert.Equal(t, "", doc.GetString("absent_field"))
}

func TestDocumentResponse_GetBool_AbsentField(t *testing.T) {
	doc := DocumentResponse{"sentinel_mode": json.RawMessage(`true`)}
	assert.True(t, doc.GetBool("sentinel_mode"))
	assert.False(t, doc.GetBool("absent_field"))
}

func TestActingAppG8ee_MatchesEnsembleConstant(t *testing.T) {
	// The ensemble Python code defines G8EE_COMPONENT = "g8ee". The harness
	// must use the same value so receipts attribute the action to g8ee.
	assert.Equal(t, "g8ee", ActingAppG8ee)
}

func TestInvestigationUpdate_OmitsUnsetFields(t *testing.T) {
	title := "Patched Title"
	updates, err := documentUpdateStruct(InvestigationUpdate{CaseTitle: &title})
	require.NoError(t, err)
	require.NotNil(t, updates)
	assert.Equal(t, "Patched Title", updates.Fields["case_title"].GetStringValue())
	_, hasStatus := updates.Fields["status"]
	assert.False(t, hasStatus)
}

func strPtr(value string) *string {
	return &value
}

// assertJSONField unmarshals a JSON string field from the envelope map and
// asserts it equals the expected value.
func assertJSONField(t *testing.T, env map[string]json.RawMessage, field, expected string) {
	t.Helper()
	raw, ok := env[field]
	require.True(t, ok, "envelope must contain field %q", field)
	var got string
	require.NoError(t, json.Unmarshal(raw, &got), "field %q must be a JSON string", field)
	assert.Equal(t, expected, got, "field %q mismatch", field)
}
