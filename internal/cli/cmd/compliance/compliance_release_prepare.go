// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliancecmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancereport "github.com/g8e-ai/g8e/v2/internal/services/compliance/report"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

var sha256HexRegexp = regexp.MustCompile(`^[0-9a-f]{64}$`)

// releasePrepareDeps are the external facts and sub-commands release
// preparation composes. They are injected so tests exercise the sequence
// without Docker, git, or a real clock.
type releasePrepareDeps struct {
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)
	signingLoader  complianceReportSigningIdentityLoader
	gatewayImage   func(ctx context.Context, container string) (string, error)
	copyGatewayDB  func(ctx context.Context, container, destDir string) (string, error)
	gitRevision    func(ctx context.Context, repoRoot string) (string, error)
	newGenerateCmd func() *cobra.Command
	newVerifyCmd   func() *cobra.Command
	newProjectCmd  func() *cobra.Command
	random         io.Reader
	now            func() time.Time
}

// releasePrepareOptions are the operator-facing inputs to release preparation.
type releasePrepareOptions struct {
	projectRoot        string
	repoRoot           string
	workDir            string
	releaseDir         string
	databasePath       string
	reportID           string
	windowStart        string
	windowEnd          string
	windowSpan         time.Duration
	maxRows            int
	organization       string
	deploymentID       string
	gatewayContainer   string
	gatewayImageDigest string
	buildIdentity      string
	sourceRevision     string
	signingMetadata    string
	signingPrivateKey  string
	newKey             bool
	assessor           string
}

func complianceReleasePrepareCmdWithConfig(deps releasePrepareDeps) *cobra.Command {
	var opts releasePrepareOptions
	cmd := &cobra.Command{
		Use:   "release-prepare",
		Short: "Prepare, generate, verify, and project release compliance evidence",
		Long: `Prepare the compliance evidence for the release named by VERSION.

Run it with no flags against a running Gateway built from the release HEAD.
The first run needs --new-key to create the report-signing key; later runs
reuse it. Run from the repository root.

The command derives the protected assessment scope from VERSION, git, the
running Gateway image, and the assessment window; copies the Gateway database
out of the container (or reads --db); exports the bounded
operational evidence for that window from it; builds the
external report and source-evidence trust policies from the report-signing
identity and the signer keys actually present in the exported evidence;
generates a public signed bundle; verifies it offline against those trust
policies; and projects the verified bundle into docs/release_notes/vX.Y.x/.

The trust policies record a first-party engineering assessment. They are
written outside the bundle and never confer external attestation. Generated
scope, trust policies, the database snapshot, and the source export stay under
--work-dir; the report-signing private key is read from --signing-private-key
(default: the runtime secrets directory) and is never copied. The AI coding
agent runs this command; it does not commit or tag.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runReleasePrepare(ctx, deps, opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.projectRoot, "project-root", "", "Runtime root holding .g8e/data/g8e.db and receiving the bundle (defaults to cwd)")
	flags.StringVar(&opts.repoRoot, "repo-root", "", "Repository root holding VERSION and docs/release_notes (defaults to cwd)")
	flags.StringVar(&opts.workDir, "work-dir", "", "Directory for the scope, trust policies, and source export (defaults to <project-root>/release-evidence/<version>)")
	flags.StringVar(&opts.releaseDir, "release-dir", "", "Release notes directory receiving the projections (defaults to <repo-root>/docs/release_notes/vX.Y.x)")
	flags.StringVar(&opts.databasePath, "db", "", "Gateway g8e.db to assess (defaults to a snapshot copied from --gateway-container into --work-dir)")
	flags.StringVar(&opts.reportID, "report-id", "", "Immutable report bundle ID (defaults to <version>-gateway-operational-public)")
	flags.StringVar(&opts.windowStart, "window-start", "", "Assessment window start, RFC 3339 (set with --window-end, or omit both to end the window at the newest ledger commitment)")
	flags.StringVar(&opts.windowEnd, "window-end", "", "Assessment window end, RFC 3339 (set with --window-start, or omit both to end the window at the newest ledger commitment)")
	flags.DurationVar(&opts.windowSpan, "window-span", constants.ComplianceReleaseDefaultWindowSpan, "Window length used when the window is selected automatically")
	flags.IntVar(&opts.maxRows, "max-rows", constants.ComplianceOperationalExportDefaultMaxRows, "Maximum receipts and commitments to export")
	flags.StringVar(&opts.organization, "organization", constants.ComplianceReleaseDefaultOrganization, "Organization recorded in the assessment scope")
	flags.StringVar(&opts.deploymentID, "deployment-id", "", "Deployment recorded in the assessment scope (defaults to "+constants.ComplianceReleaseDeploymentIDPrefix+"<window date>)")
	flags.StringVar(&opts.gatewayContainer, "gateway-container", constants.ComplianceReleaseDefaultGatewayContainer, "Running Gateway container whose image digest identifies the assessed build")
	flags.StringVar(&opts.gatewayImageDigest, "gateway-image-digest", "", "Gateway image SHA-256 digest (defaults to the --gateway-container image)")
	flags.StringVar(&opts.buildIdentity, "build-identity", "", "Build identity SHA-256 digest (defaults to the Gateway image digest)")
	flags.StringVar(&opts.sourceRevision, "source-revision", "", "Source revision (defaults to git HEAD of --repo-root)")
	flags.StringVar(&opts.signingMetadata, "signing-metadata", "", "Path to canonical report signing-key metadata (defaults to the runtime secrets directory; set with --signing-private-key)")
	flags.StringVar(&opts.signingPrivateKey, "signing-private-key", "", "Path to hex-encoded Ed25519 report private key (defaults to the runtime secrets directory; set with --signing-metadata)")
	flags.BoolVar(&opts.newKey, "new-key", false, "Generate the report-signing key at the signing key paths; refuses to overwrite")
	flags.StringVar(&opts.assessor, "assessor", constants.ComplianceReleaseDefaultAssessor, "Assessor identity recorded in the first-party trust policies")
	return cmd
}

// releasePrepareContext is the resolved, validated state shared by the
// preparation steps.
type releasePrepareContext struct {
	version     string
	repoRoot    string
	projectRoot string
	workDir     string
	releaseDir  string
	reportID    string
	scopePath   string
	reportTrust string
	sourceTrust string
	exportDir   string
	windowStart time.Time
	windowEnd   time.Time
}

func runReleasePrepare(ctx context.Context, deps releasePrepareDeps, opts releasePrepareOptions, stdout, stderr io.Writer) error {
	if opts.maxRows <= 0 || opts.windowSpan <= 0 {
		return fmt.Errorf("%w: --max-rows and --window-span must be positive", constants.ErrValidationFailed)
	}
	now := deps.now().UTC()
	prepared, err := resolveReleasePaths(opts)
	if err != nil {
		return err
	}
	fileSvc, err := deps.fileSvcFactory(prepared.projectRoot, slog.Default())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	if err := resolveSigningKeyPaths(&opts, fileSvc.Resolve(constants.SecretsDirname)); err != nil {
		return err
	}
	if err := os.MkdirAll(prepared.workDir, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("%w: create release work directory: %w", constants.ErrDirCreateFailed, err)
	}
	dbPath := opts.databasePath
	if dbPath == "" {
		if dbPath, err = deps.copyGatewayDB(ctx, opts.gatewayContainer, filepath.Join(prepared.workDir, constants.ComplianceReleaseSnapshotDirname)); err != nil {
			return err
		}
	}
	if err := resolveReleaseWindow(ctx, dbPath, opts, prepared); err != nil {
		return err
	}
	if err := resolveReleaseIdentity(ctx, deps, &opts, prepared); err != nil {
		return err
	}
	if opts.newKey {
		if err := writeNewReleaseSigningKey(deps.random, opts, prepared.version, now); err != nil {
			return err
		}
	}
	identity, err := deps.signingLoader(ctx, opts.signingMetadata, opts.signingPrivateKey)
	if err != nil {
		return err
	}
	if err := requireSigningKeyValidAt(identity, now); err != nil {
		return err
	}

	scope, err := compliancereport.BuildReleaseAssessmentScope(compliancereport.ReleaseScopeRequest{
		ScopeID:            prepared.scopeID(),
		OrganizationID:     opts.organization,
		DeploymentID:       opts.deploymentID,
		ProductVersion:     prepared.version,
		BuildIdentity:      opts.buildIdentity,
		SourceRevision:     opts.sourceRevision,
		GatewayImageDigest: opts.gatewayImageDigest,
		GatewayVersion:     prepared.version,
		WindowStart:        prepared.windowStart,
		WindowEnd:          prepared.windowEnd,
	})
	if err != nil {
		return err
	}
	if err := writeCanonicalReleaseInput(prepared.scopePath, scope); err != nil {
		return err
	}
	inventory, snapshot, err := exportOperationalEvidence(ctx, dbPath, scope, prepared.exportDir, opts.maxRows)
	if err != nil {
		return err
	}
	if inventory.ReceiptCount == 0 || inventory.CommitmentCount == 0 {
		return fmt.Errorf("%w: assessment window %s to %s holds %d receipts and %d commitments; select a window with governed activity", constants.ErrValidationFailed, prepared.windowStart.Format(time.RFC3339Nano), prepared.windowEnd.Format(time.RFC3339Nano), inventory.ReceiptCount, inventory.CommitmentCount)
	}
	signerKeys, err := compliancereport.CollectOperationalSignerKeys(snapshot)
	if err != nil {
		return err
	}
	trust := compliancereport.ReleaseTrustRequest{
		AssessmentID:     prepared.version + constants.ComplianceReleaseAssessmentIDSuffix,
		AssessorIdentity: opts.assessor,
		ScopeID:          scope.GetScopeId(),
		AssessedAt:       prepared.windowEnd,
	}
	reportTrustRequest := trust
	reportTrustRequest.PolicyID = prepared.version + constants.ComplianceReleaseReportPolicyIDSuffix
	reportTrust, err := compliancereport.BuildReleaseReportTrustPolicy(reportTrustRequest, identity)
	if err != nil {
		return err
	}
	evidenceTrustRequest := trust
	evidenceTrustRequest.PolicyID = prepared.version + constants.ComplianceReleaseEvidencePolicyIDSuffix
	evidenceTrust, err := compliancereport.BuildReleaseEvidenceTrustPolicy(evidenceTrustRequest, signerKeys)
	if err != nil {
		return err
	}
	if err := writeCanonicalReleaseInput(prepared.reportTrust, reportTrust); err != nil {
		return err
	}
	if err := writeCanonicalReleaseInput(prepared.sourceTrust, evidenceTrust); err != nil {
		return err
	}

	generated, err := runComplianceSubcommand(ctx, deps.newGenerateCmd(), []string{
		"--project-root", prepared.projectRoot,
		"--scope", prepared.scopePath,
		"--source", prepared.exportDir,
		"--report-id", prepared.reportID,
		"--profile", constants.ComplianceBundleProfilePublic,
		"--signing-metadata", opts.signingMetadata,
		"--signing-private-key", opts.signingPrivateKey,
		"--evidence-trust", prepared.sourceTrust,
	})
	if err != nil {
		return fmt.Errorf("release-prepare: generate bundle: %w", err)
	}
	bundle := strings.TrimSpace(generated)
	verified, err := runComplianceSubcommand(ctx, deps.newVerifyCmd(), []string{
		bundle,
		"--trust-policy", prepared.reportTrust,
		"--evidence-trust", prepared.sourceTrust,
	})
	if err != nil {
		return fmt.Errorf("release-prepare: verify bundle: %w", err)
	}
	report := &compliancev1.ComplianceVerificationReport{}
	if err := compliancev1.UnmarshalCanonical([]byte(strings.TrimSpace(verified)), report); err != nil {
		return fmt.Errorf("%w: decode verification report: %w", constants.ErrReportVerificationFailed, err)
	}
	if !report.GetValid() || len(report.GetFailures()) != 0 {
		return fmt.Errorf("%w: verification reported %d failures", constants.ErrReportVerificationFailed, len(report.GetFailures()))
	}
	projected, err := runComplianceSubcommand(ctx, deps.newProjectCmd(), []string{
		bundle,
		"--trust-policy", prepared.reportTrust,
		"--evidence-trust", prepared.sourceTrust,
		"--out", prepared.releaseDir,
	})
	if err != nil {
		return fmt.Errorf("release-prepare: project release evidence: %w", err)
	}

	if _, err := fmt.Fprint(stdout, projected); err != nil {
		return fmt.Errorf("release-prepare: write projection paths: %w", err)
	}
	if _, err := fmt.Fprintf(stdout, "%s\n", releasePrepareSummary(prepared, scope, inventory, signerKeys, bundle, report)); err != nil {
		return fmt.Errorf("release-prepare: write summary: %w", err)
	}
	if _, err := fmt.Fprintf(stderr, "note: source revision %s is the git HEAD of the release branch; uncommitted release changes are not part of the recorded revision\n", opts.sourceRevision); err != nil {
		return fmt.Errorf("release-prepare: write note: %w", err)
	}
	return nil
}

func (c *releasePrepareContext) scopeID() string {
	return constants.ComplianceReleaseScopeIDPrefix + c.windowEnd.Format(constants.ComplianceReleaseScopeDateLayout)
}

// resolveReleasePaths reads VERSION and resolves the directory layout. It does
// not touch the runtime database or create anything.
func resolveReleasePaths(opts releasePrepareOptions) (*releasePrepareContext, error) {
	repoRoot, err := absoluteOrWorkingDir(opts.repoRoot)
	if err != nil {
		return nil, err
	}
	projectRoot, err := absoluteOrWorkingDir(opts.projectRoot)
	if err != nil {
		return nil, err
	}
	versionBody, err := os.ReadFile(filepath.Join(repoRoot, constants.ComplianceReleaseVersionFilename))
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", constants.ErrValidationFailed, constants.ComplianceReleaseVersionFilename, err)
	}
	version := strings.TrimSpace(string(versionBody))
	if err := validateReleaseVersion(version); err != nil {
		return nil, err
	}
	prepared := &releasePrepareContext{version: version, repoRoot: repoRoot, projectRoot: projectRoot}
	prepared.workDir = opts.workDir
	if prepared.workDir == "" {
		prepared.workDir = filepath.Join(projectRoot, constants.ComplianceReleaseWorkDirname, version)
	}
	prepared.releaseDir = opts.releaseDir
	if prepared.releaseDir == "" {
		minor := version[:strings.LastIndex(version, ".")]
		prepared.releaseDir = filepath.Join(repoRoot, "docs", "release_notes", minor+".x")
	}
	prepared.reportID = opts.reportID
	if prepared.reportID == "" {
		prepared.reportID = version + constants.ComplianceReleaseReportIDSuffix
	}
	prepared.scopePath = filepath.Join(prepared.workDir, constants.ComplianceReleaseScopeFilename)
	prepared.reportTrust = filepath.Join(prepared.workDir, constants.ComplianceReleaseReportTrustFilename)
	prepared.sourceTrust = filepath.Join(prepared.workDir, constants.ComplianceReleaseEvidenceTrustFilename)
	prepared.exportDir = filepath.Join(prepared.workDir, constants.ComplianceReleaseExportDirname)
	return prepared, nil
}

func absoluteOrWorkingDir(path string) (string, error) {
	if path == "" {
		working, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("release-prepare: get working directory: %w", err)
		}
		return working, nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("release-prepare: resolve %s: %w", path, err)
	}
	return absolute, nil
}

// resolveReleaseWindow fixes the assessment window from explicit flags or, when
// both are absent, from the newest commitment in the runtime ledger.
func resolveReleaseWindow(ctx context.Context, dbPath string, opts releasePrepareOptions, prepared *releasePrepareContext) error {
	switch {
	case opts.windowStart == "" && opts.windowEnd == "":
		reader, err := storage.OpenReadOnlyOperationalEvidence(dbPath, slog.Default())
		if err != nil {
			return err
		}
		end, windowErr := reader.LatestCommitmentAt(ctx)
		if closeErr := reader.Close(); windowErr == nil && closeErr != nil {
			windowErr = fmt.Errorf("release-prepare: close runtime database: %w", closeErr)
		}
		if windowErr != nil {
			return windowErr
		}
		prepared.windowStart, prepared.windowEnd = end.Add(-opts.windowSpan), end
	case opts.windowStart != "" && opts.windowEnd != "":
		start, err := time.Parse(time.RFC3339Nano, opts.windowStart)
		if err != nil {
			return fmt.Errorf("%w: --window-start must be RFC 3339: %w", constants.ErrValidationFailed, err)
		}
		end, err := time.Parse(time.RFC3339Nano, opts.windowEnd)
		if err != nil {
			return fmt.Errorf("%w: --window-end must be RFC 3339: %w", constants.ErrValidationFailed, err)
		}
		prepared.windowStart, prepared.windowEnd = start.UTC(), end.UTC()
	default:
		return fmt.Errorf("%w: --window-start and --window-end must be set together", constants.ErrValidationFailed)
	}
	if !prepared.windowStart.Before(prepared.windowEnd) {
		return fmt.Errorf("%w: assessment window must have a start before its end", constants.ErrValidationFailed)
	}
	return nil
}

// resolveReleaseIdentity fills the build facts the operator did not supply from
// the running Gateway image and git.
func resolveReleaseIdentity(ctx context.Context, deps releasePrepareDeps, opts *releasePrepareOptions, prepared *releasePrepareContext) error {
	if opts.gatewayImageDigest == "" {
		digest, err := deps.gatewayImage(ctx, opts.gatewayContainer)
		if err != nil {
			return err
		}
		opts.gatewayImageDigest = digest
	}
	if !sha256HexRegexp.MatchString(opts.gatewayImageDigest) {
		return fmt.Errorf("%w: Gateway image digest must be lowercase SHA-256 hex", constants.ErrValidationFailed)
	}
	if opts.buildIdentity == "" {
		opts.buildIdentity = opts.gatewayImageDigest
	}
	if !sha256HexRegexp.MatchString(opts.buildIdentity) {
		return fmt.Errorf("%w: build identity must be lowercase SHA-256 hex", constants.ErrValidationFailed)
	}
	if opts.sourceRevision == "" {
		revision, err := deps.gitRevision(ctx, prepared.repoRoot)
		if err != nil {
			return err
		}
		opts.sourceRevision = revision
	}
	if opts.deploymentID == "" {
		opts.deploymentID = constants.ComplianceReleaseDeploymentIDPrefix + prepared.windowEnd.Format(constants.ComplianceReleaseScopeDateLayout)
	}
	return nil
}

// resolveSigningKeyPaths fills the report-signing key paths from the runtime
// secrets directory when neither flag is set, and rejects a lone path. Without
// --new-key both files must already exist, so a missing key is reported with
// the command that creates it instead of a bare open error.
func resolveSigningKeyPaths(opts *releasePrepareOptions, secretsDir string) error {
	opts.signingMetadata = strings.TrimSpace(opts.signingMetadata)
	opts.signingPrivateKey = strings.TrimSpace(opts.signingPrivateKey)
	switch {
	case opts.signingMetadata == "" && opts.signingPrivateKey == "":
		opts.signingMetadata = filepath.Join(secretsDir, constants.ComplianceReleaseSigningMetadataFilename)
		opts.signingPrivateKey = filepath.Join(secretsDir, constants.ComplianceReleaseSigningPrivateFilename)
	case opts.signingMetadata == "" || opts.signingPrivateKey == "":
		return fmt.Errorf("%w: set --signing-metadata and --signing-private-key together, or neither to use the runtime secrets directory", constants.ErrValidationFailed)
	}
	if opts.newKey {
		return nil
	}
	for _, path := range []string{opts.signingMetadata, opts.signingPrivateKey} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("%w: report signing key file %s does not exist; run once with --new-key to create it", constants.ErrValidationFailed, path)
		}
	}
	return nil
}

// writeNewReleaseSigningKey creates the report-signing key. It refuses to
// overwrite an existing key so a release cannot silently rotate its identity.
func writeNewReleaseSigningKey(random io.Reader, opts releasePrepareOptions, version string, now time.Time) error {
	for _, path := range []string{opts.signingMetadata, opts.signingPrivateKey} {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%w: refusing to overwrite existing signing key file %s", constants.ErrValidationFailed, path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("release-prepare: inspect signing key file %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(path), constants.PermDirPrivate); err != nil {
			return fmt.Errorf("%w: create signing key directory: %w", constants.ErrDirCreateFailed, err)
		}
	}
	metadata, privateKey, err := compliancereport.GenerateReportSigningKey(random, version+constants.ComplianceReleaseSigningKeyIDSuffix, now, constants.ComplianceReleaseNewKeyLifetime)
	if err != nil {
		return err
	}
	if err := writeCanonicalReleaseInput(opts.signingMetadata, metadata); err != nil {
		return err
	}
	if err := os.WriteFile(opts.signingPrivateKey, []byte(privateKey+"\n"), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: write signing key: %w", constants.ErrFileWriteFailed, err)
	}
	return nil
}

func requireSigningKeyValidAt(identity *compliancereport.ComplianceReportSigningIdentity, at time.Time) error {
	metadata := identity.Metadata()
	if metadata == nil || at.Before(metadata.GetCreatedAt().AsTime()) || at.After(metadata.GetExpiresAt().AsTime()) {
		return fmt.Errorf("%w: report signing key is outside its validity interval; pass --new-key with new key paths", constants.ErrReportSignatureFailed)
	}
	return nil
}

func writeCanonicalReleaseInput(path string, message proto.Message) error {
	body, err := compliancev1.MarshalCanonical(message)
	if err != nil {
		return fmt.Errorf("release-prepare: canonicalize %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, body, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: write %s: %w", constants.ErrFileWriteFailed, path, err)
	}
	return nil
}

// runComplianceSubcommand executes an existing compliance command in process
// and returns its standard output. Failures keep the command's own error.
func runComplianceSubcommand(ctx context.Context, cmd *cobra.Command, args []string) (string, error) {
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(ctx); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func releasePrepareSummary(prepared *releasePrepareContext, scope *compliancev1.AssessmentScope, inventory *evidence.OperationalSourceInventory, signerKeys []string, bundle string, report *compliancev1.ComplianceVerificationReport) string {
	return fmt.Sprintf(`release evidence prepared for %s
  scope:        %s (window %s to %s, %d receipts, %d commitments, %d source signer keys)
  bundle:       %s
  verification: %s@%s valid=%t failures=%d checksum root %s
  work dir:     %s`,
		prepared.version, scope.GetScopeId(), prepared.windowStart.Format(time.RFC3339Nano), prepared.windowEnd.Format(time.RFC3339Nano), inventory.ReceiptCount, inventory.CommitmentCount, len(signerKeys),
		bundle, report.GetVerifierId(), report.GetVerifierVersion(), report.GetValid(), len(report.GetFailures()), report.GetReproducedChecksumRoot(), prepared.workDir)
}

// defaultGatewayImageDigest reads the SHA-256 image ID of a running container.
func defaultGatewayImageDigest(ctx context.Context, container string) (string, error) {
	output, err := exec.CommandContext(ctx, constants.DockerExecutable, "inspect", "--format", "{{.Image}}", container).Output()
	if err != nil {
		return "", fmt.Errorf("%w: inspect Gateway container %q: %w", constants.ErrValidationFailed, container, err)
	}
	return strings.TrimPrefix(strings.TrimSpace(string(output)), "sha256:"), nil
}

// defaultCopyGatewayDB copies the Gateway audit database out of the running
// container into destDir and returns the copied database path. The database
// is required; its -wal and -shm companions are copied when the container has
// them. destDir is emptied first so a stale companion from an earlier run can
// never be paired with a fresh database.
func defaultCopyGatewayDB(ctx context.Context, container, destDir string) (string, error) {
	if err := os.RemoveAll(destDir); err != nil {
		return "", fmt.Errorf("%w: clear Gateway snapshot directory: %w", constants.ErrFileWriteFailed, err)
	}
	if err := os.MkdirAll(destDir, constants.PermDirPrivate); err != nil {
		return "", fmt.Errorf("%w: create Gateway snapshot directory: %w", constants.ErrDirCreateFailed, err)
	}
	dbPath := filepath.Join(destDir, constants.DbFilename)
	for _, suffix := range []string{"", constants.SQLiteWALSuffix, constants.SQLiteSHMSuffix} {
		source := container + ":" + constants.ContainerAuditVaultDB + suffix
		output, err := exec.CommandContext(ctx, constants.DockerExecutable, "cp", source, dbPath+suffix).CombinedOutput()
		switch {
		case err == nil:
		case suffix == "":
			return "", fmt.Errorf("%w: copy Gateway database from container %q (is it running? use --db to assess a local copy): %w: %s", constants.ErrValidationFailed, container, err, strings.TrimSpace(string(output)))
		default:
			slog.Default().Debug("compliance: Gateway database companion not copied", "file", constants.DbFilename+suffix, "error", err)
		}
	}
	return dbPath, nil
}

// defaultGitRevision reads the checked-out commit of the repository.
func defaultGitRevision(ctx context.Context, repoRoot string) (string, error) {
	output, err := exec.CommandContext(ctx, constants.ComplianceReleaseGitExecutable, "-C", repoRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("%w: read git revision of %s: %w", constants.ErrValidationFailed, repoRoot, err)
	}
	revision := strings.TrimSpace(string(output))
	if _, err := hex.DecodeString(revision); err != nil {
		return "", fmt.Errorf("%w: git revision %q is not hex", constants.ErrValidationFailed, revision)
	}
	return revision, nil
}

func newReleasePrepareDeps() releasePrepareDeps {
	return releasePrepareDeps{
		fileSvcFactory: shared.NewFileSvc,
		signingLoader:  loadComplianceReportSigningIdentity,
		gatewayImage:   defaultGatewayImageDigest,
		copyGatewayDB:  defaultCopyGatewayDB,
		gitRevision:    defaultGitRevision,
		newGenerateCmd: func() *cobra.Command {
			return complianceReportGenerateCmdWithConfig(shared.NewFileSvc, defaultProvenanceSourceFactory, loadComplianceReportSigningIdentity, time.Now)
		},
		newVerifyCmd: func() *cobra.Command {
			return complianceReportVerifyCmdWithConfig(loadComplianceReportBundleInput, compliancereport.VerifyComplianceReportBundle, time.Now)
		},
		newProjectCmd: func() *cobra.Command {
			return complianceReleaseEvidenceProjectionCmdWithConfig(loadComplianceReportBundleInput, compliancereport.VerifyComplianceReportBundle, time.Now)
		},
		random: rand.Reader,
		now:    time.Now,
	}
}
