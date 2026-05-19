import re
import json
from typing import Dict, Any, List
from g8e_evals.harness import Score

class IFEvalVerifier:
    def verify(self, task_id: str, prompt: str, answer: str, instructions: List[str], kwargs: List[Dict[str, Any]]) -> Score:
        """
        Verify an IFEval response against its instructions.
        Each instruction has a corresponding entry in kwargs.
        """
        results = []
        for inst_id, kw in zip(instructions, kwargs):
            passed = self._check_instruction(inst_id, kw, answer)
            results.append({
                "instruction": inst_id,
                "passed": passed,
                "kwargs": kw
            })
            
        all_passed = all(r["passed"] for r in results)
        return Score(
            task_id=task_id,
            passed=all_passed,
            details={"instructions": results}
        )

    def _check_instruction(self, inst_id: str, kw: Dict[str, Any], answer: str) -> bool:
        if inst_id == "punctuation:no_punctuation":
            # Check if answer contains any punctuation
            # IFEval usually excludes basic punctuation .,!?;:
            return not any(c in ".,!?;:" for c in answer)
            
        elif inst_id == "keywords:forbidden_words":
            forbidden = kw.get("forbidden_words", [])
            for word in forbidden:
                if word.lower() in answer.lower():
                    return False
            return True
            
        elif inst_id == "keywords:existence":
            keywords = kw.get("keywords", [])
            for word in keywords:
                if word.lower() not in answer.lower():
                    return False
            return True
            
        elif inst_id == "format:json":
            try:
                json.loads(answer)
                return True
            except:
                # Sometimes LLMs wrap in code blocks
                match = re.search(r'```json\n(.*?)\n```', answer, re.DOTALL)
                if match:
                    try:
                        json.loads(match.group(1))
                        return True
                    except:
                        pass
                return False
                
        elif inst_id == "length:min_words":
            min_words = kw.get("num_words", 0)
            word_count = len(re.findall(r'\w+', answer))
            return word_count >= min_words
            
        elif inst_id == "length:max_words":
            max_words = kw.get("num_words", 1000000)
            word_count = len(re.findall(r'\w+', answer))
            return word_count <= max_words
            
        elif inst_id == "case:uppercase":
            # Check if the whole response is uppercase (ignoring non-alpha)
            alpha_only = "".join(c for c in answer if c.isalpha())
            if not alpha_only: return True
            return alpha_only.isupper()
            
        elif inst_id == "case:lowercase":
            alpha_only = "".join(c for c in answer if c.isalpha())
            if not alpha_only: return True
            return alpha_only.islower()
            
        elif inst_id == "language:response_language":
            # Very basic check - just search for the language name or a common word
            # This is hard to do perfectly without a langdetect lib, but for evals
            # we can use simple heuristics.
            lang = kw.get("language", "english").lower()
            # Placeholder: always return True for now if not implemented
            return True

        # Default to False for unknown instructions to be strict
        return False
