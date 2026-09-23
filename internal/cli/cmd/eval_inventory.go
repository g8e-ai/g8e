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

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func modelsEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "Discover, freeze, and materialize evaluation model inventories",
	}
	cmd.AddCommand(
		modelsEvalFreezeCmd(),
		modelsEvalListCmd(deps),
		modelsEvalMaterializeCmd(deps),
	)
	return cmd
}

func modelsEvalFreezeCmd() *cobra.Command {
	var campaignID string
	var ollamaEndpoint string
	var outputPath string
	var probeCapabilities bool
	cmd := &cobra.Command{
		Use:   "freeze",
		Short: "Freeze the model registry from a live provider inventory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if campaignID == "" {
				return fmt.Errorf("evaluation: inventory freeze: %w", constants.ErrMissingRequiredField)
			}
			if ollamaEndpoint == "" {
				ollamaEndpoint = os.Getenv("G8E_OLLAMA_ENDPOINT")
			}
			if ollamaEndpoint == "" {
				return fmt.Errorf("evaluation: inventory freeze: set --ollama-endpoint or G8E_OLLAMA_ENDPOINT")
			}
			opts := evaluation.ModelInventoryOptions{RunCapabilityProbes: probeCapabilities}
			if probeCapabilities {
				probeBackend, err := evaluation.NewOllamaCapabilityProbeBackend(ollamaEndpoint, slog.Default())
				if err != nil {
					return fmt.Errorf("evaluation: inventory freeze: probe backend: %w", err)
				}
				opts.ProbeBackend = probeBackend
			}
			freeze, err := evaluation.FreezeModelInventoryFromProvider(cmd.Context(), ollamaEndpoint, campaignID, slog.Default(), opts)
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
	cmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Approved remote Ollama endpoint (default: G8E_OLLAMA_ENDPOINT)")
	cmd.Flags().StringVar(&outputPath, "output", "", "Write the inventory freeze JSON to this path")
	cmd.Flags().BoolVar(&probeCapabilities, "probe-capabilities", false, "Run bounded non-scored capability probes for each discovered model")
	return cmd
}

func modelsEvalListCmd(deps nativeEvalDeps) *cobra.Command {
	var fromPath string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List model variants from a frozen inventory file",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			variants, err := loadEvaluationInventoryVariants(cmd.Context(), fileSvc, cfg.ProjectRoot, fromPath)
			if err != nil {
				return fmt.Errorf("evaluation: inventory list: %w", err)
			}
			if output.JSONEnabled(cmd) {
				rows := make([]modelInventoryVariantJSON, 0, len(variants))
				for _, variant := range variants {
					rows = append(rows, modelInventoryVariantJSON{
						ServedModelTag: variant.GetServedModelTag(),
						VariantID:      variant.GetVariantId(),
						ModelDigest:    variant.GetModelDigest(),
					})
				}
				payload, err := json.MarshalIndent(modelInventoryListJSON{Variants: rows}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				return err
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
	return cmd
}

func modelsEvalMaterializeCmd(deps nativeEvalDeps) *cobra.Command {
	var fromPath string
	var tag string
	var tags string
	var all bool
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
			case len(selectedTags) > 0:
				selected, err = evaluation.VariantsByTags(sourceVariants, selectedTags)
				if err != nil {
					return fmt.Errorf("evaluation: inventory materialize: %w", err)
				}
			case all:
				// keep full source inventory
			case campaignID != "" && outputPath != "":
				return fmt.Errorf("evaluation: inventory materialize: specify --tag, --tags, or --all")
			default:
				return fmt.Errorf("evaluation: inventory materialize: specify --tag, --tags, or --all")
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
	ServedModelTag string `json:"served_model_tag"`
	VariantID      string `json:"variant_id"`
	ModelDigest    string `json:"model_digest"`
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
	if err := os.WriteFile(path, payload, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("evaluation: write inventory freeze: %w", err)
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
