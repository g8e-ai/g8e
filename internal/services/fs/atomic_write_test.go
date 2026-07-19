package fs

import (
	"bytes"
	"context"
	"crypto/rand"
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteFile_LargeDataPreservesContent(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	data := make([]byte, 1024*1024) // 1MB
	_, err := rand.Read(data)
	require.NoError(t, err)

	require.NoError(t, svc.WriteFile(ctx, "large.bin", data, constants.PermFilePrivate))

	got, err := svc.ReadFile(ctx, "large.bin")
	require.NoError(t, err)
	assert.True(t, bytes.Equal(data, got))
}

func TestWriteFile_EmptyDataCreatesZeroLengthFile(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	require.NoError(t, svc.WriteFile(ctx, "empty.txt", []byte{}, constants.PermFilePrivate))

	info, err := svc.Stat(ctx, "empty.txt")
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.Size())
}

func TestWriteFile_ConcurrentWritesToSamePath(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	data1 := []byte("content-1")
	data2 := []byte("content-2")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = svc.WriteFile(ctx, "concurrent.txt", data1, constants.PermFilePrivate)
	}()
	go func() {
		defer wg.Done()
		_ = svc.WriteFile(ctx, "concurrent.txt", data2, constants.PermFilePrivate)
	}()
	wg.Wait()

	got, err := svc.ReadFile(ctx, "concurrent.txt")
	require.NoError(t, err)
	assert.True(t, bytes.Equal(data1, got) || bytes.Equal(data2, got),
		"file should contain one of the two written contents")
}

func TestWriteFile_NestedDeepPath(t *testing.T) {
	svc := setupTestFS(t)
	ctx := context.Background()

	deepPath := "a/b/c/d/e/file.txt"
	require.NoError(t, svc.WriteFile(ctx, deepPath, []byte("deep"), constants.PermFilePrivate))

	got, err := svc.ReadFile(ctx, deepPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("deep"), got)
}
