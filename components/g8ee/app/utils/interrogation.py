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

import re
from typing import List, Optional


def extract_interrogation_questions(text: str) -> List[str]:
    """Extracts clarifying questions from an AI response wrapped in <interrogation> tags.
    
    The format expected is:
    <interrogation>
    1. Question one?
    2. Question two?
    3. Question three?
    </interrogation>
    
    Args:
        text: The full response text from the AI.
        
    Returns:
        A list of extracted question strings.
    """
    if not text:
        return []

    # Find content between <interrogation> and </interrogation> tags
    # Re.DOTALL is used to match newlines within the tags
    match = re.search(r"<interrogation>(.*?)</interrogation>", text, re.DOTALL | re.IGNORECASE)
    if not match:
        return []

    interrogation_content = match.group(1).strip()
    if not interrogation_content:
        return []

    # Split into lines and extract numbered questions
    questions = []
    # Match lines starting with optional whitespace, a number, a dot, and then the question
    # Example: "1. What is the error?" or " 2. Is this correct?"
    lines = interrogation_content.splitlines()
    for line in lines:
        line = line.strip()
        if not line:
            continue
            
        # Regex to match "1. Question" or "1) Question"
        q_match = re.match(r"^\d+[\.\)]\s*(.*)$", line)
        if q_match:
            question_text = q_match.group(1).strip()
            if question_text:
                questions.append(question_text)
        else:
            # If it doesn't match the numbered pattern but is a non-empty line within the tags,
            # we still want to capture it if it looks like a question.
            if line:
                questions.append(line)

    return questions
