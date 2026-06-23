// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"errors"

	clientpkg "github.com/g8e-ai/g8e/internal/tools/agentic_tool_emulator/client"
)

// GovKit carries the mock cryptographic actors the governance scenarios need.
// main builds it once (minting keys, registering trusted signers) and injects
// it before running the consensus/notary block.
type GovKit struct {
	Ensemble   *clientpkg.Ensemble
	Principal  *clientpkg.Principal
	L3Mode     string // "mock" | "suspend"
	OperatorID string
}

var kit *GovKit

// SetGovKit injects the governance actors. Call before Execute on a gov scenario.
func SetGovKit(k *GovKit) { kit = k }

var (
	ensembleProducer = clientpkg.Persona{ID: "ensemble-producer", UserAgent: "g8e-ensemble/1.x (maximal)"}
	principalActor   = clientpkg.Persona{ID: "principal", UserAgent: "g8e-principal/1.x (L3 notary)"}
)

func governanceScenarios() []Scenario {
	return []Scenario{
		{
			Name: "consensus", Title: "L2 consensus envelope (mock ensemble co-sign)", Persona: ensembleProducer, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil {
					return errors.New("gov kit not initialized (call SetGovKit)")
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return err
				}
				r.note("bound to state root %s", short(root))
				r.note("mock ensemble: %d agents co-signed transaction_hash|true with key %q",
					kit.Ensemble.AgentCount(), kit.Ensemble.KeyID)

				txHash, status, _, err := c.SubmitMaximal(ctx, ensembleProducer, clientpkg.MaximalEnvelope{
					OperatorID:     kit.OperatorID,
					ToolName:       "fs_list",
					ArgumentsJSON:  `{"path":"."}`,
					TargetResource: "localhost",
					StateRoot:      root,
					Ensemble:       kit.Ensemble, // L2 attached; no L3 (audited in consensus posture)
					TTL:            c.Config().EnvelopeTTL,
				})
				if err != nil {
					return err
				}
				r.tx(txHash)
				r.note("submitted official GovernanceEnvelope %s (admission status %d)", short(txHash), status)
				return nil
			},
		},
		{
			Name: "envelope-maximal", Title: "Official notary envelope: L2 consensus + principal L3 signing", Persona: ensembleProducer, RequiresPosture: Notary,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return errors.New("gov kit not initialized (need ensemble + principal)")
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return err
				}
				r.note("bound to state root %s", short(root))

				m := clientpkg.MaximalEnvelope{
					OperatorID:     kit.OperatorID,
					ToolName:       "fs_list",
					ArgumentsJSON:  `{"path":"."}`,
					TargetResource: "localhost",
					StateRoot:      root,
					Ensemble:       kit.Ensemble,
					TTL:            c.Config().EnvelopeTTL,
				}

				switch kit.L3Mode {
				case "mock":
					// Attach a principal Ed25519 signature over the hash as the
					// L3 proof. Simplest faithful "signing from a principal".
					m.Principal = kit.Principal
					r.note("L3 mode=mock: principal %q signs transaction_hash inline", kit.Principal.KeyID)
					txHash, status, _, err := c.SubmitMaximal(ctx, ensembleProducer, m)
					if err != nil {
						return err
					}
					r.tx(txHash)
					r.note("submitted notary envelope %s with inline principal proof (status %d)", short(txHash), status)
					return nil

				default: // "suspend" — drive the REAL out-of-band human-notary flow.
					r.note("L3 mode=suspend: submit L2-only, then principal authorizes the exact hash OOB")
					txHash, status, body, err := c.SubmitMaximal(ctx, ensembleProducer, m)
					if err != nil {
						return err
					}
					r.tx(txHash)
					r.note("envelope %s submitted (status %d); awaiting L3", short(txHash), status)

					// The gateway may echo an /approve/{hash} URL; trust our own
					// computed hash regardless and authorize it as the principal.
					if h, ok := suspendedFromBody(body); ok {
						txHash = h
					}
					ast, _, aerr := c.Approve(ctx, principalActor, txHash)
					if aerr != nil {
						return aerr
					}
					r.note("principal %q approved hash %s via OOB notary (status %d)",
						kit.Principal.KeyID, short(txHash), ast)
					return nil
				}
			},
		},
	}
}

func suspendedFromBody(body []byte) (string, bool) {
	// Reuse the JSON-RPC suspension detector by wrapping the raw body as Result.
	return clientpkg.Suspended(&clientpkg.JSONRPCResponse{Result: body})
}

func short(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12] + "…"
}
