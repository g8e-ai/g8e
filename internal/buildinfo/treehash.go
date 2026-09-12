// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package buildinfo computes source-tree state hashes for build-time
// provenance stamping. The canonical digest covers each file's
// slash-separated manifest-relative path and the SHA-256 of its content,
// ordered by path-component tuples (matching Python's PurePath ordering so
// the format stays comparable with
// g8e_evals.preflight.compute_source_tree_state_hash).
//
// Two collection modes exist:
//
//   - Tracked-source mode (git): the manifest is `git ls-files --cached
//     --others --exclude-standard`, so .gitignore defines what counts as
//     source. Tracked symlinks hash their link target (git blob
//     semantics); tracked-but-deleted files record a non-hex marker.
//   - Manifest mode (no .git, e.g. Docker build contexts): an explicit
//     list of paths under the base root is walked, skipping entries whose
//     path components match the exclude patterns. Symlinks anywhere in a
//     walked tree are rejected.
package buildinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// missingContentMarker replaces the content digest for a tracked file that
// is absent from the working tree. It is deliberately not 64-char hex so a
// missing file can never collide with real content.
const missingContentMarker = "!missing!"

// hashedEntry pairs the canonical manifest key with the SHA-256 hex digest
// of the content that key represents (file bytes, symlink target, or the
// missing-content marker).
type hashedEntry struct {
	key    string
	digest string
}

// ComputeSourceTreeHash returns the canonical digest of every regular file
// beneath root, keyed by path relative to root. Symlinks are rejected.
// This is the single-directory form and matches the Python
// compute_source_tree_state_hash digest byte-for-byte when no excludes are
// given.
func ComputeSourceTreeHash(root string, excludes ...string) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("buildinfo: stat source root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s", constants.ErrSourceTreeNotDir, root)
	}
	var entries []hashedEntry
	if err := walkDir(root, root, excludes, &entries); err != nil {
		return "", err
	}
	return digestEntries(entries), nil
}

// ComputeSourceManifestHash returns the canonical digest of the listed
// entries under base. Entries are clean relative paths naming files or
// directories; each file's key is its path relative to base. Entries that
// do not exist, escape the base, or are symlinks are rejected.
func ComputeSourceManifestHash(base string, entries, excludes []string) (string, error) {
	baseInfo, err := os.Stat(base)
	if err != nil {
		return "", fmt.Errorf("buildinfo: stat source base: %w", err)
	}
	if !baseInfo.IsDir() {
		return "", fmt.Errorf("%w: %s", constants.ErrSourceTreeNotDir, base)
	}
	var files []hashedEntry
	for _, entry := range entries {
		if err := validateManifestEntry(entry); err != nil {
			return "", err
		}
		key := filepath.ToSlash(entry)
		if excludedKey(key, excludes) {
			continue
		}
		abs := filepath.Join(base, entry)
		lstat, err := os.Lstat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("%w: %s", constants.ErrSourceTreeEntryNotFound, entry)
			}
			return "", fmt.Errorf("buildinfo: lstat manifest entry %s: %w", entry, err)
		}
		if lstat.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: %s", constants.ErrSourceTreeSymlink, entry)
		}
		if lstat.IsDir() {
			if err := walkDir(base, abs, excludes, &files); err != nil {
				return "", err
			}
			continue
		}
		if !lstat.Mode().IsRegular() {
			continue
		}
		digest, err := hashFile(abs)
		if err != nil {
			return "", err
		}
		files = append(files, hashedEntry{key: key, digest: digest})
	}
	return digestEntries(files), nil
}

// ComputeTrackedSourceHash returns the canonical digest of the source
// manifest reported by `git ls-files --cached --others --exclude-standard`
// for the work tree containing repoRoot. File content comes from the
// working tree, so uncommitted changes are captured; untracked files that
// .gitignore excludes (venvs, build output, caches) are not part of the
// manifest at all.
func ComputeTrackedSourceHash(repoRoot string) (string, error) {
	topLevel, err := git(repoRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("buildinfo: resolve work tree root: %w", err)
	}
	out, err := git(topLevel, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return "", fmt.Errorf("buildinfo: list tracked source: %w", err)
	}
	var entries []hashedEntry
	for _, name := range strings.Split(out, "\x00") {
		if name == "" {
			continue
		}
		entry, err := hashWorktreeEntry(topLevel, name)
		if err != nil {
			return "", err
		}
		entries = append(entries, entry)
	}
	return digestEntries(entries), nil
}

// SourceTreeHash picks the collection mode for base: tracked-source mode
// when base is inside a git work tree, otherwise manifest mode over the
// given entries. Manifest mode with no entries is an error.
func SourceTreeHash(base string, entries, excludes []string) (string, error) {
	if _, err := git(base, "rev-parse", "--git-dir"); err == nil {
		return ComputeTrackedSourceHash(base)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("%w: no manifest entries supplied and %s is not a git work tree", constants.ErrSourceTreeEntryNotFound, base)
	}
	return ComputeSourceManifestHash(base, entries, excludes)
}

// hashWorktreeEntry digests one git-manifest path under repoRoot. Tracked
// symlinks hash their link target (the git blob content); tracked files
// missing from the working tree record the missing marker.
func hashWorktreeEntry(repoRoot, name string) (hashedEntry, error) {
	abs := filepath.Join(repoRoot, filepath.FromSlash(name))
	lstat, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return hashedEntry{key: name, digest: missingContentMarker}, nil
		}
		return hashedEntry{}, fmt.Errorf("buildinfo: lstat tracked source %s: %w", name, err)
	}
	if lstat.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(abs)
		if err != nil {
			return hashedEntry{}, fmt.Errorf("buildinfo: readlink tracked source %s: %w", name, err)
		}
		return hashedEntry{key: name, digest: hashBytes([]byte(target))}, nil
	}
	if !lstat.Mode().IsRegular() {
		return hashedEntry{key: name, digest: missingContentMarker}, nil
	}
	digest, err := hashFile(abs)
	if err != nil {
		return hashedEntry{}, err
	}
	return hashedEntry{key: name, digest: digest}, nil
}

// walkDir appends every regular file beneath absDir, keyed by path relative
// to base. Symlinks and excluded paths are rejected or skipped before
// descent.
func walkDir(base, absDir string, excludes []string, out *[]hashedEntry) error {
	return filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("buildinfo: walk %s: %w", path, err)
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return fmt.Errorf("buildinfo: relative path %s: %w", path, err)
		}
		key := filepath.ToSlash(rel)
		if d.IsDir() {
			if excludedKey(key, excludes) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", constants.ErrSourceTreeSymlink, key)
		}
		if !d.Type().IsRegular() || excludedKey(key, excludes) {
			return nil
		}
		digest, err := hashFile(path)
		if err != nil {
			return err
		}
		*out = append(*out, hashedEntry{key: key, digest: digest})
		return nil
	})
}

// digestEntries streams sorted (key, digest) pairs into one SHA-256 in the
// canonical form key \x00 digest \x00. Keys are ordered by path-component
// tuples and duplicate keys are collapsed after sorting.
func digestEntries(entries []hashedEntry) string {
	sort.Slice(entries, func(i, j int) bool { return lessPathParts(entries[i].key, entries[j].key) })
	hasher := sha256.New()
	var prev string
	for i, entry := range entries {
		if i > 0 && entry.key == prev {
			continue
		}
		prev = entry.key
		hasher.Write([]byte(entry.key))
		hasher.Write([]byte{0})
		hasher.Write([]byte(entry.digest))
		hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

// lessPathParts orders slash-separated relative paths the way Python's
// PurePath ordering does: component by component, a strict prefix first.
// This differs from plain string ordering for names like "a.b/d" vs
// "a/c", where '.' sorts before '/' byte-wise but "a" < "a.b" by parts.
func lessPathParts(a, b string) bool {
	as := strings.Split(a, "/")
	bs := strings.Split(b, "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] != bs[i] {
			return as[i] < bs[i]
		}
	}
	return len(as) < len(bs)
}

// excludedKey reports whether any path component of key matches any
// exclude pattern (filepath.Match syntax, e.g. ".venv" or "*.egg-info").
func excludedKey(key string, excludes []string) bool {
	for _, component := range strings.Split(key, "/") {
		for _, pattern := range excludes {
			if ok, err := filepath.Match(pattern, component); err == nil && ok {
				return true
			}
		}
	}
	return false
}

// validateManifestEntry rejects entries that are absolute, empty, or
// escape the base via "..".
func validateManifestEntry(entry string) error {
	if entry == "" || filepath.IsAbs(entry) {
		return fmt.Errorf("%w: %q", constants.ErrSourceTreeEntryInvalid, entry)
	}
	clean := filepath.Clean(entry)
	if clean != entry || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q", constants.ErrSourceTreeEntryInvalid, entry)
	}
	return nil
}

func hashFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("buildinfo: read %s: %w", path, err)
	}
	return hashBytes(content), nil
}

func hashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("buildinfo: git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}
