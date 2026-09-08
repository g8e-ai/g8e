// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// BundleProfile classifies which artifacts a bundle carries. Public bundles
// exclude restricted plaintext and carry only hash-safe public projections.
// Restricted bundles carry authenticated encryption metadata for restricted
// evidence and may include authorized plaintext-digest verification records.
type BundleProfile string

const (
	ProfilePublic     BundleProfile = BundleProfile(constants.ComplianceBundleProfilePublic)
	ProfileRestricted BundleProfile = BundleProfile(constants.ComplianceBundleProfileRestricted)
)

func ParseBundleProfile(value string) (BundleProfile, error) {
	switch BundleProfile(value) {
	case ProfilePublic:
		return ProfilePublic, nil
	case ProfileRestricted:
		return ProfileRestricted, nil
	default:
		return "", fmt.Errorf("%w: %q", constants.ErrBundleProfileUnsupported, value)
	}
}

// RenderedFormat carries the rendered bytes, media type, and bundle path for a
// single compliance report format. The bundle assembler computes the SHA-256
// and byte length from the body.
type RenderedFormat struct {
	Format     Format
	MediaType  string
	BundlePath string
	Body       []byte
}

// BundleAssemblyRequest carries the canonical analysis, framework profiles,
// rendered formats, scope reference, catalog references, and the selected
// bundle profile. The assembler classifies every artifact as public or
// restricted, computes checksums, derives the checksum root, and builds the
// manifest.
type BundleAssemblyRequest struct {
	Profile             BundleProfile
	Analysis            *compliancev1.ComplianceAnalysis
	Profiles            []*compliancev1.FrameworkProfile
	RenderedFormats     []RenderedFormat
	ScopeRef            string
	ReportID            string
	GeneratedAt         time.Time
	FrameworkRefs       []*compliancev1.VersionedReference
	AssertionCatalogRef string
	CrosswalkRefs       []string
	AssessmentRefs      []string
	EvidenceIndexRef    string
	SourceArtifacts     []SourceArtifact
	RestrictedArtifacts []RestrictedArtifact
}

// RestrictedArtifact carries the bundle path, canonical bytes, media type, and
// authenticated encryption metadata for a restricted evidence artifact. The
// assembler never decrypts restricted content; it checksums the ciphertext and
// records the encryption metadata so offline verification can authenticate the
// envelope without plaintext access.
type RestrictedArtifact struct {
	BundlePath string
	Body       []byte
	MediaType  string
	Encryption *compliancev1.EvidenceEncryptionMetadata
}

type SourceArtifact struct {
	BundlePath string
	Body       []byte
	MediaType  string
}

type BundleArtifactBody struct {
	BundlePath string
	Body       []byte
}

// BundleAssemblyResult carries the assembled bundle with the manifest,
// checksummed artifacts, analysis, profiles, rendered format entries, and the
// checksum root. The manifest signature and checksum root signature are nil
// until SignBundle applies them.
type BundleAssemblyResult struct {
	Bundle          *compliancev1.ComplianceReportBundle
	ChecksumEntries []*compliancev1.ChecksumEntry
	ArtifactBodies  []BundleArtifactBody
	ManifestBytes   []byte
}

// AssembleBundle enumerates every protected artifact in the request, classifies
// each as public or restricted, computes SHA-256 checksums and byte lengths,
// derives the deterministic checksum root, and builds the manifest. The
// manifest signature and checksum root signature remain nil; SignBundle applies
// them with the dedicated compliance-report signing identity.
func AssembleBundle(request BundleAssemblyRequest) (*BundleAssemblyResult, error) {
	if err := validateBundleAssemblyRequest(request); err != nil {
		return nil, err
	}
	artifacts := make([]*compliancev1.BundleArtifact, 0, 16)
	checksumEntries := make([]*compliancev1.ChecksumEntry, 0, 16)
	artifactBodies := make([]BundleArtifactBody, 0, 16)

	analysisBytes, err := compliancev1.MarshalCanonical(request.Analysis)
	if err != nil {
		return nil, fmt.Errorf("%w: canonicalize analysis: %w", constants.ErrBundleAssemblyFailed, err)
	}
	if err := addArtifact(&artifacts, &checksumEntries, constants.ComplianceBundleAnalysisPath, analysisBytes, constants.MediaTypeJSON, constants.ComplianceBundleProfilePublic); err != nil {
		return nil, err
	}
	artifactBodies = append(artifactBodies, BundleArtifactBody{BundlePath: constants.ComplianceBundleAnalysisPath, Body: append([]byte(nil), analysisBytes...)})

	for i, profile := range request.Profiles {
		profileBytes, err := compliancev1.MarshalCanonical(profile)
		if err != nil {
			return nil, fmt.Errorf("%w: canonicalize framework profile %d: %w", constants.ErrBundleAssemblyFailed, i, err)
		}
		profilePath := fmt.Sprintf("%s/%s.json", constants.ComplianceBundleProfilesDirname, profile.GetProfileId())
		if err := addArtifact(&artifacts, &checksumEntries, profilePath, profileBytes, constants.MediaTypeJSON, constants.ComplianceBundleProfilePublic); err != nil {
			return nil, err
		}
		artifactBodies = append(artifactBodies, BundleArtifactBody{BundlePath: profilePath, Body: append([]byte(nil), profileBytes...)})
	}

	for _, source := range request.SourceArtifacts {
		if err := addSourceArtifact(&artifacts, &checksumEntries, source.BundlePath, source.Body, source.MediaType); err != nil {
			return nil, err
		}
		artifactBodies = append(artifactBodies, BundleArtifactBody{BundlePath: source.BundlePath, Body: append([]byte(nil), source.Body...)})
	}

	renderedEntries := make([]*compliancev1.RenderedFormatEntry, 0, len(request.RenderedFormats))
	for _, rendered := range request.RenderedFormats {
		if rendered.BundlePath == constants.ComplianceBundleAnalysisPath && rendered.Format == FormatJSON && rendered.MediaType == constants.MediaTypeJSON {
			if err := validateBundleArtifactInputs(rendered.BundlePath, rendered.Body, rendered.MediaType, false); err != nil {
				return nil, err
			}
			if !bytes.Equal(rendered.Body, analysisBytes) {
				return nil, fmt.Errorf("%w: rendered JSON does not match canonical analysis", constants.ErrBundleAssemblyFailed)
			}
		} else {
			if err := addArtifact(&artifacts, &checksumEntries, rendered.BundlePath, rendered.Body, rendered.MediaType, constants.ComplianceBundleProfilePublic); err != nil {
				return nil, err
			}
			artifactBodies = append(artifactBodies, BundleArtifactBody{BundlePath: rendered.BundlePath, Body: append([]byte(nil), rendered.Body...)})
		}
		renderedEntries = append(renderedEntries, &compliancev1.RenderedFormatEntry{
			Format:     string(rendered.Format),
			MediaType:  rendered.MediaType,
			BundlePath: rendered.BundlePath,
		})
	}

	if request.Profile == ProfileRestricted {
		for _, restricted := range request.RestrictedArtifacts {
			if err := addRestrictedArtifact(&artifacts, &checksumEntries, restricted); err != nil {
				return nil, err
			}
			artifactBodies = append(artifactBodies, BundleArtifactBody{BundlePath: restricted.BundlePath, Body: append([]byte(nil), restricted.Body...)})
		}
	}

	sortArtifacts(artifacts, checksumEntries)
	sort.Slice(artifactBodies, func(i, j int) bool { return artifactBodies[i].BundlePath < artifactBodies[j].BundlePath })
	checksumEntries, err = artifactDescriptorChecksumEntries(artifacts)
	if err != nil {
		return nil, err
	}

	checksumRoot, err := computeChecksumRoot(checksumEntries)
	if err != nil {
		return nil, err
	}

	manifest := &compliancev1.ComplianceReportManifest{
		ReportId:            request.ReportID,
		ReportSchemaVersion: constants.ComplianceBundleSchemaVersion,
		GeneratedAt:         timestamppb.New(request.GeneratedAt.UTC()),
		GeneratorIdentity:   constants.ComplianceBundleAssemblerID,
		GeneratorVersion:    constants.ComplianceBundleAssemblerVersion,
		ScopeRef:            request.ScopeRef,
		FrameworkRefs:       cloneVersionedRefs(request.FrameworkRefs),
		AssertionCatalogRef: request.AssertionCatalogRef,
		CrosswalkRefs:       append([]string(nil), request.CrosswalkRefs...),
		AssessmentRefs:      append([]string(nil), request.AssessmentRefs...),
		EvidenceIndexRef:    request.EvidenceIndexRef,
		ChecksumRoot:        checksumRoot,
		BundleProfile:       string(request.Profile),
	}

	manifestBytes, err := canonicalManifestBytes(manifest)
	if err != nil {
		return nil, err
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	manifest.ManifestSha256 = hex.EncodeToString(manifestDigest[:])

	bundle := &compliancev1.ComplianceReportBundle{
		Manifest:        manifest,
		Artifacts:       artifacts,
		Analysis:        proto.Clone(request.Analysis).(*compliancev1.ComplianceAnalysis),
		Profiles:        cloneProfiles(request.Profiles),
		RenderedFormats: renderedEntries,
		ChecksumRoot:    checksumRoot,
	}
	return &BundleAssemblyResult{
		Bundle:          bundle,
		ChecksumEntries: checksumEntries,
		ArtifactBodies:  artifactBodies,
		ManifestBytes:   manifestBytes,
	}, nil
}

func PersistBundle(ctx context.Context, fileSvc fs.RuntimeFileService, result *BundleAssemblyResult) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("%w: context is required", constants.ErrBundlePersistenceFailed)
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrBundlePersistenceFailed, err)
	}
	if fileSvc == nil || result == nil || result.Bundle == nil {
		return "", fmt.Errorf("%w: file service and assembled bundle are required", constants.ErrBundlePersistenceFailed)
	}
	bundle := result.Bundle
	if err := catalog.ValidateComplianceReportBundle(bundle, &compliancev1.FrameworkCatalog{Frameworks: collectFrameworkDefinitions(bundle.GetManifest().GetFrameworkRefs())}); err != nil {
		return "", fmt.Errorf("%w: validate assembled bundle: %w", constants.ErrBundlePersistenceFailed, err)
	}
	reportID := bundle.GetManifest().GetReportId()
	if reportID != strings.TrimSpace(reportID) || strings.ContainsAny(reportID, `/\\:`) || path.Clean(reportID) != reportID || reportID == "." || reportID == constants.PathParentDir {
		return "", fmt.Errorf("%w: unsafe report ID %q", constants.ErrBundlePersistenceFailed, reportID)
	}
	if len(result.ArtifactBodies) != len(bundle.GetArtifacts()) {
		return "", fmt.Errorf("%w: protected body count does not match artifact inventory", constants.ErrBundlePersistenceFailed)
	}
	bodies := make(map[string][]byte, len(result.ArtifactBodies))
	previousPath := ""
	for _, artifactBody := range result.ArtifactBodies {
		if artifactBody.BundlePath == constants.ComplianceBundleManifestPath || artifactBody.BundlePath <= previousPath {
			return "", fmt.Errorf("%w: protected body paths are duplicated, reserved, or unsorted", constants.ErrBundlePersistenceFailed)
		}
		previousPath = artifactBody.BundlePath
		bodies[artifactBody.BundlePath] = artifactBody.Body
	}
	for _, artifact := range bundle.GetArtifacts() {
		body, exists := bodies[artifact.GetBundlePath()]
		if !exists {
			return "", fmt.Errorf("%w: protected body %s is missing", constants.ErrBundlePersistenceFailed, artifact.GetBundlePath())
		}
		digest := sha256.Sum256(body)
		if hex.EncodeToString(digest[:]) != artifact.GetSha256() || int64(len(body)) != artifact.GetByteLength() {
			return "", fmt.Errorf("%w: protected body %s does not match its descriptor", constants.ErrBundlePersistenceFailed, artifact.GetBundlePath())
		}
	}
	descriptorBody, err := compliancev1.MarshalCanonical(bundle)
	if err != nil {
		return "", fmt.Errorf("%w: canonicalize bundle descriptor: %w", constants.ErrBundlePersistenceFailed, err)
	}
	bundleDir := path.Join(constants.ComplianceBundlesDirname, reportID)
	exists, err := fileSvc.FileExists(ctx, bundleDir)
	if err != nil {
		return "", fmt.Errorf("%w: inspect bundle destination: %w", constants.ErrBundlePersistenceFailed, err)
	}
	if exists {
		return "", fmt.Errorf("%w: bundle destination already exists", constants.ErrBundlePersistenceFailed)
	}
	if err := fileSvc.MkdirAll(ctx, bundleDir, constants.PermDirStandard); err != nil {
		return "", fmt.Errorf("%w: create bundle directory: %w", constants.ErrBundlePersistenceFailed, err)
	}
	for _, artifactBody := range result.ArtifactBodies {
		artifactPath := path.Join(bundleDir, artifactBody.BundlePath)
		if err := fileSvc.WriteFile(ctx, artifactPath, artifactBody.Body, constants.PermFilePublic); err != nil {
			return "", cleanupIncompleteBundle(ctx, fileSvc, bundleDir, fmt.Errorf("write protected body %s: %w", artifactBody.BundlePath, err))
		}
	}
	descriptorPath := path.Join(bundleDir, constants.ComplianceBundleManifestPath)
	if err := fileSvc.WriteFile(ctx, descriptorPath, descriptorBody, constants.PermFilePublic); err != nil {
		return "", cleanupIncompleteBundle(ctx, fileSvc, bundleDir, fmt.Errorf("write canonical bundle descriptor: %w", err))
	}
	return descriptorPath, nil
}

func cleanupIncompleteBundle(ctx context.Context, fileSvc fs.RuntimeFileService, bundleDir string, persistErr error) error {
	if err := fileSvc.RemoveAll(context.WithoutCancel(ctx), bundleDir); err != nil {
		return fmt.Errorf("%w: %w; remove incomplete bundle: %w", constants.ErrBundlePersistenceFailed, persistErr, err)
	}
	return fmt.Errorf("%w: %w", constants.ErrBundlePersistenceFailed, persistErr)
}

// SignBundle signs the checksum root and manifest root with the dedicated
// compliance-report signing identity. The checksum root signature is stored on
// the bundle; the manifest signature is stored on the manifest. The manifest
// SHA-256 is recomputed from canonical manifest bytes with the digest and
// signature fields cleared so the signed digest binds the complete manifest
// content without becoming self-referential.
func SignBundle(result *BundleAssemblyResult, identity *ComplianceReportSigningIdentity) error {
	if result == nil || result.Bundle == nil {
		return fmt.Errorf("%w: bundle assembly result is missing", constants.ErrBundleAssemblyFailed)
	}
	if identity == nil {
		return fmt.Errorf("%w: signing identity is required", constants.ErrReportSignatureFailed)
	}
	checksumRootSignature, err := identity.SignSHA256(result.Bundle.GetChecksumRoot())
	if err != nil {
		return fmt.Errorf("%w: sign checksum root: %w", constants.ErrReportSignatureFailed, err)
	}
	result.Bundle.ChecksumRootSignature = checksumRootSignature

	manifest := result.Bundle.GetManifest()
	manifest.Signature = nil
	manifestBytes, err := canonicalManifestBytes(manifest)
	if err != nil {
		return fmt.Errorf("%w: canonicalize manifest for signing: %w", constants.ErrBundleAssemblyFailed, err)
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	manifest.ManifestSha256 = hex.EncodeToString(manifestDigest[:])
	manifestSignature, err := identity.SignSHA256(manifest.ManifestSha256)
	if err != nil {
		return fmt.Errorf("%w: sign manifest root: %w", constants.ErrReportSignatureFailed, err)
	}
	manifest.Signature = manifestSignature
	result.ManifestBytes = manifestBytes
	return nil
}

func validateBundleAssemblyRequest(request BundleAssemblyRequest) error {
	if _, err := ParseBundleProfile(string(request.Profile)); err != nil {
		return err
	}
	if request.Analysis == nil {
		return fmt.Errorf("%w: analysis is required", constants.ErrBundleAssemblyFailed)
	}
	if request.ScopeRef == "" {
		return fmt.Errorf("%w: scope reference is required", constants.ErrBundleAssemblyFailed)
	}
	if request.ReportID == "" {
		return fmt.Errorf("%w: report ID is required", constants.ErrBundleAssemblyFailed)
	}
	if request.GeneratedAt.IsZero() {
		return fmt.Errorf("%w: generation timestamp is required", constants.ErrBundleAssemblyFailed)
	}
	if len(request.FrameworkRefs) == 0 {
		return fmt.Errorf("%w: framework references are required", constants.ErrBundleAssemblyFailed)
	}
	if request.AssertionCatalogRef == "" || len(request.CrosswalkRefs) == 0 || len(request.AssessmentRefs) == 0 || request.EvidenceIndexRef == "" {
		return fmt.Errorf("%w: catalog, crosswalk, assessment, and evidence index references are required", constants.ErrBundleAssemblyFailed)
	}
	if err := catalog.ValidateReportManifestReferences(
		request.ScopeRef,
		request.FrameworkRefs,
		request.AssertionCatalogRef,
		request.CrosswalkRefs,
		request.AssessmentRefs,
		request.EvidenceIndexRef,
		&compliancev1.FrameworkCatalog{Frameworks: collectFrameworkDefinitions(request.FrameworkRefs)},
	); err != nil {
		return fmt.Errorf("%w: manifest reference validation: %w", constants.ErrBundleAssemblyFailed, err)
	}
	requiredSourcePaths := append(append([]string{request.AssertionCatalogRef, request.EvidenceIndexRef}, request.CrosswalkRefs...), request.AssessmentRefs...)
	sourcePaths := make(map[string]struct{}, len(request.SourceArtifacts))
	for _, source := range request.SourceArtifacts {
		sourcePaths[source.BundlePath] = struct{}{}
	}
	for _, requiredPath := range requiredSourcePaths {
		if _, exists := sourcePaths[requiredPath]; !exists {
			return fmt.Errorf("%w: manifest-referenced source artifact %s is missing", constants.ErrBundleArtifactMissing, requiredPath)
		}
	}
	if len(request.RenderedFormats) == 0 {
		return fmt.Errorf("%w: at least one rendered format is required", constants.ErrBundleAssemblyFailed)
	}
	if len(request.Profiles) == 0 {
		return fmt.Errorf("%w: at least one framework profile is required", constants.ErrBundleAssemblyFailed)
	}
	if request.Profile == ProfileRestricted {
		for _, restricted := range request.RestrictedArtifacts {
			if restricted.Encryption == nil {
				return fmt.Errorf("%w: restricted artifact %s requires encryption metadata", constants.ErrBundleAssemblyFailed, restricted.BundlePath)
			}
		}
	}
	return nil
}

func addArtifact(artifacts *[]*compliancev1.BundleArtifact, checksums *[]*compliancev1.ChecksumEntry, bundlePath string, body []byte, mediaType, profile string) error {
	return addArtifactDescriptor(artifacts, checksums, bundlePath, body, mediaType, profile, false)
}

func addSourceArtifact(artifacts *[]*compliancev1.BundleArtifact, checksums *[]*compliancev1.ChecksumEntry, bundlePath string, body []byte, mediaType string) error {
	return addArtifactDescriptor(artifacts, checksums, bundlePath, body, mediaType, constants.ComplianceBundleProfilePublic, true)
}

func addArtifactDescriptor(artifacts *[]*compliancev1.BundleArtifact, checksums *[]*compliancev1.ChecksumEntry, bundlePath string, body []byte, mediaType, profile string, allowEmpty bool) error {
	if err := validateBundleArtifactInputs(bundlePath, body, mediaType, allowEmpty); err != nil {
		return err
	}
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	artifact := &compliancev1.BundleArtifact{
		BundlePath: bundlePath,
		Sha256:     digestHex,
		MediaType:  mediaType,
		Profile:    profile,
		ByteLength: int64(len(body)),
	}
	if err := catalog.ValidateBundleArtifact(artifact); err != nil {
		return err
	}
	*artifacts = append(*artifacts, artifact)
	*checksums = append(*checksums, &compliancev1.ChecksumEntry{
		BundlePath: bundlePath,
		Sha256:     digestHex,
	})
	return nil
}

func addRestrictedArtifact(artifacts *[]*compliancev1.BundleArtifact, checksums *[]*compliancev1.ChecksumEntry, restricted RestrictedArtifact) error {
	if err := validateBundleArtifactInputs(restricted.BundlePath, restricted.Body, restricted.MediaType, false); err != nil {
		return err
	}
	if restricted.Encryption.GetAlgorithm() != constants.EvalEvidenceEncryptionAES256GCM {
		return fmt.Errorf("%w: restricted artifact %s uses unsupported encryption algorithm %q", constants.ErrEvidenceEncryptionInvalid, restricted.BundlePath, restricted.Encryption.GetAlgorithm())
	}
	digest := sha256.Sum256(restricted.Body)
	digestHex := hex.EncodeToString(digest[:])
	artifact := &compliancev1.BundleArtifact{
		BundlePath: restricted.BundlePath,
		Sha256:     digestHex,
		MediaType:  restricted.MediaType,
		Profile:    constants.ComplianceBundleProfileRestricted,
		ByteLength: int64(len(restricted.Body)),
		Encryption: proto.Clone(restricted.Encryption).(*compliancev1.EvidenceEncryptionMetadata),
	}
	if err := catalog.ValidateBundleArtifact(artifact); err != nil {
		return err
	}
	*artifacts = append(*artifacts, artifact)
	*checksums = append(*checksums, &compliancev1.ChecksumEntry{
		BundlePath: restricted.BundlePath,
		Sha256:     digestHex,
	})
	return nil
}

func validateBundleArtifactInputs(bundlePath string, body []byte, mediaType string, allowEmpty bool) error {
	if bundlePath == "" {
		return fmt.Errorf("%w: artifact bundle path is empty", constants.ErrBundleAssemblyFailed)
	}
	if !allowEmpty && len(body) == 0 {
		return fmt.Errorf("%w: artifact %s has empty content", constants.ErrBundleArtifactMissing, bundlePath)
	}
	if int64(len(body)) > constants.ComplianceBundleMaxArtifactBytes {
		return fmt.Errorf("%w: artifact %s exceeds size limit", constants.ErrBundleAssemblyFailed, bundlePath)
	}
	if mediaType == "" {
		return fmt.Errorf("%w: artifact %s has empty media type", constants.ErrBundleAssemblyFailed, bundlePath)
	}
	return nil
}

func sortArtifacts(artifacts []*compliancev1.BundleArtifact, checksums []*compliancev1.ChecksumEntry) {
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].GetBundlePath() < artifacts[j].GetBundlePath()
	})
	sort.Slice(checksums, func(i, j int) bool {
		return checksums[i].GetBundlePath() < checksums[j].GetBundlePath()
	})
}

func artifactDescriptorChecksumEntries(artifacts []*compliancev1.BundleArtifact) ([]*compliancev1.ChecksumEntry, error) {
	entries := make([]*compliancev1.ChecksumEntry, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil {
			return nil, fmt.Errorf("%w: bundle artifact descriptor is missing", constants.ErrBundleChecksumRootFailed)
		}
		body, err := compliancev1.MarshalCanonical(artifact)
		if err != nil {
			return nil, fmt.Errorf("%w: canonicalize bundle artifact descriptor: %w", constants.ErrBundleChecksumRootFailed, err)
		}
		digest := sha256.Sum256(body)
		entries = append(entries, &compliancev1.ChecksumEntry{BundlePath: artifact.GetBundlePath(), Sha256: hex.EncodeToString(digest[:])})
	}
	return entries, nil
}

func computeChecksumRoot(checksums []*compliancev1.ChecksumEntry) (string, error) {
	if len(checksums) == 0 {
		return "", fmt.Errorf("%w: no checksum entries", constants.ErrBundleChecksumRootFailed)
	}
	if len(checksums) > constants.ComplianceBundleMaxArtifacts {
		return "", fmt.Errorf("%w: artifact count exceeds limit", constants.ErrBundleChecksumRootFailed)
	}
	seen := make(map[string]struct{}, len(checksums))
	hasher := sha256.New()
	for _, checksum := range checksums {
		if checksum == nil || checksum.BundlePath == "" || checksum.Sha256 == "" {
			return "", fmt.Errorf("%w: incomplete checksum entry", constants.ErrBundleChecksumRootFailed)
		}
		if _, exists := seen[checksum.BundlePath]; exists {
			return "", fmt.Errorf("%w: duplicate bundle path %s", constants.ErrBundleChecksumRootFailed, checksum.BundlePath)
		}
		seen[checksum.BundlePath] = struct{}{}
		hasher.Write([]byte(checksum.BundlePath))
		hasher.Write([]byte{0})
		hasher.Write([]byte(checksum.Sha256))
		hasher.Write([]byte{0})
	}
	digest := hasher.Sum(nil)
	return hex.EncodeToString(digest), nil
}

func canonicalManifestBytes(manifest *compliancev1.ComplianceReportManifest) ([]byte, error) {
	canonical := proto.Clone(manifest).(*compliancev1.ComplianceReportManifest)
	canonical.ManifestSha256 = ""
	canonical.Signature = nil
	return compliancev1.MarshalCanonical(canonical)
}

func cloneVersionedRefs(refs []*compliancev1.VersionedReference) []*compliancev1.VersionedReference {
	cloned := make([]*compliancev1.VersionedReference, 0, len(refs))
	for _, ref := range refs {
		cloned = append(cloned, proto.Clone(ref).(*compliancev1.VersionedReference))
	}
	return cloned
}

func cloneProfiles(profiles []*compliancev1.FrameworkProfile) []*compliancev1.FrameworkProfile {
	cloned := make([]*compliancev1.FrameworkProfile, 0, len(profiles))
	for _, profile := range profiles {
		cloned = append(cloned, proto.Clone(profile).(*compliancev1.FrameworkProfile))
	}
	return cloned
}

func collectFrameworkDefinitions(refs []*compliancev1.VersionedReference) []*compliancev1.FrameworkDefinition {
	definitions := make([]*compliancev1.FrameworkDefinition, 0, len(refs))
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		definitions = append(definitions, &compliancev1.FrameworkDefinition{
			FrameworkId:      ref.GetId(),
			FrameworkVersion: ref.GetVersion(),
		})
	}
	return definitions
}
