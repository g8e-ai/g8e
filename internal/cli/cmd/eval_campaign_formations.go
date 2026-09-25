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
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type formationSummaryJSON struct {
	ID                string `json:"id"`
	DisplayName       string `json:"display_name"`
	Description       string `json:"description"`
	EstimatedVRAMMiB  uint64 `json:"estimated_vram_mib"`
	MaxVRAMMiB        uint64 `json:"max_vram_mib"`
	DelegatedPrimary  bool   `json:"delegated_primary"`
	SovereignModelCount int  `json:"sovereign_model_count"`
}

type formationListJSON struct {
	Formations []formationSummaryJSON `json:"formations"`
}

type formationShowRoleJSON struct {
	Role            string `json:"role"`
	VariantID       string `json:"variant_id"`
	DisplayName     string `json:"display_name"`
	Provider        string `json:"provider"`
	Family          string `json:"family"`
	ServedModelTag  string `json:"served_model_tag"`
	Trust           string `json:"trust"`
	EstimatedVRAMMiB uint64 `json:"estimated_vram_mib"`
}

type formationShowJSON struct {
	ID               string                  `json:"id"`
	DisplayName      string                  `json:"display_name"`
	Description      string                  `json:"description"`
	EstimatedVRAMMiB uint64                  `json:"estimated_vram_mib"`
	MaxVRAMMiB       uint64                  `json:"max_vram_mib"`
	Roles            []formationShowRoleJSON `json:"roles"`
}

type formationRunRoleJSON struct {
	Role              string  `json:"role"`
	VariantID         string  `json:"variant_id"`
	ServedModelTag    string  `json:"served_model_tag"`
	ModelDigest       string  `json:"model_digest"`
	AttemptID         string  `json:"attempt_id"`
	AttestationStatus string  `json:"attestation_status"`
	PeakVRAMMiB       uint64  `json:"peak_vram_mib"`
	TokensPerSec      float64 `json:"generation_tokens_per_sec"`
}

type formationRunJSON struct {
	SchemaVersion        string                 `json:"schema_version"`
	FormationID          string                 `json:"formation_id"`
	Passed               bool                   `json:"passed"`
	PeakVRAMMiB          uint64                 `json:"peak_vram_mib"`
	MutationIntercepted  bool                   `json:"mutation_intercepted"`
	AllPolicyLayersValid bool                   `json:"all_policy_layers_valid"`
	InferenceSessionID   string                 `json:"inference_session_id"`
	CampaignID           string                 `json:"campaign_id"`
	RunID                string                 `json:"run_id"`
	Roles                []formationRunRoleJSON `json:"roles"`
}

type formationRunOptions struct {
	FormationID        string
	RegistryFile       string
	InventoryFile      string
	InferenceSessionID string
	DataSessionID      string
	OllamaEndpoint     string
	InitialState       string
	RunID              string
	AssignmentID       string
}

func campaignEvalFormationsCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "formations",
		Short: "List, inspect, and smoke-run execution-topology formations",
	}
	cmd.AddCommand(
		campaignEvalFormationsListCmd(deps),
		campaignEvalFormationsShowCmd(deps),
		campaignEvalFormationsRunCmd(deps),
	)
	return cmd
}

func campaignEvalFormationsListCmd(_ nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the checked-in execution-topology formation catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			topologies, err := evaluation.NewExecutionTopologies()
			if err != nil {
				return fmt.Errorf("evaluation: formations list: %w", err)
			}
			summaries := make([]formationSummaryJSON, 0)
			for _, formation := range topologies.Formations() {
				sovereignCount := 0
				if formation.Primary.Trust == evaluation.FormationTrustSovereign {
					sovereignCount++
				}
				if formation.Assistant.Trust == evaluation.FormationTrustSovereign {
					sovereignCount++
				}
				if formation.Lite.Trust == evaluation.FormationTrustSovereign {
					sovereignCount++
				}
				summaries = append(summaries, formationSummaryJSON{
					ID:                  formation.ID,
					DisplayName:         formation.DisplayName,
					Description:         formation.Description,
					EstimatedVRAMMiB:  formation.EstimatedVRAMMiB(),
					MaxVRAMMiB:          formation.MaxVRAMMiB,
					DelegatedPrimary:    formation.Primary.Trust == evaluation.FormationTrustDelegated,
					SovereignModelCount: sovereignCount,
				})
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(formationListJSON{Formations: summaries}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			for _, summary := range summaries {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%d MiB / %d MiB\tsovereign=%d delegated_primary=%t\n",
					summary.ID, summary.DisplayName, summary.EstimatedVRAMMiB, summary.MaxVRAMMiB, summary.SovereignModelCount, summary.DelegatedPrimary)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	return cmd
}

func campaignEvalFormationsShowCmd(_ nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <formation-id>",
		Short: "Show one catalog formation and its role bindings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			topologies, err := evaluation.NewExecutionTopologies()
			if err != nil {
				return fmt.Errorf("evaluation: formations show: %w", err)
			}
			formation, err := topologies.Formation(args[0])
			if err != nil {
				return fmt.Errorf("evaluation: formations show: %w", err)
			}
			roles := make([]formationShowRoleJSON, 0, 3)
			for _, role := range []evaluation.FormationRole{
				evaluation.FormationRolePrimary,
				evaluation.FormationRoleAssistant,
				evaluation.FormationRoleLite,
			} {
				model, err := formation.Model(role)
				if err != nil {
					return fmt.Errorf("evaluation: formations show: %w", err)
				}
				roles = append(roles, formationShowRoleJSON{
					Role:             string(role),
					VariantID:        model.VariantID,
					DisplayName:      model.DisplayName,
					Provider:         model.Provider,
					Family:           model.Family,
					ServedModelTag:   model.ServedModelTag,
					Trust:            string(model.Trust),
					EstimatedVRAMMiB: model.EstimatedModelVRAMMiB + model.EstimatedKVCacheMiB,
				})
			}
			payload := formationShowJSON{
				ID:               formation.ID,
				DisplayName:      formation.DisplayName,
				Description:      formation.Description,
				EstimatedVRAMMiB: formation.EstimatedVRAMMiB(),
				MaxVRAMMiB:       formation.MaxVRAMMiB,
				Roles:            roles,
			}
			if output.JSONEnabled(cmd) {
				body, err := json.MarshalIndent(payload, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(body))
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s — %s\n%s\nEstimated VRAM: %d MiB (budget %d MiB)\n",
				payload.DisplayName, payload.ID, payload.Description, payload.EstimatedVRAMMiB, payload.MaxVRAMMiB)
			if err != nil {
				return err
			}
			for _, role := range roles {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s (%s) trust=%s est=%d MiB\n",
					role.Role, role.ServedModelTag, role.VariantID, role.Trust, role.EstimatedVRAMMiB)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	return cmd
}

func campaignEvalFormationsRunCmd(deps nativeEvalDeps) *cobra.Command {
	opts := formationRunOptions{FormationID: "ultra-light-speedster"}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one frozen formation through the governed Inference Operator path",
		Long: `Execute Lite → Assistant → Primary for one catalog formation using frozen
registry digests and exact Operator sessions. This is a governed smoke path, not
a scored campaign assignment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.InferenceSessionID == "" {
				return fmt.Errorf("evaluation: formations run: --inference-session is required")
			}
			result, sessions, freeze, err := runFormationProductionFlow(cmd, deps, opts)
			if err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(formationRunResultJSON(result, sessions, freeze.CampaignID, opts.RunID), "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				if err != nil {
					return err
				}
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Formation %s %s\nSession: %s\nCampaign: %s\nRun: %s\nPeak VRAM: %d MiB\n",
					result.FormationID, passedLabel(result.Passed), sessions.InferenceSessionID, freeze.CampaignID, opts.RunID, result.PeakVRAMMiB)
				if err != nil {
					return err
				}
				for _, role := range result.Roles {
					_, err = fmt.Fprintf(cmd.OutOrStdout(), "  %s %s digest=%s attempt=%s\n",
						role.Role, role.Model.ServedModelTag, role.AttestationDigest, role.AttemptID)
					if err != nil {
						return err
					}
				}
			}
			if !result.Passed {
				return fmt.Errorf("evaluation: formations run: formation %s failed", result.FormationID)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.FormationID, "formation-id", opts.FormationID, "Catalog formation ID to execute")
	cmd.Flags().StringVar(&opts.RegistryFile, "registry-file", "", "Frozen inventory JSON from eval models freeze --output")
	cmd.Flags().StringVar(&opts.InventoryFile, "inventory-file", "", "Explicit inventory freeze path (runtime-relative or external)")
	cmd.Flags().StringVar(&opts.InferenceSessionID, "inference-session", "", "Exact inference Operator session ID (required)")
	cmd.Flags().StringVar(&opts.DataSessionID, "data-session", "", "Exact data Operator session ID for governed model release")
	cmd.Flags().StringVar(&opts.OllamaEndpoint, "ollama-endpoint", "", "Provider endpoint override for governed model release")
	cmd.Flags().StringVar(&opts.InitialState, "initial-state", "", "Opaque initial state bytes for the formation run")
	cmd.Flags().StringVar(&opts.RunID, "run-id", "", "Campaign run ID recorded on governed inference dispatches")
	cmd.Flags().StringVar(&opts.AssignmentID, "assignment-id", "", "Assignment ID recorded on governed inference dispatches")
	return cmd
}

func runFormationProductionFlow(cmd *cobra.Command, deps nativeEvalDeps, opts formationRunOptions) (*evaluation.FormationRunResult, campaignOperatorSessions, *evaluation.ModelInventoryFreeze, error) {
	cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, err
	}
	freeze, err := loadFormationInventoryFreeze(cmd, deps, opts.RegistryFile, opts.InventoryFile)
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	if len(freeze.Variants) == 0 || freeze.RegistryDigest == "" || freeze.CampaignID == "" {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", constants.ErrFormationRegistryBinding)
	}
	sessions, err := resolveCampaignOperatorSessions(cmd, deps, cfg, opts.InferenceSessionID, opts.DataSessionID)
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	if err := preflightProviderObservationDelivery(fileSvc, cfg); err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	modelBindings, err := evaluation.CampaignModelBindingsFromSpec(evalv1CampaignSpecFromFreeze(freeze))
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	if err := preflightCampaignModelProvenance(fileSvc, cfg, modelBindings); err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	binding, err := evaluation.FormationBindingFromCatalog(opts.FormationID, freeze.Variants)
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	chatDeps := chatEvalDeps{
		configLoader:   deps.configLoader,
		fileSvcFactory: deps.fileSvcFactory,
		authLoader:     deps.authLoader,
		clientFactory:  deps.clientFactory,
		now:            deps.now,
		newID:          deps.newID,
	}
	_, _, authContext, err := chatEvalEnvironment(cmd, chatDeps)
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	operators, err := chatEvalListOperators(cmd, chatDeps, cfg, authContext)
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	appClient, err := inferenceEvalAppClient(cfg, fileSvc, authContext, inferenceEvalDeps{
		configLoader:     deps.configLoader,
		fileSvcFactory:   deps.fileSvcFactory,
		authLoader:       deps.authLoader,
		clientFactory:    deps.clientFactory,
		appClientFactory: deps.clientFactory,
		now:              deps.now,
		newID:            deps.newID,
	})
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	dataOperator, err := evaluation.SelectCampaignDataOperator(operators, sessions.DataSessionID)
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	modelDispatcher, err := newHarnessOllamaModelCommandDispatcher(cfg, authContext, dataOperator, chatEvalDeps{
		configLoader:   deps.configLoader,
		fileSvcFactory: deps.fileSvcFactory,
		authLoader:     deps.authLoader,
		clientFactory:  deps.clientFactory,
		now:            deps.now,
		newID:          deps.newID,
	})
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	endpoint, err := resolveCampaignOllamaEndpoint(opts.OllamaEndpoint, operators, sessions.InferenceSessionID)
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	observationLoader, err := evaluation.NewCampaignFormationObservationLoader(fileSvc)
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	runID := opts.RunID
	if runID == "" {
		runID = "formation-" + deps.newID()
	}
	assignmentID := opts.AssignmentID
	if assignmentID == "" {
		assignmentID = "formation-assignment-" + deps.newID()
	}
	evaluationAttemptID := deps.newID()
	productionDeps := evaluation.FormationProductionDependencies{
		RunContext: evaluation.FormationRunContext{
			CampaignID:          freeze.CampaignID,
			RunID:               runID,
			AssignmentID:        assignmentID,
			EvaluationAttemptID: evaluationAttemptID,
			ModelRegistryDigest: freeze.RegistryDigest,
			InferenceSessionID:  sessions.InferenceSessionID,
			DataSessionID:       sessions.DataSessionID,
		},
		Variants:               freeze.Variants,
		ProvenancePreflight:    gatewayFormationProvenancePreflight{fileSvc: fileSvc, cfg: cfg},
		ObservationLoader:      observationLoader,
		InferenceDispatcher:    &harnessFormationInferenceDispatcher{client: appClient},
		ModelCommandDispatcher: modelDispatcher,
		OllamaEnvironment:      modelCommandEnvironment(endpoint),
		NewID:                  func(prefix string) string { return prefix + "-" + deps.newID() },
		Now:                    deps.now,
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Minute)
	defer cancel()
	result, err := evaluation.RunFormationProduction(ctx, binding, productionDeps, []byte(opts.InitialState))
	if err != nil {
		return nil, campaignOperatorSessions{}, nil, fmt.Errorf("evaluation: formations run: %w", err)
	}
	opts.RunID = runID
	return result, sessions, freeze, nil
}

func loadFormationInventoryFreeze(cmd *cobra.Command, deps nativeEvalDeps, registryFile, inventoryFile string) (*evaluation.ModelInventoryFreeze, error) {
	if strings.TrimSpace(registryFile) != "" {
		return evaluation.LoadModelInventoryFreezeFile(registryFile)
	}
	_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return nil, err
	}
	projectRoot, err := cmd.Flags().GetString("project-root")
	if err != nil {
		return nil, err
	}
	return loadEvaluationInventoryFreeze(cmd.Context(), fileSvc, projectRoot, inventoryFile)
}

func evalv1CampaignSpecFromFreeze(freeze *evaluation.ModelInventoryFreeze) *evalv1.EvaluationCampaignSpec {
	return &evalv1.EvaluationCampaignSpec{
		CampaignId:          freeze.CampaignID,
		ModelRegistryDigest: freeze.RegistryDigest,
		ModelRegistry:       freeze.Variants,
	}
}

type gatewayFormationProvenancePreflight struct {
	fileSvc fs.RuntimeFileService
	cfg     *config.Config
}

func (g gatewayFormationProvenancePreflight) PreflightSovereignModel(_ context.Context, model evaluation.FormationModel) (*evaluation.FormationAttestation, error) {
	if err := preflightModelProvenanceAttestation(g.fileSvc, g.cfg, model.ServedModelTag, model.ModelDigest); err != nil {
		return nil, err
	}
	return &evaluation.FormationAttestation{Verified: true, Digest: model.ModelDigest}, nil
}

type harnessFormationInferenceDispatcher struct {
	client *harnessclient.Client
}

func (d *harnessFormationInferenceDispatcher) DispatchInference(ctx context.Context, req *operatorv1.InferenceDispatchRequest) (*operatorv1.InferenceDispatchResponse, error) {
	if d == nil || d.client == nil || req == nil {
		return nil, fmt.Errorf("evaluation: formation inference dispatch: %w", constants.ErrMissingRequiredField)
	}
	response, _, err := d.client.DispatchInference(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("evaluation: formation inference dispatch: %w", err)
	}
	return response, nil
}

func formationRunResultJSON(result *evaluation.FormationRunResult, sessions campaignOperatorSessions, campaignID, runID string) formationRunJSON {
	roles := make([]formationRunRoleJSON, 0, len(result.Roles))
	for _, role := range result.Roles {
		roles = append(roles, formationRunRoleJSON{
			Role:              string(role.Role),
			VariantID:         role.Model.VariantID,
			ServedModelTag:    role.Model.ServedModelTag,
			ModelDigest:       role.Model.ModelDigest,
			AttemptID:         role.AttemptID,
			AttestationStatus: string(role.AttestationStatus),
			PeakVRAMMiB:       role.PeakVRAMMiB,
			TokensPerSec:      role.GenerationTokensPerSec,
		})
	}
	return formationRunJSON{
		SchemaVersion:        result.SchemaVersion,
		FormationID:          result.FormationID,
		Passed:               result.Passed,
		PeakVRAMMiB:          result.PeakVRAMMiB,
		MutationIntercepted:  result.MutationIntercepted,
		AllPolicyLayersValid: result.AllPolicyLayersValid,
		InferenceSessionID:   sessions.InferenceSessionID,
		CampaignID:           campaignID,
		RunID:                runID,
		Roles:                roles,
	}
}

func passedLabel(passed bool) string {
	if passed {
		return "PASSED"
	}
	return "FAILED"
}
