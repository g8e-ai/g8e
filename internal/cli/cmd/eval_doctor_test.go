// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// newEvalDoctorTestEnv builds a hermetic environment for doctor tests.
// It creates a temp directory with root markers and eval project markers
// and returns the configured dependencies, config, and project root.
func newEvalDoctorTestEnv(t *testing.T) (evalDoctorDeps, *config.Config, string) {
	t.Helper()
	tmp := t.TempDir()

	// Root markers.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, constants.EvalRootVersion), []byte("2.1.8\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, constants.EvalRootMakefile), []byte("# Makefile\n"), 0o644))

	// Eval project markers.
	evalProject := filepath.Join(tmp, constants.EvalProjectDir)
	require.NoError(t, os.MkdirAll(evalProject, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(evalProject, constants.EvalProjectPyproject), []byte("[project]\nname = \"g8e-evals\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(evalProject, constants.EvalProjectLockfile), []byte("# uv.lock\n"), 0o644))

	// Project-local interpreter marker.
	evalVenvBin := filepath.Join(evalProject, constants.EvalProjectVenvDir, "bin")
	require.NoError(t, os.MkdirAll(evalVenvBin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(evalVenvBin, "python"), []byte("#!/bin/sh\nexit 0\n"), 0o755))

	// Set up the runtime tree so the trust bundle path exists.
	fileSvc, err := fs.NewRuntimeFileService(tmp, nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	// Write a dummy trust bundle so the trust_bundle check passes.
	trustRel := constants.PkiDirname + "/" + constants.PkiSubdirTrust + "/" + constants.PkiFileGatewayBundle
	require.NoError(t, fileSvc.WriteFile(context.Background(), trustRel, []byte("dummy-ca-bundle"), constants.PermFilePublic))

	cfg := &config.Config{ProjectRoot: tmp, RuntimeDir: fileSvc.Resolve("")}

	stat := newStubStatWithEvalProject(tmp, evalProject)
	deps := evalDoctorDeps{
		configLoader:   configLoaderFor(cfg),
		fileSvcFactory: fileSvcFactoryFor(fileSvc),
		stat:           stat,
		runner:         &stubEvalRunner{},
		httpClient:     &http.Client{},
	}
	return deps, cfg, tmp
}

func TestEvalDoctor_LocalScopePassesWhenEnvironmentComplete(t *testing.T) {
	deps, _, _ := newEvalDoctorTestEnv(t)
	var stdout, stderr bytes.Buffer

	result, err := runEvalDoctor(t.Context(), deps, "", "local", &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "local", result.Scope)
	// Every local check should pass.
	for _, check := range result.Checks {
		if check.ID == "provider_config" {
			// Provider config may warn if no env vars are set.
			continue
		}
		assert.Equal(t, evalDoctorCheckPass, check.Status, "check %s should pass", check.ID)
	}
}

func TestEvalDoctor_LocalScopeFailsWhenRootMarkerMissing(t *testing.T) {
	deps, _, tmp := newEvalDoctorTestEnv(t)
	// Build a stat that reports VERSION as missing.
	evalProject := filepath.Join(tmp, constants.EvalProjectDir)
	m := map[string]bool{
		filepath.Join(tmp, constants.EvalRootMakefile):             true,
		filepath.Join(evalProject, constants.EvalProjectPyproject): true,
		filepath.Join(evalProject, constants.EvalProjectLockfile):  true,
		projectInterpreterPath(evalProject):                        true,
	}
	deps.stat = stubEvalFileStat{existing: m}
	var stdout, stderr bytes.Buffer

	result, err := runEvalDoctor(t.Context(), deps, "", "local", &stdout, &stderr)

	require.NoError(t, err, "doctor itself does not error on a failed check; it returns the result")
	var repoCheck *evalDoctorCheck
	for i := range result.Checks {
		if result.Checks[i].ID == "repository_identity" {
			repoCheck = &result.Checks[i]
			break
		}
	}
	require.NotNil(t, repoCheck)
	assert.Equal(t, evalDoctorCheckFail, repoCheck.Status)
	assert.Contains(t, repoCheck.SafeDetail, "VERSION")
}

func TestEvalDoctor_LocalScopeFailsWhenEvalProjectMissing(t *testing.T) {
	deps, _, tmp := newEvalDoctorTestEnv(t)
	// Build a stat that reports pyproject as missing.
	evalProject := filepath.Join(tmp, constants.EvalProjectDir)
	m := map[string]bool{
		filepath.Join(tmp, constants.EvalRootVersion):             true,
		filepath.Join(tmp, constants.EvalRootMakefile):            true,
		filepath.Join(evalProject, constants.EvalProjectLockfile): true,
		projectInterpreterPath(evalProject):                       true,
	}
	deps.stat = stubEvalFileStat{existing: m}
	var stdout, stderr bytes.Buffer

	result, _ := runEvalDoctor(t.Context(), deps, "", "local", &stdout, &stderr)

	var projectCheck *evalDoctorCheck
	for i := range result.Checks {
		if result.Checks[i].ID == "eval_project" {
			projectCheck = &result.Checks[i]
			break
		}
	}
	require.NotNil(t, projectCheck)
	assert.Equal(t, evalDoctorCheckFail, projectCheck.Status)
}

func TestEvalDoctor_LocalScopeWarnsWhenInterpreterNotSetUp(t *testing.T) {
	deps, _, tmp := newEvalDoctorTestEnv(t)
	// Build a stat that reports the interpreter as missing.
	evalProject := filepath.Join(tmp, constants.EvalProjectDir)
	m := map[string]bool{
		filepath.Join(tmp, constants.EvalRootVersion):              true,
		filepath.Join(tmp, constants.EvalRootMakefile):             true,
		filepath.Join(evalProject, constants.EvalProjectPyproject): true,
		filepath.Join(evalProject, constants.EvalProjectLockfile):  true,
	}
	deps.stat = stubEvalFileStat{existing: m}
	var stdout, stderr bytes.Buffer

	result, _ := runEvalDoctor(t.Context(), deps, "", "local", &stdout, &stderr)

	var interpCheck *evalDoctorCheck
	for i := range result.Checks {
		if result.Checks[i].ID == "interpreter" {
			interpCheck = &result.Checks[i]
			break
		}
	}
	require.NotNil(t, interpCheck)
	assert.Equal(t, evalDoctorCheckFail, interpCheck.Status)
	assert.Contains(t, interpCheck.CorrectiveAction, "setup")
}

func TestEvalDoctor_LocalScopeFailsWhenEngineSelfCheckFails(t *testing.T) {
	deps, _, _ := newEvalDoctorTestEnv(t)
	deps.runner = &stubEvalRunner{returnFn: func(name string, args []string) error {
		if len(args) >= 2 && args[0] == "-c" && strings.Contains(args[1], "g8e_evals") {
			return errors.New("ModuleNotFoundError: No module named 'numpy'")
		}
		return nil
	}}
	var stdout, stderr bytes.Buffer

	result, _ := runEvalDoctor(t.Context(), deps, "", "local", &stdout, &stderr)

	var engineCheck *evalDoctorCheck
	for i := range result.Checks {
		if result.Checks[i].ID == "engine_version" {
			engineCheck = &result.Checks[i]
			break
		}
	}
	require.NotNil(t, engineCheck)
	assert.Equal(t, evalDoctorCheckFail, engineCheck.Status)
	assert.Contains(t, engineCheck.CorrectiveAction, "setup")
}

func TestEvalDoctor_StackScopeAddsComponentHealthChecks(t *testing.T) {
	// Start a local HTTP server that returns 200 for /health.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	deps, cfg, _ := newEvalDoctorTestEnv(t)
	// Override the health URL helper by setting the host to the test server.
	// We need the health checks to hit our test server.
	cfg.Paths = &config.PathsConfig{Host: "127.0.0.1"}
	// We can't easily override the port, so let's use a custom httpClient
	// that redirects to our test server.
	deps.httpClient = server.Client()
	// Actually, the simplest approach is to verify that stack scope
	// produces more checks than local scope.
	var stdout, stderr bytes.Buffer

	result, err := runEvalDoctor(t.Context(), deps, "", "stack", &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "stack", result.Scope)
	// Stack scope should have local checks plus component health checks.
	var hasGatewayHealth, hasOperatorHealth, hasEnsembleHealth bool
	for _, check := range result.Checks {
		switch check.ID {
		case "gateway_health":
			hasGatewayHealth = true
		case "operator_health":
			hasOperatorHealth = true
		case "ensemble_health":
			hasEnsembleHealth = true
		}
	}
	assert.True(t, hasGatewayHealth, "stack scope should check gateway health")
	assert.True(t, hasOperatorHealth, "stack scope should check operator health")
	assert.True(t, hasEnsembleHealth, "stack scope should check ensemble health")
}

func TestEvalDoctor_ProviderScopeAddsProviderChecks(t *testing.T) {
	deps, _, _ := newEvalDoctorTestEnv(t)
	// Set a provider endpoint env var so the provider check runs.
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:11434")
	var stdout, stderr bytes.Buffer

	result, err := runEvalDoctor(t.Context(), deps, "", "provider", &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "provider", result.Scope)
	// Provider scope should include stack checks plus provider checks.
	var hasProviderHealth bool
	for _, check := range result.Checks {
		if check.ID == "provider_health" {
			hasProviderHealth = true
		}
	}
	assert.True(t, hasProviderHealth, "provider scope should check provider health")
}

func TestEvalDoctor_ProviderScopeWarnsWhenNoProviderConfigured(t *testing.T) {
	deps, _, _ := newEvalDoctorTestEnv(t)
	// Ensure no provider env vars are set.
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	var stdout, stderr bytes.Buffer

	result, _ := runEvalDoctor(t.Context(), deps, "", "provider", &stdout, &stderr)

	var providerCheck *evalDoctorCheck
	for i := range result.Checks {
		if result.Checks[i].ID == "provider_endpoint" {
			providerCheck = &result.Checks[i]
			break
		}
	}
	require.NotNil(t, providerCheck)
	assert.Equal(t, evalDoctorCheckWarn, providerCheck.Status)
}

func TestEvalDoctor_NeverSendsInferenceRequests(t *testing.T) {
	deps, _, _ := newEvalDoctorTestEnv(t)
	// The runner should only be invoked for the self-check, never for
	// any provider/inference call.
	var stdout, stderr bytes.Buffer

	_, _ = runEvalDoctor(t.Context(), deps, "", "local", &stdout, &stderr)

	runner := deps.runner.(*stubEvalRunner)
	for _, call := range runner.calls {
		// Every call should be to the project interpreter with -c and
		// the self-check expression, never to a provider endpoint.
		assert.True(t, strings.HasSuffix(call.Name, "python"), "doctor should only invoke the project interpreter, got %s", call.Name)
		assert.True(t, len(call.Args) >= 2 && call.Args[0] == "-c", "doctor should only run -c expressions")
		assert.Contains(t, call.Args[1], "g8e_evals", "doctor should only run the self-check expression")
	}
}

func TestEvalDoctor_CmdReturnsNonZeroWhenCheckFails(t *testing.T) {
	deps, _, tmp := newEvalDoctorTestEnv(t)
	// Build a stat that reports VERSION as missing.
	evalProject := filepath.Join(tmp, constants.EvalProjectDir)
	m := map[string]bool{
		filepath.Join(tmp, constants.EvalRootMakefile):             true,
		filepath.Join(evalProject, constants.EvalProjectPyproject): true,
		filepath.Join(evalProject, constants.EvalProjectLockfile):  true,
		projectInterpreterPath(evalProject):                        true,
	}
	deps.stat = stubEvalFileStat{existing: m}

	cmd := evalDoctorCmdWithDeps(deps)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalPlatformUnhealthy)
}

func TestEvalDoctor_CmdJSONOutputEmitsTypedResult(t *testing.T) {
	deps, _, _ := newEvalDoctorTestEnv(t)
	cmd := evalDoctorCmdWithDeps(deps)
	cmd.SetArgs([]string{"--json"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Provider config may warn, but that's not a failure.
	// Clear provider env vars to avoid flakiness.
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	err := cmd.Execute()
	// Doctor returns error only on failed checks, not warnings.
	// With a complete environment, all local checks pass (provider_config warns).
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "\"scope\"")
	assert.Contains(t, output, "\"checks\"")
	assert.Contains(t, output, "\"id\"")
	assert.Contains(t, output, "\"status\"")
}

func TestEvalDoctor_CmdRegistersWithExpectedFlags(t *testing.T) {
	parent := evalCmd()
	var doctorFound bool
	for _, sub := range parent.Commands() {
		if sub.Name() == "doctor" {
			doctorFound = true
			assert.NotNil(t, sub.Flags().Lookup("local"))
			assert.NotNil(t, sub.Flags().Lookup("stack"))
			assert.NotNil(t, sub.Flags().Lookup("provider"))
			assert.NotNil(t, sub.Flags().Lookup("json"))
		}
	}
	assert.True(t, doctorFound, "eval parent should register doctor subcommand")
}

func TestEvalDoctor_GatewayHealthURLUsesDiscoveryHealthPath(t *testing.T) {
	cfg := &config.Config{}
	url := evalGatewayHealthURL(cfg)
	assert.True(t, strings.HasSuffix(url, constants.APIPaths.Health), "gateway health must use %s, got %s", constants.APIPaths.Health, url)
	assert.Contains(t, url, "http://", "gateway health is served on the unauthenticated HTTP discovery surface")
}

func TestEvalDoctor_OperatorHealthWarnsWithoutClientFactory(t *testing.T) {
	deps, cfg, _ := newEvalDoctorTestEnv(t)
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, nil)
	require.NoError(t, err)

	check := checkEvalOperatorSession(deps, fileSvc, cfg)

	assert.Equal(t, "operator_health", check.ID)
	assert.Equal(t, evalDoctorCheckWarn, check.Status)
}

func TestEvalDoctor_OperatorHealthWarnsWhenNotAuthenticated(t *testing.T) {
	deps, cfg, _ := newEvalDoctorTestEnv(t)
	deps.clientFactory = panickingClientFactory()
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, nil)
	require.NoError(t, err)

	check := checkEvalOperatorSession(deps, fileSvc, cfg)

	assert.Equal(t, evalDoctorCheckWarn, check.Status)
	assert.Contains(t, check.CorrectiveAction, "enroll")
}

func newDoctorOperatorClient(t *testing.T, deps *evalDoctorDeps, fileSvc fs.RuntimeFileService, cfg *config.Config, client apiClient) {
	t.Helper()
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, &auth.Credentials{
		UserID:            "user-test",
		OperatorSessionID: "op-sess-test",
		CLISessionID:      "cli-sess-test",
		OperatorID:        "op-test",
	}))
	deps.clientFactory = mockClientFactory(client)
}

func TestEvalDoctor_OperatorHealthPassesWithActiveSession(t *testing.T) {
	deps, cfg, _ := newEvalDoctorTestEnv(t)
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, nil)
	require.NoError(t, err)
	resp, err := json.Marshal(models.OperatorSlotResponse{
		Success: true,
		Operators: []models.OperatorDocumentGo{
			{ID: "op-1", Status: constants.OperatorStatusActive},
			{ID: "op-2", Status: constants.OperatorStatusBound},
		},
	})
	require.NoError(t, err)
	newDoctorOperatorClient(t, &deps, fileSvc, cfg, &mockAPIClient{getResp: resp})

	check := checkEvalOperatorSession(deps, fileSvc, cfg)

	assert.Equal(t, evalDoctorCheckPass, check.Status)
	assert.Contains(t, check.SafeDetail, "2")
}

func TestEvalDoctor_OperatorHealthFailsWithNoActiveSession(t *testing.T) {
	deps, cfg, _ := newEvalDoctorTestEnv(t)
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, nil)
	require.NoError(t, err)
	resp, err := json.Marshal(models.OperatorSlotResponse{
		Success:   true,
		Operators: []models.OperatorDocumentGo{{ID: "op-1", Status: constants.OperatorStatusOffline}},
	})
	require.NoError(t, err)
	newDoctorOperatorClient(t, &deps, fileSvc, cfg, &mockAPIClient{getResp: resp})

	check := checkEvalOperatorSession(deps, fileSvc, cfg)

	assert.Equal(t, evalDoctorCheckFail, check.Status)
	assert.Contains(t, check.SafeDetail, "No active operator session")
}

func TestEvalDoctor_OperatorHealthFailsWhenStatusUnreachable(t *testing.T) {
	deps, cfg, _ := newEvalDoctorTestEnv(t)
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, nil)
	require.NoError(t, err)
	newDoctorOperatorClient(t, &deps, fileSvc, cfg, &mockAPIClient{getErr: errors.New("connection refused")})

	check := checkEvalOperatorSession(deps, fileSvc, cfg)

	assert.Equal(t, evalDoctorCheckFail, check.Status)
}

func TestEvalDoctor_OperatorHealthFailsOnInvalidResponse(t *testing.T) {
	deps, cfg, _ := newEvalDoctorTestEnv(t)
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, nil)
	require.NoError(t, err)
	newDoctorOperatorClient(t, &deps, fileSvc, cfg, &mockAPIClient{getResp: []byte("not-json")})

	check := checkEvalOperatorSession(deps, fileSvc, cfg)

	assert.Equal(t, evalDoctorCheckFail, check.Status)
}

// Ensure the io.Discard import is used.
var _ = io.Discard
