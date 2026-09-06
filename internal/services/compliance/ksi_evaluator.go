// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// AuditEvidenceReader provides read-only access to audit store evidence
// for KSI evaluation. SQLAuditStore satisfies this interface.
type AuditEvidenceReader interface {
	ListActionReceipts(operatorSessionID string, limit, offset int) ([]*models.ActionReceiptRecord, error)
	ListEvents(sessionID string, limit, offset int) ([]*storage.Event, error)
	ListFileMutations(limit, offset int) ([]*storage.FileMutationLog, error)
}

// LedgerEvidenceReader provides read-only access to ledger evidence
// for KSI evaluation. GitLedgerService satisfies this interface.
type LedgerEvidenceReader interface {
	GetStateMerkleRoot() (string, error)
	ListCommits(sessionID string, limit int) ([]storage.LedgerCommit, error)
}

// CommitmentEvidenceReader provides read-only access to commitment ledger
// evidence for KSI evaluation. CommitmentLedger satisfies this interface.
type CommitmentEvidenceReader interface {
	ListCommitments() ([]*storage.CommitmentRow, error)
}

type KSIHistoryReader interface {
	ListSnapshots(ctx context.Context) ([]KSIResultSet, error)
}

type GraderResult struct {
	ArtifactID    string
	GraderID      string
	GraderVersion string
	SHA256        string
	Verified      bool
	ProducedAt    time.Time
	Evidence      []*compliancev1.ComplianceEvidenceReference
}

type EvalGraderReader interface {
	ListGraderResults(ctx context.Context) ([]GraderResult, error)
}

// EvaluatorDeps holds the evidence sources the KSI evaluator reads.
// All fields are required; a nil field causes the dependent methods to
// fail-closed (return false).
type EvaluatorDeps struct {
	Audit       AuditEvidenceReader
	Ledger      LedgerEvidenceReader
	Commitments CommitmentEvidenceReader
	History     KSIHistoryReader
	Graders     EvalGraderReader
}

// KSIEvaluator evaluates KSIs against live g8e state by running registered
// automated methods per KSI and collecting evidence.
type KSIEvaluator struct {
	catalog *KSICatalog
	methods map[string][]KSIMethod
}

// NewKSIEvaluator creates a new evaluator for the given catalog.
// Methods must be registered via RegisterMethods or RegisterDefaultMethods
// before calling Evaluate.
func NewKSIEvaluator(catalog *KSICatalog) *KSIEvaluator {
	return &KSIEvaluator{
		catalog: catalog,
		methods: make(map[string][]KSIMethod),
	}
}

// RegisterMethods binds independently measured automated methods to a KSI ID.
func (e *KSIEvaluator) RegisterMethods(ksiID string, methods ...KSIMethod) error {
	if e.catalog == nil || e.catalog.FindKSI(ksiID) == nil {
		return fmt.Errorf("%w: unknown KSI ID: %s", constants.ErrKSICatalogInvalid, ksiID)
	}

	independenceKeys := make(map[string]struct{}, len(e.methods[ksiID])+len(methods))
	methodIdentities := make(map[string]struct{}, len(e.methods[ksiID])+len(methods))
	for _, method := range e.methods[ksiID] {
		independenceKeys[method.independenceKey()] = struct{}{}
		methodIdentities[method.Name+"\x00"+method.Version] = struct{}{}
	}
	for _, method := range methods {
		if err := method.validate(); err != nil {
			return fmt.Errorf("compliance: register KSI method %q: %w", method.Name, err)
		}
		key := method.independenceKey()
		if _, exists := independenceKeys[key]; exists {
			return fmt.Errorf("%w: KSI %s method %s restates artifact %s at %s with verifier %s measuring %s", constants.ErrKSIMethodNotIndependent, ksiID, method.Name, method.ArtifactIdentity, method.CollectionBoundary, method.VerifierFamily, method.MeasuredProperty)
		}
		identity := method.Name + "\x00" + method.Version
		if _, exists := methodIdentities[identity]; exists {
			return fmt.Errorf("%w: KSI %s repeats method identity %s@%s", constants.ErrKSIMethodInvalid, ksiID, method.Name, method.Version)
		}
		methodIdentities[identity] = struct{}{}
		independenceKeys[key] = struct{}{}
	}
	e.methods[ksiID] = append(e.methods[ksiID], methods...)
	return nil
}

// RegisterDefaultMethods registers g8e's built-in automated methods for all
// KSIs that g8e can evaluate. KSIs without automatable methods are left
// unregistered and will fail-closed during Evaluate if the class requires
// automated methods.
func (e *KSIEvaluator) RegisterDefaultMethods(deps EvaluatorDeps) error {
	if e.catalog == nil {
		return fmt.Errorf("%w: catalog is nil", constants.ErrKSICatalogInvalid)
	}
	for ksiID, methods := range DefaultMethods(deps) {
		if e.catalog.FindKSI(ksiID) == nil {
			continue
		}
		if err := e.RegisterMethods(ksiID, methods...); err != nil {
			return fmt.Errorf("compliance: register default methods for %s: %w", ksiID, err)
		}
	}
	return nil
}

// MethodCount returns the number of registered automated methods for a KSI.
func (e *KSIEvaluator) MethodCount(ksiID string) int {
	return len(e.methods[ksiID])
}

// Evaluate evaluates all KSIs applicable to the given certification class
// and returns a KSIResultSet bound to the given EvaluationBinding. For each KSI:
//
//  1. Runs all registered methods, collecting evidence.
//  2. A KSI is satisfied only if ALL methods return true.
//  3. Fails-closed (not_satisfied) if the method count is below the class
//     minimum (Class C: >=2, Class B: >=1).
//  4. Fails-closed if any method returns an error.
//  5. Marks not_applicable if the KSI has no applicable classes for the
//     target class (should not occur if KSIsForClass is used).
//  6. Checks staleness: a KSI whose LastValidatedUnixMs exceeds its validation
//     cycle is marked not_satisfied regardless of method results.
//  7. Stamps every result and evidence reference with the binding scope, run,
//     and evidence window so consumers can prove no result was produced from
//     evidence outside the declared assessment context.
func (e *KSIEvaluator) Evaluate(ctx context.Context, class CertificationClass, binding EvaluationBinding) (*KSIResultSet, error) {
	if err := binding.Validate(); err != nil {
		return nil, fmt.Errorf("compliance: evaluate: %w", err)
	}
	ksis := e.catalog.KSIsForClass(class)
	minMethods := MinimumMethodsForClass(class)
	now := time.Now()

	result := &KSIResultSet{
		Class:         class,
		EvaluatedAtMs: now.UnixMilli(),
		Results:       make([]KSIResult, 0, len(ksis)),
		Binding:       binding,
	}

	for i := range ksis {
		ksi := &ksis[i]
		ksiID := ksi.ID

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		res := KSIResult{
			ID:                  ksiID,
			LastValidatedUnixMs: now.UnixMilli(),
			Binding:             binding,
		}

		registered := e.methods[ksiID]
		res.MethodCount = len(registered)

		if len(registered) < minMethods {
			res.Outcome = KSIOutcomeUnsupportedAutomation
			res.Status = res.Outcome.Status()
			result.Results = append(result.Results, res)
			continue
		}

		if ksi.LastValidatedUnixMs > 0 && ksi.IsStale(now) {
			res.Outcome = KSIOutcomeStaleEvidence
			res.Status = res.Outcome.Status()
			result.Results = append(result.Results, res)
			continue
		}

		res.Outcome = KSIOutcomeSatisfied
		var allEvidence []*compliancev1.ComplianceEvidenceReference
		for _, method := range registered {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			satisfied, evidence, err := method.evaluate(ctx)
			if err != nil {
				res.Outcome = higherPriorityKSIOutcome(res.Outcome, KSIOutcomeMethodFailure)
				allEvidence = append(allEvidence, stampEvidenceBinding(newKSIEvidenceReference(EvidenceTypeExecutionID, ksiID), binding))
				continue
			}
			if !satisfied {
				res.Outcome = higherPriorityKSIOutcome(res.Outcome, method.UnsatisfiedOutcome)
			}
			for _, ref := range evidence {
				allEvidence = append(allEvidence, stampEvidenceBinding(ref, binding))
			}
		}

		res.Status = res.Outcome.Status()
		res.Evidence = allEvidence

		result.Results = append(result.Results, res)
	}

	return result, nil
}

func higherPriorityKSIOutcome(current, candidate KSIOutcome) KSIOutcome {
	if ksiOutcomePriority(candidate) > ksiOutcomePriority(current) {
		return candidate
	}
	return current
}

func ksiOutcomePriority(outcome KSIOutcome) int {
	switch outcome {
	case KSIOutcomeMethodFailure:
		return 5
	case KSIOutcomeStaleEvidence:
		return 4
	case KSIOutcomeInvalidEvidence:
		return 3
	case KSIOutcomeCustomerAttestationRequired:
		return 2
	case KSIOutcomeUnsupportedAutomation:
		return 1
	default:
		return 0
	}
}

// HasFailures returns true if any KSI in the result set is not_satisfied.
func (r *KSIResultSet) HasFailures() bool {
	for _, res := range r.Results {
		if res.Status == KSIStatusNotSatisfied {
			return true
		}
	}
	return false
}

// SatisfiedCount returns the number of satisfied KSIs in the result set.
func (r *KSIResultSet) SatisfiedCount() int {
	count := 0
	for _, res := range r.Results {
		if res.Status == KSIStatusSatisfied {
			count++
		}
	}
	return count
}

// NotSatisfiedCount returns the number of not_satisfied KSIs in the result set.
func (r *KSIResultSet) NotSatisfiedCount() int {
	count := 0
	for _, res := range r.Results {
		if res.Status == KSIStatusNotSatisfied {
			count++
		}
	}
	return count
}

// Validate checks that the result set is internally consistent: all KSI IDs
// reference KSIs in the catalog, the result set is non-empty for a non-trivial
// class, the result-set binding is populated, every result binding matches the
// result-set binding, and every evidence reference carries the same scope and
// run binding as the result that produced it.
func (r *KSIResultSet) Validate(catalog *KSICatalog) error {
	if len(r.Results) == 0 {
		return fmt.Errorf("%w: empty result set", constants.ErrValidationFailed)
	}
	if err := r.Binding.Validate(); err != nil {
		return fmt.Errorf("%w: result set binding: %w", constants.ErrValidationFailed, err)
	}
	for i, res := range r.Results {
		if res.ID == "" {
			return fmt.Errorf("%w: result at index %d has empty ID", constants.ErrValidationFailed, i)
		}
		if catalog.FindKSI(res.ID) == nil {
			return fmt.Errorf("%w: result at index %d references unknown KSI: %s", constants.ErrValidationFailed, i, res.ID)
		}
		if !res.Outcome.Valid() || res.Status != res.Outcome.Status() {
			return fmt.Errorf("%w: result at index %d has inconsistent status and outcome", constants.ErrValidationFailed, i)
		}
		if err := res.Binding.Validate(); err != nil {
			return fmt.Errorf("%w: result %s binding: %w", constants.ErrValidationFailed, res.ID, err)
		}
		if res.Binding.ScopeID != r.Binding.ScopeID || res.Binding.RunID != r.Binding.RunID || res.Binding.WindowStartUnixMs != r.Binding.WindowStartUnixMs || res.Binding.WindowEndUnixMs != r.Binding.WindowEndUnixMs || res.Binding.EvaluatorID != r.Binding.EvaluatorID || res.Binding.EvaluatorVersion != r.Binding.EvaluatorVersion || res.Binding.MethodDefinitionID != r.Binding.MethodDefinitionID {
			return fmt.Errorf("%w: result %s binding does not match result set binding", constants.ErrKSIBindingMismatch, res.ID)
		}
		for j, evidence := range res.Evidence {
			if evidence == nil {
				return fmt.Errorf("%w: result %s evidence at index %d is nil", constants.ErrValidationFailed, res.ID, j)
			}
			if evidence.GetScopeId() != "" && evidence.GetScopeId() != r.Binding.ScopeID {
				return fmt.Errorf("%w: result %s evidence %s scope %s does not match binding scope %s", constants.ErrKSIBindingMismatch, res.ID, evidence.GetArtifactId(), evidence.GetScopeId(), r.Binding.ScopeID)
			}
			if evidence.GetRunId() != "" && evidence.GetRunId() != r.Binding.RunID {
				return fmt.Errorf("%w: result %s evidence %s run %s does not match binding run %s", constants.ErrKSIBindingMismatch, res.ID, evidence.GetArtifactId(), evidence.GetRunId(), r.Binding.RunID)
			}
		}
	}
	return nil
}

func newKSIEvidenceReference(evidenceType EvidenceType, artifactID string) *compliancev1.ComplianceEvidenceReference {
	return &compliancev1.ComplianceEvidenceReference{ArtifactId: artifactID, ArtifactType: string(evidenceType)}
}

// stampEvidenceBinding writes the binding scope and run onto an evidence
// reference when those fields are empty. References that already declare a
// scope or run are left untouched so independently scoped evidence (e.g.
// imported from another source) is not silently re-bound. The evidence window
// is not stamped on individual references because the window is a property of
// the result set, not of individual evidence artifacts; consumers verify
// produced_at falls inside the window through KSIResultSet.Validate.
func stampEvidenceBinding(ref *compliancev1.ComplianceEvidenceReference, binding EvaluationBinding) *compliancev1.ComplianceEvidenceReference {
	if ref == nil {
		return nil
	}
	if ref.GetScopeId() == "" {
		ref.ScopeId = binding.ScopeID
	}
	if ref.GetRunId() == "" {
		ref.RunId = binding.RunID
	}
	return ref
}

func newVerifiedKSIEvidenceReference(evidenceType EvidenceType, artifactID, digest string, producedAt time.Time) *compliancev1.ComplianceEvidenceReference {
	now := time.Now().UTC()
	return &compliancev1.ComplianceEvidenceReference{
		ArtifactId: artifactID, ArtifactType: string(evidenceType), Sha256: digest,
		ProducedAt: timestamppb.New(producedAt), VerificationStatus: "verified",
		VerifierId: constants.KSIMethodVerifierID, VerifierVersion: constants.KSIMethodVerifierVersion,
		VerifiedAt: timestamppb.New(now),
	}
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

// DefaultMethods returns g8e's built-in automated KSIMethod closures for
// KSIs that g8e can evaluate from its audit store, ledger, and commitment
// ledger. KSIs not automatable by g8e (e.g. training-related CED KSIs) are
// omitted and will fail-closed during evaluation if the class requires
// automated methods.
func DefaultMethods(deps EvaluatorDeps) map[string][]KSIMethod {
	methods := make(map[string][]KSIMethod)

	// Reusable method factories.

	auditEventsExist := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if deps.Audit == nil {
			return false, nil, nil
		}
		events, err := deps.Audit.ListEvents("", 1, 0)
		if err != nil {
			return false, nil, err
		}
		if len(events) == 0 {
			return false, nil, nil
		}
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeExecutionID, fmt.Sprintf("events:%d", len(events)))}, nil
	}

	receiptsExist := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if deps.Audit == nil {
			return false, nil, nil
		}
		receipts, err := deps.Audit.ListActionReceipts("", 1, 0)
		if err != nil {
			return false, nil, err
		}
		if len(receipts) == 0 {
			return false, nil, nil
		}
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeReceiptID, receipts[0].TransactionID)}, nil
	}

	receiptsCryptographicallyVerified := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if deps.Audit == nil {
			return false, nil, nil
		}
		receipts, err := deps.Audit.ListActionReceipts("", 10, 0)
		if err != nil {
			return false, nil, err
		}
		if len(receipts) == 0 {
			return false, nil, nil
		}
		evidence := make([]*compliancev1.ComplianceEvidenceReference, 0, len(receipts))
		for _, record := range receipts {
			if err := ctx.Err(); err != nil {
				return false, evidence, err
			}
			if record == nil {
				return false, evidence, nil
			}
			reference := newKSIEvidenceReference(EvidenceTypeReceiptID, record.TransactionID)
			evidence = append(evidence, reference)
			receipt := record.ActionReceipt
			if receipt == nil || receipt.GetTransactionId() != record.TransactionID || receipt.GetSignerKeyId() != record.SignerKeyID || receipt.GetSignature() != record.Signature {
				return false, evidence, nil
			}
			publicKey, err := governance.SignerPublicKey(receipt.GetSignerKeyId())
			if err != nil {
				return false, evidence, nil
			}
			if governance.VerifyActionReceiptSignature(receipt, publicKey) != nil || governance.VerifyReceiptPersistenceAttestation(receipt, publicKey) != nil {
				return false, evidence, nil
			}
		}
		return true, evidence, nil
	}

	fileMutationsTracked := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if deps.Audit == nil {
			return false, nil, nil
		}
		mutations, err := deps.Audit.ListFileMutations(1, 0)
		if err != nil {
			return false, nil, err
		}
		if len(mutations) == 0 {
			return false, nil, nil
		}
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeExecutionID, fmt.Sprintf("mutations:%d", len(mutations)))}, nil
	}

	ledgerCommitsExist := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if deps.Ledger == nil {
			return false, nil, nil
		}
		commits, err := deps.Ledger.ListCommits("", 1)
		if err != nil {
			return false, nil, err
		}
		if len(commits) != 1 || commits[0].CommitHash == "" || commits[0].ParentHash == "" {
			return false, nil, nil
		}
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeLedgerCommit, commits[0].CommitHash)}, nil
	}

	commitmentChainExists := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if deps.Commitments == nil {
			return false, nil, nil
		}
		commitments, err := deps.Commitments.ListCommitments()
		if err != nil {
			return false, nil, err
		}
		if len(commitments) == 0 {
			return false, nil, nil
		}
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeLedgerCommit, commitments[0].Hash)}, nil
	}

	commitmentChainIntact := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if deps.Commitments == nil {
			return false, nil, nil
		}
		commitments, err := deps.Commitments.ListCommitments()
		if err != nil {
			return false, nil, err
		}
		if len(commitments) == 0 {
			return false, nil, nil
		}
		for i, c := range commitments {
			if i == 0 {
				if c.PriorCommitmentHash != "" {
					return false, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeLedgerCommit, c.Hash)}, nil
				}
				continue
			}
			if c.PriorCommitmentHash != commitments[i-1].Hash {
				return false, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeLedgerCommit, c.Hash)}, nil
			}
		}
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeLedgerCommit, commitments[len(commitments)-1].Hash)}, nil
	}

	auditEventsExistMethod := newKSIMethod("auditEventsExist", KSIArtifactAuditEvents, KSICollectionAuditStore, KSIVerifierExistence, KSIPropertyPresence, auditEventsExist)
	receiptsExistMethod := newKSIMethod("receiptsExist", KSIArtifactActionReceipts, KSICollectionAuditStore, KSIVerifierExistence, KSIPropertyPresence, receiptsExist)
	receiptsCryptographicallyVerifiedMethod := newKSIMethod("receiptsCryptographicallyVerified", KSIArtifactActionReceipts, KSICollectionAuditStore, KSIVerifierCryptographic, KSIPropertyReceiptPersistenceIntegrity, receiptsCryptographicallyVerified)
	fileMutationsTrackedMethod := newKSIMethod("fileMutationsTracked", KSIArtifactFileMutations, KSICollectionAuditStore, KSIVerifierExistence, KSIPropertyPresence, fileMutationsTracked)
	ledgerCommitsExistMethod := newKSIMethod("ledgerCommitsExist", KSIArtifactLedgerCommits, KSICollectionLedgerStore, KSIVerifierExistence, KSIPropertyPresence, ledgerCommitsExist)
	commitmentChainExistsMethod := newKSIMethod("commitmentChainExists", KSIArtifactCommitments, KSICollectionCommitmentStore, KSIVerifierExistence, KSIPropertyPresence, commitmentChainExists)
	commitmentChainIntactMethod := newKSIMethod("commitmentChainIntact", KSIArtifactCommitments, KSICollectionCommitmentStore, KSIVerifierStructural, KSIPropertyChainLinkage, commitmentChainIntact)
	commitmentsCryptographicallyVerified := newCommitmentsCryptographicallyVerifiedMethod(deps.Commitments)
	ledgerMerkleRootMatchesHead := newLedgerMerkleRootMatchesHeadMethod(deps.Ledger)
	independentStateObserved := newIndependentStateObservedMethod(deps.Audit)
	deterministicGraderResultsVerified := newDeterministicGraderResultsVerifiedMethod(deps.Graders)

	// Bind methods to KSIs g8e can evaluate.
	// Each KSI gets >=2 methods for Class C compliance.

	methods["KSI-CMT-01"] = []KSIMethod{auditEventsExistMethod, newKSIHistoryFreshnessMethod(deps.History, "KSI-CMT-01", 7*24*time.Hour)}
	methods["KSI-CMT-03"] = []KSIMethod{ledgerCommitsExistMethod, ledgerMerkleRootMatchesHead}
	methods["KSI-CNA-01"] = []KSIMethod{receiptsExistMethod, independentStateObserved}
	methods["KSI-IAM-05"] = []KSIMethod{receiptsExistMethod, deterministicGraderResultsVerified}
	methods["KSI-IAM-07"] = []KSIMethod{fileMutationsTrackedMethod, independentStateObserved}
	methods["KSI-MLA-03"] = []KSIMethod{auditEventsExistMethod, deterministicGraderResultsVerified}
	methods["KSI-MLA-07"] = []KSIMethod{commitmentChainExistsMethod, commitmentsCryptographicallyVerified}
	methods["KSI-MLA-08"] = []KSIMethod{receiptsCryptographicallyVerifiedMethod, newKSIHistoryFreshnessMethod(deps.History, "KSI-MLA-08", 7*24*time.Hour)}
	methods["KSI-SVC-04"] = []KSIMethod{fileMutationsTrackedMethod, deterministicGraderResultsVerified}
	methods["KSI-SVC-05"] = []KSIMethod{commitmentChainIntactMethod, ledgerMerkleRootMatchesHead}

	return methods
}

func newCommitmentsCryptographicallyVerifiedMethod(reader CommitmentEvidenceReader) KSIMethod {
	return newKSIMethod("commitmentsCryptographicallyVerified", KSIArtifactCommitments, KSICollectionCommitmentStore, KSIVerifierCryptographic, KSIPropertySignatureValidity, func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if reader == nil {
			return false, nil, nil
		}
		commitments, err := reader.ListCommitments()
		if err != nil {
			return false, nil, err
		}
		if len(commitments) == 0 {
			return false, nil, nil
		}
		evidence := make([]*compliancev1.ComplianceEvidenceReference, 0, len(commitments))
		for _, row := range commitments {
			if err := ctx.Err(); err != nil {
				return false, evidence, err
			}
			if row == nil {
				return false, evidence, nil
			}
			reference := newKSIEvidenceReference(EvidenceTypeCommitmentSignature, row.Hash)
			reference.Sha256 = row.Hash
			reference.ProducedAt = timestamppb.New(row.CommittedAt)
			reference.VerificationStatus = "failed"
			reference.VerifierId = constants.KSIMethodVerifierID
			reference.VerifierVersion = constants.KSIMethodVerifierVersion
			evidence = append(evidence, reference)
			var attestation operatorv1.CommitmentAttestation
			if json.Unmarshal(row.AttestationJSON, &attestation) != nil {
				return false, evidence, nil
			}
			canonical, err := governance.CanonicalizeCommitmentAttestation(&attestation)
			if err != nil {
				return false, evidence, nil
			}
			digest := sha256.Sum256(canonical)
			if !commitmentRowMatchesAttestation(row, &attestation, hex.EncodeToString(digest[:])) {
				return false, evidence, nil
			}
			publicKey, err := governance.SignerPublicKey(row.AuditorKeyID)
			if err != nil {
				return false, evidence, nil
			}
			signature, err := hex.DecodeString(row.Signature)
			if err != nil || !ed25519.Verify(publicKey, canonical, signature) {
				return false, evidence, nil
			}
			reference.VerificationStatus = "verified"
			reference.VerifiedAt = timestamppb.Now()
		}
		return true, evidence, nil
	})
}

func commitmentRowMatchesAttestation(row *storage.CommitmentRow, attestation *operatorv1.CommitmentAttestation, digest string) bool {
	return digest == row.Hash && attestation.GetHash() == row.Hash &&
		attestation.GetTransactionId() == row.TransactionID && attestation.GetTransactionHash() == row.TransactionHash &&
		attestation.GetPriorCommitmentHash() == row.PriorCommitmentHash && attestation.GetStateRootAtCommit() == row.StateRootAtCommit &&
		attestation.GetL2SignatureDigest() == row.L2SignatureDigest && attestation.GetWardenIntentSignatureDigest() == row.WardenIntentSignatureDigest &&
		attestation.GetHumanSignatureDigest() == row.HumanSignatureDigest && attestation.GetActionType() == row.ActionType &&
		attestation.GetTargetResource() == row.TargetResource && attestation.GetCommittedAtUnixMs() == row.CommittedAt.UnixMilli() &&
		attestation.GetAuditorKeyId() == row.AuditorKeyID && attestation.GetSignature() == row.Signature
}

func newLedgerMerkleRootMatchesHeadMethod(reader LedgerEvidenceReader) KSIMethod {
	return newKSIMethod("ledgerMerkleRootMatchesHead", KSIArtifactLedgerStateRoot, KSICollectionLedgerStore, KSIVerifierStateObservation, KSIPropertyStateRootMatchesHead, func(_ context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if reader == nil {
			return false, nil, nil
		}
		root, err := reader.GetStateMerkleRoot()
		if err != nil {
			return false, nil, err
		}
		commits, err := reader.ListCommits("", 1)
		if err != nil {
			return false, nil, err
		}
		if root == "" || len(commits) != 1 || commits[0].CommitHash == "" || commits[0].ParentHash == "" || root != commits[0].CommitHash {
			return false, nil, nil
		}
		return true, []*compliancev1.ComplianceEvidenceReference{newVerifiedKSIEvidenceReference(EvidenceTypeStateObservation, root, "", commits[0].TimestampUTC)}, nil
	})
}

func newIndependentStateObservedMethod(reader AuditEvidenceReader) KSIMethod {
	return newKSIMethod("independentStateObserved", KSIArtifactReceiptStateTransitions, KSICollectionAuditStore, KSIVerifierStateObservation, KSIPropertyStateTransitionBinding, func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if reader == nil {
			return false, nil, nil
		}
		receipts, err := reader.ListActionReceipts("", 10, 0)
		if err != nil {
			return false, nil, err
		}
		if len(receipts) == 0 {
			return false, nil, nil
		}
		evidence := make([]*compliancev1.ComplianceEvidenceReference, 0, len(receipts))
		for _, record := range receipts {
			if err := ctx.Err(); err != nil {
				return false, evidence, err
			}
			if !receiptDeclaresStateTransition(record) {
				return false, evidence, nil
			}
			evidence = append(evidence, newVerifiedKSIEvidenceReference(EvidenceTypeStateObservation, record.TransactionID, "", record.ExecutedAt))
		}
		return true, evidence, nil
	})
}

func receiptDeclaresStateTransition(record *models.ActionReceiptRecord) bool {
	return record != nil && record.TransactionID != "" && record.StateRootBefore != "" && record.StateRootAfter != "" &&
		record.StateRootBefore != record.StateRootAfter && record.ActionReceipt != nil &&
		record.ActionReceipt.GetTransactionId() == record.TransactionID && record.ActionReceipt.GetStateRootBefore() == record.StateRootBefore &&
		record.ActionReceipt.GetStateRootAfter() == record.StateRootAfter
}

func newDeterministicGraderResultsVerifiedMethod(reader EvalGraderReader) KSIMethod {
	return newKSIMethod("deterministicGraderResultsVerified", KSIArtifactGraderResults, KSICollectionEvalResults, KSIVerifierDeterministicGrader, KSIPropertyEvidenceContentAddressing, func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if reader == nil {
			return false, nil, nil
		}
		results, err := reader.ListGraderResults(ctx)
		if err != nil {
			return false, nil, err
		}
		if len(results) == 0 {
			return false, nil, nil
		}
		evidence := make([]*compliancev1.ComplianceEvidenceReference, 0, len(results))
		for _, result := range results {
			if err := ctx.Err(); err != nil {
				return false, evidence, err
			}
			if !validGraderResult(result) {
				return false, evidence, nil
			}
			reference := newVerifiedKSIEvidenceReference(EvidenceTypeGraderResult, result.ArtifactID, result.SHA256, result.ProducedAt)
			reference.ProducerIdentity = result.GraderID + "@" + result.GraderVersion
			evidence = append(evidence, reference)
		}
		return true, evidence, nil
	})
}

func validGraderResult(result GraderResult) bool {
	if result.ArtifactID == "" || result.GraderID == "" || result.GraderVersion == "" || !validSHA256(result.SHA256) || !result.Verified || result.ProducedAt.IsZero() || len(result.Evidence) == 0 {
		return false
	}
	for _, source := range result.Evidence {
		if source == nil || source.GetArtifactId() == "" || !validSHA256(source.GetSha256()) {
			return false
		}
	}
	return true
}

func newKSIHistoryFreshnessMethod(reader KSIHistoryReader, ksiID string, cycle time.Duration) KSIMethod {
	method := newKSIMethod("ksiHistoryFreshness", KSIArtifactHistorySnapshots, KSICollectionHistoryStore, KSIVerifierHistorical, KSIPropertyFreshness, func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if reader == nil {
			return false, nil, nil
		}
		snapshots, err := reader.ListSnapshots(ctx)
		if err != nil {
			return false, nil, err
		}
		latest := latestKSIResultTimestamp(snapshots, ksiID)
		now := time.Now().UTC()
		if latest <= 0 {
			return false, nil, nil
		}
		producedAt := time.UnixMilli(latest).UTC()
		if producedAt.After(now) || now.Sub(producedAt) > cycle {
			return false, nil, nil
		}
		artifactID := fmt.Sprintf("%s:%d", ksiID, latest)
		return true, []*compliancev1.ComplianceEvidenceReference{newVerifiedKSIEvidenceReference(EvidenceTypeHistoricalFreshness, artifactID, "", producedAt)}, nil
	})
	method.UnsatisfiedOutcome = KSIOutcomeStaleEvidence
	return method
}

func latestKSIResultTimestamp(snapshots []KSIResultSet, ksiID string) int64 {
	var latest int64
	for _, snapshot := range snapshots {
		for _, result := range snapshot.Results {
			if result.ID == ksiID && snapshot.EvaluatedAtMs > latest {
				latest = snapshot.EvaluatedAtMs
			}
		}
	}
	return latest
}
