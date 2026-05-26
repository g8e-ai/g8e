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

// NativeTools returns the list of native tools compiled into the Operator binary.
// These tools execute within the Operator's execution boundary locally,
// without proxying to downstream MCP servers.
func NativeTools() []Tool {
	return []Tool{
		{
			Name:        "db_discover_topology",
			Description: "Automatically scans database schemas, tables, and column data types, returning a highly compressed JSON map. AI agents need this first to prevent hallucinated queries.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the SQLite database file",
					},
				},
				"required": []string{"database_path"},
			},
		},
		{
			Name:        "db_query_validate",
			Description: "Intercepts any AI-generated SQL and runs it through EXPLAIN QUERY PLAN natively. If the engine flags an unindexed, full-table scan on a production dataset, the binary rejects the task before execution.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the SQLite database file",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "SQL query to validate",
					},
				},
				"required": []string{"database_path", "query"},
			},
		},
		{
			Name:        "db_isolated_read",
			Description: "Executes SELECT statements using a database handle opened strictly with SQLITE_OPEN_READONLY. This prevents the AI from executing destructive injections (e.g., ; DROP TABLE...).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the SQLite database file",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "SELECT query to execute",
					},
				},
				"required": []string{"database_path", "query"},
			},
		},
		{
			Name:        "db_index_triage",
			Description: "Queries internal fragmentation statistics and indexes to diagnose slow queries without letting the AI guess the performance bottleneck.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"database_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the SQLite database file",
					},
				},
				"required": []string{"database_path"},
			},
		},
		{
			Name:        "log_stream_filter",
			Description: "Reads native log paths or standard buffers, applies a regex match requested by the AI, runs the matched chunks through the scrubbing engine to redact secrets/PII, and pushes only the sanitized fragments.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"log_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the log file",
					},
					"pattern": map[string]interface{}{
						"type":        "string",
						"description": "Regex pattern to match",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of lines to return",
					},
				},
				"required": []string{"log_path", "pattern"},
			},
		},
		{
			Name:        "sys_oom_detect",
			Description: "Directly parses /var/log/dmesg or system logs to scan for Out-Of-Memory (OOM) killer events, process kills, or core panic dumps, isolating the exact failing PID.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"log_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the system log file (optional, defaults to /var/log/dmesg)",
					},
				},
			},
		},
		{
			Name:        "config_diff_mask",
			Description: "Compares application configuration states against environmental baselines. It strips out actual passwords, tokens, and salts inside the binary before outputting the structural differences to the AI.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"config_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the current configuration file",
					},
					"baseline": map[string]interface{}{
						"type":        "string",
						"description": "Baseline configuration to compare against",
					},
				},
				"required": []string{"config_path", "baseline"},
			},
		},
		{
			Name:        "proc_metric_top",
			Description: "Directly parses the Linux /proc filesystem in memory to extract process IDs, memory maps, and CPU tracking. It returns a tightly structured JSON array of the top resource-hogging processes.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of processes to return",
					},
				},
			},
		},
		{
			Name:        "fs_disk_profile",
			Description: "Recursively calculates directory sizes natively (equivalent to an optimized du --max-depth=2) starting from an approved path root. It instantly isolates unrotated log files or bloated tmp directories.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Root path to profile",
					},
					"max_depth": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum directory depth to traverse",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "proc_signal_safe",
			Description: "Allows the AI to send explicit termination signals (SIGTERM, SIGKILL) to a process, but enforces a strict binary-level denylist (e.g., rejecting attempts to kill PID 1, system init, or the operator binary itself).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"pid": map[string]interface{}{
						"type":        "integer",
						"description": "Process ID to signal",
					},
					"signal": map[string]interface{}{
						"type":        "string",
						"description": "Signal name (e.g., SIGTERM, SIGKILL)",
					},
				},
				"required": []string{"pid", "signal"},
			},
		},
		{
			Name:        "net_socket_audit",
			Description: "Directly inspects active network sockets (/proc/net/tcp and /proc/net/udp) to map established connections and confirm if expected internal microservices are actually listening.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"protocol": map[string]interface{}{
						"type":        "string",
						"description": "Protocol filter (tcp, udp, or empty for both)",
					},
				},
			},
		},
		{
			Name:        "net_endpoint_ping",
			Description: "Initiates native TCP handshakes or ICMP requests to defined target host/port combinations to verify local network routing and DNS resolution performance.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"host": map[string]interface{}{
						"type":        "string",
						"description": "Target hostname or IP address",
					},
					"port": map[string]interface{}{
						"type":        "integer",
						"description": "Target port number",
					},
				},
				"required": []string{"host", "port"},
			},
		},
		{
			Name:        "net_http_probe",
			Description: "Performs a lightweight native HTTP request (similar to curl -I) to internal API endpoints, returning only the status codes, headers, and latency metrics while discarding heavy response payloads.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "Target URL",
					},
					"method": map[string]interface{}{
						"type":        "string",
						"description": "HTTP method (defaults to HEAD)",
					},
				},
				"required": []string{"url"},
			},
		},
	}
}
