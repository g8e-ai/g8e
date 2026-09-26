// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliancecmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancereport "github.com/g8e-ai/g8e/v2/internal/services/compliance/report"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func TestValidateReleaseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid version", input: "v2.1.3"},
		{name: "valid patch zero", input: "v2.0.0"},
		{name: "valid prerelease", input: "v2.1.3-beta.1"},
		{name: "valid build metadata", input: "v2.1.3+build123"},
		{name: "empty", wantErr: true},
		{name: "missing v prefix", input: "2.1.3", wantErr: true},
		{name: "lone v", input: "v", wantErr: true},
		{name: "incomplete semver", input: "v2.1", wantErr: true},
		{name: "leading zero", input: "v02.1.0", wantErr: true},
		{name: "slash traversal", input: "v2.1.3/../../evil", wantErr: true},
		{name: "backslash traversal", input: `v2.1.3\..\evil`, wantErr: true},
		{name: "parent directory", input: "../v2.1.3", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateReleaseVersion(test.input)
			if test.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, constants.ErrValidationFailed)
				return
			}
			require.NoError(t, err)
		})
	}
}

type releaseProjectionBundleReader struct {
	bodies map[string][]byte
}

func (r *releaseProjectionBundleReader) ReadFile(ctx context.Context, bundlePath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, ok := r.bodies[bundlePath]
	if !ok {
		return nil, constants.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}

func (r *releaseProjectionBundleReader) ListFiles(context.Context) ([]string, error) {
	paths := make([]string, 0, len(r.bodies))
	for bundlePath := range r.bodies {
		paths = append(paths, bundlePath)
	}
	return paths, nil
}

func TestComplianceReleaseEvidenceProjection_UsesVerifiedPublicBundleScope(t *testing.T) {
	scope := &compliancev1.AssessmentScope{ProductVersion: "2.1.12"}
	scopeBody, err := compliancev1.MarshalCanonical(scope)
	require.NoError(t, err)
	reader := &releaseProjectionBundleReader{bodies: map[string][]byte{constants.ComplianceBundleScopeFilename: scopeBody, constants.ComplianceBundleMarkdownPath: []byte("# canonical public report\n"), constants.ComplianceBundleCSVPath: []byte("record_type\nanalysis\n")}}
	closed := false
	input := complianceReportBundleInput{
		bundle:      &compliancev1.ComplianceReportBundle{Manifest: &compliancev1.ComplianceReportManifest{BundleProfile: constants.ComplianceBundleProfilePublic}},
		reader:      reader,
		trustPolicy: &compliancev1.ComplianceReportTrustPolicy{},
		close: func() error {
			closed = true
			return nil
		},
	}
	cmd := complianceReleaseEvidenceProjectionCmdWithConfig(
		func(context.Context, string, string, string) (complianceReportBundleInput, error) { return input, nil },
		func(context.Context, compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error) {
			return &compliancev1.ComplianceVerificationReport{Valid: true}, nil
		},
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	)
	outDir := t.TempDir()
	cmd.SetArgs([]string{constants.ComplianceBundleManifestFilename, "--out", outDir, "--trust-policy", constants.ComplianceReportTrustPolicyTestFilename})
	require.NoError(t, cmd.Execute())
	assert.True(t, closed)
	markdown, err := os.ReadFile(filepath.Join(outDir, "v2.1.12"+constants.ReleaseEvidenceMarkdownSuffix))
	require.NoError(t, err)
	assert.Equal(t, "# canonical public report\n", string(markdown))
	csvBody, err := os.ReadFile(filepath.Join(outDir, "v2.1.12"+constants.ReleaseEvidenceCSVSuffix))
	require.NoError(t, err)
	assert.Equal(t, "record_type\nanalysis\n", string(csvBody))
}

func TestComplianceReleaseEvidenceProjection_RejectsUnverifiedOrRestrictedBundle(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		valid   bool
		target  error
	}{
		{name: "unverified public bundle", profile: constants.ComplianceBundleProfilePublic, target: constants.ErrReportVerificationFailed},
		{name: "verified restricted bundle", profile: constants.ComplianceBundleProfileRestricted, valid: true, target: constants.ErrBundleProfileUnsupported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := complianceReportBundleInput{
				bundle: &compliancev1.ComplianceReportBundle{Manifest: &compliancev1.ComplianceReportManifest{BundleProfile: test.profile}},
				reader: &releaseProjectionBundleReader{},
			}
			cmd := complianceReleaseEvidenceProjectionCmdWithConfig(
				func(context.Context, string, string, string) (complianceReportBundleInput, error) { return input, nil },
				func(context.Context, compliancereport.BundleVerificationRequest) (*compliancev1.ComplianceVerificationReport, error) {
					return &compliancev1.ComplianceVerificationReport{Valid: test.valid}, nil
				},
				time.Now,
			)
			cmd.SetArgs([]string{constants.ComplianceBundleManifestFilename, "--out", t.TempDir(), "--trust-policy", constants.ComplianceReportTrustPolicyTestFilename})
			err := cmd.Execute()
			require.Error(t, err)
			assert.ErrorIs(t, err, test.target)
		})
	}
}

func TestWriteReleaseProjectionArtifacts_RejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		outDir  string
		version string
	}{
		{name: "missing output directory", version: "v2.1.12"},
		{name: "traversing version", outDir: t.TempDir(), version: "../v2.1.12"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := writeReleaseProjectionArtifacts(test.outDir, test.version, nil, nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrValidationFailed)
		})
	}
}
