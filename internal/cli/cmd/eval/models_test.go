// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	digestOne   = "1111111111111111111111111111111111111111111111111111111111111111"
	digestTwo   = "2222222222222222222222222222222222222222222222222222222222222222"
	digestThree = "3333333333333333333333333333333333333333333333333333333333333333"
	digestDrift = "9999999999999999999999999999999999999999999999999999999999999999"
)

func variantFixture(tag, id, family string, params uint64, digest string) *evalv1.ModelVariant {
	return &evalv1.ModelVariant{
		VariantId:      id,
		ProviderClass:  "ollama",
		ServedModelTag: tag,
		ModelDigest:    digest,
		ModelFamily:    family,
		ParameterCount: params,
	}
}

func runModelsCmd(t *testing.T, root string, args ...string) (string, error) {
	t.Helper()
	command := evalCmdWithConfig(testDeps(root))
	var out bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&out)
	command.SetArgs(append(append([]string{"models"}, args...), "--project-root", root))
	err := command.Execute()
	return out.String(), err
}

func runModelsJSON(t *testing.T, root string, args ...string) (string, error) {
	t.Helper()
	command := evalCmdWithConfig(testDeps(root))
	rootCmd := cmdtest.GlobalJSONRoot(t, command)
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(append(append([]string{"eval", "models"}, args...), "--project-root", root))
	err := rootCmd.Execute()
	return out.String(), err
}

func seedModelScopes(t *testing.T, root string) {
	t.Helper()
	writeTestCatalogInventory(t, root, []*evalv1.ModelVariant{
		variantFixture("gemma3:1b", "gemma3-1b", "gemma3", 1_000_000_000, digestOne),
		variantFixture("granite4.2:8b", "granite4-2-8b", "granite", 8_000_000_000, digestTwo),
		variantFixture("qwen3:35b", "qwen3-35b", "qwen3", 35_000_000_000, digestThree),
	})
	writeTestModelInventory(t, root, []*evalv1.ModelVariant{
		variantFixture("gemma3:1b", "gemma3-1b", "gemma3", 1_000_000_000, digestOne),
		variantFixture("granite4.2:8b", "granite4-2-8b", "granite", 8_000_000_000, digestDrift),
	})
}

func TestModelsList_ScopesAndFilters(t *testing.T) {
	root := t.TempDir()
	seedModelScopes(t, root)

	out, err := runModelsCmd(t, root, "list", "--detailed")
	require.NoError(t, err)
	assert.Contains(t, out, "gemma3:1b")
	assert.Contains(t, out, "catalog+registry")
	assert.Contains(t, out, "qwen3:35b")

	out, err = runModelsCmd(t, root, "list", "--scope", "registry")
	require.NoError(t, err)
	assert.NotContains(t, out, "qwen3:35b")
	assert.Contains(t, out, "granite4.2:8b")

	out, err = runModelsCmd(t, root, "list", "--scope", "catalog", "--max-params", "12b", "-d")
	require.NoError(t, err)
	assert.Contains(t, out, "gemma3:1b")
	assert.Contains(t, out, "8B")
	assert.NotContains(t, out, "qwen3:35b")

	out, err = runModelsCmd(t, root, "list", "--family", "granite")
	require.NoError(t, err)
	assert.Contains(t, out, "granite4.2:8b")
	assert.NotContains(t, out, "gemma3:1b")

	_, err = runModelsCmd(t, root, "list", "--scope", "everything")
	require.ErrorContains(t, err, "--scope must be")

	_, err = runModelsCmd(t, root, "list", "--all", "--family", "granite")
	require.ErrorContains(t, err, "--all cannot be combined")
}

func TestModelsList_JSONReportsScopes(t *testing.T) {
	root := t.TempDir()
	seedModelScopes(t, root)

	out, err := runModelsJSON(t, root, "list")
	require.NoError(t, err)
	var payload modelListJSON
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	require.Len(t, payload.Variants, 3)
	scopes := map[string][]string{}
	for _, row := range payload.Variants {
		scopes[row.ServedModelTag] = row.Scopes
	}
	assert.Equal(t, []string{"catalog", "registry"}, scopes["gemma3:1b"])
	assert.Equal(t, []string{"catalog"}, scopes["qwen3:35b"])
}

func TestModelsList_ReadsRegistryWithoutDigest(t *testing.T) {
	root := t.TempDir()
	writeTestFrozenInventory(t, root, evaluation.DefaultModelInventoryRelPath,
		&evalv1.ModelVariant{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "digest", ProviderClass: "ollama"},
	)

	out, err := runModelsCmd(t, root, "list", "--scope", "registry")
	require.NoError(t, err)
	assert.Contains(t, out, "qwen3:4b")
	assert.Contains(t, out, "qwen3-4b")
}

func TestModelsList_EmptyProject(t *testing.T) {
	out, err := runModelsCmd(t, t.TempDir(), "list")
	require.NoError(t, err)
	assert.Contains(t, out, "No models found")
}

func TestModelsShow(t *testing.T) {
	root := t.TempDir()
	seedModelScopes(t, root)

	out, err := runModelsCmd(t, root, "show", "granite4-2-8b")
	require.NoError(t, err)
	assert.Contains(t, out, "Tag: granite4.2:8b")
	assert.Contains(t, out, "Digest (catalog): "+digestTwo)
	assert.Contains(t, out, "Digest (registry): "+digestDrift)
	assert.Contains(t, out, "Digest drift")

	out, err = runModelsCmd(t, root, "show", "qwen3:35b")
	require.NoError(t, err)
	assert.Contains(t, out, "Scopes: catalog")
	assert.NotContains(t, out, "Digest drift")

	out, err = runModelsJSON(t, root, "show", "gemma3:1b")
	require.NoError(t, err)
	var payload struct {
		Scopes      []string
		DigestDrift bool `json:"digest_drift"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &payload))
	assert.Equal(t, []string{"catalog", "registry"}, payload.Scopes)
	assert.False(t, payload.DigestDrift)

	_, err = runModelsCmd(t, root, "show", "nope:1b")
	require.ErrorIs(t, err, constants.ErrInferenceModelNotFound)
}

func TestModelsAdd_DefaultsToRegistryAndCreatesIt(t *testing.T) {
	root := t.TempDir()

	out, err := runModelsCmd(t, root, "add", "gemma4:12b", "--parameters", "12b", "--family", "gemma4", "--context", "8192")
	require.NoError(t, err)
	assert.Contains(t, out, "Added gemma4:12b")
	assert.Contains(t, out, "registry")

	registryPath := filepath.Join(root, constants.RuntimeDirname, evaluation.DefaultModelInventoryRelPath)
	_, statErr := os.Stat(registryPath)
	require.NoError(t, statErr)
	_, statErr = os.Stat(filepath.Join(root, evaluation.DefaultBaseModelInventoryRelPath))
	assert.True(t, os.IsNotExist(statErr), "registry add must not touch the catalog")

	out, err = runModelsCmd(t, root, "list", "--scope", "registry", "-d")
	require.NoError(t, err)
	assert.Contains(t, out, "gemma4:12b")
	assert.Contains(t, out, "12B")
	assert.Contains(t, out, "gemma4")

	// Adding the same tag again updates in place.
	_, err = runModelsCmd(t, root, "add", "gemma4:12b", "--parameters", "13b", "--digest", digestOne)
	require.NoError(t, err)
	out, err = runModelsCmd(t, root, "list", "--scope", "registry", "-d")
	require.NoError(t, err)
	assert.Contains(t, out, "13B")
	assert.Contains(t, out, digestOne)
}

func TestModelsAdd_CatalogFlagTargetsCatalogOnly(t *testing.T) {
	root := t.TempDir()
	seedModelScopes(t, root)

	_, err := runModelsCmd(t, root, "add", "granite4.2:3b", "--family", "granite", "--parameters", "3b", "--catalog")
	require.NoError(t, err)

	out, err := runModelsCmd(t, root, "list", "--scope", "catalog")
	require.NoError(t, err)
	assert.Contains(t, out, "granite4.2:3b")
	out, err = runModelsCmd(t, root, "list", "--scope", "registry")
	require.NoError(t, err)
	assert.NotContains(t, out, "granite4.2:3b")
}

func TestModelsAdd_RejectsBadInput(t *testing.T) {
	root := t.TempDir()
	_, err := runModelsCmd(t, root, "add")
	require.Error(t, err)
	_, err = runModelsCmd(t, root, "add", "gemma4:12b", "--parameters", "lots")
	require.ErrorContains(t, err, "parse parameter count")
	for _, removed := range []string{"--params", "--sync-base", "--to", "--tag"} {
		_, err = runModelsCmd(t, root, "add", "gemma4:12b", removed, "x")
		require.ErrorContains(t, err, "unknown flag", removed)
	}
}

func TestModelsRemove(t *testing.T) {
	root := t.TempDir()
	seedModelScopes(t, root)

	out, err := runModelsCmd(t, root, "remove", "gemma3:1b")
	require.NoError(t, err)
	assert.Contains(t, out, "Removed gemma3:1b (gemma3-1b) from the registry")
	assert.Contains(t, out, "1 model(s) remain")

	out, err = runModelsCmd(t, root, "list", "--scope", "registry")
	require.NoError(t, err)
	assert.NotContains(t, out, "gemma3:1b")
	assert.Contains(t, out, "granite4.2:8b")

	out, err = runModelsCmd(t, root, "list", "--scope", "catalog")
	require.NoError(t, err)
	assert.Contains(t, out, "gemma3:1b", "registry remove leaves the catalog intact")
}

func TestModelsRemove_ByFilterAndLastModelClearsRegistry(t *testing.T) {
	root := t.TempDir()
	seedModelScopes(t, root)

	_, err := runModelsCmd(t, root, "remove", "--all")
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(root, constants.RuntimeDirname, evaluation.DefaultModelInventoryRelPath))
	assert.True(t, os.IsNotExist(statErr), "removing every registry model removes the registry file")

	_, err = runModelsCmd(t, root, "remove", "--all", "--catalog")
	require.ErrorIs(t, err, constants.ErrEvaluationSelectionEmpty)

	out, err := runModelsCmd(t, root, "remove", "--family", "granite", "--catalog")
	require.NoError(t, err)
	assert.Contains(t, out, "granite4.2:8b")
}

func TestModelsRemove_EmptySelectionIsAnError(t *testing.T) {
	root := t.TempDir()
	seedModelScopes(t, root)

	_, err := runModelsCmd(t, root, "remove")
	require.ErrorIs(t, err, constants.ErrEvaluationSelectionEmpty)
	_, err = runModelsCmd(t, root, "remove", "--family", "llama")
	require.ErrorIs(t, err, constants.ErrEvaluationSelectionEmpty)
	_, err = runModelsCmd(t, root, "remove", "nope:1b")
	require.ErrorIs(t, err, constants.ErrInferenceModelNotFound)
}

func TestModelsImport_CatalogToRegistry(t *testing.T) {
	root := t.TempDir()
	writeTestCatalogInventory(t, root, []*evalv1.ModelVariant{
		variantFixture("granite4.2:3b", "granite4-2-3b", "granite", 3_000_000_000, digestOne),
		variantFixture("granite4.2:8b", "granite4-2-8b", "granite", 8_000_000_000, digestTwo),
		variantFixture("qwen3:35b", "qwen3-35b", "qwen3", 35_000_000_000, digestThree),
	})
	writeTestModelInventory(t, root, []*evalv1.ModelVariant{
		variantFixture("gemma3:1b", "gemma3-1b", "gemma3", 1_000_000_000, digestOne),
	})

	out, err := runModelsCmd(t, root, "import", "--family", "granite")
	require.NoError(t, err)
	assert.Contains(t, out, "Imported 2 model(s) into the registry")
	assert.Contains(t, out, "models=3")

	out, err = runModelsCmd(t, root, "list", "--scope", "registry")
	require.NoError(t, err)
	assert.Contains(t, out, "gemma3:1b")
	assert.Contains(t, out, "granite4.2:3b")
	assert.NotContains(t, out, "qwen3:35b")

	out, err = runModelsCmd(t, root, "import", "qwen3-35b")
	require.NoError(t, err)
	assert.Contains(t, out, "Imported 1 model(s)")
}

func TestModelsImport_CreatesRegistryAndRejectsBadSelections(t *testing.T) {
	root := t.TempDir()
	writeTestCatalogInventory(t, root, []*evalv1.ModelVariant{
		variantFixture("granite4.2:3b", "granite4-2-3b", "granite", 3_000_000_000, digestOne),
	})

	_, err := runModelsCmd(t, root, "import")
	require.ErrorIs(t, err, constants.ErrEvaluationSelectionEmpty)
	_, err = runModelsCmd(t, root, "import", "nope:1b")
	require.ErrorIs(t, err, constants.ErrInferenceModelNotFound)
	_, err = runModelsCmd(t, root, "import", "granite4.2:3b", "--all")
	require.ErrorContains(t, err, "cannot be combined")
	for _, removed := range []string{"--from", "--to", "--tags", "--tag", "--params"} {
		_, err = runModelsCmd(t, root, "import", "--all", removed, "x")
		require.ErrorContains(t, err, "unknown flag", removed)
	}

	_, err = runModelsCmd(t, root, "import", "--all")
	require.NoError(t, err)
	out, err := runModelsCmd(t, root, "list", "--scope", "registry")
	require.NoError(t, err)
	assert.Contains(t, out, "granite4.2:3b")
}

func TestModelsImport_MissingCatalogFails(t *testing.T) {
	_, err := runModelsCmd(t, t.TempDir(), "import", "--all")
	require.ErrorIs(t, err, constants.ErrNotFound)
}

func TestDiffModelScopes(t *testing.T) {
	catalog := map[string]string{"a": "d1", "b": "d2", "c": "d3", "drift": "d4"}
	registry := map[string]string{"a": "d1", "b": "d2", "drift": "dX", "reg-only": "d5"}
	provider := map[string]string{"a": "d1", "drift": "d4", "prov-only": "d6"}

	rows := diffModelScopes(catalog, registry, provider)
	status := map[string]string{}
	for _, row := range rows {
		status[row.ServedModelTag] = row.Status
	}
	assert.Equal(t, map[string]string{
		"a":         "in-sync",
		"b":         "missing-from-provider",
		"c":         "only-in-catalog",
		"drift":     "digest-drift",
		"reg-only":  "only-in-registry",
		"prov-only": "only-in-provider",
	}, status)
	assert.Equal(t, []string{"a", "b", "c", "drift", "prov-only", "reg-only"}, func() []string {
		tags := make([]string, 0, len(rows))
		for _, row := range rows {
			tags = append(tags, row.ServedModelTag)
		}
		return tags
	}())

	assert.Equal(t, "only-in-provider", diffModelScopes(nil, nil, map[string]string{"x": "d"})[0].Status)
	assert.Equal(t, "missing-from-catalog", diffModelScopes(nil, map[string]string{"x": "d"}, map[string]string{"x": "d"})[0].Status)
	assert.Empty(t, diffModelScopes(nil, nil, nil))
}

func TestShortDigest(t *testing.T) {
	assert.Equal(t, "-", shortDigest(""))
	assert.Equal(t, "abc", shortDigest("abc"))
	assert.Equal(t, "0123456789ab", shortDigest("0123456789abcdef"))
}

func TestModelsDiff_RequiresOneProviderSession(t *testing.T) {
	root := t.TempDir()
	seedModelScopes(t, root)

	// A second inference session makes the provider ambiguous.
	deps := newGatewayTestDeps(t, root, append(campaignOrchestrateOperators(), inferenceOperatorFixture("infer-op-2", "infer-session-2")))

	command := evalCmdWithConfig(deps)
	var out bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&out)
	command.SetArgs([]string{"models", "diff", "--project-root", root})
	err := command.Execute()
	require.ErrorIs(t, err, constants.ErrOperatorSessionAmbiguous)
	assert.ErrorContains(t, err, "infer-session")
	assert.ErrorContains(t, err, "infer-session-2")
}

func TestModelsPull_SelectorAndFormationsAreExclusive(t *testing.T) {
	root := t.TempDir()
	_, err := runModelsCmd(t, root, "pull", "qwen3:4b", "--formations")
	require.ErrorContains(t, err, "--formations cannot be combined")
	_, err = runModelsCmd(t, root, "pull", "--family", "qwen3", "--formations")
	require.ErrorContains(t, err, "--formations cannot be combined")
}

func TestModelsPull_SelectionIsResolvedAgainstIntakeCatalog(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "eval"), 0o755))
	catalog := `{"models":[
		{"variant_id":"glm-5-3-air","served_model_tag":"glm-5.3-air","model_family":"glm53","parameter_count":"9000000000","staging":{"method":"pending"}},
		{"variant_id":"qwen3-8-27b","served_model_tag":"qwen3.8:27b","model_family":"qwen38","parameter_count":"27000000000","staging":{"method":"pending"}}]}`
	require.NoError(t, os.WriteFile(filepath.Join(root, evaluation.DefaultRolloutIntakeCatalogRelPath), []byte(catalog), 0o644))

	_, err := runModelsCmd(t, root, "pull")
	require.ErrorIs(t, err, constants.ErrEvaluationSelectionEmpty)
	_, err = runModelsCmd(t, root, "pull", "unknown:1b")
	require.ErrorIs(t, err, constants.ErrInferenceModelNotFound)
	_, err = runModelsCmd(t, root, "pull", "--family", "llama")
	require.ErrorIs(t, err, constants.ErrEvaluationSelectionEmpty)
	_, err = runModelsCmd(t, root, "pull", "--catalog")
	require.ErrorContains(t, err, "unknown flag", "--catalog is a scope flag, not a pull flag")
}

func TestIntakeVariantsParsesParameterCounts(t *testing.T) {
	variants := intakeVariants([]evaluation.RolloutIntakeModel{
		{VariantID: "a", ServedModelTag: "a:1", ModelFamily: "fam", ParameterCount: "9000000000"},
		{VariantID: "b", ServedModelTag: "b:1", ParameterCount: "not-a-number"},
	})
	require.Len(t, variants, 2)
	assert.Equal(t, uint64(9_000_000_000), variants[0].GetParameterCount())
	assert.Equal(t, "fam", variants[0].GetModelFamily())
	assert.Zero(t, variants[1].GetParameterCount())
}
