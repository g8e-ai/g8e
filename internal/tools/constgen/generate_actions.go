// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type actionTypeMeta struct {
	GoConst   string
	Value     string
	Mutation  bool
	Bootstrap bool
}

func loadActionTypeMeta(path string) (map[string]actionTypeMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Status struct {
			ActionType map[string]struct {
				GoConst   string `json:"_go_const"`
				Value     string `json:"value"`
				Mutation  bool   `json:"_mutation"`
				Bootstrap bool   `json:"_bootstrap"`
			} `json:"action_type"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	out := make(map[string]actionTypeMeta, len(raw.Status.ActionType))
	for key, meta := range raw.Status.ActionType {
		out[key] = actionTypeMeta{
			GoConst:   meta.GoConst,
			Value:     meta.Value,
			Mutation:  meta.Mutation,
			Bootstrap: meta.Bootstrap,
		}
	}
	return out, nil
}

func generateActionTypesGo(actionTypes map[string]actionTypeMeta) (string, error) {
	keys := make([]string, 0, len(actionTypes))
	for k := range actionTypes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var mutations []string
	var bootstraps []string
	for _, key := range keys {
		meta := actionTypes[key]
		if meta.Mutation {
			mutations = append(mutations, meta.GoConst)
		}
		if meta.Bootstrap {
			bootstraps = append(bootstraps, meta.GoConst)
		}
	}

	var b strings.Builder
	b.WriteString(fileHeader("constgen"))
	b.WriteString(`package constants

// ActionType is a typed string for governed action classes.
type ActionType string

const (
`)
	for _, key := range keys {
		meta := actionTypes[key]
		fmt.Fprintf(&b, "\t%s ActionType = %q\n", meta.GoConst, meta.Value)
	}
	b.WriteString(`)

// AllActionTypes is the canonical slice of all valid action types.
var AllActionTypes = []ActionType{
`)
	for _, key := range keys {
		fmt.Fprintf(&b, "\t%s,\n", actionTypes[key].GoConst)
	}
	b.WriteString(`}

// IsMutation returns true if the action type modifies system state.
func (a ActionType) IsMutation() bool {
	switch a {
	case `)
	if len(mutations) > 0 {
		for i, goConst := range mutations {
			if i > 0 {
				b.WriteString(",\n\t\t")
			}
			b.WriteString(goConst)
		}
	}
	b.WriteString(`:
		return true
	default:
		return false
	}
}

// IsBootstrapAction returns true for platform enrollment bootstrap actions.
func (a ActionType) IsBootstrapAction() bool {
	switch a {
	case `)
	if len(bootstraps) > 0 {
		for i, goConst := range bootstraps {
			if i > 0 {
				b.WriteString(",\n\t\t")
			}
			b.WriteString(goConst)
		}
	}
	b.WriteString(`:
		return true
	default:
		return false
	}
}
`)
	return b.String(), nil
}
