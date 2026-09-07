// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// KSICategory represents a FedRAMP 20x KSI category (CR26).
type KSICategory string

const (
	KSICategoryCED KSICategory = "CED" // Cybersecurity Education
	KSICategoryCMT KSICategory = "CMT" // Change Management
	KSICategoryCNA KSICategory = "CNA" // Cloud Native Architecture
	KSICategoryIAM KSICategory = "IAM" // Identity and Access Management
	KSICategoryINR KSICategory = "INR" // Incident Response
	KSICategoryMLA KSICategory = "MLA" // Monitoring, Logging, and Auditing
	KSICategoryPIY KSICategory = "PIY" // Policy and Inventory
	KSICategoryRCP KSICategory = "RCP" // Recovery Planning
	KSICategorySVC KSICategory = "SVC" // Service Configuration
	KSICategoryTPR KSICategory = "TPR" // Supply Chain Risk
)

// KSIStatus represents the binary evaluation result for a KSI.
type KSIStatus string

const (
	KSIStatusSatisfied     KSIStatus = "satisfied"
	KSIStatusNotSatisfied  KSIStatus = "not_satisfied"
	KSIStatusNotApplicable KSIStatus = "not_applicable"
)

type KSIOutcome string

const (
	KSIOutcomeSatisfied                   KSIOutcome = "satisfied"
	KSIOutcomeMethodFailure               KSIOutcome = "method_failure"
	KSIOutcomeInvalidEvidence             KSIOutcome = "invalid_evidence"
	KSIOutcomeStaleEvidence               KSIOutcome = "stale_evidence"
	KSIOutcomeUnsupportedAutomation       KSIOutcome = "unsupported_automation"
	KSIOutcomeCustomerAttestationRequired KSIOutcome = "customer_attestation_required"
	KSIOutcomeNotApplicable               KSIOutcome = "not_applicable"
)

func (o KSIOutcome) Status() KSIStatus {
	switch o {
	case KSIOutcomeSatisfied:
		return KSIStatusSatisfied
	case KSIOutcomeNotApplicable:
		return KSIStatusNotApplicable
	default:
		return KSIStatusNotSatisfied
	}
}

func (o KSIOutcome) Valid() bool {
	switch o {
	case KSIOutcomeSatisfied, KSIOutcomeMethodFailure, KSIOutcomeInvalidEvidence, KSIOutcomeStaleEvidence, KSIOutcomeUnsupportedAutomation, KSIOutcomeCustomerAttestationRequired, KSIOutcomeNotApplicable:
		return true
	default:
		return false
	}
}

func (o KSIOutcome) validMethodUnsatisfiedOutcome() bool {
	switch o {
	case KSIOutcomeInvalidEvidence, KSIOutcomeStaleEvidence, KSIOutcomeUnsupportedAutomation, KSIOutcomeCustomerAttestationRequired:
		return true
	default:
		return false
	}
}

// CertificationClass represents a FedRAMP 20x certification class (CR26).
type CertificationClass string

const (
	ClassA CertificationClass = "A"
	ClassB CertificationClass = "B"
	ClassC CertificationClass = "C"
	ClassD CertificationClass = "D"
)

// ValidationCycle defines the re-validation cadence for a KSI.
type ValidationCycle string

const (
	ValidationCycleMachine    ValidationCycle = "7d"  // Machine-based resources: 7 days
	ValidationCycleNonMachine ValidationCycle = "90d" // Non-machine resources: 90 days
)

// EvidenceType identifies the source of evidence for a KSI evaluation.
type EvidenceType string

const (
	EvidenceTypeReceiptID           EvidenceType = "receipt_id"
	EvidenceTypeLedgerCommit        EvidenceType = "ledger_commit"
	EvidenceTypeExecutionID         EvidenceType = "execution_id"
	EvidenceTypeMerkleRoot          EvidenceType = "merkle_root"
	EvidenceTypeCommitmentSignature EvidenceType = "commitment_signature"
	EvidenceTypeStateObservation    EvidenceType = "state_observation"
	EvidenceTypeGraderResult        EvidenceType = "grader_result"
	EvidenceTypeHistoricalFreshness EvidenceType = "historical_freshness"
)

// AutomatedMethod describes a single automated method used to evaluate a KSI.
type AutomatedMethod struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// KSIArtifactIdentity identifies the evidence artifact a KSI method measures.
type KSIArtifactIdentity string

const (
	KSIArtifactAuditEvents             KSIArtifactIdentity = "audit_events"
	KSIArtifactActionReceipts          KSIArtifactIdentity = "action_receipts"
	KSIArtifactFileMutations           KSIArtifactIdentity = "file_mutations"
	KSIArtifactLedgerCommits           KSIArtifactIdentity = "ledger_commits"
	KSIArtifactCommitments             KSIArtifactIdentity = "commitments"
	KSIArtifactReceiptStateTransitions KSIArtifactIdentity = "receipt_state_transitions"
	KSIArtifactGraderResults           KSIArtifactIdentity = "grader_results"
	KSIArtifactHistorySnapshots        KSIArtifactIdentity = "ksi_history_snapshots"
	KSIArtifactLedgerStateRoot         KSIArtifactIdentity = "ledger_state_root"
)

// KSICollectionBoundary identifies the independently queried evidence source.
type KSICollectionBoundary string

const (
	KSICollectionAuditStore      KSICollectionBoundary = "audit_store"
	KSICollectionLedgerStore     KSICollectionBoundary = "ledger_store"
	KSICollectionCommitmentStore KSICollectionBoundary = "commitment_store"
	KSICollectionEvalResults     KSICollectionBoundary = "eval_results"
	KSICollectionHistoryStore    KSICollectionBoundary = "ksi_history_store"
)

// KSIVerifierFamily identifies the verification technique a method applies.
type KSIVerifierFamily string

const (
	KSIVerifierExistence           KSIVerifierFamily = "existence"
	KSIVerifierStructural          KSIVerifierFamily = "structural"
	KSIVerifierCryptographic       KSIVerifierFamily = "cryptographic"
	KSIVerifierStateObservation    KSIVerifierFamily = "state_observation"
	KSIVerifierDeterministicGrader KSIVerifierFamily = "deterministic_grader"
	KSIVerifierHistorical          KSIVerifierFamily = "historical"
)

// KSIMeasuredProperty identifies the fact established by a KSI method.
type KSIMeasuredProperty string

const (
	KSIPropertyPresence                    KSIMeasuredProperty = "presence"
	KSIPropertyChainLinkage                KSIMeasuredProperty = "chain_linkage"
	KSIPropertySignatureValidity           KSIMeasuredProperty = "signature_validity"
	KSIPropertyReceiptPersistenceIntegrity KSIMeasuredProperty = "receipt_persistence_integrity"
	KSIPropertyStateTransitionBinding      KSIMeasuredProperty = "state_transition_binding"
	KSIPropertyStateRootMatchesHead        KSIMeasuredProperty = "state_root_matches_head"
	KSIPropertyEvidenceContentAddressing   KSIMeasuredProperty = "evidence_content_addressing"
	KSIPropertyFreshness                   KSIMeasuredProperty = "freshness"
)

type ksiMethodEvaluator func(ctx context.Context) (bool, []*compliancev1.ComplianceEvidenceReference, error)

// KSIMethod binds one evaluator to the typed metadata used to prove method independence.
type KSIMethod struct {
	Name               string
	Version            string
	ArtifactIdentity   KSIArtifactIdentity
	CollectionBoundary KSICollectionBoundary
	VerifierFamily     KSIVerifierFamily
	MeasuredProperty   KSIMeasuredProperty
	UnsatisfiedOutcome KSIOutcome
	evaluate           ksiMethodEvaluator
}

func newKSIMethod(name string, artifact KSIArtifactIdentity, boundary KSICollectionBoundary, verifier KSIVerifierFamily, property KSIMeasuredProperty, evaluate ksiMethodEvaluator) KSIMethod {
	return KSIMethod{
		Name: name, Version: constants.KSIMethodDefinitionVersion, ArtifactIdentity: artifact,
		CollectionBoundary: boundary, VerifierFamily: verifier, MeasuredProperty: property,
		UnsatisfiedOutcome: KSIOutcomeInvalidEvidence, evaluate: evaluate,
	}
}

func (m KSIMethod) validate() error {
	if m.Name == "" || m.Version == "" || !m.ArtifactIdentity.valid() || !m.CollectionBoundary.valid() || !m.VerifierFamily.valid() || !m.MeasuredProperty.valid() || !m.UnsatisfiedOutcome.validMethodUnsatisfiedOutcome() || m.evaluate == nil {
		return constants.ErrKSIMethodInvalid
	}
	return nil
}

func (a KSIArtifactIdentity) valid() bool {
	switch a {
	case KSIArtifactAuditEvents, KSIArtifactActionReceipts, KSIArtifactFileMutations, KSIArtifactLedgerCommits, KSIArtifactCommitments, KSIArtifactReceiptStateTransitions, KSIArtifactGraderResults, KSIArtifactHistorySnapshots, KSIArtifactLedgerStateRoot:
		return true
	default:
		return false
	}
}

func (b KSICollectionBoundary) valid() bool {
	switch b {
	case KSICollectionAuditStore, KSICollectionLedgerStore, KSICollectionCommitmentStore, KSICollectionEvalResults, KSICollectionHistoryStore:
		return true
	default:
		return false
	}
}

func (v KSIVerifierFamily) valid() bool {
	switch v {
	case KSIVerifierExistence, KSIVerifierStructural, KSIVerifierCryptographic, KSIVerifierStateObservation, KSIVerifierDeterministicGrader, KSIVerifierHistorical:
		return true
	default:
		return false
	}
}

func (p KSIMeasuredProperty) valid() bool {
	switch p {
	case KSIPropertyPresence, KSIPropertyChainLinkage, KSIPropertySignatureValidity, KSIPropertyReceiptPersistenceIntegrity, KSIPropertyStateTransitionBinding, KSIPropertyStateRootMatchesHead, KSIPropertyEvidenceContentAddressing, KSIPropertyFreshness:
		return true
	default:
		return false
	}
}

func (m KSIMethod) independenceKey() string {
	return string(m.ArtifactIdentity) + "\x00" + string(m.CollectionBoundary) + "\x00" + string(m.VerifierFamily) + "\x00" + string(m.MeasuredProperty)
}

// KSI represents a single Key Security Indicator from the FedRAMP 20x program.
type KSI struct {
	ID                  string                                      `json:"id"`
	Title               string                                      `json:"title"`
	Category            KSICategory                                 `json:"category"`
	Description         string                                      `json:"description,omitempty"`
	ControlRefs         []string                                    `json:"control_refs"`
	OverlayRefs         []string                                    `json:"overlay_refs,omitempty"`
	ApplicableClasses   []CertificationClass                        `json:"applicable_classes"`
	ValidationCycle     ValidationCycle                             `json:"validation_cycle"`
	Status              KSIStatus                                   `json:"status"`
	AutomatedMethods    []AutomatedMethod                           `json:"automated_methods"`
	Evidence            []*compliancev1.ComplianceEvidenceReference `json:"evidence,omitempty"`
	LastValidatedUnixMs int64                                       `json:"last_validated_unix_ms,omitempty"`
}

// EvaluationBinding carries the scope, evidence window, and method version
// identity that bind a KSI evaluation to a specific assessment context. Every
// KSIResult and KSIResultSet carries a binding so consumers can prove that no
// result was produced from evidence outside the declared scope, run, attempt,
// scenario, action, transaction, or assessment window.
type AssertionAssessmentScope struct {
	AssessmentIDs []string `json:"assessment_ids"`
	AttemptIDs    []string `json:"attempt_ids,omitempty"`
	ScenarioIDs   []string `json:"scenario_ids,omitempty"`
	ActionIDs     []string `json:"action_ids,omitempty"`
}

type EvaluationBinding struct {
	ScopeID              string                   `json:"scope_id"`
	RunID                string                   `json:"run_id"`
	WindowStartUnixMs    int64                    `json:"window_start_unix_ms"`
	WindowEndUnixMs      int64                    `json:"window_end_unix_ms"`
	EvaluatorID          string                   `json:"evaluator_id"`
	EvaluatorVersion     string                   `json:"evaluator_version"`
	MethodDefinitionID   string                   `json:"method_definition_id"`
	AssertionAssessments AssertionAssessmentScope `json:"assertion_assessments"`
}

// Validate returns constants.ErrKSIBindingIncomplete when required binding
// fields are empty or the evidence window is inverted.
func (b EvaluationBinding) Validate() error {
	if b.ScopeID == "" || b.RunID == "" || b.WindowStartUnixMs <= 0 || b.WindowEndUnixMs <= 0 || b.WindowStartUnixMs > b.WindowEndUnixMs || b.EvaluatorID == "" || b.EvaluatorVersion == "" || b.MethodDefinitionID == "" {
		return fmt.Errorf("%w: scope, run, window, evaluator, and method definition are required", constants.ErrKSIBindingIncomplete)
	}
	if err := b.AssertionAssessments.Validate(); err != nil {
		return err
	}
	return nil
}

func (s AssertionAssessmentScope) Validate() error {
	if len(s.AssessmentIDs) == 0 {
		return fmt.Errorf("%w: at least one assertion assessment is required", constants.ErrKSIBindingIncomplete)
	}
	for _, values := range [][]string{s.AssessmentIDs, s.AttemptIDs, s.ScenarioIDs, s.ActionIDs} {
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if value == "" {
				return fmt.Errorf("%w: assertion assessment scope values must not be empty", constants.ErrKSIBindingIncomplete)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("%w: assertion assessment scope values must be unique", constants.ErrKSIBindingIncomplete)
			}
			seen[value] = struct{}{}
		}
	}
	return nil
}

func (s AssertionAssessmentScope) Equal(other AssertionAssessmentScope) bool {
	return equalStringSet(s.AssessmentIDs, other.AssessmentIDs) && equalStringSet(s.AttemptIDs, other.AttemptIDs) && equalStringSet(s.ScenarioIDs, other.ScenarioIDs) && equalStringSet(s.ActionIDs, other.ActionIDs)
}

func (b EvaluationBinding) Equal(other EvaluationBinding) bool {
	return b.ScopeID == other.ScopeID && b.RunID == other.RunID && b.WindowStartUnixMs == other.WindowStartUnixMs && b.WindowEndUnixMs == other.WindowEndUnixMs && b.EvaluatorID == other.EvaluatorID && b.EvaluatorVersion == other.EvaluatorVersion && b.MethodDefinitionID == other.MethodDefinitionID && b.AssertionAssessments.Equal(other.AssertionAssessments)
}

func equalStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	values := make(map[string]struct{}, len(left))
	for _, value := range left {
		values[value] = struct{}{}
	}
	for _, value := range right {
		if _, exists := values[value]; !exists {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (b EvaluationBinding) ValidateEvidence(ref *compliancev1.ComplianceEvidenceReference) error {
	if ref == nil {
		return fmt.Errorf("%w: evidence reference is nil", constants.ErrKSIBindingMismatch)
	}
	if ref.GetScopeId() != "" && ref.GetScopeId() != b.ScopeID {
		return fmt.Errorf("%w: evidence scope does not match assessment scope", constants.ErrKSIBindingMismatch)
	}
	if ref.GetRunId() != "" && ref.GetRunId() != b.RunID {
		return fmt.Errorf("%w: evidence run does not match assessment run", constants.ErrKSIBindingMismatch)
	}
	if ref.GetAttemptId() != "" && !containsString(b.AssertionAssessments.AttemptIDs, ref.GetAttemptId()) {
		return fmt.Errorf("%w: evidence attempt is outside the assertion assessment scope", constants.ErrKSIBindingMismatch)
	}
	if ref.GetScenarioId() != "" && !containsString(b.AssertionAssessments.ScenarioIDs, ref.GetScenarioId()) {
		return fmt.Errorf("%w: evidence scenario is outside the assertion assessment scope", constants.ErrKSIBindingMismatch)
	}
	if ref.GetTransactionId() != "" && !containsString(b.AssertionAssessments.ActionIDs, ref.GetTransactionId()) {
		return fmt.Errorf("%w: evidence action is outside the assertion assessment scope", constants.ErrKSIBindingMismatch)
	}
	if ref.GetProducedAt() != nil {
		if err := ref.GetProducedAt().CheckValid(); err != nil || !b.Contains(ref.GetProducedAt().AsTime().UnixMilli()) {
			return fmt.Errorf("%w: evidence timestamp is outside the assessment window", constants.ErrKSIBindingMismatch)
		}
	}
	return nil
}

// Contains returns true when the given timestamp falls inside the evidence
// window (inclusive). It is used to prove that evidence was produced within the
// declared assessment window.
func (b EvaluationBinding) Contains(producedAtMs int64) bool {
	return producedAtMs >= b.WindowStartUnixMs && producedAtMs <= b.WindowEndUnixMs
}

// KSIResult is the evaluation result for a single KSI at a point in time.
type KSIResult struct {
	ID                  string                                      `json:"id"`
	Status              KSIStatus                                   `json:"status"`
	Outcome             KSIOutcome                                  `json:"outcome"`
	Evidence            []*compliancev1.ComplianceEvidenceReference `json:"evidence,omitempty"`
	LastValidatedUnixMs int64                                       `json:"last_validated_unix_ms"`
	MethodCount         int                                         `json:"method_count"`
	Binding             EvaluationBinding                           `json:"binding"`
}

// KSIResultSet is a collection of KSI evaluation results for a target class.
type KSIResultSet struct {
	Class         CertificationClass `json:"class"`
	EvaluatedAtMs int64              `json:"evaluated_at_ms"`
	Results       []KSIResult        `json:"results"`
	Binding       EvaluationBinding  `json:"binding"`
}

// KSICatalog is the typed collection of all known KSIs from the CR26 reference.
type KSICatalog struct {
	Version string `json:"version"`
	Source  string `json:"source"`
	KSIs    []KSI  `json:"ksis"`
}

// LoadKSICatalog reads and parses the KSI catalog JSON file at the given path.
func LoadKSICatalog(path string) (*KSICatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("compliance: read KSI catalog: %w", err)
	}

	var catalog KSICatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("compliance: parse KSI catalog: %w", err)
	}

	if err := catalog.Validate(); err != nil {
		return nil, fmt.Errorf("compliance: validate KSI catalog: %w", err)
	}

	return &catalog, nil
}

// Validate checks that the catalog has required fields and no duplicate KSI IDs.
func (c *KSICatalog) Validate() error {
	if c.Version == "" {
		return fmt.Errorf("%w: catalog version is empty", constants.ErrKSICatalogInvalid)
	}
	if c.Source == "" {
		return fmt.Errorf("%w: catalog source is empty", constants.ErrKSICatalogInvalid)
	}
	if len(c.KSIs) == 0 {
		return fmt.Errorf("%w: catalog has no KSIs", constants.ErrKSICatalogInvalid)
	}

	seen := make(map[string]bool, len(c.KSIs))
	for i := range c.KSIs {
		ksi := &c.KSIs[i]
		if ksi.ID == "" {
			return fmt.Errorf("%w: KSI at index %d has empty ID", constants.ErrKSICatalogInvalid, i)
		}
		if ksi.Title == "" {
			return fmt.Errorf("%w: KSI %s has empty title", constants.ErrKSICatalogInvalid, ksi.ID)
		}
		if ksi.Category == "" {
			return fmt.Errorf("%w: KSI %s has empty category", constants.ErrKSICatalogInvalid, ksi.ID)
		}
		if seen[ksi.ID] {
			return fmt.Errorf("%w: duplicate KSI ID: %s", constants.ErrKSICatalogInvalid, ksi.ID)
		}
		seen[ksi.ID] = true
	}

	return nil
}

// FindKSI returns the KSI with the given ID, or nil if not found.
func (c *KSICatalog) FindKSI(id string) *KSI {
	for i := range c.KSIs {
		if c.KSIs[i].ID == id {
			return &c.KSIs[i]
		}
	}
	return nil
}

// KSIsForClass returns all KSIs applicable to the given certification class.
func (c *KSICatalog) KSIsForClass(class CertificationClass) []KSI {
	var result []KSI
	for _, ksi := range c.KSIs {
		for _, ac := range ksi.ApplicableClasses {
			if ac == class {
				result = append(result, ksi)
				break
			}
		}
	}
	return result
}

// MinimumMethodsForClass returns the minimum number of automated methods
// required per KSI for the given certification class.
// Class A: 0 (MAY automate), Class B: 1 (SHOULD), Class C: 2 (MUST), Class D: 4.
func MinimumMethodsForClass(class CertificationClass) int {
	switch class {
	case ClassA:
		return 0
	case ClassB:
		return 1
	case ClassC:
		return 2
	case ClassD:
		return 4
	default:
		return 0
	}
}

// IsStale returns true if the KSI's last validation exceeds its validation cycle.
func (k *KSI) IsStale(now time.Time) bool {
	if k.LastValidatedUnixMs == 0 {
		return true
	}
	lastValidated := time.UnixMilli(k.LastValidatedUnixMs)
	var cycle time.Duration
	switch k.ValidationCycle {
	case ValidationCycleMachine:
		cycle = 7 * 24 * time.Hour
	case ValidationCycleNonMachine:
		cycle = 90 * 24 * time.Hour
	default:
		return true
	}
	return now.Sub(lastValidated) > cycle
}
