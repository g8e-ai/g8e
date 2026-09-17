// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cloudflaredns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const cloudflareAPIBase = "https://api.cloudflare.com/client/v4"

// Client routes DNS records through the Cloudflare API.
type Client struct {
	token  string
	client *http.Client
}

// NewClient builds a Cloudflare DNS client from an API token.
func NewClient(token string) (*Client, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("cloudflare dns: %w: API token is required", constants.ErrMissingRequiredField)
	}
	return &Client{
		token: token,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

type apiResponse struct {
	Success  bool              `json:"success"`
	Errors   []apiError        `json:"errors"`
	Messages []json.RawMessage `json:"messages"`
	Result   json.RawMessage   `json:"result"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type zoneResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type dnsRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// ZoneIDForHostname returns the Cloudflare zone ID that owns hostname.
func (c *Client) ZoneIDForHostname(ctx context.Context, hostname string) (string, error) {
	zoneName, err := zoneNameForHostname(hostname)
	if err != nil {
		return "", err
	}
	var zones []zoneResult
	if err := c.getJSON(ctx, "/zones?name="+url.QueryEscape(zoneName), &zones); err != nil {
		return "", err
	}
	if len(zones) == 0 {
		return "", fmt.Errorf("cloudflare dns: zone %q not found in account", zoneName)
	}
	return zones[0].ID, nil
}

// UpsertTunnelCNAME creates or updates a proxied CNAME to a Cloudflare Tunnel.
func (c *Client) UpsertTunnelCNAME(ctx context.Context, hostname, tunnelID string) error {
	hostname = strings.TrimSpace(hostname)
	tunnelID = strings.TrimSpace(tunnelID)
	if hostname == "" || tunnelID == "" {
		return fmt.Errorf("cloudflare dns: %w: hostname and tunnel ID are required", constants.ErrMissingRequiredField)
	}

	zoneID, err := c.ZoneIDForHostname(ctx, hostname)
	if err != nil {
		return err
	}

	recordName := dnsRecordName(hostname)
	target := tunnelID + ".cfargotunnel.com"

	existing, err := c.listRecords(ctx, zoneID, recordName)
	if err != nil {
		return err
	}

	body := map[string]any{
		"type":    "CNAME",
		"name":    recordName,
		"content": target,
		"proxied": true,
		"ttl":     1,
	}
	if len(existing) == 0 {
		return c.postJSON(ctx, "/zones/"+zoneID+"/dns_records", body, nil)
	}

	for _, record := range existing {
		if record.Type != "CNAME" && record.Type != "Tunnel" {
			return fmt.Errorf("cloudflare dns: existing %s record %q blocks tunnel routing", record.Type, record.Name)
		}
		if err := c.patchJSON(ctx, "/zones/"+zoneID+"/dns_records/"+record.ID, body, nil); err != nil {
			return err
		}
	}
	return nil
}

func zoneNameForHostname(hostname string) (string, error) {
	hostname = strings.TrimSpace(strings.TrimSuffix(hostname, "."))
	if hostname == "" {
		return "", fmt.Errorf("cloudflare dns: %w: hostname is required", constants.ErrMissingRequiredField)
	}
	parts := strings.Split(hostname, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("cloudflare dns: %w: hostname must include a zone", constants.ErrValidationFailed)
	}
	return strings.Join(parts[len(parts)-2:], "."), nil
}

func dnsRecordName(hostname string) string {
	zoneName, err := zoneNameForHostname(hostname)
	if err != nil {
		return hostname
	}
	if hostname == zoneName {
		return zoneName
	}
	suffix := "." + zoneName
	if strings.HasSuffix(hostname, suffix) {
		return strings.TrimSuffix(hostname, suffix)
	}
	return hostname
}

func (c *Client) listRecords(ctx context.Context, zoneID, recordName string) ([]dnsRecord, error) {
	var records []dnsRecord
	path := fmt.Sprintf("/zones/%s/dns_records?name=%s", zoneID, url.QueryEscape(recordName))
	if err := c.getJSON(ctx, path, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) postJSON(ctx context.Context, path string, body any, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, body, out)
}

func (c *Client) patchJSON(ctx context.Context, path string, body any, out any) error {
	return c.doJSON(ctx, http.MethodPatch, path, body, out)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("cloudflare dns: encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, cloudflareAPIBase+path, reader)
	if err != nil {
		return fmt.Errorf("cloudflare dns: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare dns: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("cloudflare dns: read response: %w", err)
	}

	var envelope apiResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("cloudflare dns: decode response: %w", err)
	}
	if !envelope.Success {
		if len(envelope.Errors) == 0 {
			return fmt.Errorf("cloudflare dns: request failed with HTTP %d", resp.StatusCode)
		}
		return fmt.Errorf("cloudflare dns: %s", envelope.Errors[0].Message)
	}
	if out != nil && len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("cloudflare dns: decode result: %w", err)
		}
	}
	return nil
}
