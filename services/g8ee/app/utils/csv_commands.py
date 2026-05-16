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

"""CSV parsing utilities for command lists."""

from __future__ import annotations


def parse_command_csv(csv: str | None) -> list[str]:
    """Parse a comma-separated command string into an ordered, deduplicated list.

    Whitespace around entries is stripped. Empty fragments (e.g. trailing commas
    or back-to-back commas) are dropped. Order of first occurrence is preserved.
    A ``None`` or empty input yields an empty list.

    This is a generic CSV parser for base commands, used by multiple command
    validation policies (whitelist CSV override, auto-approve CSV override, etc.).

    Examples:
        "uptime,df,free"        -> ["uptime", "df", "free"]
        " uptime , df ,, free " -> ["uptime", "df", "free"]
        "uptime,uptime,df"      -> ["uptime", "df"]
        ""                      -> []
    """
    if not csv:
        return []
    seen: set[str] = set()
    out: list[str] = []
    for raw in csv.split(","):
        token = raw.strip()
        if not token or token in seen:
            continue
        seen.add(token)
        out.append(token)
    return out
