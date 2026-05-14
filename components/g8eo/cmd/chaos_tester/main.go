// chaos_tester generates a realistic distribution of governance events against
// the local g8e audit stack.  It bypasses network/TLS by driving the
// TransactionVerifier + Warden stack directly in-process, which is the same
// path exercised by the live operator when payloads arrive over pub/sub.
//
// Distribution:
//
//	70%  Good Actor  – valid sig, safe intent (FS_LIST)       → EXECUTED
//	20%  Prompt Inj  – valid sig, L1 forbidden cmd (sudo/rm)  → REJECTED (L1)
//	10%  MitM        – corrupted transaction hash              → REJECTED (hash mismatch)
//
// Usage:
//
//	cd /home/bob/g8e/components/g8eo
//	go run ./cmd/chaos_tester [--count=100] [--data-dir=.g8e/data] [--pki-dir=.g8e/pki]
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/components/g8eo/internal/mappings"
	"github.com/g8e-ai/g8e/components/g8eo/internal/services/governance"
	"github.com/g8e-ai/g8e/components/g8eo/internal/services/pubsub"
	"github.com/g8e-ai/g8e/components/g8eo/internal/services/storage"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/operatorv1"
	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
)

// ── flags ─────────────────────────────────────────────────────────────────────

var (
	flagCount   = flag.Int("count", 100, "number of payloads to fire")
	flagDataDir = flag.String("data-dir", "", "audit vault data dir (default: <cwd>/.g8e/data)")
	flagPKIDir  = flag.String("pki-dir", "", "PKI dir for trusted_signers (default: <cwd>/.g8e/pki)")
)

// ── payload categories ────────────────────────────────────────────────────────

type category int

const (
	catGoodActor    category = iota // 70 % – FS_LIST, safe
	catPromptInj                    // 20 % – EXECUTE_BASH, forbidden cmd
	catMitM                         // 10 % – corrupted hash
	catFileMutation                 // 10 % – FILE_EDIT, safe
)

func pickCategory(r *rand.Rand) category {
	n := r.Intn(100)
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

func buildGoodActorEnvelope(id int, stateRoot string, privKey ed25519.PrivateKey, keyID string) (*uap.UAPEnvelope, error) {
	payload, err := proto.Marshal(&operatorv1.FsListRequested{
		Path:        fmt.Sprintf("/tmp/chaos-%d", id),
		ExecutionId: fmt.Sprintf("exec-good-%d", id),
	})
	if err != nil {
		return nil, err
	}
	return signedEnvelope("FS_LIST", "localhost", stateRoot,
		fmt.Sprintf("good-%d-%d", id, time.Now().UnixNano()),
		payload, false, privKey, keyID)
}

func buildPromptInjEnvelope(id int, stateRoot string, privKey ed25519.PrivateKey, keyID string) (*uap.UAPEnvelope, error) {
	forbiddenCmds := []string{
		"sudo rm -rf /var/log",
		"su root -c 'cat /etc/shadow'",
		"rm -rf /home/user",
		"sudo useradd attacker",
		"sudo chmod 777 /etc/passwd",
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
		payload, false, privKey, keyID)
}

func buildFileMutationEnvelope(id int, stateRoot string, privKey ed25519.PrivateKey, keyID string) (*uap.UAPEnvelope, error) {
	payload, err := proto.Marshal(&operatorv1.FileEditRequested{
		FilePath:    fmt.Sprintf("/tmp/chaos-edit-%d.txt", id),
		Content:     fmt.Sprintf("chaos was here at %d", time.Now().UnixNano()),
		ExecutionId: fmt.Sprintf("exec-edit-%d", id),
	})
	if err != nil {
		return nil, err
	}
	// Mutations require L3 (human proof)
	return signedEnvelope("FILE_EDIT", "localhost", stateRoot,
		fmt.Sprintf("edit-%d-%d", id, time.Now().UnixNano()),
		payload, true, privKey, keyID)
}

func buildMitMEnvelope(id int, stateRoot string, privKey ed25519.PrivateKey, keyID string) (*uap.UAPEnvelope, error) {
	payload, err := proto.Marshal(&operatorv1.FsListRequested{
		Path:        "/etc",
		ExecutionId: fmt.Sprintf("exec-mitm-%d", id),
	})
	if err != nil {
		return nil, err
	}
	env, err := signedEnvelope("FS_LIST", "localhost", stateRoot,
		fmt.Sprintf("mitm-%d-%d", id, time.Now().UnixNano()),
		payload, false, privKey, keyID)
	if err != nil {
		return nil, err
	}
	// Corrupt the hash to simulate a man-in-the-middle tampering with the envelope.
	env.TransactionHash = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	return env, nil
}

func signedEnvelope(
	actionType, targetResource, stateRoot, nonceSuffix string,
	payload []byte,
	isMutation bool,
	privKey ed25519.PrivateKey,
	keyID string,
) (*uap.UAPEnvelope, error) {
	env := &uap.UAPEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().UTC().Add(5 * time.Minute)),
		SourceComponent:   commonv1.Component_COMPONENT_G8EE,
		OperatorId:        "chaos-operator",
		OperatorSessionId: "chaos-session-001",
		ActionType:        actionType,
		TargetResource:    targetResource,
		Payload:           payload,
		StateMerkleRoot:   stateRoot,
		Nonce:             fmt.Sprintf("chaos-%s-%s", nonceSuffix, hex.EncodeToString(payload[:min(4, len(payload))])),
	}

	hash, err := uap.GenerateMessageID(env)
	if err != nil {
		return nil, fmt.Errorf("hash generation: %w", err)
	}
	env.Id = hash
	env.TransactionHash = hash

	l2Sig := hex.EncodeToString(ed25519.Sign(privKey, []byte(hash+"|true")))
	env.Governance = &commonv1.GovernanceMetadata{
		L2: &commonv1.L2Metadata{
			KeyId:             keyID,
			TribunalSignature: l2Sig,
			AgentIds:          []string{"chaos-tribunal-agent"},
		},
	}

	if isMutation {
		env.Governance.L3 = &commonv1.L3Metadata{
			HumanSignature: "chaos-human-proof",
			PublicKey:      "chaos-human-pubkey",
		}
	}

	return env, nil
}

func min(a, b int) int {
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

func (m *memReplayStore) CheckAndSetNonce(nonce string, _ time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nonces[nonce] {
		return true, nil
	}
	m.nonces[nonce] = true
	return false, nil
}

// ── L3 verifier (auto-approve non-mutations; mutations need no L3 here) ───────

type chaosL3Verifier struct{}

func (c *chaosL3Verifier) VerifyL3Proof(_, _, _, _ string) (bool, error) {
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
	ledger        *storage.LedgerService
	stateRoot     *dynamicStateRoot
	mutationCount atomic.Int64
}

func (c *chaosExecutionHandler) ExecuteVerifiedTransaction(_ context.Context, eventType string, cmdMsg interface{}) error {
	msg, ok := cmdMsg.(pubsub.PubSubCommandMessage)
	if !ok {
		return nil
	}

	// Simulate ledger activity for file mutations
	if eventType == "g8e.v1.operator.file.edit.requested" && c.ledger != nil {
		req := &operatorv1.FileEditRequested{}
		if err := proto.Unmarshal(msg.Payload, req); err == nil {
			slog.Info("Chaos simulating file mutation in ledger", "file", req.FilePath)
			// Simulate the two-phase ledger commit
			res, err := c.ledger.LedgerFileWrite(msg.OperatorSessionID, req.FilePath)
			if err != nil {
				slog.Error("LedgerFileWrite failed", "error", err)
			}
			if res != nil {
				// Create the dummy file so git can see it
				_ = os.MkdirAll(filepath.Dir(req.FilePath), 0755)
				_ = os.WriteFile(req.FilePath, []byte(req.Content), 0644)
				err = c.ledger.CompleteMirrorWrite(res, msg.OperatorSessionID)
				if err != nil {
					slog.Error("CompleteMirrorWrite failed", "error", err)
				} else {
					slog.Info("Chaos ledger mutation complete", "file", req.FilePath)
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
	return nil
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

func main() {
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cwd, err := os.Getwd()
	if err != nil {
		logger.Error("cannot determine working directory", "error", err)
		os.Exit(1)
	}

	dataDir := *flagDataDir
	if dataDir == "" {
		dataDir = filepath.Join(cwd, ".g8e", "data")
	}
	pkiDir := *flagPKIDir
	if pkiDir == "" {
		pkiDir = filepath.Join(cwd, ".g8e", "pki")
	}

	logger.Info("Chaos tester starting",
		"count", *flagCount,
		"data_dir", dataDir,
		"pki_dir", pkiDir)

	// ── generate ephemeral L2 signing key ─────────────────────────────────────
	// In a real deployment the trusted signer key must be pre-provisioned in
	// <pki_dir>/trusted_signers/<keyID>.pub.  For the chaos run we generate a
	// fresh key and register it directly in the TransactionVerifier's in-memory
	// trusted signers map, which is exactly what the test suite does.
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		logger.Error("failed to generate L2 signing key", "error", err)
		os.Exit(1)
	}
	const keyID = "chaos-l2-key"
	trustedSigners := map[string]ed25519.PublicKey{keyID: pubKey}

	// ── audit vault ───────────────────────────────────────────────────────────
	gitPath, _ := findGit()
	avCfg := &storage.AuditVaultConfig{
		DataDir:                   dataDir,
		DBPath:                    "g8e.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               2048,
		RetentionDays:             90,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		GitPath:                   gitPath,
	}
	av, err := storage.NewAuditVaultService(avCfg, logger)
	if err != nil {
		logger.Error("failed to initialise audit vault", "error", err)
		os.Exit(1)
	}
	defer av.Close()

	const initialStateRoot = "chaos-state-root-v1"

	// ── governance stack ──────────────────────────────────────────────────────
	replayStore := newMemReplayStore()
	stateRootProvider := &dynamicStateRoot{root: initialStateRoot}
	l3Verifier := &chaosL3Verifier{}

	knownActionTypes := []string{
		"EXECUTE_BASH", "FILE_EDIT", "RESTORE_FILE", "SHUTDOWN",
		"FS_LIST", "FS_READ", "FS_GREP", "PORT_CHECK", "FETCH_LOGS",
	}

	// Initialize Ledger
	ledger := storage.NewLedgerService(av, nil, logger)

	verifier := governance.NewTransactionVerifier(
		logger,
		replayStore,
		stateRootProvider,
		trustedSigners,
		l3Verifier,
		knownActionTypes,
	)

	warden := &governance.Warden{
		Logger:            logger,
		TrustedNodes:      trustedSigners,
		AuditVault:        av,
		L3Verifier:        l3Verifier,
		StateRootProvider: stateRootProvider,
		ExecutionHandler:  &chaosExecutionHandler{ledger: ledger, stateRoot: stateRootProvider},
		SigningKey:        privKey,
		KeyID:             keyID,
	}

	// ── phase 1: generate and count payloads by category ───────────────────────
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	payloads := make([]category, *flagCount)
	var generatedCounters struct {
		goodActor    int
		promptInj    int
		mitM         int
		fileMutation int
	}

	for i := 0; i < *flagCount; i++ {
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
	fmt.Printf("Total payloads generated : %d\n", *flagCount)
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
	fmt.Printf("  EXECUTED       : %d (%.0f%%)\n", expectedExecuted, pct(int64(expectedExecuted), int64(*flagCount)))
	fmt.Printf("  L1_BLOCKED     : %d (%.0f%%)\n", expectedL1Blocked, pct(int64(expectedL1Blocked), int64(*flagCount)))
	fmt.Printf("  HASH_FAIL      : %d (%.0f%%)\n", expectedHashFail, pct(int64(expectedHashFail), int64(*flagCount)))
	fmt.Printf("\n")
	fmt.Printf("=== Phase 2: Running payloads through protocol ===\n")

	// ── phase 2: fire payloads through the protocol ───────────────────────────
	var cnt counters
	var wg sync.WaitGroup
	// Pre-build all envelopes to parallelize crypto work before hot path
	logger.Info("Pre-building envelopes...", "count", *flagCount)
	envelopes := make([]*uap.UAPEnvelope, *flagCount)
	var buildWg sync.WaitGroup
	buildSem := make(chan struct{}, runtime.NumCPU()*2)
	var buildErrs atomic.Int64

	for i, cat := range payloads {
		buildWg.Add(1)
		idx := i
		catCopy := cat
		buildSem <- struct{}{}
		go func(id int, c category) {
			defer buildWg.Done()
			defer func() { <-buildSem }()
			env, err := buildEnvelope(id, c, initialStateRoot, privKey, keyID)
			if err != nil {
				logger.Error("envelope build failed", "id", id, "error", err)
				buildErrs.Add(1)
				return
			}
			envelopes[id] = env
		}(idx, catCopy)
	}
	buildWg.Wait()

	if buildErrs.Load() > 0 {
		logger.Error("Failed to build some envelopes", "errors", buildErrs.Load())
	}

	// Batch rejection writer to reduce SQLite contention
	rejectionBatch := &batchRejectionWriter{
		warden:    warden,
		logger:    logger,
		flushSize: 50,
		events:    make([]*storage.Event, 0, 50),
	}
	defer rejectionBatch.flush() // ensure final batch is written

	// Execution phase: CPU-aware concurrency
	workerCount := runtime.NumCPU() * 2
	sem := make(chan struct{}, workerCount)
	logger.Info("Starting execution phase", "workers", workerCount)

	for i, cat := range payloads {
		if envelopes[i] == nil {
			continue // skip failed builds
		}
		wg.Add(1)
		idx := i
		catCopy := cat
		sem <- struct{}{}
		go func(id int, c category, env *uap.UAPEnvelope) {
			defer wg.Done()
			defer func() { <-sem }()
			// Fetch current state root at execution time (may have been updated by prior mutations)
			currentRoot, _ := stateRootProvider.GetCurrentStateRoot()
			fireOnePrebuilt(id, c, env, currentRoot, verifier, warden, logger, &cnt, rejectionBatch)
		}(idx, catCopy, envelopes[idx])
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

	successRate := pct(executed+l1Blocked+hashFail, int64(*flagCount))
	matchTotal := "✓"
	if int(total) != *flagCount {
		matchTotal = "✗"
	}
	fmt.Printf("%-23s | %5d | %-16s | %6d | %s (%.0f%% success)\n", "TOTAL", *flagCount, "", int(total), matchTotal, successRate)

	fmt.Printf("\nNote: Results are probabilistic (~60/20/10/10 distribution) and will vary by run.\n")
	fmt.Printf("Use './g8e data audit summary' to see aggregate results across all runs.\n")
	fmt.Printf("\n")
	fmt.Printf("Audit DB  : %s\n", filepath.Join(dataDir, "g8e.db"))
	fmt.Printf("Ledger    : %s\n", filepath.Join(dataDir, "ledger"))
	fmt.Printf("\n")
	printDemoQueries(filepath.Join(dataDir, "g8e.db"))
}

func buildEnvelope(id int, cat category, stateRoot string, privKey ed25519.PrivateKey, keyID string) (*uap.UAPEnvelope, error) {
	switch cat {
	case catGoodActor:
		return buildGoodActorEnvelope(id, stateRoot, privKey, keyID)
	case catPromptInj:
		return buildPromptInjEnvelope(id, stateRoot, privKey, keyID)
	case catMitM:
		return buildMitMEnvelope(id, stateRoot, privKey, keyID)
	case catFileMutation:
		return buildFileMutationEnvelope(id, stateRoot, privKey, keyID)
	default:
		return nil, fmt.Errorf("unknown category: %d", cat)
	}
}

func fireOnePrebuilt(
	id int,
	cat category,
	env *uap.UAPEnvelope,
	stateRoot string,
	verifier *governance.TransactionVerifier,
	warden *governance.Warden,
	logger *slog.Logger,
	cnt *counters,
	rejectionBatch *batchRejectionWriter,
) {
	verified, verErr := verifier.VerifyEnvelope(env)
	if verErr != nil {
		reason := classifyRejection(verErr)
		logger.Info("envelope rejected",
			"id", id,
			"category", categoryName(cat),
			"reason", verErr.Error())
		rejectionBatch.record(id, cat, env, verErr)
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
		EventType:         mappings.MapActionTypeToEventType(env.ActionType),
		OperatorSessionID: env.OperatorSessionId,
		Payload:           env.Payload,
		Timestamp:         env.Timestamp.AsTime(),
	}

	_, execErr := warden.Execute(context.Background(), verified, cmdMsg)
	if execErr != nil {
		logger.Warn("warden execution error", "id", id, "error", execErr)
		cnt.other.Add(1)
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
}

// batchRejectionWriter batches rejection events to reduce SQLite lock contention
type batchRejectionWriter struct {
	mu        sync.Mutex
	warden    *governance.Warden
	logger    *slog.Logger
	events    []*storage.Event
	flushSize int
}

func (b *batchRejectionWriter) record(id int, cat category, env *uap.UAPEnvelope, verErr error) {
	if b.warden.AuditVault == nil {
		return
	}

	reason := classifyRejection(verErr)
	event := &storage.Event{
		OperatorSessionID: "chaos-session-001",
		Timestamp:         time.Now(),
		Type:              storage.EventType(reason),
		ContentText:       fmt.Sprintf("[chaos-id:%d] %s: %s", id, categoryName(cat), verErr.Error()),
		CommandRaw:        env.ActionType + " / " + env.TargetResource,
	}

	b.mu.Lock()
	b.events = append(b.events, event)
	shouldFlush := len(b.events) >= b.flushSize
	b.mu.Unlock()

	if shouldFlush {
		b.flush()
	}
}

func (b *batchRejectionWriter) flush() {
	b.mu.Lock()
	events := b.events
	b.events = make([]*storage.Event, 0, b.flushSize)
	b.mu.Unlock()

	if len(events) == 0 {
		return
	}

	for _, ev := range events {
		if _, err := b.warden.AuditVault.RecordEvent(ev); err != nil {
			b.logger.Warn("failed to record rejection event", "error", err)
		}
	}
}

// recordRejection writes a rejection event directly to the audit vault so
// rejected envelopes appear in the events table alongside successes.
func recordRejection(
	id int,
	cat category,
	env *uap.UAPEnvelope,
	verErr error,
	warden *governance.Warden,
	logger *slog.Logger,
) {
	if warden.AuditVault == nil {
		return
	}

	reason := classifyRejection(verErr)
	event := &storage.Event{
		OperatorSessionID: "chaos-session-001",
		Timestamp:         time.Now(),
		Type:              storage.EventType(reason),
		ContentText:       fmt.Sprintf("[chaos-id:%d] %s: %s", id, categoryName(cat), verErr.Error()),
		CommandRaw:        env.ActionType + " / " + env.TargetResource,
	}
	if _, err := warden.AuditVault.RecordEvent(event); err != nil {
		logger.Warn("failed to record rejection event", "id", id, "error", err)
	}
}

func classifyRejection(err error) string {
	if err == nil {
		return "EXECUTED"
	}
	msg := err.Error()
	switch {
	case contains(msg, "TX_L1_FAILED"):
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
	for _, p := range []string{"/usr/bin/git", "/usr/local/bin/git"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("git not found")
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

	fmt.Printf("# 1. View Governance Summary (intercept rate, attack patterns, action types)\n")
	fmt.Printf("./g8e data audit --db-path '%s' summary\n\n", dbPath)

	fmt.Printf("# 2. View Recent Events (last 10)\n")
	fmt.Printf("./g8e data audit --db-path '%s' events --session chaos-session-001 --limit 10\n\n", dbPath)

	fmt.Printf("# 3. Inspect Git Ledger (audit trail)\n")
	fmt.Printf("./g8e data audit --db-path '%s' ledger log\n\n", dbPath)

	fmt.Printf("# 4. Search Ledger for specific patterns\n")
	fmt.Printf("./g8e data audit --db-path '%s' ledger grep --pattern \"FS_LIST\"\n\n", dbPath)

	fmt.Printf("# 5. Verify Ledger Integrity\n")
	fmt.Printf("./g8e data audit --db-path '%s' ledger verify\n\n", dbPath)

	fmt.Printf("# 6. View Specific Ledger Commit\n")
	fmt.Printf("./g8e data audit --db-path '%s' ledger show --commit <hash>\n\n", dbPath)

	fmt.Printf("# Note: You can also use raw sqlite3 if needed:\n")
	fmt.Printf("# sqlite3 '%s'\n\n", dbPath)
}
