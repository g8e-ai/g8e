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
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// NetDNSResolveTool performs DNS resolution similar to dig/nslookup for network debugging.
type NetDNSResolveTool struct{}

// Name returns the tool identifier.
func (t *NetDNSResolveTool) Name() string {
	return "net_dns_resolve"
}

// Description returns a human-readable description.
func (t *NetDNSResolveTool) Description() string {
	return "Performs DNS resolution (dig/nslookup equivalent) for network debugging."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *NetDNSResolveTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"hostname": map[string]interface{}{
				"type":        "string",
				"description": "Hostname to resolve",
			},
			"record_type": map[string]interface{}{
				"type":        "string",
				"description": "DNS record type (A, AAAA, MX, TXT, CNAME, NS)",
				"enum":        []string{"A", "AAAA", "MX", "TXT", "CNAME", "NS"},
			},
		},
		"required": []string{"hostname"},
	}
}

// Execute implements the tool logic.
func (t *NetDNSResolveTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		Hostname   string `json:"hostname"`
		RecordType string `json:"record_type,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.Hostname == "" {
		return CallToolResult{}, fmt.Errorf("hostname required")
	}

	recordType := req.RecordType
	if recordType == "" {
		recordType = "A"
	}

	resolver := &net.Resolver{
		PreferGo: true,
	}
	if deadline, ok := ctx.Deadline(); ok {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: time.Until(deadline)}
				return d.DialContext(ctx, network, address)
			},
		}
	}

	var result map[string]interface{}
	var err error

	switch recordType {
	case "A":
		result, err = resolveA(resolver, ctx, req.Hostname)
	case "AAAA":
		result, err = resolveAAAA(resolver, ctx, req.Hostname)
	case "MX":
		result, err = resolveMX(resolver, ctx, req.Hostname)
	case "TXT":
		result, err = resolveTXT(resolver, ctx, req.Hostname)
	case "CNAME":
		result, err = resolveCNAME(resolver, ctx, req.Hostname)
	case "NS":
		result, err = resolveNS(resolver, ctx, req.Hostname)
	default:
		return CallToolResult{}, fmt.Errorf("unsupported record type: %s", recordType)
	}

	if err != nil {
		result = map[string]interface{}{
			"hostname":    req.Hostname,
			"record_type": recordType,
			"error":       err.Error(),
		}
	}

	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", marshalErr)
	}

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

func resolveA(resolver *net.Resolver, ctx context.Context, hostname string) (map[string]interface{}, error) {
	ips, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return nil, err
	}

	var ipv4Addrs []string
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			ipv4Addrs = append(ipv4Addrs, ip.IP.String())
		}
	}

	return map[string]interface{}{
		"hostname":    hostname,
		"record_type": "A",
		"records":     ipv4Addrs,
		"count":       len(ipv4Addrs),
	}, nil
}

func resolveAAAA(resolver *net.Resolver, ctx context.Context, hostname string) (map[string]interface{}, error) {
	ips, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return nil, err
	}

	var ipv6Addrs []string
	for _, ip := range ips {
		if ip.IP.To4() == nil {
			ipv6Addrs = append(ipv6Addrs, ip.IP.String())
		}
	}

	return map[string]interface{}{
		"hostname":    hostname,
		"record_type": "AAAA",
		"records":     ipv6Addrs,
		"count":       len(ipv6Addrs),
	}, nil
}

func resolveMX(resolver *net.Resolver, ctx context.Context, hostname string) (map[string]interface{}, error) {
	records, err := resolver.LookupMX(ctx, hostname)
	if err != nil {
		return nil, err
	}

	var mxRecords []map[string]interface{}
	for _, mx := range records {
		mxRecords = append(mxRecords, map[string]interface{}{
			"host": mx.Host,
			"pref": mx.Pref,
		})
	}

	return map[string]interface{}{
		"hostname":    hostname,
		"record_type": "MX",
		"records":     mxRecords,
		"count":       len(mxRecords),
	}, nil
}

func resolveTXT(resolver *net.Resolver, ctx context.Context, hostname string) (map[string]interface{}, error) {
	records, err := resolver.LookupTXT(ctx, hostname)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"hostname":    hostname,
		"record_type": "TXT",
		"records":     records,
		"count":       len(records),
	}, nil
}

func resolveCNAME(resolver *net.Resolver, ctx context.Context, hostname string) (map[string]interface{}, error) {
	cname, err := resolver.LookupCNAME(ctx, hostname)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"hostname":    hostname,
		"record_type": "CNAME",
		"records":     []string{cname},
		"count":       1,
	}, nil
}

func resolveNS(resolver *net.Resolver, ctx context.Context, hostname string) (map[string]interface{}, error) {
	records, err := resolver.LookupNS(ctx, hostname)
	if err != nil {
		return nil, err
	}

	var nsRecords []string
	for _, ns := range records {
		nsRecords = append(nsRecords, ns.Host)
	}

	return map[string]interface{}{
		"hostname":    hostname,
		"record_type": "NS",
		"records":     nsRecords,
		"count":       len(nsRecords),
	}, nil
}
