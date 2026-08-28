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
SELECTED_KEYS = (1001, 1019, 1051, 1072, 1075)


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
