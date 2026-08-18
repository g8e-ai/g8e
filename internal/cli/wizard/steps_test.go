// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- nextStep ---

func TestNextStep_NetworkToPosture(t *testing.T) {
	assert.Equal(t, StepPosture, nextStep(StepNetwork))
}

func TestNextStep_PostureToRouting(t *testing.T) {
	assert.Equal(t, StepRouting, nextStep(StepPosture))
}

func TestNextStep_RoutingToReview(t *testing.T) {
	assert.Equal(t, StepReview, nextStep(StepRouting))
}

func TestNextStep_ReviewToDone(t *testing.T) {
	assert.Equal(t, StepDone, nextStep(StepReview))
}

func TestNextStep_DoneStaysDone(t *testing.T) {
	assert.Equal(t, StepDone, nextStep(StepDone))
}

// --- prevStep ---

func TestPrevStep_NetworkStaysNetwork(t *testing.T) {
	assert.Equal(t, StepNetwork, prevStep(StepNetwork))
}

func TestPrevStep_PostureToNetwork(t *testing.T) {
	assert.Equal(t, StepNetwork, prevStep(StepPosture))
}

func TestPrevStep_RoutingToPosture(t *testing.T) {
	assert.Equal(t, StepPosture, prevStep(StepRouting))
}

func TestPrevStep_ReviewToRouting(t *testing.T) {
	assert.Equal(t, StepRouting, prevStep(StepReview))
}

// --- stepNumber ---

func TestStepNumber_NetworkIsOne(t *testing.T) {
	assert.Equal(t, 1, stepNumber(StepNetwork))
}

func TestStepNumber_PostureIsTwo(t *testing.T) {
	assert.Equal(t, 2, stepNumber(StepPosture))
}

func TestStepNumber_RoutingIsThree(t *testing.T) {
	assert.Equal(t, 3, stepNumber(StepRouting))
}

func TestStepNumber_ReviewIsFour(t *testing.T) {
	assert.Equal(t, 4, stepNumber(StepReview))
}

func TestStepNumber_DoneIsZero(t *testing.T) {
	assert.Equal(t, 0, stepNumber(StepDone))
}

// --- stepDefs completeness ---

func TestStepDefs_AllStepsPresent(t *testing.T) {
	for _, s := range stepOrder {
		def, ok := stepDefs[s]
		assert.True(t, ok, "stepDefs should contain step %d", s)
		assert.NotEmpty(t, def.title, "step %d should have a title", s)
		assert.NotEmpty(t, def.subtitle, "step %d should have a subtitle", s)
	}
}

func TestStepDefs_NetworkTitle(t *testing.T) {
	def := stepDefs[StepNetwork]
	assert.Equal(t, "Network & Identity", def.title)
}

func TestStepDefs_PostureTitle(t *testing.T) {
	def := stepDefs[StepPosture]
	assert.Equal(t, "Security & Governance Posture", def.title)
}

func TestStepDefs_RoutingTitle(t *testing.T) {
	def := stepDefs[StepRouting]
	assert.Equal(t, "Agent Tooling & Routing", def.title)
}

func TestStepDefs_ReviewTitle(t *testing.T) {
	def := stepDefs[StepReview]
	assert.Equal(t, "Review & Confirm", def.title)
}

// --- stepOrder ---

func TestStepOrder_Sequence(t *testing.T) {
	assert.Equal(t, 4, len(stepOrder))
	assert.Equal(t, StepNetwork, stepOrder[0])
	assert.Equal(t, StepPosture, stepOrder[1])
	assert.Equal(t, StepRouting, stepOrder[2])
	assert.Equal(t, StepReview, stepOrder[3])
}

func TestStepOrder_ExcludesDone(t *testing.T) {
	for _, s := range stepOrder {
		assert.NotEqual(t, StepDone, s, "stepOrder should not include StepDone")
	}
}
