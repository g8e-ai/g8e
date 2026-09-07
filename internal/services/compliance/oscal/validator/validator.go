// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

// Package validator implements the repository-owned pure-Go OSCAL
// assessment-results validator. It validates OSCAL documents in two
// explicit stages:
//
//  1. Structural validation against the embedded official NIST OSCAL 1.1.2
//     JSON Schema (Draft-07) using the internal/jsonschema compiler and
//     validator.
//
//  2. OSCAL-specific semantic validation for UUID identity and uniqueness,
//     reference resolution, assessment-plan imports, reviewed-control
//     selections, observation subjects and evidence links, finding targets
//     and statuses, back-matter resource integrity, media types, timestamps,
//     OSCAL version binding, and content-addressed g8e evidence metadata.
//
// Both stages return deterministic typed failures. Generic Draft-07
// validation and OSCAL semantic validation are separate explicit stages
// with separate typed error codes. No OSCAL bytes are emitted, signed,
// accepted, or marked verified unless both stages pass.
//
// The validator does not import or vendor a third-party JSON Schema engine,
// invoke Python, Java, Node, a subprocess, or a network service, or modify
// the authenticated NIST schema.
package validator

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/jsonschema"
	oscalschema "github.com/g8e-ai/g8e/v2/protocol/schemas/oscal"
)

// SemanticReasonCode is a stable, machine-readable OSCAL semantic
// validation failure code. Semantic reason codes are distinct from
// structural Draft-07 reason codes.
type SemanticReasonCode string

const (
	SemanticUUIDInvalid               SemanticReasonCode = "oscal.uuid_invalid"
	SemanticUUIDDuplicate             SemanticReasonCode = "oscal.uuid_duplicate"
	SemanticRefUnresolved             SemanticReasonCode = "oscal.ref_unresolved"
	SemanticImportAPMissing           SemanticReasonCode = "oscal.import_ap_missing"
	SemanticReviewedControls          SemanticReasonCode = "oscal.reviewed_controls_invalid"
	SemanticObsSubjectMissing         SemanticReasonCode = "oscal.observation_subject_missing"
	SemanticObsEvidenceUnresolved     SemanticReasonCode = "oscal.observation_evidence_unresolved"
	SemanticFindingTargetMissing      SemanticReasonCode = "oscal.finding_target_missing"
	SemanticFindingStatusInvalid      SemanticReasonCode = "oscal.finding_status_invalid"
	SemanticBackMatterResourceMissing SemanticReasonCode = "oscal.back_matter_resource_missing"
	SemanticMediaTypeInvalid          SemanticReasonCode = "oscal.media_type_invalid"
	SemanticTimestampInvalid          SemanticReasonCode = "oscal.timestamp_invalid"
	SemanticOSCALVersionInvalid       SemanticReasonCode = "oscal.version_invalid"
	SemanticContentAddressInvalid     SemanticReasonCode = "oscal.content_address_invalid"
)

// SemanticFailure is a typed OSCAL semantic validation failure.
type SemanticFailure struct {
	Reason      SemanticReasonCode `json:"reason"`
	Message     string             `json:"message"`
	InstancePtr string             `json:"instance_ptr,omitempty"`
}

func (f SemanticFailure) Error() string {
	return fmt.Sprintf("%s: %s (at %s)", f.Reason, f.Message, f.InstancePtr)
}

// SemanticFailures is a collection of semantic failures with deterministic
// ordering.
type SemanticFailures []SemanticFailure

func (fs SemanticFailures) Error() string {
	if len(fs) == 0 {
		return "no semantic failures"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d semantic failures:", len(fs))
	for _, f := range fs {
		b.WriteString("\n  ")
		b.WriteString(f.Error())
	}
	return b.String()
}

// ValidationResult is the complete result of validating an OSCAL
// assessment-results document. It carries the validator identity, schema
// version, schema digest, structural failures, semantic failures, and an
// overall valid flag.
type ValidationResult struct {
	ValidatorID        string              `json:"validator_id"`
	ValidatorVersion   string              `json:"validator_version"`
	SchemaVersion      string              `json:"schema_version"`
	SchemaDigest       string              `json:"schema_digest"`
	StructuralFailures jsonschema.Failures `json:"structural_failures,omitempty"`
	SemanticFailures   SemanticFailures    `json:"semantic_failures,omitempty"`
	Valid              bool                `json:"valid"`
}

// Validator is the OSCAL assessment-results validator. It performs
// structural (Draft-07) and semantic (OSCAL-specific) validation against
// the embedded official NIST OSCAL 1.1.2 schema.
type Validator struct {
	compiledSchema *jsonschema.Schema
	schemaDigest   string
}

// NewValidator creates and returns an OSCAL Validator. It compiles the
// embedded official NIST OSCAL 1.1.2 assessment-results schema at
// construction time. If the schema fails to compile (which would indicate
// a tampered or corrupted embedded schema), an error is returned.
func NewValidator() (*Validator, error) {
	schemaBytes, err := oscalschema.SchemaBytes()
	if err != nil {
		return nil, fmt.Errorf("oscal validator: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiled, err := compiler.Compile(schemaBytes)
	if err != nil {
		return nil, fmt.Errorf("oscal validator: compile embedded schema: %w", err)
	}
	return &Validator{
		compiledSchema: compiled,
		schemaDigest:   oscalschema.PinnedSHA256,
	}, nil
}

// ValidatorID returns the validator identity string.
func (v *Validator) ValidatorID() string {
	return constants.OSCALValidatorID
}

// ValidatorVersion returns the validator version string.
func (v *Validator) ValidatorVersion() string {
	return constants.OSCALValidatorVersion
}

// SchemaVersion returns the OSCAL schema version being validated against.
func (v *Validator) SchemaVersion() string {
	return constants.OSCALSchemaVersion
}

// SchemaDigest returns the pinned SHA-256 digest of the embedded schema.
func (v *Validator) SchemaDigest() string {
	return v.schemaDigest
}

// Validate validates raw OSCAL assessment-results bytes. It performs both
// structural (Draft-07) and semantic (OSCAL-specific) validation and
// returns a typed ValidationResult. The result is Valid only when both
// stages produce zero failures.
func (v *Validator) Validate(data []byte) (*ValidationResult, error) {
	result := &ValidationResult{
		ValidatorID:      constants.OSCALValidatorID,
		ValidatorVersion: constants.OSCALValidatorVersion,
		SchemaVersion:    constants.OSCALSchemaVersion,
		SchemaDigest:     v.schemaDigest,
		Valid:            false,
	}

	// Stage 1: Structural validation (Draft-07)
	jv := jsonschema.NewValidator()
	structuralFailures := jv.Validate(v.compiledSchema, data)
	result.StructuralFailures = structuralFailures

	if len(structuralFailures) > 0 {
		// Structural failures mean we cannot safely perform semantic
		// validation on the document structure
		return result, nil
	}

	// Stage 2: Semantic validation (OSCAL-specific)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		// This should not happen since structural validation passed,
		// but handle it defensively
		result.SemanticFailures = SemanticFailures{{
			Reason:  SemanticTimestampInvalid,
			Message: fmt.Sprintf("failed to decode document for semantic validation: %v", err),
		}}
		return result, nil
	}

	assessmentResults, ok := doc["assessment-results"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("oscal validator: decode assessment-results root: %w", constants.ErrOSCALValidationFailed)
	}
	semanticFailures := v.validateSemantics(assessmentResults)
	result.SemanticFailures = semanticFailures

	result.Valid = len(structuralFailures) == 0 && len(semanticFailures) == 0
	return result, nil
}

// validateSemantics performs OSCAL-specific semantic validation on the
// decoded document.
func (v *Validator) validateSemantics(doc map[string]any) SemanticFailures {
	var failures SemanticFailures
	collector := &semanticCollector{}
	v.validateUUIDs(doc, "/assessment-results", collector)
	v.validateReferences(doc, "/assessment-results", collector)
	v.validateTimestamps(doc, "/assessment-results", collector)
	v.validateOSCALVersion(doc, "/assessment-results", collector)
	v.validateBackMatter(doc, "/assessment-results", collector)
	v.validateFindings(doc, "/assessment-results", collector)
	v.validateObservations(doc, "/assessment-results", collector)
	failures = collector.failures
	// Sort failures deterministically
	for i := 1; i < len(failures); i++ {
		for j := i; j > 0 && compareSemantic(failures[j], failures[j-1]) < 0; j-- {
			failures[j], failures[j-1] = failures[j-1], failures[j]
		}
	}
	return failures
}

type semanticCollector struct {
	failures SemanticFailures
}

func (sc *semanticCollector) add(f SemanticFailure) {
	if len(sc.failures) < constants.OSCALValidatorMaxFailures {
		sc.failures = append(sc.failures, f)
	}
}

func compareSemantic(a, b SemanticFailure) int {
	if a.InstancePtr != b.InstancePtr {
		if a.InstancePtr < b.InstancePtr {
			return -1
		}
		return 1
	}
	if a.Reason != b.Reason {
		if a.Reason < b.Reason {
			return -1
		}
		return 1
	}
	return 0
}

// validateUUIDs checks that every UUID field is a valid RFC 4122 UUID and
// that UUIDs are unique within their scope.
func (v *Validator) validateUUIDs(doc map[string]any, path string, sc *semanticCollector) {
	seenUUIDs := make(map[string]string) // uuid -> path
	var walk func(n any, p string)
	walk = func(n any, p string) {
		switch val := n.(type) {
		case map[string]any:
			if uuidStr, ok := val["uuid"].(string); ok {
				if !isValidUUID(uuidStr) {
					sc.add(SemanticFailure{
						Reason:      SemanticUUIDInvalid,
						Message:     fmt.Sprintf("invalid UUID %q", uuidStr),
						InstancePtr: p + "/uuid",
					})
				} else {
					if existingPath, exists := seenUUIDs[uuidStr]; exists {
						sc.add(SemanticFailure{
							Reason:      SemanticUUIDDuplicate,
							Message:     fmt.Sprintf("duplicate UUID %q at %s and %s", uuidStr, existingPath, p+"/uuid"),
							InstancePtr: p + "/uuid",
						})
					} else {
						seenUUIDs[uuidStr] = p + "/uuid"
					}
				}
			}
			for k, child := range val {
				walk(child, p+"/"+escapeToken(k))
			}
		case []any:
			for i, item := range val {
				walk(item, fmt.Sprintf("%s/%d", p, i))
			}
		}
	}
	walk(doc, path)
}

// validateReferences checks that href references to back-matter resources
// resolve.
func (v *Validator) validateReferences(doc map[string]any, path string, sc *semanticCollector) {
	// Collect back-matter resource IDs
	resourceIDs := make(map[string]bool)
	if backMatter, ok := doc["back-matter"].(map[string]any); ok {
		if resources, ok := backMatter["resources"].([]any); ok {
			for _, r := range resources {
				if res, ok := r.(map[string]any); ok {
					if uuid, ok := res["uuid"].(string); ok {
						resourceIDs[uuid] = true
					}
				}
			}
		}
	}

	// Walk the document looking for href fields that reference back-matter
	var walk func(n any, p string)
	walk = func(n any, p string) {
		switch val := n.(type) {
		case map[string]any:
			if href, ok := val["href"].(string); ok && p != path+"/import-ap" {
				// Back-matter references use the form #<uuid>
				if strings.HasPrefix(href, "#") {
					refID := href[1:]
					if !resourceIDs[refID] {
						// Only flag if it looks like a UUID reference
						if isValidUUID(refID) {
							sc.add(SemanticFailure{
								Reason:      SemanticRefUnresolved,
								Message:     fmt.Sprintf("href %q does not resolve to a back-matter resource", href),
								InstancePtr: p + "/href",
							})
						}
					}
				}
			}
			for k, child := range val {
				walk(child, p+"/"+escapeToken(k))
			}
		case []any:
			for i, item := range val {
				walk(item, fmt.Sprintf("%s/%d", p, i))
			}
		}
	}
	walk(doc, path)
}

// validateTimestamps checks that timestamp fields are valid RFC 3339.
func (v *Validator) validateTimestamps(doc map[string]any, path string, sc *semanticCollector) {
	timestampFields := map[string]bool{
		"published":     true,
		"last-modified": true,
		"modified":      true,
		"start":         true,
		"end":           true,
		"collected":     true,
		"expires":       true,
	}
	var walk func(n any, p string)
	walk = func(n any, p string) {
		switch val := n.(type) {
		case map[string]any:
			for k, child := range val {
				if timestampFields[k] {
					if ts, ok := child.(string); ok {
						if _, err := time.Parse(time.RFC3339, ts); err != nil {
							sc.add(SemanticFailure{
								Reason:      SemanticTimestampInvalid,
								Message:     fmt.Sprintf("invalid timestamp %q for field %q", ts, k),
								InstancePtr: p + "/" + escapeToken(k),
							})
						}
					}
				}
				walk(child, p+"/"+escapeToken(k))
			}
		case []any:
			for i, item := range val {
				walk(item, fmt.Sprintf("%s/%d", p, i))
			}
		}
	}
	walk(doc, path)
}

// validateOSCALVersion checks that the oscal-version field matches the
// expected version.
func (v *Validator) validateOSCALVersion(doc map[string]any, path string, sc *semanticCollector) {
	// Check metadata.oscal-version
	if metadata, ok := doc["metadata"].(map[string]any); ok {
		if version, ok := metadata["oscal-version"].(string); ok {
			if version != constants.OSCALSchemaVersion {
				sc.add(SemanticFailure{
					Reason:      SemanticOSCALVersionInvalid,
					Message:     fmt.Sprintf("oscal-version %q does not match expected %q", version, constants.OSCALSchemaVersion),
					InstancePtr: path + "/metadata/oscal-version",
				})
			}
		}
	}
}

// validateBackMatter checks back-matter resource integrity.
func (v *Validator) validateBackMatter(doc map[string]any, path string, sc *semanticCollector) {
	backMatter, ok := doc["back-matter"].(map[string]any)
	if !ok {
		return
	}
	resources, ok := backMatter["resources"].([]any)
	if !ok {
		return
	}
	for i, r := range resources {
		res, ok := r.(map[string]any)
		if !ok {
			continue
		}
		rPath := fmt.Sprintf("%s/back-matter/resources/%d", path, i)
		// Check that resources have a title or description
		_, hasTitle := res["title"]
		_, hasDescription := res["description"]
		if !hasTitle && !hasDescription {
			sc.add(SemanticFailure{
				Reason:      SemanticBackMatterResourceMissing,
				Message:     "back-matter resource is missing a title and description",
				InstancePtr: rPath,
			})
		}
		// Check media-type if present
		if props, ok := res["props"].([]any); ok {
			for j, prop := range props {
				if pm, ok := prop.(map[string]any); ok {
					if name, ok := pm["name"].(string); ok && name == "media-type" {
						if mt, ok := pm["value"].(string); ok {
							if !isValidMediaType(mt) {
								sc.add(SemanticFailure{
									Reason:      SemanticMediaTypeInvalid,
									Message:     fmt.Sprintf("invalid media-type %q", mt),
									InstancePtr: fmt.Sprintf("%s/props/%d/value", rPath, j),
								})
							}
						}
					}
				}
			}
		}
	}
}

// validateFindings checks finding targets and statuses.
func (v *Validator) validateFindings(doc map[string]any, path string, sc *semanticCollector) {
	results, ok := doc["results"].([]any)
	if !ok {
		return
	}
	validStatuses := map[string]bool{
		"satisfied":      true,
		"not-satisfied":  true,
		"not-applicable": true,
	}
	for i, result := range results {
		res, ok := result.(map[string]any)
		if !ok {
			continue
		}
		findings, ok := res["findings"].([]any)
		if !ok {
			continue
		}
		for j, f := range findings {
			finding, ok := f.(map[string]any)
			if !ok {
				continue
			}
			fPath := fmt.Sprintf("%s/results/%d/findings/%d", path, i, j)
			// Check target
			target, ok := finding["target"].(map[string]any)
			if !ok {
				sc.add(SemanticFailure{
					Reason:      SemanticFindingTargetMissing,
					Message:     "finding is missing target",
					InstancePtr: fPath,
				})
				continue
			}
			// Check target status
			status, ok := target["status"].(map[string]any)
			if !ok {
				sc.add(SemanticFailure{
					Reason:      SemanticFindingStatusInvalid,
					Message:     "finding target is missing status",
					InstancePtr: fPath + "/target",
				})
				continue
			}
			if state, ok := status["state"].(string); ok {
				if !validStatuses[state] {
					sc.add(SemanticFailure{
						Reason:      SemanticFindingStatusInvalid,
						Message:     fmt.Sprintf("invalid finding status %q", state),
						InstancePtr: fPath + "/target/status/state",
					})
				}
			}
		}
	}
}

// validateObservations checks observation subjects and evidence links.
func (v *Validator) validateObservations(doc map[string]any, path string, sc *semanticCollector) {
	results, ok := doc["results"].([]any)
	if !ok {
		return
	}
	// Collect back-matter resource IDs for evidence link resolution
	resourceIDs := make(map[string]bool)
	if backMatter, ok := doc["back-matter"].(map[string]any); ok {
		if resources, ok := backMatter["resources"].([]any); ok {
			for _, r := range resources {
				if res, ok := r.(map[string]any); ok {
					if uuid, ok := res["uuid"].(string); ok {
						resourceIDs[uuid] = true
					}
				}
			}
		}
	}
	for i, result := range results {
		res, ok := result.(map[string]any)
		if !ok {
			continue
		}
		observations, ok := res["observations"].([]any)
		if !ok {
			continue
		}
		for j, obs := range observations {
			observation, ok := obs.(map[string]any)
			if !ok {
				continue
			}
			oPath := fmt.Sprintf("%s/results/%d/observations/%d", path, i, j)
			// Check subjects
			if subjects, ok := observation["subjects"].([]any); ok {
				if len(subjects) == 0 {
					sc.add(SemanticFailure{
						Reason:      SemanticObsSubjectMissing,
						Message:     "observation has empty subjects",
						InstancePtr: oPath + "/subjects",
					})
				}
			}
			// Check relevant-evidence links
			if evidence, ok := observation["relevant-evidence"].([]any); ok {
				for k, ev := range evidence {
					evMap, ok := ev.(map[string]any)
					if !ok {
						continue
					}
					if href, ok := evMap["href"].(string); ok {
						if strings.HasPrefix(href, "#") {
							refID := href[1:]
							if isValidUUID(refID) && !resourceIDs[refID] {
								sc.add(SemanticFailure{
									Reason:      SemanticObsEvidenceUnresolved,
									Message:     fmt.Sprintf("evidence href %q does not resolve to a back-matter resource", href),
									InstancePtr: fmt.Sprintf("%s/relevant-evidence/%d/href", oPath, k),
								})
							}
						}
					}
				}
			}
		}
	}
}

// --- Helper functions ---

func isValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	// Format: 8-4-4-4-12 with hex chars and dashes at positions 8, 13, 18, 23
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

func isValidMediaType(mt string) bool {
	validMediaTypes := map[string]bool{
		constants.MediaTypeOSCALJSON: true,
		constants.MediaTypeJSON:      true,
		constants.MediaTypeMarkdown:  true,
		constants.MediaTypeHTML:      true,
		constants.MediaTypeText:      true,
		"application/octet-stream":   true,
		"text/csv":                   true,
	}
	return validMediaTypes[mt]
}

func escapeToken(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}
