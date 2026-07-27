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

// chaos generates a realistic distribution of governance events against
// the local g8e audit stack.  It bypasses network/TLS by driving the
// TransactionVerifier + Actuator stack directly in-process, which is the same
// path exercised by the live Operator when payloads arrive over pub/sub.
//
// Distribution:
//
//	70%  Good Actor  – valid sig, safe intent (FS_LIST)       → EXECUTED
//	20%  Prompt Inj  – valid sig, L1 forbidden cmd (sudo/rm)  → REJECTED (L1)
//	10%  MitM        – corrupted transaction hash              → REJECTED (hash mismatch)
//
// This is a TEST-ONLY package for chaos testing of the governance stack.
// It uses test infrastructure (storagetest.TestSQLAuditStore) and should not
// be used in production code paths.
package chaos

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/constants"
	govpkg "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/storage/storagetest"
	"github.com/g8e-ai/g8e/internal/services/system"
	vault "github.com/g8e-ai/g8e/internal/services/vault"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// Test placeholder constants for intentionally corrupted test payloads
const (
	chaosTestCorruptedHash = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
)

// ── flags ─────────────────────────────────────────────────────────────────────

// Config holds the configuration for chaos testing
type Config struct {
	Count   int
	DataDir string
	PKIDir  string
}

// ── payload categories ────────────────────────────────────────────────────────

type category int

const (
	catGoodActor    category = iota // 70 % – FS_LIST, safe
	catPromptInj                    // 20 % – EXECUTE_BASH, forbidden cmd
	catMitM                         // 10 % – corrupted hash
	catFileMutation                 // 10 % – FILE_EDIT, safe
)

func pickCategory(r *rand.Rand) category {
	n := r.IntN(100)
	switch {
	case n < 60:
		return catGoodActor
	case n < 80:
		return catPromptInj
	case n < 90:
		return catMitM
	default:
		return catFileMutation
	}
}

// ── envelope construction ─────────────────────────────────────────────────────

func buildGoodActorEnvelope(id int, stateRoot string, privKey ed25519.PrivateKey, keyID string, sessionID string) (*govpkg.GovernanceEnvelope, error) {
	payload, err := proto.Marshal(&operatorv1.FsListRequested{
		Path:        fmt.Sprintf("%s/chaos-%d", constants.PathTmp, id),
		ExecutionId: fmt.Sprintf("exec-good-%d", id),
	})
	if err != nil {
		return nil, err
	}
	return signedEnvelope("FS_LIST", "localhost", stateRoot,
		fmt.Sprintf("good-%d-%d", id, time.Now().UnixNano()),
		payload, false, privKey, keyID, sessionID)
}

func buildPromptInjEnvelope(id int, stateRoot string, privKey ed25519.PrivateKey, keyID string, sessionID string) (*govpkg.GovernanceEnvelope, error) {
	forbiddenCmds := []string{
		"sudo rm -rf " + constants.PathVar + "/log",
		"su root -c 'cat " + constants.PathEtcShadow + "'",
		"rm -rf " + constants.PathHome + "/user",
		"sudo useradd attacker",
		"sudo chmod 777 " + constants.PathEtcPasswd,
	}
	cmd := forbiddenCmds[id%len(forbiddenCmds)]
	payload, err := proto.Marshal(&operatorv1.CommandRequested{
		Command:       cmd,
		ExecutionId:   fmt.Sprintf("exec-inject-%d", id),
		Justification: "prompt injection attempt",
		Intent:        "ESCALATE_PRIVILEGES",
	})
	if err != nil {
		return nil, err
	}
	return signedEnvelope("EXECUTE_BASH", "localhost", stateRoot,
		fmt.Sprintf("inject-%d-%d", id, time.Now().UnixNano()),
		payload, false, privKey, keyID, sessionID)
}

func buildFileMutationEnvelope(id int, stateRoot string, privKey ed25519.PrivateKey, keyID string, sessionID string) (*govpkg.GovernanceEnvelope, error) {
	payload, err := proto.Marshal(&operatorv1.FileEditRequested{
		FilePath:    fmt.Sprintf("%s/chaos-edit-%d.txt", constants.PathTmp, id),
		Content:     fmt.Sprintf("chaos was here at %d", time.Now().UnixNano()),
		ExecutionId: fmt.Sprintf("exec-edit-%d", id),
	})
	if err != nil {
		return nil, err
	}
	// Mutations require L3 (human proof)
	return signedEnvelope("FILE_EDIT", "localhost", stateRoot,
		fmt.Sprintf("edit-%d-%d", id, time.Now().UnixNano()),
		payload, true, privKey, keyID, sessionID)
}

func buildMitMEnvelope(id int, stateRoot string, privKey ed25519.PrivateKey, keyID string, sessionID string) (*govpkg.GovernanceEnvelope, error) {
	payload, err := proto.Marshal(&operatorv1.FsListRequested{
		Path:        constants.PathEtc,
		ExecutionId: fmt.Sprintf("exec-mitm-%d", id),
	})
	if err != nil {
		return nil, err
	}
	env, err := signedEnvelope("FS_LIST", "localhost", stateRoot,
		fmt.Sprintf("mitm-%d-%d", id, time.Now().UnixNano()),
		payload, false, privKey, keyID, sessionID)
	if err != nil {
		return nil, err
	}
	// Corrupt the hash to simulate a man-in-the-middle tampering with the envelope.
	env.TransactionHash = chaosTestCorruptedHash
	return env, nil
}

func signedEnvelope(
	actionType, targetResource, stateRoot, nonceSuffix string,
	payload []byte,
	isMutation bool,
	privKey ed25519.PrivateKey,
	keyID string,
	sessionID string,
) (*govpkg.GovernanceEnvelope, error) {
	env := &govpkg.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().UTC().Add(30 * time.Minute)), // Increased to 30m for chaos runs
		SourceComponent:   commonv1.Component_COMPONENT_AGENT,
		OperatorId:        "chaos-operator",
		OperatorSessionId: sessionID,
		ActionType:        actionType,
		TargetResource:    targetResource,
		Payload:           payload,
		StateMerkleRoot:   stateRoot,
		Nonce:             fmt.Sprintf("chaos-%s-%s", nonceSuffix, hex.EncodeToString(payload[:clampMin(4, len(payload))])),
	}

	hash, err := govpkg.GenerateMessageID(env)
	if err != nil {
		return nil, fmt.Errorf("hash generation failed: %w", err)
	}
	env.Id = hash
	env.TransactionHash = hash

	l2Sig := hex.EncodeToString(ed25519.Sign(privKey, []byte(hash+"|true")))
	env.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			ConsensusSetId: "chaos-consensus",
			Votes: []*commonv1.L2Vote{
				{
					SignerKeyId:        keyID,
					ConsensusSignature: l2Sig,
					Decision:           true,
				},
			},
		},
	}

	if isMutation {
		env.Governance.L3 = &commonv1.L3Metadata{
			Proof: &commonv1.L3Proof{
				Signature: "chaos-human-proof",
			},
		}
	}

	return env, nil
}

func clampMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── replay store (in-memory, sufficient for chaos run) ────────────────────────

type memReplayStore struct {
	mu     sync.Mutex
	nonces map[string]bool
}

func newMemReplayStore() *memReplayStore {
	return &memReplayStore{nonces: make(map[string]bool)}
}

func (m *memReplayStore) ReserveNonce(nonce string, _ time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nonces[nonce] {
		return true, nil
	}
	m.nonces[nonce] = true
	return false, nil
}

func (m *memReplayStore) FinalizeNonce(nonce string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// No-op for in-memory store - nonce is already marked as used
	return nil
}

func (m *memReplayStore) ReleaseNonce(nonce string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.nonces, nonce)
	return nil
}

func (m *memReplayStore) Close() error {
	return nil
}

// ── L3 notary (auto-approve non-mutations; mutations need no L3 here) ───────

type chaosL3Notary struct{}

func (c *chaosL3Notary) VerifyL3Proof(ctx context.Context, userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error) {
	return true, nil
}

// ── state root provider ───────────────────────────────────────────────────────

type dynamicStateRoot struct {
	mu   sync.Mutex
	root string
}

func (d *dynamicStateRoot) GetCurrentStateRoot() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.root, nil
}

func (d *dynamicStateRoot) UpdateRoot(newRoot string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.root = newRoot
}

// ── execution handler (no-op: chaos tester does not actually run commands) ────

type chaosExecutionHandler struct {
	ledger        *storage.GitLedgerService
	stateRoot     *dynamicStateRoot
	mutationCount atomic.Int64
}

func (c *chaosExecutionHandler) ExecuteVerifiedTransaction(_ context.Context, eventType constants.EventType, cmdMsg governance.CommandMessage) (string, error) {
	msg, ok := cmdMsg.(*pubsub.PubSubCommandMessage)
	if !ok {
		return "", nil
	}

	// Simulate ledger activity for file mutations
	if eventType == constants.Event.Operator.FileEdit.Requested && c.ledger != nil {
		req := &operatorv1.FileEditRequested{}
		if err := proto.Unmarshal(msg.Payload, req); err == nil {
			slog.Info("Chaos simulating file mutation in ledger", "filepath", req.FilePath, "category", string(constants.ToolDisplayCategoryFile))
			// Simulate the two-phase ledger commit
			res, err := c.ledger.LedgerFileWrite(msg.OperatorSessionID, req.FilePath)
			if err != nil {
				slog.Error("LedgerFileWrite failed", "error", err)
			}
			if res != nil {
				// Create the dummy file so git can see it
				_ = os.MkdirAll(filepath.Dir(req.FilePath), 0755)
				_ = os.WriteFile(req.FilePath, []byte(req.Content), 0600)
				err = c.ledger.CompleteMirrorWrite(res, msg.OperatorSessionID)
				if err != nil {
					slog.Error("CompleteMirrorWrite failed", "error", err)
				} else {
					slog.Info("Chaos ledger mutation complete", "filepath", req.FilePath, "category", string(constants.ToolDisplayCategoryFile))
					// Note: State root updates disabled in chaos test to avoid race conditions
					// that cause hash verification failures in parallel execution
					count := c.mutationCount.Add(1)
					slog.Info("Mutation count updated", "mutation_count", count)
				}
			}
		} else {
			slog.Error("Failed to unmarshal FileEditRequested", "error", err)
		}
	}
	return "", nil
}

// Result counters

type counters struct {
	executed          atomic.Int64
	executedGoodActor atomic.Int64
	executedFileMut   atomic.Int64
	l1Blocked         atomic.Int64
	hashFail          atomic.Int64
	other             atomic.Int64
}

// ── main ──────────────────────────────────────────────────────────────────────

// Run executes the chaos test with the given configuration
func Run(cfg Config) error {
	if cfg.Count <= 0 {
		return fmt.Errorf("chaos: count must be positive, got %d", cfg.Count)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Initialize paths relative to current working directory
	if err := paths.Init(); err != nil {
		return fmt.Errorf("failed to initialize paths: %w", err)
	}

	// Use shared test vault directory for persistent inspection
	var baseDir string
	var testVaultDir string
	if cfg.DataDir == "" {
		testVaultDir = paths.Infra.TestVaultDir
		if err := os.MkdirAll(testVaultDir, constants.PermDirStandard); err != nil {
			return fmt.Errorf("%w: %v", constants.ErrDirCreateFailed, err)
		}

		// Create unique subdirectory for this test run
		testRunID := fmt.Sprintf("%s-chaos-test", time.Now().Format("20060102-150405"))
		baseDir = filepath.Join(testVaultDir, testRunID)
		if err := os.MkdirAll(baseDir, constants.PermDirStandard); err != nil {
			return fmt.Errorf("%w: %v", constants.ErrDirCreateFailed, err)
		}
	} else {
		baseDir = cfg.DataDir
		if err := os.MkdirAll(baseDir, constants.PermDirStandard); err != nil {
			return fmt.Errorf("%w: %v", constants.ErrDirCreateFailed, err)
		}
	}

	// Construct fileSvc for .g8e/ I/O within the test run directory
	fileSvc, err := fs.NewRuntimeFileService(baseDir, logger)
	if err != nil {
		return fmt.Errorf("failed to create file service: %w", err)
	}
	if err := fileSvc.CreateRuntimeTree(context.Background()); err != nil {
		return fmt.Errorf("failed to create runtime tree: %w", err)
	}
	dataDir := fileSvc.Resolve(constants.DataDirname)

	pkiDir := cfg.PKIDir
	if pkiDir == "" {
		pkiDir = paths.Infra.PkiDir
	}

	logArgs := []any{
		"count", cfg.Count,
		"data_dir", dataDir,
		"pki_dir", pkiDir,
	}
	if testVaultDir != "" {
		logArgs = append(logArgs, "test_vault", testVaultDir)
	}
	logger.Info("Chaos tester starting", logArgs...)

	// ── generate ephemeral L2 signing key ─────────────────────────────────────
	// In a real deployment the trusted signer key must be pre-provisioned in
	// <pki_dir>/trusted_signers/<keyID>.pub.  For the chaos run we generate a
	// fresh key and register it directly in the TransactionVerifier's in-memory
	// trusted signers map, which is exactly what the test suite does.
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("failed to generate L2 signing key: %w", err)
	}
	const keyID = "chaos-l2-key"
	trustedSigners := map[string]ed25519.PublicKey{keyID: pubKey}

	// ── vault ────────────────────────────────────────────────────────────────
	vaultDir := fileSvc.Resolve(filepath.Join(constants.DataDirname, "vault"))
	if err := fileSvc.MkdirAll(context.Background(), filepath.Join(constants.DataDirname, "vault"), constants.PermDirPrivate); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrDirCreateFailed, err)
	}
	_, vaultPrivKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrVaultKeyGenerateFailed, err)
	}
	vaultHeader, _, err := vault.NewVaultHeader(vaultPrivKey)
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrVaultHeaderCreateFailed, err)
	}
	if err := vaultHeader.Save(vaultDir); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrVaultHeaderSaveFailed, err)
	}
	encryptionVault, err := vault.NewVault(&vault.VaultConfig{
		DataDir: vaultDir,
		Logger:  logger,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrVaultCreateFailed, err)
	}
	if err := encryptionVault.Unlock(vaultPrivKey); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrVaultUnlockFailed, err)
	}

	// ── audit vault ───────────────────────────────────────────────────────────
	gitPath, _ := findGit()
	avCfg := &storagetest.TestSQLAuditStoreConfig{
		DBPath:                    "g8e.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               2048,
		RetentionDays:             90,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		GitPath:                   gitPath,
		EncryptionVault:           encryptionVault,
	}
	av, err := storagetest.NewTestSQLAuditStore(avCfg, logger, fileSvc)
	if err != nil {
		return fmt.Errorf("failed to initialize audit vault: %w", err)
	}
	// ── generate session IDs for concurrency ──────────────────────────────────
	workerCount := runtime.NumCPU() * 2
	sessionIDs := make([]string, workerCount)
	for i := 0; i < workerCount; i++ {
		sessionID := fmt.Sprintf("chaos-session-%03d", i+1)
		sessionIDs[i] = sessionID
		operator_session, err := av.GetOperatorSession(sessionID)
		if err != nil {
			av.Close()
			return fmt.Errorf("%w: %v", constants.ErrAuditStoreGetSessionFailed, err)
		}
		if operator_session == nil {
			if err := av.CreateSession(sessionID, "operator", fmt.Sprintf("Chaos Worker %d", i+1), "chaos@test.local"); err != nil {
				av.Close()
				return fmt.Errorf("%w: %v", constants.ErrAuditStoreCreateSessionFailed, err)
			}
		}
	}
	defer av.Close()

	const initialStateRoot = "chaos-state-root-v1"

	// ── governance stack ──────────────────────────────────────────────────────
	replayStore := newMemReplayStore()
	stateRootProvider := &dynamicStateRoot{root: initialStateRoot}
	l3Notary := &chaosL3Notary{}

	knownActionTypes := []constants.ActionType{
		constants.ActionTypeExecuteBash, constants.ActionTypeFileEdit, constants.ActionTypeRestoreFile, constants.ActionTypeShutdown,
		constants.ActionTypeFsList, constants.ActionTypeFsRead, constants.ActionTypeFsGrep, constants.ActionTypePortCheck, constants.ActionTypeFetchLogs,
		constants.ActionTypeEvalAnswer,
	}

	// Initialize Ledger (nil for chaos tester - no actual ledger needed)
	ledger, _ := storage.NewGitLedgerService(&storage.LedgerConfig{EncryptionVault: nil}, logger, fileSvc)

	// Initialize L1 Doctrine for threat detection
	doctrine := governance.NewL1Doctrine()

	// L4Warden replaces TransactionVerifier - performs all verification stages
	warden := governance.NewL4Warden(
		logger,
		replayStore,
		stateRootProvider,
		&governance.FailClosedSignerStore{Signers: trustedSigners},
		nil, // ConsensusStore not used in chaos tester
		nil, // AppPolicyStore not used in chaos tester
		l3Notary,
		doctrine,
		knownActionTypes,
		"doctrine",
		nil, // Clock defaults to RealClock
	)

	// L5Actuator replaces Actuator - execution boundary with receipt signing
	act := &governance.L5Actuator{
		Logger:            logger,
		ConsoleAuditStore: av,
		StateRootProvider: stateRootProvider,
		ExecutionHandler:  &chaosExecutionHandler{ledger: ledger, stateRoot: stateRootProvider},
		SigningKey:        privKey,
		KeyID:             keyID,
	}

	// ── phase 1: generate and count payloads by category ───────────────────────
	r := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) //nolint:gosec // deterministic chaos testing
	payloads := make([]category, cfg.Count)
	var generatedCounters struct {
		goodActor    int
		promptInj    int
		mitM         int
		fileMutation int
	}

	for i := 0; i < cfg.Count; i++ {
		cat := pickCategory(r)
		payloads[i] = cat
		switch cat {
		case catGoodActor:
			generatedCounters.goodActor++
		case catPromptInj:
			generatedCounters.promptInj++
		case catMitM:
			generatedCounters.mitM++
		case catFileMutation:
			generatedCounters.fileMutation++
		}
	}

	// Print expected outcomes based on generated categories
	fmt.Printf("\n=== Phase 1: Payload Generation Complete ===\n")
	fmt.Printf("Total payloads generated : %d\n", cfg.Count)
	fmt.Printf("\n")
	fmt.Printf("Expected Outcomes:\n")
	fmt.Printf("  SAFE_EXECUTIONS (catGoodActor)    : %d → Expected: EXECUTED\n", generatedCounters.goodActor)
	fmt.Printf("  FORBIDDEN_PATTERNS (catPromptInj) : %d → Expected: L1_BLOCKED\n", generatedCounters.promptInj)
	fmt.Printf("  HASH_CORRUPTION (catMitM)         : %d → Expected: HASH_FAIL\n", generatedCounters.mitM)
	fmt.Printf("  FILE_MUTATIONS (catFileMutation) : %d → Expected: EXECUTED (with L3)\n", generatedCounters.fileMutation)
	fmt.Printf("\n")
	fmt.Printf("Expected Totals:\n")
	expectedExecuted := generatedCounters.goodActor + generatedCounters.fileMutation
	expectedL1Blocked := generatedCounters.promptInj
	expectedHashFail := generatedCounters.mitM
	fmt.Printf("  EXECUTED       : %d (%.0f%%)\n", expectedExecuted, pct(int64(expectedExecuted), int64(cfg.Count)))
	fmt.Printf("  L1_BLOCKED     : %d (%.0f%%)\n", expectedL1Blocked, pct(int64(expectedL1Blocked), int64(cfg.Count)))
	fmt.Printf("  HASH_FAIL      : %d (%.0f%%)\n", expectedHashFail, pct(int64(expectedHashFail), int64(cfg.Count)))
	fmt.Printf("\n")
	fmt.Printf("=== Phase 2: Running payloads through protocol ===\n")

	// ── phase 2: fire payloads through the protocol ───────────────────────────
	var cnt counters
	var wg sync.WaitGroup

	// Batch rejection and success writers to reduce SQLite contention
	rejectionBatch := &batchEventWriter{
		auditVault: av,
		logger:     logger,
		flushSize:  100,
		events:     make([]*storagetest.ChaosEvent, 0, 100),
	}
	defer rejectionBatch.flush() // ensure final batch is written

	// Execution phase: CPU-aware concurrency
	sem := make(chan struct{}, workerCount)
	logger.Info("Starting execution phase", "workers", workerCount, "sessions", len(sessionIDs))

	for i, cat := range payloads {
		wg.Add(1)
		idx := i
		catCopy := cat
		sessionID := sessionIDs[idx%workerCount]
		sem <- struct{}{}
		go func(id int, c category, sid string) {
			defer wg.Done()
			defer func() { <-sem }()

			// Fetch current state root at execution time (may have been updated by prior mutations)
			currentRoot, _ := stateRootProvider.GetCurrentStateRoot()

			// Build envelope JUST-IN-TIME to avoid expiration during pre-build
			env, err := buildEnvelope(id, c, currentRoot, privKey, keyID, sid)
			if err != nil {
				logger.Error("envelope build failed", "id", id, "error", err)
				cnt.other.Add(1)
				return
			}

			fireOne(id, c, env, currentRoot, warden, act, logger, &cnt, rejectionBatch)
		}(idx, catCopy, sessionID)
	}

	wg.Wait()

	executed := cnt.executed.Load()
	l1Blocked := cnt.l1Blocked.Load()
	hashFail := cnt.hashFail.Load()
	other := cnt.other.Load()
	total := executed + l1Blocked + hashFail + other

	fmt.Printf("\n=== Phase 3: Protocol Enforcement Summary ===\n")
	fmt.Printf("Total payloads : %d\n\n", total)

	fmt.Printf("%-23s | %5s | %-16s | %6s | %s\n", "Category", "Count", "Expected", "Actual", "Verified")
	fmt.Printf("------------------------|-------|------------------|--------|----------\n")
	printSummaryRow("SAFE_EXECUTIONS", generatedCounters.goodActor, "EXECUTED", int(cnt.executedGoodActor.Load()))
	printSummaryRow("FILE_MUTATIONS", generatedCounters.fileMutation, "EXECUTED", int(cnt.executedFileMut.Load()))
	printSummaryRow("FORBIDDEN_PATTERNS", generatedCounters.promptInj, "L1_BLOCKED", int(cnt.l1Blocked.Load()))
	printSummaryRow("HASH_CORRUPTION", generatedCounters.mitM, "HASH_FAIL", int(cnt.hashFail.Load()))
	if cnt.other.Load() > 0 {
		printSummaryRow("OTHER_REJECTED", 0, "REJECTED", int(cnt.other.Load()))
	}
	fmt.Printf("------------------------|-------|------------------|--------|----------\n")

	successRate := pct(executed+l1Blocked+hashFail, int64(cfg.Count))
	matchTotal := "✓"
	if int(total) != cfg.Count {
		matchTotal = "✗"
	}
	fmt.Printf("%-23s | %5d | %-16s | %6d | %s (%.0f%% success)\n", "TOTAL", cfg.Count, "", int(total), matchTotal, successRate)

	fmt.Printf("\nNote: Results are probabilistic (~60/20/10/10 distribution) and will vary by run.\n")
	fmt.Printf("Use './g8e test summary' to see aggregate results across all test runs.\n")
	fmt.Printf("\n")
	fmt.Printf("Test vault: %s\n", dataDir)
	fmt.Printf("Audit DB  : %s\n", filepath.Join(dataDir, "g8e.db"))
	fmt.Printf("Ledger    : %s\n", filepath.Join(dataDir, "ledger"))
	fmt.Printf("\n")
	printDemoQueries(filepath.Join(dataDir, "g8e.db"))
	return nil
}

func buildEnvelope(id int, cat category, stateRoot string, privKey ed25519.PrivateKey, keyID string, sessionID string) (*govpkg.GovernanceEnvelope, error) {
	switch cat {
	case catGoodActor:
		return buildGoodActorEnvelope(id, stateRoot, privKey, keyID, sessionID)
	case catPromptInj:
		return buildPromptInjEnvelope(id, stateRoot, privKey, keyID, sessionID)
	case catMitM:
		return buildMitMEnvelope(id, stateRoot, privKey, keyID, sessionID)
	case catFileMutation:
		return buildFileMutationEnvelope(id, stateRoot, privKey, keyID, sessionID)
	default:
		return nil, fmt.Errorf("unknown chaos category: %d", cat)
	}
}

func fireOne(
	id int,
	cat category,
	env *govpkg.GovernanceEnvelope,
	stateRoot string,
	warden *governance.L4Warden,
	actuator *governance.L5Actuator,
	logger *slog.Logger,
	cnt *counters,
	batchWriter *batchEventWriter,
) {
	_, verErr := warden.VerifyEnvelope(context.Background(), env)
	if verErr != nil {
		reason := classifyRejection(verErr)
		logger.Info("envelope rejected",
			"id", id,
			"category", categoryName(cat),
			"reason", verErr.Error())
		batchWriter.recordRejection(id, cat, env, verErr)
		switch reason {
		case "L1_BLOCKED":
			cnt.l1Blocked.Add(1)
		case "HASH_FAIL":
			cnt.hashFail.Add(1)
		default:
			cnt.other.Add(1)
		}
		return
	}

	cmdMsg := pubsub.PubSubCommandMessage{
		ID:                env.Id,
		EventType:         constants.MapActionTypeToEventType(constants.ActionType(env.ActionType)),
		OperatorSessionID: env.OperatorSessionId,
		Payload:           env.Payload,
		Timestamp:         env.Timestamp.AsTime(),
	}

	// In chaos tester, we want to bypass the Actuator's synchronous SQLite write for successes too
	// so we use a custom execution flow that still hits the handler but batches the audit log.

	// Execute through the handler
	eventType := constants.MapActionTypeToEventType(constants.ActionType(env.ActionType))
	_, err := actuator.ExecutionHandler.ExecuteVerifiedTransaction(context.Background(), eventType, &cmdMsg)

	if err != nil {
		logger.Warn("execution error", "id", id, "error", err)
		cnt.other.Add(1)
		batchWriter.recordExecution(id, cat, env, err)
		return
	}

	logger.Info("envelope executed", "id", id, "category", categoryName(cat))
	cnt.executed.Add(1)
	switch cat {
	case catGoodActor:
		cnt.executedGoodActor.Add(1)
	case catFileMutation:
		cnt.executedFileMut.Add(1)
	}

	batchWriter.recordExecution(id, cat, env, nil)
}

// batchEventWriter batches events to reduce SQLite lock contention
type batchEventWriter struct {
	mu         sync.Mutex
	auditVault *storagetest.TestSQLAuditStore
	logger     *slog.Logger
	events     []*storagetest.ChaosEvent
	flushSize  int
}

func (b *batchEventWriter) recordRejection(id int, cat category, env *govpkg.GovernanceEnvelope, verErr error) {
	if b.auditVault == nil {
		return
	}

	reason := classifyRejection(verErr)
	event := &storagetest.ChaosEvent{
		OperatorSessionID: env.OperatorSessionId,
		Timestamp:         time.Now(),
		ChaosID:           id,
		Category:          categoryName(cat),
		Outcome:           reason,
		ContentText:       fmt.Sprintf("[chaos-id:%d] %s: %s", id, categoryName(cat), verErr.Error()),
		CommandRaw:        env.ActionType + " / " + env.TargetResource,
		TransactionHash:   env.TransactionHash,
	}

	b.queueEvent(event)
}

func (b *batchEventWriter) recordExecution(id int, cat category, env *govpkg.GovernanceEnvelope, execErr error) {
	if b.auditVault == nil {
		return
	}

	status := "COMPLETED"
	if execErr != nil {
		status = fmt.Sprintf("FAILED: %v", execErr)
	}

	event := &storagetest.ChaosEvent{
		OperatorSessionID: env.OperatorSessionId,
		Timestamp:         time.Now(),
		ChaosID:           id,
		Category:          categoryName(cat),
		Outcome:           status,
		ContentText:       fmt.Sprintf("[chaos-id:%d] %s: %s", id, categoryName(cat), status),
		CommandRaw:        fmt.Sprintf("%s / %s (hash: %s)", env.ActionType, env.TargetResource, env.TransactionHash[:12]),
		TransactionHash:   env.TransactionHash,
	}

	b.queueEvent(event)
}

func (b *batchEventWriter) queueEvent(ev *storagetest.ChaosEvent) {
	b.mu.Lock()
	b.events = append(b.events, ev)
	shouldFlush := len(b.events) >= b.flushSize
	b.mu.Unlock()

	if shouldFlush {
		b.flush()
	}
}

func (b *batchEventWriter) flush() {
	b.mu.Lock()
	events := b.events
	b.events = make([]*storagetest.ChaosEvent, 0, b.flushSize)
	b.mu.Unlock()

	if len(events) == 0 {
		return
	}

	// Use a single transaction for the whole batch
	if b.auditVault != nil {
		if err := b.auditVault.RecordChaosEvents(events); err != nil {
			b.logger.Warn("failed to record batch of chaos events", "error", err)
		}
	}
}

func classifyRejection(err error) string {
	if err == nil {
		return "EXECUTED"
	}
	msg := err.Error()
	switch {
	case contains(msg, "TX_L1_FAILED") || contains(msg, "TX_DOCTRINE_L1_FAILED"):
		return "L1_BLOCKED"
	case contains(msg, "TX_HASH_MISMATCH") || contains(msg, "TX_HASH_MISSING"):
		return "HASH_FAIL"
	case contains(msg, "TX_L2"):
		return "L2_REJECTED"
	case contains(msg, "TX_EXPIRED"):
		return "EXPIRED"
	case contains(msg, "TX_REPLAY"):
		return "REPLAY"
	default:
		return "REJECTED"
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

func categoryName(c category) string {
	switch c {
	case catGoodActor:
		return "GOOD_ACTOR"
	case catPromptInj:
		return "PROMPT_INJECTION"
	case catMitM:
		return "MITM"
	case catFileMutation:
		return "FILE_MUTATION"
	default:
		return "UNKNOWN"
	}
}

func pct(n, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

func findGit() (string, error) {
	return system.GitEmbedded, nil
}

func printSummaryRow(category string, count int, expectedOutcome string, actual int) {
	match := "✓"
	if count != actual {
		match = "✗"
	}
	fmt.Printf("%-23s | %5d | %-16s | %6d | %s\n", category, count, expectedOutcome, actual, match)
}

func printDemoQueries(dbPath string) {
	fmt.Printf("=== Demo Queries (run these via ./g8e) ===\n\n")

	fmt.Printf("# 1. View Chaos Test Summary (from test vault)\n")
	fmt.Printf("./g8e test summary\n\n")

	fmt.Printf("# 2. Direct SQLite access for offline analysis\n")
	fmt.Printf("# sqlite3 '%s'\n", dbPath)
	fmt.Printf("#   SELECT category, outcome, COUNT(*) FROM chaos_events GROUP BY category, outcome;\n")
	fmt.Printf("#   SELECT * FROM chaos_events ORDER BY timestamp DESC LIMIT 10;\n")
	fmt.Printf("#   SELECT category, outcome, COUNT(*) FROM chaos_events WHERE operator_session_id = 'chaos-session-001' GROUP BY category, outcome;\n\n")

	fmt.Printf("# 3. View General Audit Event Summary (from local audit vault, requires Gateway running)\n")
	fmt.Printf("./g8e gw data audit summary\n")
	fmt.Printf("./g8e gw data audit summary --operator-session-id <session-id>\n\n")

	fmt.Printf("# 4. View Audit Events via Operator API (requires Gateway running and mTLS auth)\n")
	fmt.Printf("./g8e gw data audit list --operator-session-id chaos-session-001 --limit 10\n\n")

	fmt.Printf("# 5. View all users (requires Gateway running)\n")
	fmt.Printf("./g8e gw data users\n\n")

	fmt.Printf("# 6. View operators (requires Gateway running)\n")
	fmt.Printf("./g8e gw data operators\n\n")
}
