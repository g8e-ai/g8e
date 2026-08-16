// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"encoding/json"

	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
)

// fsListArgs builds the fs_list arguments_json string using proper JSON
// marshaling.
func fsListArgs(path string) string {
	b, err := json.Marshal(clientpkg.FSPathArgs{Path: path})
	if err != nil {
		return ""
	}
	return string(b)
}

// fsListMap builds the typed fs_list arguments for use with MCPToolsCall,
// which accepts a client.ToolArgs value.
func fsListMap(path string) clientpkg.FSPathArgs {
	return clientpkg.FSPathArgs{Path: path}
}
