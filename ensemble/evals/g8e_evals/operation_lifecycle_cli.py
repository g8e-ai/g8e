# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import sys
from enum import StrEnum
from pathlib import Path

from pydantic import BaseModel, ConfigDict, Field, ValidationError

from g8e_evals.operation_lifecycle import check_operation, plan_operation


class OperationLifecycleAction(StrEnum):
    PLAN = "plan"
    CHECK = "check"


class OperationLifecycleRequest(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    action: OperationLifecycleAction
    config_path: str = Field(min_length=1)
    repository_root: str = Field(min_length=1)


def run_operation_lifecycle(request: OperationLifecycleRequest) -> BaseModel:
    config_path = Path(request.config_path)
    repository_root = Path(request.repository_root)
    if request.action == OperationLifecycleAction.PLAN:
        return plan_operation(config_path, repository_root)
    return check_operation(config_path, repository_root)


def main(argv: list[str] | None = None) -> int:
    args = argv if argv is not None else sys.argv[1:]
    if len(args) != 1:
        sys.stderr.write("usage: python -m g8e_evals.operation_lifecycle_cli <request-path>\n")
        return 1
    try:
        request = OperationLifecycleRequest.model_validate_json(Path(args[0]).read_bytes())
        result = run_operation_lifecycle(request)
    except (OSError, ValueError, ValidationError) as exc:
        sys.stderr.write(f"operation lifecycle failed: {exc}\n")
        return 1
    sys.stdout.write(result.model_dump_json(exclude_none=True))
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
