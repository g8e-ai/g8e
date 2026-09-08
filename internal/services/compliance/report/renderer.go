// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
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
	return []Format{FormatJSON, FormatOSCAL, FormatMarkdown, FormatHTML, FormatCLI}
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
	oscal, err := compliance.NewOSCALExporter(nil).GenerateAssessmentResults(analysis)
	if err != nil {
		return nil, fmt.Errorf("compliance report: validate analysis for rendering: %w", err)
	}

	var body []byte
	var mediaType string
	switch format {
	case FormatJSON:
		body, err = compliancev1.MarshalCanonical(analysis)
		mediaType = constants.MediaTypeJSON
	case FormatOSCAL:
		body, err = json.MarshalIndent(oscal, "", "  ")
		mediaType = constants.MediaTypeOSCALJSON
	case FormatMarkdown:
		body = []byte(renderMarkdown(analysis))
		mediaType = constants.MediaTypeMarkdown
	case FormatHTML:
		body = []byte(renderHTML(analysis))
		mediaType = constants.MediaTypeHTML
	case FormatCLI:
		body = []byte(renderCLI(analysis))
		mediaType = constants.MediaTypeText
	}
	if err != nil {
		return nil, fmt.Errorf("compliance report: render %s: %w", format, err)
	}
	if format == FormatOSCAL {
		validator, err := compliance.NewOSCALDocumentValidator()
		if err != nil {
			return nil, err
		}
		validation, err := validator.ValidateBytes(body)
		if err != nil {
			return nil, err
		}
		if !validation.GetValid() {
			return nil, fmt.Errorf("%w: rendered OSCAL has %d structural failures and %d semantic failures", constants.ErrOSCALValidationFailed, len(validation.GetStructuralFailures()), len(validation.GetSemanticFailures()))
		}
	}
	return &RenderedAnalysis{Format: format, MediaType: mediaType, Body: body}, nil
}

func renderMarkdown(analysis *compliancev1.ComplianceAnalysis) string {
	var body strings.Builder
	window := analysis.GetEvidenceWindowCompleteness()
	fmt.Fprintln(&body, "# g8e Compliance Report")
	fmt.Fprintf(&body, "\n- Analysis: `%s`\n", markdownValue(analysis.GetAnalysisId()))
	fmt.Fprintf(&body, "- Scope: `%s`\n", markdownValue(analysis.GetScopeRef()))
	fmt.Fprintf(&body, "- Generated: `%s`\n", analysis.GetGeneratedAt().AsTime().UTC().Format(time.RFC3339))
	fmt.Fprintf(&body, "- Generator: `%s@%s`\n", markdownValue(analysis.GetGeneratorIdentity()), markdownValue(analysis.GetGeneratorVersion()))
	fmt.Fprintf(&body, "- Evidence graph: `%t`\n", analysis.GetEvidenceGraphValid())
	fmt.Fprintf(&body, "\n## Evidence Window\n\nStatus: `%s`; evidence: %d expected, %d actual; window: `%s` to `%s`.\n", markdownValue(window.GetCompletenessStatus()), window.GetExpectedEvidenceCount(), window.GetActualEvidenceCount(), markdownValue(window.GetWindowStartRef()), markdownValue(window.GetWindowEndRef()))
	fmt.Fprintln(&body, "\n## Assertion Assessments\n\n| Assertion | Version | Status | Evidence level | Freshness |\n| --- | --- | --- | --- | --- |")
	for _, assessment := range sortedAssertionAssessments(analysis) {
		fmt.Fprintf(&body, "| %s | %s | %s | %s | %s |\n", markdownValue(assessment.GetAssertionRef().GetId()), markdownValue(assessment.GetAssertionRef().GetVersion()), markdownValue(assessment.GetStatus()), markdownValue(assessment.GetEvidenceLevel()), markdownValue(assessment.GetFreshnessStatus()))
	}
	fmt.Fprintln(&body, "\n## Framework Controls\n\n| Framework | Control | Status | Responsibility | Evidence level |\n| --- | --- | --- | --- | --- |")
	for _, assessment := range sortedFrameworkAssessments(analysis) {
		fmt.Fprintf(&body, "| %s | %s | %s | %s | %s |\n", markdownValue(assessment.GetFrameworkRef().GetId()), markdownValue(assessment.GetControlId()), markdownValue(assessment.GetStatus()), markdownValue(assessment.GetResponsibility()), markdownValue(assessment.GetEvidenceLevel()))
	}
	renderMarkdownList(&body, "Gaps", gapDescriptions(analysis))
	renderMarkdownList(&body, "Findings", findingDescriptions(analysis))
	renderMarkdownList(&body, "Remediation", remediationDescriptions(analysis))
	renderMarkdownList(&body, "Limitations", analysis.GetLimitations())
	return body.String()
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
	body.WriteString("</p>\n<h2>Assertion Assessments</h2>\n<table><thead><tr><th>Assertion</th><th>Version</th><th>Status</th><th>Evidence level</th><th>Freshness</th></tr></thead><tbody>\n")
	for _, assessment := range sortedAssertionAssessments(analysis) {
		fmt.Fprintf(&body, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n", html.EscapeString(assessment.GetAssertionRef().GetId()), html.EscapeString(assessment.GetAssertionRef().GetVersion()), html.EscapeString(assessment.GetStatus()), html.EscapeString(assessment.GetEvidenceLevel()), html.EscapeString(assessment.GetFreshnessStatus()))
	}
	body.WriteString("</tbody></table>\n<h2>Framework Controls</h2>\n<table><thead><tr><th>Framework</th><th>Control</th><th>Status</th><th>Responsibility</th><th>Evidence level</th></tr></thead><tbody>\n")
	for _, assessment := range sortedFrameworkAssessments(analysis) {
		fmt.Fprintf(&body, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n", html.EscapeString(assessment.GetFrameworkRef().GetId()), html.EscapeString(assessment.GetControlId()), html.EscapeString(assessment.GetStatus()), html.EscapeString(assessment.GetResponsibility()), html.EscapeString(assessment.GetEvidenceLevel()))
	}
	body.WriteString("</tbody></table>\n")
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
	fmt.Fprintf(&body, "Assertion assessments: %d%s\n", len(analysis.GetAssertionAssessments()), formatStatusCounts(assertionStatusCounts(analysis)))
	fmt.Fprintf(&body, "Framework controls: %d%s\n", len(analysis.GetFrameworkAssessments()), formatStatusCounts(frameworkStatusCounts(analysis)))
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
