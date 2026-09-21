// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type nativeEvalClientFactory func(harnessconfig.Config) (*harnessclient.Client, error)
type nativeEvalAuthLoader func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error)

type nativeEvalRunner interface {
	Run(context.Context, evaluation.RunRequest) (*evalv1.EvaluationReport, error)
}

type nativeEvalVerifier interface {
	Verify(context.Context, string) (*compliancev1.ComplianceVerificationReport, error)
}

type nativeEvalStore interface {
	evaluation.ReportStore
	SaveVerification(context.Context, string, *compliancev1.ComplianceVerificationReport) (*compliancev1.ComplianceEvidenceReference, error)
	LoadReport(context.Context, string) (*evalv1.EvaluationReport, error)
}

type nativeEvalDeps struct {
	configLoader               func(string) (*config.Config, error)
	fileSvcFactory             func(string, *slog.Logger) (fs.RuntimeFileService, error)
	createRuntimeTree          func(context.Context, fs.RuntimeFileService) error
	clientFactory              nativeEvalClientFactory
	authLoader                 nativeEvalAuthLoader
	laneFactory                func(*harnessclient.Client, fs.RuntimeFileService, harnessclient.Persona) evaluation.PlatformLane
	observerFactory            func(string) evaluation.TargetObserver
	runnerFactory              func(evaluation.PlatformLane, evaluation.TargetObserver, evaluation.ReportStore, func() time.Time, func(string) string) nativeEvalRunner
	storeFactory               func(fs.RuntimeFileService) nativeEvalStore
	verifierFactory            func(fs.RuntimeFileService, func() time.Time) nativeEvalVerifier
	campaignPublicationFactory campaignVerificationPublicationFactory
	now                        func() time.Time
	newID                      func() string
}

func evalCmd() *cobra.Command {
	return evalCmdWithConfig(nativeEvalDeps{
		configLoader:      config.Load,
		fileSvcFactory:    newFileSvc,
		createRuntimeTree: func(ctx context.Context, fileSvc fs.RuntimeFileService) error { return fileSvc.CreateRuntimeTree(ctx) },
		clientFactory:     harnessclient.New,
		authLoader:        auth.LoadClientAuthContext,
		laneFactory: func(client *harnessclient.Client, fileSvc fs.RuntimeFileService, persona harnessclient.Persona) evaluation.PlatformLane {
			return evaluation.NewCommandLane(client, evaluation.NewStore(fileSvc), persona, 0, 0)
		},
		observerFactory: evaluation.NewComposeTargetObserver,
		runnerFactory: func(lane evaluation.PlatformLane, observer evaluation.TargetObserver, store evaluation.ReportStore, now func() time.Time, newID func(string) string) nativeEvalRunner {
			return evaluation.NewRunner(evaluation.NewRegistry(), lane, observer, store, now, newID)
		},
		storeFactory: func(fileSvc fs.RuntimeFileService) nativeEvalStore { return evaluation.NewStore(fileSvc) },
		verifierFactory: func(fileSvc fs.RuntimeFileService, now func() time.Time) nativeEvalVerifier {
			return evaluation.NewVerifier(fileSvc, evaluation.NewRegistry(), now)
		},
		campaignPublicationFactory: func(cmd *cobra.Command, fileSvc fs.RuntimeFileService) (campaignVerificationPublication, error) {
			return newCampaignPublicationCoordinator(cmd, fileSvc)
		},
		now:   time.Now,
		newID: uuid.NewString,
	})
}

func evalCmdWithConfig(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "eval",
		Aliases: []string{"evals"},
		Short:   "Run and verify g8e evaluation programs",
		Long: `Platform evaluation programs and their supporting workflows.

  boundary   Native execution-boundary suite (no models)
  campaign   Model scoring through production chat/inference
  models     Provider inventory freeze and materialize
  rollout    Per-model init qualification queue
  gate       Pre-campaign acceptance gates
  dev        Local development utilities`,
	}
	cmd.PersistentFlags().String("project-root", "", "Override the repository root (defaults to cwd)")
	cmd.AddCommand(
		boundaryEvalCmd(deps),
		campaignEvalCmd(deps),
		modelsEvalCmd(deps),
		rolloutEvalCmd(deps),
		gateEvalCmd(deps),
		devEvalCmd(deps),
	)
	return cmd
}

func boundaryEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "boundary",
		Short: "Native execution-boundary suite (no models)",
	}
	cmd.AddCommand(
		boundaryEvalRunCmd(deps),
		boundaryEvalVerifyCmd(deps),
		boundaryEvalShowCmd(deps),
	)
	return cmd
}

func gateEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "Pre-campaign acceptance gates",
	}
	cmd.AddCommand(
		gateInferenceEvalCmd(deps),
		gateChatEvalCmd(deps),
	)
	return cmd
}

func devEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Local development utilities",
	}
	cmd.AddCommand(providerObserverEvalCmd(deps))
	return cmd
}

func boundaryEvalRunCmd(deps nativeEvalDeps) *cobra.Command {
	var operatorSessionID string
	command := &cobra.Command{
		Use:   "run",
		Short: "Run core-execution-boundary@1.0.0",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			authContext, err := deps.authLoader(fileSvc, cfg)
			if err != nil {
				return fmt.Errorf("evaluation: load CLI identity: %w", err)
			}
			clientConfig := nativeEvalClientConfig(cfg, authContext)
			gatewayClient, err := deps.clientFactory(clientConfig)
			if err != nil {
				return fmt.Errorf("evaluation: initialize gateway client: %w", err)
			}
			runID := deps.newID()
			target := filepath.Join(constants.EvaluationTargetContainerDir, constants.EvaluationTargetFilenamePrefix+runID+constants.FileExtText)
			marker := constants.EvaluationTargetFilenamePrefix + runID
			store := deps.storeFactory(fileSvc)
			lane := deps.laneFactory(gatewayClient, fileSvc, harnessclient.Persona{ID: "g8e-native-evaluator", CLISessionID: authContext.CLISessionID, UserID: authContext.UserID})
			runner := deps.runnerFactory(lane, deps.observerFactory(cfg.ProjectRoot), store, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
			report, runErr := runner.Run(cmd.Context(), evaluation.RunRequest{RunID: runID, PinnedOperatorSessionID: operatorSessionID, TargetResource: target, Marker: marker, Deployment: nativeEvalDeployment(cfg, authContext, runID, target)})
			if report == nil {
				return runErr
			}
			verification, verifyErr := deps.verifierFactory(fileSvc, deps.now).Verify(cmd.Context(), runID)
			if verifyErr != nil {
				return fmt.Errorf("evaluation: verify persisted run: %w", verifyErr)
			}
			verificationRef, saveErr := store.SaveVerification(cmd.Context(), runID, verification)
			if saveErr != nil {
				return saveErr
			}
			report.Run.FinalVerificationReportRef = verificationRef
			if saveErr := store.SaveReport(cmd.Context(), report); saveErr != nil {
				return saveErr
			}
			if err := writeNativeEvalRun(cmd, report, verification, output.JSONEnabled(cmd)); err != nil {
				return err
			}
			if runErr != nil {
				return runErr
			}
			if !verification.GetValid() {
				return constants.ErrEvalRunVerificationFailed
			}
			return nil
		},
	}
	command.Flags().StringVar(&operatorSessionID, "operator-session", "", "Pin the evaluation to one exact active remote Operator session")
	return command
}

func boundaryEvalVerifyCmd(deps nativeEvalDeps) *cobra.Command {
	command := &cobra.Command{
		Use:   "verify <run-id>",
		Short: "Re-verify persisted execution-boundary evidence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			report, err := deps.verifierFactory(fileSvc, deps.now).Verify(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := writeNativeVerification(cmd, report, output.JSONEnabled(cmd)); err != nil {
				return err
			}
			if !report.GetValid() {
				return constants.ErrEvalRunVerificationFailed
			}
			return nil
		},
	}
	return command
}

func boundaryEvalShowCmd(deps nativeEvalDeps) *cobra.Command {
	command := &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show a persisted execution-boundary report",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			report, err := deps.storeFactory(fileSvc).LoadReport(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				body, err := evalv1.MarshalCanonical(report)
				if err != nil {
					return fmt.Errorf("evaluation: canonicalize report: %w", err)
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Run: %s\nSuite: %s@%s\nStatus: %s\nSummary: %s\nOperator: %s\nSession: %s\n", report.GetRun().GetRunId(), report.GetRun().GetSuiteRef().GetId(), report.GetRun().GetSuiteRef().GetVersion(), report.GetSummaryStatus().String(), report.GetSummary(), report.GetRun().GetTargetOperatorId(), report.GetRun().GetTargetOperatorSessionId())
			if err != nil {
				return err
			}
			for _, verdict := range report.GetVerdicts() {
				if verdict.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
					continue
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- %s: %s (%s)\n", verdict.GetAssertionRef().GetId(), verdict.GetStatus().String(), verdict.GetFailureReason()); err != nil {
					return err
				}
			}
			return nil
		},
	}
	return command
}

func nativeEvalEnvironment(cmd *cobra.Command, deps nativeEvalDeps) (*config.Config, fs.RuntimeFileService, error) {
	projectRoot, err := cmd.Flags().GetString("project-root")
	if err != nil {
		return nil, nil, fmt.Errorf("evaluation: read project root: %w", err)
	}
	cfg, err := deps.configLoader(projectRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("evaluation: load config: %w", err)
	}
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, slog.Default())
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	if err := deps.createRuntimeTree(cmd.Context(), fileSvc); err != nil {
		return nil, nil, fmt.Errorf("evaluation: create runtime tree: %w", err)
	}
	return cfg, fileSvc, nil
}

func nativeEvalClientConfig(cfg *config.Config, authContext *auth.ClientAuthContext) harnessconfig.Config {
	credentials := harnessconfig.Auth{ClientCert: authContext.ClientCert, ClientKey: authContext.ClientKey, CABundle: cfg.ResolvedTrustBundlePath()}
	return harnessconfig.Config{MTLSBaseURL: cfg.OperatorHTTPURL(), PublicBaseURL: cfg.OperatorDiscoveryURL(), Auth: credentials, CLIAuth: credentials, UseCLIConfig: true, UserID: authContext.UserID, CLISessionID: authContext.CLISessionID, OperatorSessionID: authContext.OperatorSessionID}
}

func nativeEvalDeployment(cfg *config.Config, authContext *auth.ClientAuthContext, runID, target string) *evalv1.EvaluationDeploymentIdentity {
	return &evalv1.EvaluationDeploymentIdentity{
		DeploymentId:        runID,
		TopologyRef:         &compliancev1.VersionedReference{Id: evaluation.TopologyID, Version: evaluation.TopologyVersion},
		ControlledTarget:    target,
		IndependentObserver: constants.DockerEvaluationObserverService,
		RuntimeBoundaries: []*evalv1.EvaluationRuntimeBoundary{
			{Component: evalv1.EvaluationRuntimeComponent_EVALUATION_RUNTIME_COMPONENT_EVALUATOR, ProcessIdentity: "host-side g8e eval process", RuntimeNamespace: "Docker host workspace", Endpoint: cfg.OperatorHTTPURL(), AuthenticatedIdentity: authContext.UserID},
			{Component: evalv1.EvaluationRuntimeComponent_EVALUATION_RUNTIME_COMPONENT_GATEWAY, ProcessIdentity: constants.DockerGatewayContainer, RuntimeNamespace: "Gateway container", PersistentStore: "Gateway runtime volume", Endpoint: cfg.OperatorHTTPURL(), AuthenticatedIdentity: authContext.CLISessionID},
			{Component: evalv1.EvaluationRuntimeComponent_EVALUATION_RUNTIME_COMPONENT_OPERATOR, ProcessIdentity: constants.DockerOperatorContainer, RuntimeNamespace: "Operator container", MountedFilesystems: []string{"Operator runtime volume", "shared controlled fixture volume"}, PersistentStore: "Operator runtime volume"},
			{Component: evalv1.EvaluationRuntimeComponent_EVALUATION_RUNTIME_COMPONENT_CONTROLLED_TARGET, ProcessIdentity: target, RuntimeNamespace: "shared controlled fixture volume", MountedFilesystems: []string{"shared controlled fixture volume"}},
		},
	}
}

func writeNativeEvalRun(cmd *cobra.Command, report *evalv1.EvaluationReport, verification *compliancev1.ComplianceVerificationReport, jsonOutput bool) error {
	if jsonOutput {
		body, err := evalv1.MarshalCanonical(report)
		if err != nil {
			return fmt.Errorf("evaluation: canonicalize report: %w", err)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Run: %s\nStatus: %s\nSummary: %s\nVerification: %t\nOperator: %s\nSession: %s\n", report.GetRun().GetRunId(), report.GetSummaryStatus().String(), report.GetSummary(), verification.GetValid(), report.GetRun().GetTargetOperatorId(), report.GetRun().GetTargetOperatorSessionId())
	return err
}

func writeNativeVerification(cmd *cobra.Command, report *compliancev1.ComplianceVerificationReport, jsonOutput bool) error {
	if jsonOutput {
		body, err := compliancev1.MarshalCanonical(report)
		if err != nil {
			return fmt.Errorf("evaluation: canonicalize verification: %w", err)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Run: %s\nValid: %t\nFailures: %d\n", report.GetReportId(), report.GetValid(), len(report.GetFailures()))
	if err != nil {
		return err
	}
	for _, failure := range report.GetFailures() {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- %s: %s (%s)\n", failure.GetCode(), failure.GetReason(), failure.GetSubjectRef()); err != nil {
			return err
		}
	}
	return nil
}
