// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGatewayModeService_NoDeletedSetters is an AST lint guard asserting that
// GatewayModeService declares no SetConsensusService or LoadConsensusService
// mutator methods. Under the ModeDeps refactor (Item R), consensus is wired at
// construction time via GatewayModeDeps.
func TestGatewayModeService_NoDeletedSetters(t *testing.T) {
	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	forbiddenMethods := map[string]bool{
		"SetConsensusService":  true,
		"LoadConsensusService": true,
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
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}

			recvType := receiverTypeName(fn.Recv.List[0].Type)
			if recvType != "GatewayModeService" {
				continue
			}

			methodName := fn.Name.Name
			if forbiddenMethods[methodName] {
				pos := fset.Position(fn.Pos())
				t.Errorf("%s: GatewayModeService declares forbidden method %s — consensus must be wired via GatewayModeDeps at construction time (see Item R plan)", pos, methodName)
			}
		}
	}
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}
