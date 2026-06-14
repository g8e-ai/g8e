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
	"testing"

	"github.com/stretchr/testify/require"
)

type mockDNSResolver struct {
	lookupIPAddrFunc func(ctx context.Context, host string) ([]net.IPAddr, error)
	lookupMXFunc     func(ctx context.Context, name string) ([]*net.MX, error)
	lookupTXTFunc    func(ctx context.Context, name string) ([]string, error)
	lookupCNAMEFunc  func(ctx context.Context, name string) (string, error)
	lookupNSFunc     func(ctx context.Context, name string) ([]*net.NS, error)
}

func (m *mockDNSResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if m.lookupIPAddrFunc != nil {
		return m.lookupIPAddrFunc(ctx, host)
	}
	return nil, nil
}

func (m *mockDNSResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	if m.lookupMXFunc != nil {
		return m.lookupMXFunc(ctx, name)
	}
	return nil, nil
}

func (m *mockDNSResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	if m.lookupTXTFunc != nil {
		return m.lookupTXTFunc(ctx, name)
	}
	return nil, nil
}

func (m *mockDNSResolver) LookupCNAME(ctx context.Context, name string) (string, error) {
	if m.lookupCNAMEFunc != nil {
		return m.lookupCNAMEFunc(ctx, name)
	}
	return "", nil
}

func (m *mockDNSResolver) LookupNS(ctx context.Context, name string) ([]*net.NS, error) {
	if m.lookupNSFunc != nil {
		return m.lookupNSFunc(ctx, name)
	}
	return nil, nil
}

func TestNetDNSResolveTool_Name(t *testing.T) {
	tool := &NetDNSResolveTool{}
	require.Equal(t, "net_dns_resolve", tool.Name())
}

func TestNetDNSResolveTool_Description(t *testing.T) {
	tool := &NetDNSResolveTool{}
	require.NotEmpty(t, tool.Description())
}

func TestNetDNSResolveTool_Execute_A(t *testing.T) {
	mock := &mockDNSResolver{
		lookupIPAddrFunc: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{
				{IP: net.ParseIP("192.168.1.1")},
				{IP: net.ParseIP("2001:db8::1")}, // Should be filtered out
			}, nil
		},
	}
	tool := &NetDNSResolveTool{resolver: mock}

	args := json.RawMessage(`{"hostname": "example.com", "record_type": "A"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res NetDNSResolveResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.Equal(t, "example.com", res.Hostname)
	require.Equal(t, "A", res.RecordType)
	require.Equal(t, 1, res.Count)
	require.ElementsMatch(t, []string{"192.168.1.1"}, res.Records)
}

func TestNetDNSResolveTool_Execute_AAAA(t *testing.T) {
	mock := &mockDNSResolver{
		lookupIPAddrFunc: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{
				{IP: net.ParseIP("192.168.1.1")}, // Should be filtered out
				{IP: net.ParseIP("2001:db8::1")},
			}, nil
		},
	}
	tool := &NetDNSResolveTool{resolver: mock}

	args := json.RawMessage(`{"hostname": "example.com", "record_type": "AAAA"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res NetDNSResolveResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.Equal(t, "AAAA", res.RecordType)
	require.Equal(t, 1, res.Count)
	require.ElementsMatch(t, []string{"2001:db8::1"}, res.Records)
}

func TestNetDNSResolveTool_Execute_MX(t *testing.T) {
	mock := &mockDNSResolver{
		lookupMXFunc: func(ctx context.Context, name string) ([]*net.MX, error) {
			return []*net.MX{
				{Host: "mail.example.com.", Pref: 10},
			}, nil
		},
	}
	tool := &NetDNSResolveTool{resolver: mock}

	args := json.RawMessage(`{"hostname": "example.com", "record_type": "MX"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res NetDNSResolveResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.Equal(t, "MX", res.RecordType)
	require.Equal(t, 1, res.Count)
	
	records := res.Records.([]interface{})
	require.Len(t, records, 1)
	mx := records[0].(map[string]interface{})
	require.Equal(t, "mail.example.com.", mx["host"])
	require.Equal(t, float64(10), mx["pref"])
}

func TestNetDNSResolveTool_Execute_TXT(t *testing.T) {
	mock := &mockDNSResolver{
		lookupTXTFunc: func(ctx context.Context, name string) ([]string, error) {
			return []string{"v=spf1 include:_spf.google.com ~all"}, nil
		},
	}
	tool := &NetDNSResolveTool{resolver: mock}

	args := json.RawMessage(`{"hostname": "example.com", "record_type": "TXT"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res NetDNSResolveResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.Equal(t, "TXT", res.RecordType)
	require.ElementsMatch(t, []string{"v=spf1 include:_spf.google.com ~all"}, res.Records)
}

func TestNetDNSResolveTool_Execute_CNAME(t *testing.T) {
	mock := &mockDNSResolver{
		lookupCNAMEFunc: func(ctx context.Context, name string) (string, error) {
			return "target.example.com.", nil
		},
	}
	tool := &NetDNSResolveTool{resolver: mock}

	args := json.RawMessage(`{"hostname": "alias.example.com", "record_type": "CNAME"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res NetDNSResolveResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.Equal(t, "CNAME", res.RecordType)
	require.ElementsMatch(t, []string{"target.example.com."}, res.Records)
}

func TestNetDNSResolveTool_Execute_NS(t *testing.T) {
	mock := &mockDNSResolver{
		lookupNSFunc: func(ctx context.Context, name string) ([]*net.NS, error) {
			return []*net.NS{
				{Host: "ns1.example.com."},
			}, nil
		},
	}
	tool := &NetDNSResolveTool{resolver: mock}

	args := json.RawMessage(`{"hostname": "example.com", "record_type": "NS"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res NetDNSResolveResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.Equal(t, "NS", res.RecordType)
	require.ElementsMatch(t, []string{"ns1.example.com."}, res.Records)
}

func TestNetDNSResolveTool_Execute_InvalidArgs(t *testing.T) {
	tool := &NetDNSResolveTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid arguments")
}

func TestNetDNSResolveTool_Execute_MissingHostname(t *testing.T) {
	tool := &NetDNSResolveTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "hostname required")
}

func TestNetDNSResolveTool_Execute_UnsupportedRecordType(t *testing.T) {
	tool := &NetDNSResolveTool{}
	args := json.RawMessage(`{"hostname": "example.com", "record_type": "INVALID"}`)
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported record type")
}

func TestNetDNSResolveTool_Execute_ResolverError(t *testing.T) {
	mock := &mockDNSResolver{
		lookupIPAddrFunc: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return nil, fmt.Errorf("dns resolution failed")
		},
	}
	tool := &NetDNSResolveTool{resolver: mock}

	args := json.RawMessage(`{"hostname": "example.com", "record_type": "A"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err) // Tool handles resolver errors by returning a result with Error field

	var res NetDNSResolveResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.Equal(t, "dns resolution failed", res.Error)
}

func TestNetDNSResolveTool_Execute_DefaultType(t *testing.T) {
	mock := &mockDNSResolver{
		lookupIPAddrFunc: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		},
	}
	tool := &NetDNSResolveTool{resolver: mock}

	args := json.RawMessage(`{"hostname": "example.com"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res NetDNSResolveResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)

	require.Equal(t, "A", res.RecordType)
}
