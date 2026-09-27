// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func modelsEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "Discover, freeze, and materialize evaluation model inventories",
	}
	cmd.AddCommand(
		modelsEvalAddCmd(deps),
		modelsEvalFreezeCmd(deps),
		modelsEvalImportCmd(deps),
		modelsEvalListCmd(deps),
		modelsEvalMaterializeCmd(deps),
		modelsEvalStageCmd(deps),
	)
	return cmd
}

func modelsEvalFreezeCmd(deps nativeEvalDeps) *cobra.Command {
	var campaignID string
	var inferenceSessionID string
	var dataSessionID string
	var outputPath string
	var probeCapabilities bool
	cmd := &cobra.Command{
		Use:   "freeze",
		Short: "Freeze the model registry from a live provider inventory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if campaignID == "" {
				return fmt.Errorf("evaluation: inventory freeze: %w", constants.ErrMissingRequiredField)
			}
			maintenanceEnv, err := resolveGovernedModelMaintenance(cmd, deps, inferenceSessionID, dataSessionID)
			if err != nil {
				return fmt.Errorf("evaluation: inventory freeze: %w", err)
			}
			opts := evaluation.ModelInventoryOptions{RunCapabilityProbes: probeCapabilities}
			if probeCapabilities {
				opts.CapabilityProbeRunner = maintenanceEnv.ProbeRunner
			}
			freeze, err := evaluation.FreezeModelInventoryFromProvider(cmd.Context(), maintenanceEnv.ModelDispatcher, maintenanceEnv.Maintenance, campaignID, opts)
			if err != nil {
				return fmt.Errorf("evaluation: inventory freeze: %w", err)
			}
			if outputPath != "" {
				if err := writeModelInventoryFreezeFile(outputPath, freeze); err != nil {
					return err
				}
			}
			if output.JSONEnabled(cmd) {
				payload, err := modelInventoryFreezeJSON(freeze)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Campaign: %s\nRegistry digest: %s\nModel variants: %d\nHomogeneous matrix: %d cells (%d variants × %d roles × %d scenarios)\n",
				freeze.CampaignID,
				freeze.RegistryDigest,
				len(freeze.Variants),
				freeze.HomogeneousCellCount,
				len(freeze.Variants),
				evaluation.HomogeneousRoleCount,
				evaluation.StandardScenarioCount,
			); err != nil {
				return err
			}
			for _, variant := range freeze.Variants {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s) %s\n", variant.GetServedModelTag(), variant.GetVariantId(), variant.GetModelDigest()); err != nil {
					return err
				}
			}
			if outputPath != "" {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "\nWrote inventory freeze to %s\n", outputPath)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "\nCampaign: %s\nRegistry digest: %s\n", freeze.CampaignID, freeze.RegistryDigest)
			return err
		},
	}
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Frozen evaluation campaign ID")
	cmd.Flags().StringVar(&inferenceSessionID, "inference-session", "", "Exact inference Operator session ID (required)")
	cmd.Flags().StringVar(&dataSessionID, "data-session", "", "Exact data Operator session ID (required)")
	cmd.Flags().StringVar(&outputPath, "output", "", "Write the inventory freeze JSON to this path")
	cmd.Flags().BoolVar(&probeCapabilities, "probe-capabilities", false, "Run bounded non-scored capability probes for each discovered model")
	return cmd
}

func modelsEvalListCmd(deps nativeEvalDeps) *cobra.Command {
	var fromPath string
	var maxParamsStr string
	var family string
	var detailed bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List model variants from a frozen inventory file",
		Long: `List model variants from a frozen inventory file.

Examples:
  g8e eval models list
  g8e eval models list --max-parameters 12b
  g8e eval models list --family gemma4 --detailed
  g8e eval models list --params 8b -d`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			variants, err := loadEvaluationInventoryVariants(cmd.Context(), fileSvc, cfg.ProjectRoot, fromPath)
			if err != nil {
				return fmt.Errorf("evaluation: inventory list: %w", err)
			}
			if maxParamsStr != "" {
				maxParams, err := evaluation.ParseParameterCount(maxParamsStr)
				if err != nil {
					return err
				}
				variants = evaluation.FilterVariantsByMaxParameters(variants, maxParams)
			}
			if family != "" {
				variants = evaluation.FilterVariantsByFamily(variants, family)
			}
			if output.JSONEnabled(cmd) {
				rows := make([]modelInventoryVariantJSON, 0, len(variants))
				for _, variant := range variants {
					rows = append(rows, modelInventoryVariantJSON{
						ServedModelTag:          variant.GetServedModelTag(),
						VariantID:               variant.GetVariantId(),
						ModelDigest:             variant.GetModelDigest(),
						ModelFamily:             variant.GetModelFamily(),
						ParameterCount:          variant.GetParameterCount(),
						FormattedParameterCount: evaluation.FormatParameterCount(variant.GetParameterCount()),
						Quantization:            variant.GetQuantization(),
					})
				}
				payload, err := json.MarshalIndent(modelInventoryListJSON{Variants: rows}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}
			if detailed {
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				_, _ = fmt.Fprintln(w, "TAG\tVARIANT ID\tPARAMS\tFAMILY\tQUANT\tDIGEST")
				for _, variant := range variants {
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
						variant.GetServedModelTag(),
						variant.GetVariantId(),
						evaluation.FormatParameterCount(variant.GetParameterCount()),
						variant.GetModelFamily(),
						variant.GetQuantization(),
						variant.GetModelDigest(),
					)
				}
				return w.Flush()
			}
			for _, variant := range variants {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", variant.GetServedModelTag(), variant.GetVariantId(), variant.GetModelDigest()); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "Frozen inventory JSON path (default: runtime freeze when present, else eval/base-model-inventory.json)")
	cmd.Flags().StringVar(&maxParamsStr, "max-parameters", "", "Filter variants to parameter count <= threshold (e.g. 12b, 8b)")
	cmd.Flags().StringVar(&maxParamsStr, "params", "", "Alias for --max-parameters")
	cmd.Flags().StringVar(&family, "family", "", "Filter variants by model family (e.g. gemma4, granite)")
	cmd.Flags().BoolVarP(&detailed, "detailed", "d", false, "Show detailed table with parameters, family, quantization, and digest")
	return cmd
}

func modelsEvalAddCmd(deps nativeEvalDeps) *cobra.Command {
	var tag string
	var family string
	var paramsRaw string
	var variantID string
	var quant string
	var digest string
	var contextLimit uint32
	var providerClass string
	var toPath string
	var syncBase bool
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add or update a model variant in the model inventory",
		Long: `Add or update a model variant in the model inventory freeze, recalculating
the registry digest and homogeneous matrix size.

Examples:
  g8e eval models add --tag gemma4:12b --family gemma4 --params 12b
  g8e eval models add --tag granite4.2:8b --family granite --params 8b --sync-base
  g8e eval models add --tag qwen3.5:9b --family qwen35 --params 9b --to eval/base-model-inventory.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(tag) == "" {
				return fmt.Errorf("evaluation: models add: --tag is required")
			}
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			tag = strings.TrimSpace(tag)
			if variantID == "" {
				variantID = inference.NormalizeProviderModelVariantID(tag)
			}
			if providerClass == "" {
				providerClass = "ollama"
			}
			if quant == "" {
				quant = "Q4_K_M"
			}
			if digest == "" {
				h := sha256.Sum256([]byte(providerClass + ":" + tag))
				digest = hex.EncodeToString(h[:])
			}
			var paramCount uint64
			if paramsRaw != "" {
				var parseErr error
				paramCount, parseErr = evaluation.ParseParameterCount(paramsRaw)
				if parseErr != nil {
					return parseErr
				}
			}

			newVariant := &evalv1.ModelVariant{
				VariantId:      variantID,
				ProviderClass:  providerClass,
				ServedModelTag: tag,
				ModelDigest:    digest,
				ModelFamily:    family,
				ParameterCount: paramCount,
				Quantization:   quant,
				ContextLimit:   contextLimit,
			}

			targetPath := toPath
			if targetPath == "" {
				runtimeFull := filepath.Join(cfg.ProjectRoot, constants.RuntimeDirname, evaluation.DefaultModelInventoryRelPath)
				if exists, _ := fileSvc.FileExists(cmd.Context(), evaluation.DefaultModelInventoryRelPath); exists {
					targetPath = runtimeFull
				} else {
					targetPath = filepath.Join(cfg.ProjectRoot, evaluation.DefaultBaseModelInventoryRelPath)
				}
			} else if !filepath.IsAbs(targetPath) {
				targetPath = filepath.Join(cfg.ProjectRoot, targetPath)
			}

			freeze, err := evaluation.LoadModelInventoryFreezeFile(targetPath)
			if err != nil {
				return fmt.Errorf("evaluation: models add: load target freeze: %w", err)
			}

			updatedFreeze, err := evaluation.AddOrUpdateModelVariant(freeze, newVariant)
			if err != nil {
				return fmt.Errorf("evaluation: models add: %w", err)
			}

			if err := writeModelInventoryFreezeFile(targetPath, updatedFreeze); err != nil {
				return err
			}

			if syncBase {
				basePath := filepath.Join(cfg.ProjectRoot, evaluation.DefaultBaseModelInventoryRelPath)
				if targetPath != basePath {
					baseFreeze, baseErr := evaluation.LoadModelInventoryFreezeFile(basePath)
					if baseErr == nil {
						if updatedBase, err := evaluation.AddOrUpdateModelVariant(baseFreeze, newVariant); err == nil {
							_ = writeModelInventoryFreezeFile(basePath, updatedBase)
						}
					}
				}
				runtimePath := filepath.Join(cfg.ProjectRoot, constants.RuntimeDirname, evaluation.DefaultModelInventoryRelPath)
				if targetPath != runtimePath {
					if exists, _ := fileSvc.FileExists(cmd.Context(), evaluation.DefaultModelInventoryRelPath); exists {
						runtimeFreeze, rErr := evaluation.LoadModelInventoryFreezeFile(runtimePath)
						if rErr == nil {
							if updatedRuntime, err := evaluation.AddOrUpdateModelVariant(runtimeFreeze, newVariant); err == nil {
								_ = writeModelInventoryFreezeFile(runtimePath, updatedRuntime)
							}
						}
					}
				}
			}

			if output.JSONEnabled(cmd) {
				payload, err := protojson.Marshal(newVariant)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}

			paramsDisplay := ""
			if paramCount > 0 {
				paramsDisplay = fmt.Sprintf(" (%s)", evaluation.FormatParameterCount(paramCount))
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added %s%s [family=%s, variant_id=%s] to %s (digest=%s, models=%d)\n",
				tag, paramsDisplay, family, variantID, filepath.Base(targetPath), updatedFreeze.RegistryDigest, len(updatedFreeze.Variants))
			return err
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "Served model tag, e.g. gemma4:12b (required)")
	cmd.Flags().StringVar(&family, "family", "", "Model family, e.g. gemma4, granite, qwen35")
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Parameter count, e.g. 12b, 8b, 3b, 700m, or raw integer")
	cmd.Flags().StringVar(&paramsRaw, "parameter-count", "", "Alias for --params")
	cmd.Flags().StringVar(&variantID, "variant-id", "", "Variant ID (defaults to normalized served tag)")
	cmd.Flags().StringVar(&quant, "quantization", "Q4_K_M", "Quantization format")
	cmd.Flags().StringVar(&digest, "digest", "", "SHA-256 model digest (auto-generated if omitted)")
	cmd.Flags().Uint32Var(&contextLimit, "context-limit", 0, "Model context token limit")
	cmd.Flags().StringVar(&providerClass, "provider", "ollama", "Inference provider class")
	cmd.Flags().StringVar(&toPath, "to", "", "Target inventory JSON file (default: runtime freeze if present, else eval/base-model-inventory.json)")
	cmd.Flags().BoolVar(&syncBase, "sync-base", false, "Synchronize addition across both runtime and checked-in base inventories")
	return cmd
}

func modelsEvalImportCmd(deps nativeEvalDeps) *cobra.Command {
	var fromPath string
	var toPath string
	var tag string
	var tags string
	var all bool
	var maxParamsStr string
	var family string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import model variants from a source inventory into target inventory",
		Long: `Import model variants from a source inventory (default: eval/base-model-inventory.json)
into the runtime inventory (.g8e/eval/model-inventory.json).

Examples:
  g8e eval models import --all
  g8e eval models import --max-parameters 12b
  g8e eval models import --tags granite4.2:3b,granite4.2:8b
  g8e eval models import --family granite`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			sourcePath := fromPath
			if sourcePath == "" {
				sourcePath = filepath.Join(cfg.ProjectRoot, evaluation.DefaultBaseModelInventoryRelPath)
			} else if !filepath.IsAbs(sourcePath) {
				sourcePath = filepath.Join(cfg.ProjectRoot, sourcePath)
			}

			targetPath := toPath
			if targetPath == "" {
				targetPath = filepath.Join(cfg.ProjectRoot, constants.RuntimeDirname, evaluation.DefaultModelInventoryRelPath)
			} else if !filepath.IsAbs(targetPath) {
				targetPath = filepath.Join(cfg.ProjectRoot, targetPath)
			}

			sourceFreeze, err := evaluation.LoadModelInventoryFreezeFile(sourcePath)
			if err != nil {
				return fmt.Errorf("evaluation: models import: load source freeze: %w", err)
			}

			targetFreeze, err := evaluation.LoadModelInventoryFreezeFile(targetPath)
			if err != nil {
				targetFreeze = sourceFreeze
			}

			selectedTags := splitCSVModelTags(tags)
			if tag != "" {
				selectedTags = append(selectedTags, tag)
			}

			var toImport []*evalv1.ModelVariant
			switch {
			case len(selectedTags) > 0:
				toImport, err = evaluation.VariantsByTags(sourceFreeze.Variants, selectedTags)
				if err != nil {
					return fmt.Errorf("evaluation: models import: %w", err)
				}
			case maxParamsStr != "":
				maxParams, err := evaluation.ParseParameterCount(maxParamsStr)
				if err != nil {
					return err
				}
				toImport = evaluation.FilterVariantsByMaxParameters(sourceFreeze.Variants, maxParams)
			case family != "":
				toImport = evaluation.FilterVariantsByFamily(sourceFreeze.Variants, family)
			case all:
				toImport = sourceFreeze.Variants
			default:
				toImport = sourceFreeze.Variants
			}

			if family != "" && maxParamsStr != "" {
				toImport = evaluation.FilterVariantsByFamily(toImport, family)
			}

			if len(toImport) == 0 {
				cmd.Println("No matching models to import")
				return nil
			}

			updatedFreeze, err := evaluation.MergeModelVariants(targetFreeze, toImport)
			if err != nil {
				return fmt.Errorf("evaluation: models import: %w", err)
			}

			if err := writeModelInventoryFreezeFile(targetPath, updatedFreeze); err != nil {
				return err
			}

			if output.JSONEnabled(cmd) {
				rows := make([]modelInventoryVariantJSON, 0, len(toImport))
				for _, v := range toImport {
					rows = append(rows, modelInventoryVariantJSON{
						ServedModelTag:          v.GetServedModelTag(),
						VariantID:               v.GetVariantId(),
						ModelDigest:             v.GetModelDigest(),
						ModelFamily:             v.GetModelFamily(),
						ParameterCount:          v.GetParameterCount(),
						FormattedParameterCount: evaluation.FormatParameterCount(v.GetParameterCount()),
						Quantization:            v.GetQuantization(),
					})
				}
				payload, err := json.MarshalIndent(rows, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Imported %d model variants into %s (new digest: %s, total: %d models)\n",
				len(toImport), filepath.Base(targetPath), updatedFreeze.RegistryDigest, len(updatedFreeze.Variants))
			for _, v := range toImport {
				paramsStr := ""
				if v.GetParameterCount() > 0 {
					paramsStr = fmt.Sprintf("\t%s", evaluation.FormatParameterCount(v.GetParameterCount()))
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  + %s%s\t(%s)\n", v.GetServedModelTag(), paramsStr, v.GetVariantId())
			}
			return err
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "Source inventory JSON path (default: eval/base-model-inventory.json)")
	cmd.Flags().StringVar(&toPath, "to", "", "Target inventory JSON path (default: .g8e/eval/model-inventory.json)")
	cmd.Flags().StringVar(&tag, "tag", "", "Import one served model tag")
	cmd.Flags().StringVar(&tags, "tags", "", "Import comma-separated served model tags")
	cmd.Flags().BoolVar(&all, "all", false, "Import all variants from source")
	cmd.Flags().StringVar(&maxParamsStr, "max-parameters", "", "Import only variants with parameters <= this threshold (e.g. 12b)")
	cmd.Flags().StringVar(&maxParamsStr, "params", "", "Alias for --max-parameters")
	cmd.Flags().StringVar(&family, "family", "", "Filter imported variants by family")
	return cmd
}

func modelsEvalMaterializeCmd(deps nativeEvalDeps) *cobra.Command {
	var fromPath string
	var tag string
	var tags string
	var all bool
	var formationCatalog bool
	var campaignID string
	var outputPath string
	var outputDir string
	cmd := &cobra.Command{
		Use:   "materialize",
		Short: "Write per-model or multi-model inventory freeze files from a source inventory",
		Long: `Materialize campaign inventory files from a frozen provider inventory.

Examples:
  g8e eval models materialize --tag qwen3:4b
  g8e eval models materialize --all
  g8e eval models materialize --tags qwen3:0.6b,qwen3:4b,gemma3:4b \
    --campaign-id eval-smoke-mini --output .g8e/eval/inventories/eval-smoke-mini.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			sourceVariants, err := loadEvaluationInventoryVariants(cmd.Context(), fileSvc, cfg.ProjectRoot, fromPath)
			if err != nil {
				return fmt.Errorf("evaluation: inventory materialize: %w", err)
			}

			selectedTags := splitCSVModelTags(tags)
			if tag != "" {
				selectedTags = append(selectedTags, tag)
			}
			selected := sourceVariants
			switch {
			case formationCatalog:
				selected, err = evaluation.MaterializeFormationCatalogVariants(sourceVariants)
				if err != nil {
					return fmt.Errorf("evaluation: inventory materialize: %w", err)
				}
			case len(selectedTags) > 0:
				selected, err = evaluation.VariantsByTags(sourceVariants, selectedTags)
				if err != nil {
					return fmt.Errorf("evaluation: inventory materialize: %w", err)
				}
			case all:
				// keep full source inventory
			case campaignID != "" && outputPath != "":
				return fmt.Errorf("evaluation: inventory materialize: specify --tag, --tags, --all, or --formation-catalog")
			default:
				return fmt.Errorf("evaluation: inventory materialize: specify --tag, --tags, --all, or --formation-catalog")
			}

			if campaignID != "" {
				if outputPath == "" {
					return fmt.Errorf("evaluation: inventory materialize: --output is required with --campaign-id")
				}
				freeze, relPath, err := evaluation.MaterializeCampaignInventory(evaluation.MaterializeCampaignInventoryRequest{
					Context:     cmd.Context(),
					FileService: fileSvc,
					CampaignID:  campaignID,
					OutputPath:  normalizeRuntimeEvalPath(outputPath),
					Variants:    selected,
				})
				if err != nil {
					return fmt.Errorf("evaluation: inventory materialize: %w", err)
				}
				return writeInventoryMaterializeResult(cmd, []inventoryMaterializeLine{{
					CampaignID:     campaignID,
					ServedModelTag: fmt.Sprintf("%d models", len(selected)),
					RegistryDigest: freeze.RegistryDigest,
					CellCount:      freeze.HomogeneousCellCount,
					InventoryFile:  relPath,
				}}, output.JSONEnabled(cmd))
			}

			if outputDir == "" {
				outputDir = evaluation.DefaultCampaignInventoryRelDirname
			}
			lines := make([]inventoryMaterializeLine, 0, len(selected))
			for _, variant := range selected {
				entry, err := evaluation.MaterializeInitCampaignInventory(evaluation.MaterializeInitCampaignInventoryRequest{
					Context:         cmd.Context(),
					FileService:     fileSvc,
					InventoryRelDir: normalizeRuntimeEvalPath(outputDir),
					Variant:         variant,
				})
				if err != nil {
					return fmt.Errorf("evaluation: inventory materialize: %w", err)
				}
				lines = append(lines, inventoryMaterializeLine{
					CampaignID:     entry.CampaignID,
					ServedModelTag: entry.ServedModelTag,
					RegistryDigest: entry.ModelRegistryDigest,
					CellCount:      entry.HomogeneousCellCount,
					InventoryFile:  entry.InventoryFile,
				})
			}
			return writeInventoryMaterializeResult(cmd, lines, output.JSONEnabled(cmd))
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "Source inventory JSON path (default: runtime freeze when present, else eval/base-model-inventory.json)")
	cmd.Flags().StringVar(&tag, "tag", "", "Materialize one served model tag")
	cmd.Flags().StringVar(&tags, "tags", "", "Materialize multiple served model tags (comma-separated)")
	cmd.Flags().BoolVar(&all, "all", false, "Materialize every variant in the source inventory")
	cmd.Flags().BoolVar(&formationCatalog, "formation-catalog", false, "Materialize the sovereign served tags required by ExecutionTopologies")
	cmd.Flags().StringVar(&campaignID, "campaign-id", "", "Write one combined multi-model inventory for this campaign ID")
	cmd.Flags().StringVar(&outputPath, "output", "", "Output path for --campaign-id combined inventory")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory for per-model inventories (default: .g8e/eval/inventories)")
	return cmd
}

type inventoryMaterializeLine struct {
	CampaignID     string `json:"campaign_id"`
	ServedModelTag string `json:"served_model_tag"`
	RegistryDigest string `json:"model_registry_digest"`
	CellCount      uint64 `json:"homogeneous_cell_count"`
	InventoryFile  string `json:"inventory_file"`
}

type modelInventoryVariantJSON struct {
	ServedModelTag          string `json:"served_model_tag"`
	VariantID               string `json:"variant_id"`
	ModelDigest             string `json:"model_digest"`
	ModelFamily             string `json:"model_family,omitempty"`
	ParameterCount          uint64 `json:"parameter_count,omitempty"`
	FormattedParameterCount string `json:"formatted_parameter_count,omitempty"`
	Quantization            string `json:"quantization,omitempty"`
}

type modelInventoryListJSON struct {
	Variants []modelInventoryVariantJSON `json:"variants"`
}

type inventoryMaterializeResultJSON struct {
	Inventories []inventoryMaterializeLine `json:"inventories"`
}

func resolveEvaluationInventorySource(explicitPath, projectRoot string) (runtimePath string, externalPath string, err error) {
	explicitPath = strings.TrimSpace(explicitPath)
	if explicitPath == "" {
		return "", "", nil
	}
	if filepath.IsAbs(explicitPath) {
		return "", explicitPath, nil
	}
	normalized := filepath.ToSlash(explicitPath)
	if strings.HasPrefix(normalized, constants.RuntimeDirname+"/") {
		return strings.TrimPrefix(normalized, constants.RuntimeDirname+"/"), "", nil
	}
	if normalized == evaluation.DefaultBaseModelInventoryRelPath {
		return "", filepath.Join(projectRoot, normalized), nil
	}
	return normalized, "", nil
}

func loadEvaluationInventoryVariants(ctx context.Context, fileSvc fs.RuntimeFileService, projectRoot, explicitPath string) ([]*evalv1.ModelVariant, error) {
	runtimePath, externalPath, err := resolveEvaluationInventorySource(explicitPath, projectRoot)
	if err != nil {
		return nil, err
	}
	if externalPath != "" {
		return evaluation.LoadFrozenVariantsFromExternalSource(externalPath)
	}
	if runtimePath == "" {
		if exists, err := fileSvc.FileExists(ctx, evaluation.DefaultModelInventoryRelPath); err != nil {
			return nil, fmt.Errorf("evaluation: inventory: check runtime freeze: %w", err)
		} else if exists {
			return evaluation.LoadFrozenVariantsFromRuntime(ctx, fileSvc, evaluation.DefaultModelInventoryRelPath)
		}
		return evaluation.LoadFrozenVariantsFromExternalSource(filepath.Join(projectRoot, evaluation.DefaultBaseModelInventoryRelPath))
	}
	return evaluation.LoadFrozenVariantsFromRuntime(ctx, fileSvc, runtimePath)
}

func loadEvaluationInventoryFreeze(ctx context.Context, fileSvc fs.RuntimeFileService, projectRoot, explicitPath string) (*evaluation.ModelInventoryFreeze, error) {
	runtimePath, externalPath, err := resolveEvaluationInventorySource(explicitPath, projectRoot)
	if err != nil {
		return nil, err
	}
	if externalPath != "" {
		return evaluation.LoadModelInventoryFreezeFile(externalPath)
	}
	if runtimePath == "" {
		if exists, err := fileSvc.FileExists(ctx, evaluation.DefaultModelInventoryRelPath); err != nil {
			return nil, fmt.Errorf("evaluation: inventory: check runtime freeze: %w", err)
		} else if exists {
			return evaluation.LoadModelInventoryFreezeFromRuntime(ctx, fileSvc, evaluation.DefaultModelInventoryRelPath)
		}
		return evaluation.LoadModelInventoryFreezeFile(filepath.Join(projectRoot, evaluation.DefaultBaseModelInventoryRelPath))
	}
	return evaluation.LoadModelInventoryFreezeFromRuntime(ctx, fileSvc, runtimePath)
}

func writeInventoryMaterializeResult(cmd *cobra.Command, lines []inventoryMaterializeLine, jsonOutput bool) error {
	if jsonOutput {
		payload, err := json.MarshalIndent(inventoryMaterializeResultJSON{Inventories: lines}, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
		return err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "campaign_id=%s tag=%s registry_digest=%s cell_count=%d file=%s\n",
			line.CampaignID, line.ServedModelTag, line.RegistryDigest, line.CellCount, line.InventoryFile); err != nil {
			return err
		}
	}
	return nil
}

func writeModelInventoryFreezeFile(path string, freeze *evaluation.ModelInventoryFreeze) error {
	payload, err := modelInventoryFreezeJSON(freeze)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("evaluation: write inventory freeze: %w", err)
	}
	tmpFile, err := os.CreateTemp(dir, "model-inventory-*.tmp")
	if err != nil {
		_ = os.Chmod(path, 0600)
		if writeErr := os.WriteFile(path, payload, constants.PermFileReadOnly); writeErr != nil {
			return fmt.Errorf("evaluation: write inventory freeze: %w", writeErr)
		}
		return nil
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmpFile.Write(payload); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("evaluation: write inventory freeze: %w", err)
	}
	if err := tmpFile.Chmod(constants.PermFileReadOnly); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("evaluation: write inventory freeze: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("evaluation: write inventory freeze: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Chmod(path, 0600)
		if writeErr := os.WriteFile(path, payload, constants.PermFileReadOnly); writeErr != nil {
			return fmt.Errorf("evaluation: write inventory freeze: %w", writeErr)
		}
	}
	return nil
}

func modelInventoryFreezeJSON(freeze *evaluation.ModelInventoryFreeze) ([]byte, error) {
	if freeze == nil {
		return nil, fmt.Errorf("evaluation: inventory freeze json: %w", constants.ErrMissingRequiredField)
	}
	variants := make([]json.RawMessage, 0, len(freeze.Variants))
	for _, variant := range freeze.Variants {
		body, err := protojson.Marshal(variant)
		if err != nil {
			return nil, fmt.Errorf("evaluation: inventory freeze json: marshal variant: %w", err)
		}
		variants = append(variants, body)
	}
	payload := struct {
		CampaignID           string                              `json:"campaign_id"`
		ModelRegistryDigest  string                              `json:"model_registry_digest"`
		ModelCount           int                                 `json:"model_count"`
		HomogeneousCellCount uint64                              `json:"homogeneous_cell_count"`
		Variants             []json.RawMessage                   `json:"variants"`
		InferenceVariants    []*operatorv1.InferenceModelVariant `json:"variants_inference"`
	}{
		CampaignID:           freeze.CampaignID,
		ModelRegistryDigest:  freeze.RegistryDigest,
		ModelCount:           len(freeze.Variants),
		HomogeneousCellCount: freeze.HomogeneousCellCount,
		Variants:             variants,
		InferenceVariants:    freeze.InferenceVariants,
	}
	return json.MarshalIndent(payload, "", "  ")
}
