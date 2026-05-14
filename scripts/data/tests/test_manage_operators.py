import contextlib
import importlib.util
import io
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

SCRIPT_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SCRIPT_DIR))

spec = importlib.util.spec_from_file_location("manage_operators", SCRIPT_DIR / "manage-operators.py")
manage_operators = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(manage_operators)


class ManageOperatorsSecurityTests(unittest.TestCase):
    def test_refresh_key_never_outputs_or_returns_secret_from_response(self):
        secret = "g8e_op-1_super_secret_rotated_value"
        calls = [
            {
                "id": "op-1",
                "user_id": "user-1",
                "name": "operator-one",
                "slot_number": 0,
                "status": "offline",
            },
            {"success": True, "api_key": secret},
        ]

        def fake_operator_request(method, path, body=None):
            return calls.pop(0)

        stdout = io.StringIO()
        with patch.object(manage_operators, "operator_request", side_effect=fake_operator_request):
            with contextlib.redirect_stdout(stdout):
                result = manage_operators.OperatorManager().refresh_key("op-1", force=True)

        output = stdout.getvalue()
        self.assertEqual({"success": True}, result)
        self.assertNotIn(secret, output)
        self.assertNotIn("super_secret", output)
        self.assertNotIn("api_key", result)
        self.assertIn("not displayed", output)

    def test_get_key_command_is_not_registered(self):
        parser = manage_operators.build_parser()

        with self.assertRaises(SystemExit) as ctx:
            parser.parse_args(["get-key", "--id", "op-1"])

        self.assertEqual(2, ctx.exception.code)


if __name__ == "__main__":
    unittest.main()
