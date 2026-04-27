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

import pytest
from app.constants import ScrubType
from app.models.base import G8eBaseModel

from app.security.sentinel_scrubber import (
    ScrubResult,
    SentinelConfig,
    SentinelScrubber,
    get_sentinel_scrubber,
    scrub_user_message,
)

pytestmark = [pytest.mark.unit]

_scrubber = SentinelScrubber(SentinelConfig())


class TestSentinelConfig:
    def test_defaults(self):
        cfg = SentinelConfig()
        assert cfg.enabled is True
        assert cfg.strict_mode is False
        assert cfg.max_output_length == 50000
        assert cfg.log_scrubs is True

    def test_custom_values(self):
        cfg = SentinelConfig(enabled=False, max_output_length=100, log_scrubs=False)
        assert cfg.enabled is False
        assert cfg.max_output_length == 100
        assert cfg.log_scrubs is False

    def test_is_pydantic_model(self):
        assert issubclass(SentinelConfig, G8eBaseModel)


class TestScrubResult:
    def test_is_pydantic_model(self):
        assert issubclass(ScrubResult, G8eBaseModel)

    def test_fields(self):
        result = ScrubResult(
            scrubbed_text="clean",
            was_modified=True,
            scrub_count=2,
            scrub_types=[ScrubType.EMAIL, ScrubType.JWT],
        )
        assert result.scrubbed_text == "clean"
        assert result.was_modified is True
        assert result.scrub_count == 2
        assert result.scrub_types == [ScrubType.EMAIL, ScrubType.JWT]


class TestSentinelScrubberEmptyAndDisabled:
    def test_empty_string_returns_empty(self):
        result = _scrubber.scrub("")
        assert result.scrubbed_text == ""
        assert result.was_modified is False
        assert result.scrub_count == 0
        assert result.scrub_types == []

    def test_none_falsy_returns_empty(self):
        result = _scrubber.scrub(None)
        assert result.scrubbed_text == ""
        assert result.was_modified is False

    def test_disabled_returns_text_unchanged(self):
        scrubber = SentinelScrubber(SentinelConfig(enabled=False))
        text = "my password=supersecret and email user@example.com"
        result = scrubber.scrub(text)
        assert result.scrubbed_text == text
        assert result.was_modified is False
        assert result.scrub_count == 0

    def test_is_enabled_reflects_config(self):
        assert SentinelScrubber(SentinelConfig(enabled=True)).is_enabled() is True
        assert SentinelScrubber(SentinelConfig(enabled=False)).is_enabled() is False

    def test_clean_text_is_unchanged(self):
        text = "nginx is returning a 502 on /api/health — check the upstream config"
        result = _scrubber.scrub(text)
        assert result.scrubbed_text == text
        assert result.was_modified is False


class TestSentinelScrubberTruncation:
    def test_truncates_at_limit(self):
        scrubber = SentinelScrubber(SentinelConfig(max_output_length=10))
        result = scrubber.scrub("a" * 20)
        assert result.scrubbed_text == "aaaaaaaaaa... [TRUNCATED]"

    def test_no_truncation_when_within_limit(self):
        scrubber = SentinelScrubber(SentinelConfig(max_output_length=100))
        text = "short message"
        result = scrubber.scrub(text)
        assert result.scrubbed_text == text

    def test_truncation_disabled_when_zero(self):
        scrubber = SentinelScrubber(SentinelConfig(max_output_length=0))
        text = "x" * 1000
        result = scrubber.scrub(text)
        assert "[TRUNCATED]" not in result.scrubbed_text


class TestScrubTextConvenienceMethod:
    def test_returns_string(self):
        result = _scrubber.scrub_text("contact me at admin@example.com")
        assert isinstance(result, str)
        assert "[EMAIL]" in result
        assert "@" not in result


class TestPhase1ServiceTokens:
    def test_jwt_scrubbed(self):
        jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyMTIzIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
        result = _scrubber.scrub(f"my token is {jwt}")
        assert "[JWT]" in result.scrubbed_text
        assert "eyJ" not in result.scrubbed_text
        assert ScrubType.JWT in result.scrub_types

    def test_github_token_ghp_scrubbed(self):
        token = "ghp_" + "A" * 36
        result = _scrubber.scrub(f"set it to {token} in CI")
        assert "[GITHUB_TOKEN]" in result.scrubbed_text
        assert token not in result.scrubbed_text
        assert ScrubType.GITHUB_TOKEN in result.scrub_types

    def test_github_token_ghs_scrubbed(self):
        token = "ghs_" + "B" * 40
        result = _scrubber.scrub(token)
        assert "[GITHUB_TOKEN]" in result.scrubbed_text

    def test_gcp_api_key_scrubbed(self):
        key = "AIza" + "A" * 35
        result = _scrubber.scrub(f"key={key}")
        assert "[GCP_API_KEY]" in result.scrubbed_text
        assert key not in result.scrubbed_text

    def test_aws_access_key_akia_scrubbed(self):
        key = "AKIAIOSFODNN7EXAMPLE"
        result = _scrubber.scrub(f"aws_access_key_id = {key}")
        assert "[AWS_KEY]" in result.scrubbed_text
        assert key not in result.scrubbed_text

    def test_aws_access_key_asia_scrubbed(self):
        key = "ASIAIOSFODNN7EXAMPLE"
        result = _scrubber.scrub(key)
        assert "[AWS_KEY]" in result.scrubbed_text

    def test_slack_token_xoxb_scrubbed(self):
        token = "xoxb-1234567890123-1234567890123-abc123def456ghi"
        result = _scrubber.scrub(f"use {token} to post")
        assert "[SLACK_TOKEN]" in result.scrubbed_text
        assert token not in result.scrubbed_text

    def test_okta_token_scrubbed(self):
        token = "00" + "A" * 40
        result = _scrubber.scrub(f"the okta value is {token}")
        assert "[OKTA_TOKEN]" in result.scrubbed_text

    def test_npm_token_scrubbed(self):
        token = "npm_" + "A" * 36
        result = _scrubber.scrub(f"publish with {token}")
        assert "[NPM_TOKEN]" in result.scrubbed_text
        assert token not in result.scrubbed_text

    def test_twilio_sid_scrubbed(self):
        sid = "AC" + "a" * 32
        result = _scrubber.scrub(f"account_sid = {sid}")
        assert "[TWILIO_SID]" in result.scrubbed_text
        assert sid not in result.scrubbed_text

    def test_private_key_block_scrubbed(self):
        key_block = "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----"
        result = _scrubber.scrub(f"here is the key:\n{key_block}")
        assert "[PRIVATE_KEY]" in result.scrubbed_text
        assert "MIIEpAIBAAKCAQEA" not in result.scrubbed_text
        assert ScrubType.PRIVATE_KEY in result.scrub_types


class TestPhase2ConfigCredentials:
    def test_aws_secret_key_in_config_scrubbed(self):
        text = "aws_secret_access_key = 'wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'"
        result = _scrubber.scrub(text)
        assert "[AWS_SECRET]" in result.scrubbed_text
        assert "wJalrXUtnFEMI" not in result.scrubbed_text

    def test_heroku_key_scrubbed(self):
        text = "heroku_api_key = '12345678-1234-1234-1234-123456789012'"
        result = _scrubber.scrub(text)
        assert "[HEROKU_KEY]" in result.scrubbed_text

    def test_oauth_client_secret_scrubbed(self):
        text = "client_secret = AbCdEfGhIjKlMnOpQrStUvWx"
        result = _scrubber.scrub(text)
        assert "[OAUTH_SECRET]" in result.scrubbed_text
        assert "AbCdEfGhIjKlMnOpQrStUvWx" not in result.scrubbed_text


class TestPhase3PII:
    def test_email_scrubbed(self):
        result = _scrubber.scrub("contact alice@example.com for help")
        assert "[EMAIL]" in result.scrubbed_text
        assert "alice@example.com" not in result.scrubbed_text
        assert ScrubType.EMAIL in result.scrub_types

    def test_multiple_emails_scrubbed(self):
        result = _scrubber.scrub("from: a@foo.com to: b@bar.org")
        assert result.scrubbed_text.count("[EMAIL]") == 2

    def test_url_with_credentials_scrubbed(self):
        text = "connecting to https://admin:s3cr3t@db.internal.example.com/prod"
        result = _scrubber.scrub(text)
        assert "[URL_WITH_CREDENTIALS]" in result.scrubbed_text
        assert "s3cr3t" not in result.scrubbed_text
        assert ScrubType.URL_WITH_CREDS in result.scrub_types

    def test_url_without_credentials_preserved(self):
        text = "see the docs at https://docs.example.com/guide"
        result = _scrubber.scrub(text)
        assert result.scrubbed_text == text
        assert result.was_modified is False

    def test_postgres_connection_string_scrubbed(self):
        text = "DATABASE_URL=postgres://user:pass@localhost:5432/mydb"
        result = _scrubber.scrub(text)
        assert "[CONN_STRING]" in result.scrubbed_text
        assert "user:pass" not in result.scrubbed_text
        assert ScrubType.CONN_STRING in result.scrub_types

    def test_placeholder_not_cannibalized_by_later_contextual_scrubber(self):
        # Regression: with the original `.{0,20}` gap, the aws_secret_key
        # pattern would match across an already-inserted [AWS_KEY] placeholder
        # (because [AWS_KEY] starts with literal "AWS"), eating the placeholder
        # and producing garbled output like "Connect with [[AWS_SECRET]".
        text = (
            'Connect with AKIAIOSFODNN7EXAMPLE and '
            'aws_secret_access_key="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"'
        )
        result = _scrubber.scrub(text)
        assert "[AWS_KEY]" in result.scrubbed_text
        assert "[AWS_SECRET]" in result.scrubbed_text
        assert "AKIAIOSFODNN7EXAMPLE" not in result.scrubbed_text
        assert "wJalrXUtnFEMI" not in result.scrubbed_text

    def test_postgresql_connection_string_scrubbed(self):
        # Regression: the canonical libpq URI scheme is `postgresql://`,
        # not just `postgres://`. Both must be scrubbed.
        text = "Connection string: postgresql://dbuser:Hunter2Pass@localhost:5432/mydb"
        result = _scrubber.scrub(text)
        assert "[CONN_STRING]" in result.scrubbed_text
        assert "dbuser:Hunter2Pass" not in result.scrubbed_text
        assert ScrubType.CONN_STRING in result.scrub_types

    def test_mongodb_connection_string_scrubbed(self):
        text = "mongodb://root:password@mongo.internal:27017/admin"
        result = _scrubber.scrub(text)
        assert "[CONN_STRING]" in result.scrubbed_text

    def test_redis_connection_string_scrubbed(self):
        text = "redis://default:mypassword@redis-host:6379/0"
        result = _scrubber.scrub(text)
        assert "[CONN_STRING]" in result.scrubbed_text


class TestPhase4FinancialAndCredentialPatterns:
    def test_credit_card_scrubbed(self):
        result = _scrubber.scrub("card: 4111 1111 1111 1111")
        assert "[PII]" in result.scrubbed_text
        assert "4111" not in result.scrubbed_text

    def test_credit_card_no_spaces_scrubbed(self):
        result = _scrubber.scrub("card 4111111111111111")
        assert "[PII]" in result.scrubbed_text

    def test_ssn_scrubbed(self):
        result = _scrubber.scrub("SSN is 123-45-6789")
        assert "[PII]" in result.scrubbed_text
        assert "123-45-6789" not in result.scrubbed_text
        assert ScrubType.SSN in result.scrub_types

    def test_phone_us_scrubbed(self):
        result = _scrubber.scrub("call me at 555-867-5309")
        assert "[PHONE]" in result.scrubbed_text
        assert "555-867-5309" not in result.scrubbed_text

    def test_phone_international_scrubbed(self):
        result = _scrubber.scrub("+1-800-555-0199")
        assert "[PHONE]" in result.scrubbed_text

    def test_password_config_pattern_scrubbed(self):
        result = _scrubber.scrub("password=hunter2")
        assert "[CREDENTIAL_REFERENCE]" in result.scrubbed_text
        assert "hunter2" not in result.scrubbed_text

    def test_api_key_config_pattern_scrubbed(self):
        result = _scrubber.scrub("api_key=sk-abc123xyz")
        assert "[CREDENTIAL_REFERENCE]" in result.scrubbed_text

    def test_bearer_token_scrubbed(self):
        result = _scrubber.scrub("Authorization: Bearer eyABCdef123456")
        assert "[BEARER_TOKEN]" in result.scrubbed_text
        assert "eyABCdef123456" not in result.scrubbed_text
        assert ScrubType.BEARER_TOKEN in result.scrub_types

    def test_iban_scrubbed(self):
        result = _scrubber.scrub("IBAN: GB29NWBK60161331926819")
        assert "[IBAN]" in result.scrubbed_text
        assert "GB29NWBK60161331926819" not in result.scrubbed_text


class TestPreservedOperationalData:
    def test_ip_address_preserved(self):
        text = "the server at 192.168.1.100 is unreachable"
        result = _scrubber.scrub(text)
        assert "192.168.1.100" in result.scrubbed_text

    def test_hostname_preserved(self):
        text = "ssh into prod-web-01.internal.example.com"
        result = _scrubber.scrub(text)
        assert "prod-web-01.internal.example.com" in result.scrubbed_text

    def test_file_path_preserved(self):
        text = "edit /etc/nginx/nginx.conf and restart"
        result = _scrubber.scrub(text)
        assert "/etc/nginx/nginx.conf" in result.scrubbed_text

    def test_home_directory_path_preserved(self):
        text = "the config is at /home/user/.ssh/config"
        result = _scrubber.scrub(text)
        assert "/home/user/.ssh/config" in result.scrubbed_text

    def test_uuid_preserved(self):
        text = "investigation_id = 550e8400-e29b-41d4-a716-446655440000"
        result = _scrubber.scrub(text)
        assert "550e8400-e29b-41d4-a716-446655440000" in result.scrubbed_text

    def test_url_without_creds_preserved(self):
        text = "fetch from https://api.github.com/repos/owner/repo/releases"
        result = _scrubber.scrub(text)
        assert "https://api.github.com/repos/owner/repo/releases" in result.scrubbed_text

    def test_sha256_hash_preserved(self):
        text = "sha256: a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3"
        result = _scrubber.scrub(text)
        assert "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3" in result.scrubbed_text

    def test_aws_arn_preserved(self):
        text = "arn:aws:iam::123456789012:role/MyRole"
        result = _scrubber.scrub(text)
        assert "arn:aws:iam::123456789012:role/MyRole" in result.scrubbed_text


class TestScrubResultMetadata:
    def test_was_modified_true_when_scrubbed(self):
        result = _scrubber.scrub("email: test@example.com")
        assert result.was_modified is True

    def test_was_modified_false_when_clean(self):
        result = _scrubber.scrub("check disk space with df -h")
        assert result.was_modified is False

    def test_scrub_count_reflects_total_matches(self):
        result = _scrubber.scrub("a@b.com and c@d.com")
        assert result.scrub_count == 2

    def test_scrub_types_lists_pattern_names(self):
        result = _scrubber.scrub("user@example.com token=abc123xyz bearer abc")
        assert ScrubType.EMAIL in result.scrub_types

    def test_scrub_types_no_duplicates_per_pattern(self):
        result = _scrubber.scrub("a@b.com and c@d.com")
        assert result.scrub_types.count(ScrubType.EMAIL) == 1


class TestMultiPatternMessage:
    def test_mixed_pii_and_credentials_scrubbed(self):
        text = (
            "Contact alice@corp.com — use token ghp_" + "X" * 40 +
            " to push. DB is postgres://alice:secret123@db.internal/prod"
        )
        result = _scrubber.scrub(text)
        assert "[EMAIL]" in result.scrubbed_text
        assert "[GITHUB_TOKEN]" in result.scrubbed_text
        assert "[CONN_STRING]" in result.scrubbed_text
        assert "alice@corp.com" not in result.scrubbed_text
        assert "secret123" not in result.scrubbed_text
        assert result.scrub_count >= 3


class TestSingletonAndConvenienceFunctions:
    def test_get_sentinel_scrubber_returns_scrubber(self):
        scrubber = get_sentinel_scrubber(SentinelConfig())
        assert isinstance(scrubber, SentinelScrubber)

    def test_scrub_user_message_returns_string(self, monkeypatch):
        # We need to ensure the global _default_scrubber is initialized for scrub_user_message
        get_sentinel_scrubber(SentinelConfig())
        result = scrub_user_message("email me at user@example.com")
        assert isinstance(result, str)
        assert "[EMAIL]" in result
        assert "user@example.com" not in result

    def test_scrub_user_message_clean_input_unchanged(self):
        # Ensure initialized
        get_sentinel_scrubber(SentinelConfig())
        text = "what is the status of nginx"
        result = scrub_user_message(text)
        assert result == text
