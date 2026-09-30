// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// fakeWorkerEnrolls stands in for the installed g8e binary: it prints the same
// two records the real worker writes to start.log, keyed by its directory so
// each Operator gets a distinct request and session ID.
const fakeWorkerEnrolls = `#!/bin/sh
id=$(printf %s "$PWD" | md5sum | cut -c1-32)
printf 'Approve with: g8e auth enroll approve %s-000\n' "$id"
printf 'operator enrollment: completed\n  - operator_session_id: %s-111\n' "$id"
`

// fakeWorkerAlreadyEnrolled is a redeploy over an Operator that already holds
// issued credentials: no request is submitted, the session is logged directly.
const fakeWorkerAlreadyEnrolled = `#!/bin/sh
id=$(printf %s "$PWD" | md5sum | cut -c1-32)
printf 'OperatorSession created\n  - operator_session_id: %s-111\n' "$id"
`

const fakeWorkerRejected = `#!/bin/sh
printf 'Enrollment failed: HTTP 429\n'
`

// useFakeSSH puts ssh and scp on PATH that run the remote command, and copy the
// file, on this machine, and makes scp install worker instead of the test
// binary. The remote shell commands deploy sends therefore run for real.
func useFakeSSH(t *testing.T, worker string) {
	t.Helper()
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "ssh"), []byte("#!/bin/sh\nfor a; do cmd=\"$a\"; done\nexec sh -c \"$cmd\"\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "scp"), []byte("#!/bin/sh\nfor a; do dst=\"${a#*:}\"; done\ncp \"$FAKE_WORKER\" \"$dst\"\n"), 0o755))
	workerPath := filepath.Join(binDir, "worker")
	require.NoError(t, os.WriteFile(workerPath, []byte(worker), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_WORKER", workerPath)
}

func deployedDirID(dir string) string {
	sum := md5.Sum([]byte(dir))
	return hex.EncodeToString(sum[:])
}

func runOperatorDeploy(t *testing.T, client authcmd.APIClient, args ...string) (string, error) {
	t.Helper()
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	cmd := operatorDeployCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) { return client, nil },
		cmdtest.FileSvcFactoryFor(fileSvc),
	)
	cmd.Flags().StringP("endpoint", "e", "", "")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	err := cmd.ExecuteContext(ctx)
	return buf.String(), err
}

func TestOperatorDeployDirs(t *testing.T) {
	tests := []struct {
		name      string
		remoteDir string
		count     int
		want      []string
	}{
		{"one operator uses the remote directory itself", "~/g8e", 1, []string{"~/g8e"}},
		{"several operators get numbered subdirectories", "/opt/fleet", 3, []string{"/opt/fleet/op-00001", "/opt/fleet/op-00002", "/opt/fleet/op-00003"}},
		{"trailing slash does not double up", "/opt/fleet/", 2, []string{"/opt/fleet/op-00001", "/opt/fleet/op-00002"}},
		{"home directory", "~", 2, []string{"~/op-00001", "~/op-00002"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, operatorDeployDirs(tt.remoteDir, tt.count))
		})
	}
}

func TestOperatorDeployRejectsInvalidFlagCombinations(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"approve requires background", []string{"--hosts", "h", "--approve"}},
		{"count must be positive", []string{"--hosts", "h", "--count", "0"}},
		{"background requires endpoint", []string{"--hosts", "h", "--background"}},
		{"hosts are required", []string{"--count", "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runOperatorDeploy(t, &cmdtest.MockAPIClient{}, tt.args...)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
		})
	}
}

func TestOperatorDeployInstallsBinaryWithoutStartingIt(t *testing.T) {
	useFakeSSH(t, fakeWorkerEnrolls)
	remoteDir := filepath.Join(t.TempDir(), "fleet")

	out, err := runOperatorDeploy(t, &cmdtest.MockAPIClient{}, "--hosts", "localhost", "--remote-dir", remoteDir)
	require.NoError(t, err, out)

	info, err := os.Stat(filepath.Join(remoteDir, "g8e"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o111, "installed binary must be executable")
	assert.NoFileExists(t, filepath.Join(remoteDir, "g8e.new"), "staging file must be renamed into place")
	assert.NoFileExists(t, filepath.Join(remoteDir, operatorDeployStartLog))
}

func TestOperatorDeployApprovesEachOperatorAndReportsSessions(t *testing.T) {
	useFakeSSH(t, fakeWorkerEnrolls)
	remoteDir := filepath.Join(t.TempDir(), "fleet")
	dirs := operatorDeployDirs(remoteDir, 2)

	sessions := make([]models.OperatorDocumentGo, len(dirs))
	for i, dir := range dirs {
		sessions[i] = models.OperatorDocumentGo{
			OperatorSessionID: deployedDirID(dir) + "-111",
			Status:            constants.OperatorStatusActive,
		}
	}
	listBody, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: sessions})
	require.NoError(t, err)
	decisionBody, err := json.Marshal(models.PlatformEnrollmentDecisionResponse{State: models.PlatformEnrollmentStateApproved})
	require.NoError(t, err)
	client := &cmdtest.MockAPIClient{GetResp: listBody, PostResp: decisionBody}

	out, err := runOperatorDeploy(t, client, "--hosts", "localhost", "--remote-dir", remoteDir,
		"--count", "2", "--background", "--endpoint", "localhost", "--approve")
	require.NoError(t, err, out)

	require.Len(t, client.PostCalls, len(dirs))
	for i, dir := range dirs {
		assert.Equal(t, constants.APIPaths.AuthPlatformEnrollmentDecision, client.PostCalls[i].Path)
		assert.Equal(t, models.PlatformEnrollmentDecisionRequest{
			RequestID: deployedDirID(dir) + "-000",
			Decision:  models.PlatformEnrollmentDecisionApprove,
		}, client.PostCalls[i].Body, "each request is approved by the ID its own directory logged")
		assert.Contains(t, out, deployedDirID(dir)+"-111", "session ID for %s is reported", dir)
	}
}

func TestOperatorDeployDoesNotWaitForARequestFromAnAlreadyEnrolledOperator(t *testing.T) {
	useFakeSSH(t, fakeWorkerAlreadyEnrolled)
	remoteDir := filepath.Join(t.TempDir(), "fleet")

	listBody, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{
		{OperatorSessionID: deployedDirID(remoteDir) + "-111", Status: constants.OperatorStatusActive},
	}})
	require.NoError(t, err)
	client := &cmdtest.MockAPIClient{GetResp: listBody}

	start := time.Now()
	out, err := runOperatorDeploy(t, client, "--hosts", "localhost", "--remote-dir", remoteDir,
		"--background", "--endpoint", "localhost", "--approve")
	require.NoError(t, err, out)

	assert.Less(t, time.Since(start), operatorDeployEnrollTimeout, "must not wait out the enrollment timeout")
	assert.Empty(t, client.PostCalls, "there is no request to approve")
	assert.Contains(t, out, "already enrolled")
	assert.Contains(t, out, deployedDirID(remoteDir)+"-111")
}

func TestOperatorDeployGivesUpWhenEnrollmentKeepsBeingRejected(t *testing.T) {
	useFakeSSH(t, fakeWorkerRejected)
	remoteDir := filepath.Join(t.TempDir(), "fleet")
	client := &cmdtest.MockAPIClient{}

	out, err := runOperatorDeploy(t, client, "--hosts", "localhost", "--remote-dir", remoteDir,
		"--background", "--endpoint", "localhost", "--approve")
	require.Error(t, err, out)
	assert.ErrorIs(t, err, constants.ErrOperatorDeployFailed)
	assert.Contains(t, out, "HTTP 429", "the worker's own failure line is surfaced")
	assert.Empty(t, client.PostCalls, "nothing is approved without a request ID")
}
