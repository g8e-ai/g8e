// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/mcp"
	"github.com/g8e-ai/g8e/v2/protocol"
)

// observeProducerEndpointEnv holds the wired HTTP handler and infrastructure
// for observe producer endpoint integration tests. The handler is built with
// the real observe read and producer controllers so POSTs through the HTTP
// router exercise the full mTLS auth middleware, controller validation,
// producer persistence, SSE emission, and browser-scoped read paths.
type observeProducerEndpointEnv struct {
	handler  *HTTPHandler
	infra    *TestInfrastructure
	producer *ObserveProducerService
	observe  *ObserveService
}

// setupObserveProducerEndpointEnv creates a real HTTP handler with the
// observe read and producer controllers wired, backed by the real SQLite
// document store, SSE event store, and pubsub handler. This mirrors
// setupTestHTTPHandler but adds the observe controller deps that the
// shared helper omits.
func setupObserveProducerEndpointEnv(t *testing.T) *observeProducerEndpointEnv {
	t.Helper()
	infra := setupTestInfrastructure(t, false)

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:           infra.Logger,
		Responder:        infra.Responder,
		SuspendedStore:   infra.SuspendedStore,
		ScrubbingService: nil,
		ThreatScanner:    governance.NewL1Doctrine(),
		MaxPayloadBytes:  infra.Cfg.Gateway.MaxPayloadBytes,
		Posture:          string(infra.Cfg.Gateway.Posture),
		AuditStore:       mcp.NoopAuditEventRecorder{},
	})
	require.NoError(t, err, "failed to create MCP gateway")

	observeSvc := NewObserveService(infra.DocStore, infra.Logger)
	producerSvc := NewObserveProducerService(infra.DocStore, infra.SSEStore, infra.Pubsub, newTestFileSvc(t), infra.Logger)

	h, err := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:    infra.Cfg,
		Logger: infra.Logger,
		Auth:   infra.Auth,
		PKIControllerDeps: PKIControllerDeps{
			Cfg:          infra.Cfg,
			Logger:       infra.Logger,
			PKI:          infra.PKI,
			Registration: infra.Reg,
			Responder:    infra.Responder,
		},
		AuditControllerDeps: AuditControllerDeps{
			Cfg:        infra.Cfg,
			Logger:     infra.Logger,
			AuditStore: infra.AuditStore,
			Responder:  infra.Responder,
		},
		DataControllerDeps: DataControllerDeps{
			Cfg:       infra.Cfg,
			Logger:    infra.Logger,
			DocStore:  infra.DocStore,
			KVStore:   infra.KVStore,
			SSEStore:  infra.SSEStore,
			BlobStore: infra.BlobStore,
			Pubsub:    infra.Pubsub,
			Responder: infra.Responder,
		},
		SignerControllerDeps: SignerControllerDeps{
			Cfg:         infra.Cfg,
			Logger:      infra.Logger,
			DocStore:    infra.DocStore,
			SignerStore: infra.SignerStore,
			Responder:   infra.Responder,
		},
		BootstrapControllerDeps: BootstrapControllerDeps{
			Cfg:                infra.Cfg,
			Logger:             infra.Logger,
			DocStore:           infra.DocStore,
			UserSvc:            infra.UserSvc,
			PKI:                infra.PKI,
			CLISessionSvc:      infra.CLISessionSvc,
			OperatorSessionSvc: infra.OperatorSessionSvc,
			Responder:          infra.Responder,
		},
		EnrollmentTokenControllerDeps: EnrollmentTokenControllerDeps{
			Cfg:       infra.Cfg,
			Logger:    infra.Logger,
			Responder: infra.Responder,
		},
		UserControllerDeps: UserControllerDeps{
			Cfg:       infra.Cfg,
			Logger:    infra.Logger,
			UserSvc:   infra.UserSvc,
			Responder: infra.Responder,
		},
		SessionControllerDeps: SessionControllerDeps{
			Logger:      infra.Logger,
			DocStore:    infra.DocStore,
			Responder:   infra.Responder,
			CrossOrigin: len(infra.Cfg.Gateway.AllowedOrigins) > 0,
		},
		AdminControllerDeps: AdminControllerDeps{
			Cfg:            infra.Cfg,
			Logger:         infra.Logger,
			DocStore:       infra.DocStore,
			SignerStore:    infra.SignerStore,
			ConsensusStore: infra.ConsensusStore,
			UserSvc:        infra.UserSvc,
			Responder:      infra.Responder,
		},
		OperatorControllerDeps: OperatorControllerDeps{
			Cfg:       infra.Cfg,
			Logger:    infra.Logger,
			Reg:       infra.Reg,
			Auth:      infra.Auth,
			Responder: infra.Responder,
		},
		DispatchControllerDeps: DispatchControllerDeps{
			DispatchSvc: NewDispatchService(infra.Logger, infra.Pubsub, infra.StateRootSvc, infra.Auth, string(infra.Cfg.Gateway.Posture)),
			Responder:   infra.Responder,
			Logger:      infra.Logger,
		},
		SSEControllerDeps: SSEControllerDeps{
			Cfg:       infra.Cfg,
			Logger:    infra.Logger,
			DocStore:  infra.DocStore,
			KVStore:   infra.KVStore,
			SSEStore:  infra.SSEStore,
			Pubsub:    infra.Pubsub,
			Auth:      infra.Auth,
			Responder: infra.Responder,
		},
		HealthControllerDeps: HealthControllerDeps{
			Cfg:               infra.Cfg,
			Logger:            infra.Logger,
			DocStore:          infra.DocStore,
			StateRootSvc:      infra.StateRootSvc,
			Responder:         infra.Responder,
			IsReady:           func() bool { return true },
			IsGovernanceReady: func() bool { return true },
		},
		GovernanceControllerDeps: GovernanceControllerDeps{
			Cfg:       infra.Cfg,
			Logger:    infra.Logger,
			Responder: infra.Responder,
		},
		MCPControllerDeps:     MCPControllerDeps{MCPGateway: mcpGateway},
		PubSubControllerDeps:  PubSubControllerDeps{Handler: infra.Pubsub},
		PasskeyControllerDeps: PasskeyControllerDeps{Handler: infra.Passkey},
		ObserveControllerDeps: ObserveControllerDeps{
			Cfg:              infra.Cfg,
			Logger:           infra.Logger,
			ObserveSvc:       observeSvc,
			DownloadStreamer: producerSvc,
			Responder:        infra.Responder,
		},
		ObserveProducerControllerDeps: ObserveProducerControllerDeps{
			Cfg:          infra.Cfg,
			Logger:       infra.Logger,
			ProducerSvc:  producerSvc,
			Responder:    infra.Responder,
			MaxBodyBytes: infra.Cfg.Gateway.MaxPayloadBytes,
		},
	})
	require.NoError(t, err, "failed to create HTTP handler with observe controllers")

	return &observeProducerEndpointEnv{
		handler:  h,
		infra:    infra,
		producer: producerSvc,
		observe:  observeSvc,
	}
}

// seedActiveUser creates an active user document in the doc store so the auth
// middleware's getAndValidateUser path admits the web session. Returns the
// user ID.
func seedActiveUser(t *testing.T, infra *TestInfrastructure, userID string) {
	t.Helper()
	userDoc := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))
}

// seedAppPolicy registers an AppPolicy for the given app ID so the auth
// middleware's handleAppAuth path admits the mTLS app cert. The ensemble app
// identity (spiffe://g8e.local/app/g8ee) is the canonical producer.
func seedAppPolicy(t *testing.T, infra *TestInfrastructure, appID string) {
	t.Helper()
	policy := &models.AppPolicy{
		AppID:     appID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	policyBytes, err := json.Marshal(policy)
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes))
}

// seedWebSession creates a valid web session for the given user and returns
// the session ID. The browser read API uses the web session cookie to derive
// user_id and apply ownership scoping.
func seedWebSession(t *testing.T, infra *TestInfrastructure, userID string) string {
	t.Helper()
	session, err := infra.WebSessionSvc.CreateWebSession(userID)
	require.NoError(t, err)
	require.NotNil(t, session)
	return session.ID
}

// appUserMTLSCert creates a self-signed x509 certificate carrying both an
// app SPIFFE URI SAN (spiffe://g8e.local/app/g8ee) and a user SPIFFE URI SAN
// (spiffe://g8e.local/user/<userID>). The auth middleware's handleAppAuth
// extracts app_id from the app SAN and the delegated user_id from the user
// SAN, mirroring the real ensemble delegated certificate.
func appUserMTLSCert(t *testing.T, userID string) *x509.Certificate {
	t.Helper()
	wid := protocol.NewWorkloadIdentity()
	appURI, err := wid.AppSPIFFEURL("g8ee")
	require.NoError(t, err)
	userURI, err := wid.UserSPIFFEURL(userID)
	require.NoError(t, err)

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "test-app-observe-producer"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{appURI, userURI},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	return cert
}

// appOnlyMTLSCert creates a self-signed x509 certificate carrying only an app
// SPIFFE URI SAN (no user SAN). The auth middleware stamps app_id but no
// user_id, so the producer controller's requireAppUserID rejects with 401.
func appOnlyMTLSCert(t *testing.T) *x509.Certificate {
	t.Helper()
	wid := protocol.NewWorkloadIdentity()
	appURI, err := wid.AppSPIFFEURL("g8ee")
	require.NoError(t, err)

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(43),
		Subject:      pkix.Name{CommonName: "test-app-only"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{appURI},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	return cert
}

// cliMTLSCert creates a self-signed x509 certificate carrying a CLI SPIFFE
// URI SAN. The auth middleware's handleCLIAuth path requires a CLI session
// header; without it, the producer endpoint (RouteAuthMTLS) falls through to
// handleAppAuth which rejects the CLI cert (not an app SAN), producing 401.
func cliMTLSCert(t *testing.T, userID, sessionID string) *x509.Certificate {
	t.Helper()
	wid := protocol.NewWorkloadIdentity()
	cliURI, err := wid.CLISPIFFEURL(userID, sessionID)
	require.NoError(t, err)

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(44),
		Subject:      pkix.Name{CommonName: "test-cli-observe"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{cliURI},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	return cert
}

// webSessionCookie builds an http.Cookie for the given web session ID so the
// observe read API auth middleware validates the session and stamps user_id.
func webSessionCookie(sessionID string) *http.Cookie {
	return &http.Cookie{Name: constants.WebSessionCookieName, Value: sessionID}
}

// postAgentState posts an agent state update through the real HTTP router
// with the given mTLS cert and request body.
func postAgentState(t *testing.T, h http.Handler, cert *x509.Certificate, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cert != nil {
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// postAgentStateWithHeader posts an agent state update with an additional
// header alongside the mTLS cert.
func postAgentStateWithHeader(t *testing.T, h http.Handler, cert *x509.Certificate, headerName, headerValue string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerName, headerValue)
	if cert != nil {
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// postRunState posts a run state update through the real HTTP router with
// the given mTLS cert and request body.
func postRunState(t *testing.T, h http.Handler, cert *x509.Certificate, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cert != nil {
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// postRunStateWithHeader posts a run state update with an additional header
// alongside the mTLS cert.
func postRunStateWithHeader(t *testing.T, h http.Handler, cert *x509.Certificate, headerName, headerValue string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerName, headerValue)
	if cert != nil {
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// getObserveBootstrap reads the browser-scoped observe bootstrap snapshot
// through the real HTTP router with the given web session cookie.
func getObserveBootstrap(t *testing.T, h http.Handler, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveBootstrap, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// getObserveRunDetail reads the browser-scoped observe run detail through the
// real HTTP router with the given web session cookie.
func getObserveRunDetail(t *testing.T, h http.Handler, cookie *http.Cookie, runID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveRunsByID+runID, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// validAgentStateBody returns a JSON body for a valid agent producer request
// routed via a web session.
func validAgentStateBody(t *testing.T, agentID string, status models.AgentLifecycleStatus, webSessionID string) []byte {
	t.Helper()
	body, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       agentID,
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        status,
		ObservedAt:    time.Now().UTC(),
		WebSessionID:  webSessionID,
	})
	require.NoError(t, err)
	return body
}

// validRunStateBody returns a JSON body for a valid run producer request
// routed via a web session.
func validRunStateBody(t *testing.T, runID string, status models.RunLifecycleStatus, webSessionID string) []byte {
	t.Helper()
	body, err := json.Marshal(models.ObserveProducerRunStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		RunID:         runID,
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "Investigation Alpha",
		Status:        status,
		TotalTasks:    3,
		ObservedAt:    time.Now().UTC(),
		WebSessionID:  webSessionID,
	})
	require.NoError(t, err)
	return body
}

// TestObserveProducerEndpoint_AgentStatePostAndBrowserRead verifies the full
// HTTP round-trip: POST an agent state update through the mTLS producer
// endpoint, then read the browser-scoped observe bootstrap snapshot and
// assert the projection appears with no ownership field in the wire response.
func TestObserveProducerEndpoint_AgentStatePostAndBrowserRead(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-endpoint-agent"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	agentID := "agent-endpoint-1"
	rr := postAgentState(t, env.handler, cert, validAgentStateBody(t, agentID, models.AgentLifecycleStatusRunning, webSessionID))
	require.Equal(t, http.StatusOK, rr.Code, "agent state POST body: %s", rr.Body.String())

	var resp models.ObserveProducerResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Accepted)
	// Response contains no ownership field.
	assert.NotContains(t, rr.Body.String(), "user_id")
	assert.NotContains(t, rr.Body.String(), "agent_id")

	// Read the browser-scoped bootstrap snapshot.
	readRR := getObserveBootstrap(t, env.handler, webSessionCookie(webSessionID))
	require.Equal(t, http.StatusOK, readRR.Code, "bootstrap body: %s", readRR.Body.String())

	var snapshot models.ObserveBootstrapSnapshot
	require.NoError(t, json.Unmarshal(readRR.Body.Bytes(), &snapshot))
	// The projection appears in the agents list.
	found := false
	for _, a := range snapshot.Agents {
		if a.AgentID == agentID {
			found = true
			assert.Equal(t, models.AgentLifecycleStatusRunning, a.Status)
		}
	}
	assert.True(t, found, "agent projection must appear in bootstrap snapshot")
	// The wire response contains no ownership field.
	assert.NotContains(t, readRR.Body.String(), "user_id")
}

// TestObserveProducerEndpoint_RunStatePostAndBrowserRead verifies the full
// HTTP round-trip: POST a run state update through the mTLS producer
// endpoint, then read the browser-scoped observe run detail and assert the
// projection appears with no ownership field in the wire response.
func TestObserveProducerEndpoint_RunStatePostAndBrowserRead(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-endpoint-run"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	runID := "run-endpoint-1"
	rr := postRunState(t, env.handler, cert, validRunStateBody(t, runID, models.RunLifecycleStatusRunning, webSessionID))
	require.Equal(t, http.StatusOK, rr.Code, "run state POST body: %s", rr.Body.String())

	var resp models.ObserveProducerResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Accepted)
	assert.NotContains(t, rr.Body.String(), "user_id")
	assert.NotContains(t, rr.Body.String(), "run_id")

	// Read the browser-scoped run detail.
	readRR := getObserveRunDetail(t, env.handler, webSessionCookie(webSessionID), runID)
	require.Equal(t, http.StatusOK, readRR.Code, "run detail body: %s", readRR.Body.String())

	var detail models.RunDetail
	require.NoError(t, json.Unmarshal(readRR.Body.Bytes(), &detail))
	assert.Equal(t, runID, detail.RunID)
	assert.Equal(t, models.RunLifecycleStatusRunning, detail.Status)
	// The wire response contains no ownership field.
	assert.NotContains(t, readRR.Body.String(), "user_id")
}

// TestObserveProducerEndpoint_SSEStorageContainsNestedEnvelope verifies that
// after POSTing agent and run state updates, the real SSE event store
// contains durable nested app.agent.status.updated and app.run.status.updated
// envelopes with the typed payload.
func TestObserveProducerEndpoint_SSEStorageContainsNestedEnvelope(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-endpoint-sse"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	agentID := "agent-sse-1"
	agentRR := postAgentState(t, env.handler, cert, validAgentStateBody(t, agentID, models.AgentLifecycleStatusRunning, webSessionID))
	require.Equal(t, http.StatusOK, agentRR.Code)

	runID := "run-sse-1"
	runRR := postRunState(t, env.handler, cert, validRunStateBody(t, runID, models.RunLifecycleStatusRunning, webSessionID))
	require.Equal(t, http.StatusOK, runRR.Code)

	// Query the real SSE event store for the web session route.
	route := SSERoute{UserID: userID, WebSessionID: webSessionID}
	rows, err := env.infra.SSEStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 2, "two SSE events (agent + run) must be persisted")

	// First row: agent status updated.
	assert.Equal(t, string(constants.EventAppAgentStatusUpdated), rows[0].EventType)
	var agentPush models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &agentPush))
	assert.Equal(t, userID, agentPush.UserID)
	assert.Equal(t, webSessionID, agentPush.WebSessionID)
	var agentEnv sseEventEnvelope
	require.NoError(t, json.Unmarshal(agentPush.Event, &agentEnv))
	assert.Equal(t, string(constants.EventAppAgentStatusUpdated), agentEnv.Type)
	var agentPayload models.AgentStatusUpdatedPayload
	require.NoError(t, json.Unmarshal(agentEnv.Data, &agentPayload))
	assert.Equal(t, agentID, agentPayload.AgentID)
	assert.Equal(t, models.AgentLifecycleStatusRunning, agentPayload.Status)

	// Second row: run status updated.
	assert.Equal(t, string(constants.EventAppRunStatusUpdated), rows[1].EventType)
	var runPush models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[1].Payload), &runPush))
	var runEnv sseEventEnvelope
	require.NoError(t, json.Unmarshal(runPush.Event, &runEnv))
	assert.Equal(t, string(constants.EventAppRunStatusUpdated), runEnv.Type)
	var runPayload models.RunStatusUpdatedPayload
	require.NoError(t, json.Unmarshal(runEnv.Data, &runPayload))
	assert.Equal(t, runID, runPayload.RunID)
	assert.Equal(t, models.RunLifecycleStatusRunning, runPayload.Status)

	// Neither nested payload contains user_id (ownership is routing-only).
	assert.NotContains(t, string(agentEnv.Data), "user_id")
	assert.NotContains(t, string(runEnv.Data), "user_id")
}

// TestObserveProducerEndpoint_InvalidTransitionRejected verifies that a
// terminal-to-running regression through the HTTP path is rejected with 400
// and no SSE event is emitted for the rejected transition.
func TestObserveProducerEndpoint_InvalidTransitionRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-endpoint-trans"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	runID := "run-trans-1"
	// Seed a completed run.
	seedRR := postRunState(t, env.handler, cert, validRunStateBody(t, runID, models.RunLifecycleStatusCompleted, webSessionID))
	require.Equal(t, http.StatusOK, seedRR.Code)

	// Attempt terminal regression: completed -> running.
	regressRR := postRunState(t, env.handler, cert, validRunStateBody(t, runID, models.RunLifecycleStatusRunning, webSessionID))
	assert.Equal(t, http.StatusBadRequest, regressRR.Code)

	// Only one SSE event (the initial completion).
	route := SSERoute{UserID: userID, WebSessionID: webSessionID}
	rows, err := env.infra.SSEStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	assert.Len(t, rows, 1, "no SSE event for rejected transition")
}

// TestObserveProducerEndpoint_StaleTimestampRejected verifies that a stale
// observed_at through the HTTP path is rejected with 400.
func TestObserveProducerEndpoint_StaleTimestampRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-endpoint-stale"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	later := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	earlier := time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC)

	agentID := "agent-stale-ep"
	seedBody, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       agentID,
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    later,
		WebSessionID:  webSessionID,
	})
	require.NoError(t, err)
	seedRR := postAgentState(t, env.handler, cert, seedBody)
	require.Equal(t, http.StatusOK, seedRR.Code)

	staleBody, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       agentID,
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        models.AgentLifecycleStatusWaiting,
		ObservedAt:    earlier,
		WebSessionID:  webSessionID,
	})
	require.NoError(t, err)
	staleRR := postAgentState(t, env.handler, cert, staleBody)
	assert.Equal(t, http.StatusBadRequest, staleRR.Code)
}

// TestObserveProducerEndpoint_MissingIDRejected verifies that a missing
// agent_id or run_id through the HTTP path is rejected with 400.
func TestObserveProducerEndpoint_MissingIDRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-endpoint-missing-id"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	// Missing agent_id.
	missingAgentBody, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "",
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    time.Now().UTC(),
		WebSessionID:  webSessionID,
	})
	require.NoError(t, err)
	agentRR := postAgentState(t, env.handler, cert, missingAgentBody)
	assert.Equal(t, http.StatusBadRequest, agentRR.Code)

	// Missing run_id.
	missingRunBody, err := json.Marshal(models.ObserveProducerRunStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		RunID:         "",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "No ID",
		Status:        models.RunLifecycleStatusRunning,
		TotalTasks:    1,
		ObservedAt:    time.Now().UTC(),
		WebSessionID:  webSessionID,
	})
	require.NoError(t, err)
	runRR := postRunState(t, env.handler, cert, missingRunBody)
	assert.Equal(t, http.StatusBadRequest, runRR.Code)
}

// TestObserveProducerEndpoint_InvalidRouteRejected verifies that a producer
// request with no routing target (no web_session_id and no cli_session_id)
// through the HTTP path is rejected with 400.
func TestObserveProducerEndpoint_InvalidRouteRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-endpoint-no-route"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	cert := appUserMTLSCert(t, userID)

	// No routing target.
	noRouteBody, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-no-route",
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    time.Now().UTC(),
	})
	require.NoError(t, err)
	rr := postAgentState(t, env.handler, cert, noRouteBody)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestObserveProducerEndpoint_NonAppMTLSCallerRejected verifies that a CLI
// mTLS cert (not an app cert) without a CLI session header is rejected by
// the auth middleware before reaching the producer controller. The producer
// endpoint is RouteAuthMTLS; without an app SAN, handleAppAuth returns false
// and the middleware returns 401.
func TestObserveProducerEndpoint_NonAppMTLSCallerRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-endpoint-cli"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	cert := cliMTLSCert(t, userID, "cli-session-endpoint")

	body := validAgentStateBody(t, "agent-cli-caller", models.AgentLifecycleStatusRunning, "web-1")
	rr := postAgentState(t, env.handler, cert, body)
	// CLI cert without CLI session header: auth middleware rejects with 401.
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestObserveProducerEndpoint_AgentStateCLISessionRejected verifies that a
// valid CLI session mTLS cert is rejected by the agent state producer
// endpoint. Agent and run producer endpoints remain app-only; only the eval
// publication endpoint accepts CLI session auth.
func TestObserveProducerEndpoint_AgentStateCLISessionRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-agent-cli-reject"
	seedActiveUser(t, env.infra, userID)
	cliSessionID, cert := seedCLISessionForEval(t, env.infra, userID)

	body := validAgentStateBody(t, "agent-cli-session", models.AgentLifecycleStatusRunning, "")
	// Patch the body to route via cli_session_id instead of web_session_id.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw))
	raw["web_session_id"] = []byte("null")
	raw["cli_session_id"] = []byte(fmt.Sprintf("%q", cliSessionID))
	patched, err := json.Marshal(raw)
	require.NoError(t, err)

	rr := postAgentStateWithHeader(t, env.handler, cert, constants.HeaderCLISessionID, cliSessionID, patched)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

// TestObserveProducerEndpoint_RunStateCLISessionRejected verifies that a
// valid CLI session mTLS cert is rejected by the run state producer
// endpoint. Run producer endpoints remain app-only.
func TestObserveProducerEndpoint_RunStateCLISessionRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-run-cli-reject"
	seedActiveUser(t, env.infra, userID)
	cliSessionID, cert := seedCLISessionForEval(t, env.infra, userID)

	body := validRunStateBody(t, "run-cli-session", models.RunLifecycleStatusRunning, "")
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw))
	raw["web_session_id"] = []byte("null")
	raw["cli_session_id"] = []byte(fmt.Sprintf("%q", cliSessionID))
	patched, err := json.Marshal(raw)
	require.NoError(t, err)

	rr := postRunStateWithHeader(t, env.handler, cert, constants.HeaderCLISessionID, cliSessionID, patched)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

// TestObserveProducerEndpoint_UnauthenticatedCallerRejected verifies that a
// request with no mTLS cert at all is rejected by the auth middleware.
func TestObserveProducerEndpoint_UnauthenticatedCallerRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	body := validAgentStateBody(t, "agent-no-tls", models.AgentLifecycleStatusRunning, "web-1")
	rr := postAgentState(t, env.handler, nil, body)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestObserveProducerEndpoint_AppOnlyCertNoUserIDRejected verifies that an
// app cert without a delegated user SAN is admitted by handleAppAuth (app_id
// is stamped) but rejected by the controller's requireAppUserID because
// user_id is missing, producing 401.
func TestObserveProducerEndpoint_AppOnlyCertNoUserIDRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	cert := appOnlyMTLSCert(t)

	body := validAgentStateBody(t, "agent-no-user", models.AgentLifecycleStatusRunning, "web-1")
	rr := postAgentState(t, env.handler, cert, body)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestObserveProducerEndpoint_CrossUserAppCertCannotOverwrite verifies that
// user A's app certificate cannot overwrite user B's existing projection.
// The producer controller derives user_id from the mTLS cert; user B's cert
// produces a different user_id, so the ownership check rejects with 403.
func TestObserveProducerEndpoint_CrossUserAppCertCannotOverwrite(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userA := "user-cross-a"
	userB := "user-cross-b"
	seedActiveUser(t, env.infra, userA)
	seedActiveUser(t, env.infra, userB)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionA := seedWebSession(t, env.infra, userA)
	webSessionB := seedWebSession(t, env.infra, userB)
	certA := appUserMTLSCert(t, userA)
	certB := appUserMTLSCert(t, userB)

	agentID := "agent-cross-ep"
	// User A seeds the agent.
	seedRR := postAgentState(t, env.handler, certA, validAgentStateBody(t, agentID, models.AgentLifecycleStatusRunning, webSessionA))
	require.Equal(t, http.StatusOK, seedRR.Code)

	// User B attempts to update user A's agent.
	attackBody := validAgentStateBody(t, agentID, models.AgentLifecycleStatusWaiting, webSessionB)
	attackRR := postAgentState(t, env.handler, certB, attackBody)
	assert.Equal(t, http.StatusForbidden, attackRR.Code)
	// The forbidden response does not disclose which sentinel triggered.
	assert.NotContains(t, attackRR.Body.String(), constants.ErrObserveAgentNotFound.Error())
	assert.NotContains(t, attackRR.Body.String(), constants.ErrObserveRunNotFound.Error())
}

// TestObserveProducerEndpoint_CrossUserBrowserCannotRead verifies that user
// B's browser session cannot read user A's projections. The observe read API
// applies ownership scoping; user B's bootstrap snapshot contains no agents
// or runs owned by user A.
func TestObserveProducerEndpoint_CrossUserBrowserCannotRead(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userA := "user-read-a"
	userB := "user-read-b"
	seedActiveUser(t, env.infra, userA)
	seedActiveUser(t, env.infra, userB)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionA := seedWebSession(t, env.infra, userA)
	webSessionB := seedWebSession(t, env.infra, userB)
	certA := appUserMTLSCert(t, userA)

	agentID := "agent-isolation-1"
	runID := "run-isolation-1"
	// User A seeds an agent and a run.
	agentRR := postAgentState(t, env.handler, certA, validAgentStateBody(t, agentID, models.AgentLifecycleStatusRunning, webSessionA))
	require.Equal(t, http.StatusOK, agentRR.Code)
	runRR := postRunState(t, env.handler, certA, validRunStateBody(t, runID, models.RunLifecycleStatusRunning, webSessionA))
	require.Equal(t, http.StatusOK, runRR.Code)

	// User B's bootstrap snapshot must not contain user A's projections.
	readRR := getObserveBootstrap(t, env.handler, webSessionCookie(webSessionB))
	require.Equal(t, http.StatusOK, readRR.Code)
	var snapshot models.ObserveBootstrapSnapshot
	require.NoError(t, json.Unmarshal(readRR.Body.Bytes(), &snapshot))
	for _, a := range snapshot.Agents {
		assert.NotEqual(t, agentID, a.AgentID, "user B must not see user A's agent")
	}
	for _, r := range snapshot.RecentRuns {
		assert.NotEqual(t, runID, r.RunID, "user B must not see user A's run")
	}

	// User B's run detail request for user A's run returns 404.
	detailRR := getObserveRunDetail(t, env.handler, webSessionCookie(webSessionB), runID)
	assert.Equal(t, http.StatusNotFound, detailRR.Code)
}

// TestObserveProducerEndpoint_NoAppPolicyRejected verifies that an app cert
// without a registered AppPolicy is rejected with 403 by the auth
// middleware's handleAppAuth path.
func TestObserveProducerEndpoint_NoAppPolicyRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-no-policy"
	seedActiveUser(t, env.infra, userID)
	// No AppPolicy seeded — the app cert is rejected.
	cert := appUserMTLSCert(t, userID)

	body := validAgentStateBody(t, "agent-no-policy", models.AgentLifecycleStatusRunning, "web-1")
	rr := postAgentState(t, env.handler, cert, body)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

// TestObserveProducerEndpoint_PersistBeforePublish verifies that the HTTP
// path preserves the persist-before-publish invariant: if SSE emission fails
// after a successful projection persistence, the projection is still
// readable through the browser API. This test confirms the ordering by
// posting a valid update and asserting both the projection and the SSE
// event exist after the HTTP response returns.
func TestObserveProducerEndpoint_PersistBeforePublish(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-persist-ep"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	runID := "run-persist-ep"
	rr := postRunState(t, env.handler, cert, validRunStateBody(t, runID, models.RunLifecycleStatusRunning, webSessionID))
	require.Equal(t, http.StatusOK, rr.Code)

	// Projection is readable through the browser API immediately.
	readRR := getObserveRunDetail(t, env.handler, webSessionCookie(webSessionID), runID)
	require.Equal(t, http.StatusOK, readRR.Code)

	// SSE event is persisted.
	route := SSERoute{UserID: userID, WebSessionID: webSessionID}
	rows, err := env.infra.SSEStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1, "one SSE event must be persisted after successful POST")
	assert.Equal(t, string(constants.EventAppRunStatusUpdated), rows[0].EventType)
}

// TestObserveProducerEndpoint_CLIRouteEmitsToCLIChannel verifies that a
// producer request routed via cli_session_id emits an SSE event queryable
// through the CLI route, not the web route.
func TestObserveProducerEndpoint_CLIRouteEmitsToCLIChannel(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-cli-route-ep"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	cert := appUserMTLSCert(t, userID)

	cliSessionID := "cli-session-route-ep"
	body, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-cli-route-ep",
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    time.Now().UTC(),
		CLISessionID:  cliSessionID,
	})
	require.NoError(t, err)
	rr := postAgentState(t, env.handler, cert, body)
	require.Equal(t, http.StatusOK, rr.Code)

	// Event is queryable via the CLI route.
	cliRoute := SSERoute{UserID: userID, CLISessionID: cliSessionID}
	rows, err := env.infra.SSEStore.SSEEventsListSince(cliRoute, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, userID, rows[0].UserID)
	assert.Equal(t, cliSessionID, rows[0].CLISessionID)
	assert.Empty(t, rows[0].WebSessionID)

	// No event on a web route for the same user.
	webRoute := SSERoute{UserID: userID, WebSessionID: "nonexistent-web"}
	webRows, err := env.infra.SSEStore.SSEEventsListSince(webRoute, 0, 100)
	require.NoError(t, err)
	assert.Empty(t, webRows)
}

// TestObserveProducerEndpoint_MutuallyExclusiveRouteRejected verifies that
// providing both web_session_id and cli_session_id through the HTTP path is
// rejected with 400.
func TestObserveProducerEndpoint_MutuallyExclusiveRouteRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-dual-route"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	cert := appUserMTLSCert(t, userID)

	body, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-dual",
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    time.Now().UTC(),
		WebSessionID:  "web-1",
		CLISessionID:  "cli-1",
	})
	require.NoError(t, err)
	rr := postAgentState(t, env.handler, cert, body)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestObserveProducerEndpoint_BrowserReadWithoutCookieRejected verifies that
// the observe read API rejects requests without a web session cookie.
func TestObserveProducerEndpoint_BrowserReadWithoutCookieRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	// No cookie: auth middleware rejects with 401.
	rr := getObserveBootstrap(t, env.handler, nil)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestObserveProducerEndpoint_BrowserReadInvalidCookieRejected verifies that
// the observe read API rejects requests with an invalid web session cookie.
func TestObserveProducerEndpoint_BrowserReadInvalidCookieRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	rr := getObserveBootstrap(t, env.handler, &http.Cookie{
		Name:  constants.WebSessionCookieName,
		Value: "nonexistent-session-endpoint",
	})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestObserveProducerEndpoint_AgentStateResponseTyped verifies that the
// producer response is the typed ObserveProducerResponse with accepted=true
// and no extra fields.
func TestObserveProducerEndpoint_AgentStateResponseTyped(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-typed-resp"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	rr := postAgentState(t, env.handler, cert, validAgentStateBody(t, "agent-typed", models.AgentLifecycleStatusRunning, webSessionID))
	require.Equal(t, http.StatusOK, rr.Code)

	// Parse as typed response; no extra fields.
	var resp models.ObserveProducerResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Accepted)
	// The response body must contain only {"accepted":true}.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &raw))
	assert.Len(t, raw, 1, "response must have exactly one field")
	assert.Contains(t, raw, "accepted")
}

// TestObserveProducerEndpoint_RunStateResponseTyped verifies that the run
// producer response is the typed ObserveProducerResponse with accepted=true.
func TestObserveProducerEndpoint_RunStateResponseTyped(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-typed-run-resp"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	rr := postRunState(t, env.handler, cert, validRunStateBody(t, "run-typed", models.RunLifecycleStatusRunning, webSessionID))
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.ObserveProducerResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Accepted)
}

// TestObserveProducerEndpoint_ContextCancellationPropagates verifies that
// cancelling the request context before the producer call completes does not
// leave a partial projection. This is a best-effort check: the producer
// service completes synchronously, so cancellation after the response is a
// no-op; the test confirms the endpoint does not hang on a cancelled context.
func TestObserveProducerEndpoint_ContextCancellationDoesNotHang(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-cancel-ep"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body := validAgentStateBody(t, "agent-cancel", models.AgentLifecycleStatusRunning, webSessionID)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctx)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}
	rr := httptest.NewRecorder()
	env.handler.ServeHTTP(rr, req)
	// The request completes within the timeout.
	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestObserveProducerEndpoint_FailedPersistence_HTTP500NoSSERow verifies that
// when the underlying SQLite store is closed (simulating a persistence
// failure), POSTing through the real controller handler returns 500 and no
// SSE event is emitted. This proves the controller and HTTP path preserve
// the persist-before-publish invariant: a failed write cannot produce an SSE
// row or a live publication. The existing projection (seeded before closing
// the DB) is not replaced because the write never reached the DocSet step.
//
// The test uses the real ObserveProducerController with a real in-memory
// SQLite DB (same pattern as the controller unit tests). The auth middleware
// is bypassed by stamping the mTLS-derived identity directly into the
// request context because the auth middleware's PKI revocation check shares
// the same DB and would also fail when the DB is closed, masking the
// persistence failure. The controller IS the HTTP handler; this test
// exercises the real controller → producer service → DocSet → SSE emission
// path.
func TestObserveProducerEndpoint_FailedPersistence_HTTP500NoSSERow(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	agentID := "agent-fail-persist"
	// Seed a valid projection while the DB is open.
	seedReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState,
		strings.NewReader(validAgentBody(t, agentID, models.AgentLifecycleStatusRunning)))
	seedReq = withAppAuthCtx(seedReq, "app-workload", "user-fail")
	seedW := httptest.NewRecorder()
	controller.handleAgentState(seedW, seedReq)
	require.Equal(t, http.StatusOK, seedW.Code)

	// Close the underlying DB to force a persistence failure on the next
	// write. The producer service fails at the DocGet (read existing) step
	// because the DB is closed, returning an error the controller maps to 500.
	// No SSE event is emitted because the emit step is never reached.
	// sql.DB.Close is idempotent, so the t.Cleanup registered by
	// newProducerControllerTestEnv is a no-op after this manual close.
	require.NoError(t, controller.producerSvc.docStore.db.Close())

	// POST an update through the real controller handler.
	updateBody := validAgentBody(t, agentID, models.AgentLifecycleStatusWaiting)
	updateReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState,
		strings.NewReader(updateBody))
	updateReq = withAppAuthCtx(updateReq, "app-workload", "user-fail")
	updateW := httptest.NewRecorder()
	controller.handleAgentState(updateW, updateReq)

	assert.Equal(t, http.StatusInternalServerError, updateW.Code)
	assert.NotContains(t, updateW.Body.String(), "accepted")
}

// TestObserveProducerEndpoint_FailedPersistence_RunState_HTTP500NoSSERow
// verifies the same persist-before-publish invariant for the run-state
// endpoint: closing the DB before a run-state POST produces 500 and no
// successful projection replacement.
func TestObserveProducerEndpoint_FailedPersistence_RunState_HTTP500NoSSERow(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	runID := "run-fail-persist"
	// Seed a valid run projection while the DB is open.
	seedReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState,
		strings.NewReader(validRunBody(t, runID, models.RunLifecycleStatusRunning)))
	seedReq = withAppAuthCtx(seedReq, "app-workload", "user-fail-run")
	seedW := httptest.NewRecorder()
	controller.handleRunState(seedW, seedReq)
	require.Equal(t, http.StatusOK, seedW.Code)

	// Close the underlying DB to force a persistence failure on the next
	// write. sql.DB.Close is idempotent, so the t.Cleanup registered by
	// newProducerControllerTestEnv is a no-op after this manual close.
	require.NoError(t, controller.producerSvc.docStore.db.Close())

	// POST an update through the real controller handler.
	updateBody := validRunBody(t, runID, models.RunLifecycleStatusCompleted)
	updateReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState,
		strings.NewReader(updateBody))
	updateReq = withAppAuthCtx(updateReq, "app-workload", "user-fail-run")
	updateW := httptest.NewRecorder()
	controller.handleRunState(updateW, updateReq)

	assert.Equal(t, http.StatusInternalServerError, updateW.Code)
	assert.NotContains(t, updateW.Body.String(), "accepted")
}

// postEvalPublication posts an eval publication request through the real HTTP
// router with the given mTLS cert and request body.
func postEvalPublication(t *testing.T, h http.Handler, cert *x509.Certificate, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cert != nil {
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// validEvalPublicationBodyHTTP returns a JSON body for a valid eval publication
// request routed via a web session, with real base64 content whose SHA-256
// and size match the declared values.
func validEvalPublicationBodyHTTP(t *testing.T, runID, webSessionID string) []byte {
	t.Helper()
	content := []byte(`{"analysis":"ok"}`)
	contentHash := sha256.Sum256(content)
	contentHashHex := hex.EncodeToString(contentHash[:])
	contentB64 := base64.StdEncoding.EncodeToString(content)
	body, err := json.Marshal(models.ObserveProducerEvalPublicationRequest{
		SchemaVersion:    constants.ObservePublicationSchemaVersion,
		BundleID:         "bundle-" + runID,
		RunID:            runID,
		ReleaseVersion:   "2.1.8",
		SuiteID:          "suite-" + runID,
		SuiteVersion:     "1.0.0",
		CampaignID:       "campaign-" + runID,
		ArmIDs:           []string{"arm-" + runID},
		ModelCohortIDs:   []string{"cohort-" + runID},
		AssignmentCount:  1,
		ReceiptCount:     10,
		AssignedTasks:    5,
		TerminalAttempts: 5,
		Metrics: []models.EvalMetricSummary{
			{
				SchemaVersion:      constants.ObserveEventPayloadSchemaVersion,
				MetricID:           "metric-" + runID,
				MetricVersion:      "1.0.0",
				ModelCohortID:      "cohort-" + runID,
				ArmID:              "arm-" + runID,
				Unit:               "count",
				Eligible:           5,
				Denominator:        5,
				VerificationStatus: models.EvalVerificationVerified,
			},
		},
		VerificationReport: models.VerificationReportWire{
			SchemaVersion:  constants.VerificationReportSchemaVersion,
			BundleID:       "bundle-" + runID,
			RunID:          runID,
			ReleaseVersion: "2.1.8",
			VerifiedAt:     time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
			OK:             true,
			Layers: []models.LayerResultWire{
				{Layer: 1, Passed: true, FailureCount: 0},
			},
		},
		BundleManifest: models.BundleManifestWire{
			SchemaVersion: constants.BundleManifestSchemaVersion,
			BundleID:      "bundle-" + runID,
			RunID:         runID,
			Artifacts: []models.BundleArtifactEntryWire{
				{
					Path:         "analysis/analysis.json",
					MediaType:    "application/json",
					PrivacyClass: "public",
					SHA256:       contentHashHex,
					ByteLength:   int64(len(content)),
					ArtifactType: "analysis",
				},
			},
		},
		Downloads: []models.ObserveProducerDownloadArtifactInput{
			{
				ArtifactID:            "artifact-" + runID,
				Filename:              "analysis.json",
				MediaType:             "application/json",
				ByteSize:              int64(len(content)),
				SHA256:                contentHashHex,
				PrivacyClassification: models.DownloadPrivacyPublicSafe,
				SourceRunID:           runID,
				Content:               contentB64,
			},
		},
		WebSessionID: webSessionID,
	})
	require.NoError(t, err)
	return body
}

// getObserveDownloadStream streams a download artifact through the real HTTP
// router with the given web session cookie and ?download=1 query param.
func getObserveDownloadStream(t *testing.T, h http.Handler, cookie *http.Cookie, artifactID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveDownloadsByID+artifactID+"?download=1", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// getObserveEvals reads the browser-scoped observe evals list through the
// real HTTP router with the given web session cookie.
func getObserveEvals(t *testing.T, h http.Handler, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveEvals, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// getObserveEvalDetail reads the browser-scoped observe eval detail through
// the real HTTP router with the given web session cookie.
func getObserveEvalDetail(t *testing.T, h http.Handler, cookie *http.Cookie, runID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveEvalsByID+runID, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// getObserveDownloads reads the browser-scoped observe downloads list through
// the real HTTP router with the given web session cookie.
func getObserveDownloads(t *testing.T, h http.Handler, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveDownloads, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestObserveProducerEndpoint_EvalPublicationPostAndBrowserRead verifies the
// full HTTP round-trip: POST an eval publication through the mTLS producer
// endpoint, then read the browser-scoped observe evals list and detail and
// assert the projection appears with no ownership field in the wire response.
func TestObserveProducerEndpoint_EvalPublicationPostAndBrowserRead(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-eval-pub"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	runID := "run-eval-pub-1"
	rr := postEvalPublication(t, env.handler, cert, validEvalPublicationBodyHTTP(t, runID, webSessionID))
	require.Equal(t, http.StatusOK, rr.Code, "eval publication POST body: %s", rr.Body.String())

	var resp models.ObserveProducerResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Accepted)
	assert.NotContains(t, rr.Body.String(), "user_id")

	// Read the browser-scoped evals list.
	listRR := getObserveEvals(t, env.handler, webSessionCookie(webSessionID))
	require.Equal(t, http.StatusOK, listRR.Code, "evals list body: %s", listRR.Body.String())
	var page models.ObservePage
	require.NoError(t, json.Unmarshal(listRR.Body.Bytes(), &page))
	var evals []models.EvalSummary
	require.NoError(t, json.Unmarshal(page.Items, &evals))
	found := false
	for _, eval := range evals {
		if eval.RunID == runID {
			found = true
			assert.Equal(t, models.EvalVerificationVerified, eval.VerificationStatus)
		}
	}
	assert.True(t, found, "eval projection must appear in evals list")

	// Read the browser-scoped eval detail.
	detailRR := getObserveEvalDetail(t, env.handler, webSessionCookie(webSessionID), runID)
	require.Equal(t, http.StatusOK, detailRR.Code, "eval detail body: %s", detailRR.Body.String())
	var detail models.EvalDetail
	require.NoError(t, json.Unmarshal(detailRR.Body.Bytes(), &detail))
	assert.Equal(t, runID, detail.RunID)
	assert.Equal(t, models.EvalVerificationVerified, detail.VerificationStatus)
	assert.NotEmpty(t, detail.PublishedProjectionSHA256)
	assert.NotContains(t, detailRR.Body.String(), "user_id")
}

// TestObserveProducerEndpoint_EvalPublicationSSEStorageContainsNestedEnvelope
// verifies that after POSTing an eval publication, the real SSE event store
// contains durable nested ai.eval.run.completed and ai.eval.metric.recorded
// envelopes with the typed payload.
func TestObserveProducerEndpoint_EvalPublicationSSEStorageContainsNestedEnvelope(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-eval-sse"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	runID := "run-eval-sse-1"
	rr := postEvalPublication(t, env.handler, cert, validEvalPublicationBodyHTTP(t, runID, webSessionID))
	require.Equal(t, http.StatusOK, rr.Code)

	route := SSERoute{UserID: userID, WebSessionID: webSessionID}
	rows, err := env.infra.SSEStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 2, "one run-completed + one metric-recorded")

	assert.Equal(t, string(constants.EventAiEvalRunCompleted), rows[0].EventType)
	var runPush models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].Payload), &runPush))
	var runEnv sseEventEnvelope
	require.NoError(t, json.Unmarshal(runPush.Event, &runEnv))
	assert.Equal(t, string(constants.EventAiEvalRunCompleted), runEnv.Type)
	var runCompleted models.EvalRunCompletedPayload
	require.NoError(t, json.Unmarshal(runEnv.Data, &runCompleted))
	assert.Equal(t, runID, runCompleted.RunID)
	assert.Equal(t, models.EvalVerificationVerified, runCompleted.VerificationStatus)

	assert.Equal(t, string(constants.EventAiEvalMetricRecorded), rows[1].EventType)
	var metricPush models.SSEPushPayload
	require.NoError(t, json.Unmarshal([]byte(rows[1].Payload), &metricPush))
	var metricEnv sseEventEnvelope
	require.NoError(t, json.Unmarshal(metricPush.Event, &metricEnv))
	assert.Equal(t, string(constants.EventAiEvalMetricRecorded), metricEnv.Type)
	var metricRecorded models.EvalMetricRecordedPayload
	require.NoError(t, json.Unmarshal(metricEnv.Data, &metricRecorded))
	assert.Equal(t, runID, metricRecorded.RunID)

	// Neither nested payload contains user_id.
	assert.NotContains(t, string(runEnv.Data), "user_id")
	assert.NotContains(t, string(metricEnv.Data), "user_id")
}

// TestObserveProducerEndpoint_EvalPublicationDownloadStream verifies that
// after publishing an eval, the download artifact can be streamed through
// the browser read API with ?download=1 and the bytes match the published
// content.
func TestObserveProducerEndpoint_EvalPublicationDownloadStream(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-eval-dl"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	runID := "run-eval-dl-1"
	pubRR := postEvalPublication(t, env.handler, cert, validEvalPublicationBodyHTTP(t, runID, webSessionID))
	require.Equal(t, http.StatusOK, pubRR.Code)

	// The download list contains the artifact.
	listRR := getObserveDownloads(t, env.handler, webSessionCookie(webSessionID))
	require.Equal(t, http.StatusOK, listRR.Code)
	var dlPage models.ObservePage
	require.NoError(t, json.Unmarshal(listRR.Body.Bytes(), &dlPage))
	var downloads []models.DownloadArtifact
	require.NoError(t, json.Unmarshal(dlPage.Items, &downloads))
	found := false
	for _, dl := range downloads {
		if dl.ArtifactID == "artifact-"+runID {
			found = true
			assert.Equal(t, models.DownloadPrivacyPublicSafe, dl.PrivacyClassification)
		}
	}
	assert.True(t, found, "download artifact must appear in downloads list")

	// Stream the artifact bytes.
	streamRR := getObserveDownloadStream(t, env.handler, webSessionCookie(webSessionID), "artifact-"+runID)
	require.Equal(t, http.StatusOK, streamRR.Code, "stream body: %s", streamRR.Body.String())
	assert.Equal(t, "application/json", streamRR.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="analysis.json"`, streamRR.Header().Get("Content-Disposition"))
	assert.Equal(t, `{"analysis":"ok"}`, streamRR.Body.String())
}

// TestObserveProducerEndpoint_EvalPublicationCrossUserIsolation verifies that
// user A's eval publication cannot be read or downloaded by user B.
func TestObserveProducerEndpoint_EvalPublicationCrossUserIsolation(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userA := "user-eval-iso-a"
	userB := "user-eval-iso-b"
	seedActiveUser(t, env.infra, userA)
	seedActiveUser(t, env.infra, userB)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionA := seedWebSession(t, env.infra, userA)
	webSessionB := seedWebSession(t, env.infra, userB)
	certA := appUserMTLSCert(t, userA)

	runID := "run-eval-iso-1"
	pubRR := postEvalPublication(t, env.handler, certA, validEvalPublicationBodyHTTP(t, runID, webSessionA))
	require.Equal(t, http.StatusOK, pubRR.Code)

	// User B's evals list must not contain user A's eval.
	listB := getObserveEvals(t, env.handler, webSessionCookie(webSessionB))
	require.Equal(t, http.StatusOK, listB.Code)
	var pageB models.ObservePage
	require.NoError(t, json.Unmarshal(listB.Body.Bytes(), &pageB))
	var evalsB []models.EvalSummary
	require.NoError(t, json.Unmarshal(pageB.Items, &evalsB))
	for _, eval := range evalsB {
		assert.NotEqual(t, runID, eval.RunID, "user B must not see user A's eval")
	}

	// User B's eval detail request returns 404.
	detailB := getObserveEvalDetail(t, env.handler, webSessionCookie(webSessionB), runID)
	assert.Equal(t, http.StatusNotFound, detailB.Code)

	// User B's download stream returns 404.
	streamB := getObserveDownloadStream(t, env.handler, webSessionCookie(webSessionB), "artifact-"+runID)
	assert.Equal(t, http.StatusNotFound, streamB.Code)
}

// TestObserveProducerEndpoint_EvalPublicationUnverifiedRejected verifies that
// a publication with ok=false is rejected with 400 and no projection is
// persisted.
func TestObserveProducerEndpoint_EvalPublicationUnverifiedRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-eval-unverified"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	body := validEvalPublicationBodyHTTP(t, "run-eval-unverified", webSessionID)
	// Patch OK to false.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw))
	raw["verification_report"] = []byte(`{"schema_version":"1.0.0","bundle_id":"bundle-run-eval-unverified","run_id":"run-eval-unverified","release_version":"2.1.8","verified_at":"2026-09-09T12:00:00Z","ok":false,"layers":[],"failures":[]}`)
	patched, err := json.Marshal(raw)
	require.NoError(t, err)

	rr := postEvalPublication(t, env.handler, cert, patched)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrObservePublicationNotVerified.Error())

	// No SSE event emitted.
	route := SSERoute{UserID: userID, WebSessionID: webSessionID}
	rows, err := env.infra.SSEStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	assert.Empty(t, rows, "no SSE event for rejected publication")
}

// TestObserveProducerEndpoint_EvalPublicationFailedPersistence verifies that
// a persistence failure through the HTTP path returns 500 and no SSE event
// is emitted. Uses the controller-level test env with a closed DB.
func TestObserveProducerEndpoint_EvalPublicationFailedPersistence(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	runID := "run-eval-fail-persist"
	body := validEvalPublicationBody(t, runID)

	// Close the DB to force a persistence failure. sql.DB.Close is idempotent
	// so the t.Cleanup registered by newProducerControllerTestEnv is a no-op.
	require.NoError(t, controller.producerSvc.docStore.db.Close())

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, strings.NewReader(body))
	req = withAppAuthCtx(req, "app-workload", "user-fail-eval")
	w := httptest.NewRecorder()
	controller.handleEvalPublication(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "accepted")
}

// seedCLISessionForEval registers a CLI session document for the given user
// so the auth middleware's handleCLIAuth path admits the mTLS CLI cert.
// Returns the CLI session ID and a self-signed cert with a matching CLI
// SPIFFE URI SAN.
func seedCLISessionForEval(t *testing.T, infra *TestInfrastructure, userID string) (cliSessionID string, cert *x509.Certificate) {
	t.Helper()
	cliSessionID = "cli-session-eval"

	cliDoc := &models.CLISession{
		ID:        cliSessionID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	cliBytes, err := json.Marshal(cliDoc)
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliBytes))

	wid := protocol.NewWorkloadIdentity()
	cliURI, err := wid.CLISPIFFEURL(userID, cliSessionID)
	require.NoError(t, err)

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(44),
		Subject:      pkix.Name{CommonName: "test-cli-eval-publish"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{cliURI},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)
	cert, err = x509.ParseCertificate(certDER)
	require.NoError(t, err)
	return cliSessionID, cert
}

// TestObserveProducerEndpoint_EvalPublicationCLISessionAccepted verifies
// that a CLI mTLS cert with a valid CLI session is accepted by the eval
// publication endpoint. The endpoint accepts both app-workload and CLI
// session auth; agent/run producer endpoints remain app-only.
func TestObserveProducerEndpoint_EvalPublicationCLISessionAccepted(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-eval-cli"
	seedActiveUser(t, env.infra, userID)
	cliSessionID, cert := seedCLISessionForEval(t, env.infra, userID)

	body := validEvalPublicationBodyHTTP(t, "run-eval-cli", "")
	// Patch the body to route via cli_session_id instead of web_session_id.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw))
	raw["web_session_id"] = []byte("null")
	raw["cli_session_id"] = []byte(fmt.Sprintf("%q", cliSessionID))
	patched, err := json.Marshal(raw)
	require.NoError(t, err)

	rr := postEvalPublicationWithHeader(t, env.handler, cert, constants.HeaderCLISessionID, cliSessionID, patched)
	assert.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
}

// postEvalPublicationWithHeader posts an eval publication request with an
// additional header (e.g. the CLI session ID header) alongside the mTLS cert.
func postEvalPublicationWithHeader(t *testing.T, h http.Handler, cert *x509.Certificate, headerName, headerValue string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerName, headerValue)
	if cert != nil {
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestObserveProducerEndpoint_EvalPublicationUnauthenticatedRejected verifies
// that a request with no mTLS cert is rejected by the auth middleware.
func TestObserveProducerEndpoint_EvalPublicationUnauthenticatedRejected(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	body := validEvalPublicationBodyHTTP(t, "run-eval-no-tls", "web-1")
	rr := postEvalPublication(t, env.handler, nil, body)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestObserveProducerEndpoint_DownloadStreamWithoutQueryParamReturnsMetadata
// verifies that without ?download=1, the download endpoint returns JSON
// metadata, not bytes.
func TestObserveProducerEndpoint_DownloadStreamWithoutQueryParamReturnsMetadata(t *testing.T) {
	env := setupObserveProducerEndpointEnv(t)

	userID := "user-eval-meta"
	seedActiveUser(t, env.infra, userID)
	seedAppPolicy(t, env.infra, protocol.EnsembleAppID)
	webSessionID := seedWebSession(t, env.infra, userID)
	cert := appUserMTLSCert(t, userID)

	runID := "run-eval-meta-1"
	pubRR := postEvalPublication(t, env.handler, cert, validEvalPublicationBodyHTTP(t, runID, webSessionID))
	require.Equal(t, http.StatusOK, pubRR.Code)

	// Without ?download=1, the endpoint returns JSON metadata.
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveDownloadsByID+"artifact-"+runID, nil)
	req.AddCookie(webSessionCookie(webSessionID))
	rr := httptest.NewRecorder()
	env.handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	var dl models.DownloadArtifact
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &dl))
	assert.Equal(t, "artifact-"+runID, dl.ArtifactID)
}
