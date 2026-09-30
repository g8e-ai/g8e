// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const flagCatalog = "catalog"

func modelsEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "models",
		Aliases: []string{"model"},
		Short:   "Operate the model catalog and the frozen model registry",
		Long: `Model state lives in two named scopes.

  catalog   eval/base-model-inventory.json (checked in): models the project knows about
  registry  .g8e/eval/model-inventory.json (runtime): frozen models that runs bind to

Typical flow:
  g8e eval models pull qwen3:4b          pull onto the inference provider
  g8e eval models freeze                 provider -> registry
  g8e eval models import qwen3:4b        catalog -> registry
  g8e eval models diff                   compare catalog, registry, and provider`,
	}
	addLeaves(cmd,
		modelsListCmd(deps),
		modelsShowCmd(deps),
		modelsAddCmd(deps),
		modelsRemoveCmd(deps),
		modelsImportCmd(deps),
		modelsFreezeCmd(deps),
		modelsPullCmd(deps),
		modelsDiffCmd(deps),
	)
	return cmd
}

type modelRowJSON struct {
	ServedModelTag          string   `json:"served_model_tag"`
	VariantID               string   `json:"variant_id"`
	ModelDigest             string   `json:"model_digest"`
	ModelFamily             string   `json:"model_family,omitempty"`
	ParameterCount          uint64   `json:"parameter_count,omitempty"`
	FormattedParameterCount string   `json:"formatted_parameter_count,omitempty"`
	Quantization            string   `json:"quantization,omitempty"`
	Scopes                  []string `json:"scopes,omitempty"`
}

type modelListJSON struct {
	Variants []modelRowJSON `json:"variants"`
}

func modelRow(variant *evalv1.ModelVariant, scopes ...modelScope) modelRowJSON {
	row := modelRowJSON{
		ServedModelTag:          variant.GetServedModelTag(),
		VariantID:               variant.GetVariantId(),
		ModelDigest:             variant.GetModelDigest(),
		ModelFamily:             variant.GetModelFamily(),
		ParameterCount:          variant.GetParameterCount(),
		FormattedParameterCount: evaluation.FormatParameterCount(variant.GetParameterCount()),
		Quantization:            variant.GetQuantization(),
	}
	for _, scope := range scopes {
		row.Scopes = append(row.Scopes, string(scope))
	}
	return row
}

// scopedVariant is one variant with the scopes that hold it. Registry data
// wins over catalog data when both hold the variant.
type scopedVariant struct {
	Variant *evalv1.ModelVariant
	Scopes  []modelScope
}

func gatherScopedVariants(inventories modelInventories, cmd *cobra.Command, scope modelScope) ([]scopedVariant, error) {
	ctx := cmd.Context()
	byID := make(map[string]*scopedVariant)
	order := make([]string, 0)
	add := func(source modelScope) error {
		variants, err := inventories.variants(ctx, source)
		if err != nil {
			return fmt.Errorf("evaluation: models: read %s: %w", source, err)
		}
		for _, variant := range variants {
			if variant == nil {
				continue
			}
			entry, ok := byID[variant.GetVariantId()]
			if !ok {
				entry = &scopedVariant{}
				byID[variant.GetVariantId()] = entry
				order = append(order, variant.GetVariantId())
			}
			if entry.Variant == nil || source == modelScopeRegistry {
				entry.Variant = variant
			}
			entry.Scopes = append(entry.Scopes, source)
		}
		return nil
	}
	switch scope {
	case modelScopeCatalog, modelScopeRegistry:
		if err := add(scope); err != nil {
			return nil, err
		}
	default:
		if err := add(modelScopeCatalog); err != nil {
			return nil, err
		}
		if err := add(modelScopeRegistry); err != nil {
			return nil, err
		}
	}
	sort.Strings(order)
	result := make([]scopedVariant, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}
	return result, nil
}

func scopeLabel(scopes []modelScope) string {
	labels := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		labels = append(labels, string(scope))
	}
	return strings.Join(labels, "+")
}

func modelsListCmd(deps nativeEvalDeps) *cobra.Command {
	var scopeRaw string
	var detailed bool
	var selector ModelSelector
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List models in the catalog, the registry, or both",
		Long: `List models in the catalog, the registry, or both.

Examples:
  g8e eval models list
  g8e eval models list --scope registry --max-params 12b
  g8e eval models list --family gemma4 --detailed`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := parseModelScope(scopeRaw)
			if err != nil {
				return err
			}
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			entries, err := gatherScopedVariants(newModelInventories(fileSvc, cfg.ProjectRoot), cmd, scope)
			if err != nil {
				return err
			}
			variants := make([]*evalv1.ModelVariant, 0, len(entries))
			for _, entry := range entries {
				variants = append(variants, entry.Variant)
			}
			filtered, err := selector.Filter(variants)
			if err != nil {
				return err
			}
			keep := make(map[string]struct{}, len(filtered))
			for _, variant := range filtered {
				keep[variant.GetVariantId()] = struct{}{}
			}
			selected := entries[:0:0]
			for _, entry := range entries {
				if _, ok := keep[entry.Variant.GetVariantId()]; ok {
					selected = append(selected, entry)
				}
			}
			if output.JSONEnabled(cmd) {
				rows := make([]modelRowJSON, 0, len(selected))
				for _, entry := range selected {
					rows = append(rows, modelRow(entry.Variant, entry.Scopes...))
				}
				return output.WriteJSON(cmd.OutOrStdout(), modelListJSON{Variants: rows})
			}
			if len(selected) == 0 {
				cmd.Println("No models found")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			if detailed {
				_, _ = fmt.Fprintln(w, "TAG\tVARIANT ID\tSCOPE\tPARAMS\tFAMILY\tQUANT\tDIGEST")
			}
			for _, entry := range selected {
				v := entry.Variant
				if detailed {
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", v.GetServedModelTag(), v.GetVariantId(), scopeLabel(entry.Scopes),
						evaluation.FormatParameterCount(v.GetParameterCount()), v.GetModelFamily(), v.GetQuantization(), v.GetModelDigest())
					continue
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", v.GetServedModelTag(), v.GetVariantId(), scopeLabel(entry.Scopes))
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&scopeRaw, "scope", string(modelScopeAll), "Scope to list: catalog, registry, or all")
	cmd.Flags().BoolVarP(&detailed, "detailed", "d", false, "Show parameters, family, quantization, and digest")
	selector.bindFlags(cmd)
	return cmd
}

func modelsShowCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <model>",
		Short: "Show one model across the catalog and the registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			inventories := newModelInventories(fileSvc, cfg.ProjectRoot)
			id := strings.TrimSpace(args[0])
			found := make(map[modelScope]*evalv1.ModelVariant)
			for _, scope := range []modelScope{modelScopeCatalog, modelScopeRegistry} {
				variants, err := inventories.variants(cmd.Context(), scope)
				if err != nil {
					return fmt.Errorf("evaluation: models show: read %s: %w", scope, err)
				}
				matched, err := selectVariantsByID(variants, []string{id})
				if err == nil && len(matched) == 1 {
					found[scope] = matched[0]
				}
			}
			if len(found) == 0 {
				return fmt.Errorf("evaluation: models show: %w: %s", constants.ErrInferenceModelNotFound, id)
			}
			primary := found[modelScopeRegistry]
			if primary == nil {
				primary = found[modelScopeCatalog]
			}
			drift := found[modelScopeCatalog] != nil && found[modelScopeRegistry] != nil &&
				found[modelScopeCatalog].GetModelDigest() != found[modelScopeRegistry].GetModelDigest()
			scopes := make([]modelScope, 0, 2)
			for _, scope := range []modelScope{modelScopeCatalog, modelScopeRegistry} {
				if found[scope] != nil {
					scopes = append(scopes, scope)
				}
			}
			if output.JSONEnabled(cmd) {
				body, err := protojson.Marshal(primary)
				if err != nil {
					return err
				}
				payload := struct {
					Scopes      []string `json:"scopes"`
					DigestDrift bool     `json:"digest_drift"`
					Variant     any      `json:"variant"`
				}{DigestDrift: drift, Variant: json.RawMessage(body)}
				for _, scope := range scopes {
					payload.Scopes = append(payload.Scopes, string(scope))
				}
				return output.WriteJSON(cmd.OutOrStdout(), payload)
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Tag: %s\nVariant: %s\nProvider: %s\nFamily: %s\nParameters: %s\nQuantization: %s\nContext limit: %d\nScopes: %s\n",
				primary.GetServedModelTag(), primary.GetVariantId(), primary.GetProviderClass(), primary.GetModelFamily(),
				evaluation.FormatParameterCount(primary.GetParameterCount()), primary.GetQuantization(), primary.GetContextLimit(), scopeLabel(scopes))
			for _, scope := range scopes {
				_, _ = fmt.Fprintf(out, "Digest (%s): %s\n", scope, found[scope].GetModelDigest())
			}
			if drift {
				_, _ = fmt.Fprintln(out, "Digest drift: catalog and registry disagree")
			}
			for _, observation := range primary.GetCapabilityObservations() {
				_, _ = fmt.Fprintf(out, "Capability: %s\n", observation.String())
			}
			return nil
		},
	}
}

func modelsAddCmd(deps nativeEvalDeps) *cobra.Command {
	var family, parametersRaw, quant, digest, providerClass string
	var contextLimit uint32
	var catalog bool
	cmd := &cobra.Command{
		Use:   "add <tag>",
		Short: "Add or update one model in the registry (or the catalog with --catalog)",
		Long: `Add or update one model, recalculating the registry digest and matrix size.

Examples:
  g8e eval models add gemma4:12b --family gemma4 --parameters 12b
  g8e eval models add granite4.2:8b --family granite --parameters 8b --catalog`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tag := strings.TrimSpace(args[0])
			if tag == "" {
				return fmt.Errorf("evaluation: models add: %w", constants.ErrMissingRequiredField)
			}
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			var parameterCount uint64
			if parametersRaw != "" {
				if parameterCount, err = evaluation.ParseParameterCount(parametersRaw); err != nil {
					return err
				}
			}
			if digest == "" {
				sum := sha256.Sum256([]byte(providerClass + ":" + tag))
				digest = hex.EncodeToString(sum[:])
			}
			variant := &evalv1.ModelVariant{
				VariantId:      inference.NormalizeProviderModelVariantID(tag),
				ProviderClass:  providerClass,
				ServedModelTag: tag,
				ModelDigest:    digest,
				ModelFamily:    family,
				ParameterCount: parameterCount,
				Quantization:   quant,
				ContextLimit:   contextLimit,
			}
			scope := targetScope(catalog)
			inventories := newModelInventories(fileSvc, cfg.ProjectRoot)
			freeze, err := inventories.freezeOrEmpty(cmd.Context(), scope)
			if err != nil {
				return fmt.Errorf("evaluation: models add: %w", err)
			}
			updated, err := evaluation.AddOrUpdateModelVariant(freeze, variant)
			if err != nil {
				return fmt.Errorf("evaluation: models add: %w", err)
			}
			if err := inventories.save(cmd.Context(), scope, updated); err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), struct {
					Scope       string       `json:"scope"`
					Digest      string       `json:"model_registry_digest"`
					ModelCount  int          `json:"model_count"`
					Added       modelRowJSON `json:"added"`
					CellsPerRun uint64       `json:"homogeneous_cell_count"`
				}{string(scope), updated.RegistryDigest, len(updated.Variants), modelRow(variant, scope), updated.HomogeneousCellCount})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added %s (%s) to the %s (digest=%s, models=%d)\n",
				tag, variant.GetVariantId(), scope, updated.RegistryDigest, len(updated.Variants))
			return err
		},
	}
	cmd.Flags().StringVar(&family, "family", "", "Model family, for example gemma4, granite")
	cmd.Flags().StringVar(&parametersRaw, "parameters", "", "Parameter count, for example 12b, 700m, or a raw integer")
	cmd.Flags().StringVar(&quant, "quant", "Q4_K_M", "Quantization format")
	cmd.Flags().StringVar(&digest, "digest", "", "SHA-256 model digest (derived from provider and tag when omitted)")
	cmd.Flags().Uint32Var(&contextLimit, "context", 0, "Model context token limit")
	cmd.Flags().StringVar(&providerClass, "provider", "ollama", "Inference provider class")
	cmd.Flags().BoolVar(&catalog, flagCatalog, false, "Target the checked-in catalog instead of the registry")
	return cmd
}

func targetScope(catalog bool) modelScope {
	if catalog {
		return modelScopeCatalog
	}
	return modelScopeRegistry
}

func modelsRemoveCmd(deps nativeEvalDeps) *cobra.Command {
	var catalog bool
	var selector ModelSelector
	cmd := &cobra.Command{
		Use:     "remove <selector>",
		Aliases: []string{"rm"},
		Short:   "Remove models from the registry (or the catalog with --catalog)",
		Long: `Remove the selected models, recalculating the registry digest and matrix size.

Examples:
  g8e eval models remove glm-5.3-air
  g8e eval models remove --family granite --catalog`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			scope := targetScope(catalog)
			inventories := newModelInventories(fileSvc, cfg.ProjectRoot)
			freeze, err := inventories.load(cmd.Context(), scope)
			if err != nil {
				return fmt.Errorf("evaluation: models remove: %w", err)
			}
			removed, err := selector.withArgs(args).Resolve(freeze.Variants)
			if err != nil {
				return fmt.Errorf("evaluation: models remove: %w", err)
			}
			drop := make(map[string]struct{}, len(removed))
			for _, variant := range removed {
				drop[variant.GetVariantId()] = struct{}{}
			}
			remaining := make([]*evalv1.ModelVariant, 0, len(freeze.Variants))
			for _, variant := range freeze.Variants {
				if _, gone := drop[variant.GetVariantId()]; !gone {
					remaining = append(remaining, variant)
				}
			}
			var updated *evaluation.ModelInventoryFreeze
			if len(remaining) > 0 {
				if updated, err = evaluation.MaterializeModelRegistry(freeze.CampaignID, remaining); err != nil {
					return fmt.Errorf("evaluation: models remove: %w", err)
				}
			}
			if err := inventories.save(cmd.Context(), scope, updated); err != nil {
				return fmt.Errorf("evaluation: models remove: %w", err)
			}
			rows := make([]modelRowJSON, 0, len(removed))
			for _, variant := range removed {
				rows = append(rows, modelRow(variant, scope))
			}
			remainingCount, digest := 0, ""
			if updated != nil {
				remainingCount, digest = len(updated.Variants), updated.RegistryDigest
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), struct {
					Scope      string         `json:"scope"`
					Removed    []modelRowJSON `json:"removed"`
					ModelCount int            `json:"model_count"`
					Digest     string         `json:"model_registry_digest,omitempty"`
				}{string(scope), rows, remainingCount, digest})
			}
			for _, variant := range removed {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed %s (%s) from the %s\n", variant.GetServedModelTag(), variant.GetVariantId(), scope)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%d model(s) remain in the %s\n", remainingCount, scope)
			return err
		},
	}
	cmd.Flags().BoolVar(&catalog, flagCatalog, false, "Target the checked-in catalog instead of the registry")
	selector.bindFlags(cmd)
	return cmd
}

func modelsImportCmd(deps nativeEvalDeps) *cobra.Command {
	var selector ModelSelector
	cmd := &cobra.Command{
		Use:   "import <selector>",
		Short: "Import catalog models into the registry",
		Long: `Copy the selected catalog models into the registry, replacing entries with the same tag.

Examples:
  g8e eval models import qwen3:4b gemma3:4b
  g8e eval models import --family granite
  g8e eval models import --all --max-params 12b`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			inventories := newModelInventories(fileSvc, cfg.ProjectRoot)
			catalogFreeze, err := inventories.load(cmd.Context(), modelScopeCatalog)
			if err != nil {
				return fmt.Errorf("evaluation: models import: %w", err)
			}
			selected, err := selector.withArgs(args).Resolve(catalogFreeze.Variants)
			if err != nil {
				return fmt.Errorf("evaluation: models import: %w", err)
			}
			registry, err := inventories.freezeOrEmpty(cmd.Context(), modelScopeRegistry)
			if err != nil {
				return fmt.Errorf("evaluation: models import: %w", err)
			}
			var updated *evaluation.ModelInventoryFreeze
			if len(registry.Variants) == 0 {
				updated, err = evaluation.MaterializeModelRegistry(registry.CampaignID, selected)
			} else {
				updated, err = evaluation.MergeModelVariants(registry, selected)
			}
			if err != nil {
				return fmt.Errorf("evaluation: models import: %w", err)
			}
			if err := inventories.save(cmd.Context(), modelScopeRegistry, updated); err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				rows := make([]modelRowJSON, 0, len(selected))
				for _, variant := range selected {
					rows = append(rows, modelRow(variant, modelScopeRegistry))
				}
				return output.WriteJSON(cmd.OutOrStdout(), struct {
					Imported   []modelRowJSON `json:"imported"`
					ModelCount int            `json:"model_count"`
					Digest     string         `json:"model_registry_digest"`
				}{rows, len(updated.Variants), updated.RegistryDigest})
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Imported %d model(s) into the registry (digest=%s, models=%d)\n", len(selected), updated.RegistryDigest, len(updated.Variants))
			for _, variant := range selected {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  + %s\t%s\t(%s)\n", variant.GetServedModelTag(), evaluation.FormatParameterCount(variant.GetParameterCount()), variant.GetVariantId())
			}
			return nil
		},
	}
	selector.bindFlags(cmd)
	return cmd
}

func modelsFreezeCmd(deps nativeEvalDeps) *cobra.Command {
	var probeCapabilities bool
	cmd := &cobra.Command{
		Use:   "freeze",
		Short: "Freeze the live provider inventory into the registry",
		Long: `Query the governed inference provider and replace the registry with what it serves.

The registry then binds to the provider's exact model digests.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			env, err := resolveGovernedModelMaintenance(cmd, deps)
			if err != nil {
				return fmt.Errorf("evaluation: models freeze: %w", err)
			}
			opts := evaluation.ModelInventoryOptions{RunCapabilityProbes: probeCapabilities}
			if probeCapabilities {
				probeRunner, err := env.ResolveProbeRunner()
				if err != nil {
					return fmt.Errorf("evaluation: models freeze: %w", err)
				}
				opts.CapabilityProbeRunner = probeRunner
			}
			freeze, err := evaluation.FreezeModelInventoryFromProvider(cmd.Context(), env.ModelDispatcher, env.Maintenance, evaluation.DefaultGenesisHomogeneousCampaignID, opts)
			if err != nil {
				return fmt.Errorf("evaluation: models freeze: %w", err)
			}
			if err := newModelInventories(fileSvc, cfg.ProjectRoot).save(cmd.Context(), modelScopeRegistry, freeze); err != nil {
				return err
			}
			if output.JSONEnabled(cmd) {
				payload, err := modelInventoryFreezeJSON(freeze)
				if err != nil {
					return err
				}
				return output.WriteRawJSON(cmd.OutOrStdout(), payload)
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Registry digest: %s\nModels: %d\nHomogeneous matrix: %d cells (%d models x role-eligible scenario cells)\n",
				freeze.RegistryDigest, len(freeze.Variants), freeze.HomogeneousCellCount, len(freeze.Variants))
			for _, variant := range freeze.Variants {
				_, _ = fmt.Fprintf(out, "- %s (%s) %s\n", variant.GetServedModelTag(), variant.GetVariantId(), variant.GetModelDigest())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&probeCapabilities, "probe-capabilities", false, "Run bounded non-scored capability probes for each discovered model")
	return cmd
}

type modelDiffRow struct {
	ServedModelTag string `json:"served_model_tag"`
	Catalog        string `json:"catalog,omitempty"`
	Registry       string `json:"registry,omitempty"`
	Provider       string `json:"provider,omitempty"`
	Status         string `json:"status"`
}

type modelDiffJSON struct {
	InSync int            `json:"in_sync"`
	Rows   []modelDiffRow `json:"rows"`
}

// diffModelScopes compares served tag to digest across the three sources. The
// status names what differs: digest drift, or where the model is missing.
func diffModelScopes(catalog, registry, provider map[string]string) []modelDiffRow {
	tags := make(map[string]struct{})
	for _, source := range []map[string]string{catalog, registry, provider} {
		for tag := range source {
			tags[tag] = struct{}{}
		}
	}
	names := make([]string, 0, len(tags))
	for tag := range tags {
		names = append(names, tag)
	}
	sort.Strings(names)
	rows := make([]modelDiffRow, 0, len(names))
	for _, tag := range names {
		row := modelDiffRow{ServedModelTag: tag, Catalog: catalog[tag], Registry: registry[tag], Provider: provider[tag]}
		present := map[string]string{}
		var missing []string
		for label, source := range map[string]map[string]string{"catalog": catalog, "registry": registry, "provider": provider} {
			if digest, ok := source[tag]; ok {
				present[label] = digest
			} else {
				missing = append(missing, label)
			}
		}
		sort.Strings(missing)
		distinct := map[string]struct{}{}
		for _, digest := range present {
			distinct[digest] = struct{}{}
		}
		switch {
		case len(distinct) > 1:
			row.Status = "digest-drift"
		case len(missing) == 0:
			row.Status = "in-sync"
		case len(present) == 1:
			for label := range present {
				row.Status = "only-in-" + label
			}
		default:
			row.Status = "missing-from-" + strings.Join(missing, ",")
		}
		rows = append(rows, row)
	}
	return rows
}

func variantDigests(variants []*evalv1.ModelVariant) map[string]string {
	digests := make(map[string]string, len(variants))
	for _, variant := range variants {
		if variant != nil {
			digests[variant.GetServedModelTag()] = variant.GetModelDigest()
		}
	}
	return digests
}

func shortDigest(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	if digest == "" {
		return "-"
	}
	return digest
}

func modelsDiffCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare the catalog, the registry, and the live provider",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			inventories := newModelInventories(fileSvc, cfg.ProjectRoot)
			catalogVariants, err := inventories.variants(cmd.Context(), modelScopeCatalog)
			if err != nil {
				return fmt.Errorf("evaluation: models diff: read catalog: %w", err)
			}
			registryVariants, err := inventories.variants(cmd.Context(), modelScopeRegistry)
			if err != nil {
				return fmt.Errorf("evaluation: models diff: read registry: %w", err)
			}
			env, err := resolveGovernedModelMaintenance(cmd, deps)
			if err != nil {
				return fmt.Errorf("evaluation: models diff: %w", err)
			}
			entries, err := evaluation.ListOllamaProviderInventory(cmd.Context(), env.ModelDispatcher, env.Maintenance)
			if err != nil {
				return fmt.Errorf("evaluation: models diff: read provider: %w", err)
			}
			provider := make(map[string]string, len(entries))
			for _, entry := range entries {
				provider[entry.ServedModelTag] = entry.ModelDigest
			}
			rows := diffModelScopes(variantDigests(catalogVariants), variantDigests(registryVariants), provider)
			inSync := 0
			for _, row := range rows {
				if row.Status == "in-sync" {
					inSync++
				}
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), modelDiffJSON{InSync: inSync, Rows: rows})
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "TAG\tCATALOG\tREGISTRY\tPROVIDER\tSTATUS")
			for _, row := range rows {
				if row.Status == "in-sync" {
					continue
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", row.ServedModelTag, shortDigest(row.Catalog), shortDigest(row.Registry), shortDigest(row.Provider), row.Status)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%d model(s) in sync, %d differ\n", inSync, len(rows)-inSync)
			return err
		},
	}
	return cmd
}
