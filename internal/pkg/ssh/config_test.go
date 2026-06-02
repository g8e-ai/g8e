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

func TestResolveHost(t *testing.T) {
	t.Run("simple hostname", func(t *testing.T) {
		r := ResolveHost("myserver", "", "defaultuser", "", "")
		assert.Equal(t, "myserver", r.Original)
		assert.Equal(t, "myserver", r.Hostname)
		assert.Equal(t, "defaultuser", r.User)
		assert.Equal(t, "22", r.Port)
	})

	t.Run("user@host format", func(t *testing.T) {
		r := ResolveHost("alice@myserver", "", "", "", "")
		assert.Equal(t, "alice@myserver", r.Original)
		assert.Equal(t, "myserver", r.Hostname)
		assert.Equal(t, "alice", r.User)
		assert.Equal(t, "22", r.Port)
	})

	t.Run("host:port format", func(t *testing.T) {
		r := ResolveHost("myserver:2222", "", "defaultuser", "", "")
		assert.Equal(t, "myserver:2222", r.Original)
		assert.Equal(t, "myserver", r.Hostname)
		assert.Equal(t, "2222", r.Port)
		assert.Equal(t, "defaultuser", r.User)
	})

	t.Run("user@host:port format", func(t *testing.T) {
		r := ResolveHost("alice@myserver:2222", "", "", "", "")
		assert.Equal(t, "alice@myserver:2222", r.Original)
		assert.Equal(t, "myserver", r.Hostname)
		assert.Equal(t, "alice", r.User)
		assert.Equal(t, "2222", r.Port)
	})

	t.Run("with SSH config file", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host testhost
    HostName 192.168.1.100
    User deploy
    Port 2200
    IdentityFile ~/.ssh/test_key
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		r := ResolveHost("testhost", cfg, "", "", "")
		assert.Equal(t, "testhost", r.Original)
		assert.Equal(t, "192.168.1.100", r.Hostname)
		assert.Equal(t, "deploy", r.User)
		assert.Equal(t, "2200", r.Port)
		assert.Len(t, r.KeyFiles, 1)
	})

	t.Run("SSH config with wildcard match", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host prod-*
    User ubuntu
    Port 2222
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		r := ResolveHost("prod-web-01", cfg, "", "", "")
		assert.Equal(t, "prod-web-01", r.Original)
		assert.Equal(t, "prod-web-01", r.Hostname)
		assert.Equal(t, "ubuntu", r.User)
		assert.Equal(t, "2222", r.Port)
	})

	t.Run("SSH user flag overrides config", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host testhost
    User configuser
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		r := ResolveHost("testhost", cfg, "", "", "flaguser")
		assert.Equal(t, "flaguser", r.User)
	})

	t.Run("SSH identity file flag overrides config", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host testhost
    IdentityFile ~/.ssh/config_key
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		r := ResolveHost("testhost", cfg, "", "/path/to/flag_key", "")
		assert.Len(t, r.KeyFiles, 1)
		assert.Equal(t, "/path/to/flag_key", r.KeyFiles[0])
	})

	t.Run("default username when none provided", func(t *testing.T) {
		r := ResolveHost("myserver", "", "defaultuser", "", "")
		assert.Equal(t, "defaultuser", r.User)
	})

	t.Run("default port when none provided", func(t *testing.T) {
		r := ResolveHost("myserver", "", "", "", "")
		assert.Equal(t, "22", r.Port)
	})

	t.Run("proxy command from config", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host jump
    ProxyCommand ssh -W %h:%p jump.example.com
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		r := ResolveHost("jump", cfg, "", "", "")
		assert.Equal(t, "ssh -W %h:%p jump.example.com", r.ProxyCommand)
	})

	t.Run("missing SSH config file", func(t *testing.T) {
		r := ResolveHost("myserver", "/nonexistent/config", "defaultuser", "", "")
		assert.Equal(t, "myserver", r.Hostname)
		assert.Equal(t, "defaultuser", r.User)
		assert.Equal(t, "22", r.Port)
	})

	t.Run("port 22 from config is ignored", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host testhost
    Port 22
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		r := ResolveHost("testhost", cfg, "", "", "")
		assert.Equal(t, "22", r.Port)
	})

	t.Run("config hostname overrides parsed hostname", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host alias
    HostName 192.168.1.50
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		r := ResolveHost("alias", cfg, "", "", "")
		assert.Equal(t, "192.168.1.50", r.Hostname)
	})

	t.Run("user@host with config hostname override", func(t *testing.T) {
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		content := `
Host alias
    HostName 192.168.1.50
`
		require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

		r := ResolveHost("alice@alias", cfg, "", "", "")
		assert.Equal(t, "192.168.1.50", r.Hostname)
		assert.Equal(t, "alice", r.User)
	})
}

func TestBuildAuthMethods(t *testing.T) {
	t.Run("empty host config", func(t *testing.T) {
		r := HostConfig{}
		methods := BuildAuthMethods(r, "", "")
		assert.Empty(t, methods)
	})

	t.Run("non-existent key file", func(t *testing.T) {
		r := HostConfig{KeyFiles: []string{"/nonexistent/key"}}
		methods := BuildAuthMethods(r, "", "")
		assert.Empty(t, methods)
	})

	t.Run("invalid key file content", func(t *testing.T) {
		dir := t.TempDir()
		keyPath := filepath.Join(dir, "invalid_key")
		require.NoError(t, os.WriteFile(keyPath, []byte("not a valid key"), 0600))

		r := HostConfig{KeyFiles: []string{keyPath}}
		methods := BuildAuthMethods(r, "", "")
		assert.Empty(t, methods)
	})

	t.Run("invalid SSH agent socket", func(t *testing.T) {
		r := HostConfig{}
		methods := BuildAuthMethods(r, "/nonexistent/agent.sock", "")
		assert.Empty(t, methods)
	})

	t.Run("empty key file", func(t *testing.T) {
		dir := t.TempDir()
		keyPath := filepath.Join(dir, "empty_key")
		require.NoError(t, os.WriteFile(keyPath, []byte(""), 0600))

		r := HostConfig{KeyFiles: []string{keyPath}}
		methods := BuildAuthMethods(r, "", "")
		assert.Empty(t, methods)
	})
}

func TestBuildHostKeyCallback(t *testing.T) {
	t.Run("known_hosts file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("HOME", dir)

		_, err := BuildHostKeyCallback()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "known_hosts not found")
	})

	t.Run("malformed known_hosts file", func(t *testing.T) {
		dir := t.TempDir()
		sshDir := filepath.Join(dir, ".ssh")
		require.NoError(t, os.Mkdir(sshDir, 0700))
		khPath := filepath.Join(sshDir, "known_hosts")
		require.NoError(t, os.WriteFile(khPath, []byte("invalid known_hosts content"), 0600))

		t.Setenv("HOME", dir)

		_, err := BuildHostKeyCallback()
		assert.Error(t, err)
	})
}
