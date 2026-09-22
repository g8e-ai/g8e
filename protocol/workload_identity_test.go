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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestOperatorAndCLIIdentityHelpers(t *testing.T) {
	wid := NewWorkloadIdentity()

	operatorID := wid.OperatorSPIFFEID("org-1", "op-1", "sess-1")
	operatorURL, err := wid.OperatorSPIFFEURL("org-1", "op-1", "sess-1")
	require.NoError(t, err)
	assertOperatorURL(t, operatorURL, operatorID)
	if !wid.MatchesOperator(operatorID, "org-1", "op-1", "sess-1") {
		t.Fatal("MatchesOperator should accept generated operator SPIFFE ID")
	}

	cliID := wid.CLISPIFFEID("user-1", "cli-1")
	cliURL, err := wid.CLISPIFFEURL("user-1", "cli-1")
	require.NoError(t, err)
	assertOperatorURL(t, cliURL, cliID)
	if !wid.MatchesCLI(cliID, "user-1", "cli-1") {
		t.Fatal("MatchesCLI should accept generated CLI SPIFFE ID")
	}
	if !wid.MatchesCLISessionOnly(cliID, "cli-1") {
		t.Fatal("MatchesCLISessionOnly should accept CLI session suffix")
	}

	sessionID, ok := wid.ExtractCLISessionID(cliID)
	if !ok || sessionID != "cli-1" {
		t.Fatalf("ExtractCLISessionID() = %q, ok=%v", sessionID, ok)
	}
	userID, ok := wid.ExtractUserID(cliID)
	if !ok || userID != "user-1" {
		t.Fatalf("ExtractUserID() = %q, ok=%v", userID, ok)
	}
}

func TestAppUserAndHubIdentityHelpers(t *testing.T) {
	wid := NewWorkloadIdentity()

	appID := wid.AppSPIFFEID("agent-1")
	appURL, err := wid.AppSPIFFEURL("agent-1")
	require.NoError(t, err)
	assertOperatorURL(t, appURL, appID)
	if !wid.MatchesApp(appID, "agent-1") {
		t.Fatal("MatchesApp should accept generated app SPIFFE ID")
	}
	if !wid.IsAppSAN(appID) {
		t.Fatal("IsAppSAN should recognize app SPIFFE IDs")
	}
	if !wid.IsEnsembleApp(EnsembleAppID) {
		t.Fatal("IsEnsembleApp should recognize the ensemble app ID")
	}

	userID := wid.UserSPIFFEID("user-42")
	userURL, err := wid.UserSPIFFEURL("user-42")
	require.NoError(t, err)
	assertOperatorURL(t, userURL, userID)
	if !wid.IsUserSAN(userID) {
		t.Fatal("IsUserSAN should recognize user SPIFFE IDs")
	}
	extracted, ok := wid.ExtractUserIDFromUserSAN(userID)
	if !ok || extracted != "user-42" {
		t.Fatalf("ExtractUserIDFromUserSAN() = %q, ok=%v", extracted, ok)
	}

	hubID := wid.HubSPIFFEID()
	hubURL, err := wid.HubSPIFFEURL()
	require.NoError(t, err)
	assertOperatorURL(t, hubURL, hubID)
	if !wid.MatchesHub(hubID) {
		t.Fatal("MatchesHub should accept generated hub SPIFFE ID")
	}
}

func TestExtractOperatorSessionID(t *testing.T) {
	wid := NewWorkloadIdentity()
	spiffeID := wid.OperatorSPIFFEID("org-1", "op-1", "sess-9")
	sessionID, ok := wid.ExtractOperatorSessionID(spiffeID)
	if !ok || sessionID != "sess-9" {
		t.Fatalf("ExtractOperatorSessionID() = %q, ok=%v", sessionID, ok)
	}
	if _, ok := wid.ExtractOperatorSessionID("spiffe://g8e.local/cli/user/cli"); ok {
		t.Fatal("ExtractOperatorSessionID should reject non-operator SPIFFE IDs")
	}
}

func TestWorkloadIdentityExtractorsRejectMalformedPathSegments(t *testing.T) {
	wid := NewWorkloadIdentity()
	tests := []struct {
		name      string
		spiffeID  string
		extract   func(string) (string, bool)
		wantValue string
	}{
		{name: "cli session valid", spiffeID: wid.CLISPIFFEID("user-1", "session-1"), extract: wid.ExtractCLISessionID, wantValue: "session-1"},
		{name: "cli user valid", spiffeID: wid.CLISPIFFEID("user-1", "session-1"), extract: wid.ExtractUserID, wantValue: "user-1"},
		{name: "gateway valid", spiffeID: wid.GatewayPeerSPIFFEID("gateway-1"), extract: wid.ExtractGatewayID, wantValue: "gateway-1"},
		{name: "operator session valid", spiffeID: wid.OperatorSPIFFEID("org-1", "operator-1", "session-1"), extract: wid.ExtractOperatorSessionID, wantValue: "session-1"},
		{name: "cli missing user", spiffeID: "spiffe://g8e.local/cli//session-1", extract: wid.ExtractUserID},
		{name: "cli missing session", spiffeID: "spiffe://g8e.local/cli/user-1/", extract: wid.ExtractCLISessionID},
		{name: "cli extra segment", spiffeID: "spiffe://g8e.local/cli/user-1/session-1/extra", extract: wid.ExtractUserID},
		{name: "gateway extra segment", spiffeID: "spiffe://g8e.local/gateway/gateway-1/extra", extract: wid.ExtractGatewayID},
		{name: "operator missing component", spiffeID: "spiffe://g8e.local/operator/org-1//session-1", extract: wid.ExtractOperatorSessionID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.extract(tt.spiffeID)
			if tt.wantValue == "" {
				assert.False(t, ok)
				assert.Empty(t, got)
				return
			}
			assert.True(t, ok)
			assert.Equal(t, tt.wantValue, got)
		})
	}
}

func TestMatchesCLISessionOnlyRequiresCanonicalCLIIdentity(t *testing.T) {
	wid := NewWorkloadIdentity()
	valid := wid.CLISPIFFEID("user-1", "session-1")
	for _, tt := range []struct {
		name      string
		spiffeID  string
		sessionID string
		want      bool
	}{
		{name: "matching session", spiffeID: valid, sessionID: "session-1", want: true},
		{name: "different session", spiffeID: valid, sessionID: "session-2", want: false},
		{name: "extra path segment", spiffeID: valid + "/extra", sessionID: "session-1", want: false},
		{name: "empty session", spiffeID: "spiffe://g8e.local/cli/user-1/", sessionID: "", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, wid.MatchesCLISessionOnly(tt.spiffeID, tt.sessionID))
		})
	}
}

func TestExtractUserIDFromUserSANRejectsPathTraversal(t *testing.T) {
	wid := NewWorkloadIdentity()
	for _, spiffeID := range []string{
		"spiffe://g8e.local/user/",
		"spiffe://g8e.local/user/user-1/extra",
		"spiffe://other.local/user/user-1",
	} {
		userID, ok := wid.ExtractUserIDFromUserSAN(spiffeID)
		assert.False(t, ok, spiffeID)
		assert.Empty(t, userID)
	}
	userID, ok := wid.ExtractUserIDFromUserSAN(wid.UserSPIFFEID("user-1"))
	require.True(t, ok)
	assert.Equal(t, "user-1", userID)
}

func assertOperatorURL(t *testing.T, got *url.URL, want string) {
	t.Helper()
	if got == nil {
		t.Fatal("expected non-nil URL")
	}
	if got.String() != want {
		t.Fatalf("URL = %s, want %s", got.String(), want)
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
