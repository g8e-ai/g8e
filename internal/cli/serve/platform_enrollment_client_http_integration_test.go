// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package serve

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

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

func TestSubmitRequest_SendsTypedOperatorRequestAndReturnsGatewayResponse(t *testing.T) {
	var gotMethod, gotContentType string
	var gotBody models.PlatformEnrollmentCreateRequest
	stub := newEnrollStub(t, enrollRoutes{request: func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		createdReply("req-42")(w, r)
	}})
	client, _ := newEnrollClient(t, stub.server.URL)

	resp, err := client.submitRequest(t.Context(), "operator-csr", "cli-csr", "sys-fp", "tok-42")

	require.NoError(t, err)
	assert.Equal(t, "req-42", resp.RequestID)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, models.PlatformComponentOperator, gotBody.ComponentKind)
	assert.Equal(t, "inst-1", gotBody.InstanceID)
	assert.Equal(t, "host-1", gotBody.Hostname)
	assert.Equal(t, "sys-fp", gotBody.SystemFingerprint)
	assert.Equal(t, models.PlatformEnrollmentTokenHash("tok-42"), gotBody.TokenHash)
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

			resp, err := client.submitRequest(shortContext(t, 5*time.Second), "op", "cli", "fp", "tok")

			require.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), "request rejected")
			assert.Contains(t, err.Error(), tt.body)
			assert.EqualValues(t, 1, stub.requestHits.Load(), "a terminal rejection must not be retried")
		})
	}
}

func TestSubmitRequest_BootstrapAndNetworkFailuresAreReturnedWithoutRetry(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{request: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, constants.ErrPlatformEnrollmentRequiresBootstrap.Error())
	}})
	client, _ := newEnrollClient(t, stub.server.URL)
	_, err := client.submitRequest(t.Context(), "op", "cli", "fp", "tok")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request rejected: HTTP 403")
	assert.EqualValues(t, 1, stub.requestHits.Load())
	stub.server.Close()
	_, err = client.submitRequest(t.Context(), "op", "cli", "fp", "tok")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "submit request")
}

func TestSubmitRequest_ReturnsContextErrorWithoutContactingGatewayWhenAlreadyCancelled(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{request: createdReply("r")})
	client, _ := newEnrollClient(t, stub.server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.submitRequest(ctx, "op", "cli", "fp", "tok")

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, stub.requestHits.Load())
}

func TestSubmitRequest_RejectsUnparseableCreatedResponse(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{request: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "{not json")
	}})
	client, _ := newEnrollClient(t, stub.server.URL)

	_, err := client.submitRequest(t.Context(), "op", "cli", "fp", "tok")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse response")
}

func TestAwaitApproval_TerminalStates(t *testing.T) {
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

			err := client.awaitApproval(t.Context(), "tok", time.Minute)

			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
			assert.EqualValues(t, 1, stub.statusHits.Load(), "a terminal state completes the held request")
		})
	}
}

func TestAwaitApproval_EscapesTheRequesterTokenInTheQuery(t *testing.T) {
	const token = "a b&c=d/+?"
	var gotToken, gotCacheControl string
	stub := newEnrollStub(t, enrollRoutes{status: func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.URL.Query().Get("token")
		gotCacheControl = r.Header.Get("Cache-Control")
		statusReply(models.PlatformEnrollmentStateApproved)(w, r)
	}})
	client, _ := newEnrollClient(t, stub.server.URL)

	require.NoError(t, client.awaitApproval(t.Context(), token, time.Minute))

	assert.Equal(t, token, gotToken, "the token must survive URL encoding intact")
	assert.Equal(t, "no-store", gotCacheControl)
}

func TestAwaitApproval_FailsOnUnexpectedGatewayAnswers(t *testing.T) {
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

			err := client.awaitApproval(t.Context(), "tok", time.Minute)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.EqualValues(t, 1, stub.statusHits.Load(), "unexpected answers are not retried")
		})
	}
}

func TestAwaitApproval_StopsWhenDeadlineHasPassed(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{status: statusReply(models.PlatformEnrollmentStateApproved)})
	client, _ := newEnrollClient(t, stub.server.URL)

	err := client.awaitApproval(t.Context(), "tok", -time.Second)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "approval deadline reached before approval")
	assert.Zero(t, stub.statusHits.Load(), "an expired attempt must not query the gateway")
}

func TestAwaitApproval_ReturnsContextErrorWhenCancelled(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{status: statusReply(models.PlatformEnrollmentStateApproved)})
	client, _ := newEnrollClient(t, stub.server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.awaitApproval(ctx, "tok", time.Minute)

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, stub.statusHits.Load())
}

func TestAwaitApproval_HoldsOneRequestUntilDecision(t *testing.T) {
	started := make(chan struct{})
	decided := make(chan struct{})
	stub := newEnrollStub(t, enrollRoutes{status: func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "true", r.URL.Query().Get("wait"))
		close(started)
		select {
		case <-decided:
			statusReply(models.PlatformEnrollmentStateApproved)(w, r)
		case <-r.Context().Done():
		}
	}})
	client, _ := newEnrollClient(t, stub.server.URL)
	done := make(chan error, 1)
	go func() { done <- client.awaitApproval(t.Context(), "tok", time.Minute) }()
	<-started
	close(decided)
	require.NoError(t, <-done)
	assert.EqualValues(t, 1, stub.statusHits.Load())
}

func TestAwaitApproval_CancellationEndsHeldRequest(t *testing.T) {
	started := make(chan struct{})
	stub := newEnrollStub(t, enrollRoutes{status: func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}})
	client, _ := newEnrollClient(t, stub.server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- client.awaitApproval(ctx, "tok", time.Minute) }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	assert.EqualValues(t, 1, stub.statusHits.Load())
}

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

func TestEnroll_RefusesCorruptPendingStateWithoutContactingGateway(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{request: createdReply("r")})
	client, fileSvc := newEnrollClient(t, stub.server.URL)
	require.NoError(t, fileSvc.MkdirAll(t.Context(), filepath.Dir(client.pendingStatePath()), constants.PermDirPrivate))
	require.NoError(t, fileSvc.WriteFile(t.Context(), client.pendingStatePath(), []byte("{not json"), constants.PermFilePrivate))

	result, err := client.Enroll(shortContext(t, 5*time.Second))

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "parse pending state")
	assert.Zero(t, stub.requestHits.Load(), "a corrupt pending file must not silently trigger a brand-new enrollment")
}

func TestEnroll_ExpiredPendingStateStartsFresh(t *testing.T) {
	goodKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	goodPEM, err := encodeECPrivateKeyPEM(goodKey)
	require.NoError(t, err)

	stub := newEnrollStub(t, enrollRoutes{request: createdReply("new-req")})
	client, fileSvc := newEnrollClient(t, stub.server.URL)
	require.NoError(t, client.persistPendingState(client.pendingStatePath(), &operatorPendingState{
		RequestID: "old-expired-req", Token: "old-tok", OperatorKeyPEM: goodPEM, CLIKeyPEM: goodPEM,
		ExpiresAt: time.Now().Add(-time.Hour),
	}))

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, _ = client.Enroll(ctx)

	assert.Equal(t, int32(1), stub.requestHits.Load(), "an expired pending file must trigger a brand-new enrollment request")
	pending, err := client.loadPendingState(client.pendingStatePath())
	require.NoError(t, err)
	if assert.NotNil(t, pending) {
		assert.Equal(t, "new-req", pending.RequestID)
	}
	_ = fileSvc
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

func TestEnroll_RequestRejectionPreservesPreSubmittedState(t *testing.T) {
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
	assert.True(t, exists, "the token and request material must survive an uncertain submission")
}

func TestEnroll_ConflictClearsPendingStateForFreshStart(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{request: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}})
	client, fileSvc := newEnrollClient(t, stub.server.URL)

	_, err := client.Enroll(shortContext(t, 5*time.Second))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "start enrollment again")
	exists, err := fileSvc.FileExists(t.Context(), client.pendingStatePath())
	require.NoError(t, err)
	assert.False(t, exists, "a conflicting pending state must be cleared")
}

func TestEnroll_LostCreateResponseResumesSamePersistedRequest(t *testing.T) {
	var hits atomic.Int32
	var submitted []models.PlatformEnrollmentCreateRequest
	stub := newEnrollStub(t, enrollRoutes{
		request: func(w http.ResponseWriter, r *http.Request) {
			var req models.PlatformEnrollmentCreateRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			submitted = append(submitted, req)
			if hits.Add(1) == 1 {
				hijacker := w.(http.Hijacker)
				conn, _, err := hijacker.Hijack()
				require.NoError(t, err)
				_ = conn.Close()
				return
			}
			createdReply("req-after-loss")(w, r)
		},
		status: statusReply(models.PlatformEnrollmentStatePending),
	})
	client, fileSvc := newEnrollClient(t, stub.server.URL)
	recorder, err := NewOperatorDeploymentRecorder(fileSvc, "lost-response-launch")
	require.NoError(t, err)
	client.SetDeploymentRecorder(recorder)

	_, firstErr := client.Enroll(shortContext(t, 5*time.Second))
	require.Error(t, firstErr)
	pending, err := client.loadPendingState(client.pendingStatePath())
	require.NoError(t, err)
	require.NotNil(t, pending)
	assert.Empty(t, pending.RequestID, "the state is persisted before the first POST returns")
	assert.NotEmpty(t, pending.Token)
	assert.NotEmpty(t, pending.OperatorCSRPEM)
	assert.NotEmpty(t, pending.CLICSRPEM)
	assert.NotEmpty(t, pending.SystemFingerprint)

	_, _ = client.Enroll(shortContext(t, 100*time.Millisecond))
	require.Len(t, submitted, 2)
	assert.Equal(t, submitted[0].TokenHash, submitted[1].TokenHash)
	assert.Equal(t, submitted[0].Operator.OperatorCSRPEM, submitted[1].Operator.OperatorCSRPEM)
	assert.Equal(t, submitted[0].Operator.CLICSRPEM, submitted[1].Operator.CLICSRPEM)
	assert.Equal(t, models.PlatformEnrollmentTokenHash(pending.Token), submitted[0].TokenHash)
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
				request: createdReply("req-1"),
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

func TestEnroll_ProofsVerifyAgainstTheKeysSubmittedInTheRequest(t *testing.T) {
	var submitted models.PlatformEnrollmentCreateRequest
	var proofs models.PlatformEnrollmentProofs
	stub := newEnrollStub(t, enrollRoutes{
		request: func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&submitted))
			createdReply("req-1")(w, r)
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
	transcript, err := buildOperatorCompletionTranscript("req-1", submitted.TokenHash, "inst-1", operatorFP, cliFP)
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

func TestSubmitCompletion_HeldLeaseIsReturnedWithoutRetry(t *testing.T) {
	stub := newEnrollStub(t, enrollRoutes{complete: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}})
	client, _ := newEnrollClient(t, stub.server.URL)
	_, err := client.submitCompletion(t.Context(), "tok", "a", "b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "completion rejected: HTTP 429")
	assert.EqualValues(t, 1, stub.completeHit.Load())
}
