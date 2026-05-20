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

import contextlib
import importlib.util
import io
import sys
import unittest
from pathlib import Path
from unittest.mock import patch, MagicMock

SCRIPT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SCRIPT_DIR))

def load_module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module

manage_operators = load_module("manage_operators", SCRIPT_DIR / "manage-operators.py")
manage_users = load_module("manage_users", SCRIPT_DIR / "manage-users.py")
manage_device_links = load_module("manage_device_links", SCRIPT_DIR / "manage-device-links.py")
manage_passkeys = load_module("manage_passkeys", SCRIPT_DIR.parent / "security" / "manage-passkeys.py")


class NonInteractiveSafetyTests(unittest.TestCase):
    @patch("sys.stdin")
    def test_manage_operators_non_interactive_raises_error(self, mock_stdin):
        mock_stdin.isatty.return_value = False
        manager = manage_operators.OperatorManager()

        def fake_operator_request(method, path, body=None):
            return {
                "id": "op-123",
                "user_id": "user-456",
                "name": "op-name",
                "slot_number": 0,
                "status": "active",
            }

        with patch.object(manage_operators, "operator_request", side_effect=fake_operator_request):
            with self.assertRaises(RuntimeError) as ctx:
                manager.refresh_key("op-123", force=False)
            self.assertIn("Non-interactive environment detected", str(ctx.exception))

            with self.assertRaises(RuntimeError) as ctx:
                manager.terminate("op-123", force=False)
            self.assertIn("Non-interactive environment detected", str(ctx.exception))

    @patch("sys.stdin")
    def test_manage_users_non_interactive_raises_error(self, mock_stdin):
        mock_stdin.isatty.return_value = False
        manager = manage_users.UserManager()

        def fake_operator_request(method, path, body=None):
            return {
                "id": "user-123",
                "email": "test@g8e.ai",
                "name": "Test User",
            }

        with patch.object(manage_users, "operator_request", side_effect=fake_operator_request):
            with self.assertRaises(RuntimeError) as ctx:
                manager.delete_user("user-123", force=False)
            self.assertIn("Non-interactive environment detected", str(ctx.exception))

    @patch("sys.stdin")
    def test_manage_device_links_non_interactive_raises_error(self, mock_stdin):
        mock_stdin.isatty.return_value = False
        manager = manage_device_links.DeviceLinkManager()

        with self.assertRaises(RuntimeError) as ctx:
            manager.delete_link("dlk_abc123xyz", force=False)
        self.assertIn("Non-interactive environment detected", str(ctx.exception))

    @patch("sys.stdin")
    def test_manage_passkeys_non_interactive_raises_error(self, mock_stdin):
        mock_stdin.isatty.return_value = False
        manager = manage_passkeys.PasskeyManager()

        def fake_resolve(user_id, email):
            return "user-123"

        with patch.object(manage_passkeys, "resolve_user_id", side_effect=fake_resolve):
            with self.assertRaises(RuntimeError) as ctx:
                manager.revoke_credential("cred-123", user_id="user-123", email=None, force=False)
            self.assertIn("Non-interactive environment detected", str(ctx.exception))

            with self.assertRaises(RuntimeError) as ctx:
                manager.reset(user_id="user-123", email=None, force=False)
            self.assertIn("Non-interactive environment detected", str(ctx.exception))

            with self.assertRaises(RuntimeError) as ctx:
                manager.revoke_all(user_id="user-123", email=None, force=False)
            self.assertIn("Non-interactive environment detected", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
