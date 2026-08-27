// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPubSub_NoDeletedAdapters is an AST lint guard asserting that pubsub_commands.go
// (and the pubsub package) declares no GatewayEnvProcAdapter or GatewaySessionValidatorAdapter
// types. Under the C2 inverted construction lifecycle, the circular dependency is eliminated
// and concrete OperatorPubSubService instances are injected directly.
func TestPubSub_NoDeletedAdapters(t *testing.T) {
	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	forbiddenTypes := map[string]bool{
		"GatewayEnvProcAdapter":          true,
		"GatewaySessionValidatorAdapter": true,
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range genDecl.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if forbiddenTypes[ts.Name.Name] {
					pos := fset.Position(ts.Pos())
					t.Errorf("%s: pubsub package declares forbidden adapter type %s — concrete OperatorPubSubService must be injected directly under C2 lifecycle (see Item R plan)", pos, ts.Name.Name)
				}
			}
		}
	}
}
