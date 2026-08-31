# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import json
from collections.abc import Iterable
from pathlib import Path

from g8e_evals.benchmarks.ifeval.provenance import load_provenance, validate_dataset
from g8e_evals.harness import Task
from g8e_evals.models import TaskMetadata


class IFEvalLoader:
    def __init__(self, gold_set_path: Path):
        self.gold_set_path = gold_set_path

    def load(self) -> Iterable[Task]:
        """
        Load IFEval tasks from input_data.jsonl.
        Each line is expected to be:
        {"key": 1234, "prompt": "...", "instruction_id_list": [...], "kwargs": {...}}
        """
        if not self.gold_set_path.exists():
            raise FileNotFoundError(f"IFEval gold set not found at {self.gold_set_path}")

        provenance = load_provenance(self.gold_set_path.with_name("provenance.json"))
        validate_dataset(self.gold_set_path, provenance)
        rows = [json.loads(line) for line in self.gold_set_path.read_text().splitlines() if line.strip()]
        if [row["key"] for row in rows] != provenance.selected_keys:
            raise ValueError("IFEval subset keys do not match provenance")

        for data in rows:
            yield Task(
                id=str(data["key"]),
                prompt=data["prompt"],
                metadata=TaskMetadata(
                    benchmark="ifeval_subset",
                    expected_action_class=data.get("expected_action_class", ""),
                    expected_final_state_assertions=data.get("expected_final_state_assertions", []),
                    expected_allow_block_outcome=data.get("expected_allow_block_outcome"),
                    expected_rejection_layer=data.get("expected_rejection_layer"),
                    instruction_id_list=data["instruction_id_list"],
                    kwargs=data["kwargs"],
                ),
            )
