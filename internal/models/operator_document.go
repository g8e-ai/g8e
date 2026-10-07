// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// The canonical OperatorDocument is the protobuf message
// operatorv1.OperatorDocument. These helpers are its only JSON codec: the
// document store, HTTP responses and clients all use the protojson form with
// proto field names, never encoding/json over the generated struct.

var (
	operatorDocumentMarshal = protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}
	// Stored documents are merged field-by-field by the document store and may
	// carry keys owned by other writers, so unknown keys are tolerated on read.
	operatorDocumentUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}
)

// MarshalOperatorDocument encodes an OperatorDocument in canonical JSON.
func MarshalOperatorDocument(doc *operatorv1.OperatorDocument) ([]byte, error) {
	return operatorDocumentMarshal.Marshal(doc)
}

// UnmarshalOperatorDocument decodes canonical JSON into an OperatorDocument and
// validates the role set it carries.
func UnmarshalOperatorDocument(data []byte) (*operatorv1.OperatorDocument, error) {
	doc := &operatorv1.OperatorDocument{}
	if err := operatorDocumentUnmarshal.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("decode operator document: %w", err)
	}
	if err := OperatorRolesFromProto(doc.GetOperatorRoles()).Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

// OperatorRolesFromProto converts the proto role strings to the typed set.
func OperatorRolesFromProto(roles []string) constants.OperatorRoles {
	out := make(constants.OperatorRoles, 0, len(roles))
	for _, role := range roles {
		out = append(out, constants.OperatorRole(role))
	}
	return out
}

// OperatorRolesToProto converts a typed role set to the proto role strings.
func OperatorRolesToProto(roles constants.OperatorRoles) []string {
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, string(role))
	}
	return out
}

func marshalOperatorDocuments(docs []*operatorv1.OperatorDocument) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(docs))
	for _, doc := range docs {
		raw, err := MarshalOperatorDocument(doc)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}

func unmarshalOperatorDocuments(raws []json.RawMessage) ([]*operatorv1.OperatorDocument, error) {
	out := make([]*operatorv1.OperatorDocument, 0, len(raws))
	for _, raw := range raws {
		doc, err := UnmarshalOperatorDocument(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}

// MarshalJSON encodes the embedded documents with the canonical codec.
func (r OperatorSlotResponse) MarshalJSON() ([]byte, error) {
	docs, err := marshalOperatorDocuments(r.Operators)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Success   bool              `json:"success"`
		Operators []json.RawMessage `json:"operators"`
	}{r.Success, docs})
}

func (r *OperatorSlotResponse) UnmarshalJSON(data []byte) error {
	var wire struct {
		Success   bool              `json:"success"`
		Operators []json.RawMessage `json:"operators"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	docs, err := unmarshalOperatorDocuments(wire.Operators)
	if err != nil {
		return err
	}
	r.Success, r.Operators = wire.Success, docs
	return nil
}

// MarshalJSON encodes the embedded document with the canonical codec.
func (r OperatorResponse) MarshalJSON() ([]byte, error) {
	wire := struct {
		Success  bool            `json:"success"`
		Operator json.RawMessage `json:"operator,omitempty"`
	}{Success: r.Success}
	if r.Operator != nil {
		raw, err := MarshalOperatorDocument(r.Operator)
		if err != nil {
			return nil, err
		}
		wire.Operator = raw
	}
	return json.Marshal(wire)
}

func (r *OperatorResponse) UnmarshalJSON(data []byte) error {
	var wire struct {
		Success  bool            `json:"success"`
		Operator json.RawMessage `json:"operator,omitempty"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	r.Success, r.Operator = wire.Success, nil
	if len(wire.Operator) > 0 && string(wire.Operator) != "null" {
		doc, err := UnmarshalOperatorDocument(wire.Operator)
		if err != nil {
			return err
		}
		r.Operator = doc
	}
	return nil
}

// MarshalJSON encodes the embedded document with the canonical codec.
func (r ReauthResponse) MarshalJSON() ([]byte, error) {
	wire := struct {
		Success  bool            `json:"success"`
		Operator json.RawMessage `json:"operator"`
	}{Success: r.Success, Operator: json.RawMessage("null")}
	if r.Operator != nil {
		raw, err := MarshalOperatorDocument(r.Operator)
		if err != nil {
			return nil, err
		}
		wire.Operator = raw
	}
	return json.Marshal(wire)
}

func (r *ReauthResponse) UnmarshalJSON(data []byte) error {
	var wire struct {
		Success  bool            `json:"success"`
		Operator json.RawMessage `json:"operator"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	r.Success, r.Operator = wire.Success, nil
	if len(wire.Operator) > 0 && string(wire.Operator) != "null" {
		doc, err := UnmarshalOperatorDocument(wire.Operator)
		if err != nil {
			return err
		}
		r.Operator = doc
	}
	return nil
}
