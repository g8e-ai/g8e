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

package mcp

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setPrivateIPAllowlistForTest(t *testing.T, cidrs []string) {
	t.Helper()
	parsed := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, ipNet, err := net.ParseCIDR(c)
		require.NoError(t, err, "invalid CIDR %q", c)
		parsed = append(parsed, ipNet)
	}
	privateAllowlistMu.Lock()
	privateAllowlist = parsed
	privateAllowlistMu.Unlock()
}

func resetPrivateIPAllowlistForTest() {
	privateAllowlistMu.Lock()
	privateAllowlist = nil
	privateAllowlistMu.Unlock()
}

func TestValidateHTTPRequestURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid http URL",
			url:     "http://example.com",
			wantErr: false,
		},
		{
			name:    "valid https URL with path",
			url:     "https://example.com/path?query=1",
			wantErr: false,
		},
		{
			name:    "missing scheme",
			url:     "example.com",
			wantErr: true,
		},
		{
			name:    "ftp scheme",
			url:     "ftp://example.com",
			wantErr: true,
		},
		{
			name:    "localhost hostname",
			url:     "http://localhost",
			wantErr: true,
		},
		{
			name:    "localhost with port",
			url:     "http://localhost:8080",
			wantErr: true,
		},
		{
			name:    "127.0.0.1",
			url:     "http://127.0.0.1",
			wantErr: true,
		},
		{
			name:    "::1",
			url:     "http://[::1]",
			wantErr: true,
		},
		{
			name:    "private IP 10.x",
			url:     "http://10.0.0.1",
			wantErr: true,
		},
		{
			name:    "private IP 172.16.x",
			url:     "http://172.16.0.1",
			wantErr: true,
		},
		{
			name:    "private IP 192.168.x",
			url:     "http://192.168.1.1",
			wantErr: true,
		},
		{
			name:    "link-local 169.254.x",
			url:     "http://169.254.1.1",
			wantErr: true,
		},
		{
			name:    "empty host",
			url:     "http:///path",
			wantErr: true,
		},
		{
			name:    "invalid URL format",
			url:     "://bad-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := validateHTTPRequestURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err, tt.url)
				assert.Nil(t, parsed, tt.url)
				return
			}
			assert.NoError(t, err, tt.url)
			assert.NotNil(t, parsed, tt.url)
		})
	}
}

func TestValidateSQLQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "valid SELECT query",
			query:   "SELECT * FROM users",
			wantErr: false,
		},
		{
			name:    "valid SELECT with WHERE",
			query:   "SELECT id, name FROM users WHERE id = 1",
			wantErr: false,
		},
		{
			name:    "valid SELECT with JOIN",
			query:   "SELECT u.name, o.order_id FROM users u JOIN orders o ON u.id = o.user_id",
			wantErr: false,
		},
		{
			name:    "valid SELECT with subquery",
			query:   "SELECT * FROM users WHERE id IN (SELECT user_id FROM orders)",
			wantErr: false,
		},
		{
			name:    "valid SELECT with GROUP BY",
			query:   "SELECT COUNT(*) FROM users GROUP BY status",
			wantErr: false,
		},
		{
			name:    "valid SELECT with ORDER BY",
			query:   "SELECT * FROM users ORDER BY name ASC",
			wantErr: false,
		},
		{
			name:    "valid SELECT with LIMIT",
			query:   "SELECT * FROM users LIMIT 10",
			wantErr: false,
		},
		{
			name:    "empty query",
			query:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			query:   "   ",
			wantErr: true,
		},
		{
			name:    "trailing semicolon",
			query:   "SELECT * FROM users;",
			wantErr: true,
		},
		{
			name:    "trailing semicolon with spaces",
			query:   "SELECT * FROM users;   ",
			wantErr: true,
		},
		{
			name:    "query with semicolon in middle (allowed, DB will reject)",
			query:   "SELECT * FROM users; SELECT * FROM orders",
			wantErr: false,
		},
		{
			name:    "query with comment (allowed, DB will reject)",
			query:   "SELECT * FROM users -- comment",
			wantErr: false,
		},
		{
			name:    "query with block comment (allowed, DB will reject)",
			query:   "SELECT * FROM users /* comment */",
			wantErr: false,
		},
		{
			name:    "DROP query (allowed here, rejected by caller's SELECT check)",
			query:   "DROP TABLE users",
			wantErr: false,
		},
		{
			name:    "DELETE query (allowed here, rejected by caller's SELECT check)",
			query:   "DELETE FROM users WHERE id = 1",
			wantErr: false,
		},
		{
			name:    "INSERT query (allowed here, rejected by caller's SELECT check)",
			query:   "INSERT INTO users (name) VALUES ('test')",
			wantErr: false,
		},
		{
			name:    "UPDATE query (allowed here, rejected by caller's SELECT check)",
			query:   "UPDATE users SET name = 'test' WHERE id = 1",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSQLQuery(tt.query)
			if tt.wantErr {
				assert.Error(t, err, tt.query)
			} else {
				assert.NoError(t, err, tt.query)
			}
		})
	}
}

func TestValidateGitRepoPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid current directory",
			path:    ".",
			wantErr: false,
		},
		{
			name:    "valid relative path",
			path:    "myrepo",
			wantErr: false,
		},
		{
			name:    "valid absolute path",
			path:    "/home/user/repo",
			wantErr: false,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "path with parent directory reference",
			path:    "../repo",
			wantErr: true,
		},
		{
			name:    "path with embedded parent reference",
			path:    "repo/../other",
			wantErr: true,
		},
		{
			name:    "path with leading whitespace",
			path:    "  repo",
			wantErr: true,
		},
		{
			name:    "path with trailing whitespace",
			path:    "repo  ",
			wantErr: true,
		},
		{
			name:    "path with null byte",
			path:    "repo\x00",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitRepoPath(tt.path)
			if tt.wantErr {
				assert.Error(t, err, tt.path)
			} else {
				assert.NoError(t, err, tt.path)
			}
		})
	}
}

func TestValidateGitRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{
			name:    "valid HEAD",
			ref:     "HEAD",
			wantErr: false,
		},
		{
			name:    "valid branch name",
			ref:     "main",
			wantErr: false,
		},
		{
			name:    "valid feature branch",
			ref:     "feature/new-feature",
			wantErr: false,
		},
		{
			name:    "valid remote branch",
			ref:     "origin/main",
			wantErr: false,
		},
		{
			name:    "valid tag",
			ref:     "v1.0.0",
			wantErr: false,
		},
		{
			name:    "valid commit hash",
			ref:     "abc123def456",
			wantErr: false,
		},
		{
			name:    "valid HEAD~1",
			ref:     "HEAD~1",
			wantErr: false,
		},
		{
			name:    "valid HEAD~10",
			ref:     "HEAD~10",
			wantErr: false,
		},
		{
			name:    "empty ref",
			ref:     "",
			wantErr: true,
		},
		{
			name:    "ref with leading whitespace",
			ref:     "  main",
			wantErr: true,
		},
		{
			name:    "ref with trailing whitespace",
			ref:     "main  ",
			wantErr: true,
		},
		{
			name:    "ref with null byte",
			ref:     "main\x00",
			wantErr: true,
		},
		{
			name:    "ref with semicolon",
			ref:     "main;rm -rf",
			wantErr: true,
		},
		{
			name:    "ref with ampersand",
			ref:     "main&evil",
			wantErr: true,
		},
		{
			name:    "ref with pipe",
			ref:     "main|cat",
			wantErr: true,
		},
		{
			name:    "ref with dollar sign",
			ref:     "main$(evil)",
			wantErr: true,
		},
		{
			name:    "ref with backtick",
			ref:     "main`evil`",
			wantErr: true,
		},
		{
			name:    "ref with parentheses",
			ref:     "main(evil)",
			wantErr: true,
		},
		{
			name:    "ref with redirect",
			ref:     "main>/etc/passwd",
			wantErr: true,
		},
		{
			name:    "ref with newline",
			ref:     "main\nevil",
			wantErr: true,
		},
		{
			name:    "ref with carriage return",
			ref:     "main\revil",
			wantErr: true,
		},
		{
			name:    "absolute path",
			ref:     "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "Windows absolute path",
			ref:     "\\windows\\system32",
			wantErr: true,
		},
		{
			name:    "ref with space",
			ref:     "main branch",
			wantErr: true,
		},
		{
			name:    "ref with special chars",
			ref:     "main@#$%",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitRef(tt.ref)
			if tt.wantErr {
				assert.Error(t, err, tt.ref)
			} else {
				assert.NoError(t, err, tt.ref)
			}
		})
	}
}

func TestValidateK8sResourceName(t *testing.T) {
	tests := []struct {
		name    string
		nameStr string
		wantErr bool
	}{
		{
			name:    "valid simple name",
			nameStr: "my-pod",
			wantErr: false,
		},
		{
			name:    "valid name with numbers",
			nameStr: "pod-123",
			wantErr: false,
		},
		{
			name:    "valid name starting with number",
			nameStr: "1pod",
			wantErr: false,
		},
		{
			name:    "valid name with multiple hyphens",
			nameStr: "my-app-pod",
			wantErr: false,
		},
		{
			name:    "empty name",
			nameStr: "",
			wantErr: true,
		},
		{
			name:    "name with uppercase",
			nameStr: "MyPod",
			wantErr: true,
		},
		{
			name:    "name with underscore",
			nameStr: "my_pod",
			wantErr: true,
		},
		{
			name:    "name with special characters",
			nameStr: "my@pod",
			wantErr: true,
		},
		{
			name:    "name with leading whitespace",
			nameStr: " mypod",
			wantErr: true,
		},
		{
			name:    "name with trailing whitespace",
			nameStr: "mypod ",
			wantErr: true,
		},
		{
			name:    "name with null byte",
			nameStr: "mypod\x00",
			wantErr: true,
		},
		{
			name:    "name too long (254 chars)",
			nameStr: string(make([]byte, 254)),
			wantErr: true,
		},
		{
			name:    "name at max length (253 chars)",
			nameStr: "a" + string(make([]byte, 252)),
			wantErr: true,
		},
		{
			name:    "name starting with hyphen",
			nameStr: "-mypod",
			wantErr: true,
		},
		{
			name:    "name ending with hyphen",
			nameStr: "mypod-",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateK8sResourceName(tt.nameStr)
			if tt.wantErr {
				assert.Error(t, err, tt.nameStr)
			} else {
				assert.NoError(t, err, tt.nameStr)
			}
		})
	}
}

func TestValidateK8sNamespace(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		wantErr   bool
	}{
		{
			name:      "valid simple namespace",
			namespace: "default",
			wantErr:   false,
		},
		{
			name:      "valid namespace with numbers",
			namespace: "ns-123",
			wantErr:   false,
		},
		{
			name:      "valid namespace starting with number",
			namespace: "1ns",
			wantErr:   false,
		},
		{
			name:      "valid namespace with multiple hyphens",
			namespace: "my-app-namespace",
			wantErr:   false,
		},
		{
			name:      "empty namespace",
			namespace: "",
			wantErr:   true,
		},
		{
			name:      "namespace with uppercase",
			namespace: "MyNamespace",
			wantErr:   true,
		},
		{
			name:      "namespace with underscore",
			namespace: "my_namespace",
			wantErr:   true,
		},
		{
			name:      "namespace with special characters",
			namespace: "my@ns",
			wantErr:   true,
		},
		{
			name:      "namespace with leading whitespace",
			namespace: " myns",
			wantErr:   true,
		},
		{
			name:      "namespace with trailing whitespace",
			namespace: "myns ",
			wantErr:   true,
		},
		{
			name:      "namespace with null byte",
			namespace: "myns\x00",
			wantErr:   true,
		},
		{
			name:      "namespace too long (64 chars)",
			namespace: string(make([]byte, 64)),
			wantErr:   true,
		},
		{
			name:      "namespace at max length (63 chars)",
			namespace: "a" + string(make([]byte, 62)),
			wantErr:   true,
		},
		{
			name:      "namespace starting with hyphen",
			namespace: "-myns",
			wantErr:   true,
		},
		{
			name:      "namespace ending with hyphen",
			namespace: "myns-",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateK8sNamespace(tt.namespace)
			if tt.wantErr {
				assert.Error(t, err, tt.namespace)
			} else {
				assert.NoError(t, err, tt.namespace)
			}
		})
	}
}

func TestValidateCloudMetadataOperation(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		wantErr   bool
	}{
		{
			name:      "valid detect operation",
			operation: "detect",
			wantErr:   false,
		},
		{
			name:      "valid instance operation",
			operation: "instance",
			wantErr:   false,
		},
		{
			name:      "valid region operation",
			operation: "region",
			wantErr:   false,
		},
		{
			name:      "valid availability_zone operation",
			operation: "availability_zone",
			wantErr:   false,
		},
		{
			name:      "valid instance_type operation",
			operation: "instance_type",
			wantErr:   false,
		},
		{
			name:      "valid all operation",
			operation: "all",
			wantErr:   false,
		},
		{
			name:      "invalid operation",
			operation: "invalid",
			wantErr:   true,
		},
		{
			name:      "empty operation",
			operation: "",
			wantErr:   true,
		},
		{
			name:      "operation with injection attempt",
			operation: "detect; rm -rf /",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCloudMetadataOperation(tt.operation)
			if tt.wantErr {
				assert.Error(t, err, tt.operation)
			} else {
				assert.NoError(t, err, tt.operation)
			}
		})
	}
}

func TestValidateProcNetPath(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		wantErr  bool
	}{
		{
			name:     "valid tcp",
			protocol: "tcp",
			wantErr:  false,
		},
		{
			name:     "valid udp",
			protocol: "udp",
			wantErr:  false,
		},
		{
			name:     "valid tcp6",
			protocol: "tcp6",
			wantErr:  false,
		},
		{
			name:     "valid udp6",
			protocol: "udp6",
			wantErr:  false,
		},
		{
			name:     "valid raw",
			protocol: "raw",
			wantErr:  false,
		},
		{
			name:     "invalid protocol",
			protocol: "icmp",
			wantErr:  true,
		},
		{
			name:     "empty protocol",
			protocol: "",
			wantErr:  true,
		},
		{
			name:     "invalid uppercase",
			protocol: "TCP",
			wantErr:  true,
		},
		{
			name:     "protocol with injection",
			protocol: "tcp; rm -rf /",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProcNetPath(tt.protocol)
			if tt.wantErr {
				assert.Error(t, err, tt.protocol)
			} else {
				assert.NoError(t, err, tt.protocol)
			}
		})
	}
}

func TestSetPrivateIPAllowlist(t *testing.T) {
	t.Cleanup(func() {
		resetPrivateIPAllowlistForTest()
	})

	t.Run("valid CIDRs", func(t *testing.T) {
		setPrivateIPAllowlistForTest(t, []string{"10.43.0.0/24", "127.0.0.0/8"})
	})

	t.Run("empty slice resets allowlist", func(t *testing.T) {
		resetPrivateIPAllowlistForTest()
		assert.False(t, isIPAllowed(net.ParseIP("10.43.0.40")))
	})
}

func TestIsIPAllowed(t *testing.T) {
	t.Cleanup(func() {
		resetPrivateIPAllowlistForTest()
	})

	setPrivateIPAllowlistForTest(t, []string{"10.43.0.0/24", "127.0.0.0/8"})

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"allowed private 10.43.0.40", "10.43.0.40", true},
		{"allowed loopback 127.0.0.1", "127.0.0.1", true},
		{"not allowed private 10.42.0.1", "10.42.0.1", false},
		{"not allowed private 192.168.1.1", "192.168.1.1", false},
		{"not allowed public 8.8.8.8", "8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip)
			assert.Equal(t, tt.expected, isIPAllowed(ip))
		})
	}
}

func TestValidateHTTPRequestURL_WithAllowlist(t *testing.T) {
	t.Cleanup(func() {
		resetPrivateIPAllowlistForTest()
	})

	setPrivateIPAllowlistForTest(t, []string{"10.43.0.0/24"})

	t.Run("allowlisted private IP passes", func(t *testing.T) {
		parsed, err := validateHTTPRequestURL("http://10.43.0.40:9000/slew")
		assert.NoError(t, err)
		assert.NotNil(t, parsed)
	})

	t.Run("non-allowlisted private IP still blocked", func(t *testing.T) {
		_, err := validateHTTPRequestURL("http://10.42.0.1")
		assert.Error(t, err)
	})

	t.Run("non-allowlisted loopback still blocked", func(t *testing.T) {
		_, err := validateHTTPRequestURL("http://127.0.0.1:8080")
		assert.Error(t, err)
	})

	t.Run("public URL still allowed", func(t *testing.T) {
		parsed, err := validateHTTPRequestURL("http://example.com")
		assert.NoError(t, err)
		assert.NotNil(t, parsed)
	})
}

func TestValidateHTTPRequestURL_DefaultBlocksPrivate(t *testing.T) {
	t.Cleanup(func() {
		resetPrivateIPAllowlistForTest()
	})

	resetPrivateIPAllowlistForTest()

	_, err := validateHTTPRequestURL("http://10.43.0.40")
	assert.Error(t, err)

	_, err = validateHTTPRequestURL("http://127.0.0.1")
	assert.Error(t, err)
}
