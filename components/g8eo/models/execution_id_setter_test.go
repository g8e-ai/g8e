// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package models

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// payloadSuffixes enumerates the struct-name suffixes that indicate an
// outbound LFAA payload family emitted by the operator. Any struct matching
// one of these suffixes AND carrying an ExecutionID field MUST implement
// ExecutionIDSetter; otherwise setExecutionIDOnPayload in publish_helpers.go
// will silently drop the correlation id at publish time.
var payloadSuffixes = []string{
	"ResultPayload",
	"StatusPayload",
	"ErrorPayload",
}

func hasPayloadSuffix(name string) bool {
	for _, s := range payloadSuffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}

// parseModelsPackage parses the non-test .go files of this package so we can
// reason about type declarations and method declarations structurally. This
// is the AST auto-discovery replacement for the prior hand-maintained
// registration lists.
func parseModelsPackage(t *testing.T) *ast.Package {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse models package: %v", err)
	}
	pkg, ok := pkgs["models"]
	if !ok {
		t.Fatalf("models package not found; got %v", pkgKeys(pkgs))
	}
	return pkg
}

func pkgKeys(m map[string]*ast.Package) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// structsWithExecutionID returns the set of struct-type names declared in the
// package that have a field literally named ExecutionID of type string.
func structsWithExecutionID(pkg *ast.Package) map[string]bool {
	found := make(map[string]bool)
	for _, f := range pkg.Files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok || st.Fields == nil {
					continue
				}
				for _, field := range st.Fields.List {
					ident, ok := field.Type.(*ast.Ident)
					if !ok || ident.Name != "string" {
						continue
					}
					for _, name := range field.Names {
						if name.Name == "ExecutionID" {
							found[ts.Name.Name] = true
						}
					}
				}
			}
		}
	}
	return found
}

// setterMethodInfo captures what we need to validate a SetExecutionID method:
// the receiver type name it is attached to and whether its body is exactly
// the trivial assignment `<recv>.ExecutionID = <param>`. The body check keeps
// a typo like `p.ExecutionID = ""` from silently passing only because the
// method exists.
type setterMethodInfo struct {
	recvType  string
	trivial   bool
	paramName string
}

// collectSetExecutionIDMethods walks the package AST and returns a map of
// receiver type name -> setter info for every pointer-receiver method named
// SetExecutionID that takes exactly one string argument.
func collectSetExecutionIDMethods(pkg *ast.Package) map[string]setterMethodInfo {
	methods := make(map[string]setterMethodInfo)
	for _, f := range pkg.Files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name.Name != "SetExecutionID" || fd.Recv == nil || len(fd.Recv.List) != 1 {
				continue
			}
			star, ok := fd.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			ident, ok := star.X.(*ast.Ident)
			if !ok {
				continue
			}
			if fd.Type.Params == nil || len(fd.Type.Params.List) != 1 {
				continue
			}
			pType, ok := fd.Type.Params.List[0].Type.(*ast.Ident)
			if !ok || pType.Name != "string" || len(fd.Type.Params.List[0].Names) != 1 {
				continue
			}
			paramName := fd.Type.Params.List[0].Names[0].Name
			recvNames := fd.Recv.List[0].Names
			if len(recvNames) != 1 {
				continue
			}
			recvName := recvNames[0].Name

			info := setterMethodInfo{recvType: ident.Name, paramName: paramName}
			info.trivial = isTrivialSetExecutionIDBody(fd.Body, recvName, paramName)
			methods[ident.Name] = info
		}
	}
	return methods
}

// isTrivialSetExecutionIDBody returns true iff the body is a single statement
// `<recv>.ExecutionID = <param>`. Anything else means the setter is doing
// something surprising and deserves a human look before we trust it.
func isTrivialSetExecutionIDBody(body *ast.BlockStmt, recvName, paramName string) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	assign, ok := body.List[0].(*ast.AssignStmt)
	if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}
	sel, ok := assign.Lhs[0].(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "ExecutionID" {
		return false
	}
	lhsIdent, ok := sel.X.(*ast.Ident)
	if !ok || lhsIdent.Name != recvName {
		return false
	}
	rhsIdent, ok := assign.Rhs[0].(*ast.Ident)
	if !ok || rhsIdent.Name != paramName {
		return false
	}
	return true
}

// TestExecutionIDSetter_CoversAllOutboundPayloads is the AST auto-discovery
// guardrail. It scans the package source for every struct whose name ends in
// a known outbound-payload suffix and has an ExecutionID string field, then
// asserts a trivial pointer-receiver SetExecutionID method exists for it.
//
// This closes the loop that was previously open: adding a new outbound
// payload with an ExecutionID field used to require manual updates to this
// test file. Now the test discovers the payload automatically and fails
// loudly if execution_id_setter.go is not updated in lockstep.
func TestExecutionIDSetter_CoversAllOutboundPayloads(t *testing.T) {
	pkg := parseModelsPackage(t)
	withField := structsWithExecutionID(pkg)
	setters := collectSetExecutionIDMethods(pkg)

	var missing, nonTrivial []string
	for name := range withField {
		if !hasPayloadSuffix(name) {
			continue
		}
		info, ok := setters[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		if !info.trivial {
			nonTrivial = append(nonTrivial, name)
		}
	}

	if len(missing) > 0 {
		t.Errorf("outbound payload types missing SetExecutionID in execution_id_setter.go: %v\n"+
			"Add: func (p *<Type>) SetExecutionID(id string) { p.ExecutionID = id }",
			missing)
	}
	if len(nonTrivial) > 0 {
		t.Errorf("SetExecutionID for %v is not the trivial `recv.ExecutionID = id` assignment; "+
			"either restore the one-liner form or update this test to allow the new shape",
			nonTrivial)
	}
}

// TestExecutionIDSetter_NoOrphanMethods ensures every SetExecutionID method
// targets a type that actually has an ExecutionID field. An orphan method
// would indicate either a renamed field (silent breakage) or dead code.
func TestExecutionIDSetter_NoOrphanMethods(t *testing.T) {
	pkg := parseModelsPackage(t)
	withField := structsWithExecutionID(pkg)
	setters := collectSetExecutionIDMethods(pkg)

	var orphans []string
	for recv := range setters {
		if !withField[recv] {
			orphans = append(orphans, recv)
		}
	}
	if len(orphans) > 0 {
		t.Errorf("SetExecutionID declared on types without an ExecutionID string field: %v", orphans)
	}
}

// TestExecutionIDSetter_InterfaceWiring is a runtime smoke test that confirms
// the reflective interface dispatch in setExecutionIDOnPayload still works
// end-to-end. One representative payload is sufficient because the AST test
// above already proves method existence and body correctness for every type.
func TestExecutionIDSetter_InterfaceWiring(t *testing.T) {
	var p ExecutionIDSetter = &ExecutionResultsPayload{}
	p.SetExecutionID("exec-abc")
	if got := p.(*ExecutionResultsPayload).ExecutionID; got != "exec-abc" {
		t.Fatalf("interface dispatch did not stamp ExecutionID: got %q", got)
	}
}
