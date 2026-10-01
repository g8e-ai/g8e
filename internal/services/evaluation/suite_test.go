// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// customSuite returns a small valid custom suite carved from the default one.
func customSuite(t *testing.T, id, version string, scenarioIDs ...string) ScenarioSuite {
	t.Helper()
	def := DefaultScenarioSuite()
	custom := ScenarioSuite{SchemaVersion: SuiteSchemaVersion, ID: id, Version: version, Description: "test suite"}
	for _, scenario := range def.Scenarios {
		for _, want := range scenarioIDs {
			if scenario.ScenarioID == want {
				custom.Scenarios = append(custom.Scenarios, scenario)
			}
		}
	}
	require.Len(t, custom.Scenarios, len(scenarioIDs))
	return custom
}

func TestBuiltinSuites_RoundTripThroughTheAuthoringFileToIdenticalCatalogs(t *testing.T) {
	tests := []struct {
		name  string
		suite ScenarioSuite
		build func() (catalogDigest string, bodies map[string][2]string, err error)
	}{
		{
			name:  "default",
			suite: DefaultScenarioSuite(),
			build: func() (string, map[string][2]string, error) {
				catalog, artifacts, err := BuildScenarioCatalog()
				if err != nil {
					return "", nil, err
				}
				return catalog.GetCatalogDigest(), artifactBodies(artifacts), nil
			},
		},
		{
			name:  "smoke",
			suite: SmokeScenarioSuite(),
			build: func() (string, map[string][2]string, error) {
				catalog, artifacts, err := BuildSmokeGateScenarioCatalog()
				if err != nil {
					return "", nil, err
				}
				return catalog.GetCatalogDigest(), artifactBodies(artifacts), nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantDigest, wantBodies, err := tt.build()
			require.NoError(t, err)

			body, err := EncodeScenarioSuite(tt.suite)
			require.NoError(t, err)
			decoded, err := DecodeScenarioSuite(body)
			require.NoError(t, err)
			catalog, artifacts, err := MaterializeSuite(decoded)
			require.NoError(t, err)

			assert.Equal(t, wantDigest, catalog.GetCatalogDigest(), "an exported suite must materialize to the identical catalog")
			assert.Equal(t, wantBodies, artifactBodies(artifacts), "an exported suite must materialize to byte-identical fixtures")
		})
	}
}

func artifactBodies(artifacts map[string]ScenarioArtifacts) map[string][2]string {
	bodies := make(map[string][2]string, len(artifacts))
	for id, pair := range artifacts {
		bodies[id] = [2]string{string(pair.Input.Body), string(pair.Gold.Body)}
	}
	return bodies
}

func TestMaterializeSuite_CustomSuiteIsNotHeldToTheDefaultSuiteGates(t *testing.T) {
	custom := customSuite(t, "tiny-suite", "1.0.0", "instruction-exact-format")

	catalog, artifacts, err := MaterializeSuite(custom)
	require.NoError(t, err)

	assert.Equal(t, "tiny-suite", catalog.GetCatalogRef().GetId())
	assert.Equal(t, "1.0.0", catalog.GetCatalogRef().GetVersion())
	require.Len(t, catalog.GetScenarios(), 1)
	assert.Equal(t, "tiny-suite", catalog.GetScenarios()[0].GetInputFixtureRef().GetScopeId(), "fixture artifacts are scoped to their suite")
	assert.Equal(t, "tiny-suite", catalog.GetScenarios()[0].GetGoldCriteriaRef().GetScopeId())
	require.NoError(t, ValidateScenarioCatalog(catalog, artifacts))
	assert.Error(t, ValidateDefaultSuiteCatalog(catalog, artifacts), "the default-suite gates still reject a catalog that is not the default suite")
}

func TestMaterializeSuite_SameScenariosUnderAnotherSuiteYieldAnotherCatalog(t *testing.T) {
	first, _, err := MaterializeSuite(customSuite(t, "suite-a", "1.0.0", "instruction-exact-format"))
	require.NoError(t, err)
	second, _, err := MaterializeSuite(customSuite(t, "suite-b", "1.0.0", "instruction-exact-format"))
	require.NoError(t, err)

	assert.NotEqual(t, first.GetCatalogDigest(), second.GetCatalogDigest())
}

func TestMaterializeSuite_RejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ScenarioSuite)
		want   error
	}{
		{"wrong schema version", func(s *ScenarioSuite) { s.SchemaVersion = "9.9.9" }, constants.ErrEvaluationSuiteInvalid},
		{"uppercase id", func(s *ScenarioSuite) { s.ID = "My-Suite" }, constants.ErrEvaluationSuiteInvalid},
		{"path-like id", func(s *ScenarioSuite) { s.ID = "../escape" }, constants.ErrEvaluationSuiteInvalid},
		{"missing version", func(s *ScenarioSuite) { s.Version = " " }, constants.ErrEvaluationSuiteInvalid},
		{"no scenarios", func(s *ScenarioSuite) { s.Scenarios = nil }, constants.ErrEvaluationSuiteInvalid},
		{"duplicate scenario", func(s *ScenarioSuite) { s.Scenarios = append(s.Scenarios, s.Scenarios[0]) }, constants.ErrEvaluationSuiteInvalid},
		{"unknown category", func(s *ScenarioSuite) { s.Scenarios[0].Category = "vibes" }, constants.ErrEvaluationSuiteInvalid},
		{"unspecified category", func(s *ScenarioSuite) { s.Scenarios[0].Category = "unspecified" }, constants.ErrEvaluationSuiteInvalid},
		{"unknown role", func(s *ScenarioSuite) { s.Scenarios[0].EligibleRoles = []string{"chief"} }, constants.ErrEvaluationSuiteInvalid},
		{"unknown trajectory policy", func(s *ScenarioSuite) { s.Scenarios[0].TrajectoryPolicy = "wander" }, constants.ErrEvaluationSuiteInvalid},
		{"tool outside the registry", func(s *ScenarioSuite) {
			s.Scenarios[0].AllowedTools = []string{"no_such_tool"}
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"scenario that cannot fail", func(s *ScenarioSuite) {
			s.Scenarios[0].Gold.ContentCheck = nil
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"case title naming the evaluation", func(s *ScenarioSuite) {
			s.Scenarios[0].Input.Seed.CaseTitle = "Eval run"
		}, constants.ErrEvaluationScenarioContractInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			custom := customSuite(t, "bad-suite", "1.0.0", "instruction-exact-format")
			tt.mutate(&custom)
			_, _, err := MaterializeSuite(custom)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestDecodeScenarioSuite_RejectsUnknownFieldsAndTrailingContent(t *testing.T) {
	good, err := EncodeScenarioSuite(customSuite(t, "tiny-suite", "1.0.0", "instruction-exact-format"))
	require.NoError(t, err)

	_, err = DecodeScenarioSuite(good)
	require.NoError(t, err)

	_, err = DecodeScenarioSuite([]byte(`{"schema_version":"1.0.0","id":"x","version":"1","scenarios":[],"typo_field":1}`))
	assert.ErrorIs(t, err, constants.ErrEvaluationSuiteInvalid)

	_, err = DecodeScenarioSuite(append(append([]byte(nil), good...), []byte(`{}`)...))
	assert.ErrorIs(t, err, constants.ErrEvaluationSuiteInvalid)
}

func TestCatalogRecomputesGrades(t *testing.T) {
	assert.True(t, CatalogRecomputesGrades(nil))
	assert.True(t, CatalogRecomputesGrades(versioned(DefaultSuiteID, DefaultSuiteVersion)))
	assert.False(t, CatalogRecomputesGrades(versioned(DefaultSuiteID, "0.9.0")), "an older default-suite version was graded against fixtures this build no longer carries")
	assert.True(t, CatalogRecomputesGrades(versioned("my-suite", "3.1.4")), "a custom suite grades from its own frozen fixtures at any version")
	assert.False(t, CatalogRecomputesGrades(versioned(LegacyDefaultSuiteID, "1.0.0")), "campaigns frozen before the suite rename carry the legacy built-in id and were graded against fixtures this build no longer carries")
}

func TestSuiteStore_CreateListResolveUpdateDelete(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newCampaignMemoryFileService())

	summaries, err := store.ListSuites(ctx)
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	assert.Equal(t, DefaultSuiteID, summaries[0].ID)
	assert.True(t, summaries[0].Builtin)
	assert.Equal(t, DefaultSuiteScenarioCount, summaries[0].ScenarioCount)
	assert.Equal(t, SmokeSuiteID, summaries[1].ID)

	v1 := customSuite(t, "my-suite", "1.0.0", "instruction-exact-format")
	require.NoError(t, store.CreateSuite(ctx, v1))
	assert.ErrorIs(t, store.CreateSuite(ctx, v1), constants.ErrEvaluationSuiteExists)

	summaries, err = store.ListSuites(ctx)
	require.NoError(t, err)
	require.Len(t, summaries, 3)
	assert.Equal(t, "my-suite", summaries[2].ID)
	assert.False(t, summaries[2].Builtin)

	resolved, builtin, err := store.ResolveSuite(ctx, "my-suite")
	require.NoError(t, err)
	assert.False(t, builtin)
	assert.Equal(t, "1.0.0", resolved.Version)
	_, builtin, err = store.ResolveSuite(ctx, DefaultSuiteID)
	require.NoError(t, err)
	assert.True(t, builtin)

	catalog, artifacts, err := store.LoadSuiteCatalog(ctx, "my-suite")
	require.NoError(t, err)
	require.NoError(t, ValidateScenarioCatalog(catalog, artifacts))

	// Resubmitting identical content is a no-op; changed content at the same
	// version is refused; changed content at a new version is accepted.
	require.NoError(t, store.UpdateSuite(ctx, v1))
	changed := customSuite(t, "my-suite", "1.0.0", "instruction-exact-format", "tool-select-file-read")
	assert.ErrorIs(t, store.UpdateSuite(ctx, changed), constants.ErrEvaluationSuiteInvalid)
	changed.Version = "1.1.0"
	require.NoError(t, store.UpdateSuite(ctx, changed))
	resolved, _, err = store.ResolveSuite(ctx, "my-suite")
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", resolved.Version)
	assert.Len(t, resolved.Scenarios, 2)

	require.NoError(t, store.DeleteSuite(ctx, "my-suite"))
	_, _, err = store.ResolveSuite(ctx, "my-suite")
	assert.ErrorIs(t, err, constants.ErrNotFound)
	assert.ErrorIs(t, store.DeleteSuite(ctx, "my-suite"), constants.ErrNotFound)
	assert.ErrorIs(t, store.UpdateSuite(ctx, v1), constants.ErrNotFound)
}

func TestSuiteStore_BuiltinSuitesAreReadOnlyAndReserved(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newCampaignMemoryFileService())

	shadow := customSuite(t, DefaultSuiteID, "9.9.9", "instruction-exact-format")
	assert.ErrorIs(t, store.CreateSuite(ctx, shadow), constants.ErrEvaluationSuiteBuiltin)
	assert.ErrorIs(t, store.UpdateSuite(ctx, shadow), constants.ErrEvaluationSuiteBuiltin)
	assert.ErrorIs(t, store.DeleteSuite(ctx, DefaultSuiteID), constants.ErrEvaluationSuiteBuiltin)
	assert.ErrorIs(t, store.DeleteSuite(ctx, SmokeSuiteID), constants.ErrEvaluationSuiteBuiltin)

	// The pre-rename catalog id stays reserved: a custom suite named after it
	// would be mistaken for an old built-in run and skip grade recomputation.
	legacy := customSuite(t, LegacyDefaultSuiteID, "1.0.0", "instruction-exact-format")
	assert.ErrorIs(t, store.CreateSuite(ctx, legacy), constants.ErrEvaluationSuiteBuiltin)
	assert.ErrorIs(t, store.UpdateSuite(ctx, legacy), constants.ErrEvaluationSuiteBuiltin)
}

func TestSuiteStore_InvalidSuiteIsNeverStored(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newCampaignMemoryFileService())

	bad := customSuite(t, "bad-suite", "1.0.0", "instruction-exact-format")
	bad.Scenarios[0].AllowedTools = []string{"no_such_tool"}

	assert.ErrorIs(t, store.CreateSuite(ctx, bad), constants.ErrEvaluationScenarioContractInvalid)
	_, _, err := store.ResolveSuite(ctx, "bad-suite")
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestSuiteStore_LoadSuiteCatalogServesBuiltinsAndRejectsUnknownIDs(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newCampaignMemoryFileService())

	catalog, _, err := store.LoadSuiteCatalog(ctx, DefaultSuiteID)
	require.NoError(t, err)
	assert.Len(t, catalog.GetScenarios(), DefaultSuiteScenarioCount)

	catalog, _, err = store.LoadSuiteCatalog(ctx, SmokeSuiteID)
	require.NoError(t, err)
	assert.Equal(t, SmokeSuiteID, catalog.GetCatalogRef().GetId())
	assert.Len(t, catalog.GetScenarios(), SmokeGateScenarioCount)

	_, _, err = store.LoadSuiteCatalog(ctx, "never-created")
	assert.ErrorIs(t, err, constants.ErrNotFound)
	_, _, err = store.LoadSuiteCatalog(ctx, "../escape")
	assert.ErrorIs(t, err, constants.ErrNotFound)
}
