// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// JSONRPCRequest is an alias for response.JSONRPCRequest
type JSONRPCRequest = response.JSONRPCRequest

// JSONRPCResponse is an alias for response.JSONRPCResponse
type JSONRPCResponse = response.JSONRPCResponse

// JSONRPCError is an alias for response.JSONRPCError
type JSONRPCError = response.JSONRPCError

// CallToolRequest is the params for the "tools/call" method.
type CallToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// CallToolResult is the result for the "tools/call" method.
type CallToolResult struct {
	Content []TextContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// TextContent represents a text part of a tool response.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SuspendedTransaction is an alias for the shared models type.
type SuspendedTransaction = models.SuspendedTransaction

// ListResourcesRequest is the params for the "resources/list" method.
type ListResourcesRequest struct {
	// Optional cursor for pagination
	Cursor *string `json:"cursor,omitempty"`
}

// Resource represents an MCP resource.
type Resource struct {
	URI         string    `json:"uri"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	MimeType    string    `json:"mimeType,omitempty"`
	Metadata    *Metadata `json:"metadata,omitempty"`
}

// ReadResourceRequest is the params for the "resources/read" method.
type ReadResourceRequest struct {
	URI string `json:"uri"`
}

// ReadResourceResult is the result for the "resources/read" method.
type ReadResourceResult struct {
	Contents []TextContent `json:"contents"`
	MIMEType string        `json:"mimeType,omitempty"`
}

// ListPromptsRequest is the params for the "prompts/list" method.
type ListPromptsRequest struct {
	// Optional cursor for pagination
	Cursor *string `json:"cursor,omitempty"`
}

// Prompt represents an MCP prompt template.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Metadata    *Metadata        `json:"metadata,omitempty"`
}

// PromptArgument represents an argument for a prompt template.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ResourceTemplate represents an MCP resource template with a URI pattern.
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

// Metadata represents typed metadata for MCP resources and prompts.
type Metadata struct {
	Custom map[string]string `json:"custom,omitempty"`
}

// GetPromptRequest is the params for the "prompts/get" method.
type GetPromptRequest struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

// GetPromptResult is the result for the "prompts/get" method.
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages,omitempty"`
}

// PromptMessage represents a message in a prompt template.
type PromptMessage struct {
	Role    string      `json:"role"`
	Content TextContent `json:"content"`
}

// ToolsListResult is the result for the "tools/list" method.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// Tool represents an MCP tool.
type Tool struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	InputSchema *InputSchema `json:"inputSchema,omitempty"`
}

// ResourcesListResult is the result for the "resources/list" method.
type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

// PromptsListResult is the result for the "prompts/list" method.
type PromptsListResult struct {
	Prompts []Prompt `json:"prompts"`
}

// A2ASuspensionResponse is returned when an A2A call is suspended for L3 approval.
type A2ASuspensionResponse struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	TxHash      string `json:"tx_hash"`
	ApprovalURL string `json:"approval_url"`
	Message     string `json:"message"`
}

// A2ASuccessResponse is returned when an A2A call succeeds.
type A2ASuccessResponse struct {
	ID     string                    `json:"id"`
	Result *operatorv1.ActionReceipt `json:"result"`
}

// A2ACallRequest is the params for the "a2a/call" method.
type A2ACallRequest struct {
	SkillName   string          `json:"skill_name"`
	Payload     json.RawMessage `json:"payload"`
	ExecutionID string          `json:"execution_id,omitempty"`
}

// A2ADownstreamRequest is the request sent to a downstream A2A server.
type A2ADownstreamRequest struct {
	SkillName   string          `json:"skill_name"`
	Payload     json.RawMessage `json:"payload"`
	ExecutionID string          `json:"execution_id,omitempty"`
}

// FieldReadRequest is the params for the "read_field" tool.
type FieldReadRequest struct {
	Collection        string `json:"collection"`
	DocumentID        string `json:"document_id"`
	FieldPath         string `json:"field_path"`
	OperatorSessionID string `json:"operator_session_id"`
}

// FieldReadResult is the result for the "read_field" tool.
type FieldReadResult struct {
	Value FieldValue `json:"value"`
}

// FieldValue represents a typed field value from document storage.
type FieldValue struct {
	Str     *string               `json:"string,omitempty"`
	Int64   *int64                `json:"int64,omitempty"`
	Float64 *float64              `json:"float64,omitempty"`
	Bool    *bool                 `json:"bool,omitempty"`
	Array   []FieldValue          `json:"array,omitempty"`
	Object  map[string]FieldValue `json:"object,omitempty"`
	Null    bool                  `json:"null"`
}

// String returns a human-readable representation of the field value,
// suitable for display in MCP text content and audit log entries.
// Implements fmt.Stringer interface.
func (v FieldValue) String() string {
	switch {
	case v.Null:
		return "null"
	case v.Str != nil:
		return *v.Str
	case v.Int64 != nil:
		return fmt.Sprintf("%d", *v.Int64)
	case v.Float64 != nil:
		return fmt.Sprintf("%g", *v.Float64)
	case v.Bool != nil:
		return fmt.Sprintf("%t", *v.Bool)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// Native tool definitions compiled into the Node binary

// DBDiscoverTopologyRequest is the params for the "db_discover_topology" tool.
type DBDiscoverTopologyRequest struct {
	DatabasePath string `json:"database_path"`
}

// DBDiscoverTopologyResult is the result for the "db_discover_topology" tool.
type DBDiscoverTopologyResult struct {
	Schema map[string]map[string]string `json:"schema"`
}

// DBQueryValidateRequest is the params for the "db_query_validate" tool.
type DBQueryValidateRequest struct {
	DatabasePath string `json:"database_path"`
	Query        string `json:"query"`
}

// DBQueryValidateResult is the result for the "db_query_validate" tool.
type DBQueryValidateResult struct {
	Valid    bool   `json:"valid"`
	Plan     string `json:"plan,omitempty"`
	Warning  string `json:"warning,omitempty"`
	Rejected bool   `json:"rejected"`
	Reason   string `json:"reason,omitempty"`
}

// DBIsolatedReadRequest is the params for the "db_isolated_read" tool.
type DBIsolatedReadRequest struct {
	DatabasePath string `json:"database_path"`
	Query        string `json:"query"`
}

// DBIsolatedReadResult is the result for the "db_isolated_read" tool.
type DBIsolatedReadResult struct {
	Rows    []DBRow  `json:"rows"`
	Columns []string `json:"columns"`
	Error   string   `json:"error,omitempty"`
}

// DBRow represents a single database row with typed values.
type DBRow struct {
	Values map[string]DBValue `json:"values"`
}

// DBValue represents a typed database value.
type DBValue struct {
	String  *string  `json:"string,omitempty"`
	Int64   *int64   `json:"int64,omitempty"`
	Float64 *float64 `json:"float64,omitempty"`
	Bool    *bool    `json:"bool,omitempty"`
	Null    bool     `json:"null"`
}

// DBIndexTriageRequest is the params for the "db_index_triage" tool.
type DBIndexTriageRequest struct {
	DatabasePath string `json:"database_path"`
}

// DBIndexTriageResult is the result for the "db_index_triage" tool.
type DBIndexTriageResult struct {
	Indexes       []IndexInfo `json:"indexes"`
	Fragmentation float64     `json:"fragmentation"`
}

// IndexInfo represents database index information.
type IndexInfo struct {
	Name    string   `json:"name"`
	Table   string   `json:"table"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Used    bool     `json:"used"`
}

// LogStreamFilterRequest is the params for the "log_stream_filter" tool.
type LogStreamFilterRequest struct {
	LogPath string `json:"log_path"`
	Pattern string `json:"pattern"`
	Limit   int    `json:"limit,omitempty"`
}

// LogStreamFilterResult is the result for the "log_stream_filter" tool.
type LogStreamFilterResult struct {
	Lines []string `json:"lines"`
	Count int      `json:"count"`
}

// SysOOMDetectRequest is the params for the "sys_oom_detect" tool.
type SysOOMDetectRequest struct {
	LogPath string `json:"log_path,omitempty"`
}

// SysOOMDetectResult is the result for the "sys_oom_detect" tool.
type SysOOMDetectResult struct {
	Events []OOMEvent `json:"events"`
}

// OOMEvent represents an OOM killer event.
type OOMEvent struct {
	Timestamp string `json:"timestamp"`
	PID       int    `json:"pid"`
	Process   string `json:"process"`
	MemoryMB  int    `json:"memory_mb"`
}

// ConfigDiffMaskRequest is the params for the "config_diff_mask" tool.
type ConfigDiffMaskRequest struct {
	ConfigPath string `json:"config_path"`
	Baseline   string `json:"baseline"`
}

// ConfigDiffMaskResult is the result for the "config_diff_mask" tool.
type ConfigDiffMaskResult struct {
	Differences []ConfigDiff `json:"differences"`
}

// ConfigDiff represents a configuration difference.
type ConfigDiff struct {
	Key      string `json:"key"`
	Current  string `json:"current,omitempty"`
	Baseline string `json:"baseline,omitempty"`
	Type     string `json:"type"`
}

// ProcMetricTopRequest is the params for the "proc_metric_top" tool.
type ProcMetricTopRequest struct {
	Limit int `json:"limit,omitempty"`
}

// ProcMetricTopResult is the result for the "proc_metric_top" tool.
type ProcMetricTopResult struct {
	Processes []ProcessInfo `json:"processes"`
}

// ProcessInfo represents process information from /proc.
type ProcessInfo struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`
	User       string  `json:"user"`
	Command    string  `json:"command"`
}

// FSDiskProfileRequest is the params for the "fs_disk_profile" tool.
type FSDiskProfileRequest struct {
	Path     string `json:"path"`
	MaxDepth int    `json:"max_depth,omitempty"`
}

// FSDiskProfileResult is the result for the "fs_disk_profile" tool.
type FSDiskProfileResult struct {
	Entries []DirEntry `json:"entries"`
	TotalMB int64      `json:"total_mb"`
}

// DirEntry represents a directory entry for disk profiling.
type DirEntry struct {
	Path     string `json:"path"`
	SizeMB   int64  `json:"size_mb"`
	IsDir    bool   `json:"is_dir"`
	Modified int64  `json:"modified"`
}

// FSDiskUsageRequest is the params for the "fs_disk_usage" tool.
type FSDiskUsageRequest struct {
	Path string `json:"path,omitempty"`
}

// FSDiskUsageResult is the result for the "fs_disk_usage" tool.
type FSDiskUsageResult struct {
	Path        string           `json:"path,omitempty"`
	Filesystem  *FilesystemInfo  `json:"filesystem,omitempty"`
	Filesystems []FilesystemInfo `json:"filesystems,omitempty"`
	Count       int              `json:"count,omitempty"`
}

// FilesystemInfo represents filesystem disk usage information.
type FilesystemInfo struct {
	Path           string  `json:"path"`
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	FreeBytes      uint64  `json:"free_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedPercent    float64 `json:"used_percent"`
}

// ProcSignalSafeRequest is the params for the "proc_signal_safe" tool.
type ProcSignalSafeRequest struct {
	PID    int    `json:"pid"`
	Signal string `json:"signal"`
}

// ProcSignalSafeResult is the result for the "proc_signal_safe" tool.
type ProcSignalSafeResult struct {
	Sent   bool   `json:"sent"`
	PID    int    `json:"pid"`
	Signal string `json:"signal"`
	Error  string `json:"error,omitempty"`
}

// NetSocketAuditRequest is the params for the "net_socket_audit" tool.
type NetSocketAuditRequest struct {
	Protocol string `json:"protocol,omitempty"`
}

// NetSocketAuditResult is the result for the "net_socket_audit" tool.
type NetSocketAuditResult struct {
	Sockets []SocketInfo `json:"sockets"`
}

// SocketInfo represents network socket information.
type SocketInfo struct {
	Protocol   string `json:"protocol"`
	LocalAddr  string `json:"local_addr"`
	LocalPort  int    `json:"local_port"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	RemotePort int    `json:"remote_port,omitempty"`
	State      string `json:"state,omitempty"`
	PID        int    `json:"pid,omitempty"`
	Process    string `json:"process,omitempty"`
}

// NetDNSResolveRequest is the params for the "net_dns_resolve" tool.
type NetDNSResolveRequest struct {
	Hostname   string `json:"hostname"`
	RecordType string `json:"record_type,omitempty"`
}

// NetDNSResolveResult is the result for the "net_dns_resolve" tool.
type NetDNSResolveResult struct {
	Hostname   string     `json:"hostname"`
	RecordType string     `json:"record_type"`
	Records    DNSRecords `json:"records"`
	Count      int        `json:"count"`
	Error      string     `json:"error,omitempty"`
}

// DNSRecords represents typed DNS record data.
type DNSRecords struct {
	A     []DNSARecord    `json:"a,omitempty"`
	AAAA  []DNSAAAARecord `json:"aaaa,omitempty"`
	MX    []DNSMXRecord   `json:"mx,omitempty"`
	TXT   []DNSTXTRecord  `json:"txt,omitempty"`
	CNAME *DNSCNAMERecord `json:"cname,omitempty"`
	NS    []DNSNSRecord   `json:"ns,omitempty"`
}

// DNSARecord represents an A record.
type DNSARecord struct {
	IP string `json:"ip"`
}

// DNSAAAARecord represents an AAAA record.
type DNSAAAARecord struct {
	IP string `json:"ip"`
}

// DNSCNAMERecord represents a CNAME record.
type DNSCNAMERecord struct {
	Target string `json:"target"`
}

// DNSNSRecord represents an NS record.
type DNSNSRecord struct {
	Host string `json:"host"`
}

// DNSTXTRecord represents a TXT record.
type DNSTXTRecord struct {
	Text string `json:"text"`
}

// DNSMXRecord represents an MX DNS record.
type DNSMXRecord struct {
	Host string `json:"host"`
	Pref uint16 `json:"pref"`
}

// NetEndpointPingRequest is the params for the "net_endpoint_ping" tool.
type NetEndpointPingRequest struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// NetEndpointPingResult is the result for the "net_endpoint_ping" tool.
type NetEndpointPingResult struct {
	Reachable bool    `json:"reachable"`
	LatencyMs float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

// NetHTTPProbeRequest is the params for the "net_http_probe" tool.
type NetHTTPProbeRequest struct {
	URL    string `json:"url"`
	Method string `json:"method,omitempty"`
}

// NetHTTPProbeResult is the result for the "net_http_probe" tool.
type NetHTTPProbeResult struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	LatencyMs  float64           `json:"latency_ms"`
	Error      string            `json:"error,omitempty"`
}

// RunShellCommandRequest is the params for the "run_shell_command" tool.
type RunShellCommandRequest struct {
	Command    string   `json:"command"`
	Args       []string `json:"args,omitempty"`
	Timeout    int      `json:"timeout,omitempty"`
	WorkingDir string   `json:"working_dir,omitempty"`
	Hostnames  []string `json:"hostnames,omitempty"`
}

// RunShellCommandResult is the result for the "run_shell_command" tool.
type RunShellCommandResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	TimedOut bool   `json:"timed_out"`
	Error    string `json:"error,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

// SysInfoRequest is the params for the "sys_info" tool.
type SysInfoRequest struct{}

// SysInfoResult is the result for the "sys_info" tool.
type SysInfoResult struct {
	Hostname string `json:"hostname"`
	OS       OSInfo `json:"os"`
}

// OSInfo represents operating system information.
type OSInfo struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	Kernel      string `json:"kernel"`
	OSVersion   string `json:"os_version"`
	Uptime      string `json:"uptime"`
	LoadAverage string `json:"load_average"`
}

// SysTimeClockRequest is the params for the "sys_time_clock" tool.
type SysTimeClockRequest struct{}

// SysTimeClockResult is the result for the "sys_time_clock" tool.
type SysTimeClockResult struct {
	SystemTime SystemTimeInfo `json:"system_time"`
	NTP        NTPStatus      `json:"ntp"`
}

// SystemTimeInfo represents system time information.
type SystemTimeInfo struct {
	UTC      string `json:"utc"`
	Local    string `json:"local"`
	Unix     int64  `json:"unix"`
	UnixNano int64  `json:"unix_nano"`
	Timezone string `json:"timezone"`
	Offset   string `json:"offset"`
}

// NTPStatus represents NTP synchronization status.
type NTPStatus struct {
	Synced           bool             `json:"synced"`
	Status           string           `json:"status"`
	NTPService       string           `json:"ntp_service,omitempty"`
	NTPSynchronized  string           `json:"ntp_synchronized,omitempty"`
	ReferenceID      string           `json:"reference_id,omitempty"`
	Stratum          string           `json:"stratum,omitempty"`
	SystemTimeOffset string           `json:"system_time_offset,omitempty"`
	SelectedPeer     *NTPSelectedPeer `json:"selected_peer,omitempty"`
}

// NTPSelectedPeer represents the selected NTP peer from ntpq.
type NTPSelectedPeer struct {
	Remote  string `json:"remote"`
	RefID   string `json:"refid"`
	Stratum string `json:"stratum"`
	When    string `json:"when"`
	Poll    string `json:"poll"`
	Reach   string `json:"reach"`
	Delay   string `json:"delay"`
	Offset  string `json:"offset"`
	Jitter  string `json:"jitter"`
}

// K8sInspectRequest is the params for the "k8s_inspect" tool.
type K8sInspectRequest struct {
	Operation string `json:"operation,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// K8sInspectResult is the result for the "k8s_inspect" tool.
type K8sInspectResult struct {
	Operation   string              `json:"operation"`
	Namespace   string              `json:"namespace,omitempty"`
	Error       string              `json:"error,omitempty"`
	Pods        []K8sPodInfo        `json:"pods,omitempty"`
	Nodes       []K8sNodeInfo       `json:"nodes,omitempty"`
	Services    []K8sServiceInfo    `json:"services,omitempty"`
	Deployments []K8sDeploymentInfo `json:"deployments,omitempty"`
	Namespaces  []K8sNamespaceInfo  `json:"namespaces,omitempty"`
	ClusterInfo *K8sClusterInfo     `json:"cluster_info,omitempty"`
	PodLogs     *K8sPodLogs         `json:"pod_logs,omitempty"`
	PodDescribe *K8sPodDescribe     `json:"pod_describe,omitempty"`
	Count       int                 `json:"count,omitempty"`
}

// K8sPodInfo represents Kubernetes pod information.
type K8sPodInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
}

// K8sNodeInfo represents Kubernetes node information.
type K8sNodeInfo struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

// K8sServiceInfo represents Kubernetes service information.
type K8sServiceInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
}

// K8sDeploymentInfo represents Kubernetes deployment information.
type K8sDeploymentInfo struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	DesiredReplicas   int    `json:"desired_replicas"`
	AvailableReplicas int    `json:"available_replicas"`
	UpdatedReplicas   int    `json:"updated_replicas"`
	Ready             bool   `json:"ready"`
}

// K8sNamespaceInfo represents Kubernetes namespace information.
type K8sNamespaceInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// K8sClusterInfo represents Kubernetes cluster information.
type K8sClusterInfo struct {
	Version string `json:"version"`
	Context string `json:"context"`
	Cluster string `json:"cluster"`
}

// K8sPodLogs represents Kubernetes pod logs.
type K8sPodLogs struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Logs      string `json:"logs"`
	Truncated bool   `json:"truncated"`
}

// K8sPodDescribe represents Kubernetes pod describe output.
type K8sPodDescribe struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Describe  string `json:"describe"`
}

// NetSSHKnownHostsRequest is the params for the "net_ssh_known_hosts" tool.
type NetSSHKnownHostsRequest struct {
	SSHConfigPath  string `json:"ssh_config_path,omitempty"`
	KnownHostsPath string `json:"known_hosts_path,omitempty"`
}

// NetSSHKnownHostsResult is the result for the "net_ssh_known_hosts" tool.
type NetSSHKnownHostsResult struct {
	ConfigHosts    []SSHConfigHost `json:"config_hosts"`
	KnownHosts     []SSHKnownHost  `json:"known_hosts"`
	OS             string          `json:"os"`
	ConfigPath     string          `json:"config_path"`
	KnownHostsPath string          `json:"known_hosts_path"`
}

// SSHConfigHost represents a host from SSH config.
type SSHConfigHost struct {
	Pattern       string   `json:"pattern"`
	Hostname      string   `json:"hostname"`
	User          string   `json:"user"`
	Port          string   `json:"port"`
	IdentityFiles []string `json:"identity_files"`
	ProxyCommand  string   `json:"proxy_command"`
}

// SSHKnownHost represents a host from known_hosts file.
type SSHKnownHost struct {
	HostPattern string `json:"host_pattern"`
	KeyType     string `json:"key_type"`
	KeyHash     string `json:"key_hash"`
}

// OperatorDeployRequest is the params for the "operator_deploy" tool.
type OperatorDeployRequest struct {
	Hostnames      []string `json:"hostnames"`
	OperatorBinary string   `json:"operator_binary,omitempty"`
	OperatorArgs   []string `json:"operator_args,omitempty"`
	Timeout        int      `json:"timeout,omitempty"`
}

// OperatorDeployResult is the result for the "operator_deploy" tool.
type OperatorDeployResult struct {
	Deployments []OperatorDeploymentResult `json:"deployments"`
}

// OperatorDeploymentResult represents the deployment result for a single host.
type OperatorDeploymentResult struct {
	Hostname string `json:"hostname"`
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	Error    string `json:"error,omitempty"`
	Output   string `json:"output,omitempty"`
}

// SysServiceStatusRequest is the params for the "sys_service_status" tool.
type SysServiceStatusRequest struct {
	ServiceName string `json:"service_name"`
}

// SysServiceStatusResult is the result for the "sys_service_status" tool.
type SysServiceStatusResult struct {
	ServiceName string `json:"service_name"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
	MainPID     string `json:"main_pid"`
	ExecStart   string `json:"exec_start"`
	Error       string `json:"error,omitempty"`
}

// CloudMetadataRequest is the params for the "cloud_metadata" tool.
type CloudMetadataRequest struct {
	Operation string `json:"operation"`
}

// CloudMetadataDetectResult is the result for the "cloud_metadata" detect operation.
type CloudMetadataDetectResult struct {
	Provider string `json:"provider"`
}

// CloudMetadataInstanceResult is the result for the "cloud_metadata" instance operation.
type CloudMetadataInstanceResult struct {
	Provider   string `json:"provider"`
	InstanceID string `json:"instance_id,omitempty"`
	Name       string `json:"name,omitempty"`
	VMSize     string `json:"vm_size,omitempty"`
	Location   string `json:"location,omitempty"`
	Error      string `json:"error,omitempty"`
}

// CloudMetadataRegionResult is the result for the "cloud_metadata" region operation.
type CloudMetadataRegionResult struct {
	Provider string `json:"provider"`
	Region   string `json:"region"`
	Error    string `json:"error,omitempty"`
}

// CloudMetadataAvailabilityZoneResult is the result for the "cloud_metadata" availability_zone operation.
type CloudMetadataAvailabilityZoneResult struct {
	Provider         string `json:"provider"`
	AvailabilityZone string `json:"availability_zone"`
	Error            string `json:"error,omitempty"`
}

// CloudMetadataInstanceTypeResult is the result for the "cloud_metadata" instance_type operation.
type CloudMetadataInstanceTypeResult struct {
	Provider     string `json:"provider"`
	InstanceType string `json:"instance_type"`
	Error        string `json:"error,omitempty"`
}

// CloudMetadataAllResult is the result for the "cloud_metadata" all operation.
type CloudMetadataAllResult struct {
	Provider         string                              `json:"provider"`
	Instance         CloudMetadataInstanceResult         `json:"instance"`
	Region           CloudMetadataRegionResult           `json:"region"`
	AvailabilityZone CloudMetadataAvailabilityZoneResult `json:"availability_zone"`
	InstanceType     CloudMetadataInstanceTypeResult     `json:"instance_type"`
}

// CloudMetadataErrorResponse is the error response for the "cloud_metadata" tool.
type CloudMetadataErrorResponse struct {
	Operation string `json:"operation,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Error     string `json:"error,omitempty"`
	Message   string `json:"message,omitempty"`
}

// AzureComputeMetadata represents the compute section of Azure instance metadata.
type AzureComputeMetadata struct {
	VMSize              string `json:"vmSize,omitempty"`
	Location            string `json:"location,omitempty"`
	PlatformFaultDomain string `json:"platformFaultDomain,omitempty"`
}

// AzureInstanceMetadata represents the Azure instance metadata service response.
type AzureInstanceMetadata struct {
	Compute AzureComputeMetadata `json:"compute"`
}
