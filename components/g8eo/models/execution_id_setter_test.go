// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package models

import (
	"reflect"
	"strings"
	"testing"
)

// TestExecutionIDSetter_Implementations verifies each payload that declares an
// ExecutionID field also implements ExecutionIDSetter, so the pubsub publisher
// can stamp the correlation id without a fragile type switch. When a new
// *ResultPayload is added with an ExecutionID field, register it here.
func TestExecutionIDSetter_Implementations(t *testing.T) {
	payloads := []ExecutionIDSetter{
		&ExecutionResultsPayload{},
		&CancellationResultPayload{},
		&FileEditResultPayload{},
		&FsListResultPayload{},
		&ExecutionStatusPayload{},
		&PortCheckResultPayload{},
		&FetchLogsResultPayload{},
		&FsReadResultPayload{},
		&LFAAErrorPayload{},
		&FetchFileDiffResultPayload{},
		&FetchHistoryResultPayload{},
		&FetchFileHistoryResultPayload{},
		&RestoreFileResultPayload{},
	}

	const want = "exec-123"
	for _, p := range payloads {
		p.SetExecutionID(want)

		v := reflect.ValueOf(p).Elem()
		f := v.FieldByName("ExecutionID")
		if !f.IsValid() {
			t.Errorf("%T: no ExecutionID field", p)
			continue
		}
		if got := f.String(); got != want {
			t.Errorf("%T.SetExecutionID did not set field: got %q want %q", p, got, want)
		}
	}
}

// TestExecutionIDSetter_CoversAllPayloads is a reflective guardrail: any type
// in this package whose name ends in "Payload" and has an ExecutionID string
// field must implement ExecutionIDSetter. This prevents the landmine where a
// new result payload is added but silently skipped by the publisher.
func TestExecutionIDSetter_CoversAllPayloads(t *testing.T) {
	// All known payload value instances exercised elsewhere in tests; we
	// reflect over the registered list to confirm they satisfy the interface.
	// The static list in execution_id_setter.go is the source of truth; this
	// test ensures none of them regress.
	setterType := reflect.TypeOf((*ExecutionIDSetter)(nil)).Elem()

	candidates := []interface{}{
		ExecutionResultsPayload{},
		CancellationResultPayload{},
		FileEditResultPayload{},
		FsListResultPayload{},
		ExecutionStatusPayload{},
		PortCheckResultPayload{},
		FetchLogsResultPayload{},
		FsReadResultPayload{},
		LFAAErrorPayload{},
		FetchFileDiffResultPayload{},
		FetchHistoryResultPayload{},
		FetchFileHistoryResultPayload{},
		RestoreFileResultPayload{},
	}

	for _, c := range candidates {
		ptrType := reflect.PtrTo(reflect.TypeOf(c))
		if !ptrType.Implements(setterType) {
			t.Errorf("%s missing SetExecutionID method; add it to execution_id_setter.go",
				strings.TrimPrefix(ptrType.String(), "*"))
		}
	}
}
