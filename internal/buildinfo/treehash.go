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
// Collection uses an explicit manifest of paths under the base root, skipping
// entries whose path components match the exclude patterns. Symlinks anywhere
// in a walked tree are rejected.
package buildinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// hashedEntry pairs the canonical manifest key with the SHA-256 hex digest
// of the file bytes that key represents.
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
