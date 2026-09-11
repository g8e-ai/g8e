# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import json
import re
from typing import Any

from g8e_evals.benchmarks.ifeval.instructions_util import (
    count_sentences,
    count_words,
    detect_language,
)
from g8e_evals.harness import Response, Score, Task
from g8e_evals.models import InstructionResult, ScoreDetails

# Relational operations for comparison (upstream uses only these two).
_COMPARISON_RELATION = ("less than", "at least")


class IFEvalVerifier:
    grader_id = "ifeval_subset_verifier"
    grader_version = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        """Grade a completed task against its instruction-following constraints.

        Extracts the instruction ID list and kwargs from the task metadata
        and delegates to ``verify`` for the per-instruction checking logic.
        """
        return self.verify(
            task.id,
            task.prompt,
            response.answer,
            task.metadata.instruction_id_list,
            task.metadata.kwargs,
        )

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
                if re.search(r"\b" + re.escape(word) + r"\b", answer, flags=re.IGNORECASE):
                    return False
            return True

        if inst_id == "keywords:existence":
            keywords = kw.get("keywords", [])
            for word in keywords:
                if not re.search(re.escape(word), answer, flags=re.IGNORECASE):
                    return False
            return True

        if inst_id == "keywords:frequency":
            keyword = kw.get("keyword", "")
            frequency = kw.get("frequency")
            relation = kw.get("relation")
            if not isinstance(frequency, int) or not isinstance(relation, str):
                return False
            actual = len(re.findall(re.escape(keyword), answer, flags=re.IGNORECASE))
            if relation == "less than":
                return actual < frequency
            if relation == "at least":
                return actual >= frequency
            return False

        if inst_id == "keywords:letter_frequency":
            letter = kw.get("letter", "")
            frequency = kw.get("let_frequency")
            relation = kw.get("let_relation")
            if not isinstance(frequency, int) or not isinstance(relation, str):
                return False
            letter = letter.lower()
            count = answer.lower().count(letter)
            if relation == "less than":
                return count < frequency
            if relation == "at least":
                return count >= frequency
            return False

        if inst_id == "detectable_format:json_format":
            value = (
                answer.strip()
                .removeprefix("```json")
                .removeprefix("```Json")
                .removeprefix("```JSON")
                .removeprefix("```")
                .removesuffix("```")
                .strip()
            )
            try:
                json.loads(value)
                return True
            except ValueError:
                return False

        if inst_id == "length_constraints:number_words":
            num_words = kw.get("num_words")
            relation = kw.get("relation")
            if not isinstance(num_words, int) or not isinstance(relation, str):
                return False
            word_count = count_words(answer)
            if relation == "at least":
                return word_count >= num_words
            if relation == "less than":
                return word_count < num_words
            if relation == "at most":
                return word_count <= num_words
            if relation == "more than":
                return word_count > num_words
            return False

        if inst_id == "length_constraints:number_sentences":
            num_sentences = kw.get("num_sentences")
            relation = kw.get("relation")
            if not isinstance(num_sentences, int) or not isinstance(relation, str):
                return False
            sentence_count = count_sentences(answer)
            if relation == "less than":
                return sentence_count < num_sentences
            if relation == "at least":
                return sentence_count >= num_sentences
            return False

        if inst_id == "length_constraints:number_paragraphs":
            num_paragraphs = kw.get("num_paragraphs")
            if not isinstance(num_paragraphs, int):
                return False
            paragraphs = re.split(r"\s?\*\*\*\s?", answer)
            count = len(paragraphs)
            for index, paragraph in enumerate(paragraphs):
                if not paragraph.strip():
                    if index == 0 or index == len(paragraphs) - 1:
                        count -= 1
                    else:
                        return False
            return count == num_paragraphs

        if inst_id == "length_constraints:nth_paragraph_first_word":
            num_paragraphs = kw.get("num_paragraphs")
            nth_paragraph = kw.get("nth_paragraph")
            first_word = kw.get("first_word")
            if (
                not isinstance(num_paragraphs, int)
                or not isinstance(nth_paragraph, int)
                or not isinstance(first_word, str)
            ):
                return False
            paragraphs = re.split(r"\n\n", answer)
            count = len(paragraphs)
            for paragraph in paragraphs:
                if not paragraph.strip():
                    count -= 1
            if nth_paragraph <= count:
                paragraph = paragraphs[nth_paragraph - 1].strip()
                if not paragraph:
                    return False
            else:
                return False
            word = paragraph.split()[0].strip()
            word = word.lstrip("'").lstrip('"')
            punctuation = {".", ",", "?", "!", "'", '"'}
            extracted = ""
            for letter in word:
                if letter in punctuation:
                    break
                extracted += letter.lower()
            return count == num_paragraphs and extracted == first_word.lower()

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

        if inst_id == "change_case:capital_word_frequency":
            frequency = kw.get("capital_frequency")
            relation = kw.get("capital_relation")
            if not isinstance(frequency, int) or not isinstance(relation, str):
                return False
            words = re.findall(r"\w+", answer)
            capital_words = [w for w in words if w.isupper()]
            count = len(capital_words)
            if relation == "less than":
                return count < frequency
            if relation == "at least":
                return count >= frequency
            return False

        if inst_id == "language:response_language":
            language = kw.get("language")
            if not isinstance(language, str):
                return False
            detected = detect_language(answer)
            # Match upstream behavior: if detection is uncertain (None),
            # count the instruction as followed (returns True).
            if detected is None:
                return True
            return detected == language

        if inst_id == "detectable_content:number_placeholders":
            num_placeholders = kw.get("num_placeholders")
            if not isinstance(num_placeholders, int):
                return False
            placeholders = re.findall(r"\[.*?\]", answer)
            return len(placeholders) >= num_placeholders

        if inst_id == "detectable_content:postscript":
            postscript_marker = kw.get("postscript_marker")
            if not isinstance(postscript_marker, str):
                return False
            value_lower = answer.lower()
            if postscript_marker == "P.P.S":
                pattern = r"\s*p\.\s?p\.\s?s.*$"
            elif postscript_marker == "P.S.":
                pattern = r"\s*p\.\s?s\..*$"
            else:
                pattern = r"\s*" + re.escape(postscript_marker.lower()) + r".*$"
            postscript = re.findall(pattern, value_lower, flags=re.MULTILINE)
            return bool(postscript)

        if inst_id == "detectable_format:number_bullet_lists":
            num_bullets = kw.get("num_bullets")
            if not isinstance(num_bullets, int):
                return False
            bullet_lists = re.findall(r"^\s*\*[^\*].*$", answer, flags=re.MULTILINE)
            bullet_lists_2 = re.findall(r"^\s*-.*$", answer, flags=re.MULTILINE)
            count = len(bullet_lists) + len(bullet_lists_2)
            return count == num_bullets

        if inst_id == "detectable_format:constrained_response":
            options = ("My answer is yes.", "My answer is no.", "My answer is maybe.")
            value = answer.strip()
            for option in options:
                if option in value:
                    return True
            return False

        if inst_id == "detectable_format:number_highlighted_sections":
            num_highlights = kw.get("num_highlights")
            if not isinstance(num_highlights, int):
                return False
            count = 0
            highlights = re.findall(r"\*[^\n\*]*\*", answer)
            double_highlights = re.findall(r"\*\*[^\n\*]*\*\*", answer)
            for highlight in highlights:
                if highlight.strip("*").strip():
                    count += 1
            for highlight in double_highlights:
                if highlight.removeprefix("**").removesuffix("**").strip():
                    count += 1
            return count >= num_highlights

        if inst_id == "detectable_format:multiple_sections":
            section_spliter = kw.get("section_spliter")
            num_sections = kw.get("num_sections")
            if not isinstance(section_spliter, str) or not isinstance(num_sections, int):
                return False
            pattern = r"\s?" + re.escape(section_spliter) + r"\s?\d+\s?"
            sections = re.split(pattern, answer)
            count = len(sections) - 1
            return count >= num_sections

        if inst_id == "detectable_format:title":
            titles = re.findall(r"<<[^\n]+>>", answer)
            for title in titles:
                if title.lstrip("<").rstrip(">").strip():
                    return True
            return False

        if inst_id == "combination:two_responses":
            valid_responses: list[str] = []
            responses = answer.split("******")
            for index, response in enumerate(responses):
                if not response.strip():
                    if index != 0 and index != len(responses) - 1:
                        return False
                else:
                    valid_responses.append(response)
            return (
                len(valid_responses) == 2
                and valid_responses[0].strip() != valid_responses[1].strip()
            )

        if inst_id == "combination:repeat_prompt":
            prompt_to_repeat = kw.get("prompt_to_repeat")
            if not isinstance(prompt_to_repeat, str):
                return False
            return answer.strip().lower().startswith(
                prompt_to_repeat.strip().lower()
            )

        if inst_id == "startend:end_checker":
            end_phrase = kw.get("end_phrase")
            if not isinstance(end_phrase, str):
                return False
            value = answer.strip().strip('"').lower()
            return value.endswith(end_phrase.strip().lower())

        if inst_id == "startend:quotation":
            value = answer.strip()
            return len(value) > 1 and value[0] == '"' and value[-1] == '"'

        # Default to False for unknown instructions to be strict
        return False
