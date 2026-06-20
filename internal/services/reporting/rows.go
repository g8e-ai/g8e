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

package reporting

import "fmt"

// Row is the contract for a typed CSV row.
type Row interface {
	Columns() []string
	Record() []string
}

// ReceiptRow maps to receipts.csv.
type ReceiptRow struct {
	TransactionID   string
	TransactionHash string
	OperatorID      string
	SessionID       string
	ActionType      string
	TargetResource  string
	Status          string
	StateRootBefore string
	StateRootAfter  string
	SignerKeyID     string
	Signature       string
	ExecutedAtUTC   string
}

func (ReceiptRow) Columns() []string {
	return []string{
		"transaction_id", "transaction_hash", "operator_id", "session_id", "action_type",
		"target_resource", "status", "state_root_before", "state_root_after",
		"signer_key_id", "signature", "executed_at_utc",
	}
}

func (r ReceiptRow) Record() []string {
	return []string{
		r.TransactionID, r.TransactionHash, r.OperatorID, r.SessionID, r.ActionType,
		r.TargetResource, r.Status, r.StateRootBefore, r.StateRootAfter,
		r.SignerKeyID, r.Signature, r.ExecutedAtUTC,
	}
}

// SessionRow maps to sessions.csv.
type SessionRow struct {
	ID           string
	SessionType  string
	Title        string
	UserIdentity string
	CreatedAtUTC string
}

func (SessionRow) Columns() []string {
	return []string{"id", "session_type", "title", "user_identity", "created_at_utc"}
}

func (r SessionRow) Record() []string {
	return []string{r.ID, r.SessionType, r.Title, r.UserIdentity, r.CreatedAtUTC}
}

// EventRow maps to events.csv.
type EventRow struct {
	ID              string
	SessionID       string
	TimestampUTC    string
	Type            string
	CommandRaw      string
	ExitCode        string
	DurationMs      string
	ContentSHA256   string
	ContentSize     string
	ContentText     string
	StdoutTruncated string
	StderrTruncated string
}

func (EventRow) Columns() []string {
	return []string{
		"id", "session_id", "timestamp_utc", "type", "command_raw",
		"exit_code", "duration_ms", "content_sha256", "content_size", "content_text",
		"stdout_truncated", "stderr_truncated",
	}
}

func (r EventRow) Record() []string {
	return []string{
		r.ID, r.SessionID, r.TimestampUTC, r.Type, r.CommandRaw,
		r.ExitCode, r.DurationMs, r.ContentSHA256, r.ContentSize, r.ContentText,
		r.StdoutTruncated, r.StderrTruncated,
	}
}

// FileMutationRow maps to file_mutations.csv.
type FileMutationRow struct {
	ID               string
	EventID          string
	Filepath         string
	Operation        string
	LedgerHashBefore string
	LedgerHashAfter  string
	DiffStat         string
}

func (FileMutationRow) Columns() []string {
	return []string{
		"id", "event_id", "filepath", "operation",
		"ledger_hash_before", "ledger_hash_after", "diff_stat",
	}
}

func (r FileMutationRow) Record() []string {
	return []string{
		r.ID, r.EventID, r.Filepath, r.Operation,
		r.LedgerHashBefore, r.LedgerHashAfter, r.DiffStat,
	}
}

// ExecutionRow maps to executions.csv.
type ExecutionRow struct {
	ID           string
	TimestampUTC string
	Command      string
	ExitCode     string
	DurationMs   string
	StdoutHash   string
	StdoutSize   string
	StderrHash   string
	StderrSize   string
	CaseID       string
	TaskID       string
	OperatorID   string
}

func (ExecutionRow) Columns() []string {
	return []string{
		"id", "timestamp_utc", "command", "exit_code", "duration_ms",
		"stdout_hash", "stdout_size", "stderr_hash", "stderr_size",
		"case_id", "task_id", "operator_id",
	}
}

func (r ExecutionRow) Record() []string {
	return []string{
		r.ID, r.TimestampUTC, r.Command, r.ExitCode, r.DurationMs,
		r.StdoutHash, r.StdoutSize, r.StderrHash, r.StderrSize,
		r.CaseID, r.TaskID, r.OperatorID,
	}
}

// FileDiffRow maps to file_diffs.csv.
type FileDiffRow struct {
	ID               string
	TimestampUTC     string
	FilePath         string
	Operation        string
	LedgerHashBefore string
	LedgerHashAfter  string
	DiffStat         string
	DiffHash         string
	DiffSize         string
	SessionID        string
	CaseID           string
	OperatorID       string
}

func (FileDiffRow) Columns() []string {
	return []string{
		"id", "timestamp_utc", "file_path", "operation",
		"ledger_hash_before", "ledger_hash_after", "diff_stat",
		"diff_hash", "diff_size", "session_id", "case_id", "operator_id",
	}
}

func (r FileDiffRow) Record() []string {
	return []string{
		r.ID, r.TimestampUTC, r.FilePath, r.Operation,
		r.LedgerHashBefore, r.LedgerHashAfter, r.DiffStat,
		r.DiffHash, r.DiffSize, r.SessionID, r.CaseID, r.OperatorID,
	}
}

// CommitmentRow maps to commitments.csv.
type CommitmentRow struct {
	Seq                 string
	CommittedAtUTC      string
	TransactionID       string
	TransactionHash     string
	PriorCommitmentHash string
	Hash                string
	StateRootAtCommit   string
	ActionType          string
	TargetResource      string
	AuditorKeyID        string
	Signature           string
}

func (CommitmentRow) Columns() []string {
	return []string{
		"seq", "committed_at_utc", "transaction_id", "transaction_hash",
		"prior_commitment_hash", "hash", "state_root_at_commit",
		"action_type", "target_resource", "auditor_key_id", "signature",
	}
}

func (r CommitmentRow) Record() []string {
	return []string{
		r.Seq, r.CommittedAtUTC, r.TransactionID, r.TransactionHash,
		r.PriorCommitmentHash, r.Hash, r.StateRootAtCommit,
		r.ActionType, r.TargetResource, r.AuditorKeyID, r.Signature,
	}
}

// LedgerCommitRow maps to ledger_commits.csv.
type LedgerCommitRow struct {
	CommitHash   string
	ParentHash   string
	TimestampUTC string
	Message      string
	FilesChanged string
	DiffStat     string
}

func (LedgerCommitRow) Columns() []string {
	return []string{"commit_hash", "parent_hash", "timestamp_utc", "message", "files_changed", "diff_stat"}
}

func (r LedgerCommitRow) Record() []string {
	return []string{r.CommitHash, r.ParentHash, r.TimestampUTC, r.Message, r.FilesChanged, r.DiffStat}
}

// LedgerMerkleRootRow maps to ledger_merkle_root.csv.
type LedgerMerkleRootRow struct {
	MerkleRoot    string
	CapturedAtUTC string
}

func (LedgerMerkleRootRow) Columns() []string {
	return []string{"merkle_root", "captured_at_utc"}
}

func (r LedgerMerkleRootRow) Record() []string {
	return []string{r.MerkleRoot, r.CapturedAtUTC}
}

// ReplayNonceRow maps to replay_nonces.csv.
type ReplayNonceRow struct {
	Nonce         string
	Status        string
	ReservedAtUTC string
	UsedAtUTC     string
	ExpiresAtUTC  string
}

func (ReplayNonceRow) Columns() []string {
	return []string{"nonce", "status", "reserved_at_utc", "used_at_utc", "expires_at_utc"}
}

func (r ReplayNonceRow) Record() []string {
	return []string{r.Nonce, r.Status, r.ReservedAtUTC, r.UsedAtUTC, r.ExpiresAtUTC}
}

// SuspendedTxRow maps to suspended_transactions.csv.
type SuspendedTxRow struct {
	TxHash            string
	UserID            string
	ActionType        string
	TargetResource    string
	Status            string
	CreatedAtUTC      string
	ExpiresAtUTC      string
	ApprovedBy        string
	ApprovalSignature string
}

func (SuspendedTxRow) Columns() []string {
	return []string{
		"tx_hash", "user_id", "action_type", "target_resource", "status",
		"created_at_utc", "expires_at_utc", "approved_by", "approval_signature",
	}
}

func (r SuspendedTxRow) Record() []string {
	return []string{
		r.TxHash, r.UserID, r.ActionType, r.TargetResource, r.Status,
		r.CreatedAtUTC, r.ExpiresAtUTC, r.ApprovedBy, r.ApprovalSignature,
	}
}

// VerificationRow maps to verification_summary.csv.
type VerificationRow struct {
	Check   string
	Scope   string
	Subject string
	Result  string
	Detail  string
}

func (VerificationRow) Columns() []string {
	return []string{"check", "scope", "subject", "result", "detail"}
}

func (r VerificationRow) Record() []string {
	return []string{r.Check, r.Scope, r.Subject, r.Result, r.Detail}
}

// ManifestRow maps to manifest.csv.
type ManifestRow struct {
	File           string
	RecordType     string
	RowCount       string
	SHA256         string
	GeneratedAtUTC string
	VaultUnlocked  string
}

func (ManifestRow) Columns() []string {
	return []string{"file", "record_type", "row_count", "sha256", "generated_at_utc", "vault_unlocked"}
}

func (r ManifestRow) Record() []string {
	return []string{r.File, r.RecordType, r.RowCount, r.SHA256, r.GeneratedAtUTC, r.VaultUnlocked}
}

// intStr formats an int as a string.
func intStr(n int) string { return fmt.Sprintf("%d", n) }

// int64Str formats an int64 as a string.
func int64Str(n int64) string { return fmt.Sprintf("%d", n) }
