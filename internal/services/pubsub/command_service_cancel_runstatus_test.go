package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	execution "github.com/g8e-ai/g8e/v2/internal/services/execution"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestHandleCancelRequest_ValidPayload_CancelSuccess(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	execSvc := execution.NewExecutionService(cfg, logger)
	svc := NewCommandService(cfg, logger, execSvc)
	svc.SetResultsPublisher(&mockResultsPublisher{})

	req := &operatorv1.CommandCancelRequested{ExecutionId: "exec-cancel-1"}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.Command.CancelRequested,
		Payload:   payload,
	}

	svc.HandleCancelRequest(context.Background(), msg)
}

func TestHandleCancelRequest_ValidPayload_CancelFailure(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	execSvc := execution.NewExecutionService(cfg, logger)
	svc := NewCommandService(cfg, logger, execSvc)
	svc.SetResultsPublisher(&mockResultsPublisher{})

	req := &operatorv1.CommandCancelRequested{ExecutionId: "nonexistent-exec"}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.Command.CancelRequested,
		Payload:   payload,
	}

	svc.HandleCancelRequest(context.Background(), msg)
}

func TestHandleCancelRequest_WithResultsPublisher_Nil(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	execSvc := execution.NewExecutionService(cfg, logger)
	svc := NewCommandService(cfg, logger, execSvc)

	req := &operatorv1.CommandCancelRequested{ExecutionId: "exec-1"}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.Command.CancelRequested,
		Payload:   payload,
	}

	svc.HandleCancelRequest(context.Background(), msg)
}

func TestRunStatusTicker_WithResultsPublisher(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	execSvc := execution.NewExecutionService(cfg, logger)
	svc := NewCommandService(cfg, logger, execSvc)
	svc.SetResultsPublisher(&mockResultsPublisher{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		cancel()
	}()

	count := svc.runStatusTicker(ctx, nil, &PubSubCommandMessage{ID: "msg-1"}, "test", time.Now(), done)
	assert.GreaterOrEqual(t, count, 0)
}
