// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gwremote"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type campaignVerificationPublication interface {
	PublishRunCompletion(context.Context, string, time.Time) (int, error)
	PublishRunVerification(context.Context, string, *evalv1.EvaluationVerificationReport) (int, error)
}

type campaignVerificationPublicationFactory func(*cobra.Command, fs.RuntimeFileService) (campaignVerificationPublication, error)

type runVerifyJSON struct {
	RunID          string   `json:"run_id"`
	Status         string   `json:"status"`
	FailureCount   uint32   `json:"failure_count"`
	FailureReasons []string `json:"failure_reasons"`
}

type runCoverageJSON struct {
	RunID                 string            `json:"run_id"`
	Complete              bool              `json:"complete"`
	ExpectedCells         uint64            `json:"expected_cells"`
	ScheduledAssignments  uint32            `json:"scheduled_assignments"`
	Queued                uint32            `json:"queued"`
	Running               uint32            `json:"running"`
	Terminal              uint32            `json:"terminal"`
	Stopped               uint32            `json:"stopped"`
	DispositionCounts     map[string]uint32 `json:"disposition_counts"`
	MissingCells          []string          `json:"missing_cells"`
	DuplicateIdentities   []string          `json:"duplicate_identities"`
	ExtraAssignments      []string          `json:"extra_assignments"`
	TerminalWithoutResult []string          `json:"terminal_without_result"`
	ResultWithoutTerminal []string          `json:"result_without_terminal"`
	FailureReasons        []string          `json:"failure_reasons"`
	AccountedAt           string            `json:"accounted_at"`
}

func runsVerifyCmd(deps nativeEvalDeps) *cobra.Command {
	var coverage, requireProvenance, requireObservation bool
	cmd := &cobra.Command{
		Use:   "verify <run>",
		Short: "Independently verify a run's persisted assignment results",
		Long: `Verify a run's persisted results against its frozen campaign. Verification
works on archived runs too.

With --coverage, check only that the run's assignment matrix is fully populated
and accounted for, without full verification.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := args[0]
			if coverage && (requireProvenance || requireObservation) {
				return fmt.Errorf("evaluation: runs verify: --coverage cannot be combined with --require-provenance or --require-observation")
			}
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			store, archived, err := evaluation.LocateRun(cmd.Context(), fileSvc, runID)
			if err != nil {
				return fmt.Errorf("evaluation: runs verify: %w", err)
			}
			if coverage {
				return verifyRunCoverage(cmd, deps, store, runID)
			}
			report, err := verifyRun(cmd, deps, store, runID, requireObservation, requireProvenance, !archived, output.JSONEnabled(cmd))
			if err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), runVerifyJSON{
					RunID:          runID,
					Status:         report.GetStatus().String(),
					FailureCount:   report.GetFailureCount(),
					FailureReasons: report.GetFailureReasons(),
				})
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Run %s verification: %s (%d failure(s))\n", runID, report.GetStatus().String(), report.GetFailureCount())
			if report.GetFailureCount() > 0 {
				for _, reason := range report.GetFailureReasons() {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", reason)
				}
				return constants.ErrEvalRunVerificationFailed
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&coverage, "coverage", false, "Verify only that the assignment matrix is fully populated")
	cmd.Flags().BoolVar(&requireProvenance, "require-provenance", false, "Fail when model provenance attestation windows are missing or digest_match is false")
	cmd.Flags().BoolVar(&requireObservation, "require-observation", false, "Fail when provider-boundary observation windows are missing or incomplete")
	return cmd
}

func verifyRunCoverage(cmd *cobra.Command, deps nativeEvalDeps, store *evaluation.Store, runID string) error {
	run, err := store.LoadRun(cmd.Context(), runID)
	if err != nil {
		return fmt.Errorf("evaluation: runs verify: %w", err)
	}
	catalog, err := store.LoadScenarioCatalog(cmd.Context(), run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return fmt.Errorf("evaluation: runs verify: %w", err)
	}
	report, err := evaluation.NewCampaignPopulationAccountant(deps.now).AccountRun(cmd.Context(), store, runID, catalog)
	if err != nil {
		return fmt.Errorf("evaluation: runs verify: %w", err)
	}
	if output.JSONEnabled(cmd) {
		if err := output.WriteJSON(cmd.OutOrStdout(), runCoverageJSON{
			RunID:                 runID,
			Complete:              report.Complete,
			ExpectedCells:         report.ExpectedCells,
			ScheduledAssignments:  report.ScheduledAssignments,
			Queued:                report.QueuedCount,
			Running:               report.RunningCount,
			Terminal:              report.TerminalCount,
			Stopped:               report.StoppedCount,
			DispositionCounts:     report.DispositionCounts,
			MissingCells:          report.MissingCells,
			DuplicateIdentities:   report.DuplicateIdentities,
			ExtraAssignments:      report.ExtraAssignments,
			TerminalWithoutResult: report.TerminalWithoutResult,
			ResultWithoutTerminal: report.ResultWithoutTerminal,
			FailureReasons:        report.FailureReasons,
			AccountedAt:           report.AccountedAt.Format(time.RFC3339),
		}); err != nil {
			return err
		}
		if !report.Complete {
			return constants.ErrEvalRunVerificationFailed
		}
		return nil
	}
	state := "incomplete"
	if report.Complete {
		state = "complete"
	}
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "Run %s population accounting: %s\nExpected cells: %d\nScheduled: %d\nQueued: %d\nRunning: %d\nTerminal: %d\nStopped: %d\n",
		runID, state, report.ExpectedCells, report.ScheduledAssignments, report.QueuedCount, report.RunningCount, report.TerminalCount, report.StoppedCount)
	for _, reason := range report.FailureReasons {
		_, _ = fmt.Fprintf(out, "- %s\n", reason)
	}
	if !report.Complete {
		return constants.ErrEvalRunVerificationFailed
	}
	return nil
}

// verifyRun verifies one run's persisted results, saves the verification
// report, and, when publish is set, publishes the completion and passing
// verification projections. A publication failure is reported as a warning: it
// never invalidates a verification that already succeeded.
func verifyRun(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	store *evaluation.Store,
	runID string,
	requireObservation bool,
	requireProvenance bool,
	publish bool,
	jsonOutput bool,
) (*evalv1.EvaluationVerificationReport, error) {
	cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return nil, err
	}
	run, err := store.LoadRun(cmd.Context(), runID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs verify: %w", err)
	}
	catalog, err := store.LoadScenarioCatalog(cmd.Context(), run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs verify: %w", err)
	}
	_, artifacts, err := evaluation.LoadScenarioCatalog()
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs verify: %w", err)
	}
	verifier := evaluation.NewCampaignRunVerifier(deps.now)
	observationReader, err := gwremote.NewCampaignProviderObservationReader(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs verify: %w", err)
	}
	providerPolicy := evaluation.ProviderObservationPolicyInterim
	if requireObservation {
		providerPolicy = evaluation.ProviderObservationPolicyStrict
	}
	verifier = verifier.WithProviderObservationReader(observationReader, providerPolicy)
	provenanceReader, err := gwremote.NewCampaignModelProvenanceReader(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs verify: %w", err)
	}
	provenancePolicy := evaluation.ModelProvenancePolicyInterim
	if requireProvenance {
		provenancePolicy = evaluation.ModelProvenancePolicyStrict
	}
	verifier = verifier.WithModelProvenanceReader(provenanceReader, provenancePolicy)
	if err := evaluation.CaptureCampaignRunWitnessEvidence(cmd.Context(), store, runID, observationReader, provenanceReader); err != nil {
		return nil, fmt.Errorf("evaluation: runs verify: %w", err)
	}
	report, err := verifier.VerifyRun(cmd.Context(), store, runID, catalog, artifacts)
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs verify: %w", err)
	}
	report, _, err = evaluation.BindStoredCampaignVerificationReport(cmd.Context(), store, report, evaluation.CampaignVerificationPolicy{
		VerifierReleaseVersion: constants.EvaluationSourceVersion,
		ProviderObservation:    providerPolicy,
		ModelProvenance:        provenancePolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluation: runs verify: %w", err)
	}
	if err := store.SaveCampaignVerification(cmd.Context(), runID, report); err != nil {
		return nil, fmt.Errorf("evaluation: runs verify: %w", err)
	}
	if !publish {
		return report, nil
	}
	publicationFactory := deps.campaignPublicationFactory
	if publicationFactory == nil {
		publicationFactory = func(cmd *cobra.Command, fileSvc fs.RuntimeFileService) (campaignVerificationPublication, error) {
			return NewCampaignPublicationCoordinator(cmd, fileSvc)
		}
	}
	publication, pubErr := publicationFactory(cmd, fileSvc)
	if pubErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: run verify publication unavailable: %v\n", pubErr)
		return report, nil
	}
	completionCount, pubErr := publication.PublishRunCompletion(cmd.Context(), runID, deps.now().UTC())
	if pubErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: run verify completion publication: %v\n", pubErr)
	} else if completionCount > 0 && !jsonOutput {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Published %d completion projection record(s) to public mirror\n", completionCount)
	}
	if report.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
		published, pubErr := publication.PublishRunVerification(cmd.Context(), runID, report)
		if pubErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: run verify publication: %v\n", pubErr)
		} else if published > 0 && !jsonOutput {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Published %d verification projection record(s) to public mirror\n", published)
		}
	}
	return report, nil
}
