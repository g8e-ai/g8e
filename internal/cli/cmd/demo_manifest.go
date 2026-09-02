// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancecatalog "github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// demoOrgConfig holds the per-org metadata needed to build a DemoManifest.
type demoOrgConfig struct {
	scopeID    string
	prefix     string // scenario ID prefix for filtering canonical definitions
	manualLane bool   // true when the demo supports a manual notary escalation lane
}

var demoOrgConfigs = map[string]demoOrgConfig{
	constants.DemosOrgFedRAMP:    {scopeID: constants.DemoScopeFedRAMP, prefix: "fedramp-", manualLane: true},
	constants.DemosOrgDHS:        {scopeID: constants.DemoScopeDHS, prefix: "dhs-", manualLane: true},
	constants.DemosOrgFinance:    {scopeID: constants.DemoScopeFinance, prefix: "finance-", manualLane: false},
	constants.DemosOrgHealthcare: {scopeID: constants.DemoScopeHealthcare, prefix: "healthcare-", manualLane: false},
}

// demoProvenanceSubdirs lists the subdirectories within a demo directory whose
// files are content-hashed for manifest provenance. The compose file and these
// subdirectories constitute the immutable demo topology and configuration.
var demoProvenanceSubdirs = []string{
	constants.DemosDoctrineDir,
	constants.DemosTargetDataDir,
	constants.DemoConfigDirname,
}

// computeSHA256Hex returns the hex-encoded SHA-256 digest of data.
func computeSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// buildDemoManifest constructs a typed, validated DemoManifest for the given
// demo org. It loads canonical catalogs, filters scenario definitions by the
// org's scenario ID prefix, computes provenance hashes for the compose file and
// all files in the doctrine, target-data, and config subdirectories, collects
// the union of framework control references across all definitions, and
// declares required environment and supported lanes.
func buildDemoManifest(demoID, scopeID, runID string, generatedAt time.Time, demoDir string) (*compliancev1.DemoManifest, error) {
	orgCfg, ok := demoOrgConfigs[demoID]
	if !ok {
		return nil, fmt.Errorf("%w: unknown demo org %s", constants.ErrNotFound, demoID)
	}
	if scopeID == "" || runID == "" {
		return nil, fmt.Errorf("%w: scope ID and run ID are required", constants.ErrMissingRequiredField)
	}

	assertions, frameworks, _, err := compliancecatalog.LoadCanonicalCatalogs()
	if err != nil {
		return nil, fmt.Errorf("load canonical catalogs: %w", err)
	}
	scenarios, err := compliancecatalog.LoadDemoScenarioCatalog(assertions, frameworks)
	if err != nil {
		return nil, fmt.Errorf("load demo scenario catalog: %w", err)
	}

	var definitionRefs []*compliancev1.VersionedReference
	var definitions []*compliancev1.DemoScenarioDefinition
	frameworkControlSet := make(map[string]*compliancev1.FrameworkControlReference)
	for _, definition := range scenarios.GetDefinitions() {
		if !strings.HasPrefix(definition.GetScenarioId(), orgCfg.prefix) {
			continue
		}
		definitionRefs = append(definitionRefs, &compliancev1.VersionedReference{
			Id:      definition.GetScenarioId(),
			Version: definition.GetScenarioVersion(),
		})
		definitions = append(definitions, definition)
		for _, fcRef := range definition.GetFrameworkControlRefs() {
			key := fcRef.GetFrameworkRef().GetId() + ":" + fcRef.GetFrameworkRef().GetVersion() + ":" + fcRef.GetControlId()
			if _, exists := frameworkControlSet[key]; !exists {
				frameworkControlSet[key] = &compliancev1.FrameworkControlReference{
					FrameworkRef: &compliancev1.VersionedReference{
						Id:      fcRef.GetFrameworkRef().GetId(),
						Version: fcRef.GetFrameworkRef().GetVersion(),
					},
					ControlId: fcRef.GetControlId(),
				}
			}
		}
	}
	if len(definitionRefs) == 0 {
		return nil, fmt.Errorf("%w: no canonical scenario definitions for demo org %s", constants.ErrNotFound, demoID)
	}
	sort.Slice(definitionRefs, func(i, j int) bool {
		return definitionRefs[i].GetId() < definitionRefs[j].GetId()
	})

	provenanceHashes, err := computeDemoProvenanceHashes(demoDir)
	if err != nil {
		return nil, fmt.Errorf("compute demo provenance: %w", err)
	}

	lanes := []string{"automated"}
	if orgCfg.manualLane {
		lanes = append(lanes, "manual-notary")
	}

	manifest := &compliancev1.DemoManifest{
		DemoId:                 demoID,
		DemoVersion:            constants.DemoVersion,
		RunId:                  runID,
		ScopeId:                scopeID,
		GeneratedAt:            timestamppb.New(generatedAt),
		ScenarioDefinitionRefs: definitionRefs,
		ProvenanceHashes:       provenanceHashes,
		RequiredEnvironment:    []string{"docker", "g8e-binary"},
		FrameworkControlRefs:   collectSortedFrameworkControlRefs(frameworkControlSet),
		SupportedLanes:         lanes,
	}

	if err := compliancecatalog.ValidateDemoManifest(manifest, definitions, frameworks); err != nil {
		return nil, fmt.Errorf("validate demo manifest: %w", err)
	}
	return manifest, nil
}

// computeDemoProvenanceHashes hashes the compose file and every regular file in
// the configured provenance subdirectories. Hashes are sorted by name for
// deterministic output.
func computeDemoProvenanceHashes(demoDir string) ([]*compliancev1.NamedDigest, error) {
	var digests []*compliancev1.NamedDigest

	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	composeData, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}
	digests = append(digests, &compliancev1.NamedDigest{
		Name:   constants.DemosComposeFile,
		Sha256: computeSHA256Hex(composeData),
	})

	for _, subdir := range demoProvenanceSubdirs {
		subdirPath := filepath.Join(demoDir, subdir)
		entries, err := os.ReadDir(subdirPath)
		if err != nil {
			return nil, fmt.Errorf("read provenance subdir %s: %w", subdir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			filePath := filepath.Join(subdirPath, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("read provenance file %s/%s: %w", subdir, entry.Name(), err)
			}
			digests = append(digests, &compliancev1.NamedDigest{
				Name:   filepath.Join(subdir, entry.Name()),
				Sha256: computeSHA256Hex(data),
			})
		}
	}

	sort.Slice(digests, func(i, j int) bool {
		return digests[i].GetName() < digests[j].GetName()
	})
	return digests, nil
}

// collectSortedFrameworkControlRefs converts the framework control reference
// set into a sorted slice for deterministic manifest output.
func collectSortedFrameworkControlRefs(refs map[string]*compliancev1.FrameworkControlReference) []*compliancev1.FrameworkControlReference {
	result := make([]*compliancev1.FrameworkControlReference, 0, len(refs))
	keys := make([]string, 0, len(refs))
	for key := range refs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result = append(result, refs[key])
	}
	return result
}

// persistDemoManifest writes a typed DemoManifest as canonical protojson to
// manifest.json under the per-run demo evidence tree. The path is
// data/compliance/demo-evidence/<run-id>/manifest.json.
func persistDemoManifest(ctx context.Context, fileSvc fs.RuntimeFileService, manifest *compliancev1.DemoManifest) error {
	if manifest == nil || manifest.RunId == "" {
		return nil
	}

	runDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, manifest.RunId)
	if err := fileSvc.MkdirAll(ctx, runDir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("%w: create demo evidence run dir: %w", constants.ErrDemoEvidencePersistFailed, err)
	}

	encoded, err := compliancev1.MarshalCanonical(manifest)
	if err != nil {
		return fmt.Errorf("%w: marshal demo manifest: %w", constants.ErrDemoEvidencePersistFailed, err)
	}

	relPath := filepath.Join(runDir, constants.DemoRunManifestFilename)
	if err := fileSvc.WriteFile(ctx, relPath, encoded, constants.PermFileReadOnly); err != nil {
		return fmt.Errorf("%w: write demo manifest: %w", constants.ErrDemoEvidencePersistFailed, err)
	}
	return nil
}
