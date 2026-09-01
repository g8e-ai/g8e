// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package protocol_test

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type complianceCanonicalizationVector struct {
	Message       json.RawMessage `json:"message"`
	CanonicalJSON string          `json:"canonical_json"`
}

//go:embed vectors/compliance/control_assertion_definition.json
var complianceCanonicalizationVectorJSON []byte

func TestComplianceCanonicalizationMatchesCrossLanguageVector(t *testing.T) {
	var vector complianceCanonicalizationVector
	require.NoError(t, json.Unmarshal(complianceCanonicalizationVectorJSON, &vector))
	message := &compliancev1.ControlAssertionDefinition{}
	require.NoError(t, (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(vector.Message, message))

	encoded, err := compliancev1.MarshalCanonical(message)
	require.NoError(t, err)
	assert.Equal(t, vector.CanonicalJSON, string(encoded))

	reparsed := &compliancev1.ControlAssertionDefinition{}
	require.NoError(t, compliancev1.UnmarshalCanonical(encoded, reparsed))
	assert.True(t, proto.Equal(message, reparsed))
}
