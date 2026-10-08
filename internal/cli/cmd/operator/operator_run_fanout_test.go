// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/cli/api"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// inflightClient counts concurrent Post calls.
type inflightClient struct {
	authcmd.APIClient
	postResp []byte
	hold     time.Duration
	inflight atomic.Int32
	peak     atomic.Int32
}

func (c *inflightClient) Post(string, interface{}) ([]byte, error) {
	now := c.inflight.Add(1)
	defer c.inflight.Add(-1)
	for {
		peak := c.peak.Load()
		if now <= peak || c.peak.CompareAndSwap(peak, now) {
			break
		}
	}
	time.Sleep(c.hold)
	return c.postResp, nil
}

func fanoutTargets(n int) []operatorRunTarget {
	targets := make([]operatorRunTarget, n)
	for i := range targets {
		targets[i] = operatorRunTarget{OperatorID: fmt.Sprintf("op-%d", i), OperatorSessionID: fmt.Sprintf("session-%d", i)}
	}
	return targets
}

func TestDispatchOperatorRunNeverExceedsConcurrency(t *testing.T) {
	client := &inflightClient{postResp: mustMarshalDispatchSuccess(t, "ok"), hold: 5 * time.Millisecond}

	results := dispatchOperatorRun(client, fanoutTargets(40), "uname -n", "cli-1", 4)

	require.Len(t, results, 40)
	assert.LessOrEqual(t, client.peak.Load(), int32(4))
	assert.Greater(t, client.peak.Load(), int32(1), "dispatch should actually run in parallel")
}

func TestDispatchOperatorRunKeepsTargetOrderAndSetsTiming(t *testing.T) {
	client := &inflightClient{postResp: mustMarshalDispatchSuccess(t, "ok"), hold: 2 * time.Millisecond}
	targets := fanoutTargets(12)

	results := dispatchOperatorRun(client, targets, "uname -n", "cli-1", 3)

	for i, result := range results {
		assert.Equal(t, targets[i].OperatorSessionID, result.OperatorSessionID)
		assert.True(t, result.Success)
		assert.False(t, result.StartedAt.IsZero())
		assert.GreaterOrEqual(t, result.DurationMs, 2.0)
	}
}

func fanoutFixture(t *testing.T, operators []*operatorv1.OperatorDocument) (*cobra.Command, *cmdtest.MockAPIClient, *[]api.ClientOptions) {
	t.Helper()
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")
	listBody, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: operators})
	require.NoError(t, err)
	mockClient := &cmdtest.MockAPIClient{GetResp: listBody, PostResp: mustMarshalDispatchSuccess(t, "host")}
	var seen []api.ClientOptions
	cmd := operatorRunCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		func(_ fs.RuntimeFileService, _ *config.Config, opts api.ClientOptions) (authcmd.APIClient, error) {
			seen = append(seen, opts)
			return mockClient, nil
		},
		cmdtest.FileSvcFactoryFor(fileSvc),
	)
	return cmd, mockClient, &seen
}

func fanoutOperators() []*operatorv1.OperatorDocument {
	remote := string(constants.OperatorTypeRemote)
	return []*operatorv1.OperatorDocument{
		{Id: "op-a", OperatorSessionId: "session-a", OperatorType: remote, Status: string(constants.OperatorStatusActive)},
		{Id: "op-b", OperatorSessionId: "session-b", OperatorType: remote, Status: string(constants.OperatorStatusStale)},
		{Id: "op-c", OperatorSessionId: "session-c", OperatorType: remote, Status: string(constants.OperatorStatusActive)},
		{Id: "op-d", OperatorType: remote, Status: string(constants.OperatorStatusActive)},
	}
}

func TestOperatorRunCmd_AllActiveTargetsOnlyActiveSessionsAndEmitsSummary(t *testing.T) {
	cmd, mockClient, seen := fanoutFixture(t, fanoutOperators())
	root := &cobra.Command{Use: "g8e"}
	root.PersistentFlags().Bool("json", false, "")
	root.AddCommand(cmd)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"run", "--all-active", "--cmd", "uname -n", "--concurrency", "8", "--json"})

	require.NoError(t, root.Execute())

	assert.Len(t, mockClient.PostCalls, 2)
	var out operatorRunJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	require.Len(t, out.Results, 2)
	assert.Equal(t, "session-a", out.Results[0].OperatorSessionID)
	assert.Equal(t, "session-c", out.Results[1].OperatorSessionID)
	assert.Equal(t, 2, out.Summary.Targets)
	assert.Equal(t, 2, out.Summary.Succeeded)
	assert.Equal(t, 2, out.Summary.Concurrency, "concurrency is capped at the number of targets")
	require.Len(t, *seen, 1)
	assert.Equal(t, 8, (*seen)[0].MaxIdleConnsPerHost, "connection pool is sized to the requested concurrency")
}

func TestOperatorRunCmd_DefaultConcurrencyIsBoundedAndTextOutputHasNoSummary(t *testing.T) {
	cmd, _, seen := fanoutFixture(t, fanoutOperators())
	cmd.SetArgs([]string{"session-a", "--cmd", "uname -n"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Execute())

	require.Len(t, *seen, 1)
	assert.Equal(t, defaultOperatorCommandConcurrency, (*seen)[0].MaxIdleConnsPerHost)
	assert.Contains(t, buf.String(), "Operator session session-a (operator op-a)")
	assert.NotContains(t, buf.String(), "summary")
}

func TestOperatorRunCmd_RejectsConflictingOrInvalidSelection(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want error
	}{
		{"all-active with session id", []string{"session-a", "--all-active", "--cmd", "x"}, constants.ErrOperatorRunTargetSelection},
		{"neither session id nor all-active", []string{"--cmd", "x"}, constants.ErrOperatorRunTargetSelection},
		{"zero concurrency", []string{"session-a", "--cmd", "x", "--concurrency", "0"}, constants.ErrOperatorRunInvalidConcurrency},
		{"negative concurrency", []string{"--all-active", "--cmd", "x", "--concurrency", "-3"}, constants.ErrOperatorRunInvalidConcurrency},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, mockClient, seen := fanoutFixture(t, fanoutOperators())
			cmd.SetArgs(tt.args)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			err := cmd.Execute()

			require.ErrorIs(t, err, tt.want)
			assert.Empty(t, *seen)
			assert.Empty(t, mockClient.PostCalls)
		})
	}
}

func TestOperatorRunCmd_AllActiveWithNoActiveOperatorsFails(t *testing.T) {
	cmd, mockClient, _ := fanoutFixture(t, []*operatorv1.OperatorDocument{
		{Id: "op-b", OperatorSessionId: "session-b", OperatorType: string(constants.OperatorTypeRemote), Status: string(constants.OperatorStatusStale)},
	})
	cmd.SetArgs([]string{"--all-active", "--cmd", "x"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	require.ErrorIs(t, cmd.Execute(), constants.ErrOperatorRunNoActiveOperators)
	assert.Empty(t, mockClient.PostCalls)
}

func TestOperatorListCmd_JSONIncludesLastHeartbeatAt(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	saveTestCredentials(t, fileSvc, cfg, "user-001")
	beat := time.Date(2026, 10, 7, 12, 0, 5, 0, time.UTC)
	listBody, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: []*operatorv1.OperatorDocument{
		{Id: "op-a", OperatorSessionId: "session-a", OperatorType: string(constants.OperatorTypeRemote), Status: string(constants.OperatorStatusActive), LastHeartbeatAt: timestamppb.New(beat)},
		{Id: "op-b", OperatorSessionId: "session-b", OperatorType: string(constants.OperatorTypeRemote), Status: string(constants.OperatorStatusActive)},
	}})
	require.NoError(t, err)
	cmd := operatorListCmdWithConfig(
		func(string) (*config.Config, error) { return cfg, nil },
		authcmd.MockClientFactory(&cmdtest.MockAPIClient{GetResp: listBody}),
		cmdtest.FileSvcFactoryFor(fileSvc),
	)
	root := &cobra.Command{Use: "g8e"}
	root.PersistentFlags().Bool("json", false, "")
	root.AddCommand(cmd)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"list", "--json"})

	require.NoError(t, root.Execute())

	var out operatorListOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	require.Len(t, out.Operators, 2)
	require.NotNil(t, out.Operators[0].LastHeartbeatAt)
	assert.True(t, beat.Equal(*out.Operators[0].LastHeartbeatAt))
	assert.Nil(t, out.Operators[1].LastHeartbeatAt)
}
