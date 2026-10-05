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
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/agent"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// canaryOptions selects what the live environment canaries run against.
type canaryOptions struct {
	Model       string
	EnsembleURL string
	NoAutoBind  bool
}

// environmentCanaryRunner runs the environment canaries and returns an error
// wrapping constants.ErrEvaluationEnvironmentCanaryFailed when the harness is
// broken. It is a seam so command tests need no live stack.
type environmentCanaryRunner func(cmd *cobra.Command, opts canaryOptions) error

func (d nativeEvalDeps) runEnvironmentCanaries(cmd *cobra.Command, opts canaryOptions) error {
	if d.canaryRunner != nil {
		return d.canaryRunner(cmd, opts)
	}
	return runLiveEnvironmentCanaries(cmd, d, opts)
}

// chatProbeModel is the frozen registry identity a chat request is built with.
type chatProbeModel struct {
	Digest         string
	RegistryDigest string
	Registry       []*operatorv1.InferenceModelVariant
}

// resolveChatProbeModel looks the served model tag up in the runtime model
// registry and freezes a one-model registry for campaignID, the same way a
// campaign does, so a gate request carries the model digest and the
// campaign-bound registry digest the Gateway checks it against.
func resolveChatProbeModel(ctx context.Context, fileSvc fs.RuntimeFileService, projectRoot, campaignID, model string) (chatProbeModel, error) {
	variants, err := newModelInventories(fileSvc, projectRoot).variants(ctx, modelScopeRegistry)
	if err != nil {
		return chatProbeModel{}, fmt.Errorf("evaluation: resolve model %q: %w", model, err)
	}
	for _, variant := range variants {
		if variant.GetServedModelTag() != model {
			continue
		}
		freeze, err := evaluation.MaterializeModelRegistry(campaignID, []*evalv1.ModelVariant{variant})
		if err != nil {
			return chatProbeModel{}, fmt.Errorf("evaluation: resolve model %q: %w", model, err)
		}
		return chatProbeModel{
			Digest:         variant.GetModelDigest(),
			RegistryDigest: freeze.RegistryDigest,
			Registry:       freeze.InferenceVariants,
		}, nil
	}
	return chatProbeModel{}, fmt.Errorf("evaluation: model %q is not in the model registry (run `g8e eval models import %s`): %w", model, model, constants.ErrInferenceModelNotFound)
}

func runLiveEnvironmentCanaries(cmd *cobra.Command, deps nativeEvalDeps, opts canaryOptions) error {
	chatDeps := deps.chatDeps()
	cfg, fileSvc, authContext, err := chatEvalEnvironment(cmd, chatDeps)
	if err != nil {
		return err
	}
	operators, err := chatEvalListOperators(cmd, chatDeps, cfg, authContext)
	if err != nil {
		return err
	}
	inference, err := evaluation.SelectInferenceOperator(operators, "")
	if err != nil {
		return fmt.Errorf("evaluation: environment canaries: %w", err)
	}
	data, err := operatorcapability.SelectDataOperator(operators)
	if err != nil {
		return fmt.Errorf("evaluation: environment canaries: %w", err)
	}
	authContext, err = chatEvalBindDataOperator(cmd, chatDeps, cfg, fileSvc, authContext, operators, data.OperatorSessionID, !opts.NoAutoBind)
	if err != nil {
		return fmt.Errorf("evaluation: environment canaries: %w", err)
	}
	projectRoot, err := cmd.Flags().GetString("project-root")
	if err != nil {
		return fmt.Errorf("evaluation: environment canaries: read project root: %w", err)
	}
	model, err := resolveChatProbeModel(cmd.Context(), fileSvc, projectRoot, evaluation.EnvironmentCanaryCampaignID, opts.Model)
	if err != nil {
		return err
	}
	ensembleClient, err := chatEvalEnsembleClient(cfg, authContext, resolveChatEvalEnsembleURL(opts.EnsembleURL), chatDeps)
	if err != nil {
		return err
	}
	gatewayClient, err := chatDeps.clientFactory(nativeEvalClientConfig(cfg, authContext))
	if err != nil {
		return fmt.Errorf("evaluation: environment canaries: initialize gateway client: %w", err)
	}
	persona := harnessclient.Persona{
		ID:                "g8e-environment-canary",
		UserAgent:         "g8e-eval-environment-canary",
		UserID:            authContext.UserID,
		CLISessionID:      authContext.CLISessionID,
		OperatorID:        data.OperatorID,
		OperatorSessionID: data.OperatorSessionID,
	}
	lane := evaluation.NewCommandLane(gatewayClient, evaluation.NewStore(fileSvc), persona, 0, 0)
	reporter := newChatAcceptReporter(cmd.OutOrStdout(), output.JSONEnabled(cmd))
	report, runErr := evaluation.RunEnvironmentCanaries(cmd.Context(), evaluation.CanaryDeps{
		Chat: &campaignChatHarnessClient{client: ensembleClient},
		WaitForTrace: func(ctx context.Context, fetch func(context.Context) (evaluation.EvaluationTrace, error)) (evaluation.EvaluationTrace, error) {
			return chatEvalWaitForTrace(ctx, fetch, reporter)
		},
		WorkspaceWriter:              lane,
		WorkspaceReader:              lane,
		MCP:                          &gatewayCanaryMCP{client: gatewayClient, persona: persona},
		Persona:                      persona,
		DataOperatorID:               data.OperatorID,
		DataOperatorSessionID:        data.OperatorSessionID,
		DataOperatorWorkingDirectory: data.WorkingDirectory,
		Probe: evaluation.ChatProbeRequest{
			Model:                   opts.Model,
			ModelDigest:             model.Digest,
			ModelRegistryDigest:     model.RegistryDigest,
			ModelRegistry:           model.Registry,
			TargetOperatorSessionID: inference.OperatorSessionID,
		},
		NewID: adaptNewID(deps.newID),
	})
	return reportEnvironmentCanaries(cmd, report, runErr)
}

// reportEnvironmentCanaries prints every canary result and, when any failed,
// returns an error naming the failed canaries so the caller stops before it
// scores a model on a broken harness.
func reportEnvironmentCanaries(cmd *cobra.Command, report evaluation.CanaryReport, runErr error) error {
	var failed []string
	for _, result := range report.Results {
		if result.Passed {
			if !output.JSONEnabled(cmd) {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "CANARY PASS %s: %s\n", result.ID, result.Detail)
			}
			continue
		}
		failed = append(failed, string(result.ID))
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "ENVIRONMENT ERROR (%s): %s\n", result.ID, result.Detail)
	}
	if len(failed) > 0 {
		return fmt.Errorf("evaluation: environment canaries failed (%s): %w", strings.Join(failed, ", "), constants.ErrEvaluationEnvironmentCanaryFailed)
	}
	return runErr
}

// gatewayCanaryMCP is the live MCP verifier: the shared agent-registry
// verification plus a Gateway /mcp tools/list.
type gatewayCanaryMCP struct {
	client  *harnessclient.Client
	persona harnessclient.Persona
}

func (m *gatewayCanaryMCP) VerifyAgentIntegrations(context.Context) error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	return agent.VerifyAllIsolated(binaryPath)
}

func (m *gatewayCanaryMCP) GatewayToolNames(ctx context.Context) ([]string, error) {
	response, err := m.client.MCPToolsList(ctx, m.persona)
	if err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, fmt.Errorf("tools/list error %d: %s", response.Error.Code, response.Error.Message)
	}
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("decode tools/list result: %w", err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names, nil
}
