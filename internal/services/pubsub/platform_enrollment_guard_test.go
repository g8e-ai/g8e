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
	"testing"
)

// TestExecuteVerifiedTransaction_NoPlatformEnrollmentNilGuards is an AST lint guard
// asserting that OperatorPubSubService.ExecuteVerifiedTransaction contains no
// `rs.platformEnrollment == nil` guards. Platform enrollment handler is required in
// gateway mode and absent in outbound mode.
func TestExecuteVerifiedTransaction_NoPlatformEnrollmentNilGuards(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "pubsub_commands.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse pubsub_commands.go: %v", err)
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "ExecuteVerifiedTransaction" {
			continue
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			ifStmt, ok := n.(*ast.IfStmt)
			if !ok {
				return true
			}

			binExpr, ok := ifStmt.Cond.(*ast.BinaryExpr)
			if !ok || binExpr.Op != token.EQL {
				return true
			}

			isPlatformEnrollmentNilCheck := func(x, y ast.Expr) bool {
				ident, ok := y.(*ast.Ident)
				if !ok || ident.Name != "nil" {
					return false
				}
				if sel, ok := x.(*ast.SelectorExpr); ok {
					return sel.Sel.Name == "platformEnrollment"
				}
				if id, ok := x.(*ast.Ident); ok {
					return id.Name == "platformEnrollment"
				}
				return false
			}

			if isPlatformEnrollmentNilCheck(binExpr.X, binExpr.Y) || isPlatformEnrollmentNilCheck(binExpr.Y, binExpr.X) {
				pos := fset.Position(ifStmt.Pos())
				t.Errorf("%s: ExecuteVerifiedTransaction contains forbidden nil-check on platformEnrollment", pos)
			}

			return true
		})
	}
}
