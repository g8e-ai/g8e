// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// complianceReleaseEvidenceCmdWithConfig creates the `compliance
// release-evidence` subcommand with injectable dependencies for testing. It
// runs KSI evaluation, reads KSI history, verifies every persisted demo run,
// and writes a per-release markdown report and CSV into the output directory.
// The output directory is a source-tree path (typically
// docs/release_notes/vX.Y.x/), not a .g8e/ runtime path, so output files are
// written with os.WriteFile. Runtime evidence (KSI state, KSI history, demo
// evidence) is read through the injected RuntimeFileService.
func complianceReleaseEvidenceCmdWithConfig(
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	provenanceSourceFactory func(string) evidence.ProvenanceSource,
) *cobra.Command {
	var (
		version     string
		outDir      string
		class       string
		catalogPath string
		demoRuns    []string
		projectRoot string
		failClosed  bool
	)

	cmd := &cobra.Command{
		Use:   "release-evidence",
		Short: "Generate per-release compliance evidence report and CSV",
		Long: `Run KSI evaluation, read KSI history snapshots, and independently verify
every persisted demo evidence run, then write a per-release markdown report and
CSV into the output directory. The report captures demonstrated technical
control operation at the release boundary; it does not claim certification or
legal compliance.

The output directory is a source-tree path (typically
docs/release_notes/vX.Y.x/). Two files are written:
  vX.Y.Z-compliance-evidence.md   readable markdown report
  vX.Y.Z-compliance-evidence.csv  one row per evidence item

Runtime evidence is read from the .g8e/ runtime tree via the runtime file
service. KSI evaluation requires live audit, ledger, and commitment stores;
when those are unavailable the report records the gap honestly rather than
inventing a passing result.

When --fail-closed is set, the command exits nonzero if any KSI is not
satisfied, KSI evaluation is unavailable, or any demo run is invalid.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			if err := validateReleaseVersion(version); err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory(projectRoot, slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			cat, err := loadKSICatalog(catalogPath)
			if err != nil {
				return err
			}

			certClass, err := validateCertClass(class)
			if err != nil {
				return err
			}

			source := provenanceSourceFactory(projectRoot)

			report, err := collectReleaseEvidence(ctx, fileSvc, cat, certClass, demoRuns, source)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrComplianceReleaseEvidence, err)
			}
			report.ReleaseVersion = version
			report.CertClass = string(certClass)
			report.CatalogPath = catalogPath

			if err := writeReleaseEvidenceArtifacts(report, outDir); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrComplianceReleaseEvidence, err)
			}

			mdPath := filepath.Join(outDir, version+constants.ReleaseEvidenceMarkdownSuffix)
			csvPath := filepath.Join(outDir, version+constants.ReleaseEvidenceCSVSuffix)
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", mdPath)
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", csvPath)

			if failClosed && !report.OverallPassing() {
				return fmt.Errorf("%w: %s", constants.ErrComplianceReleaseEvidence, report.FailClosedReason())
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&version, "version", "", "Release version (required, e.g. vX.Y.Z)")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory for the markdown report and CSV (required)")
	cmd.Flags().StringVar(&class, "class", "C", "FedRAMP 20x certification class (A, B, C, D)")
	cmd.Flags().StringVar(&catalogPath, "catalog", constants.DefaultKSICatalogPath, "Path to KSI catalog JSON file")
	cmd.Flags().StringSliceVar(&demoRuns, "demo-run", nil, "Demo run ID to verify (repeatable; defaults to all persisted runs)")
	cmd.Flags().StringVar(&projectRoot, "project-root", "", "Project root directory for demo provenance (defaults to cwd)")
	cmd.Flags().BoolVar(&failClosed, "fail-closed", false, "Exit nonzero if any KSI is not satisfied or any demo run is invalid")

	_ = cmd.MarkFlagRequired("version")
	_ = cmd.MarkFlagRequired("out")

	return cmd
}

var releaseVersionRegexp = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// validateReleaseVersion checks that the version flag is a non-empty string
// beginning with the 'v' prefix and matching the semantic versioning contract.
// It explicitly rejects path separators and parent directory references.
func validateReleaseVersion(v string) error {
	if v == "" {
		return fmt.Errorf("%w: --version is required", constants.ErrValidationFailed)
	}
	if !strings.HasPrefix(v, "v") {
		return fmt.Errorf("%w: --version must begin with 'v' (got %q)", constants.ErrValidationFailed, v)
	}
	if strings.ContainsAny(v, `/\`) || strings.Contains(v, "..") {
		return fmt.Errorf("%w: --version must not contain path separators or parent directory references", constants.ErrValidationFailed)
	}
	if !releaseVersionRegexp.MatchString(v) {
		return fmt.Errorf("%w: --version must be a valid semantic version starting with 'v' (e.g. v2.1.3): %q", constants.ErrValidationFailed, v)
	}
	return nil
}

// releaseEvidenceReport is the in-memory aggregation of all compliance
// evidence collected for a release. It is the single source both renderers
// (markdown and CSV) consume; neither renderer recomputes an assessment.
type releaseEvidenceReport struct {
	ReleaseVersion string
	GeneratedAt    time.Time
	CertClass      string
	CatalogPath    string

	// KSI evaluation. KSISet is nil when the runtime stores are unavailable;
	// KSIUnavailable records the reason so the report can surface the gap.
	KSISet         *compliance.KSIResultSet
	KSIUnavailable string

	// KSI history snapshots (chronological, oldest first).
	KSIHistory []compliance.KSIResultSet

	// KSIHistoryErr records any error reading history snapshots.
	KSIHistoryErr error

	// Demo-run verification reports, one per run ID, in input order.
	DemoReports []*demoRunSummary

	// DemoEnumerationErr records any error enumerating persisted demo runs.
	DemoEnumerationErr error
}

// demoRunSummary pairs a run ID with its verification report and any error
// encountered while verifying. A nil report with a non-nil error records a
// verifier failure that prevented producing a report.
type demoRunSummary struct {
	RunID  string
	Report *compliancev1.ComplianceVerificationReport
	Err    error
}

// OverallPassing returns true when every collected evidence item is in a
// passing state: KSI evaluation is available and every KSI is satisfied or
// not applicable, and every demo run verification report is valid. An empty
// report (no KSI set, no demo runs) is not passing.
func (r *releaseEvidenceReport) OverallPassing() bool {
	if r.KSISet == nil || r.KSIHistoryErr != nil || r.DemoEnumerationErr != nil {
		return false
	}
	if len(r.KSISet.Results) == 0 && len(r.DemoReports) == 0 {
		return false
	}
	for _, ksi := range r.KSISet.Results {
		if ksi.Status != compliance.KSIStatusSatisfied && ksi.Status != compliance.KSIStatusNotApplicable {
			return false
		}
	}
	for _, dr := range r.DemoReports {
		if dr.Report == nil || !dr.Report.GetValid() {
			return false
		}
	}
	return true
}

// FailClosedReason returns a human-readable summary of the first non-passing
// condition, for use in --fail-closed error messages.
func (r *releaseEvidenceReport) FailClosedReason() string {
	if r.DemoEnumerationErr != nil {
		return fmt.Sprintf("failed to enumerate demo runs: %v", r.DemoEnumerationErr)
	}
	if r.KSIHistoryErr != nil {
		return fmt.Sprintf("failed to read KSI history: %v", r.KSIHistoryErr)
	}
	if r.KSISet == nil {
		if r.KSIUnavailable != "" {
			return "KSI evaluation unavailable: " + r.KSIUnavailable
		}
		return "KSI evaluation unavailable"
	}
	for _, ksi := range r.KSISet.Results {
		switch ksi.Status {
		case compliance.KSIStatusSatisfied, compliance.KSIStatusNotApplicable:
			// passing
		case compliance.KSIStatusNotSatisfied:
			return fmt.Sprintf("KSI %s is not satisfied", ksi.ID)
		default:
			return fmt.Sprintf("KSI %s has non-passing status: %s", ksi.ID, ksi.Status)
		}
	}
	for _, dr := range r.DemoReports {
		if dr.Report == nil {
			return fmt.Sprintf("demo run %s verification failed: %v", dr.RunID, dr.Err)
		}
		if !dr.Report.GetValid() {
			return fmt.Sprintf("demo run %s is invalid (%d failure(s))", dr.RunID, len(dr.Report.GetFailures()))
		}
	}
	if len(r.KSISet.Results) == 0 && len(r.DemoReports) == 0 {
		return "no evidence collected: KSI results and demo runs are both empty"
	}
	return "no evidence collected"
}

// collectReleaseEvidence gathers KSI evaluation, KSI history, and demo-run
// verification into a single releaseEvidenceReport. KSI evaluation failure
// (nil result) is recorded as an unavailable gap rather than aborting the
// report, so the release record honestly reflects the evidence state.
// This function performs read-only aggregation: snapshots are not persisted here.
func collectReleaseEvidence(
	ctx context.Context,
	fileSvc fs.RuntimeFileService,
	cat *compliance.KSICatalog,
	class compliance.CertificationClass,
	runIDs []string,
	source evidence.ProvenanceSource,
) (*releaseEvidenceReport, error) {
	report := &releaseEvidenceReport{GeneratedAt: time.Now().UTC()}

	// KSI evaluation. A nil result means the runtime stores could not be
	// opened; record the gap and continue so the report still captures
	// history and demo evidence.
	ksiSet := evaluateKSIs(ctx, fileSvc, cat, class)
	if ksiSet == nil {
		report.KSIUnavailable = "audit store, ledger, or commitment store unavailable"
	} else {
		report.KSISet = ksiSet
	}

	// KSI history snapshots (read-only aggregation).
	historyStore := newKSIHistoryStore(fileSvc)
	snapshots, err := historyStore.ListSnapshots(ctx)
	if err != nil {
		if !errors.Is(err, constants.ErrNotFound) {
			slog.Default().Warn("compliance: failed to read KSI history", "error", err)
			report.KSIHistoryErr = err
		}
	}
	report.KSIHistory = snapshots

	// Demo-run verification. Enumerate persisted runs when no explicit IDs
	// are supplied.
	effectiveRunIDs := runIDs
	if len(effectiveRunIDs) == 0 {
		var enumErr error
		effectiveRunIDs, enumErr = enumerateDemoRunIDs(ctx, fileSvc)
		if enumErr != nil {
			slog.Default().Warn("compliance: failed to enumerate demo runs", "error", enumErr)
			report.DemoEnumerationErr = enumErr
		}
	} else {
		sort.Strings(effectiveRunIDs)
	}

	for _, runID := range effectiveRunIDs {
		verReport, verifyErr := evidence.VerifyDemoRun(ctx, fileSvc, runID, source, time.Now().UTC())
		// VerifyDemoRun returns a non-nil report even on verification
		// failures (it records failures inside the report). A non-nil
		// error means the verifier could not produce a report at all.
		summary := &demoRunSummary{RunID: runID, Report: verReport, Err: verifyErr}
		report.DemoReports = append(report.DemoReports, summary)
	}

	return report, nil
}

// enumerateDemoRunIDs lists the persisted demo evidence run directories under
// the runtime compliance tree. Each subdirectory of
// data/compliance/demo-evidence/ is treated as a run ID.
func enumerateDemoRunIDs(ctx context.Context, fileSvc fs.RuntimeFileService) ([]string, error) {
	demoDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname)
	entries, err := fileSvc.ReadDir(ctx, demoDir)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return nil, nil // missing dir is not an error: no runs persisted
		}
		return nil, fmt.Errorf("compliance: enumerate demo runs: %w", err)
	}
	var runIDs []string
	for _, entry := range entries {
		if entry.IsDir() {
			runIDs = append(runIDs, entry.Name())
		}
	}
	sort.Strings(runIDs)
	return runIDs, nil
}

// writeReleaseEvidenceArtifacts renders the markdown report and CSV and writes
// them into outDir, creating the directory if needed.
func writeReleaseEvidenceArtifacts(report *releaseEvidenceReport, outDir string) error {
	if outDir == "" {
		return fmt.Errorf("%w: --out is required", constants.ErrValidationFailed)
	}
	cleanOutDir := filepath.Clean(outDir)
	if err := os.MkdirAll(cleanOutDir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrReportOutputDirFailed, err)
	}

	md := renderReleaseEvidenceMarkdown(report)
	mdPath := filepath.Join(cleanOutDir, report.ReleaseVersion+constants.ReleaseEvidenceMarkdownSuffix)
	relMD, err := filepath.Rel(cleanOutDir, mdPath)
	if err != nil || strings.HasPrefix(relMD, "..") || filepath.IsAbs(relMD) {
		return fmt.Errorf("%w: markdown output path escapes output directory", constants.ErrPathValidation)
	}
	if err := os.WriteFile(mdPath, []byte(md), constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrReportWriteFailed, err)
	}

	csvBytes, err := renderReleaseEvidenceCSV(report)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrComplianceReleaseEvidence, err)
	}
	csvPath := filepath.Join(cleanOutDir, report.ReleaseVersion+constants.ReleaseEvidenceCSVSuffix)
	relCSV, err := filepath.Rel(cleanOutDir, csvPath)
	if err != nil || strings.HasPrefix(relCSV, "..") || filepath.IsAbs(relCSV) {
		return fmt.Errorf("%w: csv output path escapes output directory", constants.ErrPathValidation)
	}
	if err := os.WriteFile(csvPath, csvBytes, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrReportWriteFailed, err)
	}
	return nil
}

// renderReleaseEvidenceMarkdown produces the readable markdown report from the
// collected evidence. It projects the releaseEvidenceReport into prose and
// tables without recomputing any assessment.
func renderReleaseEvidenceMarkdown(report *releaseEvidenceReport) string {
	var b strings.Builder
	genAt := report.GeneratedAt.UTC().Format(time.RFC3339)

	fmt.Fprintf(&b, "# Compliance Release Evidence\n\n")
	fmt.Fprintf(&b, "**Release Version:** %s\n", report.ReleaseVersion)
	fmt.Fprintf(&b, "**Generated At:** %s\n", genAt)
	fmt.Fprintf(&b, "**Platform:** g8e %s\n", report.ReleaseVersion)
	fmt.Fprintf(&b, "**Certification Class:** %s\n", report.CertClass)
	fmt.Fprintf(&b, "**KSI Catalog:** %s\n", report.CatalogPath)
	fmt.Fprintf(&b, "\n---\n\n")

	// Summary
	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "| Metric | Value |\n")
	fmt.Fprintf(&b, "|--------|-------|\n")
	ksiStatus := "unavailable"
	ksiSatisfied, ksiNotSatisfied, ksiNotApplicable := 0, 0, 0
	ksiTotal := 0
	if report.KSISet != nil {
		ksiStatus = "available"
		ksiTotal = len(report.KSISet.Results)
		for _, r := range report.KSISet.Results {
			switch r.Status {
			case compliance.KSIStatusSatisfied:
				ksiSatisfied++
			case compliance.KSIStatusNotSatisfied:
				ksiNotSatisfied++
			case compliance.KSIStatusNotApplicable:
				ksiNotApplicable++
			}
		}
	}
	fmt.Fprintf(&b, "| KSI evaluation | %s |\n", ksiStatus)
	fmt.Fprintf(&b, "| KSIs satisfied | %d / %d |\n", ksiSatisfied, ksiTotal)
	fmt.Fprintf(&b, "| KSIs not satisfied | %d |\n", ksiNotSatisfied)
	fmt.Fprintf(&b, "| KSIs not applicable | %d |\n", ksiNotApplicable)
	fmt.Fprintf(&b, "| KSI history snapshots | %s |\n", historySnapshotSummary(report.KSIHistory))
	validRuns, invalidRuns := 0, 0
	for _, dr := range report.DemoReports {
		if dr.Report != nil && dr.Report.GetValid() {
			validRuns++
		} else {
			invalidRuns++
		}
	}
	fmt.Fprintf(&b, "| Demo runs verified | %d |\n", len(report.DemoReports))
	fmt.Fprintf(&b, "| Demo runs valid | %d |\n", validRuns)
	fmt.Fprintf(&b, "| Demo runs invalid | %d |\n", invalidRuns)
	fmt.Fprintf(&b, "\n")

	// KSI evaluation
	fmt.Fprintf(&b, "## KSI Evaluation\n\n")
	if report.KSISet == nil {
		reason := report.KSIUnavailable
		if reason == "" {
			reason = "runtime stores unavailable"
		}
		fmt.Fprintf(&b, "_KSI evaluation unavailable: %s._\n\n", reason)
	} else {
		evalAt := msToRFC3339(report.KSISet.EvaluatedAtMs)
		fmt.Fprintf(&b, "Evaluated at %s for class %s.\n\n", evalAt, report.KSISet.Class)
		fmt.Fprintf(&b, "| KSI ID | Status | Methods | Last Validated |\n")
		fmt.Fprintf(&b, "|--------|--------|---------|----------------|\n")
		// Stable order by KSI ID.
		results := append([]compliance.KSIResult(nil), report.KSISet.Results...)
		sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
		for _, r := range results {
			fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", r.ID, r.Status, r.MethodCount, msToRFC3339OrEmpty(r.LastValidatedUnixMs))
		}
		fmt.Fprintf(&b, "\n")
	}

	// KSI history
	fmt.Fprintf(&b, "## KSI History\n\n")
	if len(report.KSIHistory) == 0 {
		fmt.Fprintf(&b, "_No KSI history snapshots persisted._\n\n")
	} else {
		earliest := msToRFC3339(report.KSIHistory[0].EvaluatedAtMs)
		latest := msToRFC3339(report.KSIHistory[len(report.KSIHistory)-1].EvaluatedAtMs)
		fmt.Fprintf(&b, "%d snapshot(s) persisted from %s to %s.\n\n", len(report.KSIHistory), earliest, latest)
	}

	// Demo-run verification
	fmt.Fprintf(&b, "## Demo-Run Verification\n\n")
	if len(report.DemoReports) == 0 {
		fmt.Fprintf(&b, "_No demo runs persisted._\n\n")
	} else {
		fmt.Fprintf(&b, "| Run ID | Valid | Failures | Verifier | Version | Checksum Root | Verified At |\n")
		fmt.Fprintf(&b, "|--------|-------|----------|----------|---------|---------------|-------------|\n")
		for _, dr := range report.DemoReports {
			if dr.Report == nil {
				errMsg := "no report"
				if dr.Err != nil {
					errMsg = dr.Err.Error()
				}
				fmt.Fprintf(&b, "| %s | error | - | - | - | - | %s |\n", dr.RunID, errMsg)
				continue
			}
			r := dr.Report
			verifiedAt := ""
			if r.GetVerifiedAt() != nil {
				verifiedAt = r.GetVerifiedAt().AsTime().UTC().Format(time.RFC3339)
			}
			fmt.Fprintf(&b, "| %s | %t | %d | %s | %s | %s | %s |\n",
				dr.RunID, r.GetValid(), len(r.GetFailures()), r.GetVerifierId(), r.GetVerifierVersion(), r.GetReproducedChecksumRoot(), verifiedAt)
		}
		fmt.Fprintf(&b, "\n")
	}

	// Claim boundaries
	fmt.Fprintf(&b, "## Claim Boundaries\n\n")
	fmt.Fprintf(&b, "This report captures demonstrated technical control operation at the %s release boundary. It does not claim certification, accreditation, authorization, or legal compliance. KSI evaluation reflects live runtime state at generation time; demo-run verification reflects persisted evidence for the listed run IDs. See the [Compliance Alignment Report](../../reference/compliance-alignment.md) for framework control mappings and the [Proof-Backed Compliance Evidence](../../reference/compliance-evidence.md) document for the evidence pipeline, assertion catalog, and roadmap.\n", report.ReleaseVersion)
	fmt.Fprintf(&b, "\n---\n\n")
	fmt.Fprintf(&b, "*Generated by `g8e compliance release-evidence`.*\n")

	return b.String()
}

// renderReleaseEvidenceCSV produces a CSV with one row per evidence item
// (KSI results and demo-run verifications). The header row documents the
// columns; empty cells indicate non-applicable fields for that evidence type.
func renderReleaseEvidenceCSV(report *releaseEvidenceReport) ([]byte, error) {
	var sb strings.Builder
	w := csv.NewWriter(&sb)

	if err := w.Write([]string{"evidence_type", "identifier", "status", "valid", "method_count", "last_validated", "failure_count", "verifier_id", "verifier_version", "checksum_root", "evaluated_at"}); err != nil {
		return nil, fmt.Errorf("compliance: write csv header: %w", err)
	}

	if report.KSISet != nil {
		evalAt := msToRFC3339(report.KSISet.EvaluatedAtMs)
		results := append([]compliance.KSIResult(nil), report.KSISet.Results...)
		sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
		for _, r := range results {
			if err := w.Write([]string{
				"ksi", r.ID, string(r.Status), "", strconv.Itoa(r.MethodCount),
				msToRFC3339OrEmpty(r.LastValidatedUnixMs), "", "", "", "", evalAt,
			}); err != nil {
				return nil, fmt.Errorf("compliance: write ksi csv row: %w", err)
			}
		}
	} else {
		if err := w.Write([]string{"ksi", "", "unavailable", "", "", "", "", "", "", "", report.GeneratedAt.UTC().Format(time.RFC3339)}); err != nil {
			return nil, fmt.Errorf("compliance: write ksi unavailable csv row: %w", err)
		}
	}

	for _, dr := range report.DemoReports {
		if dr.Report == nil {
			errMsg := "no report"
			if dr.Err != nil {
				errMsg = dr.Err.Error()
			}
			if err := w.Write([]string{"demo-run", dr.RunID, "error", "", "", "", "", "", "", "", errMsg}); err != nil {
				return nil, fmt.Errorf("compliance: write demo run error csv row: %w", err)
			}
			continue
		}
		r := dr.Report
		verifiedAt := ""
		if r.GetVerifiedAt() != nil {
			verifiedAt = r.GetVerifiedAt().AsTime().UTC().Format(time.RFC3339)
		}
		valid := "false"
		if r.GetValid() {
			valid = "true"
		}
		if err := w.Write([]string{
			"demo-run", dr.RunID, "", valid, "", "", strconv.Itoa(len(r.GetFailures())),
			r.GetVerifierId(), r.GetVerifierVersion(), r.GetReproducedChecksumRoot(), verifiedAt,
		}); err != nil {
			return nil, fmt.Errorf("compliance: write demo run csv row: %w", err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("compliance: flush csv: %w", err)
	}
	return []byte(sb.String()), nil
}

// historySnapshotSummary renders a compact description of the snapshot series.
func historySnapshotSummary(snapshots []compliance.KSIResultSet) string {
	if len(snapshots) == 0 {
		return "0"
	}
	earliest := msToRFC3339(snapshots[0].EvaluatedAtMs)
	latest := msToRFC3339(snapshots[len(snapshots)-1].EvaluatedAtMs)
	return fmt.Sprintf("%d (%s to %s)", len(snapshots), earliest, latest)
}

// msToRFC3339 converts a Unix millisecond timestamp to an RFC 3339 string.
func msToRFC3339(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// msToRFC3339OrEmpty returns an empty string for zero timestamps.
func msToRFC3339OrEmpty(ms int64) string {
	if ms == 0 {
		return ""
	}
	return msToRFC3339(ms)
}
