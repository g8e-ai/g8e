# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Hand-authored environment variable names class.
#
# INV-ENV-04 (docs/devs/devs.md): environment variables are read only for
# secrets, user-specific endpoints or identities, and host facts. Platform
# configuration (service URLs, runtime/PKI/secrets paths, provider, models,
# token limits) is a typed default or an explicit launch argument
# (app.constants.bootstrap, app.serve). Every key below has a category in
# ENV_VAR_CATEGORY; ``violation`` keys are the scheduled env-config-purge
# inventory and the set may only shrink (tests/unit/constants/test_env_vars_registry.py).


class EnvVar:
    """Collection of canonical G8E environment variable names."""

    G8E_G8EE_HTTPS_PORT = "G8E_G8EE_HTTPS_PORT"
    LLM_OPENAI_API_KEY = "G8E_LLM_OPENAI_API_KEY"
    LLM_OPENAI_ENDPOINT = "G8E_LLM_OPENAI_ENDPOINT"
    LLM_OLLAMA_ENDPOINT = "G8E_LLM_OLLAMA_ENDPOINT"
    LLM_OLLAMA_API_KEY = "G8E_LLM_OLLAMA_API_KEY"
    LLM_ANTHROPIC_API_KEY = "G8E_LLM_ANTHROPIC_API_KEY"
    LLM_ANTHROPIC_ENDPOINT = "G8E_LLM_ANTHROPIC_ENDPOINT"
    LLM_GEMINI_API_KEY = "G8E_LLM_GEMINI_API_KEY"
    LLM_LLAMACPP_ENDPOINT = "G8E_LLM_LLAMACPP_ENDPOINT"
    LLM_LLAMACPP_API_KEY = "G8E_LLM_LLAMACPP_API_KEY"
    LLM_JEV_MODEL = "G8E_LLM_JEV_MODEL"
    SESSION_ENCRYPTION_KEY = "G8E_SESSION_ENCRYPTION_KEY"
    INTERNAL_API_KEY = "G8E_INTERNAL_API_KEY"
    ALLOWED_ORIGINS = "G8E_ALLOWED_ORIGINS"
    PASSKEY_RP_NAME = "G8E_PASSKEY_RP_NAME"
    PASSKEY_RP_ID = "G8E_PASSKEY_RP_ID"
    PASSKEY_ORIGIN = "G8E_PASSKEY_ORIGIN"
    VERTEX_SEARCH_API_KEY = "G8E_VERTEX_SEARCH_API_KEY"
    GOOGLE_SEARCH_API_KEY = "G8E_GOOGLE_SEARCH_API_KEY"
    OPERATOR_SESSION_ID = "G8E_OPERATOR_SESSION_ID"
    INTERNAL_AUTH_TOKEN = "G8E_INTERNAL_AUTH_TOKEN"
    DEVICE_TOKEN = "G8E_DEVICE_TOKEN"
    DATA_DIR = "G8E_DATA_DIR"
    OPENCLAW_GATEWAY_TOKEN = "G8E_OPENCLAW_GATEWAY_TOKEN"
    SHELL = "SHELL"
    LANG = "LANG"
    TERM = "TERM"
    TZ = "TZ"
    PATH = "PATH"
    SSH_AUTH_SOCK = "SSH_AUTH_SOCK"
    USER = "USER"
    USERNAME = "USERNAME"
    LOGNAME = "LOGNAME"
    TEST_TMP_DIR = "G8E_TEST_TMP_DIR"
    STRICT_CONSTANTS_LINT = "G8E_STRICT_CONSTANTS_LINT"
    PROJECT_ROOT = "G8E_PROJECT_ROOT"
    TEST_LLM_ASSISTANT_PROVIDER = "G8E_TEST_LLM_ASSISTANT_PROVIDER"
    TEST_LLM_ASSISTANT_MODEL = "G8E_TEST_LLM_ASSISTANT_MODEL"
    TEST_LLM_LITE_PROVIDER = "G8E_TEST_LLM_LITE_PROVIDER"
    TEST_LLM_LITE_MODEL = "G8E_TEST_LLM_LITE_MODEL"
    AUDITOR_HMAC_KEY = "G8E_AUDITOR_HMAC_KEY"
    TEST_LLM_PRIMARY_PROVIDER = "G8E_TEST_LLM_PRIMARY_PROVIDER"
    TEST_LLM_PRIMARY_MODEL = "G8E_TEST_LLM_PRIMARY_MODEL"


SECRET = "secret"
USER_ENDPOINT = "user_endpoint"
HOST = "host"
VIOLATION = "violation"

# Category of every EnvVar value (INV-ENV-04). Keyed by the environment variable name.
ENV_VAR_CATEGORY: dict[str, str] = {
    # Secrets: API keys, tokens, encryption and signing keys.
    EnvVar.LLM_OPENAI_API_KEY: SECRET,
    EnvVar.LLM_OLLAMA_API_KEY: SECRET,
    EnvVar.LLM_ANTHROPIC_API_KEY: SECRET,
    EnvVar.LLM_GEMINI_API_KEY: SECRET,
    EnvVar.LLM_LLAMACPP_API_KEY: SECRET,
    EnvVar.SESSION_ENCRYPTION_KEY: SECRET,
    EnvVar.INTERNAL_API_KEY: SECRET,
    EnvVar.VERTEX_SEARCH_API_KEY: SECRET,
    EnvVar.GOOGLE_SEARCH_API_KEY: SECRET,
    EnvVar.INTERNAL_AUTH_TOKEN: SECRET,
    EnvVar.DEVICE_TOKEN: SECRET,
    EnvVar.OPENCLAW_GATEWAY_TOKEN: SECRET,
    EnvVar.AUDITOR_HMAC_KEY: SECRET,
    # User-specific endpoints with no correct universal default.
    EnvVar.LLM_OPENAI_ENDPOINT: USER_ENDPOINT,
    EnvVar.LLM_OLLAMA_ENDPOINT: USER_ENDPOINT,
    EnvVar.LLM_ANTHROPIC_ENDPOINT: USER_ENDPOINT,
    EnvVar.LLM_LLAMACPP_ENDPOINT: USER_ENDPOINT,
    # Host facts the OS provides.
    EnvVar.SHELL: HOST,
    EnvVar.LANG: HOST,
    EnvVar.TERM: HOST,
    EnvVar.TZ: HOST,
    EnvVar.PATH: HOST,
    EnvVar.SSH_AUTH_SOCK: HOST,
    EnvVar.USER: HOST,
    EnvVar.USERNAME: HOST,
    EnvVar.LOGNAME: HOST,
    # Violations: platform configuration or test switches still read from the
    # environment. Scheduled for removal (env-config-purge); may only shrink.
    EnvVar.G8E_G8EE_HTTPS_PORT: VIOLATION,
    EnvVar.LLM_JEV_MODEL: VIOLATION,
    EnvVar.ALLOWED_ORIGINS: VIOLATION,
    EnvVar.PASSKEY_RP_NAME: VIOLATION,
    EnvVar.PASSKEY_RP_ID: VIOLATION,
    EnvVar.PASSKEY_ORIGIN: VIOLATION,
    EnvVar.OPERATOR_SESSION_ID: VIOLATION,
    EnvVar.DATA_DIR: VIOLATION,
    EnvVar.TEST_TMP_DIR: VIOLATION,
    EnvVar.STRICT_CONSTANTS_LINT: VIOLATION,
    EnvVar.PROJECT_ROOT: VIOLATION,
    EnvVar.TEST_LLM_ASSISTANT_PROVIDER: VIOLATION,
    EnvVar.TEST_LLM_ASSISTANT_MODEL: VIOLATION,
    EnvVar.TEST_LLM_LITE_PROVIDER: VIOLATION,
    EnvVar.TEST_LLM_LITE_MODEL: VIOLATION,
    EnvVar.TEST_LLM_PRIMARY_PROVIDER: VIOLATION,
    EnvVar.TEST_LLM_PRIMARY_MODEL: VIOLATION,
}
