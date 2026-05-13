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
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	"github.com/g8e-ai/g8e/components/g8eo/internal/services/governance"
	"github.com/g8e-ai/g8e/components/g8eo/internal/services/storage"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/operatorv1"
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
	catGoodActor category = iota // 70 % – FS_LIST, safe
	catPromptInj                 // 20 % – EXECUTE_BASH, forbidden cmd
	catMitM                      // 10 % – corrupted hash
)

func pickCategory(r *rand.Rand) category {
	n := r.Intn(100)
	switch {
	case n < 70:
		return catGoodActor
	case n < 90:
		return catPromptInj
	default:
		return catMitM
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

type fixedStateRoot struct{ root string }

func (f *fixedStateRoot) GetCurrentStateRoot() (string, error) { return f.root, nil }

// ── execution handler (no-op: chaos tester does not actually run commands) ────

type noopExecutionHandler struct{}

func (n *noopExecutionHandler) ExecuteVerifiedTransaction(_ context.Context, eventType string, _ interface{}) error {
	return nil
}

// ── result counters ───────────────────────────────────────────────────────────

type counters struct {
	executed  atomic.Int64
	l1Blocked atomic.Int64
	hashFail  atomic.Int64
	other     atomic.Int64
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

	const stateRoot = "chaos-state-root-v1"

	// ── governance stack ──────────────────────────────────────────────────────
	replayStore := newMemReplayStore()
	stateRootProvider := &fixedStateRoot{root: stateRoot}
	l3Verifier := &chaosL3Verifier{}

	knownActionTypes := []string{
		"EXECUTE_BASH", "FILE_EDIT", "RESTORE_FILE", "SHUTDOWN",
		"FS_LIST", "FS_READ", "FS_GREP", "PORT_CHECK", "FETCH_LOGS",
	}

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
		ExecutionHandler:  &noopExecutionHandler{},
		SigningKey:        privKey,
		KeyID:             keyID,
	}

	// ── fire payloads ─────────────────────────────────────────────────────────
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var cnt counters
	var wg sync.WaitGroup
	sem := make(chan struct{}, 20) // cap concurrency to avoid DB contention

	logger.Info("Firing payloads", "count", *flagCount)

	for i := 0; i < *flagCount; i++ {
		wg.Add(1)
		cat := pickCategory(r)
		idx := i
		sem <- struct{}{}
		go func(id int, c category) {
			defer wg.Done()
			defer func() { <-sem }()
			fireOne(id, c, stateRoot, privKey, keyID, verifier, warden, logger, &cnt)
		}(idx, cat)
	}

	wg.Wait()

	executed := cnt.executed.Load()
	l1Blocked := cnt.l1Blocked.Load()
	hashFail := cnt.hashFail.Load()
	other := cnt.other.Load()
	total := executed + l1Blocked + hashFail + other

	fmt.Printf("\n=== Chaos Run Complete ===\n")
	fmt.Printf("Total payloads : %d\n", total)
	fmt.Printf("EXECUTED       : %d (%.0f%%)\n", executed, pct(executed, total))
	fmt.Printf("L1_BLOCKED     : %d (%.0f%%)\n", l1Blocked, pct(l1Blocked, total))
	fmt.Printf("HASH_FAIL      : %d (%.0f%%)\n", hashFail, pct(hashFail, total))
	fmt.Printf("OTHER_REJECTED : %d (%.0f%%)\n", other, pct(other, total))
	fmt.Printf("\n")
	fmt.Printf("Audit DB  : %s\n", filepath.Join(dataDir, "g8e.db"))
	fmt.Printf("Ledger    : %s\n", filepath.Join(dataDir, "ledger"))
	fmt.Printf("\n")
	printDemoQueries(filepath.Join(dataDir, "g8e.db"))
}

func fireOne(
	id int,
	cat category,
	stateRoot string,
	privKey ed25519.PrivateKey,
	keyID string,
	verifier *governance.TransactionVerifier,
	warden *governance.Warden,
	logger *slog.Logger,
	cnt *counters,
) {
	var env *uap.UAPEnvelope
	var err error

	switch cat {
	case catGoodActor:
		env, err = buildGoodActorEnvelope(id, stateRoot, privKey, keyID)
	case catPromptInj:
		env, err = buildPromptInjEnvelope(id, stateRoot, privKey, keyID)
	case catMitM:
		env, err = buildMitMEnvelope(id, stateRoot, privKey, keyID)
	}

	if err != nil {
		logger.Error("envelope build failed", "id", id, "error", err)
		cnt.other.Add(1)
		return
	}

	verified, verErr := verifier.VerifyEnvelope(env)
	if verErr != nil {
		reason := classifyRejection(verErr)
		logger.Info("envelope rejected",
			"id", id,
			"category", categoryName(cat),
			"reason", verErr.Error())
		recordRejection(id, cat, env, verErr, warden, logger)
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

	_, execErr := warden.Execute(context.Background(), verified, nil)
	if execErr != nil {
		logger.Warn("warden execution error", "id", id, "error", execErr)
		cnt.other.Add(1)
		return
	}

	logger.Info("envelope executed", "id", id, "category", categoryName(cat))
	cnt.executed.Add(1)
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
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func categoryName(c category) string {
	switch c {
	case catGoodActor:
		return "GOOD_ACTOR"
	case catPromptInj:
		return "PROMPT_INJECTION"
	case catMitM:
		return "MITM"
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

func printDemoQueries(dbPath string) {
	fmt.Printf("=== Demo Queries (run these in sqlite3) ===\n\n")
	fmt.Printf("sqlite3 '%s'\n\n", dbPath)

	fmt.Printf("-- 1. Intercept rate (The Insurance Policy)\n")
	fmt.Printf("SELECT type AS status, COUNT(*) AS event_count\n")
	fmt.Printf("FROM events\n")
	fmt.Printf("GROUP BY type\n")
	fmt.Printf("ORDER BY event_count DESC;\n\n")

	fmt.Printf("-- 2. Timeline of events (last 20)\n")
	fmt.Printf("SELECT timestamp, type, content_text\n")
	fmt.Printf("FROM events\n")
	fmt.Printf("ORDER BY timestamp DESC\n")
	fmt.Printf("LIMIT 20;\n\n")

	fmt.Printf("-- 3. Rejection breakdown by category\n")
	fmt.Printf("SELECT type, COUNT(*) AS blocked\n")
	fmt.Printf("FROM events\n")
	fmt.Printf("WHERE type IN ('L1_BLOCKED','HASH_FAIL','L2_REJECTED','REJECTED')\n")
	fmt.Printf("GROUP BY type;\n\n")

	fmt.Printf("-- 4. Total execution throughput\n")
	fmt.Printf("SELECT\n")
	fmt.Printf("  SUM(CASE WHEN type = 'action_receipt' THEN 1 ELSE 0 END) AS warden_receipts,\n")
	fmt.Printf("  SUM(CASE WHEN type LIKE '%%BLOCKED%%' OR type LIKE '%%FAIL%%' OR type LIKE '%%REJECTED%%' THEN 1 ELSE 0 END) AS intercepted,\n")
	fmt.Printf("  COUNT(*) AS total\n")
	fmt.Printf("FROM events;\n\n")

	fmt.Printf("=== Git Ledger (run in %s) ===\n\n", filepath.Dir(dbPath)+"/ledger")
	fmt.Printf("git -C '%s/ledger' log --pretty=format:'%%h - %%an, %%ar : %%s' -n 10\n\n", filepath.Dir(dbPath))
	fmt.Printf("# Show a specific commit:\n")
	fmt.Printf("git -C '%s/ledger' show <commit-hash>\n\n", filepath.Dir(dbPath))
}
