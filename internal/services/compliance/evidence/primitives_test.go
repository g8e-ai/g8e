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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func TestCanonicalProtoBodyEqual_ComparesAgainstCanonicalProtocolBytes(t *testing.T) {
	message := &compliancev1.ComplianceVerificationReport{ReportId: "report-1", Valid: true}
	body, err := compliancev1.MarshalCanonical(message)
	require.NoError(t, err)

	equal, err := CanonicalProtoBodyEqual(body, message)
	require.NoError(t, err)
	assert.True(t, equal)
	equal, err = CanonicalProtoBodyEqual(append(body, ' '), message)
	require.NoError(t, err)
	assert.False(t, equal)
}

func TestCanonicalProtosEqual_PreservesRepeatedFieldOrder(t *testing.T) {
	left := &compliancev1.ComplianceVerificationReport{ReportId: "report-1", Failures: []*compliancev1.VerificationFailure{{Code: "a"}, {Code: "b"}}}
	right := proto.Clone(left).(*compliancev1.ComplianceVerificationReport)

	equal, err := CanonicalProtosEqual(left, right)
	require.NoError(t, err)
	assert.True(t, equal)
	right.Failures[0], right.Failures[1] = right.Failures[1], right.Failures[0]
	equal, err = CanonicalProtosEqual(left, right)
	require.NoError(t, err)
	assert.False(t, equal)
}

func TestNewVerificationCheckResult_EmitsTypedPassedAndFailedChecks(t *testing.T) {
	passed := NewVerificationCheckResult("check-1", "verifier-1", "1.0.0", []string{"b", "a", "a"}, nil)
	assert.Equal(t, compliancev1.VerificationCheckStatus_VERIFICATION_CHECK_STATUS_PASSED, passed.GetStatus())
	assert.Equal(t, []string{"a", "b"}, passed.GetEvidenceRefs())
	failure := &compliancev1.VerificationFailure{Code: "code", SubjectRef: "subject", Reason: "reason"}
	failed := NewVerificationCheckResult("check-1", "verifier-1", "1.0.0", []string{"evidence"}, []*compliancev1.VerificationFailure{failure})
	assert.Equal(t, compliancev1.VerificationCheckStatus_VERIFICATION_CHECK_STATUS_FAILED, failed.GetStatus())
	assert.Equal(t, []string{"evidence", "subject"}, failed.GetEvidenceRefs())
	assert.Equal(t, []*compliancev1.VerificationFailure{failure}, failed.GetFailures())
}

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

func TestEqualStringSets_RequiresEqualMultiplicityIndependentOfOrder(t *testing.T) {
	tests := []struct {
		name     string
		left     []string
		right    []string
		expected bool
	}{
		{name: "same values in different order", left: []string{"a", "b", "c"}, right: []string{"c", "a", "b"}, expected: true},
		{name: "both empty", left: []string{}, right: []string{}, expected: true},
		{name: "different lengths", left: []string{"a", "b"}, right: []string{"a", "b", "c"}},
		{name: "different values", left: []string{"a", "b", "c"}, right: []string{"a", "b", "d"}},
		{name: "duplicate replaces distinct value", left: []string{"a", "a"}, right: []string{"a", "b"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, EqualStringSets(test.left, test.right))
		})
	}
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

func TestValidateVerificationReport_EnforcesCanonicalBoundSuccessfulReport(t *testing.T) {
	verifiedAt := time.Unix(1_700_000_000, 0).UTC()
	valid := &compliancev1.ComplianceVerificationReport{
		ReportId:        "run-1",
		Valid:           true,
		VerifiedAt:      timestamppb.New(verifiedAt),
		VerifierId:      constants.DemoRunVerifierID,
		VerifierVersion: constants.DemoRunVerifierVersion,
		Checks: []*compliancev1.VerificationCheckResult{NewVerificationCheckResult(
			constants.DemoRunVerificationCheck,
			constants.DemoRunVerifierID,
			constants.DemoRunVerifierVersion,
			[]string{"run-1"},
			nil,
		)},
	}
	tests := []struct {
		name      string
		mutate    func(*compliancev1.ComplianceVerificationReport)
		notAfter  time.Time
		targetErr error
	}{
		{name: "valid bound report", notAfter: verifiedAt},
		{name: "wrong report identity", mutate: func(report *compliancev1.ComplianceVerificationReport) { report.ReportId = "run-2" }, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "wrong verifier identity", mutate: func(report *compliancev1.ComplianceVerificationReport) {
			report.VerifierId = constants.EvalRunVerifierID
		}, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "wrong verifier version", mutate: func(report *compliancev1.ComplianceVerificationReport) { report.VerifierVersion = "2.0.0" }, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "invalid report", mutate: func(report *compliancev1.ComplianceVerificationReport) { report.Valid = false }, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "report contains failures", mutate: func(report *compliancev1.ComplianceVerificationReport) {
			report.Failures = []*compliancev1.VerificationFailure{{Code: constants.ErrChecksumMismatch.Error()}}
		}, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "missing verification check", mutate: func(report *compliancev1.ComplianceVerificationReport) { report.Checks = nil }, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "duplicate verification check", mutate: func(report *compliancev1.ComplianceVerificationReport) {
			report.Checks = append(report.Checks, proto.Clone(report.Checks[0]).(*compliancev1.VerificationCheckResult))
		}, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "wrong verification check identity", mutate: func(report *compliancev1.ComplianceVerificationReport) {
			report.Checks[0].CheckId = constants.EvalRunVerificationCheck
		}, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "unspecified verification check status", mutate: func(report *compliancev1.ComplianceVerificationReport) {
			report.Checks[0].Status = compliancev1.VerificationCheckStatus_VERIFICATION_CHECK_STATUS_UNSPECIFIED
		}, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "verification check lacks run evidence", mutate: func(report *compliancev1.ComplianceVerificationReport) {
			report.Checks[0].EvidenceRefs = []string{"other-run"}
		}, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "verification check verifier mismatch", mutate: func(report *compliancev1.ComplianceVerificationReport) {
			report.Checks[0].VerifierId = constants.EvalRunVerifierID
		}, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "missing verification time", mutate: func(report *compliancev1.ComplianceVerificationReport) { report.VerifiedAt = nil }, notAfter: verifiedAt, targetErr: constants.ErrReportVerificationFailed},
		{name: "verification after cutoff", notAfter: verifiedAt.Add(-time.Nanosecond), targetErr: constants.ErrReportVerificationFailed},
		{name: "missing cutoff", targetErr: constants.ErrReportVerificationFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := proto.Clone(valid).(*compliancev1.ComplianceVerificationReport)
			if test.mutate != nil {
				test.mutate(report)
			}
			body, err := compliancev1.MarshalCanonical(report)
			require.NoError(t, err)

			decoded, err := ValidateVerificationReport(body, "run-1", constants.DemoRunVerifierID, constants.DemoRunVerifierVersion, test.notAfter)
			if test.targetErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, test.targetErr)
				assert.Nil(t, decoded)
				return
			}
			require.NoError(t, err)
			assert.True(t, proto.Equal(report, decoded))
		})
	}
}

func TestValidateVerificationReport_RejectsNoncanonicalBody(t *testing.T) {
	report, err := ValidateVerificationReport([]byte(`{"report_id": "run-1"}`), "run-1", constants.DemoRunVerifierID, constants.DemoRunVerifierVersion, time.Unix(1_700_000_000, 0).UTC())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
	assert.Nil(t, report)
}
