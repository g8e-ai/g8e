// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const chatAcceptTracePollInterval = 2 * time.Second

type chatEvalDeps struct {
	configLoader         func(string) (*config.Config, error)
	fileSvcFactory       func(string, *slog.Logger) (fs.RuntimeFileService, error)
	authLoader           func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error)
	clientFactory        func(harnessconfig.Config) (*harnessclient.Client, error)
	refreshClientFactory refreshClientFactory
	now                  func() time.Time
	newID                func() string
}

type chatAcceptanceCaseResult struct {
	Case                string `json:"case"`
	AssignmentID        string `json:"assignment_id"`
	EvaluationAttemptID string `json:"evaluation_attempt_id"`
	CaseID              string `json:"case_id,omitempty"`
	InvestigationID     string `json:"investigation_id,omitempty"`
	Status              string `json:"status"`
	Error               string `json:"error,omitempty"`
	TraceDigest         string `json:"trace_digest,omitempty"`
	ModelCalls          int    `json:"model_calls,omitempty"`
}

type chatAcceptanceOutput struct {
	OperatorSessionID string                     `json:"operator_session_id"`
	Model             string                     `json:"model"`
	Passed            int                        `json:"passed"`
	Failed            int                        `json:"failed"`
	Cases             []chatAcceptanceCaseResult `json:"cases"`
}

func gateChatEvalCmd(deps nativeEvalDeps) *cobra.Command {
	shared := chatEvalDeps{
		configLoader:         deps.configLoader,
		fileSvcFactory:       deps.fileSvcFactory,
		authLoader:           deps.authLoader,
		clientFactory:        deps.clientFactory,
		refreshClientFactory: defaultRefreshClientFactory,
		now:                  deps.now,
		newID:                deps.newID,
	}
	cmd := &cobra.Command{Use: "chat", Short: "Production chat-path vertical acceptance"}
	cmd.AddCommand(gateChatEvalRunCmd(shared))
	return cmd
}

func gateChatEvalRunCmd(deps chatEvalDeps) *cobra.Command {
	var operatorSessionID string
	var dataOperatorSessionID string
	var model string
	var campaignID string
	var registryDigest string
	var registryFile string
	var ensembleURL string
	var casesCSV string
	var noAutoRefresh bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the chat-path vertical acceptance matrix through production POST /api/v1/chat",
		RunE: func(cmd *cobra.Command, args []string) error {
			if model == "" {
				return fmt.Errorf("evaluation: chat accept: --model is required")
			}
			caseIDs, err := parseChatAcceptanceCases(casesCSV)
			if err != nil {
				return err
			}
			cases, err := evaluation.SelectChatAcceptanceCases(caseIDs)
			if err != nil {
				return fmt.Errorf("evaluation: chat accept: %w", err)
			}
			cfg, fileSvc, authContext, err := chatEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			registry, err := chatEvalLoadRegistry(campaignID, registryDigest, registryFile, model)
			if err != nil {
				return err
			}
			operators, err := chatEvalListOperators(cmd, deps, cfg, authContext)
			if err != nil {
				return err
			}
			if !noAutoRefresh {
				authContext, err = chatEvalEnsureOperatorBinding(cmd, deps, cfg, fileSvc, authContext, operators, dataOperatorSessionID)
				if err != nil {
					return fmt.Errorf("evaluation: chat accept: %w", err)
				}
			}
			selected, err := evaluation.SelectInferenceOperator(operators, operatorSessionID)
			if err != nil {
				return err
			}
			dataOperator, err := chatEvalResolveDataOperator(operators, authContext, dataOperatorSessionID)
			if err != nil {
				return fmt.Errorf("evaluation: chat accept: %w", err)
			}
			resolvedEnsembleURL := resolveChatEvalEnsembleURL(ensembleURL)
			ensembleClient, err := chatEvalEnsembleClient(cfg, authContext, resolvedEnsembleURL, deps)
			if err != nil {
				return err
			}
			persona := harnessclient.Persona{
				ID:                "g8e-chat-acceptance",
				UserAgent:         "g8e-eval-chat-acceptance",
				UserID:            authContext.UserID,
				CLISessionID:      authContext.CLISessionID,
				OperatorID:        dataOperator.OperatorID,
				OperatorSessionID: dataOperator.OperatorSessionID,
			}
			reporter := newChatAcceptReporter(cmd.OutOrStdout(), output.JSONEnabled(cmd))
			reporter.writeSetup(len(cases), model, selected.OperatorSessionID, dataOperator.OperatorSessionID, resolvedEnsembleURL)

			results := make([]chatAcceptanceCaseResult, 0, len(cases))
			failures := 0
			for caseIndex, acceptanceCase := range cases {
				assignmentID := deps.newID()
				attemptID := deps.newID()
				baseReq := evaluation.ChatProbeRequest{
					AssignmentID:            assignmentID,
					EvaluationAttemptID:     attemptID,
					CampaignID:              registry.CampaignID,
					RunID:                   "chat-accept-run",
					ScenarioID:              string(acceptanceCase.ID),
					Model:                   model,
					ModelDigest:             registry.ModelDigest,
					TargetOperatorSessionID: selected.OperatorSessionID,
					ModelRegistryDigest:     registry.Digest,
					ModelRegistry:           registry.Variants,
				}
				probeReq := acceptanceCase.Apply(baseReq)
				chatReq, err := evaluation.BuildChatProbeRequest(probeReq, dataOperator.OperatorID, dataOperator.OperatorSessionID)
				if err != nil {
					return fmt.Errorf("evaluation: chat accept: %w", err)
				}
				chatReq.Context.UserID = authContext.UserID
				chatReq.Context.CLISessionID = authContext.CLISessionID

				reporter.caseStart(caseIndex+1, len(cases), string(acceptanceCase.ID), probeReq.AssignmentID, probeReq.EvaluationAttemptID)
				ctx, cancel := context.WithTimeout(cmd.Context(), 8*time.Minute)
				chatResp, runErr := ensembleClient.EnsembleChat(ctx, persona, chatReq)
				if runErr == nil && chatResp != nil {
					reporter.chatSubmitted(chatResp.CaseID, chatResp.InvestigationID)
				} else if runErr != nil {
					reporter.chatSubmitFailed(runErr)
				}
				var trace evaluation.EvaluationTrace
				if runErr == nil {
					trace, runErr = chatEvalWaitForTrace(ctx, func(pollCtx context.Context) (evaluation.EvaluationTrace, error) {
						rawTrace, err := ensembleClient.GetEvaluationTrace(pollCtx, persona, probeReq.AssignmentID, probeReq.EvaluationAttemptID)
						if err != nil {
							return nil, err
						}
						return evaluation.EvaluationTrace(rawTrace), nil
					}, reporter)
				}
				cancel()

				entry := chatAcceptanceCaseResult{
					Case:                string(acceptanceCase.ID),
					AssignmentID:        probeReq.AssignmentID,
					EvaluationAttemptID: probeReq.EvaluationAttemptID,
				}
				if chatResp != nil {
					entry.CaseID = chatResp.CaseID
					entry.InvestigationID = chatResp.InvestigationID
				}
				if runErr != nil {
					entry.Status = "failed"
					entry.Error = runErr.Error()
					failures++
				} else if err := evaluation.ValidateChatProbeTrace(probeReq, trace); err != nil {
					entry.Status = "failed"
					entry.Error = err.Error()
					failures++
				} else if err := evaluation.ValidateChatAcceptanceCase(acceptanceCase.ID, probeReq, trace); err != nil {
					entry.Status = "failed"
					entry.Error = err.Error()
					failures++
				} else {
					entry.Status = "passed"
					entry.TraceDigest, _ = trace["trace_digest"].(string)
					entry.ModelCalls = len(trace["model_calls"].([]any))
				}
				results = append(results, entry)
				if output.JSONEnabled(cmd) {
					continue
				}
				if entry.Status == "passed" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "PASS %s\n", acceptanceCase.ID)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "FAIL %s: %s\n", acceptanceCase.ID, entry.Error)
				}
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(chatAcceptanceOutput{
					OperatorSessionID: selected.OperatorSessionID,
					Model:             model,
					Passed:            len(cases) - failures,
					Failed:            failures,
					Cases:             results,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				if err != nil {
					return err
				}
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nPhase 1A chat acceptance: %d passed, %d failed\n", len(cases)-failures, failures)
			}
			if failures > 0 {
				return fmt.Errorf("evaluation: chat accept: %d case(s) failed", failures)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&operatorSessionID, "inference-session", "", "Pin acceptance to one exact inference Operator session")
	cmd.Flags().StringVar(&dataOperatorSessionID, "data-session", "", "Pin chat binding to one exact data Operator session")
	cmd.Flags().BoolVar(&noAutoRefresh, "no-auto-refresh", false, "Do not refresh stale CLI operator bindings before acceptance")
	cmd.Flags().StringVar(&model, "model", "", "Frozen campaign model tag for all chat roles")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Frozen evaluation campaign ID")
	cmd.Flags().StringVar(&registryDigest, "registry-digest", "", "Frozen campaign model registry digest")
	cmd.Flags().StringVar(&registryFile, "registry-file", "", "JSON file produced by eval models freeze --output --json")
	cmd.Flags().StringVar(&ensembleURL, "ensemble-url", "", "g8ee HTTP surface (default: http://localhost:8000)")
	cmd.Flags().StringVar(&casesCSV, "cases", "", "Comma-separated case IDs (default: full Phase 1A chat matrix)")
	return cmd
}

type chatAcceptReporter struct {
	out   io.Writer
	quiet bool
}

func newChatAcceptReporter(out io.Writer, quiet bool) *chatAcceptReporter {
	return &chatAcceptReporter{out: out, quiet: quiet}
}

func (r *chatAcceptReporter) enabled() bool {
	return r != nil && !r.quiet && r.out != nil
}

func (r *chatAcceptReporter) writef(format string, args ...any) {
	if !r.enabled() {
		return
	}
	_, _ = fmt.Fprintf(r.out, format+"\n", args...)
}

func (r *chatAcceptReporter) writeSetup(caseCount int, model, inferenceSessionID, dataSessionID, ensembleURL string) {
	r.writef("Running Phase 1A chat acceptance (%d case(s))", caseCount)
	r.writef("  model: %s", model)
	r.writef("  inference operator session: %s", inferenceSessionID)
	r.writef("  data operator session: %s", dataSessionID)
	r.writef("  ensemble: %s", ensembleURL)
}

func (r *chatAcceptReporter) caseStart(caseNum, caseTotal int, caseID, assignmentID, attemptID string) {
	r.writef("[%d/%d] %s: submitting POST /api/v1/chat", caseNum, caseTotal, caseID)
	r.writef("  assignment_id=%s evaluation_attempt_id=%s", assignmentID, attemptID)
}

func (r *chatAcceptReporter) chatSubmitted(caseID, investigationID string) {
	r.writef("  chat accepted: case_id=%s investigation_id=%s", caseID, investigationID)
	r.writef("  waiting for evaluation trace (poll every 2s, timeout 8m per case)")
}

func (r *chatAcceptReporter) chatSubmitFailed(err error) {
	r.writef("  chat submit failed: %v", err)
}

func (r *chatAcceptReporter) traceWaiting() {
	r.writef("  trace not yet available")
}

func (r *chatAcceptReporter) traceFetchRetrying(err error, elapsed time.Duration) {
	r.writef("  trace lookup still pending after %s: %v", formatChatAcceptElapsed(elapsed), err)
}

func (r *chatAcceptReporter) traceFetchFailed(err error) {
	r.writef("  trace lookup failed: %v", err)
}

func chatTraceFetchIsFatal(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "status 401") ||
		strings.Contains(message, "status 403") ||
		strings.Contains(message, "authentication required")
}

func (r *chatAcceptReporter) traceProgress(status string, elapsed time.Duration, modelCalls int) {
	r.writef("  trace status=%s elapsed=%s model_calls=%d", status, formatChatAcceptElapsed(elapsed), modelCalls)
}

func (r *chatAcceptReporter) traceTerminal(status string, elapsed time.Duration) {
	r.writef("  trace %s after %s", status, formatChatAcceptElapsed(elapsed))
}

func formatChatAcceptElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", d.Round(time.Second)/time.Second)
	}
	minutes := d / time.Minute
	seconds := (d % time.Minute).Round(time.Second) / time.Second
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

func resolveChatEvalEnsembleURL(ensembleURL string) string {
	if strings.TrimSpace(ensembleURL) != "" {
		return strings.TrimSpace(ensembleURL)
	}
	if envURL := strings.TrimSpace(os.Getenv("G8E_ENSEMBLE_URL")); envURL != "" {
		return envURL
	}
	return network.LocalhostHTTPURL(constants.EnsembleDefaultPort)
}

func chatEvalWaitForTrace(
	ctx context.Context,
	fetch func(context.Context) (evaluation.EvaluationTrace, error),
	reporter *chatAcceptReporter,
) (evaluation.EvaluationTrace, error) {
	return chatEvalWaitForTraceWithPoll(ctx, fetch, reporter, chatAcceptTracePollInterval)
}

func chatEvalWaitForTraceWithPoll(
	ctx context.Context,
	fetch func(context.Context) (evaluation.EvaluationTrace, error),
	reporter *chatAcceptReporter,
	pollInterval time.Duration,
) (evaluation.EvaluationTrace, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	started := time.Now()
	lastStatus := ""
	lastReport := started
	reportedWaiting := false
	for {
		trace, err := fetch(ctx)
		if err == nil {
			status, _ := trace["status"].(string)
			if status == "completed" || status == "failed" {
				reporter.traceTerminal(status, time.Since(started))
				return trace, nil
			}
			now := time.Now()
			modelCalls := 0
			if calls, ok := trace["model_calls"].([]any); ok {
				modelCalls = len(calls)
			}
			if status != lastStatus || now.Sub(lastReport) >= 15*time.Second {
				reporter.traceProgress(status, now.Sub(started), modelCalls)
				lastStatus = status
				lastReport = now
			}
		} else if chatTraceFetchIsFatal(err) {
			reporter.traceFetchFailed(err)
			return nil, fmt.Errorf("evaluation: chat accept: wait for trace: %w", err)
		} else if !reportedWaiting {
			reporter.traceWaiting()
			reportedWaiting = true
			lastReport = time.Now()
		} else if time.Since(lastReport) >= 15*time.Second {
			reporter.traceFetchRetrying(err, time.Since(started))
			lastReport = time.Now()
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return nil, fmt.Errorf("evaluation: chat accept: wait for trace: %w", err)
			}
			return nil, fmt.Errorf("evaluation: chat accept: wait for trace: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

type chatRegistrySelection struct {
	CampaignID  string
	Digest      string
	Variants    []*operatorv1.InferenceModelVariant
	ModelDigest string
}

func chatEvalLoadRegistry(campaignID, registryDigest, registryFile, model string) (*chatRegistrySelection, error) {
	if registryFile != "" {
		freeze, err := loadRegistryFreezeFile(registryFile)
		if err != nil {
			return nil, err
		}
		variant, err := freeze.LookupModelVariant(model)
		if err != nil {
			return nil, fmt.Errorf("evaluation: chat accept: lookup model variant: %w", err)
		}
		return &chatRegistrySelection{
			CampaignID:  freeze.CampaignID,
			Digest:      freeze.Digest,
			Variants:    freeze.Variants,
			ModelDigest: variant.GetDigest(),
		}, nil
	}
	if campaignID == "" || registryDigest == "" {
		return nil, fmt.Errorf("evaluation: chat accept: set --registry-file or both --campaign-id and --registry-digest")
	}
	return nil, fmt.Errorf("evaluation: chat accept: model digest lookup requires --registry-file")
}

func chatEvalEnvironment(cmd *cobra.Command, deps chatEvalDeps) (*config.Config, fs.RuntimeFileService, *auth.ClientAuthContext, error) {
	projectRoot, err := cmd.Flags().GetString("project-root")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("evaluation: read project root: %w", err)
	}
	cfg, err := deps.configLoader(projectRoot)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("evaluation: load config: %w", err)
	}
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, slog.Default())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	authContext, err := deps.authLoader(fileSvc, cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("evaluation: load CLI identity: %w", err)
	}
	return cfg, fileSvc, authContext, nil
}

func chatEvalEnsureOperatorBinding(
	cmd *cobra.Command,
	deps chatEvalDeps,
	cfg *config.Config,
	fileSvc fs.RuntimeFileService,
	authContext *auth.ClientAuthContext,
	operators []models.OperatorDocumentGo,
	pinnedDataSessionID string,
) (*auth.ClientAuthContext, error) {
	targetSessionID := pinnedDataSessionID
	if targetSessionID == "" {
		active, err := evaluation.SelectDataOperator(operators, "")
		if err != nil {
			return authContext, nil
		}
		targetSessionID = active.OperatorSessionID
	}
	if authContext.OperatorSessionID == targetSessionID {
		return authContext, nil
	}
	if deps.refreshClientFactory == nil {
		return nil, fmt.Errorf("enrolled CLI operator session %q is not active (current data operator session %q); run './g8e auth refresh'", authContext.OperatorSessionID, targetSessionID)
	}
	refresh, err := deps.refreshClientFactory(cfg).Refresh(cmd.Context(), fileSvc)
	if err != nil {
		return nil, fmt.Errorf("refresh stale operator binding: %w; run './g8e auth refresh'", err)
	}
	if refresh.OperatorSessionID != targetSessionID {
		return nil, fmt.Errorf("gateway refresh returned operator session %q but active data operator is %q; rebuild the gateway image and run './g8e auth refresh'", refresh.OperatorSessionID, targetSessionID)
	}
	creds, err := auth.LoadCredentials(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("load credentials after refresh: %w", err)
	}
	if creds == nil {
		return nil, fmt.Errorf("%w: local CLI credentials are absent after refresh", constants.ErrNotAuthenticated)
	}
	creds.CLISessionID = refresh.CLISessionID
	creds.OperatorSessionID = refresh.OperatorSessionID
	creds.OperatorID = refresh.OperatorID
	if err := auth.SaveCredentials(fileSvc, cfg, creds); err != nil {
		return nil, fmt.Errorf("save credentials after refresh: %w", err)
	}
	if !output.JSONEnabled(cmd) {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Refreshed CLI operator binding to session %s\n", refresh.OperatorSessionID)
	}
	return &auth.ClientAuthContext{
		OperatorSessionID: refresh.OperatorSessionID,
		CLISessionID:      refresh.CLISessionID,
		UserID:            refresh.UserID,
		OperatorID:        refresh.OperatorID,
		ClientCert:        authContext.ClientCert,
		ClientKey:         authContext.ClientKey,
	}, nil
}

func chatEvalResolveDataOperator(
	operators []models.OperatorDocumentGo,
	authContext *auth.ClientAuthContext,
	pinnedSessionID string,
) (*evaluation.DataOperatorStatus, error) {
	if pinnedSessionID != "" {
		return evaluation.SelectDataOperator(operators, pinnedSessionID)
	}
	if authContext.OperatorSessionID == "" {
		return nil, fmt.Errorf("evaluation: chat accept requires enrolled CLI operator session; run './g8e auth refresh' or pass --data-session")
	}
	selected, err := evaluation.SelectDataOperator(operators, authContext.OperatorSessionID)
	if err != nil {
		active, listErr := evaluation.SelectDataOperator(operators, "")
		if listErr == nil {
			return nil, fmt.Errorf("%w: enrolled CLI operator session %q is not active (current data operator session %q); run './g8e auth refresh'", err, authContext.OperatorSessionID, active.OperatorSessionID)
		}
		return nil, err
	}
	if authContext.OperatorID != "" {
		selected.OperatorID = authContext.OperatorID
	}
	return selected, nil
}

func chatEvalListOperators(
	cmd *cobra.Command,
	deps chatEvalDeps,
	cfg *config.Config,
	authContext *auth.ClientAuthContext,
) ([]models.OperatorDocumentGo, error) {
	gatewayClient, err := deps.clientFactory(nativeEvalClientConfig(cfg, authContext))
	if err != nil {
		return nil, fmt.Errorf("evaluation: initialize gateway client: %w", err)
	}
	operators, _, err := gatewayClient.ListOperators(cmd.Context())
	if err != nil {
		return nil, fmt.Errorf("evaluation: list operators: %w", err)
	}
	return operators, nil
}

func chatEvalEnsembleClient(cfg *config.Config, authContext *auth.ClientAuthContext, ensembleURL string, deps chatEvalDeps) (*harnessclient.Client, error) {
	clientConfig := harnessconfig.Config{
		EnsembleBaseURL: ensembleURL,
		UserID:          authContext.UserID,
		CLISessionID:    authContext.CLISessionID,
	}
	return deps.clientFactory(clientConfig)
}

func parseChatAcceptanceCases(raw string) ([]evaluation.ChatAcceptanceCaseID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	caseIDs := make([]evaluation.ChatAcceptanceCaseID, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		caseIDs = append(caseIDs, evaluation.ChatAcceptanceCaseID(part))
	}
	if len(caseIDs) == 0 {
		return nil, fmt.Errorf("evaluation: chat accept: no cases selected")
	}
	return caseIDs, nil
}
