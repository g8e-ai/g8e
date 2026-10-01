// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package agenttoolsconstants embeds the agent tool registry that g8ee
// generates from its tool specs and real tool handlers (tool schemas plus
// frozen model-visible guidance vectors). It is never hand-edited; regenerate
// it with `make agent-tool-registry`.
package agenttoolsconstants

import (
	"bytes"
	_ "embed"
)

//go:embed agent-tool-registry.json
var agentToolRegistryJSON []byte

// AgentToolRegistryJSON returns a copy of the generated registry document.
func AgentToolRegistryJSON() []byte {
	return bytes.Clone(agentToolRegistryJSON)
}
