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
	"fmt"
	"io"
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
	config *LedgerConfig
	logger *slog.Logger
	mu     sync.Mutex
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

// ── Constructor ─────────────────────────────────────────────────────────

// NewGitLedgerService creates a new GitLedgerService.
// EncryptionVault in config is required for encryption at rest.
func NewGitLedgerService(config *LedgerConfig, logger *slog.Logger) (*GitLedgerService, error) {
	if config == nil {
		return nil, fmt.Errorf("ledger: config is required for git ledger service")
	}

	if config.EncryptionVault == nil {
		return nil, fmt.Errorf("ledger: EncryptionVault is required for git ledger service")
	}

	return &GitLedgerService{
		config: config,
		logger: logger,
	}, nil
}

// ── State ───────────────────────────────────────────────────────────────

// IsEncryptionEnabled returns whether file encryption is enabled.
func (s *GitLedgerService) IsEncryptionEnabled() bool {
	return s.config.EncryptionVault != nil && s.config.EncryptionVault.IsUnlocked()
}

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
		return filepath.Join(s.config.BaseDir, "files"), nil
	}

	sessionsRoot := filepath.Join(s.config.BaseDir, "sessions")
	sessionPath := filepath.Join(sessionsRoot, operatorSessionID)

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := os.Stat(filepath.Join(sessionPath, ".git"))
	if err == nil {
		return sessionPath, nil
	}

	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		return "", fmt.Errorf("ledger: failed to create Operator session ledger directory: %w", err)
	}

	if err := s.initGitRepo(sessionPath); err != nil {
		return "", fmt.Errorf("ledger: failed to initialize Operator session git repo: %w", err)
	}

	s.logger.Info("Initialized new session ledger", "operator_session_id", operatorSessionID, "path", sessionPath)
	return sessionPath, nil
}

// initGitRepo initializes a git repository in the specified directory using native go-git.
func (s *GitLedgerService) initGitRepo(path string) error {
	gitDir := filepath.Join(path, ".git")

	if _, err := os.Stat(gitDir); err == nil {
		return nil
	}

	repo, err := git.PlainInit(path, false)
	if err != nil {
		return fmt.Errorf("ledger: git init failed: %w", err)
	}

	gitignore := filepath.Join(path, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("# g8e Ledger\n"), 0600); err != nil {
		return fmt.Errorf("ledger: failed to create .gitignore: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("ledger: failed to get worktree: %w", err)
	}

	if _, err := w.Add(".gitignore"); err != nil {
		return fmt.Errorf("ledger: failed to git add .gitignore: %w", err)
	}

	_, err = w.Commit("Initial ledger commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "g8e-operator",
			Email: "g8e-operator@system",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("ledger: failed to create initial commit: %w", err)
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
	pathParts := []string{ledgerDir, "files"}
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
	return "files/" + relPath
}

// ── File copy ───────────────────────────────────────────────────────────

// copyToLedger copies a file from the host to the ledger, encrypting it if the vault is unlocked.
// It uses streaming for unencrypted files to prevent OOM.
func (s *GitLedgerService) copyToLedger(srcPath, dstPath string) error {
	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("ledger: failed to create mirror directory: %w", err)
	}

	if s.IsEncryptionEnabled() {
		// For encrypted files, we currently read the whole content since the Vault API
		// only supports byte-slice encryption (AES-GCM).
		// We limit the size to prevent OOM.
		info, err := os.Stat(srcPath)
		if err != nil {
			return fmt.Errorf("ledger: failed to stat source file: %w", err)
		}

		const maxEncryptedSize = 100 * 1024 * 1024 // 100MB safety limit
		if info.Size() > maxEncryptedSize {
			return fmt.Errorf("ledger: file too large for encrypted ledger mirror: %d bytes (max %d)", info.Size(), maxEncryptedSize)
		}

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("ledger: failed to read source file: %w", err)
		}

		encrypted, err := s.config.EncryptionVault.Encrypt(content)
		if err != nil {
			return fmt.Errorf("ledger: failed to encrypt file content: %w", err)
		}

		if err := os.WriteFile(dstPath+".enc", encrypted, 0600); err != nil {
			return fmt.Errorf("ledger: failed to write encrypted destination file: %w", err)
		}
		return nil
	}

	// For unencrypted files, use streaming
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("ledger: failed to open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("ledger: failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("ledger: failed to stream copy to ledger: %w", err)
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
		return nil, fmt.Errorf("ledger: failed to get session ledger path: %w", err)
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
			result.Error = fmt.Sprintf("failed to copy file to ledger: %v", err)
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
		return fmt.Errorf("ledger: failed to get session ledger path: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.copyToLedger(result.FilePath, result.LedgerPath); err != nil {
		result.Error = fmt.Sprintf("failed to copy post-mutation file to ledger: %v", err)
		return fmt.Errorf("ledger: failed to copy post-mutation file to ledger: %w", err)
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
		return nil, fmt.Errorf("ledger: failed to get session ledger path: %w", err)
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
		return fmt.Errorf("ledger: failed to get session ledger path: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(result.LedgerPath); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("Failed to remove mirror file", "path", result.LedgerPath, string(constants.ConnectionStateError), err)
	}

	hashAfter, err := s.snapshotLedger(ledgerDir, fmt.Sprintf("Post-deletion: %s via OperatorSession %s", result.FilePath, operatorSessionID))
	if err != nil {
		s.logger.Warn("Failed to snapshot post-deletion state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashAfter = hashAfter

	result.DiffStat = "file deleted"
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
		return nil, fmt.Errorf("ledger: failed to get session ledger path: %w", err)
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
		return fmt.Errorf("ledger: failed to get session ledger path: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.copyToLedger(result.FilePath, result.LedgerPath); err != nil {
		result.Error = fmt.Sprintf("failed to copy created file to ledger: %v", err)
		return fmt.Errorf("ledger: failed to copy created file to ledger: %w", err)
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

	ledgerDir := filepath.Join(s.config.BaseDir, "files")
	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		return "", fmt.Errorf("ledger: failed to open ledger git repo: %w", err)
	}
	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("ledger: failed to get HEAD ref: %w", err)
	}
	return ref.Hash().String(), nil
}

// GetFileHistory retrieves the git history for a specific file.
func (s *GitLedgerService) GetFileHistory(filePath string, limit int, operatorSessionID string) ([]FileHistoryEntry, error) {
	if !s.gitReady() {
		return nil, fmt.Errorf("ledger is disabled")
	}

	if limit <= 0 {
		limit = 50
	}

	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return nil, fmt.Errorf("ledger: failed to get session ledger path: %w", err)
	}

	relPath := s.getGitRelativePath(filePath)

	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		return nil, fmt.Errorf("ledger: failed to open git repo: %w", err)
	}

	// Get all commits and filter by file path
	cIter, err := repo.Log(&git.LogOptions{})
	if err != nil {
		return nil, fmt.Errorf("ledger: git log failed: %w", err)
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
		return nil, fmt.Errorf("ledger: failed to iterate commits: %w", err)
	}

	s.logger.Debug("GetFileHistory result", "entries", len(entries), "relPath", relPath)
	return entries, nil
}

// GetFileAtCommit retrieves the content of a file at a specific commit, decrypting if the vault is unlocked.
func (s *GitLedgerService) GetFileAtCommit(filePath, commitHash, operatorSessionID string) (string, error) {
	if !s.gitReady() {
		return "", fmt.Errorf("ledger is disabled")
	}

	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return "", fmt.Errorf("ledger: failed to get session ledger path: %w", err)
	}

	relPath := s.getGitRelativePath(filePath)

	if !s.IsEncryptionEnabled() {
		return "", fmt.Errorf("ledger: vault is locked, cannot decrypt file from ledger")
	}

	encryptedRelPath := relPath + ".enc"
	content, err := s.gitShowFile(ledgerDir, commitHash, encryptedRelPath)
	if err != nil {
		return "", fmt.Errorf("ledger: encrypted file not found in commit: %w", err)
	}

	decrypted, err := s.config.EncryptionVault.Decrypt([]byte(content))
	if err != nil {
		return "", fmt.Errorf("ledger: failed to decrypt file content: %w", err)
	}
	return string(decrypted), nil
}

// RestoreFileFromCommit restores a file to its state at a specific commit.
func (s *GitLedgerService) RestoreFileFromCommit(filePath, commitHash, operatorSessionID string) error {
	if !s.gitReady() {
		return fmt.Errorf("ledger is disabled")
	}

	// Get session ledger path and file content before acquiring lock to avoid deadlock
	// GetFileAtCommit internally calls GetSessionLedgerPath which also acquires the mutex
	ledgerDir, err := s.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return fmt.Errorf("ledger: failed to get session ledger path: %w", err)
	}

	content, err := s.GetFileAtCommit(filePath, commitHash, operatorSessionID)
	if err != nil {
		return fmt.Errorf("ledger: failed to get file at commit: %w", err)
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

	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		return fmt.Errorf("ledger: failed to write restored file: %w", err)
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
		return "", fmt.Errorf("ledger: failed to open git repo: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("ledger: failed to get worktree: %w", err)
	}

	err = w.AddWithOptions(&git.AddOptions{All: true})
	if err != nil && err != git.ErrEmptyCommit {
		return "", fmt.Errorf("ledger: git add failed: %w", err)
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
		return "", fmt.Errorf("ledger: git commit failed: %w", err)
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
		return "", fmt.Errorf("ledger: failed to open git repo: %w", err)
	}

	commit, err := repo.CommitObject(plumbing.NewHash(commitHash))
	if err != nil {
		return "", fmt.Errorf("ledger: failed to find commit %s: %w", commitHash, err)
	}

	file, err := commit.File(relPath)
	if err != nil {
		return "", fmt.Errorf("ledger: failed to find file %s in commit %s: %w", relPath, commitHash, err)
	}

	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("ledger: failed to read file contents: %w", err)
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
