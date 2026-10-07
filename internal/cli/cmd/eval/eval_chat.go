// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
)

const (
	chatAcceptTracePollInterval = 2 * time.Second
	// chatAcceptCampaignID is the campaign the acceptance chats are issued under;
	// the registry digest each request carries is bound to it.
	chatAcceptCampaignID = "chat-accept"
)

type chatEvalDeps struct {
	configLoader      func(string) (*config.Config, error)
	fileSvcFactory    func(string, *slog.Logger) (fs.RuntimeFileService, error)
	authLoader        func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error)
	clientFactory     func(harnessconfig.Config) (*harnessclient.Client, error)
	bindClientFactory func(*config.Config) chatEvalBindClient
	now               func() time.Time
	newID             func() (string, error)
}

// chatEvalBindClient is the part of the enrollment client that reads and
// changes the operator sessions bound to the CLI session.
type chatEvalBindClient interface {
	Bind(ctx context.Context, fileSvc fs.RuntimeFileService, operatorSessionIDs []string) (auth.CLISessionBind, error)
	SessionInfo(ctx context.Context, fileSvc fs.RuntimeFileService) (auth.CLISessionInfo, error)
}

func defaultChatEvalBindClient(cfg *config.Config) chatEvalBindClient {
	return auth.NewEnrollmentClient(cfg, nil)
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

func gatesChatEvalCmd(deps nativeEvalDeps) *cobra.Command {
	var model string
	var ensembleURL string
	var casesCSV string
	var noAutoBind bool
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Chat-path vertical acceptance through production POST /api/v1/chat",
		Long: `Run the environment canaries, then the chat-path acceptance cases.

The canaries fail fast with "ENVIRONMENT ERROR (<canary>)" and run no case when
the harness is broken rather than the model:
  tools-declared        the provider received every bound registry tool and the eval tool-gate bypass is recorded
  seed-delivered        a seeded investigation was applied by g8ee and echoed unchanged
  workspace-reachable   a fixture file written to the attempt workspace reads back through governed dispatch
  guidance-delivered    seeded tool guidance reached g8ee byte for byte from the agent tool registry
  registry-mcp          every agent registry entry verifies and the Gateway /mcp tools/list answers

The model must be in the frozen inventory (g8e eval models).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if model == "" {
				return fmt.Errorf("evaluation: gates chat: %w", constants.ErrEvaluationModelRequired)
			}
			caseIDs, err := parseChatAcceptanceCases(casesCSV)
			if err != nil {
				return err
			}
			cases, err := evaluation.SelectChatAcceptanceCases(caseIDs)
			if err != nil {
				return fmt.Errorf("evaluation: gates chat: %w", err)
			}
			// The canaries prove the harness is intact before any case runs, so a
			// broken environment is never reported as a model failure.
			if err := deps.runEnvironmentCanaries(cmd, canaryOptions{Model: model, EnsembleURL: ensembleURL, NoAutoBind: noAutoBind}); err != nil {
				return err
			}
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			projectRoot, err := cmd.Flags().GetString("project-root")
			if err != nil {
				return fmt.Errorf("evaluation: read project root: %w", err)
			}
			probeModel, err := resolveChatProbeModel(cmd.Context(), fileSvc, projectRoot, chatAcceptCampaignID, model)
			if err != nil {
				return err
			}
			authContext, err := deps.authLoader(fileSvc, cfg)
			if err != nil {
				return fmt.Errorf("evaluation: load CLI identity: %w", err)
			}
			gatewayClient, err := deps.clientFactory(nativeEvalClientConfig(cfg, authContext))
			if err != nil {
				return fmt.Errorf("evaluation: initialize gateway client: %w", err)
			}
			operators, _, err := gatewayClient.ListOperators(cmd.Context())
			if err != nil {
				return fmt.Errorf("evaluation: list operators: %w", err)
			}
			selectedInference, err := evaluation.SelectInferenceOperator(operators, "")
			if err != nil {
				return fmt.Errorf("evaluation: chat accept: %w", err)
			}
			selectedData, err := operatorcapability.SelectDataOperator(operators)
			if err != nil {
				return fmt.Errorf("evaluation: chat accept: %w", err)
			}
			authContext, err = chatEvalBindDataOperator(cmd, deps.chatDeps(), cfg, fileSvc, authContext, operators, selectedData.OperatorSessionID, !noAutoBind)
			if err != nil {
				return fmt.Errorf("evaluation: chat accept: %w", err)
			}
			resolvedEnsembleURL := resolveChatEvalEnsembleURL(ensembleURL)
			ensembleClient, err := chatEvalEnsembleClient(authContext, resolvedEnsembleURL, deps.chatDeps())
			if err != nil {
				return fmt.Errorf("evaluation: chat accept: %w", err)
			}
			persona := harnessclient.Persona{
				ID:                "g8e-chat-acceptance",
				UserAgent:         "g8e-eval-chat-acceptance",
				UserID:            authContext.UserID,
				CLISessionID:      authContext.CLISessionID,
				OperatorID:        selectedData.OperatorID,
				OperatorSessionID: selectedData.OperatorSessionID,
			}
			reporter := newChatAcceptReporter(cmd.OutOrStdout(), output.JSONEnabled(cmd))
			reporter.writeSetup(len(cases), model, selectedInference.OperatorSessionID, selectedData.OperatorSessionID, resolvedEnsembleURL)

			results := make([]chatAcceptanceCaseResult, 0, len(cases))
			failures := 0
			for caseIndex, acceptanceCase := range cases {
				assignmentID, err := deps.newID()
				if err != nil {
					return err
				}
				attemptID, err := deps.newID()
				if err != nil {
					return err
				}
				baseReq := evaluation.ChatProbeRequest{
					AssignmentID:            assignmentID,
					EvaluationAttemptID:     attemptID,
					CampaignID:              chatAcceptCampaignID,
					RunID:                   "chat-accept-run",
					ScenarioID:              string(acceptanceCase.ID),
					Model:                   model,
					ModelDigest:             probeModel.Digest,
					ModelRegistryDigest:     probeModel.RegistryDigest,
					ModelRegistry:           probeModel.Registry,
					TargetOperatorSessionID: selectedInference.OperatorSessionID,
				}
				probeReq := acceptanceCase.Apply(baseReq)
				chatReq, err := evaluation.BuildChatProbeRequest(probeReq, selectedData.OperatorID, selectedData.OperatorSessionID)
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
						return evaluation.DecodeEvaluationTrace(rawTrace)
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
					OperatorSessionID: selectedInference.OperatorSessionID,
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
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nChat acceptance: %d passed, %d failed\n", len(cases)-failures, failures)
			}
			if failures > 0 {
				return fmt.Errorf("evaluation: chat accept: %d case(s) failed", failures)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "Requested provider model tag")
	cmd.Flags().StringVar(&ensembleURL, "ensemble-url", "", "g8ee HTTP surface (default: http://localhost:8000)")
	cmd.Flags().StringVar(&casesCSV, "cases", "", "Comma-separated case IDs (default: every chat acceptance case)")
	cmd.Flags().BoolVar(&noAutoBind, "no-auto-bind", false, "Do not bind the data-operator to the CLI session when it is not bound")
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
	r.writef("Running chat acceptance (%d case(s))", caseCount)
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

// chatEvalBoundSessionIDs reads the operator sessions bound to the CLI
// session, primary first.
func chatEvalBoundSessionIDs(ctx context.Context, client chatEvalBindClient, fileSvc fs.RuntimeFileService) ([]string, error) {
	info, err := client.SessionInfo(ctx, fileSvc)
	if err != nil {
		return nil, fmt.Errorf("evaluation: read CLI session bindings: %w", err)
	}
	if len(info.BoundOperatorSessionIDs) > 0 {
		return info.BoundOperatorSessionIDs, nil
	}
	if info.OperatorSessionID != "" {
		return []string{info.OperatorSessionID}, nil
	}
	return nil, nil
}

// chatEvalBindSessions binds the CLI session to operatorSessionIDs in one call
// and stores the replacement CLI session in the local credentials. Binding
// replaces the CLI session, so the returned auth context carries the new one.
func chatEvalBindSessions(
	cmd *cobra.Command,
	cfg *config.Config,
	fileSvc fs.RuntimeFileService,
	client chatEvalBindClient,
	authContext *auth.ClientAuthContext,
	operatorSessionIDs []string,
) (*auth.ClientAuthContext, error) {
	bind, err := client.Bind(cmd.Context(), fileSvc, operatorSessionIDs)
	if err != nil {
		return nil, fmt.Errorf("evaluation: bind operator sessions: %w", err)
	}
	creds, err := auth.LoadCredentials(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load credentials after bind: %w", err)
	}
	if creds == nil {
		return nil, fmt.Errorf("%w: local CLI credentials are absent after bind", constants.ErrNotAuthenticated)
	}
	creds.CLISessionID = bind.CLISessionID
	creds.OperatorSessionID = bind.OperatorSessionID
	creds.OperatorID = bind.OperatorID
	if err := auth.SaveCredentials(fileSvc, cfg, creds); err != nil {
		return nil, fmt.Errorf("evaluation: save credentials after bind: %w", err)
	}
	if !output.JSONEnabled(cmd) {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Bound CLI session to %d operator session(s); primary %s\n", len(bind.Bound), bind.OperatorSessionID)
	}
	return &auth.ClientAuthContext{
		OperatorSessionID: bind.OperatorSessionID,
		CLISessionID:      bind.CLISessionID,
		UserID:            bind.UserID,
		OperatorID:        bind.OperatorID,
		ClientCert:        authContext.ClientCert,
		ClientKey:         authContext.ClientKey,
	}, nil
}

// chatEvalBindDataOperator makes the data-operator one of the operator
// sessions bound to the CLI session. A CLI session holds many bound operators,
// so an already-bound data-operator leaves the CLI session untouched. When it
// is not bound, autoBind issues one bind call for the still-active bound
// sessions plus the data-operator; otherwise ErrDataOperatorNotBound is
// returned.
func chatEvalBindDataOperator(
	cmd *cobra.Command,
	deps chatEvalDeps,
	cfg *config.Config,
	fileSvc fs.RuntimeFileService,
	authContext *auth.ClientAuthContext,
	operators []*operatorv1.OperatorDocument,
	dataSessionID string,
	autoBind bool,
) (*auth.ClientAuthContext, error) {
	client := deps.bindClientFactory(cfg)
	bound, err := chatEvalBoundSessionIDs(cmd.Context(), client, fileSvc)
	if err != nil {
		return nil, err
	}
	if slices.Contains(bound, dataSessionID) {
		return authContext, nil
	}
	if !autoBind {
		return nil, fmt.Errorf("%w: session %s", constants.ErrDataOperatorNotBound, dataSessionID)
	}
	targets := make([]string, 0, len(bound)+1)
	for _, sessionID := range bound {
		if slices.ContainsFunc(operators, func(op operatorv1.OperatorDocument) bool {
			return op.OperatorSessionID == sessionID && op.Status == constants.OperatorStatusActive
		}) {
			targets = append(targets, sessionID)
		}
	}
	return chatEvalBindSessions(cmd, cfg, fileSvc, client, authContext, append(targets, dataSessionID))
}

func chatEvalListOperators(
	cmd *cobra.Command,
	deps chatEvalDeps,
	cfg *config.Config,
	authContext *auth.ClientAuthContext,
) ([]*operatorv1.OperatorDocument, error) {
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

type campaignChatHarnessClient struct {
	client *harnessclient.Client
}

func (w *campaignChatHarnessClient) EnsembleChat(ctx context.Context, persona harnessclient.Persona, req harnessclient.EnsembleChatRequest) (*harnessclient.EnsembleChatResponse, error) {
	return w.client.EnsembleChat(ctx, persona, req)
}

func (w *campaignChatHarnessClient) GetEvaluationTrace(ctx context.Context, persona harnessclient.Persona, assignmentID, evaluationAttemptID string) (evaluation.EvaluationTrace, error) {
	raw, err := w.client.GetEvaluationTrace(ctx, persona, assignmentID, evaluationAttemptID)
	if err != nil {
		return nil, err
	}
	return evaluation.DecodeEvaluationTrace(raw)
}

func chatEvalEnsembleClient(authContext *auth.ClientAuthContext, ensembleURL string, deps chatEvalDeps) (*harnessclient.Client, error) {
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
		return nil, fmt.Errorf("evaluation: chat accept: %w", constants.ErrEvaluationNoCasesSelected)
	}
	return caseIDs, nil
}
