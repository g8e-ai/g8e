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
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/pathutil"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const releasePrepareTestDigest = "5f0c3c1f0d5b4e2a9c7d8e6f1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d"

var releasePrepareTestNow = time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC)

type releasePrepareFixture struct {
	repoRoot    string
	projectRoot string
	metadata    string
	privateKey  string
	opts        releasePrepareOptions
	deps        releasePrepareDeps
	calls       map[string][]string
	verifyValid bool
	imageCalls  int
	dbPath      string
	copyCalls   []string
	copyErr     error
}

func newReleasePrepareFixture(t *testing.T) *releasePrepareFixture {
	t.Helper()
	base := testutil.TempDir(t)
	fixture := &releasePrepareFixture{
		repoRoot:    filepath.Join(base, "repo"),
		projectRoot: filepath.Join(base, "runtime"),
		calls:       make(map[string][]string),
		verifyValid: true,
	}
	require.NoError(t, os.MkdirAll(fixture.repoRoot, constants.PermDirStandard))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.repoRoot, constants.ComplianceReleaseVersionFilename), []byte("v2.2.2\n"), constants.PermFilePrivate))
	fixture.metadata = filepath.Join(base, "signing-metadata.json")
	fixture.privateKey = filepath.Join(base, "signing-private-key.hex")
	fixture.opts = releasePrepareOptions{
		projectRoot:       fixture.projectRoot,
		repoRoot:          fixture.repoRoot,
		windowSpan:        constants.ComplianceReleaseDefaultWindowSpan,
		maxRows:           constants.ComplianceOperationalExportDefaultMaxRows,
		organization:      "Example Org",
		gatewayContainer:  constants.ComplianceReleaseDefaultGatewayContainer,
		assessor:          constants.ComplianceReleaseDefaultAssessor,
		signingMetadata:   fixture.metadata,
		signingPrivateKey: fixture.privateKey,
		newKey:            true,
	}
	bundlePath := filepath.Join(fixture.projectRoot, "bundle", "bundle.json")
	fixture.deps = releasePrepareDeps{
		fileSvcFactory: fs.NewRuntimeFileService,
		signingLoader:  loadComplianceReportSigningIdentity,
		gatewayImage: func(context.Context, string) (string, error) {
			fixture.imageCalls++
			return releasePrepareTestDigest, nil
		},
		copyGatewayDB: func(_ context.Context, container, destDir string) (string, error) {
			fixture.copyCalls = append(fixture.copyCalls, container, destDir)
			return fixture.dbPath, fixture.copyErr
		},
		gitRevision:    func(context.Context, string) (string, error) { return strings.Repeat("a", 40), nil },
		newGenerateCmd: func() *cobra.Command { return fixture.fakeCommand("generate", bundlePath+"\n") },
		newVerifyCmd: func() *cobra.Command {
			report, err := compliancev1.MarshalCanonical(&compliancev1.ComplianceVerificationReport{
				ReportId:               "v2.2.2-gateway-operational-public",
				Valid:                  fixture.verifyValid,
				VerifierId:             constants.ComplianceBundleVerifierID,
				VerifierVersion:        constants.ComplianceBundleVerifierVersion,
				ReproducedChecksumRoot: releasePrepareTestDigest,
			})
			require.NoError(t, err)
			return fixture.fakeCommand("verify", string(report)+"\n")
		},
		newProjectCmd: func() *cobra.Command {
			return fixture.fakeCommand("project", "wrote docs/release_notes/v2.2.x/v2.2.2-compliance-evidence.md\n")
		},
		random: rand.Reader,
		now:    func() time.Time { return releasePrepareTestNow },
	}
	return fixture
}

// fakeCommand records the raw arguments an in-process sub-command receives and
// prints canned output, so the orchestration is tested without re-running the
// generate, verify, and projection implementations that have their own tests.
func (f *releasePrepareFixture) fakeCommand(name, output string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			f.calls[name] = append([]string(nil), args...)
			_, err := fmt.Fprint(cmd.OutOrStdout(), output)
			return err
		},
	}
}

func (f *releasePrepareFixture) flagValue(t *testing.T, call, flag string) string {
	t.Helper()
	args, ok := f.calls[call]
	require.True(t, ok, "%s was not run", call)
	for index, arg := range args {
		if arg == flag && index+1 < len(args) {
			return args[index+1]
		}
	}
	t.Fatalf("%s was run without %s: %v", call, flag, args)
	return ""
}

// seedRuntimeDatabase creates the runtime audit database the export reads.
// The schema carries only the columns the read-only snapshot queries.
func (f *releasePrepareFixture) seedRuntimeDatabase(t *testing.T, receiptSigner, auditor string, receiptsAt, commitmentsAt []time.Time) {
	t.Helper()
	fileSvc, err := fs.NewRuntimeFileService(f.projectRoot, testutil.NewTestLogger())
	require.NoError(t, err)
	dbPath := pathutil.ResolveDBPath(fileSvc.Resolve(constants.DataDirname), constants.DbFilename)
	f.dbPath = dbPath
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), constants.PermDirStandard))
	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`
		CREATE TABLE receipts (transaction_id TEXT PRIMARY KEY, executed_at_ms INTEGER NOT NULL, receipt_json TEXT);
		CREATE TABLE commitment_ledger (id INTEGER PRIMARY KEY AUTOINCREMENT, transaction_id TEXT NOT NULL, committed_at_unix_ms INTEGER NOT NULL, attestation_json BLOB);
		CREATE TABLE events (seq INTEGER, prev_hash TEXT, hash TEXT, type TEXT, operator_session_id TEXT, timestamp TEXT, content_digest TEXT, transaction_id TEXT, content_text TEXT, encrypted INTEGER);
	`)
	require.NoError(t, err)
	for index, executedAt := range receiptsAt {
		transactionID := fmt.Sprintf("tx-%d", index)
		body, marshalErr := compliancev1.MarshalCanonical(&operatorv1.ActionReceipt{TransactionId: transactionID, SignerKeyId: receiptSigner})
		require.NoError(t, marshalErr)
		_, err = db.Exec(`INSERT INTO receipts (transaction_id, executed_at_ms, receipt_json) VALUES (?, ?, ?)`, transactionID, executedAt.UnixMilli(), string(body))
		require.NoError(t, err)
	}
	for index, committedAt := range commitmentsAt {
		body, marshalErr := compliancev1.MarshalCanonical(&operatorv1.CommitmentAttestation{AuditorKeyId: auditor})
		require.NoError(t, marshalErr)
		_, err = db.Exec(`INSERT INTO commitment_ledger (transaction_id, committed_at_unix_ms, attestation_json) VALUES (?, ?, ?)`, fmt.Sprintf("tx-%d", index), committedAt.UnixMilli(), body)
		require.NoError(t, err)
	}
}

func releasePrepareSignerKey(t *testing.T) string {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return hex.EncodeToString(publicKey)
}

func TestRunReleasePrepare_DerivesInputsThenGeneratesVerifiesAndProjectsInOrder(t *testing.T) {
	fixture := newReleasePrepareFixture(t)
	receiptSigner, auditor := releasePrepareSignerKey(t), releasePrepareSignerKey(t)
	commitAt := time.Date(2026, 9, 28, 2, 58, 52, 999_000_000, time.UTC)
	fixture.seedRuntimeDatabase(t, receiptSigner, auditor,
		[]time.Time{commitAt.Add(-3 * time.Second), commitAt.Add(-time.Second)}, []time.Time{commitAt.Add(-2 * time.Second), commitAt})

	var stdout, stderr bytes.Buffer
	require.NoError(t, runReleasePrepare(context.Background(), fixture.deps, fixture.opts, &stdout, &stderr))

	workDir := filepath.Join(fixture.projectRoot, constants.ComplianceReleaseWorkDirname, "v2.2.2")
	scope, err := loadComplianceAssessmentScope(filepath.Join(workDir, constants.ComplianceReleaseScopeFilename))
	require.NoError(t, err)
	assert.Equal(t, "2.2.2", scope.GetProductVersion())
	assert.Equal(t, "gateway-operational-20260928", scope.GetScopeId())
	assert.Equal(t, commitAt, scope.GetAssessmentWindowEnd().AsTime(), "the window ends at the newest ledger commitment")
	assert.Equal(t, commitAt.Add(-constants.ComplianceReleaseDefaultWindowSpan), scope.GetAssessmentWindowStart().AsTime())
	assert.Equal(t, releasePrepareTestDigest, scope.GetBuildIdentity(), "build identity defaults to the Gateway image digest")
	assert.Equal(t, strings.Repeat("a", 40), scope.GetSourceRevision())
	assert.Equal(t, 1, fixture.imageCalls)
	assert.Equal(t, []string{constants.ComplianceReleaseDefaultGatewayContainer, filepath.Join(workDir, constants.ComplianceReleaseSnapshotDirname)}, fixture.copyCalls,
		"the database is snapshotted from the Gateway container into the work directory")

	reportTrustBody, err := os.ReadFile(filepath.Join(workDir, constants.ComplianceReleaseReportTrustFilename))
	require.NoError(t, err)
	reportTrust := &compliancev1.ComplianceReportTrustPolicy{}
	require.NoError(t, compliancev1.UnmarshalCanonical(reportTrustBody, reportTrust))
	require.Len(t, reportTrust.GetTrustedKeys(), 1)
	assert.Equal(t, []string{scope.GetScopeId()}, reportTrust.GetTrustedKeys()[0].GetAllowedScopeRefs())
	assert.Equal(t, constants.ComplianceReleaseDefaultAssessor, reportTrust.GetTrustedKeys()[0].GetAssessorIdentity())

	evidenceTrustBody, err := os.ReadFile(filepath.Join(workDir, constants.ComplianceReleaseEvidenceTrustFilename))
	require.NoError(t, err)
	evidenceTrust := &compliancev1.ComplianceEvidenceTrustPolicy{}
	require.NoError(t, compliancev1.UnmarshalCanonical(evidenceTrustBody, evidenceTrust))
	var trusted []string
	for _, key := range evidenceTrust.GetTrustedKeys() {
		trusted = append(trusted, key.GetKeyId())
	}
	assert.ElementsMatch(t, []string{receiptSigner, auditor}, trusted, "source trust covers exactly the signers present in the exported evidence")

	assert.Equal(t, constants.ComplianceBundleProfilePublic, fixture.flagValue(t, "generate", "--profile"))
	assert.Equal(t, filepath.Join(workDir, constants.ComplianceReleaseScopeFilename), fixture.flagValue(t, "generate", "--scope"))
	assert.Equal(t, filepath.Join(workDir, constants.ComplianceReleaseExportDirname), fixture.flagValue(t, "generate", "--source"))
	assert.Equal(t, "v2.2.2-gateway-operational-public", fixture.flagValue(t, "generate", "--report-id"))
	assert.Equal(t, fixture.privateKey, fixture.flagValue(t, "generate", "--signing-private-key"))
	assert.Equal(t, filepath.Join(workDir, constants.ComplianceReleaseReportTrustFilename), fixture.flagValue(t, "verify", "--trust-policy"))
	assert.Equal(t, filepath.Join(workDir, constants.ComplianceReleaseEvidenceTrustFilename), fixture.flagValue(t, "verify", "--evidence-trust"))
	assert.Equal(t, filepath.Join(fixture.repoRoot, "docs", "release_notes", "v2.2.x"), fixture.flagValue(t, "project", "--out"))
	assert.Equal(t, filepath.Join(fixture.projectRoot, "bundle", "bundle.json"), fixture.calls["project"][0], "projection consumes the bundle that generation printed")

	assert.Contains(t, stdout.String(), "valid=true failures=0")
	assert.Contains(t, stderr.String(), "uncommitted release changes are not part of the recorded revision")
	keyBody, err := os.Stat(fixture.privateKey)
	require.NoError(t, err)
	assert.Equal(t, testutil.FileMode(constants.PermFilePrivate, keyBody.IsDir()), keyBody.Mode().Perm(), "the generated private key is owner-only")
}

func TestRunReleasePrepare_StopsBeforeProjectionWhenVerificationFails(t *testing.T) {
	fixture := newReleasePrepareFixture(t)
	commitAt := time.Date(2026, 9, 28, 2, 58, 52, 0, time.UTC)
	fixture.seedRuntimeDatabase(t, releasePrepareSignerKey(t), releasePrepareSignerKey(t), []time.Time{commitAt.Add(-time.Second)}, []time.Time{commitAt})
	fixture.verifyValid = false

	err := runReleasePrepare(context.Background(), fixture.deps, fixture.opts, &bytes.Buffer{}, &bytes.Buffer{})

	require.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	assert.NotContains(t, fixture.calls, "project", "an unverified bundle must never be projected into the release notes")
}

func TestRunReleasePrepare_RejectsWindowWithoutReceiptsBeforeGenerating(t *testing.T) {
	fixture := newReleasePrepareFixture(t)
	commitAt := time.Date(2026, 9, 28, 2, 58, 52, 0, time.UTC)
	fixture.seedRuntimeDatabase(t, releasePrepareSignerKey(t), releasePrepareSignerKey(t), []time.Time{commitAt.Add(-time.Hour)}, []time.Time{commitAt})

	err := runReleasePrepare(context.Background(), fixture.deps, fixture.opts, &bytes.Buffer{}, &bytes.Buffer{})

	require.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Empty(t, fixture.calls, "no bundle is generated from an empty assessment window")
}

func TestRunReleasePrepare_RejectsLoneSigningKeyPath(t *testing.T) {
	fixture := newReleasePrepareFixture(t)
	fixture.opts.signingMetadata = ""

	err := runReleasePrepare(context.Background(), fixture.deps, fixture.opts, &bytes.Buffer{}, &bytes.Buffer{})

	require.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Empty(t, fixture.copyCalls, "no database is copied before the key paths are valid")
}

func TestRunReleasePrepare_DatabaseFlagSkipsGatewaySnapshot(t *testing.T) {
	fixture := newReleasePrepareFixture(t)
	commitAt := time.Date(2026, 9, 28, 2, 58, 52, 0, time.UTC)
	fixture.seedRuntimeDatabase(t, releasePrepareSignerKey(t), releasePrepareSignerKey(t), []time.Time{commitAt.Add(-time.Second)}, []time.Time{commitAt})
	fixture.opts.databasePath = fixture.dbPath
	fixture.dbPath = ""

	require.NoError(t, runReleasePrepare(context.Background(), fixture.deps, fixture.opts, &bytes.Buffer{}, &bytes.Buffer{}))

	assert.Empty(t, fixture.copyCalls)
}

func TestRunReleasePrepare_StopsWhenGatewaySnapshotFails(t *testing.T) {
	fixture := newReleasePrepareFixture(t)
	fixture.copyErr = fmt.Errorf("container is not running")

	err := runReleasePrepare(context.Background(), fixture.deps, fixture.opts, &bytes.Buffer{}, &bytes.Buffer{})

	require.ErrorIs(t, err, fixture.copyErr)
	assert.Empty(t, fixture.calls)
}

func TestResolveSigningKeyPaths(t *testing.T) {
	secretsDir := filepath.Join(testutil.TempDir(t), constants.SecretsDirname)

	t.Run("defaults to the runtime secrets directory", func(t *testing.T) {
		opts := releasePrepareOptions{newKey: true}
		require.NoError(t, resolveSigningKeyPaths(&opts, secretsDir))
		assert.Equal(t, filepath.Join(secretsDir, constants.ComplianceReleaseSigningMetadataFilename), opts.signingMetadata)
		assert.Equal(t, filepath.Join(secretsDir, constants.ComplianceReleaseSigningPrivateFilename), opts.signingPrivateKey)
	})

	t.Run("a missing key names --new-key", func(t *testing.T) {
		err := resolveSigningKeyPaths(&releasePrepareOptions{}, secretsDir)
		require.ErrorIs(t, err, constants.ErrValidationFailed)
		assert.Contains(t, err.Error(), "--new-key")
	})

	t.Run("an existing key is reused without --new-key", func(t *testing.T) {
		require.NoError(t, os.MkdirAll(secretsDir, constants.PermDirPrivate))
		for _, name := range []string{constants.ComplianceReleaseSigningMetadataFilename, constants.ComplianceReleaseSigningPrivateFilename} {
			require.NoError(t, os.WriteFile(filepath.Join(secretsDir, name), []byte("x"), constants.PermFilePrivate))
		}
		require.NoError(t, resolveSigningKeyPaths(&releasePrepareOptions{}, secretsDir))
	})

	t.Run("explicit paths are kept and must be set together", func(t *testing.T) {
		opts := releasePrepareOptions{newKey: true, signingMetadata: "m.json", signingPrivateKey: "k.hex"}
		require.NoError(t, resolveSigningKeyPaths(&opts, secretsDir))
		assert.Equal(t, "m.json", opts.signingMetadata)
		require.ErrorIs(t, resolveSigningKeyPaths(&releasePrepareOptions{signingPrivateKey: "k.hex"}, secretsDir), constants.ErrValidationFailed)
	})
}

func TestWriteNewReleaseSigningKey_CreatesMissingKeyDirectory(t *testing.T) {
	dir := filepath.Join(testutil.TempDir(t), "nested", constants.SecretsDirname)
	opts := releasePrepareOptions{
		signingMetadata:   filepath.Join(dir, constants.ComplianceReleaseSigningMetadataFilename),
		signingPrivateKey: filepath.Join(dir, constants.ComplianceReleaseSigningPrivateFilename),
	}

	require.NoError(t, writeNewReleaseSigningKey(rand.Reader, opts, "v2.2.2", releasePrepareTestNow))

	assert.FileExists(t, opts.signingMetadata)
	assert.FileExists(t, opts.signingPrivateKey)
}

func TestResolveReleasePaths_DefaultsFollowTheVersionFile(t *testing.T) {
	fixture := newReleasePrepareFixture(t)

	prepared, err := resolveReleasePaths(fixture.opts)
	require.NoError(t, err)
	assert.Equal(t, "v2.2.2", prepared.version)
	assert.Equal(t, "v2.2.2-gateway-operational-public", prepared.reportID)
	assert.Equal(t, filepath.Join(fixture.projectRoot, constants.ComplianceReleaseWorkDirname, "v2.2.2"), prepared.workDir)
	assert.Equal(t, filepath.Join(fixture.repoRoot, "docs", "release_notes", "v2.2.x"), prepared.releaseDir)

	require.NoError(t, os.WriteFile(filepath.Join(fixture.repoRoot, constants.ComplianceReleaseVersionFilename), []byte("2.2.2\n"), constants.PermFilePrivate))
	_, err = resolveReleasePaths(fixture.opts)
	require.ErrorIs(t, err, constants.ErrValidationFailed, "VERSION must carry the v prefix")
}

func TestResolveReleaseWindow_ExplicitBoundsMustBeSetTogether(t *testing.T) {
	fixture := newReleasePrepareFixture(t)
	explicit := fixture.opts
	explicit.windowStart, explicit.windowEnd = "2026-09-28T02:58:19Z", "2026-09-28T02:58:52.999Z"
	prepared := &releasePrepareContext{}

	require.NoError(t, resolveReleaseWindow(context.Background(), "", explicit, prepared))
	assert.Equal(t, time.Date(2026, 9, 28, 2, 58, 19, 0, time.UTC), prepared.windowStart)
	assert.Equal(t, time.Date(2026, 9, 28, 2, 58, 52, 999_000_000, time.UTC), prepared.windowEnd)

	for name, mutate := range map[string]func(*releasePrepareOptions){
		"only start":      func(o *releasePrepareOptions) { o.windowEnd = "" },
		"only end":        func(o *releasePrepareOptions) { o.windowStart = "" },
		"inverted":        func(o *releasePrepareOptions) { o.windowStart, o.windowEnd = o.windowEnd, o.windowStart },
		"malformed start": func(o *releasePrepareOptions) { o.windowStart = "yesterday" },
		"malformed end":   func(o *releasePrepareOptions) { o.windowEnd = "today" },
	} {
		t.Run(name, func(t *testing.T) {
			options := explicit
			mutate(&options)
			err := resolveReleaseWindow(context.Background(), "", options, &releasePrepareContext{})
			require.ErrorIs(t, err, constants.ErrValidationFailed)
		})
	}
}

func TestResolveReleaseIdentity_QueriesRuntimeOnlyForFactsTheOperatorOmitted(t *testing.T) {
	fixture := newReleasePrepareFixture(t)
	prepared := &releasePrepareContext{repoRoot: fixture.repoRoot, windowEnd: releasePrepareTestNow}

	explicit := fixture.opts
	explicit.gatewayImageDigest = releasePrepareTestDigest
	explicit.sourceRevision = strings.Repeat("b", 40)
	require.NoError(t, resolveReleaseIdentity(context.Background(), fixture.deps, &explicit, prepared))
	assert.Zero(t, fixture.imageCalls, "an explicit digest must not inspect the container")
	assert.Equal(t, strings.Repeat("b", 40), explicit.sourceRevision)
	assert.Equal(t, releasePrepareTestDigest, explicit.buildIdentity)
	assert.Equal(t, "g8e-unified-compose-20260928", explicit.deploymentID)

	invalid := fixture.opts
	invalid.gatewayImageDigest = "sha256:not-hex"
	require.ErrorIs(t, resolveReleaseIdentity(context.Background(), fixture.deps, &invalid, prepared), constants.ErrValidationFailed)
}

func TestWriteNewReleaseSigningKey_RefusesToOverwriteAnExistingIdentity(t *testing.T) {
	fixture := newReleasePrepareFixture(t)
	require.NoError(t, writeNewReleaseSigningKey(rand.Reader, fixture.opts, "v2.2.2", releasePrepareTestNow))

	identity, err := loadComplianceReportSigningIdentity(context.Background(), fixture.metadata, fixture.privateKey)
	require.NoError(t, err)
	assert.Equal(t, "v2.2.2"+constants.ComplianceReleaseSigningKeyIDSuffix, identity.Metadata().GetKeyId())

	err = writeNewReleaseSigningKey(rand.Reader, fixture.opts, "v2.2.2", releasePrepareTestNow)
	require.ErrorIs(t, err, constants.ErrValidationFailed, "a release must not silently rotate its report-signing identity")
}

func TestRequireSigningKeyValidAt_RejectsExpiredAndNotYetValidKeys(t *testing.T) {
	fixture := newReleasePrepareFixture(t)
	require.NoError(t, writeNewReleaseSigningKey(rand.Reader, fixture.opts, "v2.2.2", releasePrepareTestNow))
	identity, err := loadComplianceReportSigningIdentity(context.Background(), fixture.metadata, fixture.privateKey)
	require.NoError(t, err)

	require.NoError(t, requireSigningKeyValidAt(identity, releasePrepareTestNow.Add(time.Hour)))
	require.ErrorIs(t, requireSigningKeyValidAt(identity, releasePrepareTestNow.Add(constants.ComplianceReleaseNewKeyLifetime+time.Hour)), constants.ErrReportSignatureFailed)
	require.ErrorIs(t, requireSigningKeyValidAt(identity, releasePrepareTestNow.Add(-time.Hour)), constants.ErrReportSignatureFailed)
}
