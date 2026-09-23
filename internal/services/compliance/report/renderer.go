// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type Format string

const (
	FormatJSON     Format = "json"
	FormatOSCAL    Format = "oscal"
	FormatMarkdown Format = "markdown"
	FormatHTML     Format = "html"
	FormatCSV      Format = "csv"
	FormatCLI      Format = "cli"
)

type RenderedAnalysis struct {
	Format    Format
	MediaType string
	Body      []byte
}

type statusCount struct {
	Status string
	Count  int
}

func SupportedFormats() []Format {
	return []Format{FormatJSON, FormatOSCAL, FormatMarkdown, FormatHTML, FormatCSV, FormatCLI}
}

func ParseFormat(value string) (Format, error) {
	format := Format(value)
	for _, supported := range SupportedFormats() {
		if format == supported {
			return format, nil
		}
	}
	return "", fmt.Errorf("%w: unsupported compliance report format %q", constants.ErrValidationFailed, value)
}

func RenderComplianceAnalysis(analysis *compliancev1.ComplianceAnalysis, format Format) (*RenderedAnalysis, error) {
	if _, err := ParseFormat(string(format)); err != nil {
		return nil, err
	}
	if err := compliance.ValidateAnalysis(analysis); err != nil {
		return nil, fmt.Errorf("compliance report: validate analysis for rendering: %w", err)
	}

	var body []byte
	var mediaType string
	var err error
	switch format {
	case FormatJSON:
		body, err = compliancev1.MarshalCanonical(analysis)
		mediaType = constants.MediaTypeJSON
	case FormatOSCAL:
		oscal, exportErr := compliance.NewOSCALExporter(nil).GenerateAssessmentResults(analysis)
		if exportErr != nil {
			return nil, fmt.Errorf("compliance report: render %s: %w", format, exportErr)
		}
		body, err = json.MarshalIndent(oscal, "", "  ")
		mediaType = constants.MediaTypeOSCALJSON
	case FormatMarkdown:
		body = []byte(renderMarkdown(analysis))
		mediaType = constants.MediaTypeMarkdown
	case FormatHTML:
		body = []byte(renderHTML(analysis))
		mediaType = constants.MediaTypeHTML
	case FormatCSV:
		body, err = renderCSV(analysis)
		mediaType = constants.MediaTypeCSV
	case FormatCLI:
		body = []byte(renderCLI(analysis))
		mediaType = constants.MediaTypeText
	}
	if err != nil {
		return nil, fmt.Errorf("compliance report: render %s: %w", format, err)
	}
	return &RenderedAnalysis{Format: format, MediaType: mediaType, Body: body}, nil
}

func RenderPublicReleaseProjection(analysis *compliancev1.ComplianceAnalysis, format Format) (*RenderedAnalysis, error) {
	if analysis == nil {
		return nil, fmt.Errorf("%w: compliance analysis is required", constants.ErrValidationFailed)
	}
	var body []byte
	var mediaType string
	var err error
	switch format {
	case FormatMarkdown:
		body = []byte(renderPublicMarkdown(analysis))
		mediaType = constants.MediaTypeMarkdown
	case FormatCSV:
		body, err = renderPublicCSV(analysis)
		mediaType = constants.MediaTypeCSV
	default:
		return nil, fmt.Errorf("%w: unsupported public release projection format %q", constants.ErrValidationFailed, format)
	}
	if err != nil {
		return nil, fmt.Errorf("compliance report: render public %s projection: %w", format, err)
	}
	return &RenderedAnalysis{Format: format, MediaType: mediaType, Body: body}, nil
}

func renderPublicMarkdown(analysis *compliancev1.ComplianceAnalysis) string {
	var body strings.Builder
	window := analysis.GetEvidenceWindowCompleteness()
	fmt.Fprintln(&body, "# g8e Evidence-Native Assurance Report")
	fmt.Fprintf(&body, "\n## Assessment Boundary\n\n- Analysis: `%s`\n- Scope digest: `%s`\n- Generated: `%s`\n- Generator: `%s@%s`\n- Evidence graph: `%t`\n", markdownValue(analysis.GetAnalysisId()), markdownValue(analysis.GetAssessmentScopeSha256()), analysis.GetGeneratedAt().AsTime().UTC().Format(time.RFC3339), markdownValue(analysis.GetGeneratorIdentity()), markdownValue(analysis.GetGeneratorVersion()), analysis.GetEvidenceGraphValid())
	fmt.Fprintf(&body, "\n## Evidence Window\n\nStatus: `%s`; evidence: %d expected, %d actual; window: `%s` to `%s`.\n", markdownValue(window.GetCompletenessStatus()), window.GetExpectedEvidenceCount(), window.GetActualEvidenceCount(), markdownValue(window.GetWindowStartRef()), markdownValue(window.GetWindowEndRef()))
	fmt.Fprintln(&body, "\n## g8e-Native Assertions\n\n| Assertion | Version | Status | Evidence level | Freshness | Coverage | Proof digests |\n| --- | --- | --- | --- | --- | --- | --- |")
	publicRefs := publicEvidenceReferenceSet(analysis)
	for _, assessment := range sortedAssertionAssessments(analysis) {
		refs := append(append([]string(nil), assessment.GetEvidenceRefs()...), assessment.GetMetricRefs()...)
		fmt.Fprintf(&body, "| %s | %s | %s | %s | %s | %s | %s |\n", markdownValue(assessment.GetAssertionRef().GetId()), markdownValue(assessment.GetAssertionRef().GetVersion()), markdownValue(assessment.GetStatus()), markdownValue(assessment.GetEvidenceLevel()), markdownValue(assessment.GetFreshnessStatus()), markdownValue(coverageSummary(assessment.GetCoverage())), markdownValue(strings.Join(filterPublicEvidenceRefs(refs, publicRefs), ", ")))
	}
	fmt.Fprintln(&body, "\n## External Alignment\n\n| Framework | Control | Status | Responsibility | Evidence level |\n| --- | --- | --- | --- | --- |")
	for _, assessment := range sortedFrameworkAssessments(analysis) {
		fmt.Fprintf(&body, "| %s | %s | %s | %s | %s |\n", markdownValue(assessment.GetFrameworkRef().GetId()), markdownValue(assessment.GetControlId()), markdownValue(assessment.GetStatus()), markdownValue(assessment.GetResponsibility()), markdownValue(assessment.GetEvidenceLevel()))
	}
	fmt.Fprintln(&body, "\n## Proof Digest Inventory\n\n| Artifact | Type | Verification | Verifier |\n| --- | --- | --- | --- |")
	for _, resource := range publicEvidenceResources(analysis) {
		fmt.Fprintf(&body, "| %s | %s | %s | %s |\n", markdownValue(resource.GetArtifactId()), markdownValue(resource.GetArtifactType()), markdownValue(resource.GetVerificationStatus()), markdownValue(versionedReferenceValue(&compliancev1.VersionedReference{Id: resource.GetVerifierId(), Version: resource.GetVerifierVersion()})))
	}
	fmt.Fprintln(&body, "\n## Diagnostic Summary\n\n| Code | Severity | Count |\n| --- | --- | --- |")
	for _, diagnostic := range publicDiagnosticCounts(analysis) {
		fmt.Fprintf(&body, "| %s | %s | %d |\n", markdownValue(diagnostic.code), markdownValue(diagnostic.severity), diagnostic.count)
	}
	fmt.Fprintln(&body, "\n## Claim Boundaries\n\nThis public projection reports reproduced technical assertions and conservative external alignment. It is not certification, accreditation, authorization, legal compliance, or recurring operating-effectiveness evidence. Source-local identities, runtime locations, free-form diagnostics, and source bodies remain available only in the independently verifiable complete bundle.")
	return body.String()
}

func renderPublicCSV(analysis *compliancev1.ComplianceAnalysis) ([]byte, error) {
	var body strings.Builder
	writer := csv.NewWriter(&body)
	header := []string{"record_type", "identifier", "reference", "status", "evidence_level", "freshness", "selected_subjects", "assessed_subjects", "failed_subjects", "unavailable_subjects", "proof_digests", "verification_status", "verifier", "diagnostic_severity", "count"}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("compliance report: write public CSV header: %w", err)
	}
	write := func(row []string) error {
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("compliance report: write public CSV row: %w", err)
		}
		return nil
	}
	analysisRow := make([]string, len(header))
	analysisRow[0] = "analysis"
	analysisRow[1] = analysis.GetAnalysisId()
	analysisRow[2] = analysis.GetAssessmentScopeSha256()
	analysisRow[3] = fmt.Sprintf("graph_valid=%t", analysis.GetEvidenceGraphValid())
	if err := write(analysisRow); err != nil {
		return nil, err
	}
	publicRefs := publicEvidenceReferenceSet(analysis)
	for _, assessment := range sortedAssertionAssessments(analysis) {
		coverage := assessment.GetCoverage()
		refs := append(append([]string(nil), assessment.GetEvidenceRefs()...), assessment.GetMetricRefs()...)
		if err := write([]string{"assertion", assessment.GetAssertionRef().GetId(), versionedReferenceValue(assessment.GetAssertionRef()), assessment.GetStatus(), assessment.GetEvidenceLevel(), assessment.GetFreshnessStatus(), int32String(coverage.GetSelectedSubjectCount()), int32String(coverage.GetAssessedSubjectCount()), int32String(coverage.GetFailedSubjectCount()), int32String(coverage.GetUnavailableSubjectCount()), strings.Join(filterPublicEvidenceRefs(refs, publicRefs), ";"), "", versionedReferenceValue(assessment.GetVerifierRef()), "", ""}); err != nil {
			return nil, err
		}
	}
	for _, assessment := range sortedFrameworkAssessments(analysis) {
		if err := write([]string{"framework_control", assessment.GetControlId(), versionedReferenceValue(assessment.GetFrameworkRef()), assessment.GetStatus(), assessment.GetEvidenceLevel(), "", "", "", "", "", "", "", "", "", ""}); err != nil {
			return nil, err
		}
	}
	for _, resource := range publicEvidenceResources(analysis) {
		if err := write([]string{"evidence_digest", resource.GetArtifactId(), resource.GetArtifactType(), "", "", "", "", "", "", "", "", resource.GetVerificationStatus(), versionedReferenceValue(&compliancev1.VersionedReference{Id: resource.GetVerifierId(), Version: resource.GetVerifierVersion()}), "", ""}); err != nil {
			return nil, err
		}
	}
	for _, diagnostic := range publicDiagnosticCounts(analysis) {
		if err := write([]string{"diagnostic_summary", diagnostic.code, "", "", "", "", "", "", "", "", "", "", "", diagnostic.severity, fmt.Sprintf("%d", diagnostic.count)}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("compliance report: flush public CSV: %w", err)
	}
	return []byte(body.String()), nil
}

type publicDiagnosticCount struct {
	code     string
	severity string
	count    int
}

func publicDiagnosticCounts(analysis *compliancev1.ComplianceAnalysis) []publicDiagnosticCount {
	counts := make(map[string]int)
	for _, diagnostic := range analysis.GetDiagnostics() {
		key := publicDiagnosticCode(diagnostic.GetCode()) + "\x00" + publicDiagnosticSeverity(diagnostic.GetSeverity())
		counts[key]++
	}
	result := make([]publicDiagnosticCount, 0, len(counts))
	for key, count := range counts {
		parts := strings.SplitN(key, "\x00", 2)
		result = append(result, publicDiagnosticCount{code: parts[0], severity: parts[1], count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].code == result[j].code {
			return result[i].severity < result[j].severity
		}
		return result[i].code < result[j].code
	})
	return result
}

func publicDiagnosticCode(code string) string {
	switch code {
	case "assessment_context_unavailable",
		"required_evidence_missing",
		"required_evidence_stale",
		"campaign_native_assertion_unmapped",
		"campaign_population_incomplete",
		"campaign_verification_failure",
		"evaluation_candidate_native",
		"evaluation_candidate_campaign",
		"evaluation_candidate_incomplete",
		"evaluation_candidate_unsupported",
		"evaluation_candidate_malformed",
		"evaluation_candidate_outside_window":
		return code
	default:
		return "assessment_diagnostic"
	}
}

func publicDiagnosticSeverity(severity string) string {
	switch severity {
	case "info", "warning", "error":
		return severity
	default:
		return "unknown"
	}
}

func publicEvidenceResources(analysis *compliancev1.ComplianceAnalysis) []*compliancev1.ComplianceEvidenceReference {
	resources := make([]*compliancev1.ComplianceEvidenceReference, 0, len(analysis.GetEvidenceResources()))
	for _, resource := range analysis.GetEvidenceResources() {
		if resource == nil || !isPublicEvidenceDigest(resource.GetArtifactId(), resource.GetSha256()) {
			continue
		}
		resources = append(resources, resource)
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].GetArtifactId() < resources[j].GetArtifactId() })
	return resources
}

func publicEvidenceReferenceSet(analysis *compliancev1.ComplianceAnalysis) map[string]struct{} {
	refs := make(map[string]struct{})
	for _, resource := range publicEvidenceResources(analysis) {
		refs[resource.GetArtifactId()] = struct{}{}
	}
	return refs
}

func filterPublicEvidenceRefs(refs []string, allowed map[string]struct{}) []string {
	result := make([]string, 0, len(refs))
	for _, ref := range refs {
		if _, ok := allowed[ref]; ok {
			result = append(result, ref)
		}
	}
	return sortedStrings(result)
}

func isPublicEvidenceDigest(artifactID, digest string) bool {
	if len(digest) != sha256.Size*2 || !strings.HasSuffix(artifactID, ":sha256:"+digest) {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func renderMarkdown(analysis *compliancev1.ComplianceAnalysis) string {
	var body strings.Builder
	window := analysis.GetEvidenceWindowCompleteness()
	fmt.Fprintln(&body, "# g8e Compliance Report")
	fmt.Fprintf(&body, "\n## Assessment Boundary\n\n- Analysis: `%s`\n- Scope: `%s`\n- Scope digest: `%s`\n- Generated: `%s`\n- Generator: `%s@%s`\n- Evidence graph: `%t`\n", markdownValue(analysis.GetAnalysisId()), markdownValue(analysis.GetScopeRef()), markdownValue(analysis.GetAssessmentScopeSha256()), analysis.GetGeneratedAt().AsTime().UTC().Format(time.RFC3339), markdownValue(analysis.GetGeneratorIdentity()), markdownValue(analysis.GetGeneratorVersion()), analysis.GetEvidenceGraphValid())
	fmt.Fprintf(&body, "\n## Evidence Window\n\nStatus: `%s`; evidence: %d expected, %d actual; window: `%s` to `%s`.\n", markdownValue(window.GetCompletenessStatus()), window.GetExpectedEvidenceCount(), window.GetActualEvidenceCount(), markdownValue(window.GetWindowStartRef()), markdownValue(window.GetWindowEndRef()))
	fmt.Fprintln(&body, "\n## Assertion Assessments\n\n| Assertion | Version | Status | Evidence level | Freshness | Coverage | Evidence references |\n| --- | --- | --- | --- | --- | --- | --- |")
	for _, assessment := range sortedAssertionAssessments(analysis) {
		fmt.Fprintf(&body, "| %s | %s | %s | %s | %s | %s | %s |\n", markdownValue(assessment.GetAssertionRef().GetId()), markdownValue(assessment.GetAssertionRef().GetVersion()), markdownValue(assessment.GetStatus()), markdownValue(assessment.GetEvidenceLevel()), markdownValue(assessment.GetFreshnessStatus()), markdownValue(coverageSummary(assessment.GetCoverage())), markdownValue(strings.Join(sortedStrings(append(append([]string(nil), assessment.GetEvidenceRefs()...), assessment.GetMetricRefs()...)), ", ")))
	}
	fmt.Fprintln(&body, "\n## Framework Controls\n\n| Framework | Control | Status | Responsibility | Evidence level |\n| --- | --- | --- | --- | --- |")
	for _, assessment := range sortedFrameworkAssessments(analysis) {
		fmt.Fprintf(&body, "| %s | %s | %s | %s | %s |\n", markdownValue(assessment.GetFrameworkRef().GetId()), markdownValue(assessment.GetControlId()), markdownValue(assessment.GetStatus()), markdownValue(assessment.GetResponsibility()), markdownValue(assessment.GetEvidenceLevel()))
	}
	renderMarkdownEvidence(&body, analysis)
	renderMarkdownDiagnostics(&body, analysis)
	renderMarkdownList(&body, "Gaps", gapDescriptions(analysis))
	renderMarkdownList(&body, "Findings", findingDescriptions(analysis))
	renderMarkdownList(&body, "Remediation", remediationDescriptions(analysis))
	renderMarkdownList(&body, "Limitations", analysis.GetLimitations())
	return body.String()
}

func renderMarkdownEvidence(body *strings.Builder, analysis *compliancev1.ComplianceAnalysis) {
	fmt.Fprintln(body, "\n## Evidence Inventory\n\n| Artifact | Type | Source admission | Run | Scenario | Verification | Bundle path |\n| --- | --- | --- | --- | --- | --- | --- |")
	for _, resource := range sortedEvidenceResources(analysis) {
		fmt.Fprintf(body, "| %s | %s | %s | %s | %s | %s | %s |\n", markdownValue(resource.GetArtifactId()), markdownValue(resource.GetArtifactType()), markdownValue(resource.GetSourceAdmissionId()), markdownValue(resource.GetRunId()), markdownValue(resource.GetScenarioId()), markdownValue(resource.GetVerificationStatus()), markdownValue(resource.GetBundlePath()))
	}
	fmt.Fprint(body, "\n### Evidence Links\n\n")
	for _, link := range sortedEvidenceLinks(analysis) {
		fmt.Fprintf(body, "- `%s` %s `%s`\n", markdownValue(link.GetSourceRef()), markdownValue(link.GetLinkType()), markdownValue(link.GetTargetRef()))
	}
}

func renderMarkdownDiagnostics(body *strings.Builder, analysis *compliancev1.ComplianceAnalysis) {
	renderMarkdownList(body, "Diagnostics", diagnosticDescriptions(analysis))
}

func renderCSV(analysis *compliancev1.ComplianceAnalysis) ([]byte, error) {
	var body strings.Builder
	writer := csv.NewWriter(&body)
	header := []string{"record_type", "identifier", "reference", "status", "evidence_level", "freshness", "selected_subjects", "assessed_subjects", "failed_subjects", "unavailable_subjects", "evidence_refs", "metric_refs", "source_admission_id", "run_id", "attempt_id", "scenario_id", "transaction_id", "verification_status", "verifier", "bundle_path", "diagnostic_code", "diagnostic_severity", "diagnostic_message"}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("compliance report: write CSV header: %w", err)
	}
	analysisRow := make([]string, len(header))
	analysisRow[0] = "analysis"
	analysisRow[1] = analysis.GetAnalysisId()
	analysisRow[2] = analysis.GetScopeRef()
	analysisRow[3] = fmt.Sprintf("graph_valid=%t", analysis.GetEvidenceGraphValid())
	analysisRow[19] = analysis.GetAssessmentScopeSha256()
	if err := writer.Write(analysisRow); err != nil {
		return nil, fmt.Errorf("compliance report: write CSV analysis row: %w", err)
	}
	write := func(row []string) error {
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("compliance report: write CSV row: %w", err)
		}
		return nil
	}
	for _, assessment := range sortedAssertionAssessments(analysis) {
		coverage := assessment.GetCoverage()
		if err := write([]string{"assertion", assessment.GetAssessmentId(), versionedReferenceValue(assessment.GetAssertionRef()), assessment.GetStatus(), assessment.GetEvidenceLevel(), assessment.GetFreshnessStatus(), int32String(coverage.GetSelectedSubjectCount()), int32String(coverage.GetAssessedSubjectCount()), int32String(coverage.GetFailedSubjectCount()), int32String(coverage.GetUnavailableSubjectCount()), strings.Join(sortedStrings(assessment.GetEvidenceRefs()), ";"), strings.Join(sortedStrings(assessment.GetMetricRefs()), ";"), "", "", "", "", "", "", versionedReferenceValue(assessment.GetVerifierRef()), coverageSummary(coverage), "", "", ""}); err != nil {
			return nil, err
		}
	}
	for _, assessment := range sortedFrameworkAssessments(analysis) {
		if err := write([]string{"framework_control", assessment.GetAssessmentId(), versionedReferenceValue(assessment.GetFrameworkRef()) + "/" + assessment.GetControlId(), assessment.GetStatus(), assessment.GetEvidenceLevel(), "", "", "", "", "", strings.Join(sortedStrings(assessment.GetFindings()), ";"), strings.Join(sortedStrings(assessment.GetAssertionAssessmentRefs()), ";"), "", "", "", "", "", "", "", "", "", "", ""}); err != nil {
			return nil, err
		}
	}
	for _, resource := range sortedEvidenceResources(analysis) {
		if err := write([]string{"evidence", resource.GetArtifactId(), resource.GetArtifactType(), "", "", "", "", "", "", "", "", "", resource.GetSourceAdmissionId(), resource.GetRunId(), resource.GetAttemptId(), resource.GetScenarioId(), resource.GetTransactionId(), resource.GetVerificationStatus(), versionedReferenceValue(&compliancev1.VersionedReference{Id: resource.GetVerifierId(), Version: resource.GetVerifierVersion()}), resource.GetBundlePath(), "", "", ""}); err != nil {
			return nil, err
		}
	}
	for _, link := range sortedEvidenceLinks(analysis) {
		if err := write([]string{"evidence_link", link.GetSourceRef(), link.GetTargetRef(), "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", link.GetLinkType(), "", "", "", ""}); err != nil {
			return nil, err
		}
	}
	for _, diagnostic := range sortedDiagnostics(analysis.GetDiagnostics()) {
		subject := diagnostic.GetSubject()
		if err := write([]string{"diagnostic", diagnostic.GetSourceAdmissionId(), "", "", "", "", "", "", "", "", "", "", diagnostic.GetSourceAdmissionId(), subject.GetRunId(), subject.GetAttemptId(), subject.GetScenarioId(), subject.GetTransactionId(), "", "", "", diagnostic.GetCode(), diagnostic.GetSeverity(), diagnostic.GetMessage()}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("compliance report: flush CSV: %w", err)
	}
	return []byte(body.String()), nil
}

func coverageSummary(coverage *compliancev1.AssessmentCoverage) string {
	if coverage == nil {
		return "not recorded"
	}
	return fmt.Sprintf("%d selected, %d assessed, %d failed, %d unavailable", coverage.GetSelectedSubjectCount(), coverage.GetAssessedSubjectCount(), coverage.GetFailedSubjectCount(), coverage.GetUnavailableSubjectCount())
}

func int32String(value int32) string {
	return fmt.Sprintf("%d", value)
}

func versionedReferenceValue(reference *compliancev1.VersionedReference) string {
	if reference == nil {
		return ""
	}
	if reference.GetVersion() == "" {
		return reference.GetId()
	}
	return reference.GetId() + "@" + reference.GetVersion()
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func sortedEvidenceResources(analysis *compliancev1.ComplianceAnalysis) []*compliancev1.ComplianceEvidenceReference {
	resources := append([]*compliancev1.ComplianceEvidenceReference(nil), analysis.GetEvidenceResources()...)
	sort.Slice(resources, func(i, j int) bool { return resources[i].GetArtifactId() < resources[j].GetArtifactId() })
	return resources
}

func sortedEvidenceLinks(analysis *compliancev1.ComplianceAnalysis) []*compliancev1.EvidenceLink {
	links := append([]*compliancev1.EvidenceLink(nil), analysis.GetEvidenceLinks()...)
	sort.Slice(links, func(i, j int) bool {
		if links[i].GetSourceRef() != links[j].GetSourceRef() {
			return links[i].GetSourceRef() < links[j].GetSourceRef()
		}
		return links[i].GetTargetRef() < links[j].GetTargetRef()
	})
	return links
}

func sortedDiagnostics(diagnostics []*compliancev1.AssessmentDiagnostic) []*compliancev1.AssessmentDiagnostic {
	result := append([]*compliancev1.AssessmentDiagnostic(nil), diagnostics...)
	sort.Slice(result, func(i, j int) bool {
		left := result[i].GetCode() + result[i].GetSourceAdmissionId() + result[i].GetMessage()
		right := result[j].GetCode() + result[j].GetSourceAdmissionId() + result[j].GetMessage()
		return left < right
	})
	return result
}

func evidenceLinkDescriptions(analysis *compliancev1.ComplianceAnalysis) []string {
	values := make([]string, 0, len(analysis.GetEvidenceLinks()))
	for _, link := range sortedEvidenceLinks(analysis) {
		values = append(values, fmt.Sprintf("%s %s %s", link.GetSourceRef(), link.GetLinkType(), link.GetTargetRef()))
	}
	return values
}

func diagnosticDescriptions(analysis *compliancev1.ComplianceAnalysis) []string {
	values := make([]string, 0, len(analysis.GetDiagnostics()))
	for _, diagnostic := range sortedDiagnostics(analysis.GetDiagnostics()) {
		subject := diagnostic.GetSubject()
		values = append(values, fmt.Sprintf("%s [%s] source=%s subject=%s: %s", diagnostic.GetCode(), diagnostic.GetSeverity(), diagnostic.GetSourceAdmissionId(), subjectSelectionValue(subject), diagnostic.GetMessage()))
	}
	return values
}

func subjectSelectionValue(subject *compliancev1.AssessmentSubjectSelection) string {
	if subject == nil {
		return ""
	}
	values := []string{subject.GetRunId(), subject.GetAttemptId(), subject.GetScenarioId(), subject.GetTransactionId()}
	return strings.Join(sortedStrings(values), "/")
}

func renderMarkdownList(body *strings.Builder, title string, values []string) {
	fmt.Fprintf(body, "\n## %s\n", title)
	if len(values) == 0 {
		fmt.Fprintln(body, "\nNone.")
		return
	}
	for _, value := range values {
		fmt.Fprintf(body, "\n- %s", markdownValue(value))
	}
	body.WriteByte('\n')
}

func renderHTML(analysis *compliancev1.ComplianceAnalysis) string {
	var body strings.Builder
	window := analysis.GetEvidenceWindowCompleteness()
	body.WriteString("<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>g8e Compliance Report</title></head><body>\n")
	body.WriteString("<h1>g8e Compliance Report</h1>\n<dl>")
	htmlDefinition(&body, "Analysis", analysis.GetAnalysisId())
	htmlDefinition(&body, "Scope", analysis.GetScopeRef())
	htmlDefinition(&body, "Generated", analysis.GetGeneratedAt().AsTime().UTC().Format(time.RFC3339))
	htmlDefinition(&body, "Generator", analysis.GetGeneratorIdentity()+"@"+analysis.GetGeneratorVersion())
	htmlDefinition(&body, "Evidence graph", fmt.Sprint(analysis.GetEvidenceGraphValid()))
	body.WriteString("</dl>\n<h2>Evidence Window</h2>\n<p>")
	fmt.Fprintf(&body, "Status: %s; evidence: %d expected, %d actual; window: %s to %s.", html.EscapeString(window.GetCompletenessStatus()), window.GetExpectedEvidenceCount(), window.GetActualEvidenceCount(), html.EscapeString(window.GetWindowStartRef()), html.EscapeString(window.GetWindowEndRef()))
	body.WriteString("</p>\n<h2>Assertion Assessments</h2>\n<table><thead><tr><th>Assertion</th><th>Version</th><th>Status</th><th>Evidence level</th><th>Freshness</th><th>Coverage</th><th>Evidence references</th></tr></thead><tbody>\n")
	for _, assessment := range sortedAssertionAssessments(analysis) {
		refs := append(append([]string(nil), assessment.GetEvidenceRefs()...), assessment.GetMetricRefs()...)
		fmt.Fprintf(&body, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n", html.EscapeString(assessment.GetAssertionRef().GetId()), html.EscapeString(assessment.GetAssertionRef().GetVersion()), html.EscapeString(assessment.GetStatus()), html.EscapeString(assessment.GetEvidenceLevel()), html.EscapeString(assessment.GetFreshnessStatus()), html.EscapeString(coverageSummary(assessment.GetCoverage())), html.EscapeString(strings.Join(sortedStrings(refs), ", ")))
	}
	body.WriteString("</tbody></table>\n<h2>Framework Controls</h2>\n<table><thead><tr><th>Framework</th><th>Control</th><th>Status</th><th>Responsibility</th><th>Evidence level</th></tr></thead><tbody>\n")
	for _, assessment := range sortedFrameworkAssessments(analysis) {
		fmt.Fprintf(&body, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n", html.EscapeString(assessment.GetFrameworkRef().GetId()), html.EscapeString(assessment.GetControlId()), html.EscapeString(assessment.GetStatus()), html.EscapeString(assessment.GetResponsibility()), html.EscapeString(assessment.GetEvidenceLevel()))
	}
	body.WriteString("</tbody></table>\n<h2>Evidence Inventory</h2>\n<table><thead><tr><th>Artifact</th><th>Type</th><th>Source admission</th><th>Run</th><th>Scenario</th><th>Verification</th><th>Bundle path</th></tr></thead><tbody>\n")
	for _, resource := range sortedEvidenceResources(analysis) {
		fmt.Fprintf(&body, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n", html.EscapeString(resource.GetArtifactId()), html.EscapeString(resource.GetArtifactType()), html.EscapeString(resource.GetSourceAdmissionId()), html.EscapeString(resource.GetRunId()), html.EscapeString(resource.GetScenarioId()), html.EscapeString(resource.GetVerificationStatus()), html.EscapeString(resource.GetBundlePath()))
	}
	body.WriteString("</tbody></table>\n")
	renderHTMLList(&body, "Evidence Links", evidenceLinkDescriptions(analysis))
	renderHTMLList(&body, "Diagnostics", diagnosticDescriptions(analysis))
	renderHTMLList(&body, "Gaps", gapDescriptions(analysis))
	renderHTMLList(&body, "Findings", findingDescriptions(analysis))
	renderHTMLList(&body, "Remediation", remediationDescriptions(analysis))
	renderHTMLList(&body, "Limitations", analysis.GetLimitations())
	body.WriteString("</body></html>\n")
	return body.String()
}

func htmlDefinition(body *strings.Builder, term, value string) {
	fmt.Fprintf(body, "<dt>%s</dt><dd>%s</dd>", html.EscapeString(term), html.EscapeString(value))
}

func renderHTMLList(body *strings.Builder, title string, values []string) {
	fmt.Fprintf(body, "<h2>%s</h2>\n", html.EscapeString(title))
	if len(values) == 0 {
		body.WriteString("<p>None.</p>\n")
		return
	}
	body.WriteString("<ul>\n")
	for _, value := range values {
		fmt.Fprintf(body, "<li>%s</li>\n", html.EscapeString(value))
	}
	body.WriteString("</ul>\n")
}

func renderCLI(analysis *compliancev1.ComplianceAnalysis) string {
	var body strings.Builder
	window := analysis.GetEvidenceWindowCompleteness()
	fmt.Fprintf(&body, "Compliance analysis: %s\n", analysis.GetAnalysisId())
	fmt.Fprintf(&body, "Scope: %s\n", analysis.GetScopeRef())
	fmt.Fprintf(&body, "Generated: %s\n", analysis.GetGeneratedAt().AsTime().UTC().Format(time.RFC3339))
	fmt.Fprintf(&body, "Evidence graph: valid=%t\n", analysis.GetEvidenceGraphValid())
	fmt.Fprintf(&body, "Evidence window: %s (%d/%d)\n", window.GetCompletenessStatus(), window.GetActualEvidenceCount(), window.GetExpectedEvidenceCount())
	fmt.Fprintf(&body, "Scope digest: %s\n", analysis.GetAssessmentScopeSha256())
	fmt.Fprintf(&body, "Assertion assessments: %d%s\n", len(analysis.GetAssertionAssessments()), formatStatusCounts(assertionStatusCounts(analysis)))
	for _, assessment := range sortedAssertionAssessments(analysis) {
		fmt.Fprintf(&body, "Assertion %s: status=%s evidence=%s freshness=%s coverage=%s refs=%s\n", assessment.GetAssessmentId(), assessment.GetStatus(), assessment.GetEvidenceLevel(), assessment.GetFreshnessStatus(), coverageSummary(assessment.GetCoverage()), strings.Join(sortedStrings(append(append([]string(nil), assessment.GetEvidenceRefs()...), assessment.GetMetricRefs()...)), ","))
	}
	fmt.Fprintf(&body, "Framework controls: %d%s\n", len(analysis.GetFrameworkAssessments()), formatStatusCounts(frameworkStatusCounts(analysis)))
	fmt.Fprintf(&body, "Evidence resources: %d\nDiagnostics: %d\n", len(analysis.GetEvidenceResources()), len(analysis.GetDiagnostics()))
	for _, link := range sortedEvidenceLinks(analysis) {
		fmt.Fprintf(&body, "Evidence link: %s %s %s\n", link.GetSourceRef(), link.GetLinkType(), link.GetTargetRef())
	}
	for _, diagnostic := range sortedDiagnostics(analysis.GetDiagnostics()) {
		fmt.Fprintf(&body, "Diagnostic %s [%s]: %s\n", diagnostic.GetCode(), diagnostic.GetSeverity(), diagnostic.GetMessage())
	}
	fmt.Fprintf(&body, "Gaps: %d\nFindings: %d\nRemediation: %d\nLimitations: %d\n", len(analysis.GetGaps()), len(analysis.GetFindings()), len(analysis.GetRemediation()), len(analysis.GetLimitations()))
	return body.String()
}

func sortedAssertionAssessments(analysis *compliancev1.ComplianceAnalysis) []*compliancev1.ControlAssertionAssessment {
	assessments := append([]*compliancev1.ControlAssertionAssessment(nil), analysis.GetAssertionAssessments()...)
	sort.Slice(assessments, func(i, j int) bool {
		return assessments[i].GetAssessmentId() < assessments[j].GetAssessmentId()
	})
	return assessments
}

func sortedFrameworkAssessments(analysis *compliancev1.ComplianceAnalysis) []*compliancev1.FrameworkControlAssessment {
	assessments := append([]*compliancev1.FrameworkControlAssessment(nil), analysis.GetFrameworkAssessments()...)
	sort.Slice(assessments, func(i, j int) bool {
		return assessments[i].GetAssessmentId() < assessments[j].GetAssessmentId()
	})
	return assessments
}

func assertionStatusCounts(analysis *compliancev1.ComplianceAnalysis) []statusCount {
	statuses := make([]string, 0, len(analysis.GetAssertionAssessments()))
	for _, assessment := range analysis.GetAssertionAssessments() {
		statuses = append(statuses, assessment.GetStatus())
	}
	return countStatuses(statuses)
}

func frameworkStatusCounts(analysis *compliancev1.ComplianceAnalysis) []statusCount {
	statuses := make([]string, 0, len(analysis.GetFrameworkAssessments()))
	for _, assessment := range analysis.GetFrameworkAssessments() {
		statuses = append(statuses, assessment.GetStatus())
	}
	return countStatuses(statuses)
}

func countStatuses(statuses []string) []statusCount {
	sort.Strings(statuses)
	counts := make([]statusCount, 0, len(statuses))
	for _, status := range statuses {
		if len(counts) == 0 || counts[len(counts)-1].Status != status {
			counts = append(counts, statusCount{Status: status, Count: 1})
			continue
		}
		counts[len(counts)-1].Count++
	}
	return counts
}

func formatStatusCounts(counts []statusCount) string {
	if len(counts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(counts))
	for _, count := range counts {
		parts = append(parts, fmt.Sprintf("%s=%d", count.Status, count.Count))
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func gapDescriptions(analysis *compliancev1.ComplianceAnalysis) []string {
	values := make([]string, 0, len(analysis.GetGaps()))
	for _, gap := range analysis.GetGaps() {
		values = append(values, gap.GetGapId()+": "+gap.GetDescription())
	}
	sort.Strings(values)
	return values
}

func findingDescriptions(analysis *compliancev1.ComplianceAnalysis) []string {
	values := make([]string, 0, len(analysis.GetFindings()))
	for _, finding := range analysis.GetFindings() {
		values = append(values, finding.GetFindingId()+" ["+finding.GetSeverity()+"]: "+finding.GetDescription())
	}
	sort.Strings(values)
	return values
}

func remediationDescriptions(analysis *compliancev1.ComplianceAnalysis) []string {
	values := make([]string, 0, len(analysis.GetRemediation()))
	for _, remediation := range analysis.GetRemediation() {
		values = append(values, remediation.GetRemediationId()+" ["+remediation.GetPriority()+", "+remediation.GetOwner()+"]: "+remediation.GetAction())
	}
	sort.Strings(values)
	return values
}

func markdownValue(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.ReplaceAll(value, "\n", " ")
}
