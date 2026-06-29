// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractFirstTxHash(t *testing.T) {
	t.Run("single transaction", func(t *testing.T) {
		body := `{"transactions":[{"transaction_hash":"abc123"}]}`
		assert.Equal(t, "abc123", extractFirstTxHash(body))
	})

	t.Run("multiple transactions returns first", func(t *testing.T) {
		body := `{"transactions":[{"transaction_hash":"first"},{"transaction_hash":"second"}]}`
		assert.Equal(t, "first", extractFirstTxHash(body))
	})

	t.Run("empty transactions list", func(t *testing.T) {
		body := `{"transactions":[]}`
		assert.Equal(t, "", extractFirstTxHash(body))
	})

	t.Run("nil transactions field", func(t *testing.T) {
		body := `{}`
		assert.Equal(t, "", extractFirstTxHash(body))
	})

	t.Run("invalid JSON", func(t *testing.T) {
		body := `not json at all`
		assert.Equal(t, "", extractFirstTxHash(body))
	})

	t.Run("empty string", func(t *testing.T) {
		assert.Equal(t, "", extractFirstTxHash(""))
	})

	t.Run("missing transaction_hash field", func(t *testing.T) {
		body := `{"transactions":[{"other_field":"value"}]}`
		assert.Equal(t, "", extractFirstTxHash(body))
	})
}
