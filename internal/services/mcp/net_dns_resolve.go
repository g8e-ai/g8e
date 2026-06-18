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
type NetDNSResolveTool struct {
	// resolver is used for DNS lookups. If nil, a default resolver is used.
	// This is primarily for testing.
	resolver dnsResolver
}

type dnsResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupCNAME(ctx context.Context, name string) (string, error)
	LookupNS(ctx context.Context, name string) ([]*net.NS, error)
}

// Name returns the tool identifier.
func (t *NetDNSResolveTool) Name() string {
	return "net_dns_resolve"
}

// Description returns a human-readable description.
func (t *NetDNSResolveTool) Description() string {
	return "Performs DNS resolution (dig/nslookup equivalent) for network debugging."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *NetDNSResolveTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"hostname": {
				Type:        "string",
				Description: "Hostname to resolve",
			},
			"record_type": {
				Type:        "string",
				Description: "DNS record type (A, AAAA, MX, TXT, CNAME, NS)",
				Enum:        []string{"A", "AAAA", "MX", "TXT", "CNAME", "NS"},
			},
		},
		Required: []string{"hostname"},
	}
}

// Execute implements the tool logic.
func (t *NetDNSResolveTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req NetDNSResolveRequest
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

	resolver := t.resolver
	if resolver == nil {
		netResolver := &net.Resolver{
			PreferGo: true,
		}
		if deadline, ok := ctx.Deadline(); ok {
			netResolver.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: time.Until(deadline)}
				return d.DialContext(ctx, network, address)
			}
		}
		resolver = netResolver
	}

	var result NetDNSResolveResult
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
		result = NetDNSResolveResult{
			Hostname:   req.Hostname,
			RecordType: recordType,
			Error:      err.Error(),
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

func resolveA(resolver dnsResolver, ctx context.Context, hostname string) (NetDNSResolveResult, error) {
	ips, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return NetDNSResolveResult{}, err
	}

	var aRecords []DNSARecord
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			aRecords = append(aRecords, DNSARecord{IP: ip.IP.String()})
		}
	}

	return NetDNSResolveResult{
		Hostname:   hostname,
		RecordType: "A",
		Records:    DNSRecords{A: aRecords},
		Count:      len(aRecords),
	}, nil
}

func resolveAAAA(resolver dnsResolver, ctx context.Context, hostname string) (NetDNSResolveResult, error) {
	ips, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return NetDNSResolveResult{}, err
	}

	var aaaaRecords []DNSAAAARecord
	for _, ip := range ips {
		if ip.IP.To4() == nil {
			aaaaRecords = append(aaaaRecords, DNSAAAARecord{IP: ip.IP.String()})
		}
	}

	return NetDNSResolveResult{
		Hostname:   hostname,
		RecordType: "AAAA",
		Records:    DNSRecords{AAAA: aaaaRecords},
		Count:      len(aaaaRecords),
	}, nil
}

func resolveMX(resolver dnsResolver, ctx context.Context, hostname string) (NetDNSResolveResult, error) {
	records, err := resolver.LookupMX(ctx, hostname)
	if err != nil {
		return NetDNSResolveResult{}, err
	}

	var mxRecords []DNSMXRecord
	for _, mx := range records {
		mxRecords = append(mxRecords, DNSMXRecord{
			Host: mx.Host,
			Pref: mx.Pref,
		})
	}

	return NetDNSResolveResult{
		Hostname:   hostname,
		RecordType: "MX",
		Records:    DNSRecords{MX: mxRecords},
		Count:      len(mxRecords),
	}, nil
}

func resolveTXT(resolver dnsResolver, ctx context.Context, hostname string) (NetDNSResolveResult, error) {
	records, err := resolver.LookupTXT(ctx, hostname)
	if err != nil {
		return NetDNSResolveResult{}, err
	}

	var txtRecords []DNSTXTRecord
	for _, txt := range records {
		txtRecords = append(txtRecords, DNSTXTRecord{Text: txt})
	}

	return NetDNSResolveResult{
		Hostname:   hostname,
		RecordType: "TXT",
		Records:    DNSRecords{TXT: txtRecords},
		Count:      len(txtRecords),
	}, nil
}

func resolveCNAME(resolver dnsResolver, ctx context.Context, hostname string) (NetDNSResolveResult, error) {
	cname, err := resolver.LookupCNAME(ctx, hostname)
	if err != nil {
		return NetDNSResolveResult{}, err
	}

	return NetDNSResolveResult{
		Hostname:   hostname,
		RecordType: "CNAME",
		Records:    DNSRecords{CNAME: &DNSCNAMERecord{Target: cname}},
		Count:      1,
	}, nil
}

func resolveNS(resolver dnsResolver, ctx context.Context, hostname string) (NetDNSResolveResult, error) {
	records, err := resolver.LookupNS(ctx, hostname)
	if err != nil {
		return NetDNSResolveResult{}, err
	}

	var nsRecords []DNSNSRecord
	for _, ns := range records {
		nsRecords = append(nsRecords, DNSNSRecord{Host: ns.Host})
	}

	return NetDNSResolveResult{
		Hostname:   hostname,
		RecordType: "NS",
		Records:    DNSRecords{NS: nsRecords},
		Count:      len(nsRecords),
	}, nil
}
