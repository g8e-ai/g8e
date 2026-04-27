import re

with open('/home/bob/g8e/components/g8ee/app/security/sentinel_scrubber.py', 'r') as f:
    content = f.read()

# Make sure Enum is there
if 'from enum import IntEnum' not in content:
    content = content.replace('import re', 'import re\nfrom enum import IntEnum')

priority_class = """
class ScrubberPriority(IntEnum):
    EXACT_CREDENTIAL = 10
    URL_OR_CONNECTION = 20
    CONTEXTUAL_CREDENTIAL = 30
    GENERIC_PII = 40
"""
if 'class ScrubberPriority' not in content:
    content = content.replace('class SentinelConfig', priority_class + '\n\nclass SentinelConfig')

# Replace the RegexScrubber __init__
old_init = """    def __init__(self, name: str, pattern: str, replacement: str, flags: int = 0):
        self.name = name
        self.pattern = re.compile(pattern, flags)
        self.replacement = replacement"""

new_init = """    def __init__(self, name: str, pattern: str, replacement: str, flags: int = 0, priority: ScrubberPriority = ScrubberPriority.GENERIC_PII):
        self.name = name
        self.pattern = re.compile(pattern, flags)
        self.replacement = replacement
        self.priority = priority"""
if 'priority: ScrubberPriority' not in content:
    content = content.replace(old_init, new_init)

# I will find the block from `@classmethod\n    def _initialize_scrubbers` to `return scrubbers\n`
import ast

def find_method_range(code_str, method_name):
    lines = code_str.split('\n')
    start_idx = -1
    for i, line in enumerate(lines):
        if method_name in line and 'def ' in line:
            start_idx = i
            break
            
    if start_idx == -1: return -1, -1
    
    # backtrack to include @classmethod
    if start_idx > 0 and '@classmethod' in lines[start_idx-1]:
        start_idx -= 1
        
    end_idx = start_idx + 1
    while end_idx < len(lines):
        line = lines[end_idx]
        if line.strip() == 'return scrubbers':
            return start_idx, end_idx
        end_idx += 1
        
    return -1, -1

start, end = find_method_range(content, '_initialize_scrubbers')

new_block = """    @classmethod
    def _initialize_scrubbers(cls) -> list[RegexScrubber]:
        # More specific patterns must come before generic ones.
        scrubbers = []

        # Exact Credentials (EXACT_CREDENTIAL)
        scrubbers.append(RegexScrubber("jwt", r"\\beyJ[A-Za-z0-9_-]*\\.eyJ[A-Za-z0-9_-]*\\.[A-Za-z0-9_-]*\\b", "[JWT]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("sg_api_key", r"\\bSG\\.[0-9A-Za-z_-]{22}\\.[0-9A-Za-z_-]{43}\\b", "[API_KEY]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("github_token", r"\\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{36,}\\b", "[GITHUB_TOKEN]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("gcp_api_key", r"\\bAIza[0-9A-Za-z_-]{35}\\b", "[GCP_API_KEY]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("aws_access_key", r"\\b(AKIA|ABIA|ACCA|ASIA)[0-9A-Z]{16}\\b", "[AWS_KEY]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("slack_token", r"\\b(xoxb|xoxp|xoxs|xapp)-[0-9A-Za-z-]{24,}\\b", "[SLACK_TOKEN]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("okta_api_token", r"\\b00[A-Za-z0-9_-]{40}\\b", "[OKTA_TOKEN]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("azure_client_secret", r"\\b[A-Za-z0-9]{3,8}~[A-Za-z0-9._-]{34,}\\b", "[AZURE_SECRET]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("twilio_sid", r"\\bAC[a-f0-9]{32}\\b", "[TWILIO_SID]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("npm_token", r"\\bnpm_[A-Za-z0-9]{36}\\b", "[NPM_TOKEN]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("pypi_token", r"\\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_-]{50,}\\b", "[PYPI_TOKEN]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("discord_token", r"\\b[MN][A-Za-z\\d]{23,}\\.[\\w-]{6}\\.[\\w-]{27}\\b", "[DISCORD_TOKEN]", priority=ScrubberPriority.EXACT_CREDENTIAL))
        scrubbers.append(RegexScrubber("private_key", r"-----BEGIN[^-]+PRIVATE KEY-----[\\s\\S]*?-----END[^-]+PRIVATE KEY-----", "[PRIVATE_KEY]", priority=ScrubberPriority.EXACT_CREDENTIAL))

        # URL or Connections (URL_OR_CONNECTION)
        scrubbers.append(RegexScrubber("url_with_creds", r"https?://[^:]+:[^@]+@[^\\s<>\"{}|\\\\^`\\[\\]]+", "[URL_WITH_CREDENTIALS]", priority=ScrubberPriority.URL_OR_CONNECTION))
        scrubbers.append(RegexScrubber("conn_string", r"(?:mysql|postgres(?:ql)?|mongodb|redis|amqp|jdbc)://[^\\s]+", "[CONN_STRING]", re.IGNORECASE, priority=ScrubberPriority.URL_OR_CONNECTION))

        # Contextual Credentials (CONTEXTUAL_CREDENTIAL)
        scrubbers.append(RegexScrubber("aws_secret_key", r"aws[^\\[\\]]{0,20}secret[^\\[\\]]{0,20}['\"][0-9a-zA-Z/+=]{40}['\"]", "[AWS_SECRET]", re.IGNORECASE, priority=ScrubberPriority.CONTEXTUAL_CREDENTIAL))
        scrubbers.append(RegexScrubber("azure_secret", r"azure[^\\[\\]]{0,20}(secret|password|key)[^\\[\\]]{0,20}['\"][A-Za-z0-9_\\-\\.~]{32,}['\"]", "[AZURE_SECRET]", re.IGNORECASE, priority=ScrubberPriority.CONTEXTUAL_CREDENTIAL))
        scrubbers.append(RegexScrubber("oauth_secret", r"(client.?secret|oauth.?secret)\\s*[=:]\\s*['\"]?[A-Za-z0-9_\\-]{20,}['\"]?", "[OAUTH_SECRET]", re.IGNORECASE, priority=ScrubberPriority.CONTEXTUAL_CREDENTIAL))
        scrubbers.append(RegexScrubber("heroku_key", r"heroku[^\\[\\]]{0,20}(api.?key|token)[^\\[\\]]{0,20}['\"]?[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}['\"]?", "[HEROKU_KEY]", re.IGNORECASE, priority=ScrubberPriority.CONTEXTUAL_CREDENTIAL))
        scrubbers.append(RegexScrubber("password_config", r"(?:password|passwd|pwd|secret|token|api_key|apikey)\\s*[=:]\\s*(?!\\[)[^\\s\\[]+", "[CREDENTIAL_REFERENCE]", re.IGNORECASE, priority=ScrubberPriority.CONTEXTUAL_CREDENTIAL))
        scrubbers.append(RegexScrubber("bearer_token", r"bearer\\s+[a-zA-Z0-9_\\-\\.]+", "[BEARER_TOKEN]", re.IGNORECASE, priority=ScrubberPriority.CONTEXTUAL_CREDENTIAL))

        # Generic PII (GENERIC_PII)
        scrubbers.append(RegexScrubber("email", r"[A-Za-z0-9._%+'-]*@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}\\b", "[EMAIL]", priority=ScrubberPriority.GENERIC_PII))
        scrubbers.append(RegexScrubber("credit_card", r"\\b(?:\\d{4}[- ]?){3}\\d{4}\\b", "[PII]", priority=ScrubberPriority.GENERIC_PII))
        scrubbers.append(RegexScrubber("ssn", r"\\b\\d{3}-\\d{2}-\\d{4}\\b", "[PII]", priority=ScrubberPriority.GENERIC_PII))
        scrubbers.append(RegexScrubber("phone", r"\\b(?:\\+\\d{1,3}[- ]?)?\\(?\\d{3}\\)?[- ]?\\d{3}[- ]?\\d{4}\\b", "[PHONE]", priority=ScrubberPriority.GENERIC_PII))
        scrubbers.append(RegexScrubber("iban", r"\\b[A-Z]{2}\\d{2}[A-Z0-9]{4,30}\\b", "[IBAN]", priority=ScrubberPriority.GENERIC_PII))

        scrubbers.sort(key=lambda s: s.priority)
        return scrubbers"""

if start != -1 and end != -1:
    lines = content.split('\n')
    lines = lines[:start] + [new_block] + lines[end+1:]
    content = '\n'.join(lines)

with open('/home/bob/g8e/components/g8ee/app/security/sentinel_scrubber.py', 'w') as f:
    f.write(content)
