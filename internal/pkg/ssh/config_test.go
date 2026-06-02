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

package ssh

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	t.Run("single host block", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host myserver
    HostName 192.168.1.10
    User deploy
    Port 2222
    IdentityFile ~/.ssh/id_ed25519
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		blocks := ParseConfig(cfg)
		require.Contains(t, blocks, "myserver")
		b := blocks["myserver"]
		assert.Equal(t, "192.168.1.10", b.Hostname)
		assert.Equal(t, "deploy", b.User)
		assert.Equal(t, "2222", b.Port)
		assert.Len(t, b.IdentityFiles, 1)
	})

	t.Run("multiple blocks", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host prod-*
    User ubuntu
    IdentityFile ~/.ssh/prod_key

Host staging
    HostName 10.0.1.5
    User admin
    Port 2200
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		blocks := ParseConfig(cfg)
		assert.Len(t, blocks, 2)
		assert.Contains(t, blocks, "prod-*")
		assert.Contains(t, blocks, "staging")
	})

	t.Run("equals delimiter", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host equalhost
    HostName=10.0.0.1
    User=admin
    Port=22
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		blocks := ParseConfig(cfg)
		require.Contains(t, blocks, "equalhost")
		b := blocks["equalhost"]
		assert.Equal(t, "10.0.0.1", b.Hostname)
		assert.Equal(t, "admin", b.User)
	})

	t.Run("missing file", func(t *testing.T) {
		blocks := ParseConfig("/nonexistent/.ssh/config")
		assert.Empty(t, blocks)
	})

	t.Run("comments", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
# Global comment
Host commented
    # Inline comment
    HostName 10.0.0.2
    User ops
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		blocks := ParseConfig(cfg)
		require.Contains(t, blocks, "commented")
		assert.Equal(t, "10.0.0.2", blocks["commented"].Hostname)
	})

	t.Run("multiple identity files", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host multi
    IdentityFile ~/.ssh/key1
    IdentityFile ~/.ssh/key2
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		blocks := ParseConfig(cfg)
		require.Contains(t, blocks, "multi")
		assert.Len(t, blocks["multi"].IdentityFiles, 2)
	})
}

func TestMatchBlock(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		blocks := map[string]*ConfigBlock{
			"myhost": {Hostname: "1.2.3.4"},
		}
		b := MatchBlock(blocks, "myhost")
		require.NotNil(t, b)
		assert.Equal(t, "1.2.3.4", b.Hostname)
	})

	t.Run("wildcard match", func(t *testing.T) {
		blocks := map[string]*ConfigBlock{
			"prod-*": {User: "ubuntu"},
		}
		b := MatchBlock(blocks, "prod-web-01")
		require.NotNil(t, b)
		assert.Equal(t, "ubuntu", b.User)
	})

	t.Run("no match", func(t *testing.T) {
		blocks := map[string]*ConfigBlock{
			"staging": {Hostname: "10.0.0.1"},
		}
		b := MatchBlock(blocks, "production")
		assert.Nil(t, b)
	})
}

func TestPatternMatch(t *testing.T) {
	cases := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"*", "anything", true},
		{"prod-*", "prod-web-01", true},
		{"prod-*", "staging-01", false},
		{"host?", "host1", true},
		{"host?", "host12", false},
		{"*.example.com", "foo.example.com", true},
		{"*.example.com", "example.com", false},
		{"exact", "exact", true},
		{"exact", "not-exact", false},
	}
	for _, tc := range cases {
		got := PatternMatch(tc.pattern, tc.input)
		assert.Equal(t, tc.want, got, "PatternMatch(%q, %q)", tc.pattern, tc.input)
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"*", "anything", true},
		{"prod-*", "prod-web-01", true},
		{"prod-*", "staging-01", false},
		{"host?", "host1", true},
		{"host?", "host12", false},
		{"*.example.com", "foo.example.com", true},
		{"*.example.com", "example.com", false},
		{"exact", "exact", true},
		{"exact", "not-exact", false},
	}
	for _, tc := range cases {
		got := MatchGlob(tc.pattern, tc.input)
		assert.Equal(t, tc.want, got, "MatchGlob(%q, %q)", tc.pattern, tc.input)
	}
}

func TestExpandTilde(t *testing.T) {
	t.Run("with tilde", func(t *testing.T) {
		home, _ := os.UserHomeDir()
		got := ExpandTilde("~/.ssh/id_ed25519")
		assert.Equal(t, filepath.Join(home, ".ssh/id_ed25519"), got)
	})

	t.Run("without tilde", func(t *testing.T) {
		got := ExpandTilde("/absolute/path")
		assert.Equal(t, "/absolute/path", got)
	})
}
