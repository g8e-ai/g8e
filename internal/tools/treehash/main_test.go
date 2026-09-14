package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunTreehash_DefaultsToExplicitManifestCollection(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	called := false

	code := runTreehash(
		[]string{"-base", "/source", "cmd", "internal"},
		&stdout,
		&stderr,
		func(base string, entries, excludes []string) (string, error) {
			called = true
			assert.Equal(t, "/source", base)
			assert.Equal(t, []string{"cmd", "internal"}, entries)
			assert.Empty(t, excludes)
			return "a" + strings.Repeat("0", 63), nil
		},
	)

	assert.Zero(t, code)
	assert.True(t, called)
	assert.Empty(t, stderr.String())
}

func TestRunTreehash_RejectsGitAndAutoModes(t *testing.T) {
	for _, mode := range []string{"git", "auto"} {
		t.Run(mode, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			code := runTreehash(
				[]string{"-mode", mode, "cmd"},
				&stdout,
				&stderr,
				func(string, []string, []string) (string, error) {
					t.Fatal("manifest collector must not run for a rejected mode")
					return "", nil
				},
			)

			assert.NotZero(t, code)
			assert.Contains(t, stderr.String(), "manifest")
		})
	}
}
