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

package wizard

// Step represents a wizard step in the state machine.
type Step int

const (
	StepNetwork Step = iota // Step 1: Network & Identity
	StepPosture             // Step 2: Security & Governance Posture
	StepRouting             // Step 3: Agent Tooling & Routing
	StepVault               // Step 4: Vault Strictness
	StepReview              // Step 5: Review & Confirm
	StepDone                // Wizard complete
)

type stepDef struct {
	step     Step
	title    string
	subtitle string
}

var stepOrder = []Step{
	StepNetwork,
	StepPosture,
	StepRouting,
	StepVault,
	StepReview,
}

var stepDefs = map[Step]stepDef{
	StepNetwork: {StepNetwork, "Network & Identity", "Configure how operators and clients reach this gateway."},
	StepPosture: {StepPosture, "Security & Governance Posture", "Select the enforcement level for governance signatures."},
	StepRouting: {StepRouting, "Agent Tooling & Routing", "Route gateway traffic to downstream MCP or A2A servers."},
	StepVault:   {StepVault, "Vault Strictness", "Control whether the gateway fails closed without a vault key."},
	StepReview:  {StepReview, "Review & Confirm", "Verify all settings before starting the gateway."},
}

// nextStep determines the next step in the wizard sequence.
func nextStep(current Step) Step {
	idx := int(current)
	if idx+1 >= len(stepOrder) {
		return StepDone
	}
	return stepOrder[idx+1]
}

// prevStep determines the previous step in the wizard sequence.
func prevStep(current Step) Step {
	idx := int(current)
	if idx <= 0 {
		return stepOrder[0]
	}
	return stepOrder[idx-1]
}

// stepNumber returns the 1-indexed step number for display.
func stepNumber(s Step) int {
	for i, st := range stepOrder {
		if st == s {
			return i + 1
		}
	}
	return 0
}
