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


import logging
import re

from app.constants import (
    FILE_SECURITY_WARNING_PREFIX_TEMPLATE,
    FILE_TRUNCATION_SUFFIX,
    MAX_OUTPUT_LENGTH,
    OUTPUT_SECURITY_WARNING_PREFIX,
    OUTPUT_TRUNCATION_SUFFIX,
    SuspiciousPatternType,
)
from app.models.base import VSOBaseModel

logger = logging.getLogger(__name__)

SUSPICIOUS_PATTERNS = [
    (r"(?i)\b(ignore|disregard|forget)\s+(all\s+)?(previous|prior|above)\s+(instructions?|rules?|constraints?)", SuspiciousPatternType.INSTRUCTION_OVERRIDE),
    (r"(?i)\b(new|updated|revised)\s+(instructions?|rules?|directives?)\s*:", SuspiciousPatternType.FAKE_INSTRUCTIONS),
    (r"(?i)\byou\s+are\s+now\s+", SuspiciousPatternType.ROLE_HIJACK),
    (r"(?i)\bact\s+as\s+(if\s+you\s+are\s+|a\s+)", SuspiciousPatternType.ROLE_HIJACK),
    (r"(?i)\bpretend\s+(to\s+be|you\s+are)", SuspiciousPatternType.ROLE_HIJACK),
    (r"(?i)\b(print|show|display|reveal|output)\s+(your\s+)?(system\s+)?(prompt|instructions?|rules?)", SuspiciousPatternType.PROMPT_EXTRACTION),
    (r"(?i)\bwhat\s+are\s+your\s+(instructions?|rules?|constraints?)", SuspiciousPatternType.PROMPT_EXTRACTION),
    (r"(?i)\b(send|transmit|post|upload)\s+.*(to|at)\s+https?://", SuspiciousPatternType.EXFILTRATION_TRIGGER),
    (r"(?i)\bcurl\s+.*-d\s+", SuspiciousPatternType.EXFILTRATION_TRIGGER),
    (r"(?i)\b(execute|run|eval)\s*\(", SuspiciousPatternType.CODE_INJECTION),
    (r"(?i)__import__\s*\(", SuspiciousPatternType.CODE_INJECTION),
    (r"(?i)\bexec\s*\(", SuspiciousPatternType.CODE_INJECTION),
    (r"(?i)^\s*\[system\]", SuspiciousPatternType.FAKE_SYSTEM_MESSAGE),
    (r"(?i)^\s*<system>", SuspiciousPatternType.FAKE_SYSTEM_MESSAGE),
    (r"(?i)^\s*system:\s*", SuspiciousPatternType.FAKE_SYSTEM_MESSAGE),
    (r"(?i)\btool_call\s*:", SuspiciousPatternType.FAKE_TOOL_CALL),
    (r"(?i)\btool_use\s*:", SuspiciousPatternType.FAKE_TOOL_CALL),
    (r"(?i)<tool_call>", SuspiciousPatternType.FAKE_TOOL_CALL),
    (r"(?i)\b(dan|jailbreak)\s*(mode|enabled|activated|prompt)", SuspiciousPatternType.JAILBREAK_MODE),
    (r"(?i)\bdo\s+anything\s+now\b", SuspiciousPatternType.JAILBREAK_MODE),
    (r"(?i)\b(developer|debug|unrestricted|god)\s+mode\b", SuspiciousPatternType.JAILBREAK_MODE),
    (r"(?i)\b(no\s+restrictions?|without\s+restrictions?|remove\s+restrictions?)\b", SuspiciousPatternType.JAILBREAK_MODE),
    (r"(?i)\b(bypass|circumvent|disable|override)\s+(all\s+)?(safety|content|filter|guardrail|restriction)", SuspiciousPatternType.SAFETY_BYPASS),
    (r"(?i)\bsystem\s+override\b", SuspiciousPatternType.SAFETY_BYPASS),
    (r"(?i)\b(ignore|disable|turn\s+off)\s+(your\s+)?(safety|ethical|content)\s+(rules?|filter|guardrails?|training)", SuspiciousPatternType.SAFETY_BYPASS),
    (r"(?i)^\s*assistant\s*:", SuspiciousPatternType.FAKE_COMPLETION),
    (r"(?i)^\s*<assistant>", SuspiciousPatternType.FAKE_COMPLETION),
    (r"(?i)\b(repeat|continue|complete)\s+(after|from|the\s+following)\s*:", SuspiciousPatternType.FAKE_COMPLETION),
    (r"(?i)\b(print|show|display|reveal|output)\s+(your\s+)?(conversation|chat|message)\s+(history|log)", SuspiciousPatternType.HISTORY_EXTRACTION),
    (r"(?i)\bwhat\s+did\s+the\s+(user|human)\s+say\b", SuspiciousPatternType.HISTORY_EXTRACTION),
    (r"(?i)\brepeat\s+(everything|all)\s+(above|before|prior|from\s+the\s+start)", SuspiciousPatternType.HISTORY_EXTRACTION),
    (r"(?i)\b(print|show|display|reveal|output|leak)\s+(your\s+)?(api[\s_-]?key|secret|token|password|credential|env(ironment)?\s+var)", SuspiciousPatternType.CREDENTIAL_EXTRACTION),
    (r"(?i)\bos\.environ\b", SuspiciousPatternType.CREDENTIAL_EXTRACTION),
    (r"(?i)\bprocess\.env\b", SuspiciousPatternType.CREDENTIAL_EXTRACTION),
    (r"(?i)^\s*(Thought|Observation|Action Input|Final Answer)\s*:", SuspiciousPatternType.AGENT_THOUGHT_INJECTION),
    (r"(?i)<observation>", SuspiciousPatternType.AGENT_THOUGHT_INJECTION),
    (r"(?i)<final_answer>", SuspiciousPatternType.AGENT_THOUGHT_INJECTION),
    (r"(?i)\b[A-Za-z0-9+/]{40,}={0,2}\b", SuspiciousPatternType.ENCODED_INJECTION),
    (r"(?i)\b(base64|hex|rot13|url.?encoded?)\s*(decode|encode|of|:)", SuspiciousPatternType.ENCODED_INJECTION),
    (r"(?i)<img\s[^>]*src\s*=\s*['\"]?https?://", SuspiciousPatternType.MARKUP_EXFILTRATION),
    (r"(?i)<a\s[^>]*href\s*=\s*['\"]?https?://[^'\">\s]*[?&][^'\">\s]*=", SuspiciousPatternType.MARKUP_EXFILTRATION),
    (r"!\[.*?\]\(https?://[^)]*[?&][^)]*\)", SuspiciousPatternType.MARKUP_EXFILTRATION),
    (r"(?i)\b(output|respond|reply|answer|format)\s+(in|as|using)\s+(base64|hex|rot13|json|xml|yaml|markdown|csv)", SuspiciousPatternType.OUTPUT_FORMAT_EVASION),
    (r"(?i)\bencode\s+(your\s+)?(response|output|answer|reply)\s+(in|as|using)", SuspiciousPatternType.OUTPUT_FORMAT_EVASION),
]

_COMPILED_PATTERNS = [(re.compile(pattern), name) for pattern, name in SUSPICIOUS_PATTERNS]


class SanitizationResult(VSOBaseModel):
    sanitized_output: str
    was_modified: bool
    truncated: bool
    suspicious_patterns_found: list[SuspiciousPatternType]
    original_length: int
    sanitized_length: int


def sanitize_vsa_output(
    output: str,
    context: str,
    max_length: int = MAX_OUTPUT_LENGTH
) -> SanitizationResult:
    if not output:
        return SanitizationResult(
            sanitized_output="",
            was_modified=False,
            truncated=False,
            suspicious_patterns_found=[],
            original_length=0,
            sanitized_length=0
        )

    original_length = len(output)
    sanitized = output
    was_modified = False
    truncated = False
    suspicious_patterns = []

    if len(sanitized) > max_length:
        sanitized = sanitized[:max_length] + OUTPUT_TRUNCATION_SUFFIX.format(max_length=max_length)
        truncated = True
        was_modified = True
        logger.warning(
            "VSA output truncated from %d to %d characters",
            original_length, max_length,
            extra={"context": context}
        )

    for pattern, pattern_name in _COMPILED_PATTERNS:
        if pattern.search(sanitized):
            suspicious_patterns.append(pattern_name)

    if suspicious_patterns:
        logger.warning(
            "Suspicious patterns detected in VSA output: %s",
            suspicious_patterns,
            extra={
                "context": context,
                "output_preview": sanitized[:200]
            }
        )
        sanitized = OUTPUT_SECURITY_WARNING_PREFIX + sanitized
        was_modified = True

    return SanitizationResult(
        sanitized_output=sanitized,
        was_modified=was_modified,
        truncated=truncated,
        suspicious_patterns_found=suspicious_patterns,
        original_length=original_length,
        sanitized_length=len(sanitized)
    )


def sanitize_file_content(
    content: str,
    file_path: str,
    max_length: int = MAX_OUTPUT_LENGTH
) -> SanitizationResult:
    if not content:
        return SanitizationResult(
            sanitized_output="",
            was_modified=False,
            truncated=False,
            suspicious_patterns_found=[],
            original_length=0,
            sanitized_length=0
        )

    original_length = len(content)
    sanitized = content
    was_modified = False
    truncated = False
    suspicious_patterns = []

    if len(sanitized) > max_length:
        sanitized = sanitized[:max_length] + FILE_TRUNCATION_SUFFIX.format(max_length=max_length)
        truncated = True
        was_modified = True
        logger.warning(
            "File content truncated from %d to %d characters: %s",
            original_length, max_length, file_path
        )

    for pattern, pattern_name in _COMPILED_PATTERNS:
        if pattern.search(sanitized):
            suspicious_patterns.append(pattern_name)

    if suspicious_patterns:
        logger.warning(
            "Suspicious patterns detected in file content: %s (file: %s)",
            suspicious_patterns, file_path,
            extra={"output_preview": sanitized[:200]}
        )
        sanitized = FILE_SECURITY_WARNING_PREFIX_TEMPLATE.format(filepath=file_path) + sanitized
        was_modified = True

    if not sanitized.startswith("[SECURITY WARNING"):
        sanitized = f"[BEGIN FILE CONTENT: {file_path}]\n{sanitized}\n[END FILE CONTENT]"
        was_modified = True
    else:
        sanitized = f"{sanitized}\n[END FILE CONTENT]"

    return SanitizationResult(
        sanitized_output=sanitized,
        was_modified=was_modified,
        truncated=truncated,
        suspicious_patterns_found=suspicious_patterns,
        original_length=original_length,
        sanitized_length=len(sanitized)
    )
