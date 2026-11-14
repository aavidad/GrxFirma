# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Pruebas del arnés sin SDK, claves ni conexiones a la red."""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


SCRIPTS = Path(__file__).resolve().parents[1]


class UnsignedReproducibilityTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        scripts = self.root / "scripts/mobile/android"
        scripts.mkdir(parents=True)
        self.script = scripts / "verify-unsigned-reproducibility.sh"
        shutil.copy2(SCRIPTS / self.script.name, self.script)
        project = self.root / "mobile/android"
        project.mkdir(parents=True)
        gradle = project / "gradlew"
        gradle.write_text(r'''#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$COMMAND_LOG"
mkdir -p app/build/outputs/apk/production/release
contents=apk
if [[ "${DIFFERENT_APKS:-0}" == 1 ]]; then
    contents=$(wc -l < "$COMMAND_LOG")
fi
printf '%s' "$contents" > app/build/outputs/apk/production/release/app-production-release-unsigned.apk
''', encoding="utf-8")
        gradle.chmod(0o755)
        binaries = self.root / "bin"
        binaries.mkdir()
        git = binaries / "git"
        git.write_text(r'''#!/usr/bin/env bash
if [[ "$3" == rev-parse ]]; then
    printf '%040d\n' 1
else
    printf '%s\n' 1700000000
fi
''', encoding="utf-8")
        git.chmod(0o755)
        self.env = {key: value for key, value in os.environ.items()
                    if not key.startswith("GRXFIRMA_ANDROID_KEY")}
        self.env["PATH"] = str(binaries) + os.pathsep + self.env["PATH"]
        self.log = self.root / "commands.log"
        self.env["COMMAND_LOG"] = str(self.log)
        self.evidence = self.root / "evidence"

    def run_script(self) -> subprocess.CompletedProcess:
        return subprocess.run(["bash", str(self.script), str(self.evidence)],
                              env=self.env, capture_output=True, text=True, check=False)

    def test_compares_two_clean_builds_and_preserves_hashes(self) -> None:
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.evidence / "first.apk").read_bytes(),
                         (self.evidence / "second.apk").read_bytes())
        self.assertEqual(len((self.evidence / "SHA256SUMS.txt").read_text().splitlines()), 2)
        commands = self.log.read_text().splitlines()
        self.assertEqual(len(commands), 2)
        for command in commands:
            self.assertIn("--no-build-cache clean assembleProductionRelease", command)

    def test_reports_difference_and_keeps_both_apks(self) -> None:
        self.env["DIFFERENT_APKS"] = "1"
        result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("son distintos", result.stderr)
        self.assertNotEqual((self.evidence / "first.apk").read_bytes(),
                            (self.evidence / "second.apk").read_bytes())

    def test_rejects_signing_credentials_before_building(self) -> None:
        self.env["GRXFIRMA_ANDROID_KEYSTORE"] = "clave-no-utilizable"
        result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("retire GRXFIRMA_ANDROID_KEYSTORE", result.stderr)
        self.assertFalse(self.log.exists())

    def test_does_not_overwrite_existing_evidence(self) -> None:
        self.evidence.mkdir()
        original = self.evidence / "first.apk"
        original.write_bytes(b"evidencia anterior")
        result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(original.read_bytes(), b"evidencia anterior")
        self.assertFalse(self.log.exists())


if __name__ == "__main__":
    unittest.main()
