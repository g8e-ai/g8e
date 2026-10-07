// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

const evalBackupManifestVersion = "1.0.0"

// evalBackupConflictSample bounds how many conflicting paths a restore error names.
const evalBackupConflictSample = 5

// EvalBackupFile is one file captured in a backup snapshot. Path is runtime-relative.
type EvalBackupFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// EvalBackupManifest is the integrity record written last into a snapshot. A
// snapshot directory without one is incomplete and is never restored.
type EvalBackupManifest struct {
	SchemaVersion string           `json:"schema_version"`
	CreatedAt     string           `json:"created_at"`
	Files         []EvalBackupFile `json:"files"`
}

// EvalBackupReport describes a completed backup.
type EvalBackupReport struct {
	SnapshotDir string
	CreatedAt   time.Time
	Files       []EvalBackupFile
	TotalBytes  int64
	// Unchanged is set by CreateIfChanged when the evidence matched the newest
	// existing snapshot, in which case SnapshotDir names that snapshot and
	// nothing new was kept.
	Unchanged bool
}

// EvalRestoreReport describes a completed restore.
type EvalRestoreReport struct {
	SnapshotDir string
	Restored    []EvalBackupFile
	Unchanged   []EvalBackupFile
}

// EvalBackup copies evaluation evidence between the runtime tree and a plain
// directory outside it. Runtime I/O goes through the file service; the
// snapshot side is ordinary host storage, not runtime state.
type EvalBackup struct {
	fileSvc fs.RuntimeFileService
	now     func() time.Time
}

func NewEvalBackup(fileSvc fs.RuntimeFileService, now func() time.Time) *EvalBackup {
	return &EvalBackup{fileSvc: fileSvc, now: now}
}

// evalBackupRoots are the runtime-relative trees a backup covers: run and
// campaign evidence, and the rollout queue with the frozen model inventory.
func evalBackupRoots() []string {
	return []string{constants.EvaluationDataPath, constants.EvaluationDirname}
}

// evalBackupTransient reports process-local state that must not be carried
// across a wipe: a restored lease would make a dead run look held.
func evalBackupTransient(relPath string) bool {
	switch path.Base(relPath) {
	case constants.EvaluationRunLeaseFilename, constants.EvaluationActiveRunFilename:
		return true
	}
	return false
}

// Create writes a new timestamped snapshot under outputDir and returns it.
func (b *EvalBackup) Create(ctx context.Context, outputDir string) (*EvalBackupReport, error) {
	destRoot, err := b.validateDestination(outputDir)
	if err != nil {
		return nil, err
	}
	files, err := b.collect(ctx)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, constants.ErrEvaluationBackupEmpty
	}

	createdAt := b.now().UTC()
	snapshotBase := filepath.Join(destRoot, constants.EvaluationBackupDirPrefix+createdAt.Format(constants.EvaluationBackupTimestampLayout))
	snapshotDir := snapshotBase
	if err := os.MkdirAll(destRoot, constants.PermDirPrivate); err != nil {
		return nil, fmt.Errorf("evaluation: backup: create destination: %w", err)
	}
	// Reserve each name atomically so simultaneous and same-second backups
	// cannot overwrite one another. Keep the unsuffixed name for the first.
	for sequence := 1; ; sequence++ {
		err := os.Mkdir(snapshotDir, constants.PermDirPrivate)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("evaluation: backup: create snapshot directory: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snapshotDir = fmt.Sprintf("%s-%06d", snapshotBase, sequence)
	}

	report, err := b.writeSnapshot(ctx, snapshotDir, createdAt, files)
	if err != nil {
		// The directory was created by this call, so removing it cannot touch prior backups.
		_ = os.RemoveAll(snapshotDir)
		return nil, err
	}
	return report, nil
}

// CreateIfChanged is Create for repeated automatic use: when the evidence is
// identical to the newest complete snapshot in outputDir, the new snapshot is
// discarded and the report names the existing one with Unchanged set.
func (b *EvalBackup) CreateIfChanged(ctx context.Context, outputDir string) (*EvalBackupReport, error) {
	destRoot, err := b.validateDestination(outputDir)
	if err != nil {
		return nil, err
	}
	previousDir, err := LatestEvalBackupSnapshot(destRoot)
	if err != nil && !errors.Is(err, constants.ErrEvaluationBackupNone) {
		return nil, err
	}
	report, err := b.Create(ctx, destRoot)
	if err != nil || previousDir == "" {
		return report, err
	}
	previous, err := readEvalBackupManifest(previousDir)
	if err != nil {
		return nil, err
	}
	if !evalBackupSameFiles(previous.Files, report.Files) {
		return report, nil
	}
	// The directory was created by this call, so removing it cannot touch prior backups.
	if err := os.RemoveAll(report.SnapshotDir); err != nil {
		return nil, fmt.Errorf("evaluation: backup: discard unchanged snapshot: %w", err)
	}
	report.SnapshotDir = previousDir
	report.Unchanged = true
	return report, nil
}

func evalBackupSameFiles(a, b []EvalBackupFile) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// LatestEvalBackupSnapshot returns the newest complete snapshot directory in
// backupDir. A snapshot without a valid manifest is an interrupted backup and
// is skipped. It returns ErrEvaluationBackupNone when there is no such snapshot.
func LatestEvalBackupSnapshot(backupDir string) (string, error) {
	abs, err := filepath.Abs(backupDir)
	if err != nil {
		return "", fmt.Errorf("evaluation: backup: resolve directory: %w", err)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", constants.ErrEvaluationBackupNone, abs)
		}
		return "", fmt.Errorf("evaluation: backup: list %s: %w", abs, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), constants.EvaluationBackupDirPrefix) {
			names = append(names, entry.Name())
		}
	}
	// The timestamp layout is fixed-width UTC, so name order is creation order.
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names {
		dir := filepath.Join(abs, name)
		if _, err := readEvalBackupManifest(dir); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("%w: %s", constants.ErrEvaluationBackupNone, abs)
}

func (b *EvalBackup) writeSnapshot(ctx context.Context, snapshotDir string, createdAt time.Time, relPaths []string) (*EvalBackupReport, error) {
	report := &EvalBackupReport{SnapshotDir: snapshotDir, CreatedAt: createdAt}
	for _, relPath := range relPaths {
		data, err := b.fileSvc.ReadFile(ctx, relPath)
		if err != nil {
			return nil, fmt.Errorf("evaluation: backup: read %s: %w", relPath, err)
		}
		target := filepath.Join(snapshotDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(target), constants.PermDirPrivate); err != nil {
			return nil, fmt.Errorf("evaluation: backup: create directory for %s: %w", relPath, err)
		}
		if err := os.WriteFile(target, data, constants.PermFilePrivate); err != nil {
			return nil, fmt.Errorf("evaluation: backup: write %s: %w", relPath, err)
		}
		report.Files = append(report.Files, EvalBackupFile{Path: relPath, Size: int64(len(data)), SHA256: evalBackupDigest(data)})
		report.TotalBytes += int64(len(data))
	}

	manifest, err := json.MarshalIndent(EvalBackupManifest{
		SchemaVersion: evalBackupManifestVersion,
		CreatedAt:     createdAt.Format(time.RFC3339),
		Files:         report.Files,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("evaluation: backup: encode manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(snapshotDir, constants.EvaluationBackupManifestFilename), manifest, constants.PermFilePrivate); err != nil {
		return nil, fmt.Errorf("evaluation: backup: write manifest: %w", err)
	}
	return report, nil
}

// Restore verifies every file in a snapshot against its manifest, then writes
// them into the runtime tree. Existing files with identical content are left
// alone; differing ones fail the restore before any write unless overwrite is set.
func (b *EvalBackup) Restore(ctx context.Context, snapshotDir string, overwrite bool) (*EvalRestoreReport, error) {
	snapshotDir, err := filepath.Abs(snapshotDir)
	if err != nil {
		return nil, fmt.Errorf("evaluation: restore: resolve snapshot directory: %w", err)
	}
	manifest, err := readEvalBackupManifest(snapshotDir)
	if err != nil {
		return nil, err
	}

	payloads := make(map[string][]byte, len(manifest.Files))
	for _, file := range manifest.Files {
		data, err := os.ReadFile(filepath.Join(snapshotDir, filepath.FromSlash(file.Path)))
		if err != nil {
			return nil, fmt.Errorf("evaluation: restore: read %s: %w", file.Path, err)
		}
		if int64(len(data)) != file.Size || evalBackupDigest(data) != file.SHA256 {
			return nil, fmt.Errorf("evaluation: restore: %s: %w", file.Path, constants.ErrEvaluationBackupIntegrity)
		}
		payloads[file.Path] = data
	}

	report := &EvalRestoreReport{SnapshotDir: snapshotDir}
	var pending, conflicts []EvalBackupFile
	for _, file := range manifest.Files {
		same, exists, err := b.matchesExisting(ctx, file.Path, file.SHA256)
		if err != nil {
			return nil, err
		}
		switch {
		case exists && same:
			report.Unchanged = append(report.Unchanged, file)
		case exists && !overwrite:
			conflicts = append(conflicts, file)
		default:
			pending = append(pending, file)
		}
	}
	if len(conflicts) > 0 {
		return nil, evalBackupConflictError(conflicts)
	}

	for _, file := range pending {
		if err := b.fileSvc.WriteFile(ctx, file.Path, payloads[file.Path], constants.PermFilePrivate); err != nil {
			return nil, fmt.Errorf("evaluation: restore: write %s: %w", file.Path, err)
		}
		report.Restored = append(report.Restored, file)
	}
	return report, nil
}

func (b *EvalBackup) matchesExisting(ctx context.Context, relPath, digest string) (same, exists bool, err error) {
	exists, err = b.fileSvc.FileExists(ctx, relPath)
	if err != nil {
		return false, false, fmt.Errorf("evaluation: restore: check %s: %w", relPath, err)
	}
	if !exists {
		return false, false, nil
	}
	current, err := b.fileSvc.ReadFile(ctx, relPath)
	if err != nil {
		return false, true, fmt.Errorf("evaluation: restore: read existing %s: %w", relPath, err)
	}
	return evalBackupDigest(current) == digest, true, nil
}

func evalBackupConflictError(conflicts []EvalBackupFile) error {
	names := make([]string, 0, evalBackupConflictSample)
	for i, file := range conflicts {
		if i == evalBackupConflictSample {
			break
		}
		names = append(names, file.Path)
	}
	more := ""
	if extra := len(conflicts) - len(names); extra > 0 {
		more = fmt.Sprintf(" and %d more", extra)
	}
	return fmt.Errorf("evaluation: restore: %d file(s) differ from the backup (%s%s); pass --overwrite to replace them: %w",
		len(conflicts), strings.Join(names, ", "), more, constants.ErrEvaluationBackupConflict)
}

// collect lists every runtime-relative file under the backup roots, sorted.
func (b *EvalBackup) collect(ctx context.Context) ([]string, error) {
	seen := make(map[string]struct{})
	var files []string
	for _, root := range evalBackupRoots() {
		if err := b.walk(ctx, root, func(relPath string) {
			if _, dup := seen[relPath]; dup {
				return
			}
			seen[relPath] = struct{}{}
			files = append(files, relPath)
		}); err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

func (b *EvalBackup) walk(ctx context.Context, relDir string, visit func(string)) error {
	entries, err := b.fileSvc.ReadDir(ctx, relDir)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("evaluation: backup: list %s: %w", relDir, err)
	}
	for _, entry := range entries {
		relPath := path.Join(relDir, entry.Name())
		switch {
		case entry.IsDir():
			if err := b.walk(ctx, relPath, visit); err != nil {
				return err
			}
		case entry.Type().IsRegular():
			if !evalBackupTransient(relPath) {
				visit(relPath)
			}
		default:
			return fmt.Errorf("evaluation: backup: %s: %w", relPath, constants.ErrEvaluationBackupUnsupportedEntry)
		}
	}
	return nil
}

// validateDestination resolves outputDir to an absolute path and rejects any
// location inside the runtime directory, where a wipe or the backup's own
// source walk would reach it.
func (b *EvalBackup) validateDestination(outputDir string) (string, error) {
	if strings.TrimSpace(outputDir) == "" {
		return "", fmt.Errorf("%w: empty path", constants.ErrEvaluationBackupDestinationInvalid)
	}
	abs, err := filepath.Abs(outputDir)
	if err != nil {
		return "", fmt.Errorf("evaluation: backup: resolve destination: %w", err)
	}
	runtimeDir := b.fileSvc.Resolve("")
	if evalBackupWithin(runtimeDir, abs) || evalBackupWithin(evalBackupResolveExisting(runtimeDir), evalBackupResolveExisting(abs)) {
		return "", fmt.Errorf("%w: %s", constants.ErrEvaluationBackupDestinationInvalid, abs)
	}
	return abs, nil
}

func evalBackupWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// evalBackupResolveExisting resolves symlinks in the longest existing prefix of
// p, so a not-yet-created destination beneath a symlinked directory still resolves.
func evalBackupResolveExisting(p string) string {
	remainder := ""
	for current := p; ; {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(resolved, remainder)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return p
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
	}
}

func readEvalBackupManifest(snapshotDir string) (*EvalBackupManifest, error) {
	raw, err := os.ReadFile(filepath.Join(snapshotDir, constants.EvaluationBackupManifestFilename))
	if err != nil {
		return nil, fmt.Errorf("evaluation: restore: read manifest in %s: %w: %w", snapshotDir, constants.ErrEvaluationBackupManifestInvalid, err)
	}
	var manifest EvalBackupManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("evaluation: restore: decode manifest: %w: %w", constants.ErrEvaluationBackupManifestInvalid, err)
	}
	if manifest.SchemaVersion != evalBackupManifestVersion {
		return nil, fmt.Errorf("evaluation: restore: manifest schema %q: %w", manifest.SchemaVersion, constants.ErrEvaluationBackupManifestInvalid)
	}
	seen := make(map[string]struct{}, len(manifest.Files))
	for _, file := range manifest.Files {
		if err := validateEvalBackupEntry(file); err != nil {
			return nil, err
		}
		if _, dup := seen[file.Path]; dup {
			return nil, fmt.Errorf("evaluation: restore: duplicate manifest path %q: %w", file.Path, constants.ErrEvaluationBackupManifestInvalid)
		}
		seen[file.Path] = struct{}{}
	}
	return &manifest, nil
}

// validateEvalBackupEntry confines a manifest path to the backup roots so a
// tampered manifest cannot direct a write elsewhere in the runtime tree.
func validateEvalBackupEntry(file EvalBackupFile) error {
	if cleaned := path.Clean(file.Path); cleaned != file.Path || path.IsAbs(file.Path) || file.Path == "." {
		return fmt.Errorf("evaluation: restore: manifest path %q is not a clean relative path: %w", file.Path, constants.ErrEvaluationBackupManifestInvalid)
	}
	inRoot := false
	for _, root := range evalBackupRoots() {
		if strings.HasPrefix(file.Path, root+"/") {
			inRoot = true
			break
		}
	}
	if !inRoot || evalBackupTransient(file.Path) {
		return fmt.Errorf("evaluation: restore: manifest path %q is outside the evaluation evidence trees: %w", file.Path, constants.ErrEvaluationBackupManifestInvalid)
	}
	if file.Size < 0 || len(file.SHA256) != sha256.Size*2 {
		return fmt.Errorf("evaluation: restore: manifest entry %q has an invalid size or digest: %w", file.Path, constants.ErrEvaluationBackupManifestInvalid)
	}
	return nil
}

func evalBackupDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
