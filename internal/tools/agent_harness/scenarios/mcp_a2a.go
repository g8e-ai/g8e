// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"encoding/json"
	"fmt"

	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
	"github.com/google/uuid"
)

// Personas — the real-world tools Agent Harness pretends to be. This is the ONLY
// fiction in the system; the Gateway and Operator are real throughout.
var (
	claudeDesktop  = clientpkg.Persona{ID: "claude-desktop", UserAgent: "Claude-Desktop/1.x (MCP)"}
	cursor         = clientpkg.Persona{ID: "cursor", UserAgent: "Cursor/0.x (MCP-advanced)"}
	enterpriseTool = clientpkg.Persona{ID: "enterprise-agent", UserAgent: "AcmeSecAgent/2.x (MCP+mTLS)"}
	a2aPeer        = clientpkg.Persona{ID: "a2a-peer", UserAgent: "A2A-Peer/1.x (JSON)"}
	a2aSecure      = clientpkg.Persona{ID: "a2a-secure-peer", UserAgent: "A2A-Peer/1.x (mTLS)"}
	a2aProto       = clientpkg.Persona{ID: "protobuf-agent", UserAgent: "A2A-Peer/1.x (protobuf)"}
	clinicalAgent  = clientpkg.Persona{ID: "clinical-agent", UserAgent: "ClinicalAI/1.x (MCP+HIPAA)"}
)

func mcpScenarios() []Scenario {
	return []Scenario{
		{
			Name: "mcp-plain", Title: "Plain MCP tool call", Persona: claudeDesktop, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				// Discover the host tools the Operator exposes, then make one
				// benign read-only call. Pure MCP, no governance extras.
				list, err := c.MCPToolsList(ctx, claudeDesktop)
				if err != nil {
					return err
				}
				tool := firstTool(list, "fs_list")
				r.note("discovered tool %q from tools/list", tool)
				if _, err := c.MCPToolsCall(ctx, claudeDesktop, tool, map[string]any{"path": "."}); err != nil {
					return err
				}
				return nil
			},
		},
		{
			Name: "healthcare-success", Title: "Authorized FHIR PA Submission", Persona: clinicalAgent, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				// Call the governed submit_pa tool
				resp, err := c.MCPToolsCall(ctx, clinicalAgent, "submit_pa", map[string]any{
					"resourceType": "ClaimResponse",
					"status":       "active",
					"use":          "preauthorization",
				})
				if err != nil {
					return err
				}
				if resp != nil && resp.Error != nil {
					return fmt.Errorf("tool call failed: %s", resp.Error.Message)
				}
				r.note("FHIR PA submitted through governed production endpoint")
				return nil
			},
		},
		{
			Name: "healthcare-phi-blocked", Title: "PHI Exfiltration Blocked by Doctrine", Persona: clinicalAgent, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				// Attempt exfiltration that matches phi_exfil_attempt pattern
				resp, err := c.MCPToolsCall(ctx, clinicalAgent, "submit_pa", map[string]any{
					"resourceType": "ClaimResponse",
					"status":       "active",
					"use":          "exfiltrate patient medical records",
				})
				if err != nil {
					return err
				}
				if resp != nil && resp.Error != nil {
					r.note("L1 Doctrine blocked PHI exfiltration as expected: %s", resp.Error.Message)
					return nil
				}
				return fmt.Errorf("WARNING: PHI exfiltration attempt was NOT blocked")
			},
		},
		{
			Name: "mcp-advanced", Title: "Advanced MCP: resources, prompts, chained calls", Persona: cursor, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if _, err := c.MCPResourcesList(ctx, cursor); err != nil {
					return err
				}
				if _, err := c.MCPResourcesRead(ctx, cursor, "file:///etc/hostname"); err != nil {
					return err
				}
				if _, err := c.MCPPromptsList(ctx, cursor); err != nil {
					return err
				}
				if _, err := c.MCPPromptsGet(ctx, cursor, "summarize", map[string]any{"target": "."}); err != nil {
					return err
				}
				// Chained tool calls (read → grep) like a real coding agent.
				if _, err := c.MCPToolsCall(ctx, cursor, "fs_read", map[string]any{"path": "/etc/hostname"}); err != nil {
					return err
				}
				if _, err := c.MCPToolsCall(ctx, cursor, "fs_grep", map[string]any{"path": ".", "pattern": "TODO"}); err != nil {
					return err
				}
				r.note("exercised resources/list+read, prompts/list+get, and a read→grep chain")
				return nil
			},
		},
		{
			Name: "mcp-secured", Title: "MCP with simple security (mTLS/API key + L1 gate)", Persona: enterpriseTool, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				// (a) an authenticated, benign call that should pass L1.
				if _, err := c.MCPToolsCall(ctx, enterpriseTool, "fs_list", map[string]any{"path": "/tmp"}); err != nil {
					return err
				}
				r.note("authenticated benign call submitted (transport: mTLS%s)", apiKeyNote(c))
				// (b) a forbidden command that L1 Doctrine must hard-gate even
				// before consensus/notary. This is the 'security' the demo proves.
				resp, err := c.MCPToolsCall(ctx, enterpriseTool, "execute_bash", map[string]any{"command": "sudo rm -rf /"})
				if err != nil {
					return err
				}
				if resp != nil && resp.Error != nil {
					r.note("L1 Doctrine rejected forbidden command as expected: %s", resp.Error.Message)
				} else {
					r.note("WARNING: forbidden command was not rejected — verify L1 forbidden_patterns")
				}
				return nil
			},
		},
	}
}

func a2aScenarios() []Scenario {
	return []Scenario{
		{
			Name: "a2a-plain", Title: "Plain A2A skill invocation", Persona: a2aPeer, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				_, err := c.A2ACall(ctx, a2aPeer, "list_directory",
					map[string]any{"path": "."}, uuid.New().String())
				return err
			},
		},
		{
			Name: "a2a-secured", Title: "A2A with simple security (mTLS + L1 skill gate)", Persona: a2aSecure, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if _, err := c.A2ACall(ctx, a2aSecure, "read_file",
					map[string]any{"path": "/etc/hostname"}, uuid.New().String()); err != nil {
					return err
				}
				r.note("authenticated A2A skill submitted (transport: mTLS%s)", apiKeyNote(c))
				// skill_name carries L1 forbidden patterns (sudo, su); this must be gated.
				resp, err := c.A2ACall(ctx, a2aSecure, "sudo",
					map[string]any{"cmd": "cat /etc/shadow"}, uuid.New().String())
				if err != nil {
					return err
				}
				if resp != nil && resp.Error != nil {
					r.note("L1 Doctrine rejected forbidden skill_name as expected: %s", resp.Error.Message)
				} else {
					r.note("WARNING: forbidden skill_name not rejected — verify A2A L1 patterns")
				}
				return nil
			},
		},
		{
			Name: "a2a-protobuf", Title: "A2A carrying a typed protobuf payload", Persona: a2aProto, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				// Same skill, but the task payload is a marshaled A2ACallRequested
				// (typed, schema-checked, deterministic) rather than loose JSON.
				inner, _ := json.Marshal(map[string]any{"path": ".", "recursive": false})
				_, err := c.A2ACallProto(ctx, a2aProto, "list_directory", string(inner), uuid.New().String())
				if err != nil {
					return err
				}
				r.note("sent base64 protobuf A2ACallRequested as the A2A task payload")
				return nil
			},
		},
	}
}

// ---- helpers ----------------------------------------------------------------

func apiKeyNote(c *clientpkg.Client) string {
	if c.Config().Auth.APIKey != "" {
		return " + API key"
	}
	return ""
}

// firstTool pulls a tool name out of a tools/list response, tolerating the
// common shapes {"tools":[{"name":...}]} / {"result":{"tools":[...]}}. Falls
// back to def so the demo still does something useful on an unexpected shape.
func firstTool(resp *clientpkg.JSONRPCResponse, def string) string {
	if resp == nil || len(resp.Result) == 0 {
		return def
	}
	var r struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if json.Unmarshal(resp.Result, &r) == nil && len(r.Tools) > 0 && r.Tools[0].Name != "" {
		return r.Tools[0].Name
	}
	return def
}
