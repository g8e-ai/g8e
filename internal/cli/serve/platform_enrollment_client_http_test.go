// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/auth"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// enrollStub is a gateway stand-in whose three enrollment endpoints are
// supplied per test. A nil route answers 404. Hit counters let tests assert
// that the client did (or did not) talk to the gateway.
type enrollStub struct {
	server      *httptest.Server
	requestHits atomic.Int32
	statusHits  atomic.Int32
	completeHit atomic.Int32
}

type enrollRoutes struct {
	request  http.HandlerFunc
	status   http.HandlerFunc
	complete http.HandlerFunc
}

func newEnrollStub(t *testing.T, routes enrollRoutes) *enrollStub {
	t.Helper()
	stub := &enrollStub{}
	mux := http.NewServeMux()
	wrap := func(counter *atomic.Int32, h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			counter.Add(1)
			if h == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc(constants.APIPaths.AuthPlatformEnrollmentRequest, wrap(&stub.requestHits, routes.request))
	mux.HandleFunc(constants.APIPaths.AuthPlatformEnrollmentStatus, wrap(&stub.statusHits, routes.status))
	mux.HandleFunc(constants.APIPaths.AuthPlatformEnrollmentComplete, wrap(&stub.completeHit, routes.complete))
	stub.server = httptest.NewServer(mux)
	t.Cleanup(stub.server.Close)
	return stub
}

func newEnrollClient(t *testing.T, gatewayURL string) (*OperatorPlatformEnrollmentClient, fs.RuntimeFileService) {
	t.Helper()
	fileSvc := newTestFileSvc(t)
	client, err := NewOperatorPlatformEnrollmentClient(gatewayURL, "inst-1", "host-1", fileSvc, testLogger())
	require.NoError(t, err)
	return client, fileSvc
}

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func statusReply(state models.PlatformEnrollmentState) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, models.PlatformEnrollmentStatusResponse{
			RequestID: "req-1", ComponentKind: models.PlatformComponentOperator, State: state,
		})
	}
}

func createdReply(requestID, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusCreated, models.PlatformEnrollmentCreateResponse{
			RequestID: requestID, Token: token, ComponentKind: models.PlatformComponentOperator,
			ExpiresAt: time.Now().Add(30 * time.Minute).UTC(),
		})
	}
}

func shortContext(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNewOperatorPlatformEnrollmentClient_RejectsMissingIdentity(t *testing.T) {
	tests := []struct {
		name                      string
		url, instanceID, hostname string
	}{
		{"missing gateway URL", "", "inst", "host"},
		{"missing instance id", "http://gw:8080", "", "host"},
		{"missing hostname", "http://gw:8080", "inst", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewOperatorPlatformEnrollmentClient(tt.url, tt.instanceID, tt.hostname, newTestFileSvc(t), testLogger())

			require.ErrorIs(t, err, constants.ErrInternal)
			assert.Nil(t, client)
		})
	}
}

func TestNewOperatorPlatformEnrollmentClient_NormalizesTrailingSlashes(t *testing.T) {
	client, err := NewOperatorPlatformEnrollmentClient("http://gw:8080///", "inst", "host", newTestFileSvc(t), testLogger())

	require.NoError(t, err)
	assert.Equal(t, "http://gw:8080", client.gatewayHTTPURL)
}

func TestOperatorPlatformEnrollmentClient_SetFingerprintOptions_ReplacesPriorOptions(t *testing.T) {
	client, _ := newEnrollClient(t, "http://gw:8080")
	first := auth.FingerprintOptions{LocalDir: "/srv/a", Account: "alice", Port: 8443, Role: "data"}
	second := auth.FingerprintOptions{LocalDir: "/srv/b", Role: "inference"}

	client.SetFingerprintOptions(first)
	assert.Equal(t, first, client.fingerprintOpts)
	client.SetFingerprintOptions(second)
	assert.Equal(t, second, client.fingerprintOpts)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestParseRetryAfter_AcceptsOnlyPositiveSeconds(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"1", time.Second},
		{"30", 30 * time.Second},
		{"0", 0},
		{"-5", 0},
		{"soon", 0},
		{"Wed, 21 Oct 2015 07:28:00 GMT", 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, parseRetryAfter(tt.in))
		})
	}
}

func TestTrimTrailingSlash(t *testing.T) {
	tests := map[string]string{
		"":               "",
		"http://gw":      "http://gw",
		"http://gw/":     "http://gw",
		"http://gw///":   "http://gw",
		"http://gw/a/b/": "http://gw/a/b",
		"/":              "",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, trimTrailingSlash(in))
		})
	}
}

func TestParseECPrivateKeyPEM_RoundTripsEveryAcceptedEncoding(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	t.Run("SEC1 EC PRIVATE KEY", func(t *testing.T) {
		encoded, err := encodeECPrivateKeyPEM(key)
		require.NoError(t, err)

		parsed, err := parseECPrivateKeyPEM(encoded)

		require.NoError(t, err)
		assert.True(t, key.Equal(parsed))
	})

	t.Run("PKCS8 PRIVATE KEY", func(t *testing.T) {
		der, err := x509.MarshalPKCS8PrivateKey(key)
		require.NoError(t, err)

		parsed, err := parseECPrivateKeyPEM(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))

		require.NoError(t, err)
		assert.True(t, key.Equal(parsed))
	})
}

func TestParseECPrivateKeyPEM_RejectsAnythingThatIsNotAnECKey(t *testing.T) {
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	edDER, err := x509.MarshalPKCS8PrivateKey(edKey)
	require.NoError(t, err)

	tests := []struct {
		name    string
		in      string
		wantMsg string
	}{
		{"no PEM block", "not pem at all", "no PEM block found"},
		{"unexpected PEM type", string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("x")})), `unexpected PEM type "RSA PRIVATE KEY"`},
		{"PKCS8 holding a non-EC key", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: edDER})), "not an EC key"},
		{"PKCS8 with undecodable body", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("junk")})), "parse PKCS8 private key"},
		{"SEC1 with undecodable body", string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("junk")})), "parse EC private key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := parseECPrivateKeyPEM(tt.in)

			require.Error(t, err)
			assert.Nil(t, key)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestCsrFingerprint_RejectsMalformedRequests(t *testing.T) {
	validCSR, _, err := generateTestCSR(t, "fp-test")
	require.NoError(t, err)
	block, _ := pem.Decode([]byte(validCSR))
	require.NotNil(t, block)
	tampered := append([]byte(nil), block.Bytes...)
	tampered[len(tampered)-1] ^= 0xff

	tests := []struct {
		name string
		in   string
	}{
		{"not PEM", "garbage"},
		{"wrong PEM block type", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: block.Bytes}))},
		{"undecodable DER", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: []byte("junk")}))},
		{"tampered signature", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: tampered}))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp, err := csrFingerprint(tt.in)

			require.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidCSR)
			assert.Empty(t, fp)
		})
	}
}

// ---------------------------------------------------------------------------
// submitRequest
// ---------------------------------------------------------------------------

func TestSubmitRequest_SendsTypedOperatorRequestAndReturnsGatewayResponse(t *testing.T) {
	var gotMethod, gotContentType string
	var gotBody models.PlatformEnrollmentCreateRequest
	stub := newEnrollStub(t, enrollRoutes{request: func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		createdReply("req-42", "token-42")(w, r)
	}})
	client, _ := newEnrollClient(t, stub.server.URL)

	resp, err := client.submitRequest(t.Context(), "operator-csr", "cli-csr", "sys-fp")

	require.NoError(t, err)
	assert.Equal(t, "req-42", resp.RequestID)
	assert.Equal(t, "token-42", resp.Token)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, models.PlatformComponentOperator, gotBody.ComponentKind)
	assert.Equal(t, "inst-1", gotBody.InstanceID)
	assert.Equal(t, "host-1", gotBody.Hostname)
	assert.Equal(t, "sys-fp", gotBody.SystemFingerprint)
	require.NotNil(t, gotBody.Operator)
	assert.Equal(t, "operator-csr", gotBody.Operator.OperatorCSRPEM)
	assert.Equal(t, "cli-csr", gotBody.Operator.CLICSRPEM)
}

func TestSubmitRequest_RejectionsAreTerminalAndNotRetried(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"validation failure", http.StatusBadRequest, "csr rejected"},
		{"server error", http.StatusInternalServerError, "boom"},
		{"forbidden for a reason other than bootstrap", http.StatusForbidden, "owner policy denies this"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newEnrollStub(t, enrollRoutes{request: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}})
			client, _ := newEnrollClient(t, stub.server.URL)

			resp, err := client.submitRequest(shortContext(t, 5*time.Second), "op", "cli", "fp")

			require.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), "request rejected")
			assert.Contains(t, err.Error(), tt.body)
			assert.EqualValues(t, 1, stub.requestHits.Load(), "a terminal rejection must not be retried")
		})
	}
}

func TestSubmitRequest_WaitsForGatewayBootstrapInsteadOfFailing(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{request: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, constants.ErrPlatformEnrollmentRequiresBootstrap.Error())
	}})
	client, _ := newEnrollClient(t, stub.server.URL)

	resp, err := client.submitRequest(shortContext(t, 300*time.Millisecond), "op", "cli", "fp")

	require.ErrorIs(t, err, context.DeadlineExceeded, "the client waits for bootstrap until its context ends")
	assert.Nil(t, resp)
	assert.EqualValues(t, 1, stub.requestHits.Load(), "the client must back off rather than hammer an unbootstrapped gateway")
}

func TestSubmitRequest_RetriesNetworkFailuresUntilContextEnds(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{})
	url := stub.server.URL
	stub.server.Close()
	client, _ := newEnrollClient(t, url)

	resp, err := client.submitRequest(shortContext(t, 300*time.Millisecond), "op", "cli", "fp")

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Nil(t, resp)
}

func TestSubmitRequest_ReturnsContextErrorWithoutContactingGatewayWhenAlreadyCancelled(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{request: createdReply("r", "t")})
	client, _ := newEnrollClient(t, stub.server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.submitRequest(ctx, "op", "cli", "fp")

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, stub.requestHits.Load())
}

func TestSubmitRequest_RejectsUnparseableCreatedResponse(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{request: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "{not json")
	}})
	client, _ := newEnrollClient(t, stub.server.URL)

	_, err := client.submitRequest(t.Context(), "op", "cli", "fp")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse response")
}

// ---------------------------------------------------------------------------
// pollUntilApproved
// ---------------------------------------------------------------------------

func TestPollUntilApproved_TerminalStates(t *testing.T) {
	tests := []struct {
		name    string
		state   models.PlatformEnrollmentState
		wantErr string
	}{
		{"approved", models.PlatformEnrollmentStateApproved, ""},
		{"already completed", models.PlatformEnrollmentStateCompleted, ""},
		{"denied by the owner", models.PlatformEnrollmentStateDenied, "denied by the owner"},
		{"expired", models.PlatformEnrollmentStateExpired, "has expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newEnrollStub(t, enrollRoutes{status: statusReply(tt.state)})
			client, _ := newEnrollClient(t, stub.server.URL)

			err := client.pollUntilApproved(t.Context(), "tok", time.Minute)

			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
			assert.EqualValues(t, 1, stub.statusHits.Load(), "a terminal state ends polling immediately")
		})
	}
}

func TestPollUntilApproved_EscapesTheRequesterTokenInTheQuery(t *testing.T) {
	const token = "a b&c=d/+?"
	var gotToken, gotCacheControl string
	stub := newEnrollStub(t, enrollRoutes{status: func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.URL.Query().Get("token")
		gotCacheControl = r.Header.Get("Cache-Control")
		statusReply(models.PlatformEnrollmentStateApproved)(w, r)
	}})
	client, _ := newEnrollClient(t, stub.server.URL)

	require.NoError(t, client.pollUntilApproved(t.Context(), token, time.Minute))

	assert.Equal(t, token, gotToken, "the token must survive URL encoding intact")
	assert.Equal(t, "no-store", gotCacheControl)
}

func TestPollUntilApproved_FailsOnUnexpectedGatewayAnswers(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantMsg string
	}{
		{
			name: "non-OK status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, "unknown token")
			},
			wantMsg: "status query failed: HTTP 404: unknown token",
		},
		{
			name: "malformed body",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, "{nope")
			},
			wantMsg: "parse status response",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newEnrollStub(t, enrollRoutes{status: tt.handler})
			client, _ := newEnrollClient(t, stub.server.URL)

			err := client.pollUntilApproved(t.Context(), "tok", time.Minute)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.EqualValues(t, 1, stub.statusHits.Load(), "unexpected answers are not retried")
		})
	}
}

func TestPollUntilApproved_StopsWhenDeadlineHasPassed(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{status: statusReply(models.PlatformEnrollmentStateApproved)})
	client, _ := newEnrollClient(t, stub.server.URL)

	err := client.pollUntilApproved(t.Context(), "tok", -time.Second)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "polling deadline reached before approval")
	assert.Zero(t, stub.statusHits.Load(), "an expired attempt must not query the gateway")
}

func TestPollUntilApproved_ReturnsContextErrorWhenCancelled(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{status: statusReply(models.PlatformEnrollmentStateApproved)})
	client, _ := newEnrollClient(t, stub.server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.pollUntilApproved(ctx, "tok", time.Minute)

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, stub.statusHits.Load())
}

func TestPollUntilApproved_HonorsGatewayBackpressure(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "429 with Retry-After",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
			},
		},
		{
			name: "429 without Retry-After falls back to the client backoff",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
		},
		{
			name: "still pending with Retry-After",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "60")
				statusReply(models.PlatformEnrollmentStatePending)(w, r)
			},
		},
		{
			name:    "still pending without Retry-After",
			handler: statusReply(models.PlatformEnrollmentStatePending),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newEnrollStub(t, enrollRoutes{status: tt.handler})
			client, _ := newEnrollClient(t, stub.server.URL)

			err := client.pollUntilApproved(shortContext(t, 300*time.Millisecond), "tok", time.Minute)

			require.ErrorIs(t, err, context.DeadlineExceeded, "polling waits out the backoff until the caller's context ends")
			assert.EqualValues(t, 1, stub.statusHits.Load(), "the client must back off instead of re-polling immediately")
		})
	}
}

func TestPollUntilApproved_RetriesNetworkFailuresUntilContextEnds(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{})
	url := stub.server.URL
	stub.server.Close()
	client, _ := newEnrollClient(t, url)

	err := client.pollUntilApproved(shortContext(t, 300*time.Millisecond), "tok", time.Minute)

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// ---------------------------------------------------------------------------
// submitCompletion
// ---------------------------------------------------------------------------

func TestSubmitCompletion_SendsTokenAndBothProofs(t *testing.T) {
	var gotBody models.PlatformEnrollmentCompleteRequest
	var gotCacheControl string
	stub := newEnrollStub(t, enrollRoutes{complete: func(w http.ResponseWriter, r *http.Request) {
		gotCacheControl = r.Header.Get("Cache-Control")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		respondJSON(w, http.StatusCreated, models.PlatformEnrollmentCompleteResponse{
			RequestID: "req-1", ComponentKind: models.PlatformComponentOperator,
			Operator: &models.PlatformEnrollmentOperatorCredentials{OperatorID: "op-9"},
		})
	}})
	client, _ := newEnrollClient(t, stub.server.URL)

	resp, err := client.submitCompletion(t.Context(), "tok", "operator-proof", "cli-proof")

	require.NoError(t, err)
	require.NotNil(t, resp.Operator)
	assert.Equal(t, "op-9", resp.Operator.OperatorID)
	assert.Equal(t, "tok", gotBody.Token)
	assert.Equal(t, "operator-proof", gotBody.Proofs.Operator)
	assert.Equal(t, "cli-proof", gotBody.Proofs.CLI)
	assert.Equal(t, "no-store", gotCacheControl)
}

func TestSubmitCompletion_FailsOnRejectionMalformedBodyOrUnreachableGateway(t *testing.T) {
	t.Run("rejected by the gateway", func(t *testing.T) {
		stub := newEnrollStub(t, enrollRoutes{complete: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "proof invalid")
		}})
		client, _ := newEnrollClient(t, stub.server.URL)

		_, err := client.submitCompletion(t.Context(), "tok", "a", "b")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "completion rejected: HTTP 403: proof invalid")
	})

	t.Run("an OK status is not accepted as a completion", func(t *testing.T) {
		stub := newEnrollStub(t, enrollRoutes{complete: func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(w, http.StatusOK, models.PlatformEnrollmentCompleteResponse{})
		}})
		client, _ := newEnrollClient(t, stub.server.URL)

		_, err := client.submitCompletion(t.Context(), "tok", "a", "b")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "completion rejected: HTTP 200")
	})

	t.Run("malformed completion body", func(t *testing.T) {
		stub := newEnrollStub(t, enrollRoutes{complete: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, "{nope")
		}})
		client, _ := newEnrollClient(t, stub.server.URL)

		_, err := client.submitCompletion(t.Context(), "tok", "a", "b")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse completion response")
	})

	t.Run("gateway unreachable", func(t *testing.T) {
		stub := newEnrollStub(t, enrollRoutes{})
		url := stub.server.URL
		stub.server.Close()
		client, _ := newEnrollClient(t, url)

		_, err := client.submitCompletion(t.Context(), "tok", "a", "b")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "submit completion")
	})
}

// ---------------------------------------------------------------------------
// writeCredentials
// ---------------------------------------------------------------------------

func readRuntimeFile(t *testing.T, fileSvc fs.RuntimeFileService, rel string) (string, os.FileMode) {
	t.Helper()
	data, err := fileSvc.ReadFile(t.Context(), rel)
	require.NoError(t, err)
	info, err := fileSvc.Stat(t.Context(), rel)
	require.NoError(t, err)
	return string(data), info.Mode().Perm()
}

func TestWriteCredentials_PersistsChainsKeysTrustBundleAndActuatorSigner(t *testing.T) {
	client, fileSvc := newEnrollClient(t, "http://gw:8080")
	creds := &models.PlatformEnrollmentOperatorCredentials{
		OperatorCert:      "OPERATOR-CERT",
		OperatorCertChain: "OPERATOR-CHAIN",
		CLICert:           "CLI-CERT",
		CLICertChain:      "CLI-CHAIN",
		HubTrustBundle:    "TRUST-BUNDLE",
		ActuatorKeyID:     "actuator-key-1",
		ActuatorPubKey:    "ACTUATOR-PUB",
	}

	require.NoError(t, client.writeCredentials(creds, "OPERATOR-KEY", "CLI-KEY"))

	content, perm := readRuntimeFile(t, fileSvc, client.operatorCertPath())
	assert.Equal(t, "OPERATOR-CERT\nOPERATOR-CHAIN", content, "the chain is appended to the leaf certificate")
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), perm)

	content, perm = readRuntimeFile(t, fileSvc, client.operatorKeyPath())
	assert.Equal(t, "OPERATOR-KEY", content)
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), perm, "private keys must be 0600")

	content, perm = readRuntimeFile(t, fileSvc, client.cliCertPath())
	assert.Equal(t, "CLI-CERT\nCLI-CHAIN", content)
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), perm)

	content, perm = readRuntimeFile(t, fileSvc, client.cliKeyPath())
	assert.Equal(t, "CLI-KEY", content)
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), perm)

	content, perm = readRuntimeFile(t, fileSvc, client.trustBundlePath())
	assert.Equal(t, "TRUST-BUNDLE", content)
	assert.Equal(t, os.FileMode(constants.PermFilePublic), perm, "the trust bundle is public")

	signerPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrustedSigners, "actuator-key-1"+constants.PublicKeySuffix)
	content, perm = readRuntimeFile(t, fileSvc, signerPath)
	assert.Equal(t, "ACTUATOR-PUB", content)
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), perm)
}

func TestWriteCredentials_OmitsOptionalMaterialThatWasNotIssued(t *testing.T) {
	tests := []struct {
		name  string
		creds *models.PlatformEnrollmentOperatorCredentials
	}{
		{"no trust bundle and no actuator key", &models.PlatformEnrollmentOperatorCredentials{OperatorCert: "OC", CLICert: "CC"}},
		{"actuator key id without key material", &models.PlatformEnrollmentOperatorCredentials{OperatorCert: "OC", CLICert: "CC", ActuatorKeyID: "k1"}},
		{"actuator key material without key id", &models.PlatformEnrollmentOperatorCredentials{OperatorCert: "OC", CLICert: "CC", ActuatorPubKey: "PUB"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, fileSvc := newEnrollClient(t, "http://gw:8080")

			require.NoError(t, client.writeCredentials(tt.creds, "OK", "CK"))

			content, _ := readRuntimeFile(t, fileSvc, client.operatorCertPath())
			assert.Equal(t, "OC", content, "no chain means the leaf certificate alone")
			assert.NoFileExists(t, fileSvc.Resolve(client.trustBundlePath()))
			signers := fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirTrustedSigners, "k1"+constants.PublicKeySuffix))
			assert.NoFileExists(t, signers)
		})
	}
}

// ---------------------------------------------------------------------------
// Enroll validation and failure branches
// ---------------------------------------------------------------------------

func TestEnroll_RefusesCorruptPendingStateWithoutContactingGateway(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{request: createdReply("r", "t")})
	client, fileSvc := newEnrollClient(t, stub.server.URL)
	require.NoError(t, fileSvc.MkdirAll(t.Context(), filepath.Dir(client.pendingStatePath()), constants.PermDirPrivate))
	require.NoError(t, fileSvc.WriteFile(t.Context(), client.pendingStatePath(), []byte("{not json"), constants.PermFilePrivate))

	result, err := client.Enroll(shortContext(t, 5*time.Second))

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "parse pending state")
	assert.Zero(t, stub.requestHits.Load(), "a corrupt pending file must not silently trigger a brand-new enrollment")
}

func TestEnroll_ResumeRejectsCorruptPersistedKeys(t *testing.T) {
	goodKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	goodPEM, err := encodeECPrivateKeyPEM(goodKey)
	require.NoError(t, err)

	tests := []struct {
		name                string
		operatorKey, cliKey string
		wantMsg             string
	}{
		{"corrupt operator key", "garbage", goodPEM, "resume operator key"},
		{"corrupt cli key", goodPEM, "garbage", "resume cli key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newEnrollStub(t, enrollRoutes{})
			client, _ := newEnrollClient(t, stub.server.URL)
			require.NoError(t, client.persistPendingState(client.pendingStatePath(), &operatorPendingState{
				RequestID: "req-1", Token: "tok", OperatorKeyPEM: tt.operatorKey, CLIKeyPEM: tt.cliKey,
				ExpiresAt: time.Now().Add(time.Hour),
			}))

			_, err := client.Enroll(shortContext(t, 5*time.Second))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.Zero(t, stub.statusHits.Load(), "an unusable resume state must not poll the gateway")
		})
	}
}

func TestEnroll_RequestRejectionLeavesNoPendingState(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{request: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "invalid csr")
	}})
	client, fileSvc := newEnrollClient(t, stub.server.URL)

	_, err := client.Enroll(shortContext(t, 5*time.Second))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "request rejected")
	exists, err := fileSvc.FileExists(t.Context(), client.pendingStatePath())
	require.NoError(t, err)
	assert.False(t, exists, "a rejected request must not leave resumable state behind")
}

func TestEnroll_RefusesDeduplicatedResponseThatCarriesNoToken(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{request: createdReply("dup-req", "")})
	client, fileSvc := newEnrollClient(t, stub.server.URL)

	_, err := client.Enroll(shortContext(t, 5*time.Second))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no token")
	assert.Contains(t, err.Error(), "dup-req", "the error names the request so the owner can find it")
	exists, err := fileSvc.FileExists(t.Context(), client.pendingStatePath())
	require.NoError(t, err)
	assert.False(t, exists, "without a token there is nothing resumable to persist")
	assert.Zero(t, stub.statusHits.Load())
}

func TestEnroll_RejectsIncompleteCompletionResponses(t *testing.T) {
	tests := []struct {
		name     string
		response models.PlatformEnrollmentCompleteResponse
		wantMsg  string
	}{
		{
			name:     "no operator credentials",
			response: models.PlatformEnrollmentCompleteResponse{RequestID: "req-1", ComponentKind: models.PlatformComponentOperator},
			wantMsg:  "missing operator credentials",
		},
		{
			name: "operator certificate missing",
			response: models.PlatformEnrollmentCompleteResponse{
				RequestID: "req-1", ComponentKind: models.PlatformComponentOperator,
				Operator: &models.PlatformEnrollmentOperatorCredentials{CLICert: "cli"},
			},
			wantMsg: "missing certificates",
		},
		{
			name: "cli certificate missing",
			response: models.PlatformEnrollmentCompleteResponse{
				RequestID: "req-1", ComponentKind: models.PlatformComponentOperator,
				Operator: &models.PlatformEnrollmentOperatorCredentials{OperatorCert: "op"},
			},
			wantMsg: "missing certificates",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newEnrollStub(t, enrollRoutes{
				request: createdReply("req-1", "tok-1"),
				status:  statusReply(models.PlatformEnrollmentStateApproved),
				complete: func(w http.ResponseWriter, _ *http.Request) {
					respondJSON(w, http.StatusCreated, tt.response)
				},
			})
			client, fileSvc := newEnrollClient(t, stub.server.URL)

			result, err := client.Enroll(shortContext(t, 10*time.Second))

			require.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.NoFileExists(t, fileSvc.Resolve(client.operatorCertPath()), "no credentials may be written from an incomplete response")
			assert.NoFileExists(t, fileSvc.Resolve(client.operatorKeyPath()))
			exists, err := fileSvc.FileExists(t.Context(), client.pendingStatePath())
			require.NoError(t, err)
			assert.True(t, exists, "pending state is kept so the attempt can be resumed")
		})
	}
}

// csrPublicKey extracts the ECDSA public key a CSR PEM commits to.
func csrPublicKey(t *testing.T, csrPEM string) *ecdsa.PublicKey {
	t.Helper()
	block, _ := pem.Decode([]byte(csrPEM))
	require.NotNil(t, block)
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	key, ok := csr.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok, "CSR must carry an ECDSA key")
	return key
}

func TestEnroll_ProofsVerifyAgainstTheKeysSubmittedInTheRequest(t *testing.T) {
	var submitted models.PlatformEnrollmentCreateRequest
	var proofs models.PlatformEnrollmentProofs
	stub := newEnrollStub(t, enrollRoutes{
		request: func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&submitted))
			createdReply("req-1", "tok-1")(w, r)
		},
		status: statusReply(models.PlatformEnrollmentStateApproved),
		complete: func(w http.ResponseWriter, r *http.Request) {
			var req models.PlatformEnrollmentCompleteRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			proofs = req.Proofs
			respondJSON(w, http.StatusCreated, models.PlatformEnrollmentCompleteResponse{
				RequestID: "req-1", ComponentKind: models.PlatformComponentOperator,
				Operator: &models.PlatformEnrollmentOperatorCredentials{
					OperatorCert: generateSelfSignedCertPEM(t, "op"), CLICert: generateSelfSignedCertPEM(t, "cli"),
					OperatorID: "op-1", OperatorSessionID: "sess-1", CLISessionID: "cli-1", Posture: "consensus",
				},
			})
		},
	})
	client, _ := newEnrollClient(t, stub.server.URL)

	result, err := client.Enroll(shortContext(t, 10*time.Second))

	require.NoError(t, err)
	assert.Equal(t, "op-1", result.OperatorID)
	assert.Equal(t, "consensus", result.Posture)
	require.NotNil(t, submitted.Operator)
	operatorFP, err := csrFingerprint(submitted.Operator.OperatorCSRPEM)
	require.NoError(t, err)
	cliFP, err := csrFingerprint(submitted.Operator.CLICSRPEM)
	require.NoError(t, err)
	assert.NotEqual(t, operatorFP, cliFP, "the operator and CLI identities use distinct keys")

	// The gateway verifies each proof against the key committed to in the
	// matching CSR, over the canonical transcript for this request.
	transcript, err := buildOperatorCompletionTranscript("req-1", tokenHash("tok-1"), "inst-1", operatorFP, cliFP)
	require.NoError(t, err)
	digest := sha256.Sum256(transcript)
	verify := func(proof string, pub *ecdsa.PublicKey) bool {
		sig, decodeErr := base64.RawURLEncoding.DecodeString(proof)
		require.NoError(t, decodeErr)
		return ecdsa.VerifyASN1(pub, digest[:], sig)
	}
	operatorKey := csrPublicKey(t, submitted.Operator.OperatorCSRPEM)
	cliKey := csrPublicKey(t, submitted.Operator.CLICSRPEM)
	assert.True(t, verify(proofs.Operator, operatorKey), "the operator proof must verify against the operator CSR key")
	assert.True(t, verify(proofs.CLI, cliKey), "the CLI proof must verify against the CLI CSR key")
	assert.False(t, verify(proofs.Operator, cliKey), "the operator proof must not verify against the CLI key")
	assert.False(t, verify(proofs.CLI, operatorKey), "the CLI proof must not verify against the operator key")
}
