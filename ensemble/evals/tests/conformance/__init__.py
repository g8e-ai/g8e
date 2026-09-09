# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version  2.0.

"""Shared test infrastructure for the typed executable grader conformance matrix.

This package is test infrastructure, not a collected test module. The
per-suite conformance test files under ``tests/`` import the
``ConformanceCase`` record, context mutation helpers, proportion-grader
spec table, strict-grader case builders, and the aggregated registry from
this package and parametrize over the cases that belong to their graders.
"""
