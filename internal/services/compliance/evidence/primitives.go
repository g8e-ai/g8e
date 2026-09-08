// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// maxArtifactBytes is the default size limit for a single evidence artifact
// read by an importer. Importers may override this with a larger or smaller
// limit.
const maxArtifactBytes = 16 << 20

// ReadResult holds the bytes and digest of a read artifact.
type ReadResult struct {
	Bytes  []byte
	SHA256 string
}

// ReadAndDigest reads a file from the given reader, enforces the size limit,
// and computes the SHA-256 digest of the bytes.
func ReadAndDigest(reader ArtifactReader, ctx context.Context, path string, maxBytes int) (ReadResult, error) {
	if maxBytes <= 0 {
		maxBytes = maxArtifactBytes
	}
	body, err := reader.ReadFile(ctx, path)
	if err != nil {
		return ReadResult{}, err
	}
	if len(body) > maxBytes {
		return ReadResult{}, constants.ErrEvidenceArtifactTooLarge
	}
	digest := sha256.Sum256(body)
	return ReadResult{Bytes: body, SHA256: hex.EncodeToString(digest[:])}, nil
}

// UnmarshalCanonicalProto unmarshals canonical JSON bytes into a proto
// message using the compliance canonical decoder.
func UnmarshalCanonicalProto(body []byte, msg proto.Message) error {
	return compliancev1.UnmarshalCanonical(body, msg)
}

// MarshalCanonicalProto marshals a proto message into canonical JSON bytes.
func MarshalCanonicalProto(msg proto.Message) ([]byte, error) {
	return compliancev1.MarshalCanonical(msg)
}

func CanonicalProtoBodyEqual(body []byte, expected proto.Message) (bool, error) {
	canonical, err := compliancev1.MarshalCanonical(expected)
	if err != nil {
		return false, fmt.Errorf("canonicalize expected protocol message: %w", err)
	}
	return bytes.Equal(body, canonical), nil
}

func CanonicalProtosEqual(left, right proto.Message) (bool, error) {
	leftBody, err := compliancev1.MarshalCanonical(left)
	if err != nil {
		return false, fmt.Errorf("canonicalize left protocol message: %w", err)
	}
	rightBody, err := compliancev1.MarshalCanonical(right)
	if err != nil {
		return false, fmt.Errorf("canonicalize right protocol message: %w", err)
	}
	return bytes.Equal(leftBody, rightBody), nil
}

func NewVerificationCheckResult(checkID, verifierID, verifierVersion string, evidenceRefs []string, failures []*compliancev1.VerificationFailure) *compliancev1.VerificationCheckResult {
	refs := append([]string(nil), evidenceRefs...)
	for _, failure := range failures {
		refs = append(refs, failure.GetSubjectRef())
	}
	sort.Strings(refs)
	uniqueRefs := refs[:0]
	for _, reference := range refs {
		if reference != "" && (len(uniqueRefs) == 0 || uniqueRefs[len(uniqueRefs)-1] != reference) {
			uniqueRefs = append(uniqueRefs, reference)
		}
	}
	status := compliancev1.VerificationCheckStatus_VERIFICATION_CHECK_STATUS_PASSED
	if len(failures) > 0 {
		status = compliancev1.VerificationCheckStatus_VERIFICATION_CHECK_STATUS_FAILED
	}
	return &compliancev1.VerificationCheckResult{CheckId: checkID, Status: status, EvidenceRefs: uniqueRefs, VerifierId: verifierID, VerifierVersion: verifierVersion, Failures: append([]*compliancev1.VerificationFailure(nil), failures...)}
}

// VerifyDigest compares the SHA-256 of the given bytes to the expected
// hex-encoded digest.
func VerifyDigest(body []byte, expected string) bool {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]) == expected
}

// ContentReferenceForBody computes the content-addressed reference for the
// given prefix and body bytes: "<prefix>:sha256:<hex-digest>".
func ContentReferenceForBody(prefix string, body []byte) string {
	digest := sha256.Sum256(body)
	return prefix + ":sha256:" + hex.EncodeToString(digest[:])
}

// ParseContentReference splits a content-addressed reference into its prefix,
// digest, and validity flag. The format is "<prefix>:sha256:<64-hex-chars>".
func ParseContentReference(reference string) (string, string, bool) {
	parts := strings.Split(reference, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "sha256" || len(parts[2]) != sha256.Size*2 || strings.ToLower(parts[2]) != parts[2] {
		return "", "", false
	}
	decoded, err := hex.DecodeString(parts[2])
	return parts[0], parts[2], err == nil && len(decoded) == sha256.Size
}

// ParseExpectedContentReference parses a content reference and verifies the
// prefix matches the expected value.
func ParseExpectedContentReference(reference, expectedPrefix string) (string, string, bool) {
	prefix, digest, ok := ParseContentReference(reference)
	return prefix, digest, ok && prefix == expectedPrefix
}

// ValidateCanonicalJSON verifies that the bytes are valid JSON with no
// trailing data and that the encoding is canonical compact (no extra
// whitespace).
func ValidateCanonicalJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON contains trailing data")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return err
	}
	if !bytes.Equal(body, compact.Bytes()) {
		return fmt.Errorf("JSON is not canonical compact encoding")
	}
	return nil
}

// ValidPathElement returns true if the value is a single safe path component
// (no separators, no traversal, not empty).
func ValidPathElement(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !filepath.IsAbs(value)
}

// ValidRelativePath returns true if the value is a safe relative path that
// does not escape the root via traversal.
func ValidRelativePath(value string) bool {
	clean := filepath.Clean(value)
	return clean != "." && clean != ".." && !filepath.IsAbs(clean) && !strings.HasPrefix(clean, ".."+string(os.PathSeparator))
}

// SignerPublicKey decodes a hex-encoded Ed25519 public key.
func SignerPublicKey(keyID string) (ed25519.PublicKey, error) {
	return governance.SignerPublicKey(keyID)
}

// ClassifyReadError maps a read error to the appropriate evidence error
// constant.
func ClassifyReadError(err error) error {
	if errors.Is(err, constants.ErrNotFound) {
		return constants.ErrUnresolvedReference
	}
	if errors.Is(err, constants.ErrEvidenceArtifactTooLarge) {
		return constants.ErrEvidenceArtifactTooLarge
	}
	return constants.ErrInvalidEvidenceGraph
}

// Contains returns true if the target string appears in the values slice.
func Contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// EqualStringSets returns true if both slices contain the same set of
// strings (order-independent).
func EqualStringSets(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// VersionedKey returns "<id>@<version>".
func VersionedKey(id, version string) string {
	return id + "@" + version
}

// VerifyReceiptSignature verifies an ActionReceipt signature against the
// given Ed25519 public key. Returns nil if the signature is valid.
func VerifyReceiptSignature(receipt *operatorv1.ActionReceipt, publicKey ed25519.PublicKey) error {
	return governance.VerifyActionReceiptSignature(receipt, publicKey)
}

// VerifyReceiptPersistence verifies the final-persistence attestation on an
// ActionReceipt. Returns nil if the attestation is valid.
func VerifyReceiptPersistence(receipt *operatorv1.ActionReceipt, publicKey ed25519.PublicKey) error {
	return governance.VerifyReceiptPersistenceAttestation(receipt, publicKey)
}

func ReceiptActionType(receipt *operatorv1.ActionReceipt) (string, error) {
	return governance.DeterministicStageActionType(receipt)
}

// ReceiptInvestigationBound returns true if any deterministic stage evidence
// in the receipt carries one of the given investigation IDs.
func ReceiptInvestigationBound(receipt *operatorv1.ActionReceipt, investigationIDs []string) bool {
	for _, stage := range receipt.GetDeterministicStageEvidence() {
		if Contains(investigationIDs, stage.GetInvestigationId()) {
			return true
		}
	}
	return false
}

// ReceiptAttestationError maps a persistence attestation error to the
// appropriate typed error constant.
func ReceiptAttestationError(err error) error {
	for _, target := range []error{constants.ErrReceiptPersistenceAttestationMissing, constants.ErrReceiptPersistenceSignatureMismatch, constants.ErrReceiptPersistenceAttestationInvalid} {
		if errors.Is(err, target) {
			return target
		}
	}
	return constants.ErrReceiptPersistenceAttestationInvalid
}

// DemoScope returns the canonical scope ID for a demo organization ID.
func DemoScope(demoID string) string {
	switch demoID {
	case constants.DemosOrgFedRAMP:
		return constants.DemoScopeFedRAMP
	case constants.DemosOrgDHS:
		return constants.DemoScopeDHS
	case constants.DemosOrgFinance:
		return constants.DemoScopeFinance
	case constants.DemosOrgHealthcare:
		return constants.DemoScopeHealthcare
	default:
		return ""
	}
}

func ValidateVerificationReport(body []byte, reportID, verifierID, verifierVersion string, notAfter time.Time) (*compliancev1.ComplianceVerificationReport, error) {
	report := &compliancev1.ComplianceVerificationReport{}
	if err := compliancev1.UnmarshalCanonical(body, report); err != nil {
		return nil, fmt.Errorf("%w: decode canonical verification report: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if reportID == "" || verifierID == "" || verifierVersion == "" || notAfter.IsZero() || report.GetReportId() != reportID || report.GetVerifierId() != verifierID || report.GetVerifierVersion() != verifierVersion || !report.GetValid() || len(report.GetFailures()) != 0 || report.GetVerifiedAt() == nil || report.GetVerifiedAt().CheckValid() != nil || report.GetVerifiedAt().AsTime().After(notAfter) {
		return nil, fmt.Errorf("%w: verification report is invalid or does not match its declared binding", constants.ErrReportVerificationFailed)
	}
	expectedCheckID := ""
	switch {
	case verifierID == constants.DemoRunVerifierID && verifierVersion == constants.DemoRunVerifierVersion:
		expectedCheckID = constants.DemoRunVerificationCheck
	case verifierID == constants.EvalRunVerifierID && verifierVersion == constants.EvalRunVerifierVersion:
		expectedCheckID = constants.EvalRunVerificationCheck
	default:
		return nil, fmt.Errorf("%w: protected source verifier is unsupported", constants.ErrReportVerificationFailed)
	}
	if len(report.GetChecks()) != 1 {
		return nil, fmt.Errorf("%w: protected source verification report requires exactly one check", constants.ErrReportVerificationFailed)
	}
	check := report.GetChecks()[0]
	if check == nil || check.GetCheckId() != expectedCheckID || check.GetStatus() != compliancev1.VerificationCheckStatus_VERIFICATION_CHECK_STATUS_PASSED || check.GetVerifierId() != verifierID || check.GetVerifierVersion() != verifierVersion || len(check.GetEvidenceRefs()) != 1 || check.GetEvidenceRefs()[0] != reportID || len(check.GetFailures()) != 0 {
		return nil, fmt.Errorf("%w: protected source verification check is invalid or does not match its declared binding", constants.ErrReportVerificationFailed)
	}
	return report, nil
}

type healthcareMetricObservation struct {
	Action          string `json:"action"`
	RequestID       string `json:"request_id"`
	ResourceType    string `json:"resource_type"`
	Subject         string `json:"subject"`
	MeasuredValue   int64  `json:"measured_value"`
	ThresholdValue  int64  `json:"threshold_value"`
	RunID           string `json:"run_id"`
	ScenarioID      string `json:"scenario_id"`
	Status          string `json:"status"`
	AutoApproved    bool   `json:"auto_approved"`
	ReportableToOHA bool   `json:"reportable_to_oha"`
	EvaluatedAt     string `json:"evaluated_at"`
}

type healthcareMetricCollection struct {
	CollectorID             string                      `json:"collector_id"`
	CollectorVersion        string                      `json:"collector_version"`
	Boundary                string                      `json:"boundary"`
	InitialStateFixtureRef  string                      `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string                    `json:"terminal_state_assertions"`
	CollectedAt             time.Time                   `json:"collected_at"`
	Observation             healthcareMetricObservation `json:"observation"`
}

type healthcareMetricExpectation struct {
	MetricID, SubjectRef, Unit, Action, Status string
	AutoApproved, ReportableToOHA              bool
}

func ValidateDemoStateObservation(result *compliancev1.DemoScenarioResult, reference string, body []byte) error {
	if result == nil || ContentReferenceForBody("state-observation", body) != reference || !Contains(result.GetStateObservationRefs(), reference) {
		return fmt.Errorf("%w: state observation is not declared by the scenario result", constants.ErrUnresolvedReference)
	}
	for _, step := range result.GetStepResults() {
		if Contains(step.GetEvidenceRefs(), reference) && step.GetProtocolResult() == string(body) {
			return nil
		}
	}
	return fmt.Errorf("%w: state-observation body is not bound to a scenario step", constants.ErrUnresolvedReference)
}

func ValidateDemoMetricEvidence(result *compliancev1.DemoScenarioResult, metric *compliancev1.DemoMetricEvidence, observationBody []byte) error {
	if result == nil || metric == nil || metric.GetScenarioRef() == nil || metric.GetGraderRef() == nil || metric.GetEvaluatedAt() == nil || metric.GetEvaluatedAt().CheckValid() != nil {
		return fmt.Errorf("%w: metric evidence is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	expectations := map[string]healthcareMetricExpectation{
		"healthcare-gold-card": {
			MetricID: "healthcare-provider-approval-rate", SubjectRef: "PA-2026-0043", Unit: "percent",
			Action: "gold-card", Status: "AUTO_APPROVED", AutoApproved: true,
		},
		"healthcare-sla-breach": {
			MetricID: "healthcare-sla-elapsed-days", SubjectRef: "PA-2026-0044", Unit: "days",
			Action: "sla-check", Status: "SLA_BREACHED", ReportableToOHA: true,
		},
	}
	expected, ok := expectations[result.GetScenarioRef().GetId()]
	if !ok {
		return fmt.Errorf("%w: metric evidence is unsupported for scenario %s", constants.ErrUnsupportedGrader, result.GetScenarioRef().GetId())
	}
	if metric.GetMetricId() != expected.MetricID || metric.GetMetricVersion() != constants.DemoMetricEvidenceVersion ||
		metric.GetRunId() != result.GetRunId() || metric.GetScopeId() != result.GetScopeId() ||
		metric.GetScenarioRef().GetId() != result.GetScenarioRef().GetId() || metric.GetScenarioRef().GetVersion() != result.GetScenarioRef().GetVersion() ||
		metric.GetSubjectRef() != expected.SubjectRef || metric.GetUnit() != expected.Unit ||
		metric.GetComparison() != constants.DemoMetricComparisonGreaterThanOrEqual ||
		metric.GetGraderRef().GetId() != constants.DemoMetricGraderID || metric.GetGraderRef().GetVersion() != constants.DemoMetricGraderVersion {
		return fmt.Errorf("%w: metric identity, scope, or grader binding is invalid", constants.ErrInvalidEvidenceGraph)
	}
	if ContentReferenceForBody("state-observation", observationBody) != metric.GetSourceEvidenceRef() || !Contains(result.GetStateObservationRefs(), metric.GetSourceEvidenceRef()) {
		return fmt.Errorf("%w: metric source observation binding is invalid", constants.ErrEvidenceScopeMismatch)
	}
	decoder := json.NewDecoder(bytes.NewReader(observationBody))
	decoder.DisallowUnknownFields()
	collection := healthcareMetricCollection{}
	if err := decoder.Decode(&collection); err != nil {
		return fmt.Errorf("%w: decode metric source observation: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("%w: metric source observation contains trailing JSON", constants.ErrEvidenceArtifactMalformed)
	}
	observation := collection.Observation
	evaluatedAt, err := time.Parse(time.RFC3339Nano, observation.EvaluatedAt)
	if err != nil || collection.CollectedAt.Before(evaluatedAt) {
		return fmt.Errorf("%w: metric source timestamps are invalid", constants.ErrInvalidEvidenceGraph)
	}
	if collection.CollectorID != "healthcare-actuator-state" || collection.CollectorVersion != "1.0.0" || collection.Boundary != "healthcare-actuator" ||
		collection.InitialStateFixtureRef == "" || len(collection.TerminalStateAssertions) == 0 || observation.RunID != result.GetRunId() ||
		observation.ScenarioID != result.GetScenarioRef().GetId() || observation.RequestID != expected.SubjectRef || observation.Action != expected.Action ||
		observation.ResourceType != "ClaimResponse" || observation.Status != expected.Status || observation.AutoApproved != expected.AutoApproved ||
		observation.ReportableToOHA != expected.ReportableToOHA || metric.GetMeasuredValue() != observation.MeasuredValue ||
		metric.GetThresholdValue() != observation.ThresholdValue || !metric.GetEvaluatedAt().AsTime().Equal(evaluatedAt) {
		return fmt.Errorf("%w: metric does not reproduce its bound source observation", constants.ErrInvalidEvidenceGraph)
	}
	if metric.GetPassed() != (metric.GetMeasuredValue() >= metric.GetThresholdValue()) || !metric.GetPassed() {
		return fmt.Errorf("%w: metric grade does not reproduce the registered comparison", constants.ErrInvalidEvidenceGraph)
	}
	return nil
}
