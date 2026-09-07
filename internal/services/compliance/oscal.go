// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// ---------------------------------------------------------------------------
// OSCAL component-definition
// ---------------------------------------------------------------------------

// OSCALComponentDefinition is the top-level OSCAL component-definition model.
// It describes the g8e platform as a component with control implementations
// keyed by KSI category.
type OSCALComponentDefinition struct {
	UUID       string           `json:"uuid"`
	Metadata   OSCALMetadata    `json:"metadata"`
	Components []OSCALComponent `json:"components"`
	BackMatter OSCALBackMatter  `json:"back-matter,omitempty"`
}

// OSCALMetadata holds common OSCAL metadata fields.
type OSCALMetadata struct {
	Title        string `json:"title"`
	Published    string `json:"published"`
	LastModified string `json:"last-modified"`
	Version      string `json:"version"`
	OscalVersion string `json:"oscal-version"`
}

// OSCALComponent describes a single component (g8e platform).
type OSCALComponent struct {
	UUID                   string                       `json:"uuid"`
	Type                   string                       `json:"type"`
	Title                  string                       `json:"title"`
	Description            string                       `json:"description"`
	ControlImplementations []OSCALControlImplementation `json:"control-implementations,omitempty"`
}

// OSCALControlImplementation groups implemented controls by KSI category.
type OSCALControlImplementation struct {
	UUID                string                    `json:"uuid"`
	Source              string                    `json:"source"`
	Description         string                    `json:"description"`
	ImplementedControls []OSCALImplementedControl `json:"implemented-requirements"`
}

// OSCALImplementedControl describes a single implemented control (per KSI).
type OSCALImplementedControl struct {
	ControlID   string           `json:"control-id"`
	Description string           `json:"description,omitempty"`
	Statements  []OSCALStatement `json:"statements,omitempty"`
}

// OSCALStatement links a KSI to a control statement with method evidence.
type OSCALStatement struct {
	StatementID string `json:"statement-id"`
	Description string `json:"description,omitempty"`
}

// ---------------------------------------------------------------------------
// OSCAL assessment-results
// ---------------------------------------------------------------------------

// OSCALAssessmentResults is the top-level OSCAL assessment-results model.
// It contains per-KSI observations and results with evidence anchors.
type OSCALAssessmentResults struct {
	AssessmentResults OSCALAssessmentResultsBody `json:"assessment-results"`
}

type OSCALAssessmentResultsBody struct {
	UUID       string          `json:"uuid"`
	Metadata   OSCALMetadata   `json:"metadata"`
	ImportAP   OSCALImportAP   `json:"import-ap"`
	Results    []OSCALResult   `json:"results"`
	BackMatter OSCALBackMatter `json:"back-matter,omitempty"`
}

type OSCALImportAP struct {
	Href string `json:"href"`
}

// OSCALResult holds the assessment results for a single evaluation run.
type OSCALResult struct {
	UUID             string                 `json:"uuid"`
	Title            string                 `json:"title"`
	Description      string                 `json:"description,omitempty"`
	Start            string                 `json:"start"`
	End              string                 `json:"end,omitempty"`
	ReviewedControls *OSCALReviewedControls `json:"reviewed-controls"`
	Observations     []OSCALObservation     `json:"observations,omitempty"`
	Findings         []OSCALFinding         `json:"findings,omitempty"`
}

type OSCALReviewedControls struct {
	ControlSelections []OSCALControlSelection `json:"control-selections,omitempty"`
}

type OSCALControlSelection struct {
	IncludeControls []OSCALControlID `json:"include-controls,omitempty"`
}

type OSCALControlID struct {
	ControlID string `json:"control-id"`
}

// OSCALObservation records evidence for a single KSI evaluation.
type OSCALObservation struct {
	UUID             string                  `json:"uuid"`
	Title            string                  `json:"title"`
	Description      string                  `json:"description"`
	Methods          []string                `json:"methods,omitempty"`
	Props            []OSCALProp             `json:"props,omitempty"`
	Subjects         []OSCALSubject          `json:"subjects,omitempty"`
	RelevantEvidence []OSCALRelevantEvidence `json:"relevant-evidence,omitempty"`
	Collected        string                  `json:"collected"`
}

// OSCALSubject references the KSI being assessed.
type OSCALSubject struct {
	SubjectUUID string `json:"subject-uuid"`
	Type        string `json:"type"`
	Title       string `json:"title,omitempty"`
}

// OSCALRelevantEvidence anchors evidence to g8e artifacts (receipts, ledger, LFAA).
type OSCALRelevantEvidence struct {
	Href        string `json:"href"`
	Description string `json:"description"`
}

// OSCALFinding records the pass/fail result for a KSI.
type OSCALFinding struct {
	UUID        string             `json:"uuid"`
	Title       string             `json:"title"`
	Description string             `json:"description,omitempty"`
	Target      OSCALFindingTarget `json:"target"`
}

// OSCALFindingTarget references the control and KSI being assessed.
type OSCALFindingTarget struct {
	Type     string                   `json:"type"`
	TargetID string                   `json:"target-id"`
	Status   OSCALFindingTargetStatus `json:"status"`
}

type OSCALFindingTargetStatus struct {
	State string `json:"state"`
}

// OSCALBackMatter holds references and resources.
type OSCALBackMatter struct {
	Resources []OSCALResource `json:"resources,omitempty"`
}

// OSCALResource describes a referenced catalog or content-addressed evidence resource.
type OSCALResource struct {
	UUID        string      `json:"uuid"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	Props       []OSCALProp `json:"props,omitempty"`
}

// OSCALProp is a key-value property.
type OSCALProp struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ---------------------------------------------------------------------------
// Exporter
// ---------------------------------------------------------------------------

const (
	oscalUUIDNamespaceName      = "g8e.ai/compliance/oscal/v1"
	oscalEvidenceLinkReferences = "references"
)

// OSCALExporter generates component definitions from the KSI catalog and assessment results from canonical compliance analysis.
type OSCALExporter struct {
	catalog *KSICatalog
}

// NewOSCALExporter creates a new exporter for the given catalog.
func NewOSCALExporter(catalog *KSICatalog) *OSCALExporter {
	return &OSCALExporter{catalog: catalog}
}

// GenerateComponentDefinition produces an OSCAL component-definition document
// describing the g8e platform and its control implementations, one per KSI
// category.
func (e *OSCALExporter) GenerateComponentDefinition() (*OSCALComponentDefinition, error) {
	if e.catalog == nil {
		return nil, fmt.Errorf("%w: nil KSI catalog", constants.ErrValidationFailed)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Group KSIs by category to build control-implementations.
	categoryKSIs := make(map[KSICategory][]KSI)
	var categories []KSICategory
	for _, ksi := range e.catalog.KSIs {
		if _, exists := categoryKSIs[ksi.Category]; !exists {
			categories = append(categories, ksi.Category)
		}
		categoryKSIs[ksi.Category] = append(categoryKSIs[ksi.Category], ksi)
	}
	sort.Slice(categories, func(i, j int) bool {
		return string(categories[i]) < string(categories[j])
	})

	var controlImpls []OSCALControlImplementation
	for _, cat := range categories {
		ksis := categoryKSIs[cat]

		// Group implemented controls by control-id, merging statements from
		// multiple KSIs that reference the same control. OSCAL 1.1.2 requires
		// control-id to be unique within a control-implementation.
		implemented := make([]OSCALImplementedControl, 0)
		controlIndex := make(map[string]int)
		for _, ksi := range ksis {
			if len(ksi.ControlRefs) == 0 {
				return nil, fmt.Errorf("%w: KSI %s has no control refs", constants.ErrValidationFailed, ksi.ID)
			}
			var statements []OSCALStatement
			for _, m := range ksi.AutomatedMethods {
				statements = append(statements, OSCALStatement{
					StatementID: ksi.ID + ":" + m.Name,
					Description: m.Description,
				})
			}
			for _, controlRef := range ksi.ControlRefs {
				if idx, exists := controlIndex[controlRef]; exists {
					implemented[idx].Statements = append(implemented[idx].Statements, statements...)
					implemented[idx].Description = implemented[idx].Description + "; " + ksi.Title
				} else {
					controlIndex[controlRef] = len(implemented)
					implemented = append(implemented, OSCALImplementedControl{
						ControlID:   controlRef,
						Description: ksi.Title,
						Statements:  statements,
					})
				}
			}
		}
		controlImpls = append(controlImpls, OSCALControlImplementation{
			UUID:                generateUUID("control-implementation", e.catalog.Source, e.catalog.Version, string(cat)),
			Source:              "FedRAMP 20x KSI catalog (CR26)",
			Description:         "g8e control implementations for KSI category " + string(cat),
			ImplementedControls: implemented,
		})
	}

	return &OSCALComponentDefinition{
		UUID: generateUUID("component-definition", e.catalog.Source, e.catalog.Version),
		Metadata: OSCALMetadata{
			Title:        "g8e Platform Component Definition",
			Published:    now,
			LastModified: now,
			Version:      e.catalog.Version,
			OscalVersion: "1.1.2",
		},
		Components: []OSCALComponent{
			{
				UUID:                   generateUUID("component", e.catalog.Source, e.catalog.Version, "g8e-platform"),
				Type:                   "software",
				Title:                  "g8e Zero-Trust Execution Platform",
				Description:            "g8e is a zero-trust execution platform for agentic infrastructure. Mutations are typed, signed, state-bound, and verified through a 5-layer gauntlet.",
				ControlImplementations: controlImpls,
			},
		},
		BackMatter: OSCALBackMatter{
			Resources: []OSCALResource{
				{
					UUID:        generateUUID("catalog-resource", e.catalog.Source, e.catalog.Version),
					Title:       "FedRAMP 20x KSI Catalog",
					Description: "CR26 Key Security Indicators reference catalog",
					Props: []OSCALProp{
						{Name: "source", Value: e.catalog.Source},
						{Name: "version", Value: e.catalog.Version},
					},
				},
			},
		},
	}, nil
}

// GenerateAssessmentResults produces an OSCAL assessment-results document from
// canonical compliance analysis. Assertion assessments become observations,
// framework control assessments become findings, and every evidence anchor
// resolves to a typed content-addressed resource in back matter.
func (e *OSCALExporter) GenerateAssessmentResults(analysis *compliancev1.ComplianceAnalysis) (*OSCALAssessmentResults, error) {
	if err := validateOSCALAnalysis(analysis); err != nil {
		return nil, err
	}

	resources, resourceIndex, err := buildOSCALEvidenceResources(analysis)
	if err != nil {
		return nil, err
	}
	observations, err := buildOSCALObservations(analysis, resourceIndex)
	if err != nil {
		return nil, err
	}
	findings, err := buildOSCALFindings(analysis)
	if err != nil {
		return nil, err
	}
	generatedAt := analysis.GetGeneratedAt().AsTime().UTC().Format(time.RFC3339)
	document := &OSCALAssessmentResults{
		AssessmentResults: OSCALAssessmentResultsBody{
			UUID: generateUUID("assessment-results", analysis.GetAnalysisId()),
			Metadata: OSCALMetadata{
				Title:        "g8e Compliance Assessment Results",
				Published:    generatedAt,
				LastModified: generatedAt,
				Version:      analysis.GetAnalysisSchemaVersion(),
				OscalVersion: "1.1.2",
			},
			ImportAP: OSCALImportAP{Href: "urn:g8e:assessment-plan:" + analysis.GetAnalysisId()},
			Results: []OSCALResult{{
				UUID:             generateUUID("result", analysis.GetAnalysisId(), analysis.GetScopeRef()),
				Title:            "Compliance Assessment for " + analysis.GetScopeRef(),
				Description:      "Canonical cross-framework compliance analysis " + analysis.GetAnalysisId(),
				Start:            generatedAt,
				ReviewedControls: buildOSCALReviewedControls(findings),
				Observations:     observations,
				Findings:         findings,
			}},
			BackMatter: OSCALBackMatter{Resources: resources},
		},
	}
	if err := validateOSCALAssessmentResults(document); err != nil {
		return nil, err
	}
	validator, err := NewOSCALDocumentValidator()
	if err != nil {
		return nil, err
	}
	validation, err := validator.ValidateAssessmentResults(document)
	if err != nil {
		return nil, err
	}
	if !validation.GetValid() {
		return nil, fmt.Errorf("%w: %d structural failures and %d semantic failures", constants.ErrOSCALValidationFailed, len(validation.GetStructuralFailures()), len(validation.GetSemanticFailures()))
	}
	return document, nil
}

func validateOSCALAnalysis(analysis *compliancev1.ComplianceAnalysis) error {
	if analysis == nil {
		return fmt.Errorf("%w: nil compliance analysis", constants.ErrValidationFailed)
	}
	if analysis.GetAnalysisId() == "" || analysis.GetAnalysisSchemaVersion() == "" || analysis.GetScopeRef() == "" || analysis.GetGeneratorIdentity() == "" || analysis.GetGeneratorVersion() == "" {
		return fmt.Errorf("%w: compliance analysis identity is incomplete", constants.ErrValidationFailed)
	}
	if analysis.GetGeneratedAt() == nil || analysis.GetGeneratedAt().CheckValid() != nil {
		return fmt.Errorf("%w: compliance analysis generation time is invalid", constants.ErrValidationFailed)
	}
	if !analysis.GetEvidenceGraphValid() || len(analysis.GetEvidenceGraphFailures()) != 0 {
		return fmt.Errorf("%w: compliance analysis evidence graph is invalid", constants.ErrInvalidEvidenceGraph)
	}
	return nil
}

func validateOSCALAssessmentResults(document *OSCALAssessmentResults) error {
	if document == nil {
		return fmt.Errorf("%w: nil OSCAL assessment-results document", constants.ErrValidationFailed)
	}
	body := document.AssessmentResults
	if _, err := uuid.Parse(body.UUID); err != nil || body.Metadata.Title == "" || body.Metadata.LastModified == "" || body.Metadata.Version == "" || body.Metadata.OscalVersion != "1.1.2" || body.ImportAP.Href == "" || len(body.Results) == 0 {
		return fmt.Errorf("%w: OSCAL assessment-results root is incomplete", constants.ErrValidationFailed)
	}
	resourceUUIDs := make(map[string]struct{}, len(body.BackMatter.Resources))
	for _, resource := range body.BackMatter.Resources {
		if _, err := uuid.Parse(resource.UUID); err != nil {
			return fmt.Errorf("%w: OSCAL resource UUID is invalid", constants.ErrValidationFailed)
		}
		resourceUUIDs[resource.UUID] = struct{}{}
	}
	for _, result := range body.Results {
		if _, err := uuid.Parse(result.UUID); err != nil || result.Title == "" || result.Description == "" || result.Start == "" || result.ReviewedControls == nil || len(result.ReviewedControls.ControlSelections) == 0 {
			return fmt.Errorf("%w: OSCAL result is incomplete", constants.ErrValidationFailed)
		}
		for _, selection := range result.ReviewedControls.ControlSelections {
			if len(selection.IncludeControls) == 0 {
				return fmt.Errorf("%w: OSCAL reviewed controls are empty", constants.ErrValidationFailed)
			}
			for _, control := range selection.IncludeControls {
				if control.ControlID == "" {
					return fmt.Errorf("%w: OSCAL reviewed control ID is empty", constants.ErrValidationFailed)
				}
			}
		}
		for _, observation := range result.Observations {
			_, collectedErr := time.Parse(time.RFC3339, observation.Collected)
			if _, err := uuid.Parse(observation.UUID); err != nil || observation.Title == "" || observation.Description == "" || len(observation.Methods) == 0 || collectedErr != nil {
				return fmt.Errorf("%w: OSCAL observation is incomplete", constants.ErrValidationFailed)
			}
			for _, method := range observation.Methods {
				if method != "EXAMINE" && method != "INTERVIEW" && method != "TEST" && method != "UNKNOWN" {
					return fmt.Errorf("%w: OSCAL observation method %q is invalid", constants.ErrValidationFailed, method)
				}
			}
			for _, evidence := range observation.RelevantEvidence {
				resourceUUID := strings.TrimPrefix(evidence.Href, "#")
				if resourceUUID == evidence.Href {
					continue
				}
				if _, exists := resourceUUIDs[resourceUUID]; !exists {
					return fmt.Errorf("%w: OSCAL relevant evidence %s", constants.ErrUnresolvedReference, evidence.Href)
				}
			}
		}
		for _, finding := range result.Findings {
			state := finding.Target.Status.State
			if _, err := uuid.Parse(finding.UUID); err != nil || finding.Title == "" || finding.Description == "" || (finding.Target.Type != "statement-id" && finding.Target.Type != "objective-id") || finding.Target.TargetID == "" || (state != "satisfied" && state != "not-satisfied") {
				return fmt.Errorf("%w: OSCAL finding is incomplete", constants.ErrValidationFailed)
			}
		}
	}
	return nil
}

func buildOSCALReviewedControls(findings []OSCALFinding) *OSCALReviewedControls {
	seen := make(map[string]struct{}, len(findings))
	controls := make([]OSCALControlID, 0, len(findings))
	for _, finding := range findings {
		controlID := finding.Target.TargetID
		if _, exists := seen[controlID]; exists {
			continue
		}
		seen[controlID] = struct{}{}
		controls = append(controls, OSCALControlID{ControlID: controlID})
	}
	sort.Slice(controls, func(i, j int) bool {
		return controls[i].ControlID < controls[j].ControlID
	})
	if len(controls) == 0 {
		return &OSCALReviewedControls{}
	}
	return &OSCALReviewedControls{ControlSelections: []OSCALControlSelection{{IncludeControls: controls}}}
}

func buildOSCALEvidenceResources(analysis *compliancev1.ComplianceAnalysis) ([]OSCALResource, map[string]OSCALResource, error) {
	evidenceResources := append([]*compliancev1.ComplianceEvidenceReference(nil), analysis.GetEvidenceResources()...)
	sort.Slice(evidenceResources, func(i, j int) bool {
		return evidenceResources[i].GetArtifactId() < evidenceResources[j].GetArtifactId()
	})
	resources := make([]OSCALResource, 0, len(evidenceResources))
	index := make(map[string]OSCALResource, len(evidenceResources))
	for _, evidenceResource := range evidenceResources {
		if evidenceResource == nil {
			return nil, nil, fmt.Errorf("%w: nil evidence resource", constants.ErrUnresolvedReference)
		}
		artifactType, digest, ok := parseOSCALContentAddress(evidenceResource.GetArtifactId())
		if !ok || artifactType != evidenceResource.GetArtifactType() || digest != evidenceResource.GetSha256() {
			return nil, nil, fmt.Errorf("%w: invalid content-addressed evidence resource %s", constants.ErrUnresolvedReference, evidenceResource.GetArtifactId())
		}
		if evidenceResource.GetMediaType() == "" || evidenceResource.GetSchemaRef() == "" || evidenceResource.GetProducerIdentity() == "" || evidenceResource.GetVerificationStatus() == "" || evidenceResource.GetBundlePath() == "" {
			return nil, nil, fmt.Errorf("%w: incomplete evidence resource %s", constants.ErrUnresolvedReference, evidenceResource.GetArtifactId())
		}
		if evidenceResource.GetScopeId() != analysis.GetScopeRef() {
			return nil, nil, fmt.Errorf("%w: evidence resource %s belongs to scope %s", constants.ErrEvidenceScopeMismatch, evidenceResource.GetArtifactId(), evidenceResource.GetScopeId())
		}
		if _, exists := index[evidenceResource.GetArtifactId()]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate evidence resource %s", constants.ErrEvidenceDuplicateID, evidenceResource.GetArtifactId())
		}
		resource := OSCALResource{
			UUID:        generateUUID("evidence-resource", analysis.GetAnalysisId(), evidenceResource.GetArtifactId()),
			Title:       evidenceResource.GetArtifactType() + " evidence",
			Description: "Content-addressed evidence produced by " + evidenceResource.GetProducerIdentity(),
			Props:       oscalEvidenceProps(evidenceResource),
		}
		resources = append(resources, resource)
		index[evidenceResource.GetArtifactId()] = resource
	}
	linkIDs := make(map[string]struct{}, len(analysis.GetEvidenceLinks()))
	for _, link := range analysis.GetEvidenceLinks() {
		if link == nil || link.GetLinkType() != oscalEvidenceLinkReferences {
			return nil, nil, fmt.Errorf("%w: invalid evidence link", constants.ErrUnresolvedReference)
		}
		if _, exists := index[link.GetSourceRef()]; !exists {
			return nil, nil, fmt.Errorf("%w: evidence link source %s", constants.ErrUnresolvedReference, link.GetSourceRef())
		}
		if _, exists := index[link.GetTargetRef()]; !exists {
			return nil, nil, fmt.Errorf("%w: evidence link target %s", constants.ErrUnresolvedReference, link.GetTargetRef())
		}
		linkID := link.GetSourceRef() + "\x00" + link.GetTargetRef() + "\x00" + link.GetLinkType()
		if _, exists := linkIDs[linkID]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate evidence link", constants.ErrEvidenceDuplicateID)
		}
		linkIDs[linkID] = struct{}{}
	}
	return resources, index, nil
}

func oscalEvidenceProps(resource *compliancev1.ComplianceEvidenceReference) []OSCALProp {
	values := []OSCALProp{
		{Name: "artifact-id", Value: resource.GetArtifactId()},
		{Name: "artifact-type", Value: resource.GetArtifactType()},
		{Name: "sha256", Value: resource.GetSha256()},
		{Name: "media-type", Value: resource.GetMediaType()},
		{Name: "schema-ref", Value: resource.GetSchemaRef()},
		{Name: "producer-identity", Value: resource.GetProducerIdentity()},
		{Name: "verification-status", Value: resource.GetVerificationStatus()},
		{Name: "verifier-id", Value: resource.GetVerifierId()},
		{Name: "verifier-version", Value: resource.GetVerifierVersion()},
		{Name: "scope-id", Value: resource.GetScopeId()},
		{Name: "run-id", Value: resource.GetRunId()},
		{Name: "attempt-id", Value: resource.GetAttemptId()},
		{Name: "scenario-id", Value: resource.GetScenarioId()},
		{Name: "transaction-id", Value: resource.GetTransactionId()},
		{Name: "bundle-path", Value: resource.GetBundlePath()},
	}
	props := make([]OSCALProp, 0, len(values)+7)
	for _, value := range values {
		if value.Value != "" {
			props = append(props, value)
		}
	}
	if producedAt := resource.GetProducedAt(); producedAt != nil && producedAt.CheckValid() == nil {
		props = append(props, OSCALProp{Name: "produced-at", Value: producedAt.AsTime().UTC().Format(time.RFC3339)})
	}
	if verifiedAt := resource.GetVerifiedAt(); verifiedAt != nil && verifiedAt.CheckValid() == nil {
		props = append(props, OSCALProp{Name: "verified-at", Value: verifiedAt.AsTime().UTC().Format(time.RFC3339)})
	}
	if encryption := resource.GetEncryption(); encryption != nil {
		encryptionValues := []OSCALProp{
			{Name: "encryption-algorithm", Value: encryption.GetAlgorithm()},
			{Name: "encryption-key-id", Value: encryption.GetKeyId()},
			{Name: "encryption-authorization-scope", Value: encryption.GetAuthorizationScope()},
			{Name: "plaintext-sha256", Value: encryption.GetPlaintextSha256()},
			{Name: "authenticated-metadata-sha256", Value: encryption.GetAuthenticatedMetadataSha256()},
		}
		for _, value := range encryptionValues {
			if value.Value != "" {
				props = append(props, value)
			}
		}
	}
	return props
}

func buildOSCALObservations(analysis *compliancev1.ComplianceAnalysis, resourceIndex map[string]OSCALResource) ([]OSCALObservation, error) {
	assessments := append([]*compliancev1.ControlAssertionAssessment(nil), analysis.GetAssertionAssessments()...)
	sort.Slice(assessments, func(i, j int) bool {
		return assessments[i].GetAssessmentId() < assessments[j].GetAssessmentId()
	})
	observations := make([]OSCALObservation, 0, len(assessments))
	for _, assessment := range assessments {
		if assessment == nil || assessment.GetAssessmentId() == "" || assessment.GetAssertionRef() == nil || assessment.GetVerifierRef() == nil || assessment.GetEvaluatedAt() == nil || assessment.GetEvaluatedAt().CheckValid() != nil || assessment.GetScopeId() != analysis.GetScopeRef() {
			return nil, fmt.Errorf("%w: assertion assessment is incomplete or cross-scope", constants.ErrValidationFailed)
		}
		evidenceRefs := append([]string(nil), assessment.GetEvidenceRefs()...)
		evidenceRefs = append(evidenceRefs, assessment.GetMetricRefs()...)
		sort.Strings(evidenceRefs)
		relevantEvidence := make([]OSCALRelevantEvidence, 0, len(evidenceRefs))
		seen := make(map[string]struct{}, len(evidenceRefs))
		for _, evidenceRef := range evidenceRefs {
			if _, exists := seen[evidenceRef]; exists {
				continue
			}
			seen[evidenceRef] = struct{}{}
			resource, exists := resourceIndex[evidenceRef]
			if !exists {
				return nil, fmt.Errorf("%w: assertion assessment %s evidence %s", constants.ErrUnresolvedReference, assessment.GetAssessmentId(), evidenceRef)
			}
			relevantEvidence = append(relevantEvidence, OSCALRelevantEvidence{Href: "#" + resource.UUID, Description: evidenceRef})
		}
		methodID := assessment.GetVerifierRef().GetId()
		if assessment.GetVerifierRef().GetVersion() != "" {
			methodID += "@" + assessment.GetVerifierRef().GetVersion()
		}
		observations = append(observations, OSCALObservation{
			UUID:        generateUUID("observation", analysis.GetAnalysisId(), assessment.GetAssessmentId(), assessment.GetAssertionRef().GetId(), assessment.GetAssertionRef().GetVersion()),
			Title:       "Assertion " + assessment.GetAssertionRef().GetId() + " Assessment",
			Description: fmt.Sprintf("%s at evidence level %s with %s evidence", assessment.GetStatus(), assessment.GetEvidenceLevel(), assessment.GetFreshnessStatus()),
			Methods:     []string{"TEST"},
			Props:       []OSCALProp{{Name: "g8e-verifier", Value: methodID}},
			Collected:   assessment.GetEvaluatedAt().AsTime().UTC().Format(time.RFC3339),
			Subjects: []OSCALSubject{{
				SubjectUUID: generateUUID("assertion-subject", analysis.GetAnalysisId(), assessment.GetAssessmentId(), assessment.GetAssertionRef().GetId(), assessment.GetAssertionRef().GetVersion()),
				Type:        "component",
				Title:       assessment.GetAssessmentId(),
			}},
			RelevantEvidence: relevantEvidence,
		})
	}
	return observations, nil
}

func buildOSCALFindings(analysis *compliancev1.ComplianceAnalysis) ([]OSCALFinding, error) {
	assessments := append([]*compliancev1.FrameworkControlAssessment(nil), analysis.GetFrameworkAssessments()...)
	sort.Slice(assessments, func(i, j int) bool {
		return assessments[i].GetAssessmentId() < assessments[j].GetAssessmentId()
	})
	assertionAssessments := make(map[string]struct{}, len(analysis.GetAssertionAssessments()))
	for _, assessment := range analysis.GetAssertionAssessments() {
		if assessment != nil {
			assertionAssessments[assessment.GetAssessmentId()] = struct{}{}
		}
	}
	findings := make([]OSCALFinding, 0, len(assessments))
	for _, assessment := range assessments {
		if assessment == nil || assessment.GetAssessmentId() == "" || assessment.GetFrameworkRef() == nil || assessment.GetControlId() == "" || assessment.GetScopeId() != analysis.GetScopeRef() {
			return nil, fmt.Errorf("%w: framework assessment is incomplete or cross-scope", constants.ErrValidationFailed)
		}
		for _, assertionRef := range assessment.GetAssertionAssessmentRefs() {
			if _, exists := assertionAssessments[assertionRef]; !exists {
				return nil, fmt.Errorf("%w: framework assessment %s assertion %s", constants.ErrUnresolvedReference, assessment.GetAssessmentId(), assertionRef)
			}
		}
		findings = append(findings, OSCALFinding{
			UUID:        generateUUID("finding", analysis.GetAnalysisId(), assessment.GetAssessmentId(), assessment.GetFrameworkRef().GetId(), assessment.GetFrameworkRef().GetVersion(), assessment.GetControlId()),
			Title:       assessment.GetFrameworkRef().GetId() + " " + assessment.GetControlId() + " Finding",
			Description: fmt.Sprintf("%s at evidence level %s; responsibility: %s", assessment.GetStatus(), assessment.GetEvidenceLevel(), assessment.GetResponsibility()),
			Target: OSCALFindingTarget{
				Type:     "objective-id",
				TargetID: assessment.GetControlId(),
				Status:   OSCALFindingTargetStatus{State: oscalFindingStatus(assessment.GetStatus())},
			},
		})
	}
	return findings, nil
}

func parseOSCALContentAddress(address string) (string, string, bool) {
	parts := strings.Split(address, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "sha256" || len(parts[2]) != 64 || strings.ToLower(parts[2]) != parts[2] {
		return "", "", false
	}
	for _, character := range parts[2] {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return "", "", false
		}
	}
	return parts[0], parts[2], true
}

func oscalFindingStatus(status string) string {
	switch status {
	case "satisfied":
		return "satisfied"
	default:
		return "not-satisfied"
	}
}

// generateUUID derives a deterministic RFC 4122 UUID v5 from a record kind and length-delimited canonical identities.
func generateUUID(kind string, identities ...string) string {
	var name strings.Builder
	name.WriteString(strconv.Itoa(len(kind)))
	name.WriteByte(':')
	name.WriteString(kind)
	for _, identity := range identities {
		name.WriteString(strconv.Itoa(len(identity)))
		name.WriteByte(':')
		name.WriteString(identity)
	}
	namespace := uuid.NewSHA1(uuid.NameSpaceOID, []byte(oscalUUIDNamespaceName))
	return uuid.NewSHA1(namespace, []byte(name.String())).String()
}
