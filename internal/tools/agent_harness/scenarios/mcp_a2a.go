// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"encoding/json"
	"fmt"

	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
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
				if _, err := c.MCPToolsCall(ctx, claudeDesktop, tool, clientpkg.FSPathArgs{Path: "."}); err != nil {
					return err
				}
				return nil
			},
		},
		{
			Name: "healthcare-success", Title: "Authorized FHIR PA Submission", Persona: clinicalAgent, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				// Submit a governed PA request via the native run_shell_command tool
				// driving the paop wrapper. The doctrine engine evaluates the
				// payload against all 11 PHI/HIPAA rules before execution.
				resp, err := c.MCPToolsCall(ctx, clinicalAgent, "run_shell_command", paopMap("submit", "PA-2026-0045", "ClaimResponse", "preauthorization"))
				if err != nil {
					return fmt.Errorf("submit PA: %w", err)
				}
				if resp != nil && resp.Error != nil {
					return fmt.Errorf("PA submission rejected: %s", resp.Error.Message)
				}
				r.note("FHIR PA PA-2026-0045 submitted through governed native tool endpoint")
				return nil
			},
		},
		{
			Name: "healthcare-phi-blocked", Title: "PHI Exfiltration Blocked by Doctrine", Persona: clinicalAgent, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				// Attempt exfiltration that matches the phi_exfil_attempt doctrine
				// pattern. The command string "exfiltrate patient medical records"
				// triggers L1 at confidence 0.95 before reaching the operator.
				resp, err := c.MCPToolsCall(ctx, clinicalAgent, "run_shell_command", shellCommandMap("exfiltrate patient medical records"))
				if err != nil {
					return fmt.Errorf("submit exfiltration: %w", err)
				}
				if resp == nil || resp.Error == nil {
					return fmt.Errorf("PHI exfiltration was accepted — expected L1 rejection")
				}
				r.note("L1 Doctrine blocked PHI exfiltration as expected: %s", resp.Error.Message)
				return nil
			},
		},
		{
			Name: "healthcare-gold-card", Title: "Gold Card Auto-Approval (HB 3134 §6)", Persona: clinicalAgent, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				resp, err := c.MCPToolsCall(ctx, clinicalAgent, "run_shell_command", paopMap("gold-card", "PA-2026-0043", "ClaimResponse", "Dr. Priya Nair 96%"))
				if err != nil {
					return fmt.Errorf("submit gold-card PA: %w", err)
				}
				if resp != nil && resp.Error != nil {
					return fmt.Errorf("gold card PA submission failed: %s", resp.Error.Message)
				}
				r.note("PA-2026-0043 gold-card operation recorded through governed endpoint (Dr. Priya Nair, 96%% historic approval)")
				r.note("reporting outcome is a pre-seeded fixture and is not run-bound evidence")
				return nil
			},
		},
		{
			Name: "healthcare-sla-breach", Title: "SLA Breach and OHA Reporting (2026 CCO Medicaid Rule)", Persona: clinicalAgent, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				resp, err := c.MCPToolsCall(ctx, clinicalAgent, "run_shell_command", paopMap("sla-check", "PA-2026-0044", "ClaimResponse", "Dr. James O'Brien 10 days"))
				if err != nil {
					return fmt.Errorf("submit SLA query: %w", err)
				}
				if resp != nil && resp.Error != nil {
					return fmt.Errorf("SLA breach query failed: %s", resp.Error.Message)
				}
				r.note("PA-2026-0044 SLA query recorded through governed endpoint (Dr. James O'Brien, 10 days elapsed)")
				r.note("reporting outcome is a pre-seeded fixture and is not run-bound evidence")
				return nil
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
				// MCPPromptsGet accepts schema-less map[string]any arguments because
				// prompt argument shapes vary per prompt definition on the server
				// (see protocols.go). The map literal here is the required input
				// shape for that schema-less API, not a known-shape model.
				if _, err := c.MCPPromptsGet(ctx, cursor, "summarize", map[string]any{"target": "."}); err != nil {
					return err
				}
				// Chained tool calls (read → grep) like a real coding agent.
				if _, err := c.MCPToolsCall(ctx, cursor, "fs_read", clientpkg.FSPathArgs{Path: "/etc/hostname"}); err != nil {
					return err
				}
				if _, err := c.MCPToolsCall(ctx, cursor, "fs_grep", clientpkg.FSGrepArgs{Path: ".", Pattern: "TODO"}); err != nil {
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
				if _, err := c.MCPToolsCall(ctx, enterpriseTool, "fs_list", clientpkg.FSPathArgs{Path: "/tmp"}); err != nil {
					return err
				}
				r.note("authenticated benign call submitted (transport: mTLS%s)", apiKeyNote(c))
				// (b) a forbidden command that L1 Doctrine must hard-gate even
				// before consensus/notary. This is the 'security' the demo proves.
				resp, err := c.MCPToolsCall(ctx, enterpriseTool, "execute_bash", clientpkg.ExecuteBashArgs{Command: "sudo rm -rf /"})
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
				// A2ACall accepts schema-less map[string]any payloads because skill
				// payload shapes vary per skill definition on the server (see
				// protocols.go). The map literal here is the required input shape
				// for that schema-less API, not a known-shape model.
				_, err := c.A2ACall(ctx, a2aPeer, "list_directory",
					map[string]any{"path": "."}, uuid.NewString())
				return err
			},
		},
		{
			Name: "a2a-secured", Title: "A2A with simple security (mTLS + L1 skill gate)", Persona: a2aSecure, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				// A2ACall payload: schema-less map[string]any per A2ACall API (see protocols.go).
				if _, err := c.A2ACall(ctx, a2aSecure, "read_file",
					map[string]any{"path": "/etc/hostname"}, uuid.NewString()); err != nil {
					return err
				}
				r.note("authenticated A2A skill submitted (transport: mTLS%s)", apiKeyNote(c))
				// skill_name carries L1 forbidden patterns (sudo, su); this must be gated.
				// A2ACall payload: schema-less map[string]any per A2ACall API (see protocols.go).
				resp, err := c.A2ACall(ctx, a2aSecure, "sudo",
					map[string]any{"cmd": "cat /etc/shadow"}, uuid.NewString())
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
				// listDirectoryPayload captures the known shape of the list_directory
				// skill payload; unlike A2ACall (which takes schema-less
				// map[string]any), A2ACallProto accepts a JSON string we construct
				// from a typed struct.
				inner, _ := json.Marshal(listDirectoryPayload{Path: ".", Recursive: false})
				_, err := c.A2ACallProto(ctx, a2aProto, "list_directory", string(inner), uuid.NewString())
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

// paopArgs builds the run_shell_command arguments_json that drives the
// healthcare PA operation via the `paop` wrapper (the bridge that lets the
// agent's governed execution simulate PA submissions without an external
// downstream MCP server — exactly the DHS dataop / FedRAMP cloudop pattern).
func paopArgs(action, requestID, resourceType, detail string) string {
	return shellCommandArgs("paop", action, requestID, resourceType, detail)
}

func paopMap(action, requestID, resourceType, detail string) clientpkg.ShellCommandArgs {
	return shellCommandMap("paop", action, requestID, resourceType, detail)
}

// listDirectoryPayload is the typed A2A skill payload for the list_directory
// skill, marshaled as the A2ACallRequested payload_json in the protobuf
// scenario. A2ACallProto accepts a JSON string rather than the schema-less
// map[string]any that A2ACall uses, so the call site constructs a typed struct
// and marshals it directly.
type listDirectoryPayload struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
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
