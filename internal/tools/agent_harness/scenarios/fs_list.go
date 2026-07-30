// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import "encoding/json"

// fsListJSON is the typed structure for fs_list arguments.
// Using json.Marshal instead of raw string literals ensures proper string
// escaping and complies with devs.md "no ad-hoc JSON" rule.
type fsListJSON struct {
	Path string `json:"path"`
}

// fsListArgs builds the fs_list arguments_json string using proper JSON
// marshaling.
func fsListArgs(path string) string {
	b, err := json.Marshal(fsListJSON{Path: path})
	if err != nil {
		return ""
	}
	return string(b)
}

// fsListMap builds the fs_list arguments as a map[string]any for use with
// MCPToolsCall, which expects map args rather than a JSON string.
func fsListMap(path string) map[string]any {
	return map[string]any{"path": path}
}
