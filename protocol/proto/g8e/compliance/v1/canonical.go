// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliancev1

import (
	"bytes"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var canonicalMarshalOptions = protojson.MarshalOptions{UseProtoNames: true}
var canonicalUnmarshalOptions = protojson.UnmarshalOptions{DiscardUnknown: false}

func MarshalCanonical(message proto.Message) ([]byte, error) {
	encoded, err := canonicalMarshalOptions.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("compliance protocol: marshal canonical protojson: %w", err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, encoded); err != nil {
		return nil, fmt.Errorf("compliance protocol: compact canonical protojson: %w", err)
	}
	return compact.Bytes(), nil
}

func UnmarshalCanonical(encoded []byte, message proto.Message) error {
	if err := canonicalUnmarshalOptions.Unmarshal(encoded, message); err != nil {
		return fmt.Errorf("compliance protocol: unmarshal canonical protojson: %w", err)
	}
	canonical, err := MarshalCanonical(message)
	if err != nil {
		return err
	}
	if !bytes.Equal(encoded, canonical) {
		index := 0
		for index < len(encoded) && index < len(canonical) && encoded[index] == canonical[index] {
			index++
		}
		return fmt.Errorf("compliance protocol: input is not canonical protojson at byte %d", index)
	}
	return nil
}
