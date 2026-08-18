// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"encoding/json"

	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
)

// shellCommandArgs builds the run_shell_command arguments_json string using
// proper JSON marshaling. The command is the wrapper script name (e.g.
// "dataop", "cloudop", "slew"), and args are the positional arguments passed
// to it. The timeout is fixed at 10 seconds.
func shellCommandArgs(command string, args ...string) string {
	b, err := json.Marshal(clientpkg.ShellCommandArgs{
		Command: command,
		Args:    args,
		Timeout: 10,
	})
	if err != nil {
		return ""
	}
	return string(b)
}

// shellCommandMap builds the typed run_shell_command arguments for use with
// MCPToolsCall, which accepts a client.ToolArgs value.
func shellCommandMap(command string, args ...string) clientpkg.ShellCommandArgs {
	return clientpkg.ShellCommandArgs{
		Command: command,
		Args:    args,
		Timeout: 10,
	}
}
