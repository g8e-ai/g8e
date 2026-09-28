// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// ArchiveKind names what an archive manifest describes.
type ArchiveKind string

const (
	ArchiveKindCampaign ArchiveKind = "campaign"
	ArchiveKindRun      ArchiveKind = "run"
)

// ArchiveManifest records who archived a campaign or run and when. It is
// written beside the archived directory.
type ArchiveManifest struct {
	Kind       ArchiveKind `json:"kind"`
	ID         string      `json:"id"`
	CampaignID string      `json:"campaign_id,omitempty"`
	RunIDs     []string    `json:"run_ids,omitempty"`
	ArchivedAt time.Time   `json:"archived_at"`
	ArchivedBy string      `json:"archived_by"`
}

// CampaignExists reports whether the store holds a campaign spec.
func (s *Store) CampaignExists(ctx context.Context, campaignID string) (bool, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(campaignID) {
		return false, fmt.Errorf("%w: file service and campaign ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	return s.files.FileExists(ctx, s.layout.campaignSpecPath(campaignID))
}

// ListCampaignRunIDs returns the IDs of the store's runs bound to one campaign.
func (s *Store) ListCampaignRunIDs(ctx context.Context, campaignID string) ([]string, error) {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(campaignID) {
		return nil, fmt.Errorf("%w: file service and campaign ID are required", constants.ErrEvidenceArtifactMalformed)
	}
	byCampaign, err := s.listRunIDsByCampaign(ctx)
	if err != nil {
		return nil, err
	}
	return byCampaign[campaignID], nil
}

// ListRunIDs returns every run ID the store holds.
func (s *Store) ListRunIDs(ctx context.Context) ([]string, error) {
	if s == nil || s.files == nil {
		return nil, fmt.Errorf("%w: file service is required", constants.ErrEvidenceArtifactMalformed)
	}
	byCampaign, err := s.listRunIDsByCampaign(ctx)
	if err != nil {
		return nil, err
	}
	var runIDs []string
	for _, ids := range byCampaign {
		runIDs = append(runIDs, ids...)
	}
	sort.Strings(runIDs)
	return runIDs, nil
}

// LocateRun returns the store that holds a campaign run and whether the run is
// archived. Archived runs read from the archive, against their campaign
// wherever it lives, so every read of a run works whether or not it is
// archived.
func LocateRun(ctx context.Context, files fs.RuntimeFileService, runID string) (*Store, bool, error) {
	active := NewStore(files)
	exists, err := active.RunExists(ctx, runID)
	if err != nil {
		return nil, false, err
	}
	if exists {
		return active, false, nil
	}
	hybrid := NewArchivedRunStore(files)
	exists, err = hybrid.RunExists(ctx, runID)
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, fmt.Errorf("evaluation: run %q: %w", runID, constants.ErrNotFound)
	}
	run, err := hybrid.LoadRun(ctx, runID)
	if err != nil {
		return nil, false, err
	}
	campaignActive, err := active.CampaignExists(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return nil, false, err
	}
	if campaignActive {
		return hybrid, true, nil
	}
	return NewArchivedStore(files), true, nil
}

// LocateCampaign returns the store that holds a campaign and whether the
// campaign is archived.
func LocateCampaign(ctx context.Context, files fs.RuntimeFileService, campaignID string) (*Store, bool, error) {
	active := NewStore(files)
	exists, err := active.CampaignExists(ctx, campaignID)
	if err != nil {
		return nil, false, err
	}
	if exists {
		return active, false, nil
	}
	archived := NewArchivedStore(files)
	exists, err = archived.CampaignExists(ctx, campaignID)
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, fmt.Errorf("evaluation: campaign %q: %w", campaignID, constants.ErrNotFound)
	}
	return archived, true, nil
}

// Archiver moves campaigns and runs into and out of the archive. Archive is the
// only removal: nothing is deleted, and archiving does not retract anything
// already published.
type Archiver struct {
	files fs.RuntimeFileService
	now   func() time.Time
	live  LeaseLiveness
}

// NewArchiver returns an Archiver. live reports whether a run lease is held by
// a running process.
func NewArchiver(files fs.RuntimeFileService, now func() time.Time, live LeaseLiveness) *Archiver {
	if now == nil {
		now = time.Now
	}
	return &Archiver{files: files, now: now, live: live}
}

func archivedRunDir(runID string) string { return archivedEvalLayout().runDir(runID) }

func archivedCampaignDir(campaignID string) string {
	return archivedEvalLayout().campaignDir(campaignID)
}

func archiveManifestPath(dir string) string { return dir + constants.EvaluationArchiveManifestSuffix }

// ArchiveRun moves one run into the archive. A running run must be cancelled
// first.
func (a *Archiver) ArchiveRun(ctx context.Context, runID, identity string) (*ArchiveManifest, error) {
	store, archived, err := LocateRun(ctx, a.files, runID)
	if err != nil {
		return nil, err
	}
	if archived {
		return nil, fmt.Errorf("evaluation: run %q: %w", runID, constants.ErrEvaluationArchived)
	}
	if err := a.rejectRunning(ctx, store, runID); err != nil {
		return nil, err
	}
	run, err := store.LoadRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	manifest := &ArchiveManifest{
		Kind:       ArchiveKindRun,
		ID:         runID,
		CampaignID: run.GetCampaignBinding().GetCampaignId(),
		ArchivedAt: a.now().UTC(),
		ArchivedBy: identity,
	}
	if err := a.move(ctx, activeEvalLayout().runDir(runID), archivedRunDir(runID)); err != nil {
		return nil, err
	}
	if err := a.writeManifest(ctx, archivedRunDir(runID), manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// UnarchiveRun returns one archived run to the active tree. Its campaign must
// be active.
func (a *Archiver) UnarchiveRun(ctx context.Context, runID string) (*ArchiveManifest, error) {
	store, archived, err := LocateRun(ctx, a.files, runID)
	if err != nil {
		return nil, err
	}
	if !archived {
		return nil, fmt.Errorf("evaluation: run %q: %w", runID, constants.ErrEvaluationNotArchived)
	}
	run, err := store.LoadRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	campaignID := run.GetCampaignBinding().GetCampaignId()
	if campaignActive, err := NewStore(a.files).CampaignExists(ctx, campaignID); err != nil {
		return nil, err
	} else if !campaignActive {
		return nil, fmt.Errorf("evaluation: run %q belongs to archived campaign %q; unarchive the campaign first: %w", runID, campaignID, constants.ErrEvaluationArchived)
	}
	manifest, err := a.readManifest(ctx, archivedRunDir(runID))
	if err != nil {
		return nil, err
	}
	if err := a.move(ctx, archivedRunDir(runID), activeEvalLayout().runDir(runID)); err != nil {
		return nil, err
	}
	if err := a.files.Remove(ctx, archiveManifestPath(archivedRunDir(runID))); err != nil {
		return nil, fmt.Errorf("evaluation: remove archive manifest: %w", err)
	}
	return manifest, nil
}

// ArchiveCampaign moves a campaign and every one of its active runs into the
// archive in one operation. It fails before moving anything when any run is
// running.
func (a *Archiver) ArchiveCampaign(ctx context.Context, campaignID, identity string) (*ArchiveManifest, error) {
	store, archived, err := LocateCampaign(ctx, a.files, campaignID)
	if err != nil {
		return nil, err
	}
	if archived {
		return nil, fmt.Errorf("evaluation: campaign %q: %w", campaignID, constants.ErrEvaluationArchived)
	}
	runIDs, err := store.ListCampaignRunIDs(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	for _, runID := range runIDs {
		if err := a.rejectRunning(ctx, store, runID); err != nil {
			return nil, err
		}
	}
	manifest := &ArchiveManifest{
		Kind:       ArchiveKindCampaign,
		ID:         campaignID,
		RunIDs:     runIDs,
		ArchivedAt: a.now().UTC(),
		ArchivedBy: identity,
	}
	type moved struct{ from, to string }
	var done []moved
	rollback := func(cause error) error {
		errs := []error{cause}
		for i := len(done) - 1; i >= 0; i-- {
			if err := a.files.Rename(ctx, done[i].to, done[i].from); err != nil {
				errs = append(errs, fmt.Errorf("evaluation: roll back archive move %s: %w", done[i].to, err))
			}
		}
		return errors.Join(errs...)
	}
	for _, runID := range runIDs {
		from, to := activeEvalLayout().runDir(runID), archivedRunDir(runID)
		if err := a.move(ctx, from, to); err != nil {
			return nil, rollback(err)
		}
		done = append(done, moved{from: from, to: to})
	}
	campaignFrom, campaignTo := activeEvalLayout().campaignDir(campaignID), archivedCampaignDir(campaignID)
	if err := a.move(ctx, campaignFrom, campaignTo); err != nil {
		return nil, rollback(err)
	}
	if err := a.writeManifest(ctx, campaignTo, manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// UnarchiveCampaign returns an archived campaign, and the runs archived with
// it, to the active tree.
func (a *Archiver) UnarchiveCampaign(ctx context.Context, campaignID string) (*ArchiveManifest, error) {
	_, archived, err := LocateCampaign(ctx, a.files, campaignID)
	if err != nil {
		return nil, err
	}
	if !archived {
		return nil, fmt.Errorf("evaluation: campaign %q: %w", campaignID, constants.ErrEvaluationNotArchived)
	}
	manifest, err := a.readManifest(ctx, archivedCampaignDir(campaignID))
	if err != nil {
		return nil, err
	}
	if err := a.move(ctx, archivedCampaignDir(campaignID), activeEvalLayout().campaignDir(campaignID)); err != nil {
		return nil, err
	}
	for _, runID := range manifest.RunIDs {
		exists, err := a.files.FileExists(ctx, archivedEvalLayout().runStatePath(runID))
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		if err := a.move(ctx, archivedRunDir(runID), activeEvalLayout().runDir(runID)); err != nil {
			return nil, err
		}
		if err := a.files.Remove(ctx, archiveManifestPath(archivedRunDir(runID))); err != nil {
			return nil, fmt.Errorf("evaluation: remove archive manifest: %w", err)
		}
	}
	if err := a.files.Remove(ctx, archiveManifestPath(archivedCampaignDir(campaignID))); err != nil {
		return nil, fmt.Errorf("evaluation: remove archive manifest: %w", err)
	}
	return manifest, nil
}

// RejectArchivedRun returns constants.ErrEvaluationArchived when the run is in
// the archive. Commands that would change a run call it first.
func RejectArchivedRun(ctx context.Context, files fs.RuntimeFileService, runID string) error {
	_, archived, err := LocateRun(ctx, files, runID)
	if err != nil {
		return err
	}
	if archived {
		return fmt.Errorf("evaluation: run %q: %w", runID, constants.ErrEvaluationArchived)
	}
	return nil
}

// RejectArchivedCampaign returns constants.ErrEvaluationArchived when the
// campaign is in the archive.
func RejectArchivedCampaign(ctx context.Context, files fs.RuntimeFileService, campaignID string) error {
	_, archived, err := LocateCampaign(ctx, files, campaignID)
	if err != nil {
		return err
	}
	if archived {
		return fmt.Errorf("evaluation: campaign %q: %w", campaignID, constants.ErrEvaluationArchived)
	}
	return nil
}

func (a *Archiver) rejectRunning(ctx context.Context, store *Store, runID string) error {
	lease, err := store.LoadRunLease(ctx, runID)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return nil
		}
		return err
	}
	if a.live != nil && a.live(*lease) {
		return fmt.Errorf("evaluation: run %q: %w", runID, constants.ErrEvaluationRunRunning)
	}
	return nil
}

func (a *Archiver) move(ctx context.Context, from, to string) error {
	if exists, err := a.files.FileExists(ctx, to); err != nil {
		return fmt.Errorf("evaluation: archive: check destination: %w", err)
	} else if exists {
		return fmt.Errorf("evaluation: archive: destination %s already exists", to)
	}
	if err := a.files.MkdirAll(ctx, filepath.Dir(to), constants.PermDirStandard); err != nil {
		return fmt.Errorf("evaluation: archive: create %s: %w", filepath.Dir(to), err)
	}
	if err := a.files.Rename(ctx, from, to); err != nil {
		return fmt.Errorf("evaluation: archive: move %s: %w", from, err)
	}
	return nil
}

func (a *Archiver) writeManifest(ctx context.Context, dir string, manifest *ArchiveManifest) error {
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("evaluation: archive manifest: %w", err)
	}
	if err := a.files.WriteFile(ctx, archiveManifestPath(dir), body, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("evaluation: archive manifest: %w", err)
	}
	return nil
}

func (a *Archiver) readManifest(ctx context.Context, dir string) (*ArchiveManifest, error) {
	body, err := a.files.ReadFile(ctx, archiveManifestPath(dir))
	if err != nil {
		return nil, fmt.Errorf("evaluation: read archive manifest: %w", err)
	}
	manifest := &ArchiveManifest{}
	if err := json.Unmarshal(body, manifest); err != nil {
		return nil, fmt.Errorf("evaluation: decode archive manifest: %w", err)
	}
	return manifest, nil
}

// LoadArchiveManifest reads the manifest beside an archived run or campaign.
func LoadArchiveManifest(ctx context.Context, files fs.RuntimeFileService, kind ArchiveKind, id string) (*ArchiveManifest, error) {
	if !complianceevidence.ValidPathElement(id) {
		return nil, fmt.Errorf("%w: archive ID is invalid", constants.ErrEvidenceArtifactMalformed)
	}
	archiver := &Archiver{files: files}
	switch kind {
	case ArchiveKindRun:
		return archiver.readManifest(ctx, archivedRunDir(id))
	case ArchiveKindCampaign:
		return archiver.readManifest(ctx, archivedCampaignDir(id))
	default:
		return nil, fmt.Errorf("evaluation: unknown archive kind %q", kind)
	}
}
