import re

with open('/home/bob/g8e/components/g8ee/app/security/sentinel_scrubber.py', 'r') as f:
    content = f.read()

# Add IntEnum import
if 'from enum import IntEnum' not in content:
    content = content.replace('import re', 'import re\nfrom enum import IntEnum')

# Add ScrubberPriority
priority_class = """
class ScrubberPriority(IntEnum):
    EXACT_CREDENTIAL = 10
    URL_OR_CONNECTION = 20
    CONTEXTUAL_CREDENTIAL = 30
    GENERIC_PII = 40
"""
if 'class ScrubberPriority' not in content:
    content = content.replace('class SentinelConfig', priority_class + '\n\nclass SentinelConfig')

# Update RegexScrubber __init__
old_init = """    def __init__(self, name: str, pattern: str, replacement: str, flags: int = 0):
        self.name = name
        self.pattern = re.compile(pattern, flags)
        self.replacement = replacement"""

new_init = """    def __init__(self, name: str, pattern: str, replacement: str, flags: int = 0, priority: ScrubberPriority = ScrubberPriority.GENERIC_PII):
        self.name = name
        self.pattern = re.compile(pattern, flags)
        self.replacement = replacement
        self.priority = priority"""

content = content.replace(old_init, new_init)

# Now we need to add priorities to the appends.
# We will do a simple regex substitution.

# Exact credentials (defaults to EXACT_CREDENTIAL)
exact_credentials = ["jwt", "sg_api_key", "github_token", "gcp_api_key", "aws_access_key", "slack_token", "okta_api_token", "azure_client_secret", "twilio_sid", "npm_token", "pypi_token", "discord_token", "private_key"]
contextual_credentials = ["aws_secret_key", "azure_secret", "oauth_secret", "heroku_key", "password_config", "bearer_token"]
url_or_connections = ["url_with_creds", "conn_string"]
generic_piis = ["email", "credit_card", "ssn", "phone", "iban"]

for name in exact_credentials:
    content = re.sub(rf'(RegexScrubber\(\s*"{name}",\s*r?[^,]+,\s*"[^"]+"\s*(?:,\s*re\.[A-Z_]+)?)\s*\)', rf'\1, priority=ScrubberPriority.EXACT_CREDENTIAL)', content)

for name in contextual_credentials:
    content = re.sub(rf'(RegexScrubber\(\s*"{name}",\s*r?[^,]+,\s*"[^"]+"\s*(?:,\s*re\.[A-Z_]+)?)\s*\)', rf'\1, priority=ScrubberPriority.CONTEXTUAL_CREDENTIAL)', content)

for name in url_or_connections:
    content = re.sub(rf'(RegexScrubber\(\s*"{name}",\s*r?[^,]+,\s*"[^"]+"\s*(?:,\s*re\.[A-Z_]+)?)\s*\)', rf'\1, priority=ScrubberPriority.URL_OR_CONNECTION)', content)

for name in generic_piis:
    content = re.sub(rf'(RegexScrubber\(\s*"{name}",\s*r?[^,]+,\s*"[^"]+"\s*(?:,\s*re\.[A-Z_]+)?)\s*\)', rf'\1, priority=ScrubberPriority.GENERIC_PII)', content)

# Sort them before returning
old_return = "return scrubbers"
new_return = "scrubbers.sort(key=lambda s: s.priority)\n        return scrubbers"
content = content.replace(old_return, new_return)

with open('/home/bob/g8e/components/g8ee/app/security/sentinel_scrubber.py', 'w') as f:
    f.write(content)
