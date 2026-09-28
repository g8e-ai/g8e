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
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type inferenceAcceptanceResultJSON struct {
	Case            string `json:"case"`
	ProviderAttempt string `json:"provider_attempt"`
	Stream          bool   `json:"stream"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	ProgressEvents  int    `json:"progress_events,omitempty"`
	ResultDigest    string `json:"result_digest,omitempty"`
}

type inferenceAcceptanceOutputJSON struct {
	OperatorSessionID string                          `json:"operator_session_id"`
	Model             string                          `json:"model"`
	Passed            int                             `json:"passed"`
	Failed            int                             `json:"failed"`
	Cases             []inferenceAcceptanceResultJSON `json:"cases"`
}

type inferenceEvalDeps struct {
	configLoader     func(string) (*config.Config, error)
	fileSvcFactory   func(string, *slog.Logger) (fs.RuntimeFileService, error)
	authLoader       func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error)
	clientFactory    func(harnessconfig.Config) (*harnessclient.Client, error)
	appClientFactory func(harnessconfig.Config) (*harnessclient.Client, error)
	now              func() time.Time
	newID            func() string
}

func gatesInferenceEvalCmd(deps nativeEvalDeps) *cobra.Command {
	var model string
	var role string
	var casesCSV string
	cmd := &cobra.Command{
		Use:   "inference",
		Short: "Inference-only vertical acceptance matrix",
		RunE: func(cmd *cobra.Command, args []string) error {
			if model == "" {
				return fmt.Errorf("evaluation: gates inference: --model is required")
			}
			caseIDs, err := parseInferenceAcceptanceCases(casesCSV)
			if err != nil {
				return err
			}
			cases, err := evaluation.SelectInferenceAcceptanceCases(caseIDs)
			if err != nil {
				return fmt.Errorf("evaluation: gates inference: %w", err)
			}
			cfg, _, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			authContext, err := deps.authLoader(nil, cfg)
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
			selected, err := evaluation.SelectInferenceOperator(operators, "")
			if err != nil {
				return fmt.Errorf("evaluation: gates inference: %w", err)
			}
			appClient, err := inferenceEvalAppClientFrom(cfg, authContext, deps.clientFactory)
			if err != nil {
				return fmt.Errorf("evaluation: gates inference: %w", err)
			}
			probeRole, err := parseInferenceProbeRole(role)
			if err != nil {
				return fmt.Errorf("evaluation: gates inference: %w", err)
			}
			baseReq := evaluation.InferenceProbeRequest{
				ProviderAttemptID:       deps.newID(),
				Role:                    probeRole,
				Model:                   model,
				TargetOperatorSessionID: selected.OperatorSessionID,
			}
			results := make([]inferenceAcceptanceResultJSON, 0, len(cases))
			failures := 0
			for _, acceptanceCase := range cases {
				probeReq := acceptanceCase.Apply(baseReq)
				probeReq.ProviderAttemptID = deps.newID()
				probeReq.Stream = acceptanceCase.Stream
				ctx, cancel := context.WithTimeout(cmd.Context(), 6*time.Minute)
				response, progress, runErr := inferenceEvalExecuteProbe(ctx, appClient, probeReq)
				cancel()
				entry := inferenceAcceptanceResultJSON{
					Case:            string(acceptanceCase.ID),
					ProviderAttempt: probeReq.ProviderAttemptID,
					Stream:          probeReq.Stream,
				}
				if runErr != nil {
					entry.Status = "failed"
					entry.Error = runErr.Error()
					failures++
				} else if err := evaluation.ValidateInferenceProbeStream(probeReq, progress, response); err != nil {
					entry.Status = "failed"
					entry.Error = err.Error()
					failures++
				} else if err := evaluation.ValidateInferenceAcceptanceCase(acceptanceCase.ID, response); err != nil {
					entry.Status = "failed"
					entry.Error = err.Error()
					failures++
				} else {
					entry.Status = "passed"
					entry.ProgressEvents = len(progress)
					entry.ResultDigest = response.GetResult().GetResultDigest()
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
				payload, err := json.MarshalIndent(inferenceAcceptanceOutputJSON{
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
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nPhase 1A inference acceptance: %d passed, %d failed\n", len(cases)-failures, failures)
			}
			if failures > 0 {
				return fmt.Errorf("evaluation: gates inference: %d case(s) failed", failures)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "Requested provider model tag")
	cmd.Flags().StringVar(&role, "role", "primary", "Default governed model role for cases that do not override it")
	cmd.Flags().StringVar(&casesCSV, "cases", "", "Comma-separated case IDs (default: full Phase 1A inference matrix)")
	return cmd
}

func gatesProbeEvalCmd(deps nativeEvalDeps) *cobra.Command {
	var model string
	var role string
	var prompt string
	var seed int32 = -1
	var stream bool
	cmd := &cobra.Command{
		Use:   "probe <model>",
		Short: "Single non-scored governed inference probe",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			model = args[0]
			cfg, _, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			authContext, err := deps.authLoader(nil, cfg)
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
			selected, err := evaluation.SelectInferenceOperator(operators, "")
			if err != nil {
				return fmt.Errorf("evaluation: gates probe: %w", err)
			}
			appClient, err := inferenceEvalAppClientFrom(cfg, authContext, deps.clientFactory)
			if err != nil {
				return fmt.Errorf("evaluation: gates probe: %w", err)
			}
			probeRole, err := parseInferenceProbeRole(role)
			if err != nil {
				return fmt.Errorf("evaluation: gates probe: %w", err)
			}
			probeReq := evaluation.InferenceProbeRequest{
				ProviderAttemptID:       deps.newID(),
				Role:                    probeRole,
				Model:                   model,
				TargetOperatorSessionID: selected.OperatorSessionID,
				Prompt:                  prompt,
				Stream:                  stream,
			}
			if seed >= 0 {
				probeReq.Seed = &seed
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 6*time.Minute)
			defer cancel()
			response, progress, err := inferenceEvalExecuteProbe(ctx, appClient, probeReq)
			if err != nil {
				return fmt.Errorf("evaluation: gates probe: %w", err)
			}
			if err := evaluation.ValidateInferenceProbeStream(probeReq, progress, response); err != nil {
				return fmt.Errorf("evaluation: gates probe: %w", err)
			}
			if output.JSONEnabled(cmd) {
				body, err := protojson.Marshal(response)
				if err != nil {
					return fmt.Errorf("evaluation: gates probe: marshal response: %w", err)
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return err
			}
			result := response.GetResult()
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Probe accepted\nSession: %s\nModel: %s\nAttempt: %s\nResult digest: %s\nOutput hash: %s\nParts: %d\nProgress events: %d\n", selected.OperatorSessionID, result.GetRequestedModel(), result.GetProviderAttemptId(), result.GetResultDigest(), result.GetOutputHash(), len(result.GetParts()), len(progress))
			return err
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "[DEPRECATED] use positional argument instead")
	cmd.Flags().StringVar(&role, "role", "primary", "Governed model role: primary, assistant, or lite")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Probe prompt (default: Reply with exactly: probe-ok)")
	cmd.Flags().Int32Var(&seed, "seed", -1, "Optional deterministic generation seed (omit for provider default)")
	cmd.Flags().BoolVar(&stream, "stream", false, "Request live NDJSON progress telemetry during generation")
	return cmd
}


func inferenceEvalAppClientFrom(cfg *config.Config, authContext *auth.ClientAuthContext, clientFactory func(harnessconfig.Config) (*harnessclient.Client, error)) (*harnessclient.Client, error) {
	certFile, keyFile, err := resolveInferenceProbeAppCredentials(nil, cfg)
	if err != nil {
		return nil, err
	}
	trustBundle := cfg.ResolvedTrustBundlePath()
	appConfig := harnessconfig.Config{
		MTLSBaseURL: cfg.OperatorHTTPURL(),
		Auth: harnessconfig.Auth{
			ClientCert: certFile,
			ClientKey:  keyFile,
			CABundle:   trustBundle,
		},
		UserID: authContext.UserID,
	}
	return clientFactory(appConfig)
}

func inferenceEvalExecuteProbe(
	ctx context.Context,
	appClient *harnessclient.Client,
	probeReq evaluation.InferenceProbeRequest,
) (*operatorv1.InferenceDispatchResponse, []*operatorv1.InferenceProgressEvent, error) {
	dispatchReq, err := evaluation.BuildInferenceProbeDispatchRequest(probeReq)
	if err != nil {
		return nil, nil, err
	}
	if probeReq.Stream {
		streamResult, err := appClient.DispatchInferenceStream(ctx, dispatchReq)
		if err != nil {
			return nil, nil, err
		}
		return streamResult.Completion, streamResult.Progress, nil
	}
	response, _, err := appClient.DispatchInference(ctx, dispatchReq)
	return response, nil, err
}

func parseInferenceAcceptanceCases(raw string) ([]evaluation.InferenceAcceptanceCaseID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	caseIDs := make([]evaluation.InferenceAcceptanceCaseID, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		caseIDs = append(caseIDs, evaluation.InferenceAcceptanceCaseID(part))
	}
	if len(caseIDs) == 0 {
		return nil, fmt.Errorf("evaluation: inference accept: no cases selected")
	}
	return caseIDs, nil
}

func inferenceEvalGatewayClient(cmd *cobra.Command, deps inferenceEvalDeps) (*config.Config, fs.RuntimeFileService, *auth.ClientAuthContext, *harnessclient.Client, error) {
	projectRoot, err := cmd.Flags().GetString("project-root")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("evaluation: read project root: %w", err)
	}
	cfg, err := deps.configLoader(projectRoot)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("evaluation: load config: %w", err)
	}
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, slog.Default())
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	authContext, err := deps.authLoader(fileSvc, cfg)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("evaluation: load CLI identity: %w", err)
	}
	client, err := deps.clientFactory(nativeEvalClientConfig(cfg, authContext))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("evaluation: initialize gateway client: %w", err)
	}
	return cfg, fileSvc, authContext, client, nil
}

func inferenceEvalAppClient(cfg *config.Config, fileSvc fs.RuntimeFileService, authContext *auth.ClientAuthContext, deps inferenceEvalDeps) (*harnessclient.Client, error) {
	certFile, keyFile, err := resolveInferenceProbeAppCredentials(fileSvc, cfg)
	if err != nil {
		return nil, err
	}
	trustBundle := cfg.ResolvedTrustBundlePath()
	appConfig := harnessconfig.Config{
		MTLSBaseURL: cfg.OperatorHTTPURL(),
		Auth: harnessconfig.Auth{
			ClientCert: certFile,
			ClientKey:  keyFile,
			CABundle:   trustBundle,
		},
		UserID: authContext.UserID,
	}
	return deps.appClientFactory(appConfig)
}

func resolveInferenceProbeAppCredentials(fileSvc fs.RuntimeFileService, cfg *config.Config) (string, string, error) {
	issuedAppCert := filepath.Join(cfg.Paths.Infra.AppCertDir, "g8ee"+constants.FileExtCert)
	issuedAppKey := filepath.Join(cfg.Paths.Infra.AppCertDir, "g8ee"+constants.FileExtKey)
	pairs := []struct{ cert, key string }{
		{os.Getenv(string(constants.EnvVar.AppCert)), os.Getenv(string(constants.EnvVar.AppKey))},
		{issuedAppCert, issuedAppKey},
		{cfg.AppCertFile("g8ee"), cfg.AppKeyFile("g8ee")},
	}
	for _, pair := range pairs {
		if pair.cert == "" || pair.key == "" {
			continue
		}
		if _, err := os.Stat(pair.cert); err != nil {
			continue
		}
		if _, err := os.Stat(pair.key); err != nil {
			continue
		}
		return pair.cert, pair.key, nil
	}
	return "", "", fmt.Errorf("evaluation: inference probe requires delegated app credentials via G8E_APP_CERT/G8E_APP_KEY or enrolled apps/g8ee cert")
}

func parseInferenceProbeRole(role string) (models.InferenceModelRole, error) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "primary":
		return models.InferenceModelRolePrimary, nil
	case "assistant":
		return models.InferenceModelRoleAssistant, nil
	case "lite", "light":
		return models.InferenceModelRoleLite, nil
	default:
		return models.InferenceModelRoleUnspecified, constants.ErrInferenceRoleInvalid
	}
}

func loadRegistryFreezeFile(path string) (*evaluation.ModelRegistryFreeze, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("evaluation: read registry file: %w", err)
	}
	var payload struct {
		CampaignID string                              `json:"campaign_id"`
		Digest     string                              `json:"model_registry_digest"`
		Variants   []*operatorv1.InferenceModelVariant `json:"variants"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("evaluation: decode registry file: %w", err)
	}
	if payload.CampaignID == "" || payload.Digest == "" || len(payload.Variants) == 0 {
		return nil, fmt.Errorf("evaluation: registry file: %w", constants.ErrInferenceModelRegistryInvalid)
	}
	return &evaluation.ModelRegistryFreeze{CampaignID: payload.CampaignID, Digest: payload.Digest, Variants: payload.Variants}, nil
}
