// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	stdfs "io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const campaignExportSchemaVersion = "1.1.0"

type campaignRunSummaryExport struct {
	SchemaVersion            string   `json:"schema_version"`
	RecordType               string   `json:"record_type"`
	RunID                    string   `json:"run_id"`
	CampaignID               string   `json:"campaign_id"`
	Lane                     string   `json:"lane"`
	CatalogID                string   `json:"catalog_id"`
	CatalogVersion           string   `json:"catalog_version"`
	CatalogDigest            string   `json:"catalog_digest"`
	CampaignDigest           string   `json:"campaign_digest,omitempty"`
	ModelRegistryDigest      string   `json:"model_registry_digest"`
	ExportedAt               string   `json:"exported_at"`
	ScheduledAssignments     uint32   `json:"scheduled_assignments"`
	TerminalAssignments      uint32   `json:"terminal_assignments"`
	PassedAssignments        uint32   `json:"passed_assignments"`
	FailedAssignments        uint32   `json:"failed_assignments"`
	ModelCount               uint32   `json:"model_count"`
	PopulationComplete       bool     `json:"population_complete"`
	ExpectedCells            uint64   `json:"expected_cells"`
	PopulationFailureReasons []string `json:"population_failure_reasons"`
}

type campaignExportSchemaTable struct {
	Name       string                   `json:"name"`
	PrimaryKey campaignExportPrimaryKey `json:"primary_key"`
}

type campaignExportPrimaryKey []string

func (key campaignExportPrimaryKey) MarshalJSON() ([]byte, error) {
	if len(key) == 1 {
		return json.Marshal(key[0])
	}
	return json.Marshal([]string(key))
}

type campaignExportSchemaFile struct {
	Format      string                      `json:"format"`
	Description string                      `json:"description"`
	Tables      []campaignExportSchemaTable `json:"tables,omitempty"`
}

type campaignExportSchemaDocument struct {
	SchemaVersion string                              `json:"schema_version"`
	Description   string                              `json:"description"`
	Files         map[string]campaignExportSchemaFile `json:"files"`
}

type campaignModelSummaryIdentity struct {
	VariantID string `json:"variant_id"`
	Role      string `json:"role"`
}

type campaignModelSummaryFallback struct {
	Role      string `json:"role"`
	VariantID string `json:"variant_id"`
}

// CampaignExportFile describes one artifact written by ExportRun.
type CampaignExportFile struct {
	Name        string `json:"name"`
	Format      string `json:"format"`
	Path        string `json:"path"`
	RecordCount uint32 `json:"record_count"`
}

// CampaignExportReport summarizes one disclosure-safe export bundle.
type CampaignExportReport struct {
	RunID               string               `json:"run_id"`
	CampaignID          string               `json:"campaign_id"`
	OutputDir           string               `json:"output_dir"`
	ExportedAt          time.Time            `json:"exported_at"`
	AssignmentCount     uint32               `json:"assignment_count"`
	TerminalResultCount uint32               `json:"terminal_result_count"`
	Files               []CampaignExportFile `json:"files"`
}

// CampaignExportAssignmentRecord is one disclosure-safe assignment export row.
type CampaignExportAssignmentRecord struct {
	SchemaVersion         string                                   `json:"schema_version"`
	RecordType            string                                   `json:"record_type"`
	Projection            *evalv1.PublicAssignmentResultProjection `json:"projection"`
	BenchmarkObservations *PublicBenchmarkObservations             `json:"benchmark_observations,omitempty"`
	ResourceSummary       *PublicResourceSummary                   `json:"resource_summary,omitempty"`
}

type exportResourceMetric struct {
	Value             *float64 `json:"value,omitempty"`
	UnavailableReason string   `json:"unavailable_reason,omitempty"`
}

type exportResourceSummary struct {
	LatencyMS      *exportResourceMetric `json:"latency_ms,omitempty"`
	InputTokens    *exportResourceMetric `json:"input_tokens,omitempty"`
	OutputTokens   *exportResourceMetric `json:"output_tokens,omitempty"`
	ThinkingTokens *exportResourceMetric `json:"thinking_tokens,omitempty"`
	CacheTokens    *exportResourceMetric `json:"cache_tokens,omitempty"`
	Retries        *exportResourceMetric `json:"retries,omitempty"`
}

// MarshalJSON keeps the export wrapper named while encoding its protobuf
// projection with the same canonical protojson used by publication.
func (record CampaignExportAssignmentRecord) MarshalJSON() ([]byte, error) {
	projection, err := evalv1.MarshalCanonical(record.Projection)
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal assignment export projection: %w", err)
	}
	type exportRecord struct {
		SchemaVersion         string                       `json:"schema_version"`
		RecordType            string                       `json:"record_type"`
		Projection            json.RawMessage              `json:"projection"`
		BenchmarkObservations *PublicBenchmarkObservations `json:"benchmark_observations,omitempty"`
		ResourceSummary       json.RawMessage              `json:"resource_summary,omitempty"`
	}
	resource, err := marshalExportResourceSummary(record.ResourceSummary)
	if err != nil {
		return nil, err
	}
	return json.Marshal(exportRecord{
		SchemaVersion:         record.SchemaVersion,
		RecordType:            record.RecordType,
		Projection:            projection,
		BenchmarkObservations: record.BenchmarkObservations,
		ResourceSummary:       resource,
	})
}

func (record *CampaignExportAssignmentRecord) UnmarshalJSON(body []byte) error {
	var raw struct {
		SchemaVersion         string                       `json:"schema_version"`
		RecordType            string                       `json:"record_type"`
		Projection            json.RawMessage              `json:"projection"`
		BenchmarkObservations *PublicBenchmarkObservations `json:"benchmark_observations,omitempty"`
		ResourceSummary       json.RawMessage              `json:"resource_summary,omitempty"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	projection := &evalv1.PublicAssignmentResultProjection{}
	if err := evalv1.UnmarshalCanonical(raw.Projection, projection); err != nil {
		return fmt.Errorf("evaluation: unmarshal assignment export projection: %w", err)
	}
	resource, err := unmarshalExportResourceSummary(raw.ResourceSummary)
	if err != nil {
		return err
	}
	record.SchemaVersion = raw.SchemaVersion
	record.RecordType = raw.RecordType
	record.Projection = projection
	record.BenchmarkObservations = raw.BenchmarkObservations
	record.ResourceSummary = resource
	return nil
}

func marshalExportResourceSummary(summary *PublicResourceSummary) (json.RawMessage, error) {
	if summary == nil {
		return nil, nil
	}
	encode := func(metric PublicResourceMetric) *exportResourceMetric {
		if metric.Value == nil && metric.UnavailableReason == evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED {
			return nil
		}
		return &exportResourceMetric{Value: metric.Value, UnavailableReason: publicUnavailableReasonString(metric.UnavailableReason)}
	}
	body, err := json.Marshal(exportResourceSummary{
		LatencyMS: encode(summary.LatencyMS), InputTokens: encode(summary.InputTokens), OutputTokens: encode(summary.OutputTokens),
		ThinkingTokens: encode(summary.ThinkingTokens), CacheTokens: encode(summary.CacheTokens), Retries: encode(summary.Retries),
	})
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal assignment export resources: %w", err)
	}
	return body, nil
}

func unmarshalExportResourceSummary(body json.RawMessage) (*PublicResourceSummary, error) {
	if len(body) == 0 || string(body) == "null" {
		return nil, nil
	}
	var raw exportResourceSummary
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("evaluation: unmarshal assignment export resources: %w", err)
	}
	decode := func(metric *exportResourceMetric) (PublicResourceMetric, error) {
		if metric == nil {
			return PublicResourceMetric{}, nil
		}
		if metric.UnavailableReason == "" {
			return PublicResourceMetric{Value: metric.Value}, nil
		}
		reason, ok := publicUnavailableReason(metric.UnavailableReason)
		if !ok {
			return PublicResourceMetric{}, fmt.Errorf("evaluation: unmarshal assignment export resources: unknown unavailable reason %q", metric.UnavailableReason)
		}
		return PublicResourceMetric{UnavailableReason: reason}, nil
	}
	latency, err := decode(raw.LatencyMS)
	if err != nil {
		return nil, err
	}
	input, err := decode(raw.InputTokens)
	if err != nil {
		return nil, err
	}
	output, err := decode(raw.OutputTokens)
	if err != nil {
		return nil, err
	}
	thinking, err := decode(raw.ThinkingTokens)
	if err != nil {
		return nil, err
	}
	cache, err := decode(raw.CacheTokens)
	if err != nil {
		return nil, err
	}
	retries, err := decode(raw.Retries)
	if err != nil {
		return nil, err
	}
	return &PublicResourceSummary{LatencyMS: latency, InputTokens: input, OutputTokens: output, ThinkingTokens: thinking, CacheTokens: cache, Retries: retries}, nil
}

func publicUnavailableReasonString(reason evalv1.PublicUnavailableReason) string {
	if reason == evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(reason.String(), "PUBLIC_UNAVAILABLE_REASON_"))
}

func publicUnavailableReason(value string) (evalv1.PublicUnavailableReason, bool) {
	for _, reason := range []evalv1.PublicUnavailableReason{
		evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_HISTORICAL_NOT_CAPTURED,
		evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_NOT_CAPTURED,
		evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE,
		evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SCENARIO_NOT_APPLICABLE,
		evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE,
		evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS,
	} {
		if publicUnavailableReasonString(reason) == value {
			return reason, true
		}
	}
	return evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED, false
}

// CampaignExporter materializes disclosure-safe JSONL, CSV, and SQLite exports
// from canonical campaign records using public projections only.
type CampaignExporter struct {
	now func() time.Time
}

func NewCampaignExporter(now func() time.Time) *CampaignExporter {
	if now == nil {
		now = time.Now
	}
	return &CampaignExporter{now: now}
}

// ExportRun writes one disclosure-safe export bundle beneath outputDir.
func (e *CampaignExporter) ExportRun(
	ctx context.Context,
	store *Store,
	fileSvc fs.RuntimeFileService,
	runID string,
	outputDir string,
) (*CampaignExportReport, error) {
	if e == nil || store == nil || fileSvc == nil || runID == "" {
		return nil, fmt.Errorf("evaluation: export campaign run: %w", constants.ErrMissingRequiredField)
	}
	if outputDir == "" {
		outputDir = filepath.Join(evaluationRunDir(runID), "export")
	}
	run, err := store.LoadRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	campaignID := run.GetCampaignBinding().GetCampaignId()
	spec, err := store.LoadCampaignSpec(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	catalog, err := store.LoadScenarioCatalog(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	assignments, err := store.ListAssignments(ctx, runID)
	if err != nil {
		return nil, err
	}
	observationReader, err := NewCampaignProviderObservationReader(fileSvc)
	if err != nil {
		return nil, err
	}

	exportedAt := e.now().UTC()
	assignmentRecords := make([]CampaignExportAssignmentRecord, 0)
	results := make(map[string]*evalv1.EvaluationAssignmentResult, len(assignments))
	for _, assignment := range assignments {
		exists, err := store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		result, err := store.LoadAssignmentResult(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return nil, err
		}
		results[assignment.GetAssignmentId()] = result
	}

	verificationReport, err := store.LoadCampaignVerification(ctx, runID)
	if err != nil && !errors.Is(err, constants.ErrNotFound) && !errors.Is(err, stdfs.ErrNotExist) {
		return nil, fmt.Errorf("evaluation: load campaign verification for export: %w", err)
	}
	applicability, err := BuildRunVerificationApplicability(run, spec, catalog, assignments, results, verificationReport)
	if err != nil && verificationReport != nil {
		return nil, fmt.Errorf("evaluation: resolve export verification applicability: %w", err)
	}
	verified := verificationReport != nil && verificationReport.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS && applicability != nil && applicability.Applicable
	var boundReport *evalv1.EvaluationVerificationReport
	if verificationReport != nil && boundVerificationMetadataComplete(verificationReport) {
		if applicability == nil || !applicability.Applicable {
			return nil, fmt.Errorf("evaluation: resolve export verification applicability: %w", constants.ErrEvidenceScopeMismatch)
		}
		boundReport = verificationReport
	}
	for _, assignment := range assignments {
		result := results[assignment.GetAssignmentId()]
		if result == nil {
			continue
		}
		category, err := ScenarioCategoryForAssignment(catalog, assignment)
		if err != nil {
			return nil, err
		}
		record, err := e.buildAssignmentExportRecord(ctx, store, run, catalog, assignment, result, category, observationReader, verified, verificationReport)
		if err != nil {
			return nil, err
		}
		assignmentRecords = append(assignmentRecords, record)
	}

	aggregateState, err := CollectRunAggregateState(assignments, results)
	if err != nil {
		return nil, err
	}
	aggregateRecords, err := BuildRunAggregateViewRecords(run, aggregateState, boundReport, exportedAt)
	if err != nil {
		return nil, err
	}
	if verified {
		verifiedRecords, projectionErr := BuildVerifiedModelSummaryViewRecords(VerifiedModelSummaryProjectionInput{
			Run: run, Spec: spec, Catalog: catalog, Assignments: assignments, Results: results,
			Report: verificationReport, Applicability: applicability,
		})
		if projectionErr != nil {
			return nil, fmt.Errorf("evaluation: project verified model summaries for export: %w", projectionErr)
		}
		aggregateRecords = replaceModelSummaryRecords(aggregateRecords, verifiedRecords)
	}
	var evaluationSummaryBody []byte
	modelSummaryLines := make([][]byte, 0, len(aggregateRecords))
	for _, record := range aggregateRecords {
		var header struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(record.Body, &header); err != nil {
			return nil, fmt.Errorf("evaluation: classify aggregate export record: %w", err)
		}
		switch header.Kind {
		case "evaluation_summary":
			if evaluationSummaryBody != nil {
				return nil, fmt.Errorf("evaluation: classify aggregate export record: duplicate evaluation summary: %w", constants.ErrEvidenceArtifactMalformed)
			}
			evaluationSummaryBody = record.Body
		case "model_summary":
			modelSummaryLines = append(modelSummaryLines, record.Body)
		}
	}
	if evaluationSummaryBody == nil {
		return nil, fmt.Errorf("evaluation: classify aggregate export record: missing evaluation summary: %w", constants.ErrEvidenceArtifactMalformed)
	}
	populationReport, err := NewCampaignPopulationAccountant(e.now).AccountRun(ctx, store, runID, catalog)
	if err != nil {
		return nil, err
	}

	if err := fileSvc.MkdirAll(ctx, outputDir, constants.PermDirStandard); err != nil {
		return nil, fmt.Errorf("evaluation: export campaign run: create output directory: %w", err)
	}

	report := &CampaignExportReport{
		RunID:               runID,
		CampaignID:          campaignID,
		OutputDir:           outputDir,
		ExportedAt:          exportedAt,
		AssignmentCount:     uint32(len(assignments)),
		TerminalResultCount: uint32(len(assignmentRecords)),
	}

	schemaBody, err := json.MarshalIndent(buildCampaignExportSchemaDocument(), "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeExportFile(ctx, fileSvc, outputDir, constants.EvaluationExportSchemaFilename, schemaBody, report); err != nil {
		return nil, err
	}

	runSummaryBody, err := json.MarshalIndent(buildCampaignRunSummaryExport(
		run,
		spec,
		catalog,
		exportedAt,
		populationReport,
		aggregateState,
	), "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeExportFile(ctx, fileSvc, outputDir, constants.EvaluationRunSummaryFilename, runSummaryBody, report); err != nil {
		return nil, err
	}
	if err := writeExportFile(ctx, fileSvc, outputDir, constants.CampaignExportEvaluationSummaryFilename, evaluationSummaryBody, report); err != nil {
		return nil, err
	}

	assignmentsJSONL, err := marshalJSONL(assignmentRecords)
	if err != nil {
		return nil, err
	}
	if err := writeExportFile(ctx, fileSvc, outputDir, constants.EvaluationAssignmentsJSONLFilename, assignmentsJSONL, report); err != nil {
		return nil, err
	}

	modelSummariesJSONL := bytes.Join(modelSummaryLines, []byte("\n"))
	if len(modelSummaryLines) > 0 {
		modelSummariesJSONL = append(modelSummariesJSONL, '\n')
	}
	if err := writeExportFile(ctx, fileSvc, outputDir, constants.EvaluationModelSummariesJSONLFilename, modelSummariesJSONL, report); err != nil {
		return nil, err
	}

	assignmentsCSV, err := buildAssignmentResultsCSV(assignmentRecords)
	if err != nil {
		return nil, err
	}
	if err := writeExportFile(ctx, fileSvc, outputDir, constants.EvaluationAssignmentsCSVFilename, assignmentsCSV, report); err != nil {
		return nil, err
	}

	modelSummariesCSV, err := buildModelSummariesCSV(aggregateState)
	if err != nil {
		return nil, err
	}
	if err := writeExportFile(ctx, fileSvc, outputDir, constants.EvaluationModelSummariesCSVFilename, modelSummariesCSV, report); err != nil {
		return nil, err
	}

	sqlitePath := filepath.Join(outputDir, constants.EvaluationCampaignExportSQLiteFilename)
	if err := fileSvc.Remove(ctx, sqlitePath); err != nil {
		return nil, fmt.Errorf("evaluation: export campaign run: remove existing sqlite export: %w", err)
	}
	if err := e.writeSQLiteExport(
		ctx,
		fileSvc.Resolve(sqlitePath),
		run,
		spec,
		catalog,
		exportedAt,
		populationReport,
		assignmentRecords,
		aggregateState,
		modelSummaryLines,
	); err != nil {
		return nil, err
	}
	report.Files = append(report.Files, CampaignExportFile{
		Name:        "campaign_export.sqlite",
		Format:      "sqlite",
		Path:        sqlitePath,
		RecordCount: uint32(len(assignmentRecords)) + uint32(len(modelSummaryLines)),
	})
	return report, nil
}

func (e *CampaignExporter) buildAssignmentExportRecord(
	ctx context.Context,
	store *Store,
	run *evalv1.EvaluationRun,
	catalog *evalv1.EvaluationScenarioCatalog,
	assignment *evalv1.EvaluationAssignment,
	result *evalv1.EvaluationAssignmentResult,
	category evalv1.EvaluationScenarioCategory,
	observationReader *CampaignProviderObservationReader,
	verified bool,
	report *evalv1.EvaluationVerificationReport,
) (CampaignExportAssignmentRecord, error) {
	verificationStatus := "unverified"
	if verified {
		verificationStatus = "verified"
	}
	record := CampaignExportAssignmentRecord{SchemaVersion: campaignExportSchemaVersion, RecordType: publicMessageTypeAssignmentResult}
	generatedCatalog, artifacts, catalogErr := LoadScenarioCatalog()
	if catalogErr == nil && generatedCatalog.GetCatalogDigest() == catalog.GetCatalogDigest() {
		scenario, resolveErr := ResolvePublicScenarioContext(ctx, store, run, catalog, assignment, artifacts)
		if resolveErr == nil {
			composed, composeErr := BuildPublicAssignmentProjection(ctx, PublicAssignmentBuildInput{
				Assignment: assignment, Result: result, ScenarioContext: scenario,
				ObservationReader: observationReader, VerificationStatus: verificationStatus,
				VerificationMetadata: exportVerificationMetadata(report, verified),
			})
			if composeErr != nil {
				return CampaignExportAssignmentRecord{}, composeErr
			}
			record.Projection = composed.Projection
			record.BenchmarkObservations = composed.Extensions.BenchmarkObservations
			record.ResourceSummary = composed.Extensions.ResourceSummary
			return record, nil
		}
		if !errors.Is(resolveErr, constants.ErrEvidenceArtifactMalformed) {
			return CampaignExportAssignmentRecord{}, resolveErr
		}
	}

	benchmark, err := observationReader.BuildPublicBenchmarkObservations(ctx, result)
	if err != nil {
		return CampaignExportAssignmentRecord{}, err
	}
	projection, err := BuildAssignmentResultProjection(assignment, result, category, DerivePublicSummaryStatus(result), verificationStatus)
	if err != nil {
		return CampaignExportAssignmentRecord{}, err
	}
	projection.VerificationMetadata = exportVerificationMetadata(report, verified)
	record.Projection = projection
	record.BenchmarkObservations = benchmark
	record.ResourceSummary, err = BuildPublicResourceSummary(result)
	if err != nil {
		return CampaignExportAssignmentRecord{}, err
	}
	return record, nil
}

func exportVerificationMetadata(report *evalv1.EvaluationVerificationReport, verified bool) *evalv1.PublicVerificationMetadata {
	if report == nil {
		return nil
	}
	metadata := &evalv1.PublicVerificationMetadata{
		Provenance:              evalv1.PublicVerificationProvenance_PUBLIC_VERIFICATION_PROVENANCE_LEGACY_UNBOUND,
		VerifierState:           evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		VerifierReleaseVersion:  report.GetVerifierReleaseVersion(),
		VerifierContractVersion: report.GetVerifierContractVersion(),
		PopulationDigest:        report.GetVerifiedPopulationDigest(),
	}
	if report.GetReportDigestRef() != nil {
		metadata.ReportDigest = report.GetReportDigestRef().GetSha256()
	}
	if verified {
		metadata.Provenance = evalv1.PublicVerificationProvenance_PUBLIC_VERIFICATION_PROVENANCE_BOUND
		metadata.VerifierState = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
	}
	return metadata
}

func replaceModelSummaryRecords(base, verified []CampaignViewRecord) []CampaignViewRecord {
	result := make([]CampaignViewRecord, 0, len(base)+len(verified))
	for _, record := range base {
		var envelope struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(record.Body, &envelope); err == nil && envelope.Kind == "model_summary" {
			continue
		}
		result = append(result, record)
	}
	result = append(result, verified...)
	return result
}

func writeExportFile(ctx context.Context, fileSvc fs.RuntimeFileService, outputDir, name string, body []byte, report *CampaignExportReport) error {
	path := filepath.Join(outputDir, name)
	if err := fileSvc.WriteFile(ctx, path, body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("evaluation: export campaign run: write %s: %w", name, err)
	}
	recordCount := countExportRecords(name, body)
	report.Files = append(report.Files, CampaignExportFile{
		Name:        name,
		Format:      exportFormatForName(name),
		Path:        path,
		RecordCount: recordCount,
	})
	return nil
}

func countExportRecords(name string, body []byte) uint32 {
	switch {
	case strings.HasSuffix(name, ".jsonl"):
		if len(body) == 0 {
			return 0
		}
		return uint32(bytes.Count(body, []byte("\n")))
	case strings.HasSuffix(name, ".csv"):
		lines := bytes.Count(body, []byte("\n"))
		if lines == 0 {
			return 0
		}
		return uint32(lines - 1)
	default:
		return 1
	}
}

func exportFormatForName(name string) string {
	switch {
	case strings.HasSuffix(name, ".jsonl"):
		return "jsonl"
	case strings.HasSuffix(name, ".csv"):
		return "csv"
	case strings.HasSuffix(name, ".sqlite"):
		return "sqlite"
	default:
		return "json"
	}
}

func marshalJSONL(records []CampaignExportAssignmentRecord) ([]byte, error) {
	if len(records) == 0 {
		return nil, nil
	}
	lines := make([][]byte, 0, len(records))
	for _, record := range records {
		body, err := json.Marshal(record)
		if err != nil {
			return nil, fmt.Errorf("evaluation: export campaign run: marshal assignment jsonl: %w", err)
		}
		lines = append(lines, body)
	}
	body := bytes.Join(lines, []byte("\n"))
	return append(body, '\n'), nil
}

func buildCampaignRunSummaryExport(
	run *evalv1.EvaluationRun,
	spec *evalv1.EvaluationCampaignSpec,
	catalog *evalv1.EvaluationScenarioCatalog,
	exportedAt time.Time,
	population *CampaignPopulationReport,
	aggregate *runAggregateState,
) campaignRunSummaryExport {
	summary := campaignRunSummaryExport{
		SchemaVersion:            campaignExportSchemaVersion,
		RecordType:               "run_summary",
		RunID:                    run.GetRunId(),
		CampaignID:               run.GetCampaignBinding().GetCampaignId(),
		Lane:                     run.GetLane().String(),
		CatalogID:                catalog.GetCatalogRef().GetId(),
		CatalogVersion:           catalog.GetCatalogRef().GetVersion(),
		CatalogDigest:            catalog.GetCatalogDigest(),
		ModelRegistryDigest:      spec.GetModelRegistryDigest(),
		ExportedAt:               exportedAt.Format(time.RFC3339Nano),
		ScheduledAssignments:     aggregate.Scheduled,
		TerminalAssignments:      aggregate.Terminal,
		PassedAssignments:        aggregate.Passed,
		FailedAssignments:        aggregate.Failed,
		ModelCount:               aggregate.ModelCount,
		PopulationComplete:       population.Complete,
		ExpectedCells:            population.ExpectedCells,
		PopulationFailureReasons: population.FailureReasons,
	}
	if binding := run.GetCampaignBinding(); binding != nil {
		summary.CampaignDigest = binding.GetCampaignDigest()
	}
	return summary
}

func buildCampaignExportSchemaDocument() campaignExportSchemaDocument {
	file := func(format, description string) campaignExportSchemaFile {
		return campaignExportSchemaFile{Format: format, Description: description}
	}
	return campaignExportSchemaDocument{
		SchemaVersion: campaignExportSchemaVersion,
		Description:   "Disclosure-safe campaign export bundle derived from public projections.",
		Files: map[string]campaignExportSchemaFile{
			constants.EvaluationExportSchemaFilename:          file("json", "This schema document."),
			constants.EvaluationRunSummaryFilename:            file("json", "Run-level metadata, catalog/registry digests, and aggregate counters."),
			constants.CampaignExportEvaluationSummaryFilename: file("json", "Explorer evaluation_summary projection with typed headline metrics and bound verification metadata."),
			constants.EvaluationAssignmentsJSONLFilename:      file("jsonl", "One named export wrapper per terminal assignment containing canonical protobuf JSON under projection plus approved benchmark_observations and resource_summary extensions."),
			constants.EvaluationModelSummariesJSONLFilename:   file("jsonl", "One explorer model_summary snapshot per variant-role bucket."),
			constants.EvaluationAssignmentsCSVFilename:        file("csv", "Tabular assignment results with disclosure-safe benchmark columns."),
			constants.EvaluationModelSummariesCSVFilename:     file("csv", "Tabular model-role aggregate counters."),
			constants.EvaluationCampaignExportSQLiteFilename: {
				Format:      "sqlite",
				Description: "Relational export with export_metadata, assignment_results, and model_summaries tables.",
				Tables: []campaignExportSchemaTable{
					{Name: "export_metadata", PrimaryKey: campaignExportPrimaryKey{"run_id"}},
					{Name: "assignment_results", PrimaryKey: campaignExportPrimaryKey{"assignment_id"}},
					{Name: "model_summaries", PrimaryKey: campaignExportPrimaryKey{"variant_id", "role"}},
				},
			},
		},
	}
}

func buildAssignmentResultsCSV(records []CampaignExportAssignmentRecord) ([]byte, error) {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	header := []string{
		"assignment_id",
		"run_id",
		"scenario_id",
		"scenario_category",
		"lane",
		"designated_role",
		"variant_id",
		"lifecycle_status",
		"summary_status",
		"result_digest",
		"verification_status",
		"completed_at",
		"unavailable_metric_reasons",
		"model_load_ms",
		"time_to_first_token_ms",
		"generation_ms",
		"whole_task_ms",
		"vram_peak_bytes",
		"system_ram_peak_bytes",
		"latency_ms",
		"input_tokens",
		"output_tokens",
		"retries",
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, record := range records {
		projection := record.Projection
		if projection == nil {
			continue
		}
		row := []string{
			projection.GetAssignmentId(),
			projection.GetRunId(),
			projection.GetScenarioId(),
			projection.GetScenarioCategory().String(),
			projection.GetLane().String(),
			projection.GetDesignatedRole().String(),
			projection.GetVariantId(),
			projection.GetLifecycleStatus().String(),
			projection.GetSummaryStatus().String(),
			projection.GetResultDigest(),
			projection.GetVerificationStatus(),
			formatTimestamp(projection.GetCompletedAt()),
			strings.Join(projection.GetUnavailableMetricReasons(), ";"),
			formatMetricValue(record.BenchmarkObservations, metricModelLoadMS),
			formatMetricValue(record.BenchmarkObservations, metricTimeToFirstTokenMS),
			formatMetricValue(record.BenchmarkObservations, metricGenerationMS),
			formatMetricValue(record.BenchmarkObservations, metricWholeTaskMS),
			formatMetricValue(record.BenchmarkObservations, metricVRAMPeakBytes),
			formatMetricValue(record.BenchmarkObservations, metricSystemRAMPeakBytes),
			formatResourceMetric(record.ResourceSummary, func(summary *PublicResourceSummary) PublicResourceMetric { return summary.LatencyMS }),
			formatResourceMetric(record.ResourceSummary, func(summary *PublicResourceSummary) PublicResourceMetric { return summary.InputTokens }),
			formatResourceMetric(record.ResourceSummary, func(summary *PublicResourceSummary) PublicResourceMetric { return summary.OutputTokens }),
			formatResourceMetric(record.ResourceSummary, func(summary *PublicResourceSummary) PublicResourceMetric { return summary.Retries }),
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

const (
	metricModelLoadMS        = "model_load_ms"
	metricTimeToFirstTokenMS = "time_to_first_token_ms"
	metricGenerationMS       = "generation_ms"
	metricWholeTaskMS        = "whole_task_ms"
	metricVRAMPeakBytes      = "vram_peak_bytes"
	metricSystemRAMPeakBytes = "system_ram_peak_bytes"
)

func formatMetricValue(observations *PublicBenchmarkObservations, metric string) string {
	if observations == nil {
		return ""
	}
	if observations.Timing != nil {
		switch metric {
		case metricModelLoadMS:
			return formatPublicMetric(observations.Timing.ModelLoadMS)
		case metricTimeToFirstTokenMS:
			return formatPublicMetric(observations.Timing.TimeToFirstTokenMS)
		case metricGenerationMS:
			return formatPublicMetric(observations.Timing.GenerationMS)
		case metricWholeTaskMS:
			return formatPublicMetric(observations.Timing.WholeTaskMS)
		}
	}
	if observations.GPU != nil {
		switch metric {
		case metricVRAMPeakBytes:
			return formatPublicMetric(observations.GPU.VRAMPeakBytes)
		case metricSystemRAMPeakBytes:
			return formatPublicMetric(observations.GPU.SystemRAMPeakBytes)
		}
	}
	return ""
}

func formatPublicMetric(value *PublicMetricValue) string {
	if value == nil || value.Value == nil {
		return ""
	}
	return fmt.Sprintf("%.3f", *value.Value)
}

func formatResourceMetric(summary *PublicResourceSummary, selectMetric func(*PublicResourceSummary) PublicResourceMetric) string {
	if summary == nil {
		return ""
	}
	metric := selectMetric(summary)
	if metric.Value == nil {
		return ""
	}
	return fmt.Sprintf("%.3f", *metric.Value)
}

func buildModelSummariesCSV(state *runAggregateState) ([]byte, error) {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	header := []string{
		"variant_id",
		"role",
		"scheduled",
		"terminal",
		"passed",
		"failed",
		"evaluation_coverage",
		"pass_rate_estimate",
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	if state == nil {
		writer.Flush()
		return buffer.Bytes(), writer.Error()
	}
	keys := make([]string, 0, len(state.VariantRoles))
	for key := range state.VariantRoles {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		bucket := state.VariantRoles[key]
		coverage := 0.0
		if bucket.Scheduled > 0 {
			coverage = float64(bucket.Terminal) / float64(bucket.Scheduled)
		}
		passEstimate := ""
		if bucket.Terminal > 0 {
			passEstimate = fmt.Sprintf("%.6f", float64(bucket.Passed)/float64(bucket.Terminal))
		}
		row := []string{
			bucket.VariantID,
			bucket.Role,
			fmt.Sprintf("%d", bucket.Scheduled),
			fmt.Sprintf("%d", bucket.Terminal),
			fmt.Sprintf("%d", bucket.Passed),
			fmt.Sprintf("%d", bucket.Failed),
			fmt.Sprintf("%.6f", coverage),
			passEstimate,
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func formatTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().UTC().Format(time.RFC3339Nano)
}

func (e *CampaignExporter) writeSQLiteExport(
	ctx context.Context,
	path string,
	run *evalv1.EvaluationRun,
	spec *evalv1.EvaluationCampaignSpec,
	catalog *evalv1.EvaluationScenarioCatalog,
	exportedAt time.Time,
	population *CampaignPopulationReport,
	assignments []CampaignExportAssignmentRecord,
	aggregate *runAggregateState,
	modelSummaries [][]byte,
) (err error) {
	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(path), slog.Default())
	if err != nil {
		return fmt.Errorf("evaluation: export campaign run: open sqlite: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("evaluation: export campaign run: close sqlite: %w", closeErr)
		}
	}()

	schema := `
CREATE TABLE export_metadata (
  schema_version TEXT NOT NULL,
  run_id TEXT PRIMARY KEY,
  campaign_id TEXT NOT NULL,
  catalog_digest TEXT NOT NULL,
  model_registry_digest TEXT NOT NULL,
  exported_at TEXT NOT NULL,
  assignment_count INTEGER NOT NULL,
  terminal_result_count INTEGER NOT NULL,
  population_complete INTEGER NOT NULL,
  expected_cells INTEGER NOT NULL
);
CREATE TABLE assignment_results (
  assignment_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  scenario_id TEXT NOT NULL,
  scenario_category TEXT NOT NULL,
  lane TEXT NOT NULL,
  designated_role TEXT,
  variant_id TEXT,
  lifecycle_status TEXT NOT NULL,
  summary_status TEXT NOT NULL,
  result_digest TEXT NOT NULL,
  verification_status TEXT NOT NULL,
  completed_at TEXT,
  unavailable_metric_reasons TEXT,
  model_load_ms REAL,
  time_to_first_token_ms REAL,
  generation_ms REAL,
  whole_task_ms REAL,
  vram_peak_bytes REAL,
  system_ram_peak_bytes REAL,
  projection_json TEXT NOT NULL
);
CREATE TABLE model_summaries (
  variant_id TEXT NOT NULL,
  role TEXT NOT NULL,
  scheduled INTEGER NOT NULL,
  terminal INTEGER NOT NULL,
  passed INTEGER NOT NULL,
  failed INTEGER NOT NULL,
  evaluation_coverage REAL,
  pass_rate_estimate REAL,
  summary_json TEXT NOT NULL,
  PRIMARY KEY (variant_id, role)
);`
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("evaluation: export campaign run: init sqlite schema: %w", err)
	}

	populationComplete := 0
	if population.Complete {
		populationComplete = 1
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO export_metadata (
  schema_version, run_id, campaign_id, catalog_digest, model_registry_digest,
  exported_at, assignment_count, terminal_result_count, population_complete, expected_cells
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		campaignExportSchemaVersion,
		run.GetRunId(),
		run.GetCampaignBinding().GetCampaignId(),
		catalog.GetCatalogDigest(),
		spec.GetModelRegistryDigest(),
		exportedAt.Format(time.RFC3339Nano),
		population.ScheduledAssignments,
		len(assignments),
		populationComplete,
		population.ExpectedCells,
	); err != nil {
		return fmt.Errorf("evaluation: export campaign run: insert export metadata: %w", err)
	}

	for _, record := range assignments {
		projection := record.Projection
		if projection == nil {
			continue
		}
		projectionJSON, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `
INSERT INTO assignment_results (
  assignment_id, run_id, scenario_id, scenario_category, lane, designated_role, variant_id,
  lifecycle_status, summary_status, result_digest, verification_status, completed_at,
  unavailable_metric_reasons, model_load_ms, time_to_first_token_ms, generation_ms,
  whole_task_ms, vram_peak_bytes, system_ram_peak_bytes, projection_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			projection.GetAssignmentId(),
			projection.GetRunId(),
			projection.GetScenarioId(),
			projection.GetScenarioCategory().String(),
			projection.GetLane().String(),
			projection.GetDesignatedRole().String(),
			projection.GetVariantId(),
			projection.GetLifecycleStatus().String(),
			projection.GetSummaryStatus().String(),
			projection.GetResultDigest(),
			projection.GetVerificationStatus(),
			formatTimestamp(projection.GetCompletedAt()),
			strings.Join(projection.GetUnavailableMetricReasons(), ";"),
			nullFloat(parseMetricValue(record.BenchmarkObservations, metricModelLoadMS)),
			nullFloat(parseMetricValue(record.BenchmarkObservations, metricTimeToFirstTokenMS)),
			nullFloat(parseMetricValue(record.BenchmarkObservations, metricGenerationMS)),
			nullFloat(parseMetricValue(record.BenchmarkObservations, metricWholeTaskMS)),
			nullFloat(parseMetricValue(record.BenchmarkObservations, metricVRAMPeakBytes)),
			nullFloat(parseMetricValue(record.BenchmarkObservations, metricSystemRAMPeakBytes)),
			string(projectionJSON),
		); err != nil {
			return fmt.Errorf("evaluation: export campaign run: insert assignment result: %w", err)
		}
	}

	keys := make([]string, 0, len(aggregate.VariantRoles))
	for key := range aggregate.VariantRoles {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	summaryByKey := map[string][]byte{}
	for _, raw := range modelSummaries {
		var body campaignModelSummaryIdentity
		if err := json.Unmarshal(raw, &body); err != nil {
			return fmt.Errorf("evaluation: export campaign run: decode model summary identity: %w", err)
		}
		summaryByKey[body.VariantID+":"+body.Role] = raw
	}
	for _, key := range keys {
		bucket := aggregate.VariantRoles[key]
		coverage := 0.0
		if bucket.Scheduled > 0 {
			coverage = float64(bucket.Terminal) / float64(bucket.Scheduled)
		}
		var passEstimate *float64
		if bucket.Terminal > 0 {
			estimate := float64(bucket.Passed) / float64(bucket.Terminal)
			passEstimate = &estimate
		}
		summaryJSON := string(summaryByKey[key])
		if summaryJSON == "" {
			fallback, err := json.Marshal(campaignModelSummaryFallback{
				Role:      bucket.Role,
				VariantID: bucket.VariantID,
			})
			if err != nil {
				return err
			}
			summaryJSON = string(fallback)
		}
		if _, err := db.ExecContext(ctx, `
INSERT INTO model_summaries (
  variant_id, role, scheduled, terminal, passed, failed,
  evaluation_coverage, pass_rate_estimate, summary_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			bucket.VariantID,
			bucket.Role,
			bucket.Scheduled,
			bucket.Terminal,
			bucket.Passed,
			bucket.Failed,
			coverage,
			passEstimate,
			summaryJSON,
		); err != nil {
			return fmt.Errorf("evaluation: export campaign run: insert model summary: %w", err)
		}
	}
	return nil
}

func parseMetricValue(observations *PublicBenchmarkObservations, metric string) *float64 {
	raw := formatMetricValue(observations, metric)
	if raw == "" {
		return nil
	}
	var value float64
	if _, err := fmt.Sscanf(raw, "%f", &value); err != nil {
		return nil
	}
	return &value
}

func nullFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
