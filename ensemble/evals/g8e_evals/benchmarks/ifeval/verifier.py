# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import json
import re
from typing import Any

from g8e_evals.harness import Score
from g8e_evals.models import InstructionResult, ScoreDetails


class IFEvalVerifier:
    def verify(
        self,
        task_id: str,
        prompt: str,
        answer: str,
        instructions: list[str],
        kwargs: list[dict[str, Any]],
    ) -> Score:
        """
        Verify an IFEval response against its instructions.
        Each instruction has a corresponding entry in kwargs.
        """
        # Global non-empty check
        if not answer or not answer.strip():
            results = [
                InstructionResult(instruction=inst_id, passed=False, kwargs=kw)
                for inst_id, kw in zip(instructions, kwargs, strict=True)
            ]
            return Score(
                task_id=task_id,
                passed=False,
                details=ScoreDetails(instructions=results, error="Empty answer"),
            )

        results = [
            InstructionResult(
                instruction=inst_id,
                passed=self._check_instruction(inst_id, kw, answer),
                kwargs=kw,
            )
            for inst_id, kw in zip(instructions, kwargs, strict=True)
        ]

        all_passed = all(result.passed for result in results)
        return Score(
            task_id=task_id,
            passed=all_passed,
            details=ScoreDetails(instructions=results),
        )

    def _check_instruction(self, inst_id: str, kw: dict[str, Any], answer: str) -> bool:
        if inst_id == "punctuation:no_comma":
            # Check if the answer contains a comma
            # Canonical IFEval no_comma permits all other punctuation
            # The global non-empty check rejects empty responses
            return "," not in answer

        if inst_id == "keywords:forbidden_words":
            forbidden = kw.get("forbidden_words", [])
            if not answer.strip() and forbidden:
                return False
            for word in forbidden:
                if word.lower() in answer.lower():
                    return False
            return True

        if inst_id == "keywords:existence":
            keywords = kw.get("keywords", [])
            for word in keywords:
                if word.lower() not in answer.lower():
                    return False
            return True

        if inst_id == "detectable_format:json_format":
            try:
                json.loads(answer)
                return True
            except json.JSONDecodeError:
                # Sometimes LLMs wrap in code blocks
                match = re.search(r"```json\n(.*?)\n```", answer, re.DOTALL)
                if match:
                    try:
                        json.loads(match.group(1))
                        return True
                    except json.JSONDecodeError:
                        pass
                return False

        if inst_id == "length_constraints:number_words":
            num_words = kw.get("num_words")
            relation = kw.get("relation")
            if not isinstance(num_words, int) or not isinstance(relation, str):
                return False
            word_count = len(re.findall(r"\w+", answer))
            if relation == "at least":
                return word_count >= num_words
            if relation == "less than":
                return word_count < num_words
            if relation == "at most":
                return word_count <= num_words
            if relation == "more than":
                return word_count > num_words
            return False

        if inst_id == "change_case:english_capital":
            # Check if the whole response is uppercase (ignoring non-alpha)
            alpha_only = "".join(c for c in answer if c.isalpha())
            if not alpha_only:
                return False
            return alpha_only.isupper()

        if inst_id == "change_case:english_lowercase":
            alpha_only = "".join(c for c in answer if c.isalpha())
            if not alpha_only:
                return False
            return alpha_only.islower()

        if inst_id == "language:response_language":
            # Language verification is not implemented in this partial evaluator.
            # This verifier cannot identify the requested response language reliably.
            # Unsupported instructions fail closed rather than becoming passes.
            # The canonical upstream evaluator replaces this partial implementation.
            return False

        # Default to False for unknown instructions to be strict
        return False
