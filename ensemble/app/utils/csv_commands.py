# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

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
