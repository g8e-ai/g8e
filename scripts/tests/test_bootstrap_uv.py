# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression coverage for the uv bootstrap script."""

import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
BOOTSTRAP_SCRIPT = REPO_ROOT / "scripts" / "bootstrap-uv.sh"


class BootstrapUVTests(unittest.TestCase):
    def test_existing_installation_skips_download(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            fake_home = temp / "home"
            fake_home.mkdir()
            fake_bin = temp / "bin"
            fake_bin.mkdir()

            # Existing uv executable in PATH
            fake_uv = fake_bin / "uv"
            fake_uv.write_text("#!/bin/sh\nexit 0\n")
            fake_uv.chmod(fake_uv.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

            # Fake curl that fails if called
            fake_curl = fake_bin / "curl"
            fake_curl.write_text("#!/bin/sh\necho 'curl called unexpectedly' >&2\nexit 99\n")
            fake_curl.chmod(fake_curl.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

            env = os.environ.copy()
            env["HOME"] = str(fake_home)
            env["PATH"] = f"{fake_bin}{os.pathsep}{env.get('PATH', '')}"

            result = subprocess.run(
                ["bash", str(BOOTSTRAP_SCRIPT)],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                env=env,
                check=False,
            )

            self.assertEqual(result.returncode, 0, f"stdout: {result.stdout}\nstderr: {result.stderr}")
            self.assertNotIn("curl called unexpectedly", result.stderr)
            self.assertFalse((fake_home / ".local" / "bin" / "uv").exists())

    def test_successful_installation_downloads_and_installs(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            fake_home = temp / "home"
            fake_home.mkdir()
            fake_bin = temp / "bin"
            fake_bin.mkdir()

            # Mock curl that writes an installer script simulating astral's install.sh
            fake_curl = fake_bin / "curl"
            fake_curl.write_text(
                '#!/bin/sh\n'
                'out=""\n'
                'while [ $# -gt 0 ]; do\n'
                '  if [ "$1" = "-o" ]; then out="$2"; shift 2; else shift; fi\n'
                'done\n'
                'if [ -n "$out" ]; then\n'
                '  cat <<\'INSTALLER\' > "$out"\n'
                '#!/bin/sh\n'
                'mkdir -p "$UV_INSTALL_DIR"\n'
                'cat <<\'UVECHO\' > "$UV_INSTALL_DIR/uv"\n'
                '#!/bin/sh\n'
                'echo "uv 0.5.0"\n'
                'exit 0\n'
                'UVECHO\n'
                'chmod +x "$UV_INSTALL_DIR/uv"\n'
                'exit 0\n'
                'INSTALLER\n'
                '  exit 0\n'
                'fi\n'
                'exit 1\n'
            )
            fake_curl.chmod(fake_curl.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

            # Strip existing uv from PATH
            clean_paths = [
                p for p in os.environ.get("PATH", "").split(os.pathsep)
                if not (Path(p) / "uv").exists()
            ]
            env = os.environ.copy()
            env["HOME"] = str(fake_home)
            env["PATH"] = f"{fake_bin}{os.pathsep}{os.pathsep.join(clean_paths)}"

            result = subprocess.run(
                ["bash", str(BOOTSTRAP_SCRIPT)],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                env=env,
                check=False,
            )

            self.assertEqual(result.returncode, 0, f"stdout: {result.stdout}\nstderr: {result.stderr}")
            installed_uv = fake_home / ".local" / "bin" / "uv"
            self.assertTrue(installed_uv.exists())
            self.assertTrue(os.access(installed_uv, os.X_OK))

    def test_download_failure_reports_error(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            fake_home = temp / "home"
            fake_home.mkdir()
            fake_bin = temp / "bin"
            fake_bin.mkdir()

            # Mock curl that simulates download failure (exit code 22 / 404)
            fake_curl = fake_bin / "curl"
            fake_curl.write_text("#!/bin/sh\necho 'curl: (22) The requested URL returned error: 404' >&2\nexit 22\n")
            fake_curl.chmod(fake_curl.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

            # Strip existing uv from PATH
            clean_paths = [
                p for p in os.environ.get("PATH", "").split(os.pathsep)
                if not (Path(p) / "uv").exists()
            ]
            env = os.environ.copy()
            env["HOME"] = str(fake_home)
            env["PATH"] = f"{fake_bin}{os.pathsep}{os.pathsep.join(clean_paths)}"

            result = subprocess.run(
                ["bash", str(BOOTSTRAP_SCRIPT)],
                cwd=REPO_ROOT,
                capture_output=True,
                text=True,
                env=env,
                check=False,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("could not install uv", result.stderr)


if __name__ == "__main__":
    unittest.main()
