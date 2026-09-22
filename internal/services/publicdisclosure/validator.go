// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.

package publicdisclosure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	historicalVersion = "1.0.0"
	enrichedVersion   = "1.1.0"
	maxRecordBytes    = 256 * 1024
	maxArrayEntries   = 128
	maxStringBytes    = 512
)

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidateAssignmentRecord validates a campaign assignment envelope. Records
// without a campaign message_type are handled by the general feed validator.
func ValidateAssignmentRecord(envelopeVersion string, recordBytes []byte) error {
	if len(recordBytes) == 0 {
		return schemaError("empty assignment record")
	}
	if len(recordBytes) > maxRecordBytes {
		return fmt.Errorf("public assignment record: %w", constants.ErrEvidenceArtifactTooLarge)
	}
	fields, err := decodeObject(recordBytes)
	if err != nil {
		return schemaError(err.Error())
	}
	messageType, ok := stringField(fields, "message_type")
	if !ok {
		return nil
	}
	if messageType != "PublicAssignmentLifecycleRecord" && messageType != "PublicAssignmentResultProjection" {
		return fmt.Errorf("unsupported campaign message type %q: %w", messageType, constants.ErrEvidenceSchemaMismatch)
	}
	var envelope struct {
		SchemaVersion string          `json:"schema_version"`
		MessageType   string          `json:"message_type"`
		Idempotency   string          `json:"idempotency_key"`
		Record        json.RawMessage `json:"record"`
	}
	if err := decodeStrict(recordBytes, &envelope); err != nil {
		return schemaError(err.Error())
	}
	if envelope.SchemaVersion == "" || envelope.MessageType == "" || envelope.Idempotency == "" || len(envelope.Record) == 0 {
		return schemaError("incomplete campaign envelope")
	}
	if envelopeVersion != "" && envelopeVersion != envelope.SchemaVersion {
		return schemaError("envelope version mismatch")
	}
	if envelope.MessageType == "PublicAssignmentLifecycleRecord" {
		if envelope.SchemaVersion != historicalVersion {
			return schemaError("unsupported lifecycle version")
		}
		var lifecycle evalv1.PublicAssignmentLifecycleRecord
		if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(envelope.Record, &lifecycle); err != nil {
			return schemaError(err.Error())
		}
		if lifecycle.GetAssignmentId() == "" || lifecycle.GetRunId() == "" || lifecycle.GetScenarioId() == "" {
			return schemaError("missing lifecycle identity")
		}
		return validateStrings(envelope.Record)
	}
	if envelope.SchemaVersion != historicalVersion && envelope.SchemaVersion != enrichedVersion {
		return schemaError("unsupported assignment version")
	}
	return validateResult(envelope.SchemaVersion, envelope.Record)
}

func validateResult(version string, raw json.RawMessage) error {
	fields, err := decodeObject(raw)
	if err != nil {
		return schemaError(err.Error())
	}
	extensions := make(map[string]json.RawMessage)
	for _, key := range []string{"benchmark_observations", "resource_summary"} {
		if value, ok := fields[key]; ok {
			extensions[key] = value
			delete(fields, key)
		}
	}
	projection, err := json.Marshal(fields)
	if err != nil {
		return schemaError(err.Error())
	}
	var record evalv1.PublicAssignmentResultProjection
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(projection, &record); err != nil {
		return schemaError(err.Error())
	}
	if record.GetAssignmentId() == "" || record.GetRunId() == "" || record.GetScenarioId() == "" {
		return schemaError("missing assignment identity")
	}
	if record.GetResultDigest() != "" && !hashPattern.MatchString(record.GetResultDigest()) {
		return fmt.Errorf("invalid result digest: %w", constants.ErrEvidenceArtifactMalformed)
	}
	if len(record.GetUnavailableMetricReasons()) > 16 || len(record.GetDecomposedScores()) > maxArrayEntries || len(record.GetSemanticGradeSummaries()) > maxArrayEntries || len(record.GetEvidenceBindings()) > 32 {
		return fmt.Errorf("assignment bounds: %w", constants.ErrEvidenceArtifactTooLarge)
	}
	if err := validateBindings(record.GetEvidenceBindings()); err != nil {
		return err
	}
	if version == historicalVersion && len(extensions) > 0 {
		return schemaError("enriched fields in historical envelope")
	}
	if value, ok := extensions["benchmark_observations"]; ok {
		if err := validateBenchmark(value); err != nil {
			return err
		}
	}
	if value, ok := extensions["resource_summary"]; ok {
		if err := validateResource(value); err != nil {
			return err
		}
	}
	return validateStrings(raw)
}

func validateBindings(bindings []*evalv1.PublicEvidenceBinding) error {
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		if binding == nil || !hashPattern.MatchString(binding.GetSha256()) || !bounded(binding.GetSchemaRef(), 128) || !bounded(binding.GetKind(), 128) {
			return fmt.Errorf("malformed evidence binding: %w", constants.ErrEvidenceArtifactMalformed)
		}
		key := binding.GetSha256() + "\x00" + binding.GetSchemaRef() + "\x00" + binding.GetKind()
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate evidence binding: %w", constants.ErrEvidenceArtifactMalformed)
		}
		seen[key] = struct{}{}
	}
	return nil
}

type metric struct {
	Value             *float64 `json:"value,omitempty"`
	UnavailableReason string   `json:"unavailable_reason,omitempty"`
}

type benchmark struct {
	GradeSummaries     []json.RawMessage          `json:"grade_summaries,omitempty"`
	ToolScorecard      map[string]json.RawMessage `json:"tool_scorecard,omitempty"`
	Timing             map[string]json.RawMessage `json:"timing,omitempty"`
	GPU                map[string]json.RawMessage `json:"gpu,omitempty"`
	UnavailableReasons []string                   `json:"unavailable_reasons"`
}

func validateBenchmark(raw json.RawMessage) error {
	var value benchmark
	if err := decodeStrict(raw, &value); err != nil {
		return schemaError(err.Error())
	}
	if len(value.GradeSummaries) > 64 || len(value.ToolScorecard) > 10 || len(value.UnavailableReasons) > 16 {
		return fmt.Errorf("benchmark bounds: %w", constants.ErrEvidenceArtifactTooLarge)
	}
	for _, grade := range value.GradeSummaries {
		fields, err := decodeObject(grade)
		if err != nil {
			return schemaError(err.Error())
		}
		if _, ok := fields["detail"]; ok {
			return fmt.Errorf("private grade detail: %w", constants.ErrPublicFeedRestrictedField)
		}
		if err := allowed(fields, "criterion_id", "status", "method", "judge_variant_id", "explanation_code"); err != nil {
			return err
		}
	}
	for key, value := range value.ToolScorecard {
		if !toolDimension(key) {
			return schemaError("unknown tool score dimension")
		}
		if err := validateMetric(value); err != nil {
			return err
		}
	}
	for _, value := range value.Timing {
		if err := validateMetric(value); err != nil {
			return err
		}
	}
	for _, value := range value.GPU {
		if err := validateMetric(value); err != nil {
			return err
		}
	}
	return nil
}

func validateResource(raw json.RawMessage) error {
	fields, err := decodeObject(raw)
	if err != nil {
		return schemaError(err.Error())
	}
	if err := allowed(fields, "latency_ms", "input_tokens", "output_tokens", "thinking_tokens", "cache_tokens", "retries"); err != nil {
		return err
	}
	for _, value := range fields {
		if err := validateMetric(value); err != nil {
			return err
		}
	}
	return nil
}

func validateMetric(raw json.RawMessage) error {
	var value metric
	if err := decodeStrict(raw, &value); err != nil {
		return schemaError(err.Error())
	}
	if value.Value == nil && value.UnavailableReason == "" || value.Value != nil && value.UnavailableReason != "" {
		return schemaError("metric must be observed or unavailable")
	}
	if value.Value != nil && (math.IsNaN(*value.Value) || math.IsInf(*value.Value, 0) || *value.Value < 0) {
		return fmt.Errorf("invalid metric: %w", constants.ErrEvidenceArtifactMalformed)
	}
	if value.UnavailableReason != "" && !unavailableReason(value.UnavailableReason) {
		return schemaError("unknown unavailable reason")
	}
	return nil
}

func validateStrings(raw []byte) error {
	var value json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return walkStrings(value)
}

func walkStrings(value json.RawMessage) error {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return err
		}
		if len(text) > maxStringBytes || strings.Contains(text, "BEGIN PRIVATE KEY") || strings.Contains(text, "spiffe://") || strings.Contains(text, "../") || strings.Contains(text, "/.g8e/") {
			return fmt.Errorf("restricted assignment text: %w", constants.ErrPublicFeedRestrictedField)
		}
		return nil
	}
	if trimmed[0] == '[' {
		var values []json.RawMessage
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return err
		}
		if len(values) > maxArrayEntries {
			return fmt.Errorf("assignment array bound: %w", constants.ErrEvidenceArtifactTooLarge)
		}
		for _, child := range values {
			if err := walkStrings(child); err != nil {
				return err
			}
		}
		return nil
	}
	if trimmed[0] == '{' {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &fields); err != nil {
			return err
		}
		for _, child := range fields {
			if err := walkStrings(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func decodeObject(data []byte) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := decodeStrict(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("object required")
	}
	return fields, nil
}

func stringField(fields map[string]json.RawMessage, key string) (string, bool) {
	value, ok := fields[key]
	if !ok {
		return "", false
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		return "", true
	}
	return result, true
}

func allowed(fields map[string]json.RawMessage, keys ...string) error {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	for key := range fields {
		if _, ok := set[key]; !ok {
			return fmt.Errorf("unknown assignment field %q: %w", key, constants.ErrEvidenceSchemaMismatch)
		}
	}
	return nil
}

func bounded(value string, max int) bool {
	return value != "" && len(value) <= max && !strings.ContainsAny(value, "\r\n")
}

func toolDimension(value string) bool {
	switch value {
	case "tool_recognition", "tool_selection", "argument_schema", "argument_semantics", "permission_compliance", "result_interpretation", "follow_up_decision", "unnecessary_tool_calls", "looping", "recovery":
		return true
	default:
		return false
	}
}

func unavailableReason(value string) bool {
	switch value {
	case "historical_not_captured", "source_not_captured", "source_unavailable", "scenario_not_applicable", "incomplete_contributor_evidence", "no_scored_calls":
		return true
	default:
		return false
	}
}

// ValidatePublicFeedRecord validates the payload schema associated with one
// public-feed transport record type. The transport enum alone is not enough:
// browsers dispatch by the payload's closed kind vocabulary as well.
func ValidatePublicFeedRecord(recordType models.PublicFeedRecordType, recordBytes []byte) error {
	switch recordType {
	case models.PublicFeedRecordTypeProjection:
		fields, err := decodeObject(recordBytes)
		if err != nil {
			return publicFeedSchemaError(err.Error())
		}
		if _, hasMessageType := fields["message_type"]; hasMessageType {
			if err := ValidateAssignmentRecord("", recordBytes); err != nil {
				return fmt.Errorf("public feed projection: %w", err)
			}
			return nil
		}
		return validateExplorerViewRecord(recordBytes, false)
	case models.PublicFeedRecordTypeEvent:
		return validateExplorerViewRecord(recordBytes, true)
	case models.PublicFeedRecordTypeProofManifest:
		return validatePublicFeedManifest(recordBytes)
	case models.PublicFeedRecordTypeKeyRevocation:
		return validatePublicFeedKeyRevocation(recordBytes)
	default:
		return fmt.Errorf("%s: %q", constants.ErrPublicFeedRecordTypeInvalid, recordType)
	}
}

func validateExplorerViewRecord(recordBytes []byte, event bool) error {
	fields, err := decodeObject(recordBytes)
	if err != nil {
		return publicFeedSchemaError(err.Error())
	}
	if err := validateExplorerEnvelope(fields, event); err != nil {
		return err
	}
	if event {
		if err := allowed(fields, "schema_version", "kind", "dataset_id", "quality_state", "observed_at", "source_revision_label", "event_id", "run_id", "assignment_id", "task_id", "variant_id", "role", "lifecycle_status", "completed", "total", "stage_label", "metric_delta", "feed_sequence"); err != nil {
			return publicFeedSchemaError(err.Error())
		}
		eventID, eventIDOK := stringField(fields, "event_id")
		runID, runIDOK := stringField(fields, "run_id")
		lifecycleStatus, lifecycleOK := stringField(fields, "lifecycle_status")
		if !eventIDOK || eventID == "" || !runIDOK || runID == "" || !lifecycleOK || !publicFeedLifecycleStatus(lifecycleStatus) {
			return publicFeedSchemaError("missing or invalid event identity")
		}
		completed, err := integerField(fields, "completed")
		if err != nil {
			return publicFeedSchemaError(err.Error())
		}
		total, err := integerField(fields, "total")
		if err != nil {
			return publicFeedSchemaError(err.Error())
		}
		if completed < 0 || total < 0 || completed > total {
			return publicFeedSchemaError("event progress is invalid")
		}
		if _, ok := fields["role"]; ok {
			role, roleOK := stringField(fields, "role")
			if !roleOK || !publicFeedRole(role) {
				return publicFeedSchemaError("invalid event role")
			}
		}
		if value, ok := fields["feed_sequence"]; ok {
			var sequence int64
			if err := json.Unmarshal(value, &sequence); err != nil || sequence < 0 {
				return publicFeedSchemaError("invalid event feed_sequence")
			}
		}
		return nil
	}
	if _, ok := fields["kind"]; !ok {
		return publicFeedSchemaError("missing projection kind")
	}
	return nil
}

func validateExplorerEnvelope(fields map[string]json.RawMessage, event bool) error {
	schemaVersion, ok := stringField(fields, "schema_version")
	if !ok || !publicFeedViewSchemaVersion(schemaVersion) {
		return publicFeedSchemaError("unsupported schema_version")
	}
	kind, ok := stringField(fields, "kind")
	if !ok || (event && !publicFeedEventKind(kind)) || (!event && !publicFeedSnapshotKind(kind)) {
		return publicFeedSchemaError("unsupported kind")
	}
	datasetID, datasetPresent := stringField(fields, "dataset_id")
	qualityState, qualityPresent := stringField(fields, "quality_state")
	observedAt, observedPresent := stringField(fields, "observed_at")
	if !datasetPresent || datasetID == "" || !qualityPresent || !publicFeedQualityState(qualityState) || !observedPresent || observedAt == "" {
		return publicFeedSchemaError("missing or invalid envelope field")
	}
	if _, err := time.Parse(time.RFC3339Nano, observedAt); err != nil {
		return publicFeedSchemaError("observed_at must be RFC3339")
	}
	return nil
}

func validatePublicFeedManifest(recordBytes []byte) error {
	var manifest models.PublicProofManifest
	if err := decodeStrict(recordBytes, &manifest); err != nil {
		return publicFeedSchemaError(err.Error())
	}
	if manifest.SchemaVersion == "" || manifest.ProofRootSHA256 == "" || manifest.CampaignID == "" || manifest.CampaignRevision == "" || manifest.VerifiedIndexGenerationHash == "" || manifest.ArtifactCount <= 0 || len(manifest.Artifacts) != manifest.ArtifactCount || manifest.VerifierInstructions == "" || manifest.GeneratedAt.IsZero() || manifest.SigningKeyID == "" || manifest.Signature == "" {
		return publicFeedSchemaError("incomplete proof manifest")
	}
	return nil
}

func validatePublicFeedKeyRevocation(recordBytes []byte) error {
	var record models.PublicKeyRevocationRecord
	if err := decodeStrict(recordBytes, &record); err != nil {
		return publicFeedSchemaError(err.Error())
	}
	if record.SourceID == "" || record.RevokedKeyID == "" || record.RevokedAt.IsZero() || record.NewKeyID == "" || record.RevocationSignature == "" {
		return publicFeedSchemaError("incomplete key revocation")
	}
	return nil
}

func integerField(fields map[string]json.RawMessage, field string) (int64, error) {
	value, ok := fields[field]
	if !ok {
		return 0, fmt.Errorf("missing event field %s", field)
	}
	var result int64
	if err := json.Unmarshal(value, &result); err != nil {
		return 0, fmt.Errorf("event field %s must be an integer", field)
	}
	return result, nil
}

func publicFeedViewSchemaVersion(value string) bool {
	switch value {
	case "1.0.0", "1.1.0", "1.2.0", "1.3.0", "1.4.0", "1.5.0":
		return true
	default:
		return false
	}
}

func publicFeedSnapshotKind(value string) bool {
	switch value {
	case "catalog_snapshot", "model_summary", "suite_summary", "evaluation_summary", "assignment_result", "methodology_snapshot":
		return true
	default:
		return false
	}
}

func publicFeedEventKind(value string) bool {
	switch value {
	case "evaluation_queued", "evaluation_started", "assignment_started", "stage_updated", "assignment_completed", "assignment_failed", "metric_updated", "evaluation_completed", "evaluation_failed", "evaluation_stopped":
		return true
	default:
		return false
	}
}

func publicFeedQualityState(value string) bool {
	switch value {
	case "verified_public", "exploratory_verified", "exploratory_partial", "legacy_unverified", "live_in_progress", "terminal_failed", "dead_evidence", "not_evaluated", "unavailable":
		return true
	default:
		return false
	}
}

func publicFeedLifecycleStatus(value string) bool {
	switch value {
	case "queued", "running", "completed", "failed", "stopped":
		return true
	default:
		return false
	}
}

func publicFeedRole(value string) bool {
	switch value {
	case "primary", "assistant", "lite":
		return true
	default:
		return false
	}
}

func publicFeedSchemaError(detail string) error {
	return fmt.Errorf("public feed record schema: %s: %w", detail, constants.ErrPublicFeedRecordSchemaInvalid)
}

func schemaError(detail string) error {
	return fmt.Errorf("public assignment schema: %s: %w", detail, constants.ErrEvidenceSchemaMismatch)
}
