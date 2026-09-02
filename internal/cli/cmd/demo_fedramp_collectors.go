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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

const (
	fedRAMPCloudOperationCollectorID      = "fedramp-cloud-operation"
	fedRAMPCloudOperationCollectorVersion = "1.0.0"
	fedRAMPCloudLogCollectorID            = "fedramp-cloud-operations-log"
	fedRAMPCloudLogCollectorVersion       = "1.0.0"
	fedRAMPCloudServiceBoundary           = "fedramp-sovereign-cloud-service"
	fedRAMPAuditVaultCollectorID          = "fedramp-gateway-audit-vault"
	fedRAMPAuditVaultCollectorVersion     = "1.0.0"
	fedRAMPGatewayAuditVaultBoundary      = "fedramp-gateway-audit-vault"
)

type fedRAMPCloudOperationExpectation struct {
	RunID                   string
	ScenarioID              string
	Action                  string
	ResourceID              string
	Detail                  string
	OperationFound          bool
	InitialStateFixtureRef  string
	TerminalStateAssertions []string
	NotBefore               time.Time
}

type fedRAMPCloudOperationObservation struct {
	Action             string `json:"action"`
	Detail             string `json:"detail"`
	ObservedAt         string `json:"observed_at"`
	OperationFound     bool   `json:"operation_found"`
	OperationTimestamp string `json:"operation_timestamp"`
	ResourceID         string `json:"resource_id"`
	RunID              string `json:"run_id"`
	ScenarioID         string `json:"scenario_id"`
}

type fedRAMPCloudOperationObservationWire struct {
	Action             string `json:"action"`
	Detail             string `json:"detail"`
	ObservedAt         string `json:"observed_at"`
	OperationFound     *bool  `json:"operation_found"`
	OperationTimestamp string `json:"operation_timestamp"`
	ResourceID         string `json:"resource_id"`
	RunID              string `json:"run_id"`
	ScenarioID         string `json:"scenario_id"`
}

type fedRAMPCloudOperationCollection struct {
	CollectorID             string                           `json:"collector_id"`
	CollectorVersion        string                           `json:"collector_version"`
	Boundary                string                           `json:"boundary"`
	InitialStateFixtureRef  string                           `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string                         `json:"terminal_state_assertions"`
	CollectedAt             time.Time                        `json:"collected_at"`
	Observation             fedRAMPCloudOperationObservation `json:"observation"`
}

func decodeFedRAMPCloudOperationObservation(raw []byte, expected fedRAMPCloudOperationExpectation, collectedAt time.Time) (*fedRAMPCloudOperationCollection, error) {
	if expected.InitialStateFixtureRef == "" || len(expected.TerminalStateAssertions) == 0 {
		return nil, fmt.Errorf("%w: FedRAMP cloud operation collector lacks canonical fixture binding", constants.ErrInvalidEvidenceGraph)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire fedRAMPCloudOperationObservationWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: decode FedRAMP cloud operation observation: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: FedRAMP cloud operation observation contains trailing JSON", constants.ErrInvalidEvidenceGraph)
	}
	if wire.OperationFound == nil {
		return nil, fmt.Errorf("%w: FedRAMP cloud operation observation omits operation presence", constants.ErrInvalidEvidenceGraph)
	}
	observed := fedRAMPCloudOperationObservation{
		Action: wire.Action, Detail: wire.Detail, ObservedAt: wire.ObservedAt, OperationFound: *wire.OperationFound,
		OperationTimestamp: wire.OperationTimestamp, ResourceID: wire.ResourceID, RunID: wire.RunID, ScenarioID: wire.ScenarioID,
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observed.ObservedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: FedRAMP cloud operation observed_at: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	operationAt, err := time.Parse(time.RFC3339Nano, observed.OperationTimestamp)
	if err != nil {
		return nil, fmt.Errorf("%w: FedRAMP cloud operation operation_timestamp: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if observedAt.Before(expected.NotBefore) || observedAt.After(collectedAt) || operationAt.Before(expected.NotBefore) || operationAt.After(observedAt) {
		return nil, fmt.Errorf("%w: FedRAMP cloud operation timestamp is outside the scenario collection window", constants.ErrInvalidEvidenceGraph)
	}
	if observed.RunID != expected.RunID || observed.ScenarioID != expected.ScenarioID || observed.Action != expected.Action ||
		observed.ResourceID != expected.ResourceID || observed.Detail != expected.Detail || observed.OperationFound != expected.OperationFound {
		return nil, fmt.Errorf("%w: FedRAMP cloud operation does not match the canonical terminal fixture", constants.ErrInvalidEvidenceGraph)
	}
	return &fedRAMPCloudOperationCollection{
		CollectorID: fedRAMPCloudOperationCollectorID, CollectorVersion: fedRAMPCloudOperationCollectorVersion, Boundary: fedRAMPCloudServiceBoundary,
		InitialStateFixtureRef: expected.InitialStateFixtureRef, TerminalStateAssertions: append([]string(nil), expected.TerminalStateAssertions...),
		CollectedAt: collectedAt, Observation: observed,
	}, nil
}

func encodeFedRAMPCloudOperationCollection(collection *fedRAMPCloudOperationCollection) ([]byte, string, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode FedRAMP cloud operation collection: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return encoded, "state-observation:sha256:" + hex.EncodeToString(digest[:]), nil
}

type fedRAMPCloudLogExpectation struct {
	RunID                   string
	ScenarioID              string
	LogPath                 string
	Persisted               bool
	InitialStateFixtureRef  string
	TerminalStateAssertions []string
	NotBefore               time.Time
}

type fedRAMPCloudLogObservation struct {
	EntryCount int    `json:"entry_count"`
	LogPath    string `json:"log_path"`
	ObservedAt string `json:"observed_at"`
	Persisted  bool   `json:"persisted"`
	RunID      string `json:"run_id"`
	ScenarioID string `json:"scenario_id"`
	SizeBytes  int64  `json:"size_bytes"`
}

type fedRAMPCloudLogObservationWire struct {
	EntryCount *int   `json:"entry_count"`
	LogPath    string `json:"log_path"`
	ObservedAt string `json:"observed_at"`
	Persisted  *bool  `json:"persisted"`
	RunID      string `json:"run_id"`
	ScenarioID string `json:"scenario_id"`
	SizeBytes  *int64 `json:"size_bytes"`
}

type fedRAMPCloudLogCollection struct {
	CollectorID             string                     `json:"collector_id"`
	CollectorVersion        string                     `json:"collector_version"`
	Boundary                string                     `json:"boundary"`
	InitialStateFixtureRef  string                     `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string                   `json:"terminal_state_assertions"`
	CollectedAt             time.Time                  `json:"collected_at"`
	Observation             fedRAMPCloudLogObservation `json:"observation"`
}

func decodeFedRAMPCloudLogObservation(raw []byte, expected fedRAMPCloudLogExpectation, collectedAt time.Time) (*fedRAMPCloudLogCollection, error) {
	if expected.InitialStateFixtureRef == "" || len(expected.TerminalStateAssertions) == 0 {
		return nil, fmt.Errorf("%w: FedRAMP cloud log collector lacks canonical fixture binding", constants.ErrInvalidEvidenceGraph)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire fedRAMPCloudLogObservationWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: decode FedRAMP cloud log observation: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: FedRAMP cloud log observation contains trailing JSON", constants.ErrInvalidEvidenceGraph)
	}
	if wire.Persisted == nil || wire.EntryCount == nil || wire.SizeBytes == nil {
		return nil, fmt.Errorf("%w: FedRAMP cloud log observation omits typed persistence fields", constants.ErrInvalidEvidenceGraph)
	}
	observed := fedRAMPCloudLogObservation{
		EntryCount: *wire.EntryCount, LogPath: wire.LogPath, ObservedAt: wire.ObservedAt, Persisted: *wire.Persisted,
		RunID: wire.RunID, ScenarioID: wire.ScenarioID, SizeBytes: *wire.SizeBytes,
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observed.ObservedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: FedRAMP cloud log observed_at: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if observedAt.Before(expected.NotBefore) || observedAt.After(collectedAt) {
		return nil, fmt.Errorf("%w: FedRAMP cloud log timestamp is outside the scenario collection window", constants.ErrInvalidEvidenceGraph)
	}
	if observed.RunID != expected.RunID || observed.ScenarioID != expected.ScenarioID || observed.LogPath != expected.LogPath ||
		observed.Persisted != expected.Persisted || observed.EntryCount <= 0 || observed.SizeBytes <= 0 {
		return nil, fmt.Errorf("%w: FedRAMP cloud log observation does not match the canonical terminal fixture", constants.ErrInvalidEvidenceGraph)
	}
	return &fedRAMPCloudLogCollection{
		CollectorID: fedRAMPCloudLogCollectorID, CollectorVersion: fedRAMPCloudLogCollectorVersion, Boundary: fedRAMPCloudServiceBoundary,
		InitialStateFixtureRef: expected.InitialStateFixtureRef, TerminalStateAssertions: append([]string(nil), expected.TerminalStateAssertions...),
		CollectedAt: collectedAt, Observation: observed,
	}, nil
}

func encodeFedRAMPCloudLogCollection(collection *fedRAMPCloudLogCollection) ([]byte, string, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode FedRAMP cloud log collection: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return encoded, "state-observation:sha256:" + hex.EncodeToString(digest[:]), nil
}

type fedRAMPAuditVaultExpectation struct {
	RunID                   string
	ScenarioID              string
	DatabasePath            string
	Persisted               bool
	InitialStateFixtureRef  string
	TerminalStateAssertions []string
	NotBefore               time.Time
}

type fedRAMPAuditVaultObservation struct {
	DatabasePath string `json:"database_path"`
	ObservedAt   string `json:"observed_at"`
	Persisted    bool   `json:"persisted"`
	RunID        string `json:"run_id"`
	ScenarioID   string `json:"scenario_id"`
	SizeBytes    int64  `json:"size_bytes"`
}

type fedRAMPAuditVaultObservationWire struct {
	DatabasePath string `json:"database_path"`
	ObservedAt   string `json:"observed_at"`
	Persisted    *bool  `json:"persisted"`
	RunID        string `json:"run_id"`
	ScenarioID   string `json:"scenario_id"`
	SizeBytes    *int64 `json:"size_bytes"`
}

type fedRAMPAuditVaultCollection struct {
	CollectorID             string                       `json:"collector_id"`
	CollectorVersion        string                       `json:"collector_version"`
	Boundary                string                       `json:"boundary"`
	InitialStateFixtureRef  string                       `json:"initial_state_fixture_ref"`
	TerminalStateAssertions []string                     `json:"terminal_state_assertions"`
	CollectedAt             time.Time                    `json:"collected_at"`
	Observation             fedRAMPAuditVaultObservation `json:"observation"`
}

func decodeFedRAMPAuditVaultObservation(raw []byte, expected fedRAMPAuditVaultExpectation, collectedAt time.Time) (*fedRAMPAuditVaultCollection, error) {
	if expected.InitialStateFixtureRef == "" || len(expected.TerminalStateAssertions) == 0 {
		return nil, fmt.Errorf("%w: FedRAMP audit vault collector lacks canonical fixture binding", constants.ErrInvalidEvidenceGraph)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire fedRAMPAuditVaultObservationWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("%w: decode FedRAMP audit vault observation: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: FedRAMP audit vault observation contains trailing JSON", constants.ErrInvalidEvidenceGraph)
	}
	if wire.Persisted == nil || wire.SizeBytes == nil {
		return nil, fmt.Errorf("%w: FedRAMP audit vault observation omits typed persistence fields", constants.ErrInvalidEvidenceGraph)
	}
	observed := fedRAMPAuditVaultObservation{
		DatabasePath: wire.DatabasePath, ObservedAt: wire.ObservedAt, Persisted: *wire.Persisted,
		RunID: wire.RunID, ScenarioID: wire.ScenarioID, SizeBytes: *wire.SizeBytes,
	}
	observedAt, err := time.Parse(time.RFC3339Nano, observed.ObservedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: FedRAMP audit vault observed_at: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if observedAt.Before(expected.NotBefore) || observedAt.After(collectedAt) {
		return nil, fmt.Errorf("%w: FedRAMP audit vault timestamp is outside the scenario collection window", constants.ErrInvalidEvidenceGraph)
	}
	if observed.RunID != expected.RunID || observed.ScenarioID != expected.ScenarioID || observed.DatabasePath != expected.DatabasePath ||
		observed.Persisted != expected.Persisted || observed.SizeBytes <= 0 {
		return nil, fmt.Errorf("%w: FedRAMP audit vault observation does not match the canonical terminal fixture", constants.ErrInvalidEvidenceGraph)
	}
	return &fedRAMPAuditVaultCollection{
		CollectorID: fedRAMPAuditVaultCollectorID, CollectorVersion: fedRAMPAuditVaultCollectorVersion, Boundary: fedRAMPGatewayAuditVaultBoundary,
		InitialStateFixtureRef: expected.InitialStateFixtureRef, TerminalStateAssertions: append([]string(nil), expected.TerminalStateAssertions...),
		CollectedAt: collectedAt, Observation: observed,
	}, nil
}

func encodeFedRAMPAuditVaultCollection(collection *fedRAMPAuditVaultCollection) ([]byte, string, error) {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return nil, "", fmt.Errorf("%w: encode FedRAMP audit vault collection: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	digest := sha256.Sum256(encoded)
	return encoded, "state-observation:sha256:" + hex.EncodeToString(digest[:]), nil
}

func runFedRAMPCollector(ctx context.Context, demoDir string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, arguments[0], arguments[1:]...)
	command.Dir = demoDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("%w: FedRAMP state collector: %v: %s", constants.ErrInvalidEvidenceGraph, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func collectFedRAMPCloudOperationEvidence(ctx context.Context, demoDir string, result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition, action, resourceID, detail string) (string, string, error) {
	raw, err := runFedRAMPCollector(ctx, demoDir, "docker", "compose", "exec", "-T", "cloudsvc", "python", constants.ContainerFedRAMPCloudCollectorFile,
		"operation", result.GetRunId(), result.GetScenarioRef().GetId(), constants.ContainerCloudSvcOpsLog, action, resourceID, detail)
	if err != nil {
		return "", "", err
	}
	collection, err := decodeFedRAMPCloudOperationObservation(raw, fedRAMPCloudOperationExpectation{
		RunID: result.GetRunId(), ScenarioID: result.GetScenarioRef().GetId(), Action: action, ResourceID: resourceID, Detail: detail, OperationFound: true,
		InitialStateFixtureRef: definition.GetInitialStateFixtureRef(), TerminalStateAssertions: definition.GetTerminalStateAssertions(),
		NotBefore: result.GetStartedAt().AsTime().Truncate(time.Second),
	}, time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	encoded, evidenceRef, err := encodeFedRAMPCloudOperationCollection(collection)
	return string(encoded), evidenceRef, err
}

func collectFedRAMPCloudLogEvidence(ctx context.Context, demoDir string, result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition) (string, string, error) {
	raw, err := runFedRAMPCollector(ctx, demoDir, "docker", "compose", "exec", "-T", "cloudsvc", "python", constants.ContainerFedRAMPCloudCollectorFile,
		"log", result.GetRunId(), result.GetScenarioRef().GetId(), constants.ContainerCloudSvcOpsLog)
	if err != nil {
		return "", "", err
	}
	collection, err := decodeFedRAMPCloudLogObservation(raw, fedRAMPCloudLogExpectation{
		RunID: result.GetRunId(), ScenarioID: result.GetScenarioRef().GetId(), LogPath: constants.ContainerCloudSvcOpsLog, Persisted: true,
		InitialStateFixtureRef: definition.GetInitialStateFixtureRef(), TerminalStateAssertions: definition.GetTerminalStateAssertions(),
		NotBefore: result.GetStartedAt().AsTime().Truncate(time.Second),
	}, time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	encoded, evidenceRef, err := encodeFedRAMPCloudLogCollection(collection)
	return string(encoded), evidenceRef, err
}

func collectFedRAMPAuditVaultEvidence(ctx context.Context, demoDir string, result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition) (string, string, error) {
	raw, err := runFedRAMPCollector(ctx, demoDir, "sh", constants.DemosFedRAMPAuditVaultCollectorFile,
		result.GetRunId(), result.GetScenarioRef().GetId(), "gateway", constants.ContainerAuditVaultDB)
	if err != nil {
		return "", "", err
	}
	collection, err := decodeFedRAMPAuditVaultObservation(raw, fedRAMPAuditVaultExpectation{
		RunID: result.GetRunId(), ScenarioID: result.GetScenarioRef().GetId(), DatabasePath: constants.ContainerAuditVaultDB, Persisted: true,
		InitialStateFixtureRef: definition.GetInitialStateFixtureRef(), TerminalStateAssertions: definition.GetTerminalStateAssertions(),
		NotBefore: result.GetStartedAt().AsTime().Truncate(time.Second),
	}, time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	encoded, evidenceRef, err := encodeFedRAMPAuditVaultCollection(collection)
	return string(encoded), evidenceRef, err
}
