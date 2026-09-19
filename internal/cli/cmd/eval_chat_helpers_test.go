// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestChatAcceptReporter_WritesEnabledMessages(t *testing.T) {
	var output bytes.Buffer
	reporter := &chatAcceptReporter{out: &output, quiet: false}
	reporter.writeSetup(2, "qwen3:4b", "inf-1", "data-1", "http://127.0.0.1:8000")
	reporter.caseStart(1, 2, "case-a", "assign-1", "attempt-1")
	reporter.chatSubmitted("case-a", "investigation-1")
	reporter.chatSubmitFailed(errors.New("submit failed"))
	reporter.traceWaiting()
	reporter.traceFetchRetrying(errors.New("pending"), 2*time.Second)
	reporter.traceFetchFailed(errors.New("lookup failed"))

	text := output.String()
	assert.Contains(t, text, "Phase 1A chat acceptance")
	assert.Contains(t, text, "qwen3:4b")
	assert.Contains(t, text, "assignment_id=assign-1")
	assert.Contains(t, text, "investigation_id=investigation-1")
	assert.Contains(t, text, "chat submit failed")
	assert.Contains(t, text, "trace lookup failed")
}

func TestChatAcceptReporter_SilentWhenDisabled(t *testing.T) {
	var output bytes.Buffer
	reporter := &chatAcceptReporter{out: &output, quiet: true}
	reporter.writeSetup(1, "qwen3:4b", "inf-1", "data-1", "http://127.0.0.1:8000")
	assert.Empty(t, output.String())
}
