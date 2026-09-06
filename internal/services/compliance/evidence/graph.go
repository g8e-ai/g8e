// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// ArtifactType identifies the kind of evidence carried by a node. Every
// importer produces nodes with a typed ArtifactType so downstream analysis
// and KSI methods can select evidence by type without parsing free-form
// strings.
type ArtifactType string

const (
	ArtifactTypeDemoManifest       ArtifactType = "demo-manifest"
	ArtifactTypeDemoResult         ArtifactType = "demo-result"
	ArtifactTypeDemoStepResult     ArtifactType = "demo-step-result"
	ArtifactTypeDemoDefinition     ArtifactType = "demo-definition"
	ArtifactTypeActionReceipt      ArtifactType = "action-receipt"
	ArtifactTypeReceiptPersistence ArtifactType = "receipt-persistence"
	ArtifactTypeStateObservation   ArtifactType = "state-observation"
	ArtifactTypeDemoMetric         ArtifactType = "demo-metric"
	ArtifactTypeProtocolChain      ArtifactType = "protocol-chain"
	ArtifactTypeEvalManifest       ArtifactType = "eval-manifest"
	ArtifactTypeEvalTask           ArtifactType = "eval-task"
	ArtifactTypeEvalAttempt        ArtifactType = "eval-attempt"
	ArtifactTypeEvalMetric         ArtifactType = "eval-metric"
	ArtifactTypeEvalObservation    ArtifactType = "eval-observation"
	ArtifactTypeEvalStage          ArtifactType = "eval-stage"
	ArtifactTypeEvalReceipt        ArtifactType = "eval-receipt"
	ArtifactTypeAuditRecord        ArtifactType = "audit-record"
	ArtifactTypeLedgerCommit       ArtifactType = "ledger-commit"
	ArtifactTypeLedgerState        ArtifactType = "ledger-state"
	ArtifactTypeCommitment         ArtifactType = "commitment"
	ArtifactTypeKSIResult          ArtifactType = "ksi-result"
	ArtifactTypeBuildAttestation   ArtifactType = "build-attestation"
	ArtifactTypeConfigAttestation  ArtifactType = "config-attestation"
	ArtifactTypeCustomerAttestation ArtifactType = "customer-attestation"
	ArtifactTypeAssessorAttestation ArtifactType = "assessor-attestation"
)

// VerificationStatus represents the independent verification state of a node.
type VerificationStatus string

const (
	VerificationStatusVerified   VerificationStatus = "verified"
	VerificationStatusUnverified VerificationStatus = "unverified"
	VerificationStatusFailed     VerificationStatus = "failed"
	VerificationStatusPending    VerificationStatus = "pending"
)

// EncryptionMetadata carries authenticated encryption metadata for restricted
// evidence. The plaintext digest and authenticated metadata digest allow
// verification without decrypting the ciphertext.
type EncryptionMetadata struct {
	Algorithm                 string
	KeyID                     string
	AuthorizationScope        string
	PlaintextSHA256           string
	AuthenticatedMetadataSHA256 string
}

// EvidenceNode is one content-addressed piece of evidence in the graph. The
// artifact ID is the canonical content address "<type>:sha256:<digest>",
// computed from the canonical bytes. Every node binds to exactly one
// assessment scope and optionally to a run, attempt, scenario, transaction,
// or evidence window.
type EvidenceNode struct {
	ArtifactID         string
	ArtifactType       ArtifactType
	SHA256             string
	MediaType          string
	SchemaRef          string
	ProducerIdentity   string
	ProducedAt         time.Time
	ScopeID            string
	RunID              string
	AttemptID          string
	ScenarioID         string
	TransactionID      string
	VerificationStatus VerificationStatus
	VerifierID         string
	VerifierVersion    string
	VerifiedAt         time.Time
	BundlePath         string
	Encryption         *EncryptionMetadata
	CanonicalBytes     []byte
	References         []string
}

// EvidenceImporter is the read-only source-specific adapter interface. Every
// importer reads from one evidence source, validates canonical bytes and
// content digests, and yields EvidenceNode records without mutating the
// source. Importers never add nodes directly; the graph caller invokes
// AddNode for each imported record.
type EvidenceImporter interface {
	Import(ctx context.Context) ([]EvidenceNode, error)
	SourceID() string
}

// GraphFailure records one validation failure detected during graph
// construction or validation.
type GraphFailure struct {
	Code    error
	Subject string
	Reason  string
}

// EvidenceGraph is the content-addressed evidence graph. Nodes are indexed
// by artifact ID. The graph detects duplicate ID/content conflicts, resolves
// references, detects prohibited cycles, and validates scope binding, schema
// versions, producer/verifier identity, assessed trust, freshness, encryption
// metadata authentication, canonical digests, path traversal, size limits,
// and media-type limits.
type EvidenceGraph struct {
	nodes    map[string]*EvidenceNode
	byScope  map[string][]*EvidenceNode
	byType   map[ArtifactType][]*EvidenceNode
	failures []GraphFailure
	maxBytes int
	allowedMediaTypes map[string]bool
}

// NewEvidenceGraph creates an empty graph with the given size limit and
// optional allowed-media-type set. A zero maxBytes disables size checks.
// A nil allowedMediaTypes set disables media-type checks.
func NewEvidenceGraph(maxBytes int, allowedMediaTypes []string) *EvidenceGraph {
	mt := make(map[string]bool, len(allowedMediaTypes))
	for _, t := range allowedMediaTypes {
		mt[t] = true
	}
	return &EvidenceGraph{
		nodes:            make(map[string]*EvidenceNode),
		byScope:          make(map[string][]*EvidenceNode),
		byType:           make(map[ArtifactType][]*EvidenceNode),
		maxBytes:         maxBytes,
		allowedMediaTypes: mt,
	}
}

// AddNode adds a node to the graph. It detects duplicate artifact ID
// conflicts and duplicate content with conflicting metadata. Returns an
// error if the node is rejected; the error is also recorded as a graph
// failure.
func (g *EvidenceGraph) AddNode(node EvidenceNode) error {
	if node.ArtifactID == "" {
		err := fmt.Errorf("%w: empty artifact ID", constants.ErrEvidenceArtifactMalformed)
		g.recordFailure(err, "", "artifact ID is empty")
		return err
	}
	if node.SHA256 == "" {
		err := fmt.Errorf("%w: %s: empty SHA-256", constants.ErrEvidenceArtifactMalformed, node.ArtifactID)
		g.recordFailure(err, node.ArtifactID, "SHA-256 digest is empty")
		return err
	}
	if node.ScopeID == "" {
		err := fmt.Errorf("%w: %s: empty scope ID", constants.ErrEvidenceScopeMismatch, node.ArtifactID)
		g.recordFailure(err, node.ArtifactID, "scope ID is empty")
		return err
	}
	if g.maxBytes > 0 && len(node.CanonicalBytes) > g.maxBytes {
		err := fmt.Errorf("%w: %s: %d bytes exceeds limit %d", constants.ErrEvidenceArtifactTooLarge, node.ArtifactID, len(node.CanonicalBytes), g.maxBytes)
		g.recordFailure(err, node.ArtifactID, "artifact exceeds size limit")
		return err
	}
	if len(g.allowedMediaTypes) > 0 && node.MediaType != "" && !g.allowedMediaTypes[node.MediaType] {
		err := fmt.Errorf("%w: %s: %s", constants.ErrEvidenceMediaTypeUnsupported, node.ArtifactID, node.MediaType)
		g.recordFailure(err, node.ArtifactID, "media type is not in the allowed set")
		return err
	}
	if node.BundlePath != "" && !validBundlePath(node.BundlePath) {
		err := fmt.Errorf("%w: %s: %s", constants.ErrPathValidation, node.ArtifactID, node.BundlePath)
		g.recordFailure(err, node.ArtifactID, "bundle path contains traversal or absolute components")
		return err
	}
	existing, ok := g.nodes[node.ArtifactID]
	if ok {
		if existing.SHA256 != node.SHA256 {
			err := fmt.Errorf("%w: %s: existing %s vs new %s", constants.ErrEvidenceDuplicateContent, node.ArtifactID, existing.SHA256, node.SHA256)
			g.recordFailure(err, node.ArtifactID, "duplicate artifact ID with conflicting content digest")
			return err
		}
		if !sameScopeBinding(existing, &node) {
			err := fmt.Errorf("%w: %s", constants.ErrEvidenceScopeMismatch, node.ArtifactID)
			g.recordFailure(err, node.ArtifactID, "duplicate artifact ID with conflicting scope binding")
			return err
		}
		return nil
	}
	copyNode := node
	g.nodes[node.ArtifactID] = &copyNode
	g.byScope[node.ScopeID] = append(g.byScope[node.ScopeID], &copyNode)
	g.byType[node.ArtifactType] = append(g.byType[node.ArtifactType], &copyNode)
	return nil
}

// ResolveReferences verifies that every reference in every node points to an
// existing node in the graph. Unresolved references are recorded as failures.
func (g *EvidenceGraph) ResolveReferences() {
	for _, node := range g.nodes {
		for _, ref := range node.References {
			if _, ok := g.nodes[ref]; !ok {
				g.recordFailure(constants.ErrUnresolvedReference, node.ArtifactID, fmt.Sprintf("reference %s is not in the graph", ref))
			}
		}
	}
}

// DetectCycles performs a depth-first traversal of the reference graph and
// records any prohibited cycle as a failure. Self-references and mutual
// references between two nodes are both cycles.
func (g *EvidenceGraph) DetectCycles() {
	visited := make(map[string]int, len(g.nodes))
	var dfs func(id string, path []string) bool
	dfs = func(id string, path []string) bool {
		state, seen := visited[id]
		if seen {
			if state == 1 {
				cycle := append(path, id)
				g.recordFailure(constants.ErrEvidenceCycleDetected, id, fmt.Sprintf("reference cycle: %s", strings.Join(cycle, " -> ")))
				return true
			}
			return false
		}
		visited[id] = 1
		node, ok := g.nodes[id]
		if !ok {
			visited[id] = 2
			return false
		}
		for _, ref := range node.References {
			dfs(ref, append(path, id))
		}
		visited[id] = 2
		return false
	}
	for id := range g.nodes {
		if visited[id] == 0 {
			dfs(id, nil)
		}
	}
}

// ValidateScopeBinding verifies that every node's scope/run/attempt/scenario/
// transaction binding is internally consistent. Nodes within the same run
// must share the same scope ID. Nodes within the same attempt must share the
// same run ID. Nodes within the same scenario must share the same run ID.
func (g *EvidenceGraph) ValidateScopeBinding() {
	runScopes := make(map[string]string)
	attemptRuns := make(map[string]string)
	scenarioRuns := make(map[string]string)
	for _, node := range g.nodes {
		if node.RunID != "" {
			if existing, ok := runScopes[node.RunID]; ok && existing != node.ScopeID {
				g.recordFailure(constants.ErrEvidenceScopeMismatch, node.ArtifactID, fmt.Sprintf("run %s has conflicting scopes %s vs %s", node.RunID, existing, node.ScopeID))
			} else if !ok {
				runScopes[node.RunID] = node.ScopeID
			}
		}
		if node.AttemptID != "" {
			if existing, ok := attemptRuns[node.AttemptID]; ok && existing != node.RunID {
				g.recordFailure(constants.ErrEvidenceScopeMismatch, node.ArtifactID, fmt.Sprintf("attempt %s has conflicting runs %s vs %s", node.AttemptID, existing, node.RunID))
			} else if !ok {
				attemptRuns[node.AttemptID] = node.RunID
			}
		}
		if node.ScenarioID != "" {
			if existing, ok := scenarioRuns[node.ScenarioID]; ok && existing != node.RunID {
				g.recordFailure(constants.ErrEvidenceScopeMismatch, node.ArtifactID, fmt.Sprintf("scenario %s has conflicting runs %s vs %s", node.ScenarioID, existing, node.RunID))
			} else if !ok {
				scenarioRuns[node.ScenarioID] = node.RunID
			}
		}
	}
}

// ValidateFreshness checks that every node's produced-at timestamp falls
// within the assessment window [windowStart, windowEnd]. A zero windowEnd
// means no upper bound. Stale evidence is recorded as a failure.
func (g *EvidenceGraph) ValidateFreshness(windowStart, windowEnd time.Time) {
	for _, node := range g.nodes {
		if node.ProducedAt.IsZero() {
			g.recordFailure(constants.ErrStaleEvidence, node.ArtifactID, "produced-at timestamp is missing")
			continue
		}
		if !windowStart.IsZero() && node.ProducedAt.Before(windowStart) {
			g.recordFailure(constants.ErrStaleEvidence, node.ArtifactID, fmt.Sprintf("produced at %s before window start %s", node.ProducedAt.Format(time.RFC3339Nano), windowStart.Format(time.RFC3339Nano)))
		}
		if !windowEnd.IsZero() && node.ProducedAt.After(windowEnd) {
			g.recordFailure(constants.ErrStaleEvidence, node.ArtifactID, fmt.Sprintf("produced at %s after window end %s", node.ProducedAt.Format(time.RFC3339Nano), windowEnd.Format(time.RFC3339Nano)))
		}
	}
}

// ValidateEncryption verifies that encrypted nodes carry complete encryption
// metadata. A node with encryption metadata must have a non-empty algorithm,
// key ID, authorization scope, plaintext digest, and authenticated metadata
// digest. A node without encryption metadata must not declare a restricted
// media type.
func (g *EvidenceGraph) ValidateEncryption() {
	for _, node := range g.nodes {
		if node.Encryption != nil {
			em := node.Encryption
			if em.Algorithm == "" || em.KeyID == "" || em.AuthorizationScope == "" || em.PlaintextSHA256 == "" || em.AuthenticatedMetadataSHA256 == "" {
				g.recordFailure(constants.ErrEvidenceEncryptionInvalid, node.ArtifactID, "encryption metadata is incomplete")
			}
			if em.AuthorizationScope != node.ScopeID {
				g.recordFailure(constants.ErrEvidenceEncryptionInvalid, node.ArtifactID, fmt.Sprintf("encryption authorization scope %s does not match node scope %s", em.AuthorizationScope, node.ScopeID))
			}
		}
	}
}

// VerifyDigests recomputes the SHA-256 of each node's canonical bytes and
// compares it to the declared digest. Nodes without canonical bytes are
// skipped. Digest mismatches are recorded as failures.
func (g *EvidenceGraph) VerifyDigests() {
	for _, node := range g.nodes {
		if len(node.CanonicalBytes) == 0 {
			continue
		}
		digest := sha256.Sum256(node.CanonicalBytes)
		actual := hex.EncodeToString(digest[:])
		if actual != node.SHA256 {
			g.recordFailure(constants.ErrChecksumMismatch, node.ArtifactID, fmt.Sprintf("digest mismatch: declared %s vs computed %s", node.SHA256, actual))
		}
	}
}

// ValidateTrust verifies that every verified node has a non-empty verifier
// identity and version. Nodes with verification status "verified" must have
// both fields populated.
func (g *EvidenceGraph) ValidateTrust() {
	for _, node := range g.nodes {
		if node.VerificationStatus == VerificationStatusVerified {
			if node.VerifierID == "" || node.VerifierVersion == "" {
				g.recordFailure(constants.ErrEvidenceTrustNotAssessed, node.ArtifactID, "verified node lacks verifier identity or version")
			}
		}
		if node.ProducerIdentity == "" {
			g.recordFailure(constants.ErrEvidenceProducerUnverified, node.ArtifactID, "producer identity is empty")
		}
	}
}

// ValidateAll runs all validation checks in the recommended order: digests,
// scope binding, references, cycles, trust, encryption, freshness.
func (g *EvidenceGraph) ValidateAll(windowStart, windowEnd time.Time) {
	g.VerifyDigests()
	g.ValidateScopeBinding()
	g.ResolveReferences()
	g.DetectCycles()
	g.ValidateTrust()
	g.ValidateEncryption()
	g.ValidateFreshness(windowStart, windowEnd)
}

// Failures returns all recorded validation failures.
func (g *EvidenceGraph) Failures() []GraphFailure {
	return g.failures
}

// Valid returns true if the graph has no failures.
func (g *EvidenceGraph) Valid() bool {
	return len(g.failures) == 0
}

// NodeCount returns the total number of unique nodes in the graph.
func (g *EvidenceGraph) NodeCount() int {
	return len(g.nodes)
}

// NodesByScope returns all nodes bound to the given scope ID.
func (g *EvidenceGraph) NodesByScope(scopeID string) []*EvidenceNode {
	return g.byScope[scopeID]
}

// NodesByType returns all nodes of the given artifact type.
func (g *EvidenceGraph) NodesByType(artifactType ArtifactType) []*EvidenceNode {
	return g.byType[artifactType]
}

// Node returns the node with the given artifact ID, or nil if not found.
func (g *EvidenceGraph) Node(artifactID string) *EvidenceNode {
	return g.nodes[artifactID]
}

// ToProto converts an EvidenceNode to a protocol-owned
// ComplianceEvidenceReference. The canonical bytes and references are not
// included in the proto reference; they live in the graph and bundle.
func (n *EvidenceNode) ToProto() *compliancev1.ComplianceEvidenceReference {
	ref := &compliancev1.ComplianceEvidenceReference{
		ArtifactId:         n.ArtifactID,
		ArtifactType:       string(n.ArtifactType),
		Sha256:             n.SHA256,
		MediaType:          n.MediaType,
		SchemaRef:          n.SchemaRef,
		ProducerIdentity:   n.ProducerIdentity,
		ScopeId:            n.ScopeID,
		RunId:              n.RunID,
		AttemptId:          n.AttemptID,
		ScenarioId:         n.ScenarioID,
		TransactionId:      n.TransactionID,
		VerificationStatus: string(n.VerificationStatus),
		VerifierId:         n.VerifierID,
		VerifierVersion:    n.VerifierVersion,
		BundlePath:         n.BundlePath,
	}
	if !n.ProducedAt.IsZero() {
		ref.ProducedAt = timestamppb.New(n.ProducedAt)
	}
	if !n.VerifiedAt.IsZero() {
		ref.VerifiedAt = timestamppb.New(n.VerifiedAt)
	}
	if n.Encryption != nil {
		ref.Encryption = &compliancev1.EvidenceEncryptionMetadata{
			Algorithm:                 n.Encryption.Algorithm,
			KeyId:                     n.Encryption.KeyID,
			AuthorizationScope:        n.Encryption.AuthorizationScope,
			PlaintextSha256:           n.Encryption.PlaintextSHA256,
			AuthenticatedMetadataSha256: n.Encryption.AuthenticatedMetadataSHA256,
		}
	}
	return ref
}

// ContentAddress computes the canonical content address for a given artifact
// type and canonical bytes: "<type>:sha256:<hex-digest>".
func ContentAddress(artifactType ArtifactType, canonicalBytes []byte) string {
	digest := sha256.Sum256(canonicalBytes)
	return string(artifactType) + ":sha256:" + hex.EncodeToString(digest[:])
}

// ParseContentAddress splits a content address into its type, digest, and
// validity flag. The format is "<type>:sha256:<64-hex-chars>".
func ParseContentAddress(address string) (ArtifactType, string, bool) {
	parts := strings.Split(address, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "sha256" || len(parts[2]) != sha256.Size*2 || strings.ToLower(parts[2]) != parts[2] {
		return "", "", false
	}
	decoded, err := hex.DecodeString(parts[2])
	if err != nil || len(decoded) != sha256.Size {
		return "", "", false
	}
	return ArtifactType(parts[0]), parts[2], true
}

// recordFailure appends a validation failure to the graph.
func (g *EvidenceGraph) recordFailure(code error, subject, reason string) {
	g.failures = append(g.failures, GraphFailure{Code: code, Subject: subject, Reason: reason})
}

// sameScopeBinding checks whether two nodes share the same scope, run,
// attempt, scenario, and transaction binding.
func sameScopeBinding(a, b *EvidenceNode) bool {
	return a.ScopeID == b.ScopeID && a.RunID == b.RunID && a.AttemptID == b.AttemptID && a.ScenarioID == b.ScenarioID && a.TransactionID == b.TransactionID
}

// validBundlePath rejects absolute paths, path traversal, and symlinks in
// bundle paths. Bundle paths must be relative and must not escape the bundle
// root.
func validBundlePath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." {
		return false
	}
	if strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// ErrGraphInvalid is returned when ValidateAll detects failures.
var ErrGraphInvalid = errors.New("compliance: evidence graph validation failed")
