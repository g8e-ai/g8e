// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectionsRegistry is the shape of protocol/constants/collections.json.
type collectionsRegistry struct {
	Collections map[string]struct {
		GoConst  string `json:"_go_const"`
		Value    string `json:"value"`
		Governed bool   `json:"_governed"`
	} `json:"collections"`
}

// TestCollectionIsGovernedDocumentMatchesRegistry keeps the Go governed
// document set in step with the `_governed` flag in collections.json, which
// the L4 Warden enforces for DOCUMENT_UPDATE and DOCUMENT_DELETE.
func TestCollectionIsGovernedDocumentMatchesRegistry(t *testing.T) {
	data, err := os.ReadFile("../../protocol/constants/collections.json")
	require.NoError(t, err)

	var registry collectionsRegistry
	require.NoError(t, json.Unmarshal(data, &registry))
	require.NotEmpty(t, registry.Collections)

	governed := 0
	for key, entry := range registry.Collections {
		if entry.Governed {
			governed++
		}
		assert.Equal(t, entry.Governed, CollectionName(entry.Value).IsGovernedDocument(),
			"collections.json %s (_governed=%v) disagrees with CollectionName.IsGovernedDocument", key, entry.Governed)
	}
	assert.Positive(t, governed, "collections.json marks no collection _governed")
}

// TestCollectionIsGovernedDocumentExcludesPlatformAuthority names the
// collections whose exclusion is the point of the governed document set.
func TestCollectionIsGovernedDocumentExcludesPlatformAuthority(t *testing.T) {
	for _, c := range []CollectionName{
		CollectionUsers,
		CollectionTrustedSigners,
		CollectionAppPolicies,
		CollectionRevokedCertificates,
		CollectionPlatformEnrollments,
		CollectionOperators,
		CollectionOperatorSessions,
		CollectionCLISessions,
		CollectionWebSessions,
		CollectionSettings,
		CollectionConsensus,
		CollectionEnrollmentTokens,
	} {
		assert.False(t, c.IsGovernedDocument(), "%s must not be writable through DOCUMENT_UPDATE or DOCUMENT_DELETE", c)
	}
}
