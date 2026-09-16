// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func inventoryEvalCmd(_ nativeEvalDeps) *cobra.Command {
	var campaignID string
	var ollamaEndpoint string
	var outputPath string
	var probeCapabilities bool
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Discover and freeze the complete provider model inventory",
	}
	freezeCmd := &cobra.Command{
		Use:   "freeze",
		Short: "Freeze the North Star model registry from a live provider inventory",
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
			freeze, err := evaluation.FreezeNorthStarModelInventoryFromProvider(cmd.Context(), ollamaEndpoint, campaignID, slog.Default(), opts)
			if err != nil {
				return fmt.Errorf("evaluation: inventory freeze: %w", err)
			}
			if outputPath != "" {
				if err := writeModelInventoryFreezeFile(outputPath, freeze); err != nil {
					return err
				}
			}
			if jsonOutput {
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
				evaluation.NorthStarHomogeneousRoleCount,
				evaluation.NorthStarScenarioCount,
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
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "\nExport for docker compose:\nG8E_INFERENCE_CAMPAIGN_ID=%s\nG8E_INFERENCE_MODEL_REGISTRY_DIGEST=%s\n", freeze.CampaignID, freeze.RegistryDigest)
			return err
		},
	}
	freezeCmd.Flags().StringVar(&campaignID, "campaign-id", "", "Frozen evaluation campaign ID")
	freezeCmd.Flags().StringVar(&ollamaEndpoint, "ollama-endpoint", "", "Approved remote Ollama endpoint (default: G8E_OLLAMA_ENDPOINT)")
	freezeCmd.Flags().StringVar(&outputPath, "output", "", "Write the inventory freeze JSON to this path")
	freezeCmd.Flags().BoolVar(&probeCapabilities, "probe-capabilities", false, "Run bounded non-scored capability probes for each discovered model")
	freezeCmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON inventory freeze")
	cmd.AddCommand(freezeCmd)
	return cmd
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
		CampaignID           string            `json:"campaign_id"`
		ModelRegistryDigest  string            `json:"model_registry_digest"`
		ModelCount           int               `json:"model_count"`
		HomogeneousCellCount uint64            `json:"homogeneous_cell_count"`
		Variants             []json.RawMessage `json:"variants"`
		InferenceVariants    any               `json:"variants_inference"`
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
