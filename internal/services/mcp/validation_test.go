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
	"testing"
)

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
				if err == nil {
					t.Errorf("validateHTTPRequestURL(%q) expected error, got nil", tt.url)
				}
				if parsed != nil {
					t.Errorf("validateHTTPRequestURL(%q) expected nil parsed URL on error, got %v", tt.url, parsed)
				}
				return
			}
			if err != nil {
				t.Errorf("validateHTTPRequestURL(%q) unexpected error: %v", tt.url, err)
			}
			if parsed == nil {
				t.Errorf("validateHTTPRequestURL(%q) expected non-nil parsed URL", tt.url)
			}
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
				if err == nil {
					t.Errorf("validateSQLQuery(%q) expected error, got nil", tt.query)
				}
			} else {
				if err != nil {
					t.Errorf("validateSQLQuery(%q) unexpected error: %v", tt.query, err)
				}
			}
		})
	}
}
