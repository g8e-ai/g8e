"""Static regression test for ServiceFactory.create_all_services.

The factory is a long, linear wiring procedure. If a local is referenced
before it is assigned, g8ee crashes with UnboundLocalError during the
FastAPI lifespan startup -- not at import time -- so the bug only
surfaces when the service actually boots.

Regression: ``chat_task_manager`` was passed to ``AIToolService`` before
being constructed, breaking g8ee startup.
"""

from __future__ import annotations

import ast
import inspect
import textwrap

from app.services.service_factory import ServiceFactory


def _collect_local_names(func_def: ast.FunctionDef | ast.AsyncFunctionDef) -> set[str]:
    locals_: set[str] = set()
    for node in ast.walk(func_def):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.Lambda)) and node is not func_def:
            continue
        if isinstance(node, ast.Name) and isinstance(node.ctx, ast.Store):
            locals_.add(node.id)
    return locals_


def test_create_all_services_has_no_use_before_assignment() -> None:
    source = textwrap.dedent(inspect.getsource(ServiceFactory.create_all_services))
    tree = ast.parse(source)
    func_def = tree.body[0]
    assert isinstance(func_def, (ast.FunctionDef, ast.AsyncFunctionDef))

    local_names = _collect_local_names(func_def)
    assigned: set[str] = {arg.arg for arg in func_def.args.args + func_def.args.kwonlyargs}
    errors: list[str] = []

    def record_target(target: ast.AST) -> None:
        if isinstance(target, ast.Name):
            assigned.add(target.id)
        elif isinstance(target, (ast.Tuple, ast.List)):
            for elt in target.elts:
                record_target(elt)
        elif isinstance(target, ast.Starred):
            record_target(target.value)

    def visit(node: ast.AST) -> None:
        if isinstance(node, ast.Assign):
            visit(node.value)
            for t in node.targets:
                record_target(t)
            return
        if isinstance(node, ast.AnnAssign):
            if node.value is not None:
                visit(node.value)
            record_target(node.target)
            return
        if isinstance(node, ast.AugAssign):
            visit(node.value)
            record_target(node.target)
            return
        if isinstance(node, ast.Name):
            if isinstance(node.ctx, ast.Load) and node.id in local_names and node.id not in assigned:
                errors.append(f"{node.id!r} used at line {node.lineno} before assignment")
            return
        for child in ast.iter_child_nodes(node):
            visit(child)

    for stmt in func_def.body:
        visit(stmt)

    assert not errors, "use-before-assignment in create_all_services: " + "; ".join(errors)
