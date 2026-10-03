// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package tests

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/g8e-ai/g8e/v2/protocol"
	"github.com/g8e-ai/g8e/v2/test/fixtures"
)

// The g8ee enrollment client is Python and hand-encodes the completion
// transcript the Gateway verifies, so only a run of the real client against a
// real Gateway proves the two agree end to end. These tests start a Gateway,
// run ensemble/app/services/infra/app_enrollment_service.py as a subprocess with
// an isolated runtime directory, decide the request as the owner, and verify
// what the client persisted. They fail, never skip, when the ensemble
// virtualenv is missing: run `make setup` in ensemble/ first.

const (
	ensembleEnrollmentInstanceID = "ensemble-integration-test"
	ensembleEnrollmentTimeout    = 90 * time.Second
)

const ensembleEnrollScript = `
import asyncio, json, sys

from app.constants.bootstrap import BootstrapSettings, configure_bootstrap

configure_bootstrap(BootstrapSettings(gateway_http_url=sys.argv[1], runtime_dir=sys.argv[2]))

from app.services.infra.app_enrollment_service import AppEnrollmentService

service = AppEnrollmentService(instance_id="` + ensembleEnrollmentInstanceID + `", hostname="integration-host")
identity = asyncio.run(service.enroll())
print(json.dumps({
    "app_id": identity.app_id,
    "cert_path": identity.cert_path,
    "key_path": identity.key_path,
    "ca_cert_path": identity.ca_cert_path,
}))
`

// syncBuffer is a bytes.Buffer safe to read for diagnostics while the client
// process is still writing to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type ensembleEnrollmentRun struct {
	err        error
	stdout     string
	stderr     string
	runtimeDir string
}

func ensembleDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve test file location")
	return filepath.Join(filepath.Dir(thisFile), "..", "ensemble")
}

// runEnsembleEnrollment starts the real Python enrollment client against the
// fixture Gateway, waits for its request to become pending, applies the owner
// decision, and returns the process outcome.
func runEnsembleEnrollment(t *testing.T, f *fixtures.GatewayFixture, ownerID string, decision models.PlatformEnrollmentDecision) ensembleEnrollmentRun {
	t.Helper()

	ensemble := ensembleDir(t)
	python := filepath.Join(ensemble, ".venv", "bin", "python")
	require.FileExists(t, python, "ensemble virtualenv is required for the enrollment integration test; run `make setup` in ensemble/")

	runtimeDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), ensembleEnrollmentTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, python, "-c", ensembleEnrollScript,
		network.LocalhostHTTPURL(f.Service.GetHTTPPort()), runtimeDir)
	cmd.Dir = ensemble
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + runtimeDir,
		"PYTHONDONTWRITEBYTECODE=1",
	}
	var stdout, stderr syncBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	enrollSvc := f.Service.GetPlatformEnrollmentService()
	var requestID string
	require.Eventually(t, func() bool {
		pending, err := enrollSvc.ListPending(context.Background())
		if err != nil {
			return false
		}
		for _, req := range pending.Requests {
			if req.InstanceID == ensembleEnrollmentInstanceID {
				requestID = req.RequestID
				return true
			}
		}
		return false
	}, 60*time.Second, 100*time.Millisecond, "ensemble enrollment request never became pending; stderr:\n%s", stderr.String())

	_, err := enrollSvc.Decide(context.Background(), ownerID, models.PlatformEnrollmentDecisionRequest{
		RequestID: requestID,
		Decision:  decision,
	})
	require.NoError(t, err)

	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		t.Fatalf("ensemble enrollment client did not finish within %s; stderr:\n%s", ensembleEnrollmentTimeout, stderr.String())
	}

	return ensembleEnrollmentRun{err: waitErr, stdout: stdout.String(), stderr: stderr.String(), runtimeDir: runtimeDir}
}

func newEnsembleEnrollmentGateway(t *testing.T) (*fixtures.GatewayFixture, string) {
	t.Helper()
	f := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          t.Name(),
		Posture:           config.PostureDoctrine,
		AllowTestPortZero: true,
	})
	f.WaitForReady(t)

	// The Gateway refuses platform enrollment until an owner exists.
	owner, err := f.Service.GetUserService().CreateUser()
	require.NoError(t, err)
	return f, owner.ID
}

func TestEnsembleEnrollmentClientEnrollsAgainstRealGateway(t *testing.T) {
	f, ownerID := newEnsembleEnrollmentGateway(t)

	run := runEnsembleEnrollment(t, f, ownerID, models.PlatformEnrollmentDecisionApprove)
	require.NoError(t, run.err, "client must complete enrollment; stderr:\n%s", run.stderr)

	lines := strings.Split(strings.TrimSpace(run.stdout), "\n")
	var identity struct {
		AppID      string `json:"app_id"`
		CertPath   string `json:"cert_path"`
		KeyPath    string `json:"key_path"`
		CACertPath string `json:"ca_cert_path"`
	}
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &identity), "client stdout: %s", run.stdout)

	expectedAppID := protocol.NewWorkloadIdentity().AppSPIFFEID("g8ee")
	assert.Equal(t, expectedAppID, identity.AppID)
	for _, p := range []string{identity.CertPath, identity.KeyPath, identity.CACertPath} {
		assert.True(t, strings.HasPrefix(p, run.runtimeDir), "%s must live under the isolated runtime dir", p)
	}

	certPEM, err := os.ReadFile(identity.CertPath)
	require.NoError(t, err)
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block, "issued certificate must be PEM")
	leaf, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// The Gateway binds the app identity and the approving owner (the human
	// delegator) into the certificate; nothing else.
	var gotURIs []string
	for _, u := range leaf.URIs {
		gotURIs = append(gotURIs, u.String())
	}
	assert.ElementsMatch(t, []string{expectedAppID, protocol.NewWorkloadIdentity().UserSPIFFEID(ownerID)}, gotURIs)
	assert.Contains(t, leaf.ExtKeyUsage, x509.ExtKeyUsageClientAuth)

	// The trust bundle the client pinned must contain the Gateway's real root
	// and the issued certificate must chain to it.
	bundlePEM, err := os.ReadFile(identity.CACertPath)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(bundlePEM), "client-pinned CA bundle must parse")
	rootBlock, _ := pem.Decode(testutil.ReadRootCA(t, f.PKIDir))
	require.NotNil(t, rootBlock)
	assert.Contains(t, string(bundlePEM), string(pem.EncodeToMemory(rootBlock)), "pinned bundle must contain the Gateway root CA")
	_, err = leaf.Verify(x509.VerifyOptions{
		Roots:         pool,
		Intermediates: pool,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	require.NoError(t, err, "issued certificate must chain to the Gateway trust bundle")

	// The private key the client wrote is the one the Gateway certified.
	keyPEM, err := os.ReadFile(identity.KeyPath)
	require.NoError(t, err)
	keyBlock, _ := pem.Decode(keyPEM)
	require.NotNil(t, keyBlock)
	parsedKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	require.NoError(t, err)
	privKey, ok := parsedKey.(*ecdsa.PrivateKey)
	require.True(t, ok, "enrolled key must be ECDSA")
	pubKey, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok)
	assert.True(t, privKey.PublicKey.Equal(pubKey), "certificate must certify the client's private key")

	keyInfo, err := os.Stat(identity.KeyPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), keyInfo.Mode().Perm(), "private key must not be group or world readable")

	// The Gateway records the enrollment as completed, and the client removed
	// its pending-attempt state.
	enrolled, err := f.Service.GetPlatformEnrollmentService().ListEnrolled(context.Background())
	require.NoError(t, err)
	var found bool
	for _, e := range enrolled.Enrollments {
		if e.InstanceID == ensembleEnrollmentInstanceID {
			found = true
			assert.Equal(t, models.PlatformComponentEnsemble, e.ComponentKind)
			assert.Equal(t, models.PlatformEnrollmentStateCompleted, e.State)
		}
	}
	assert.True(t, found, "Gateway must list the ensemble enrollment")

	pending, err := filepath.Glob(filepath.Join(run.runtimeDir, "pki", "pending-enrollment", "*"))
	require.NoError(t, err)
	assert.Empty(t, pending, "pending enrollment state must be removed after completion")
}

func TestEnsembleEnrollmentClientFailsClosedWhenOwnerDenies(t *testing.T) {
	f, ownerID := newEnsembleEnrollmentGateway(t)

	run := runEnsembleEnrollment(t, f, ownerID, models.PlatformEnrollmentDecisionDeny)
	require.Error(t, run.err, "client must not report success for a denied request; stdout:\n%s", run.stdout)

	assert.NoFileExists(t, filepath.Join(run.runtimeDir, "pki", "issued", "apps", "g8ee.crt"))
	assert.NoFileExists(t, filepath.Join(run.runtimeDir, "pki", "issued", "apps", "g8ee.key"))
	assert.Empty(t, strings.TrimSpace(run.stdout), "no identity may be emitted for a denied request")
}
