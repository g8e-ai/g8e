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

//go:build integration

package compliance

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/pathutil"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/storage/storagetest"
	"github.com/g8e-ai/g8e/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

const (
	integrationSessionID = "ksi-integration-session"
	integrationTxID      = "ksi-integration-tx-1"
	integrationCommit1   = "9f2c1ab4e6d84f01a2b3c4d5e6f708192a3b4c5d"
	integrationCommit2   = "1a2b3c4d5e6f708192a3b4c5d9f2c1ab4e6d84f0"
)

// integrationEvaluatorFixture holds real storage backends wired exactly like
// the production openEvaluatorDeps path: SQLAuditStore for audit evidence,
// GitLedgerService for ledger evidence, and CommitmentLedger sharing g8e.db.
type integrationEvaluatorFixture struct {
	deps        EvaluatorDeps
	auditStore  *storage.SQLAuditStore
	ledger      *storage.GitLedgerService
	commitments *storage.CommitmentLedger
	fileSvc     fs.RuntimeFileService
	tempDir     string
}

// newIntegrationEvaluatorFixture creates the real storage backends against a
// temp-rooted file service. No mocks: on-disk SQLite, real git ledger.
func newIntegrationEvaluatorFixture(t *testing.T) *integrationEvaluatorFixture {
	t.Helper()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available - skipping git-dependent integration test")
	}

	tempDir := testutil.TempDir(t)
	fileSvc := storagetest.NewTestFileSvc(t, tempDir)

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	testVault := storagetest.CreateTestVault(t, vaultDir, privKey)

	logger := testutil.NewTestLogger()

	auditCfg := storage.DefaultAuditStoreConfig()
	auditCfg.EncryptionVault = testVault
	auditStore, err := storage.NewSQLAuditStore(auditCfg, logger, fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { auditStore.Close() })

	dbPath := pathutil.ResolveDBPath(fileSvc.Resolve(constants.DataDirname), constants.DbFilename)
	mainDB, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), logger)
	require.NoError(t, err)
	t.Cleanup(func() { mainDB.Close() })
	commitments := storage.NewCommitmentLedger(mainDB, logger)

	ledger, err := storage.NewGitLedgerService(&storage.LedgerConfig{
		GitPath:         gitPath,
		EncryptionVault: testVault,
	}, logger, fileSvc)
	require.NoError(t, err)

	return &integrationEvaluatorFixture{
		deps: EvaluatorDeps{
			Audit:       auditStore,
			Ledger:      ledger,
			Commitments: commitments,
		},
		auditStore:  auditStore,
		ledger:      ledger,
		commitments: commitments,
		fileSvc:     fileSvc,
		tempDir:     tempDir,
	}
}

// seedEvidence records real events, a signed receipt, a file mutation, git
// ledger commits, and an intact two-entry commitment chain.
func (f *integrationEvaluatorFixture) seedEvidence(t *testing.T) {
	t.Helper()

	require.NoError(t, f.auditStore.CreateSession(integrationSessionID, string(constants.UserRoleOperator), "KSI integration", "test-user"))

	now := time.Now().UTC()
	eventID, err := f.auditStore.RecordEvent(&storage.Event{
		OperatorSessionID: integrationSessionID,
		Timestamp:         now,
		Type:              constants.EventAppCaseCreated,
		ContentText:       "integration test event",
	})
	require.NoError(t, err)
	require.Positive(t, eventID)

	require.NoError(t, f.auditStore.RecordActionReceipt(&models.ActionReceiptRecord{
		TransactionID:     integrationTxID,
		TransactionHash:   "tx-hash-1",
		OperatorID:        "operator-1",
		OperatorSessionID: integrationSessionID,
		ActionType:        constants.ActionTypeExecuteBash,
		TargetResource:    "localhost",
		Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ExecutedAt:        now,
		SignerKeyID:       "key-1",
		Signature:         "signature-1",
		Timestamp:         now,
	}))

	require.NoError(t, f.auditStore.RecordFileMutation(&storage.FileMutationLog{
		EventID:   eventID,
		Filepath:  "integration-target.txt",
		Operation: storage.FileMutationCreate,
	}))

	targetPath := filepath.Join(f.tempDir, "integration-target.txt")
	require.NoError(t, os.WriteFile(targetPath, []byte("integration evidence"), constants.PermFilePrivate))
	mirror, err := f.ledger.MirrorFileCreate("", targetPath)
	require.NoError(t, err)
	require.NotNil(t, mirror)
	require.NoError(t, f.ledger.CompleteMirrorCreate(mirror, ""))

	appendCommitment := func(txID string, committedAtMs int64, priorHash, hash string) {
		attestation := []byte(fmt.Sprintf(
			`{"transaction_id":%q,"transaction_hash":"tx-hash","committed_at_unix_ms":%d,"action_type":%q,"auditor_key_id":"auditor-1","signature":"sig"}`,
			txID, committedAtMs, string(constants.ActionTypeExecuteBash)))
		require.NoError(t, f.commitments.AppendCommitmentJSON(attestation, priorHash, hash))
	}
	appendCommitment("tx-1", now.Add(-time.Second).UnixMilli(), "", integrationCommit1)
	appendCommitment("tx-2", now.UnixMilli(), integrationCommit1, integrationCommit2)
}

// loadRealKSICatalog loads the shipped docs-as-code KSI catalog.
func loadRealKSICatalog(t *testing.T) *KSICatalog {
	t.Helper()
	catalog, err := LoadKSICatalog(filepath.Join("..", "..", "..", "docs", "reference", "ksi-catalog.json"))
	require.NoError(t, err)
	return catalog
}

// automatableKSIIDs mirrors the KSI IDs bound in DefaultMethods.
var automatableKSIIDs = []string{
	"KSI-CMT-01", "KSI-CMT-03", "KSI-CNA-01",
	"KSI-IAM-05", "KSI-IAM-07",
	"KSI-MLA-03", "KSI-MLA-07", "KSI-MLA-08",
	"KSI-SVC-04", "KSI-SVC-05",
}

// TestKSIEvaluator_Integration_SeededEvidenceSatisfiesAutomatableKSIs runs the
// evaluator with default methods against real seeded storage and verifies that
// automatable KSIs are satisfied with evidence anchors that reference real
// receipt IDs, commit hashes, and commitment hashes from the test stores.
func TestKSIEvaluator_Integration_SeededEvidenceSatisfiesAutomatableKSIs(t *testing.T) {
	fixture := newIntegrationEvaluatorFixture(t)
	fixture.seedEvidence(t)

	catalog := loadRealKSICatalog(t)
	evaluator := NewKSIEvaluator(catalog)
	evaluator.RegisterDefaultMethods(fixture.deps)

	resultSet, err := evaluator.Evaluate(context.Background(), ClassC)
	require.NoError(t, err)
	require.NoError(t, resultSet.Validate(catalog))
	require.Len(t, resultSet.Results, len(catalog.KSIsForClass(ClassC)))

	merkleRoot, err := fixture.ledger.GetStateMerkleRoot()
	require.NoError(t, err)
	require.NotEmpty(t, merkleRoot)

	commits, err := fixture.ledger.ListCommits("", 10)
	require.NoError(t, err)
	require.NotEmpty(t, commits)
	commitHashes := make(map[string]bool, len(commits))
	for _, c := range commits {
		commitHashes[c.CommitHash] = true
	}

	resultsByID := make(map[string]KSIResult, len(resultSet.Results))
	for _, res := range resultSet.Results {
		resultsByID[res.ID] = res
	}

	for _, ksiID := range automatableKSIIDs {
		res, ok := resultsByID[ksiID]
		require.True(t, ok, "result set missing %s", ksiID)
		assert.Equal(t, KSIStatusSatisfied, res.Status, "%s should be satisfied against seeded evidence", ksiID)
		assert.GreaterOrEqual(t, res.MethodCount, MinimumMethodsForClass(ClassC))
		assert.NotEmpty(t, res.Evidence, "%s should carry evidence anchors", ksiID)
	}

	// Evidence anchors resolve to real receipt IDs from the audit store.
	assert.Equal(t, integrationTxID, firstEvidenceRef(t, resultsByID["KSI-CMT-03"], EvidenceTypeReceiptID))
	assert.Equal(t, integrationTxID, firstEvidenceRef(t, resultsByID["KSI-MLA-08"], EvidenceTypeReceiptID))

	// Evidence anchors resolve to real commitment hashes from the ledger.
	assert.Equal(t, integrationCommit1, firstEvidenceRef(t, resultsByID["KSI-MLA-07"], EvidenceTypeLedgerCommit))
	assert.Equal(t, integrationCommit2, firstEvidenceRef(t, resultsByID["KSI-SVC-05"], EvidenceTypeLedgerCommit))

	// Merkle root evidence matches the real git ledger HEAD.
	assert.Equal(t, merkleRoot, firstEvidenceRef(t, resultsByID["KSI-MLA-07"], EvidenceTypeMerkleRoot))

	// Ledger commit evidence references a commit that exists in the real repo.
	commitRef := firstEvidenceRef(t, resultsByID["KSI-CMT-01"], EvidenceTypeLedgerCommit)
	assert.True(t, commitHashes[commitRef], "evidence commit %s not found in real ledger commits", commitRef)

	// Non-automatable KSIs fail-closed even with seeded evidence.
	assert.Equal(t, KSIStatusNotSatisfied, resultsByID["KSI-CED-01"].Status)
	assert.Positive(t, resultSet.SatisfiedCount())
	assert.Positive(t, resultSet.NotSatisfiedCount())
}

// TestKSIEvaluator_Integration_EmptyStoresFailClosed verifies that the
// evaluator marks every KSI not_satisfied when the real stores hold no
// evidence: no events, no receipts, no mutations, no ledger commits, no
// commitments.
func TestKSIEvaluator_Integration_EmptyStoresFailClosed(t *testing.T) {
	fixture := newIntegrationEvaluatorFixture(t)

	catalog := loadRealKSICatalog(t)
	evaluator := NewKSIEvaluator(catalog)
	evaluator.RegisterDefaultMethods(fixture.deps)

	resultSet, err := evaluator.Evaluate(context.Background(), ClassC)
	require.NoError(t, err)
	require.NoError(t, resultSet.Validate(catalog))
	require.NotEmpty(t, resultSet.Results)

	for _, res := range resultSet.Results {
		assert.Equal(t, KSIStatusNotSatisfied, res.Status, "%s must fail-closed on empty stores", res.ID)
	}
	assert.Zero(t, resultSet.SatisfiedCount())
	assert.True(t, resultSet.HasFailures())
}

// firstEvidenceRef returns the reference of the first evidence anchor of the
// given type, failing the test if none exists.
func firstEvidenceRef(t *testing.T, res KSIResult, evidenceType EvidenceType) string {
	t.Helper()
	for _, ev := range res.Evidence {
		if ev.Type == evidenceType {
			return ev.Reference
		}
	}
	t.Fatalf("no evidence of type %s in result %s", evidenceType, res.ID)
	return ""
}
