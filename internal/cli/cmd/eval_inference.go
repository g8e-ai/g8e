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
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type inferenceEvalDeps struct {
	configLoader     func(string) (*config.Config, error)
	fileSvcFactory   func(string, *slog.Logger) (fs.RuntimeFileService, error)
	authLoader       func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error)
	clientFactory    func(harnessconfig.Config) (*harnessclient.Client, error)
	appClientFactory func(harnessconfig.Config) (*harnessclient.Client, error)
	now              func() time.Time
	newID            func() string
}

func inferenceEvalCmd(deps nativeEvalDeps) *cobra.Command {
	shared := inferenceEvalDeps{
		configLoader:     deps.configLoader,
		fileSvcFactory:   deps.fileSvcFactory,
		authLoader:       deps.authLoader,
		clientFactory:    deps.clientFactory,
		appClientFactory: deps.clientFactory,
		now:              deps.now,
		newID:            deps.newID,
	}
	cmd := &cobra.Command{Use: "inference", Short: "Inspect and probe the campaign Inference Operator"}
	cmd.AddCommand(
		inferenceEvalStatusCmd(shared),
		inferenceEvalFreezeRegistryCmd(shared),
		inferenceEvalProbeCmd(shared),
		inferenceEvalAcceptCmd(shared),
	)
	return cmd
}

func inferenceEvalStatusCmd(deps inferenceEvalDeps) *cobra.Command {
	var operatorSessionID string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Verify that an inference-capable Operator is enrolled and active",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, authContext, client, err := inferenceEvalGatewayClient(cmd, deps)
			if err != nil {
				return err
			}
			operators, _, err := client.ListOperators(cmd.Context())
			if err != nil {
				return fmt.Errorf("evaluation: list operators: %w", err)
			}
			selected, err := evaluation.SelectInferenceOperator(operators, operatorSessionID)
			if err != nil {
				return fmt.Errorf("evaluation: inference operator status: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]string{
					"operator_id":         selected.OperatorID,
					"operator_session_id": selected.OperatorSessionID,
					"status":              selected.Status,
					"gateway":             cfg.OperatorHTTPURL(),
					"user_id":             authContext.UserID,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Inference operator active\nOperator ID: %s\nSession ID: %s\nGateway: %s\nUser: %s\n", selected.OperatorID, selected.OperatorSessionID, cfg.OperatorHTTPURL(), authContext.UserID)
			return err
		},
	}
	cmd.Flags().StringVar(&operatorSessionID, "operator-session", "", "Pin the status check to one exact inference Operator session")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}

func inferenceEvalFreezeRegistryCmd(deps inferenceEvalDeps) *cobra.Command {
	var campaignID string
	var ollamaEndpoint string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "freeze-registry",
		Short: "Freeze the campaign model registry from a live Ollama inventory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if campaignID == "" {
				return fmt.Errorf("evaluation: freeze registry: %w", constants.ErrMissingRequiredField)
			}
			if ollamaEndpoint == "" {
				ollamaEndpoint = os.Getenv("G8E_OLLAMA_ENDPOINT")
			}
			if ollamaEndpoint == "" {
				return fmt.Errorf("evaluation: freeze registry: set --ollama-endpoint or G8E_OLLAMA_ENDPOINT")
			}
			freeze, err := evaluation.FreezeModelRegistryFromProvider(cmd.Context(), ollamaEndpoint, campaignID, slog.Default())
			if err != nil {
				return fmt.Errorf("evaluation: freeze registry: %w", err)
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"campaign_id":           freeze.CampaignID,
					"model_registry_digest": freeze.Digest,
					"model_count":           len(freeze.Variants),
					"variants":              freeze.Variants,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Campaign: %s\nRegistry digest: %s\nModels: %d\n", freeze.CampaignID, freeze.Digest, len(freeze.Variants)); err != nil {
				return err
			}
			for _, variant := range freeze.Variants {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- %s %s\n", variant.GetModel(), variant.GetDigest()); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Frozen evaluation campaign ID")
	cmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Approved remote Ollama endpoint (default: G8E_OLLAMA_ENDPOINT)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON registry freeze")
	return cmd
}

func inferenceEvalProbeCmd(deps inferenceEvalDeps) *cobra.Command {
	var operatorSessionID string
	var model string
	var role string
	var campaignID string
	var registryDigest string
	var registryFile string
	var prompt string
	var seed int32 = -1
	var stream bool
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "probe",
		Short: "Run one non-scored governed inference probe through the exact Inference Operator session",
		RunE: func(cmd *cobra.Command, args []string) error {
			if model == "" {
				return fmt.Errorf("evaluation: inference probe: --model is required")
			}
			selected, probeReq, appClient, err := inferenceEvalPrepareProbe(cmd, deps, operatorSessionID, model, role, campaignID, registryDigest, registryFile, prompt, seed, stream)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 6*time.Minute)
			defer cancel()
			response, progress, err := inferenceEvalExecuteProbe(ctx, appClient, probeReq)
			if err != nil {
				return fmt.Errorf("evaluation: inference probe: %w", err)
			}
			if err := evaluation.ValidateInferenceProbeStream(probeReq, progress, response); err != nil {
				return fmt.Errorf("evaluation: inference probe: %w", err)
			}
			if jsonOutput {
				body, err := protojson.Marshal(response)
				if err != nil {
					return fmt.Errorf("evaluation: inference probe: marshal response: %w", err)
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return err
			}
			result := response.GetResult()
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Probe accepted\nSession: %s\nModel: %s\nAttempt: %s\nResult digest: %s\nOutput hash: %s\nParts: %d\nProgress events: %d\n", selected.OperatorSessionID, result.GetRequestedModel(), result.GetProviderAttemptId(), result.GetResultDigest(), result.GetOutputHash(), len(result.GetParts()), len(progress))
			return err
		},
	}
	cmd.Flags().StringVar(&operatorSessionID, "operator-session", "", "Pin the probe to one exact inference Operator session")
	cmd.Flags().StringVar(&model, "model", "", "Requested provider model tag")
	cmd.Flags().StringVar(&role, "role", "primary", "Governed model role: primary, assistant, or lite")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Frozen evaluation campaign ID for campaign-mode probes")
	cmd.Flags().StringVar(&registryDigest, "registry-digest", "", "Frozen campaign model registry digest")
	cmd.Flags().StringVar(&registryFile, "registry-file", "", "JSON file produced by eval inference freeze-registry --json")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Probe prompt (default: Reply with exactly: probe-ok)")
	cmd.Flags().Int32Var(&seed, "seed", -1, "Optional deterministic generation seed (omit for provider default)")
	cmd.Flags().BoolVar(&stream, "stream", false, "Request live NDJSON progress telemetry during generation")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit canonical InferenceDispatchResponse protojson")
	return cmd
}

func inferenceEvalAcceptCmd(deps inferenceEvalDeps) *cobra.Command {
	var operatorSessionID string
	var model string
	var role string
	var campaignID string
	var registryDigest string
	var registryFile string
	var casesCSV string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "accept",
		Short: "Run the Phase 1A inference-only vertical acceptance matrix through the exact Inference Operator session",
		RunE: func(cmd *cobra.Command, args []string) error {
			if model == "" {
				return fmt.Errorf("evaluation: inference accept: --model is required")
			}
			caseIDs, err := parseInferenceAcceptanceCases(casesCSV)
			if err != nil {
				return err
			}
			cases, err := evaluation.SelectInferenceAcceptanceCases(caseIDs)
			if err != nil {
				return fmt.Errorf("evaluation: inference accept: %w", err)
			}
			selected, baseReq, appClient, err := inferenceEvalPrepareProbe(cmd, deps, operatorSessionID, model, role, campaignID, registryDigest, registryFile, "", -1, false)
			if err != nil {
				return err
			}
			results := make([]map[string]any, 0, len(cases))
			failures := 0
			for _, acceptanceCase := range cases {
				probeReq := acceptanceCase.Apply(baseReq)
				probeReq.ProviderAttemptID = deps.newID()
				probeReq.Stream = acceptanceCase.Stream
				ctx, cancel := context.WithTimeout(cmd.Context(), 6*time.Minute)
				response, progress, runErr := inferenceEvalExecuteProbe(ctx, appClient, probeReq)
				cancel()
				entry := map[string]any{
					"case":             string(acceptanceCase.ID),
					"provider_attempt": probeReq.ProviderAttemptID,
					"stream":           probeReq.Stream,
				}
				if runErr != nil {
					entry["status"] = "failed"
					entry["error"] = runErr.Error()
					failures++
				} else if err := evaluation.ValidateInferenceProbeStream(probeReq, progress, response); err != nil {
					entry["status"] = "failed"
					entry["error"] = err.Error()
					failures++
				} else if err := evaluation.ValidateInferenceAcceptanceCase(acceptanceCase.ID, response); err != nil {
					entry["status"] = "failed"
					entry["error"] = err.Error()
					failures++
				} else {
					entry["status"] = "passed"
					entry["progress_events"] = len(progress)
					entry["result_digest"] = response.GetResult().GetResultDigest()
				}
				results = append(results, entry)
				if jsonOutput {
					continue
				}
				status := entry["status"].(string)
				if status == "passed" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "PASS %s\n", acceptanceCase.ID)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "FAIL %s: %s\n", acceptanceCase.ID, entry["error"])
				}
			}
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"operator_session_id": selected.OperatorSessionID,
					"model":               model,
					"passed":              len(cases) - failures,
					"failed":              failures,
					"cases":               results,
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
				return fmt.Errorf("evaluation: inference accept: %d case(s) failed", failures)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&operatorSessionID, "operator-session", "", "Pin acceptance to one exact inference Operator session")
	cmd.Flags().StringVar(&model, "model", "", "Requested provider model tag")
	cmd.Flags().StringVar(&role, "role", "primary", "Default governed model role for cases that do not override it")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Frozen evaluation campaign ID for campaign-mode acceptance")
	cmd.Flags().StringVar(&registryDigest, "registry-digest", "", "Frozen campaign model registry digest")
	cmd.Flags().StringVar(&registryFile, "registry-file", "", "JSON file produced by eval inference freeze-registry --json")
	cmd.Flags().StringVar(&casesCSV, "cases", "", "Comma-separated case IDs (default: full Phase 1A inference matrix)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON acceptance report")
	return cmd
}

func inferenceEvalPrepareProbe(
	cmd *cobra.Command,
	deps inferenceEvalDeps,
	operatorSessionID, model, role, campaignID, registryDigest, registryFile, prompt string,
	seed int32,
	stream bool,
) (*evaluation.InferenceOperatorStatus, evaluation.InferenceProbeRequest, *harnessclient.Client, error) {
	cfg, fileSvc, authContext, listClient, err := inferenceEvalGatewayClient(cmd, deps)
	if err != nil {
		return nil, evaluation.InferenceProbeRequest{}, nil, err
	}
	operators, _, err := listClient.ListOperators(cmd.Context())
	if err != nil {
		return nil, evaluation.InferenceProbeRequest{}, nil, fmt.Errorf("evaluation: list operators: %w", err)
	}
	selected, err := evaluation.SelectInferenceOperator(operators, operatorSessionID)
	if err != nil {
		return nil, evaluation.InferenceProbeRequest{}, nil, err
	}
	appClient, err := inferenceEvalAppClient(cfg, fileSvc, authContext, deps)
	if err != nil {
		return nil, evaluation.InferenceProbeRequest{}, nil, err
	}
	probeRole, err := parseInferenceProbeRole(role)
	if err != nil {
		return nil, evaluation.InferenceProbeRequest{}, nil, err
	}
	var registry []*operatorv1.InferenceModelVariant
	modelDigest := ""
	if registryFile != "" {
		freeze, err := loadRegistryFreezeFile(registryFile)
		if err != nil {
			return nil, evaluation.InferenceProbeRequest{}, nil, err
		}
		campaignID = freeze.CampaignID
		registryDigest = freeze.Digest
		registry = freeze.Variants
		variant, err := freeze.LookupModelVariant(model)
		if err != nil {
			return nil, evaluation.InferenceProbeRequest{}, nil, fmt.Errorf("evaluation: lookup model variant: %w", err)
		}
		modelDigest = variant.GetDigest()
	}
	probeReq := evaluation.InferenceProbeRequest{
		ProviderAttemptID:       deps.newID(),
		Role:                    probeRole,
		Model:                   model,
		ModelDigest:             modelDigest,
		TargetOperatorSessionID: selected.OperatorSessionID,
		Prompt:                  prompt,
		CampaignID:              campaignID,
		ModelRegistryDigest:     registryDigest,
		ModelRegistry:           registry,
		Stream:                  stream,
	}
	if seed >= 0 {
		probeReq.Seed = &seed
	}
	return selected, probeReq, appClient, nil
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
