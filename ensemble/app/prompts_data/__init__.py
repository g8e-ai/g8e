# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Prompts data package for g8ee Ensemble system prompts and tool descriptions.

Prompt data modules:
- loader.py: Prompt loading utilities
- core/: Core prompt templates
- modes/: Agent mode-specific prompts
- system/: System-level prompts
- tools/: Tool description prompts
- tribunal/: Tribunal-related prompts
"""

from .loader import load_mode_prompts, load_prompt

__all__ = ["load_mode_prompts", "load_prompt"]
