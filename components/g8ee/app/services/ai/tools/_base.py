# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Shared helpers for per-tool modules.

Kept tiny on purpose: each tool module is self-contained, and only two
helpers are genuinely shared.
"""

from __future__ import annotations

from typing import TypeVar

T = TypeVar("T")


def convert_args_to_payload(
    args_dict: dict[str, object],
    payload_cls: type[T],
    execution_id: str,
    **extra_fields: object,
) -> T:
    """Convert raw LLM tool args into a downstream Payload, injecting ``execution_id``.

    Centralises the Args -> Payload conversion so the ``execution_id`` hand-off
    cannot be forgotten when adding new tools.
    """
    return payload_cls.model_validate(
        {**args_dict, "execution_id": execution_id, **extra_fields}
    )
