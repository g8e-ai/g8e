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

	"github.com/g8e-ai/g8e/v2/internal/cli/tui"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancecatalog "github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// defaultDHSHarnessConfig returns the config matching the DHS compose topology.
func defaultDHSHarnessConfig() harnessConfig {
	return defaultHarnessConfig("agent-coalition")
}

func newDHSSovereignIngestScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgDHS, constants.DemoScopeDHS,
		"L1 doctrine admits // L2 consensus quorum met // L5 actuator records INGEST")
}

func newDHSDisconnectedOperationsScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgDHS, constants.DemoScopeDHS,
		"Datalink severed // Local governance continues // Git ledger + SQLite vault")
}

func newDHSCueScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgDHS, constants.DemoScopeDHS,
		"L2 quorum admits cue // L5 actuator records CUE")
}

func dhsScenarioDefinitionIDs(scenario string) ([]string, error) {
	switch scenario {
	case "1":
		return []string{"dhs-ingest"}, nil
	case "2":
		return []string{"dhs-disconnected-operations"}, nil
	case "3":
		return []string{"dhs-cue"}, nil
	case "4":
		return []string{"dhs-destruction-block", "dhs-destruction-purge"}, nil
	default:
		return nil, fmt.Errorf("%w: invalid scenario number for dhs: %q (valid: 1-4)", constants.ErrNotFound, scenario)
	}
}

func newDHSDestructionScenarioResults(startedAt time.Time, blockDefinition, purgeDefinition *compliancev1.DemoScenarioDefinition) []*compliancev1.DemoScenarioResult {
	return []*compliancev1.DemoScenarioResult{
		newDemoEvidenceScenarioResult(startedAt, blockDefinition, constants.DemosOrgDHS, constants.DemoScopeDHS,
			"L1 blocks audit wipe // audit vault remains intact"),
		newDemoEvidenceScenarioResult(startedAt, purgeDefinition, constants.DemosOrgDHS, constants.DemoScopeDHS,
			"L1+L2 admit governed purge // L5 actuator records PURGE"),
	}
}

const (
	dhsDataServiceCollectorID      = "dhs-data-service-state"
	dhsDataServiceCollectorVersion = "1.0.0"
	dhsDataServiceBoundary         = "dhs-sovereign-data-service"
)

type dhsDataServiceExpectation struct {
	RunID                   string
	ScenarioID              string
	Action                  string
	RecordID                string
	Detail                  string
	OperationFound          bool
	InitialStateFixtureRef  string
	TerminalStateAssertions []string
	NotBefore               time.Time
}

type dhsDataServiceObservation struct {
	Action             string `json:"action"`
	Detail             string `json:"detail"`
	ObservedAt         string `json:"observed_at"`
	OperationFound     bool   `json:"operation_found"`
	OperationTimestamp string `json:"operation_timestamp"`
	RecordID           string `json:"record_id"`
	RunID              string `json:"run_id"`
	ScenarioID         string `json:"scenario_id"`
}

type dhsDataServiceCollection struct {
	CollectorID             string                    `json:"collector_id"`
	CollectorVersion        string                    `json:"collector_version"`
	Boundary                string                    `json:"boundary"`
	InitialStateFixtureRef  string                    `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string                  `json:"terminal_state_assertions"`
	CollectedAt             time.Time                 `json:"collected_at"`
	Observation             dhsDataServiceObservation `json:"observation"`
}

type dhsDataServiceObservationWire struct {
	Action             string `json:"action"`
	Detail             string `json:"detail"`
	ObservedAt         string `json:"observed_at"`
	OperationFound     *bool  `json:"operation_found"`
	OperationTimestamp string `json:"operation_timestamp"`
	RecordID           string `json:"record_id"`
	RunID              string `json:"run_id"`
	ScenarioID         string `json:"scenario_id"`
}

func decodeDHSDataServiceObservation(raw []byte, expected dhsDataServiceExpectation, collectedAt time.Time) (*dhsDataServiceCollection, error) {
	if expected.InitialStateFixtureRef == "" || len(expected.TerminalStateAssertions) == 0 {
		return nil, fmt.Errorf("%w: DHS data-service collector lacks canonical fixture binding", constants.ErrInvalidEvidenceGraph)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire dhsDataServiceObservationWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: decode DHS data-service observation: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: DHS data-service observation contains trailing JSON", constants.ErrInvalidEvidenceGraph)
	}
	if wire.OperationFound == nil {
		return nil, fmt.Errorf("%w: DHS data-service observation omits operation presence", constants.ErrInvalidEvidenceGraph)
	}
	observed := dhsDataServiceObservation{
		Action: wire.Action, Detail: wire.Detail, ObservedAt: wire.ObservedAt, OperationFound: *wire.OperationFound,
		OperationTimestamp: wire.OperationTimestamp, RecordID: wire.RecordID, RunID: wire.RunID, ScenarioID: wire.ScenarioID,
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observed.ObservedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: DHS data-service observation observed_at: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	operationAt, err := time.Parse(time.RFC3339Nano, observed.OperationTimestamp)
	if err != nil {
		return nil, fmt.Errorf("%w: DHS data-service observation operation_timestamp: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if observedAt.Before(expected.NotBefore) || observedAt.After(collectedAt) || operationAt.Before(expected.NotBefore) || operationAt.After(observedAt) {
		return nil, fmt.Errorf("%w: DHS data-service observation timestamp is outside the scenario collection window", constants.ErrInvalidEvidenceGraph)
	}
	if observed.RunID != expected.RunID || observed.ScenarioID != expected.ScenarioID || observed.Action != expected.Action ||
		observed.RecordID != expected.RecordID || observed.Detail != expected.Detail || observed.OperationFound != expected.OperationFound {
		return nil, fmt.Errorf("%w: DHS data-service observation does not match the canonical terminal fixture", constants.ErrInvalidEvidenceGraph)
	}
	return &dhsDataServiceCollection{
		CollectorID: dhsDataServiceCollectorID, CollectorVersion: dhsDataServiceCollectorVersion, Boundary: dhsDataServiceBoundary,
		InitialStateFixtureRef: expected.InitialStateFixtureRef, TerminalStateAssertions: append([]string(nil), expected.TerminalStateAssertions...),
		CollectedAt: collectedAt, Observation: observed,
	}, nil
}

func encodeDHSDataServiceCollection(collection *dhsDataServiceCollection) ([]byte, string, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode DHS data-service collection: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return encoded, "state-observation:sha256:" + hex.EncodeToString(digest[:]), nil
}

func collectDHSDataServiceEvidence(ctx context.Context, demoDir string, result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition, action, recordID, detail string) (string, string, error) {
	command := exec.CommandContext(ctx, "docker", "compose", "exec", "-T", "datasvc", "python", constants.ContainerVerifyOpsPy,
		result.GetRunId(), result.GetScenarioRef().GetId(), action, recordID, detail)
	command.Dir = demoDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", "", fmt.Errorf("%w: DHS data-service collector: %v: %s", constants.ErrInvalidEvidenceGraph, err, strings.TrimSpace(stderr.String()))
	}
	collection, err := decodeDHSDataServiceObservation(stdout.Bytes(), dhsDataServiceExpectation{
		RunID: result.GetRunId(), ScenarioID: result.GetScenarioRef().GetId(), Action: action, RecordID: recordID, Detail: detail,
		OperationFound: true, InitialStateFixtureRef: definition.GetInitialStateFixtureRef(),
		TerminalStateAssertions: definition.GetTerminalStateAssertions(), NotBefore: result.GetStartedAt().AsTime().Truncate(time.Second),
	}, time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	encoded, evidenceRef, err := encodeDHSDataServiceCollection(collection)
	if err != nil {
		return "", "", err
	}
	return string(encoded), evidenceRef, nil
}

const (
	dhsNetworkCollectorID      = "dhs-network-membership"
	dhsNetworkCollectorVersion = "1.0.0"
	dhsNetworkBoundary         = "docker-network-control-plane"
)

type dhsNetworkExpectation struct {
	RunID                   string
	ScenarioID              string
	NetworkName             string
	ContainerName           string
	Connected               bool
	InitialStateFixtureRef  string
	TerminalStateAssertions []string
	NotBefore               time.Time
}

type dhsNetworkObservation struct {
	Connected     bool   `json:"connected"`
	ContainerName string `json:"container_name"`
	NetworkName   string `json:"network_name"`
	ObservedAt    string `json:"observed_at"`
	RunID         string `json:"run_id"`
	ScenarioID    string `json:"scenario_id"`
}

type dhsNetworkCollection struct {
	CollectorID             string                `json:"collector_id"`
	CollectorVersion        string                `json:"collector_version"`
	Boundary                string                `json:"boundary"`
	InitialStateFixtureRef  string                `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string              `json:"terminal_state_assertions"`
	CollectedAt             time.Time             `json:"collected_at"`
	Observation             dhsNetworkObservation `json:"observation"`
}

type dhsNetworkObservationWire struct {
	Connected     *bool  `json:"connected"`
	ContainerName string `json:"container_name"`
	NetworkName   string `json:"network_name"`
	ObservedAt    string `json:"observed_at"`
	RunID         string `json:"run_id"`
	ScenarioID    string `json:"scenario_id"`
}

func decodeDHSNetworkObservation(raw []byte, expected dhsNetworkExpectation, collectedAt time.Time) (*dhsNetworkCollection, error) {
	if expected.InitialStateFixtureRef == "" || len(expected.TerminalStateAssertions) == 0 {
		return nil, fmt.Errorf("%w: DHS network collector lacks canonical fixture binding", constants.ErrInvalidEvidenceGraph)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire dhsNetworkObservationWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: decode DHS network observation: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: DHS network observation contains trailing JSON", constants.ErrInvalidEvidenceGraph)
	}
	if wire.Connected == nil {
		return nil, fmt.Errorf("%w: DHS network observation omits container membership", constants.ErrInvalidEvidenceGraph)
	}
	observed := dhsNetworkObservation{
		Connected: *wire.Connected, ContainerName: wire.ContainerName, NetworkName: wire.NetworkName,
		ObservedAt: wire.ObservedAt, RunID: wire.RunID, ScenarioID: wire.ScenarioID,
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observed.ObservedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: DHS network observation observed_at: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if observedAt.Before(expected.NotBefore) || observedAt.After(collectedAt) {
		return nil, fmt.Errorf("%w: DHS network observation timestamp is outside the scenario collection window", constants.ErrInvalidEvidenceGraph)
	}
	if observed.RunID != expected.RunID || observed.ScenarioID != expected.ScenarioID || observed.NetworkName != expected.NetworkName ||
		observed.ContainerName != expected.ContainerName || observed.Connected != expected.Connected {
		return nil, fmt.Errorf("%w: DHS network observation does not match the canonical terminal fixture", constants.ErrInvalidEvidenceGraph)
	}
	return &dhsNetworkCollection{
		CollectorID: dhsNetworkCollectorID, CollectorVersion: dhsNetworkCollectorVersion, Boundary: dhsNetworkBoundary,
		InitialStateFixtureRef: expected.InitialStateFixtureRef, TerminalStateAssertions: append([]string(nil), expected.TerminalStateAssertions...),
		CollectedAt: collectedAt, Observation: observed,
	}, nil
}

func encodeDHSNetworkCollection(collection *dhsNetworkCollection) ([]byte, string, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode DHS network collection: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return encoded, "state-observation:sha256:" + hex.EncodeToString(digest[:]), nil
}

func collectDHSNetworkEvidence(ctx context.Context, demoDir string, result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition, connected bool) (string, string, error) {
	command := exec.CommandContext(ctx, "sh", constants.DemosDHSNetworkCollectorFile,
		result.GetRunId(), result.GetScenarioRef().GetId(), constants.DemosDHSPerimeterNetwork, constants.DemosDHSCoalitionDatalinkCtnr)
	command.Dir = demoDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", "", fmt.Errorf("%w: DHS network collector: %v: %s", constants.ErrInvalidEvidenceGraph, err, strings.TrimSpace(stderr.String()))
	}
	collection, err := decodeDHSNetworkObservation(stdout.Bytes(), dhsNetworkExpectation{
		RunID: result.GetRunId(), ScenarioID: result.GetScenarioRef().GetId(), NetworkName: constants.DemosDHSPerimeterNetwork,
		ContainerName: constants.DemosDHSCoalitionDatalinkCtnr, Connected: connected,
		InitialStateFixtureRef: definition.GetInitialStateFixtureRef(), TerminalStateAssertions: definition.GetTerminalStateAssertions(),
		NotBefore: result.GetStartedAt().AsTime().Truncate(time.Second),
	}, time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	encoded, evidenceRef, err := encodeDHSNetworkCollection(collection)
	if err != nil {
		return "", "", err
	}
	return string(encoded), evidenceRef, nil
}

// ── DHS local gateway health collector ──────────────────────────────────────

const (
	dhsGatewayHealthCollectorID      = "dhs-local-gateway-health"
	dhsGatewayHealthCollectorVersion = "1.0.0"
	dhsGatewayHealthBoundary         = "dhs-local-gateway-endpoint"
)

type dhsGatewayHealthExpectation struct {
	RunID                   string
	ScenarioID              string
	Endpoint                string
	Available               bool
	InitialStateFixtureRef  string
	TerminalStateAssertions []string
	NotBefore               time.Time
}

type dhsGatewayHealthObservation struct {
	Available  bool   `json:"available"`
	Endpoint   string `json:"endpoint"`
	ObservedAt string `json:"observed_at"`
	RunID      string `json:"run_id"`
	ScenarioID string `json:"scenario_id"`
}

type dhsGatewayHealthCollection struct {
	CollectorID             string                      `json:"collector_id"`
	CollectorVersion        string                      `json:"collector_version"`
	Boundary                string                      `json:"boundary"`
	InitialStateFixtureRef  string                      `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string                    `json:"terminal_state_assertions"`
	CollectedAt             time.Time                   `json:"collected_at"`
	Observation             dhsGatewayHealthObservation `json:"observation"`
}

type dhsGatewayHealthObservationWire struct {
	Available  *bool  `json:"available"`
	Endpoint   string `json:"endpoint"`
	ObservedAt string `json:"observed_at"`
	RunID      string `json:"run_id"`
	ScenarioID string `json:"scenario_id"`
}

func decodeDHSGatewayHealthObservation(raw []byte, expected dhsGatewayHealthExpectation, collectedAt time.Time) (*dhsGatewayHealthCollection, error) {
	if expected.InitialStateFixtureRef == "" || len(expected.TerminalStateAssertions) == 0 {
		return nil, fmt.Errorf("%w: DHS gateway health collector lacks canonical fixture binding", constants.ErrInvalidEvidenceGraph)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire dhsGatewayHealthObservationWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: decode DHS gateway health observation: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: DHS gateway health observation contains trailing JSON", constants.ErrInvalidEvidenceGraph)
	}
	if wire.Available == nil {
		return nil, fmt.Errorf("%w: DHS gateway health observation omits availability", constants.ErrInvalidEvidenceGraph)
	}
	observed := dhsGatewayHealthObservation{
		Available: *wire.Available, Endpoint: wire.Endpoint, ObservedAt: wire.ObservedAt,
		RunID: wire.RunID, ScenarioID: wire.ScenarioID,
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observed.ObservedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: DHS gateway health observation observed_at: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if observedAt.Before(expected.NotBefore) || observedAt.After(collectedAt) {
		return nil, fmt.Errorf("%w: DHS gateway health observation timestamp is outside the scenario collection window", constants.ErrInvalidEvidenceGraph)
	}
	if observed.RunID != expected.RunID || observed.ScenarioID != expected.ScenarioID ||
		observed.Endpoint != expected.Endpoint || observed.Available != expected.Available {
		return nil, fmt.Errorf("%w: DHS gateway health observation does not match the canonical terminal fixture", constants.ErrInvalidEvidenceGraph)
	}
	return &dhsGatewayHealthCollection{
		CollectorID: dhsGatewayHealthCollectorID, CollectorVersion: dhsGatewayHealthCollectorVersion, Boundary: dhsGatewayHealthBoundary,
		InitialStateFixtureRef: expected.InitialStateFixtureRef, TerminalStateAssertions: append([]string(nil), expected.TerminalStateAssertions...),
		CollectedAt: collectedAt, Observation: observed,
	}, nil
}

func encodeDHSGatewayHealthCollection(collection *dhsGatewayHealthCollection) ([]byte, string, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode DHS gateway health collection: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return encoded, "state-observation:sha256:" + hex.EncodeToString(digest[:]), nil
}

func collectDHSGatewayHealthEvidence(ctx context.Context, demoDir string, result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition) (string, string, error) {
	command := exec.CommandContext(ctx, "sh", constants.DemosDHSGatewayHealthCollectorFile,
		result.GetRunId(), result.GetScenarioRef().GetId(), constants.DemosDHSGatewayHealthEndpoint)
	command.Dir = demoDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", "", fmt.Errorf("%w: DHS gateway health collector: %v: %s", constants.ErrInvalidEvidenceGraph, err, strings.TrimSpace(stderr.String()))
	}
	collection, err := decodeDHSGatewayHealthObservation(stdout.Bytes(), dhsGatewayHealthExpectation{
		RunID: result.GetRunId(), ScenarioID: result.GetScenarioRef().GetId(), Endpoint: constants.DemosDHSGatewayHealthEndpoint,
		Available: true, InitialStateFixtureRef: definition.GetInitialStateFixtureRef(),
		TerminalStateAssertions: definition.GetTerminalStateAssertions(), NotBefore: result.GetStartedAt().AsTime().Truncate(time.Second),
	}, time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	encoded, evidenceRef, err := encodeDHSGatewayHealthCollection(collection)
	if err != nil {
		return "", "", err
	}
	return string(encoded), evidenceRef, nil
}

// ── DHS local ledger persistence collector ──────────────────────────────────

const (
	dhsLedgerPersistenceCollectorID      = "dhs-local-ledger-persistence"
	dhsLedgerPersistenceCollectorVersion = "1.0.0"
	dhsLedgerPersistenceBoundary         = "dhs-operator-local-ledger"
)

type dhsLedgerPersistenceExpectation struct {
	RunID                   string
	ScenarioID              string
	Container               string
	Directory               string
	Persisted               bool
	InitialStateFixtureRef  string
	TerminalStateAssertions []string
	NotBefore               time.Time
}

type dhsLedgerPersistenceObservation struct {
	Persisted  bool   `json:"persisted"`
	Directory  string `json:"directory"`
	EntryCount int    `json:"entry_count"`
	ObservedAt string `json:"observed_at"`
	RunID      string `json:"run_id"`
	ScenarioID string `json:"scenario_id"`
}

type dhsLedgerPersistenceCollection struct {
	CollectorID             string                          `json:"collector_id"`
	CollectorVersion        string                          `json:"collector_version"`
	Boundary                string                          `json:"boundary"`
	InitialStateFixtureRef  string                          `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string                        `json:"terminal_state_assertions"`
	CollectedAt             time.Time                       `json:"collected_at"`
	Observation             dhsLedgerPersistenceObservation `json:"observation"`
}

type dhsLedgerPersistenceObservationWire struct {
	Persisted  *bool  `json:"persisted"`
	Directory  string `json:"directory"`
	EntryCount *int   `json:"entry_count"`
	ObservedAt string `json:"observed_at"`
	RunID      string `json:"run_id"`
	ScenarioID string `json:"scenario_id"`
}

func decodeDHSLedgerPersistenceObservation(raw []byte, expected dhsLedgerPersistenceExpectation, collectedAt time.Time) (*dhsLedgerPersistenceCollection, error) {
	if expected.InitialStateFixtureRef == "" || len(expected.TerminalStateAssertions) == 0 {
		return nil, fmt.Errorf("%w: DHS ledger collector lacks canonical fixture binding", constants.ErrInvalidEvidenceGraph)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire dhsLedgerPersistenceObservationWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: decode DHS ledger observation: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: DHS ledger observation contains trailing JSON", constants.ErrInvalidEvidenceGraph)
	}
	if wire.Persisted == nil {
		return nil, fmt.Errorf("%w: DHS ledger observation omits persistence", constants.ErrInvalidEvidenceGraph)
	}
	if wire.EntryCount == nil {
		return nil, fmt.Errorf("%w: DHS ledger observation omits entry count", constants.ErrInvalidEvidenceGraph)
	}
	observed := dhsLedgerPersistenceObservation{
		Persisted: *wire.Persisted, Directory: wire.Directory, EntryCount: *wire.EntryCount,
		ObservedAt: wire.ObservedAt, RunID: wire.RunID, ScenarioID: wire.ScenarioID,
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observed.ObservedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: DHS ledger observation observed_at: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if observedAt.Before(expected.NotBefore) || observedAt.After(collectedAt) {
		return nil, fmt.Errorf("%w: DHS ledger observation timestamp is outside the scenario collection window", constants.ErrInvalidEvidenceGraph)
	}
	if observed.RunID != expected.RunID || observed.ScenarioID != expected.ScenarioID ||
		observed.Directory != expected.Directory || observed.Persisted != expected.Persisted || observed.EntryCount <= 0 {
		return nil, fmt.Errorf("%w: DHS ledger observation does not match the canonical terminal fixture", constants.ErrInvalidEvidenceGraph)
	}
	return &dhsLedgerPersistenceCollection{
		CollectorID: dhsLedgerPersistenceCollectorID, CollectorVersion: dhsLedgerPersistenceCollectorVersion, Boundary: dhsLedgerPersistenceBoundary,
		InitialStateFixtureRef: expected.InitialStateFixtureRef, TerminalStateAssertions: append([]string(nil), expected.TerminalStateAssertions...),
		CollectedAt: collectedAt, Observation: observed,
	}, nil
}

func encodeDHSLedgerPersistenceCollection(collection *dhsLedgerPersistenceCollection) ([]byte, string, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode DHS ledger collection: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return encoded, "state-observation:sha256:" + hex.EncodeToString(digest[:]), nil
}

func collectDHSLedgerPersistenceEvidence(ctx context.Context, demoDir string, result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition) (string, string, error) {
	command := exec.CommandContext(ctx, "sh", constants.DemosDHSLedgerCollectorFile,
		result.GetRunId(), result.GetScenarioRef().GetId(), "operator", constants.ContainerLedgerFilesDir)
	command.Dir = demoDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", "", fmt.Errorf("%w: DHS ledger collector: %v: %s", constants.ErrInvalidEvidenceGraph, err, strings.TrimSpace(stderr.String()))
	}
	collection, err := decodeDHSLedgerPersistenceObservation(stdout.Bytes(), dhsLedgerPersistenceExpectation{
		RunID: result.GetRunId(), ScenarioID: result.GetScenarioRef().GetId(), Container: "operator",
		Directory: constants.ContainerLedgerFilesDir, Persisted: true,
		InitialStateFixtureRef:  definition.GetInitialStateFixtureRef(),
		TerminalStateAssertions: definition.GetTerminalStateAssertions(), NotBefore: result.GetStartedAt().AsTime().Truncate(time.Second),
	}, time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	encoded, evidenceRef, err := encodeDHSLedgerPersistenceCollection(collection)
	if err != nil {
		return "", "", err
	}
	return string(encoded), evidenceRef, nil
}

// ── DHS local audit vault persistence collector ─────────────────────────────

const (
	dhsAuditVaultPersistenceCollectorID      = "dhs-local-audit-vault-persistence"
	dhsAuditVaultPersistenceCollectorVersion = "1.0.0"
	dhsAuditVaultPersistenceBoundary         = "dhs-operator-local-audit-vault"
)

type dhsAuditVaultPersistenceExpectation struct {
	RunID                   string
	ScenarioID              string
	Container               string
	DatabasePath            string
	Persisted               bool
	InitialStateFixtureRef  string
	TerminalStateAssertions []string
	NotBefore               time.Time
}

type dhsAuditVaultPersistenceObservation struct {
	Persisted    bool   `json:"persisted"`
	DatabasePath string `json:"database_path"`
	SizeBytes    int64  `json:"size_bytes"`
	ObservedAt   string `json:"observed_at"`
	RunID        string `json:"run_id"`
	ScenarioID   string `json:"scenario_id"`
}

type dhsAuditVaultPersistenceCollection struct {
	CollectorID             string                              `json:"collector_id"`
	CollectorVersion        string                              `json:"collector_version"`
	Boundary                string                              `json:"boundary"`
	InitialStateFixtureRef  string                              `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string                            `json:"terminal_state_assertions"`
	CollectedAt             time.Time                           `json:"collected_at"`
	Observation             dhsAuditVaultPersistenceObservation `json:"observation"`
}

type dhsAuditVaultPersistenceObservationWire struct {
	Persisted    *bool  `json:"persisted"`
	DatabasePath string `json:"database_path"`
	SizeBytes    *int64 `json:"size_bytes"`
	ObservedAt   string `json:"observed_at"`
	RunID        string `json:"run_id"`
	ScenarioID   string `json:"scenario_id"`
}

func decodeDHSAuditVaultPersistenceObservation(raw []byte, expected dhsAuditVaultPersistenceExpectation, collectedAt time.Time) (*dhsAuditVaultPersistenceCollection, error) {
	if expected.InitialStateFixtureRef == "" || len(expected.TerminalStateAssertions) == 0 {
		return nil, fmt.Errorf("%w: DHS audit vault collector lacks canonical fixture binding", constants.ErrInvalidEvidenceGraph)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire dhsAuditVaultPersistenceObservationWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: decode DHS audit vault observation: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: DHS audit vault observation contains trailing JSON", constants.ErrInvalidEvidenceGraph)
	}
	if wire.Persisted == nil {
		return nil, fmt.Errorf("%w: DHS audit vault observation omits persistence", constants.ErrInvalidEvidenceGraph)
	}
	if wire.SizeBytes == nil {
		return nil, fmt.Errorf("%w: DHS audit vault observation omits size bytes", constants.ErrInvalidEvidenceGraph)
	}
	observed := dhsAuditVaultPersistenceObservation{
		Persisted: *wire.Persisted, DatabasePath: wire.DatabasePath, SizeBytes: *wire.SizeBytes,
		ObservedAt: wire.ObservedAt, RunID: wire.RunID, ScenarioID: wire.ScenarioID,
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observed.ObservedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: DHS audit vault observation observed_at: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if observedAt.Before(expected.NotBefore) || observedAt.After(collectedAt) {
		return nil, fmt.Errorf("%w: DHS audit vault observation timestamp is outside the scenario collection window", constants.ErrInvalidEvidenceGraph)
	}
	if observed.RunID != expected.RunID || observed.ScenarioID != expected.ScenarioID ||
		observed.DatabasePath != expected.DatabasePath || observed.Persisted != expected.Persisted || observed.SizeBytes <= 0 {
		return nil, fmt.Errorf("%w: DHS audit vault observation does not match the canonical terminal fixture", constants.ErrInvalidEvidenceGraph)
	}
	return &dhsAuditVaultPersistenceCollection{
		CollectorID: dhsAuditVaultPersistenceCollectorID, CollectorVersion: dhsAuditVaultPersistenceCollectorVersion, Boundary: dhsAuditVaultPersistenceBoundary,
		InitialStateFixtureRef: expected.InitialStateFixtureRef, TerminalStateAssertions: append([]string(nil), expected.TerminalStateAssertions...),
		CollectedAt: collectedAt, Observation: observed,
	}, nil
}

func encodeDHSAuditVaultPersistenceCollection(collection *dhsAuditVaultPersistenceCollection) ([]byte, string, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode DHS audit vault collection: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return encoded, "state-observation:sha256:" + hex.EncodeToString(digest[:]), nil
}

func collectDHSAuditVaultPersistenceEvidence(ctx context.Context, demoDir string, result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition) (string, string, error) {
	command := exec.CommandContext(ctx, "sh", constants.DemosDHSAuditVaultCollectorFile,
		result.GetRunId(), result.GetScenarioRef().GetId(), "operator", constants.ContainerAuditVaultDB)
	command.Dir = demoDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", "", fmt.Errorf("%w: DHS audit vault collector: %v: %s", constants.ErrInvalidEvidenceGraph, err, strings.TrimSpace(stderr.String()))
	}
	collection, err := decodeDHSAuditVaultPersistenceObservation(stdout.Bytes(), dhsAuditVaultPersistenceExpectation{
		RunID: result.GetRunId(), ScenarioID: result.GetScenarioRef().GetId(), Container: "operator",
		DatabasePath: constants.ContainerAuditVaultDB, Persisted: true,
		InitialStateFixtureRef:  definition.GetInitialStateFixtureRef(),
		TerminalStateAssertions: definition.GetTerminalStateAssertions(), NotBefore: result.GetStartedAt().AsTime().Truncate(time.Second),
	}, time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	encoded, evidenceRef, err := encodeDHSAuditVaultPersistenceCollection(collection)
	if err != nil {
		return "", "", err
	}
	return string(encoded), evidenceRef, nil
}

func runDHSScenario(ctx context.Context, demoDir, scenario string) ([]*compliancev1.DemoScenarioResult, error) {
	hcfg := defaultDHSHarnessConfig()
	var result *compliancev1.DemoScenarioResult
	var results []*compliancev1.DemoScenarioResult
	var hasErrors bool

	switch scenario {
	case "1":
		definition, err := loadDemoScenarioDefinition("dhs-ingest")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newDHSSovereignIngestScenarioResult(startedAt, definition)
		hcfg = bindHarnessConfig(hcfg, result)

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 1 — Sovereign Multi-Source Ingest (LOE 1)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: A coalition source connector submits a real")
		demoPrintln("          GovernanceEnvelope wrapping a run_shell_command that")
		demoPrintln("          drives the Sovereign Data Service (L5 actuator). L1")
		demoPrintln("          doctrine admits the envelope; L2 consensus quorum is")
		demoPrintln("          met and verified. The ingest is executed and a signed")
		demoPrintln("          receipt is written to the hash-chained ledger —")
		demoPrintln("          provable chain-of-custody.")
		demoPrintln()

		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-ingest", "doctrine check")
		demoEmitter.Ledger(tui.LevelInfo, "Scenario 1 started: Sovereign Multi-Source Ingest")

		step1Started := time.Now().UTC()
		step1OK := demoScenarioStep(ctx, demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"})
		step1Completed := time.Now().UTC()
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-ingest-step-1", "gateway health check", step1Started, step1Completed,
			step1OK, true, "curl gateway health endpoint"))
		if !step1OK {
			hasErrors = true
		}

		step2Started := time.Now().UTC()
		step2OK := demoScenarioStep(ctx, demoDir, "Step 2: Verify operator enrollment (mTLS certs)",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"test", "-f", constants.ContainerOperatorCert})
		step2Completed := time.Now().UTC()
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-ingest-step-2", "operator enrollment check", step2Started, step2Completed,
			step2OK, true, "docker compose exec operator test client certificate"))
		if !step2OK {
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Run real dhs-ingest via agent ────────────────")
		demoPrintln("  L1 doctrine admits; L2 consensus quorum met and verified → L5 actuator records INGEST:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-ingest", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-ingest", "consensus quorum")
		demoEmitter.Ledger(tui.LevelInfo, "L1 doctrine admitted envelope for dhs-ingest")
		hcfg.JSON = true
		step3Started := time.Now().UTC()
		harnessResults, harnessErr := runHarnessWithJSON(ctx, demoDir, "dhs-ingest via agent",
			harnessRun("dhs-ingest", hcfg))
		step3Completed := time.Now().UTC()
		step3OK := harnessErr == nil
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-ingest-step-3", "dhs-ingest harness", step3Started, step3Completed,
			step3OK, true, "agent harness dhs-ingest"))
		if !step3OK {
			fmt.Println("  (dhs-ingest harness scenario failed)")
			fmt.Println()
			hasErrors = true
		} else if len(harnessResults) == 0 || !applyHarnessAuthoritativeIdentity(result, &harnessResults[0]) {
			fmt.Println("  (dhs-ingest harness emitted no authoritative receipt)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-ingest", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-ingest", "actuator executing")
		demoEmitter.Ledger(tui.LevelInfo, "L2 consensus quorum met and verified (3/5)")

		step4Started := time.Now().UTC()
		protocolResult, evidenceRef, step4Err := collectDHSDataServiceEvidence(ctx, demoDir, result, definition, "INGEST", "TRK-CBP-0001", "NIPR")
		step4Result := buildDemoStepResult(
			"dhs-ingest-step-4", "independent state observation: ingest recorded", step4Started, time.Now().UTC(),
			step4Err == nil, true, protocolResult)
		if step4Err != nil {
			hasErrors = true
		} else {
			step4Result.EvidenceRefs = append(step4Result.EvidenceRefs, evidenceRef)
			result.StateObservationRefs = append(result.StateObservationRefs, evidenceRef)
		}
		result.StepResults = append(result.StepResults, step4Result)

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-ingest", "INGEST recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded INGEST — signed receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 1 FAILED — one or more steps failed")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 1 — Sovereign ingest governed end to end.")
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met and verified.")
			fmt.Println("         L5 actuator recorded the ingest; signed receipt in hash-chained ledger.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 1 PASSED — Sovereign ingest governed end to end")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate dhs-ingest scenario result: %w", err)
		}

	case "2":
		definition, err := loadDemoScenarioDefinition("dhs-disconnected-operations")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newDHSDisconnectedOperationsScenarioResult(startedAt, definition)
		hcfg = bindHarnessConfig(hcfg, result)

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 2 — Resilient Disconnected Operations (LOE 2)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: The Mission Partner datalink is severed, simulating a")
		demoPrintln("          contested, comms-denied corridor. The gateway and operator")
		demoPrintln("          keep governing ingest locally and commit every decision to")
		demoPrintln("          the Git-backed ledger and SQLite audit vault — no cloud, no")
		demoPrintln("          loss of sovereign control. State reconciles when the link")
		demoPrintln("          is restored.")
		demoPrintln()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 2 started: Resilient Disconnected Operations")

		step1Started := time.Now().UTC()
		step1OK := demoScenarioStep(ctx, demoDir, "Step 1: Confirm gateway is live before disconnect",
			[]string{"curl", "-s", "http://localhost:8087/api/v1/health"})
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-1", "gateway health check before disconnect", step1Started, time.Now().UTC(),
			step1OK, true, "curl gateway health endpoint"))
		if !step1OK {
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Sever the Mission Partner datalink ───────────────────")
		demoEmitter.Ledger(tui.LevelWarn, "Mission Partner datalink severed — entering comms-denied mode")
		step2Started := time.Now().UTC()
		step2Err := demoStep(ctx, demoDir, "sever datalink", false,
			"docker", "network", "disconnect",
			constants.DemosDHSPerimeterNetwork, constants.DemosDHSCoalitionDatalinkCtnr,
		)
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-2", "sever mission partner datalink", step2Started, time.Now().UTC(),
			step2Err == nil, true, "docker network disconnect mission partner datalink"))
		if step2Err != nil {
			fmt.Printf("  (sever datalink failed: %v)\n\n", step2Err)
			hasErrors = true
		}

		step3Started := time.Now().UTC()
		protocolResult, evidenceRef, step3Err := collectDHSNetworkEvidence(ctx, demoDir, result, definition, false)
		step3Result := buildDemoStepResult(
			"dhs-disconnected-step-3", "independent state observation: datalink detached", step3Started, time.Now().UTC(),
			step3Err == nil, true, protocolResult)
		if step3Err != nil {
			hasErrors = true
		} else {
			step3Result.EvidenceRefs = append(step3Result.EvidenceRefs, evidenceRef)
			result.StateObservationRefs = append(result.StateObservationRefs, evidenceRef)
		}
		result.StepResults = append(result.StepResults, step3Result)

		step4Started := time.Now().UTC()
		step4Protocol, step4Ref, step4Err := collectDHSGatewayHealthEvidence(ctx, demoDir, result, definition)
		step4Result := buildDemoStepResult(
			"dhs-disconnected-step-4", "independent state observation: local gateway available", step4Started, time.Now().UTC(),
			step4Err == nil, true, step4Protocol)
		if step4Err != nil {
			hasErrors = true
		} else {
			step4Result.EvidenceRefs = append(step4Result.EvidenceRefs, step4Ref)
			result.StateObservationRefs = append(result.StateObservationRefs, step4Ref)
		}
		result.StepResults = append(result.StepResults, step4Result)

		demoPrintln("  ── Step 5: Govern an ingest while disconnected ──────────────────")
		demoPrintln("  Running dhs-ingest through the gateway (consensus) with the datalink severed:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-ingest-disco", "doctrine check (local)")
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-ingest-disco", "doctrine admitted (local)")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-ingest-disco", "local consensus")
		hcfg.JSON = true
		step5Started := time.Now().UTC()
		harnessResults, step5Err := runHarnessWithJSON(ctx, demoDir, "dhs-ingest while disconnected",
			harnessRun("dhs-ingest", hcfg))
		step5OK := step5Err == nil
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-5", "dhs-ingest harness while disconnected", step5Started, time.Now().UTC(),
			step5OK, true, "agent harness dhs-ingest while datalink detached"))
		if !step5OK {
			fmt.Println("  (ingest while disconnected failed — operator may not be processing locally)")
			fmt.Println()
			hasErrors = true
		} else if len(harnessResults) == 0 || !applyHarnessAuthoritativeIdentity(result, &harnessResults[0]) {
			fmt.Println("  (dhs-ingest while disconnected harness emitted no authoritative receipt)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-ingest-disco", "local quorum met")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-ingest-disco", "local INGEST recorded")
		demoEmitter.Ledger(tui.LevelInfo, "Governance continued locally while disconnected — Git ledger + SQLite vault persisted")

		step6Started := time.Now().UTC()
		step6Protocol, step6Ref, step6Err := collectDHSLedgerPersistenceEvidence(ctx, demoDir, result, definition)
		step6Result := buildDemoStepResult(
			"dhs-disconnected-step-6", "independent state observation: local ledger persisted", step6Started, time.Now().UTC(),
			step6Err == nil, true, step6Protocol)
		if step6Err != nil {
			hasErrors = true
		} else {
			step6Result.EvidenceRefs = append(step6Result.EvidenceRefs, step6Ref)
			result.StateObservationRefs = append(result.StateObservationRefs, step6Ref)
		}
		result.StepResults = append(result.StepResults, step6Result)

		step7Started := time.Now().UTC()
		step7Protocol, step7Ref, step7Err := collectDHSAuditVaultPersistenceEvidence(ctx, demoDir, result, definition)
		step7Result := buildDemoStepResult(
			"dhs-disconnected-step-7", "independent state observation: local audit vault persisted", step7Started, time.Now().UTC(),
			step7Err == nil, true, step7Protocol)
		if step7Err != nil {
			hasErrors = true
		} else {
			step7Result.EvidenceRefs = append(step7Result.EvidenceRefs, step7Ref)
			result.StateObservationRefs = append(result.StateObservationRefs, step7Ref)
		}
		result.StepResults = append(result.StepResults, step7Result)

		demoPrintln("  ── Step 8: Restore the Mission Partner datalink ─────────────────")
		step8Started := time.Now().UTC()
		step8Err := demoStep(ctx, demoDir, "restore datalink", false,
			"docker", "network", "connect",
			constants.DemosDHSPerimeterNetwork, constants.DemosDHSCoalitionDatalinkCtnr,
		)
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-8", "restore mission partner datalink", step8Started, time.Now().UTC(),
			step8Err == nil, false, "docker network connect mission partner datalink"))
		restorationFailed := step8Err != nil
		if step8Err != nil {
			fmt.Printf("  (warning: restore datalink failed: %v)\n\n", step8Err)
		} else {
			demoEmitter.Ledger(tui.LevelInfo, "Mission Partner datalink restored")
		}

		step9Started := time.Now().UTC()
		if restorationFailed {
			result.StepResults = append(result.StepResults, &compliancev1.DemoStepResult{
				StepId: "dhs-disconnected-step-9", Operation: "independent state observation: datalink restored",
				StartedAt: timestamppb.New(step9Started), CompletedAt: timestamppb.New(time.Now().UTC()),
				Status: demoStatusSkipped, ProtocolResult: "datalink restoration unavailable", Failure: "restore datalink step failed", Required: false,
			})
		} else {
			protocolResult, evidenceRef, step9Err := collectDHSNetworkEvidence(ctx, demoDir, result, definition, true)
			step9Result := buildDemoStepResult(
				"dhs-disconnected-step-9", "independent state observation: datalink restored", step9Started, time.Now().UTC(),
				step9Err == nil, false, protocolResult)
			restorationFailed = step9Err != nil
			if step9Err == nil {
				step9Result.EvidenceRefs = append(step9Result.EvidenceRefs, evidenceRef)
				result.StateObservationRefs = append(result.StateObservationRefs, evidenceRef)
			}
			result.StepResults = append(result.StepResults, step9Result)
		}

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 2 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 2 FAILED — one or more steps failed")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 2 — Continuity of coverage verified under comms denial.")
			fmt.Println("         Governance continued locally; Git ledger + SQLite vault persisted all decisions.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 2 PASSED — Continuity of coverage verified under comms denial")
		}

		if restorationFailed {
			result.MetricsSummary += " // datalink restoration FAILED (reported separately)"
			fmt.Println("  [RESTORATION FAILURE] Datalink could not be restored — continuity claim holds,")
			fmt.Println("    but the environment requires manual reconnection before subsequent scenarios.")
			demoEmitter.Ledger(tui.LevelWarn, "Datalink restoration failed — continuity verified but manual reconnection required")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate dhs-disconnected-operations scenario result: %w", err)
		}

	case "3":
		definition, err := loadDemoScenarioDefinition("dhs-cue")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newDHSCueScenarioResult(startedAt, definition)
		hcfg = bindHarnessConfig(hcfg, result)

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 3 — Governed Predictive Cueing (LOE 3 & 4)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: An authorized interdiction cue with L2 ensemble quorum")
		demoPrintln("          is admitted and executed by the L5 actuator (dhs-cue).")
		demoPrintln("          This demonstrates that L2 BFT consensus is a real")
		demoPrintln("          fail-closed gate, not just an audit annotation.")
		demoPrintln()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 started: Governed Predictive Cueing")

		step1Started := time.Now().UTC()
		step1OK := demoScenarioStep(ctx, demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"})
		step1Completed := time.Now().UTC()
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-cue-step-1", "gateway health check", step1Started, step1Completed,
			step1OK, true, "curl gateway health endpoint"))
		if !step1OK {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-cue", "doctrine check")
		demoPrintln("  ── Step 2: Run dhs-cue via agent (L2 quorum → admit) ────")
		demoPrintln("  L2 consensus quorum met (decision=true) → admitted → L5 actuator records CUE:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-cue", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-cue", "consensus deliberation")
		demoEmitter.Consensus(constants.ConsensusMemberAxiom, true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus(constants.ConsensusMemberConcord, true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus(constants.ConsensusMemberVariance, true, true, 3, 5, tui.ConsensusPending, "")
		hcfg.JSON = true
		step2Started := time.Now().UTC()
		harnessResults, harnessErr := runHarnessWithJSON(ctx, demoDir, "dhs-cue via agent",
			harnessRun("dhs-cue", hcfg))
		step2Completed := time.Now().UTC()
		step2OK := harnessErr == nil
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-cue-step-2", "dhs-cue harness", step2Started, step2Completed,
			step2OK, true, "agent harness dhs-cue"))
		if !step2OK {
			fmt.Println("  (dhs-cue harness scenario failed)")
			fmt.Println()
			hasErrors = true
		} else if len(harnessResults) == 0 || !applyHarnessAuthoritativeIdentity(result, &harnessResults[0]) {
			fmt.Println("  (dhs-cue harness emitted no authoritative receipt)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-cue", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-cue", "actuator executing")
		demoEmitter.Consensus(constants.ConsensusMemberAxiom, true, true, 3, 5, tui.ConsensusReached, "cue-hash-001")
		demoEmitter.Ledger(tui.LevelInfo, "L2 consensus quorum met (3/5) — cue admitted")

		step3Started := time.Now().UTC()
		protocolResult, evidenceRef, step3Err := collectDHSDataServiceEvidence(ctx, demoDir, result, definition, "CUE", "TRK-CBP-0001", "TASKING-DHS-2026-077")
		step3Result := buildDemoStepResult(
			"dhs-cue-step-3", "independent state observation: cue recorded", step3Started, time.Now().UTC(),
			step3Err == nil, true, protocolResult)
		if step3Err != nil {
			hasErrors = true
		} else {
			step3Result.EvidenceRefs = append(step3Result.EvidenceRefs, evidenceRef)
			result.StateObservationRefs = append(result.StateObservationRefs, evidenceRef)
		}
		result.StepResults = append(result.StepResults, step3Result)

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-cue", "CUE recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded CUE — signed receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 3 FAILED — one or more steps failed")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 3 — Predictive cueing governed by L2 consensus.")
			fmt.Println("         Authorized cue admitted with quorum.")
			fmt.Println("         CUE operation recorded by the L5 actuator.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 PASSED — Predictive cueing governed by L2 consensus")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate dhs-cue scenario result: %w", err)
		}

	case "4":
		definitionIDs, err := dhsScenarioDefinitionIDs(scenario)
		if err != nil {
			return nil, err
		}
		blockDefinition, err := loadDemoScenarioDefinition(definitionIDs[0])
		if err != nil {
			return nil, err
		}
		purgeDefinition, err := loadDemoScenarioDefinition(definitionIDs[1])
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		results = newDHSDestructionScenarioResults(startedAt, blockDefinition, purgeDefinition)
		blockResult, purgeResult := results[0], results[1]
		hcfg = bindHarnessConfig(hcfg, blockResult)
		var blockHasErrors, purgeHasErrors bool

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 4 — Sovereign Destruction + Tamper-Proof Audit (LOE 2)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: A compromised connector tries to wipe the audit trail")
		demoPrintln("          with 'rm -rf /var/log/g8e' — L1 doctrine rejects it at")
		demoPrintln("          admission (the data-destruction threat detector fires).")
		demoPrintln("          Then a governed retention purge is admitted by L1 doctrine")
		demoPrintln("          with L2 consensus quorum met, and the L5 actuator records")
		demoPrintln("          the PURGE with a cryptographic destruction receipt.")
		demoPrintln()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 started: Sovereign Destruction + Tamper-Proof Audit")

		demoPrintln("  ── Step 1: Run dhs-evidence-block via agent (L1 reject) ──")
		demoPrintln("  L1 doctrine detects 'rm -rf /var/log/g8e' → rejected at admission:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-evidence-block", "doctrine check")
		hcfg.JSON = true
		step1Started := time.Now().UTC()
		harnessResults, step1Err := runHarnessWithJSON(ctx, demoDir, "dhs-evidence-block via agent",
			harnessRun("dhs-evidence-block", hcfg))
		// The harness exits successfully only after it verifies that doctrine blocked
		// the action. A nonzero exit means the expected rejection was not verified.
		step1OK := step1Err == nil
		blockResult.StepResults = append(blockResult.StepResults, buildDemoStepResult(
			"dhs-destruction-block-step-1", "dhs-evidence-block harness (L1 doctrine reject)", step1Started, time.Now().UTC(),
			step1OK, true, "agent harness dhs-evidence-block"))
		if !step1OK {
			fmt.Println("  (dhs-evidence-block harness scenario failed)")
			fmt.Println()
			blockHasErrors = true
		} else {
			if len(harnessResults) == 0 || !applyHarnessAuthoritativeIdentity(blockResult, &harnessResults[0]) {
				blockHasErrors = true
			}
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusFailed, "dhs-evidence-block", "DATA DESTRUCTION ATTEMPT BLOCKED")
		demoEmitter.Ledger(tui.LevelCritical, "L1 doctrine BLOCKED: 'rm -rf /var/log/g8e' — data-destruction threat detected at admission")

		step2Started := time.Now().UTC()
		step2Protocol, step2Ref, step2Err := collectDHSAuditVaultPersistenceEvidence(ctx, demoDir, blockResult, blockDefinition)
		step2Result := buildDemoStepResult(
			"dhs-destruction-block-step-2", "independent state observation: audit vault present and non-empty", step2Started, time.Now().UTC(),
			step2Err == nil, true, step2Protocol)
		if step2Err != nil {
			blockHasErrors = true
		} else {
			step2Result.EvidenceRefs = append(step2Result.EvidenceRefs, step2Ref)
			blockResult.StateObservationRefs = append(blockResult.StateObservationRefs, step2Ref)
		}
		blockResult.StepResults = append(blockResult.StepResults, step2Result)
		blockResult.CompletedAt = timestamppb.New(time.Now().UTC())
		if blockHasErrors {
			blockResult.Status = demoStatusFailed
			blockResult.VerificationStatus = "unverifiable"
			blockResult.Failure = "one or more required steps failed"
		} else {
			blockResult.VerificationStatus = "verified"
		}

		demoPrintln("  ── Step 3: Run dhs-purge via agent (admit) ──────────────")
		demoPrintln("  L1 doctrine admits; L2 consensus quorum met → L5 actuator records PURGE:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-purge", "doctrine check")
		hcfg = bindHarnessConfig(hcfg, purgeResult)
		hcfg.JSON = true
		step3Started := time.Now().UTC()
		purgeHarnessResults, step3Err := runHarnessWithJSON(ctx, demoDir, "dhs-purge via agent",
			harnessRun("dhs-purge", hcfg))
		step3OK := step3Err == nil
		purgeResult.StepResults = append(purgeResult.StepResults, buildDemoStepResult(
			"dhs-destruction-purge-step-1", "dhs-purge harness", step3Started, time.Now().UTC(),
			step3OK, true, "agent harness dhs-purge"))
		if !step3OK {
			fmt.Println("  (dhs-purge harness scenario failed)")
			fmt.Println()
			purgeHasErrors = true
		} else if len(purgeHarnessResults) == 0 || !applyHarnessAuthoritativeIdentity(purgeResult, &purgeHarnessResults[0]) {
			fmt.Println("  (dhs-purge harness emitted no authoritative receipt)")
			fmt.Println()
			purgeHasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-purge", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-purge", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-purge", "actuator executing")
		demoEmitter.Ledger(tui.LevelInfo, "L1+L2 admitted governed purge — L5 actuator recording PURGE with destruction receipt")

		step4Started := time.Now().UTC()
		protocolResult, evidenceRef, step4Err := collectDHSDataServiceEvidence(ctx, demoDir, purgeResult, purgeDefinition, "PURGE", "VPR-0001", "RETENTION-30D")
		step4Result := buildDemoStepResult(
			"dhs-destruction-purge-step-2", "independent state observation: purge recorded", step4Started, time.Now().UTC(),
			step4Err == nil, true, protocolResult)
		if step4Err != nil {
			purgeHasErrors = true
		} else {
			step4Result.EvidenceRefs = append(step4Result.EvidenceRefs, evidenceRef)
			purgeResult.StateObservationRefs = append(purgeResult.StateObservationRefs, evidenceRef)
		}
		purgeResult.StepResults = append(purgeResult.StepResults, step4Result)
		purgeResult.CompletedAt = timestamppb.New(time.Now().UTC())
		if purgeHasErrors {
			purgeResult.Status = demoStatusFailed
			purgeResult.VerificationStatus = "unverifiable"
			purgeResult.Failure = "one or more required steps failed"
		} else {
			purgeResult.VerificationStatus = "verified"
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-purge", "PURGE recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded PURGE — cryptographic destruction receipt in hash-chained ledger")
		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if blockHasErrors || purgeHasErrors {
			fmt.Println("  [FAIL] Scenario 4 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 4 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 4 — Destruction governed and provable.")
			fmt.Println("         L1 blocked the audit-wipe; L1+L2 admitted governed purge with receipt.")
			fmt.Println("         Independent verification confirms the audit vault is intact after the blocked attempt.")
			fmt.Println("         PURGE operation recorded by the L5 actuator.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 PASSED — Destruction governed and provable")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(blockResult, blockDefinition, blockResult.ScopeId); err != nil {
			return nil, fmt.Errorf("validate dhs-destruction-block scenario result: %w", err)
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(purgeResult, purgeDefinition, purgeResult.ScopeId); err != nil {
			return nil, fmt.Errorf("validate dhs-destruction-purge scenario result: %w", err)
		}

	default:
		_, err := dhsScenarioDefinitionIDs(scenario)
		return nil, err
	}
	if len(results) > 0 {
		return results, nil
	}
	return []*compliancev1.DemoScenarioResult{result}, nil
}
