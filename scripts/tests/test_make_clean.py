# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this source is
# released under the Apache License, Version 2.0.

"""Regression coverage for the runtime-safe Makefile clean target."""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


class MakeCleanTests(unittest.TestCase):
    def test_clean_preserves_runtime_and_identities_without_running_gateway(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            retained = {
                ".g8e/pki/root/root_ca.crt": b"gateway-ca",
                ".g8e/data/g8e.db": b"gateway-data",
                ".g8e/vault/key": b"gateway-vault-key",
                ".local.dev/full/ensemble/.g8e/pki/issued/apps/g8ee.crt": b"app-identity",
                ".local.dev/full/workloads.json": b"[]",
            }
            for name, content in retained.items():
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(content)
            calls = root / "calls"
            binary = root / "g8e"
            binary.write_text(
                f"#!/bin/sh\nprintf '%s\\n' \"$*\" >> '{calls}'\nexit 99\n"
            )
            binary.chmod(0o755)
            fake_bin = root / "bin"
            fake_bin.mkdir()
            fake_go = fake_bin / "go"
            fake_go.write_text("#!/bin/sh\nexit 0\n")
            fake_go.chmod(0o755)
            (root / "coverage.out").write_text("build artifact")
            (root / ".g8e-test-tmp").mkdir()

            env = os.environ.copy()
            env["PATH"] = f"{fake_bin}{os.pathsep}{env['PATH']}"
            result = subprocess.run(
                ["make", "-f", str(REPO_ROOT / "Makefile"), "clean"],
                cwd=root,
                stdin=subprocess.DEVNULL,
                text=True,
                capture_output=True,
                env=env,
                check=False,
            )

            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse(calls.exists())
            for name, content in retained.items():
                self.assertEqual((root / name).read_bytes(), content)
            self.assertFalse(list(root.glob(".g8e-*")))
            self.assertFalse((root / "bin").exists())
            self.assertFalse((root / "coverage.out").exists())
            self.assertEqual(binary.read_text().splitlines()[-1], "exit 99")


if __name__ == "__main__":
    unittest.main()
