package pubsub

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	execution "github.com/g8e-ai/g8e/internal/services/execution"
	pubsubtest "github.com/g8e-ai/g8e/internal/services/pubsub/pubsubtest"
	"github.com/g8e-ai/g8e/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestHandleFsListRequest_ValidPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	client := pubsubtest.NewMockOperatorPubSubClient()
	fileEditSvc := execution.NewFileEditService(cfg, logger)
	svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

	req := &operatorv1.FsListRequested{Path: tmpDir, MaxDepth: 1, MaxEntries: 10}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.FsList.Requested,
		Payload:   payload,
	}

	svc.HandleFsListRequest(context.Background(), msg)
}

func TestHandleFsListRequest_WithVaultWriter(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	client := pubsubtest.NewMockOperatorPubSubClient()
	fileEditSvc := execution.NewFileEditService(cfg, logger)
	svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

	svc.vaultWriter = NewVaultWriter(cfg, logger, nil)

	req := &operatorv1.FsListRequested{Path: tmpDir}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.FsList.Requested,
		Payload:   payload,
	}

	svc.HandleFsListRequest(context.Background(), msg)
}

func TestHandleFsGrepRequest_ValidPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world\ntest line\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	client := pubsubtest.NewMockOperatorPubSubClient()
	fileEditSvc := execution.NewFileEditService(cfg, logger)
	svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

	req := &operatorv1.FsGrepRequested{Path: tmpDir, Pattern: "hello", MaxMatches: 10}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.FsGrep.Requested,
		Payload:   payload,
	}

	svc.HandleFsGrepRequest(context.Background(), msg)
}

func TestHandleFsReadRequest_ValidPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("file content"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	client := pubsubtest.NewMockOperatorPubSubClient()
	fileEditSvc := execution.NewFileEditService(cfg, logger)
	svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

	req := &operatorv1.FsReadRequested{Path: testFile}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.FsRead.Requested,
		Payload:   payload,
	}

	svc.HandleFsReadRequest(context.Background(), msg)
}

func TestHandleFsReadRequest_NonExistentPath(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	client := pubsubtest.NewMockOperatorPubSubClient()
	fileEditSvc := execution.NewFileEditService(cfg, logger)
	svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

	req := &operatorv1.FsReadRequested{Path: "/nonexistent/file/path"}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.FsRead.Requested,
		Payload:   payload,
	}

	svc.HandleFsReadRequest(context.Background(), msg)
}

func TestHandleFileEditRequest_InvalidOperation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	client := pubsubtest.NewMockOperatorPubSubClient()
	fileEditSvc := execution.NewFileEditService(cfg, logger)
	svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

	req := &operatorv1.FileEditRequested{
		FilePath:  filepath.Join(tmpDir, "test.txt"),
		Operation: "invalid_op",
	}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.FileEdit.Requested,
		Payload:   payload,
	}

	svc.HandleFileEditRequest(context.Background(), msg)
}

func TestHandleFsListRequest_InvalidPath(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	client := pubsubtest.NewMockOperatorPubSubClient()
	fileEditSvc := execution.NewFileEditService(cfg, logger)
	svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

	req := &operatorv1.FsListRequested{Path: "/nonexistent/path/that/does/not/exist"}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.FsList.Requested,
		Payload:   payload,
	}

	svc.HandleFsListRequest(context.Background(), msg)
}

func TestPayloadToFileEditRequest_InvalidOperation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	req := &operatorv1.FileEditRequested{
		FilePath:  filepath.Join(tmpDir, "test.txt"),
		Operation: "invalid",
	}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.FileEdit.Requested,
		Payload:   payload,
	}

	_, err := payloadToFileEditRequest(msg)
	assert.Error(t, err)
}

func TestPayloadToFileEditRequest_WithTaskID(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	req := &operatorv1.FileEditRequested{
		FilePath:  filepath.Join(tmpDir, "test.txt"),
		Operation: "write",
		Content:   "test",
	}
	payload, _ := proto.Marshal(req)
	taskID := "task-123"
	msg := &PubSubCommandMessage{
		ID:              "msg-1",
		EventType:       constants.Event.Operator.FileEdit.Requested,
		TaskID:          &taskID,
		Payload:         payload,
	}

	editReq, err := payloadToFileEditRequest(msg)
	assert.NoError(t, err)
	assert.Equal(t, "task-123", editReq.TaskID)
}
