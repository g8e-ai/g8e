"""Pytest configuration for evals tests.

Sets up required environment variables for tests that import from g8ee and protocol.
"""
import os
import sys
from pathlib import Path

# Set up environment for tests that import from g8e_protocol
G8E_ROOT = Path(__file__).parent.parent.parent
os.environ.setdefault("G8E_PROTOCOL_DIR", str(G8E_ROOT / "protocol"))

# Protocol python package path
PROTOCOL_PYTHON_PATH = str(G8E_ROOT / "protocol" / "python")

# Add to PYTHONPATH environment variable
pythonpath = f"{PROTOCOL_PYTHON_PATH}{os.pathsep}{os.environ.get('PYTHONPATH', '')}"
os.environ.setdefault("PYTHONPATH", pythonpath)

# Ensure path is in sys.path
if PROTOCOL_PYTHON_PATH not in sys.path:
    sys.path.insert(0, PROTOCOL_PYTHON_PATH)
