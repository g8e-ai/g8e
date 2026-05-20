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

import json
from pathlib import Path
from typing import Iterable

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

        with open(self.gold_set_path, 'r') as f:
            for line in f:
                if not line.strip():
                    continue
                data = json.loads(line)
                yield Task(
                    id=str(data.get("key")),
                    prompt=data.get("prompt"),
                    metadata=TaskMetadata(
                        benchmark="ifeval",
                        instruction_id_list=data.get("instruction_id_list", []),
                        kwargs=data.get("kwargs", [])
                    )
                )
