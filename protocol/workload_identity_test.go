// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package protocol

import (
	"net/url"
	"testing"
)

func TestGatewayPeerSPIFFEID(t *testing.T) {
	wid := NewWorkloadIdentity()
	gatewayID := "gw-12345"

	expected := "spiffe://g8e.local/gateway/gw-12345"
	result := wid.GatewayPeerSPIFFEID(gatewayID)

	if result != expected {
		t.Errorf("GatewayPeerSPIFFEID() = %s, want %s", result, expected)
	}
}

func TestGatewayPeerSPIFFEURL(t *testing.T) {
	wid := NewWorkloadIdentity()
	gatewayID := "gw-12345"

	result, err := wid.GatewayPeerSPIFFEURL(gatewayID)
	if err != nil {
		t.Fatalf("GatewayPeerSPIFFEURL() error = %v", err)
	}

	expected := "spiffe://g8e.local/gateway/gw-12345"
	if result.String() != expected {
		t.Errorf("GatewayPeerSPIFFEURL() = %s, want %s", result.String(), expected)
	}
}

func TestMatchesGatewayPeer(t *testing.T) {
	wid := NewWorkloadIdentity()
	gatewayID := "gw-12345"

	spiffeID := wid.GatewayPeerSPIFFEID(gatewayID)

	if !wid.MatchesGatewayPeer(spiffeID, gatewayID) {
		t.Errorf("MatchesGatewayPeer() should return true for matching gateway ID")
	}

	if wid.MatchesGatewayPeer(spiffeID, "different-gw") {
		t.Errorf("MatchesGatewayPeer() should return false for different gateway ID")
	}

	if wid.MatchesGatewayPeer("spiffe://g8e.local/operator/org/op/session", gatewayID) {
		t.Errorf("MatchesGatewayPeer() should return false for non-gateway SPIFFE ID")
	}
}

func TestExtractGatewayID(t *testing.T) {
	wid := NewWorkloadIdentity()
	gatewayID := "gw-12345"
	spiffeID := wid.GatewayPeerSPIFFEID(gatewayID)

	extracted, ok := wid.ExtractGatewayID(spiffeID)
	if !ok {
		t.Errorf("ExtractGatewayID() should return true for valid gateway SPIFFE ID")
	}
	if extracted != gatewayID {
		t.Errorf("ExtractGatewayID() = %s, want %s", extracted, gatewayID)
	}

	// Test invalid SPIFFE ID
	_, ok = wid.ExtractGatewayID("spiffe://g8e.local/operator/org/op/session")
	if ok {
		t.Errorf("ExtractGatewayID() should return false for non-gateway SPIFFE ID")
	}

	// Test malformed SPIFFE ID
	_, ok = wid.ExtractGatewayID("spiffe://g8e.local/gateway")
	if ok {
		t.Errorf("ExtractGatewayID() should return false for malformed gateway SPIFFE ID")
	}

	// Test wrong trust domain
	_, ok = wid.ExtractGatewayID("spiffe://other.local/gateway/gw-12345")
	if ok {
		t.Errorf("ExtractGatewayID() should return false for wrong trust domain")
	}
}

func TestGatewayPeerSPIFFEIDFormat(t *testing.T) {
	wid := NewWorkloadIdentity()
	gatewayID := "test-gateway-with-dashes"

	spiffeID := wid.GatewayPeerSPIFFEID(gatewayID)

	// Verify it's a valid URL
	parsed, err := url.Parse(spiffeID)
	if err != nil {
		t.Fatalf("Failed to parse SPIFFE ID as URL: %v", err)
	}

	if parsed.Scheme != "spiffe" {
		t.Errorf("SPIFFE ID scheme should be 'spiffe', got %s", parsed.Scheme)
	}

	if parsed.Host != TrustDomain {
		t.Errorf("SPIFFE ID host should be '%s', got %s", TrustDomain, parsed.Host)
	}

	expectedPath := "/gateway/" + gatewayID
	if parsed.Path != expectedPath {
		t.Errorf("SPIFFE ID path should be '%s', got %s", expectedPath, parsed.Path)
	}
}
