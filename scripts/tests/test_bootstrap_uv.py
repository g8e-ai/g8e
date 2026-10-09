# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression coverage for the uv bootstrap script."""

import os
import shlex
import shutil
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


def bash_executable():
    if os.name == "nt" and (git := shutil.which("git")):
        git_bash = Path(git).parents[1] / "bin" / "bash.exe"
        if git_bash.exists():
            return str(git_bash)
    return "bash"


BASH = bash_executable()


def bash_path(path):
    """Return a path understood by the selected Bash (WSL, MSYS, or POSIX)."""
    if os.name != "nt":
        return str(path)
    if "git" in BASH.lower():
        return subprocess.run(
            ["cygpath", "-u", str(path)],
            capture_output=True,
            text=True,
            check=True,
        ).stdout.strip()
    converted = subprocess.run(
        [
            BASH,
            "-c",
            'command -v wslpath >/dev/null && wslpath -u "$1"',
            "bash",
            str(path),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    if converted.returncode == 0:
        return converted.stdout.strip()
    return subprocess.run(
        ["cygpath", "-u", str(path)],
        capture_output=True,
        text=True,
        check=True,
    ).stdout.strip()


def bash_environment(fake_home, fake_bin):
    env = os.environ.copy()
    env["HOME"] = bash_path(fake_home)
    bin_path = bash_path(fake_bin)
    if os.name == "nt" and "git" in BASH.lower():
        git_usr_bin = bash_path(Path(BASH).parents[1] / "usr" / "bin")
        git_mingw_bin = bash_path(Path(BASH).parents[1] / "mingw64" / "bin")
        env["PATH"] = f"{bin_path}:{git_mingw_bin}:{git_usr_bin}:/usr/bin:/bin"
    else:
        env["PATH"] = f"{bin_path}:/usr/bin:/bin"
    return env


def run_bootstrap(env):
    command = (
        f"export HOME={shlex.quote(env['HOME'])} PATH={shlex.quote(env['PATH'])}; "
        "source scripts/bootstrap-uv.sh"
    )
    return subprocess.run(
        [BASH, "--noprofile", "--norc", "-c", command],
        cwd=REPO_ROOT,
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )


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

            env = bash_environment(fake_home, fake_bin)

            result = run_bootstrap(env)

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

            env = bash_environment(fake_home, fake_bin)

            result = run_bootstrap(env)

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

            env = bash_environment(fake_home, fake_bin)

            result = run_bootstrap(env)

            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn("could not install uv", result.stderr)


if __name__ == "__main__":
    unittest.main()
