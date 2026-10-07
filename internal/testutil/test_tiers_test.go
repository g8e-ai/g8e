// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testutil

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Check source rather than opening a listener to test tier enforcement. This
// also checks platform-specific files that the current host does not compile.
func TestTestTiers_ListenersRequireIntegrationAndBoundaryTestsStaySerial(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test sources")
	}
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	fset := token.NewFileSet()
	for _, dir := range []string{"internal", "test"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			unit := true
			for _, line := range strings.Split(string(body), "\n") {
				if strings.HasPrefix(line, "//go:build ") {
					expr, err := constraint.Parse(line)
					if err != nil {
						return err
					}
					unit = testBuildAllowsUnit(expr)
					break
				}
			}
			file, err := parser.ParseFile(fset, path, body, 0)
			if err != nil {
				return err
			}
			imports := make(map[string]string)
			for _, imp := range file.Imports {
				pkg, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					return err
				}
				name := filepath.Base(pkg)
				if imp.Name != nil {
					name = imp.Name.Name
				}
				imports[name] = pkg
			}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if !unit && sel.Sel.Name == "Parallel" {
					t.Errorf("%s: integration/E2E tests must not call Parallel (INV-TEST-RUN-04)", fset.Position(call.Pos()))
				}
				id, ok := sel.X.(*ast.Ident)
				if !ok || !unit {
					return true
				}
				listener := false
				switch imports[id.Name] {
				case "net/http/httptest":
					listener = sel.Sel.Name == "NewServer" || sel.Sel.Name == "NewTLSServer" || sel.Sel.Name == "NewUnstartedServer"
				case "net":
					listener = strings.HasPrefix(sel.Sel.Name, "Listen") || strings.HasPrefix(sel.Sel.Name, "Dial")
				case "net/http":
					listener = sel.Sel.Name == "ListenAndServe" || sel.Sel.Name == "ListenAndServeTLS"
				}
				if listener {
					t.Errorf("%s: socket tests require integration or e2e build tags (INV-TEST-RUN-02)", fset.Position(call.Pos()))
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// A platform constraint alone (linux, !windows, etc.) does not exclude Tier 1.
// Evaluate all other tag combinations with the two boundary tiers disabled.
func testBuildAllowsUnit(expr constraint.Expr) bool {
	tags := make(map[string]int)
	var collect func(constraint.Expr)
	collect = func(expr constraint.Expr) {
		switch expr := expr.(type) {
		case *constraint.TagExpr:
			if expr.Tag != "integration" && expr.Tag != "e2e" {
				if _, exists := tags[expr.Tag]; !exists {
					tags[expr.Tag] = len(tags)
				}
			}
		case *constraint.NotExpr:
			collect(expr.X)
		case *constraint.AndExpr:
			collect(expr.X)
			collect(expr.Y)
		case *constraint.OrExpr:
			collect(expr.X)
			collect(expr.Y)
		}
	}
	collect(expr)
	for bits := 0; bits < 1<<len(tags); bits++ {
		if expr.Eval(func(tag string) bool {
			index, exists := tags[tag]
			return exists && bits&(1<<index) != 0
		}) {
			return true
		}
	}
	return false
}

func TestTestBuildAllowsUnit_PlatformConstraintsDoNotReplaceTierTags(t *testing.T) {
	for _, tc := range []struct {
		expr string
		unit bool
	}{
		{"integration", false},
		{"integration || e2e", false},
		{"linux && integration", false},
		{"!windows && integration", false},
		{"linux && !windows", true},
		{"integration || linux", true},
		{"!integration", true},
		{"e2e || !windows", true},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			expr, err := constraint.Parse("//go:build " + tc.expr)
			if err != nil {
				t.Fatal(err)
			}
			if got := testBuildAllowsUnit(expr); got != tc.unit {
				t.Errorf("unit availability = %v, want %v", got, tc.unit)
			}
		})
	}
}
