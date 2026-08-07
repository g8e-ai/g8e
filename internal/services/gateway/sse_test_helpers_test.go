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
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

// makeTestAppCert creates a self-signed x509 certificate with app SPIFFE URI SANs.
// Used to simulate mTLS app workload identities for SSE push tests.
func makeTestAppCert(t *testing.T, spiffeURIs []string) *x509.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	var uris []*url.URL
	for _, s := range spiffeURIs {
		u, err := url.Parse(s)
		require.NoError(t, err)
		uris = append(uris, u)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-app-cert"},
		NotBefore:    time.Now().Add(-1 * time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		URIs:         uris,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	return cert
}

// makeTLSRequest creates an http.Request with r.TLS set to simulate mTLS auth.
func makeTLSRequest(method, path string, body string, cert *x509.Certificate) *http.Request {
	var bodyReader strings.Reader
	if body != "" {
		bodyReader = *strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, &bodyReader)
	if cert != nil {
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		}
	}
	return req
}

// seedOperatorDoc inserts an operator document into the DocStore for test setup.
// The document is stored with operatorSessionID as the key, matching the SSE push
// code's DocGet(operators, operatorSessionID) lookup pattern.
func seedOperatorDoc(t *testing.T, h *HTTPHandler, opID, userID, operatorSessionID string) {
	t.Helper()
	op := models.OperatorDocumentGo{
		ID:                opID,
		UserID:            userID,
		Status:            constants.OperatorStatusActive,
		OperatorSessionID: operatorSessionID,
	}
	opBytes, err := json.Marshal(op)
	require.NoError(t, err)
	err = h.dataController.docStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorSessionID, opBytes)
	require.NoError(t, err)
}

// seedUserDoc inserts a user document with active status into the DocStore.
func seedUserDoc(t *testing.T, h *HTTPHandler, userID string) {
	t.Helper()
	userBytes, err := json.Marshal(map[string]interface{}{
		"status": string(constants.UserStatusActive),
	})
	require.NoError(t, err)
	err = h.dataController.docStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes)
	require.NoError(t, err)
}

// seedCLISessionDoc inserts a CLI session document into the DocStore for test setup.
func seedCLISessionDoc(t *testing.T, h *HTTPHandler, cliSessionID, userID, operatorSessionID string) {
	t.Helper()
	cliSess := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
	}
	cliBytes, err := json.Marshal(cliSess)
	require.NoError(t, err)
	err = h.dataController.docStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliBytes)
	require.NoError(t, err)
}

// bindWebSessionToOperators sets the KV binding from web session to operator session IDs.
func bindWebSessionToOperators(t *testing.T, h *HTTPHandler, webSessionID string, operatorSessionIDs []string) {
	t.Helper()
	raw, err := json.Marshal(operatorSessionIDs)
	require.NoError(t, err)
	err = h.dataController.kvStore.KVSet(sessionWebBindKey(webSessionID), string(raw), 0)
	require.NoError(t, err)
}

// bindOperatorToWebSession sets the KV binding from operator session to web session ID.
func bindOperatorToWebSession(t *testing.T, h *HTTPHandler, operatorSessionID, webSessionID string) {
	t.Helper()
	err := h.dataController.kvStore.KVSet(sessionOperatorBindKey(operatorSessionID), webSessionID, 0)
	require.NoError(t, err)
}

// seedCLISessionCtx seeds a CLI session document and returns a pre-stamped context
// plus the generated IDs. This reduces the repeated seedCLISessionDoc + context
// stamping boilerplate (opSessID + userID + cliSessionID) in stream/authorize tests.
// The suffix is used to generate unique IDs per test to avoid cross-test collisions.
func seedCLISessionCtx(t *testing.T, h *HTTPHandler, suffix string) (ctx context.Context, userID, cliSessionID, opSessID string) {
	t.Helper()
	opSessID = "opsess-" + suffix
	userID = "user-" + suffix
	cliSessionID = "cli-" + suffix
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)
	ctx = context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, cliSessionID)
	return ctx, userID, cliSessionID, opSessID
}

// runStreamWithCancel runs handleInternalSSEStream in a goroutine with a cancellable
// context, sleeps for sleepDur, then cancels and waits for the goroutine to finish.
// Returns the response recorder and body. This encapsulates the goroutine + cancel +
// sleep + <-done pattern repeated across stream tests. Tests that need to publish
// pub/sub events mid-stream or use a custom ResponseWriter should inline their own
// goroutine logic instead.
func runStreamWithCancel(t *testing.T, h *HTTPHandler, req *http.Request, sleepDur time.Duration) (*httptest.ResponseRecorder, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	streamCtx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(rr, req)
		close(done)
	}()

	time.Sleep(sleepDur)
	cancel()
	<-done

	return rr, rr.Body.String()
}

// errorWriter implements http.ResponseWriter but always returns an error on Write.
// It also implements http.Flusher as a no-op so the SSE handler can proceed.
type errorWriter struct {
	header http.Header
}

func (w *errorWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *errorWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("broken pipe: simulated write error")
}

func (w *errorWriter) WriteHeader(int) {}

func (w *errorWriter) Flush() {}
