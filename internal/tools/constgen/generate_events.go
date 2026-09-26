// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"fmt"
	"sort"
	"strings"
)

type hierarchyNode struct {
	children map[string]*hierarchyNode
	leaf     string
}

func generateEventsGo(reg registryFile, hierarchy map[string]string, actionTypes map[string]actionTypeMeta) (string, error) {
	keys := sortedEventKeys(reg.Events)

	var b strings.Builder
	b.WriteString(fileHeader("constgen"))
	b.WriteString(`package constants

import "fmt"

// EventType is a typed string for event types.
type EventType string

`)

	for _, key := range keys {
		entry := reg.Events[key]
		b.WriteString(fmt.Sprintf("const %s EventType = %q\n", entry.GoConst, entry.Value))
	}

	b.WriteString(`
// EventKind classifies registry entries.
type EventKind string

const (
	EventKindRequest EventKind = "request"
	EventKindOutcome EventKind = "outcome"
	EventKindFact    EventKind = "fact"
	EventKindStream  EventKind = "stream"
)

// EventRegistryEntry is typed metadata for one registry event.
type EventRegistryEntry struct {
	Key               string
	Kind              EventKind
	Transport         []string
	Producers         []string
	Persistence       string
	GovernanceAction  ActionType
	GovernancePayload string
	Reserved          bool
}

// EventRegistry provides metadata lookup for EventType values.
type EventRegistry struct {
	byType map[EventType]EventRegistryEntry
}

// Lookup returns registry metadata for an event type.
func (r EventRegistry) Lookup(event EventType) (EventRegistryEntry, bool) {
	entry, ok := r.byType[event]
	return entry, ok
}

// ActionFor returns the governed action class for a request event.
func (r EventRegistry) ActionFor(event EventType) (ActionType, error) {
	entry, ok := r.byType[event]
	if !ok {
		return "", fmt.Errorf("unknown event type %q", event)
	}
	if entry.GovernanceAction == "" {
		return "", fmt.Errorf("event %q is not a governed request", event)
	}
	return entry.GovernanceAction, nil
}

// Registry is the generated event metadata map keyed by EventType.
var Registry = EventRegistry{byType: map[EventType]EventRegistryEntry{
`)

	for _, key := range keys {
		entry := reg.Events[key]
		b.WriteString(fmt.Sprintf("\t%s: {\n", entry.GoConst))
		b.WriteString(fmt.Sprintf("\t\tKey: %q,\n", key))
		b.WriteString(fmt.Sprintf("\t\tKind: EventKind%s,\n", titleCase(entry.Kind)))
		b.WriteString(fmt.Sprintf("\t\tTransport: %s,\n", goStringSlice(entry.Transport)))
		b.WriteString(fmt.Sprintf("\t\tProducers: %s,\n", goStringSlice(entry.Producers)))
		b.WriteString(fmt.Sprintf("\t\tPersistence: %q,\n", entry.Persistence))
		if entry.Governance != nil {
			goConst, err := actionTypeGoConst(actionTypes, entry.Governance.ActionType)
			if err != nil {
				return "", fmt.Errorf("%s: %v", key, err)
			}
			b.WriteString(fmt.Sprintf("\t\tGovernanceAction: %s,\n", goConst))
			b.WriteString(fmt.Sprintf("\t\tGovernancePayload: %q,\n", entry.Governance.Payload))
		}
		if entry.Reserved {
			b.WriteString("\t\tReserved: true,\n")
		}
		b.WriteString("\t},\n")
	}

	b.WriteString("}}\n\n")
	b.WriteString(generateOperatorHierarchy(hierarchy))
	return b.String(), nil
}

func sortedEventKeys(events map[string]eventEntry) []string {
	keys := make([]string, 0, len(events))
	for k := range events {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func goStringSlice(values []string) string {
	if len(values) == 0 {
		return "nil"
	}
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return "[]string{" + strings.Join(quoted, ", ") + "}"
}

func actionTypeGoConst(actionTypes map[string]actionTypeMeta, value string) (string, error) {
	for _, meta := range actionTypes {
		if meta.Value == value {
			return meta.GoConst, nil
		}
	}
	return "", fmt.Errorf("unknown governance.action_type %q", value)
}

func generateOperatorHierarchy(hierarchy map[string]string) string {
	root := &hierarchyNode{children: map[string]*hierarchyNode{}}
	for goConst, path := range hierarchy {
		parts := strings.Split(path, ".")
		node := root
		for i, part := range parts {
			if node.children[part] == nil {
				node.children[part] = &hierarchyNode{children: map[string]*hierarchyNode{}}
			}
			node = node.children[part]
			if i == len(parts)-1 {
				node.leaf = goConst
			}
		}
	}

	var b strings.Builder
	typeNames := map[string]string{}
	collectHierarchyTypeNames(root, "Operator", typeNames)
	emitHierarchyTypes(&b, root, "Operator", typeNames)
	emitHierarchyVar(&b, root, "Operator", typeNames)
	return b.String()
}

func collectHierarchyTypeNames(node *hierarchyNode, path string, typeNames map[string]string) {
	if len(node.children) == 0 {
		return
	}
	typeNames[path] = "_Event" + path
	for _, child := range sortedMapKeys(node.children) {
		childNode := node.children[child]
		if childNode.leaf != "" && len(childNode.children) == 0 {
			continue
		}
		collectHierarchyTypeNames(childNode, path+child, typeNames)
	}
}

func emitHierarchyTypes(b *strings.Builder, node *hierarchyNode, path string, typeNames map[string]string) {
	if len(node.children) == 0 {
		return
	}
	b.WriteString(fmt.Sprintf("type %s struct {\n", typeNames[path]))
	childNames := sortedMapKeys(node.children)
	for _, child := range childNames {
		childNode := node.children[child]
		if childNode.leaf != "" && len(childNode.children) == 0 {
			b.WriteString(fmt.Sprintf("\t%s EventType\n", child))
			continue
		}
		childPath := path + child
		b.WriteString(fmt.Sprintf("\t%s %s\n", child, typeNames[childPath]))
	}
	b.WriteString("}\n\n")
	for _, child := range childNames {
		childNode := node.children[child]
		if childNode.leaf != "" && len(childNode.children) == 0 {
			continue
		}
		emitHierarchyTypes(b, childNode, path+child, typeNames)
	}
}

func emitHierarchyVar(b *strings.Builder, node *hierarchyNode, path string, typeNames map[string]string) {
	b.WriteString("var Event = struct {\n")
	b.WriteString(fmt.Sprintf("\tOperator %s\n", typeNames["Operator"]))
	b.WriteString("}{\n")
	b.WriteString(fmt.Sprintf("\tOperator: %s{\n", typeNames["Operator"]))
	emitHierarchyLiteral(b, node, typeNames, "Operator", "\t\t")
	b.WriteString("\t},\n")
	b.WriteString("}\n")
}

func emitHierarchyLiteral(b *strings.Builder, node *hierarchyNode, typeNames map[string]string, path string, indent string) {
	childNames := sortedMapKeys(node.children)
	for _, child := range childNames {
		childNode := node.children[child]
		childPath := path + child
		if childNode.leaf != "" && len(childNode.children) == 0 {
			b.WriteString(fmt.Sprintf("%s%s: %s,\n", indent, child, childNode.leaf))
			continue
		}
		b.WriteString(fmt.Sprintf("%s%s: %s{\n", indent, child, typeNames[childPath]))
		emitHierarchyLiteral(b, childNode, typeNames, childPath, indent+"\t")
		b.WriteString(fmt.Sprintf("%s},\n", indent))
	}
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
