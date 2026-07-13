// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	vault "github.com/g8e-ai/g8e/internal/services/vault"
)

// ── Types ───────────────────────────────────────────────────────────────

// LedgerConfig holds configuration for the git-backed ledger service.
type LedgerConfig struct {
	BaseDir         string       // Base directory for ledgers
	GitPath         string       // Path to git binary
	EncryptionVault *vault.Vault // Vault for encryption at rest
}

// GitLedgerService maintains a git-backed version control of all files modified by the operator.
type GitLedgerService struct {
	config      *LedgerConfig
	logger      *slog.Logger
	fileSvc     fs.RuntimeFileService
	baseRelPath string
	mu          sync.Mutex
}

// LedgerResult contains the result of a file ledger operation.
type LedgerResult struct {
	FilePath         string
	Operation        FileMutationOperation
	LedgerHashBefore string
	LedgerHashAfter  string
	DiffStat         string
	DiffContent      string // Full diff content (raw, unscrubbed)
	LedgerPath       string
	Success          bool
	Error            string
}

// FileHistoryEntry represents a single entry in a file's history.
type FileHistoryEntry struct {
	CommitHash string
	Timestamp  time.Time
	Message    string
	FilePath   string
}

// LedgerCommit represents a single git commit in the ledger for export.
type LedgerCommit struct {
	CommitHash   string
	ParentHash   string
	TimestampUTC time.Time
	Message      string
	FilesChanged int
	DiffStat     string
}

// ── Constructor ─────────────────────────────────────────────────────────

// NewGitLedgerService creates a new GitLedgerService.
// EncryptionVault in config is required for encryption at rest.
// The base directory, files/ git repo, and sessions/ directory are eagerly
// initialized at construction time so the ledger is ready before any
// transactions occur.
func NewGitLedgerService(config *LedgerConfig, logger *slog.Logger, fileSvc fs.RuntimeFileService) (*GitLedgerService, error) {
	if config == nil {
		return nil, fmt.Errorf("ledger: %w", constants.ErrLedgerConfigRequired)
	}

	if config.EncryptionVault == nil {
		return nil, fmt.Errorf("ledger: %w", constants.ErrLedgerVaultRequired)
	}

	s := &GitLedgerService{
		config:      config,
		logger:      logger,
		fileSvc:     fileSvc,
		baseRelPath: filepath.Join(constants.DataDirname, constants.LedgerDirname),
	}

	if err := s.bootstrap(); err != nil {
		return nil, fmt.Errorf("ledger: %w", err)
	}

	return s, nil
}

// bootstrap creates the base directory structure and initializes the default
// files/ git repo and sessions/ directory eagerly at construction time.
func (s *GitLedgerService) bootstrap() error {
	filesRelPath := filepath.Join(s.baseRelPath, constants.FilesDirname)
	sessionsRelPath := filepath.Join(s.baseRelPath, constants.SessionsDirname)

	dirs := []string{
		s.baseRelPath,
		filesRelPath,
		sessionsRelPath,
	}

	for _, dir := range dirs {
		if err := s.fileSvc.MkdirAll(context.Background(), dir, constants.PermDirStandard); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrInternal, err)
		}
	}

	if err := s.initGitRepo(filesRelPath); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	s.logger.Info("Ledger bootstrapped",
		"base_dir", s.config.BaseDir,
		"files_dir", s.fileSvc.Resolve(filesRelPath),
		"sessions_dir", s.fileSvc.Resolve(sessionsRelPath))

	return nil
}

// ── State ───────────────────────────────────────────────────────────────

// gitReady returns true if the ledger can perform git operations.
// Nil-safe: a nil receiver or nil config short-circuits to false so callers
// that forward requests to an unconfigured ledger (e.g. HistoryHandler built
// without local storage) degrade gracefully instead of panicking.
func (s *GitLedgerService) gitReady() bool {
	if s == nil {
		return false
	}
	return s.config != nil && s.config.GitPath != ""
}

// ── Session management ──────────────────────────────────────────────────

// GetSessionLedgerPath returns the ledger path for a specific session, initializing it if needed.
func (s *GitLedgerService) GetSessionLedgerPath(operatorSessionID string) (string, error) {
	if operatorSessionID == "" {
		return s.fileSvc.Resolve(filepath.Join(s.baseRelPath, constants.FilesDirname)), nil
	}

	sessionRelPath := filepath.Join(s.baseRelPath, constants.SessionsDirname, operatorSessionID)
	gitRelPath := filepath.Join(sessionRelPath, constants.GitDirname)

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, _ := s.fileSvc.FileExists(context.Background(), gitRelPath)
	if exists {
		return s.fileSvc.Resolve(sessionRelPath), nil
	}

	if err := s.fileSvc.MkdirAll(context.Background(), sessionRelPath, constants.PermDirStandard); err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	if err := s.initGitRepo(sessionRelPath); err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	sessionPath := s.fileSvc.Resolve(sessionRelPath)
	s.logger.Info("Initialized new session ledger", "operator_session_id", operatorSessionID, "path", sessionPath)
	return sessionPath, nil
}

// initGitRepo initializes a git repository in the specified directory using native go-git.
func (s *GitLedgerService) initGitRepo(relPath string) error {
	gitRelPath := filepath.Join(relPath, constants.GitDirname)

	exists, _ := s.fileSvc.FileExists(context.Background(), gitRelPath)
	if exists {
		return nil
	}

	absPath := s.fileSvc.Resolve(relPath)
	repo, err := git.PlainInit(absPath, false)
	if err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	gitignoreRelPath := filepath.Join(relPath, constants.GitignoreFilename)
	if err := s.fileSvc.WriteFile(context.Background(), gitignoreRelPath, []byte("# g8e Ledger\n"), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	if _, err := w.Add(constants.GitignoreFilename); err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	_, err = w.Commit("Initial ledger commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "g8e-operator",
			Email: "g8e-operator@system",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	return nil
}

// ── Path helpers ────────────────────────────────────────────────────────

// normalizeToGitPath converts a file path to a git-relative path with forward slashes.
// This is the single source of truth for path normalization across the ledger service.
func (s *GitLedgerService) normalizeToGitPath(filePath string) string {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	cleanPath := absPath
	// Remove Windows drive letter (e.g., "C:")
	if len(cleanPath) >= 2 && cleanPath[1] == ':' {
		cleanPath = cleanPath[2:]
	}
	// Remove leading slash/backslash
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	cleanPath = strings.TrimPrefix(cleanPath, "\\")
	// Convert backslashes to forward slashes for consistent git paths
	cleanPath = strings.ReplaceAll(cleanPath, "\\", "/")

	return cleanPath
}

// getLedgerPath returns the path where a file should be mirrored in the ledger.
func (s *GitLedgerService) getLedgerPath(ledgerDir, filePath string) string {
	// Use normalizeToGitPath for consistent path normalization
	cleanPath := s.normalizeToGitPath(filePath)

	// Files are stored in 'files/' subdirectory within the ledger repository
	// Split the forward-slash path into components for proper filepath.Join behavior on Windows
	components := strings.Split(cleanPath, "/")
	pathParts := []string{ledgerDir, constants.FilesDirname}
	for _, comp := range components {
		if comp != "" {
			pathParts = append(pathParts, comp)
		}
	}
	return filepath.Join(pathParts...)
}

// getGitRelativePath returns the git-relative path for a file (always with forward slashes).
func (s *GitLedgerService) getGitRelativePath(filePath string) string {
	relPath := s.normalizeToGitPath(filePath)
	// The file is stored under "files/" in the git repository
	return constants.FilesDirname + "/" + relPath
}

// ── File copy ───────────────────────────────────────────────────────────

// copyToLedger copies a file from the host to the ledger, encrypting it if the vault is unlocked.
func (s *GitLedgerService) copyToLedger(srcPath, dstPath string) (err error) {
	relDstPath, err := s.fileSvc.Rel(dstPath)
	if err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	if s.config.EncryptionVault != nil && s.config.EncryptionVault.IsUnlocked() {
		info, err := os.Stat(srcPath)
		if err != nil {
			return fmt.Errorf("ledger: %w", constants.ErrStatFailed)
		}

		const maxEncryptedSize = 100 * 1024 * 1024 // 100MB safety limit
		if info.Size() > maxEncryptedSize {
			return fmt.Errorf("ledger: %w", constants.ErrInternal)
		}

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("ledger: %w", constants.ErrFileOpenFailed)
		}

		encrypted, err := s.config.EncryptionVault.Encrypt(content)
		if err != nil {
			return fmt.Errorf("ledger: %w", constants.ErrInternal)
		}

		if err := s.fileSvc.WriteFile(context.Background(), relDstPath+".enc", encrypted, constants.PermFilePrivate); err != nil {
			return fmt.Errorf("ledger: %w", constants.ErrInternal)
		}
		return nil
	}

	content, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrFileOpenFailed)
	}

	if err := s.fileSvc.WriteFile(context.Background(), relDstPath, content, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	return nil
}

// ── Two-phase mirror: Write ─────────────────────────────────────────────

// LedgerFileWrite begins the two-phase commit for a file write. Call CompleteMirrorWrite after the write.
func (s *GitLedgerService) LedgerFileWrite(operatorSessionID, filePath string) (*LedgerResult, error) {
	if !s.gitReady() {
		return nil, nil
	}

	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result := &LedgerResult{
		FilePath:  filePath,
		Operation: FileMutationWrite,
	}

	ledgerPath := s.getLedgerPath(ledgerDir, filePath)
	result.LedgerPath = ledgerPath

	if _, err := os.Stat(filePath); err == nil {
		if err := s.copyToLedger(filePath, ledgerPath); err != nil {
			result.Error = err.Error()
		}
	}

	hashBefore, err := s.snapshotLedger(ledgerDir, fmt.Sprintf("Pre-mutation backup: %s", filePath))
	if err != nil {
		s.logger.Warn("Failed to snapshot pre-mutation state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashBefore = hashBefore

	result.Success = true
	return result, nil
}

// CompleteMirrorWrite completes the mirror operation after the file write.
func (s *GitLedgerService) CompleteMirrorWrite(result *LedgerResult, operatorSessionID string) error {
	if !s.gitReady() || result == nil {
		return nil
	}

	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.copyToLedger(result.FilePath, result.LedgerPath); err != nil {
		result.Error = err.Error()
		return err
	}

	hashAfter, err := s.snapshotLedger(ledgerDir, fmt.Sprintf("Post-mutation: %s via OperatorSession %s", result.FilePath, operatorSessionID))
	if err != nil {
		s.logger.Warn("Failed to snapshot post-mutation state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashAfter = hashAfter

	result.DiffStat = s.calculateDiffStat(ledgerDir, result.LedgerHashBefore, result.LedgerHashAfter)
	result.DiffContent = s.calculateDiffContent(ledgerDir, result.LedgerHashBefore, result.LedgerHashAfter)

	s.logger.Info("File mutation mirrored",
		string(constants.ToolDisplayCategoryFile), result.FilePath,
		"hash_before", truncateHash(result.LedgerHashBefore),
		"hash_after", truncateHash(result.LedgerHashAfter),
		"diff_stat", result.DiffStat,
		"diff_size", len(result.DiffContent))

	return nil
}

// ── Two-phase mirror: Delete ────────────────────────────────────────────

// MirrorFileDelete begins the two-phase commit for a file deletion. Call CompleteMirrorDelete after the deletion.
func (s *GitLedgerService) MirrorFileDelete(operatorSessionID, filePath string) (*LedgerResult, error) {
	if !s.gitReady() {
		return nil, nil
	}

	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result := &LedgerResult{
		FilePath:  filePath,
		Operation: FileMutationDelete,
	}

	ledgerPath := s.getLedgerPath(ledgerDir, filePath)
	result.LedgerPath = ledgerPath

	if _, err := os.Stat(filePath); err == nil {
		if err := s.copyToLedger(filePath, ledgerPath); err != nil {
			s.logger.Warn("Failed to backup file before deletion", string(constants.ToolDisplayCategoryFile), filePath, string(constants.ConnectionStateError), err)
		}
	}

	hashBefore, err := s.snapshotLedger(ledgerDir, fmt.Sprintf("Pre-deletion backup: %s", filePath))
	if err != nil {
		s.logger.Warn("Failed to snapshot pre-deletion state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashBefore = hashBefore

	result.Success = true
	return result, nil
}

// CompleteMirrorDelete completes the mirror operation after file deletion.
func (s *GitLedgerService) CompleteMirrorDelete(result *LedgerResult, operatorSessionID string) error {
	if !s.gitReady() || result == nil {
		return nil
	}

	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	relLedgerPath, _ := s.fileSvc.Rel(result.LedgerPath)
	if err := s.fileSvc.Remove(context.Background(), relLedgerPath); err != nil && !errors.Is(err, constants.ErrNotFound) {
		s.logger.Warn("Failed to remove mirror file", "path", result.LedgerPath, string(constants.ConnectionStateError), err)
	}

	hashAfter, err := s.snapshotLedger(ledgerDir, fmt.Sprintf("Post-deletion: %s via OperatorSession %s", result.FilePath, operatorSessionID))
	if err != nil {
		s.logger.Warn("Failed to snapshot post-deletion state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashAfter = hashAfter

	result.DiffStat = constants.LedgerStatusFileDeleted
	result.DiffContent = s.calculateDiffContent(ledgerDir, result.LedgerHashBefore, result.LedgerHashAfter)

	s.logger.Info("File deletion mirrored",
		string(constants.ToolDisplayCategoryFile), result.FilePath,
		"hash_before", truncateHash(result.LedgerHashBefore),
		"hash_after", truncateHash(result.LedgerHashAfter),
		"diff_size", len(result.DiffContent))

	return nil
}

// ── Two-phase mirror: Create ────────────────────────────────────────────

// MirrorFileCreate begins the two-phase commit for a file creation. Call CompleteMirrorCreate after the creation.
func (s *GitLedgerService) MirrorFileCreate(operatorSessionID, filePath string) (*LedgerResult, error) {
	if !s.gitReady() {
		return nil, nil
	}

	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result := &LedgerResult{
		FilePath:  filePath,
		Operation: FileMutationCreate,
	}

	ledgerPath := s.getLedgerPath(ledgerDir, filePath)
	result.LedgerPath = ledgerPath

	hashBefore, err := s.snapshotLedger(ledgerDir, fmt.Sprintf("Pre-creation state for: %s", filePath))
	if err != nil {
		s.logger.Warn("Failed to snapshot pre-creation state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashBefore = hashBefore

	result.Success = true
	return result, nil
}

// CompleteMirrorCreate completes the mirror operation after file creation.
func (s *GitLedgerService) CompleteMirrorCreate(result *LedgerResult, operatorSessionID string) error {
	if !s.gitReady() || result == nil {
		return nil
	}

	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.copyToLedger(result.FilePath, result.LedgerPath); err != nil {
		result.Error = err.Error()
		return err
	}

	hashAfter, err := s.snapshotLedger(ledgerDir, fmt.Sprintf("Post-creation: %s via OperatorSession %s", result.FilePath, operatorSessionID))
	if err != nil {
		s.logger.Warn("Failed to snapshot post-creation state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashAfter = hashAfter

	if info, err := os.Stat(result.FilePath); err == nil {
		lineCount := s.countLines(result.FilePath)
		result.DiffStat = fmt.Sprintf("+%d lines, %d bytes (new file)", lineCount, info.Size())
	}

	result.DiffContent = s.calculateDiffContent(ledgerDir, result.LedgerHashBefore, result.LedgerHashAfter)

	s.logger.Info("File creation mirrored",
		string(constants.ToolDisplayCategoryFile), result.FilePath,
		"hash_after", truncateHash(result.LedgerHashAfter),
		"diff_stat", result.DiffStat,
		"diff_size", len(result.DiffContent))

	return nil
}

// ── History & Retrieval ─────────────────────────────────────────────────

// GetStateMerkleRoot returns the current git commit hash as the state merkle root.
// This provides a BFT-verifiable snapshot of the ledger state at a point in time.
func (s *GitLedgerService) GetStateMerkleRoot() (string, error) {
	if !s.gitReady() {
		return "", nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ledgerDir := s.fileSvc.Resolve(filepath.Join(s.baseRelPath, constants.FilesDirname))
	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}
	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}
	return ref.Hash().String(), nil
}

// ListCommits lists commits from the ledger repo for a session (or the files repo if sessionID is empty).
// Returns up to limit commits ordered newest-first (repo.Log order), then reversed to oldest-first for the CSV.
func (s *GitLedgerService) ListCommits(sessionID string, limit int) ([]LedgerCommit, error) {
	if !s.gitReady() {
		return nil, nil
	}

	if limit <= 0 {
		limit = 500
	}

	ledgerDir, err := s.GetSessionLedgerPath(sessionID)
	if err != nil {
		return nil, fmt.Errorf("ledger: list commits: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		return nil, nil
	}

	cIter, err := repo.Log(&git.LogOptions{})
	if err != nil {
		return nil, nil
	}
	defer cIter.Close()

	var commits []LedgerCommit
	count := 0
	_ = cIter.ForEach(func(c *object.Commit) error {
		if count >= limit {
			return storer.ErrStop
		}

		parentHash := ""
		if c.NumParents() > 0 {
			parentHash = c.ParentHashes[0].String()
		}

		stats, _ := c.Stats()
		filesChanged := 0
		diffStat := ""
		if stats != nil {
			filesChanged = len(stats)
			diffStat = stats.String()
		}

		commits = append(commits, LedgerCommit{
			CommitHash:   c.Hash.String(),
			ParentHash:   parentHash,
			TimestampUTC: c.Author.When.UTC(),
			Message:      strings.TrimSpace(c.Message),
			FilesChanged: filesChanged,
			DiffStat:     diffStat,
		})
		count++
		return nil
	})

	return commits, nil
}

// GetFileHistory retrieves the git history for a specific file.
func (s *GitLedgerService) GetFileHistory(filePath string, limit int, operatorSessionID string) ([]FileHistoryEntry, error) {
	if !s.gitReady() {
		return nil, fmt.Errorf("ledger: %w", constants.ErrLedgerDisabled)
	}

	if limit <= 0 {
		limit = 50
	}

	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	relPath := s.getGitRelativePath(filePath)

	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	// Get all commits and filter by file path
	cIter, err := repo.Log(&git.LogOptions{})
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", constants.ErrInternal)
	}
	defer cIter.Close()

	var entries []FileHistoryEntry
	count := 0
	err = cIter.ForEach(func(c *object.Commit) error {
		if count >= limit {
			return storer.ErrStop
		}

		// Check if this commit modified the file
		files, err := c.Files()
		if err != nil {
			return nil
		}

		found := false
		err = files.ForEach(func(file *object.File) error {
			filePath := file.Name
			// Normalize path for comparison (go-git may return backslashes on Windows)
			normalizedPath := filepath.ToSlash(filePath)
			// Strip .enc suffix if present (encrypted files are stored with .enc extension)
			normalizedPath = strings.TrimSuffix(normalizedPath, ".enc")
			if normalizedPath == relPath {
				found = true
			}
			return nil
		})
		if err != nil {
			return nil
		}

		if found {
			entries = append(entries, FileHistoryEntry{
				CommitHash: c.Hash.String(),
				Timestamp:  c.Author.When,
				Message:    strings.TrimSpace(c.Message),
				FilePath:   filePath,
			})
			count++
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	s.logger.Debug("GetFileHistory result", "entries", len(entries), "relPath", relPath)
	return entries, nil
}

// GetFileAtCommit retrieves the content of a file at a specific commit, decrypting if the vault is unlocked.
func (s *GitLedgerService) GetFileAtCommit(filePath, commitHash, operatorSessionID string) (string, error) {
	if !s.gitReady() {
		return "", fmt.Errorf("ledger: %w", constants.ErrLedgerDisabled)
	}

	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	relPath := s.getGitRelativePath(filePath)

	if s.config.EncryptionVault == nil || !s.config.EncryptionVault.IsUnlocked() {
		return "", fmt.Errorf("ledger: %w", constants.ErrLedgerVaultRequired)
	}

	encryptedRelPath := relPath + ".enc"
	content, err := s.gitShowFile(ledgerDir, commitHash, encryptedRelPath)
	if err != nil {
		return "", err
	}

	decrypted, err := s.config.EncryptionVault.Decrypt([]byte(content))
	if err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}
	return string(decrypted), nil
}

// RestoreFileFromCommit restores a file to its state at a specific commit.
func (s *GitLedgerService) RestoreFileFromCommit(filePath, commitHash, operatorSessionID string) error {
	if !s.gitReady() {
		return fmt.Errorf("ledger: %w", constants.ErrLedgerDisabled)
	}

	// Get session ledger path and file content before acquiring lock to avoid deadlock
	// GetFileAtCommit internally calls GetSessionLedgerPath which also acquires the mutex
	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	content, err := s.GetFileAtCommit(filePath, commitHash, operatorSessionID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ledgerPath := s.getLedgerPath(ledgerDir, filePath)
	if _, err := os.Stat(filePath); err == nil {
		if err := s.copyToLedger(filePath, ledgerPath); err != nil {
			s.logger.Warn("Failed to backup current state before restoration", string(constants.ConnectionStateError), err)
		}
	}

	_, _ = s.snapshotLedger(ledgerDir, fmt.Sprintf("Pre-restoration state: %s", filePath))

	if err := os.WriteFile(filePath, []byte(content), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	if err := s.copyToLedger(filePath, ledgerPath); err != nil {
		s.logger.Warn("Failed to mirror restored file", string(constants.ConnectionStateError), err)
	}

	_, _ = s.snapshotLedger(ledgerDir, fmt.Sprintf("Restored: %s to commit %s via OperatorSession %s", filePath, truncateHash(commitHash), operatorSessionID))

	s.logger.Info("File restored from commit",
		string(constants.ToolDisplayCategoryFile), filePath,
		"commit", truncateHash(commitHash),
		string(constants.SessionKeyPrefixOperator), operatorSessionID)

	return nil
}

// GetDiffContent returns the full diff content between two commits.
func (s *GitLedgerService) GetDiffContent(hashBefore, hashAfter string, operatorSessionID string) string {
	if !s.gitReady() {
		return ""
	}
	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		s.logger.Warn("Failed to get session ledger path for diff content", string(constants.ConnectionStateError), err)
		return ""
	}
	return s.calculateDiffContent(ledgerDir, hashBefore, hashAfter)
}

// GetDiffStat returns the diff statistics between two commits.
func (s *GitLedgerService) GetDiffStat(hashBefore, hashAfter string, operatorSessionID string) string {
	if !s.gitReady() {
		return ""
	}
	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		s.logger.Warn("Failed to get session ledger path for diff stat", string(constants.ConnectionStateError), err)
		return ""
	}
	return s.calculateDiffStat(ledgerDir, hashBefore, hashAfter)
}

// ── Internal git helpers ────────────────────────────────────────────────

// snapshotLedger creates a git commit and returns the commit hash.
func (s *GitLedgerService) snapshotLedger(ledgerDir, message string) (string, error) {
	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	w, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	err = w.AddWithOptions(&git.AddOptions{All: true})
	if err != nil && err != git.ErrEmptyCommit {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	commitMsg := fmt.Sprintf("[%s] %s", time.Now().UTC().Format(time.RFC3339), message)
	hash, err := w.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "g8e-operator",
			Email: "g8e-operator@system",
			When:  time.Now(),
		},
		AllowEmptyCommits: true,
	})
	if err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	return hash.String(), nil
}

// calculateDiffStat calculates the diff statistics between two commits.
func (s *GitLedgerService) calculateDiffStat(ledgerDir, hashBefore, hashAfter string) string {
	if hashBefore == "" || hashAfter == "" {
		return ""
	}

	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		s.logger.Warn("Failed to open git repo for diff stat", "error", err)
		return ""
	}

	commitBefore, err := repo.CommitObject(plumbing.NewHash(hashBefore))
	if err != nil {
		s.logger.Warn("Failed to get commitBefore for diff stat", "error", err)
		return ""
	}

	commitAfter, err := repo.CommitObject(plumbing.NewHash(hashAfter))
	if err != nil {
		s.logger.Warn("Failed to get commitAfter for diff stat", "error", err)
		return ""
	}

	patch, err := commitBefore.Patch(commitAfter)
	if err != nil {
		s.logger.Warn("Failed to generate patch for diff stat", "error", err)
		return ""
	}

	stats := patch.Stats()
	if len(stats) == 0 {
		return ""
	}

	numFiles := len(stats)
	totalAdd := 0
	totalDel := 0
	for _, s := range stats {
		totalAdd += s.Addition
		totalDel += s.Deletion
	}

	var parts []string
	if numFiles == 1 {
		parts = append(parts, "1 file changed")
	} else {
		parts = append(parts, fmt.Sprintf("%d files changed", numFiles))
	}

	if totalAdd > 0 {
		if totalAdd == 1 {
			parts = append(parts, "1 insertion(+)")
		} else {
			parts = append(parts, fmt.Sprintf("%d insertions(+)", totalAdd))
		}
	}

	if totalDel > 0 {
		if totalDel == 1 {
			parts = append(parts, "1 deletion(-)")
		} else {
			parts = append(parts, fmt.Sprintf("%d deletions(-)", totalDel))
		}
	}

	return strings.Join(parts, ", ")
}

// calculateDiffContent computes the full diff content between two commits.
func (s *GitLedgerService) calculateDiffContent(ledgerDir, hashBefore, hashAfter string) string {
	if hashBefore == "" || hashAfter == "" {
		return ""
	}

	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		s.logger.Warn("Failed to open git repo for diff content", "error", err)
		return ""
	}

	commitBefore, err := repo.CommitObject(plumbing.NewHash(hashBefore))
	if err != nil {
		s.logger.Warn("Failed to get commitBefore for diff content", "error", err)
		return ""
	}

	commitAfter, err := repo.CommitObject(plumbing.NewHash(hashAfter))
	if err != nil {
		s.logger.Warn("Failed to get commitAfter for diff content", "error", err)
		return ""
	}

	patch, err := commitBefore.Patch(commitAfter)
	if err != nil {
		s.logger.Warn("Failed to generate patch for diff content", "error", err)
		return ""
	}

	return patch.String()
}

// gitShowFile retrieves a file's content at a specific commit.
func (s *GitLedgerService) gitShowFile(ledgerDir, commitHash, relPath string) (string, error) {
	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	commit, err := repo.CommitObject(plumbing.NewHash(commitHash))
	if err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	file, err := commit.File(relPath)
	if err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrPathNotFound)
	}

	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("ledger: %w", constants.ErrInternal)
	}

	return content, nil
}

// countLines counts the number of lines in a file.
func (s *GitLedgerService) countLines(filePath string) int {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}
	return strings.Count(string(content), "\n") + 1
}

// ── Utilities ─────────────────────────────────────────────────────────

// truncateHash safely truncates a git hash for logging.
func truncateHash(hash string) string {
	if len(hash) >= 12 {
		return hash[:12]
	}
	return hash
}
