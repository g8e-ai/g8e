// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import "github.com/spf13/cobra"

const (
	// annotationJSON records whether a leaf command reports structured output
	// through the global --json flag. Every leaf declares one of the two values.
	annotationJSON = "g8e.eval/json"

	jsonSupported = "supported"
	jsonNone      = "none"
)

// jsonLeaf declares that a leaf reports structured data through --json.
func jsonLeaf(cmd *cobra.Command) *cobra.Command {
	return annotateJSON(cmd, jsonSupported)
}

// noJSON declares that a leaf streams text and has no structured form.
func noJSON(cmd *cobra.Command) *cobra.Command {
	return annotateJSON(cmd, jsonNone)
}

func annotateJSON(cmd *cobra.Command, value string) *cobra.Command {
	if _, declared := cmd.Annotations[annotationJSON]; declared {
		return cmd
	}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string, 1)
	}
	cmd.Annotations[annotationJSON] = value
	return cmd
}

// addLeaves registers structured-output leaf commands on a group.
func addLeaves(group *cobra.Command, leaves ...*cobra.Command) {
	for _, leaf := range leaves {
		group.AddCommand(jsonLeaf(leaf))
	}
}
