// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import "encoding/json"

// shellCommandJSON is the typed structure for run_shell_command arguments.
// Using json.Marshal instead of fmt.Sprintf ensures proper string escaping and
// complies with devs.md "no ad-hoc JSON" rule.
type shellCommandJSON struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout"`
}

// shellCommandArgs builds the run_shell_command arguments_json string using
// proper JSON marshaling. The command is the wrapper script name (e.g.
// "dataop", "cloudop", "slew"), and args are the positional arguments passed
// to it. The timeout is fixed at 10 seconds.
func shellCommandArgs(command string, args ...string) string {
	b, err := json.Marshal(shellCommandJSON{
		Command: command,
		Args:    args,
		Timeout: 10,
	})
	if err != nil {
		return ""
	}
	return string(b)
}
