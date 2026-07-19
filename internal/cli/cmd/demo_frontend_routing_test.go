package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunFrontendScenario_InvalidNumberReturnsError(t *testing.T) {
	_, err := runFrontendScenario("", "99")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid scenario number")
}

func TestRunFrontendScenario_EmptyStringReturnsError(t *testing.T) {
	_, err := runFrontendScenario("", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid scenario number")
}
