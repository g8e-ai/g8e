// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

func newPlatformEnrollmentCSR(t *testing.T) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "ignored"}}, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})), key
}

func TestValidatePlatformEnrollmentRequestComputesTypedFingerprints(t *testing.T) {
	appCSR, _ := newPlatformEnrollmentCSR(t)
	request := models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Hostname:      "dashboard.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: appCSR},
	}

	fingerprints, err := validatePlatformEnrollmentRequest(request)
	require.NoError(t, err)
	assert.Len(t, fingerprints.App, sha256.Size*2)
	assert.Empty(t, fingerprints.Operator)
	assert.Empty(t, fingerprints.CLI)
}

func TestValidatePlatformEnrollmentRequestRejectsDuplicateOperatorKeys(t *testing.T) {
	csr, _ := newPlatformEnrollmentCSR(t)
	request := models.PlatformEnrollmentCreateRequest{
		ComponentKind:     models.PlatformComponentOperator,
		InstanceID:        "operator-1",
		Hostname:          "operator.local",
		SystemFingerprint: "fingerprint",
		Operator: &models.PlatformOperatorCSRPayload{
			OperatorCSRPEM: csr,
			CLICSRPEM:      csr,
		},
	}

	_, err := validatePlatformEnrollmentRequest(request)
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentDuplicateKey)
}

func TestVerifyPlatformEnrollmentProofsRequiresEverySubmittedKey(t *testing.T) {
	operatorCSR, operatorKey := newPlatformEnrollmentCSR(t)
	cliCSR, cliKey := newPlatformEnrollmentCSR(t)
	request := &models.PlatformEnrollmentRequest{
		ID:            "request-id",
		TokenHash:     platformEnrollmentTokenHash("token"),
		ComponentKind: models.PlatformComponentOperator,
		InstanceID:    "operator-1",
		Operator: &models.PlatformOperatorCSRPayload{
			OperatorCSRPEM: operatorCSR,
			CLICSRPEM:      cliCSR,
		},
	}
	fingerprints, err := validatePlatformEnrollmentRequest(models.PlatformEnrollmentCreateRequest{
		ComponentKind:     request.ComponentKind,
		InstanceID:        request.InstanceID,
		Hostname:          "operator.local",
		SystemFingerprint: "fingerprint",
		Operator:          request.Operator,
	})
	require.NoError(t, err)
	request.Fingerprints = fingerprints
	transcript, err := platformEnrollmentCompletionTranscript(request)
	require.NoError(t, err)
	digest := sha256.Sum256(transcript)
	operatorSignature, err := ecdsa.SignASN1(rand.Reader, operatorKey, digest[:])
	require.NoError(t, err)
	cliSignature, err := ecdsa.SignASN1(rand.Reader, cliKey, digest[:])
	require.NoError(t, err)

	err = verifyPlatformEnrollmentProofs(request, models.PlatformEnrollmentProofs{
		Operator: base64.RawURLEncoding.EncodeToString(operatorSignature),
		CLI:      base64.RawURLEncoding.EncodeToString(cliSignature),
	})
	assert.NoError(t, err)

	err = verifyPlatformEnrollmentProofs(request, models.PlatformEnrollmentProofs{
		Operator: base64.RawURLEncoding.EncodeToString(operatorSignature),
	})
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentProofRequired)
}

func TestPlatformEnrollmentCompletionTranscriptIsDeterministicAndBound(t *testing.T) {
	request := &models.PlatformEnrollmentRequest{
		ID:            "request-id",
		TokenHash:     platformEnrollmentTokenHash("token"),
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-1",
		Fingerprints:  models.PlatformEnrollmentCSRFingerprints{App: "app-fingerprint"},
	}

	first, err := platformEnrollmentCompletionTranscript(request)
	require.NoError(t, err)
	second, err := platformEnrollmentCompletionTranscript(request)
	require.NoError(t, err)
	assert.Equal(t, first, second)

	request.InstanceID = "dashboard-2"
	changed, err := platformEnrollmentCompletionTranscript(request)
	require.NoError(t, err)
	assert.NotEqual(t, first, changed)
}
