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
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

const compliancePhase1MessageCount = 20
const compliancePhase2DemoMessageCount = 7

type complianceCanonicalizationVector struct {
	Message       json.RawMessage `json:"message"`
	CanonicalJSON string          `json:"canonical_json"`
}

type compliancePhase1Vector struct {
	MessageType   string `json:"message_type"`
	CanonicalJSON string `json:"canonical_json"`
}

type compliancePhase1VectorSet struct {
	Vectors []compliancePhase1Vector `json:"vectors"`
}

//go:embed vectors/compliance/control_assertion_definition.json
var complianceCanonicalizationVectorJSON []byte

//go:embed vectors/compliance/phase1_records.json
var compliancePhase1VectorJSON []byte

//go:embed vectors/compliance/phase2_demo_records.json
var compliancePhase2DemoVectorJSON []byte

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

func TestCompliancePhase1RecordsMatchCrossLanguageVectors(t *testing.T) {
	validateComplianceVectorSet(t, compliancePhase1VectorJSON, compliancePhase1MessageCount)
}

func TestCompliancePhase2DemoRecordsMatchCrossLanguageVectors(t *testing.T) {
	validateComplianceVectorSet(t, compliancePhase2DemoVectorJSON, compliancePhase2DemoMessageCount)
}

func validateComplianceVectorSet(t *testing.T, encodedVectorSet []byte, expectedCount int) {
	t.Helper()
	var vectorSet compliancePhase1VectorSet
	require.NoError(t, json.Unmarshal(encodedVectorSet, &vectorSet))
	require.Len(t, vectorSet.Vectors, expectedCount)
	seen := make(map[string]struct{}, len(vectorSet.Vectors))

	for _, vector := range vectorSet.Vectors {
		_, exists := seen[vector.MessageType]
		require.False(t, exists, "duplicate vector for %s", vector.MessageType)
		seen[vector.MessageType] = struct{}{}
		t.Run(vector.MessageType, func(t *testing.T) {
			fullName := protoreflect.FullName("g8e.compliance.v1." + vector.MessageType)
			messageType, err := protoregistry.GlobalTypes.FindMessageByName(fullName)
			require.NoError(t, err)
			message := messageType.New().Interface()
			encoded := []byte(vector.CanonicalJSON)
			require.NoError(t, compliancev1.UnmarshalCanonical(encoded, message))

			canonical, err := compliancev1.MarshalCanonical(message)
			require.NoError(t, err)
			assert.Equal(t, encoded, canonical)
		})
	}
}
