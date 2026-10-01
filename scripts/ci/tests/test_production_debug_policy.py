# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]


class ProductionDebugPolicyTest(unittest.TestCase):
    def test_shell_packagers_compile_every_go_binary_as_production(self) -> None:
        scripts = sorted(
            path
            for platform in ("linux", "windows", "macos")
            for path in (ROOT / "packaging" / platform).glob("build*.sh")
        )
        invocations = 0
        for path in scripts:
            lines = path.read_text(encoding="utf-8").splitlines()
            for index, line in enumerate(lines):
                if "grxfirma_go_build" not in line:
                    continue
                invocations += 1
                block = "\n".join(lines[index : index + 8])
                self.assertRegex(
                    block,
                    r"-tags\s+production(?:,fyne_gui)?(?:\s|\\)",
                    f"{path.relative_to(ROOT)} no fija production:\n{block}",
                )
        self.assertGreaterEqual(invocations, 20)

    def test_powershell_packagers_compile_every_go_binary_as_production(self) -> None:
        scripts = sorted((ROOT / "packaging" / "windows").glob("build*.ps1"))
        invocations = 0
        for path in scripts:
            lines = path.read_text(encoding="utf-8").splitlines()
            for index, line in enumerate(lines):
                if "Invoke-GrxFirmaGoBuild" not in line:
                    continue
                invocations += 1
                block = "\n".join(lines[index : index + 18])
                self.assertIn(
                    '"production',
                    block,
                    f"{path.relative_to(ROOT)} no fija production:\n{block}",
                )
        self.assertGreaterEqual(invocations, 10)

    def test_public_installers_do_not_generate_debug_launchers(self) -> None:
        linux_installer = (
            ROOT / "packaging/linux/install-suite.sh"
        ).read_text(encoding="utf-8")
        linux_builder = (
            ROOT / "packaging/linux/build-suite.sh"
        ).read_text(encoding="utf-8")
        windows_installer = (
            ROOT / "packaging/windows/install-afirmauri.ps1"
        ).read_text(encoding="utf-8")

        for forbidden in (
            "export GRXFIRMA_DEBUG=1",
            "export GRXFIRMA_DESKTOP_DEBUG=1",
            "afirmauri-handler pid=",
            "browser-bridge pid=",
        ):
            self.assertNotIn(forbidden, linux_installer)
            self.assertNotIn(forbidden, linux_builder)
        self.assertNotRegex(
            linux_builder,
            re.compile(r"cp .*grxfirma(?:-manual)?-debug\.desktop"),
        )
        self.assertNotRegex(
            linux_installer,
            re.compile(r"sed .*grxfirma(?:v2|-manual)?-debug\.desktop"),
        )
        self.assertNotIn("set GRXFIRMA_DEBUG=1", windows_installer)
        self.assertNotIn("$handlerDebugContent", windows_installer)

    def test_production_policy_has_both_build_variants(self) -> None:
        policy_dir = (
            ROOT / "internal/adapters/outbound/common/logging"
        )
        development = (
            policy_dir / "debug_policy_default.go"
        ).read_text(encoding="utf-8")
        production = (
            policy_dir / "debug_policy_production.go"
        ).read_text(encoding="utf-8")
        self.assertIn("//go:build !production", development)
        self.assertIn("debugLevelAllowed = true", development)
        self.assertIn("//go:build production", production)
        self.assertIn("debugLevelAllowed = false", production)


if __name__ == "__main__":
    unittest.main()
