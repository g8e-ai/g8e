package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileSvc_ReturnsNonNilService(t *testing.T) {
	svc, err := newFileSvc()
	require.NoError(t, err)
	assert.NotNil(t, svc)
}
