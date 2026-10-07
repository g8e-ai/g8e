// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package inference reads and writes the signed-in user's model settings: the
// primary, assistant, and lite role selections and the evaluation judge model.
package inference

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

const (
	settingsLLMGetSuffix = "/llm/get"
	settingsLLMSuffix    = "/llm"
)

// roleSelection is one role's provider and model as g8ee stores it.
type roleSelection struct {
	Provider *string `json:"provider,omitempty"`
	Model    *string `json:"model,omitempty"`
}

// roleUpdate is the request form of a role selection. A nil Provider clears
// the role (g8ee refuses to clear primary).
type roleUpdate struct {
	Provider *string `json:"provider"`
	Model    *string `json:"model,omitempty"`
}

type evalJudgeUpdate struct {
	Model string `json:"model"`
}

// settingsUpdateRequest is the body of POST /api/v1/settings/llm. The Gateway
// proxy adds the caller's request context. Provider connections (endpoints and
// API keys) are managed in the console and are never sent from here.
type settingsUpdateRequest struct {
	Primary   *roleUpdate      `json:"primary,omitempty"`
	Assistant *roleUpdate      `json:"assistant,omitempty"`
	Lite      *roleUpdate      `json:"lite,omitempty"`
	EvalJudge *evalJudgeUpdate `json:"eval_judge,omitempty"`
}

// settingsResponse is the part of the g8ee response this command prints.
// Provider options (which report only whether a key is set) are ignored.
type settingsResponse struct {
	Primary        roleSelection `json:"primary"`
	Assistant      roleSelection `json:"assistant"`
	Lite           roleSelection `json:"lite"`
	EvalJudgeModel *string       `json:"eval_judge_model,omitempty"`
}

// Cmd returns the inference command group.
func Cmd() *cobra.Command {
	return cmdWithConfig(shared.LoadConfig, authcmd.DefaultAPIClientFactory, shared.NewFileSvc)
}

func cmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inference",
		Short: "Show or set the signed-in user's model roles and eval judge",
		Long: `Show or set the model settings stored in the signed-in user's settings:
the primary, assistant, and lite roles and the evaluation judge model. These
are per-user settings, not Gateway platform settings. Provider endpoints and
API keys are managed in the console.`,
	}
	cmd.AddCommand(
		showCmd(configLoader, clientFactory, fileSvcFactory),
		setCmd(configLoader, clientFactory, fileSvcFactory),
	)
	return cmd
}

func showCmd(
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the primary, assistant, lite, and eval judge models",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient(configLoader, clientFactory, fileSvcFactory)
			if err != nil {
				return err
			}
			settings, err := post(client, settingsLLMGetSuffix, struct{}{})
			if err != nil {
				return err
			}
			return render(cmd, settings)
		},
	}
}

func setCmd(
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	var primary, assistant, lite, judge string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set the primary, assistant, lite, or eval judge model",
		Long: `Set one or more model roles for the signed-in user. A role value is
<provider>:<model>, split at the first colon, for example ollama:qwen3:1.7b or
g8e:qwen3:1.7b. Roles you do not pass are left unchanged.

An empty value clears assistant, lite, or --judge, so resolution falls back
(lite, then assistant, then primary; the judge falls back to lite). The
primary role cannot be cleared. The change applies on the next chat or eval
request; no restart is needed.`,
		Example: `  g8e inference set --primary g8e:qwen3:4b --lite g8e:qwen3:1.7b
  g8e inference set --judge qwen3:1.7b
  g8e inference set --lite ""`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := buildUpdate(cmd, primary, assistant, lite, judge)
			if err != nil {
				return err
			}
			client, err := newClient(configLoader, clientFactory, fileSvcFactory)
			if err != nil {
				return err
			}
			settings, err := post(client, settingsLLMSuffix, request)
			if err != nil {
				return err
			}
			return render(cmd, settings)
		},
	}
	cmd.Flags().StringVar(&primary, "primary", "", "Primary role as <provider>:<model>")
	cmd.Flags().StringVar(&assistant, "assistant", "", "Assistant role as <provider>:<model>; empty clears it")
	cmd.Flags().StringVar(&lite, "lite", "", "Lite role as <provider>:<model>; empty clears it")
	cmd.Flags().StringVar(&judge, "judge", "", "Eval judge model tag; empty clears it")
	return cmd
}

// buildUpdate turns the flags the caller actually passed into a request.
func buildUpdate(cmd *cobra.Command, primary, assistant, lite, judge string) (settingsUpdateRequest, error) {
	var request settingsUpdateRequest
	roles := []struct {
		flag   string
		value  string
		target **roleUpdate
	}{
		{"primary", primary, &request.Primary},
		{"assistant", assistant, &request.Assistant},
		{"lite", lite, &request.Lite},
	}
	for _, role := range roles {
		if !cmd.Flags().Changed(role.flag) {
			continue
		}
		update, err := parseRole(role.flag, role.value)
		if err != nil {
			return settingsUpdateRequest{}, err
		}
		*role.target = update
	}
	if cmd.Flags().Changed("judge") {
		request.EvalJudge = &evalJudgeUpdate{Model: strings.TrimSpace(judge)}
	}
	if request.Primary == nil && request.Assistant == nil && request.Lite == nil && request.EvalJudge == nil {
		return settingsUpdateRequest{}, fmt.Errorf("%w: pass at least one of --primary, --assistant, --lite, --judge", constants.ErrMissingRequiredField)
	}
	return request, nil
}

func parseRole(flag, value string) (*roleUpdate, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if flag == "primary" {
			return nil, fmt.Errorf("%w: --primary cannot be cleared", constants.ErrLLMRoleSelectionInvalid)
		}
		return &roleUpdate{}, nil
	}
	provider, model, ok := strings.Cut(value, ":")
	provider, model = strings.TrimSpace(provider), strings.TrimSpace(model)
	if !ok || provider == "" || model == "" {
		return nil, fmt.Errorf("%w: --%s %q must be <provider>:<model>", constants.ErrLLMRoleSelectionInvalid, flag, value)
	}
	return &roleUpdate{Provider: &provider, Model: &model}, nil
}

func newClient(
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) (authcmd.APIClient, error) {
	cfg, err := configLoader("")
	if err != nil {
		return nil, fmt.Errorf("inference: load config: %w", err)
	}
	fileSvc, err := fileSvcFactory("", slog.Default())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	client, err := clientFactory(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	return client, nil
}

func post(client authcmd.APIClient, suffix string, body any) (settingsResponse, error) {
	raw, err := client.Post(constants.APIPaths.EnsembleSettingsPrefix+suffix, body)
	if err != nil {
		return settingsResponse{}, fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	var settings settingsResponse
	if err := json.Unmarshal(raw, &settings); err != nil {
		return settingsResponse{}, fmt.Errorf("inference: decode settings response: %w", err)
	}
	return settings, nil
}

func render(cmd *cobra.Command, settings settingsResponse) error {
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		out, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return fmt.Errorf("inference: encode settings: %w", err)
		}
		cmd.Println(string(out))
		return nil
	}
	cmd.Printf("primary    %s\n", roleLabel(settings.Primary))
	cmd.Printf("assistant  %s\n", roleLabel(settings.Assistant))
	cmd.Printf("lite       %s\n", roleLabel(settings.Lite))
	judge := "(unset; uses the lite chain)"
	if settings.EvalJudgeModel != nil && *settings.EvalJudgeModel != "" {
		judge = *settings.EvalJudgeModel
	}
	cmd.Printf("judge      %s\n", judge)
	return nil
}

func roleLabel(role roleSelection) string {
	if role.Provider == nil || role.Model == nil || *role.Provider == "" || *role.Model == "" {
		return "(unset)"
	}
	return *role.Provider + ":" + *role.Model
}
