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
	root := flag.String("root", "../../../..", "Repository root directory")
	flag.Parse()

	// Convert root to absolute path to ensure consistent resolution regardless of CWD
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving absolute path for root: %v\n", err)
		os.Exit(1)
	}
	root = &absRoot

	// Validate root path by checking for expected marker files/directories
	expectedMarkers := []string{
		filepath.Join(*root, "services", "g8eo"),
		filepath.Join(*root, "protocol"),
		filepath.Join(*root, "VERSION"),
	}
	for _, marker := range expectedMarkers {
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: root path validation failed - expected marker not found: %s\n", marker)
			fmt.Fprintf(os.Stderr, "The resolved root directory is: %s\n", *root)
			fmt.Fprintf(os.Stderr, "Use --root to specify the correct repository root.\n")
			os.Exit(1)
		}
	}

	fmt.Printf("Exporting constants from Go SSOT (root: %s)...\n", *root)

	snapshot := constants.Registry()

	// Resolve output directories
	protocolConstantsDir := filepath.Join(*root, "protocol", "constants")
	protocolPythonDir := filepath.Join(*root, "protocol", "python", "g8e_protocol")
	appConstantsDir := filepath.Join(*root, "services", "g8ee", "app", "constants")
	scriptDir := filepath.Join(*root, "scripts", "cmd")
	docsReferenceDir := filepath.Join(*root, "docs", "reference")

	// Ensure directories exist
	for _, dir := range []string{protocolConstantsDir, protocolPythonDir, appConstantsDir, scriptDir, docsReferenceDir} {
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
		InternalPrefix: constants.APIPaths.InternalPrefix,
		OperatorPrefix: constants.APIPaths.OperatorPrefix,
		G8ee:           constants.APIPaths.G8ee,
		G8eeFull:       make(map[string]string),
		Client:         constants.APIPaths.Client,
		ClientFull:     make(map[string]string),
	}
	for k, v := range constants.APIPaths.G8ee {
		apiPathsFull.G8eeFull[k] = constants.APIPaths.InternalPrefix + v
	}
	for k, v := range constants.APIPaths.Client {
		apiPathsFull.ClientFull[k] = constants.APIPaths.OperatorPrefix + v
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

	// Emit Python module files - protocol package uses simple module-level constants (external API)
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
	// Dynamically iterate over all status categories
	categoryKeys := make([]string, 0, len(snapshot.Status))
	for category := range snapshot.Status {
		categoryKeys = append(categoryKeys, category)
	}
	sort.Strings(categoryKeys)
	for _, category := range categoryKeys {
		entries := snapshot.Status[category]
		entryKeys := make([]string, 0, len(entries))
		for k := range entries {
			entryKeys = append(entryKeys, k)
		}
		sort.Strings(entryKeys)
		for _, k := range entryKeys {
			flattenedStatus[category+"."+k] = entries[k]
		}
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

	// Emit g8ee app constants Python files - use StrEnum for events/status, module-level for others
	emitG8eePythonModule := func(filename string, groupName string, entries map[string]constants.Entry, asStrEnum bool, className string) {
		path := filepath.Join(appConstantsDir, filename)
		lines := []string{
			"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
			"",
		}
		if asStrEnum {
			lines = append(lines, "from enum import StrEnum", "")
			if className != "" {
				lines = append(lines, fmt.Sprintf("class %s(StrEnum):", className))
			} else {
				// Convert groupName to PascalCase for class name
				classParts := strings.Split(groupName, "_")
				var generatedClassName string
				for _, part := range classParts {
					if len(part) > 0 {
						generatedClassName += strings.ToUpper(string(part[0])) + part[1:]
					}
				}
				lines = append(lines, fmt.Sprintf("class %s(StrEnum):", generatedClassName))
			}
			lines = append(lines, fmt.Sprintf(`    """Generated from protocol/constants/%s.json"""`, groupName))
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

	// Emit g8ee status constants as StrEnum classes (grouped by category)
	emitG8eePythonStatusModule := func(filename string, entries map[string]constants.Entry) {
		path := filepath.Join(appConstantsDir, filename)
		lines := []string{
			"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
			"",
			"from enum import StrEnum",
			"",
		}

		// Group entries by category (the full key before the first dot)
		// Keys are in format "category.entry_name" e.g., "action_status.cancelled"
		categoryMap := make(map[string][]struct {
			constName string
			value     string
		})
		for key, entry := range entries {
			// Find the first dot to separate category from entry name
			firstDot := strings.Index(key, ".")
			if firstDot == -1 {
				continue // Skip malformed keys
			}
			category := key[:firstDot]
			categoryMap[category] = append(categoryMap[category], struct {
				constName string
				value     string
			}{
				constName: entry.PythonConst,
				value:     entry.Value,
			})
		}

		// Emit each category as a StrEnum class
		categoryKeys := make([]string, 0, len(categoryMap))
		for category := range categoryMap {
			categoryKeys = append(categoryKeys, category)
		}
		sort.Strings(categoryKeys)

		for _, category := range categoryKeys {
			// Convert category to PascalCase class name
			// Category names are in snake_case (e.g., "action_status", "ai_task_id")
			// Keep acronyms fully capitalized (e.g., "ai_source" → "AISource", "api_key_status" → "APIKeyStatus")
			// Special cases: "llm_models" → "LLMs", "g8e_action_type" → "G8eActionType", "g8e_availability" → "G8eAvailability"
			var className string
			if category == "llm_models" {
				className = "LLMs"
			} else if category == "g8e_action_type" {
				className = "G8eActionType"
			} else if category == "g8e_availability" {
				className = "G8eAvailability"
			} else {
				acronyms := map[string]bool{
					"ai":   true,
					"api":  true,
					"cli":  true,
					"kv":   true,
					"llm":  true,
					"ssh":  true,
					"tcp":  true,
					"udp":  true,
					"g8e":  true,
					"g8ee": true,
					"g8eo": true,
				}
				classParts := strings.Split(category, "_")
				for _, part := range classParts {
					if len(part) > 0 {
						// If the part is a known acronym, keep it fully capitalized
						if acronyms[strings.ToLower(part)] {
							className += strings.ToUpper(part)
						} else {
							className += strings.ToUpper(string(part[0])) + part[1:]
						}
					}
				}
			}

			lines = append(lines, fmt.Sprintf("class %s(StrEnum):", className))
			lines = append(lines, fmt.Sprintf(`    """Generated from protocol/constants/status.json - %s"""`, category))

			// Sort entries within category
			entries := categoryMap[category]
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].constName < entries[j].constName
			})

			for _, entry := range entries {
				lines = append(lines, fmt.Sprintf("    %s = %q", entry.constName, entry.value))
			}
			lines = append(lines, "")
		}

		content := strings.Join(lines, "\n")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  Wrote %s\n", path)
	}

	emitG8eePythonModule("generated_collections.py", "collections", snapshot.Collections, false, "")
	emitG8eePythonModule("generated_events.py", "events", snapshot.Events, true, "EventType")
	emitG8eePythonStatusModule("generated_status.py", flattenedStatus)
	emitG8eePythonModule("generated_headers.py", "headers", snapshot.Headers, false, "")
	emitG8eePythonModule("generated_intents.py", "intents", snapshot.Intents, false, "")
	emitG8eePythonModule("generated_channels.py", "channels", snapshot.Channels, false, "")

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
	emitAPIPathsShell := func(filename string) {
		path := filepath.Join(scriptDir, filename)
		lines := []string{
			"#!/usr/bin/env bash",
			"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
			"",
			fmt.Sprintf("export G8E_API_INTERNAL_PREFIX=%q", constants.APIPaths.InternalPrefix),
			fmt.Sprintf("export G8E_API_OPERATOR_PREFIX=%q", constants.APIPaths.OperatorPrefix),
			"",
		}

		// G8EE paths
		g8eeKeys := make([]string, 0, len(constants.APIPaths.G8ee))
		for k := range constants.APIPaths.G8ee {
			g8eeKeys = append(g8eeKeys, k)
		}
		sort.Strings(g8eeKeys)
		for _, k := range g8eeKeys {
			varName := strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
			lines = append(lines,
				fmt.Sprintf("export G8E_API_G8EE_%s=%q", varName, constants.APIPaths.G8ee[k]),
				fmt.Sprintf("export G8E_API_G8EE_%s_FULL=%q", varName, constants.APIPaths.InternalPrefix+constants.APIPaths.G8ee[k]),
			)
		}
		lines = append(lines, "")

		// Client paths
		clientKeys := make([]string, 0, len(constants.APIPaths.Client))
		for k := range constants.APIPaths.Client {
			clientKeys = append(clientKeys, k)
		}
		sort.Strings(clientKeys)
		for _, k := range clientKeys {
			varName := strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
			lines = append(lines,
				fmt.Sprintf("export G8E_API_CLIENT_%s=%q", varName, constants.APIPaths.Client[k]),
				fmt.Sprintf("export G8E_API_CLIENT_%s_FULL=%q", varName, constants.APIPaths.OperatorPrefix+constants.APIPaths.Client[k]),
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
	emitAPIPathsShell("api_paths.sh")

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
			"      --trust-bundle <path> Path to trust bundle PEM file (default: .g8e/pki/hub-bundle.pem or fetch from /.well-known/g8e/pki/hub-bundle.pem)",
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
