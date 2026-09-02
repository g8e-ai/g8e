// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package scenarios is the heart of Agent Harness: an ordered, flexible registry of
// impersonations. Each scenario wears a persona (some real-world AI tool) and
// exercises one slice of the protocol surface against a REAL Gateway+Operator.
package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

// Posture is the Gateway enforcement mode a scenario needs. The harness has
// dedicated doctrine, consensus, and notary suites. Ratify is a supported
// Gateway posture but does not yet have a dedicated scenario suite.
//
//	doctrine  - L1 enforced, L2/L3 audited           (--doctrine)
//	consensus - L1/L2 enforced, L3 audited            (--consensus)
//	notary    - L1/L2/L3 strictly enforced            (--notary)
type Posture string

const (
	Doctrine  Posture = "doctrine"
	Consensus Posture = "consensus"
	Notary    Posture = "notary"

	DhsScenarioPrefix      = "dhs-"
	FedRAMPScenarioPrefix  = "fedramp-"
	EnsembleScenarioPrefix = "ensemble-"
)

// Result is the detailed, auditable record of one scenario run.
type Result struct {
	Name            string               `json:"name"`
	Title           string               `json:"title"`
	Persona         string               `json:"persona"`
	RequiresPosture Posture              `json:"requires_posture"`
	StartedAt       time.Time            `json:"started_at"`
	DurationMS      int64                `json:"duration_ms"`
	RunID           string               `json:"run_id,omitempty"`
	ScenarioID      string               `json:"scenario_id,omitempty"`
	Exchanges       []clientpkg.Exchange `json:"exchanges"`
	TxHashes        []string             `json:"tx_hashes,omitempty"`
	AttemptIDs      []string             `json:"attempt_ids,omitempty"`
	ExecutionIDs    []string             `json:"execution_ids,omitempty"`
	TransactionIDs  []string             `json:"transaction_ids,omitempty"`
	Receipts        []clientpkg.Receipt  `json:"receipts,omitempty"`
	Notes           []string             `json:"notes,omitempty"`
	OK              bool                 `json:"ok"`
	Err             string               `json:"error,omitempty"`
}

func (r *Result) note(f string, a ...any) { r.Notes = append(r.Notes, fmt.Sprintf(f, a...)) }
func (r *Result) tx(h string) {
	if h != "" {
		r.TxHashes = append(r.TxHashes, h)
	}
}

func (r *Result) retainToolReceipt(resp *clientpkg.JSONRPCResponse) error {
	if resp == nil || len(resp.Result) == 0 {
		return constants.ErrHarnessReceiptReferenceMissing
	}
	var result struct {
		Receipt *clientpkg.Receipt `json:"receipt"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("agent harness: decode tool receipt reference: %w", err)
	}
	if result.Receipt == nil {
		return constants.ErrHarnessReceiptReferenceMissing
	}
	if result.Receipt.ExecutionID == "" || result.Receipt.TransactionID == "" || result.Receipt.TransactionHash == "" || result.Receipt.SignerKeyID == "" || result.Receipt.Signature == "" {
		return constants.ErrHarnessReceiptReferenceInvalid
	}
	r.ExecutionIDs = append(r.ExecutionIDs, result.Receipt.ExecutionID)
	r.TransactionIDs = append(r.TransactionIDs, result.Receipt.TransactionID)
	r.Receipts = append(r.Receipts, *result.Receipt)
	return nil
}

// Scenario is one impersonation. Run does the work; the runner handles
// recording, timing, and error capture.
type Scenario struct {
	Name            string
	Title           string
	Persona         clientpkg.Persona
	RequiresPosture Posture
	Run             func(ctx context.Context, c *clientpkg.Client, r *Result) error
}

// Registry is the ordered demo script. The first block runs under doctrine;
// the consensus/notary block runs after you flip the Gateway's notary node.
func Registry() []Scenario {
	var s []Scenario
	s = append(s, mcpScenarios()...)
	s = append(s, a2aScenarios()...)
	s = append(s, governanceScenarios()...)
	s = append(s, ensembleScenarios()...)
	s = append(s, dhsScenarios()...)
	s = append(s, financeScenarios()...)
	s = append(s, fedrampScenarios()...)
	return s
}

// Find returns the scenario with the given name, if any.
func Find(name string) (Scenario, bool) {
	for _, sc := range Registry() {
		if sc.Name == name {
			return sc, true
		}
	}
	return Scenario{}, false
}

// Execute runs a single scenario against the client and returns its Result.
func Execute(ctx context.Context, c *clientpkg.Client, sc Scenario) Result {
	r := Result{
		Name:            sc.Name,
		Title:           sc.Title,
		Persona:         sc.Persona.ID,
		RequiresPosture: sc.RequiresPosture,
		StartedAt:       time.Now(),
		RunID:           os.Getenv(string(constants.EnvVar.DemoRunID)),
		ScenarioID:      os.Getenv(string(constants.EnvVar.DemoScenarioID)),
		AttemptIDs:      []string{uuid.NewString()},
	}
	c.Record(&r.Exchanges) // every HTTP call lands in this result
	defer c.Record(nil)

	fmt.Fprintf(os.Stderr, "▶ %-18s [%s as %s]\n", sc.Name, sc.RequiresPosture, sc.Persona.ID)
	err := sc.Run(ctx, c, &r)
	r.DurationMS = time.Since(r.StartedAt).Milliseconds()
	if err != nil {
		r.Err = err.Error()
		r.OK = false
		return r
	}
	r.OK = true
	return r
}
