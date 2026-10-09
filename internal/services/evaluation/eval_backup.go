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

// Manifest versions name the snapshot layout. Version 1 snapshots hold a full
// copy of every file beside the manifest. Version 2 snapshots hold only the
// manifest; file content lives once per distinct digest in the sibling
// evalBackupObjectsDir, shared by every snapshot.
const (
	evalBackupManifestVersionInline  = "1.0.0"
	evalBackupManifestVersionObjects = "2.0.0"
	evalBackupObjectsDir             = "objects"
)

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
	// SnapshotDir is the restored snapshot, or the backup directory when every
	// snapshot was merged by RestoreAll.
	SnapshotDir string
	// Snapshots counts the snapshots the restored file set was drawn from.
	Snapshots int
	Restored  []EvalBackupFile
	Unchanged []EvalBackupFile
}

// evalBackupSource is one file to restore and where its verified content is read from.
type evalBackupSource struct {
	file    EvalBackupFile
	payload string
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
// File content is stored once per digest beneath outputDir/objects, so only
// files the backup directory has not seen before are copied.
func (b *EvalBackup) Create(ctx context.Context, outputDir string) (*EvalBackupReport, error) {
	destRoot, err := b.validateDestination(outputDir)
	if err != nil {
		return nil, err
	}
	report, err := b.capture(ctx, destRoot)
	if err != nil {
		return nil, err
	}
	return b.commitSnapshot(ctx, destRoot, report)
}

// CreateIfChanged is Create for repeated automatic use: when the evidence is
// identical to the newest complete snapshot in outputDir, no snapshot is
// written and the report names the existing one with Unchanged set.
func (b *EvalBackup) CreateIfChanged(ctx context.Context, outputDir string) (*EvalBackupReport, error) {
	destRoot, err := b.validateDestination(outputDir)
	if err != nil {
		return nil, err
	}
	previousDir, err := LatestEvalBackupSnapshot(destRoot)
	if err != nil && !errors.Is(err, constants.ErrEvaluationBackupNone) {
		return nil, err
	}
	report, err := b.capture(ctx, destRoot)
	if err != nil {
		return nil, err
	}
	if previousDir != "" {
		previous, err := readEvalBackupManifest(previousDir)
		if err != nil {
			return nil, err
		}
		if evalBackupSameFiles(previous.Files, report.Files) {
			report.SnapshotDir = previousDir
			report.Unchanged = true
			return report, nil
		}
	}
	return b.commitSnapshot(ctx, destRoot, report)
}

// capture reads the evidence and stores any content the object store lacks.
// It writes no snapshot, so an unchanged tree costs reads and hashes only.
func (b *EvalBackup) capture(ctx context.Context, destRoot string) (*EvalBackupReport, error) {
	relPaths, err := b.collect(ctx)
	if err != nil {
		return nil, err
	}
	if len(relPaths) == 0 {
		return nil, constants.ErrEvaluationBackupEmpty
	}
	if err := os.MkdirAll(destRoot, constants.PermDirPrivate); err != nil {
		return nil, fmt.Errorf("evaluation: backup: create destination: %w", err)
	}
	report := &EvalBackupReport{CreatedAt: b.now().UTC()}
	for _, relPath := range relPaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := b.fileSvc.ReadFile(ctx, relPath)
		if err != nil {
			return nil, fmt.Errorf("evaluation: backup: read %s: %w", relPath, err)
		}
		file := EvalBackupFile{Path: relPath, Size: int64(len(data)), SHA256: evalBackupDigest(data)}
		if err := storeEvalBackupObject(destRoot, file, data); err != nil {
			return nil, err
		}
		report.Files = append(report.Files, file)
		report.TotalBytes += file.Size
	}
	return report, nil
}

// commitSnapshot reserves a snapshot directory and writes its manifest last,
// so a directory without one is an interrupted backup that is never restored.
func (b *EvalBackup) commitSnapshot(ctx context.Context, destRoot string, report *EvalBackupReport) (*EvalBackupReport, error) {
	snapshotBase := filepath.Join(destRoot, constants.EvaluationBackupDirPrefix+report.CreatedAt.Format(constants.EvaluationBackupTimestampLayout))
	snapshotDir := snapshotBase
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
	manifest, err := json.MarshalIndent(EvalBackupManifest{
		SchemaVersion: evalBackupManifestVersionObjects,
		CreatedAt:     report.CreatedAt.Format(time.RFC3339),
		Files:         report.Files,
	}, "", "  ")
	if err == nil {
		err = os.WriteFile(filepath.Join(snapshotDir, constants.EvaluationBackupManifestFilename), manifest, constants.PermFilePrivate)
	}
	if err != nil {
		// The directory was created by this call, so removing it cannot touch prior backups.
		_ = os.RemoveAll(snapshotDir)
		return nil, fmt.Errorf("evaluation: backup: write manifest: %w", err)
	}
	report.SnapshotDir = snapshotDir
	return report, nil
}

// evalBackupObjectPath is where the content for sha lives in the object store
// shared by the snapshots in backupDir.
func evalBackupObjectPath(backupDir, sha string) string {
	return filepath.Join(backupDir, evalBackupObjectsDir, sha[:2], sha)
}

// storeEvalBackupObject writes data to the object store unless an object of
// the expected size is already there. Objects appear atomically (temp file,
// then rename), so a present object is always complete.
func storeEvalBackupObject(backupDir string, file EvalBackupFile, data []byte) error {
	target := evalBackupObjectPath(backupDir, file.SHA256)
	if info, err := os.Stat(target); err == nil && info.Size() == file.Size {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), constants.PermDirPrivate); err != nil {
		return fmt.Errorf("evaluation: backup: create object directory for %s: %w", file.Path, err)
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".tmp-*")
	if err != nil {
		return fmt.Errorf("evaluation: backup: store %s: %w", file.Path, err)
	}
	tempName := temp.Name()
	_, err = temp.Write(data)
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tempName, constants.PermFilePrivate)
	}
	if err == nil {
		err = os.Rename(tempName, target)
	}
	if err != nil {
		_ = os.Remove(tempName)
		// Windows does not replace an existing file with Rename. Another
		// concurrent snapshot may have published this content-addressed object
		// after our initial Stat; in that case the desired result already exists.
		if info, statErr := os.Stat(target); statErr == nil && info.Size() == file.Size {
			return nil
		}
		return fmt.Errorf("evaluation: backup: store %s: %w", file.Path, err)
	}
	return nil
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
	abs, names, err := evalBackupSnapshotNames(backupDir)
	if err != nil {
		return "", err
	}
	for i := len(names) - 1; i >= 0; i-- {
		dir := filepath.Join(abs, names[i])
		if _, err := readEvalBackupManifest(dir); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("%w: %s", constants.ErrEvaluationBackupNone, abs)
}

// AllEvalBackupSnapshots returns all complete snapshot directories in backupDir,
// sorted oldest first (creation order). A snapshot without a valid manifest is
// an interrupted backup and is skipped. It returns ErrEvaluationBackupNone when
// there are no such snapshots.
func AllEvalBackupSnapshots(backupDir string) ([]string, error) {
	abs, names, err := evalBackupSnapshotNames(backupDir)
	if err != nil {
		return nil, err
	}
	var snapshots []string
	for _, name := range names {
		dir := filepath.Join(abs, name)
		if _, err := readEvalBackupManifest(dir); err == nil {
			snapshots = append(snapshots, dir)
		}
	}
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("%w: %s", constants.ErrEvaluationBackupNone, abs)
	}
	return snapshots, nil
}

// evalBackupSnapshotNames lists snapshot directory names oldest first. The
// timestamp layout is fixed-width UTC, so name order is creation order.
func evalBackupSnapshotNames(backupDir string) (string, []string, error) {
	abs, err := filepath.Abs(backupDir)
	if err != nil {
		return "", nil, fmt.Errorf("evaluation: backup: resolve directory: %w", err)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil, fmt.Errorf("%w: %s", constants.ErrEvaluationBackupNone, abs)
		}
		return "", nil, fmt.Errorf("evaluation: backup: list %s: %w", abs, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), constants.EvaluationBackupDirPrefix) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return abs, names, nil
}

// snapshotSources pairs each manifest entry with the file holding its content.
func snapshotSources(snapshotDir string, manifest *EvalBackupManifest) []evalBackupSource {
	sources := make([]evalBackupSource, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		payload := filepath.Join(snapshotDir, filepath.FromSlash(file.Path))
		if manifest.SchemaVersion == evalBackupManifestVersionObjects {
			payload = evalBackupObjectPath(filepath.Dir(snapshotDir), file.SHA256)
		}
		sources = append(sources, evalBackupSource{file: file, payload: payload})
	}
	return sources
}

// Restore restores one snapshot into the runtime tree. Existing files with
// identical content are left alone; differing ones fail the restore before any
// write unless overwrite is set.
func (b *EvalBackup) Restore(ctx context.Context, snapshotDir string, overwrite bool) (*EvalRestoreReport, error) {
	snapshotDir, err := filepath.Abs(snapshotDir)
	if err != nil {
		return nil, fmt.Errorf("evaluation: restore: resolve snapshot directory: %w", err)
	}
	manifest, err := readEvalBackupManifest(snapshotDir)
	if err != nil {
		return nil, err
	}
	report, err := b.restoreSources(ctx, snapshotSources(snapshotDir, manifest), overwrite)
	if err != nil {
		return nil, err
	}
	report.SnapshotDir = snapshotDir
	report.Snapshots = 1
	return report, nil
}

// RestoreAll restores every file any complete snapshot in backupDir recorded,
// taking each path from the newest snapshot that holds it. Evidence a later
// snapshot no longer lists (a deleted run, say) is still recovered, and the
// work is one pass over the merged set rather than one restore per snapshot.
func (b *EvalBackup) RestoreAll(ctx context.Context, backupDir string, overwrite bool) (*EvalRestoreReport, error) {
	snapshots, err := AllEvalBackupSnapshots(backupDir)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]evalBackupSource)
	for _, snapshotDir := range snapshots {
		manifest, err := readEvalBackupManifest(snapshotDir)
		if err != nil {
			return nil, err
		}
		for _, source := range snapshotSources(snapshotDir, manifest) {
			merged[source.file.Path] = source
		}
	}
	sources := make([]evalBackupSource, 0, len(merged))
	for _, source := range merged {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].file.Path < sources[j].file.Path })
	report, err := b.restoreSources(ctx, sources, overwrite)
	if err != nil {
		return nil, err
	}
	report.SnapshotDir = filepath.Dir(snapshots[0])
	report.Snapshots = len(snapshots)
	return report, nil
}

// restoreSources writes the sources into the runtime tree. Files already in
// place with the right content are skipped without reading the backup, and
// content is read one file at a time, so memory stays near one file. Every
// file that will be written is verified against its digest before the first
// write, so a corrupt backup changes nothing.
func (b *EvalBackup) restoreSources(ctx context.Context, sources []evalBackupSource, overwrite bool) (*EvalRestoreReport, error) {
	report := &EvalRestoreReport{}
	var pending []evalBackupSource
	var conflicts []EvalBackupFile
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		same, exists, err := b.matchesExisting(ctx, source.file)
		if err != nil {
			return nil, err
		}
		switch {
		case exists && same:
			report.Unchanged = append(report.Unchanged, source.file)
		case exists && !overwrite:
			conflicts = append(conflicts, source.file)
		default:
			pending = append(pending, source)
		}
	}
	if len(conflicts) > 0 {
		return nil, evalBackupConflictError(conflicts)
	}
	for _, source := range pending {
		if _, err := readVerifiedEvalBackupPayload(source); err != nil {
			return nil, err
		}
	}
	for _, source := range pending {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := readVerifiedEvalBackupPayload(source)
		if err != nil {
			return nil, err
		}
		if err := b.fileSvc.WriteFile(ctx, source.file.Path, data, constants.PermFilePrivate); err != nil {
			return nil, fmt.Errorf("evaluation: restore: write %s: %w", source.file.Path, err)
		}
		report.Restored = append(report.Restored, source.file)
	}
	return report, nil
}

func readVerifiedEvalBackupPayload(source evalBackupSource) ([]byte, error) {
	data, err := os.ReadFile(source.payload)
	if err != nil {
		return nil, fmt.Errorf("evaluation: restore: read %s: %w", source.file.Path, err)
	}
	if int64(len(data)) != source.file.Size || evalBackupDigest(data) != source.file.SHA256 {
		return nil, fmt.Errorf("evaluation: restore: %s: %w", source.file.Path, constants.ErrEvaluationBackupIntegrity)
	}
	return data, nil
}

// matchesExisting compares a runtime file with the backup entry, reading it
// only when its size already matches.
func (b *EvalBackup) matchesExisting(ctx context.Context, file EvalBackupFile) (same, exists bool, err error) {
	exists, err = b.fileSvc.FileExists(ctx, file.Path)
	if err != nil {
		return false, false, fmt.Errorf("evaluation: restore: check %s: %w", file.Path, err)
	}
	if !exists {
		return false, false, nil
	}
	info, err := b.fileSvc.Stat(ctx, file.Path)
	if err != nil {
		return false, true, fmt.Errorf("evaluation: restore: stat existing %s: %w", file.Path, err)
	}
	if info.Size() != file.Size {
		return false, true, nil
	}
	current, err := b.fileSvc.ReadFile(ctx, file.Path)
	if err != nil {
		return false, true, fmt.Errorf("evaluation: restore: read existing %s: %w", file.Path, err)
	}
	return evalBackupDigest(current) == file.SHA256, true, nil
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
	if manifest.SchemaVersion != evalBackupManifestVersionInline && manifest.SchemaVersion != evalBackupManifestVersionObjects {
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
