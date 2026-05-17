"""Pytest configuration for evals tests.

Sets up required environment variables for tests that import from g8ee and protocol.
"""
import os
import sys
from pathlib import Path

# Set up environment for tests that import from g8ee and protocol
G8E_ROOT = Path(__file__).parent.parent.parent
os.environ.setdefault("G8E_PROTOCOL_DIR", str(G8E_ROOT / "protocol"))
pythonpath = f"{G8E_ROOT / 'services/g8ee'}:{G8E_ROOT / 'protocol'}{os.pathsep}{os.environ.get('PYTHONPATH', '')}"
os.environ.setdefault("PYTHONPATH", pythonpath)

# Ensure paths are in sys.path
if str(G8E_ROOT / "services/g8ee") not in sys.path:
    sys.path.insert(0, str(G8E_ROOT / "services/g8ee"))
if str(G8E_ROOT / "protocol") not in sys.path:
    sys.path.insert(0, str(G8E_ROOT / "protocol"))
