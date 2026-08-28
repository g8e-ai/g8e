# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import json
import subprocess

from pydantic import BaseModel, ConfigDict, Field, ValidationError


class AuthBridgeError(Exception):
    pass


class CLIAuthContext(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    operator_session_id: str = Field(min_length=1)
    cli_session_id: str = Field(min_length=1)
    user_id: str = Field(min_length=1)
    operator_id: str = ""
    client_cert: str = Field(min_length=1)
    client_key: str = Field(min_length=1)


def load_cli_auth_context(g8e_cli: str) -> CLIAuthContext:
    try:
        result = subprocess.run(
            [g8e_cli, "auth", "context"],
            capture_output=True,
            check=False,
            text=True,
        )
    except OSError as error:
        raise AuthBridgeError(f"could not execute {g8e_cli}: {error}") from error
    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip() or f"exit status {result.returncode}"
        raise AuthBridgeError(detail)
    try:
        payload = json.loads(result.stdout)
    except json.JSONDecodeError as error:
        raise AuthBridgeError(f"invalid JSON from {g8e_cli} auth context: {error}") from error
    try:
        return CLIAuthContext.model_validate(payload)
    except ValidationError as error:
        raise AuthBridgeError(f"invalid authentication context: {error}") from error
