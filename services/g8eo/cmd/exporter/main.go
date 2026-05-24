package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
)

type CollectionsExport struct {
	Collections map[string]constants.Entry `json:"collections"`
}

type EventsExport struct {
	Events map[string]constants.Entry `json:"events"`
}

type StatusExport struct {
	Status constants.StatusSnapshot `json:"status"`
}

type SendersExport struct {
	Senders map[string]constants.Entry `json:"senders"`
}

type KVKeysExport struct {
	CachePrefix string            `json:"cache.prefix"`
	KeySchema   map[string]string `json:"key.schema"`
	SessionType map[string]string `json:"session.type"`
}

type ChannelsExport struct {
	Channels map[string]constants.Entry `json:"channels"`
}

type PubSubExport struct {
	PubSub map[string]constants.Entry `json:"pubsub"`
}

type IntentsExport struct {
	Intents map[string]constants.Entry `json:"intents"`
}

type PromptsExport struct {
	Prompts map[string]constants.Entry `json:"prompts"`
}

type HeadersExport struct {
	Headers map[string]constants.Entry `json:"headers"`
}

type DocumentIdsExport struct {
	DocumentIds map[string]constants.Entry `json:"document_ids"`
}

type PlatformExport struct {
	Platform map[string]constants.Entry `json:"platform"`
}

type AgentsExport struct {
	Agents map[string]string `json:"agents"`
}

type PortsExport struct {
	Ports map[string]int `json:"ports"`
}

type EnvVarsExport struct {
	EnvVars map[string]string `json:"env_vars"`
}

type TimestampExport struct {
	Timestamp map[string]string `json:"timestamp"`
}

// isUpper returns true if the rune is an uppercase letter
func isUpper(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

// mergeMaps recursively merges existing map into new map, preserving hand-authored keys
// Go SSOT takes precedence at each level
func mergeMaps(newData, existing map[string]interface{}) map[string]interface{} {
	for key, existingValue := range existing {
		if newValue, exists := newData[key]; exists {
			// Both have this key - check if both are maps for recursive merge
			if newMap, ok := newValue.(map[string]interface{}); ok {
				if existingMap, ok := existingValue.(map[string]interface{}); ok {
					newData[key] = mergeMaps(newMap, existingMap)
				}
			}
			// If not both maps, Go SSOT value takes precedence (do nothing)
		} else {
			// Key only in existing - preserve it
			newData[key] = existingValue
		}
	}
	return newData
}

// emptyMapIfNil returns an empty map if the input map is nil
func emptyMapIfNil(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}

func main() {
	root := flag.String("root", "../..", "Repository root directory")
	flag.Parse()

	fmt.Printf("Exporting constants from Go SSOT (root: %s)...\n", *root)

	snapshot := constants.Registry()

	// Resolve output directories
	protocolConstantsDir := filepath.Join(*root, "protocol", "constants")
	protocolPythonDir := filepath.Join(*root, "protocol", "python", "g8e_protocol")
	appConstantsDir := filepath.Join(*root, "services", "g8ee", "app", "constants")
	scriptDir := filepath.Join(*root, "scripts", "cmd")

	// Ensure directories exist
	for _, dir := range []string{protocolConstantsDir, protocolPythonDir, appConstantsDir, scriptDir} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	// Emit JSON files with merge support for hand-authored content
	emitJSON := func(filename string, data interface{}, merge bool) {
		path := filepath.Join(protocolConstantsDir, filename)
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling %s: %v\n", filename, err)
			os.Exit(1)
		}

		// If file exists and merge is enabled, merge with existing content to preserve hand-authored keys
		if merge {
			if existingData, err := os.ReadFile(path); err == nil {
				var existing map[string]interface{}
				if err := json.Unmarshal(existingData, &existing); err == nil {
					var newData map[string]interface{}
					if err := json.Unmarshal(jsonData, &newData); err == nil {
						// Merge: Go SSOT takes precedence, but preserve hand-authored keys recursively
						newData = mergeMaps(newData, existing)
						jsonData, err = json.MarshalIndent(newData, "", "  ")
						if err != nil {
							fmt.Fprintf(os.Stderr, "Error marshaling merged %s: %v\n", filename, err)
							os.Exit(1)
						}
					}
				}
			}
		}

		if err := os.WriteFile(path, jsonData, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  Wrote %s\n", path)
	}

	emitJSON("collections.json", CollectionsExport{Collections: snapshot.Collections}, true)
	emitJSON("events.json", EventsExport{Events: snapshot.Events}, true)
	emitJSON("status.json", StatusExport{Status: snapshot.Status}, true)
	emitJSON("senders.json", SendersExport{Senders: snapshot.Senders}, true)
	emitJSON("kv_keys.json", KVKeysExport{
		CachePrefix: snapshot.KVKeys.CachePrefix,
		KeySchema:   emptyMapIfNil(snapshot.KVKeys.KeySchema),
		SessionType: emptyMapIfNil(snapshot.KVKeys.SessionType),
	}, true)
	emitJSON("channels.json", ChannelsExport{Channels: snapshot.Channels}, true)
	emitJSON("pubsub.json", PubSubExport{PubSub: snapshot.PubSub}, true)
	emitJSON("intents.json", IntentsExport{Intents: snapshot.Intents}, true)
	emitJSON("prompts.json", PromptsExport{Prompts: snapshot.Prompts}, true)
	emitJSON("headers.json", HeadersExport{Headers: snapshot.Headers}, true)
	emitJSON("document_ids.json", DocumentIdsExport{
		DocumentIds: snapshot.DocumentIds.DocumentIds,
	}, true)
	emitJSON("platform.json", PlatformExport{Platform: snapshot.Platform}, true)
	emitJSON("agents.json", AgentsExport{Agents: snapshot.Agents}, true)
	emitJSON("timestamp.json", TimestampExport{Timestamp: snapshot.Timestamp}, true)
	// Emit api_paths.json with full paths
	apiPathsFull := struct {
		InternalPrefix string            `json:"internal_prefix"`
		OperatorPrefix string            `json:"operator_prefix"`
		G8ee           map[string]string `json:"g8ee"`
		G8eeFull       map[string]string `json:"g8ee_full"`
		Client         map[string]string `json:"client"`
		ClientFull     map[string]string `json:"client_full"`
	}{
		InternalPrefix: constants.ApiPaths.InternalPrefix,
		OperatorPrefix: constants.ApiPaths.OperatorPrefix,
		G8ee:           constants.ApiPaths.G8ee,
		G8eeFull:       make(map[string]string),
		Client:         constants.ApiPaths.Client,
		ClientFull:     make(map[string]string),
	}
	for k, v := range constants.ApiPaths.G8ee {
		apiPathsFull.G8eeFull[k] = constants.ApiPaths.InternalPrefix + v
	}
	for k, v := range constants.ApiPaths.Client {
		apiPathsFull.ClientFull[k] = constants.ApiPaths.OperatorPrefix + v
	}
	emitJSON("api_paths.json", apiPathsFull, true)

	// Symlink or copy api_paths.json to g8ee app constants
	// This ensures the protocol version is the sole source of truth.
	apiPathsSrc := filepath.Join(protocolConstantsDir, "api_paths.json")
	apiPathsDest := filepath.Join(appConstantsDir, "api_paths.json")
	if err := os.Remove(apiPathsDest); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error removing old api_paths.json: %v\n", err)
		os.Exit(1)
	}
	// Copy the file content
	// #nosec G304 - internal exporter tool reading from known protocol directory
	apiPathsData, err := os.ReadFile(apiPathsSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading source api_paths.json: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(apiPathsDest, apiPathsData, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing destination api_paths.json: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Synced %s to %s\n", apiPathsSrc, apiPathsDest)

	// Emit Python module files
	emitPythonModule := func(filename string, groupName string, entries map[string]constants.Entry) {
		path := filepath.Join(protocolPythonDir, filename)
		lines := []string{
			"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
			"",
		}
		// Sort keys for deterministic output
		sortedKeys := make([]string, 0, len(entries))
		for k := range entries {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)
		for _, k := range sortedKeys {
			entry := entries[k]
			lines = append(lines, fmt.Sprintf("%s = %q", entry.PythonConst, entry.Value))
		}
		lines = append(lines, "")
		content := strings.Join(lines, "\n")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  Wrote %s\n", path)
	}

	emitPythonModule("collections.py", "collections", snapshot.Collections)
	emitPythonModule("events.py", "events", snapshot.Events)

	// Flatten StatusSnapshot for Python emission with deterministic ordering
	flattenedStatus := make(map[string]constants.Entry)
	// Sort each nested map's keys before flattening
	attachmentTypeKeys := make([]string, 0, len(snapshot.Status.AttachmentType))
	for k := range snapshot.Status.AttachmentType {
		attachmentTypeKeys = append(attachmentTypeKeys, k)
	}
	sort.Strings(attachmentTypeKeys)
	for _, k := range attachmentTypeKeys {
		flattenedStatus["attachment.type."+k] = snapshot.Status.AttachmentType[k]
	}

	userRoleKeys := make([]string, 0, len(snapshot.Status.UserRole))
	for k := range snapshot.Status.UserRole {
		userRoleKeys = append(userRoleKeys, k)
	}
	sort.Strings(userRoleKeys)
	for _, k := range userRoleKeys {
		flattenedStatus["user_role."+k] = snapshot.Status.UserRole[k]
	}

	userStatusKeys := make([]string, 0, len(snapshot.Status.UserStatus))
	for k := range snapshot.Status.UserStatus {
		userStatusKeys = append(userStatusKeys, k)
	}
	sort.Strings(userStatusKeys)
	for _, k := range userStatusKeys {
		flattenedStatus["user_status."+k] = snapshot.Status.UserStatus[k]
	}

	operatorStatusKeys := make([]string, 0, len(snapshot.Status.OperatorStatus))
	for k := range snapshot.Status.OperatorStatus {
		operatorStatusKeys = append(operatorStatusKeys, k)
	}
	sort.Strings(operatorStatusKeys)
	for _, k := range operatorStatusKeys {
		flattenedStatus["operator_status."+k] = snapshot.Status.OperatorStatus[k]
	}

	executionStatusKeys := make([]string, 0, len(snapshot.Status.ExecutionStatus))
	for k := range snapshot.Status.ExecutionStatus {
		executionStatusKeys = append(executionStatusKeys, k)
	}
	sort.Strings(executionStatusKeys)
	for _, k := range executionStatusKeys {
		flattenedStatus["execution_status."+k] = snapshot.Status.ExecutionStatus[k]
	}

	emitPythonModule("status.py", "status", flattenedStatus)

	emitPythonModule("senders.py", "senders", snapshot.Senders)
	emitPythonModule("channels.py", "channels", snapshot.Channels)
	emitPythonModule("pubsub.py", "pubsub", snapshot.PubSub)
	emitPythonModule("intents.py", "intents", snapshot.Intents)
	emitPythonModule("prompts.py", "prompts", snapshot.Prompts)
	emitPythonModule("headers.py", "headers", snapshot.Headers)
	emitPythonModule("document_ids.py", "document_ids", snapshot.DocumentIds.DocumentIds)
	emitPythonModule("platform.py", "platform", snapshot.Platform)
	// Skip paths.py - it's hand-authored with complex logic

	// Emit g8ee app constants Python files
	emitG8eePythonModule := func(filename string, groupName string, entries map[string]constants.Entry, asStrEnum bool) {
		path := filepath.Join(appConstantsDir, filename)
		lines := []string{
			"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
			"",
		}
		if asStrEnum {
			lines = append(lines, "from enum import StrEnum", "")
			lines = append(lines, "class EventType(StrEnum):")
			lines = append(lines, `    """Generated from protocol/constants/events.json"""`)
		}
		// Sort keys for deterministic output
		sortedKeys := make([]string, 0, len(entries))
		for k := range entries {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)
		for _, k := range sortedKeys {
			entry := entries[k]
			if asStrEnum {
				lines = append(lines, fmt.Sprintf("    %s = %q", entry.PythonConst, entry.Value))
			} else {
				lines = append(lines, fmt.Sprintf("%s = %q", entry.PythonConst, entry.Value))
			}
		}
		lines = append(lines, "")
		content := strings.Join(lines, "\n")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  Wrote %s\n", path)
	}

	emitG8eePythonModule("generated_collections.py", "collections", snapshot.Collections, false)
	emitG8eePythonModule("generated_events.py", "events", snapshot.Events, true)
	emitG8eePythonModule("generated_status.py", "status", flattenedStatus, false)
	emitG8eePythonModule("generated_headers.py", "headers", snapshot.Headers, false)
	emitG8eePythonModule("generated_intents.py", "intents", snapshot.Intents, false)
	emitG8eePythonModule("generated_channels.py", "channels", snapshot.Channels, false)

	// Emit headers.sh
	emitHeadersShell := func(filename string) {
		path := filepath.Join(scriptDir, filename)
		lines := []string{
			"#!/usr/bin/env bash",
			"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
			"",
		}
		// Sort keys for deterministic output
		sortedKeys := make([]string, 0, len(snapshot.Headers))
		for k := range snapshot.Headers {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)
		for _, k := range sortedKeys {
			entry := snapshot.Headers[k]
			// Convert PascalCase to SCREAMING_SNAKE_CASE
			// e.g., HeaderDeviceToken -> DEVICE_TOKEN
			// e.g., HeaderCLISessionID -> CLI_SESSION_ID
			goConst := entry.GoConst
			// Strip "Header" prefix if present
			goConst = strings.TrimPrefix(goConst, "Header")
			// Convert to SCREAMING_SNAKE_CASE
			var result []rune
			for i, r := range goConst {
				if i > 0 && isUpper(r) && (i+1 < len(goConst) && !isUpper(rune(goConst[i+1])) || (i > 0 && !isUpper(rune(goConst[i-1])))) {
					result = append(result, '_')
				}
				result = append(result, r)
			}
			varName := strings.ToUpper(string(result))
			// Replace any remaining hyphens with underscores
			varName = strings.ReplaceAll(varName, "-", "_")
			lines = append(lines, fmt.Sprintf("export G8E_HEADER_%s=%q", varName, entry.Value))
		}
		lines = append(lines, "")
		content := strings.Join(lines, "\n")
		// #nosec G306 - shell script needs execute permission
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  Wrote %s\n", path)
	}

	emitHeadersShell("headers.sh")

	// Emit api_paths.sh
	emitApiPathsShell := func(filename string) {
		path := filepath.Join(scriptDir, filename)
		lines := []string{
			"#!/usr/bin/env bash",
			"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
			"",
			fmt.Sprintf("export G8E_API_INTERNAL_PREFIX=%q", constants.ApiPaths.InternalPrefix),
			fmt.Sprintf("export G8E_API_OPERATOR_PREFIX=%q", constants.ApiPaths.OperatorPrefix),
			"",
		}

		// G8EE paths
		g8eeKeys := make([]string, 0, len(constants.ApiPaths.G8ee))
		for k := range constants.ApiPaths.G8ee {
			g8eeKeys = append(g8eeKeys, k)
		}
		sort.Strings(g8eeKeys)
		for _, k := range g8eeKeys {
			varName := strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
			lines = append(lines,
				fmt.Sprintf("export G8E_API_G8EE_%s=%q", varName, constants.ApiPaths.G8ee[k]),
				fmt.Sprintf("export G8E_API_G8EE_%s_FULL=%q", varName, constants.ApiPaths.InternalPrefix+constants.ApiPaths.G8ee[k]),
			)
		}
		lines = append(lines, "")

		// Client paths
		clientKeys := make([]string, 0, len(constants.ApiPaths.Client))
		for k := range constants.ApiPaths.Client {
			clientKeys = append(clientKeys, k)
		}
		sort.Strings(clientKeys)
		for _, k := range clientKeys {
			varName := strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
			lines = append(lines,
				fmt.Sprintf("export G8E_API_CLIENT_%s=%q", varName, constants.ApiPaths.Client[k]),
				fmt.Sprintf("export G8E_API_CLIENT_%s_FULL=%q", varName, constants.ApiPaths.OperatorPrefix+constants.ApiPaths.Client[k]),
			)
		}
		lines = append(lines, "")

		content := strings.Join(lines, "\n")
		// #nosec G306 - shell script needs execute permission
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  Wrote %s\n", path)
	}
	emitApiPathsShell("api_paths.sh")

	// Emit CLI reference markdown
	emitCLIReference := func(filename string) {
		path := filepath.Join(*root, "docs", "reference", filename)
		lines := []string{
			"# CLI Reference",
			"",
			"This document is auto-generated from the CLI help output. Do not edit manually.",
			"",
			"## g8e Platform Commands",
			"",
			"The `g8e` CLI is the primary interface for platform management.",
			"",
			"```",
			"g8e is a zero-trust execution substrate for agentic infrastructure.",
			"The CLI manages the Governance Gateway (g8eg), Governed Operator (g8eo),",
			"and optional application-layer adapters (g8ee).",
			"",
			"Usage:",
			"  g8e [command]",
			"",
			"Available Commands:",
			"  apps        Manage optional application-layer adapters",
			"  auth        Authentication and session management",
			"  data        Administer the local substrate over mTLS",
			"  evals       Run evaluation benchmarks",
			"  help        Help about any command",
			"  platform    Manage the Governance Gateway (g8eg) lifecycle",
			"  security    Security validation checks",
			"  setup       Bootstrap platform dependencies and configuration",
			"  test        Run test suites",
			"  vars        Environment variable management",
			"",
			"Flags:",
			"  -h, --help   help for g8e",
			"",
			"Use \"g8e [command] --help\" for more information about a command.",
			"```",
			"",
			"## g8eo Operator Binary",
			"",
			"The `g8e.operator` binary is the host-side Policy Execution Point (PEP).",
			"",
			"```",
			"Usage: g8e.operator [options]",
			"",
			"Options:",
			"  -k, --key <key>         API key (or set G8E_OPERATOR_API_KEY)",
			"  -D, --device-token <tok> Device link token for operator deployment",
			"  -e, --endpoint <host>     Operator endpoint: IP address of the Docker host running operator",
			fmt.Sprintf("      --trust-bundle <path> Path to trust bundle PEM file (default: .g8e/pki/hub-bundle.pem or fetch from /.well-known/g8e/pki/hub-bundle.pem)"),
			"      --working-dir <dir>   Working directory (default: directory operator was launched from)",
			"                            All commands and data storage are anchored to this directory",
			fmt.Sprintf("      --http-port <port>    HTTPS port to dial for auth/bootstrap (default: %d)", constants.Ports.OperatorHttps),
			"  -c, --cloud             Cloud Operator mode (for AWS/cloud CLI)",
			"  -p, --provider <name>   Cloud provider: aws, gcp, azure",
			"  -s, --local-storage     Store audit data locally instead of cloud (default: on)",
			"                          When enabled, data is stored in ./.g8e/ relative to launch directory",
			"  -l, --log <level>       Log level: info, error, debug (default: info)",
			"  -G, --no-git            Disable ledger (git-backed file versioning)",
			"      --heartbeat-interval <dur> Heartbeat interval (e.g. 60s, 2m); overrides the 30s default",
			"  -v, --version           Show version",
			"",
			"Gateway Mode (platform persistence + pub/sub broker):",
			"  --doctrine                Gateway mode: L1 enforced, L2/L3 audited (default)",
			"  --consensus               Gateway mode: L1/L2 enforced, L3 audited",
			"  --notary                  Gateway mode: L1/L2/L3 strictly enforced",
			fmt.Sprintf("  --http-listen-port <port>   HTTPS port for mTLS API (default: %d)", constants.Ports.OperatorHttps),
			fmt.Sprintf("  --bootstrap-listen-port <port> Bootstrap TLS port for device-link enrollment (default: %d)", constants.Ports.OperatorBootstrapHttps),
			fmt.Sprintf("  --public-listen-port <port> Public browser/BYO bootstrap port (default: %d)", constants.Ports.OperatorPublicHttps),
			"  --data-dir <dir>            Data directory for SQLite (default: .g8e/data in working directory)",
			"  --pki-dir <dir>             Directory for TLS certificates (default: .g8e/pki)",
			"  --secrets-dir <dir>         Directory for platform secrets (default: .g8e/secrets)",
			"  --passkey-rp-id <id>        RP ID for passkey operations (default: localhost)",
			"  --passkey-rp-name <name>    RP Name for passkey operations (default: g8e)",
			"",
			"Vault Management:",
			"  --rekey-vault           Re-encrypt vault with new API key",
			"  --old-key <key>         Old API key (required for --rekey-vault)",
			"  --verify-vault          Verify vault integrity",
			"  --reset-vault           Reset vault (DESTROYS ALL DATA)",
			"",
			"OpenClaw Node Host Mode:",
			"  --openclaw              Connect to an OpenClaw Gateway as a node host",
			fmt.Sprintf("  --openclaw-url <url>    OpenClaw Gateway WebSocket URL (e.g. ws://%s:18789)", constants.DefaultEndpoint),
			"  --openclaw-token <tok>  Auth token (or set OPENCLAW_GATEWAY_TOKEN)",
			"  --openclaw-node-id <id> Node ID advertised to the Gateway (default: hostname)",
			"  --openclaw-name <name>  Display name shown in OpenClaw UI (default: node ID)",
			"```",
		}
		content := strings.Join(lines, "\n")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  Wrote %s\n", path)
	}

	emitCLIReference("cli.md")

	fmt.Println("Constants exported successfully.")
}
