# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Setup reuses working dependencies and installs only missing or stale ones."""

import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

try:
    from test_bootstrap_uv import BASH, bash_environment, bash_path
except ImportError:
    from scripts.tests.test_bootstrap_uv import BASH, bash_environment, bash_path

REPO_ROOT = Path(__file__).resolve().parents[2]
MAKE = shutil.which("make")
NODE_PROJECTS = ("protocol/node", "console", "g8e-adapter", "website")


class DevSetupTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.bin = self.root / "tooling"
        self.bin.mkdir()
        self.home = self.root / "home"
        self.home.mkdir()
        self.calls = self.root / "calls"
        (self.root / "scripts/lib").mkdir(parents=True)
        shutil.copy(REPO_ROOT / "scripts/lib/dev-setup-common.sh", self.root / "scripts/lib")
        shutil.copy(REPO_ROOT / "Makefile", self.root)
        (self.root / "VERSION").write_text("v2.3.2\n")
        (self.root / "go.mod").write_text("module example.org/setup\ngo 1.26.6\n")
        (self.root / "g8e-adapter/src").mkdir(parents=True)
        for directory in (*NODE_PROJECTS, "evaluation-explorer"):
            self.node_project(directory)
        self.env = bash_environment(self.home, self.bin)
        self.env["SETUP_TEST_CALLS"] = bash_path(self.calls)
        self.env["SETUP_TEST_BIN"] = bash_path(self.bin)
        self.env["SETUP_TEST_PYTHON"] = bash_path(Path(sys.executable))
        self.executable("go", '''
if [ "$1" = install ]; then
    echo "go $*" >> "$SETUP_TEST_CALLS"
    exit "${SETUP_TEST_INSTALL_STATUS:-0}"
fi
if [ "$1" = env ] && [ "$2" = GOBIN ]; then echo "$SETUP_TEST_BIN"; fi
''')
        self.executable("npm", '''
echo "npm $*" >> "$SETUP_TEST_CALLS"
if [ "$1" = ls ]; then
    [ ! -f "$3/broken" ]
elif [ "$1" = ci ]; then
    [ "${SETUP_TEST_INSTALL_STATUS:-0}" = 0 ] || exit "$SETUP_TEST_INSTALL_STATUS"
    dir="${3:-.}"
    mkdir -p "$dir/node_modules"
    touch "$dir/node_modules/.package-lock.json"
fi
''')
        self.executable("make", 'echo "make $*" >> "$SETUP_TEST_CALLS"')
        self.executable("uv", 'exit "${SETUP_TEST_UV_STATUS:-0}"')

    def executable(self, name, body, directory=None):
        path = (directory or self.bin) / name
        path.write_text("#!/bin/bash\n" + body.strip() + "\n")
        path.chmod(0o755)

    def node_project(self, directory):
        root = self.root / directory
        (root / "node_modules").mkdir(parents=True, exist_ok=True)
        for filename in ("package.json", "package-lock.json"):
            path = root / filename
            path.write_text("{}\n")
            os.utime(path, (100, 100))
        marker = root / "node_modules/.package-lock.json"
        marker.write_text("{}\n")
        os.utime(marker, (200, 200))

    def run_bash(self, body):
        command = (
            f"export HOME={shlex.quote(self.env['HOME'])} PATH={shlex.quote(self.env['PATH'])}; "
            "set -euo pipefail; source scripts/lib/dev-setup-common.sh; " + body
        )
        return subprocess.run(
            [BASH, "--noprofile", "--norc", "-c", command], cwd=self.root,
            env=self.env, capture_output=True, text=True, check=False,
        )

    def run_make(self, target, *arguments):
        return subprocess.run(
            [MAKE, "--no-print-directory", target, "MAKE=make", *arguments],
            cwd=self.root, env=self.env, capture_output=True, text=True, check=False,
        )

    def logged_calls(self):
        return self.calls.read_text().splitlines() if self.calls.exists() else []

    def assert_succeeded(self, result):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def python_environment(self):
        bindir = self.root / ".venv/bin"
        bindir.mkdir(parents=True)
        self.executable("python", '''
if [ "$1" = --version ]; then echo "Python 3.12.3"; else exec "$SETUP_TEST_PYTHON" "$@"; fi
''', bindir)
        for tool in ("pytest", "ruff", "pyright"):
            self.executable(tool, "exit 0", bindir)
        site = self.root / "site-packages"
        site.mkdir()
        for name, directory in (("g8e", "protocol/python"), ("g8ee", "ensemble")):
            project = self.root / directory
            project.mkdir(parents=True, exist_ok=True)
            manifest = project / "pyproject.toml"
            manifest.write_text("[project]\n")
            os.utime(manifest, (100, 100))
            dist = site / f"{name}-1.0.dist-info"
            dist.mkdir()
            metadata = dist / "METADATA"
            metadata.write_text(f"Metadata-Version: 2.1\nName: {name}\nVersion: 1.0\n")
            os.utime(metadata, (200, 200))
            (dist / "RECORD").write_text(f"{dist.name}/METADATA,,\n")
            (dist / "direct_url.json").write_text(json.dumps({
                "url": project.as_uri(), "dir_info": {"editable": True},
            }))
        for module in ("grpc_tools", "g8e"):
            (site / f"{module}.py").write_text("")
        self.env["PYTHONPATH"] = str(site)

    def test_go_installs_only_missing_tools(self):
        self.executable("ready", "exit 0")
        result = self.run_make(
            "dev-tools-if-needed",
            "GO_DEV_TOOL_PKGS=example.org/cmd/ready@v1.0 example.org/cmd/missing@v2.0",
        )
        self.assert_succeeded(result)
        self.assertEqual(self.logged_calls(), ["go install example.org/cmd/missing@v2.0"])

    def test_setup_finds_uv_without_reloading_shell_profile(self):
        local_bin = self.home / ".local/bin"
        local_bin.mkdir(parents=True)
        self.executable("uv", 'echo "uv 0.11.21"', local_bin)
        result = self.run_bash("SCRIPT_DIR=$PWD/scripts; g8e_setup_init; g8e_check_uv")
        self.assert_succeeded(result)
        self.assertIn("uv: detected (v0.11.21)", result.stdout)

    def test_go_install_failure_stops_setup(self):
        self.env["SETUP_TEST_INSTALL_STATUS"] = "23"
        result = self.run_make(
            "dev-tools-if-needed",
            "GO_DEV_TOOL_PKGS=example.org/cmd/first@v1.0 example.org/cmd/second@v1.0",
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.logged_calls(), ["go install example.org/cmd/first@v1.0"])

    def test_node_installs_only_missing_stale_or_broken_projects(self):
        for directory, state in zip(NODE_PROJECTS, ("ready", "missing", "stale", "broken")):
            root = self.root / directory
            if state == "missing":
                (root / "node_modules/.package-lock.json").unlink()
            elif state == "stale":
                os.utime(root / "package-lock.json", (300, 300))
            elif state == "broken":
                (root / "broken").touch()
        result = self.run_make("dev-node")
        self.assert_succeeded(result)
        installs = [call for call in self.logged_calls() if call.startswith("npm ci")]
        self.assertEqual(installs, [f"npm ci --prefix {directory}" for directory in NODE_PROJECTS[1:]])

    def test_repeated_node_setup_skips_installation(self):
        marker = self.root / "console/node_modules/.package-lock.json"
        marker.unlink()
        self.assert_succeeded(self.run_make("dev-node"))
        self.calls.unlink()
        self.assert_succeeded(self.run_make("dev-node"))
        self.assertFalse(any(call.startswith("npm ci") for call in self.logged_calls()))
        self.assertIn("npm run build --prefix g8e-adapter", self.logged_calls())

    def test_node_install_failure_stops_before_build(self):
        (self.root / "console/node_modules/.package-lock.json").unlink()
        self.env["SETUP_TEST_INSTALL_STATUS"] = "23"
        self.assertNotEqual(self.run_make("dev-node").returncode, 0)
        self.assertNotIn("npm run build --prefix g8e-adapter", self.logged_calls())

    def test_explorer_rebuild_reuses_node_dependencies(self):
        self.assert_succeeded(self.run_bash("g8e_build_evaluation_explorer"))
        self.assertEqual(self.logged_calls(), [
            "npm ls --prefix evaluation-explorer --depth=0 --offline", "npm run build",
        ])

    def test_existing_explorer_bundle_skips_dependency_checks_and_build(self):
        (self.root / "evaluation-explorer/dist").mkdir()
        (self.root / "evaluation-explorer/dist/index.html").touch()
        self.assert_succeeded(self.run_bash("g8e_build_evaluation_explorer"))
        self.assertEqual(self.logged_calls(), [])

    def test_python_installs_only_for_missing_stale_or_inconsistent_environment(self):
        self.assert_succeeded(self.run_make("dev-python-if-needed"))
        self.assertEqual(self.logged_calls(), ["make dev-python"])
        self.calls.unlink()
        self.python_environment()
        self.assert_succeeded(self.run_make("dev-python-if-needed"))
        self.assertEqual(self.logged_calls(), [])
        os.utime(self.root / "ensemble/pyproject.toml", (300, 300))
        self.assert_succeeded(self.run_make("dev-python-if-needed"))
        self.assertEqual(self.logged_calls(), ["make dev-python"])
        self.calls.unlink()
        os.utime(self.root / "ensemble/pyproject.toml", (100, 100))
        self.env["SETUP_TEST_UV_STATUS"] = "1"
        self.assert_succeeded(self.run_make("dev-python-if-needed"))
        self.assertEqual(self.logged_calls(), ["make dev-python"])

    @unittest.skipUnless(os.name == "nt" and shutil.which("pwsh"), "requires native Windows PowerShell")
    def test_windows_path_prefers_current_checkout_and_removes_stale_checkout(self):
        source = (REPO_ROOT / "scripts/windows-setup.ps1").read_text(encoding="utf-8")
        functions = re.search(
            r"(?ms)^function Test-G8ERepositoryRoot \{.*?(?=^function Configure-Path)", source,
        ).group()
        current = self.root / "current"
        stale = self.root / "stale"
        ordinary = self.root / "ordinary"
        for checkout in (current, stale):
            (checkout / "scripts").mkdir(parents=True)
            (checkout / "go.mod").write_text("module github.com/g8e-ai/g8e/v2\n", encoding="utf-8")
            (checkout / "scripts/windows-setup.ps1").touch()
        (ordinary / "scripts").mkdir(parents=True)
        (ordinary / "go.mod").write_text("module example.org/not-g8e\n", encoding="utf-8")
        (ordinary / "scripts/windows-setup.ps1").touch()
        script = self.root / "path-test.ps1"
        script.write_text(
            '$ErrorActionPreference = "Stop"\n' + functions + '\n'
            '$segments = @($args[2], $args[1], $args[0], $args[1])\n'
            '$result = @(Get-G8ECheckoutPathSegments -Segments $segments -CurrentRoot $args[0])\n'
            '$result | ConvertTo-Json -Compress\n',
            encoding="utf-8",
        )
        result = subprocess.run(
            ["pwsh", "-NoProfile", "-NonInteractive", "-File", str(script),
             str(current), str(stale), str(ordinary)],
            capture_output=True, text=True, check=False,
        )
        self.assert_succeeded(result)
        self.assertEqual(json.loads(result.stdout), [str(current), str(ordinary)])

    @unittest.skipUnless(os.name == "nt" and shutil.which("pwsh"), "requires native Windows PowerShell")
    def test_windows_explorer_rebuild_reuses_node_dependencies(self):
        source = (REPO_ROOT / "scripts/windows-setup.ps1").read_text(encoding="utf-8")
        functions = re.search(
            r"(?ms)^function Test-NodeDependencies \{.*?(?=^function Configure-Path)", source,
        ).group()
        script = self.root / "explorer-test.ps1"
        script.write_text(
            '$ErrorActionPreference = "Stop"\n'
            '$Script:ExplorerDir = $args[0]\n'
            '$Script:ExplorerDist = Join-Path $args[0] "dist/index.html"\n'
            + functions + "\nBuild-EvaluationExplorer\n", encoding="utf-8",
        )
        (self.bin / "npm.cmd").write_text('@echo off\necho npm %*>>"%SETUP_TEST_CALLS%"\nexit /b 0\n')
        env = os.environ.copy()
        env["PATH"] = f"{self.bin}{os.pathsep}{env['PATH']}"
        env["SETUP_TEST_CALLS"] = str(self.calls)
        result = subprocess.run(
            ["pwsh", "-NoProfile", "-NonInteractive", "-File", str(script),
             str(self.root / "evaluation-explorer")], env=env,
            capture_output=True, text=True, check=False,
        )
        self.assert_succeeded(result)
        calls = self.logged_calls()
        self.assertFalse(any(call.startswith(("npm ci", "npm install")) for call in calls))
        self.assertIn("npm run build", calls)


if __name__ == "__main__":
    unittest.main()
