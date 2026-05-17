import json
from pathlib import Path
from typing import Iterable, Dict, Any
from g8e_evals.harness import Task

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
                    metadata={
                        "benchmark": "ifeval",
                        "instruction_id_list": data.get("instruction_id_list"),
                        "kwargs": data.get("kwargs", {})
                    }
                )
