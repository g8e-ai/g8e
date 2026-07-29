package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopFieldReader_GetField_ReturnsEmptyNoError(t *testing.T) {
	t.Parallel()
	reader := NoopFieldReader{}
	val, err := reader.GetField("any-collection", "any-id", "any.field.path")
	require.NoError(t, err, "NoopFieldReader must never return an error")
	assert.Equal(t, FieldValue{}, val, "NoopFieldReader must return empty FieldValue")
}

func TestNoopFieldReader_SatisfiesInterface(t *testing.T) {
	t.Parallel()
	var _ FieldReader = NoopFieldReader{}
}
