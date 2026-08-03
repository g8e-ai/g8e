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
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/internal/constants"
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
	UUID     string        `json:"uuid"`
	Metadata OSCALMetadata `json:"metadata"`
	Results  []OSCALResult `json:"results"`
}

// OSCALResult holds the assessment results for a single evaluation run.
type OSCALResult struct {
	UUID         string             `json:"uuid"`
	Title        string             `json:"title"`
	Description  string             `json:"description,omitempty"`
	Start        string             `json:"start"`
	End          string             `json:"end,omitempty"`
	Observations []OSCALObservation `json:"observations,omitempty"`
	Findings     []OSCALFinding     `json:"findings,omitempty"`
}

// OSCALObservation records evidence for a single KSI evaluation.
type OSCALObservation struct {
	UUID             string                  `json:"uuid"`
	Title            string                  `json:"title"`
	Description      string                  `json:"description"`
	Methods          []OSCALMethodRef        `json:"methods,omitempty"`
	Subjects         []OSCALSubject          `json:"subjects,omitempty"`
	RelevantEvidence []OSCALRelevantEvidence `json:"relevant-evidence,omitempty"`
}

// OSCALMethodRef references the assessment method used.
type OSCALMethodRef struct {
	MethodID string `json:"method-id"`
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
	TargetID string `json:"target-id"`
	Status   string `json:"status"`
}

// OSCALBackMatter holds references and resources.
type OSCALBackMatter struct {
	Resources []OSCALResource `json:"resources,omitempty"`
}

// OSCALResource describes a referenced resource (KSI catalog).
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

// OSCALExporter generates OSCAL JSON from a KSI catalog and evaluation results.
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
		var implemented []OSCALImplementedControl
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
				implemented = append(implemented, OSCALImplementedControl{
					ControlID:   controlRef,
					Description: ksi.Title,
					Statements:  statements,
				})
			}
		}
		controlImpls = append(controlImpls, OSCALControlImplementation{
			UUID:                generateUUID(),
			Source:              "FedRAMP 20x KSI catalog (CR26)",
			Description:         "g8e control implementations for KSI category " + string(cat),
			ImplementedControls: implemented,
		})
	}

	return &OSCALComponentDefinition{
		UUID: generateUUID(),
		Metadata: OSCALMetadata{
			Title:        "g8e Platform Component Definition",
			Published:    now,
			LastModified: now,
			Version:      e.catalog.Version,
			OscalVersion: "1.1.2",
		},
		Components: []OSCALComponent{
			{
				UUID:                   generateUUID(),
				Type:                   "software",
				Title:                  "g8e Zero-Trust Execution Platform",
				Description:            "g8e is a zero-trust execution platform for agentic infrastructure. Mutations are typed, signed, state-bound, and verified through a 5-layer gauntlet.",
				ControlImplementations: controlImpls,
			},
		},
		BackMatter: OSCALBackMatter{
			Resources: []OSCALResource{
				{
					UUID:        generateUUID(),
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
// a KSIResultSet. Each KSI result becomes an observation with evidence anchors
// and a finding with pass/fail status.
func (e *OSCALExporter) GenerateAssessmentResults(resultSet *KSIResultSet) (*OSCALAssessmentResults, error) {
	if e.catalog == nil {
		return nil, fmt.Errorf("%w: nil KSI catalog", constants.ErrValidationFailed)
	}
	if resultSet == nil {
		return nil, fmt.Errorf("%w: nil KSI result set", constants.ErrValidationFailed)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	evaluatedAt := time.UnixMilli(resultSet.EvaluatedAtMs).UTC().Format(time.RFC3339)

	var observations []OSCALObservation
	var findings []OSCALFinding

	for _, res := range resultSet.Results {
		ksi := e.catalog.FindKSI(res.ID)
		if ksi == nil {
			return nil, fmt.Errorf("%w: result references unknown KSI: %s", constants.ErrValidationFailed, res.ID)
		}

		// Build observation with evidence anchors.
		var relevantEvidence []OSCALRelevantEvidence
		for _, ev := range res.Evidence {
			relevantEvidence = append(relevantEvidence, OSCALRelevantEvidence{
				Href:        "#" + string(ev.Type) + ":" + ev.Reference,
				Description: ev.Description,
			})
		}

		observations = append(observations, OSCALObservation{
			UUID:        generateUUID(),
			Title:       "KSI " + res.ID + " Evaluation",
			Description: ksi.Title,
			Methods:     []OSCALMethodRef{{MethodID: "TEST-AUTOMATED"}},
			Subjects: []OSCALSubject{
				{
					SubjectUUID: generateUUID(),
					Type:        "assessment-target",
					Title:       res.ID,
				},
			},
			RelevantEvidence: relevantEvidence,
		})

		// Map KSI status to OSCAL finding status.
		status := "not-satisfied"
		switch res.Status {
		case KSIStatusSatisfied:
			status = "satisfied"
		case KSIStatusNotApplicable:
			status = "not-applicable"
		}

		findings = append(findings, OSCALFinding{
			UUID:        generateUUID(),
			Title:       "KSI " + res.ID + " Finding",
			Description: fmt.Sprintf("KSI %s (%s): %s — %d automated methods evaluated", res.ID, ksi.Title, res.Status, res.MethodCount),
			Target: OSCALFindingTarget{
				TargetID: res.ID,
				Status:   status,
			},
		})
	}

	return &OSCALAssessmentResults{
		UUID: generateUUID(),
		Metadata: OSCALMetadata{
			Title:        "g8e KSI Assessment Results",
			Published:    now,
			LastModified: now,
			Version:      e.catalog.Version,
			OscalVersion: "1.1.2",
		},
		Results: []OSCALResult{
			{
				UUID:         generateUUID(),
				Title:        "FedRAMP 20x Class " + string(resultSet.Class) + " KSI Assessment",
				Description:  fmt.Sprintf("Automated KSI evaluation for FedRAMP 20x Certification Class %s", resultSet.Class),
				Start:        evaluatedAt,
				Observations: observations,
				Findings:     findings,
			},
		},
	}, nil
}

// generateUUID produces a random RFC 4122 UUID v4 string using crypto/rand
// via the google/uuid package. Each call returns a unique UUID.
func generateUUID() string {
	return uuid.New().String()
}
