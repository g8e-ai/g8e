package cmd

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileSvc_ReturnsNonNilService(t *testing.T) {
	svc, err := newFileSvc("", slog.Default())
	require.NoError(t, err)
	assert.NotNil(t, svc)
}
