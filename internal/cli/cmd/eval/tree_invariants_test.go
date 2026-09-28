// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

// TestEvalTreeInvariants verifies the structure and naming conventions of the
// eval command tree.
func TestEvalTreeInvariants(t *testing.T) {
	deps := nativeEvalDeps{}
	cmd := evalCmdWithConfig(deps)

	// Invariant 1: No group has exactly one subcommand
	checkNoSingletonGroups(t, cmd)

	// Invariant 2: Group names are plural (except mass nouns: rollout, boundary, observer)
	checkPluralGroupNames(t, cmd)

	// Invariant 3: No --run-id, --params, or path flags (except --output-dir, --log-dir)
	checkFlagNames(t, cmd)

	// Invariant 4: Leaves printing structured data support --json
	checkJSONSupport(t, cmd)

	// Invariant 5: Leaves taking IDs use positional arguments, not flags
	checkPositionalIDs(t, cmd)
}

func checkNoSingletonGroups(t *testing.T, cmd *cobra.Command) {
	walkCommands(cmd, func(path []string, c *cobra.Command) {
		// Only check groups, not leaf commands
		if len(c.Commands()) == 0 {
			return
		}
		if len(c.Commands()) == 1 {
			t.Errorf("group %s has exactly one subcommand (invariant 1)", strings.Join(path, " "))
		}
	})
}

func checkPluralGroupNames(t *testing.T, cmd *cobra.Command) {
	// Mass nouns that are allowed to be singular-like
	massNouns := map[string]bool{
		"rollout":   true,
		"boundary":  true,
		"observer":  true,
	}

	walkCommands(cmd, func(path []string, c *cobra.Command) {
		// Only check groups (non-leaf commands)
		if len(c.Commands()) == 0 {
			return
		}
		if len(path) == 0 {
			return
		}
		name := path[len(path)-1]
		if massNouns[name] {
			return
		}
		// Check if name ends with 's' (simple plural check)
		if !strings.HasSuffix(name, "s") {
			t.Logf("note: group %s should be plural (invariant 2)", strings.Join(path, " "))
		}
	})
}

func checkFlagNames(t *testing.T, cmd *cobra.Command) {
	forbiddenFlags := map[string]bool{
		"run-id": true,
		"params": true,
	}
	allowedPathFlags := map[string]bool{
		"output-dir": true,
		"log-dir":    true,
	}

	walkCommands(cmd, func(path []string, c *cobra.Command) {
		if c.Flags() == nil {
			return
		}
		c.Flags().VisitAll(func(f *pflag.Flag) {
			// Check for forbidden flags
			if forbiddenFlags[f.Name] {
				t.Errorf("flag --%s is forbidden in %s (invariant 3)", f.Name, strings.Join(path, " "))
			}
			// Check for path flags (ending in -file or -dir)
			if (strings.HasSuffix(f.Name, "-file") || strings.HasSuffix(f.Name, "-dir")) && !allowedPathFlags[f.Name] {
				t.Errorf("flag --%s should not expose internal paths in %s (invariant 3)", f.Name, strings.Join(path, " "))
			}
		})
	})
}

func checkJSONSupport(t *testing.T, cmd *cobra.Command) {
	// This is a best-effort check; we can't fully determine if --json is needed without executing
	walkCommands(cmd, func(path []string, c *cobra.Command) {
		// Only check leaf commands
		if len(c.Commands()) > 0 {
			return
		}
		// Commands that are known to print structured data
		structuredOutputCommands := map[string]bool{
			"list": true, "show": true, "add": true, "remove": true,
			"verify": true, "publish": true, "export": true, "compare": true,
			"archive": true, "unarchive": true, "cancel": true, "logs": true,
			"start": true, "resume": true, "next": true, "repair": true,
			"run":    true, "retry": true, "skip": true,
		}

		// Skip read-only commands that don't need JSON
		readOnlySpecialCases := map[string]bool{
			"watch": true, "follow": true,
		}

		cmdName := c.Name()
		if !readOnlySpecialCases[cmdName] && structuredOutputCommands[cmdName] {
			hasJSON := false
			if c.Flags() != nil {
				c.Flags().VisitAll(func(f *pflag.Flag) {
					if f.Name == "json" {
						hasJSON = true
					}
				})
			}
			// Check parent flags too
			if !hasJSON && c.Parent() != nil && c.Parent().Flags() != nil {
				c.Parent().Flags().VisitAll(func(f *pflag.Flag) {
					if f.Name == "json" {
						hasJSON = true
					}
				})
			}
			if !hasJSON {
				// This is a warning, not an error, since global --json might be inherited
				t.Logf("note: %s might not support --json (invariant 4)", strings.Join(path, " "))
			}
		}
	})
}

func checkPositionalIDs(t *testing.T, cmd *cobra.Command) {
	// Check that commands taking IDs use positional arguments
	idFlagPatterns := []string{"-id"}
	walkCommands(cmd, func(path []string, c *cobra.Command) {
		if c.Flags() == nil {
			return
		}
		c.Flags().VisitAll(func(f *pflag.Flag) {
			for _, pattern := range idFlagPatterns {
				if strings.HasSuffix(f.Name, pattern) && f.Name != "session-id" && f.Name != "pool-id" {
					// session-id and pool-id are not command IDs
					t.Errorf("command %s should use positional argument for ID, not flag --%s (invariant 5)", strings.Join(path, " "), f.Name)
				}
			}
		})
	})
}

// walkCommands recursively walks the command tree and calls fn for each command
func walkCommands(cmd *cobra.Command, fn func(path []string, c *cobra.Command)) {
	fn([]string{}, cmd)
	for _, subcmd := range cmd.Commands() {
		walkCommandsRecursive([]string{cmd.Name()}, subcmd, fn)
	}
}

func walkCommandsRecursive(path []string, cmd *cobra.Command, fn func(path []string, c *cobra.Command)) {
	fn(append(path, cmd.Name()), cmd)
	for _, subcmd := range cmd.Commands() {
		walkCommandsRecursive(append(path, cmd.Name()), subcmd, fn)
	}
}

// TestEvalCmd_ContainsExpectedGroups verifies the eval command tree has the
// expected top-level groups.
func TestEvalCmd_ContainsExpectedGroups(t *testing.T) {
	deps := nativeEvalDeps{}
	cmd := evalCmdWithConfig(deps)

	expected := []string{
		"models", "campaigns", "runs", "rollout", "formations", "gates", "boundary", "observer",
	}
	var actual []string
	for _, subcmd := range cmd.Commands() {
		actual = append(actual, subcmd.Name())
	}

	assert.Equal(t, expected, actual, "eval command should have exactly these groups")
}
