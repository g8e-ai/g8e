// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func TestReadAndDigest_ReturnsBytesAndDigest(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	reader := &memoryArtifactReader{files: map[string][]byte{"path": body}}
	result, err := ReadAndDigest(reader, context.Background(), "path", 0)
	require.NoError(t, err)
	assert.Equal(t, body, result.Bytes)
	digest := sha256.Sum256(body)
	assert.Equal(t, hex.EncodeToString(digest[:]), result.SHA256)
}

func TestReadAndDigest_PropagatesReadError(t *testing.T) {
	reader := &memoryArtifactReader{files: map[string][]byte{}}
	_, err := ReadAndDigest(reader, context.Background(), "missing", 0)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrNotFound))
}

func TestReadAndDigest_EnforcesSizeLimit(t *testing.T) {
	body := []byte("this is more than 5 bytes")
	reader := &memoryArtifactReader{files: map[string][]byte{"path": body}}
	_, err := ReadAndDigest(reader, context.Background(), "path", 5)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvidenceArtifactTooLarge))
}

func TestReadAndDigest_DefaultMaxBytesWhenZero(t *testing.T) {
	body := make([]byte, 100)
	reader := &memoryArtifactReader{files: map[string][]byte{"path": body}}
	result, err := ReadAndDigest(reader, context.Background(), "path", 0)
	require.NoError(t, err)
	assert.Equal(t, body, result.Bytes)
}

func TestVerifyDigest_MatchesCorrectDigest(t *testing.T) {
	body := []byte(`{"verified":true}`)
	digest := sha256.Sum256(body)
	assert.True(t, VerifyDigest(body, hex.EncodeToString(digest[:])))
}

func TestVerifyDigest_RejectsIncorrectDigest(t *testing.T) {
	assert.False(t, VerifyDigest([]byte(`{}`), "wrong"))
}

func TestContentReferenceForBody_FormatAndDeterminism(t *testing.T) {
	body := []byte(`{"ref":"test"}`)
	ref := ContentReferenceForBody("action-receipt", body)
	digest := sha256.Sum256(body)
	expected := "action-receipt:sha256:" + hex.EncodeToString(digest[:])
	assert.Equal(t, expected, ref)
}

func TestParseContentReference_ValidReference(t *testing.T) {
	body := []byte(`{}`)
	ref := ContentReferenceForBody("demo-manifest", body)
	prefix, digest, ok := ParseContentReference(ref)
	require.True(t, ok)
	assert.Equal(t, "demo-manifest", prefix)
	assert.Len(t, digest, 64)
}

func TestParseContentReference_InvalidReferences(t *testing.T) {
	emptyDigest := sha256.Sum256(nil)
	tests := []struct {
		name      string
		reference string
	}{
		{"empty", ""},
		{"missing prefix", ":sha256:" + hex.EncodeToString(emptyDigest[:])},
		{"wrong algorithm", "demo:md5:abc"},
		{"uppercase digest", "demo:sha256:" + strings.ToUpper(hex.EncodeToString(emptyDigest[:]))},
		{"too short", "demo:sha256:abc"},
		{"too many parts", "demo:sha256:abc:extra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, ok := ParseContentReference(tt.reference)
			assert.False(t, ok)
		})
	}
}

func TestParseExpectedContentReference_MatchesPrefix(t *testing.T) {
	body := []byte(`{}`)
	ref := ContentReferenceForBody("action-receipt", body)
	prefix, digest, ok := ParseExpectedContentReference(ref, "action-receipt")
	require.True(t, ok)
	assert.Equal(t, "action-receipt", prefix)
	assert.Len(t, digest, 64)
}

func TestParseExpectedContentReference_RejectsWrongPrefix(t *testing.T) {
	body := []byte(`{}`)
	ref := ContentReferenceForBody("action-receipt", body)
	_, _, ok := ParseExpectedContentReference(ref, "demo-manifest")
	assert.False(t, ok)
}

func TestValidateCanonicalJSON_AcceptsCanonicalCompact(t *testing.T) {
	body := []byte(`{"key":"value","num":42}`)
	require.NoError(t, ValidateCanonicalJSON(body))
}

func TestValidateCanonicalJSON_RejectsNonCanonicalWhitespace(t *testing.T) {
	body := []byte(`{"key": "value"}`)
	err := ValidateCanonicalJSON(body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "canonical compact")
}

func TestValidateCanonicalJSON_RejectsTrailingData(t *testing.T) {
	body := []byte(`{"key":"value"}extra`)
	err := ValidateCanonicalJSON(body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing data")
}

func TestValidateCanonicalJSON_RejectsInvalidJSON(t *testing.T) {
	body := []byte(`{invalid}`)
	err := ValidateCanonicalJSON(body)
	require.Error(t, err)
}

func TestValidPathElement_AcceptsSafeElement(t *testing.T) {
	assert.True(t, ValidPathElement("manifest.json"))
	assert.True(t, ValidPathElement("abc123"))
}

func TestValidPathElement_RejectsUnsafeElements(t *testing.T) {
	assert.False(t, ValidPathElement(""))
	assert.False(t, ValidPathElement("."))
	assert.False(t, ValidPathElement(".."))
	assert.False(t, ValidPathElement("a/b"))
	assert.False(t, ValidPathElement("/absolute"))
}

func TestValidRelativePath_AcceptsSafeRelativePath(t *testing.T) {
	assert.True(t, ValidRelativePath("receipts/abc.json"))
	assert.True(t, ValidRelativePath("a/b/c.json"))
}

func TestValidRelativePath_RejectsUnsafePaths(t *testing.T) {
	assert.False(t, ValidRelativePath(""))
	assert.False(t, ValidRelativePath("."))
	assert.False(t, ValidRelativePath(".."))
	assert.False(t, ValidRelativePath("../escape"))
	assert.False(t, ValidRelativePath("/absolute"))
}

func TestSignerPublicKey_DecodesValidHexKey(t *testing.T) {
	_, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	hexKey := hex.EncodeToString(pub)
	decoded, err := SignerPublicKey(hexKey)
	require.NoError(t, err)
	assert.Equal(t, ed25519.PublicKeySize, len(decoded))
	assert.Equal(t, pub, decoded)
}

func TestSignerPublicKey_RejectsInvalidInput(t *testing.T) {
	_, err := SignerPublicKey("not-hex")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrTrustedSignerKeyNotFound))

	_, err = SignerPublicKey(hex.EncodeToString([]byte("short")))
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrTrustedSignerKeyNotFound))
}

func TestClassifyReadError_MapsNotFoundToUnresolvedReference(t *testing.T) {
	assert.True(t, errors.Is(ClassifyReadError(constants.ErrNotFound), constants.ErrUnresolvedReference))
}

func TestClassifyReadError_PreservesTooLarge(t *testing.T) {
	assert.True(t, errors.Is(ClassifyReadError(constants.ErrEvidenceArtifactTooLarge), constants.ErrEvidenceArtifactTooLarge))
}

func TestClassifyReadError_MapsUnknownToInvalidGraph(t *testing.T) {
	assert.True(t, errors.Is(ClassifyReadError(errors.New("unknown")), constants.ErrInvalidEvidenceGraph))
}

func TestContains_FindsTarget(t *testing.T) {
	assert.True(t, Contains([]string{"a", "b", "c"}, "b"))
	assert.False(t, Contains([]string{"a", "b", "c"}, "d"))
	assert.False(t, Contains([]string{}, "a"))
}

func TestEqualStringSets_OrderIndependent(t *testing.T) {
	assert.True(t, EqualStringSets([]string{"a", "b", "c"}, []string{"c", "a", "b"}))
	assert.True(t, EqualStringSets([]string{}, []string{}))
	assert.False(t, EqualStringSets([]string{"a", "b"}, []string{"a", "b", "c"}))
	assert.False(t, EqualStringSets([]string{"a", "b", "c"}, []string{"a", "b", "d"}))
}

func TestVersionedKey_ConcatenatesWithAtSign(t *testing.T) {
	assert.Equal(t, "grader@1.0.0", VersionedKey("grader", "1.0.0"))
}

func TestMarshalCanonicalProto_RoundTripsDemoManifest(t *testing.T) {
	manifest := &compliancev1.DemoManifest{
		DemoId: "fedramp", RunId: "run-1", ScopeId: "scope-1",
		GeneratedAt: timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
	}
	body, err := MarshalCanonicalProto(manifest)
	require.NoError(t, err)
	assert.NotEmpty(t, body)

	decoded := &compliancev1.DemoManifest{}
	require.NoError(t, UnmarshalCanonicalProto(body, decoded))
	assert.Equal(t, manifest.GetDemoId(), decoded.GetDemoId())
	assert.Equal(t, manifest.GetRunId(), decoded.GetRunId())
}

func TestDemoScope_MapsKnownDemoIDs(t *testing.T) {
	assert.Equal(t, constants.DemoScopeFedRAMP, DemoScope(constants.DemosOrgFedRAMP))
	assert.Equal(t, constants.DemoScopeHealthcare, DemoScope(constants.DemosOrgHealthcare))
}

func TestDemoScope_ReturnsEmptyForUnknownID(t *testing.T) {
	assert.Equal(t, "", DemoScope("unknown"))
}
