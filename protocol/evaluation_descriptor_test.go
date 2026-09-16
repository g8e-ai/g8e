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
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

//go:embed descriptors/eval/v1/model_campaign.json
var evaluationModelCampaignDescriptorJSON []byte

type evaluationModelCampaignDescriptor struct {
	SchemaVersion    string                                 `json:"schema_version"`
	ProtobufPackage  string                                 `json:"protobuf_package"`
	Enums            map[string][]string                    `json:"enums"`
	Messages         map[string]evaluationMessageDescriptor `json:"messages"`
	CanonicalVectors []evaluationCanonicalVectorBinding     `json:"canonical_vectors"`
}

type evaluationMessageDescriptor struct {
	Visibility       string   `json:"visibility"`
	Fields           []string `json:"fields"`
	ProhibitedFields []string `json:"prohibited_fields"`
	OneofGroups      []string `json:"oneof_groups"`
}

type evaluationCanonicalVectorBinding struct {
	MessageType string `json:"message_type"`
	VectorPath  string `json:"vector_path"`
}

func TestEvaluationModelCampaignDescriptorMatchesProtobuf(t *testing.T) {
	var descriptor evaluationModelCampaignDescriptor
	require.NoError(t, json.Unmarshal(evaluationModelCampaignDescriptorJSON, &descriptor))
	assert.Equal(t, "1.0.0", descriptor.SchemaVersion)
	assert.Equal(t, "g8e.eval.v1", descriptor.ProtobufPackage)
	require.NotEmpty(t, descriptor.Enums)
	require.NotEmpty(t, descriptor.Messages)
	require.Len(t, descriptor.CanonicalVectors, 4)

	for enumName, values := range descriptor.Enums {
		t.Run("enum/"+enumName, func(t *testing.T) {
			enumType, err := protoregistry.GlobalTypes.FindEnumByName(
				protoreflect.FullName(descriptor.ProtobufPackage + "." + enumName),
			)
			require.NoError(t, err)
			liveValues := enumValueNames(enumType.Descriptor())
			assert.Equal(t, liveValues, values)
		})
	}

	for messageName, messageDescriptor := range descriptor.Messages {
		t.Run("message/"+messageName, func(t *testing.T) {
			messageType, err := protoregistry.GlobalTypes.FindMessageByName(
				protoreflect.FullName(descriptor.ProtobufPackage + "." + messageName),
			)
			require.NoError(t, err)
			liveFields, liveOneofs := messageFieldNames(messageType.Descriptor())
			assert.Equal(t, messageDescriptor.Fields, liveFields)
			assert.Equal(t, normalizeStringSlice(messageDescriptor.OneofGroups), normalizeStringSlice(liveOneofs))

			switch messageDescriptor.Visibility {
			case "public":
				assert.True(t, isPublicModelCampaignMessage(messageName))
			case "private":
				assert.False(t, isPublicModelCampaignMessage(messageName))
			default:
				t.Fatalf("unexpected visibility %q", messageDescriptor.Visibility)
			}

			for _, prohibited := range messageDescriptor.ProhibitedFields {
				assert.Nil(t, messageType.Descriptor().Fields().ByName(protoreflect.Name(prohibited)))
			}
		})
	}
}

func TestEvaluationModelCampaignDescriptorRegistersRequiredMessages(t *testing.T) {
	var descriptor evaluationModelCampaignDescriptor
	require.NoError(t, json.Unmarshal(evaluationModelCampaignDescriptorJSON, &descriptor))

	required := []string{
		"EvaluationCampaignSpec",
		"EvaluationScenarioCatalog",
		"EvaluationScenarioDefinition",
		"ModelVariant",
		"EvaluationAssignment",
		"EvaluationAssignmentResult",
		"ModelInferenceRecord",
		"GovernedActionBinding",
		"EvaluationVerificationReport",
		"PublicAssignmentResultProjection",
	}
	for _, name := range required {
		_, ok := descriptor.Messages[name]
		assert.True(t, ok, "descriptor must include %s", name)
	}
}

func enumValueNames(descriptor protoreflect.EnumDescriptor) []string {
	values := descriptor.Values()
	names := make([]string, 0, values.Len())
	for index := 0; index < values.Len(); index++ {
		names = append(names, string(values.Get(index).Name()))
	}
	sort.Strings(names)
	return names
}

func messageFieldNames(descriptor protoreflect.MessageDescriptor) ([]string, []string) {
	fields := descriptor.Fields()
	fieldNames := make([]string, 0, fields.Len())
	oneofGroups := make([]string, 0)
	for index := 0; index < fields.Len(); index++ {
		field := fields.Get(index)
		if field.ContainingOneof() != nil && !field.ContainingOneof().IsSynthetic() {
			oneofName := string(field.ContainingOneof().Name())
			if !containsString(oneofGroups, oneofName) {
				oneofGroups = append(oneofGroups, oneofName)
			}
			continue
		}
		fieldNames = append(fieldNames, string(field.Name()))
	}
	sort.Strings(fieldNames)
	sort.Strings(oneofGroups)
	return fieldNames, oneofGroups
}

func isPublicModelCampaignMessage(name string) bool {
	return name == "PublicCampaignIdentity" ||
		name == "PublicModelVariantIdentity" ||
		name == "PublicAssignmentLifecycleRecord" ||
		name == "PublicModelCallSummary" ||
		name == "PublicAssignmentResultProjection"
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return values
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
