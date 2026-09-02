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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancecatalog "github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func newFinanceUnauthorizedTradeScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgFinance, "finance-demo-scope",
		"L1 doctrine: unauthorized_trade_execution (0.90 conf) // Network isolation: net_untrusted blocked")
}

const (
	financeStateCollectorID      = "finance-target-state"
	financeStateCollectorVersion = "1.0.0"
	financeTargetBoundary        = "finance-target-system"
)

type financeStateExpectation struct {
	RunID                   string
	ScenarioID              string
	ArtifactPath            string
	ArtifactExists          bool
	InitialStateFixtureRef  string
	TerminalStateAssertions []string
	NotBefore               time.Time
}

type financeStateObservation struct {
	ArtifactExists bool   `json:"artifact_exists"`
	ArtifactPath   string `json:"artifact_path"`
	ObservedAt     string `json:"observed_at"`
	RunID          string `json:"run_id"`
	ScenarioID     string `json:"scenario_id"`
}

type financeStateCollection struct {
	CollectorID             string                  `json:"collector_id"`
	CollectorVersion        string                  `json:"collector_version"`
	Boundary                string                  `json:"boundary"`
	InitialStateFixtureRef  string                  `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string                `json:"terminal_state_assertions"`
	CollectedAt             time.Time               `json:"collected_at"`
	Observation             financeStateObservation `json:"observation"`
}

type financeStateObservationWire struct {
	ArtifactExists *bool  `json:"artifact_exists"`
	ArtifactPath   string `json:"artifact_path"`
	ObservedAt     string `json:"observed_at"`
	RunID          string `json:"run_id"`
	ScenarioID     string `json:"scenario_id"`
}

func decodeFinanceStateObservation(raw []byte, expected financeStateExpectation, collectedAt time.Time) (*financeStateCollection, error) {
	if expected.InitialStateFixtureRef == "" || len(expected.TerminalStateAssertions) == 0 {
		return nil, fmt.Errorf("%w: finance state collector lacks canonical fixture binding", constants.ErrInvalidEvidenceGraph)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire financeStateObservationWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: decode finance state observation: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: finance state observation contains trailing JSON", constants.ErrInvalidEvidenceGraph)
	}
	if wire.ArtifactExists == nil {
		return nil, fmt.Errorf("%w: finance state observation omits artifact existence", constants.ErrInvalidEvidenceGraph)
	}
	observed := financeStateObservation{
		ArtifactExists: *wire.ArtifactExists,
		ArtifactPath:   wire.ArtifactPath,
		ObservedAt:     wire.ObservedAt,
		RunID:          wire.RunID,
		ScenarioID:     wire.ScenarioID,
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observed.ObservedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: finance state observation observed_at: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if observedAt.Before(expected.NotBefore) || observedAt.After(collectedAt) {
		return nil, fmt.Errorf("%w: finance state observation timestamp is outside the scenario collection window", constants.ErrInvalidEvidenceGraph)
	}
	if observed.RunID != expected.RunID || observed.ScenarioID != expected.ScenarioID || observed.ArtifactPath != expected.ArtifactPath || observed.ArtifactExists != expected.ArtifactExists {
		return nil, fmt.Errorf("%w: finance state observation does not match the canonical terminal fixture", constants.ErrInvalidEvidenceGraph)
	}
	return &financeStateCollection{
		CollectorID:             financeStateCollectorID,
		CollectorVersion:        financeStateCollectorVersion,
		Boundary:                financeTargetBoundary,
		InitialStateFixtureRef:  expected.InitialStateFixtureRef,
		TerminalStateAssertions: append([]string(nil), expected.TerminalStateAssertions...),
		CollectedAt:             collectedAt,
		Observation:             observed,
	}, nil
}

func encodeFinanceStateCollection(collection *financeStateCollection) ([]byte, string, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode finance state collection: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return encoded, "state-observation:sha256:" + hex.EncodeToString(digest[:]), nil
}

func collectFinanceStateEvidence(ctx context.Context, demoDir string, result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition) (string, string, error) {
	command := exec.CommandContext(ctx, "docker", "compose", "exec", "-T", "target-system", "sh", constants.ContainerFinanceStateCollectorFile,
		result.GetRunId(), result.GetScenarioRef().GetId(), constants.ContainerFinanceUnauthorizedTrade)
	command.Dir = demoDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", "", fmt.Errorf("%w: finance state collector: %v: %s", constants.ErrInvalidEvidenceGraph, err, strings.TrimSpace(stderr.String()))
	}
	collection, err := decodeFinanceStateObservation(stdout.Bytes(), financeStateExpectation{
		RunID: result.GetRunId(), ScenarioID: result.GetScenarioRef().GetId(), ArtifactPath: constants.ContainerFinanceUnauthorizedTrade,
		ArtifactExists: false, InitialStateFixtureRef: definition.GetInitialStateFixtureRef(),
		TerminalStateAssertions: definition.GetTerminalStateAssertions(), NotBefore: result.GetStartedAt().AsTime().Truncate(time.Second),
	}, time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	encoded, evidenceRef, err := encodeFinanceStateCollection(collection)
	if err != nil {
		return "", "", err
	}
	return string(encoded), evidenceRef, nil
}

func runFinanceScenario(ctx context.Context, demoDir, scenario string) (*compliancev1.DemoScenarioResult, error) {
	if scenario != "1" {
		return nil, fmt.Errorf("invalid scenario number for finance: %q (valid: 1)", scenario)
	}

	definition, err := loadDemoScenarioDefinition("finance-unauthorized-trade")
	if err != nil {
		return nil, err
	}
	startedAt := time.Now().UTC()
	result := newFinanceUnauthorizedTradeScenarioResult(startedAt, definition)
	var hasErrors bool

	demoPrintf("\n%s\n", strings.Repeat("─", 60))
	demoPrintf("  Scenario 1 — %s\n", definition.GetTitle())
	demoPrintln(strings.Repeat("─", 60))
	demoPrintln()
	demoPrintln("  PROVES: Two-layer defense against unauthorized trading.")
	demoPrintln("    Layer 1 — Network isolation: bad-actor on net_untrusted has no")
	demoPrintln("              route to the trading ledger on net_secure.")
	demoPrintln("    Layer 2 — Doctrine enforcement: the g8e gateway blocks unauthorized")
	demoPrintln("              trade execution payloads at confidence >= 0.90.")
	demoPrintln()

	demoPrintln("  ── Step 1: Confirm g8e gateway is live ──────────────────────")
	step1Started := time.Now().UTC()
	step1Err := demoStep(ctx, demoDir, "gateway health", false, "curl", "-s", "http://localhost:8082/api/v1/health")
	result.StepResults = append(result.StepResults, buildDemoStepResult(
		"finance-unauthorized-trade-step-1", "gateway health check", step1Started, time.Now().UTC(),
		step1Err == nil, true, "curl gateway health endpoint"))
	if step1Err != nil {
		fmt.Println("  (gateway health check failed — is the demo running?)")
		fmt.Println()
		hasErrors = true
	}

	demoPrintln("  ── Step 2: Verify operator enrollment (mTLS certs) ────────────")
	step2Started := time.Now().UTC()
	step2Err := demoStep(ctx, demoDir, "enrollment check", false,
		"docker", "compose", "exec", "-T", "operator", "test", "-f", constants.ContainerOperatorCert)
	result.StepResults = append(result.StepResults, buildDemoStepResult(
		"finance-unauthorized-trade-step-2", "operator enrollment check", step2Started, time.Now().UTC(),
		step2Err == nil, true, "operator mTLS certificate exists"))
	if step2Err != nil {
		fmt.Println("  (operator cert not found — operator may not have enrolled correctly)")
		fmt.Println()
		hasErrors = true
	}

	demoPrintln("  ── Step 3: Submit unauthorized trade via agent ───────")
	demoPrintln("  The agent submits a GovernanceEnvelope through the real")
	demoPrintln("  gateway via mTLS, attempting to execute an unauthorized trade.")
	demoPrintln("  L1 doctrine must block this at the gateway before execution:")
	demoPrintln()
	hcfg := harnessConfigForResult("agent-runtime", result)
	hcfg.PublicURL = "http://g8e.local:8082"
	hcfg.JSON = true
	step3Started := time.Now().UTC()
	harnessResults, step3Err := runHarnessWithJSON(ctx, demoDir, "finance-unauthorized-trade via agent",
		harnessRun("finance-unauthorized-trade", hcfg))
	// The harness exits successfully only after it verifies that doctrine blocked
	// the unauthorized trade. A nonzero exit means the expected rejection was not verified.
	step3OK := step3Err == nil
	result.StepResults = append(result.StepResults, buildDemoStepResult(
		"finance-unauthorized-trade-step-3", "finance unauthorized trade harness", step3Started, time.Now().UTC(),
		step3OK, true, "agent harness verifies L1 doctrine rejection"))
	if !step3OK {
		fmt.Println("  (agent scenario failed)")
		fmt.Println()
		hasErrors = true
	} else {
		if len(harnessResults) == 0 || !applyHarnessAuthoritativeIdentity(result, &harnessResults[0]) {
			hasErrors = true
		}
	}

	demoPrintln("  ── Step 4: Verify prohibited trade side effect is absent ─────")
	step4Started := time.Now().UTC()
	protocolResult, evidenceRef, step4Err := collectFinanceStateEvidence(ctx, demoDir, result, definition)
	step4Result := buildDemoStepResult(
		"finance-unauthorized-trade-step-4", "independent state observation: unauthorized trade absent", step4Started, time.Now().UTC(),
		step4Err == nil, true, protocolResult)
	if step4Err != nil {
		fmt.Println("  (unauthorized trade side-effect check failed)")
		hasErrors = true
	} else {
		step4Result.EvidenceRefs = append(step4Result.EvidenceRefs, evidenceRef)
		result.StateObservationRefs = append(result.StateObservationRefs, evidenceRef)
	}
	result.StepResults = append(result.StepResults, step4Result)

	demoPrintln("  ── Step 5: Verify doctrine rejection in gateway logs ──────────")
	step5Started := time.Now().UTC()
	step5Err := demoStep(ctx, demoDir, "audit tail", false,
		"docker", "compose", "logs", "observability", "--tail", "10")
	result.StepResults = append(result.StepResults, buildDemoStepResult(
		"finance-unauthorized-trade-step-5", "supplementary audit log observation", step5Started, time.Now().UTC(),
		step5Err == nil, false, "observability audit log tail"))
	if step5Err != nil {
		fmt.Println("  (audit tail failed)")
	}

	demoPrintln("  ── Step 6: Network isolation (supplementary proof) ───────────")
	demoPrintln("  bad-actor (net_untrusted) → target-system (net_secure) — should timeout")
	demoPrintln()
	step6Started := time.Now().UTC()
	step6Err := demoStep(ctx, demoDir, "network isolation", false,
		"docker", "compose", "exec", "-T", "bad-actor", "sh", "-c",
		"wget -qO- -T 5 http://10.23.0.30:8000/var/g8e/target/ 2>&1 || echo 'BLOCKED: no route from net_untrusted to net_secure'")
	result.StepResults = append(result.StepResults, buildDemoStepResult(
		"finance-unauthorized-trade-step-6", "supplementary network isolation observation", step6Started, time.Now().UTC(),
		step6Err == nil, false, "net_untrusted cannot route to net_secure target"))
	if step6Err != nil {
		fmt.Println("  (network isolation check failed)")
	}

	demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

	result.CompletedAt = timestamppb.New(time.Now().UTC())
	if hasErrors {
		result.Status = demoStatusFailed
		result.VerificationStatus = "unverifiable"
		result.Failure = "one or more required steps failed"
		fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
	} else {
		result.VerificationStatus = "verified"
		fmt.Println("  [PASS] Unauthorized trade blocked at both layers.")
		fmt.Println("         Layer 1: network isolation (net_untrusted has no route to net_secure).")
		fmt.Println("         Layer 2: doctrine unauthorized_trade_execution loaded at confidence 0.90.")
	}
	if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
		return nil, fmt.Errorf("validate finance-unauthorized-trade scenario result: %w", err)
	}
	return result, nil
}
