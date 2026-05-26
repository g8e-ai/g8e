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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// JSONEntry represents a constant entry from JSON files
type JSONEntry struct {
	Value       interface{} `json:"value"`
	GoConst     string      `json:"_go_const"`
	PythonConst string      `json:"_python_const"`
	Mutation    bool        `json:"_mutation"`
}

// NestedStatusEntry represents the nested structure of status.json
type NestedStatusEntry struct {
	Value       string `json:"value"`
	GoConst     string `json:"_go_const"`
	PythonConst string `json:"_python_const"`
}

// JSONFile represents the structure of a constants JSON file
type JSONFile struct {
	Collections map[string]JSONEntry            `json:"collections"`
	Events      map[string]JSONEntry            `json:"events"`
	Status      map[string]map[string]JSONEntry `json:"status"`
	Senders     map[string]JSONEntry            `json:"senders"`
	KVKeys      map[string]JSONEntry            `json:"kv_keys"`
	Channels    map[string]JSONEntry            `json:"channels"`
	PubSub      map[string]JSONEntry            `json:"pubsub"`
	Intents     map[string]JSONEntry            `json:"intents"`
	Prompts     map[string]JSONEntry            `json:"prompts"`
	Headers     map[string]JSONEntry            `json:"headers"`
	DocumentIds map[string]JSONEntry            `json:"document_ids"`
	Platform    map[string]JSONEntry            `json:"platform"`
	Agents      map[string]string               `json:"agents"`
	Timestamp   map[string]string               `json:"timestamp"`
	APIPaths    interface{}                     `json:"api_paths"`
}

func main() {
	// Get paths
	constantsDir := "."
	if len(os.Args) > 1 {
		constantsDir = os.Args[1]
	}
	protocolConstantsDir := filepath.Join(constantsDir, "..", "..", "protocol", "constants")
	if len(os.Args) > 2 {
		protocolConstantsDir = os.Args[2]
	}

	// Read all JSON files
	// Note: paths.json, ports.json, env_vars.json, and api_paths.json are internal-only
	// constants not exported to the registry snapshot (see check_registry.go)
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
	}

	var allData JSONFile
	for jsonFile := range jsonFiles {
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
		if fileData.Timestamp != nil {
			allData.Timestamp = fileData.Timestamp
		}
		if fileData.APIPaths != nil {
			allData.APIPaths = fileData.APIPaths
		}
	}

	// Validate that all required fields are present
	if err := validateRequiredFields(allData); err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
		os.Exit(1)
	}

	// Generate registry.go content
	output := generateRegistry(allData)

	// Write to registry.go
	registryPath := filepath.Join(constantsDir, "registry.go")
	if err := os.WriteFile(registryPath, []byte(output), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing registry.go: %v\n", err)
		os.Exit(1)
	}
	if err := gofmtFile(registryPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting registry.go: %v\n", err)
		os.Exit(1)
	}

	// Generate status.go content from status.json
	statusOutput := generateStatusConstants(allData.Status)

	// Write to status_generated.go
	statusPath := filepath.Join(constantsDir, "status_generated.go")
	if err := os.WriteFile(statusPath, []byte(statusOutput), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing status_generated.go: %v\n", err)
		os.Exit(1)
	}
	if err := gofmtFile(statusPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting status_generated.go: %v\n", err)
		os.Exit(1)
	}

	// Generate headers.go content from headers.json
	headersOutput := generateHeaderConstants(allData.Headers)

	// Write to headers_generated.go
	headersPath := filepath.Join(constantsDir, "headers_generated.go")
	if err := os.WriteFile(headersPath, []byte(headersOutput), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing headers_generated.go: %v\n", err)
		os.Exit(1)
	}
	if err := gofmtFile(headersPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting headers_generated.go: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated registry.go, status_generated.go, and headers_generated.go from JSON source files\n")
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
	sb.WriteString("// DocumentIdsSnapshot represents the nested structure for document IDs.\n")
	sb.WriteString("type DocumentIdsSnapshot struct {\n")
	sb.WriteString("\tDocumentIds map[string]Entry `json:\"document_ids\"`\n")
	sb.WriteString("}\n")
	sb.WriteString("\n")
	sb.WriteString("// StatusSnapshot represents the nested structure for status values.\n")
	sb.WriteString("// This is a dynamic map to support all status categories without hardcoding.\n")
	sb.WriteString("type StatusSnapshot map[string]map[string]Entry\n")
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
	sb.WriteString("\tTimestamp   map[string]string   `json:\"timestamp\"`\n")
	sb.WriteString("\tAPIPaths    interface{}         `json:\"api_paths\"`\n")
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
				key, valueToString(entry.Value), entry.GoConst, entry.PythonConst))
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
				key, valueToString(entry.Value), entry.GoConst, entry.PythonConst))
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
				key, valueToString(entry.Value), entry.GoConst, entry.PythonConst))
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
				key, valueToString(entry.Value), entry.GoConst, entry.PythonConst))
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
				key, valueToString(entry.Value), entry.GoConst, entry.PythonConst))
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
		keys := sortedKeys(data.KVKeys)
		for _, key := range keys {
			entry := data.KVKeys[key]
			if key == "cache.prefix" {
				cachePrefix = valueToString(entry.Value)
			} else if strings.HasPrefix(key, "key.schema.") {
				keySchema[strings.TrimPrefix(key, "key.schema.")] = valueToString(entry.Value)
			} else if strings.HasPrefix(key, "session.type.") {
				sessionType[strings.TrimPrefix(key, "session.type.")] = valueToString(entry.Value)
			}
		}
		sb.WriteString(fmt.Sprintf("\t\t\tCachePrefix: \"%s\",\n", cachePrefix))
		sb.WriteString("\t\t\tKeySchema: map[string]string{\n")
		keySchemaKeys := make([]string, 0, len(keySchema))
		for key := range keySchema {
			keySchemaKeys = append(keySchemaKeys, key)
		}
		sort.Strings(keySchemaKeys)
		for _, key := range keySchemaKeys {
			val := keySchema[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\t\"%s\": \"%s\",\n", key, val))
		}
		sb.WriteString("\t\t\t},\n")
		sb.WriteString("\t\t\tSessionType: map[string]string{\n")
		sessionTypeKeys := make([]string, 0, len(sessionType))
		for key := range sessionType {
			sessionTypeKeys = append(sessionTypeKeys, key)
		}
		sort.Strings(sessionTypeKeys)
		for _, key := range sessionTypeKeys {
			val := sessionType[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\t\"%s\": \"%s\",\n", key, val))
		}
		sb.WriteString("\t\t\t},\n")
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

	// Status (nested structure - dynamic map)
	if data.Status != nil {
		sb.WriteString("\t\tStatus: StatusSnapshot{\n")
		categories := make([]string, 0, len(data.Status))
		for category := range data.Status {
			categories = append(categories, category)
		}
		sort.Strings(categories)
		for _, category := range categories {
			entries := data.Status[category]
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": map[string]Entry{\n", category))
			keys := sortedKeys(entries)
			for _, key := range keys {
				entry := entries[key]
				sb.WriteString(fmt.Sprintf("\t\t\t\t\"%s\": {Value: \"%s\", GoConst: \"%s\", PythonConst: \"%s\"},\n",
					key, valueToString(entry.Value), entry.GoConst, entry.PythonConst))
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
			value := data.Agents[key]
			sb.WriteString(fmt.Sprintf("\t\t\t\"%s\": %s,\n", key, formatGoValue(value)))
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

	// APIPaths (placeholder for now)
	sb.WriteString("\t\tAPIPaths: nil,\n")

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

func generateStatusConstants(status map[string]map[string]JSONEntry) string {
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
	sb.WriteString("import \"fmt\"\n")
	sb.WriteString("\n")
	sb.WriteString("// Code generated by generate_registry.go. DO NOT EDIT.\n")
	sb.WriteString("// Source: protocol/constants/status.json\n")
	sb.WriteString("// To regenerate: go run ./internal/constants/generate_registry.go\n")
	sb.WriteString("\n")

	// Group constants by their type prefix to emit type definitions
	// Type names are derived from the JSON category name (e.g., "user_role" -> "UserRole")
	typeGroups := make(map[string][]struct {
		constName string
		value     string
		category  string
		isNumeric bool
	})

	// Sort categories for deterministic iteration
	categories := make([]string, 0, len(status))
	for category := range status {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	for _, category := range categories {
		entries := status[category]
		// Convert category name to PascalCase type name (e.g., "user_role" -> "UserRole")
		typeName := categoryToTypeName(category)
		for _, entry := range entries {
			if entry.GoConst == "" {
				continue
			}
			// Check if the value is numeric
			isNumeric := isNumericValue(entry.Value)
			typeGroups[typeName] = append(typeGroups[typeName], struct {
				constName string
				value     string
				category  string
				isNumeric bool
			}{
				constName: entry.GoConst,
				value:     valueToString(entry.Value),
				category:  category,
				isNumeric: isNumeric,
			})
		}
	}

	// Emit type definitions and constants in sorted order for determinism
	typeNames := make([]string, 0, len(typeGroups))
	for typeName := range typeGroups {
		typeNames = append(typeNames, typeName)
	}
	sort.Strings(typeNames)

	for _, typeName := range typeNames {
		consts := typeGroups[typeName]
		// Sort constants by constName for determinism
		sort.Slice(consts, func(i, j int) bool {
			return consts[i].constName < consts[j].constName
		})

		// Determine if this is a numeric type based on the first constant
		typeKind := "string"
		if len(consts) > 0 && consts[0].isNumeric {
			typeKind = "int"
		}

		// Emit type definition
		sb.WriteString(fmt.Sprintf("// %s is a typed %s for %s.\n", typeName, typeKind, toSnakeCase(typeName)))
		sb.WriteString(fmt.Sprintf("type %s %s\n\n", typeName, typeKind))

		// Emit const block
		sb.WriteString("const (\n")
		for _, c := range consts {
			if c.isNumeric {
				sb.WriteString(fmt.Sprintf("\t%s %s = %s\n", c.constName, typeName, c.value))
			} else {
				sb.WriteString(fmt.Sprintf("\t%s %s = %s\n", c.constName, typeName, formatGoValue(c.value)))
			}
		}
		sb.WriteString(")\n\n")
	}

	// Emit AllActionTypes() and ValidateAllActionTypes() for action_type category
	if actionTypes, ok := status["action_type"]; ok {
		sb.WriteString("// AllActionTypes returns the complete list of defined action types.\n")
		sb.WriteString("// This is the single source of truth for valid action types in the system.\n")
		sb.WriteString("// Any new action type must be added to the constants above and will automatically\n")
		sb.WriteString("// be included in this list.\n")
		sb.WriteString("func AllActionTypes() []ActionType {\n")
		sb.WriteString("\treturn []ActionType{\n")
		keys := sortedKeys(actionTypes)
		for _, key := range keys {
			entry := actionTypes[key]
			if entry.GoConst != "" {
				sb.WriteString(fmt.Sprintf("\t\t%s,\n", entry.GoConst))
			}
		}
		sb.WriteString("\t}\n")
		sb.WriteString("}\n\n")

		sb.WriteString("// ValidateAllActionTypes checks that AllActionTypes() includes all defined ActionType constants.\n")
		sb.WriteString("// This is a compile-time invariant check to prevent action type drift.\n")
		sb.WriteString("func ValidateAllActionTypes() error {\n")
		sb.WriteString("\tallTypes := AllActionTypes()\n")
		sb.WriteString("\ttypeMap := make(map[ActionType]bool)\n")
		sb.WriteString("\tfor _, t := range allTypes {\n")
		sb.WriteString("\t\ttypeMap[t] = true\n")
		sb.WriteString("\t}\n")
		sb.WriteString("\n")
		sb.WriteString("\t// All defined constants must be in the list\n")
		sb.WriteString("\trequiredTypes := []ActionType{\n")
		for _, key := range keys {
			entry := actionTypes[key]
			if entry.GoConst != "" {
				sb.WriteString(fmt.Sprintf("\t\t%s,\n", entry.GoConst))
			}
		}
		sb.WriteString("\t}\n")
		sb.WriteString("\n")
		sb.WriteString("\tfor _, t := range requiredTypes {\n")
		sb.WriteString("\t\tif !typeMap[t] {\n")
		sb.WriteString("\t\t\treturn fmt.Errorf(\"action type %s is missing from AllActionTypes()\", t)\n")
		sb.WriteString("\t\t}\n")
		sb.WriteString("\t}\n")
		sb.WriteString("\n")
		sb.WriteString("\treturn nil\n")
		sb.WriteString("}\n\n")

		sb.WriteString("// IsMutation returns true if the action type modifies system state.\n")
		sb.WriteString("// This is a strongly-typed intrinsic property of the action definition.\n")
		sb.WriteString("// Mutation classification is defined in protocol/constants/status.json via the _mutation field.\n")
		sb.WriteString("// Actions marked as mutations require L3 Notary (human-presence) verification.\n")
		sb.WriteString("func IsMutation(actionType ActionType) bool {\n")
		sb.WriteString("\tswitch actionType {\n")
		for _, key := range keys {
			entry := actionTypes[key]
			if entry.GoConst != "" && entry.Mutation {
				sb.WriteString(fmt.Sprintf("\tcase %s:\n", entry.GoConst))
				sb.WriteString("\t\treturn true\n")
			}
		}
		sb.WriteString("\tdefault:\n")
		sb.WriteString("\t\treturn false\n")
		sb.WriteString("\t}\n")
		sb.WriteString("}\n\n")

		sb.WriteString("// init validates action type SSOT at package load time.\n")
		sb.WriteString("func init() {\n")
		sb.WriteString("\tif err := ValidateAllActionTypes(); err != nil {\n")
		sb.WriteString("\t\tpanic(fmt.Sprintf(\"action type SSOT validation failed: %v\", err))\n")
		sb.WriteString("\t}\n")
		sb.WriteString("}\n")
	}

	return sb.String()
}

func generateHeaderConstants(headers map[string]JSONEntry) string {
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
	sb.WriteString("// Source: protocol/constants/headers.json\n")
	sb.WriteString("// To regenerate: go run ./internal/constants/generate_registry.go\n")
	sb.WriteString("\n")

	// Emit const block for headers
	sb.WriteString("const (\n")
	keys := sortedKeys(headers)
	for _, key := range keys {
		entry := headers[key]
		if entry.GoConst != "" {
			sb.WriteString(fmt.Sprintf("\t%s = %s // #nosec G101 - constant string, not credential\n", entry.GoConst, formatGoValue(valueToString(entry.Value))))
		}
	}
	sb.WriteString(")\n")

	return sb.String()
}

func categoryToTypeName(category string) string {
	// Convert snake_case category name to PascalCase type name
	// e.g., "user_role" -> "UserRole", "approval.error.type" -> "ApprovalErrorType"
	parts := strings.Split(category, ".")
	var result []rune
	for _, part := range parts {
		if part == "" {
			continue
		}
		// Convert snake_case to PascalCase within each part
		// e.g., "user_role" -> "UserRole"
		subParts := strings.Split(part, "_")
		for _, subPart := range subParts {
			if subPart == "" {
				continue
			}
			// Capitalize first letter of each sub-part
			result = append(result, []rune(strings.ToUpper(string(subPart[0])))...)
			result = append(result, []rune(subPart[1:])...)
		}
	}
	return string(result)
}

func isNumericValue(v interface{}) bool {
	switch val := v.(type) {
	case int, int64, float64:
		return true
	case string:
		// Try to parse as int
		if _, err := fmt.Sscanf(val, "%d", new(int)); err == nil {
			return true
		}
		// Try to parse as float
		if _, err := fmt.Sscanf(val, "%f", new(float64)); err == nil {
			return true
		}
		return false
	default:
		return false
	}
}

func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

func valueToString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int, int64, float64:
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func formatGoValue(v string) string {
	// If the value is a number, return it without quotes
	// If it's a string, return it with quotes
	if v == "" {
		return `""`
	}
	// Check if it's a numeric string
	if _, err := fmt.Sscanf(v, "%d", new(int)); err == nil {
		return v
	}
	if _, err := fmt.Sscanf(v, "%f", new(float64)); err == nil {
		return v
	}
	return fmt.Sprintf(`"%s"`, v)
}

func validateRequiredFields(data JSONFile) error {
	// Validate Collections
	if data.Collections != nil {
		for key, entry := range data.Collections {
			if entry.GoConst == "" {
				return fmt.Errorf("collections.%s: missing required field _go_const", key)
			}
			if entry.PythonConst == "" {
				return fmt.Errorf("collections.%s: missing required field _python_const", key)
			}
		}
	}

	// Validate Events
	if data.Events != nil {
		for key, entry := range data.Events {
			if entry.GoConst == "" {
				return fmt.Errorf("events.%s: missing required field _go_const", key)
			}
			if entry.PythonConst == "" {
				return fmt.Errorf("events.%s: missing required field _python_const", key)
			}
		}
	}

	// Validate Status (nested structure)
	if data.Status != nil {
		for category, entries := range data.Status {
			for key, entry := range entries {
				if entry.GoConst == "" {
					return fmt.Errorf("status.%s.%s: missing required field _go_const", category, key)
				}
				if entry.PythonConst == "" {
					return fmt.Errorf("status.%s.%s: missing required field _python_const", category, key)
				}
			}
		}
	}

	// Validate Senders
	if data.Senders != nil {
		for key, entry := range data.Senders {
			if entry.GoConst == "" {
				return fmt.Errorf("senders.%s: missing required field _go_const", key)
			}
			if entry.PythonConst == "" {
				return fmt.Errorf("senders.%s: missing required field _python_const", key)
			}
		}
	}

	// Validate Channels
	if data.Channels != nil {
		for key, entry := range data.Channels {
			if entry.GoConst == "" {
				return fmt.Errorf("channels.%s: missing required field _go_const", key)
			}
			if entry.PythonConst == "" {
				return fmt.Errorf("channels.%s: missing required field _python_const", key)
			}
		}
	}

	// Validate Intents
	if data.Intents != nil {
		for key, entry := range data.Intents {
			if entry.GoConst == "" {
				return fmt.Errorf("intents.%s: missing required field _go_const", key)
			}
			if entry.PythonConst == "" {
				return fmt.Errorf("intents.%s: missing required field _python_const", key)
			}
		}
	}

	// Validate Prompts
	if data.Prompts != nil {
		for key, entry := range data.Prompts {
			if entry.GoConst == "" {
				return fmt.Errorf("prompts.%s: missing required field _go_const", key)
			}
			if entry.PythonConst == "" {
				return fmt.Errorf("prompts.%s: missing required field _python_const", key)
			}
		}
	}

	// Validate Headers
	if data.Headers != nil {
		for key, entry := range data.Headers {
			if entry.GoConst == "" {
				return fmt.Errorf("headers.%s: missing required field _go_const", key)
			}
			if entry.PythonConst == "" {
				return fmt.Errorf("headers.%s: missing required field _python_const", key)
			}
		}
	}

	// Validate DocumentIds
	if data.DocumentIds != nil {
		for key, entry := range data.DocumentIds {
			if entry.GoConst == "" {
				return fmt.Errorf("document_ids.%s: missing required field _go_const", key)
			}
			if entry.PythonConst == "" {
				return fmt.Errorf("document_ids.%s: missing required field _python_const", key)
			}
		}
	}

	// Validate Platform
	if data.Platform != nil {
		for key, entry := range data.Platform {
			if entry.GoConst == "" {
				return fmt.Errorf("platform.%s: missing required field _go_const", key)
			}
			if entry.PythonConst == "" {
				return fmt.Errorf("platform.%s: missing required field _python_const", key)
			}
		}
	}

	// Validate PubSub
	if data.PubSub != nil {
		for key, entry := range data.PubSub {
			if entry.GoConst == "" {
				return fmt.Errorf("pubsub.%s: missing required field _go_const", key)
			}
			if entry.PythonConst == "" {
				return fmt.Errorf("pubsub.%s: missing required field _python_const", key)
			}
		}
	}

	// KVKeys, Agents, and Timestamp are special cases that don't require _go_const/_python_const
	// KVKeys uses nested structure with different field naming
	// Agents and Timestamp are simple string maps

	return nil
}

func gofmtFile(filePath string) error {
	cmd := exec.Command("gofmt", "-s", "-w", filePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gofmt failed: %w", err)
	}
	return nil
}
