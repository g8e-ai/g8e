// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancereport "github.com/g8e-ai/g8e/v2/internal/services/compliance/report"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func complianceReportSigningFixtureForTest(t *testing.T, scopeID string) (*compliancereport.ComplianceReportSigningIdentity, *compliancev1.ComplianceReportTrustPolicy, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	digest := sha256.Sum256(publicKey)
	createdAt := time.Unix(1_699_999_900, 0).UTC()
	metadata := &compliancev1.ComplianceReportSigningKeyMetadata{
		KeyId:           "report-key-1",
		Algorithm:       constants.ComplianceReportSignatureAlgorithm,
		Purpose:         constants.ComplianceReportSigningPurpose,
		PublicKeySha256: hex.EncodeToString(digest[:]),
		CreatedAt:       timestamppb.New(createdAt),
		ExpiresAt:       timestamppb.New(createdAt.Add(time.Hour)),
	}
	identity, err := compliancereport.NewComplianceReportSigningIdentity(metadata, privateKey)
	require.NoError(t, err)
	policy := &compliancev1.ComplianceReportTrustPolicy{
		PolicyId:      "policy-1",
		PolicyVersion: "1.0.0",
		TrustedKeys: []*compliancev1.ComplianceReportTrustedKey{{
			Metadata:         metadata,
			PublicKey:        hex.EncodeToString(publicKey),
			AssessmentId:     "assessment-1",
			AssessorIdentity: "assessor-1",
			AssessedAt:       timestamppb.New(createdAt),
			AllowedScopeRefs: []string{scopeID},
		}},
	}
	return identity, policy, privateKey
}

func complianceReportSigningIdentityForTest(t *testing.T) *compliancereport.ComplianceReportSigningIdentity {
	identity, _, _ := complianceReportSigningFixtureForTest(t, evidence.EvalScopeID("evidence-graph-suite"))
	return identity
}

func complianceReportSigningIdentityLoaderForTest(t *testing.T) complianceReportSigningIdentityLoader {
	t.Helper()
	identity := complianceReportSigningIdentityForTest(t)
	return func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error) {
		return identity, nil
	}
}

func configureComplianceReportGenerateCommand(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	require.NoError(t, cmd.Flags().Set("report-id", "report-1"))
	require.NoError(t, cmd.Flags().Set("signing-metadata", constants.ComplianceReportSigningMetadataTestFilename))
	require.NoError(t, cmd.Flags().Set("signing-private-key", constants.ComplianceReportSigningPrivateKeyTestFilename))
}

func runComplianceReportGenerateCommand(t *testing.T, evalRuns []string) ([]byte, error) {
	t.Helper()
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), complianceReportSigningIdentityLoaderForTest(t))
	configureComplianceReportGenerateCommand(t, cmd)
	windowStart := time.Unix(1_699_999_999, 0).UTC()
	windowEnd := time.Unix(1_700_000_100, 0).UTC()
	require.NoError(t, cmd.Flags().Set("scope-id", evidence.EvalScopeID("evidence-graph-suite")))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", strconv.FormatInt(windowStart.UnixMilli(), 10)))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", strconv.FormatInt(windowEnd.UnixMilli(), 10)))
	for _, runID := range evalRuns {
		require.NoError(t, cmd.Flags().Set("eval-run", runID))
	}
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	return bytes.TrimSpace(output.Bytes()), cmd.RunE(cmd, nil)
}

func TestComplianceReportCmd_ContainsGenerateAndVerifySubcommands(t *testing.T) {
	cmd := complianceReportCmd()
	require.Len(t, cmd.Commands(), 2)
	assert.Equal(t, "generate", cmd.Commands()[0].Name())
	assert.Equal(t, "verify", cmd.Commands()[1].Name())
}

func TestComplianceReportGenerateCmdWithConfig_PersistsSignedBundleWithEveryProtectedRenderer(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	runID := persistMinimalEvidenceGraphEvalFixture(t, fileSvc)
	scopeID := evidence.EvalScopeID("evidence-graph-suite")
	_, policy, privateKey := complianceReportSigningFixtureForTest(t, scopeID)
	inputDir := t.TempDir()
	metadataPath := filepath.Join(inputDir, constants.ComplianceReportSigningMetadataTestFilename)
	metadataBody, err := compliancev1.MarshalCanonical(policy.GetTrustedKeys()[0].GetMetadata())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(metadataPath, metadataBody, constants.PermFileReadOnly))
	privateKeyPath := filepath.Join(inputDir, constants.ComplianceReportSigningPrivateKeyTestFilename)
	require.NoError(t, os.WriteFile(privateKeyPath, []byte(hex.EncodeToString(privateKey)), constants.PermFileReadOnly))
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), loadComplianceReportSigningIdentity)
	configureComplianceReportGenerateCommand(t, cmd)
	require.NoError(t, cmd.Flags().Set("signing-metadata", metadataPath))
	require.NoError(t, cmd.Flags().Set("signing-private-key", privateKeyPath))
	windowStart := time.Unix(1_699_999_999, 0).UTC()
	windowEnd := time.Unix(1_700_000_100, 0).UTC()
	require.NoError(t, cmd.Flags().Set("scope-id", evidence.EvalScopeID("evidence-graph-suite")))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", strconv.FormatInt(windowStart.UnixMilli(), 10)))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", strconv.FormatInt(windowEnd.UnixMilli(), 10)))
	require.NoError(t, cmd.Flags().Set("eval-run", runID))
	var output bytes.Buffer
	cmd.SetOut(&output)

	require.NoError(t, cmd.RunE(cmd, nil))
	descriptorPath, err := fileSvc.Rel(string(bytes.TrimSpace(output.Bytes())))
	require.NoError(t, err)
	descriptorBody, err := fileSvc.ReadFile(context.Background(), descriptorPath)
	require.NoError(t, err)
	bundle := &compliancev1.ComplianceReportBundle{}
	require.NoError(t, compliancev1.UnmarshalCanonical(descriptorBody, bundle))
	assert.Equal(t, evidence.EvalScopeID("evidence-graph-suite"), bundle.GetAnalysis().GetScopeRef())
	assert.NotNil(t, bundle.GetManifest().GetSignature())
	assert.NotNil(t, bundle.GetChecksumRootSignature())
	require.Len(t, bundle.GetRenderedFormats(), len(compliancereport.SupportedFormats()))
	for _, rendered := range bundle.GetRenderedFormats() {
		artifactPath := path.Join(constants.ComplianceBundlesDirname, "report-1", rendered.GetBundlePath())
		body, err := fileSvc.ReadFile(context.Background(), artifactPath)
		require.NoError(t, err)
		assert.NotEmpty(t, body)
	}
	trustBody, err := compliancev1.MarshalCanonical(policy)
	require.NoError(t, err)
	trustPath := filepath.Join(t.TempDir(), constants.ComplianceReportTrustPolicyTestFilename)
	require.NoError(t, os.WriteFile(trustPath, trustBody, constants.PermFilePublic))
	verifyCmd := complianceReportVerifyCmdWithConfig(loadComplianceReportBundleInput, compliancereport.VerifyComplianceReportBundle, func() time.Time { return windowEnd })
	require.NoError(t, verifyCmd.Flags().Set("trust-policy", trustPath))
	var verificationOutput bytes.Buffer
	verifyCmd.SetOut(&verificationOutput)

	require.NoError(t, verifyCmd.RunE(verifyCmd, []string{fileSvc.Resolve(descriptorPath)}))
	verificationReport := &compliancev1.ComplianceVerificationReport{}
	require.NoError(t, compliancev1.UnmarshalCanonical(bytes.TrimSpace(verificationOutput.Bytes()), verificationReport))
	assert.True(t, verificationReport.GetValid())
	assert.Empty(t, verificationReport.GetFailures())
}

func TestBuildDemoVerificationArtifacts_EmbedsValidExistingVerifierResult(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	projectRoot := writeDemoProvenanceTree(t)
	runID := persistMinimalDemoRunFixture(t, fileSvc, projectRoot)
	verifiedAt := time.Unix(1_800_000_000, 0).UTC()

	artifacts, err := buildDemoVerificationArtifacts(context.Background(), fileSvc, evidence.NewDemoDirectoryProvenanceSource(projectRoot), []string{runID}, verifiedAt)

	require.NoError(t, err)
	artifactByPath := make(map[string]compliancereport.SourceArtifact, len(artifacts))
	for _, artifact := range artifacts {
		artifactByPath[artifact.BundlePath] = artifact
	}
	verificationPath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceVerificationFilename)
	report := &compliancev1.ComplianceVerificationReport{}
	require.NoError(t, compliancev1.UnmarshalCanonical(artifactByPath[verificationPath].Body, report))
	assert.True(t, report.GetValid())
	assert.Equal(t, constants.DemoRunVerifierID, report.GetVerifierId())
	assert.NotEmpty(t, artifactByPath[path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceRuntimeDirname, constants.DemoRunManifestFilename)].Body)
	assert.NotEmpty(t, artifactByPath[path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceRuntimeDirname, constants.DemoRunResultsFilename)].Body)
	provenanceArtifacts, err := evidence.NewDemoDirectoryProvenanceSource(projectRoot).Artifacts(context.Background(), constants.DemosOrgFedRAMP)
	require.NoError(t, err)
	for _, expected := range provenanceArtifacts {
		bundlePath := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceProvenanceDirname, constants.ComplianceBundleSourceArtifactsDirname, filepath.ToSlash(expected.Name))
		assert.Equal(t, expected.Body, artifactByPath[bundlePath].Body)
	}
	assert.NotEmpty(t, artifactByPath[path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceBundleSourceDemosDirname, runID, constants.ComplianceBundleSourceProvenanceDirname, constants.DemoRunDefinitionsFilename)].Body)
}

func TestComplianceReportGenerateCmdWithConfig_RejectsUnsupportedProfile(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), complianceReportSigningIdentityLoaderForTest(t))
	configureComplianceReportGenerateCommand(t, cmd)
	require.NoError(t, cmd.Flags().Set("scope-id", "scope-1"))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", "1700000000000"))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", "1700000001000"))
	require.NoError(t, cmd.Flags().Set("eval-run", "run-1"))
	require.NoError(t, cmd.Flags().Set("profile", "confidential"))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrBundleProfileUnsupported)
}

func TestComplianceReportGenerateCmdWithConfig_RejectsMissingEvidenceRuns(t *testing.T) {
	body, err := runComplianceReportGenerateCommand(t, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Empty(t, body)
}

func TestComplianceReportGenerateCmdWithConfig_FailsClosedOnImporterFailure(t *testing.T) {
	body, err := runComplianceReportGenerateCommand(t, []string{"missing-eval-run"})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.Empty(t, body)
}

func TestComplianceReportGenerateCmdWithConfig_RejectsInvalidEvidenceWindow(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	cmd := complianceReportGenerateCmdWithConfig(fileSvcFactoryFor(fileSvc), stubProvenanceSourceFactory(nil), complianceReportSigningIdentityLoaderForTest(t))
	require.NoError(t, cmd.Flags().Set("scope-id", "scope-1"))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", "1700000001000"))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", "1700000000000"))
	require.NoError(t, cmd.Flags().Set("eval-run", "run-1"))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestComplianceReportVerifyCmdWithConfig_PrintsTypedValidReport(t *testing.T) {
	verifiedAt := time.Unix(1_700_000_100, 0).UTC()
	loaderCalled := false
	cmd := complianceReportVerifyCmdWithConfig(
		func(_ context.Context, bundlePath, trustPolicyPath string) (complianceReportBundleInput, error) {
			loaderCalled = true
			assert.Equal(t, "bundle.json", bundlePath)
			assert.Equal(t, "assessed-trust.json", trustPolicyPath)
			return complianceReportBundleInput{}, nil
		},
		func(_ context.Context, request compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error) {
			assert.Equal(t, verifiedAt, request.VerifiedAt)
			return &compliancev1.ComplianceVerificationReport{
				ReportId:               "report-1",
				Valid:                  true,
				VerifiedAt:             requestTimestamp(verifiedAt),
				VerifierId:             constants.ComplianceBundleVerifierID,
				VerifierVersion:        constants.ComplianceBundleVerifierVersion,
				ReproducedChecksumRoot: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}, nil
		},
		func() time.Time { return verifiedAt },
	)
	require.NoError(t, cmd.Flags().Set("trust-policy", "assessed-trust.json"))
	var output bytes.Buffer
	cmd.SetOut(&output)

	require.NoError(t, cmd.RunE(cmd, []string{"bundle.json"}))
	assert.True(t, loaderCalled)
	report := &compliancev1.ComplianceVerificationReport{}
	require.NoError(t, compliancev1.UnmarshalCanonical(bytes.TrimSpace(output.Bytes()), report))
	assert.True(t, report.GetValid())
	assert.Equal(t, "report-1", report.GetReportId())
}

func TestComplianceReportVerifyCmdWithConfig_ReturnsFailureAfterPrintingInvalidReport(t *testing.T) {
	verifiedAt := time.Unix(1_700_000_100, 0).UTC()
	cmd := complianceReportVerifyCmdWithConfig(
		func(context.Context, string, string) (complianceReportBundleInput, error) {
			return complianceReportBundleInput{}, nil
		},
		func(context.Context, compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error) {
			return &compliancev1.ComplianceVerificationReport{
				ReportId:               "report-1",
				Valid:                  false,
				VerifiedAt:             requestTimestamp(verifiedAt),
				VerifierId:             constants.ComplianceBundleVerifierID,
				VerifierVersion:        constants.ComplianceBundleVerifierVersion,
				ReproducedChecksumRoot: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Failures:               []*compliancev1.VerificationFailure{{Code: constants.ErrChecksumMismatch.Error(), SubjectRef: constants.ComplianceBundleAnalysisPath, Reason: "digest mismatch"}},
			}, nil
		},
		func() time.Time { return verifiedAt },
	)
	require.NoError(t, cmd.Flags().Set("trust-policy", "assessed-trust.json"))
	var output bytes.Buffer
	cmd.SetOut(&output)

	err := cmd.RunE(cmd, []string{"bundle.json"})

	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.Contains(t, output.String(), constants.ErrChecksumMismatch.Error())
}

func TestComplianceReportVerifyCmdWithConfig_PropagatesReaderCloseFailure(t *testing.T) {
	closeErr := errors.New("close failed")
	verifiedAt := time.Unix(1_700_000_100, 0).UTC()
	cmd := complianceReportVerifyCmdWithConfig(
		func(context.Context, string, string) (complianceReportBundleInput, error) {
			return complianceReportBundleInput{close: func() error { return closeErr }}, nil
		},
		func(context.Context, compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error) {
			return &compliancev1.ComplianceVerificationReport{ReportId: "report-1", Valid: true, VerifiedAt: timestamppb.New(verifiedAt), VerifierId: constants.ComplianceBundleVerifierID, VerifierVersion: constants.ComplianceBundleVerifierVersion, ReproducedChecksumRoot: strings.Repeat("a", 64)}, nil
		},
		func() time.Time { return verifiedAt },
	)
	require.NoError(t, cmd.Flags().Set("trust-policy", "assessed-trust.json"))

	err := cmd.RunE(cmd, []string{"bundle.json"})

	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.ErrorIs(t, err, closeErr)
}

func TestComplianceReportVerifyCmdWithConfig_RequiresExternalTrustPolicy(t *testing.T) {
	cmd := complianceReportVerifyCmdWithConfig(
		func(context.Context, string, string) (complianceReportBundleInput, error) {
			t.Fatal("loader must not run without an explicit trust policy")
			return complianceReportBundleInput{}, nil
		},
		compliancereport.VerifyComplianceReportBundle,
		time.Now,
	)

	err := cmd.RunE(cmd, []string{"bundle.json"})

	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestComplianceBundleRootReader_RejectsSymlinkEntries(t *testing.T) {
	rootPath := t.TempDir()
	targetPath := filepath.Join(rootPath, constants.ComplianceBundleAnalysisPath)
	require.NoError(t, os.WriteFile(targetPath, []byte(`{}`), constants.PermFilePublic))
	require.NoError(t, os.Symlink(constants.ComplianceBundleAnalysisPath, filepath.Join(rootPath, constants.ComplianceBundleUnexpectedTestPath)))
	root, err := os.OpenRoot(rootPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })

	_, err = (&complianceBundleRootReader{root: root}).ListFiles(context.Background())

	assert.ErrorIs(t, err, constants.ErrUnexpectedEvidenceArtifact)
}

func TestLoadComplianceReportBundleInput_RejectsTrustPolicyInsideBundle(t *testing.T) {
	rootPath := t.TempDir()
	bundlePath := filepath.Join(rootPath, constants.ComplianceBundleManifestPath)
	trustPath := filepath.Join(rootPath, constants.ComplianceBundleUnexpectedTestPath)
	require.NoError(t, os.WriteFile(bundlePath, []byte(`{}`), constants.PermFilePublic))
	require.NoError(t, os.WriteFile(trustPath, []byte(`{}`), constants.PermFilePublic))

	input, err := loadComplianceReportBundleInput(context.Background(), bundlePath, trustPath)

	assert.ErrorIs(t, err, constants.ErrEvidenceTrustNotAssessed)
	assert.Nil(t, input.bundle)
}

func requestTimestamp(value time.Time) *timestamppb.Timestamp {
	return timestamppb.New(value)
}
