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

import os
from pathlib import Path
from typing import Union

def validate_safe_path(path: Union[str, Path], root: Union[str, Path]) -> Path:
    """
    Ensures a path is safe and stays within the specified root directory.
    
    Args:
        path: The path to validate (absolute or relative)
        root: The allowed root directory
        
    Returns:
        The resolved absolute Path object
        
    Raises:
        ValueError: If the path is invalid or attempts traversal outside the root
    """
    if not path:
        raise ValueError("Empty path provided")
    
    root_path = Path(root).resolve()
    
    # Clean and resolve the target path
    # Path.resolve() handles '..' segments and redundant slashes
    try:
        if Path(path).is_absolute():
            target_path = Path(path).resolve()
        else:
            target_path = (root_path / path).resolve()
    except Exception as e:
        raise ValueError(f"Invalid path format: {e}")
    
    # Security check: Ensure target_path is within root_path
    try:
        # relative_to raises ValueError if target_path is not under root_path
        target_path.relative_to(root_path)
    except ValueError:
        raise ValueError(f"Path traversal detected: {path} is outside of {root}")
    
    return target_path

def is_shell_required(command: str) -> bool:
    """
    Checks if a command string contains shell metacharacters.
    """
    # Common shell metacharacters
    metachars = set("|&><$();`\\*?[]~")
    return any(char in metachars for char in command)
