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

	"github.com/g8e-ai/g8e/internal/constants"
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
	EvidenceTypeReceiptID    EvidenceType = "receipt_id"
	EvidenceTypeLedgerCommit EvidenceType = "ledger_commit"
	EvidenceTypeExecutionID  EvidenceType = "execution_id"
	EvidenceTypeMerkleRoot   EvidenceType = "merkle_root"
)

// Evidence points to a specific artifact in g8e's audit store, ledger,
// or LFAA execution vault that supports a KSI evaluation result.
type Evidence struct {
	Type        EvidenceType `json:"type"`
	Reference   string       `json:"reference"`
	Description string       `json:"description,omitempty"`
}

// AutomatedMethod describes a single automated method used to evaluate a KSI.
type AutomatedMethod struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// KSIMethod is a function that evaluates a single KSI against live g8e state.
// It returns whether the KSI is satisfied, supporting evidence, or an error.
type KSIMethod func(ctx context.Context) (bool, []Evidence, error)

// KSI represents a single Key Security Indicator from the FedRAMP 20x program.
type KSI struct {
	ID                  string               `json:"id"`
	Title               string               `json:"title"`
	Category            KSICategory          `json:"category"`
	Description         string               `json:"description,omitempty"`
	ControlRefs         []string             `json:"control_refs"`
	OverlayRefs         []string             `json:"overlay_refs,omitempty"`
	ApplicableClasses   []CertificationClass `json:"applicable_classes"`
	ValidationCycle     ValidationCycle      `json:"validation_cycle"`
	Status              KSIStatus            `json:"status"`
	AutomatedMethods    []AutomatedMethod    `json:"automated_methods"`
	Evidence            []Evidence           `json:"evidence,omitempty"`
	LastValidatedUnixMs int64                `json:"last_validated_unix_ms,omitempty"`
}

// KSIResult is the evaluation result for a single KSI at a point in time.
type KSIResult struct {
	ID                  string     `json:"id"`
	Status              KSIStatus  `json:"status"`
	Evidence            []Evidence `json:"evidence,omitempty"`
	LastValidatedUnixMs int64      `json:"last_validated_unix_ms"`
	MethodCount         int        `json:"method_count"`
}

// KSIResultSet is a collection of KSI evaluation results for a target class.
type KSIResultSet struct {
	Class         CertificationClass `json:"class"`
	EvaluatedAtMs int64              `json:"evaluated_at_ms"`
	Results       []KSIResult        `json:"results"`
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
