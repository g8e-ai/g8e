// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build ignore
// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// JSONEntry represents a constant entry from JSON files
type JSONEntry struct {
	Value       string `json:"value"`
	GoConst     string `json:"_go_const"`
	PythonConst string `json:"_python_const"`
}

// JSONFile represents the structure of a constants JSON file
type JSONFile struct {
	Collections map[string]JSONEntry `json:"collections"`
	Events      map[string]JSONEntry `json:"events"`
	Status      map[string]JSONEntry `json:"status"`
	Senders     map[string]JSONEntry `json:"senders"`
	KVKeys      map[string]JSONEntry `json:"kv_keys"`
	Channels    map[string]JSONEntry `json:"channels"`
	PubSub      map[string]JSONEntry `json:"pubsub"`
	Intents     map[string]JSONEntry `json:"intents"`
	Prompts     map[string]JSONEntry `json:"prompts"`
	Headers     map[string]JSONEntry `json:"headers"`
	DocumentIds map[string]JSONEntry `json:"document_ids"`
	Platform    map[string]JSONEntry `json:"platform"`
	Agents      map[string]string    `json:"agents"`
	Paths       map[string]JSONEntry `json:"paths"`
	Ports       map[string]int       `json:"ports"`
	EnvVars     map[string]string    `json:"env_vars"`
	Timestamp   map[string]string    `json:"timestamp"`
	ApiPaths    interface{}          `json:"api_paths"`
}

func main() {
	// Get paths
	constantsDir := "."
	if len(os.Args) > 1 {
		constantsDir = os.Args[1]
	}
	protocolConstantsDir := filepath.Join(constantsDir, "..", "..", "..", "..", "protocol", "constants")
	if len(os.Args) > 2 {
		protocolConstantsDir = os.Args[2]
	}

	// Read all JSON files
	jsonFiles := map[string]string{
		"collections.json":  "Collections",
		"events.json":       "Events",
		"headers.json":      "Headers",
		"channels.json":     "Channels",
		"intents.json":      "Intents",
		"document_ids.json": "DocumentIds",
		"kv_keys.json":      "KVKeys",
		"status.json":       "Status",
		"senders.json":      "Senders",
		"prompts.json":      "Prompts",
		"pubsub.json":       "PubSub",
		"platform.json":     "Platform",
		"agents.json":       "Agents",
		"timestamp.json":    "Timestamp",
		"ports.json":        "Ports",
		"env_vars.json":     "EnvVars",
		"paths.json":        "Paths",
		"api_paths.json":    "ApiPaths",
	}

	var allData JSONFile
	for jsonFile, _ := range jsonFiles {
		filePath := filepath.Join(protocolConstantsDir, jsonFile)
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", jsonFile, err)
			os.Exit(1)
		}

		var fileData JSONFile
		if err := json.Unmarshal(data, &fileData); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", jsonFile, err)
			os.Exit(1)
		}

		// Merge into allData
		if fileData.Collections != nil {
			allData.Collections = fileData.Collections
		}
		if fileData.Events != nil {
			allData.Events = fileData.Events
		}
		if fileData.Status != nil {
			allData.Status = fileData.Status
		}
		if fileData.Senders != nil {
			allData.Senders = fileData.Senders
		}
		if fileData.KVKeys != nil {
			allData.KVKeys = fileData.KVKeys
		}
		if fileData.Channels != nil {
			allData.Channels = fileData.Channels
		}
		if fileData.PubSub != nil {
			allData.PubSub = fileData.PubSub
		}
		if fileData.Intents != nil {
			allData.Intents = fileData.Intents
		}
		if fileData.Prompts != nil {
			allData.Prompts = fileData.Prompts
		}
		if fileData.Headers != nil {
			allData.Headers = fileData.Headers
		}
		if fileData.DocumentIds != nil {
			allData.DocumentIds = fileData.DocumentIds
		}
		if fileData.Platform != nil {
			allData.Platform = fileData.Platform
		}
		if fileData.Agents != nil {
			allData.Agents = fileData.Agents
		}
		if fileData.Paths != nil {
			allData.Paths = fileData.Paths
		}
		if fileData.Ports != nil {
			allData.Ports = fileData.Ports
		}
		if fileData.EnvVars != nil {
			allData.EnvVars = fileData.EnvVars
		}
		if fileData.Timestamp != nil {
			allData.Timestamp = fileData.Timestamp
		}
		if fileData.ApiPaths != nil {
			allData.ApiPaths = fileData.ApiPaths
		}
	}

	// Generate registry.go content
	output := generateRegistry(allData)

	// Write to registry.go
	registryPath := filepath.Join(constantsDir, "registry.go")
	if err := os.WriteFile(registryPath, []byte(output), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing registry.go: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated registry.go from JSON source files\n")
}

func generateRegistry(data JSONFile) string {
	var sb strings.Builder

	sb.WriteString("// Copyright (c) 2026 Lateralus Labs, LLC.\n")
	sb.WriteString("// Licensed under the Apache License, Version 2.0 (the \"License\");\n")
	sb.WriteString("// you may not use this file except in compliance with the License.\n")
	sb.WriteString("// You may obtain a copy of the License at\n")
	sb.WriteString("//\n")
	sb.WriteString("//     http://www.apache.org/licenses/LICENSE-2.0\n")
	sb.WriteString("//\n")
	sb.WriteString("// Unless required by applicable law or agreed to in writing, software\n")
	sb.WriteString("// distributed under the License is distributed on an \"AS IS\" BASIS,\n")
	sb.WriteString("// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.\n")
	sb.WriteString("// See the License for the specific language governing permissions and\n")
	sb.WriteString("// limitations under the License.\n")
	sb.WriteString("\n")
	sb.WriteString("package constants\n")
	sb.WriteString("\n")
	sb.WriteString("// Code generated by generate_registry.go. DO NOT EDIT.\n")
	sb.WriteString("// Source: protocol/constants/*.json\n")
	sb.WriteString("// To regenerate: go run ./internal/constants/generate_registry.go\n")
	sb.WriteString("\n")
	sb.WriteString("// Entry represents a constant with its value and naming metadata.\n")
	sb.WriteString("type Entry struct {\n")
	sb.WriteString("\tValue       string `json:\"value\"`\n")
	sb.WriteString("\tGoConst     string `json:\"_go_const\"`\n")
	sb.WriteString("\tPythonConst string `json:\"_python_const\"`\n")
	sb.WriteString("}\n")
	sb.WriteString("\n")
	sb.WriteString("// KVKeysSnapshot represents the nested structure for KV keys.\n")
	sb.WriteString("type KVKeysSnapshot struct {\n")
	sb.WriteString("\tCachePrefix string            `json:\"cache.prefix\"`\n")
	sb.WriteString("\tKeySchema   map[string]string `json:\"key.schema\"`\n")
	sb.WriteString("\tSessionType map[string]string `json:\"session.type\"`\n")
	sb.WriteString("}\n")
	sb.WriteString("\n")
	sb.WriteString("// PathsSnapshot represents the nested structure for paths.\n")
	sb.WriteString("type PathsSnapshot struct {\n")
	sb.WriteString("\tInfra map[string]string `json:\"infra\"`\n")
	sb.WriteString("\tG8ee  map[string]string `json:\"g8ee\"`\n")
	sb.WriteString("\tPorts map[string]int    `json:\"ports\"`\n")
	sb.WriteString("\tHost  string            `json:\"host\"`\n")
	sb.WriteString("}\n")
	sb.WriteString("\n")
	sb.WriteString("// DocumentIdsSnapshot represents the nested structure for document IDs.\n")
	sb.WriteString("type DocumentIdsSnapshot struct {\n")
	sb.WriteString("\tDocumentIds map[string]Entry `json:\"document_ids\"`\n")
	sb.WriteString("}\n")
	sb.WriteString("\n")
	sb.WriteString("// StatusSnapshot represents the nested structure for status values.\n")
	sb.WriteString("type StatusSnapshot struct {\n")
	sb.WriteString("\tAttachmentType    map[string]Entry `json:\"attachment.type\"`\n")
	sb.WriteString("\tUserRole          map[string]Entry `json:\"user_role\"`\n")
	sb.WriteString("\tUserStatus        map[string]Entry `json:\"user_status\"`\n")
	sb.WriteString("\tOperatorStatus    map[string]Entry `json:\"operator_status\"`\n")
	sb.WriteString("\tExecutionStatus   map[string]Entry `json:\"execution_status\"`\n")
	sb.WriteString("\tTribunalOutcome   map[string]Entry `json:\"tribunal.outcome\"`\n")
	sb.WriteString("\tApprovalErrorType map[string]Entry `json:\"approval.error.type\"`\n")
	sb.WriteString("\tLlmModels         map[string]Entry `json:\"llm.models\"`\n")
	sb.WriteString("}\n")
	sb.WriteString("\n")
	sb.WriteString("// Snapshot is the complete constants registry snapshot.\n")
	sb.WriteString("type Snapshot struct {\n")
	sb.WriteString("\tCollections map[string]Entry    `json:\"collections\"`\n")
	sb.WriteString("\tEvents      map[string]Entry    `json:\"events\"`\n")
	sb.WriteString("\tStatus      StatusSnapshot      `json:\"status\"`\n")
	sb.WriteString("\tSenders     map[string]Entry    `json:\"senders\"`\n")
	sb.WriteString("\tKVKeys      KVKeysSnapshot      `json:\"kv_keys\"`\n")
	sb.WriteString("\tChannels    map[string]Entry    `json:\"channels\"`\n")
	sb.WriteString("\tPubSub      map[string]Entry    `json:\"pubsub\"`\n")
	sb.WriteString("\tIntents     map[string]Entry    `json:\"intents\"`\n")
	sb.WriteString("\tPrompts     map[string]Entry    `json:\"prompts\"`\n")
	sb.WriteString("\tHeaders     map[string]Entry    `json:\"headers\"`\n")
	sb.WriteString("\tDocumentIds DocumentIdsSnapshot `json:\"document_ids\"`\n")
	sb.WriteString("\tPlatform    map[string]Entry    `json:\"platform\"`\n")
	sb.WriteString("\tAgents      map[string]string   `json:\"agents\"`\n")
	sb.WriteString("\tPaths       PathsSnapshot       `json:\"paths\"`\n")
	sb.WriteString("\tPorts       map[string]int      `json:\"ports\"`\n")
	sb.WriteString("\tEnvVars     map[string]string   `json:\"env_vars\"`\n")
	sb.WriteString("\tTimestamp   map[string]string   `json:\"timestamp\"`\n")
	sb.WriteString("\tApiPaths    interface{}         `json:\"api_paths\"`\n")
	sb.WriteString("}\n")
	sb.WriteString("\n")
	sb.WriteString("// Registry returns the complete constants snapshot.\n")
	sb.WriteString("func Registry() Snapshot {\n")
	sb.WriteString("\treturn Snapshot{\n")

	// Collections
	if data.Collections != nil {
		sb.WriteString("\t\tCollections: map[string]Entry{\n")
		keys := sortedKeys(data.Collections)
		for _, key := range keys {
			entry := data.Collections[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
				key, entry.Value, entry.GoConst, entry.PythonConst))
		}
		sb.WriteString("\t\t},\n")
	}

	// Channels
	if data.Channels != nil {
		sb.WriteString("\t\tChannels: map[string]Entry{\n")
		keys := sortedKeys(data.Channels)
		for _, key := range keys {
			entry := data.Channels[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
				key, entry.Value, entry.GoConst, entry.PythonConst))
		}
		sb.WriteString("\t\t},\n")
	}

	// DocumentIds
	if data.DocumentIds != nil {
		sb.WriteString("\t\tDocumentIds: DocumentIdsSnapshot{\n")
		sb.WriteString("\t\t\tDocumentIds: map[string]Entry{\n")
		keys := sortedKeys(data.DocumentIds)
		for _, key := range keys {
			entry := data.DocumentIds[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
				key, entry.Value, entry.GoConst, entry.PythonConst))
		}
		sb.WriteString("\t\t\t},\n")
		sb.WriteString("\t\t},\n")
	}

	// Headers
	if data.Headers != nil {
		sb.WriteString("\t\tHeaders: map[string]Entry{\n")
		keys := sortedKeys(data.Headers)
		for _, key := range keys {
			entry := data.Headers[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
				key, entry.Value, entry.GoConst, entry.PythonConst))
		}
		sb.WriteString("\t\t},\n")
	}

	// Intents
	if data.Intents != nil {
		sb.WriteString("\t\tIntents: map[string]Entry{\n")
		keys := sortedKeys(data.Intents)
		for _, key := range keys {
			entry := data.Intents[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
				key, entry.Value, entry.GoConst, entry.PythonConst))
		}
		sb.WriteString("\t\t},\n")
	}

	// KVKeys (special handling for nested structure)
	if data.KVKeys != nil {
		sb.WriteString("\t\tKVKeys: KVKeysSnapshot{\n")
		// Extract cache prefix if present
		cachePrefix := ""
		keySchema := make(map[string]string)
		sessionType := make(map[string]string)
		for key, entry := range data.KVKeys {
			if key == "cache.prefix" {
				cachePrefix = entry.Value
			} else if strings.HasPrefix(key, "key.schema.") {
				keySchema[strings.TrimPrefix(key, "key.schema.")] = entry.Value
			} else if strings.HasPrefix(key, "session.type.") {
				sessionType[strings.TrimPrefix(key, "session.type.")] = entry.Value
			}
		}
		sb.WriteString(fmt.Sprintf("\t\t\tCachePrefix: \"%s\",\n", cachePrefix))
		sb.WriteString("\t\t\tKeySchema: map[string]string{\n")
		for key, val := range keySchema {
			sb.WriteString(fmt.Sprintf("\t\t\t\t\"%s\": \"%s\",\n", key, val))
		}
		sb.WriteString("\t\t\t},\n")
		sb.WriteString("\t\t\tSessionType: map[string]string{\n")
		for key, val := range sessionType {
			sb.WriteString(fmt.Sprintf("\t\t\t\t\"%s\": \"%s\",\n", key, val))
		}
		sb.WriteString("\t\t\t},\n")
		sb.WriteString("\t\t},\n")
	}

	// Paths (special handling)
	sb.WriteString("\t\tPaths: PathsSnapshot{\n")
	sb.WriteString("\t\t\tInfra: map[string]string{\n")
	sb.WriteString("\t\t\t\t\"db_path\": Paths.Infra.DbPath,\n")
	sb.WriteString("\t\t\t\t\"pki_dir\": Paths.Infra.PkiDir,\n")
	sb.WriteString("\t\t\t\t\"secrets_dir\": Paths.Infra.SecretsDir,\n")
	sb.WriteString("\t\t\t\t\"ca_cert_path\": Paths.Infra.CaCertPath,\n")
	sb.WriteString("\t\t\t\t\"app_cert_dir\": Paths.Infra.AppCertDir,\n")
	sb.WriteString("\t\t\t\t\"docs_dir\": Paths.Infra.DocsDir,\n")
	sb.WriteString("\t\t\t\t\"protocol_dir\": Paths.Infra.ProtocolDir,\n")
	sb.WriteString("\t\t\t\t\"protocol_constants_dir\": Paths.Infra.ProtocolConstantsDir,\n")
	sb.WriteString("\t\t\t\t\"protocol_models_dir\": Paths.Infra.ProtocolModelsDir,\n")
	sb.WriteString("\t\t\t\t\"ssh_config_path\": Paths.Infra.SshConfigPath,\n")
	sb.WriteString("\t\t\t},\n")
	sb.WriteString("\t\t\tG8ee: map[string]string{\n")
	sb.WriteString("\t\t\t\t\"app_dir\": \"/app/services/g8ee\",\n")
	sb.WriteString("\t\t\t\t\"config_dir\": \"/app/services/g8ee/config\",\n")
	sb.WriteString("\t\t\t\t\"tests_dir\": \"/app/services/g8ee/tests\",\n")
	sb.WriteString("\t\t\t\t\"cert_name\": \"g8ee\",\n")
	sb.WriteString("\t\t\t},\n")
	sb.WriteString("\t\t\tPorts: map[string]int{\n")
	sb.WriteString("\t\t\t\t\"operator_https\": Ports.OperatorHttps,\n")
	sb.WriteString("\t\t\t\t\"operator_bootstrap_https\": Ports.OperatorBootstrapHttps,\n")
	sb.WriteString("\t\t\t\t\"operator_public_https\": Ports.OperatorPublicHttps,\n")
	sb.WriteString("\t\t\t\t\"g8ee_https\": Ports.G8eeHttps,\n")
	sb.WriteString("\t\t\t\t\"openclaw_gateway\": Ports.OpenclawGateway,\n")
	sb.WriteString("\t\t\t},\n")
	sb.WriteString("\t\t\tHost: \"localhost\",\n")
	sb.WriteString("\t\t},\n")

	// Ports (special handling for snake_case keys with PascalCase field names)
	sb.WriteString("\t\tPorts: map[string]int{\n")
	sb.WriteString("\t\t\t\"operator_https\": Ports.OperatorHttps,\n")
	sb.WriteString("\t\t\t\"operator_bootstrap_https\": Ports.OperatorBootstrapHttps,\n")
	sb.WriteString("\t\t\t\"operator_public_https\": Ports.OperatorPublicHttps,\n")
	sb.WriteString("\t\t\t\"g8ee_https\": Ports.G8eeHttps,\n")
	sb.WriteString("\t\t\t\"openclaw_gateway\": Ports.OpenclawGateway,\n")
	sb.WriteString("\t\t},\n")

	// EnvVars
	if data.EnvVars != nil {
		sb.WriteString("\t\tEnvVars: map[string]string{\n")
		keys := sortedStringKeys(data.EnvVars)
		for _, key := range keys {
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": string(EnvVar.%s),\n", key, key))
		}
		sb.WriteString("\t\t},\n")
	}

	// Events
	if data.Events != nil {
		sb.WriteString("\t\t// Events and Status are large files; emit flat maps for now since Python models use extra=\"allow\"\n")
		sb.WriteString("\t\t// These will be refined to full Entry structures in a follow-up\n")
		sb.WriteString("\t\tEvents: map[string]Entry{\n")
		keys := sortedKeys(data.Events)
		for _, key := range keys {
			entry := data.Events[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
				key, entry.Value, entry.GoConst, entry.PythonConst))
		}
		sb.WriteString("\t\t},\n")
	}

	// Status (nested structure)
	if data.Status != nil {
		sb.WriteString("\t\tStatus: StatusSnapshot{\n")
		// Group status by type based on key prefixes
		groups := groupStatusKeys(data.Status)
		for groupType, entries := range groups {
			sb.WriteString(fmt.Sprintf("\t\t\t%s: map[string]Entry{\n", groupType))
			keys := sortedKeys(entries)
			for _, key := range keys {
				entry := entries[key]
				sb.WriteString(fmt.Sprintf("\t\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
					key, entry.Value, entry.GoConst, entry.PythonConst))
			}
			sb.WriteString("\t\t\t},\n")
		}
		sb.WriteString("\t\t},\n")
	}

	// Senders
	if data.Senders != nil {
		sb.WriteString("\t\tSenders: map[string]Entry{\n")
		keys := sortedKeys(data.Senders)
		for _, key := range keys {
			entry := data.Senders[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
				key, entry.Value, entry.GoConst, entry.PythonConst))
		}
		sb.WriteString("\t\t},\n")
	}

	// Prompts
	if data.Prompts != nil {
		sb.WriteString("\t\tPrompts: map[string]Entry{\n")
		keys := sortedKeys(data.Prompts)
		for _, key := range keys {
			entry := data.Prompts[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
				key, entry.Value, entry.GoConst, entry.PythonConst))
		}
		sb.WriteString("\t\t},\n")
	}

	// PubSub
	if data.PubSub != nil {
		sb.WriteString("\t\tPubSub: map[string]Entry{\n")
		keys := sortedKeys(data.PubSub)
		for _, key := range keys {
			entry := data.PubSub[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
				key, entry.Value, entry.GoConst, entry.PythonConst))
		}
		sb.WriteString("\t\t},\n")
	}

	// Platform
	if data.Platform != nil {
		sb.WriteString("\t\tPlatform: map[string]Entry{\n")
		keys := sortedKeys(data.Platform)
		for _, key := range keys {
			entry := data.Platform[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
				key, entry.Value, entry.GoConst, entry.PythonConst))
		}
		sb.WriteString("\t\t},\n")
	}

	// Agents
	if data.Agents != nil {
		sb.WriteString("\t\tAgents: map[string]string{\n")
		keys := sortedStringKeys(data.Agents)
		for _, key := range keys {
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": %s,\n", key, key))
		}
		sb.WriteString("\t\t},\n")
	}

	// Timestamp
	if data.Timestamp != nil {
		sb.WriteString("\t\tTimestamp: map[string]string{\n")
		keys := sortedStringKeys(data.Timestamp)
		for _, key := range keys {
			// Map JSON key to Go constant name
			goConst := key
			if key == "FormatRFC3339" {
				goConst = "TimestampFormatRFC3339"
			}
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": %s,\n", key, goConst))
		}
		sb.WriteString("\t\t},\n")
	}

	// ApiPaths (placeholder for now)
	sb.WriteString("\t\tApiPaths: nil,\n")

	sb.WriteString("\t}\n")
	sb.WriteString("}\n")

	return sb.String()
}

func sortedKeys(m map[string]JSONEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedIntKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func groupStatusKeys(status map[string]JSONEntry) map[string]map[string]JSONEntry {
	groups := make(map[string]map[string]JSONEntry)
	for key, entry := range status {
		var group string
		if strings.HasPrefix(key, "attachment.type.") {
			group = "AttachmentType"
			key = strings.TrimPrefix(key, "attachment.type.")
		} else if strings.HasPrefix(key, "user_role.") {
			group = "UserRole"
			key = strings.TrimPrefix(key, "user_role.")
		} else if strings.HasPrefix(key, "user_status.") {
			group = "UserStatus"
			key = strings.TrimPrefix(key, "user_status.")
		} else if strings.HasPrefix(key, "operator_status.") {
			group = "OperatorStatus"
			key = strings.TrimPrefix(key, "operator_status.")
		} else if strings.HasPrefix(key, "execution_status.") {
			group = "ExecutionStatus"
			key = strings.TrimPrefix(key, "execution_status.")
		} else if strings.HasPrefix(key, "tribunal.outcome.") {
			group = "TribunalOutcome"
			key = strings.TrimPrefix(key, "tribunal.outcome.")
		} else if strings.HasPrefix(key, "approval.error.type.") {
			group = "ApprovalErrorType"
			key = strings.TrimPrefix(key, "approval.error.type.")
		} else if strings.HasPrefix(key, "llm.models.") {
			group = "LlmModels"
			key = strings.TrimPrefix(key, "llm.models.")
		} else {
			group = "Other"
		}

		if groups[group] == nil {
			groups[group] = make(map[string]JSONEntry)
		}
		groups[group][key] = entry
	}
	return groups
}
