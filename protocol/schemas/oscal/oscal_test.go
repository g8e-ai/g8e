// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package oscal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaBytes_MatchesPinnedDigest(t *testing.T) {
	b, err := SchemaBytes()
	require.NoError(t, err)
	assert.Len(t, b, PinnedByteLength)
	sum := sha256.Sum256(b)
	assert.Equal(t, PinnedSHA256, hex.EncodeToString(sum[:]))
}

func TestSchemaBytes_IsACopy(t *testing.T) {
	b1, err := SchemaBytes()
	require.NoError(t, err)
	b2, err := SchemaBytes()
	require.NoError(t, err)
	assert.NotSame(t, &b1[0], &b2[0])
	assert.Equal(t, b1, b2)
}

func TestSchemaBytes_DecodesAsValidJSON(t *testing.T) {
	b, err := SchemaBytes()
	require.NoError(t, err)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(b, &schema))
	assert.Equal(t, "http://json-schema.org/draft-07/schema#", schema["$schema"])
	assert.Equal(t, SchemaID, schema["$id"])
	assert.Equal(t, "object", schema["type"])
	defs, ok := schema["definitions"].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, defs)
}

func TestVerifySchemaDigest_PassesForEmbeddedSchema(t *testing.T) {
	assert.NoError(t, VerifySchemaDigest())
}

func TestProvenanceBytes_DecodesAndMatchesSchema(t *testing.T) {
	p, err := LoadProvenance()
	require.NoError(t, err)
	assert.Equal(t, "oscal-assessment-results", p.SchemaType)
	assert.Equal(t, SchemaVersion, p.SchemaVersion)
	assert.Equal(t, SchemaID, p.SchemaID)
	assert.Equal(t, SchemaJSONDraft, p.JSONSchemaDraft)
	assert.Equal(t, PinnedByteLength, p.ByteLength)
	assert.Equal(t, PinnedSHA256, p.SHA256)
	assert.NotEmpty(t, p.SourceURL)
	assert.NotEmpty(t, p.SourceTag)
	assert.NotEmpty(t, p.License)
	assert.NotEmpty(t, p.RetrievalMethod)
}

func TestProvenanceBytes_IsValidJSON(t *testing.T) {
	var raw map[string]any
	require.NoError(t, json.Unmarshal(ProvenanceBytes(), &raw))
	assert.NotEmpty(t, raw)
}

func TestPinnedSHA256_IsValidHex(t *testing.T) {
	assert.Len(t, PinnedSHA256, 64)
	for _, c := range PinnedSHA256 {
		assert.True(t, strings.ContainsRune("0123456789abcdef", c), "invalid hex char %q", c)
	}
}
