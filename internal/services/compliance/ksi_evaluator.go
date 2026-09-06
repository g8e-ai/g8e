// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"context"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
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

// EvaluatorDeps holds the evidence sources the KSI evaluator reads.
// All fields are required; a nil field causes the dependent methods to
// fail-closed (return false).
type EvaluatorDeps struct {
	Audit       AuditEvidenceReader
	Ledger      LedgerEvidenceReader
	Commitments CommitmentEvidenceReader
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

// RegisterMethods binds automated methods to a KSI ID.
// Returns an error if the KSI ID is not in the catalog.
func (e *KSIEvaluator) RegisterMethods(ksiID string, methods ...KSIMethod) error {
	if e.catalog.FindKSI(ksiID) == nil {
		return fmt.Errorf("compliance: RegisterMethods: unknown KSI ID: %s", ksiID)
	}
	e.methods[ksiID] = append(e.methods[ksiID], methods...)
	return nil
}

// RegisterDefaultMethods registers g8e's built-in automated methods for all
// KSIs that g8e can evaluate. KSIs without automatable methods are left
// unregistered and will fail-closed during Evaluate if the class requires
// automated methods.
func (e *KSIEvaluator) RegisterDefaultMethods(deps EvaluatorDeps) {
	for ksiID, methods := range DefaultMethods(deps) {
		e.methods[ksiID] = methods
	}
}

// MethodCount returns the number of registered automated methods for a KSI.
func (e *KSIEvaluator) MethodCount(ksiID string) int {
	return len(e.methods[ksiID])
}

// Evaluate evaluates all KSIs applicable to the given certification class
// and returns a KSIResultSet. For each KSI:
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
func (e *KSIEvaluator) Evaluate(ctx context.Context, class CertificationClass) (*KSIResultSet, error) {
	ksis := e.catalog.KSIsForClass(class)
	minMethods := MinimumMethodsForClass(class)
	now := time.Now()

	result := &KSIResultSet{
		Class:         class,
		EvaluatedAtMs: now.UnixMilli(),
		Results:       make([]KSIResult, 0, len(ksis)),
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
		}

		registered := e.methods[ksiID]
		res.MethodCount = len(registered)

		if len(registered) < minMethods {
			res.Status = KSIStatusNotSatisfied
			result.Results = append(result.Results, res)
			continue
		}

		if ksi.LastValidatedUnixMs > 0 && ksi.IsStale(now) {
			res.Status = KSIStatusNotSatisfied
			result.Results = append(result.Results, res)
			continue
		}

		allSatisfied := true
		var allEvidence []*compliancev1.ComplianceEvidenceReference
		for _, method := range registered {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			satisfied, evidence, err := method(ctx)
			if err != nil {
				allSatisfied = false
				allEvidence = append(allEvidence, newKSIEvidenceReference(EvidenceTypeExecutionID, ksiID))
				continue
			}
			if !satisfied {
				allSatisfied = false
			}
			allEvidence = append(allEvidence, evidence...)
		}

		if allSatisfied {
			res.Status = KSIStatusSatisfied
		} else {
			res.Status = KSIStatusNotSatisfied
		}
		res.Evidence = allEvidence

		result.Results = append(result.Results, res)
	}

	return result, nil
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
// reference KSIs in the catalog, and the result set is non-empty for a
// non-trivial class.
func (r *KSIResultSet) Validate(catalog *KSICatalog) error {
	if len(r.Results) == 0 {
		return fmt.Errorf("%w: empty result set", constants.ErrValidationFailed)
	}
	for i, res := range r.Results {
		if res.ID == "" {
			return fmt.Errorf("%w: result at index %d has empty ID", constants.ErrValidationFailed, i)
		}
		if catalog.FindKSI(res.ID) == nil {
			return fmt.Errorf("%w: result at index %d references unknown KSI: %s", constants.ErrValidationFailed, i, res.ID)
		}
	}
	return nil
}

func newKSIEvidenceReference(evidenceType EvidenceType, artifactID string) *compliancev1.ComplianceEvidenceReference {
	return &compliancev1.ComplianceEvidenceReference{ArtifactId: artifactID, ArtifactType: string(evidenceType)}
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

	receiptsHaveSignatures := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
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
		for _, r := range receipts {
			if r.Signature == "" || r.SignerKeyID == "" {
				return false, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeReceiptID, r.TransactionID)}, nil
			}
		}
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeReceiptID, receipts[0].TransactionID)}, nil
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

	merkleRootExists := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if deps.Ledger == nil {
			return false, nil, nil
		}
		root, err := deps.Ledger.GetStateMerkleRoot()
		if err != nil {
			return false, nil, err
		}
		if root == "" {
			return false, nil, nil
		}
		return true, []*compliancev1.ComplianceEvidenceReference{newKSIEvidenceReference(EvidenceTypeMerkleRoot, root)}, nil
	}

	ledgerCommitsExist := func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error) {
		if deps.Ledger == nil {
			return false, nil, nil
		}
		commits, err := deps.Ledger.ListCommits("", 1)
		if err != nil {
			return false, nil, err
		}
		if len(commits) == 0 {
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

	// Bind methods to KSIs g8e can evaluate.
	// Each KSI gets >=2 methods for Class C compliance.

	methods["KSI-CMT-01"] = []KSIMethod{auditEventsExist, ledgerCommitsExist}
	methods["KSI-CMT-03"] = []KSIMethod{ledgerCommitsExist, receiptsExist}
	methods["KSI-CNA-01"] = []KSIMethod{receiptsExist, auditEventsExist}
	methods["KSI-IAM-05"] = []KSIMethod{receiptsExist, auditEventsExist}
	methods["KSI-IAM-07"] = []KSIMethod{fileMutationsTracked, receiptsExist}
	methods["KSI-MLA-03"] = []KSIMethod{auditEventsExist, receiptsExist}
	methods["KSI-MLA-07"] = []KSIMethod{commitmentChainExists, merkleRootExists}
	methods["KSI-MLA-08"] = []KSIMethod{receiptsHaveSignatures, commitmentChainExists}
	methods["KSI-SVC-04"] = []KSIMethod{fileMutationsTracked, ledgerCommitsExist}
	methods["KSI-SVC-05"] = []KSIMethod{merkleRootExists, commitmentChainIntact}

	return methods
}
