// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhase0OSCAL_RandomUUIDsPreventByteIdenticalOutput documents that the
// current OSCAL export uses random RFC 4122 UUID v4 for every document, result,
// observation, finding, subject, component, control-implementation, and
// back-matter resource identifier. Regenerating an assessment from identical
// inputs (same catalog, same result set, same evaluated-at timestamp) produces
// different UUIDs and therefore different byte output on every run.
//
// This non-determinism prevents reproducible verification: an independent
// environment cannot confirm that two bundles from the same inputs are
// byte-identical. Phase 4 replaces random UUIDs with deterministic
// namespace-derived identifiers bound to report, control, assertion, and
// evidence identities. When the fix lands, this test is flipped to assert
// byte-identical output.
func TestPhase0OSCAL_RandomUUIDsPreventByteIdenticalOutput(t *testing.T) {
	catalog := oscalTestCatalog()
	resultSet := oscalTestResultSet()

	// Freeze the timestamp so the only source of difference is UUIDs.
	// We do this by pinning EvaluatedAtMs; the OSCAL exporter also calls
	// time.Now() for Published/LastModified, so we cannot fully eliminate
	// time-based variance. Instead we compare UUIDs directly.
	resultSet.EvaluatedAtMs = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	exporter := NewOSCALExporter(catalog)

	doc1, err := exporter.GenerateAssessmentResults(resultSet)
	require.NoError(t, err)
	doc2, err := exporter.GenerateAssessmentResults(resultSet)
	require.NoError(t, err)

	// Marshal both to canonical JSON (field order is stable from struct order).
	raw1, err := json.Marshal(doc1)
	require.NoError(t, err)
	raw2, err := json.Marshal(doc2)
	require.NoError(t, err)

	// The two outputs differ because UUIDs are random. After Phase 4, identical
	// inputs plus a frozen generator version produce byte-identical unsigned
	// output, and this assertion flips to assert.Equal.
	assert.NotEqual(t, string(raw1), string(raw2),
		phase0RegressionBeforeFix+
			": identical inputs produce different OSCAL output because UUIDs are random v4; "+
			"deterministic regeneration is impossible today. After Phase 4 this flips to Equal.")
}

// TestPhase0OSCAL_RandomUUIDsDifferAcrossDocuments documents that every call to
// generateUUID produces a unique value, so even within a single document
// generation pass, no two UUID-bearing records share a deterministic
// relationship. This is the root cause of non-reproducible bundles.
func TestPhase0OSCAL_RandomUUIDsDifferAcrossDocuments(t *testing.T) {
	exporter := NewOSCALExporter(oscalTestCatalog())

	compDef1, err := exporter.GenerateComponentDefinition()
	require.NoError(t, err)
	compDef2, err := exporter.GenerateComponentDefinition()
	require.NoError(t, err)

	// The top-level component-definition UUID differs on every call.
	assert.NotEqual(t, compDef1.UUID, compDef2.UUID,
		phase0RegressionBeforeFix+
			": component-definition UUID is random and differs across identical-input calls")

	// Component UUIDs also differ.
	require.NotEmpty(t, compDef1.Components)
	require.NotEmpty(t, compDef2.Components)
	assert.NotEqual(t, compDef1.Components[0].UUID, compDef2.Components[0].UUID,
		phase0RegressionBeforeFix+
			": component UUID is random and differs across identical-input calls")
}

// TestPhase0OSCAL_GenerateUUIDIsRandomV4 directly exercises the generateUUID
// helper to lock the current behavior: two calls return different values.
func TestPhase0OSCAL_GenerateUUIDIsRandomV4(t *testing.T) {
	u1 := generateUUID()
	u2 := generateUUID()

	assert.NotEqual(t, u1, u2,
		phase0RegressionBeforeFix+
			": generateUUID returns a random value on every call; "+
			"Phase 4 replaces this with deterministic namespace-derived identifiers")
}
