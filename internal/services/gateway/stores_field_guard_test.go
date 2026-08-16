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

// TestStoresFieldNotLeakedPastGatewayDB is a lint guard asserting that no
// production struct outside gateway_db.go declares a field of type *Stores.
// The Stores mega-struct is an Interface Segregation smell (Smell 1 in the
// codemap remediation plan): consumers that need 2 stores get a handle to 11.
// The remediation replaced *Stores fields on GatewayModeService and
// G8eoService with narrow per-store fields and accessor methods on
// CanonicalDBService. This test prevents regressions where a new or existing
// production struct re-introduces the *Stores field.
//
// Allowed exceptions:
//   - gateway_db.go: CanonicalDBService.stores is the lifecycle owner of the
//     Stores bundle and is permitted to hold it.
func TestStoresFieldNotLeakedPastGatewayDB(t *testing.T) {
	dir := "." // test runs with package directory as cwd
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	// allowedStructs maps "filename.go:StructName" to an allow marker. Only
	// these production struct declarations may hold a *Stores field.
	allowedStructs := map[string]bool{
		"gateway_db.go:CanonicalDBService": true, // lifecycle owner
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

		// Walk top-level declarations to find named struct types and their
		// fields. This gives us the struct name alongside the StructType,
		// which ast.Inspect alone does not provide.
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
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				structName := ts.Name.Name
				for _, field := range st.Fields.List {
					if !isStoresPtr(field.Type) {
						continue
					}
					key := name + ":" + structName
					if allowedStructs[key] {
						continue
					}
					pos := fset.Position(field.Pos())
					t.Errorf("%s: struct %s declares *Stores field %q — only CanonicalDBService (gateway_db.go) "+
						"may hold *Stores; use narrow accessor methods on CanonicalDBService instead "+
						"(see codemap remediation plan, Smell 1)",
						pos, structName, fieldName(field))
				}
			}
		}
	}
}

// isStoresPtr reports whether expr is the type *Stores (pointer to Stores).
func isStoresPtr(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	id, ok := star.X.(*ast.Ident)
	return ok && id.Name == "Stores"
}

// fieldName extracts the field name for error reporting. For embedded fields
// or anonymous fields, returns the type name.
func fieldName(field *ast.Field) string {
	if len(field.Names) > 0 {
		return field.Names[0].Name
	}
	// Embedded field — use the type expression as the name.
	return "<embedded>"
}

// TestStoresDoesNotExportRawDB is a lint guard asserting that the Stores
// aggregation struct does not expose a raw *sqliteutil.DB handle. The Stores
// struct exists to give consumers typed store services; a raw DB field lets
// callers bypass every store and defeats the repository pattern (Smell 7 in
// the codemap remediation plan). Consumers needing direct DB access during
// construction (NewConsensusStoreService, NewStateRootService) are constructed
// inside OpenCanonicalDBService where the DB is already in scope. This test
// prevents regressions where the DB field is re-added to Stores.
func TestStoresDoesNotExportRawDB(t *testing.T) {
	dir := "." // test runs with package directory as cwd
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read package dir: %v", err)
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
				if !ok || ts.Name.Name != "Stores" {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range st.Fields.List {
					if isSqliteutilDBPtr(field.Type) {
						pos := fset.Position(field.Pos())
						t.Errorf("%s: Stores declares raw *sqliteutil.DB field %q — "+
							"Stores must only expose typed store services; "+
							"construct raw-DB consumers inside OpenCanonicalDBService "+
							"instead of leaking the DB handle (see codemap remediation plan, Smell 7)",
							pos, fieldName(field))
					}
				}
			}
		}
	}
}

// isSqliteutilDBPtr reports whether expr is the type *sqliteutil.DB.
func isSqliteutilDBPtr(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "sqliteutil" && sel.Sel.Name == "DB"
}
