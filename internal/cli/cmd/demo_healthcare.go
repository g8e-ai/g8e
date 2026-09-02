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
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancecatalog "github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func newHealthcareSuccessScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition, runID string) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgHealthcare, constants.DemoScopeHealthcare, runID,
		"11 PHI/HIPAA rules evaluated, FHIR PA submission recorded")
}

func newHealthcarePHIBlockedScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition, runID string) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgHealthcare, constants.DemoScopeHealthcare, runID,
		"Network isolation verified // L1 doctrine rejection verified at 0.95 confidence")
}

func newHealthcareGoldCardScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition, runID string) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgHealthcare, constants.DemoScopeHealthcare, runID,
		"96% approval rate evaluated against 90% threshold // AUTO_APPROVED")
}

func newHealthcareSLABreachScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition, runID string) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgHealthcare, constants.DemoScopeHealthcare, runID,
		"10 elapsed days evaluated against 7-day SLA // SLA_BREACHED // OHA reportable")
}

const (
	healthcareStateCollectorID      = "healthcare-actuator-state"
	healthcareStateCollectorVersion = "1.0.0"
	healthcareActuatorBoundary      = "healthcare-actuator"
)

type healthcareStateExpectation struct {
	RunID                   string
	ScenarioID              string
	RequestID               string
	Action                  string
	ResourceType            string
	Subject                 string
	Status                  string
	MeasuredValue           int
	ThresholdValue          int
	AutoApproved            bool
	ReportableToOHA         bool
	InitialStateFixtureRef  string
	TerminalStateAssertions []string
	NotBefore               time.Time
}

type healthcareStateObservation struct {
	Action          string `json:"action"`
	RequestID       string `json:"request_id"`
	ResourceType    string `json:"resource_type"`
	Subject         string `json:"subject"`
	MeasuredValue   int    `json:"measured_value"`
	ThresholdValue  int    `json:"threshold_value"`
	RunID           string `json:"run_id"`
	ScenarioID      string `json:"scenario_id"`
	Status          string `json:"status"`
	AutoApproved    bool   `json:"auto_approved"`
	ReportableToOHA bool   `json:"reportable_to_oha"`
	EvaluatedAt     string `json:"evaluated_at"`
}

type healthcareStateCollection struct {
	CollectorID             string                     `json:"collector_id"`
	CollectorVersion        string                     `json:"collector_version"`
	Boundary                string                     `json:"boundary"`
	InitialStateFixtureRef  string                     `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string                   `json:"terminal_state_assertions"`
	CollectedAt             time.Time                  `json:"collected_at"`
	Observation             healthcareStateObservation `json:"observation"`
}

type healthcareStateObservationWire struct {
	Action          string `json:"action"`
	RequestID       string `json:"request_id"`
	ResourceType    string `json:"resource_type"`
	Subject         string `json:"subject"`
	MeasuredValue   *int   `json:"measured_value"`
	ThresholdValue  *int   `json:"threshold_value"`
	RunID           string `json:"run_id"`
	ScenarioID      string `json:"scenario_id"`
	Status          string `json:"status"`
	AutoApproved    *bool  `json:"auto_approved"`
	ReportableToOHA *bool  `json:"reportable_to_oha"`
	EvaluatedAt     string `json:"evaluated_at"`
}

func decodeHealthcareStateObservation(raw []byte, expected healthcareStateExpectation, collectedAt time.Time) (*healthcareStateCollection, error) {
	if expected.InitialStateFixtureRef == "" || len(expected.TerminalStateAssertions) == 0 {
		return nil, fmt.Errorf("%w: healthcare state collector lacks canonical fixture binding", constants.ErrInvalidEvidenceGraph)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire healthcareStateObservationWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: decode healthcare state observation: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: healthcare state observation contains trailing JSON", constants.ErrInvalidEvidenceGraph)
	}
	if wire.MeasuredValue == nil || wire.ThresholdValue == nil || wire.AutoApproved == nil || wire.ReportableToOHA == nil {
		return nil, fmt.Errorf("%w: healthcare state observation omits required typed fields", constants.ErrInvalidEvidenceGraph)
	}
	observed := healthcareStateObservation{
		Action: wire.Action, RequestID: wire.RequestID, ResourceType: wire.ResourceType, Subject: wire.Subject,
		MeasuredValue: *wire.MeasuredValue, ThresholdValue: *wire.ThresholdValue, RunID: wire.RunID, ScenarioID: wire.ScenarioID,
		Status: wire.Status, AutoApproved: *wire.AutoApproved, ReportableToOHA: *wire.ReportableToOHA, EvaluatedAt: wire.EvaluatedAt,
	}
	evaluatedAt, err := time.Parse(time.RFC3339Nano, observed.EvaluatedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: healthcare state observation evaluated_at: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if evaluatedAt.Before(expected.NotBefore) || evaluatedAt.After(collectedAt) {
		return nil, fmt.Errorf("%w: healthcare state observation timestamp is outside the scenario collection window", constants.ErrInvalidEvidenceGraph)
	}
	if observed.RunID != expected.RunID || observed.ScenarioID != expected.ScenarioID || observed.RequestID != expected.RequestID ||
		observed.Action != expected.Action || observed.ResourceType != expected.ResourceType || observed.Subject != expected.Subject ||
		observed.Status != expected.Status || observed.MeasuredValue != expected.MeasuredValue || observed.ThresholdValue != expected.ThresholdValue ||
		observed.AutoApproved != expected.AutoApproved || observed.ReportableToOHA != expected.ReportableToOHA {
		return nil, fmt.Errorf("%w: healthcare state observation does not match the canonical terminal fixture", constants.ErrInvalidEvidenceGraph)
	}
	return &healthcareStateCollection{
		CollectorID:             healthcareStateCollectorID,
		CollectorVersion:        healthcareStateCollectorVersion,
		Boundary:                healthcareActuatorBoundary,
		InitialStateFixtureRef:  expected.InitialStateFixtureRef,
		TerminalStateAssertions: append([]string(nil), expected.TerminalStateAssertions...),
		CollectedAt:             collectedAt,
		Observation:             observed,
	}, nil
}

func encodeHealthcareStateCollection(collection *healthcareStateCollection) ([]byte, string, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode healthcare state collection: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return encoded, "state-observation:sha256:" + hex.EncodeToString(digest[:]), nil
}

func collectHealthcareStateEvidence(ctx context.Context, demoDir string, result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition, requestID, action, resourceType, subject, status string, measuredValue, thresholdValue int, autoApproved, reportableToOHA bool) (string, string, error) {
	command := exec.CommandContext(ctx, "docker", "compose", "exec", "-T", "healthcare-actuator", "python", constants.ContainerVerifyPAPy,
		result.GetRunId(), result.GetScenarioRef().GetId(), requestID, action, status, strconv.Itoa(measuredValue), strconv.Itoa(thresholdValue))
	command.Dir = demoDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", "", fmt.Errorf("%w: healthcare state collector: %v: %s", constants.ErrInvalidEvidenceGraph, err, strings.TrimSpace(stderr.String()))
	}
	collection, err := decodeHealthcareStateObservation(stdout.Bytes(), healthcareStateExpectation{
		RunID: result.GetRunId(), ScenarioID: result.GetScenarioRef().GetId(), RequestID: requestID, Action: action,
		ResourceType: resourceType, Subject: subject, Status: status, MeasuredValue: measuredValue, ThresholdValue: thresholdValue,
		AutoApproved: autoApproved, ReportableToOHA: reportableToOHA, InitialStateFixtureRef: definition.GetInitialStateFixtureRef(),
		TerminalStateAssertions: definition.GetTerminalStateAssertions(), NotBefore: result.GetStartedAt().AsTime(),
	}, time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	encoded, evidenceRef, err := encodeHealthcareStateCollection(collection)
	if err != nil {
		return "", "", err
	}
	return string(encoded), evidenceRef, nil
}

const (
	healthcareNetworkCollectorID      = "healthcare-network-isolation"
	healthcareNetworkCollectorVersion = "1.0.0"
	healthcareNetworkBoundary         = "healthcare-network"
	healthcareUntrustedBoundary       = "net_untrusted"
	healthcareInternalBoundary        = "net_internal"
	healthcareInternalGatewayEndpoint = "http://10.22.0.10:8080/"
)

type healthcareNetworkExpectation struct {
	RunID                   string
	ScenarioID              string
	SourceBoundary          string
	TargetBoundary          string
	TargetEndpoint          string
	Reachable               bool
	InitialStateFixtureRef  string
	TerminalStateAssertions []string
	NotBefore               time.Time
}

type healthcareNetworkObservation struct {
	ObservedAt     string `json:"observed_at"`
	Reachable      bool   `json:"reachable"`
	RunID          string `json:"run_id"`
	ScenarioID     string `json:"scenario_id"`
	SourceBoundary string `json:"source_boundary"`
	TargetBoundary string `json:"target_boundary"`
	TargetEndpoint string `json:"target_endpoint"`
}

type healthcareNetworkCollection struct {
	CollectorID             string                       `json:"collector_id"`
	CollectorVersion        string                       `json:"collector_version"`
	Boundary                string                       `json:"boundary"`
	InitialStateFixtureRef  string                       `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string                     `json:"terminal_state_assertions"`
	CollectedAt             time.Time                    `json:"collected_at"`
	Observation             healthcareNetworkObservation `json:"observation"`
}

type healthcareNetworkObservationWire struct {
	ObservedAt     string `json:"observed_at"`
	Reachable      *bool  `json:"reachable"`
	RunID          string `json:"run_id"`
	ScenarioID     string `json:"scenario_id"`
	SourceBoundary string `json:"source_boundary"`
	TargetBoundary string `json:"target_boundary"`
	TargetEndpoint string `json:"target_endpoint"`
}

func decodeHealthcareNetworkObservation(raw []byte, expected healthcareNetworkExpectation, collectedAt time.Time) (*healthcareNetworkCollection, error) {
	if expected.InitialStateFixtureRef == "" || len(expected.TerminalStateAssertions) == 0 {
		return nil, fmt.Errorf("%w: healthcare network collector lacks canonical fixture binding", constants.ErrInvalidEvidenceGraph)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire healthcareNetworkObservationWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: decode healthcare network observation: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: healthcare network observation contains trailing JSON", constants.ErrInvalidEvidenceGraph)
	}
	if wire.Reachable == nil {
		return nil, fmt.Errorf("%w: healthcare network observation omits reachability", constants.ErrInvalidEvidenceGraph)
	}
	observed := healthcareNetworkObservation{
		ObservedAt: wire.ObservedAt, Reachable: *wire.Reachable, RunID: wire.RunID, ScenarioID: wire.ScenarioID,
		SourceBoundary: wire.SourceBoundary, TargetBoundary: wire.TargetBoundary, TargetEndpoint: wire.TargetEndpoint,
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observed.ObservedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: healthcare network observation observed_at: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if observedAt.Before(expected.NotBefore) || observedAt.After(collectedAt) {
		return nil, fmt.Errorf("%w: healthcare network observation timestamp is outside the scenario collection window", constants.ErrInvalidEvidenceGraph)
	}
	if observed.RunID != expected.RunID || observed.ScenarioID != expected.ScenarioID || observed.SourceBoundary != expected.SourceBoundary ||
		observed.TargetBoundary != expected.TargetBoundary || observed.TargetEndpoint != expected.TargetEndpoint || observed.Reachable != expected.Reachable {
		return nil, fmt.Errorf("%w: healthcare network observation does not match the canonical terminal fixture", constants.ErrInvalidEvidenceGraph)
	}
	return &healthcareNetworkCollection{
		CollectorID:             healthcareNetworkCollectorID,
		CollectorVersion:        healthcareNetworkCollectorVersion,
		Boundary:                healthcareNetworkBoundary,
		InitialStateFixtureRef:  expected.InitialStateFixtureRef,
		TerminalStateAssertions: append([]string(nil), expected.TerminalStateAssertions...),
		CollectedAt:             collectedAt,
		Observation:             observed,
	}, nil
}

func encodeHealthcareNetworkCollection(collection *healthcareNetworkCollection) ([]byte, string, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode healthcare network collection: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return encoded, "state-observation:sha256:" + hex.EncodeToString(digest[:]), nil
}

func collectHealthcareNetworkEvidence(ctx context.Context, demoDir string, result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition) (string, string, error) {
	command := exec.CommandContext(ctx, "docker", "compose", "exec", "-T", "bad-actor", "sh", constants.ContainerHealthcareNetCollectorFile,
		result.GetRunId(), result.GetScenarioRef().GetId(), healthcareUntrustedBoundary, healthcareInternalBoundary, healthcareInternalGatewayEndpoint)
	command.Dir = demoDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", "", fmt.Errorf("%w: healthcare network collector: %v: %s", constants.ErrInvalidEvidenceGraph, err, strings.TrimSpace(stderr.String()))
	}
	collection, err := decodeHealthcareNetworkObservation(stdout.Bytes(), healthcareNetworkExpectation{
		RunID: result.GetRunId(), ScenarioID: result.GetScenarioRef().GetId(), SourceBoundary: healthcareUntrustedBoundary,
		TargetBoundary: healthcareInternalBoundary, TargetEndpoint: healthcareInternalGatewayEndpoint, Reachable: false,
		InitialStateFixtureRef: definition.GetInitialStateFixtureRef(), TerminalStateAssertions: definition.GetTerminalStateAssertions(),
		NotBefore: result.GetStartedAt().AsTime().Truncate(time.Second),
	}, time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	encoded, evidenceRef, err := encodeHealthcareNetworkCollection(collection)
	if err != nil {
		return "", "", err
	}
	return string(encoded), evidenceRef, nil
}

func runHealthcareScenario(ctx context.Context, fileSvc fs.RuntimeFileService, demoDir, runID, scenario string) (*compliancev1.DemoScenarioResult, error) {
	var result *compliancev1.DemoScenarioResult

	switch scenario {
	case "1":
		definition, err := loadDemoScenarioDefinition("healthcare-success")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newHealthcareSuccessScenarioResult(startedAt, definition, runID)
		var hasErrors bool

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintf("  Scenario 1 — %s\n", definition.GetTitle())
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: An authorized agent on net_internal submits a PA")
		demoPrintln("          request through the g8e gateway via the native")
		demoPrintln("          run_shell_command tool driving the paop wrapper.")
		demoPrintln("          Every request passes through the doctrine engine")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm g8e gateway is live ──────────────────────")
		step1Started := time.Now().UTC()
		step1Err := demoStep(ctx, demoDir, "gateway health", false,
			"curl", "-s", "http://localhost:8081/api/v1/health")
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-success-step-1", "gateway health check", step1Started, time.Now().UTC(),
			step1Err == nil, true, "curl gateway health endpoint"))
		if step1Err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Submit PA request through the gateway ───────────")
		demoPrintln("  Request path: agent-runtime → gateway (g8e.local:8443) [Governed run_shell_command]")
		demoPrintln()
		hcfg := harnessConfigForResult("agent-runtime", result)
		hcfg.PublicURL = "http://g8e.local:8081"
		hcfg.JSON = true
		step2Started := time.Now().UTC()
		harnessResults, step2Err := runHarnessWithJSON(ctx, demoDir, "fhir request",
			harnessRun("healthcare-success", hcfg))
		step2OK := step2Err == nil
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-success-step-2", "healthcare success harness", step2Started, time.Now().UTC(),
			step2OK, true, "agent harness submits PA through governed native tool"))
		if !step2OK {
			fmt.Println("  (healthcare-success harness scenario failed)")
			fmt.Println()
			hasErrors = true
		} else if len(harnessResults) == 0 || !applyAndPersistHarnessIdentity(ctx, fileSvc, runID, result, &harnessResults[0]) {
			fmt.Println("  (healthcare-success harness emitted no authoritative receipt)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Independently verify the PA submission record ────")
		step3Started := time.Now().UTC()
		protocolResult, evidenceRef, step3Err := collectHealthcareStateEvidence(ctx, demoDir, result, definition,
			"PA-2026-0045", "submit", "ClaimResponse", "preauthorization", "SUBMITTED", 0, 0, false, false)
		step3Result := buildDemoStepResult(
			"healthcare-success-step-3", "independent state observation: PA submission recorded", step3Started, time.Now().UTC(),
			step3Err == nil, true, protocolResult)
		if step3Err != nil {
			fmt.Println("  (PA submission state observation failed)")
			hasErrors = true
		} else {
			step3Result.EvidenceRefs = append(step3Result.EvidenceRefs, evidenceRef)
			result.StateObservationRefs = append(result.StateObservationRefs, evidenceRef)
		}
		result.StepResults = append(result.StepResults, step3Result)

		demoPrintln("  ── Step 4: View g8e enforcement audit ───────────────────────")
		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")
		demoPrintln()
		step4Started := time.Now().UTC()
		step4Err := demoStep(ctx, demoDir, "audit tail", false,
			"docker", "compose", "logs", "observability", "--tail", "10")
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-success-step-4", "supplementary audit log observation", step4Started, time.Now().UTC(),
			step4Err == nil, false, "observability audit log tail"))
		if step4Err != nil {
			fmt.Println("  (warning: audit tail failed)")
		}

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 1 — PA request submitted through governed native tool.")
			fmt.Println("         Doctrine engine evaluated the payload against all 11 PHI/HIPAA rules.")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate healthcare-success scenario result: %w", err)
		}

	case "2":
		definition, err := loadDemoScenarioDefinition("healthcare-gold-card")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newHealthcareGoldCardScenarioResult(startedAt, definition, runID)
		var hasErrors bool

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintf("  Scenario 2 — %s\n", definition.GetTitle())
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: The governed healthcare actuator evaluates a 96% historical")
		demoPrintln("          approval rate against the configured 90% threshold and records")
		demoPrintln("          the run-bound AUTO_APPROVED terminal state.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm g8e gateway is live ──────────────────────")
		step1Started := time.Now().UTC()
		step1Err := demoStep(ctx, demoDir, "gateway health", false,
			"curl", "-s", "http://localhost:8081/api/v1/health")
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-gold-card-step-1", "gateway health check", step1Started, time.Now().UTC(),
			step1Err == nil, true, "curl gateway health endpoint"))
		if step1Err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Evaluate gold-card policy through the gateway ────")
		demoPrintln("  PA-2026-0043 (Dr. Priya Nair, 96% historic approval rate) is evaluated")
		demoPrintln("  against the 90% threshold after the governed envelope is admitted.")
		demoPrintln()
		hcfg := harnessConfigForResult("agent-runtime", result)
		hcfg.PublicURL = "http://g8e.local:8081"
		hcfg.JSON = true
		step2Started := time.Now().UTC()
		harnessResults, step2Err := runHarnessWithJSON(ctx, demoDir, "gold-card PA via agent",
			harnessRun("healthcare-gold-card", hcfg))
		step2OK := step2Err == nil
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-gold-card-step-2", "governed gold-card threshold evaluation", step2Started, time.Now().UTC(),
			step2OK, true, "agent harness invokes the healthcare actuator after governance"))
		if !step2OK {
			fmt.Println("  (healthcare-gold-card harness scenario failed)")
			fmt.Println()
			hasErrors = true
		} else if len(harnessResults) == 0 || !applyAndPersistHarnessIdentity(ctx, fileSvc, runID, result, &harnessResults[0]) {
			fmt.Println("  (healthcare-gold-card harness emitted no authoritative receipt)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Collect the run-bound AUTO_APPROVED state ────────")
		step3Started := time.Now().UTC()
		protocolResult, evidenceRef, step3Err := collectHealthcareStateEvidence(ctx, demoDir, result, definition,
			"PA-2026-0043", "gold-card", "ClaimResponse", "Dr. Priya Nair", "AUTO_APPROVED", 96, 90, true, false)
		step3Result := buildDemoStepResult(
			"healthcare-gold-card-step-3", "independent state observation: gold-card auto-approved", step3Started, time.Now().UTC(),
			step3Err == nil, true, protocolResult)
		if step3Err != nil {
			fmt.Println("  (gold-card state observation failed)")
			hasErrors = true
		} else {
			step3Result.EvidenceRefs = append(step3Result.EvidenceRefs, evidenceRef)
			result.StateObservationRefs = append(result.StateObservationRefs, evidenceRef)
			result.MetricRefs = append(result.MetricRefs, "metric:healthcare-provider-approval-rate:96", "metric:healthcare-gold-card-threshold:90")
		}
		result.StepResults = append(result.StepResults, step3Result)

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 2 — One or more steps failed.")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 2 — Gold-card threshold evaluated and auto-approved.")
			fmt.Println("         The typed terminal observation is bound to this scenario run.")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate healthcare-gold-card scenario result: %w", err)
		}

	case "3":
		definition, err := loadDemoScenarioDefinition("healthcare-sla-breach")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newHealthcareSLABreachScenarioResult(startedAt, definition, runID)
		var hasErrors bool

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintf("  Scenario 3 — %s\n", definition.GetTitle())
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: The governed healthcare actuator evaluates 10 elapsed days")
		demoPrintln("          against the seven-day SLA and records a run-bound")
		demoPrintln("          SLA_BREACHED state that is reportable to OHA.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm g8e gateway is live ──────────────────────")
		step1Started := time.Now().UTC()
		step1Err := demoStep(ctx, demoDir, "gateway health", false,
			"curl", "-s", "http://localhost:8081/api/v1/health")
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-sla-breach-step-1", "gateway health check", step1Started, time.Now().UTC(),
			step1Err == nil, true, "curl gateway health endpoint"))
		if step1Err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Evaluate the SLA through the gateway ─────────────")
		demoPrintln("  PA-2026-0044 (Dr. James O'Brien, 10 days elapsed) is evaluated")
		demoPrintln("  against the seven-day SLA after the governed envelope is admitted.")
		demoPrintln()
		hcfg := harnessConfigForResult("agent-runtime", result)
		hcfg.PublicURL = "http://g8e.local:8081"
		hcfg.JSON = true
		step2Started := time.Now().UTC()
		harnessResults, step2Err := runHarnessWithJSON(ctx, demoDir, "SLA breach evaluation via agent",
			harnessRun("healthcare-sla-breach", hcfg))
		step2OK := step2Err == nil
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-sla-breach-step-2", "governed SLA evaluation", step2Started, time.Now().UTC(),
			step2OK, true, "agent harness invokes the healthcare actuator after governance"))
		if !step2OK {
			fmt.Println("  (healthcare-sla-breach harness scenario failed)")
			fmt.Println()
			hasErrors = true
		} else if len(harnessResults) == 0 || !applyAndPersistHarnessIdentity(ctx, fileSvc, runID, result, &harnessResults[0]) {
			fmt.Println("  (healthcare-sla-breach harness emitted no authoritative receipt)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Collect the run-bound SLA_BREACHED state ─────────")
		step3Started := time.Now().UTC()
		protocolResult, evidenceRef, step3Err := collectHealthcareStateEvidence(ctx, demoDir, result, definition,
			"PA-2026-0044", "sla-check", "ClaimResponse", "Dr. James O'Brien", "SLA_BREACHED", 10, 7, false, true)
		step3Result := buildDemoStepResult(
			"healthcare-sla-breach-step-3", "independent state observation: SLA breached and reportable", step3Started, time.Now().UTC(),
			step3Err == nil, true, protocolResult)
		if step3Err != nil {
			fmt.Println("  (SLA breach state observation failed)")
			hasErrors = true
		} else {
			step3Result.EvidenceRefs = append(step3Result.EvidenceRefs, evidenceRef)
			result.StateObservationRefs = append(result.StateObservationRefs, evidenceRef)
			result.MetricRefs = append(result.MetricRefs, "metric:healthcare-sla-elapsed-days:10", "metric:healthcare-sla-threshold-days:7")
		}
		result.StepResults = append(result.StepResults, step3Result)

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 3 — SLA breach calculated and marked reportable.")
			fmt.Println("         The typed terminal observation is bound to this scenario run.")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate healthcare-sla-breach scenario result: %w", err)
		}

	case "4":
		definition, err := loadDemoScenarioDefinition("healthcare-phi-blocked")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newHealthcarePHIBlockedScenarioResult(startedAt, definition, runID)
		var hasErrors bool

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintf("  Scenario 4 — %s\n", definition.GetTitle())
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: Two-layer defense.")
		demoPrintln("    Layer 1 — Network isolation: bad-actor on net_untrusted has no")
		demoPrintln("              route to net_internal or net_secure.")
		demoPrintln("    Layer 2 — Doctrine enforcement: the g8e gateway blocks PHI")
		demoPrintln("              exfiltration payloads at confidence ≥0.95 (phi_exfil_attempt).")
		demoPrintln()

		demoPrintln("  ── Layer 1: Network isolation ────────────────────────────────")
		demoPrintln("  bad-actor (net_untrusted) → gateway (net_internal) — should timeout")
		demoPrintln()
		step1Started := time.Now().UTC()
		protocolResult, evidenceRef, step1Err := collectHealthcareNetworkEvidence(ctx, demoDir, result, definition)
		step1Result := buildDemoStepResult(
			"healthcare-phi-blocked-step-1", "independent state observation: untrusted network isolated", step1Started, time.Now().UTC(),
			step1Err == nil, true, protocolResult)
		if step1Err != nil {
			fmt.Println("  (network isolation check failed)")
			hasErrors = true
		} else {
			step1Result.EvidenceRefs = append(step1Result.EvidenceRefs, evidenceRef)
			result.StateObservationRefs = append(result.StateObservationRefs, evidenceRef)
		}
		result.StepResults = append(result.StepResults, step1Result)

		demoPrintln("  ── Layer 2: g8e doctrine enforcement ─────────────────────────")
		demoPrintln("  Submit a PHI exfiltration attempt through the production-ready")
		demoPrintln("  governed native tool endpoint (mTLS + Protocol Envelopes):")
		demoPrintln()
		hcfg := harnessConfigForResult("agent-runtime", result)
		hcfg.PublicURL = "http://g8e.local:8081"
		hcfg.JSON = true
		step2Started := time.Now().UTC()
		harnessResults, step2Err := runHarnessWithJSON(ctx, demoDir, "phi exfiltration",
			harnessRun("healthcare-phi-blocked", hcfg))
		// The harness exits successfully only after it verifies that doctrine blocked
		// the PHI exfiltration attempt. A nonzero exit means the expected rejection was not verified.
		step2OK := step2Err == nil
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-phi-blocked-step-2", "healthcare PHI exfiltration harness", step2Started, time.Now().UTC(),
			step2OK, true, "agent harness verifies L1 doctrine rejection"))
		if !step2OK {
			fmt.Println("  (healthcare-phi-blocked harness scenario failed)")
			fmt.Println()
			hasErrors = true
		} else {
			if len(harnessResults) == 0 || !applyAndPersistHarnessIdentity(ctx, fileSvc, runID, result, &harnessResults[0]) {
				hasErrors = true
			}
		}
		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")
		demoPrintln()

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 4 — One or more steps failed.")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 4 — PHI exfiltration blocked at both layers.")
			fmt.Println("         Layer 1: network isolation (net_untrusted has no route to net_internal).")
			fmt.Println("         Layer 2: doctrine phi_exfil_attempt loaded at confidence 0.95.")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate healthcare-phi-blocked scenario result: %w", err)
		}

	default:
		return nil, fmt.Errorf("invalid scenario number for healthcare: %q (valid: 1-4)", scenario)
	}

	return result, nil
}
