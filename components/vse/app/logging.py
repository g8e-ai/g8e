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

import json
import logging
import re
import sys
from typing import Any

from .models.settings import VSEPlatformSettings

PII_FIELDS: list[str] = [
    "email",
    "to_email",
    "customer_email",
    "user_email",
    "name",
    "customer_name",
    "user_name",
    "display_name",
    "first_name",
    "last_name",
    "full_name",
]

EMAIL_REGEX = re.compile(r"[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}")


def redact_email(email: str) -> str:
    if "@" not in email:
        return email

    at_index = email.index("@")
    local_part = email[:at_index]
    domain_part = email[at_index + 1:]
    if len(local_part) <= 2:
        redacted_local = "*" * len(local_part)
    else:
        redacted_local = local_part[0] + "*" * (len(local_part) - 2) + local_part[-1]
    if len(domain_part) <= 2:
        redacted_domain = "*" * len(domain_part)
    else:
        redacted_domain = domain_part[0] + "*" * (len(domain_part) - 2) + domain_part[-1]

    return f"{redacted_local}@{redacted_domain}"


def redact_value(field_name: str, value: Any) -> Any:
    if value is None:
        return value

    lower_field = field_name.lower()

    is_pii_field = any(pii.lower() == lower_field for pii in PII_FIELDS)
    if is_pii_field:
        if isinstance(value, str):
            if "@" in value:
                return redact_email(value)
            if len(value) > 0:
                return value[0] + "*" * max(0, len(value) - 1)
        return "***"

    if isinstance(value, str) and "@" in value:
        return EMAIL_REGEX.sub(lambda m: redact_email(m.group(0)), value)

    return value


def redact_pii(obj: Any, depth: int = 0) -> Any:
    if depth > 10 or obj is None:
        return obj

    if not isinstance(obj, (dict, list)):
        return obj

    if isinstance(obj, list):
        return [redact_pii(item, depth + 1) for item in obj]

    sanitized = {}
    for key, value in obj.items():
        if isinstance(value, (dict, list)):
            sanitized[key] = redact_pii(value, depth + 1)
        else:
            sanitized[key] = redact_value(key, value)
    return sanitized


def redact_message(message: str) -> str:
    if not isinstance(message, str) or "@" not in message:
        return message

    return EMAIL_REGEX.sub(lambda m: redact_email(m.group(0)), message)

try:
    from uvicorn.logging import AccessFormatter as AccessFormatter
    HAS_UVICORN = True
except ImportError:
    HAS_UVICORN = False
    AccessFormatter  # type: ignore[assignment,misc]

COMPONENT_PATTERN = re.compile(
    r"(?:"
    r"\/components\/(vs[a-z]+)\/"
    r"|\/app\/components\/(vs[a-z]+)\/"
    r"|^components\.(vs[a-z]+)"
    r"|\/app\/(vs[a-z]+)\/"
    r"|^(vs[a-z]+)\."
    r")"
)

_logger_component_registry: dict[str, str] = {}

class ComponentFormatter(logging.Formatter):

    def __init__(self, *args, component_name=None, **kwargs):
        super().__init__(*args, **kwargs)
        self.component_name = component_name
        self.standard_attributes = {
            "name", "msg", "args", "levelname", "levelno", "pathname", "filename",
            "module", "lineno", "funcName", "created", "msecs", "relativeCreated",
            "thread", "threadName", "processName", "process", "message", "exc_info",
            "exc_text", "stack_info", "component", "asctime"
        }

    def format(self, record):
        if self.component_name:
            component = self.component_name
        else:
            component = ""
            if hasattr(record, "name") and record.name:
                logger_parts = record.name.split(".")
                for i in range(len(logger_parts), 0, -1):
                    parent_name = ".".join(logger_parts[:i])
                    if parent_name in _logger_component_registry:
                        component = _logger_component_registry[parent_name]
                        break

            if not component:
                if hasattr(record, "module") and record.module:
                    match = COMPONENT_PATTERN.search(record.module)
                    if match:
                        component = next((g for g in match.groups() if g), "")

                if not component and hasattr(record, "pathname"):
                    match = COMPONENT_PATTERN.search(record.pathname)
                    if match:
                        component = next((g for g in match.groups() if g), "")
                if not component and hasattr(record, "name"):
                    match = COMPONENT_PATTERN.search(record.name)
                    if match:
                        component = next((g for g in match.groups() if g), "")

        record.component = component
        if hasattr(record, "msg") and isinstance(record.msg, str):
            record.msg = redact_message(record.msg)

        formatted_message = super().format(record)
        extra_fields = {}
        for key, value in record.__dict__.items():
            if key not in self.standard_attributes:
                extra_fields[key] = value

        if extra_fields:
            extra_fields = redact_pii(extra_fields)
        if extra_fields:
            try:
                extra_str = json.dumps(extra_fields, sort_keys=True, separators=(",", ":"))
                formatted_message += f" | extra={extra_str}"
            except (TypeError, ValueError):
                extra_parts = []
                for key, value in extra_fields.items():
                    try:
                        extra_parts.append(f"{key}={value!r}")
                    except Exception:
                        extra_parts.append(f"{key}=<unserializable>")
                if extra_parts:
                    formatted_message += " | extra={" + ", ".join(extra_parts) + "}"

        return formatted_message

def register_component_logger(logger_name: str, component_name: str):
    _logger_component_registry[logger_name] = component_name

def setup_logging(settings: VSEPlatformSettings, component_name: str):
    if settings.enable_logging:
        log_level = getattr(logging, settings.log_level, logging.INFO)

        if component_name:
            register_component_logger(component_name, component_name)
            register_component_logger(f"{component_name}.", component_name)
            register_component_logger(f"components.{component_name}", component_name)
            register_component_logger("shared", component_name)
            register_component_logger("shared.", component_name)

        logger = logging.getLogger()

        if logger.hasHandlers():
            for handler in logger.handlers[:]:
                logger.removeHandler(handler)

        handler = logging.StreamHandler(sys.stdout)
        formatter = ComponentFormatter(
            "%(asctime)s - %(name)s - %(levelname)s - [%(component)s:%(pathname)s:%(lineno)d] - %(message)s",
            component_name=component_name
        )
        handler.setFormatter(formatter)
        logger.addHandler(handler)
        logger.setLevel(log_level)

        logging.getLogger("urllib3").setLevel(logging.INFO)
        logging.getLogger("aiohttp").setLevel(logging.WARNING)
        logging.getLogger("openai").setLevel(logging.WARNING)

        logger.info(f"Logging configured with level {settings.log_level}")

        if HAS_UVICORN and AccessFormatter is not None:
            uvicorn_access_logger = logging.getLogger("uvicorn.access")
            if uvicorn_access_logger.hasHandlers():
                for handler in uvicorn_access_logger.handlers[:]:
                    uvicorn_access_logger.removeHandler(handler)
            uvicorn_access_handler = logging.StreamHandler(sys.stdout)
            uvicorn_access_formatter = AccessFormatter(
                fmt='%(asctime)s - %(levelname)s - %(client_addr)s - "%(request_line)s" %(status_code)s'
            )
            uvicorn_access_handler.setFormatter(uvicorn_access_formatter)
            uvicorn_access_logger.addHandler(uvicorn_access_handler)
            uvicorn_access_logger.setLevel(log_level)
            uvicorn_access_logger.propagate = False
            logger.info(f"Uvicorn access logging configured with level {settings.log_level}")
        else:
            logger.info("Uvicorn not available - skipping uvicorn access logging configuration")

    else:
        logger = logging.getLogger()
        if logger.hasHandlers():
            for handler in logger.handlers[:]:
                logger.removeHandler(handler)
        logger.addHandler(logging.NullHandler())
        logger.setLevel(logging.CRITICAL + 1)


def get_logger(name: str) -> logging.Logger:
    """Utility to get a logger by name."""
    return logging.getLogger(name)
