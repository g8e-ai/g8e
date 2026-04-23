import shlex
import logging

logger = logging.getLogger(__name__)

def normalise_command(raw: str) -> str:
    """Normalise a command string by stripping cosmetic differences.

    Normalization steps (applied in order):
    1. Strip markdown code fences
    2. Strip common prefixes
    3. Strip comment lines (# prefix)
    4. Strip shebang lines (#!/bin/bash, etc.)
    5. Strip trailing semicolons
    6. Collapse multiple spaces to single spaces (outside quoted strings)
    7. Strip trailing newlines

    Returns the first line if multi-line with unbalanced quotes, or empty string
    if the command is invalid.
    """
    if not raw:
        return ""

    # Strip markdown code fences
    for fence in ["```bash", "```sh", "```"]:
        if raw.startswith(fence):
            raw = raw[len(fence):].strip()
            if raw.endswith("```"):
                raw = raw[:-3].strip()
            break

    # Strip common prefixes
    prefixes = ["Command:", "The command is:", "Final command:"]
    for prefix in prefixes:
        if raw.startswith(prefix):
            raw = raw[len(prefix):].strip()

    # Strip comment lines (lines starting with #)
    lines = raw.split("\n")
    lines = [line for line in lines if not line.strip().startswith("#")]
    raw = "\n".join(lines).strip()

    # Strip shebang lines
    if raw.startswith("#!"):
        lines = raw.split("\n")
        if len(lines) > 1:
            raw = "\n".join(lines[1:]).strip()
        else:
            raw = ""

    # Strip trailing semicolons
    raw = raw.rstrip(";").strip()

    # Collapse multiple spaces to single spaces (simple version - outside quoted strings)
    # This is a conservative approach; full quoted-string-aware parsing would be more complex
    # We'll use shlex.split and join to be more robust if possible, but fallback to simple split
    try:
        parts = shlex.split(raw)
        raw_collapsed = " ".join(parts)
        # If shlex.split works, we still want to keep the original raw if it has multi-lines we want to preserve
        # but for the "simple" collapse, we can use it.
    except ValueError:
        # If shlex.split fails due to unbalanced quotes, we'll try to just collapse spaces normally
        # but we might be returning an invalid command anyway.
        parts = raw.split()
        raw_collapsed = " ".join(parts)

    # Strip trailing newlines
    raw = raw.rstrip("\n")

    # Handle multi-line commands (heredocs, etc.)
    lines = raw.split("\n")
    if len(lines) > 1:
        # Check for heredocs first - they must be preserved as multi-line
        if "<<" in raw:
            return raw.strip()
        
        # Check if the first line has valid shell syntax
        first_line = lines[0].strip()
        try:
            shlex.split(first_line)
            return first_line
        except ValueError:
            # First line invalid, return empty
            return ""

    # Validate shell syntax
    try:
        shlex.split(raw)
        return raw.strip()
    except ValueError:
        return ""
