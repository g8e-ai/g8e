// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// validNode returns a fully-populated EvidenceNode suitable for AddNode. The
// canonical bytes are hashed to produce a consistent SHA-256 and artifact ID.
func validNode(artifactType ArtifactType, scopeID string, body []byte) EvidenceNode {
	digest := sha256.Sum256(body)
	return EvidenceNode{
		ArtifactID:         ContentAddress(artifactType, body),
		ArtifactType:       artifactType,
		SHA256:             hex.EncodeToString(digest[:]),
		MediaType:          "application/json",
		SchemaRef:          "test-schema",
		ProducerIdentity:   "test-producer",
		ProducedAt:         time.Unix(1_700_000_000, 0).UTC(),
		ScopeID:            scopeID,
		RunID:              "run-1",
		VerificationStatus: VerificationStatusVerified,
		VerifierID:         "test-verifier",
		VerifierVersion:    "1.0.0",
		VerifiedAt:         time.Unix(1_700_000_001, 0).UTC(),
		CanonicalBytes:     body,
	}
}

func TestContentAddress_FormatAndDeterminism(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	addr := ContentAddress(ArtifactTypeDemoManifest, body)
	digest := sha256.Sum256(body)
	expected := "demo-manifest:sha256:" + hex.EncodeToString(digest[:])
	assert.Equal(t, expected, addr)

	// Same input produces the same address.
	assert.Equal(t, expected, ContentAddress(ArtifactTypeDemoManifest, body))

	// Different input produces a different address.
	other := ContentAddress(ArtifactTypeDemoManifest, []byte(`{"hello":"other"}`))
	assert.NotEqual(t, expected, other)
}

func TestParseContentAddress_ValidAddress(t *testing.T) {
	body := []byte(`{}`)
	addr := ContentAddress(ArtifactTypeActionReceipt, body)
	artifactType, digest, ok := ParseContentAddress(addr)
	require.True(t, ok)
	assert.Equal(t, ArtifactTypeActionReceipt, artifactType)
	assert.Len(t, digest, 64)
}

func TestParseContentAddress_InvalidAddresses(t *testing.T) {
	emptyDigest := sha256.Sum256(nil)
	tests := []struct {
		name    string
		address string
	}{
		{"empty", ""},
		{"missing type", ":sha256:" + hex.EncodeToString(emptyDigest[:])},
		{"wrong algorithm", "demo-manifest:md5:abc"},
		{"uppercase digest", "demo-manifest:sha256:ABCDEF0123456789"},
		{"too short", "demo-manifest:sha256:abc"},
		{"too many parts", "demo-manifest:sha256:abc:extra"},
		{"non-hex digest", "demo-manifest:sha256:" + string(make([]byte, 64))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, ok := ParseContentAddress(tt.address)
			assert.False(t, ok)
		})
	}
}

func TestEvidenceGraph_AddNode_AcceptsValidNode(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	require.NoError(t, g.AddNode(node))
	assert.Equal(t, 1, g.NodeCount())
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_AddNode_RejectsEmptyArtifactID(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.ArtifactID = ""
	err := g.AddNode(node)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvidenceArtifactMalformed))
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_AddNode_RejectsEmptySHA256(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.SHA256 = ""
	err := g.AddNode(node)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvidenceArtifactMalformed))
}

func TestEvidenceGraph_AddNode_RejectsEmptyScopeID(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.ScopeID = ""
	err := g.AddNode(node)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvidenceScopeMismatch))
}

func TestEvidenceGraph_AddNode_RejectsOversizedArtifact(t *testing.T) {
	g := NewEvidenceGraph(10, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`this is more than 10 bytes`))
	err := g.AddNode(node)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvidenceArtifactTooLarge))
}

func TestEvidenceGraph_AddNode_AllowsZeroMaxBytes(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	large := make([]byte, 1024)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", large)
	require.NoError(t, g.AddNode(node))
	assert.Equal(t, 1, g.NodeCount())
}

func TestEvidenceGraph_AddNode_RejectsUnsupportedMediaType(t *testing.T) {
	g := NewEvidenceGraph(0, []string{"application/json"})
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.MediaType = "application/xml"
	err := g.AddNode(node)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvidenceMediaTypeUnsupported))
}

func TestEvidenceGraph_AddNode_AllowsEmptyMediaTypeWithAllowList(t *testing.T) {
	g := NewEvidenceGraph(0, []string{"application/json"})
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.MediaType = ""
	require.NoError(t, g.AddNode(node))
}

func TestEvidenceGraph_AddNode_RejectsPathTraversalBundlePath(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.BundlePath = "../escape.json"
	err := g.AddNode(node)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrPathValidation))
}

func TestEvidenceGraph_AddNode_RejectsAbsoluteBundlePath(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.BundlePath = "/etc/passwd"
	err := g.AddNode(node)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrPathValidation))
}

func TestEvidenceGraph_AddNode_AcceptsDuplicateWithSameContent(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	body := []byte(`{"id":"dup"}`)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", body)
	require.NoError(t, g.AddNode(node))
	require.NoError(t, g.AddNode(node))
	assert.Equal(t, 1, g.NodeCount())
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_AddNode_RejectsDuplicateWithConflictingContent(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node1 := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{"v":1}`))
	require.NoError(t, g.AddNode(node1))
	// Construct a node with the same ArtifactID but a different SHA-256.
	otherDigest := sha256.Sum256([]byte(`{"v":2}`))
	node2 := node1
	node2.SHA256 = hex.EncodeToString(otherDigest[:])
	err := g.AddNode(node2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvidenceDuplicateContent))
}

func TestEvidenceGraph_AddNode_RejectsDuplicateWithConflictingScope(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	body := []byte(`{"id":"dup"}`)
	node1 := validNode(ArtifactTypeDemoManifest, "scope-1", body)
	require.NoError(t, g.AddNode(node1))
	node2 := node1
	node2.ScopeID = "scope-2"
	err := g.AddNode(node2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvidenceScopeMismatch))
}

func TestEvidenceGraph_ResolveReferences_DetectsUnresolved(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoResult, "scope-1", []byte(`{}`))
	node.References = []string{"demo-manifest:sha256:missing"}
	require.NoError(t, g.AddNode(node))
	g.ResolveReferences()
	assert.False(t, g.Valid())
	found := false
	for _, f := range g.Failures() {
		if errors.Is(f.Code, constants.ErrUnresolvedReference) {
			found = true
		}
	}
	assert.True(t, found)
}

func TestEvidenceGraph_ResolveReferences_PassesForResolved(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	manifest := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{"m":1}`))
	require.NoError(t, g.AddNode(manifest))
	result := validNode(ArtifactTypeDemoResult, "scope-1", []byte(`{"r":1}`))
	result.References = []string{manifest.ArtifactID}
	require.NoError(t, g.AddNode(result))
	g.ResolveReferences()
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_DetectCycles_DetectsSelfReference(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoResult, "scope-1", []byte(`{}`))
	node.ArtifactID = "self-ref"
	node.References = []string{"self-ref"}
	require.NoError(t, g.AddNode(node))
	g.DetectCycles()
	assert.False(t, g.Valid())
	found := false
	for _, f := range g.Failures() {
		if errors.Is(f.Code, constants.ErrEvidenceCycleDetected) {
			found = true
		}
	}
	assert.True(t, found)
}

func TestEvidenceGraph_DetectCycles_DetectsMutualReference(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	nodeA := validNode(ArtifactTypeDemoResult, "scope-1", []byte(`{"a":1}`))
	nodeA.ArtifactID = "node-a"
	nodeA.References = []string{"node-b"}
	require.NoError(t, g.AddNode(nodeA))
	nodeB := validNode(ArtifactTypeDemoResult, "scope-1", []byte(`{"b":1}`))
	nodeB.ArtifactID = "node-b"
	nodeB.References = []string{"node-a"}
	require.NoError(t, g.AddNode(nodeB))
	g.DetectCycles()
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_DetectCycles_PassesForAcyclic(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	a := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{"a":1}`))
	a.ArtifactID = "node-a"
	require.NoError(t, g.AddNode(a))
	b := validNode(ArtifactTypeDemoResult, "scope-1", []byte(`{"b":1}`))
	b.ArtifactID = "node-b"
	b.References = []string{"node-a"}
	require.NoError(t, g.AddNode(b))
	g.DetectCycles()
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_ValidateScopeBinding_DetectsRunScopeConflict(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node1 := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{"1":1}`))
	node1.RunID = "run-x"
	require.NoError(t, g.AddNode(node1))
	node2 := validNode(ArtifactTypeDemoResult, "scope-2", []byte(`{"2":1}`))
	node2.RunID = "run-x"
	require.NoError(t, g.AddNode(node2))
	g.ValidateScopeBinding()
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_ValidateScopeBinding_DetectsAttemptRunConflict(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node1 := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{"1":1}`))
	node1.RunID = "run-1"
	node1.AttemptID = "att-1"
	require.NoError(t, g.AddNode(node1))
	node2 := validNode(ArtifactTypeDemoResult, "scope-1", []byte(`{"2":1}`))
	node2.RunID = "run-2"
	node2.AttemptID = "att-1"
	require.NoError(t, g.AddNode(node2))
	g.ValidateScopeBinding()
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_ValidateScopeBinding_DetectsScenarioRunConflict(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node1 := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{"1":1}`))
	node1.RunID = "run-1"
	node1.ScenarioID = "scen-1"
	require.NoError(t, g.AddNode(node1))
	node2 := validNode(ArtifactTypeDemoResult, "scope-1", []byte(`{"2":1}`))
	node2.RunID = "run-2"
	node2.ScenarioID = "scen-1"
	require.NoError(t, g.AddNode(node2))
	g.ValidateScopeBinding()
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_ValidateScopeBinding_PassesForConsistentBindings(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	for i := 0; i < 3; i++ {
		node := validNode(ArtifactTypeDemoResult, "scope-1", []byte(fmt.Sprintf(`{"i":%d}`, i)))
		node.RunID = "run-1"
		node.AttemptID = "att-1"
		node.ScenarioID = "scen-1"
		require.NoError(t, g.AddNode(node))
	}
	g.ValidateScopeBinding()
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_ValidateFreshness_DetectsMissingTimestamp(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.ProducedAt = time.Time{}
	require.NoError(t, g.AddNode(node))
	g.ValidateFreshness(time.Time{}, time.Time{})
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_ValidateFreshness_DetectsBeforeWindowStart(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.ProducedAt = time.Unix(1_000_000_000, 0).UTC()
	require.NoError(t, g.AddNode(node))
	g.ValidateFreshness(time.Unix(2_000_000_000, 0).UTC(), time.Time{})
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_ValidateFreshness_DetectsAfterWindowEnd(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.ProducedAt = time.Unix(3_000_000_000, 0).UTC()
	require.NoError(t, g.AddNode(node))
	g.ValidateFreshness(time.Time{}, time.Unix(2_000_000_000, 0).UTC())
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_ValidateFreshness_PassesWithinWindow(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.ProducedAt = time.Unix(1_500_000_000, 0).UTC()
	require.NoError(t, g.AddNode(node))
	g.ValidateFreshness(time.Unix(1_000_000_000, 0).UTC(), time.Unix(2_000_000_000, 0).UTC())
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_ValidateEncryption_DetectsIncompleteMetadata(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.Encryption = &EncryptionMetadata{Algorithm: "aes-256-gcm"}
	require.NoError(t, g.AddNode(node))
	g.ValidateEncryption()
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_ValidateEncryption_AcceptsDistinctAccessAuthorizationScope(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.Encryption = &EncryptionMetadata{
		Algorithm:                   "aes-256-gcm",
		KeyID:                       "key-1",
		AuthorizationScope:          "restricted-evidence-readers",
		PlaintextSHA256:             "abc",
		AuthenticatedMetadataSHA256: "def",
	}
	require.NoError(t, g.AddNode(node))
	g.ValidateEncryption()
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_ValidateEncryption_PassesForCompleteMatchingMetadata(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.Encryption = &EncryptionMetadata{
		Algorithm:                   "aes-256-gcm",
		KeyID:                       "key-1",
		AuthorizationScope:          "scope-1",
		PlaintextSHA256:             "abc",
		AuthenticatedMetadataSHA256: "def",
	}
	require.NoError(t, g.AddNode(node))
	g.ValidateEncryption()
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_ValidateEncryption_PassesForNoEncryption(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	require.NoError(t, g.AddNode(node))
	g.ValidateEncryption()
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_VerifyDigests_DetectsMismatch(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.SHA256 = "deadbeef" + "00000000000000000000000000000000000000000000000000000000000000"
	require.NoError(t, g.AddNode(node))
	// AddNode does not verify digests; VerifyDigests does.
	g.VerifyDigests()
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_VerifyDigests_PassesForMatchingDigest(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	body := []byte(`{"verified":true}`)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", body)
	require.NoError(t, g.AddNode(node))
	g.VerifyDigests()
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_VerifyDigests_SkipsNodesWithoutCanonicalBytes(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.CanonicalBytes = nil
	require.NoError(t, g.AddNode(node))
	g.VerifyDigests()
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_ValidateTrust_DetectsMissingVerifier(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.VerificationStatus = VerificationStatusVerified
	node.VerifierID = ""
	require.NoError(t, g.AddNode(node))
	g.ValidateTrust()
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_ValidateTrust_DetectsMissingProducer(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.ProducerIdentity = ""
	require.NoError(t, g.AddNode(node))
	g.ValidateTrust()
	assert.False(t, g.Valid())
}

func TestEvidenceGraph_ValidateTrust_PassesForUnverifiedWithoutVerifier(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.VerificationStatus = VerificationStatusUnverified
	node.VerifierID = ""
	node.VerifierVersion = ""
	require.NoError(t, g.AddNode(node))
	g.ValidateTrust()
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_ValidateAll_RunsAllChecksInOrder(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	// Add a node with a bad digest to trigger VerifyDigests.
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.SHA256 = "wrong"
	require.NoError(t, g.AddNode(node))
	g.ValidateAll(time.Time{}, time.Time{})
	assert.False(t, g.Valid())
	assert.Greater(t, len(g.Failures()), 0)
}

func TestEvidenceGraph_ValidateAll_PassesForValidGraph(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	manifest := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{"m":1}`))
	require.NoError(t, g.AddNode(manifest))
	result := validNode(ArtifactTypeDemoResult, "scope-1", []byte(`{"r":1}`))
	result.References = []string{manifest.ArtifactID}
	require.NoError(t, g.AddNode(result))
	g.ValidateAll(time.Unix(1_000_000_000, 0).UTC(), time.Unix(2_000_000_000, 0).UTC())
	assert.True(t, g.Valid())
}

func TestEvidenceGraph_NodesByScope_ReturnsBoundNodes(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	require.NoError(t, g.AddNode(validNode(ArtifactTypeDemoManifest, "scope-a", []byte(`{"a":1}`))))
	require.NoError(t, g.AddNode(validNode(ArtifactTypeDemoResult, "scope-a", []byte(`{"a":2}`))))
	require.NoError(t, g.AddNode(validNode(ArtifactTypeDemoManifest, "scope-b", []byte(`{"b":1}`))))
	assert.Len(t, g.NodesByScope("scope-a"), 2)
	assert.Len(t, g.NodesByScope("scope-b"), 1)
	assert.Empty(t, g.NodesByScope("scope-c"))
}

func TestEvidenceGraph_NodesByType_ReturnsTypedNodes(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	require.NoError(t, g.AddNode(validNode(ArtifactTypeDemoManifest, "scope-a", []byte(`{"a":1}`))))
	require.NoError(t, g.AddNode(validNode(ArtifactTypeDemoManifest, "scope-a", []byte(`{"a":2}`))))
	require.NoError(t, g.AddNode(validNode(ArtifactTypeDemoResult, "scope-a", []byte(`{"a":3}`))))
	assert.Len(t, g.NodesByType(ArtifactTypeDemoManifest), 2)
	assert.Len(t, g.NodesByType(ArtifactTypeDemoResult), 1)
	assert.Empty(t, g.NodesByType(ArtifactTypeActionReceipt))
}

func TestEvidenceGraph_Node_ReturnsNodeByID(t *testing.T) {
	g := NewEvidenceGraph(0, nil)
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	require.NoError(t, g.AddNode(node))
	found := g.Node(node.ArtifactID)
	require.NotNil(t, found)
	assert.Equal(t, node.ArtifactID, found.ArtifactID)
	assert.Nil(t, g.Node("nonexistent"))
}

func TestEvidenceNode_ToProto_RoundTripsCoreFields(t *testing.T) {
	node := validNode(ArtifactTypeActionReceipt, "scope-1", []byte(`{}`))
	node.TransactionID = "tx-001"
	node.Encryption = &EncryptionMetadata{
		Algorithm:                   "aes-256-gcm",
		KeyID:                       "key-1",
		AuthorizationScope:          "scope-1",
		PlaintextSHA256:             "abc123",
		AuthenticatedMetadataSHA256: "def456",
	}
	ref := node.ToProto()
	assert.Equal(t, node.ArtifactID, ref.GetArtifactId())
	assert.Equal(t, string(node.ArtifactType), ref.GetArtifactType())
	assert.Equal(t, node.SHA256, ref.GetSha256())
	assert.Equal(t, node.MediaType, ref.GetMediaType())
	assert.Equal(t, node.ProducerIdentity, ref.GetProducerIdentity())
	assert.Equal(t, node.ScopeID, ref.GetScopeId())
	assert.Equal(t, node.RunID, ref.GetRunId())
	assert.Equal(t, node.AttemptID, ref.GetAttemptId())
	assert.Equal(t, node.ScenarioID, ref.GetScenarioId())
	assert.Equal(t, node.TransactionID, ref.GetTransactionId())
	assert.Equal(t, string(node.VerificationStatus), ref.GetVerificationStatus())
	assert.Equal(t, node.VerifierID, ref.GetVerifierId())
	assert.Equal(t, node.VerifierVersion, ref.GetVerifierVersion())
	assert.Equal(t, node.BundlePath, ref.GetBundlePath())
	assert.NotNil(t, ref.GetProducedAt())
	assert.NotNil(t, ref.GetVerifiedAt())
	require.NotNil(t, ref.GetEncryption())
	assert.Equal(t, "aes-256-gcm", ref.GetEncryption().GetAlgorithm())
	assert.Equal(t, "key-1", ref.GetEncryption().GetKeyId())
	assert.Equal(t, "scope-1", ref.GetEncryption().GetAuthorizationScope())
	assert.Equal(t, "abc123", ref.GetEncryption().GetPlaintextSha256())
	assert.Equal(t, "def456", ref.GetEncryption().GetAuthenticatedMetadataSha256())
}

func TestEvidenceNode_ToProto_OmitsZeroTimestamps(t *testing.T) {
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	node.ProducedAt = time.Time{}
	node.VerifiedAt = time.Time{}
	ref := node.ToProto()
	assert.Nil(t, ref.GetProducedAt())
	assert.Nil(t, ref.GetVerifiedAt())
}

func TestEvidenceNode_ToProto_OmitsNilEncryption(t *testing.T) {
	node := validNode(ArtifactTypeDemoManifest, "scope-1", []byte(`{}`))
	ref := node.ToProto()
	assert.Nil(t, ref.GetEncryption())
}

func TestValidBundlePath_RejectsUnsafePaths(t *testing.T) {
	assert.False(t, validBundlePath(""))
	assert.False(t, validBundlePath("/absolute/path"))
	assert.False(t, validBundlePath(".."))
	assert.False(t, validBundlePath("../escape"))
	assert.False(t, validBundlePath("../../escape"))
}

func TestValidBundlePath_AcceptsSafePaths(t *testing.T) {
	assert.True(t, validBundlePath("manifest.json"))
	assert.True(t, validBundlePath("receipts/abc.json"))
	assert.True(t, validBundlePath("a/b/c.json"))
}
