# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import json
from pathlib import Path
from collections.abc import Iterable

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

        with open(self.gold_set_path) as f:
            for line in f:
                if not line.strip():
                    continue
                data = json.loads(line)
                yield Task(
                    id=str(data.get("key")),
                    prompt=data.get("prompt"),
                    metadata=TaskMetadata(
                        benchmark="ifeval_subset",
                        instruction_id_list=data.get("instruction_id_list", []),
                        kwargs=data.get("kwargs", [])
                    )
                )
