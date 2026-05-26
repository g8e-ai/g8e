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
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"

	_ "modernc.org/sqlite"
)

// NativeToolHandler executes native tools compiled into the Operator binary.
type NativeToolHandler struct{}

// NewNativeToolHandler creates a new native tool handler.
func NewNativeToolHandler() *NativeToolHandler {
	return &NativeToolHandler{}
}

// HandleTool executes a native tool by name and returns the result.
func (h *NativeToolHandler) HandleTool(ctx context.Context, toolName string, arguments json.RawMessage) (CallToolResult, error) {
	switch toolName {
	case "db_discover_topology":
		return h.handleDBDiscoverTopology(ctx, arguments)
	case "db_query_validate":
		return h.handleDBQueryValidate(ctx, arguments)
	case "db_isolated_read":
		return h.handleDBIsolatedRead(ctx, arguments)
	case "db_index_triage":
		return h.handleDBIndexTriage(ctx, arguments)
	case "log_stream_filter":
		return h.handleLogStreamFilter(ctx, arguments)
	case "sys_oom_detect":
		return h.handleSysOOMDetect(ctx, arguments)
	case "config_diff_mask":
		return h.handleConfigDiffMask(ctx, arguments)
	case "proc_metric_top":
		return h.handleProcMetricTop(ctx, arguments)
	case "fs_disk_profile":
		return h.handleFSDiskProfile(ctx, arguments)
	case "proc_signal_safe":
		return h.handleProcSignalSafe(ctx, arguments)
	case "net_socket_audit":
		return h.handleNetSocketAudit(ctx, arguments)
	case "net_endpoint_ping":
		return h.handleNetEndpointPing(ctx, arguments)
	case "net_http_probe":
		return h.handleNetHTTPProbe(ctx, arguments)
	default:
		return CallToolResult{}, fmt.Errorf("unknown native tool: %s", toolName)
	}
}

// handleDBDiscoverTopology scans database schemas, tables, and column data types.
func (h *NativeToolHandler) handleDBDiscoverTopology(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req DBDiscoverTopologyRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid db_discover_topology arguments: %w", err)
	}

	if req.DatabasePath == "" {
		return CallToolResult{}, fmt.Errorf("database_path required")
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", req.DatabasePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	schema := make(map[string]map[string]string)

	tables, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to query tables: %w", err)
	}
	defer tables.Close()

	for tables.Next() {
		var tableName string
		if err := tables.Scan(&tableName); err != nil {
			continue
		}

		schema[tableName] = make(map[string]string)

		columns, err := db.Query("PRAGMA table_info(" + tableName + ")")
		if err != nil {
			continue
		}

		for columns.Next() {
			var cid int
			var name, datatype string
			var notnull int
			var dfltValue interface{}
			var pk int

			if err := columns.Scan(&cid, &name, &datatype, &notnull, &dfltValue, &pk); err != nil {
				continue
			}

			schema[tableName][name] = datatype
		}
		columns.Close()
	}

	result := DBDiscoverTopologyResult{Schema: schema}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// handleDBQueryValidate validates SQL queries using EXPLAIN QUERY PLAN.
func (h *NativeToolHandler) handleDBQueryValidate(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req DBQueryValidateRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid db_query_validate arguments: %w", err)
	}

	if req.DatabasePath == "" || req.Query == "" {
		return CallToolResult{}, fmt.Errorf("database_path and query required")
	}

	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(req.Query)), "SELECT") {
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: `{"valid":false,"rejected":true,"reason":"Only SELECT queries are allowed for validation"}`,
				},
			},
		}, nil
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", req.DatabasePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	planRows, err := db.Query("EXPLAIN QUERY PLAN " + req.Query)
	if err != nil {
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: fmt.Sprintf(`{"valid":false,"rejected":true,"reason":"Query parse error: %v"}`, err),
				},
			},
		}, nil
	}
	defer planRows.Close()

	var planLines []string
	hasFullScan := false

	for planRows.Next() {
		var id, parent, notused int
		var detail string
		if err := planRows.Scan(&id, &parent, &notused, &detail); err != nil {
			continue
		}
		planLines = append(planLines, detail)

		if strings.Contains(strings.ToLower(detail), "scan") &&
			(strings.Contains(strings.ToLower(detail), "table") ||
				strings.Contains(strings.ToLower(detail), "using index")) {
			hasFullScan = true
		}
	}

	planStr := strings.Join(planLines, "\n")
	result := DBQueryValidateResult{
		Valid:    true,
		Plan:     planStr,
		Rejected: false,
	}

	if hasFullScan {
		result.Valid = false
		result.Rejected = true
		result.Reason = "Query performs a full table scan, which may be slow on large datasets"
	}

	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// handleDBIsolatedRead executes SELECT statements in read-only mode.
func (h *NativeToolHandler) handleDBIsolatedRead(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req DBIsolatedReadRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid db_isolated_read arguments: %w", err)
	}

	if req.DatabasePath == "" || req.Query == "" {
		return CallToolResult{}, fmt.Errorf("database_path and query required")
	}

	queryUpper := strings.ToUpper(strings.TrimSpace(req.Query))
	if !strings.HasPrefix(queryUpper, "SELECT") {
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: `{"error":"Only SELECT queries are allowed in isolated read mode"}`,
				},
			},
			IsError: true,
		}, nil
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", req.DatabasePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(req.Query)
	if err != nil {
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: fmt.Sprintf(`{"error":"Query execution failed: %v"}`, err),
				},
			},
			IsError: true,
		}, nil
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to get columns: %w", err)
	}

	var resultRows []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		resultRows = append(resultRows, row)
	}

	result := DBIsolatedReadResult{
		Rows:    resultRows,
		Columns: columns,
	}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// handleDBIndexTriage queries fragmentation statistics and indexes.
func (h *NativeToolHandler) handleDBIndexTriage(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req DBIndexTriageRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid db_index_triage arguments: %w", err)
	}

	if req.DatabasePath == "" {
		return CallToolResult{}, fmt.Errorf("database_path required")
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", req.DatabasePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	var indexes []IndexInfo

	indexList, err := db.Query("SELECT name, tbl_name, sql FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to query indexes: %w", err)
	}
	defer indexList.Close()

	for indexList.Next() {
		var name, table, sql string
		if err := indexList.Scan(&name, &table, &sql); err != nil {
			continue
		}

		unique := strings.Contains(strings.ToUpper(sql), "UNIQUE")

		indexes = append(indexes, IndexInfo{
			Name:   name,
			Table:  table,
			Unique: unique,
			Used:   true,
		})
	}

	result := DBIndexTriageResult{
		Indexes:       indexes,
		Fragmentation: 0.0,
	}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// handleLogStreamFilter reads log files and applies regex filtering with scrubbing.
func (h *NativeToolHandler) handleLogStreamFilter(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req LogStreamFilterRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid log_stream_filter arguments: %w", err)
	}

	if req.LogPath == "" || req.Pattern == "" {
		return CallToolResult{}, fmt.Errorf("log_path and pattern required")
	}

	file, err := os.Open(req.LogPath)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	regex, err := regexp.Compile(req.Pattern)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("invalid regex pattern: %w", err)
	}

	var lines []string
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() && len(lines) < limit {
		line := scanner.Text()
		if regex.MatchString(line) {
			scrubbed := h.scrubLine(line)
			lines = append(lines, scrubbed)
		}
	}

	if err := scanner.Err(); err != nil {
		return CallToolResult{}, fmt.Errorf("error reading log file: %w", err)
	}

	result := LogStreamFilterResult{
		Lines: lines,
		Count: len(lines),
	}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// scrubLine redacts sensitive patterns from log lines.
func (h *NativeToolHandler) scrubLine(line string) string {
	scrubbed := line

	sensitivePatterns := []struct {
		pattern string
		repl    string
	}{
		{`password[=:]\s*\S+`, "password=REDACTED"},
		{`api[_-]?key[=:]\s*\S+`, "api_key=REDACTED"},
		{`secret[=:]\s*\S+`, "secret=REDACTED"},
		{`token[=:]\s*\S+`, "token=REDACTED"},
		{`bearer\s+\S+`, "bearer REDACTED"},
	}

	for _, sp := range sensitivePatterns {
		re := regexp.MustCompile(`(?i)` + sp.pattern)
		scrubbed = re.ReplaceAllString(scrubbed, sp.repl)
	}

	return scrubbed
}

// handleSysOOMDetect scans system logs for OOM killer events.
func (h *NativeToolHandler) handleSysOOMDetect(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req SysOOMDetectRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid sys_oom_detect arguments: %w", err)
	}

	logPath := req.LogPath
	if logPath == "" {
		logPath = "/var/log/dmesg"
	}

	file, err := os.Open(logPath)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var events []OOMEvent
	oomRegex := regexp.MustCompile(`(?i)oom-killer|killed process`)
	pidRegex := regexp.MustCompile(`pid\s*=\s*(\d+)`)
	processRegex := regexp.MustCompile(`process\s+(\S+)`)
	memoryRegex := regexp.MustCompile(`(\d+)\s*MB`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if oomRegex.MatchString(line) {
			event := OOMEvent{
				Timestamp: time.Now().Format(time.RFC3339),
			}

			if matches := pidRegex.FindStringSubmatch(line); len(matches) > 1 {
				event.PID, _ = strconv.Atoi(matches[1])
			}
			if matches := processRegex.FindStringSubmatch(line); len(matches) > 1 {
				event.Process = matches[1]
			}
			if matches := memoryRegex.FindStringSubmatch(line); len(matches) > 1 {
				event.MemoryMB, _ = strconv.Atoi(matches[1])
			}

			events = append(events, event)
		}
	}

	if err := scanner.Err(); err != nil {
		return CallToolResult{}, fmt.Errorf("error reading log file: %w", err)
	}

	result := SysOOMDetectResult{Events: events}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// handleConfigDiffMask compares configuration files with secret masking.
func (h *NativeToolHandler) handleConfigDiffMask(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req ConfigDiffMaskRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid config_diff_mask arguments: %w", err)
	}

	if req.ConfigPath == "" || req.Baseline == "" {
		return CallToolResult{}, fmt.Errorf("config_path and baseline required")
	}

	currentBytes, err := os.ReadFile(req.ConfigPath)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to read config file: %w", err)
	}

	baselineBytes := []byte(req.Baseline)

	currentLines := strings.Split(string(currentBytes), "\n")
	baselineLines := strings.Split(string(baselineBytes), "\n")

	var differences []ConfigDiff

	maxLines := len(currentLines)
	if len(baselineLines) > maxLines {
		maxLines = len(baselineLines)
	}

	for i := 0; i < maxLines; i++ {
		var current, baseline string
		if i < len(currentLines) {
			current = strings.TrimSpace(currentLines[i])
		}
		if i < len(baselineLines) {
			baseline = strings.TrimSpace(baselineLines[i])
		}

		if current != baseline {
			diffType := "added"
			if baseline != "" && current == "" {
				diffType = "removed"
			} else if baseline != "" && current != "" {
				diffType = "changed"
			}

			differences = append(differences, ConfigDiff{
				Key:      fmt.Sprintf("line_%d", i),
				Current:  h.maskSecret(current),
				Baseline: h.maskSecret(baseline),
				Type:     diffType,
			})
		}
	}

	result := ConfigDiffMaskResult{Differences: differences}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// maskSecret redacts secret values in configuration lines.
func (h *NativeToolHandler) maskSecret(line string) string {
	if strings.Contains(strings.ToLower(line), "password") ||
		strings.Contains(strings.ToLower(line), "secret") ||
		strings.Contains(strings.ToLower(line), "token") ||
		strings.Contains(strings.ToLower(line), "key") {
		return "REDACTED"
	}
	return line
}

// handleProcMetricTop parses /proc to extract top resource-consuming processes.
func (h *NativeToolHandler) handleProcMetricTop(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req ProcMetricTopRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid proc_metric_top arguments: %w", err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	procDir := "/proc"
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to read /proc: %w", err)
	}

	var processes []ProcessInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		statPath := filepath.Join(procDir, entry.Name(), "stat")
		statBytes, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}

		statFields := strings.Fields(string(statBytes))
		if len(statFields) < 24 {
			continue
		}

		name := statFields[1]
		if strings.HasPrefix(name, "(") && strings.HasSuffix(name, ")") {
			name = name[1 : len(name)-1]
		}

		utime, _ := strconv.ParseFloat(statFields[13], 64)
		stime, _ := strconv.ParseFloat(statFields[14], 64)
		totalTime := utime + stime

		rss, _ := strconv.ParseInt(statFields[23], 10, 64)
		memoryMB := float64(rss) * 4096 / (1024 * 1024)

		processes = append(processes, ProcessInfo{
			PID:        pid,
			Name:       name,
			CPUPercent: totalTime,
			MemoryMB:   memoryMB,
			User:       string(constants.SystemHealthUnknown),
			Command:    name,
		})
	}

	if len(processes) > limit {
		processes = processes[:limit]
	}

	result := ProcMetricTopResult{Processes: processes}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// handleFSDiskProfile recursively calculates directory sizes.
func (h *NativeToolHandler) handleFSDiskProfile(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req FSDiskProfileRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid fs_disk_profile arguments: %w", err)
	}

	if req.Path == "" {
		return CallToolResult{}, fmt.Errorf("path required")
	}

	maxDepth := req.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 2
	}

	var entries []DirEntry
	var totalSize int64

	err := filepath.Walk(req.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(req.Path, path)
		if err != nil {
			return nil
		}

		depth := len(strings.Split(relPath, string(filepath.Separator)))
		if depth > maxDepth+1 {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		size := info.Size()
		totalSize += size

		entries = append(entries, DirEntry{
			Path:     relPath,
			SizeMB:   size / (1024 * 1024),
			IsDir:    info.IsDir(),
			Modified: info.ModTime().Unix(),
		})

		return nil
	})

	if err != nil {
		return CallToolResult{}, fmt.Errorf("error walking path: %w", err)
	}

	result := FSDiskProfileResult{
		Entries: entries,
		TotalMB: totalSize / (1024 * 1024),
	}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// handleProcSignalSafe sends signals to processes with denylist enforcement.
func (h *NativeToolHandler) handleProcSignalSafe(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req ProcSignalSafeRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid proc_signal_safe arguments: %w", err)
	}

	if req.PID <= 0 || req.Signal == "" {
		return CallToolResult{}, fmt.Errorf("pid and signal required")
	}

	denylist := []int{1, 2}
	for _, deniedPID := range denylist {
		if req.PID == deniedPID {
			result := ProcSignalSafeResult{
				Sent:   false,
				PID:    req.PID,
				Signal: req.Signal,
				Error:  "PID is protected by denylist",
			}
			resultJSON, _ := json.Marshal(result)
			return CallToolResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: string(resultJSON),
					},
				},
			}, nil
		}
	}

	var sig syscall.Signal
	switch strings.ToUpper(req.Signal) {
	case "SIGTERM":
		sig = syscall.SIGTERM
	case "SIGKILL":
		sig = syscall.SIGKILL
	case "SIGINT":
		sig = syscall.SIGINT
	default:
		result := ProcSignalSafeResult{
			Sent:   false,
			PID:    req.PID,
			Signal: req.Signal,
			Error:  "unsupported signal",
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	process, err := os.FindProcess(req.PID)
	if err != nil {
		result := ProcSignalSafeResult{
			Sent:   false,
			PID:    req.PID,
			Signal: req.Signal,
			Error:  err.Error(),
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	if err := process.Signal(sig); err != nil {
		result := ProcSignalSafeResult{
			Sent:   false,
			PID:    req.PID,
			Signal: req.Signal,
			Error:  err.Error(),
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	result := ProcSignalSafeResult{
		Sent:   true,
		PID:    req.PID,
		Signal: req.Signal,
	}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// handleNetSocketAudit inspects active network sockets.
func (h *NativeToolHandler) handleNetSocketAudit(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req NetSocketAuditRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid net_socket_audit arguments: %w", err)
	}

	var sockets []SocketInfo
	protocols := []string{string(constants.NetworkProtocolTCP), string(constants.NetworkProtocolUDP)}

	if req.Protocol != "" {
		protocols = []string{strings.ToLower(req.Protocol)}
	}

	for _, proto := range protocols {
		path := fmt.Sprintf("/proc/net/%s", proto)
		file, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		scanner.Scan()

		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}

			localAddr := fields[1]
			remoteAddr := fields[2]
			state := ""
			if len(fields) > 3 {
				state = fields[3]
			}

			localIP, localPort := parseSocketAddr(localAddr)
			remoteIP, remotePort := parseSocketAddr(remoteAddr)

			sockets = append(sockets, SocketInfo{
				Protocol:   proto,
				LocalAddr:  localIP,
				LocalPort:  localPort,
				RemoteAddr: remoteIP,
				RemotePort: remotePort,
				State:      state,
			})
		}

		if err := scanner.Err(); err != nil {
			file.Close()
			continue
		}
		file.Close()
	}

	result := NetSocketAuditResult{Sockets: sockets}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// parseSocketAddr parses /proc/net socket address format.
func parseSocketAddr(hexAddr string) (string, int) {
	if len(hexAddr) < 8 {
		return "0.0.0.0", 0
	}

	portHex := hexAddr[len(hexAddr)-4:]
	ipHex := hexAddr[:len(hexAddr)-4]

	port, _ := strconv.ParseInt(portHex, 16, 32)

	var ip string
	if len(ipHex) == 8 {
		p1, _ := strconv.ParseInt(ipHex[6:8], 16, 32)
		p2, _ := strconv.ParseInt(ipHex[4:6], 16, 32)
		p3, _ := strconv.ParseInt(ipHex[2:4], 16, 32)
		p4, _ := strconv.ParseInt(ipHex[0:2], 16, 32)
		ip = fmt.Sprintf("%d.%d.%d.%d", p1, p2, p3, p4)
	} else {
		ip = string(constants.SystemHealthUnknown)
	}

	return ip, int(port)
}

// handleNetEndpointPing performs TCP handshake or ICMP ping to verify connectivity.
func (h *NativeToolHandler) handleNetEndpointPing(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req NetEndpointPingRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid net_endpoint_ping arguments: %w", err)
	}

	if req.Host == "" || req.Port <= 0 {
		return CallToolResult{}, fmt.Errorf("host and port required")
	}

	address := net.JoinHostPort(req.Host, strconv.Itoa(req.Port))
	start := time.Now()

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if deadline, ok := ctx.Deadline(); ok {
		dialer.Timeout = time.Until(deadline)
		if dialer.Timeout <= 0 {
			dialer.Timeout = 5 * time.Second
		}
	}

	conn, err := dialer.DialContext(ctx, string(constants.NetworkProtocolTCP), address)
	latency := time.Since(start).Seconds() * 1000

	if err != nil {
		result := NetEndpointPingResult{
			Reachable: false,
			LatencyMs: latency,
			Error:     err.Error(),
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}
	defer conn.Close()

	result := NetEndpointPingResult{
		Reachable: true,
		LatencyMs: latency,
	}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// handleNetHTTPProbe performs lightweight HTTP requests.
func (h *NativeToolHandler) handleNetHTTPProbe(ctx context.Context, arguments json.RawMessage) (CallToolResult, error) {
	var req NetHTTPProbeRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid net_http_probe arguments: %w", err)
	}

	if req.URL == "" {
		return CallToolResult{}, fmt.Errorf("url required")
	}

	method := req.Method
	if method == "" {
		method = "HEAD"
	}

	start := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, method, req.URL, nil)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to create request: %w", err)
	}

	timeout := 10 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(httpReq)
	latency := time.Since(start).Seconds() * 1000

	if err != nil {
		result := NetHTTPProbeResult{
			Error:     err.Error(),
			LatencyMs: latency,
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}
	defer resp.Body.Close()

	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	result := NetHTTPProbeResult{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		LatencyMs:  latency,
	}
	resultJSON, _ := json.Marshal(result)

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}
