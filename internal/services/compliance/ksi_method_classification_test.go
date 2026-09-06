// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type expectedMethodMetadata struct {
	artifact KSIArtifactIdentity
	boundary KSICollectionBoundary
	verifier KSIVerifierFamily
	property KSIMeasuredProperty
}

var expectedDefaultMethodMetadata = map[string]expectedMethodMetadata{
	"auditEventsExist": {
		artifact: KSIArtifactAuditEvents, boundary: KSICollectionAuditStore,
		verifier: KSIVerifierExistence, property: KSIPropertyPresence,
	},
	"receiptsExist": {
		artifact: KSIArtifactActionReceipts, boundary: KSICollectionAuditStore,
		verifier: KSIVerifierExistence, property: KSIPropertyPresence,
	},
	"receiptsCryptographicallyVerified": {
		artifact: KSIArtifactActionReceipts, boundary: KSICollectionAuditStore,
		verifier: KSIVerifierCryptographic, property: KSIPropertyReceiptPersistenceIntegrity,
	},
	"fileMutationsTracked": {
		artifact: KSIArtifactFileMutations, boundary: KSICollectionAuditStore,
		verifier: KSIVerifierExistence, property: KSIPropertyPresence,
	},
	"ledgerCommitsExist": {
		artifact: KSIArtifactLedgerCommits, boundary: KSICollectionLedgerStore,
		verifier: KSIVerifierExistence, property: KSIPropertyPresence,
	},
	"commitmentChainExists": {
		artifact: KSIArtifactCommitments, boundary: KSICollectionCommitmentStore,
		verifier: KSIVerifierExistence, property: KSIPropertyPresence,
	},
	"commitmentChainIntact": {
		artifact: KSIArtifactCommitments, boundary: KSICollectionCommitmentStore,
		verifier: KSIVerifierStructural, property: KSIPropertyChainLinkage,
	},
	"commitmentsCryptographicallyVerified": {
		artifact: KSIArtifactCommitments, boundary: KSICollectionCommitmentStore,
		verifier: KSIVerifierCryptographic, property: KSIPropertySignatureValidity,
	},
	"ledgerMerkleRootMatchesHead": {
		artifact: KSIArtifactLedgerStateRoot, boundary: KSICollectionLedgerStore,
		verifier: KSIVerifierStateObservation, property: KSIPropertyStateRootMatchesHead,
	},
	"independentStateObserved": {
		artifact: KSIArtifactReceiptStateTransitions, boundary: KSICollectionAuditStore,
		verifier: KSIVerifierStateObservation, property: KSIPropertyStateTransitionBinding,
	},
	"deterministicGraderResultsVerified": {
		artifact: KSIArtifactGraderResults, boundary: KSICollectionEvalResults,
		verifier: KSIVerifierDeterministicGrader, property: KSIPropertyEvidenceContentAddressing,
	},
	"ksiHistoryFreshness": {
		artifact: KSIArtifactHistorySnapshots, boundary: KSICollectionHistoryStore,
		verifier: KSIVerifierHistorical, property: KSIPropertyFreshness,
	},
}

func TestDefaultMethods_EveryMethodHasTypedIndependenceMetadata(t *testing.T) {
	registered := DefaultMethods(EvaluatorDeps{})
	seenNames := make(map[string]bool, len(expectedDefaultMethodMetadata))
	totalRegistered := 0

	for ksiID, methods := range registered {
		independenceKeys := make(map[string]bool, len(methods))
		for _, method := range methods {
			require.NoError(t, method.validate(), "%s/%s", ksiID, method.Name)
			expected, ok := expectedDefaultMethodMetadata[method.Name]
			require.True(t, ok, "unexpected method metadata for %s/%s", ksiID, method.Name)
			assert.Equal(t, expected.artifact, method.ArtifactIdentity)
			assert.Equal(t, expected.boundary, method.CollectionBoundary)
			assert.Equal(t, expected.verifier, method.VerifierFamily)
			assert.Equal(t, expected.property, method.MeasuredProperty)
			if method.Name == "ksiHistoryFreshness" {
				assert.Equal(t, KSIOutcomeStaleEvidence, method.UnsatisfiedOutcome)
			} else {
				assert.Equal(t, KSIOutcomeInvalidEvidence, method.UnsatisfiedOutcome)
			}
			assert.False(t, independenceKeys[method.independenceKey()], "%s contains methods that restate the same fact", ksiID)
			independenceKeys[method.independenceKey()] = true
			seenNames[method.Name] = true
			totalRegistered++
		}
	}

	assert.Equal(t, 20, totalRegistered)
	assert.Equal(t, len(expectedDefaultMethodMetadata), len(seenNames))
}

func TestDefaultMethods_IncludeEvidenceBackedVerifierFamilies(t *testing.T) {
	counts := make(map[KSIVerifierFamily]int)
	for _, methods := range DefaultMethods(EvaluatorDeps{}) {
		for _, method := range methods {
			counts[method.VerifierFamily]++
		}
	}

	assert.Greater(t, counts[KSIVerifierCryptographic], 1)
	assert.Greater(t, counts[KSIVerifierStateObservation], 0)
	assert.Greater(t, counts[KSIVerifierHistorical], 0)
	assert.Greater(t, counts[KSIVerifierDeterministicGrader], 0)
	assert.Greater(t, counts[KSIVerifierExistence], 0)
	assert.Greater(t, counts[KSIVerifierStructural], 0)
}

func TestDefaultMethods_UnregisteredKSIsRemainUnsupported(t *testing.T) {
	catalog := testCatalog()
	registered := DefaultMethods(EvaluatorDeps{})
	unregistered := 0
	for _, ksi := range catalog.KSIs {
		if len(registered[ksi.ID]) == 0 {
			unregistered++
		}
	}

	assert.Greater(t, unregistered, 0)
}
