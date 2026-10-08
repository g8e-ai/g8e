// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"log/slog"
	"sync/atomic"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
)

// StateRootService owns the incremental state Merkle commitments. It maintains
// two roots:
//   - Bound root: commits to bound-state rows (documents, bound KV entries and
//     blobs). This is the freshness root that gates transaction admission.
//   - Observed root: commits to observed-state rows (telemetry, environmental
//     readings). It does NOT gate admission but is chained into the audit ledger.
//
// This tiering implements §8 of the position paper: conflating observed and
// bound state causes the binding root to churn continuously, making every
// in-flight envelope stale and degrading fail-closed into fail-always.
//
// The bound root also incorporates the token keymap hash from the ScrubbingService
// (§9): this binds the token→value mapping into the state Merkle root so that a
// rehydration substitution is a broken transaction, not silent corruption.
//
// Commitment maintenance (algorithm stateCommitmentAlgorithm):
//   - Schema triggers record the identity of every changed committed row in
//     state_commitment_dirty, inside the writer's own transaction. Every writer
//     participates, including direct SQL, deletes and TTL maintenance.
//   - A root read with no dirty rows reads one committed tree root.
//   - Otherwise one BEGIN IMMEDIATE transaction rehashes only the dirty leaves,
//     their buckets and the bucket ancestor paths, clears the dirty set and reads
//     the new root. The result always describes one committed snapshot; a rollback
//     publishes nothing. No Go mutex is held across SQL or hashing.
//
// Tree shape: a leaf identity is SHA-256 over (source, key); its first two bytes
// select one of 65,536 buckets. A bucket digest commits to its leaves ordered by
// identity. Above the buckets is a fixed 16-ary tree of stateTreeDepth levels, so
// one changed leaf touches one bucket and stateTreeDepth ancestors regardless of
// history size. Only non-empty nodes are stored; empty subtrees use precomputed
// digests, so the root depends only on the committed leaf set.
type StateRootService struct {
	db     *sqliteutil.DB
	logger *slog.Logger

	// Optional keymap hash provider for token binding (§9)
	keymapHashProvider atomic.Pointer[KeymapHashProvider]

	// flushedLeaves counts leaf identities processed by commitment flushes. It
	// lets tests assert that work follows the change set, not history size.
	flushedLeaves atomic.Int64
}

// KeymapHashProvider returns a deterministic hash of the token keymap.
// Implemented by ScrubbingService to bind token→value mappings into the state root.
type KeymapHashProvider interface {
	TokenKeymapHash() string
}

const (
	// stateCommitmentAlgorithm identifies the leaf/tree encoding. A database whose
	// recorded algorithm differs is rebuilt once at open.
	stateCommitmentAlgorithm = 2

	stateTierBound    = "bound"
	stateTierObserved = "observed"

	stateSourceDocuments = "documents"
	stateSourceKV        = "kv_store"
	stateSourceBlobs     = "blobs"

	stateTreeFanout = 16
	stateTreeDepth  = 4 // levels above the buckets; buckets live at this level
)

// Domain separation tags; every hashed field is length-prefixed.
var (
	stateTagLeafID = []byte("g8e/state/v2/leaf-id")
	stateTagLeaf   = []byte("g8e/state/v2/leaf")
	stateTagBucket = []byte("g8e/state/v2/bucket")
	stateTagNode   = []byte("g8e/state/v2/node")
	stateTagRoot   = []byte("g8e/state/v2/root")
)

// stateEmptyDigests[level] is the digest of an empty subtree at that level.
var stateEmptyDigests = func() [stateTreeDepth + 1][]byte {
	var empty [stateTreeDepth + 1][]byte
	empty[stateTreeDepth] = stateBucketDigest(nil)
	for level := stateTreeDepth - 1; level >= 0; level-- {
		children := make([][]byte, stateTreeFanout)
		for i := range children {
			children[i] = empty[level+1]
		}
		empty[level] = stateNodeDigest(level, children)
	}
	return empty
}()

// NewStateRootService creates a new state root service.
func NewStateRootService(db *sqliteutil.DB, logger *slog.Logger) *StateRootService {
	return &StateRootService{
		db:     db,
		logger: logger,
	}
}

// SetKeymapHashProvider injects the token keymap hash provider.
// When set, the keymap hash is incorporated into the bound state root.
func (s *StateRootService) SetKeymapHashProvider(p KeymapHashProvider) {
	s.keymapHashProvider.Store(&p)
}

// GetCurrentStateRoot returns the committed bound state root, first folding in
// any committed changes not yet reflected in the tree.
func (s *StateRootService) GetCurrentStateRoot() (string, error) {
	treeRoot, err := s.committedTreeRoot(stateTierBound)
	if err != nil {
		return "", err
	}
	keymapHash := ""
	if p := s.keymapHashProvider.Load(); p != nil && *p != nil {
		keymapHash = (*p).TokenKeymapHash()
	}
	return stateRootDigest(stateTierBound, treeRoot, keymapHash), nil
}

// GetObservedStateRoot returns the observed-state commitment root.
// This root is separate from the bound root and does NOT gate transaction
// admission. It is used for audit ledger chaining of observed evidence.
func (s *StateRootService) GetObservedStateRoot() (string, error) {
	treeRoot, err := s.committedTreeRoot(stateTierObserved)
	if err != nil {
		return "", err
	}
	return stateRootDigest(stateTierObserved, treeRoot, ""), nil
}

// committedTreeRoot reads the tier's tree root and the dirty marker in one
// statement (one snapshot). Pending changes are flushed first.
func (s *StateRootService) committedTreeRoot(tier string) ([]byte, error) {
	var dirty bool
	var digest []byte
	err := s.db.QueryRowWithRetry(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM state_commitment_dirty),
		        (SELECT digest FROM state_nodes WHERE tier = ? AND level = 0 AND idx = 0)`,
		tier,
	).Scan(&dirty, &digest)
	if err != nil {
		return nil, fmt.Errorf("%w: read committed root: %w", constants.ErrStateRootCalculate, err)
	}
	if dirty {
		err = s.db.ExecInImmediateTxWithRetry(context.Background(), func(conn *sql.Conn) error {
			if err := s.flush(conn); err != nil {
				return err
			}
			digest = nil
			err := conn.QueryRowContext(context.Background(),
				"SELECT digest FROM state_nodes WHERE tier = ? AND level = 0 AND idx = 0", tier).Scan(&digest)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: read flushed root: %w", constants.ErrStateRootCalculate, err)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if digest == nil {
		return stateEmptyDigests[0], nil
	}
	return digest, nil
}

// RebuildCommitment discards the persisted tree and rebuilds it from every
// committed row, then records the current algorithm. It runs in one transaction,
// so readers see either the previous commitment or the complete rebuild.
func (s *StateRootService) RebuildCommitment(ctx context.Context) error {
	return s.db.ExecInImmediateTxWithRetry(ctx, func(conn *sql.Conn) error {
		for _, stmt := range []string{
			"DELETE FROM state_leaves",
			"DELETE FROM state_nodes",
			"DELETE FROM state_commitment_dirty",
			"INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'documents', collection, id FROM documents",
			"INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'kv_store', key, '' FROM kv_store",
			"INSERT INTO state_commitment_dirty (source, k1, k2) SELECT 'blobs', namespace, id FROM blobs",
		} {
			if _, err := conn.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("%w: rebuild: %w", constants.ErrStateRootPersist, err)
			}
		}
		if err := s.flush(conn); err != nil {
			return err
		}
		_, err := conn.ExecContext(ctx,
			`INSERT INTO state_commitment (id, algorithm) VALUES (1, ?)
			 ON CONFLICT(id) DO UPDATE SET algorithm = excluded.algorithm`, stateCommitmentAlgorithm)
		if err != nil {
			return fmt.Errorf("%w: record algorithm: %w", constants.ErrStateRootPersist, err)
		}
		return nil
	})
}

// CommitmentAlgorithm returns the algorithm recorded for the persisted tree, or
// zero when none has been built.
func (s *StateRootService) CommitmentAlgorithm() (int, error) {
	var algorithm int
	err := s.db.QueryRowWithRetry(context.Background(), "SELECT algorithm FROM state_commitment WHERE id = 1").Scan(&algorithm)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("%w: read algorithm: %w", constants.ErrStateRootCalculate, err)
	}
	return algorithm, nil
}

type stateDirtyKey struct {
	source, k1, k2 string
}

type stateTierBucket struct {
	tier   string
	bucket int
}

// flush applies every pending dirty row to the leaves and recomputes the
// affected buckets and ancestor paths. The caller owns the write transaction.
func (s *StateRootService) flush(conn *sql.Conn) error {
	ctx := context.Background()
	keys, err := readDirtyKeys(ctx, conn)
	if err != nil || len(keys) == 0 {
		return err
	}

	dirtyBuckets := make(map[stateTierBucket]struct{})
	for _, key := range keys {
		leafID := stateLeafID(key.source, key.k1, key.k2)
		bucket := stateBucketOf(leafID)
		tier, digest, present, err := readLeafContent(ctx, conn, key)
		if err != nil {
			return err
		}

		var oldTier string
		var oldDigest []byte
		err = conn.QueryRowContext(ctx, "SELECT tier, digest FROM state_leaves WHERE leaf_id = ?", leafID).Scan(&oldTier, &oldDigest)
		had := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: read leaf: %w", constants.ErrStateRootCalculate, err)
		}

		if had && (!present || oldTier != tier) {
			if _, err := conn.ExecContext(ctx, "DELETE FROM state_leaves WHERE leaf_id = ?", leafID); err != nil {
				return fmt.Errorf("%w: delete leaf: %w", constants.ErrStateRootPersist, err)
			}
			dirtyBuckets[stateTierBucket{oldTier, bucket}] = struct{}{}
		}
		if present && (!had || oldTier != tier || !bytes.Equal(oldDigest, digest)) {
			if _, err := conn.ExecContext(ctx,
				`INSERT INTO state_leaves (leaf_id, tier, bucket, digest) VALUES (?, ?, ?, ?)
				 ON CONFLICT(leaf_id) DO UPDATE SET tier = excluded.tier, bucket = excluded.bucket, digest = excluded.digest`,
				leafID, tier, bucket, digest); err != nil {
				return fmt.Errorf("%w: write leaf: %w", constants.ErrStateRootPersist, err)
			}
			dirtyBuckets[stateTierBucket{tier, bucket}] = struct{}{}
		}
	}
	if _, err := conn.ExecContext(ctx, "DELETE FROM state_commitment_dirty"); err != nil {
		return fmt.Errorf("%w: clear dirty set: %w", constants.ErrStateRootPersist, err)
	}
	s.flushedLeaves.Add(int64(len(keys)))

	byTier := make(map[string]map[int]struct{})
	for tb := range dirtyBuckets {
		if byTier[tb.tier] == nil {
			byTier[tb.tier] = make(map[int]struct{})
		}
		byTier[tb.tier][tb.bucket] = struct{}{}
	}
	for tier, buckets := range byTier {
		if err := recomputeTreePaths(ctx, conn, tier, buckets); err != nil {
			return err
		}
	}
	return nil
}

func readDirtyKeys(ctx context.Context, conn *sql.Conn) ([]stateDirtyKey, error) {
	rows, err := conn.QueryContext(ctx, "SELECT source, k1, k2 FROM state_commitment_dirty")
	if err != nil {
		return nil, fmt.Errorf("%w: read dirty set: %w", constants.ErrStateRootQueryTable, err)
	}
	defer rows.Close()
	var keys []stateDirtyKey
	for rows.Next() {
		var key stateDirtyKey
		if err := rows.Scan(&key.source, &key.k1, &key.k2); err != nil {
			return nil, fmt.Errorf("%w: scan dirty set: %w", constants.ErrStateRootIterateRows, err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: iterate dirty set: %w", constants.ErrStateRootIterateRows, err)
	}
	return keys, nil
}

// readLeafContent reads the committed row behind a dirty key and returns its
// tier and leaf digest. present is false when the row no longer exists.
func readLeafContent(ctx context.Context, conn *sql.Conn, key stateDirtyKey) (tier string, digest []byte, present bool, err error) {
	switch key.source {
	case stateSourceDocuments:
		var data string
		err = conn.QueryRowContext(ctx, "SELECT data FROM documents WHERE collection = ? AND id = ?", key.k1, key.k2).Scan(&data)
		if err == nil {
			return stateTierBound, stateLeafDigest(key, []byte(data)), true, nil
		}
	case stateSourceKV:
		var value, expiresAt string
		err = conn.QueryRowContext(ctx,
			"SELECT value, COALESCE(expires_at, ''), state_tier FROM kv_store WHERE key = ?", key.k1).Scan(&value, &expiresAt, &tier)
		if err == nil {
			return tier, stateLeafDigest(key, []byte(value), []byte(expiresAt)), true, nil
		}
	case stateSourceBlobs:
		var size int64
		var contentType, expiresAt string
		var data []byte
		err = conn.QueryRowContext(ctx,
			"SELECT size, content_type, data, COALESCE(expires_at, ''), state_tier FROM blobs WHERE namespace = ? AND id = ?",
			key.k1, key.k2).Scan(&size, &contentType, &data, &expiresAt, &tier)
		if err == nil {
			sizeBytes := binary.BigEndian.AppendUint64(nil, uint64(size))
			return tier, stateLeafDigest(key, sizeBytes, []byte(contentType), data, []byte(expiresAt)), true, nil
		}
	default:
		return "", nil, false, fmt.Errorf("%w: %q", constants.ErrStateRootUnknownSource, key.source)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	return "", nil, false, fmt.Errorf("%w: read %s row: %w", constants.ErrStateRootQueryTable, key.source, err)
}

// recomputeTreePaths rehashes the given buckets and each ancestor once.
func recomputeTreePaths(ctx context.Context, conn *sql.Conn, tier string, buckets map[int]struct{}) error {
	parents := make(map[int]struct{})
	for bucket := range buckets {
		digest, err := readBucketDigest(ctx, conn, tier, bucket)
		if err != nil {
			return err
		}
		if err := putStateNode(ctx, conn, tier, stateTreeDepth, bucket, digest); err != nil {
			return err
		}
		parents[bucket/stateTreeFanout] = struct{}{}
	}
	for level := stateTreeDepth - 1; level >= 0; level-- {
		next := make(map[int]struct{})
		for idx := range parents {
			children, err := readChildDigests(ctx, conn, tier, level+1, idx)
			if err != nil {
				return err
			}
			if err := putStateNode(ctx, conn, tier, level, idx, stateNodeDigest(level, children)); err != nil {
				return err
			}
			next[idx/stateTreeFanout] = struct{}{}
		}
		parents = next
	}
	return nil
}

func readBucketDigest(ctx context.Context, conn *sql.Conn, tier string, bucket int) ([]byte, error) {
	rows, err := conn.QueryContext(ctx,
		"SELECT leaf_id, digest FROM state_leaves WHERE tier = ? AND bucket = ? ORDER BY leaf_id", tier, bucket)
	if err != nil {
		return nil, fmt.Errorf("%w: read bucket: %w", constants.ErrStateRootQueryTable, err)
	}
	defer rows.Close()
	var leaves [][2][]byte
	for rows.Next() {
		var leafID, digest []byte
		if err := rows.Scan(&leafID, &digest); err != nil {
			return nil, fmt.Errorf("%w: scan bucket: %w", constants.ErrStateRootIterateRows, err)
		}
		leaves = append(leaves, [2][]byte{leafID, digest})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: iterate bucket: %w", constants.ErrStateRootIterateRows, err)
	}
	return stateBucketDigest(leaves), nil
}

func readChildDigests(ctx context.Context, conn *sql.Conn, tier string, childLevel, parent int) ([][]byte, error) {
	first := parent * stateTreeFanout
	children := make([][]byte, stateTreeFanout)
	for i := range children {
		children[i] = stateEmptyDigests[childLevel]
	}
	rows, err := conn.QueryContext(ctx,
		"SELECT idx, digest FROM state_nodes WHERE tier = ? AND level = ? AND idx BETWEEN ? AND ?",
		tier, childLevel, first, first+stateTreeFanout-1)
	if err != nil {
		return nil, fmt.Errorf("%w: read child nodes: %w", constants.ErrStateRootQueryTable, err)
	}
	defer rows.Close()
	for rows.Next() {
		var idx int
		var digest []byte
		if err := rows.Scan(&idx, &digest); err != nil {
			return nil, fmt.Errorf("%w: scan child node: %w", constants.ErrStateRootIterateRows, err)
		}
		children[idx-first] = digest
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: iterate child nodes: %w", constants.ErrStateRootIterateRows, err)
	}
	return children, nil
}

// putStateNode stores a node digest, or removes it when the subtree is empty.
func putStateNode(ctx context.Context, conn *sql.Conn, tier string, level, idx int, digest []byte) error {
	var err error
	if bytes.Equal(digest, stateEmptyDigests[level]) {
		_, err = conn.ExecContext(ctx, "DELETE FROM state_nodes WHERE tier = ? AND level = ? AND idx = ?", tier, level, idx)
	} else {
		_, err = conn.ExecContext(ctx,
			`INSERT INTO state_nodes (tier, level, idx, digest) VALUES (?, ?, ?, ?)
			 ON CONFLICT(tier, level, idx) DO UPDATE SET digest = excluded.digest`,
			tier, level, idx, digest)
	}
	if err != nil {
		return fmt.Errorf("%w: write node: %w", constants.ErrStateRootPersist, err)
	}
	return nil
}

// --- Canonical encoding ---

func writeStateField(h hash.Hash, field []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(field)))
	h.Write(length[:])
	h.Write(field)
}

func stateLeafID(source, k1, k2 string) []byte {
	h := sha256.New()
	writeStateField(h, stateTagLeafID)
	writeStateField(h, []byte(source))
	writeStateField(h, []byte(k1))
	writeStateField(h, []byte(k2))
	return h.Sum(nil)
}

func stateBucketOf(leafID []byte) int {
	return int(binary.BigEndian.Uint16(leafID[:2]))
}

func stateLeafDigest(key stateDirtyKey, fields ...[]byte) []byte {
	h := sha256.New()
	writeStateField(h, stateTagLeaf)
	writeStateField(h, []byte(key.source))
	writeStateField(h, []byte(key.k1))
	writeStateField(h, []byte(key.k2))
	for _, f := range fields {
		writeStateField(h, f)
	}
	return h.Sum(nil)
}

// stateBucketDigest commits to (leaf_id, digest) pairs in leaf_id order.
func stateBucketDigest(leaves [][2][]byte) []byte {
	h := sha256.New()
	writeStateField(h, stateTagBucket)
	for _, leaf := range leaves {
		h.Write(leaf[0])
		h.Write(leaf[1])
	}
	return h.Sum(nil)
}

func stateNodeDigest(level int, children [][]byte) []byte {
	h := sha256.New()
	writeStateField(h, stateTagNode)
	h.Write([]byte{byte(level)})
	for _, child := range children {
		h.Write(child)
	}
	return h.Sum(nil)
}

func stateRootDigest(tier string, treeRoot []byte, keymapHash string) string {
	h := sha256.New()
	writeStateField(h, stateTagRoot)
	writeStateField(h, []byte(tier))
	writeStateField(h, treeRoot)
	writeStateField(h, []byte(keymapHash))
	return hex.EncodeToString(h.Sum(nil))
}
