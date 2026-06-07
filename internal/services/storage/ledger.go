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

	"github.com/g8e-ai/g8e/internal/constants"
	vault "github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// LedgerConfig holds configuration for the git-backed ledger service
type LedgerConfig struct {
	BaseDir         string // Base directory for ledgers
	GitPath         string // Path to git binary
	EncryptionVault *vault.Vault
}

// GitLedgerService maintains a git-backed version control of all files modified by the operator.
type GitLedgerService struct {
	config          *LedgerConfig
	encryptionVault *vault.Vault
	logger          *slog.Logger

	mu sync.Mutex
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

// NewGitLedgerService creates a new GitLedgerService.
// EncryptionVault in config is required for encryption at rest.
func NewGitLedgerService(config *LedgerConfig, logger *slog.Logger) (*GitLedgerService, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required for git ledger service")
	}

	if config.EncryptionVault == nil {
		return nil, fmt.Errorf("EncryptionVault is required for git ledger service")
	}

	return &GitLedgerService{
		config:          config,
		encryptionVault: config.EncryptionVault,
		logger:          logger,
	}, nil
}

// IsEncryptionEnabled returns whether file encryption is enabled.
func (lms *GitLedgerService) IsEncryptionEnabled() bool {
	return lms.encryptionVault != nil && lms.encryptionVault.IsUnlocked()
}

// gitReady returns true if the ledger can perform git operations.
// Nil-safe: a nil receiver or nil config short-circuits to false so callers
// that forward requests to an unconfigured ledger (e.g. HistoryHandler built
// without local storage) degrade gracefully instead of panicking.
func (lms *GitLedgerService) gitReady() bool {
	if lms == nil {
		return false
	}
	return lms.config != nil && lms.config.GitPath != ""
}

// truncateHash safely truncates a git hash for logging.
func truncateHash(hash string) string {
	if len(hash) >= 12 {
		return hash[:12]
	}
	return hash
}

// GetSessionLedgerPath returns the ledger path for a specific session, initializing it if needed.
func (lms *GitLedgerService) GetSessionLedgerPath(operatorSessionID string) (string, error) {
	if operatorSessionID == "" {
		return filepath.Join(lms.config.BaseDir, "files"), nil
	}

	sessionsRoot := filepath.Join(lms.config.BaseDir, "sessions")
	sessionPath := filepath.Join(sessionsRoot, operatorSessionID)

	lms.mu.Lock()
	defer lms.mu.Unlock()

	_, err := os.Stat(filepath.Join(sessionPath, ".git"))
	if err == nil {
		return sessionPath, nil
	}

	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create Operator session ledger directory: %w", err)
	}

	if err := lms.initGitRepo(sessionPath); err != nil {
		return "", fmt.Errorf("failed to initialize Operator session git repo: %w", err)
	}

	lms.logger.Info("Initialized new session ledger", "operator_session_id", operatorSessionID, "path", sessionPath)
	return sessionPath, nil
}

// initGitRepo initializes a git repository in the specified directory using native go-git
func (lms *GitLedgerService) initGitRepo(path string) error {
	gitDir := filepath.Join(path, ".git")

	if _, err := os.Stat(gitDir); err == nil {
		return nil
	}

	repo, err := git.PlainInit(path, false)
	if err != nil {
		return fmt.Errorf("git init failed: %w", err)
	}

	gitignore := filepath.Join(path, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("# g8e Ledger\n"), 0600); err != nil {
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	if _, err := w.Add(".gitignore"); err != nil {
		return fmt.Errorf("failed to git add .gitignore: %w", err)
	}

	_, err = w.Commit("Initial ledger commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "g8e-operator",
			Email: "g8e-operator@system",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create initial commit: %w", err)
	}

	return nil
}

// getLedgerPath returns the path where a file should be mirrored in the ledger.
func (lms *GitLedgerService) getLedgerPath(ledgerDir, filePath string) string {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	cleanPath := strings.TrimPrefix(absPath, "/")
	// Files are stored in 'files/' subdirectory within the ledger repository
	return filepath.Join(ledgerDir, "files", cleanPath)
}

// copyToLedger copies a file from the host to the ledger, encrypting it if the vault is unlocked.
// It uses streaming for unencrypted files to prevent OOM.
func (lms *GitLedgerService) copyToLedger(srcPath, dstPath string) error {
	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create mirror directory: %w", err)
	}

	if lms.IsEncryptionEnabled() {
		// For encrypted files, we currently read the whole content since the Vault API
		// only supports byte-slice encryption (AES-GCM).
		// We limit the size to prevent OOM.
		info, err := os.Stat(srcPath)
		if err != nil {
			return fmt.Errorf("failed to stat source file: %w", err)
		}

		const maxEncryptedSize = 100 * 1024 * 1024 // 100MB safety limit
		if info.Size() > maxEncryptedSize {
			return fmt.Errorf("file too large for encrypted ledger mirror: %d bytes (max %d)", info.Size(), maxEncryptedSize)
		}

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("failed to read source file: %w", err)
		}

		encrypted, err := lms.encryptionVault.Encrypt(content)
		if err != nil {
			return fmt.Errorf("failed to encrypt file content: %w", err)
		}

		if err := os.WriteFile(dstPath+".enc", encrypted, 0600); err != nil {
			return fmt.Errorf("failed to write encrypted destination file: %w", err)
		}
		return nil
	}

	// For unencrypted files, use streaming
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to stream copy to ledger: %w", err)
	}

	return nil
}

// LedgerFileWrite begins the two-phase commit for a file write. Call CompleteMirrorWrite after the write.
func (lms *GitLedgerService) LedgerFileWrite(operatorSessionID, filePath string) (*LedgerResult, error) {
	if !lms.gitReady() {
		return nil, nil
	}

	ledgerDir, err := lms.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session ledger path: %w", err)
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	result := &LedgerResult{
		FilePath:  filePath,
		Operation: FileMutationWrite,
	}

	ledgerPath := lms.getLedgerPath(ledgerDir, filePath)
	result.LedgerPath = ledgerPath

	if _, err := os.Stat(filePath); err == nil {
		if err := lms.copyToLedger(filePath, ledgerPath); err != nil {
			result.Error = fmt.Sprintf("failed to copy file to ledger: %v", err)
		}
	}

	hashBefore, err := lms.snapshotLedger(ledgerDir, fmt.Sprintf("Pre-mutation backup: %s", filePath))
	if err != nil {
		lms.logger.Warn("Failed to snapshot pre-mutation state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashBefore = hashBefore

	result.Success = true
	return result, nil
}

// CompleteMirrorWrite completes the mirror operation after the file write.
func (lms *GitLedgerService) CompleteMirrorWrite(result *LedgerResult, operatorSessionID string) error {
	if !lms.gitReady() || result == nil {
		return nil
	}

	ledgerDir, err := lms.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return fmt.Errorf("failed to get session ledger path: %w", err)
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	if err := lms.copyToLedger(result.FilePath, result.LedgerPath); err != nil {
		result.Error = fmt.Sprintf("failed to copy post-mutation file to ledger: %v", err)
		return fmt.Errorf("failed to copy post-mutation file to ledger: %w", err)
	}

	hashAfter, err := lms.snapshotLedger(ledgerDir, fmt.Sprintf("Post-mutation: %s via OperatorSession %s", result.FilePath, operatorSessionID))
	if err != nil {
		lms.logger.Warn("Failed to snapshot post-mutation state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashAfter = hashAfter

	result.DiffStat = lms.calculateDiffStat(ledgerDir, result.LedgerHashBefore, result.LedgerHashAfter)
	result.DiffContent = lms.calculateDiffContent(ledgerDir, result.LedgerHashBefore, result.LedgerHashAfter)

	lms.logger.Info("File mutation mirrored",
		string(constants.ToolDisplayCategoryFile), result.FilePath,
		"hash_before", truncateHash(result.LedgerHashBefore),
		"hash_after", truncateHash(result.LedgerHashAfter),
		"diff_stat", result.DiffStat,
		"diff_size", len(result.DiffContent))

	return nil
}

// MirrorFileDelete begins the two-phase commit for a file deletion. Call CompleteMirrorDelete after the deletion.
func (lms *GitLedgerService) MirrorFileDelete(operatorSessionID, filePath string) (*LedgerResult, error) {
	if !lms.gitReady() {
		return nil, nil
	}

	ledgerDir, err := lms.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session ledger path: %w", err)
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	result := &LedgerResult{
		FilePath:  filePath,
		Operation: FileMutationDelete,
	}

	ledgerPath := lms.getLedgerPath(ledgerDir, filePath)
	result.LedgerPath = ledgerPath

	if _, err := os.Stat(filePath); err == nil {
		if err := lms.copyToLedger(filePath, ledgerPath); err != nil {
			lms.logger.Warn("Failed to backup file before deletion", string(constants.ToolDisplayCategoryFile), filePath, string(constants.ConnectionStateError), err)
		}
	}

	hashBefore, err := lms.snapshotLedger(ledgerDir, fmt.Sprintf("Pre-deletion backup: %s", filePath))
	if err != nil {
		lms.logger.Warn("Failed to snapshot pre-deletion state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashBefore = hashBefore

	result.Success = true
	return result, nil
}

// CompleteMirrorDelete completes the mirror operation after file deletion.
func (lms *GitLedgerService) CompleteMirrorDelete(result *LedgerResult, operatorSessionID string) error {
	if !lms.gitReady() || result == nil {
		return nil
	}

	ledgerDir, err := lms.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return fmt.Errorf("failed to get session ledger path: %w", err)
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	if err := os.Remove(result.LedgerPath); err != nil && !os.IsNotExist(err) {
		lms.logger.Warn("Failed to remove mirror file", "path", result.LedgerPath, string(constants.ConnectionStateError), err)
	}

	hashAfter, err := lms.snapshotLedger(ledgerDir, fmt.Sprintf("Post-deletion: %s via OperatorSession %s", result.FilePath, operatorSessionID))
	if err != nil {
		lms.logger.Warn("Failed to snapshot post-deletion state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashAfter = hashAfter

	result.DiffStat = "file deleted"
	result.DiffContent = lms.calculateDiffContent(ledgerDir, result.LedgerHashBefore, result.LedgerHashAfter)

	lms.logger.Info("File deletion mirrored",
		string(constants.ToolDisplayCategoryFile), result.FilePath,
		"hash_before", truncateHash(result.LedgerHashBefore),
		"hash_after", truncateHash(result.LedgerHashAfter),
		"diff_size", len(result.DiffContent))

	return nil
}

// MirrorFileCreate begins the two-phase commit for a file creation. Call CompleteMirrorCreate after the creation.
func (lms *GitLedgerService) MirrorFileCreate(operatorSessionID, filePath string) (*LedgerResult, error) {
	if !lms.gitReady() {
		return nil, nil
	}

	ledgerDir, err := lms.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session ledger path: %w", err)
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	result := &LedgerResult{
		FilePath:  filePath,
		Operation: FileMutationCreate,
	}

	ledgerPath := lms.getLedgerPath(ledgerDir, filePath)
	result.LedgerPath = ledgerPath

	hashBefore, err := lms.snapshotLedger(ledgerDir, fmt.Sprintf("Pre-creation state for: %s", filePath))
	if err != nil {
		lms.logger.Warn("Failed to snapshot pre-creation state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashBefore = hashBefore

	result.Success = true
	return result, nil
}

// CompleteMirrorCreate completes the mirror operation after file creation.
func (lms *GitLedgerService) CompleteMirrorCreate(result *LedgerResult, operatorSessionID string) error {
	if !lms.gitReady() || result == nil {
		return nil
	}

	ledgerDir, err := lms.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return fmt.Errorf("failed to get session ledger path: %w", err)
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	if err := lms.copyToLedger(result.FilePath, result.LedgerPath); err != nil {
		result.Error = fmt.Sprintf("failed to copy created file to ledger: %v", err)
		return fmt.Errorf("failed to copy created file to ledger: %w", err)
	}

	hashAfter, err := lms.snapshotLedger(ledgerDir, fmt.Sprintf("Post-creation: %s via OperatorSession %s", result.FilePath, operatorSessionID))
	if err != nil {
		lms.logger.Warn("Failed to snapshot post-creation state", string(constants.ConnectionStateError), err)
	}
	result.LedgerHashAfter = hashAfter

	if info, err := os.Stat(result.FilePath); err == nil {
		lineCount := lms.countLines(result.FilePath)
		result.DiffStat = fmt.Sprintf("+%d lines, %d bytes (new file)", lineCount, info.Size())
	}

	result.DiffContent = lms.calculateDiffContent(ledgerDir, result.LedgerHashBefore, result.LedgerHashAfter)

	lms.logger.Info("File creation mirrored",
		string(constants.ToolDisplayCategoryFile), result.FilePath,
		"hash_after", truncateHash(result.LedgerHashAfter),
		"diff_stat", result.DiffStat,
		"diff_size", len(result.DiffContent))

	return nil
}

// GetStateMerkleRoot returns the current git commit hash as the state merkle root.
// This provides a BFT-verifiable snapshot of the ledger state at a point in time.
func (lms *GitLedgerService) GetStateMerkleRoot() (string, error) {
	if !lms.gitReady() {
		return "", nil
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	ledgerDir := filepath.Join(lms.config.BaseDir, "files")
	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		return "", fmt.Errorf("failed to open ledger git repo: %w", err)
	}
	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD ref: %w", err)
	}
	return ref.Hash().String(), nil
}

// snapshotLedger creates a git commit and returns the commit hash.
func (lms *GitLedgerService) snapshotLedger(ledgerDir, message string) (string, error) {
	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		return "", fmt.Errorf("failed to open git repo: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	err = w.AddWithOptions(&git.AddOptions{All: true})
	if err != nil && err != git.ErrEmptyCommit {
		return "", fmt.Errorf("git add failed: %w", err)
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
		return "", fmt.Errorf("git commit failed: %w", err)
	}

	return hash.String(), nil
}

// calculateDiffStat calculates the diff statistics between two commits.
func (lms *GitLedgerService) calculateDiffStat(ledgerDir, hashBefore, hashAfter string) string {
	if hashBefore == "" || hashAfter == "" {
		return ""
	}

	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		lms.logger.Warn("Failed to open git repo for diff stat", "error", err)
		return ""
	}

	commitBefore, err := repo.CommitObject(plumbing.NewHash(hashBefore))
	if err != nil {
		lms.logger.Warn("Failed to get commitBefore for diff stat", "error", err)
		return ""
	}

	commitAfter, err := repo.CommitObject(plumbing.NewHash(hashAfter))
	if err != nil {
		lms.logger.Warn("Failed to get commitAfter for diff stat", "error", err)
		return ""
	}

	patch, err := commitBefore.Patch(commitAfter)
	if err != nil {
		lms.logger.Warn("Failed to generate patch for diff stat", "error", err)
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
func (lms *GitLedgerService) calculateDiffContent(ledgerDir, hashBefore, hashAfter string) string {
	if hashBefore == "" || hashAfter == "" {
		return ""
	}

	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		lms.logger.Warn("Failed to open git repo for diff content", "error", err)
		return ""
	}

	commitBefore, err := repo.CommitObject(plumbing.NewHash(hashBefore))
	if err != nil {
		lms.logger.Warn("Failed to get commitBefore for diff content", "error", err)
		return ""
	}

	commitAfter, err := repo.CommitObject(plumbing.NewHash(hashAfter))
	if err != nil {
		lms.logger.Warn("Failed to get commitAfter for diff content", "error", err)
		return ""
	}

	patch, err := commitBefore.Patch(commitAfter)
	if err != nil {
		lms.logger.Warn("Failed to generate patch for diff content", "error", err)
		return ""
	}

	return patch.String()
}

// GetDiffContent returns the full diff content between two commits.
func (lms *GitLedgerService) GetDiffContent(hashBefore, hashAfter string, operatorSessionID string) string {
	if !lms.gitReady() {
		return ""
	}
	ledgerDir, err := lms.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		lms.logger.Warn("Failed to get session ledger path for diff content", string(constants.ConnectionStateError), err)
		return ""
	}
	return lms.calculateDiffContent(ledgerDir, hashBefore, hashAfter)
}

// GetDiffStat returns the diff statistics between two commits.
func (lms *GitLedgerService) GetDiffStat(hashBefore, hashAfter string, operatorSessionID string) string {
	if !lms.gitReady() {
		return ""
	}
	ledgerDir, err := lms.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		lms.logger.Warn("Failed to get session ledger path for diff stat", string(constants.ConnectionStateError), err)
		return ""
	}
	return lms.calculateDiffStat(ledgerDir, hashBefore, hashAfter)
}

// countLines counts the number of lines in a file.
func (lms *GitLedgerService) countLines(filePath string) int {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}
	return strings.Count(string(content), "\n") + 1
}

// GetFileHistory retrieves the git history for a specific file.
func (lms *GitLedgerService) GetFileHistory(filePath string, limit int, operatorSessionID string) ([]FileHistoryEntry, error) {
	if !lms.gitReady() {
		return nil, fmt.Errorf("ledger is disabled")
	}
	if lms.config == nil {
		return nil, fmt.Errorf("ledger is disabled")
	}

	if limit <= 0 {
		limit = 50
	}

	ledgerDir, err := lms.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session ledger path: %w", err)
	}

	ledgerPath := lms.getLedgerPath(ledgerDir, filePath)
	relPath, err := filepath.Rel(ledgerDir, ledgerPath)
	if err != nil {
		relPath = ledgerPath
	}

	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repo: %w", err)
	}

	cIter, err := repo.Log(&git.LogOptions{FileName: &relPath})
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}
	defer cIter.Close()

	var entries []FileHistoryEntry
	count := 0
	err = cIter.ForEach(func(c *object.Commit) error {
		if count >= limit {
			return storer.ErrStop
		}
		entries = append(entries, FileHistoryEntry{
			CommitHash: c.Hash.String(),
			Timestamp:  c.Author.When,
			Message:    strings.TrimSpace(c.Message),
			FilePath:   filePath,
		})
		count++
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return entries, nil
}

// FileHistoryEntry represents a single entry in a file's history.
type FileHistoryEntry struct {
	CommitHash string
	Timestamp  time.Time
	Message    string
	FilePath   string
}

// GetFileAtCommit retrieves the content of a file at a specific commit, decrypting if the vault is unlocked.
func (lms *GitLedgerService) GetFileAtCommit(filePath, commitHash, operatorSessionID string) (string, error) {
	if !lms.gitReady() {
		return "", fmt.Errorf("ledger is disabled")
	}

	ledgerDir, err := lms.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get session ledger path: %w", err)
	}

	ledgerPath := lms.getLedgerPath(ledgerDir, filePath)
	relPath, err := filepath.Rel(ledgerDir, ledgerPath)
	if err != nil {
		relPath = ledgerPath
	}

	if !lms.IsEncryptionEnabled() {
		return "", fmt.Errorf("vault is locked, cannot decrypt file from ledger")
	}

	encryptedRelPath := relPath + ".enc"
	content, err := lms.gitShowFile(ledgerDir, commitHash, encryptedRelPath)
	if err != nil {
		return "", fmt.Errorf("encrypted file not found in commit: %w", err)
	}

	decrypted, err := lms.encryptionVault.Decrypt([]byte(content))
	if err != nil {
		return "", fmt.Errorf("failed to decrypt file content: %w", err)
	}
	return string(decrypted), nil
}

// gitShowFile retrieves a file's content at a specific commit.
func (lms *GitLedgerService) gitShowFile(ledgerDir, commitHash, relPath string) (string, error) {
	repo, err := git.PlainOpen(ledgerDir)
	if err != nil {
		return "", fmt.Errorf("failed to open git repo: %w", err)
	}

	commit, err := repo.CommitObject(plumbing.NewHash(commitHash))
	if err != nil {
		return "", fmt.Errorf("failed to find commit %s: %w", commitHash, err)
	}

	file, err := commit.File(relPath)
	if err != nil {
		return "", fmt.Errorf("failed to find file %s in commit %s: %w", relPath, commitHash, err)
	}

	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("failed to read file contents: %w", err)
	}

	return content, nil
}

// RestoreFileFromCommit restores a file to its state at a specific commit.
func (lms *GitLedgerService) RestoreFileFromCommit(filePath, commitHash, operatorSessionID string) error {
	if !lms.gitReady() {
		return fmt.Errorf("ledger is disabled")
	}

	// Get session ledger path and file content before acquiring lock to avoid deadlock
	// GetFileAtCommit internally calls GetSessionLedgerPath which also acquires the mutex
	ledgerDir, err := lms.GetSessionLedgerPath(operatorSessionID)
	if err != nil {
		return fmt.Errorf("failed to get session ledger path: %w", err)
	}

	content, err := lms.GetFileAtCommit(filePath, commitHash, operatorSessionID)
	if err != nil {
		return fmt.Errorf("failed to get file at commit: %w", err)
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	ledgerPath := lms.getLedgerPath(ledgerDir, filePath)
	if _, err := os.Stat(filePath); err == nil {
		if err := lms.copyToLedger(filePath, ledgerPath); err != nil {
			lms.logger.Warn("Failed to backup current state before restoration", string(constants.ConnectionStateError), err)
		}
	}

	_, _ = lms.snapshotLedger(ledgerDir, fmt.Sprintf("Pre-restoration state: %s", filePath))

	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write restored file: %w", err)
	}

	if err := lms.copyToLedger(filePath, ledgerPath); err != nil {
		lms.logger.Warn("Failed to mirror restored file", string(constants.ConnectionStateError), err)
	}

	_, _ = lms.snapshotLedger(ledgerDir, fmt.Sprintf("Restored: %s to commit %s via OperatorSession %s", filePath, truncateHash(commitHash), operatorSessionID))

	lms.logger.Info("File restored from commit",
		string(constants.ToolDisplayCategoryFile), filePath,
		"commit", truncateHash(commitHash),
		string(constants.SessionKeyPrefixOperator), operatorSessionID)

	return nil
}
