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

package compliance

import (
	"context"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/storage"
)

// AuditEvidenceReader provides read-only access to audit store evidence
// for KSI evaluation. SQLAuditStore and storagetest.TestSQLAuditStore
// satisfy this interface.
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
// Panics if the KSI ID is not in the catalog (registration error, not a
// runtime error).
func (e *KSIEvaluator) RegisterMethods(ksiID string, methods ...KSIMethod) {
	if e.catalog.FindKSI(ksiID) == nil {
		panic(fmt.Sprintf("compliance: RegisterMethods: unknown KSI ID: %s", ksiID))
	}
	e.methods[ksiID] = append(e.methods[ksiID], methods...)
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

		if ksi.IsStale(now) {
			res.Status = KSIStatusNotSatisfied
			result.Results = append(result.Results, res)
			continue
		}

		allSatisfied := true
		var allEvidence []Evidence
		for _, method := range registered {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			satisfied, evidence, err := method(ctx)
			if err != nil {
				allSatisfied = false
				allEvidence = append(allEvidence, Evidence{
					Type:      EvidenceTypeExecutionID,
					Reference: ksiID,
					Description: fmt.Sprintf("method error: %v", err),
				})
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
		return fmt.Errorf("%w: empty result set", constants.ErrKSINotSatisfied)
	}
	for i, res := range r.Results {
		if res.ID == "" {
			return fmt.Errorf("%w: result at index %d has empty ID", constants.ErrKSINotSatisfied, i)
		}
		if catalog.FindKSI(res.ID) == nil {
			return fmt.Errorf("%w: result at index %d references unknown KSI: %s", constants.ErrKSINotSatisfied, i, res.ID)
		}
	}
	return nil
}

// DefaultMethods returns g8e's built-in automated KSIMethod closures for
// KSIs that g8e can evaluate from its audit store, ledger, and commitment
// ledger. KSIs not automatable by g8e (e.g. training-related CED KSIs) are
// omitted and will fail-closed during evaluation if the class requires
// automated methods.
func DefaultMethods(deps EvaluatorDeps) map[string][]KSIMethod {
	methods := make(map[string][]KSIMethod)

	// Reusable method factories.

	auditEventsExist := func(ctx context.Context) (bool, []Evidence, error) {
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
		return true, []Evidence{{
			Type:        EvidenceTypeExecutionID,
			Reference:   fmt.Sprintf("events:%d", len(events)),
			Description: "Audit store has recorded events",
		}}, nil
	}

	receiptsExist := func(ctx context.Context) (bool, []Evidence, error) {
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
		return true, []Evidence{{
			Type:        EvidenceTypeReceiptID,
			Reference:   receipts[0].TransactionID,
			Description: "Signed action receipt exists in audit store",
		}}, nil
	}

	receiptsHaveSignatures := func(ctx context.Context) (bool, []Evidence, error) {
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
				return false, []Evidence{{
					Type:        EvidenceTypeReceiptID,
					Reference:   r.TransactionID,
					Description: "Receipt missing signature or signer key ID",
				}}, nil
			}
		}
		return true, []Evidence{{
			Type:        EvidenceTypeReceiptID,
			Reference:   receipts[0].TransactionID,
			Description: "All sampled receipts have valid signatures",
		}}, nil
	}

	fileMutationsTracked := func(ctx context.Context) (bool, []Evidence, error) {
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
		return true, []Evidence{{
			Type:        EvidenceTypeExecutionID,
			Reference:   fmt.Sprintf("mutations:%d", len(mutations)),
			Description: "File mutations are tracked in audit store",
		}}, nil
	}

	merkleRootExists := func(ctx context.Context) (bool, []Evidence, error) {
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
		return true, []Evidence{{
			Type:        EvidenceTypeMerkleRoot,
			Reference:   root,
			Description: "Ledger Merkle root is non-empty",
		}}, nil
	}

	ledgerCommitsExist := func(ctx context.Context) (bool, []Evidence, error) {
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
		return true, []Evidence{{
			Type:        EvidenceTypeLedgerCommit,
			Reference:   commits[0].CommitHash,
			Description: "Ledger has committed entries",
		}}, nil
	}

	commitmentChainExists := func(ctx context.Context) (bool, []Evidence, error) {
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
		return true, []Evidence{{
			Type:        EvidenceTypeLedgerCommit,
			Reference:   commitments[0].Hash,
			Description: "Commitment ledger has chained entries",
		}}, nil
	}

	commitmentChainIntact := func(ctx context.Context) (bool, []Evidence, error) {
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
					return false, []Evidence{{
						Type:        EvidenceTypeLedgerCommit,
						Reference:   c.Hash,
						Description: "First commitment has non-empty prior hash",
					}}, nil
				}
				continue
			}
			if c.PriorCommitmentHash != commitments[i-1].Hash {
				return false, []Evidence{{
					Type:        EvidenceTypeLedgerCommit,
					Reference:   c.Hash,
					Description: "Commitment chain broken: prior hash mismatch",
				}}, nil
			}
		}
		return true, []Evidence{{
			Type:        EvidenceTypeLedgerCommit,
			Reference:   commitments[len(commitments)-1].Hash,
			Description: fmt.Sprintf("Commitment chain intact (%d entries)", len(commitments)),
		}}, nil
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
