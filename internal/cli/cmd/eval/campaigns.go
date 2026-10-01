// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const campaignStatusNoRuns = "no-runs"

func campaignsEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "campaigns",
		Aliases: []string{"campaign"},
		Short:   "Define, inspect, and archive evaluation campaigns",
		Long: `A campaign is the frozen definition of an evaluation: which models, which
scenarios, how many repetitions, and in which lane. Executions of a campaign are
runs (g8e eval runs).`,
	}
	addLeaves(cmd,
		campaignsListCmd(deps),
		campaignsShowCmd(deps),
		campaignsCreateCmd(deps),
		campaignsArchiveCmd(deps),
		campaignsUnarchiveCmd(deps),
	)
	return cmd
}

type campaignRowJSON struct {
	CampaignID          string   `json:"campaign_id"`
	Lane                string   `json:"lane"`
	ModelCount          int      `json:"model_count"`
	ScenarioCount       uint32   `json:"scenario_count"`
	RepetitionCount     uint32   `json:"repetition_count"`
	ModelRegistryDigest string   `json:"model_registry_digest"`
	CatalogDigest       string   `json:"catalog_digest"`
	StackCount          int      `json:"stack_count,omitempty"`
	Archived            bool     `json:"archived"`
	Status              string   `json:"status"`
	RunIDs              []string `json:"run_ids"`
}

type campaignListJSON struct {
	Campaigns []campaignRowJSON `json:"campaigns"`
}

type campaignShowJSON struct {
	campaignRowJSON
	Models   []string                    `json:"models"`
	Runs     []runRow                    `json:"runs"`
	Archive  *evaluation.ArchiveManifest `json:"archive,omitempty"`
	CellsPer uint64                      `json:"cells_per_run"`
}

// campaignLane reports the lane of a campaign from its frozen stack set: a
// campaign with stacks is a system-lane campaign.
func campaignLane(ctx context.Context, store *evaluation.Store, campaignID string) (string, int, error) {
	stackSet, err := store.LoadHeterogeneousStackSet(ctx, campaignID)
	if err != nil {
		if isMissingRecord(err) {
			return campaignLaneModelRole, 0, nil
		}
		return "", 0, err
	}
	return campaignLaneSystem, len(stackSet.Stacks), nil
}

func isMissingRecord(err error) bool {
	return errors.Is(err, constants.ErrNotFound) || errors.Is(err, iofs.ErrNotExist)
}

func campaignRunRows(ctx context.Context, fileSvc fs.RuntimeFileService, runIDs []string, live evaluation.LeaseLiveness, deps nativeEvalDeps) ([]runRow, error) {
	rows := make([]runRow, 0, len(runIDs))
	for _, runID := range runIDs {
		store, archived, err := evaluation.LocateRun(ctx, fileSvc, runID)
		if err != nil {
			return nil, err
		}
		row, _, err := summarizeRun(ctx, store, archived, runID, live, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
		if err != nil {
			return nil, err
		}
		rows = append(rows, *row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].StartedAt < rows[j].StartedAt })
	return rows, nil
}

func latestRunStatus(rows []runRow) string {
	if len(rows) == 0 {
		return campaignStatusNoRuns
	}
	return rows[len(rows)-1].Status
}

func campaignsListCmd(deps nativeEvalDeps) *cobra.Command {
	var statusFilter string
	var includeArchived bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List campaigns",
		Long: `List campaigns with the status of their latest run.

Archived campaigns are hidden unless --archived is given.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			control, err := deps.runControl.process(fileSvc)
			if err != nil {
				return err
			}
			live := leaseLiveness(control)
			type source struct {
				store    *evaluation.Store
				archived bool
			}
			sources := []source{{store: evaluation.NewStore(fileSvc)}}
			if includeArchived {
				sources = append(sources, source{store: evaluation.NewArchivedStore(fileSvc), archived: true})
			}
			rows := make([]campaignRowJSON, 0)
			for _, src := range sources {
				entries, err := src.store.ListCampaigns(cmd.Context())
				if err != nil {
					return fmt.Errorf("evaluation: campaigns list: %w", err)
				}
				for _, entry := range entries {
					lane, stacks, err := campaignLane(cmd.Context(), src.store, entry.CampaignID)
					if err != nil {
						return fmt.Errorf("evaluation: campaigns list: %w", err)
					}
					runRows, err := campaignRunRows(cmd.Context(), fileSvc, entry.RunIDs, live, deps)
					if err != nil {
						return fmt.Errorf("evaluation: campaigns list: %w", err)
					}
					status := latestRunStatus(runRows)
					if statusFilter != "" && status != statusFilter {
						continue
					}
					rows = append(rows, campaignRowJSON{
						CampaignID:          entry.CampaignID,
						Lane:                lane,
						ModelCount:          entry.ModelCount,
						ScenarioCount:       entry.ScenarioCount,
						RepetitionCount:     entry.RepetitionCount,
						ModelRegistryDigest: entry.ModelRegistryDigest,
						CatalogDigest:       entry.CatalogDigest,
						StackCount:          stacks,
						Archived:            src.archived,
						Status:              status,
						RunIDs:              append([]string{}, entry.RunIDs...),
					})
				}
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), campaignListJSON{Campaigns: rows})
			}
			if len(rows) == 0 {
				cmd.Println("No campaigns found")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "CAMPAIGN\tLANE\tMODELS\tSCENARIOS\tREPS\tRUNS\tSTATUS\tARCHIVED")
			for _, row := range rows {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%s\t%t\n",
					row.CampaignID, row.Lane, row.ModelCount, row.ScenarioCount, row.RepetitionCount, len(row.RunIDs), row.Status, row.Archived)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&statusFilter, "status", "", "Only campaigns whose latest run has this status (for example verified, running, no-runs)")
	cmd.Flags().BoolVar(&includeArchived, "archived", false, "Include archived campaigns")
	return cmd
}

func campaignsShowCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <campaign>",
		Short: "Show one campaign and its runs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			campaignID := args[0]
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			control, err := deps.runControl.process(fileSvc)
			if err != nil {
				return err
			}
			store, archived, err := evaluation.LocateCampaign(cmd.Context(), fileSvc, campaignID)
			if err != nil {
				return fmt.Errorf("evaluation: campaigns show: %w", err)
			}
			spec, err := store.LoadCampaignSpec(cmd.Context(), campaignID)
			if err != nil {
				return fmt.Errorf("evaluation: campaigns show: %w", err)
			}
			lane, stacks, err := campaignLane(cmd.Context(), store, campaignID)
			if err != nil {
				return fmt.Errorf("evaluation: campaigns show: %w", err)
			}
			runIDs, err := campaignRunIDs(cmd.Context(), fileSvc, store, archived, campaignID)
			if err != nil {
				return fmt.Errorf("evaluation: campaigns show: %w", err)
			}
			runRows, err := campaignRunRows(cmd.Context(), fileSvc, runIDs, leaseLiveness(control), deps)
			if err != nil {
				return fmt.Errorf("evaluation: campaigns show: %w", err)
			}
			payload := campaignShowJSON{
				campaignRowJSON: campaignRowJSON{
					CampaignID:          campaignID,
					Lane:                lane,
					ModelCount:          len(spec.GetModelRegistry()),
					ScenarioCount:       spec.GetScenarioCount(),
					RepetitionCount:     spec.GetRepetitionCount(),
					ModelRegistryDigest: spec.GetModelRegistryDigest(),
					CatalogDigest:       spec.GetCatalogDigest(),
					StackCount:          stacks,
					Archived:            archived,
					Status:              latestRunStatus(runRows),
					RunIDs:              runIDs,
				},
				Runs:     runRows,
				CellsPer: campaignCells(spec, lane, stacks),
			}
			for _, variant := range spec.GetModelRegistry() {
				payload.Models = append(payload.Models, variant.GetServedModelTag())
			}
			if archived {
				if payload.Archive, err = evaluation.LoadArchiveManifest(cmd.Context(), fileSvc, evaluation.ArchiveKindCampaign, campaignID); err != nil {
					return fmt.Errorf("evaluation: campaigns show: %w", err)
				}
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), payload)
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Campaign: %s\nLane: %s\nModels: %d (%s)\nScenarios: %d\nRepetitions: %d\nCells per run: %d\nModel registry digest: %s\nCatalog digest: %s\nStatus: %s\nArchived: %t\n",
				campaignID, lane, payload.ModelCount, strings.Join(payload.Models, ", "), payload.ScenarioCount, payload.RepetitionCount,
				payload.CellsPer, payload.ModelRegistryDigest, payload.CatalogDigest, payload.Status, archived)
			if stacks > 0 {
				_, _ = fmt.Fprintf(out, "Stacks: %d\n", stacks)
			}
			if payload.Archive != nil {
				_, _ = fmt.Fprintf(out, "Archived at: %s by %s\n", payload.Archive.ArchivedAt.Format("2006-01-02T15:04:05Z"), payload.Archive.ArchivedBy)
			}
			if len(runRows) == 0 {
				_, _ = fmt.Fprintln(out, "Runs: none")
				return nil
			}
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "RUN\tSTATUS\tPROGRESS\tSTARTED")
			for _, row := range runRows {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%d/%d\t%s\n", row.RunID, row.Status, row.Terminal, row.ExpectedAssignments, row.StartedAt)
			}
			return w.Flush()
		},
	}
}

// campaignRunIDs lists every run of a campaign, active and archived.
func campaignRunIDs(ctx context.Context, fileSvc fs.RuntimeFileService, store *evaluation.Store, archived bool, campaignID string) ([]string, error) {
	runIDs, err := store.ListCampaignRunIDs(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if archived {
		return runIDs, nil
	}
	archivedIDs, err := evaluation.NewArchivedRunStore(fileSvc).ListCampaignRunIDs(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	runIDs = append(runIDs, archivedIDs...)
	sort.Strings(runIDs)
	return runIDs, nil
}

func campaignCells(spec *evalv1.EvaluationCampaignSpec, lane string, stacks int) uint64 {
	if lane == campaignLaneSystem {
		return evaluation.ComputeHeterogeneousMatrixSize(uint64(stacks))
	}
	return evaluation.ComputeHomogeneousMatrixSize(uint64(len(spec.GetModelRegistry()))) * uint64(spec.GetRepetitionCount())
}

// campaignCreateSpec is everything that defines one campaign.
type campaignCreateSpec struct {
	CampaignID   string
	Variants     []*evalv1.ModelVariant
	Repetitions  uint32
	System       bool
	Seed         uint64
	Formations   bool
	FormationIDs []string
	Smoke        bool
}

type campaignCreateResult struct {
	Spec     *evalv1.EvaluationCampaignSpec
	StackSet *evaluation.HeterogeneousStackSet
	Lane     string
}

// createCampaign freezes a campaign: its model registry, scenario catalog and
// artifacts, and for the system lane its stack set. The per-campaign model
// inventory is written as an internal artifact. Creating a campaign whose ID
// already holds the same frozen spec is a no-op.
func createCampaign(ctx context.Context, deps nativeEvalDeps, fileSvc fs.RuntimeFileService, spec campaignCreateSpec) (*campaignCreateResult, error) {
	if !complianceevidence.ValidPathElement(spec.CampaignID) {
		return nil, fmt.Errorf("evaluation: campaigns create: invalid campaign ID %q: %w", spec.CampaignID, constants.ErrMissingRequiredField)
	}
	if _, archived, err := evaluation.LocateCampaign(ctx, fileSvc, spec.CampaignID); err == nil && archived {
		return nil, fmt.Errorf("evaluation: campaigns create: campaign %q: %w", spec.CampaignID, constants.ErrEvaluationArchived)
	} else if err != nil && !errors.Is(err, constants.ErrNotFound) {
		return nil, err
	}
	freeze, err := evaluation.MaterializeModelRegistry(spec.CampaignID, spec.Variants)
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaigns create: %w", err)
	}
	if err := evaluation.ValidateModelRegistry(freeze); err != nil {
		return nil, fmt.Errorf("evaluation: campaigns create: %w", err)
	}
	var (
		catalog   *evalv1.EvaluationScenarioCatalog
		artifacts map[string]evaluation.ScenarioArtifacts
	)
	if spec.Smoke {
		catalog, artifacts, err = evaluation.LoadSmokeGateScenarioCatalog()
	} else {
		catalog, artifacts, err = evaluation.LoadScenarioCatalog()
	}
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaigns create: %w", err)
	}
	var stackSet *evaluation.HeterogeneousStackSet
	switch {
	case spec.Formations:
		stackSet, err = evaluation.GenerateFormationCatalogStackSet(evaluation.FormationCatalogStackGenerationRequest{
			CampaignID:   spec.CampaignID,
			Seed:         spec.Seed,
			Variants:     freeze.Variants,
			FormationIDs: spec.FormationIDs,
		})
	case spec.System:
		stackSet, err = evaluation.GenerateHeterogeneousStackSet(evaluation.HeterogeneousStackGenerationRequest{
			CampaignID: spec.CampaignID,
			Seed:       spec.Seed,
			Variants:   freeze.Variants,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaigns create: %w", err)
	}
	if stackSet == nil && len(freeze.Variants) != 1 {
		return nil, fmt.Errorf("evaluation: campaigns create: %d models selected: %w", len(freeze.Variants), constants.ErrEvaluationCampaignSubjectInvalid)
	}
	store := evaluation.NewStore(fileSvc)
	controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string { return prefix + "-" + deps.newID() })
	campaignSpec, err := controller.CreateCampaign(ctx, evaluation.CampaignCreateRequest{
		CampaignID:        spec.CampaignID,
		Catalog:           catalog,
		Inventory:         freeze,
		ScenarioArtifacts: artifacts,
		RepetitionCount:   spec.Repetitions,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluation: campaigns create: %w", err)
	}
	if stackSet != nil {
		if err := store.SaveHeterogeneousStackSet(ctx, spec.CampaignID, stackSet); err != nil {
			return nil, fmt.Errorf("evaluation: campaigns create: %w", err)
		}
	}
	if _, _, err := evaluation.MaterializeCampaignInventory(evaluation.MaterializeCampaignInventoryRequest{
		Context:     ctx,
		FileService: fileSvc,
		CampaignID:  spec.CampaignID,
		Variants:    freeze.Variants,
	}); err != nil {
		return nil, fmt.Errorf("evaluation: campaigns create: %w", err)
	}
	lane := campaignLaneModelRole
	if stackSet != nil {
		lane = campaignLaneSystem
	}
	return &campaignCreateResult{Spec: campaignSpec, StackSet: stackSet, Lane: lane}, nil
}

type campaignCreateJSON struct {
	CampaignID          string `json:"campaign_id"`
	Lane                string `json:"lane"`
	ModelCount          int    `json:"model_count"`
	ScenarioCount       uint32 `json:"scenario_count"`
	RepetitionCount     uint32 `json:"repetition_count"`
	CellsPerRun         uint64 `json:"cells_per_run"`
	ModelRegistryDigest string `json:"model_registry_digest"`
	CatalogDigest       string `json:"catalog_digest"`
	StackCount          int    `json:"stack_count,omitempty"`
	StackSetDigest      string `json:"stack_set_digest,omitempty"`
}

func campaignsCreateCmd(deps nativeEvalDeps) *cobra.Command {
	var reps uint32
	var lane string
	var seed uint64
	var formations []string
	var allFormations bool
	var selector ModelSelector
	cmd := &cobra.Command{
		Use:   "create <campaign> <selector>",
		Short: "Freeze a campaign over registry models or formations",
		Long: `Freeze a campaign: the selected registry models, the scenario catalog, and the
repetition count. Nothing executes until a run starts (g8e eval runs start).

A model campaign takes a model selector (positional models, --family,
--max-params, or --all) and defaults to the model-role lane.

A formation campaign takes --formations <id>... or --all-formations and always
runs in the system lane.

Examples:
  g8e eval campaigns create eval-qwen qwen3:4b qwen3:8b --reps 3
  g8e eval campaigns create eval-small --max-params 4b
  g8e eval campaigns create eval-formations --all-formations --seed 17
  g8e eval campaigns create eval-two --formations qwen-powerhouse,ultra-efficient-speedster`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			campaignID := args[0]
			selector = selector.withArgs(args[1:])
			formationCampaign := allFormations || len(formations) > 0
			if allFormations && len(formations) > 0 {
				return fmt.Errorf("evaluation: campaigns create: --formations and --all-formations are mutually exclusive")
			}
			if lane != campaignLaneModelRole && lane != campaignLaneSystem {
				return fmt.Errorf("evaluation: campaigns create: --lane must be %s or %s (got %q)", campaignLaneModelRole, campaignLaneSystem, lane)
			}
			if formationCampaign && cmd.Flags().Changed("lane") && lane != campaignLaneSystem {
				return fmt.Errorf("evaluation: campaigns create: --lane %s is incompatible with --formations/--all-formations, which always run in the system lane", lane)
			}
			if formationCampaign && selector.IsSet() {
				msg := fmt.Sprintf("evaluation: campaigns create: formations run in the system lane and take no model selector, but got %s", selector.describe())
				if len(selector.IDs) > 0 {
					msg += "; if you meant to pass multiple --formations, separate them with a comma (--formations a,b) or repeat the flag (--formations a --formations b) — space-separated values after a flag are parsed as extra positional arguments"
				}
				return errors.New(msg)
			}
			if !formationCampaign && !selector.IsSet() {
				return fmt.Errorf("evaluation: campaigns create: name models, pass --family, --max-params, or --all, or use --formations or --all-formations: %w", constants.ErrEvaluationSelectionEmpty)
			}
			cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			registry, err := newModelInventories(fileSvc, cfg.ProjectRoot).variants(cmd.Context(), modelScopeRegistry)
			if err != nil {
				return fmt.Errorf("evaluation: campaigns create: %w", err)
			}
			if len(registry) == 0 {
				return fmt.Errorf("evaluation: campaigns create: the model registry is empty (run `g8e eval models freeze` or `g8e eval models import`): %w", constants.ErrEvaluationSelectionEmpty)
			}
			create := campaignCreateSpec{CampaignID: campaignID, Repetitions: reps, Seed: seed}
			if formationCampaign {
				create.Formations = true
				create.FormationIDs = formations
				if create.Variants, err = evaluation.MaterializeFormationVariants(registry, formations); err != nil {
					return fmt.Errorf("evaluation: campaigns create: %w", err)
				}
			} else {
				create.System = lane == campaignLaneSystem
				if create.Variants, err = selector.Resolve(registry); err != nil {
					return fmt.Errorf("evaluation: campaigns create: %w", err)
				}
			}
			result, err := createCampaign(cmd.Context(), deps, fileSvc, create)
			if err != nil {
				return err
			}
			stacks := 0
			stackDigest := ""
			if result.StackSet != nil {
				stacks, stackDigest = len(result.StackSet.Stacks), result.StackSet.SetDigest
			}
			payload := campaignCreateJSON{
				CampaignID:          campaignID,
				Lane:                result.Lane,
				ModelCount:          len(result.Spec.GetModelRegistry()),
				ScenarioCount:       result.Spec.GetScenarioCount(),
				RepetitionCount:     result.Spec.GetRepetitionCount(),
				CellsPerRun:         campaignCells(result.Spec, result.Lane, stacks),
				ModelRegistryDigest: result.Spec.GetModelRegistryDigest(),
				CatalogDigest:       result.Spec.GetCatalogDigest(),
				StackCount:          stacks,
				StackSetDigest:      stackDigest,
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), payload)
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Campaign created\nCampaign: %s\nLane: %s\nModels: %d\nScenarios: %d\nRepetitions: %d\nCells per run: %d\nModel registry digest: %s\nCatalog digest: %s\n",
				payload.CampaignID, payload.Lane, payload.ModelCount, payload.ScenarioCount, payload.RepetitionCount, payload.CellsPerRun, payload.ModelRegistryDigest, payload.CatalogDigest)
			if stacks > 0 {
				_, _ = fmt.Fprintf(out, "Stacks: %d (set digest %s)\n", stacks, stackDigest)
			}
			_, err = fmt.Fprintf(out, "\nNext: g8e eval runs start %s\n", campaignID)
			return err
		},
	}
	cmd.Flags().Uint32Var(&reps, "reps", 1, "Repetitions of each matrix cell")
	cmd.Flags().StringVar(&lane, "lane", campaignLaneModelRole, "Evaluation lane: model-role or system")
	cmd.Flags().Uint64Var(&seed, "seed", 0, "Deterministic stack generation seed (system lane)")
	cmd.Flags().StringSliceVar(&formations, "formations", nil, "Formation catalog IDs to evaluate (system lane)")
	cmd.Flags().BoolVar(&allFormations, "all-formations", false, "Evaluate every formation in the catalog (system lane)")
	selector.bindFlags(cmd)
	return cmd
}
