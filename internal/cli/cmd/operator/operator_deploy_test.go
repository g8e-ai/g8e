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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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

type mockRemoteDeployClient struct {
	mu              sync.Mutex
	host            string
	port            int
	identityFile    string
	isWSL           bool
	wslError        error
	uploadErr       error
	preflightErr    error
	startErr        error
	lastLaunchID    string
	state           *models.OperatorDeploymentState
	recordPreflight bool
	remoteDir       string
	uploaded        []string
	requests        []models.DeployHostRequest
}

func (m *mockRemoteDeployClient) IsWSL() bool {
	return m.isWSL
}

func (m *mockRemoteDeployClient) Close() error {
	return nil
}

func (m *mockRemoteDeployClient) UploadBinary(ctx context.Context, sourceBinary, remoteDir string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.uploadErr != nil {
		return "", m.uploadErr
	}
	if m.isWSL && m.wslError != nil {
		return "", fmt.Errorf("%w: WSL bootstrap failed on %s: %v", constants.ErrWSLUnavailable, m.host, m.wslError)
	}
	m.remoteDir = remoteDir
	_ = os.MkdirAll(remoteDir, 0o755)
	m.uploaded = append(m.uploaded, remoteDir)
	return filepath.Join(remoteDir, ".deploy-bin"), nil
}

func (m *mockRemoteDeployClient) ExecuteAgent(ctx context.Context, req models.DeployHostRequest) (models.DeployHostResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requests = append(m.requests, req)
	if m.isWSL && m.wslError != nil {
		return models.DeployHostResponse{}, fmt.Errorf("%w: %v", constants.ErrWSLUnavailable, m.wslError)
	}

	switch req.Action {
	case models.DeployHostActionPrepare:
		dirs := req.Dirs
		if len(dirs) == 0 && req.DestDir != "" {
			dirs = []string{req.DestDir}
		}
		for _, d := range dirs {
			_ = os.MkdirAll(d, 0o755)
		}
		return models.DeployHostResponse{Success: true, ResolvedDirs: dirs}, nil

	case models.DeployHostActionLink:
		if req.DestDir != "" {
			_ = os.MkdirAll(req.DestDir, 0o755)
			binName := req.Binary
			if binName == "" {
				binName = "g8e"
			}
			targetPath := filepath.Join(req.DestDir, binName)
			_ = os.WriteFile(targetPath, []byte("fake-operator-binary"), 0o755)
		}
		return models.DeployHostResponse{Success: true}, nil

	case models.DeployHostActionPreflight:
		if m.recordPreflight || m.preflightErr != nil {
			var lines []string
			lines = append(lines, "operator", "gateway-preflight")
			lines = append(lines, req.PreflightArgs...)
			preflightContent := strings.Join(lines, "\n") + "\n"
			if m.remoteDir != "" {
				_ = os.MkdirAll(m.remoteDir, 0o755)
				_ = os.WriteFile(filepath.Join(m.remoteDir, "preflight.txt"), []byte(preflightContent), 0o644)
			}
		}
		if m.preflightErr != nil {
			return models.DeployHostResponse{Success: false, Error: m.preflightErr.Error()}, m.preflightErr
		}
		return models.DeployHostResponse{Success: true}, nil

	case models.DeployHostActionStart:
		if m.startErr != nil {
			return models.DeployHostResponse{Success: false, Error: m.startErr.Error()}, m.startErr
		}
		for _, a := range req.Args {
			if strings.HasPrefix(a, "--deployment-id=") {
				m.lastLaunchID = strings.TrimPrefix(a, "--deployment-id=")
			}
		}
		var lines []string
		lines = append(lines, "./g8e", "operator", "start")
		lines = append(lines, req.Args...)
		hasWorkingDir := false
		for _, a := range req.Args {
			if a == "--working-dir" || strings.HasPrefix(a, "--working-dir=") {
				hasWorkingDir = true
				break
			}
		}
		if !hasWorkingDir && req.WorkingDir != "" {
			lines = append(lines, "--working-dir", req.WorkingDir)
		}
		argsContent := strings.Join(lines, "\n") + "\n"
		if req.WorkingDir != "" {
			_ = os.MkdirAll(req.WorkingDir, 0o755)
			_ = os.WriteFile(filepath.Join(req.WorkingDir, "args.txt"), []byte(argsContent), 0o644)
		}
		return models.DeployHostResponse{Success: true, PID: 1234}, nil

	case models.DeployHostActionState:
		if m.state != nil {
			st := *m.state
			if st.LaunchID == "" {
				st.LaunchID = m.lastLaunchID
			}
			return models.DeployHostResponse{Success: true, State: &st}, nil
		}
		return models.DeployHostResponse{Success: true, State: &models.OperatorDeploymentState{
			Phase:             models.OperatorDeploymentPhasePendingApproval,
			RequestID:         "req-1",
			OperatorSessionID: "session-1",
			LaunchID:          m.lastLaunchID,
		}}, nil

	default:
		return models.DeployHostResponse{Success: true}, nil
	}
}

func defaultTestRemoteFactory(t *testing.T, customMock ...*mockRemoteDeployClient) remoteDeployClientFactory {
	t.Helper()
	return func(ctx context.Context, host string, port int, identityFile string) (remoteDeployClient, error) {
		if len(customMock) > 0 && customMock[0] != nil {
			m := customMock[0]
			m.host = host
			m.port = port
			m.identityFile = identityFile
			return m, nil
		}
		return &mockRemoteDeployClient{
			host:         host,
			port:         port,
			identityFile: identityFile,
		}, nil
	}
}

// runOperatorDeploy runs deploy against a Gateway that announces every worker
// as staged and ready.
func runOperatorDeploy(t *testing.T, client authcmd.APIClient, args ...string) (string, error) {
	t.Helper()
	return runOperatorDeployWith(t, client, scriptedConnector(true, constants.OperatorStatusActive), args...)
}

func runOperatorDeployWith(t *testing.T, client authcmd.APIClient, connect deploymentEventsConnector, args ...string) (string, error) {
	t.Helper()
	return runOperatorDeployWithFactory(t, client, connect, defaultTestRemoteFactory(t), args...)
}

func runOperatorDeployWithFactory(t *testing.T, client authcmd.APIClient, connect deploymentEventsConnector, remoteFactory remoteDeployClientFactory, args ...string) (string, error) {
	t.Helper()
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")

	cmd := operatorDeployCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) { return client, nil },
		cmdtest.FileSvcFactoryFor(fileSvc),
		connect,
		remoteFactory,
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
	remoteDir := filepath.Join(t.TempDir(), "fleet")
	dirs := operatorDeployDirs(remoteDir, 2)

	pending := models.PlatformEnrollmentPendingResponse{}
	response := models.PlatformEnrollmentBatchDecisionResponse{ReceiptID: "batch-receipt"}
	for i := range dirs {
		id := fmt.Sprintf("req-%d", i+1)
		pending.Requests = append(pending.Requests, models.PlatformEnrollmentPendingRequest{RequestID: id})
		response.Requests = append(response.Requests, models.PlatformEnrollmentDecisionResponse{RequestID: id, State: models.PlatformEnrollmentStateApproved})
	}
	listBody, err := json.Marshal(pending)
	require.NoError(t, err)
	decisionBody, err := json.Marshal(response)
	require.NoError(t, err)
	client := &cmdtest.MockAPIClient{GetResp: listBody, PostResp: decisionBody}

	out, err := runOperatorDeploy(t, client, "--hosts", "host", "--dest-dir", remoteDir,
		"--count", "2", "--background", "--endpoint", "localhost", "--operator-endpoint", "192.168.1.50", "--approve", "--log", "error")
	require.NoError(t, err, out)

	assert.Equal(t, []string{constants.APIPaths.AuthPlatformEnrollmentPending}, client.GetCalls)
	require.Len(t, client.PostCalls, 1)
	require.Equal(t, constants.APIPaths.AuthPlatformEnrollmentBatchDecision, client.PostCalls[0].Path)
	decision := client.PostCalls[0].Body.(models.PlatformEnrollmentBatchDecisionRequest)
	require.Len(t, decision.Requests, len(dirs))
	for i := range dirs {
		assert.Equal(t, fmt.Sprintf("req-%d", i+1), decision.Requests[i].RequestID)
		assert.Contains(t, out, fmt.Sprintf("session %s", fmt.Sprintf("session-%d", i+1)))
	}
}

func TestOperatorDeployDoesNotWaitForARequestFromAnAlreadyEnrolledOperator(t *testing.T) {
	remoteDir := filepath.Join(t.TempDir(), "fleet")

	client := &cmdtest.MockAPIClient{}

	start := time.Now()
	out, err := runOperatorDeployWith(t, client, scriptedConnector(false, constants.OperatorStatusActive),
		"--hosts", "host", "--dest-dir", remoteDir, "--background", "--endpoint", "localhost", "--operator-endpoint", "192.168.1.50", "--approve")
	require.NoError(t, err, out)

	assert.Less(t, time.Since(start), operatorDeployEnrollTimeout, "must not wait out the enrollment timeout")
	assert.Empty(t, client.GetCalls, "command readiness does not require a registry status lookup")
	assert.Empty(t, client.PostCalls, "there is no request to approve")
	assert.Contains(t, out, "already enrolled")
	assert.Contains(t, out, "session-1")
}

func TestOperatorDeployFailsWhenAWorkerIsNeverAnnouncedAndNamesItsOwnFailure(t *testing.T) {
	remoteDir := filepath.Join(t.TempDir(), "fleet")
	client := &cmdtest.MockAPIClient{}
	previous := operatorDeployEnrollTimeout
	operatorDeployEnrollTimeout = 200 * time.Millisecond
	t.Cleanup(func() { operatorDeployEnrollTimeout = previous })

	mock := &mockRemoteDeployClient{
		state: &models.OperatorDeploymentState{
			Phase: models.OperatorDeploymentPhaseFailed,
			Error: "HTTP 429",
		},
	}

	out, err := runOperatorDeployWithFactory(t, client, scriptedConnector(false, ""),
		defaultTestRemoteFactory(t, mock),
		"--hosts", "host", "--dest-dir", remoteDir, "--background", "--endpoint", "localhost", "--operator-endpoint", "192.168.1.50", "--approve")
	require.Error(t, err, out)
	assert.ErrorIs(t, err, constants.ErrOperatorDeployFailed)
	assert.Contains(t, err.Error()+out, "HTTP 429", "the worker's own failure is surfaced")
	assert.Empty(t, client.PostCalls, "nothing is approved without a request ID")
}

func TestOperatorDeployDoesNotCallASubscriptionOnAStoppedOperatorReady(t *testing.T) {
	remoteDir := filepath.Join(t.TempDir(), "fleet")
	listBody, err := json.Marshal(models.PlatformEnrollmentPendingResponse{
		Requests: []models.PlatformEnrollmentPendingRequest{{RequestID: "req-1"}},
	})
	require.NoError(t, err)
	decisionBody, err := json.Marshal(models.PlatformEnrollmentBatchDecisionResponse{
		ReceiptID: "batch-receipt",
		Requests:  []models.PlatformEnrollmentDecisionResponse{{RequestID: "req-1", State: models.PlatformEnrollmentStateApproved}},
	})
	require.NoError(t, err)

	out, err := runOperatorDeployWith(t, &cmdtest.MockAPIClient{GetResp: listBody, PostResp: decisionBody},
		scriptedConnector(true, constants.OperatorStatusStopped),
		"--hosts", "host", "--dest-dir", remoteDir, "--background", "--endpoint", "localhost", "--operator-endpoint", "192.168.1.50", "--approve")
	require.ErrorIs(t, err, constants.ErrOperatorDeployFailed, out)
	assert.Contains(t, err.Error(), "stopped")
}

func TestOperatorDeployLocalBatchSharesBinaryAndPreservesEarlierBatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "fleet")
	client := &cmdtest.MockAPIClient{}
	out, err := runOperatorDeploy(t, client, "--local", "--dest-dir", root, "--count", "10", "--parallel", "4")
	require.NoError(t, err, out)
	binaryName := "g8e"
	if runtime.GOOS == "windows" {
		binaryName = "g8e.exe"
	}
	first, err := os.Stat(filepath.Join(root, "op-00001", binaryName))
	require.NoError(t, err)
	for _, dir := range operatorDeployDirs(root, 10) {
		info, err := os.Stat(filepath.Join(dir, binaryName))
		require.NoError(t, err)
		assert.True(t, os.SameFile(first, info), "binaries must share one inode")
	}
	out, err = runOperatorDeploy(t, client, "--local", "--dest-dir", root, "--count", "2", "--start-index", "11", "--parallel", "2")
	require.NoError(t, err, out)
	unchanged, err := os.Stat(filepath.Join(root, "op-00001", binaryName))
	require.NoError(t, err)
	assert.True(t, os.SameFile(first, unchanged), "appending a batch must preserve existing deployments")
	require.FileExists(t, filepath.Join(root, "op-00012", binaryName))
	// Explicit start-index with count=1 must still use a numbered directory.
	out, err = runOperatorDeploy(t, client, "--local", "--dest-dir", root, "--start-index", "5000")
	require.NoError(t, err, out)
	require.FileExists(t, filepath.Join(root, "op-05000", binaryName))
	dirs := operatorDeployDirs(root, 5000)
	require.Len(t, dirs, 5000)
	assert.Equal(t, filepath.ToSlash(filepath.Join(root, "op-05000")), filepath.ToSlash(dirs[4999]))
}

func TestOperatorDeployRejectsUnsafeBatchesAndRoles(t *testing.T) {
	for _, flags := range [][]string{
		{"--count", "5001"}, {"--count", "2", "--start-index", "5000"},
		{"--start-index", "0"}, {"--parallel", "0"}, {"--parallel", strconv.Itoa(constants.PlatformEnrollmentMaxLiveOperatorRequests + 1)},
		{"--roles", "unknown"},

		{"--local"}, {"--hosts", "a,,b"}, {"--hosts", "a,a"}, {"--hosts", "-oProxyCommand=bad"},
	} {
		_, err := runOperatorDeploy(t, &cmdtest.MockAPIClient{}, append([]string{"--hosts", "host"}, flags...)...)
		require.Error(t, err, flags)
	}
}

func TestOperatorDeployRoleFlagsReachWorkerWithoutShellExpansion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "fleet")
	payload := "http://provider:11434/path?q='$(touch INJECTED)'"
	client := &cmdtest.MockAPIClient{}
	out, err := runOperatorDeploy(t, client, "--hosts", "host", "--dest-dir", root,
		"--roles", "inference", "--inference-ollama-endpoint", payload, "--background", "--endpoint", "localhost", "--operator-endpoint", "192.168.1.50")
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
		scriptedConnector(true, constants.OperatorStatusActive),
		defaultTestRemoteFactory(t),
	)
	cmd.Flags().StringP("endpoint", "e", "", "")
	cmd.SetArgs([]string{"--hosts", "host", "--dest-dir", root, "--background", "--approve", "--endpoint", "localhost", "--operator-endpoint", "192.168.1.2"})
	// The mock API has no pending list, so approval fails after proving both
	// independently resolved addresses.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = cmd.ExecuteContext(ctx)
	var data []byte
	require.Eventually(t, func() bool {
		var err error
		data, err = os.ReadFile(filepath.Join(root, "args.txt"))
		return err == nil && len(data) > 0
	}, 5*time.Second, 20*time.Millisecond)
	assert.Contains(t, string(data), "--endpoint\n192.168.1.2\n")
	assert.NotContains(t, string(data), "--endpoint\nlocalhost\n")
	assert.Contains(t, ownerURL, "localhost")
}

func TestOperatorDeployForwardsGatewayPortsToTheWorker(t *testing.T) {
	root := filepath.Join(t.TempDir(), "fleet")
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")
	cmd := operatorDeployCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) {
			return &cmdtest.MockAPIClient{}, nil
		},
		cmdtest.FileSvcFactoryFor(fileSvc),
		scriptedConnector(true, constants.OperatorStatusActive),
		defaultTestRemoteFactory(t),
	)
	cmd.Flags().StringP("endpoint", "e", "", "")
	cmd.SetArgs([]string{"--hosts", "host", "--dest-dir", root, "--background", "--endpoint", "gateway", "--operator-endpoint", "192.168.1.50", "--gateway-http-port", "9080", "--gateway-https-port", "9443"})
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

func TestOperatorDeployApprovalClientDialsTheWorkerGatewayPorts(t *testing.T) {
	t.Cleanup(func() { config.SetEndpointOverride("") })
	cfg := &config.Config{}
	tests := []struct {
		name      string
		args      []string
		wantHTTP  string
		wantHTTPS string
	}{
		{"defaults untouched", []string{"--endpoint", "localhost"}, "http://localhost:8080", "https://localhost:8443"},
		{"endpoint host with ports", []string{"--endpoint", "localhost", "--gateway-http-port", "18080", "--gateway-https-port", "18443"}, "http://localhost:18080", "https://localhost:18443"},
		{"endpoint port replaced", []string{"--endpoint", "gw.example:9080", "--gateway-https-port", "9443"}, "http://gw.example:9080", "https://gw.example:9443"},
		{"no owner endpoint", []string{"--gateway-https-port", "9443"}, "http://localhost:8080", "https://localhost:9443"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.SetEndpointOverride("")
			cmd := operatorDeployCmdWithConfig(nil, nil, nil, nil)
			cmd.Flags().StringP("endpoint", "e", "", "")
			require.NoError(t, cmd.ParseFlags(tt.args))
			if endpoint, _ := cmd.Flags().GetString("endpoint"); endpoint != "" {
				config.SetEndpointOverride(endpoint) // as the root PersistentPreRunE does without -p
			}
			useDeployGatewayPorts(cmd)
			assert.Equal(t, tt.wantHTTP, cfg.OperatorDiscoveryURL())
			assert.Equal(t, tt.wantHTTPS, cfg.OperatorHTTPURL())
		})
	}
}

func TestOperatorDeployStopsBeforeStartingWorkersWhenTheGatewayIsUnreachable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "fleet")
	mock := &mockRemoteDeployClient{
		preflightErr:    errors.New("connection refused"),
		recordPreflight: true,
	}
	out, err := runOperatorDeployWithFactory(t, &cmdtest.MockAPIClient{},
		scriptedConnector(true, constants.OperatorStatusActive),
		defaultTestRemoteFactory(t, mock),
		"--hosts", "host", "--dest-dir", root,
		"--background", "--endpoint", "192.168.1.2", "--operator-endpoint", "192.168.1.2", "--gateway-https-port", "9443")
	require.ErrorIs(t, err, constants.ErrOperatorDeployFailed, out)
	assert.Contains(t, err.Error(), "--listen-host 0.0.0.0")
	data, readErr := os.ReadFile(filepath.Join(root, "preflight.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "operator\ngateway-preflight\n192.168.1.2\n--gateway-https-port=9443\n", string(data))
	assert.NoFileExists(t, filepath.Join(root, operatorDeployStartLog), "no worker may start")
}

func TestOperatorDeployRejectsGatewayPortsOutOfRange(t *testing.T) {
	for _, flag := range []string{"--gateway-http-port", "--gateway-https-port"} {
		for _, port := range []string{"-1", "65536"} {
			_, err := runOperatorDeploy(t, &cmdtest.MockAPIClient{}, "--hosts", "host", "--background", "--endpoint", "gateway", "--operator-endpoint", "192.168.1.50", flag, port)
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
	root := filepath.Join(t.TempDir(), "fleet")
	const workers = 12
	expected := make(map[string]bool, workers)
	pending := models.PlatformEnrollmentPendingResponse{Requests: []models.PlatformEnrollmentPendingRequest{{RequestID: "unrelated"}}}
	response := models.PlatformEnrollmentBatchDecisionResponse{ReceiptID: "batch-receipt"}
	for i := 1; i <= workers; i++ {
		id := fmt.Sprintf("req-%d", i)
		expected[id] = true
		pending.Requests = append(pending.Requests, models.PlatformEnrollmentPendingRequest{RequestID: id})
		response.Requests = append(response.Requests, models.PlatformEnrollmentDecisionResponse{RequestID: id, State: models.PlatformEnrollmentStateApproved})
	}
	body, err := json.Marshal(pending)
	require.NoError(t, err)
	postBody, err := json.Marshal(response)
	require.NoError(t, err)
	client := &cmdtest.MockAPIClient{GetResp: body, PostResp: postBody}

	out, err := runOperatorDeploy(t, client, "--hosts", "host", "--dest-dir", root,
		"--count", "12", "--parallel", "4", "--background", "--approve", "--endpoint", "localhost", "--operator-endpoint", "192.168.1.50")
	require.NoError(t, err, out)
	assert.Equal(t, []string{constants.APIPaths.AuthPlatformEnrollmentPending}, client.GetCalls)
	require.Len(t, client.PostCalls, 1)
	for _, decision := range client.PostCalls[0].Body.(models.PlatformEnrollmentBatchDecisionRequest).Requests {
		require.True(t, expected[decision.RequestID], "only this cohort may be approved")
		delete(expected, decision.RequestID)
	}
	assert.Empty(t, expected)
	for i := 1; i <= workers; i++ {
		assert.Contains(t, out, fmt.Sprintf("session-%d", i))
	}
}

func TestOperatorDeployMasterKey_OnlyExplicitTargetPathIsForwarded(t *testing.T) {
	t.Setenv(string(constants.EnvVar.MasterKeyFile), "/developer/private/key")
	cmd := operatorDeployCmd()
	args, err := operatorDeployStartArgs(cmd)
	require.NoError(t, err)
	for _, arg := range args {
		assert.NotContains(t, arg, "master-key-file")
	}
	require.NoError(t, cmd.Flags().Set("master-key-file", "/target/private/key"))
	args, err = operatorDeployStartArgs(cmd)
	require.NoError(t, err)
	assert.Contains(t, args, "--master-key-file=/target/private/key")
	assert.NotContains(t, args, "--master-key-file=/developer/private/key")
}

func TestIsWindowsPath_WSLDetection(t *testing.T) {
	windowsPaths := []string{
		`C:\Users\admin`,
		`c:\fleet`,
		`C:/Users/admin`,
		`/C:/fleet`,
		`/c:/Users/admin/app`,
		`D:\data`,
		`\\wsl$\Ubuntu\home`,
		`path\with\backslashes`,
	}
	for _, p := range windowsPaths {
		assert.True(t, isWindowsPath(p), "expected %q to be recognized as Windows path", p)
	}

	posixPaths := []string{
		"/home/user",
		"/var/log",
		"/tmp/fleet",
		"relative/posix/path",
		"",
		" ",
	}
	for _, p := range posixPaths {
		assert.False(t, isWindowsPath(p), "expected %q NOT to be recognized as Windows path", p)
	}

	assert.Equal(t, "C:/fleet", toWindowsFilePath("/C:/fleet"))
	assert.Equal(t, "c:/Users/admin/app", toWindowsFilePath("/c:/Users/admin/app"))
	assert.Equal(t, "C:\\Users", toWindowsFilePath("C:\\Users"))
	assert.Equal(t, "/home/user", toWindowsFilePath("/home/user"))
}

func TestValidateOperatorDeployFlags_TargetResolution(t *testing.T) {
	t.Run("local only with dest-dir", func(t *testing.T) {
		cmd := operatorDeployCmd()
		require.NoError(t, cmd.ParseFlags([]string{"--local", "--dest-dir", "/tmp/fleet"}))
		hosts, workerEndpoint, err := validateOperatorDeployFlags(cmd, true, "", "", "", "/tmp/fleet", 1, 1, 1, false, "", false)
		require.NoError(t, err)
		assert.Equal(t, "local", hosts)
		assert.Empty(t, workerEndpoint)
	})

	t.Run("local only without dest-dir fails", func(t *testing.T) {
		cmd := operatorDeployCmd()
		require.NoError(t, cmd.ParseFlags([]string{"--local"}))
		_, _, err := validateOperatorDeployFlags(cmd, true, "", "", "", "~", 1, 1, 1, false, "", false)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
	})

	t.Run("local with remote-dir alias succeeds", func(t *testing.T) {
		cmd := operatorDeployCmd()
		require.NoError(t, cmd.ParseFlags([]string{"--local", "--remote-dir", "/tmp/fleet"}))
		hosts, _, err := validateOperatorDeployFlags(cmd, true, "", "", "", "/tmp/fleet", 1, 1, 1, false, "", false)
		require.NoError(t, err)
		assert.Equal(t, "local", hosts)
	})

	t.Run("combine local and hosts", func(t *testing.T) {
		cmd := operatorDeployCmd()
		require.NoError(t, cmd.ParseFlags([]string{"--local", "--hosts", "remote1,remote2", "--dest-dir", "/tmp/fleet"}))
		hosts, _, err := validateOperatorDeployFlags(cmd, true, "remote1,remote2", "", "", "/tmp/fleet", 1, 1, 1, false, "", false)
		require.NoError(t, err)
		assert.Equal(t, "local,remote1,remote2", hosts)
	})

	t.Run("rejects duplicate targets", func(t *testing.T) {
		cmd := operatorDeployCmd()
		require.NoError(t, cmd.ParseFlags([]string{"--local", "--hosts", "remote1,local", "--dest-dir", "/tmp/fleet"}))
		_, _, err := validateOperatorDeployFlags(cmd, true, "remote1,local", "", "", "/tmp/fleet", 1, 1, 1, false, "", false)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
	})

	t.Run("localhost and 127.0.0.1 recognized as local targets", func(t *testing.T) {
		assert.True(t, isLocalTarget("local"))
		assert.True(t, isLocalTarget("localhost"))
		assert.True(t, isLocalTarget("127.0.0.1"))
		assert.False(t, isLocalTarget("remote.example.com"))
		assert.False(t, isLocalTarget("192.168.1.100"))
		assert.False(t, isRemoteTarget("localhost"))
		assert.True(t, isRemoteTarget("remote.example.com"))
	})
}

func TestValidateOperatorDeployFlags_RemoteOperatorEndpoint(t *testing.T) {
	t.Run("remote target background requires non-empty operator-endpoint", func(t *testing.T) {
		cmd := operatorDeployCmd()
		cmd.Flags().StringP("endpoint", "e", "", "")
		require.NoError(t, cmd.ParseFlags([]string{"--hosts", "remote1", "--dest-dir", "/tmp/fleet", "--background", "--endpoint", "192.168.1.50"}))
		_, _, err := validateOperatorDeployFlags(cmd, false, "remote1", "", "", "/tmp/fleet", 1, 1, 1, true, "", false)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
		assert.ErrorIs(t, err, constants.ErrOperatorEndpointInvalid)
	})

	t.Run("remote target background rejects loopback operator-endpoint", func(t *testing.T) {
		for _, loopback := range []string{"localhost", "127.0.0.1", "127.0.0.2", "::1", "http://localhost:8080"} {
			cmd := operatorDeployCmd()
			cmd.Flags().StringP("endpoint", "e", "", "")
			require.NoError(t, cmd.ParseFlags([]string{"--hosts", "remote1", "--dest-dir", "/tmp/fleet", "--background", "--endpoint", "gateway", "--operator-endpoint", loopback}))
			_, _, err := validateOperatorDeployFlags(cmd, false, "remote1", "", "", "/tmp/fleet", 1, 1, 1, true, loopback, false)
			require.Error(t, err, loopback)
			assert.ErrorIs(t, err, constants.ErrOperatorEndpointInvalid, loopback)
		}
	})

	t.Run("remote target background accepts valid non-loopback endpoint", func(t *testing.T) {
		cmd := operatorDeployCmd()
		cmd.Flags().StringP("endpoint", "e", "", "")
		require.NoError(t, cmd.ParseFlags([]string{"--hosts", "remote1", "--dest-dir", "/tmp/fleet", "--background", "--endpoint", "localhost", "--operator-endpoint", "192.168.1.50"}))
		hosts, workerEndpoint, err := validateOperatorDeployFlags(cmd, false, "remote1", "", "", "/tmp/fleet", 1, 1, 1, true, "192.168.1.50", false)
		require.NoError(t, err)
		assert.Equal(t, "remote1", hosts)
		assert.Equal(t, "192.168.1.50", workerEndpoint)
	})

	t.Run("local-only target background permits loopback and defaults to endpoint", func(t *testing.T) {
		cmd := operatorDeployCmd()
		cmd.Flags().StringP("endpoint", "e", "", "")
		require.NoError(t, cmd.ParseFlags([]string{"--local", "--dest-dir", "/tmp/fleet", "--background", "--endpoint", "localhost"}))
		hosts, workerEndpoint, err := validateOperatorDeployFlags(cmd, true, "", "", "", "/tmp/fleet", 1, 1, 1, true, "", false)
		require.NoError(t, err)
		assert.Equal(t, "local", hosts)
		assert.Equal(t, "localhost", workerEndpoint)
	})
}

func TestRemoteDeployClient_WSLUnavailable(t *testing.T) {
	t.Run("upload binary wraps ErrWSLUnavailable when WSL fails", func(t *testing.T) {
		mock := &mockRemoteDeployClient{
			host:     "wsl-host",
			isWSL:    true,
			wslError: errors.New("wsl.exe: distribution not found"),
		}
		_, err := mock.UploadBinary(context.Background(), "/fake/source", "/dest")
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrWSLUnavailable)
	})

	t.Run("execute agent wraps ErrWSLUnavailable when WSL fails", func(t *testing.T) {
		mock := &mockRemoteDeployClient{
			host:     "wsl-host",
			isWSL:    true,
			wslError: errors.New("wsl.exe exec failed"),
		}
		_, err := mock.ExecuteAgent(context.Background(), models.DeployHostRequest{Action: models.DeployHostActionPrepare})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrWSLUnavailable)
	})

	t.Run("operator deploy command surfaces ErrWSLUnavailable on WSL bootstrap failure", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "fleet")
		mock := &mockRemoteDeployClient{
			host:     "wsl-host",
			isWSL:    true,
			wslError: errors.New("wsl.exe: distribution not found"),
		}
		_, err := runOperatorDeployWithFactory(
			t,
			&cmdtest.MockAPIClient{},
			scriptedConnector(true, constants.OperatorStatusActive),
			defaultTestRemoteFactory(t, mock),
			"--hosts", "wsl-host",
			"--dest-dir", root,
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrWSLUnavailable)
	})
}
