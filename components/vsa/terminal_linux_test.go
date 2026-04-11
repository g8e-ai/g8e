//go:build linux
// +build linux

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

package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadObfuscatedInput_SimpleKey(t *testing.T) {
	input := strings.NewReader("myapikey\n")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Equal(t, "myapikey", result)
	assert.Equal(t, "********\n", out.String())
}

func TestReadObfuscatedInput_CarriageReturnSubmits(t *testing.T) {
	input := strings.NewReader("abc\r")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Equal(t, "abc", result)
	assert.Equal(t, "***\n", out.String())
}

func TestReadObfuscatedInput_NewlineSubmits(t *testing.T) {
	input := strings.NewReader("abc\n")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Equal(t, "abc", result)
}

func TestReadObfuscatedInput_BackspaceRemovesLastChar(t *testing.T) {
	input := strings.NewReader("abc\x7f\n")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Equal(t, "ab", result)
	assert.Contains(t, out.String(), "\b \b")
}

func TestReadObfuscatedInput_DeleteRemovesLastChar(t *testing.T) {
	input := strings.NewReader("abc\x08\n")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Equal(t, "ab", result)
}

func TestReadObfuscatedInput_BackspaceOnEmptyIsNoop(t *testing.T) {
	input := strings.NewReader("\x7f\x7fabc\n")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Equal(t, "abc", result)
	assert.NotContains(t, out.String(), "\b \b")
}

func TestReadObfuscatedInput_MultipleBackspaces(t *testing.T) {
	input := strings.NewReader("abcde\x7f\x7f\x7fxy\n")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Equal(t, "abxy", result)
}

func TestReadObfuscatedInput_CtrlCReturnsError(t *testing.T) {
	input := strings.NewReader("abc\x03")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.Error(t, err)
	assert.Equal(t, "interrupted", err.Error())
	assert.Empty(t, result)
	assert.Contains(t, out.String(), "\n")
}

func TestReadObfuscatedInput_CtrlCAfterSomeInput(t *testing.T) {
	input := strings.NewReader("secret\x03")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.Error(t, err)
	assert.Equal(t, "interrupted", err.Error())
	assert.Empty(t, result)
}

func TestReadObfuscatedInput_NonPrintableCharsIgnored(t *testing.T) {
	input := strings.NewReader("\x01\x02\x04\x1babc\n")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Equal(t, "abc", result)
	assert.Equal(t, "***\n", out.String())
}

func TestReadObfuscatedInput_AllPrintableASCII(t *testing.T) {
	var chars strings.Builder
	for c := byte(32); c <= 126; c++ {
		chars.WriteByte(c)
	}
	chars.WriteByte('\n')

	input := strings.NewReader(chars.String())
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Len(t, result, 95)
	assert.Equal(t, chars.String()[:95], result)
}

func TestReadObfuscatedInput_EmptyInput(t *testing.T) {
	input := strings.NewReader("\n")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestReadObfuscatedInput_EOFReturnsError(t *testing.T) {
	input := strings.NewReader("abc")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.Error(t, err)
	assert.Equal(t, io.EOF, err)
	assert.Empty(t, result)
}

func TestReadObfuscatedInput_EachCharEchosStar(t *testing.T) {
	input := strings.NewReader("hello\n")
	var out bytes.Buffer

	_, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Equal(t, "*****\n", out.String())
}

func TestReadObfuscatedInput_BackspaceErasesStarSequence(t *testing.T) {
	input := strings.NewReader("ab\x7fc\n")
	var out bytes.Buffer

	result, err := readObfuscatedInput(input, &out)

	require.NoError(t, err)
	assert.Equal(t, "ac", result)
	assert.Equal(t, "**\b \b*\n", out.String())
}

func TestPromptForAPIKey_FallbackPath(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, writeErr := io.WriteString(w, "my-secret-key\n")
	require.NoError(t, writeErr)
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		r.Close()
	})

	result, err := promptForAPIKey()

	require.NoError(t, err)
	assert.Equal(t, "my-secret-key", result)
}

func TestPromptForAPIKey_FallbackPath_TrimsWhitespace(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, writeErr := io.WriteString(w, "  trimmed-key  \n")
	require.NoError(t, writeErr)
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		r.Close()
	})

	result, err := promptForAPIKey()

	require.NoError(t, err)
	assert.Equal(t, "trimmed-key", result)
}

func TestPromptForAPIKey_FallbackPath_EmptyKey(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, writeErr := io.WriteString(w, "\n")
	require.NoError(t, writeErr)
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		r.Close()
	})

	result, err := promptForAPIKey()

	require.NoError(t, err)
	assert.Equal(t, "", result)
}
