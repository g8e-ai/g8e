// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package docker

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestReportEvalAppEnrollment_MissingIdentityExplainsHowToEnroll(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	reportEvalAppEnrollment(cmd, fileSvc, cfg)

	out := buf.String()
	assert.Contains(t, out, constants.EvaluationAppName)
	assert.Contains(t, out, "not ready")
	assert.Contains(t, out, "./g8e auth enroll app "+constants.EvaluationAppName)
	assert.Contains(t, out, "./g8e auth enroll approve <request-id> --yes")
}

func TestReportEvalAppEnrollment_ValidIdentityReportsEnrolled(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: constants.EvaluationAppName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	ctx := context.Background()
	require.NoError(t, fileSvc.WriteFile(ctx, cmdtest.MustRel(t, fileSvc, cfg.AppCertFile(constants.EvaluationAppName)), certPEM, 0o600))
	require.NoError(t, fileSvc.WriteFile(ctx, cmdtest.MustRel(t, fileSvc, cfg.AppKeyFile(constants.EvaluationAppName)), keyPEM, 0o600))

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	reportEvalAppEnrollment(cmd, fileSvc, cfg)

	out := buf.String()
	assert.Contains(t, out, "enrolled")
	assert.NotContains(t, out, "not ready")
	assert.NotContains(t, out, "auth enroll app")
}

// evalEnrollerStub stands in for the host app enrollment client: it blocks
// until the owner decision is posted (or the context ends), then completes.
type evalEnrollerStub struct {
	approved chan struct{}
	err      error
}

func (e *evalEnrollerStub) Enroll(ctx context.Context, _ io.Writer) (*models.PlatformEnrollmentCompleteResponse, error) {
	if e.err != nil {
		return nil, e.err
	}
	select {
	case <-e.approved:
		return &models.PlatformEnrollmentCompleteResponse{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type evalApprovalAPIClient struct {
	stubWalkthroughAPIClient
	approved chan struct{}
}

func (c *evalApprovalAPIClient) Post(path string, body interface{}) ([]byte, error) {
	resp, err := c.stubWalkthroughAPIClient.Post(path, body)
	if err == nil {
		close(c.approved)
	}
	return resp, err
}

func pendingEvalRequests(t *testing.T, reqs ...models.PlatformEnrollmentPendingRequest) map[string][]byte {
	t.Helper()
	return map[string][]byte{constants.APIPaths.AuthPlatformEnrollmentPending: mustMarshalPendingResp(t, reqs)}
}

func TestRunDockerInitEvalEnrollment_ApprovesOnlyTheEvalApplication(t *testing.T) {
	approved := make(chan struct{})
	client := &evalApprovalAPIClient{approved: approved}
	client.getResponses = pendingEvalRequests(t,
		models.PlatformEnrollmentPendingRequest{RequestID: "req-other", ComponentKind: models.PlatformComponentApplication, ComponentName: "someone-else"},
		models.PlatformEnrollmentPendingRequest{RequestID: "req-eval", ComponentKind: models.PlatformComponentApplication, ComponentName: constants.EvaluationAppName},
		models.PlatformEnrollmentPendingRequest{RequestID: "req-op", ComponentKind: models.PlatformComponentOperator, ComponentName: constants.EvaluationAppName},
	)
	client.postResponses = map[string][]byte{constants.APIPaths.AuthPlatformEnrollmentDecision: []byte(`{}`)}

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(&buf)

	err := runDockerInitEvalEnrollment(cmd, client, &evalEnrollerStub{approved: approved}, time.Millisecond, 5*time.Second)

	require.NoError(t, err)
	require.Len(t, client.postCalls, 1, "only the g8e-eval application request may be approved")
	decision, ok := client.postCalls[0].body.(models.PlatformEnrollmentDecisionRequest)
	require.True(t, ok)
	assert.Equal(t, "req-eval", decision.RequestID)
	assert.Equal(t, models.PlatformEnrollmentDecisionApprove, decision.Decision)
}

func TestRunDockerInitEvalEnrollment_EnrollerFailureIsReported(t *testing.T) {
	client := &evalApprovalAPIClient{approved: make(chan struct{})}
	client.getResponses = pendingEvalRequests(t)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runDockerInitEvalEnrollment(cmd, client, &evalEnrollerStub{err: errors.New("gateway unreachable")}, time.Millisecond, 5*time.Second)

	require.ErrorIs(t, err, constants.ErrDockerInitApprovalFailed)
	assert.ErrorContains(t, err, "gateway unreachable")
	assert.Empty(t, client.postCalls)
}

func TestRunDockerInitEvalEnrollment_TimesOutWhenNoRequestAppears(t *testing.T) {
	client := &evalApprovalAPIClient{approved: make(chan struct{})}
	client.getResponses = pendingEvalRequests(t)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runDockerInitEvalEnrollment(cmd, client, &evalEnrollerStub{approved: client.approved}, time.Millisecond, 50*time.Millisecond)

	require.ErrorIs(t, err, constants.ErrDockerInitApprovalFailed)
	assert.Empty(t, client.postCalls)
}
