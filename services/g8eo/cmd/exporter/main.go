package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

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

	// Ensure directories exist
	if err := os.MkdirAll(protocolConstantsDir, 0750); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", protocolConstantsDir, err)
		os.Exit(1)
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
		Client         map[string]string `json:"client"`
		ClientFull     map[string]string `json:"client_full"`
	}{
		InternalPrefix: constants.APIPaths.InternalPrefix,
		OperatorPrefix: constants.APIPaths.OperatorPrefix,
		Client:         constants.APIPaths.Client,
		ClientFull:     make(map[string]string),
	}
	for k, v := range constants.APIPaths.Client {
		apiPathsFull.ClientFull[k] = constants.APIPaths.OperatorPrefix + v
	}
	emitJSON("api_paths.json", apiPathsFull, true)

	fmt.Println("Constants exported successfully.")
}
