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
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gwremote"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type formationSummaryJSON struct {
	ID                  string `json:"id"`
	DisplayName         string `json:"display_name"`
	Description         string `json:"description"`
	EstimatedVRAMMiB    uint64 `json:"estimated_vram_mib"`
	MaxVRAMMiB          uint64 `json:"max_vram_mib"`
	DelegatedPrimary    bool   `json:"delegated_primary"`
	SovereignModelCount int    `json:"sovereign_model_count"`
}

type formationListJSON struct {
	Formations []formationSummaryJSON `json:"formations"`
}

type formationShowRoleJSON struct {
	Role             string `json:"role"`
	VariantID        string `json:"variant_id"`
	DisplayName      string `json:"display_name"`
	Provider         string `json:"provider"`
	Family           string `json:"family"`
	ServedModelTag   string `json:"served_model_tag"`
	Trust            string `json:"trust"`
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

func extractModelName(tag string) string {
	parts := strings.Split(tag, ":")
	if len(parts) > 0 {
		base := parts[0]
		base = strings.TrimPrefix(base, "hf.co/")
		base = strings.TrimPrefix(base, "milkey/")
		base = strings.TrimPrefix(base, "kitsonk/")
		base = strings.TrimPrefix(base, "Impulse2000/")
		base = strings.TrimPrefix(base, "Randomblock1/")
		if idx := strings.LastIndex(base, "/"); idx != -1 {
			base = base[idx+1:]
		}
		return base
	}
	return tag
}

func formationsListEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the checked-in execution-topology formation catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(cmd)
			if err != nil {
				return err
			}
			topologies, err := newFormationCatalog(projectRoot).topologies()
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
					EstimatedVRAMMiB:    formation.EstimatedVRAMMiB(),
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
			type tableRow struct {
				id          string
				displayName string
				primary     string
				assistant   string
				lite        string
				vram        string
			}
			var rows []tableRow
			for i, formation := range topologies.Formations() {
				summary := summaries[i]
				primary, _ := formation.Model(evaluation.FormationRolePrimary)
				assistant, _ := formation.Model(evaluation.FormationRoleAssistant)
				lite, _ := formation.Model(evaluation.FormationRoleLite)
				primaryStr := fmt.Sprintf("%s (%s)", primary.ServedModelTag, primary.Provider)
				assistantStr := fmt.Sprintf("%s (%s)", assistant.ServedModelTag, assistant.Provider)
				liteStr := fmt.Sprintf("%s (%s)", lite.ServedModelTag, lite.Provider)
				vramStr := fmt.Sprintf("%d/%d", summary.EstimatedVRAMMiB, summary.MaxVRAMMiB)
				displayName := summary.DisplayName
				if strings.HasPrefix(summary.ID, "heterogeneous-") {
					displayName = fmt.Sprintf("%s / %s / %s",
						extractModelName(primary.ServedModelTag),
						extractModelName(assistant.ServedModelTag),
						extractModelName(lite.ServedModelTag))
				}
				rows = append(rows, tableRow{
					id:          summary.ID,
					displayName: displayName,
					primary:     primaryStr,
					assistant:   assistantStr,
					lite:        liteStr,
					vram:        vramStr,
				})
			}
			widths := [6]int{
				len("Formation ID"),
				len("Display Name"),
				len("Primary"),
				len("Assistant"),
				len("Lite"),
				len("VRAM"),
			}
			for _, row := range rows {
				if len(row.id) > widths[0] {
					widths[0] = len(row.id)
				}
				if len(row.displayName) > widths[1] {
					widths[1] = len(row.displayName)
				}
				if len(row.primary) > widths[2] {
					widths[2] = len(row.primary)
				}
				if len(row.assistant) > widths[3] {
					widths[3] = len(row.assistant)
				}
				if len(row.lite) > widths[4] {
					widths[4] = len(row.lite)
				}
				if len(row.vram) > widths[5] {
					widths[5] = len(row.vram)
				}
			}
			headerFmt := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds\n",
				widths[0], widths[1], widths[2], widths[3], widths[4], widths[5])
			separatorFmt := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds\n",
				widths[0], widths[1], widths[2], widths[3], widths[4], widths[5])
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), headerFmt,
				"Formation ID", "Display Name", "Primary", "Assistant", "Lite", "VRAM")
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), separatorFmt,
				strings.Repeat("-", widths[0]), strings.Repeat("-", widths[1]),
				strings.Repeat("-", widths[2]), strings.Repeat("-", widths[3]),
				strings.Repeat("-", widths[4]), strings.Repeat("-", widths[5]))
			for _, row := range rows {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), headerFmt,
					row.id, row.displayName, row.primary, row.assistant, row.lite, row.vram)
			}
			return nil
		},
	}
	return jsonLeaf(cmd)
}

func formationsShowEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <formation-id>",
		Short: "Show one catalog formation and its role bindings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(cmd)
			if err != nil {
				return err
			}
			topologies, err := newFormationCatalog(projectRoot).topologies()
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
	return jsonLeaf(cmd)
}

// formationRoleFlagSet binds the CLI flags for one formation role (primary,
// assistant, or lite) to an evaluation.FormationModel.
type formationRoleFlagSet struct {
	displayName   string
	provider      string
	family        string
	providerClass string
	servedTag     string
	trust         string
	quant         string
	parameters    string
	modelVRAM     uint64
	kvCacheVRAM   uint64
	modelDigest   string
}

func bindFormationRoleFlags(cmd *cobra.Command, prefix, label string) *formationRoleFlagSet {
	f := &formationRoleFlagSet{}
	cmd.Flags().StringVar(&f.displayName, prefix+"-display-name", "", label+" display name")
	cmd.Flags().StringVar(&f.provider, prefix+"-provider", "", label+" provider, for example Alibaba")
	cmd.Flags().StringVar(&f.family, prefix+"-family", "", label+" model family, for example \"Qwen 3.5\"")
	cmd.Flags().StringVar(&f.providerClass, prefix+"-provider-class", "ollama", label+" inference provider class")
	cmd.Flags().StringVar(&f.servedTag, prefix+"-tag", "", label+" served model tag, for example qwen3.5:9b (required)")
	cmd.Flags().StringVar(&f.trust, prefix+"-trust", "sovereign", label+" trust class: sovereign or delegated")
	cmd.Flags().StringVar(&f.quant, prefix+"-quant", "", label+" quantization format, required for sovereign roles (e.g. Q4_K_M)")
	cmd.Flags().StringVar(&f.parameters, prefix+"-parameters", "", label+" parameter count, for example 9b")
	cmd.Flags().Uint64Var(&f.modelVRAM, prefix+"-model-vram", 0, label+" estimated model VRAM MiB (sovereign only)")
	cmd.Flags().Uint64Var(&f.kvCacheVRAM, prefix+"-kv-cache-vram", 0, label+" estimated KV-cache VRAM MiB (sovereign only)")
	cmd.Flags().StringVar(&f.modelDigest, prefix+"-digest", "", label+" model digest (resolved against the frozen registry at run time when omitted)")
	return f
}

func (f *formationRoleFlagSet) toFormationModel() (evaluation.FormationModel, error) {
	tag := strings.TrimSpace(f.servedTag)
	if tag == "" {
		return evaluation.FormationModel{}, fmt.Errorf("served model tag is required: %w", constants.ErrMissingRequiredField)
	}
	variantID := inference.NormalizeProviderModelVariantID(tag)
	var parameterCount uint64
	if strings.TrimSpace(f.parameters) != "" {
		count, err := evaluation.ParseParameterCount(f.parameters)
		if err != nil {
			return evaluation.FormationModel{}, err
		}
		parameterCount = count
	}
	return evaluation.FormationModel{
		VariantID:             variantID,
		DisplayName:           f.displayName,
		Provider:              f.provider,
		Family:                f.family,
		ProviderClass:         f.providerClass,
		ServedModelTag:        tag,
		Trust:                 evaluation.FormationTrust(strings.ToLower(strings.TrimSpace(f.trust))),
		Quantization:          f.quant,
		ParameterCount:        parameterCount,
		EstimatedModelVRAMMiB: f.modelVRAM,
		EstimatedKVCacheMiB:   f.kvCacheVRAM,
		ModelDigest:           f.modelDigest,
	}, nil
}

func formationsAddEvalCmd(deps nativeEvalDeps) *cobra.Command {
	var displayName, description string
	var maxVRAM uint64
	var relaxedValidation bool
	var primaryFlags, assistantFlags, liteFlags *formationRoleFlagSet
	cmd := &cobra.Command{
		Use:   "add <formation-id>",
		Short: "Add or update one formation in the checked-in catalog overlay",
		Long: `Add a new formation to the catalog, or replace an existing entry with the same
ID, writing the change to the project's eval/formation-catalog-overlay.json.
The formation must define all three roles.

Example:
  g8e eval formations add my-formation \
    --display-name "My Formation" --description "Qwen primary with light triage." \
    --primary-tag qwen3.5:9b --primary-provider Alibaba --primary-family "Qwen 3.5" \
    --primary-quant Q4_K_M --primary-parameters 9b --primary-model-vram 5632 --primary-kv-cache-vram 512 \
    --assistant-tag ministral-3:3b --assistant-provider Mistral --assistant-family "Mistral 3" \
    --assistant-quant Q4_K_M --assistant-parameters 3.8b --assistant-model-vram 2560 --assistant-kv-cache-vram 512 \
    --lite-tag gemma3:1b --lite-provider Google --lite-family "Gemma 3" \
    --lite-quant Q4_K_M --lite-parameters 1b --lite-model-vram 768 --lite-kv-cache-vram 256`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return fmt.Errorf("evaluation: formations add: %w", constants.ErrMissingRequiredField)
			}
			primary, err := primaryFlags.toFormationModel()
			if err != nil {
				return fmt.Errorf("evaluation: formations add: primary role: %w", err)
			}
			assistant, err := assistantFlags.toFormationModel()
			if err != nil {
				return fmt.Errorf("evaluation: formations add: assistant role: %w", err)
			}
			lite, err := liteFlags.toFormationModel()
			if err != nil {
				return fmt.Errorf("evaluation: formations add: lite role: %w", err)
			}
			if maxVRAM == 0 {
				maxVRAM = evaluation.FormationMaxVRAMMiB
			}
			formation := evaluation.Formation{
				ID:                id,
				DisplayName:       displayName,
				Description:       description,
				MaxVRAMMiB:        maxVRAM,
				RelaxedValidation: relaxedValidation,
				Primary:           primary,
				Assistant:         assistant,
				Lite:              lite,
			}

			projectRoot, err := resolveProjectRoot(cmd)
			if err != nil {
				return err
			}
			catalog := newFormationCatalog(projectRoot)
			topologies, err := catalog.topologies()
			if err != nil {
				return fmt.Errorf("evaluation: formations add: %w", err)
			}
			_, existErr := topologies.Formation(id)
			existed := existErr == nil

			updated, err := evaluation.AddOrUpdateFormation(topologies, formation)
			if err != nil {
				return fmt.Errorf("evaluation: formations add: %w", err)
			}
			overlay, err := catalog.overlay()
			if err != nil {
				return fmt.Errorf("evaluation: formations add: %w", err)
			}
			upsertOverlayFormation(overlay, formation)
			if err := catalog.save(overlay); err != nil {
				return fmt.Errorf("evaluation: formations add: %w", err)
			}

			verb := "Added"
			if existed {
				verb = "Updated"
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), struct {
					ID             string `json:"id"`
					Action         string `json:"action"`
					FormationCount int    `json:"formation_count"`
				}{id, strings.ToLower(verb), len(updated.Formations())})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s formation %s (catalog now has %d formations)\n", verb, id, len(updated.Formations()))
			return err
		},
	}
	cmd.Flags().StringVar(&displayName, "display-name", "", "Formation display name")
	cmd.Flags().StringVar(&description, "description", "", "Formation description")
	cmd.Flags().Uint64Var(&maxVRAM, "max-vram", 0, "Maximum estimated VRAM budget in MiB (defaults to the catalog budget)")
	cmd.Flags().BoolVar(&relaxedValidation, "relaxed-validation", false, "Skip the provider/family overlap and VRAM budget gates")
	primaryFlags = bindFormationRoleFlags(cmd, "primary", "Primary")
	assistantFlags = bindFormationRoleFlags(cmd, "assistant", "Assistant")
	liteFlags = bindFormationRoleFlags(cmd, "lite", "Lite")
	return jsonLeaf(cmd)
}

func formationsRemoveEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <formation-id>",
		Aliases: []string{"rm"},
		Short:   "Remove one formation from the catalog",
		Long: `Remove a formation the project overlay added, or record a checked-in default
formation ID as removed. The catalog can never be left empty.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return fmt.Errorf("evaluation: formations remove: %w", constants.ErrMissingRequiredField)
			}
			defaults, err := evaluation.NewExecutionTopologies()
			if err != nil {
				return fmt.Errorf("evaluation: formations remove: %w", err)
			}

			projectRoot, err := resolveProjectRoot(cmd)
			if err != nil {
				return err
			}
			catalog := newFormationCatalog(projectRoot)
			topologies, err := catalog.topologies()
			if err != nil {
				return fmt.Errorf("evaluation: formations remove: %w", err)
			}
			updated, _, err := evaluation.RemoveFormation(topologies, id)
			if err != nil {
				return fmt.Errorf("evaluation: formations remove: %w", err)
			}
			overlay, err := catalog.overlay()
			if err != nil {
				return fmt.Errorf("evaluation: formations remove: %w", err)
			}
			removeOverlayFormation(defaults, overlay, id)
			if err := catalog.save(overlay); err != nil {
				return fmt.Errorf("evaluation: formations remove: %w", err)
			}

			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), struct {
					ID             string `json:"id"`
					FormationCount int    `json:"formation_count"`
				}{id, len(updated.Formations())})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed formation %s (catalog now has %d formations)\n", id, len(updated.Formations()))
			return err
		},
	}
	return jsonLeaf(cmd)
}

// upsertOverlayFormation replaces the overlay entry matching formation.ID, or
// appends one, and un-removes that ID if a prior removal recorded it.
func upsertOverlayFormation(overlay *evaluation.FormationCatalogOverlay, formation evaluation.Formation) {
	replaced := false
	for i, existing := range overlay.Formations {
		if existing.ID == formation.ID {
			overlay.Formations[i] = formation
			replaced = true
			break
		}
	}
	if !replaced {
		overlay.Formations = append(overlay.Formations, formation)
	}
	remaining := make([]string, 0, len(overlay.RemovedFormationIDs))
	for _, removedID := range overlay.RemovedFormationIDs {
		if removedID != formation.ID {
			remaining = append(remaining, removedID)
		}
	}
	overlay.RemovedFormationIDs = remaining
}

// removeOverlayFormation drops id from the overlay's own formations and, when
// id also names a checked-in default, records it as removed so the default
// does not resurface once the overlay entry is gone.
func removeOverlayFormation(defaults *evaluation.ExecutionTopologies, overlay *evaluation.FormationCatalogOverlay, id string) {
	formations := make([]evaluation.Formation, 0, len(overlay.Formations))
	for _, existing := range overlay.Formations {
		if existing.ID != id {
			formations = append(formations, existing)
		}
	}
	overlay.Formations = formations
	if _, err := defaults.Formation(id); err == nil {
		for _, removedID := range overlay.RemovedFormationIDs {
			if removedID == id {
				return
			}
		}
		overlay.RemovedFormationIDs = append(overlay.RemovedFormationIDs, id)
	}
}

func formationsSmokeEvalCmd(deps nativeEvalDeps) *cobra.Command {
	var runAll bool
	var initialState string
	cmd := &cobra.Command{
		Use:   "smoke [formations...]",
		Short: "Run one or all formations through the governed Inference Operator path",
		Long: `Execute Lite → Assistant → Primary for one or more catalog formations using frozen
registry digests and exact Operator sessions. Use --all to batch-run every sovereign
execution topology in sequence. This is a governed smoke path, not a scored campaign assignment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := resolveOperatorSessions(cmd, deps, operatorRoleInference, operatorRoleData)
			if err != nil {
				return fmt.Errorf("evaluation: formations smoke: %w", err)
			}
			// Smoke runs bind against the checked-in catalog only (see
			// FormationBindingFromCatalog); overlay-added or -updated
			// formations are not yet runnable through the governed path.
			topologies, err := evaluation.NewExecutionTopologies()
			if err != nil {
				return fmt.Errorf("evaluation: formations smoke: %w", err)
			}

			var formations []string
			if runAll {
				for _, f := range topologies.Formations() {
					formations = append(formations, f.ID)
				}
			} else {
				formations = args
				if len(formations) == 0 {
					return fmt.Errorf("evaluation: formations smoke: specify formation IDs or use --all: %w", constants.ErrEvaluationFlagsInvalid)
				}
			}

			type smokeSummaryItem struct {
				FormationID string `json:"formation_id"`
				Passed      bool   `json:"passed"`
				PeakVRAMMiB uint64 `json:"peak_vram_mib"`
				Error       string `json:"error,omitempty"`
			}
			var batchResults []formationRunJSON
			var summaries []smokeSummaryItem
			hasFailure := false

			for _, fID := range formations {
				if !output.JSONEnabled(cmd) {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "=== START Formation %s ===\n", fID)
				}
				result, runID, err := runFormationSmoke(cmd, deps, sessions, fID, initialState)
				if err != nil {
					hasFailure = true
					summaries = append(summaries, smokeSummaryItem{
						FormationID: fID,
						Passed:      false,
						Error:       err.Error(),
					})
					if !output.JSONEnabled(cmd) {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "FAIL Formation %s: %v\n", fID, err)
					}
					continue
				}
				batchResults = append(batchResults, formationRunResultJSON(result, sessions, runID))
				summaries = append(summaries, smokeSummaryItem{
					FormationID: fID,
					Passed:      result.Passed,
					PeakVRAMMiB: result.PeakVRAMMiB,
				})
				if !result.Passed {
					hasFailure = true
				}
				if !output.JSONEnabled(cmd) {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Formation %s %s (Peak VRAM: %d MiB)\n",
						result.FormationID, passedLabel(result.Passed), result.PeakVRAMMiB)
					for _, role := range result.Roles {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s %s digest=%s attempt=%s\n",
							role.Role, role.Model.ServedModelTag, role.AttestationDigest, role.AttemptID)
					}
				}
			}

			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(batchResults, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				if err != nil {
					return err
				}
			} else if len(formations) > 1 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "\n--- Formation Batch Summary ---")
				for _, s := range summaries {
					status := "PASS"
					if !s.Passed {
						status = "FAIL"
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-25s %s (Peak VRAM: %d MiB)\n", s.FormationID, status, s.PeakVRAMMiB)
				}
			}

			if hasFailure {
				return fmt.Errorf("evaluation: formations smoke: one or more formations failed: %w", constants.ErrEvaluationFormationSmokeFailed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&runAll, "all", false, "Run all sovereign formations in the catalog in sequence")
	cmd.Flags().StringVar(&initialState, "initial-state", "", "Opaque initial state bytes for the formation run")
	return jsonLeaf(cmd)
}

func runFormationSmoke(cmd *cobra.Command, deps nativeEvalDeps, sessions operatorSessions, formationID, initialState string) (*evaluation.FormationRunResult, string, error) {
	cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return nil, "", err
	}
	freeze, err := loadFormationInventoryFreeze(cmd, deps, "", "")
	if err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	if len(freeze.Variants) == 0 || freeze.RegistryDigest == "" || freeze.CampaignID == "" {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", constants.ErrFormationRegistryBinding)
	}
	if err := gwremote.PreflightProviderObservationDelivery(fileSvc, cfg); err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	binding, err := evaluation.FormationBindingFromCatalog(formationID, freeze.Variants)
	if err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	formation, err := evaluation.BindFormation(binding)
	if err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	if err := gwremote.PreflightCampaignModelProvenance(fileSvc, cfg, evaluation.CampaignModelBindingsFromFormation(formation)); err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	chatDeps := deps.chatDeps()
	_, _, authContext, err := chatEvalEnvironment(cmd, chatDeps)
	if err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	operators, err := chatEvalListOperators(cmd, chatDeps, cfg, authContext)
	if err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	appClient, err := inferenceEvalAppClient(fileSvc, cfg, authContext, deps.clientFactory)
	if err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	dataOperator, err := operatorcapability.SelectDataOperator(operators)
	if err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	modelDispatcher, err := newHarnessOllamaModelCommandDispatcher(cfg, authContext, dataOperator, chatDeps)
	if err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	endpoint, err := resolveCampaignOllamaEndpoint(operators, sessions.InferenceSessionID)
	if err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	observationLoader, err := gwremote.NewCampaignFormationObservationLoader(fileSvc, cfg)
	if err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	runID := "formation-" + deps.newID()
	assignmentID := "formation-assignment-" + deps.newID()
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
	result, err := evaluation.RunFormationProduction(ctx, binding, productionDeps, []byte(initialState))
	if err != nil {
		return nil, "", fmt.Errorf("evaluation: formations smoke: %w", err)
	}
	return result, runID, nil
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

type gatewayFormationProvenancePreflight struct {
	fileSvc fs.RuntimeFileService
	cfg     *config.Config
}

func (g gatewayFormationProvenancePreflight) PreflightSovereignModel(_ context.Context, model evaluation.FormationModel) (*evaluation.FormationAttestation, error) {
	window, err := gwremote.LoadModelProvenanceAttestation(g.fileSvc, g.cfg, model.ServedModelTag, model.ModelDigest)
	if err != nil {
		if preflightErr := gwremote.PreflightModelProvenanceAttestation(g.fileSvc, g.cfg, model.ServedModelTag, model.ModelDigest); preflightErr != nil {
			return nil, preflightErr
		}
		return &evaluation.FormationAttestation{Verified: true, Digest: model.ModelDigest}, nil
	}
	return &evaluation.FormationAttestation{Verified: true, Digest: model.ModelDigest, Window: window}, nil
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

func formationRunResultJSON(result *evaluation.FormationRunResult, sessions operatorSessions, runID string) formationRunJSON {
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

type campaignFormationProductionOptions struct {
	InferenceSessionID string
	DataSessionID      string
}

// buildCampaignFormationProductionDeps wires the governed provenance,
// observation, allocation, and release dependencies every campaign formation
// run uses, whichever runner executes the roles.
func buildCampaignFormationProductionDeps(
	cmd *cobra.Command,
	deps nativeEvalDeps,
	cfg *config.Config,
	fileSvc fs.RuntimeFileService,
	authContext *auth.ClientAuthContext,
	dataOperator *operatorcapability.DataOperatorStatus,
	operators []models.OperatorDocumentGo,
	variants []*evalv1.ModelVariant,
	registryDigest string,
	opts campaignFormationProductionOptions,
) (evaluation.FormationProductionDependencies, error) {
	appClient, err := inferenceEvalAppClient(fileSvc, cfg, authContext, deps.clientFactory)
	if err != nil {
		return evaluation.FormationProductionDependencies{}, fmt.Errorf("evaluation: campaign formation dependencies: %w", err)
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
		return evaluation.FormationProductionDependencies{}, fmt.Errorf("evaluation: campaign formation dependencies: %w", err)
	}
	endpoint, err := resolveCampaignOllamaEndpoint(operators, opts.InferenceSessionID)
	if err != nil {
		return evaluation.FormationProductionDependencies{}, fmt.Errorf("evaluation: campaign formation dependencies: %w", err)
	}
	observationLoader, err := gwremote.NewCampaignFormationObservationLoader(fileSvc, cfg)
	if err != nil {
		return evaluation.FormationProductionDependencies{}, fmt.Errorf("evaluation: campaign formation dependencies: %w", err)
	}
	return evaluation.FormationProductionDependencies{
		Variants:               variants,
		ProvenancePreflight:    gatewayFormationProvenancePreflight{fileSvc: fileSvc, cfg: cfg},
		ObservationLoader:      observationLoader,
		InferenceDispatcher:    &harnessFormationInferenceDispatcher{client: appClient},
		ModelCommandDispatcher: modelDispatcher,
		OllamaEnvironment:      modelCommandEnvironment(endpoint),
		NewID:                  func(prefix string) string { return prefix + "-" + deps.newID() },
		Now:                    deps.now,
		RunContext: evaluation.FormationRunContext{
			ModelRegistryDigest: registryDigest,
			InferenceSessionID:  opts.InferenceSessionID,
			DataSessionID:       opts.DataSessionID,
		},
	}, nil
}
