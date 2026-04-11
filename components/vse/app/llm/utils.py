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

import ipaddress
from urllib.parse import urlparse


def schema_to_dict(schema) -> dict:
    """Recursively convert a ToolDeclaration parameter schema to a plain dictionary."""
    if isinstance(schema, dict):
        return schema

    result = {}
    if hasattr(schema, "type") and schema.type is not None:
        t = schema.type
        result["type"] = t.value.lower() if hasattr(t, "value") else str(t).lower()

    if hasattr(schema, "description") and schema.description:
        result["description"] = schema.description

    if hasattr(schema, "enum") and schema.enum:
        result["enum"] = schema.enum

    if hasattr(schema, "properties") and schema.properties:
        result["properties"] = {
            k: schema_to_dict(v) for k, v in schema.properties.items()
        }

    if hasattr(schema, "required") and schema.required:
        result["required"] = schema.required

    if hasattr(schema, "items") and schema.items:
        result["items"] = schema_to_dict(schema.items)

    return result


def is_internal_endpoint(url: str | None) -> bool:
    """
    Check if a URL refers to an internal network endpoint.
    Checks for:
    - Localhost/loopback
    - RFC 1918 private IP ranges
    - Known internal hostnames (.internal, .local)
    - g8e platform services (vsod, vsa, vsodb)
    """
    if not url:
        return False

    try:
        parsed = urlparse(url)
        hostname = parsed.hostname
        if not hostname:
            return False

        hostname_lower = hostname.lower()

        # Localhost checks
        if hostname_lower in ("localhost", "127.0.0.1", "::1"):
            return True

        # g8e platform services (Docker Compose service names)
        if hostname_lower in ("vsod", "vsa", "vsodb"):
            return True

        # Internal TLDs
        if hostname_lower.endswith((".internal", ".local")):
            return True

        # IP range checks
        try:
            ip = ipaddress.ip_address(hostname)
            if ip.is_loopback or ip.is_private:
                return True
        except ValueError:
            # Not an IP address, just a hostname
            pass

        return False

    except Exception:
        # Fallback to simple substring match if parsing fails
        if not url:
            return False
        lower_url = url.lower()
        return any(x in lower_url for x in ("localhost", "127.0.0.1", ".internal", ".local", "vsod", "vsa", "vsodb"))


def is_ollama_endpoint(url: str | None) -> bool:
    """
    Check if a URL likely refers to an Ollama endpoint.
    Checks for the 'ollama' string or the default port 11434.
    """
    if not url:
        return False

    try:
        parsed = urlparse(url)
        if parsed.port == 11434:
            return True
        
        hostname = parsed.hostname
        if hostname and "ollama" in hostname.lower():
            return True
            
        if "ollama" in parsed.path.lower():
            return True

        return False
    except Exception:
        return "11434" in url or "ollama" in url.lower()
