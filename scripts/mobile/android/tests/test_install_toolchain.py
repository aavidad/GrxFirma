# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[4]
INSTALLER = ROOT / "scripts/mobile/android/install-toolchain.sh"
WORKFLOW = ROOT / ".github/workflows/release.yml"


class InstallToolchainTest(unittest.TestCase):
    def test_github_jobs_persist_the_exact_toolchain_environment(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        invocation = 'install-toolchain.sh --github-env "$GITHUB_ENV"'
        self.assertEqual(3, workflow.count(invocation))

    def test_environment_file_is_fail_closed_and_does_not_use_eval(self) -> None:
        installer = INSTALLER.read_text(encoding="utf-8")
        self.assertIn('[[ -L "$GITHUB_ENV_FILE"', installer)
        self.assertIn('! -f "$GITHUB_ENV_FILE"', installer)
        self.assertIn("contiene saltos de línea", installer)
        self.assertNotIn("eval ", installer)
        for name in ("JAVA_HOME", "ANDROID_HOME", "ANDROID_SDK_ROOT", "PATH"):
            self.assertIn(f"printf '{name}=%s", installer)


if __name__ == "__main__":
    unittest.main()
