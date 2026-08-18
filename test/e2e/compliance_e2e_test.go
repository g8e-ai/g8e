// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/services/compliance"
)

const (
	// operatorWorkDir matches working_dir in docker-compose.yml; the operator
	// state tree lives at <workdir>/.g8e in the mounted volume.
	operatorWorkDir = "/root"
	// ksiCatalogContainerPath is the KSI catalog copied into the runtime image
	// by the Dockerfile (docs/reference stage).
	ksiCatalogContainerPath = "/docs/reference/ksi-catalog.json"
	// ledgerGitDir is the git-backed file ledger inside the operator state tree.
	ledgerGitDir = operatorWorkDir + "/.g8e/data/ledger/files/.git"
	// complianceOutDir is the default OSCAL export directory inside the operator state tree.
	complianceOutDir = operatorWorkDir + "/.g8e/data/compliance"
)

// dockerExecOperator runs a command inside the operator container with the
// working directory aligned to the operator state tree, returning stdout.
func dockerExecOperator(t *testing.T, container string, args ...string) string {
	t.Helper()
	full := append([]string{"exec", "-w", operatorWorkDir, container}, args...)
	cmd := exec.Command("docker", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), "docker exec %v failed: %s", args, stderr.String())
	return stdout.String()
}

// parseJSONFromOutput decodes the first JSON document found in CLI output.
func parseJSONFromOutput(t *testing.T, output string, target interface{}) {
	t.Helper()
	objStart := strings.Index(output, "{")
	arrStart := strings.Index(output, "[")
	var start int
	switch {
	case objStart < 0 && arrStart < 0:
		start = -1
	case objStart < 0:
		start = arrStart
	case arrStart < 0:
		start = objStart
	default:
		start = min(objStart, arrStart)
	}
	require.GreaterOrEqual(t, start, 0, "output contains no JSON document: %s", output)
	require.NoError(t, json.NewDecoder(strings.NewReader(output[start:])).Decode(target))
}

// ledgerHeadCommit reads the current HEAD commit hash of the operator's
// git-backed file ledger via the loose ref file.
func ledgerHeadCommit(t *testing.T, container string) string {
	t.Helper()
	head := strings.TrimSpace(dockerExecOperator(t, container, "cat", ledgerGitDir+"/HEAD"))
	require.True(t, strings.HasPrefix(head, "ref: "), "unexpected ledger HEAD content: %q", head)
	ref := strings.TrimSpace(strings.TrimPrefix(head, "ref: "))
	return strings.TrimSpace(dockerExecOperator(t, container, "cat", ledgerGitDir+"/"+ref))
}

// gitObjectExists checks that a commit hash resolves to a loose object in the
// operator's ledger repository.
func gitObjectExists(t *testing.T, container, hash string) bool {
	t.Helper()
	if len(hash) != 40 {
		return false
	}
	objectPath := ledgerGitDir + "/objects/" + hash[:2] + "/" + hash[2:]
	cmd := exec.Command("docker", "exec", container, "test", "-f", objectPath)
	return cmd.Run() == nil
}

// TestDockerCompliance_KSIPipeline runs the compliance CLI pipeline inside the
// operator container against real runtime state: KSI evaluation, history
// snapshot persistence, and OSCAL artifact export, then verifies that evidence
// anchors resolve to real commit hashes from the operator's git ledger.
func TestDockerCompliance_KSIPipeline(t *testing.T) {
	if sharedFixture == nil {
		t.Skip("Docker E2E fixture not available")
	}
	f := sharedFixture
	opContainer := f.ContainerPrefix + "-operator"

	// Ensure the operator has finished bootstrap before evaluating.
	f.CheckOperatorContainer(t)

	var resultSet compliance.KSIResultSet

	t.Run("ksi evaluation produces non-empty result set", func(t *testing.T) {
		out := dockerExecOperator(t, opContainer,
			"/g8e", "compliance", "ksi", "--class", "C", "--catalog", ksiCatalogContainerPath)
		parseJSONFromOutput(t, out, &resultSet)

		assert.Equal(t, compliance.ClassC, resultSet.Class)
		assert.Positive(t, resultSet.EvaluatedAtMs)
		require.NotEmpty(t, resultSet.Results, "KSI result set must not be empty")
		for _, res := range resultSet.Results {
			assert.NotEmpty(t, res.ID)
			assert.Contains(t, []compliance.KSIStatus{
				compliance.KSIStatusSatisfied,
				compliance.KSIStatusNotSatisfied,
				compliance.KSIStatusNotApplicable,
			}, res.Status, "KSI %s has invalid status", res.ID)
			// Fail-closed invariant: satisfied KSIs must carry evidence.
			if res.Status == compliance.KSIStatusSatisfied {
				assert.NotEmpty(t, res.Evidence, "satisfied KSI %s must carry evidence anchors", res.ID)
			}
		}
	})

	t.Run("history snapshot persisted and listed by ksi-history", func(t *testing.T) {
		out := dockerExecOperator(t, opContainer, "/g8e", "compliance", "ksi-history")

		var snapshots []compliance.KSIResultSet
		parseJSONFromOutput(t, out, &snapshots)
		require.NotEmpty(t, snapshots, "ksi-history must list the snapshot written by the ksi command")
		latest := snapshots[len(snapshots)-1]
		assert.Equal(t, compliance.ClassC, latest.Class)
		assert.NotEmpty(t, latest.Results)
	})

	var assessment compliance.OSCALAssessmentResults

	t.Run("oscal export writes component-definition and assessment-results", func(t *testing.T) {
		dockerExecOperator(t, opContainer,
			"/g8e", "compliance", "export", "--format", "oscal", "--class", "C", "--catalog", ksiCatalogContainerPath)

		compDefOut := dockerExecOperator(t, opContainer, "cat", complianceOutDir+"/component-definition.json")
		var compDef compliance.OSCALComponentDefinition
		parseJSONFromOutput(t, compDefOut, &compDef)
		assert.NotEmpty(t, compDef.UUID)
		assert.NotEmpty(t, compDef.Components)

		assessOut := dockerExecOperator(t, opContainer, "cat", complianceOutDir+"/assessment-results.json")
		parseJSONFromOutput(t, assessOut, &assessment)
		assert.NotEmpty(t, assessment.UUID)
		require.NotEmpty(t, assessment.Results)
	})

	t.Run("evidence anchors resolve to real ledger commit hashes", func(t *testing.T) {
		require.NotEmpty(t, resultSet.Results, "depends on ksi evaluation subtest")
		headCommit := ledgerHeadCommit(t, opContainer)
		require.Len(t, headCommit, 40, "ledger HEAD must be a full commit hash")

		anchorCount := 0
		for _, res := range resultSet.Results {
			for _, ev := range res.Evidence {
				switch ev.Type {
				case compliance.EvidenceTypeMerkleRoot:
					anchorCount++
					assert.Equal(t, headCommit, ev.Reference,
						"merkle root anchor in %s must equal the operator ledger HEAD", res.ID)
				case compliance.EvidenceTypeLedgerCommit:
					// Commitment-chain anchors reference commitment hashes; git
					// commit anchors must resolve to objects in the ledger repo.
					if gitObjectExists(t, opContainer, ev.Reference) {
						anchorCount++
						continue
					}
					assert.NotEmpty(t, ev.Reference, "ledger commit anchor in %s must be non-empty", res.ID)
				}
			}
		}
		assert.Positive(t, anchorCount, "expected at least one resolvable commit hash anchor")

		// OSCAL assessment-results relevant-evidence hrefs carry the same anchors.
		for _, oscalRes := range assessment.Results {
			for _, obs := range oscalRes.Observations {
				for _, rev := range obs.RelevantEvidence {
					assert.NotEmpty(t, rev.Href, "relevant evidence href must be non-empty")
				}
			}
		}
	})
}
