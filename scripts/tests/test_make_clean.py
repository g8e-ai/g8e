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
    def test_clean_stops_gateway_and_archives_runtime_before_build_cleanup(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / ".g8e").mkdir()
            (root / ".g8e" / "state").write_text("preserve me")
            calls = root / "calls"
            binary = root / "g8e"
            binary.write_text(
                "#!/bin/sh\n"
                f"printf '%s\\n' \"$*\" >> '{calls}'\n"
                "[ \"$1 $2\" = 'gw clean' ]\n"
                "[ \"$3 $4\" = '--yes --skip-backup' ]\n"
                "mv .g8e .g8e-archive\n"
            )
            binary.chmod(0o755)
            fake_bin = root / "bin"
            fake_bin.mkdir()
            fake_go = fake_bin / "go"
            fake_go.write_text("#!/bin/sh\nexit 0\n")
            fake_go.chmod(0o755)

            env = os.environ.copy()
            env["PATH"] = f"{fake_bin}{os.pathsep}{env['PATH']}"
            result = subprocess.run(
                ["make", "-f", str(REPO_ROOT / "Makefile"), "clean"],
                cwd=root,
                input="y\n",
                text=True,
                capture_output=True,
                env=env,
                check=False,
            )

            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual((root / "calls").read_text(), "gw clean --yes --skip-backup\n")
            self.assertEqual((root / ".g8e-archive" / "state").read_text(), "preserve me")
            self.assertFalse((root / ".g8e").exists())


if __name__ == "__main__":
    unittest.main()
