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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	vault "github.com/g8e-ai/g8e/components/g8eo/services/vault"
)

// LedgerService maintains a git-backed version control of all files modified by the operator.
type LedgerService struct {
	auditVault      *AuditVaultService
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

// NewLedgerService creates a new LedgerService.
func NewLedgerService(auditVault *AuditVaultService, encryptionVault *vault.Vault, logger *slog.Logger) *LedgerService {
	return &LedgerService{
		auditVault:      auditVault,
		encryptionVault: encryptionVault,
		logger:          logger,
	}
}

// IsEncryptionEnabled returns whether file encryption is enabled.
func (lms *LedgerService) IsEncryptionEnabled() bool {
	return lms.encryptionVault != nil && lms.encryptionVault.IsUnlocked()
}

// gitReady returns true if the ledger can perform git operations.
func (lms *LedgerService) gitReady() bool {
	return lms.auditVault != nil && lms.auditVault.IsEnabled() && lms.auditVault.IsGitAvailable()
}

// truncateHash safely truncates a git hash for logging.
func truncateHash(hash string) string {
	if len(hash) >= 12 {
		return hash[:12]
	}
	return hash
}

// LedgerFileWrite begins the two-phase commit for a file write. Call CompleteMirrorWrite after the write.
func (lms *LedgerService) LedgerFileWrite(operatorSessionID, filePath string) (*LedgerResult, error) {
	if !lms.gitReady() {
		return nil, nil
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	result := &LedgerResult{
		FilePath:  filePath,
		Operation: FileMutationWrite,
	}

	ledgerPath := lms.getLedgerPath(filePath)
	result.LedgerPath = ledgerPath

	if _, err := os.Stat(filePath); err == nil {
		if err := lms.copyToLedger(filePath, ledgerPath); err != nil {
			result.Error = fmt.Sprintf("failed to copy file to ledger: %v", err)
		}
	}

	hashBefore, err := lms.snapshotLedger(fmt.Sprintf("Pre-mutation backup: %s", filePath))
	if err != nil {
		lms.logger.Warn("Failed to snapshot pre-mutation state", "error", err)
	}
	result.LedgerHashBefore = hashBefore

	result.Success = true
	return result, nil
}

// CompleteMirrorWrite completes the mirror operation after the file write.
func (lms *LedgerService) CompleteMirrorWrite(result *LedgerResult, operatorSessionID string) error {
	if !lms.gitReady() || result == nil {
		return nil
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	if err := lms.copyToLedger(result.FilePath, result.LedgerPath); err != nil {
		result.Error = fmt.Sprintf("failed to copy post-mutation file to ledger: %v", err)
		return fmt.Errorf("failed to copy post-mutation file to ledger: %w", err)
	}

	hashAfter, err := lms.snapshotLedger(fmt.Sprintf("Post-mutation: %s via OperatorSession %s", result.FilePath, operatorSessionID))
	if err != nil {
		lms.logger.Warn("Failed to snapshot post-mutation state", "error", err)
	}
	result.LedgerHashAfter = hashAfter

	result.DiffStat = lms.calculateDiffStat(result.LedgerHashBefore, result.LedgerHashAfter)
	result.DiffContent = lms.calculateDiffContent(result.LedgerHashBefore, result.LedgerHashAfter)

	lms.logger.Info("File mutation mirrored",
		"file", result.FilePath,
		"hash_before", truncateHash(result.LedgerHashBefore),
		"hash_after", truncateHash(result.LedgerHashAfter),
		"diff_stat", result.DiffStat,
		"diff_size", len(result.DiffContent))

	return nil
}

// MirrorFileDelete begins the two-phase commit for a file deletion. Call CompleteMirrorDelete after the deletion.
func (lms *LedgerService) MirrorFileDelete(operatorSessionID, filePath string) (*LedgerResult, error) {
	if !lms.gitReady() {
		return nil, nil
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	result := &LedgerResult{
		FilePath:  filePath,
		Operation: FileMutationDelete,
	}

	ledgerPath := lms.getLedgerPath(filePath)
	result.LedgerPath = ledgerPath

	if _, err := os.Stat(filePath); err == nil {
		if err := lms.copyToLedger(filePath, ledgerPath); err != nil {
			lms.logger.Warn("Failed to backup file before deletion", "file", filePath, "error", err)
		}
	}

	hashBefore, err := lms.snapshotLedger(fmt.Sprintf("Pre-deletion backup: %s", filePath))
	if err != nil {
		lms.logger.Warn("Failed to snapshot pre-deletion state", "error", err)
	}
	result.LedgerHashBefore = hashBefore

	result.Success = true
	return result, nil
}

// CompleteMirrorDelete completes the mirror operation after file deletion.
func (lms *LedgerService) CompleteMirrorDelete(result *LedgerResult, operatorSessionID string) error {
	if !lms.gitReady() || result == nil {
		return nil
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	if err := os.Remove(result.LedgerPath); err != nil && !os.IsNotExist(err) {
		lms.logger.Warn("Failed to remove mirror file", "path", result.LedgerPath, "error", err)
	}

	hashAfter, err := lms.snapshotLedger(fmt.Sprintf("Post-deletion: %s via OperatorSession %s", result.FilePath, operatorSessionID))
	if err != nil {
		lms.logger.Warn("Failed to snapshot post-deletion state", "error", err)
	}
	result.LedgerHashAfter = hashAfter

	result.DiffStat = "file deleted"
	result.DiffContent = lms.calculateDiffContent(result.LedgerHashBefore, result.LedgerHashAfter)

	lms.logger.Info("File deletion mirrored",
		"file", result.FilePath,
		"hash_before", truncateHash(result.LedgerHashBefore),
		"hash_after", truncateHash(result.LedgerHashAfter),
		"diff_size", len(result.DiffContent))

	return nil
}

// MirrorFileCreate begins the two-phase commit for a file creation. Call CompleteMirrorCreate after the creation.
func (lms *LedgerService) MirrorFileCreate(operatorSessionID, filePath string) (*LedgerResult, error) {
	if !lms.gitReady() {
		return nil, nil
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	result := &LedgerResult{
		FilePath:  filePath,
		Operation: FileMutationCreate,
	}

	ledgerPath := lms.getLedgerPath(filePath)
	result.LedgerPath = ledgerPath

	hashBefore, err := lms.snapshotLedger(fmt.Sprintf("Pre-creation state for: %s", filePath))
	if err != nil {
		lms.logger.Warn("Failed to snapshot pre-creation state", "error", err)
	}
	result.LedgerHashBefore = hashBefore

	result.Success = true
	return result, nil
}

// CompleteMirrorCreate completes the mirror operation after file creation.
func (lms *LedgerService) CompleteMirrorCreate(result *LedgerResult, operatorSessionID string) error {
	if !lms.gitReady() || result == nil {
		return nil
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	if err := lms.copyToLedger(result.FilePath, result.LedgerPath); err != nil {
		result.Error = fmt.Sprintf("failed to copy created file to ledger: %v", err)
		return fmt.Errorf("failed to copy created file to ledger: %w", err)
	}

	hashAfter, err := lms.snapshotLedger(fmt.Sprintf("Post-creation: %s via OperatorSession %s", result.FilePath, operatorSessionID))
	if err != nil {
		lms.logger.Warn("Failed to snapshot post-creation state", "error", err)
	}
	result.LedgerHashAfter = hashAfter

	if info, err := os.Stat(result.FilePath); err == nil {
		lineCount := lms.countLines(result.FilePath)
		result.DiffStat = fmt.Sprintf("+%d lines, %d bytes (new file)", lineCount, info.Size())
	}

	result.DiffContent = lms.calculateDiffContent(result.LedgerHashBefore, result.LedgerHashAfter)

	lms.logger.Info("File creation mirrored",
		"file", result.FilePath,
		"hash_after", truncateHash(result.LedgerHashAfter),
		"diff_stat", result.DiffStat,
		"diff_size", len(result.DiffContent))

	return nil
}

// getLedgerPath returns the path where a file should be mirrored in the ledger.
func (lms *LedgerService) getLedgerPath(filePath string) string {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	cleanPath := strings.TrimPrefix(absPath, "/")
	return filepath.Join(lms.auditVault.filesPath, cleanPath)
}

// copyToLedger copies a file from the host to the ledger, encrypting it if the vault is unlocked.
// It uses streaming for unencrypted files to prevent OOM.
func (lms *LedgerService) copyToLedger(srcPath, dstPath string) error {
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

// gitExec runs a git command in the ledger directory.
func (lms *LedgerService) gitExec(args ...string) error {
	gitPath := lms.auditVault.GetGitPath()
	if gitPath == "" {
		return fmt.Errorf("git not available")
	}
	cmd := exec.Command(gitPath, args...)
	cmd.Dir = lms.auditVault.GetLedgerGitDir()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %v (stderr: %s)", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// gitOutput runs a git command in the ledger directory and returns stdout.
func (lms *LedgerService) gitOutput(args ...string) (string, error) {
	gitPath := lms.auditVault.GetGitPath()
	if gitPath == "" {
		return "", fmt.Errorf("git not available")
	}
	cmd := exec.Command(gitPath, args...)
	cmd.Dir = lms.auditVault.GetLedgerGitDir()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v (stderr: %s)", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// snapshotLedger creates a git commit and returns the commit hash.
func (lms *LedgerService) snapshotLedger(message string) (string, error) {
	if err := lms.gitExec("add", "-A"); err != nil {
		return "", fmt.Errorf("git add failed: %w", err)
	}

	commitMsg := fmt.Sprintf("[%s] %s", time.Now().UTC().Format(time.RFC3339), message)
	if err := lms.gitExec("commit", "-m", commitMsg, "--allow-empty"); err != nil {
		return "", fmt.Errorf("git commit failed: %w", err)
	}

	hash, err := lms.gitOutput("rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	return hash, nil
}

// calculateDiffStat calculates the diff statistics between two commits.
func (lms *LedgerService) calculateDiffStat(hashBefore, hashAfter string) string {
	if hashBefore == "" || hashAfter == "" {
		return ""
	}

	output, err := lms.gitOutput("diff", "--stat", hashBefore, hashAfter)
	if err != nil || output == "" {
		return ""
	}

	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

// calculateDiffContent computes the full diff content between two commits.
func (lms *LedgerService) calculateDiffContent(hashBefore, hashAfter string) string {
	if hashBefore == "" || hashAfter == "" {
		return ""
	}

	output, err := lms.gitOutput("diff", hashBefore, hashAfter)
	if err != nil {
		lms.logger.Warn("Failed to calculate diff content", "error", err)
		return ""
	}

	return output
}

// GetDiffContent returns the full diff content between two commits.
func (lms *LedgerService) GetDiffContent(hashBefore, hashAfter string) string {
	return lms.calculateDiffContent(hashBefore, hashAfter)
}

// GetDiffStat returns the diff statistics between two commits.
func (lms *LedgerService) GetDiffStat(hashBefore, hashAfter string) string {
	return lms.calculateDiffStat(hashBefore, hashAfter)
}

// countLines counts the number of lines in a file.
func (lms *LedgerService) countLines(filePath string) int {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}
	return strings.Count(string(content), "\n") + 1
}

// GetFileHistory retrieves the git history for a specific file.
func (lms *LedgerService) GetFileHistory(filePath string, limit int) ([]FileHistoryEntry, error) {
	if !lms.gitReady() {
		return nil, fmt.Errorf("ledger is disabled")
	}

	if limit <= 0 {
		limit = 50
	}

	ledgerPath := lms.getLedgerPath(filePath)
	relPath, err := filepath.Rel(lms.auditVault.GetLedgerPath(), ledgerPath)
	if err != nil {
		relPath = ledgerPath
	}

	output, err := lms.gitOutput("log",
		fmt.Sprintf("-n%d", limit),
		"--format=%H\t%aI\t%s",
		"--", relPath)
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	if output == "" {
		return nil, nil
	}

	var entries []FileHistoryEntry
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}

		ts, err := time.Parse(time.RFC3339, parts[1])
		if err != nil {
			lms.logger.Warn("Failed to parse commit timestamp", "raw", parts[1], "error", err)
			ts = time.Time{}
		}

		entries = append(entries, FileHistoryEntry{
			CommitHash: parts[0],
			Timestamp:  ts,
			Message:    strings.TrimSpace(parts[2]),
			FilePath:   filePath,
		})
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
func (lms *LedgerService) GetFileAtCommit(filePath, commitHash string) (string, error) {
	if !lms.gitReady() {
		return "", fmt.Errorf("ledger is disabled")
	}

	ledgerPath := lms.getLedgerPath(filePath)
	relPath, err := filepath.Rel(lms.auditVault.GetLedgerPath(), ledgerPath)
	if err != nil {
		relPath = ledgerPath
	}

	encryptedRelPath := relPath + ".enc"
	content, err := lms.gitShowFile(commitHash, encryptedRelPath)
	if err == nil {
		if !lms.IsEncryptionEnabled() {
			return "", fmt.Errorf("encrypted file found but vault is locked")
		}

		decrypted, err := lms.encryptionVault.Decrypt([]byte(content))
		if err != nil {
			return "", fmt.Errorf("failed to decrypt file content: %w", err)
		}
		return string(decrypted), nil
	}

	content, err = lms.gitShowFile(commitHash, relPath)
	if err != nil {
		return "", fmt.Errorf("file not found in commit: %w", err)
	}

	return content, nil
}

// gitShowFile retrieves a file's content at a specific commit.
func (lms *LedgerService) gitShowFile(commitHash, relPath string) (string, error) {
	gitPath := lms.auditVault.GetGitPath()
	if gitPath == "" {
		return "", fmt.Errorf("git not available")
	}
	ref := fmt.Sprintf("%s:%s", commitHash, relPath)
	cmd := exec.Command(gitPath, "show", ref)
	cmd.Dir = lms.auditVault.GetLedgerGitDir()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git show %s: %w", ref, err)
	}
	return stdout.String(), nil
}

// RestoreFileFromCommit restores a file to its state at a specific commit.
func (lms *LedgerService) RestoreFileFromCommit(filePath, commitHash, operatorSessionID string) error {
	if !lms.gitReady() {
		return fmt.Errorf("ledger is disabled")
	}

	lms.mu.Lock()
	defer lms.mu.Unlock()

	content, err := lms.GetFileAtCommit(filePath, commitHash)
	if err != nil {
		return fmt.Errorf("failed to get file at commit: %w", err)
	}

	ledgerPath := lms.getLedgerPath(filePath)
	if _, err := os.Stat(filePath); err == nil {
		if err := lms.copyToLedger(filePath, ledgerPath); err != nil {
			lms.logger.Warn("Failed to backup current state before restoration", "error", err)
		}
	}

	_, _ = lms.snapshotLedger(fmt.Sprintf("Pre-restoration state: %s", filePath))

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write restored file: %w", err)
	}

	if err := lms.copyToLedger(filePath, ledgerPath); err != nil {
		lms.logger.Warn("Failed to mirror restored file", "error", err)
	}

	_, _ = lms.snapshotLedger(fmt.Sprintf("Restored: %s to commit %s via OperatorSession %s", filePath, truncateHash(commitHash), operatorSessionID))

	lms.logger.Info("File restored from commit",
		"file", filePath,
		"commit", truncateHash(commitHash),
		"operator_session", operatorSessionID)

	return nil
}
