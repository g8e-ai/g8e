// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const envVarsJSONFilename = "env_vars.json"

// envCategoryViolation marks platform configuration that still arrives through
// the environment (INV-ENV-04). The set may only shrink.
const envCategoryViolation = "violation"

var validEnvCategories = map[string]bool{
	"secret":             true,
	"user_endpoint":      true,
	"host":               true,
	envCategoryViolation: true,
}

// allowedEnvViolations is the ratchet: the exact set of registry keys allowed to
// carry category "violation". It is the env-config-purge follow-on inventory.
// Removing an entry here requires removing the env read; adding one is not
// allowed, so new platform configuration cannot enter through the environment.
var allowedEnvViolations = map[string]bool{
	"G8E_CONSENSUS_ID":         true,
	"G8E_CONSENSUS_URL":        true,
	"G8E_CONSENSUS_BOOTSTRAP":  true,
	"G8E_VAULT_DIR":            true,
	"G8E_VAULT_KEY":            true,
	"G8E_OPERATOR_SESSION_ID":  true,
	"G8E_PASSKEY_RP_ID":        true,
	"G8E_PASSKEY_RP_NAME":      true,
	"G8E_PASSKEY_RP_ORIGINS":   true,
	"G8E_PUBLIC_BASE_URL":      true,
	"G8E_ALLOWED_ORIGINS":      true,
	"G8E_DOCTRINE_DIR":         true,
	"LATTICE_POSTURE_FLOOR":    true,
	"OLLAMA_HOST":              true,
	"G8E_DEMO_RUN_ID":          true,
	"G8E_DEMO_SCENARIO_ID":     true,
	"G8E_HARNESS_POLL_TIMEOUT": true,
	"G8E_HARNESS_LLM_PROVIDER": true,
	"G8E_HARNESS_LLM_MODEL":    true,
	"G8E_HARNESS_LLM_ENDPOINT": true,
	"G8E_TEST_REEXEC":          true,
}

type envVarRegistryEntry struct {
	GoConst     string `json:"_go_const"`
	Value       string `json:"value"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

func loadEnvVarRegistry(t *testing.T) map[string]envVarRegistryEntry {
	t.Helper()
	data, err := os.ReadFile(protocolConstantsPath(envVarsJSONFilename))
	require.NoError(t, err)
	var doc struct {
		EnvVars map[string]envVarRegistryEntry `json:"env_vars"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.NotEmpty(t, doc.EnvVars)
	return doc.EnvVars
}

// TestEnvVarRegistry_MatchesGoConstants asserts the typed Go registry and the
// protocol JSON registry describe exactly the same keys, and every key carries a
// valid category.
func TestEnvVarRegistry_MatchesGoConstants(t *testing.T) {
	registry := loadEnvVarRegistry(t)
	byGoConst := make(map[string]envVarRegistryEntry, len(registry))
	for name, entry := range registry {
		assert.Truef(t, validEnvCategories[entry.Category], "%s: category %q is not one of secret, user_endpoint, host, violation", name, entry.Category)
		assert.NotEmptyf(t, entry.Description, "%s: description is required", name)
		require.NotEmptyf(t, entry.GoConst, "%s: _go_const is required", name)
		byGoConst[entry.GoConst] = entry
	}

	goFields := reflect.TypeOf(EnvVar)
	goValues := reflect.ValueOf(EnvVar)
	for i := 0; i < goFields.NumField(); i++ {
		field := goFields.Field(i).Name
		value := string(goValues.Field(i).Interface().(EnvVarKey))
		entry, ok := byGoConst["EnvVar."+field]
		if !assert.Truef(t, ok, "EnvVar.%s has no entry in %s", field, envVarsJSONFilename) {
			continue
		}
		assert.Equalf(t, value, entry.Value, "EnvVar.%s value differs from %s", field, envVarsJSONFilename)
		delete(byGoConst, "EnvVar."+field)
	}
	for goConst := range byGoConst {
		assert.Failf(t, "stale registry entry", "%s lists %s, which is not a field of constants.EnvVar", envVarsJSONFilename, goConst)
	}
}

// TestEnvVarRegistry_ViolationSetOnlyShrinks fails when a key is classified as a
// violation without being in the allowlist (new platform config via env), and
// when an allowlisted key is no longer a violation (remove it from the ratchet).
func TestEnvVarRegistry_ViolationSetOnlyShrinks(t *testing.T) {
	registry := loadEnvVarRegistry(t)
	actual := map[string]bool{}
	for _, entry := range registry {
		if entry.Category == envCategoryViolation {
			actual[entry.Value] = true
		}
	}
	for key := range actual {
		assert.Truef(t, allowedEnvViolations[key], "%s is classified as a violation but is not in the ratchet allowlist: platform configuration must not be added to the environment (INV-ENV-04)", key)
	}
	for key := range allowedEnvViolations {
		assert.Truef(t, actual[key], "%s is in the ratchet allowlist but is no longer a violation in %s: remove it from allowedEnvViolations", key, envVarsJSONFilename)
	}
}

// TestProductionGoEnvReads_UseRegistryKeys asserts no production Go code reads
// the environment through a raw string literal; every key goes through the
// typed constants.EnvVar registry (INV-TYPE-06, INV-ENV-04).
func TestProductionGoEnvReads_UseRegistryKeys(t *testing.T) {
	root := repoRoot(t)
	var offenders []string
	for _, dir := range []string{"internal", "cmd"} {
		files, err := walkProductionGoFiles(filepath.Join(root, dir))
		require.NoError(t, err)
		for _, path := range files {
			offenders = append(offenders, rawEnvReads(t, path, root)...)
		}
	}
	sort.Strings(offenders)
	assert.Empty(t, offenders, "os.Getenv/os.LookupEnv must take string(constants.EnvVar.<Key>), not a string literal")
}

func rawEnvReads(t *testing.T, path, root string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err)

	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Getenv" && sel.Sel.Name != "LookupEnv") {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "os" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		key, err := strconv.Unquote(lit.Value)
		if err != nil {
			key = lit.Value
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		found = append(found, rel+":"+strconv.Itoa(fset.Position(lit.Pos()).Line)+" os."+sel.Sel.Name+"("+strconv.Quote(key)+")")
		return true
	})
	return found
}
