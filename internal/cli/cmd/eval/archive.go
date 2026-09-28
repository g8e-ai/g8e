// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

type archiveResultJSON struct {
	Kind       string   `json:"kind"`
	ID         string   `json:"id"`
	CampaignID string   `json:"campaign_id,omitempty"`
	RunIDs     []string `json:"run_ids,omitempty"`
	ArchivedAt string   `json:"archived_at,omitempty"`
	ArchivedBy string   `json:"archived_by,omitempty"`
	Archived   bool     `json:"archived"`
}

func archiveResult(manifest *evaluation.ArchiveManifest, archived bool) archiveResultJSON {
	result := archiveResultJSON{
		Kind:       string(manifest.Kind),
		ID:         manifest.ID,
		CampaignID: manifest.CampaignID,
		RunIDs:     manifest.RunIDs,
		Archived:   archived,
	}
	if archived {
		result.ArchivedAt = manifest.ArchivedAt.Format(time.RFC3339)
		result.ArchivedBy = manifest.ArchivedBy
	}
	return result
}

// newArchiver builds the archiver for a command: it moves records through the
// runtime file service and refuses to move a run whose lease holder is alive.
func newArchiver(deps nativeEvalDeps, fileSvc fs.RuntimeFileService) (*evaluation.Archiver, error) {
	control, err := deps.runControl.process(fileSvc)
	if err != nil {
		return nil, err
	}
	return evaluation.NewArchiver(fileSvc, deps.now, leaseLiveness(control)), nil
}

// operatorIdentity names who is archiving: the enrolled CLI operator.
func operatorIdentity(deps nativeEvalDeps, cmd *cobra.Command) (string, error) {
	cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	if err != nil {
		return "", err
	}
	authContext, err := deps.authLoader(fileSvc, cfg)
	if err != nil {
		return "", fmt.Errorf("evaluation: load CLI identity: %w", err)
	}
	if authContext.OperatorID != "" {
		return authContext.OperatorID, nil
	}
	return authContext.UserID, nil
}

const publicRecordNotice = "Any public projections already published remain in the public record."

func campaignsArchiveCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "archive <campaign>",
		Short: "Archive a campaign and all of its runs",
		Long: `Move a campaign and every one of its runs into the archive in one operation.

Nothing is deleted. Archived campaigns are hidden from list and rejected by
runs start, but stay readable by show. The operation fails before moving
anything if any run is running; cancel it first.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			identity, err := operatorIdentity(deps, cmd)
			if err != nil {
				return err
			}
			archiver, err := newArchiver(deps, fileSvc)
			if err != nil {
				return err
			}
			manifest, err := archiver.ArchiveCampaign(cmd.Context(), args[0], identity)
			if err != nil {
				return fmt.Errorf("evaluation: campaigns archive: %w", err)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), archiveResult(manifest, true))
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Archived campaign %s and %d run(s)\n%s\n", manifest.ID, len(manifest.RunIDs), publicRecordNotice)
			return err
		},
	}
}

func campaignsUnarchiveCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "unarchive <campaign>",
		Short: "Return an archived campaign and its runs to the active tree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			archiver, err := newArchiver(deps, fileSvc)
			if err != nil {
				return err
			}
			manifest, err := archiver.UnarchiveCampaign(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("evaluation: campaigns unarchive: %w", err)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), archiveResult(manifest, false))
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Unarchived campaign %s and %d run(s)\n", manifest.ID, len(manifest.RunIDs))
			return err
		},
	}
}

func runsArchiveCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "archive <run>",
		Short: "Archive one run",
		Long: `Move one run into the archive. Nothing is deleted.

Archived runs are hidden from list and rejected by resume, publish, and repair,
but stay readable by show, verify, export, and compare. A running run cannot be
archived; cancel it first.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			identity, err := operatorIdentity(deps, cmd)
			if err != nil {
				return err
			}
			archiver, err := newArchiver(deps, fileSvc)
			if err != nil {
				return err
			}
			manifest, err := archiver.ArchiveRun(cmd.Context(), args[0], identity)
			if err != nil {
				return fmt.Errorf("evaluation: runs archive: %w", err)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), archiveResult(manifest, true))
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Archived run %s\n%s\n", manifest.ID, publicRecordNotice)
			return err
		},
	}
}

func runsUnarchiveCmd(deps nativeEvalDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "unarchive <run>",
		Short: "Return an archived run to the active tree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			archiver, err := newArchiver(deps, fileSvc)
			if err != nil {
				return err
			}
			manifest, err := archiver.UnarchiveRun(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("evaluation: runs unarchive: %w", err)
			}
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), archiveResult(manifest, false))
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Unarchived run %s\n", manifest.ID)
			return err
		},
	}
}
