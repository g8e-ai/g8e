package pubsub

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	execution "github.com/g8e-ai/g8e/internal/services/execution"
	pubsubtest "github.com/g8e-ai/g8e/internal/services/pubsub/pubsubtest"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestHandleFsListRequest_ValidPath(t *testing.T) {
	t.Parallel()
	tmpDir := testutil.TempDir(t)

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
	tmpDir := testutil.TempDir(t)

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
	tmpDir := testutil.TempDir(t)
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
	tmpDir := testutil.TempDir(t)
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
	tmpDir := testutil.TempDir(t)

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
	tmpDir := testutil.TempDir(t)
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
	tmpDir := testutil.TempDir(t)
	req := &operatorv1.FileEditRequested{
		FilePath:  filepath.Join(tmpDir, "test.txt"),
		Operation: "write",
		Content:   "test",
	}
	payload, _ := proto.Marshal(req)
	taskID := "task-123"
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.FileEdit.Requested,
		TaskID:    &taskID,
		Payload:   payload,
	}

	editReq, err := payloadToFileEditRequest(msg)
	assert.NoError(t, err)
	assert.Equal(t, "task-123", editReq.TaskID)
}

func TestHandleFsReadRequest_ScrubbingRedactsSecrets(t *testing.T) {
	t.Parallel()
	tmpDir := testutil.TempDir(t)
	testFile := filepath.Join(tmpDir, "secret.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("password=secret123 api_key=ghp_test_token"), 0644))

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	client := pubsubtest.NewMockOperatorPubSubClient()
	fileEditSvc := execution.NewFileEditService(cfg, logger)
	svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

	scrubbingSvc, err := scrubbing.NewScrubbingService(context.Background(), scrubbing.DefaultConfig(), logger, nil)
	require.NoError(t, err)
	svc.SetScrubbingService(scrubbingSvc)

	req := &operatorv1.FsReadRequested{Path: testFile}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.FsRead.Requested,
		Payload:   payload,
	}

	svc.HandleFsReadRequest(context.Background(), msg)

	published := client.LastPublished()
	require.NotNil(t, published)

	var env commonv1.GovernanceEnvelope
	require.NoError(t, protojson.Unmarshal(published.Data, &env))

	var readResult operatorv1.FsReadResult
	require.NoError(t, proto.Unmarshal(env.Payload, &readResult))
	assert.NotContains(t, readResult.Content, "secret123", "password value must be redacted")
	assert.NotContains(t, readResult.Content, "ghp_test_token", "api key must be redacted")
}

func TestHandleFsReadRequest_TruncatesLargeFile(t *testing.T) {
	t.Parallel()
	tmpDir := testutil.TempDir(t)
	testFile := filepath.Join(tmpDir, "large.txt")
	largeContent := strings.Repeat("A", 2048)
	require.NoError(t, os.WriteFile(testFile, []byte(largeContent), 0644))

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	client := pubsubtest.NewMockOperatorPubSubClient()
	fileEditSvc := execution.NewFileEditService(cfg, logger)
	svc := NewFileOpsService(cfg, logger, fileEditSvc, client)

	req := &operatorv1.FsReadRequested{Path: testFile, MaxSize: 100}
	payload, _ := proto.Marshal(req)
	msg := &PubSubCommandMessage{
		ID:        "msg-1",
		EventType: constants.Event.Operator.FsRead.Requested,
		Payload:   payload,
	}

	svc.HandleFsReadRequest(context.Background(), msg)

	published := client.LastPublished()
	require.NotNil(t, published)

	var env commonv1.GovernanceEnvelope
	require.NoError(t, protojson.Unmarshal(published.Data, &env))

	var readResult operatorv1.FsReadResult
	require.NoError(t, proto.Unmarshal(env.Payload, &readResult))
	assert.True(t, readResult.Truncated, "file larger than max_size must be truncated")
	assert.Equal(t, int64(100), readResult.SizeBytes)
}
