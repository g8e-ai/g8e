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

"""
Sentinel Scrubber for VSE - Data Sovereignty for User Chat Messages.

This module scrubs sensitive data from user chat messages before they are sent
to the LLM provider. It implements the same zero-trust principles as the
VSA Sentinel (Go implementation) but for the VSE Python side.

Sentinel scrubs ONLY actual sensitive data:
  - Credentials (API keys, tokens, passwords, private keys)
  - PII (emails, credit cards, SSNs, phone numbers, IBANs)
  - Connection strings with embedded credentials
  - Bearer tokens and password config patterns

Sentinel does NOT scrub system/operational data the AI needs for troubleshooting:
  - IP addresses, hostnames, MAC addresses
  - File paths (including home directories)
  - URLs (unless they contain embedded credentials)
  - UUIDs, AWS ARNs, AWS account IDs
  - Filenames, hashes, base64 content

IMPORTANT: This differs from output_sanitizer.py which defends against prompt
injection. Sentinel is about DATA SOVEREIGNTY - preventing user-provided
sensitive data from reaching the cloud AI.
"""

import logging
import re

from app.models.base import VSOBaseModel

logger = logging.getLogger(__name__)


class SentinelConfig(VSOBaseModel):
    """Configuration for the Sentinel scrubber."""
    enabled: bool = True
    strict_mode: bool = False
    max_output_length: int = 50000
    log_scrubs: bool = True


class ScrubResult(VSOBaseModel):
    """Result of Sentinel scrubbing."""
    scrubbed_text: str
    was_modified: bool
    scrub_count: int
    scrub_types: list[str]


class RegexScrubber:
    """A single regex-based scrubber pattern."""

    def __init__(self, name: str, pattern: str, replacement: str, flags: int = 0):
        self.name = name
        self.pattern = re.compile(pattern, flags)
        self.replacement = replacement

    def scrub(self, text: str) -> tuple[str, int]:
        result, count = self.pattern.subn(self.replacement, text)
        return result, count


class SentinelScrubber:
    """
    Zero-trust data scrubber for user chat messages.
    
    Applies regex-based scrubbing to remove sensitive data before
    messages are sent to the cloud AI.
    """

    _scrubbers: list[RegexScrubber] = []

    def __init__(self, config: SentinelConfig):
        self.config = config or SentinelConfig()

    @classmethod
    def _initialize_scrubbers(cls) -> list[RegexScrubber]:
        # More specific patterns must come before generic ones.
        scrubbers = []

        # JWT
        scrubbers.append(RegexScrubber(
            "jwt",
            r"\beyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*\b",
            "[JWT]"
        ))

        scrubbers.append(RegexScrubber(
            "sg_api_key",
            r"\bSG\.[0-9A-Za-z_-]{22}\.[0-9A-Za-z_-]{43}\b",
            "[API_KEY]"
        ))

        scrubbers.append(RegexScrubber(
            "github_token",
            r"\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{36,}\b",
            "[GITHUB_TOKEN]"
        ))

        scrubbers.append(RegexScrubber(
            "gcp_api_key",
            r"\bAIza[0-9A-Za-z_-]{35}\b",
            "[GCP_API_KEY]"
        ))

        scrubbers.append(RegexScrubber(
            "aws_access_key",
            r"\b(AKIA|ABIA|ACCA|ASIA)[0-9A-Z]{16}\b",
            "[AWS_KEY]"
        ))

        scrubbers.append(RegexScrubber(
            "slack_token",
            r"\b(xoxb|xoxp|xoxs|xapp)-[0-9A-Za-z-]{24,}\b",
            "[SLACK_TOKEN]"
        ))

        scrubbers.append(RegexScrubber(
            "okta_api_token",
            r"\b00[A-Za-z0-9_-]{40}\b",
            "[OKTA_TOKEN]"
        ))

        scrubbers.append(RegexScrubber(
            "azure_client_secret",
            r"\b[A-Za-z0-9]{3,8}~[A-Za-z0-9._-]{34,}\b",
            "[AZURE_SECRET]"
        ))

        scrubbers.append(RegexScrubber(
            "twilio_sid",
            r"\bAC[a-f0-9]{32}\b",
            "[TWILIO_SID]"
        ))

        scrubbers.append(RegexScrubber(
            "npm_token",
            r"\bnpm_[A-Za-z0-9]{36}\b",
            "[NPM_TOKEN]"
        ))

        scrubbers.append(RegexScrubber(
            "pypi_token",
            r"\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_-]{50,}\b",
            "[PYPI_TOKEN]"
        ))

        scrubbers.append(RegexScrubber(
            "discord_token",
            r"\b[MN][A-Za-z\d]{23,}\.[\w-]{6}\.[\w-]{27}\b",
            "[DISCORD_TOKEN]"
        ))

        scrubbers.append(RegexScrubber(
            "private_key",
            r"-----BEGIN[^-]+PRIVATE KEY-----[\s\S]*?-----END[^-]+PRIVATE KEY-----",
            "[PRIVATE_KEY]"
        ))

        scrubbers.append(RegexScrubber(
            "aws_secret_key",
            r'aws.{0,20}secret.{0,20}[\'"][0-9a-zA-Z/+=]{40}[\'"]',
            "[AWS_SECRET]",
            re.IGNORECASE
        ))

        scrubbers.append(RegexScrubber(
            "azure_secret",
            r'azure.{0,20}(secret|password|key).{0,20}[\'"][A-Za-z0-9_\-\.~]{32,}[\'"]',
            "[AZURE_SECRET]",
            re.IGNORECASE
        ))

        scrubbers.append(RegexScrubber(
            "oauth_secret",
            r'(client.?secret|oauth.?secret)\s*[=:]\s*[\'"]?[A-Za-z0-9_\-]{20,}[\'"]?',
            "[OAUTH_SECRET]",
            re.IGNORECASE
        ))

        scrubbers.append(RegexScrubber(
            "heroku_key",
            r'heroku.{0,20}(api.?key|token).{0,20}[\'"]?[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}[\'"]?',
            "[HEROKU_KEY]",
            re.IGNORECASE
        ))

        # URL-with-creds must come before email (prevents matching host portion of user:pass@host.com)
        scrubbers.append(RegexScrubber(
            "url_with_creds",
            r'https?://[^:]+:[^@]+@[^\s<>"{}|\\^`\[\]]+',
            "[URL_WITH_CREDENTIALS]"
        ))

        scrubbers.append(RegexScrubber(
            "conn_string",
            r"(?:mysql|postgres|mongodb|redis|amqp|jdbc)://[^\s]+",
            "[CONN_STRING]",
            re.IGNORECASE
        ))

        scrubbers.append(RegexScrubber(
            "email",
            r"[A-Za-z0-9._%+\'-]*@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b",
            "[EMAIL]"
        ))

        scrubbers.append(RegexScrubber(
            "credit_card",
            r"\b(?:\d{4}[- ]?){3}\d{4}\b",
            "[PII]"
        ))

        scrubbers.append(RegexScrubber(
            "ssn",
            r"\b\d{3}-\d{2}-\d{4}\b",
            "[PII]"
        ))

        scrubbers.append(RegexScrubber(
            "phone",
            r"\b(?:\+\d{1,3}[- ]?)?\(?\d{3}\)?[- ]?\d{3}[- ]?\d{4}\b",
            "[PHONE]"
        ))

        scrubbers.append(RegexScrubber(
            "password_config",
            r"(?:password|passwd|pwd|secret|token|api_key|apikey)\s*[=:]\s*(?!\[)[^\s\[]+",
            "[CREDENTIAL_REFERENCE]",
            re.IGNORECASE
        ))

        scrubbers.append(RegexScrubber(
            "iban",
            r"\b[A-Z]{2}\d{2}[A-Z0-9]{4,30}\b",
            "[IBAN]"
        ))

        scrubbers.append(RegexScrubber(
            "bearer_token",
            r"bearer\s+[a-zA-Z0-9_\-\.]+",
            "[BEARER_TOKEN]",
            re.IGNORECASE
        ))

        return scrubbers

    def is_enabled(self) -> bool:
        """Check if Sentinel scrubbing is enabled."""
        return self.config.enabled

    def scrub_text(self, text: str) -> str:
        """
        Scrub sensitive data from text.
        
        Args:
            text: Input text to scrub
            
        Returns:
            Scrubbed text with sensitive data replaced by placeholders
        """
        result = self.scrub(text)
        return result.scrubbed_text

    def scrub(self, text: str) -> ScrubResult:
        """
        Scrub sensitive data from text with detailed result.
        
        Args:
            text: Input text to scrub
            
        Returns:
            ScrubResult with scrubbed text and metadata
        """
        if not text:
            return ScrubResult(
                scrubbed_text="",
                was_modified=False,
                scrub_count=0,
                scrub_types=[]
            )

        if not self.config.enabled:
            return ScrubResult(
                scrubbed_text=text,
                was_modified=False,
                scrub_count=0,
                scrub_types=[]
            )

        result = text
        total_count = 0
        scrub_types = []

        for scrubber in self._scrubbers:
            new_result, count = scrubber.scrub(result)
            if count > 0:
                result = new_result
                total_count += count
                scrub_types.append(scrubber.name)

        if self.config.max_output_length > 0 and len(result) > self.config.max_output_length:
            result = result[:self.config.max_output_length] + "... [TRUNCATED]"

        was_modified = total_count > 0

        if was_modified and self.config.log_scrubs:
            logger.info(
                "Sentinel scrubbed %d patterns from user message: %s",
                total_count,
                scrub_types,
                extra={"scrub_count": total_count, "scrub_types": scrub_types}
            )

        return ScrubResult(
            scrubbed_text=result,
            was_modified=was_modified,
            scrub_count=total_count,
            scrub_types=scrub_types
        )


SentinelScrubber._scrubbers = SentinelScrubber._initialize_scrubbers()


_default_scrubber: SentinelScrubber | None = None


def get_sentinel_scrubber(config: SentinelConfig | None = None) -> SentinelScrubber:
    global _default_scrubber
    if _default_scrubber is None:
        if config is None:
            config = SentinelConfig()
        _default_scrubber = SentinelScrubber(config)
    return _default_scrubber


def scrub_user_message(message: str) -> str:
    return get_sentinel_scrubber().scrub_text(message)
