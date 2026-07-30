// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
)

// The FedRAMP "Sovereign Cloud Governance" scenarios exercise the REAL g8e
// governance path for a FedRAMP-authorized cloud service provider:
//
//   - a cloud service operator submits a genuine GovernanceEnvelope wrapping
//     a run_shell_command that drives the Sovereign Cloud Service (the L5
//     actuator, analogous to the DHS datasvc);
//   - cloud resource operations (provision / configure / destroy / revert) are
//     admitted only after L1 doctrine + L2 consensus (+ L3 notary where required);
//   - a resource destruction requires out-of-band principal (FedRAMP
//     authorizing official) approval — the real L3 notary suspend/approve flow;
//   - an attempt to wipe the audit trail is rejected by L1 before it ever
//     reaches the actuator;
//   - a configuration revert with L2 consensus demonstrates governed rollback.
//
// Nothing here is mocked except the actor identities (personas) and the
// resource content. The Gateway, Operator, hashing, consensus verification,
// notary flow, hash-chained ledger, and signed receipts are all real.

// fedrampCloudOperator is the identity a FedRAMP cloud service operator wears.
var fedrampCloudOperator = clientpkg.Persona{
	ID:        "fedramp-cloud-operator",
	UserAgent: "g8e-fedramp-operator/1.x (sovereign cloud governance)",
}

// fedrampAuthorizingOfficial is the identity the FedRAMP authorizing official
// wears when approving resource destruction out-of-band (the L3 principal).
var fedrampAuthorizingOfficial = clientpkg.Persona{
	ID:        "fedramp-authorizing-official",
	UserAgent: "g8e-fedramp-ao/1.x (L3 notary)",
}

// FedRAMPArgs holds the configurable parameters for the FedRAMP scenarios.
// Defaults target the Sovereign Cloud Service endpoint on net_secure in the
// fedramp demo compose topology; a different topology can override the endpoint.
var FedRAMPArgs = struct {
	CloudSvcEndpoint string
}{
	CloudSvcEndpoint: "10.73.0.50:9100",
}

// cloudopArgs builds the run_shell_command arguments_json that drives the
// Sovereign Cloud Service via the `cloudop` wrapper (the bridge that lets the
// operator's governed execution reach the actuator without tripping the
// curl/wget denylist — exactly the DHS dataop pattern).
func cloudopArgs(action, resourceID, detail string) string {
	return shellCommandArgs("cloudop", action, FedRAMPArgs.CloudSvcEndpoint, resourceID, detail)
}

func fedrampScenarios() []Scenario {
	return []Scenario{
		{
			Name: "fedramp-provision", Title: "FedRAMP: governed cloud resource provisioning with L2 consensus", Persona: fedrampCloudOperator, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("cloud operator provisions fedramp-vm-prod-01 (FIPS-199-MODERATE, AWS GovCloud) — L2 consensus + L3 notary")

				txHash, status, body, err := c.SubmitMaximal(ctx, fedrampCloudOperator, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     cloudopArgs("provision", "fedramp-vm-prod-01", "FIPS-199-MODERATE"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					Principal:         kit.Principal,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit provision envelope: %w", err)
				}
				r.tx(txHash)
				r.note("provision envelope %s submitted (admission status %d)", short(txHash), status)
				if status >= 400 {
					return fmt.Errorf("provision envelope rejected (status %d): %s", status, string(body))
				}
				if summary, failed := receiptFailed(body); failed {
					return fmt.Errorf("provision tool execution failed: %s", summary)
				}
				r.note("admitted — operator executing cloudop provision via L5 actuator; provenance receipt written to ledger")
				return nil
			},
		},
		{
			Name: "fedramp-deny", Title: "FedRAMP: unauthorized audit trail destruction blocked by L1 doctrine", Persona: fedrampCloudOperator, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("compromised operator tries to destroy the audit trail: run_shell_command 'rm -rf /var/cloudsvc'")

				txHash, status, body, err := c.SubmitMaximal(ctx, fedrampCloudOperator, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     shellCommandArgs("rm -rf /var/cloudsvc"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					Principal:         kit.Principal,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit audit-wipe envelope: %w", err)
				}
				r.tx(txHash)
				r.note("audit-wipe envelope %s submitted (status %d)", short(txHash), status)
				if status < 400 {
					return fmt.Errorf("audit-wipe was accepted (status %d) — expected L1 rejection", status)
				}
				r.note("correctly rejected by L1 doctrine (CR-26 audit integrity) — audit trail is tamper-evident")
				r.note("response: %s", string(body))
				return nil
			},
		},
		{
			Name: "fedramp-escalate", Title: "FedRAMP: resource destruction gated on authorizing official approval (L3)", Persona: fedrampAuthorizingOfficial, RequiresPosture: Notary,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("operator requests destruction of fedramp-vm-classified-01 (FIPS-199-HIGH)")

				m := clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     cloudopArgs("destroy", "fedramp-vm-classified-01", "FIPS-199-HIGH"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					TTL:               c.Config().EnvelopeTTL,
				}

				switch kit.L3Mode {
				case "mock":
					r.note("L3 mode=mock: principal %q signs transaction_hash inline", kit.Principal.KeyID)
					m.Principal = kit.Principal
					txHash, status, body, err := c.SubmitMaximal(ctx, fedrampCloudOperator, m)
					if err != nil {
						return fmt.Errorf("submit destroy envelope: %w", err)
					}
					r.tx(txHash)
					r.note("destroy envelope %s submitted with inline principal proof (status %d)", short(txHash), status)
					if status >= 400 {
						return fmt.Errorf("destroy envelope rejected (status %d): %s", status, string(body))
					}
					if summary, failed := receiptFailed(body); failed {
						return fmt.Errorf("destroy tool execution failed: %s", summary)
					}
					r.note("authorizing official %q approved inline (mock L3); destruction executed", kit.Principal.KeyID)
					return nil

				case "webauthn":
					r.note("L3 mode=webauthn: software passkey signs transaction_hash inline")
					m.Authenticator = kit.Authenticator
					txHash, status, body, err := c.SubmitMaximal(ctx, fedrampCloudOperator, m)
					if err != nil {
						return fmt.Errorf("submit destroy envelope: %w", err)
					}
					r.tx(txHash)
					r.note("destroy envelope %s submitted with inline WebAuthn proof (status %d)", short(txHash), status)
					if status >= 400 {
						return fmt.Errorf("destroy envelope rejected (status %d): %s", status, string(body))
					}
					if summary, failed := receiptFailed(body); failed {
						return fmt.Errorf("destroy tool execution failed: %s", summary)
					}
					r.note("WebAuthn L3 proof verified; destruction executed")
					return nil

				default:
					r.note("L3 mode=suspend: submit L2-only; resource stays under CSP control until the authorizing official signs")
					txHash, status, body, err := c.SubmitMaximal(ctx, fedrampCloudOperator, m)
					if err != nil {
						return fmt.Errorf("submit destroy envelope: %w", err)
					}
					r.tx(txHash)
					r.note("destroy envelope %s submitted (status %d); awaiting authorizing official approval", short(txHash), status)

					suspendedHash, ok := suspendedFromBody(body)
					if !ok {
						if status >= 400 {
							return fmt.Errorf("destroy envelope rejected (status %d): %s", status, string(body))
						}
						return fmt.Errorf("destroy envelope was not suspended (status %d): expected suspension pending L3 notary approval", status)
					}
					txHash = suspendedHash
					ast, approveBody, aerr := c.Approve(ctx, fedrampAuthorizingOfficial, txHash)
					if aerr != nil {
						return fmt.Errorf("authorizing official approve: %w", aerr)
					}
					if ast >= 400 {
						return fmt.Errorf("authorizing official approval rejected (status %d)", ast)
					}
					if summary, failed := receiptFailed(approveBody); failed {
						return fmt.Errorf("destroy tool execution failed: %s", summary)
					}
					r.note("authorizing official %q approved hash %s out-of-band (status %d)", kit.Principal.KeyID, short(txHash), ast)
					r.note("cryptographic proof: principal Ed25519 signature over the exact transaction hash — destruction now executes")
					return nil
				}
			},
		},
		{
			Name: "fedramp-revert", Title: "FedRAMP: governed configuration revert under L2 consensus quorum", Persona: fedrampCloudOperator, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("operator reverts configuration on fedramp-iam-roles-01 with signed tasking — quorum %d", kit.Ensemble.AgentCount())

				txHash, status, body, err := c.SubmitMaximal(ctx, fedrampCloudOperator, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     cloudopArgs("revert", "fedramp-iam-roles-01", "CM-7-ROLLBACK"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					Principal:         kit.Principal,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit revert envelope: %w", err)
				}
				r.tx(txHash)
				r.note("revert envelope %s submitted (admission status %d)", short(txHash), status)
				if status >= 400 {
					return fmt.Errorf("revert envelope rejected (status %d): %s", status, string(body))
				}
				if summary, failed := receiptFailed(body); failed {
					return fmt.Errorf("revert tool execution failed: %s", summary)
				}
				r.note("admitted — ensemble quorum reached; revert executed and recorded with full configuration lineage")
				return nil
			},
		},
		{
			Name: "fedramp-evidence-block", Title: "FedRAMP: attempt to wipe the gateway audit vault is rejected by L1 doctrine", Persona: fedrampCloudOperator, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("compromised operator tries to wipe the gateway audit vault: run_shell_command 'rm -rf /root/.g8e/data'")

				txHash, status, body, err := c.SubmitMaximal(ctx, fedrampCloudOperator, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     shellCommandArgs("rm -rf /root/.g8e/data"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					Principal:         kit.Principal,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit vault-wipe envelope: %w", err)
				}
				r.tx(txHash)
				r.note("vault-wipe envelope %s submitted (status %d)", short(txHash), status)
				if status < 400 {
					return fmt.Errorf("vault-wipe was accepted (status %d) — expected L1 rejection", status)
				}
				r.note("correctly rejected by L1 doctrine (CR-26 audit integrity) — audit vault is tamper-evident")
				r.note("response: %s", string(body))
				return nil
			},
		},
	}
}
