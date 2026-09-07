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
	"sort"
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
	SemanticEvidenceScopeMismatch     SemanticReasonCode = "oscal.evidence_scope_mismatch"
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
	v.validateAssessmentPlan(doc, "/assessment-results", collector)
	v.validateReviewedControls(doc, "/assessment-results", collector)
	v.validateTimestamps(doc, "/assessment-results", collector)
	v.validateResultIntervals(doc, "/assessment-results", collector)
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
			for _, k := range sortedMapKeys(val) {
				walk(val[k], p+"/"+escapeToken(k))
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
						sc.add(SemanticFailure{
							Reason:      SemanticRefUnresolved,
							Message:     fmt.Sprintf("href %q does not resolve to a back-matter resource", href),
							InstancePtr: p + "/href",
						})
					}
				}
			}
			for _, k := range sortedMapKeys(val) {
				walk(val[k], p+"/"+escapeToken(k))
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
func (v *Validator) validateAssessmentPlan(doc map[string]any, path string, sc *semanticCollector) {
	importAP, ok := doc["import-ap"].(map[string]any)
	if !ok {
		sc.add(SemanticFailure{Reason: SemanticImportAPMissing, Message: "assessment-results is missing import-ap", InstancePtr: path + "/import-ap"})
		return
	}
	href, ok := importAP["href"].(string)
	if !ok || strings.TrimSpace(href) == "" {
		sc.add(SemanticFailure{Reason: SemanticImportAPMissing, Message: "assessment-plan href is missing", InstancePtr: path + "/import-ap/href"})
		return
	}
	if strings.HasPrefix(href, "#") && !backMatterResourceIDs(doc)[strings.TrimPrefix(href, "#")] {
		sc.add(SemanticFailure{Reason: SemanticImportAPMissing, Message: fmt.Sprintf("assessment-plan href %q does not resolve to back matter", href), InstancePtr: path + "/import-ap/href"})
	}
}

func (v *Validator) validateReviewedControls(doc map[string]any, path string, sc *semanticCollector) {
	results, _ := doc["results"].([]any)
	for i, item := range results {
		result, _ := item.(map[string]any)
		reviewed, ok := result["reviewed-controls"].(map[string]any)
		resultPath := fmt.Sprintf("%s/results/%d/reviewed-controls", path, i)
		if !ok {
			sc.add(SemanticFailure{Reason: SemanticReviewedControls, Message: "reviewed-controls is missing", InstancePtr: resultPath})
			continue
		}
		selections, _ := reviewed["control-selections"].([]any)
		seen := make(map[string]struct{})
		selected := 0
		for j, selectionValue := range selections {
			selection, _ := selectionValue.(map[string]any)
			includeControls, _ := selection["include-controls"].([]any)
			_, includesAll := selection["include-all"]
			if len(includeControls) == 0 && !includesAll {
				sc.add(SemanticFailure{Reason: SemanticReviewedControls, Message: "control selection includes no controls", InstancePtr: fmt.Sprintf("%s/control-selections/%d", resultPath, j)})
			}
			for k, controlValue := range includeControls {
				control, _ := controlValue.(map[string]any)
				controlID, _ := control["control-id"].(string)
				controlPath := fmt.Sprintf("%s/control-selections/%d/include-controls/%d/control-id", resultPath, j, k)
				if strings.TrimSpace(controlID) == "" {
					sc.add(SemanticFailure{Reason: SemanticReviewedControls, Message: "reviewed control ID is empty", InstancePtr: controlPath})
					continue
				}
				selected++
				if _, exists := seen[controlID]; exists {
					sc.add(SemanticFailure{Reason: SemanticReviewedControls, Message: fmt.Sprintf("reviewed control %q is duplicated", controlID), InstancePtr: controlPath})
				}
				seen[controlID] = struct{}{}
			}
		}
		if len(selections) == 0 || selected == 0 && !reviewedControlsIncludeAll(selections) {
			sc.add(SemanticFailure{Reason: SemanticReviewedControls, Message: "reviewed-controls has no selected controls", InstancePtr: resultPath})
		}
	}
}

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
			for _, k := range sortedMapKeys(val) {
				child := val[k]
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
func (v *Validator) validateResultIntervals(doc map[string]any, path string, sc *semanticCollector) {
	results, _ := doc["results"].([]any)
	for i, item := range results {
		result, _ := item.(map[string]any)
		resultPath := fmt.Sprintf("%s/results/%d", path, i)
		start, startOK := parseSemanticTime(result["start"])
		end, endOK := parseSemanticTime(result["end"])
		if endOK && startOK && end.Before(start) {
			sc.add(SemanticFailure{Reason: SemanticTimestampInvalid, Message: "result end precedes start", InstancePtr: resultPath + "/end"})
		}
		observations, _ := result["observations"].([]any)
		for j, observationValue := range observations {
			observation, _ := observationValue.(map[string]any)
			collected, ok := parseSemanticTime(observation["collected"])
			if !ok || !startOK {
				continue
			}
			if collected.Before(start) || endOK && collected.After(end) {
				sc.add(SemanticFailure{Reason: SemanticTimestampInvalid, Message: "observation collection time is outside the result interval", InstancePtr: fmt.Sprintf("%s/observations/%d/collected", resultPath, j)})
			}
		}
	}
}

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
		title, _ := res["title"].(string)
		description, _ := res["description"].(string)
		if strings.TrimSpace(title) == "" && strings.TrimSpace(description) == "" {
			sc.add(SemanticFailure{
				Reason:      SemanticBackMatterResourceMissing,
				Message:     "back-matter resource is missing a title and description",
				InstancePtr: rPath,
			})
		}
		v.validateResourceProps(res, rPath, sc)
	}
}

// validateFindings checks finding targets and statuses.
func (v *Validator) validateResourceProps(resource map[string]any, path string, sc *semanticCollector) {
	props, _ := resource["props"].([]any)
	values := make(map[string]string, len(props))
	positions := make(map[string]int, len(props))
	for i, propValue := range props {
		prop, _ := propValue.(map[string]any)
		name, _ := prop["name"].(string)
		value, _ := prop["value"].(string)
		if _, exists := values[name]; exists {
			sc.add(SemanticFailure{Reason: SemanticContentAddressInvalid, Message: fmt.Sprintf("resource property %q is duplicated", name), InstancePtr: fmt.Sprintf("%s/props/%d", path, i)})
			continue
		}
		values[name] = value
		positions[name] = i
	}
	if mediaType, exists := values["media-type"]; exists && !isValidMediaType(mediaType) {
		sc.add(SemanticFailure{Reason: SemanticMediaTypeInvalid, Message: fmt.Sprintf("invalid media-type %q", mediaType), InstancePtr: resourcePropPath(path, positions, "media-type")})
	}
	artifactID, contentAddressed := values["artifact-id"]
	if !contentAddressed {
		return
	}
	artifactType, digest, ok := parseContentAddress(artifactID)
	if !ok {
		sc.add(SemanticFailure{Reason: SemanticContentAddressInvalid, Message: fmt.Sprintf("invalid artifact content address %q", artifactID), InstancePtr: resourcePropPath(path, positions, "artifact-id")})
		return
	}
	required := []string{"artifact-type", "sha256", "media-type", "schema-ref", "producer-identity", "verification-status", "scope-id", "bundle-path"}
	for _, name := range required {
		if strings.TrimSpace(values[name]) == "" {
			sc.add(SemanticFailure{Reason: SemanticContentAddressInvalid, Message: fmt.Sprintf("content-addressed resource property %q is missing", name), InstancePtr: path + "/props"})
		}
	}
	if values["artifact-type"] != artifactType {
		sc.add(SemanticFailure{Reason: SemanticContentAddressInvalid, Message: "artifact-type does not match artifact-id", InstancePtr: resourcePropPath(path, positions, "artifact-type")})
	}
	if values["sha256"] != digest {
		sc.add(SemanticFailure{Reason: SemanticContentAddressInvalid, Message: "sha256 does not match artifact-id", InstancePtr: resourcePropPath(path, positions, "sha256")})
	}
	if status := values["verification-status"]; status != "verified" && status != "unverified" && status != "failed" && status != "pending" {
		sc.add(SemanticFailure{Reason: SemanticContentAddressInvalid, Message: fmt.Sprintf("invalid verification status %q", status), InstancePtr: resourcePropPath(path, positions, "verification-status")})
	} else if status == "verified" && (strings.TrimSpace(values["verifier-id"]) == "" || strings.TrimSpace(values["verifier-version"]) == "") {
		sc.add(SemanticFailure{Reason: SemanticContentAddressInvalid, Message: "verified content-addressed resource is missing verifier identity or version", InstancePtr: path + "/props"})
	}
	for _, name := range []string{"produced-at", "verified-at"} {
		if value, exists := values[name]; exists {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				sc.add(SemanticFailure{Reason: SemanticTimestampInvalid, Message: fmt.Sprintf("invalid timestamp %q for property %q", value, name), InstancePtr: resourcePropPath(path, positions, name)})
			}
		}
	}
	for _, name := range []string{"plaintext-sha256", "authenticated-metadata-sha256"} {
		if value, exists := values[name]; exists && !isLowerSHA256(value) {
			sc.add(SemanticFailure{Reason: SemanticContentAddressInvalid, Message: fmt.Sprintf("invalid SHA-256 property %q", name), InstancePtr: resourcePropPath(path, positions, name)})
		}
	}
	encryptionProps := []string{"encryption-algorithm", "encryption-key-id", "encryption-authorization-scope", "plaintext-sha256", "authenticated-metadata-sha256"}
	hasEncryption := false
	for _, name := range encryptionProps {
		if _, exists := values[name]; exists {
			hasEncryption = true
		}
	}
	if hasEncryption {
		for _, name := range encryptionProps {
			if strings.TrimSpace(values[name]) == "" {
				sc.add(SemanticFailure{Reason: SemanticContentAddressInvalid, Message: fmt.Sprintf("encrypted content-addressed resource property %q is missing", name), InstancePtr: path + "/props"})
			}
		}
	}
}

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
		reviewedControlIDs := resultReviewedControlIDs(res)
		includeAll := resultReviewedControlsIncludeAll(res)
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
			targetID, _ := target["target-id"].(string)
			targetType, _ := target["type"].(string)
			controlID := findingTargetControlID(targetID, targetType)
			if _, exists := reviewedControlIDs[controlID]; !exists && !includeAll {
				sc.add(SemanticFailure{
					Reason:      SemanticFindingTargetMissing,
					Message:     fmt.Sprintf("finding target %q is not in reviewed controls", targetID),
					InstancePtr: fPath + "/target/target-id",
				})
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
	resourceIDs := backMatterResourceIDs(doc)
	resourceScopes, hasContentAddressedResources := backMatterResourceScopes(doc)
	subjectIDs := collectSubjectIDs(doc)
	for i, result := range results {
		res, ok := result.(map[string]any)
		if !ok {
			continue
		}
		resultPath := fmt.Sprintf("%s/results/%d", path, i)
		resultScope, hasResultScope := namedPropValue(res, "g8e-scope-id")
		if hasContentAddressedResources && (!hasResultScope || strings.TrimSpace(resultScope) == "") {
			sc.add(SemanticFailure{Reason: SemanticEvidenceScopeMismatch, Message: "result with content-addressed g8e evidence is missing g8e-scope-id", InstancePtr: resultPath + "/props"})
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
			oPath := fmt.Sprintf("%s/observations/%d", resultPath, j)
			// Check subjects
			subjects, ok := observation["subjects"].([]any)
			if !ok || len(subjects) == 0 {
				sc.add(SemanticFailure{
					Reason:      SemanticObsSubjectMissing,
					Message:     "observation has no subjects",
					InstancePtr: oPath + "/subjects",
				})
			}
			for k, subjectValue := range subjects {
				subject, _ := subjectValue.(map[string]any)
				subjectUUID, _ := subject["subject-uuid"].(string)
				subjectType, _ := subject["type"].(string)
				// When the document defines no subjects of the declared type
				// locally, the subject may be defined in an externally
				// imported assessment plan (import-ap with a non-fragment
				// href). Skip resolution in that case rather than failing
				// on a reference the validator cannot reach.
				if len(subjectIDs[subjectType]) == 0 {
					continue
				}
				if !subjectIDs[subjectType][subjectUUID] {
					sc.add(SemanticFailure{
						Reason:      SemanticObsSubjectMissing,
						Message:     fmt.Sprintf("observation subject %q of type %q does not resolve", subjectUUID, subjectType),
						InstancePtr: fmt.Sprintf("%s/subjects/%d/subject-uuid", oPath, k),
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
							if !resourceIDs[refID] {
								sc.add(SemanticFailure{
									Reason:      SemanticObsEvidenceUnresolved,
									Message:     fmt.Sprintf("evidence href %q does not resolve to a back-matter resource", href),
									InstancePtr: fmt.Sprintf("%s/relevant-evidence/%d/href", oPath, k),
								})
							} else if resourceScope, exists := resourceScopes[refID]; exists && hasResultScope && resourceScope != resultScope {
								sc.add(SemanticFailure{
									Reason:      SemanticEvidenceScopeMismatch,
									Message:     fmt.Sprintf("evidence scope %q does not match result scope %q", resourceScope, resultScope),
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

func backMatterResourceIDs(doc map[string]any) map[string]bool {
	ids := make(map[string]bool)
	backMatter, _ := doc["back-matter"].(map[string]any)
	resources, _ := backMatter["resources"].([]any)
	for _, value := range resources {
		resource, _ := value.(map[string]any)
		if id, ok := resource["uuid"].(string); ok {
			ids[id] = true
		}
	}
	return ids
}

func backMatterResourceScopes(doc map[string]any) (map[string]string, bool) {
	scopes := make(map[string]string)
	contentAddressed := false
	backMatter, _ := doc["back-matter"].(map[string]any)
	resources, _ := backMatter["resources"].([]any)
	for _, value := range resources {
		resource, _ := value.(map[string]any)
		if _, hasArtifactID := namedPropValue(resource, "artifact-id"); !hasArtifactID {
			continue
		}
		contentAddressed = true
		id, _ := resource["uuid"].(string)
		scope, _ := namedPropValue(resource, "scope-id")
		scopes[id] = scope
	}
	return scopes, contentAddressed
}

func namedPropValue(record map[string]any, name string) (string, bool) {
	props, _ := record["props"].([]any)
	for _, value := range props {
		prop, _ := value.(map[string]any)
		if propName, _ := prop["name"].(string); propName == name {
			propValue, ok := prop["value"].(string)
			return propValue, ok
		}
	}
	return "", false
}

func reviewedControlsIncludeAll(selections []any) bool {
	for _, value := range selections {
		selection, _ := value.(map[string]any)
		if _, ok := selection["include-all"]; ok {
			return true
		}
	}
	return false
}

func resultReviewedControlIDs(result map[string]any) map[string]struct{} {
	ids := make(map[string]struct{})
	reviewed, _ := result["reviewed-controls"].(map[string]any)
	selections, _ := reviewed["control-selections"].([]any)
	for _, selectionValue := range selections {
		selection, _ := selectionValue.(map[string]any)
		controls, _ := selection["include-controls"].([]any)
		for _, controlValue := range controls {
			control, _ := controlValue.(map[string]any)
			if id, ok := control["control-id"].(string); ok {
				ids[id] = struct{}{}
			}
		}
	}
	return ids
}

func resultReviewedControlsIncludeAll(result map[string]any) bool {
	reviewed, _ := result["reviewed-controls"].(map[string]any)
	selections, _ := reviewed["control-selections"].([]any)
	return reviewedControlsIncludeAll(selections)
}

// findingTargetControlID derives the parent control ID from a finding
// target-id. OSCAL finding targets use three target types: "control-id"
// (the target-id is the control ID directly), "objective-id" (the
// target-id has the form "<control-id>_obj"), and "statement-id" (the
// target-id has the form "<control-id>_smt.<suffix>" or
// "<control-id>_smt"). For any other target type, the target-id is
// returned unchanged and the caller decides whether it must match a
// reviewed control.
func findingTargetControlID(targetID, targetType string) string {
	switch targetType {
	case "objective-id":
		if idx := strings.LastIndex(targetID, "_obj"); idx > 0 {
			return targetID[:idx]
		}
	case "statement-id":
		if idx := strings.Index(targetID, "_smt"); idx > 0 {
			return targetID[:idx]
		}
	}
	return targetID
}

func parseSemanticTime(value any) (time.Time, bool) {
	timestamp, ok := value.(string)
	if !ok {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, timestamp)
	return parsed, err == nil
}

func collectSubjectIDs(doc map[string]any) map[string]map[string]bool {
	ids := map[string]map[string]bool{
		"component":      {},
		"inventory-item": {},
		"location":       {},
		"party":          {},
		"user":           {},
		"resource":       {},
	}
	collectionTypes := map[string]string{
		"components":      "component",
		"inventory-items": "inventory-item",
		"locations":       "location",
		"parties":         "party",
		"users":           "user",
		"resources":       "resource",
	}
	var walk func(any)
	walk = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			for key, child := range node {
				if subjectType, ok := collectionTypes[key]; ok {
					items, _ := child.([]any)
					for _, item := range items {
						record, _ := item.(map[string]any)
						if id, ok := record["uuid"].(string); ok {
							ids[subjectType][id] = true
						}
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(doc)
	return ids
}

func parseContentAddress(value string) (string, string, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || parts[1] != "sha256" || !isLowerSHA256(parts[2]) {
		return "", "", false
	}
	return parts[0], parts[2], true
}

func isLowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func resourcePropPath(path string, positions map[string]int, name string) string {
	position, ok := positions[name]
	if !ok {
		return path + "/props"
	}
	return fmt.Sprintf("%s/props/%d/value", path, position)
}

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

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func escapeToken(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}
