// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// evalPreflightResult carries the resolved environment, config, file
// service, candidate identity, inventory digest, lease verification, and
// provider-free check result. Both check and start consume it so the
// fail-closed lease gate runs exactly once before engine launch and the
// provider-free checks are not repeated.
type evalPreflightResult struct {
	Env             evalLifecycleEnvironment
	Cfg             *config.Config
	FileSvc         fs.RuntimeFileService
	Candidate       evalCandidateIdentity
	InventoryDigest string
	LeaseStoreDir   string
	Verification    evalLeaseStartVerificationResult
	CheckResult     evalOperationCheckResult
}

// runEvalPreflight is the single shared preflight path. It resolves the
// eval environment, loads config, builds the file service, computes the
// candidate identity and model inventory digest, runs the fail-closed
// lease gate, then runs the provider-free config/authority/evidence/
// report-root/disk checks and the trust-bundle and stack-health checks.
// check and start both consume this result so lease verification cannot
// be duplicated or bypassed: start never re-runs check or lease
// verification, and check never reaches the engine.
func runEvalPreflight(ctx context.Context, deps evalLeaseDeps, configPath string, commandFamily evalCommandFamily) (evalPreflightResult, error) {
	env, err := resolveEvalLifecycleEnvironment(ctx, deps, configPath)
	if err != nil {
		return evalPreflightResult{}, err
	}
	cfg, err := deps.configLoader("")
	if err != nil {
		return evalPreflightResult{}, fmt.Errorf("eval: load config: %w", err)
	}
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, slog.Default())
	if err != nil {
		return evalPreflightResult{}, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	leaseStoreDir := fileSvc.Resolve(constants.EvalLeaseDirname)
	candidate, err := deps.candidateResolver.Resolve(ctx, env.RepositoryRoot)
	if err != nil {
		return evalPreflightResult{}, err
	}
	inventoryDigest, err := deps.modelInventoryResolver.Digest(ctx, env.ConfigPath)
	if err != nil {
		return evalPreflightResult{}, err
	}
	// The single fail-closed lease gate before engine launch.
	verification, err := invokeLeaseStartVerification(ctx, deps, env.InterpreterPath, evalLeaseStartVerificationRequest{
		ConfigPath:           env.ConfigPath,
		LeaseStoreDir:        leaseStoreDir,
		RepositoryRoot:       env.RepositoryRoot,
		Candidate:            candidate,
		ModelInventoryDigest: inventoryDigest,
		CommandFamily:        commandFamily,
		CommandVersion:       models.EvalEngineRequestSchemaVersion,
	})
	if err != nil {
		return evalPreflightResult{}, err
	}
	if !verification.Verified {
		return evalPreflightResult{}, mapLeaseVerificationFailureCode(verification.FailureCode, verification.FailureDetail)
	}
	preflight := evalPreflightResult{
		Env:             env,
		Cfg:             cfg,
		FileSvc:         fileSvc,
		Candidate:       candidate,
		InventoryDigest: inventoryDigest,
		LeaseStoreDir:   leaseStoreDir,
		Verification:    verification,
	}
	// Provider-free config, authority, evidence-key, report-root, disk checks.
	payload, err := invokeEvalOperationLifecycle(ctx, deps, env, "check")
	if err != nil {
		return preflight, fmt.Errorf("%w: %w", constants.ErrEvalAuthorityInvalid, err)
	}
	var checkResult evalOperationCheckResult
	if err := json.Unmarshal(payload, &checkResult); err != nil {
		return preflight, fmt.Errorf("%w: parse check result: %w", constants.ErrEvalConfigInvalid, err)
	}
	// Trust bundle check.
	trustCheck := checkEvalTrustBundle(ctx, fileSvc)
	checkResult.Checks = append(checkResult.Checks, evalOperationCheck{CheckID: trustCheck.ID, Status: string(trustCheck.Status), SafeDetail: trustCheck.SafeDetail})
	if trustCheck.Status == evalDoctorCheckFail {
		preflight.CheckResult = checkResult
		return preflight, fmt.Errorf("%w: %s", constants.ErrEvalPlatformIdentityUnavailable, trustCheck.SafeDetail)
	}
	// Stack health checks (no inference).
	if deps.httpClient != nil {
		doctorDeps := evalDoctorDeps{httpClient: deps.httpClient, clientFactory: deps.clientFactory}
		for _, healthCheck := range runEvalDoctorStackChecks(ctx, doctorDeps, fileSvc, cfg) {
			checkResult.Checks = append(checkResult.Checks, evalOperationCheck{CheckID: healthCheck.ID, Status: string(healthCheck.Status), SafeDetail: healthCheck.SafeDetail})
			if healthCheck.Status == evalDoctorCheckFail {
				preflight.CheckResult = checkResult
				return preflight, fmt.Errorf("%w: %s: %s", constants.ErrEvalPlatformUnhealthy, healthCheck.ID, healthCheck.SafeDetail)
			}
		}
	}
	preflight.CheckResult = checkResult
	return preflight, nil
}
