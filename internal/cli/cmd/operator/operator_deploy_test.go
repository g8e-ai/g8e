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
	"runtime"
	"strings"
	"testing"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// The worker fixture exposes the same non-secret deployment-state command as
// the installed binary. Its human output contains no IDs or readiness markers.
const fakeWorkerStatePreamble = `#!/bin/sh
state='@state@'
id=$(printf %s "$PWD" | md5sum | cut -c1-32)
if [ "$2" = deployment-state ]; then
  if [ ! -f "$state" ]; then printf 'null\n'; exit; fi
  cat "$state"
  launch=$(sed -n 's/.*"launch_id":"\([^"]*\)".*/\1/p' "$state")
  printf '{"launch_id":"%s","phase":"ready","operator_session_id":"%s-111","updated_at":"2026-10-08T00:00:00Z"}\n' "$launch" "$id" > "$state"
  exit
fi
for arg; do case "$arg" in --deployment-id=*) launch=${arg#*=};; esac; done
mkdir -p "$(dirname "$state")"
`

const fakeWorkerEnrolls = fakeWorkerStatePreamble + `
printf '{"launch_id":"%s","phase":"pending_approval","request_id":"%s-000","updated_at":"2026-10-08T00:00:00Z"}\n' "$launch" "$id" > "$state"
printf 'Worker staged\n'
`

const fakeWorkerAlreadyEnrolled = fakeWorkerStatePreamble + `
printf '{"launch_id":"%s","phase":"ready","operator_session_id":"%s-111","updated_at":"2026-10-08T00:00:00Z"}\n' "$launch" "$id" > "$state"
printf 'Worker started\n'
`

const fakeWorkerRejected = fakeWorkerStatePreamble + `
printf '{"launch_id":"%s","phase":"failed","error":"HTTP 429","updated_at":"2026-10-08T00:00:00Z"}\n' "$launch" > "$state"
`

// useFakeSSH puts ssh and scp on PATH that run the remote command, and copy the
// file, on this machine, and makes scp install worker instead of the test
// binary. The remote shell commands deploy sends therefore run for real.
func useFakeSSH(t *testing.T, worker string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("SSH fixture executes a POSIX remote shell locally; host-native deployment is tested separately")
	}
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "ssh"), []byte("#!/bin/sh\nfor a; do cmd=\"$a\"; done\nexec sh -c \"$cmd\"\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "scp"), []byte("#!/bin/sh\nfor a; do dst=\"${a#*:}\"; done\ncp \"$FAKE_WORKER\" \"$dst\"\n"), 0o755))
	workerPath := filepath.Join(binDir, "worker")
	require.NoError(t, os.WriteFile(workerPath, []byte(strings.ReplaceAll(worker, "@state@", constants.OperatorDeploymentStatePath)), 0o755))
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
	cmd.SetArgs(append([]string{"--parallel", "1"}, args...))

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

func TestOperatorDeployStagesFleetBeforeOneApprovalAndReportsSessions(t *testing.T) {
	useFakeSSH(t, fakeWorkerEnrolls)
	remoteDir := filepath.Join(t.TempDir(), "fleet")
	dirs := operatorDeployDirs(remoteDir, 2)

	sessions := make([]*operatorv1.OperatorDocument, len(dirs))
	for i, dir := range dirs {
		sessions[i] = &operatorv1.OperatorDocument{
			OperatorSessionId: deployedDirID(dir) + "-111",
			Status:            string(constants.OperatorStatusActive),
		}
	}
	pending := models.PlatformEnrollmentPendingResponse{}
	response := models.PlatformEnrollmentBatchDecisionResponse{ReceiptID: "batch-receipt"}
	for _, dir := range dirs {
		id := deployedDirID(dir) + "-000"
		pending.Requests = append(pending.Requests, models.PlatformEnrollmentPendingRequest{RequestID: id})
		response.Requests = append(response.Requests, models.PlatformEnrollmentDecisionResponse{RequestID: id, State: models.PlatformEnrollmentStateApproved})
	}
	listBody, err := json.Marshal(struct {
		Success   bool                                      `json:"success"`
		Operators []*operatorv1.OperatorDocument            `json:"operators"`
		Requests  []models.PlatformEnrollmentPendingRequest `json:"requests"`
	}{true, sessions, pending.Requests})
	require.NoError(t, err)
	decisionBody, err := json.Marshal(response)
	require.NoError(t, err)
	client := &cmdtest.MockAPIClient{GetResp: listBody, PostResp: decisionBody}

	out, err := runOperatorDeploy(t, client, "--hosts", "localhost", "--remote-dir", remoteDir,
		"--count", "2", "--background", "--endpoint", "localhost", "--approve")
	require.NoError(t, err, out)

	require.Len(t, client.PostCalls, 1)
	require.Equal(t, constants.APIPaths.AuthPlatformEnrollmentBatchDecision, client.PostCalls[0].Path)
	decision := client.PostCalls[0].Body.(models.PlatformEnrollmentBatchDecisionRequest)
	require.Len(t, decision.Requests, len(dirs))
	for i, dir := range dirs {
		assert.Equal(t, deployedDirID(dir)+"-000", decision.Requests[i].RequestID)
		assert.Contains(t, out, deployedDirID(dir)+"-111")
	}
}

func TestOperatorDeployDoesNotWaitForARequestFromAnAlreadyEnrolledOperator(t *testing.T) {
	useFakeSSH(t, fakeWorkerAlreadyEnrolled)
	remoteDir := filepath.Join(t.TempDir(), "fleet")

	listBody, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: []*operatorv1.OperatorDocument{
		{OperatorSessionId: deployedDirID(remoteDir) + "-111", Status: string(constants.OperatorStatusActive)},
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

func TestOperatorDeployLocalBatchSharesBinaryAndPreservesEarlierBatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "fleet")
	client := &cmdtest.MockAPIClient{}
	out, err := runOperatorDeploy(t, client, "--local", "--dest-dir", root, "--count", "10", "--parallel", "4")
	require.NoError(t, err, out)
	first, err := os.Stat(filepath.Join(root, "op-00001", "g8e"))
	require.NoError(t, err)
	for _, dir := range operatorDeployDirs(root, 10) {
		info, err := os.Stat(filepath.Join(dir, "g8e"))
		require.NoError(t, err)
		assert.True(t, os.SameFile(first, info), "binaries must share one inode")
	}
	out, err = runOperatorDeploy(t, client, "--local", "--dest-dir", root, "--count", "2", "--start-index", "11", "--parallel", "2")
	require.NoError(t, err, out)
	unchanged, err := os.Stat(filepath.Join(root, "op-00001", "g8e"))
	require.NoError(t, err)
	assert.True(t, os.SameFile(first, unchanged), "appending a batch must preserve existing deployments")
	require.FileExists(t, filepath.Join(root, "op-00012", "g8e"))
	// Explicit start-index with count=1 must still use a numbered directory.
	out, err = runOperatorDeploy(t, client, "--local", "--dest-dir", root, "--start-index", "5000")
	require.NoError(t, err, out)
	require.FileExists(t, filepath.Join(root, "op-05000", "g8e"))
	dirs := operatorDeployDirs(root, 5000)
	require.Len(t, dirs, 5000)
	assert.Equal(t, filepath.ToSlash(filepath.Join(root, "op-05000")), filepath.ToSlash(dirs[4999]))
}

func TestOperatorDeployRejectsUnsafeBatchesAndRoles(t *testing.T) {
	for _, flags := range [][]string{
		{"--count", "5001"}, {"--count", "2", "--start-index", "5000"},
		{"--start-index", "0"}, {"--parallel", "0"}, {"--parallel", "101"},
		{"--roles", "unknown"},

		{"--local"}, {"--hosts", "a,,b"}, {"--hosts", "a,a"}, {"--hosts", "-oProxyCommand=bad"},
	} {
		_, err := runOperatorDeploy(t, &cmdtest.MockAPIClient{}, append([]string{"--hosts", "host"}, flags...)...)
		require.Error(t, err, flags)
	}
}

func TestOperatorDeployRoleFlagsReachWorkerWithoutShellExpansion(t *testing.T) {
	useFakeSSH(t, "#!/bin/sh\nif [ \"$2\" = start ]; then printf '%s\\n' \"$@\" > args.txt; fi\n"+fakeWorkerAlreadyEnrolled[10:])
	root := filepath.Join(t.TempDir(), "fleet")
	payload := "http://provider:11434/path?q='$(touch INJECTED)'"
	client := &cmdtest.MockAPIClient{}
	out, err := runOperatorDeploy(t, client, "--hosts", "host", "--dest-dir", root,
		"--roles", "inference", "--inference-ollama-endpoint", payload, "--background", "--endpoint", "localhost")
	require.NoError(t, err, out)
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(filepath.Join(root, "args.txt"))
		return err == nil && bytes.Contains(data, []byte("--working-dir"))
	}, time.Second, 10*time.Millisecond)
	data, err := os.ReadFile(filepath.Join(root, "args.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "--roles=inference\n")
	assert.Contains(t, string(data), "--inference-ollama-endpoint="+payload+"\n")
	assert.NotContains(t, string(data), "--provenance-operator-enabled=true\n")
	assert.NoFileExists(t, filepath.Join(root, "INJECTED"))
}

func TestOperatorDeployRoleIdentities(t *testing.T) {
	args := []string{"--provenance-operator-enabled=true", "--model-storage-root={dir}/models"}
	first := operatorDeployArgsForDir(args, "host", "/fleet/op-00001")
	again := operatorDeployArgsForDir(args, "host", "/fleet/op-00001")
	other := operatorDeployArgsForDir(args, "host", "/fleet/op-00002")
	assert.Equal(t, first, again)
	assert.NotEqual(t, first[len(first)-1], other[len(other)-1])
	assert.Contains(t, first, "--model-storage-root=/fleet/op-00001/models")
	explicit := operatorDeployArgsForDir(append(args, "--provenance-operator-id={host}-{name}"), "host", "/fleet/op-00001")
	assert.Contains(t, explicit, "--provenance-operator-id=host-op-00001")
	assert.Len(t, explicit, 3)
}

func TestOperatorDeploySeparatesOwnerAndWorkerEndpoints(t *testing.T) {
	useFakeSSH(t, "#!/bin/sh\nif [ \"$2\" = start ]; then printf '%s\\n' \"$@\" > args.txt; fi\n"+fakeWorkerAlreadyEnrolled[10:])
	root := filepath.Join(t.TempDir(), "fleet")
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")
	var ownerURL string
	cmd := operatorDeployCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		func(_ fs.RuntimeFileService, got *config.Config) (authcmd.APIClient, error) {
			ownerURL = got.OperatorPublicURL()
			return &cmdtest.MockAPIClient{}, nil
		},
		cmdtest.FileSvcFactoryFor(fileSvc),
	)
	cmd.Flags().StringP("endpoint", "e", "", "")
	cmd.SetArgs([]string{"--hosts", "host", "--dest-dir", root, "--background", "--approve", "--endpoint", "localhost", "--operator-endpoint", "192.168.1.2"})
	// The fake session is not in the API response, so final online verification
	// fails after proving both independently resolved addresses.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = cmd.ExecuteContext(ctx)
	data, err := os.ReadFile(filepath.Join(root, "args.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "--endpoint\n192.168.1.2\n")
	assert.NotContains(t, string(data), "--endpoint\nlocalhost\n")
	assert.Contains(t, ownerURL, "localhost")
}

func TestOperatorDeployForwardsGatewayPortsToTheWorker(t *testing.T) {
	useFakeSSH(t, "#!/bin/sh\nif [ \"$2\" = start ]; then printf '%s\\n' \"$@\" > args.txt; fi\n"+fakeWorkerAlreadyEnrolled[10:])
	root := filepath.Join(t.TempDir(), "fleet")
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")
	cmd := operatorDeployCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) {
			return &cmdtest.MockAPIClient{}, nil
		},
		cmdtest.FileSvcFactoryFor(fileSvc),
	)
	cmd.Flags().StringP("endpoint", "e", "", "")
	cmd.SetArgs([]string{"--hosts", "host", "--dest-dir", root, "--background", "--endpoint", "gateway", "--gateway-http-port", "9080", "--gateway-https-port", "9443"})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = cmd.ExecuteContext(ctx)
	var data []byte
	require.Eventually(t, func() bool {
		var err error
		data, err = os.ReadFile(filepath.Join(root, "args.txt"))
		return err == nil && len(data) > 0
	}, 5*time.Second, 20*time.Millisecond)
	assert.Contains(t, string(data), "--gateway-http-port=9080\n")
	assert.Contains(t, string(data), "--gateway-https-port=9443\n")
}

func TestOperatorDeployRejectsGatewayPortsOutOfRange(t *testing.T) {
	for _, flag := range []string{"--gateway-http-port", "--gateway-https-port"} {
		for _, port := range []string{"-1", "65536"} {
			_, err := runOperatorDeploy(t, &cmdtest.MockAPIClient{}, "--hosts", "host", "--background", "--endpoint", "gateway", flag, port)
			require.ErrorIs(t, err, constants.ErrOperatorGatewayPortInvalid, flag+" "+port)
		}
	}
}

func TestOperatorDeployRejectsInvalidWorkerEndpoints(t *testing.T) {
	for _, endpoint := range []string{"http://gateway", "gateway:8080", "", "gateway name"} {
		args := []string{"--hosts", "host", "--background", "--operator-endpoint", endpoint}
		_, err := runOperatorDeploy(t, &cmdtest.MockAPIClient{}, args...)
		require.Error(t, err, endpoint)
	}
}

func TestOperatorDeployRejectsConflictingDockerTransportBeforeExecution(t *testing.T) {
	for _, args := range [][]string{
		{"--local", "--dest-dir", "/operators", "--docker-context", "remote", "--docker-image", "image"},
		{"--hosts", "host", "--docker-context", "remote", "--docker-image", "image", "--dest-dir", "/operators"},
		{"--docker-context", "remote", "--dest-dir", "/operators"},
		{"--docker-context", "remote", "--docker-image", "image", "--dest-dir", "relative"},
	} {
		_, err := runOperatorDeploy(t, &cmdtest.MockAPIClient{}, args...)
		require.Error(t, err, args)
	}
}

func TestOperatorDeployParallelEnrollmentApprovesOnlyOwnRequests(t *testing.T) {
	useFakeSSH(t, fakeWorkerEnrolls)
	root := filepath.Join(t.TempDir(), "fleet")
	dirs := operatorDeployDirs(root, 12)
	sessions := make([]*operatorv1.OperatorDocument, len(dirs))
	expected := make(map[string]bool, len(dirs))
	for i, dir := range dirs {
		sessions[i] = &operatorv1.OperatorDocument{OperatorSessionId: deployedDirID(dir) + "-111", Status: string(constants.OperatorStatusActive)}
		expected[deployedDirID(dir)+"-000"] = true
	}
	pending := models.PlatformEnrollmentPendingResponse{Requests: []models.PlatformEnrollmentPendingRequest{{RequestID: "unrelated"}}}
	response := models.PlatformEnrollmentBatchDecisionResponse{ReceiptID: "batch-receipt"}
	for _, dir := range dirs {
		id := deployedDirID(dir) + "-000"
		pending.Requests = append(pending.Requests, models.PlatformEnrollmentPendingRequest{RequestID: id})
		response.Requests = append(response.Requests, models.PlatformEnrollmentDecisionResponse{RequestID: id, State: models.PlatformEnrollmentStateApproved})
	}
	body, err := json.Marshal(struct {
		Success   bool                                      `json:"success"`
		Operators []*operatorv1.OperatorDocument            `json:"operators"`
		Requests  []models.PlatformEnrollmentPendingRequest `json:"requests"`
	}{true, sessions, pending.Requests})
	require.NoError(t, err)
	postBody, err := json.Marshal(response)
	require.NoError(t, err)
	client := &cmdtest.MockAPIClient{GetResp: body, PostResp: postBody}

	out, err := runOperatorDeploy(t, client, "--hosts", "host", "--dest-dir", root,
		"--count", "12", "--parallel", "4", "--background", "--approve", "--endpoint", "localhost")
	require.NoError(t, err, out)
	require.Len(t, client.PostCalls, 1)
	for _, decision := range client.PostCalls[0].Body.(models.PlatformEnrollmentBatchDecisionRequest).Requests {
		require.True(t, expected[decision.RequestID], "only this cohort may be approved")
		delete(expected, decision.RequestID)
	}
	assert.Empty(t, expected)
	for _, op := range sessions {
		assert.Contains(t, out, op.OperatorSessionId)
	}
}
