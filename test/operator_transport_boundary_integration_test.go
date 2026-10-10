// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package tests

// Operator transport boundary contracts. An Operator reaches the Gateway over
// websockets only: one bootstrap socket delivers its identity once, its
// identity lives in process memory only, and every later exchange rides the
// mTLS pub/sub socket, which the Operator redials with that in-memory identity
// whenever the network drops it. A stopped Operator has no identity and must
// enroll again. Each test drives a real Gateway, the real enrollment service,
// and a real Operator runtime through an OperatorWireWitness that records the
// wire and models network loss.

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
	"github.com/g8e-ai/g8e/v2/test/fixtures"
)

// boundaryTimeout bounds each contract: enrollment, start, and the waits that
// follow on a local Gateway.
const boundaryTimeout = 90 * time.Second

// boundaryRig is a real Gateway, its owner, and the network Operators reach
// it through.
type boundaryRig struct {
	gateway *fixtures.GatewayFixture
	owner   *fixtures.OwnerConsole
	witness *fixtures.OperatorWireWitness
}

// newBoundaryRig runs the Gateway in the doctrine posture the scale harness
// uses: the transport boundary holds in every posture, and an owner decision
// here needs no WebAuthn ceremony.
func newBoundaryRig(t *testing.T, policy fixtures.OwnerPolicy) *boundaryRig {
	t.Helper()
	f := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          t.Name(),
		Posture:           config.PostureDoctrine,
		AllowTestPortZero: true,
	})
	f.WaitForReady(t)
	return &boundaryRig{
		gateway: f,
		owner:   fixtures.NewOwnerConsole(t, f, policy),
		witness: fixtures.NewOperatorWireWitness(t, f),
	}
}

// startOperator enrolls and starts an Operator on a fresh host and waits for
// the Gateway to receive its first heartbeat.
func (r *boundaryRig) startOperator(t *testing.T, ctx context.Context) (*fixtures.OperatorHost, *fixtures.OperatorProcess) {
	t.Helper()
	host := fixtures.NewOperatorHost(t)
	op := r.witness.LaunchOperator(t, host, fixtures.OperatorProcessOptions{})
	require.NoError(t, op.AwaitStarted(ctx))
	_, err := op.AwaitHeartbeat(ctx)
	require.NoError(t, err)
	return host, op
}

// operatorStatus reads an Operator's status the way every Gateway reader does.
func (r *boundaryRig) operatorStatus(t *testing.T, ctx context.Context, operatorID string) constants.OperatorStatus {
	t.Helper()
	doc, err := r.gateway.Service.GetDocStore().DocGet(ctx, marshaler.CollectionName(constants.CollectionOperators), operatorID)
	require.NoError(t, err)
	require.NotNil(t, doc, "operator %s has no document", operatorID)
	operatorDoc, err := models.OperatorDocumentFromStore(doc)
	require.NoError(t, err)
	return constants.OperatorStatus(operatorDoc.GetStatus())
}

// pubsubRedial matches the mTLS pub/sub handshake of the Operator holding serial.
func pubsubRedial(serial string) func(fixtures.WireExchange) bool {
	return func(e fixtures.WireExchange) bool {
		return e.Upgrade && e.Listener == fixtures.WireListenerTLS && e.ClientCertSerial == serial
	}
}

// httpRequestsAfter returns the HTTP requests recorded at or after mark.
func httpRequestsAfter(exchanges []fixtures.WireExchange, mark int) []fixtures.WireExchange {
	var requests []fixtures.WireExchange
	for i := mark; i < len(exchanges); i++ {
		if !exchanges[i].Upgrade {
			requests = append(requests, exchanges[i])
		}
	}
	return requests
}

// T1: an Operator enrolling and starting against a real Gateway makes zero
// HTTP requests. Its only exchanges are websocket handshakes.
func TestOperatorTransportBoundary_EnrollAndStartMakeNoHTTPRequests(t *testing.T) {
	rig := newBoundaryRig(t, fixtures.OwnerApprovesEach)
	ctx, cancel := context.WithTimeout(t.Context(), boundaryTimeout)
	defer cancel()

	_, op := rig.startOperator(t, ctx)
	leaf, err := op.Leaf()
	require.NoError(t, err)

	assert.Empty(t, rig.witness.HTTPRequests(), "the Operator made HTTP requests; it may only open websockets")
	_, err = rig.witness.AwaitExchangeAfter(ctx, 0, pubsubRedial(leaf.SerialNumber.String()))
	assert.NoError(t, err, "the Operator never opened its mTLS pub/sub socket with its issued certificate")
}

// T2: after enrollment and start, the Operator's host holds no identity
// material: no certificate, key, or trust bundle, and no enrollment state.
func TestOperatorTransportBoundary_IdentityNeverTouchesDisk(t *testing.T) {
	rig := newBoundaryRig(t, fixtures.OwnerApprovesEach)
	ctx, cancel := context.WithTimeout(t.Context(), boundaryTimeout)
	defer cancel()

	host, op := rig.startOperator(t, ctx)
	leaf, err := op.Leaf()
	require.NoError(t, err)

	pemMarker := []byte("-----BEGIN ")
	forbiddenDirs := []string{
		filepath.Join(constants.PkiDirname, constants.PkiSubdirPendingEnroll),
		filepath.Join(constants.PkiDirname, constants.PkiSubdirTrustedSigners),
	}
	var violations []string
	err = filepath.WalkDir(host.WorkingDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(host.WorkingDir, path)
		if err != nil {
			return err
		}
		for _, dir := range forbiddenDirs {
			if strings.Contains(filepath.ToSlash(rel), filepath.ToSlash(dir)+"/") {
				violations = append(violations, rel+": enrollment state")
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, pemMarker) {
			violations = append(violations, rel+": PEM block")
		}
		if bytes.Contains(data, leaf.Raw) {
			violations = append(violations, rel+": issued certificate")
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, violations, "identity material written under the Operator runtime root")
}

// T4: the connection that delivers the issued identity drops after the
// Gateway committed it. The same running Operator recovers that identity with
// no new request and no new approval.
func TestOperatorTransportBoundary_IdentityLostInTransitIsRecovered(t *testing.T) {
	rig := newBoundaryRig(t, fixtures.OwnerApprovesEach)
	ctx, cancel := context.WithTimeout(t.Context(), boundaryTimeout)
	defer cancel()

	rig.witness.ArmIdentityCut()
	op := rig.witness.LaunchOperator(t, fixtures.NewOperatorHost(t), fixtures.OperatorProcessOptions{})
	require.NoError(t, rig.witness.AwaitIdentityCut(ctx))

	require.NoError(t, op.AwaitStarted(ctx), "the Operator must recover the identity the Gateway committed")
	_, err := op.AwaitHeartbeat(ctx)
	require.NoError(t, err)

	requests := rig.owner.EnrollmentRequests()
	require.Len(t, requests, 1, "recovery must not create a second enrollment request")
	committed, err := rig.owner.CommittedOperatorCertificate(ctx, requests[0])
	require.NoError(t, err)
	leaf, err := op.Leaf()
	require.NoError(t, err)
	assert.Equal(t, committed.SerialNumber.String(), leaf.SerialNumber.String(), "the Operator must hold the identity the Gateway committed")
}

// T5: the network drops the pub/sub socket while the Operator runs. The
// Operator redials with its in-memory identity, heartbeats resume, and the
// Gateway reads it as active. A sever and a partition are each survived.
func TestOperatorTransportBoundary_PubSubRedialsWithInMemoryIdentity(t *testing.T) {
	rig := newBoundaryRig(t, fixtures.OwnerApprovesEach)
	ctx, cancel := context.WithTimeout(t.Context(), boundaryTimeout)
	defer cancel()

	_, op := rig.startOperator(t, ctx)
	leaf, err := op.Leaf()
	require.NoError(t, err)
	workload, err := op.Workload()
	require.NoError(t, err)
	serial := leaf.SerialNumber.String()
	started := rig.witness.Mark()

	t.Run("sever", func(t *testing.T) {
		mark := rig.witness.Mark()
		require.Positive(t, rig.witness.Sever())
		_, err := rig.witness.AwaitExchangeAfter(ctx, mark, pubsubRedial(serial))
		require.NoError(t, err)
		_, err = op.AwaitHeartbeatAfter(ctx, op.HeartbeatMark())
		require.NoError(t, err)
	})

	t.Run("partition", func(t *testing.T) {
		mark := rig.witness.Mark()
		rig.witness.Partition()
		require.NoError(t, rig.witness.AwaitRefusedDials(ctx, 2), "the Operator must keep redialing through a partition")
		rig.witness.Heal()
		_, err := rig.witness.AwaitExchangeAfter(ctx, mark, pubsubRedial(serial))
		require.NoError(t, err)
		_, err = op.AwaitHeartbeatRecordedAfter(ctx, op.HeartbeatMark())
		require.NoError(t, err)
	})

	assert.Equal(t, constants.OperatorStatusActive, rig.operatorStatus(t, ctx, workload.OperatorID))
	assert.Empty(t, httpRequestsAfter(rig.witness.Exchanges(), started), "reconnecting must not make HTTP requests")
	assert.Len(t, rig.owner.EnrollmentRequests(), 1, "reconnecting must not re-enroll")
}

// T6: a stopped Operator's identity is gone with its process. A fresh process
// on the same host must enroll again and receives a new identity.
func TestOperatorTransportBoundary_RestartRequiresReenrollment(t *testing.T) {
	rig := newBoundaryRig(t, fixtures.OwnerApprovesEach)
	ctx, cancel := context.WithTimeout(t.Context(), boundaryTimeout)
	defer cancel()

	host, first := rig.startOperator(t, ctx)
	firstLeaf, err := first.Leaf()
	require.NoError(t, err)
	firstWorkload, err := first.Workload()
	require.NoError(t, err)
	require.NoError(t, first.Stop(ctx))

	second := rig.witness.LaunchOperator(t, host, fixtures.OperatorProcessOptions{})
	require.NoError(t, second.AwaitStarted(ctx))
	_, err = second.AwaitHeartbeat(ctx)
	require.NoError(t, err)
	secondLeaf, err := second.Leaf()
	require.NoError(t, err)
	secondWorkload, err := second.Workload()
	require.NoError(t, err)

	assert.Len(t, rig.owner.EnrollmentRequests(), 2, "the restarted Operator must create its own enrollment request")
	assert.NotEqual(t, firstLeaf.SerialNumber.String(), secondLeaf.SerialNumber.String(), "the restarted Operator must hold a newly issued certificate")
	assert.NotEqual(t, firstWorkload.OperatorSessionID, secondWorkload.OperatorSessionID, "the restarted Operator must hold a new session")
}

// T7: staleness is derived when it is read. Silence past the window reads as
// stale without a stored transition or a status event; one heartbeat after
// the network returns reads as active.
func TestOperatorTransportBoundary_StalenessIsDerivedAtRead(t *testing.T) {
	rig := newBoundaryRig(t, fixtures.OwnerApprovesEach)
	ctx, cancel := context.WithTimeout(t.Context(), boundaryTimeout)
	defer cancel()

	_, op := rig.startOperator(t, ctx)
	workload, err := op.Workload()
	require.NoError(t, err)

	rig.witness.Partition()
	require.NoError(t, rig.witness.AwaitRefusedDials(ctx, 1))

	// The last heartbeat recedes past any stale window without waiting it out.
	kv := rig.gateway.Service.GetKVStore()
	key := constants.OperatorHeartbeatKeyPrefix + workload.OperatorID
	raw, ok := kv.KVGet(ctx, key)
	require.True(t, ok, "the Gateway recorded no heartbeat for %s", workload.OperatorID)
	var telemetry map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &telemetry))
	silentSince, err := json.Marshal(time.Now().UTC().Add(-10 * time.Minute))
	require.NoError(t, err)
	telemetry["last_heartbeat_at"] = silentSince
	backdated, err := json.Marshal(telemetry)
	require.NoError(t, err)
	require.NoError(t, kv.KVSetObserved(ctx, key, string(backdated), 0))

	mark := rig.owner.Mark()
	assert.Equal(t, constants.OperatorStatusStale, rig.operatorStatus(t, ctx, workload.OperatorID), "silence past the window must read as stale")
	for _, event := range rig.owner.EventsAfter(mark) {
		assert.False(t, fixtures.IsOperatorStatusEvent(event, workload.OperatorID, constants.EventOperatorStatusUpdatedStale),
			"reading staleness must not store a stale transition")
	}

	rig.witness.Heal()
	_, err = op.AwaitHeartbeatRecordedAfter(ctx, op.HeartbeatMark())
	require.NoError(t, err)
	assert.Equal(t, constants.OperatorStatusActive, rig.operatorStatus(t, ctx, workload.OperatorID), "one recorded heartbeat must read as active")
}

// T8: the HTTP enrollment route admits no Operator. An application request on
// the same route is the control: it is still accepted.
func TestOperatorTransportBoundary_HTTPEnrollmentRejectsOperators(t *testing.T) {
	rig := newBoundaryRig(t, fixtures.OwnerHoldsRequests)
	ctx, cancel := context.WithTimeout(t.Context(), boundaryTimeout)
	defer cancel()

	operatorCSR, _, err := serve.GenerateCSR("g8eo")
	require.NoError(t, err)
	cliCSR, _, err := serve.GenerateCSR("g8eo-cli")
	require.NoError(t, err)
	appCSR, _, err := serve.GenerateCSR("t8-control")
	require.NoError(t, err)

	operatorCode := postEnrollmentRequest(t, ctx, rig.gateway, models.PlatformEnrollmentCreateRequest{
		ComponentKind:     models.PlatformComponentOperator,
		InstanceID:        "t8-operator",
		Hostname:          "t8-host",
		SystemFingerprint: "t8-fingerprint",
		Operator:          &models.PlatformOperatorCSRPayload{OperatorCSRPEM: operatorCSR, CLICSRPEM: cliCSR},
	})
	assert.True(t, operatorCode >= 400 && operatorCode < 500, "an Operator enrollment over HTTP must be refused, got %d", operatorCode)

	appCode := postEnrollmentRequest(t, ctx, rig.gateway, models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentApplication,
		AppName:       "t8-control",
		InstanceID:    "t8-app",
		Hostname:      "t8-host",
		App:           &models.PlatformAppCSRPayload{CSRPEM: appCSR},
	})
	assert.Equal(t, http.StatusCreated, appCode, "application enrollment stays on HTTP")

	pending, err := rig.gateway.Service.GetPlatformEnrollmentService().ListPending(ctx)
	require.NoError(t, err)
	for _, request := range pending.Requests {
		assert.NotEqual(t, models.PlatformComponentOperator, request.ComponentKind, "an Operator request is pending from HTTP: %s", request.RequestID)
	}
}

// postEnrollmentRequest submits req with a fresh token to the Gateway HTTP
// port and returns the status code.
func postEnrollmentRequest(t *testing.T, ctx context.Context, f *fixtures.GatewayFixture, req models.PlatformEnrollmentCreateRequest) int {
	t.Helper()
	token, err := models.NewPlatformEnrollmentToken()
	require.NoError(t, err)
	req.TokenHash = models.PlatformEnrollmentTokenHash(token)
	body, err := json.Marshal(req)
	require.NoError(t, err)
	url := network.LocalhostHTTPURL(f.Service.GetHTTPPort()) + constants.APIPaths.AuthPlatformEnrollmentRequest
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// T3: a burst of Operators enrolls at once and the owner approves them in one
// decision. Every committed identity reaches the Operator that requested it,
// and no two Operators share one.
func TestOperatorTransportBoundary_BurstDeliversEveryCommittedIdentity(t *testing.T) {
	requireBurstBuild(t)
	rig := newBoundaryRig(t, fixtures.OwnerHoldsRequests)
	ctx, cancel := context.WithTimeout(t.Context(), burstTimeout)
	defer cancel()

	ops := make([]*fixtures.OperatorProcess, burstOperators)
	for i := range ops {
		ops[i] = rig.witness.LaunchOperator(t, fixtures.NewOperatorHost(t), fixtures.OperatorProcessOptions{HeartbeatInterval: burstHeartbeatInterval})
	}

	requestIDs, err := rig.owner.AwaitEnrollmentRequests(ctx, burstOperators)
	require.NoError(t, err)
	require.Len(t, requestIDs, burstOperators, "every Operator must create exactly one request")
	require.NoError(t, rig.owner.ApproveBatch(ctx, requestIDs))

	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []string
	for i, op := range ops {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := op.AwaitStarted(ctx)
			if err == nil {
				_, err = op.AwaitHeartbeat(ctx)
			}
			if err != nil {
				mu.Lock()
				failures = append(failures, fmt.Sprintf("operator %d: %v", i, err))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	require.Empty(t, failures, "%d of %d Operators never came up", len(failures), burstOperators)

	held := make(map[string]*x509.Certificate, len(ops))
	for i, op := range ops {
		leaf, err := op.Leaf()
		require.NoError(t, err)
		serial := leaf.SerialNumber.String()
		_, dup := held[serial]
		require.False(t, dup, "operator %d holds a certificate another Operator holds", i)
		held[serial] = leaf
	}
	for _, id := range requestIDs {
		stored, err := rig.owner.StoredRequest(ctx, id)
		require.NoError(t, err)
		committed, err := rig.owner.CommittedOperatorCertificate(ctx, id)
		require.NoError(t, err)
		serial := committed.SerialNumber.String()
		leaf, ok := held[serial]
		if !assert.True(t, ok, "request %s committed serial %s, which no Operator holds", id, serial) {
			continue
		}
		workload, ok := fixtures.OperatorWorkloadOf(leaf)
		require.True(t, ok)
		assert.Equal(t, stored.OperatorID, workload.OperatorID, "request %s", id)
		assert.Equal(t, stored.OperatorSessionID, workload.OperatorSessionID, "request %s", id)
		delete(held, serial)
	}
	assert.Empty(t, held, "Operators hold certificates no request committed")
}
