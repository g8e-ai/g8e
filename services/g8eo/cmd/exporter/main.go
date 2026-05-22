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

	// Emit JSON files
	emitJSON := func(filename string, data interface{}) {
		path := filepath.Join(protocolConstantsDir, filename)
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling %s: %v\n", filename, err)
			os.Exit(1)
		}
		if err := os.WriteFile(path, jsonData, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  Wrote %s\n", path)
	}

	emitJSON("collections.json", CollectionsExport{Collections: snapshot.Collections})
	emitJSON("events.json", EventsExport{Events: snapshot.Events})
	emitJSON("status.json", StatusExport{Status: snapshot.Status})
	emitJSON("senders.json", SendersExport{Senders: snapshot.Senders})
	emitJSON("kv_keys.json", KVKeysExport{
		CachePrefix: snapshot.KVKeys.CachePrefix,
		KeySchema:   snapshot.KVKeys.KeySchema,
		SessionType: snapshot.KVKeys.SessionType,
	})
	emitJSON("channels.json", ChannelsExport{Channels: snapshot.Channels})
	emitJSON("pubsub.json", PubSubExport{PubSub: snapshot.PubSub})
	emitJSON("intents.json", IntentsExport{Intents: snapshot.Intents})
	emitJSON("prompts.json", PromptsExport{Prompts: snapshot.Prompts})
	emitJSON("headers.json", HeadersExport{Headers: snapshot.Headers})
	emitJSON("document_ids.json", DocumentIdsExport{
		DocumentIds: snapshot.DocumentIds.DocumentIds,
	})
	emitJSON("platform.json", PlatformExport{Platform: snapshot.Platform})
	emitJSON("agents.json", AgentsExport{Agents: snapshot.Agents})
	emitJSON("paths.json", snapshot.Paths)
	emitJSON("ports.json", PortsExport{Ports: snapshot.Ports})
	emitJSON("env_vars.json", EnvVarsExport{EnvVars: snapshot.EnvVars})
	emitJSON("timestamp.json", TimestampExport{Timestamp: snapshot.Timestamp})
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
	emitJSON("api_paths.json", apiPathsFull)

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
	emitG8eePythonModule := func(filename string, groupName string, entries map[string]constants.Entry) {
		path := filepath.Join(appConstantsDir, filename)
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

	emitG8eePythonModule("generated_collections.py", "collections", snapshot.Collections)
	emitG8eePythonModule("generated_events.py", "events", snapshot.Events)
	emitG8eePythonModule("generated_status.py", "status", flattenedStatus)
	emitG8eePythonModule("generated_headers.py", "headers", snapshot.Headers)
	emitG8eePythonModule("generated_intents.py", "intents", snapshot.Intents)
	emitG8eePythonModule("generated_channels.py", "channels", snapshot.Channels)

	// Emit generated_paths.py for g8ee
	emitG8eePathsPython := func() {
		path := filepath.Join(appConstantsDir, "generated_paths.py")
		lines := []string{
			"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
			"",
			"# Path constants",
		}
		// Sort keys for deterministic output
		infraKeys := make([]string, 0, len(snapshot.Paths.Infra))
		for k := range snapshot.Paths.Infra {
			infraKeys = append(infraKeys, k)
		}
		sort.Strings(infraKeys)
		for _, key := range infraKeys {
			value := snapshot.Paths.Infra[key]
			constName := "PATH_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
			lines = append(lines, fmt.Sprintf("%s = %q", constName, value))
		}
		lines = append(lines, "", "# Port constants")
		portsKeys := make([]string, 0, len(snapshot.Paths.Ports))
		for k := range snapshot.Paths.Ports {
			portsKeys = append(portsKeys, k)
		}
		sort.Strings(portsKeys)
		for _, key := range portsKeys {
			value := snapshot.Paths.Ports[key]
			// Convert lowercase key to uppercase constant name
			// Add G8E_ prefix for g8ee-specific ports
			prefix := "PORT_"
			if strings.HasPrefix(key, "g8ee") {
				prefix = "G8E_PORT_"
			}
			constName := prefix + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
			lines = append(lines, fmt.Sprintf("%s = %d", constName, value))
		}
		lines = append(lines, "", "class PathConstants:")
		for _, key := range infraKeys {
			value := snapshot.Paths.Infra[key]
			constName := "PATH_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
			lines = append(lines, fmt.Sprintf("    %s = %q", constName, value))
		}
		lines = append(lines, "", "class PortConstants:")
		for _, key := range portsKeys {
			value := snapshot.Paths.Ports[key]
			// Convert lowercase key to uppercase constant name
			// Add G8E_ prefix for g8ee-specific ports
			prefix := "PORT_"
			if strings.HasPrefix(key, "g8ee") {
				prefix = "G8E_PORT_"
			}
			constName := prefix + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
			lines = append(lines, fmt.Sprintf("    %s = %d", constName, value))
		}
		lines = append(lines, "")

		content := strings.Join(lines, "\n")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  Wrote %s\n", path)
	}

	emitG8eePathsPython()

	// Emit generated_paths.py for protocol
	emitProtocolPathsPython := func() {
		path := filepath.Join(protocolPythonDir, "generated_paths.py")
		lines := []string{
			"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
			"",
			"# Path constants",
		}
		// Sort keys for deterministic output
		infraKeys := make([]string, 0, len(snapshot.Paths.Infra))
		for k := range snapshot.Paths.Infra {
			infraKeys = append(infraKeys, k)
		}
		sort.Strings(infraKeys)
		for _, key := range infraKeys {
			value := snapshot.Paths.Infra[key]
			constName := "PATH_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
			lines = append(lines, fmt.Sprintf("%s = %q", constName, value))
		}
		lines = append(lines, "", "# Port constants")
		portsKeys := make([]string, 0, len(snapshot.Paths.Ports))
		for k := range snapshot.Paths.Ports {
			portsKeys = append(portsKeys, k)
		}
		sort.Strings(portsKeys)
		for _, key := range portsKeys {
			value := snapshot.Paths.Ports[key]
			// Convert lowercase key to uppercase constant name
			// Add G8E_ prefix for g8ee-specific ports
			prefix := "PORT_"
			if strings.HasPrefix(key, "g8ee") {
				prefix = "G8E_PORT_"
			}
			constName := prefix + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
			lines = append(lines, fmt.Sprintf("%s = %d", constName, value))
		}
		lines = append(lines, "", "class PathConstants:")
		for _, key := range infraKeys {
			constName := "PATH_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
			lines = append(lines, fmt.Sprintf("    %s = %q", constName, snapshot.Paths.Infra[key]))
		}
		lines = append(lines, "", "class PortConstants:")
		for _, key := range portsKeys {
			value := snapshot.Paths.Ports[key]
			prefix := "PORT_"
			if strings.HasPrefix(key, "g8ee") {
				prefix = "G8E_PORT_"
			}
			constName := prefix + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
			lines = append(lines, fmt.Sprintf("    %s = %d", constName, value))
		}
		lines = append(lines, "")

		content := strings.Join(lines, "\n")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  Wrote %s\n", path)
	}

	emitProtocolPathsPython()

	// Emit shell scripts
	emitShell := func(filename string, data map[string]string, prefix string) {
		path := filepath.Join(scriptDir, filename)
		lines := []string{
			"#!/usr/bin/env bash",
			"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
			"",
		}
		// Sort keys for deterministic output
		sortedKeys := make([]string, 0, len(data))
		for k := range data {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)
		for _, k := range sortedKeys {
			value := data[k]
			varName := strings.ToUpper(strings.ReplaceAll(value, "-", "_"))
			lines = append(lines, fmt.Sprintf("export %s%s=%q", prefix, varName, value))
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

	emitShell("env_vars.sh", snapshot.EnvVars, "G8E_ENV_")

	// Emit paths.sh with nested structure
	emitPathsShell := func(filename string) {
		path := filepath.Join(scriptDir, filename)
		lines := []string{
			"#!/usr/bin/env bash",
			"# Code generated by services/g8eo/cmd/exporter. DO NOT EDIT.",
			"",
		}
		// Sort keys for deterministic output
		infraKeys := make([]string, 0, len(snapshot.Paths.Infra))
		for k := range snapshot.Paths.Infra {
			infraKeys = append(infraKeys, k)
		}
		sort.Strings(infraKeys)
		for _, key := range infraKeys {
			value := snapshot.Paths.Infra[key]
			varName := strings.ToUpper(key)
			lines = append(lines, fmt.Sprintf("export G8E_PATH_%s=%q", varName, value))
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

	emitPathsShell("paths.sh")

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

	fmt.Println("Constants exported successfully.")
}
