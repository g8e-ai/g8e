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

//go:build integration

package gateway

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

func TestClassifyEnvelopeError_Exhaustive(t *testing.T) {
	cases := []struct {
		err      error
		expected int
	}{
		{nil, http.StatusOK},
		{governance.ErrTransactionIDMissing, http.StatusForbidden},
		{governance.ErrUnknownActionType, http.StatusForbidden},
		{governance.ErrPayloadMissing, http.StatusForbidden},
		{governance.ErrPayloadDecodeFailed, http.StatusForbidden},
		{governance.ErrL1ValidationFailed, http.StatusForbidden},
		{governance.ErrTransactionHashMissing, http.StatusForbidden},
		{governance.ErrTransactionHashMismatch, http.StatusForbidden},
		{governance.ErrTransactionExpired, http.StatusForbidden},
		{governance.ErrNonceMissing, http.StatusForbidden},
		{governance.ErrTransactionReplay, http.StatusForbidden},
		{governance.ErrStateRootMissing, http.StatusForbidden},
		{governance.ErrStateRootRequired, http.StatusForbidden},
		{governance.ErrStateRootMismatch, http.StatusForbidden},
		{governance.ErrL2SignatureMissing, http.StatusForbidden},
		{governance.ErrL2SignatureInvalid, http.StatusForbidden},
		{governance.ErrL2ConsensusNotConfigured, http.StatusForbidden},
		{governance.ErrL3ProofMissing, http.StatusForbidden},
		{governance.ErrL3ProofInvalid, http.StatusForbidden},
		{governance.ErrL3NotaryNotConfigured, http.StatusForbidden},
		{governance.ErrTxInFlight, http.StatusForbidden},
		{governance.ErrInvalidEnvelope, http.StatusBadRequest},
		{fmt.Errorf("%w: some error", constants.ErrTxInvalidEnvelope), http.StatusBadRequest},
		{constants.ErrPubSubEmptyPayload, http.StatusBadRequest},
		{fmt.Errorf("payload exceeds 1234 byte limit: %w", constants.ErrPayloadExceedsLimit), http.StatusBadRequest},
		{fmt.Errorf("some random error"), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("err=%v", tc.err), func(t *testing.T) {
			require.Equal(t, tc.expected, classifyEnvelopeError(tc.err))
		})
	}
}

func TestVerifyEnvelopeIdentityBinding_Exhaustive(t *testing.T) {

	mustParseURL := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			panic(err)
		}
		return u
	}

	cases := []struct {
		name              string
		uris              []*url.URL
		operatorID        string
		operatorSessionID string
		source            commonv1.Component
		expectErr         bool
	}{
		{
			name:              "CLI session match",
			uris:              []*url.URL{mustParseURL("spiffe://g8e.local/cli/user-1/sess-1")},
			operatorSessionID: "sess-1",
			expectErr:         false,
		},
		{
			name:              "CLI session mismatch",
			uris:              []*url.URL{mustParseURL("spiffe://g8e.local/cli/user-1/sess-2")},
			operatorSessionID: "sess-1",
			expectErr:         true,
		},
		{
			name:              "Operator ID and session match",
			uris:              []*url.URL{mustParseURL("spiffe://g8e.local/operator/org-1/op-1/sess-1")},
			operatorID:        "op-1",
			operatorSessionID: "sess-1",
			expectErr:         false,
		},
		{
			name:              "Operator ID mismatch",
			uris:              []*url.URL{mustParseURL("spiffe://g8e.local/operator/org-1/op-2/sess-1")},
			operatorID:        "op-1",
			operatorSessionID: "sess-1",
			expectErr:         true,
		},
		{
			name:              "Operator session mismatch",
			uris:              []*url.URL{mustParseURL("spiffe://g8e.local/operator/org-1/op-1/sess-2")},
			operatorID:        "op-1",
			operatorSessionID: "sess-1",
			expectErr:         true,
		},
		{
			name:       "App match",
			uris:       []*url.URL{mustParseURL("spiffe://g8e.local/app/op-1")},
			operatorID: "op-1",
			source:     commonv1.Component_COMPONENT_AGENT,
			expectErr:  false,
		},
		{
			name:       "App mismatch",
			uris:       []*url.URL{mustParseURL("spiffe://g8e.local/app/op-2")},
			operatorID: "op-1",
			source:     commonv1.Component_COMPONENT_AGENT,
			expectErr:  true,
		},
		{
			name:       "Non-app component skips App check",
			uris:       []*url.URL{mustParseURL("spiffe://g8e.local/app/op-1")},
			operatorID: "op-1",
			source:     commonv1.Component_COMPONENT_G8EO,
			expectErr:  true,
		},
		{
			name:       "Multiple URIs, one matches",
			uris:       []*url.URL{mustParseURL("spiffe://g8e.local/unknown"), mustParseURL("spiffe://g8e.local/app/op-1")},
			operatorID: "op-1",
			source:     commonv1.Component_COMPONENT_AGENT,
			expectErr:  false,
		},
		{
			name:              "Wrong trust domain",
			uris:              []*url.URL{mustParseURL("spiffe://other.local/operator/org-1/op-1/sess-1")},
			operatorID:        "op-1",
			operatorSessionID: "sess-1",
			expectErr:         true,
		},
		{
			name:              "Operator match but wrong path prefix",
			uris:              []*url.URL{mustParseURL("spiffe://g8e.local/other/org-1/op-1/sess-1")},
			operatorID:        "op-1",
			operatorSessionID: "sess-1",
			expectErr:         true,
		},
		{
			name:              "Multiple URIs, none match",
			uris:              []*url.URL{mustParseURL("spiffe://g8e.local/unknown1"), mustParseURL("spiffe://g8e.local/unknown2")},
			operatorID:        "op-1",
			operatorSessionID: "sess-1",
			expectErr:         true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.TLS = &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{
					{
						URIs: tc.uris,
					},
				},
			}
			envelope := marshalEnvelope(t, &commonv1.GovernanceEnvelope{
				OperatorId:        tc.operatorID,
				OperatorSessionId: tc.operatorSessionID,
				SourceComponent:   tc.source,
			})
			err := verifyEnvelopeIdentityBinding(req, envelope)
			if tc.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "certificate URI SAN does not match envelope identity claims")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
