// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderResponseCapture_IngestsRawTailWhileProgressDeliveryIsBlocked(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader, writer := io.Pipe()
		defer reader.Close()
		defer writer.Close()
		first := `{"model":"test-model","message":{"content":"hello"}}` + "\n"
		tail := `{"model":"test-model","done":true,"done_reason":"stop"}` + "\n"
		capture := newProviderResponseCapture(defaultMaxResponseBytes, time.Now())
		go capture.receive(reader)
		backend, err := NewOllamaBackend("http://localhost:11434", testutil.NewTestLogger())
		require.NoError(t, err)
		reporting := make(chan struct{})
		release := make(chan struct{})
		decoded := make(chan error, 1)
		go func() {
			_, _, _, err := backend.decodeChatStream(capture, capture.arrivalTime, true, nil, "attempt-blocked",
				func(*operatorv1.InferenceProgressEvent) error {
					close(reporting)
					<-release
					return nil
				})
			decoded <- err
		}()
		written := make(chan error, 1)
		go func() {
			_, err := io.WriteString(writer, first)
			if err == nil {
				<-reporting
				_, err = io.WriteString(writer, tail)
			}
			_ = writer.Close()
			written <- err
		}()
		<-reporting
		synctest.Wait()
		select {
		case <-capture.finished:
			raw, err := capture.result()
			assert.NoError(t, err)
			assert.Equal(t, first+tail, string(raw))
		default:
			t.Error("progress delivery blocked provider ingestion")
		}
		close(release)
		assert.NoError(t, <-written)
		assert.NoError(t, <-decoded)
	})
}
