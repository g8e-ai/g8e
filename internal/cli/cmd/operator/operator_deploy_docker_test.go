// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.

package operatorcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

type fakeDockerRunner struct {
	calls [][]string
	run   func([]string) ([]byte, error)
}

func (f *fakeDockerRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	return f.run(args)
}

func joinedDockerCalls(calls [][]string) string {
	var lines []string
	for _, call := range calls {
		lines = append(lines, strings.Join(call, " "))
	}
	return strings.Join(lines, "\n")
}

func preparedFakeDocker(t *testing.T) (*deployDocker, *fakeDockerRunner) {
	t.Helper()
	fake := &fakeDockerRunner{run: func(args []string) ([]byte, error) {
		line := strings.Join(args, " ")
		switch {
		case strings.Contains(line, " info "):
			return []byte("linux/x86_64\n"), nil
		case strings.Contains(line, " image inspect "):
			return []byte("sha256:resolved linux/amd64\n"), nil
		case strings.Contains(line, " volume inspect "):
			return nil, errors.New("Error response from daemon: get volume: no such volume")
		case strings.Contains(line, " container inspect "):
			return nil, errors.New("Error response from daemon: no such container")
		default:
			return []byte("ok\n"), nil
		}
	}}
	d := newDeployDocker("livingroom-node", "g8e-operator:local", "/operators/fleet", nil)
	d.runner = fake
	require.NoError(t, d.prepare(context.Background(), "192.168.1.2"))
	return d, fake
}

func TestDockerDeploymentBuildsOwnedUnpublishedContainer(t *testing.T) {
	d, fake := preparedFakeDocker(t)
	dir := "/operators/fleet/op-00001"
	op, err := d.prepareOperator(context.Background(), dir, "192.168.1.2", []string{"--inference-enabled=true"})
	require.NoError(t, err)
	all := joinedDockerCalls(fake.calls)
	for _, call := range fake.calls {
		assert.Equal(t, []string{"--context", "livingroom-node"}, call[:2])
	}
	assert.Contains(t, all, "volume create --label "+dockerManagedLabel+"=true")
	assert.Contains(t, all, "container create --name "+op.container)
	assert.Contains(t, all, "--mount type=volume,source="+op.volume+",target="+dir)
	assert.Contains(t, all, "sha256:resolved operator start --endpoint 192.168.1.2")
	assert.Contains(t, all, "--inference-enabled=true --working-dir "+dir)
	assert.NotContains(t, all, " --publish ")
	assert.NotContains(t, all, " -p ")
}

func TestDockerDeploymentIdentityIsStableAndUniqueAtScale(t *testing.T) {
	d := newDeployDocker("livingroom-node", "image", "/operators/fleet", nil)
	first := d.spec("/operators/fleet/op-00001", nil)
	assert.Equal(t, first, d.spec("/operators/fleet/op-00001", nil))
	seen := make(map[string]bool, 5000)
	for i := 1; i <= 5000; i++ {
		op := d.spec(fmt.Sprintf("/operators/fleet/op-%05d", i), nil)
		require.False(t, seen[op.container], op.container)
		seen[op.container] = true
		assert.LessOrEqual(t, len(op.hostname), 63)
	}
	other := newDeployDocker("livingroom-node", "image", "/operators/other", nil)
	assert.NotEqual(t, first.container, other.spec("/operators/other/op-00001", nil).container)
}

func TestDockerDeploymentRefusesUnownedVolume(t *testing.T) {
	d, fake := preparedFakeDocker(t)
	fake.run = func(args []string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), " volume inspect ") {
			return []byte("  \n"), nil
		}
		return []byte("ok\n"), nil
	}
	_, err := d.prepareOperator(context.Background(), "/operators/fleet/op-00001", "gateway", nil)
	require.ErrorContains(t, err, "refusing to adopt")
	assert.NotContains(t, joinedDockerCalls(fake.calls), "container rm")
}

func TestDockerRedeployPreservesOwnedVolumeAndReplacesOwnedContainer(t *testing.T) {
	d, fake := preparedFakeDocker(t)
	dir := "/operators/fleet/op-00001"
	fake.calls = nil
	fake.run = func(args []string) ([]byte, error) {
		line := strings.Join(args, " ")
		switch {
		case strings.Contains(line, " volume inspect "), strings.Contains(line, " container inspect "):
			return []byte("true " + d.deployment + " " + dir + "\n"), nil
		default:
			return []byte("ok\n"), nil
		}
	}
	_, err := d.prepareOperator(context.Background(), dir, "gateway", nil)
	require.NoError(t, err)
	all := joinedDockerCalls(fake.calls)
	assert.NotContains(t, all, "volume create")
	assert.NotContains(t, all, "volume rm")
	assert.Contains(t, all, "container stop --time 10")
	assert.Contains(t, all, "container rm")
	assert.Contains(t, all, "container create")
}

func TestDockerDeploymentRefusesUnownedContainer(t *testing.T) {
	d, fake := preparedFakeDocker(t)
	dir := "/operators/fleet/op-00001"
	fake.run = func(args []string) ([]byte, error) {
		line := strings.Join(args, " ")
		if strings.Contains(line, " volume inspect ") {
			return []byte("true " + d.deployment + " " + dir + "\n"), nil
		}
		if strings.Contains(line, " container inspect ") {
			return []byte("false someone-else /other\n"), nil
		}
		return []byte("ok\n"), nil
	}
	_, err := d.prepareOperator(context.Background(), dir, "gateway", nil)
	require.ErrorContains(t, err, "refusing to replace")
	assert.NotContains(t, joinedDockerCalls(fake.calls), "container rm")
}

func TestDockerDeploymentRejectsMountCoveringRuntime(t *testing.T) {
	d := newDeployDocker("livingroom-node", "image", "/operators/fleet", []string{"type=bind,source=/srv,target=/operators,readonly"})
	err := d.validateMounts()
	require.ErrorContains(t, err, "covers the Operator runtime")
}

func TestOperatorDeployDockerMountFlagPreservesCommaSeparatedSpec(t *testing.T) {
	cmd := operatorDeployCmd()
	spec := "type=bind,source=/srv/models,target=/models,readonly"
	require.NoError(t, cmd.ParseFlags([]string{"--docker-mount", spec}))
	got, err := cmd.Flags().GetStringArray("docker-mount")
	require.NoError(t, err)
	assert.Equal(t, []string{spec}, got)
}

func TestDockerDeploymentReportsExitedContainerImmediately(t *testing.T) {
	d := newDeployDocker("livingroom-node", "image", "/operators/fleet", nil)
	d.imageID = "sha256:resolved"
	dir := "/operators/fleet/op-00001"
	op := d.spec(dir, nil)
	d.operators[dir] = op
	d.runner = &fakeDockerRunner{run: func(args []string) ([]byte, error) {
		line := strings.Join(args, " ")
		if strings.Contains(line, " container logs ") {
			return []byte("fatal configuration error\n"), nil
		}
		if strings.Contains(line, " container inspect ") {
			return []byte("false 17\n"), nil
		}
		return nil, nil
	}}
	_, err := d.awaitRequestID(context.Background(), dir)
	require.ErrorContains(t, err, "exited (17)")
	assert.ErrorContains(t, err, "fatal configuration error")
}

func TestDockerDeploymentReadsOnlyCurrentLaunchLogs(t *testing.T) {
	d := newDeployDocker("livingroom-node", "image", "/operators/fleet", nil)
	dir := "/operators/fleet/op-00001"
	op := d.spec(dir, nil)
	op.launchTime = time.Now().UTC()
	d.operators[dir] = op
	fake := &fakeDockerRunner{run: func([]string) ([]byte, error) { return nil, nil }}
	d.runner = fake
	_, err := d.readStartLog(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, fake.calls, 1)
	assert.Contains(t, fake.calls[0], "--tail")
	assert.Contains(t, fake.calls[0], "200")
	assert.Contains(t, fake.calls[0], "--since")
}

func TestDockerDeploymentRestartsContainerForEnrollmentRetry(t *testing.T) {
	d := newDeployDocker("livingroom-node", "image", "/operators/fleet", nil)
	dir := "/operators/fleet/op-00001"
	op := d.spec(dir, nil)
	d.operators[dir] = op
	fake := &fakeDockerRunner{run: func([]string) ([]byte, error) { return nil, nil }}
	d.runner = fake

	require.NoError(t, d.startOperator(context.Background(), dir, "gateway"))
	require.NoError(t, d.startOperator(context.Background(), dir, "gateway"))

	require.Len(t, fake.calls, 3)
	assert.Contains(t, strings.Join(fake.calls[0], " "), "container start "+op.container)
	assert.Contains(t, strings.Join(fake.calls[1], " "), "container stop --time 10 "+op.container)
	assert.Contains(t, strings.Join(fake.calls[2], " "), "container start "+op.container)
}

func TestDockerDeploymentDiscoversProgressWithoutReadingLogs(t *testing.T) {
	d := newDeployDocker("host", "image", "/operators", nil)
	dir := "/operators/op-00001"
	op := d.spec(dir, nil)
	op.launchID = "current-launch"
	d.operators[dir] = op
	state := models.OperatorDeploymentState{
		LaunchID: op.launchID, Phase: models.OperatorDeploymentPhasePendingApproval,
		RequestID: "request", UpdatedAt: time.Now().UTC(),
	}
	fake := &fakeDockerRunner{run: func(args []string) ([]byte, error) {
		line := strings.Join(args, " ")
		if strings.Contains(line, "container inspect") {
			return []byte("true 0"), nil
		}
		require.Contains(t, line, "container exec "+op.container+" ./g8e operator deployment-state --working-dir "+dir)
		return json.Marshal(state)
	}}
	d.runner = fake
	requestID, err := d.awaitRequestID(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "request", requestID)
	state.Phase = models.OperatorDeploymentPhaseReady
	state.OperatorSessionID = "session"
	sessionID, err := d.awaitSessionID(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "session", sessionID)
	require.NoError(t, d.awaitReady(context.Background(), dir))
	state.LaunchID = "old-launch"
	progress, err := d.readDeploymentState(context.Background(), dir)
	require.NoError(t, err)
	require.Nil(t, progress)
	assert.NotContains(t, joinedDockerCalls(fake.calls), "container logs")
}
