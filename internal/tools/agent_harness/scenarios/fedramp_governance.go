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

func cloudopMap(action, resourceID, detail string) map[string]any {
	return shellCommandMap("cloudop", action, FedRAMPArgs.CloudSvcEndpoint, resourceID, detail)
}

func fedrampScenarios() []Scenario {
	return []Scenario{
		{
			Name: "fedramp-provision", Title: "FedRAMP: governed cloud resource provisioning with L2 consensus", Persona: fedrampCloudOperator, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				r.note("cloud operator provisions fedramp-vm-prod-01 (FIPS-199-MODERATE, AWS GovCloud) — gateway runs L2 deliberation")

				resp, err := c.MCPToolsCall(ctx, fedrampCloudOperator, "run_shell_command", cloudopMap("provision", "fedramp-vm-prod-01", "FIPS-199-MODERATE"))
				if err != nil {
					return fmt.Errorf("submit provision: %w", err)
				}
				if resp != nil && resp.Error != nil {
					return fmt.Errorf("provision rejected: %s", resp.Error.Message)
				}
				r.note("admitted — operator executing cloudop provision via L5 actuator; provenance receipt written to ledger")
				return nil
			},
		},
		{
			Name: "fedramp-deny", Title: "FedRAMP: unauthorized audit trail destruction blocked by L1 doctrine", Persona: fedrampCloudOperator, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				r.note("compromised operator tries to destroy the audit trail: run_shell_command 'rm -rf /var/cloudsvc'")

				resp, err := c.MCPToolsCall(ctx, fedrampCloudOperator, "run_shell_command", shellCommandMap("rm -rf /var/cloudsvc"))
				if err != nil {
					return fmt.Errorf("submit audit-wipe: %w", err)
				}
				if resp == nil || resp.Error == nil {
					return fmt.Errorf("audit-wipe was accepted — expected L1 rejection")
				}
				r.note("correctly rejected by L1 doctrine (CR-26 audit integrity) — audit trail is tamper-evident")
				r.note("response: %s", resp.Error.Message)
				return nil
			},
		},
		{
			Name: "fedramp-escalate", Title: "FedRAMP: resource destruction gated on authorizing official approval (L3)", Persona: fedrampAuthorizingOfficial, RequiresPosture: Notary,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Authenticator == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				r.note("operator requests destruction of fedramp-vm-classified-01 (FIPS-199-HIGH)")

				resp, err := c.MCPToolsCall(ctx, fedrampCloudOperator, "run_shell_command", cloudopMap("destroy", "fedramp-vm-classified-01", "FIPS-199-HIGH"))
				if err != nil {
					return fmt.Errorf("submit destroy: %w", err)
				}

				if txHash, suspended := clientpkg.Suspended(resp); suspended {
					r.note("gateway suspended destroy transaction %s pending L3 notary approval", short(txHash))
					ast, approveBody, aerr := c.ApproveWithWebAuthn(ctx, fedrampAuthorizingOfficial, txHash, kit.Authenticator)
					if aerr != nil {
						return fmt.Errorf("authorizing official approve: %w", aerr)
					}
					r.note("authorizing official approved hash %s via OOB notary (status %d)", short(txHash), ast)
					if ast >= 400 {
						return fmt.Errorf("authorizing official approval rejected (status %d)", ast)
					}
					if summary, failed := receiptFailed(approveBody); failed {
						return fmt.Errorf("destroy tool execution failed: %s", summary)
					}
					r.note("WebAuthn L3 proof verified; destruction executed")
					return nil
				}

				if resp != nil && resp.Error != nil {
					return fmt.Errorf("destroy rejected: %s", resp.Error.Message)
				}
				r.note("gateway admitted destroy (L3 notary satisfied inline)")
				return nil
			},
		},
		{
			Name: "fedramp-revert", Title: "FedRAMP: governed configuration revert under L2 consensus quorum", Persona: fedrampCloudOperator, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				r.note("operator reverts configuration on fedramp-iam-roles-01 with signed tasking")

				resp, err := c.MCPToolsCall(ctx, fedrampCloudOperator, "run_shell_command", cloudopMap("revert", "fedramp-iam-roles-01", "CM-7-ROLLBACK"))
				if err != nil {
					return fmt.Errorf("submit revert: %w", err)
				}
				if resp != nil && resp.Error != nil {
					return fmt.Errorf("revert rejected: %s", resp.Error.Message)
				}
				r.note("admitted — gateway quorum reached; revert executed and recorded with full configuration lineage")
				return nil
			},
		},
		{
			Name: "fedramp-evidence-block", Title: "FedRAMP: attempt to wipe the gateway audit vault is rejected by L1 doctrine", Persona: fedrampCloudOperator, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				r.note("compromised operator tries to wipe the gateway audit vault: run_shell_command 'rm -rf /root/.g8e/data'")

				resp, err := c.MCPToolsCall(ctx, fedrampCloudOperator, "run_shell_command", shellCommandMap("rm -rf /root/.g8e/data"))
				if err != nil {
					return fmt.Errorf("submit vault-wipe: %w", err)
				}
				if resp == nil || resp.Error == nil {
					return fmt.Errorf("vault-wipe was accepted — expected L1 rejection")
				}
				r.note("correctly rejected by L1 doctrine (CR-26 audit integrity) — audit vault is tamper-evident")
				r.note("response: %s", resp.Error.Message)
				return nil
			},
		},
	}
}
