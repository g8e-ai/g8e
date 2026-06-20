// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

// Package scenarios is the heart of Agentic Tool Emulator: an ordered, flexible registry of
// impersonations. Each scenario wears a persona (some real-world AI tool) and
// exercises one slice of the protocol surface against a REAL Gateway+Operator.
package scenarios

import (
	"context"
	"fmt"
	"os"
	"time"

	clientpkg "github.com/g8e-ai/g8e/test/agentic_tool_emulator/client"
)

// Posture is the Gateway enforcement mode a scenario needs.
//
//	doctrine  - L1 enforced, L2/L3 audited           (--doctrine)
//	consensus - L1/L2 enforced, L3 audited            (--consensus)
//	notary    - L1/L2/L3 strictly enforced            (--notary)
type Posture string

const (
	Doctrine  Posture = "doctrine"
	Consensus Posture = "consensus"
	Notary    Posture = "notary"
)

// Result is the detailed, auditable record of one scenario run.
type Result struct {
	Name            string               `json:"name"`
	Title           string               `json:"title"`
	Persona         string               `json:"persona"`
	RequiresPosture Posture              `json:"requires_posture"`
	StartedAt       time.Time            `json:"started_at"`
	DurationMS      int64                `json:"duration_ms"`
	Exchanges       []clientpkg.Exchange `json:"exchanges"`
	TxHashes        []string             `json:"tx_hashes,omitempty"`
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
