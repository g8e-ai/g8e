# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import argparse
import hashlib
from pathlib import Path

from pydantic import BaseModel

UPSTREAM_REVISION = "041338718b4e8151372fd63677104c65b73a0a4e"
UPSTREAM_URL = f"https://raw.githubusercontent.com/google-research/google-research/{UPSTREAM_REVISION}/instruction_following_eval/data/input_data.jsonl"
UPSTREAM_SHA256 = "67ffeee0fcb87c317c5b08a2de85557b4a7e96ada6178aa645b4954fe4b53d49"
SELECTED_KEYS = (
    13, 16, 19, 24, 30, 32, 102, 122, 127, 136, 142, 143, 152, 163, 164,
    167, 168, 179, 181, 201, 202, 209, 218, 219, 227, 240, 247, 251, 260,
    279, 281, 286, 288, 292, 295, 296, 301, 321, 322, 331, 332, 334, 337,
    340, 343, 349, 357, 1001, 1005, 1019, 1051, 1072, 1075, 1082, 1087,
    1094, 1098, 1108, 1128, 1130, 1131, 1147, 1154, 1162, 1187, 1220, 1236,
    1246, 1248, 1259, 1268, 1286, 1307, 1322, 1332, 1367, 1372, 1377, 1381,
    1393, 1446, 1480, 1498, 1518, 1531, 1537, 1548, 1551, 1566, 1580, 1592,
    1620, 1629, 1634, 1644, 1645, 1656, 1659, 1733, 1776, 1802, 1857, 1927,
    1934, 1954, 1996, 1999, 2023, 2107, 2136, 2215, 2250, 2485, 2567, 2602,
    2820, 2849, 3749, 3750, 3751,
)


class SourceRow(BaseModel):
    key: int


def read_source(source: Path) -> list[str]:
    content = source.read_bytes()
    digest = hashlib.sha256(content).hexdigest()
    if digest != UPSTREAM_SHA256:
        raise ValueError(f"upstream IFEval SHA-256 mismatch: {digest}")
    return content.decode().splitlines()


def select_rows(lines: list[str]) -> list[str]:
    selected = {
        row.key: line
        for line in lines
        if (row := SourceRow.model_validate_json(line)).key in SELECTED_KEYS
    }
    missing = set(SELECTED_KEYS) - selected.keys()
    if missing:
        raise ValueError(f"upstream IFEval rows missing: {sorted(missing)}")
    return [selected[key] for key in SELECTED_KEYS]


def write_subset(destination: Path, rows: list[str]) -> None:
    destination.write_text("\n".join(rows) + "\n")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("destination", type=Path)
    args = parser.parse_args()
    write_subset(args.destination, select_rows(read_source(args.source)))


if __name__ == "__main__":
    main()
