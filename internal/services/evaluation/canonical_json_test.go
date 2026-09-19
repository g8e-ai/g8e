// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalCanonicalJSONObject_EscapesHTMLSensitiveCharacters(t *testing.T) {
	t.Parallel()
	trace := map[string]any{
		"designated_role_output": "Action & Safeguard <done>",
		"trace_digest":           "",
	}
	canonical, err := MarshalCanonicalJSONObject(trace)
	require.NoError(t, err)
	assert.Contains(t, string(canonical), "\\u0026")
	assert.Contains(t, string(canonical), "\\u003c")
	assert.NotContains(t, string(canonical), " & ")
}
