// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ImporterError records a failed importer invocation during graph
// construction. The graph is marked invalid when any importer fails.
type ImporterError struct {
	SourceID string `json:"source_id"`
	RunID    string `json:"run_id"`
	Error    string `json:"error"`
}

type runBoundEvidenceImporter interface {
	sourceRunID() string
}

// EvidenceGraphReport is the typed result of building and validating an
// evidence graph from one or more importers. It captures the graph's
// validity, node counts by type and scope, validation failures, and any
// importer errors encountered during construction.
type EvidenceGraphReport struct {
	VerifierID      string          `json:"verifier_id"`
	VerifierVersion string          `json:"verifier_version"`
	Valid           bool            `json:"valid"`
	NodeCount       int             `json:"node_count"`
	NodesByType     map[string]int  `json:"nodes_by_type"`
	NodesByScope    map[string]int  `json:"nodes_by_scope"`
	Failures        []GraphFailure  `json:"failures"`
	ImporterErrors  []ImporterError `json:"importer_errors"`
	VerifiedAt      time.Time       `json:"verified_at"`
}

// BuildAndValidateGraph constructs an evidence graph from the given
// importers, adds all imported nodes, runs the full validation gauntlet,
// and returns a typed report. Importer failures are recorded as
// ImporterErrors and mark the graph invalid rather than aborting
// construction; nodes from successful importers are still validated.
// The freshness window [windowStart, windowEnd] is applied during
// ValidateFreshness; zero values disable the respective bound.
func BuildAndValidateGraph(
	ctx context.Context,
	importers []EvidenceImporter,
	windowStart, windowEnd time.Time,
	verifiedAt time.Time,
) *EvidenceGraphReport {
	report := &EvidenceGraphReport{
		VerifierID:      constants.EvidenceGraphVerifierID,
		VerifierVersion: constants.EvidenceGraphVerifierVersion,
		NodesByType:     make(map[string]int),
		NodesByScope:    make(map[string]int),
		VerifiedAt:      verifiedAt,
	}

	graph := NewEvidenceGraph(constants.EvidenceGraphMaxBytes, nil)

	for _, importer := range importers {
		if importer == nil {
			continue
		}
		nodes, err := importer.Import(ctx)
		if err != nil {
			runID := ""
			if runBoundImporter, ok := importer.(runBoundEvidenceImporter); ok {
				runID = runBoundImporter.sourceRunID()
			}
			report.ImporterErrors = append(report.ImporterErrors, ImporterError{
				SourceID: importer.SourceID(),
				RunID:    runID,
				Error:    err.Error(),
			})
			continue
		}
		for _, node := range nodes {
			if addErr := graph.AddNode(node); addErr != nil {
				// AddNode already recorded the failure in the graph.
				continue
			}
		}
	}

	graph.ValidateAll(windowStart, windowEnd)

	report.Valid = graph.Valid() && len(report.ImporterErrors) == 0
	report.NodeCount = graph.NodeCount()
	report.Failures = graph.Failures()

	for artifactType, nodes := range graph.byType {
		report.NodesByType[string(artifactType)] = len(nodes)
	}
	for scopeID, nodes := range graph.byScope {
		report.NodesByScope[scopeID] = len(nodes)
	}

	return report
}

// MarshalJSON produces canonical compact JSON for the report.
func (r *EvidenceGraphReport) MarshalJSON() ([]byte, error) {
	type alias EvidenceGraphReport
	return json.Marshal((*alias)(r))
}

// String returns the canonical compact JSON representation of the report.
func (r *EvidenceGraphReport) String() string {
	body, err := r.MarshalJSON()
	if err != nil {
		return fmt.Sprintf(`{"valid":false,"error":"marshal failed: %s"}`, err)
	}
	return string(body)
}
