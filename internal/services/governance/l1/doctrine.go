// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package l1

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// DoctrineValidator provides L1 (Technical Bedrock) validation.
// L1 is the foundational hard gate that enforces forbidden patterns,
// blacklist/whitelist rules, and intent validation.
type DoctrineValidator interface {
	// ValidatePayload checks a typed protobuf payload for L1 violations.
	// Returns a list of violation descriptions (empty if valid).
	ValidatePayload(msg proto.Message) []string

	// ValidateIntent checks if a cloud intent is allowed by doctrine.
	// Returns true if the intent is in the allowlist.
	ValidateIntent(intent constants.CloudIntent) bool
}

// ProtoDoctrineValidator implements DoctrineValidator using protobuf field options.
// It checks the (g8e.common.v1).forbidden_patterns extension on string fields.
type ProtoDoctrineValidator struct{}

// NewProtoDoctrineValidator creates a new protobuf-based doctrine validator.
func NewProtoDoctrineValidator() *ProtoDoctrineValidator {
	return &ProtoDoctrineValidator{}
}

// ValidatePayload checks a typed protobuf payload for forbidden pattern violations.
func (v *ProtoDoctrineValidator) ValidatePayload(msg proto.Message) []string {
	var violations []string
	md := msg.ProtoReflect().Descriptor()
	fields := md.Fields()

	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		opts := fd.Options()
		if opts == nil || !proto.HasExtension(opts, commonv1.E_ForbiddenPatterns) {
			continue
		}
		patternsStr, ok := proto.GetExtension(opts, commonv1.E_ForbiddenPatterns).(string)
		if !ok || patternsStr == "" {
			continue
		}
		val := msg.ProtoReflect().Get(fd)
		if fd.Kind() != protoreflect.StringKind {
			continue
		}
		strVal := val.String()
		for _, p := range strings.Split(patternsStr, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			matched, err := regexp.MatchString(p, strVal)
			if err == nil && matched {
				violations = append(violations, fmt.Sprintf("field %s violates pattern %s", fd.Name(), p))
			}
		}
	}

	return violations
}

// ValidateIntent is a placeholder for intent-based doctrine validation.
// This will be integrated with Sentinel's intent allowlist in a follow-up.
func (v *ProtoDoctrineValidator) ValidateIntent(intent constants.CloudIntent) bool {
	// TODO: Integrate with Sentinel's intent validation
	// For now, allow all intents - this is a temporary bridge
	return true
}
