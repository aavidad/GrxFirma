# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Compile the actual version macro, without building or running the product."""

import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

WINDOWS = Path(__file__).resolve().parents[1]


class NsisProductVersionTests(unittest.TestCase):
    def test_all_installers_include_version_resource(self):
        for script in WINDOWS.glob("*.nsi"):
            with self.subTest(script=script.name):
                source = script.read_text(encoding="utf-8")
                self.assertIn('!include "product-version.nsh"', source)
                self.assertIn("!insertmacro GrxFirmaProductVersion", source)

    def test_compile_version_resource(self):
        compiler = shutil.which("makensis")
        if compiler is None:
            self.skipTest("NSIS compiler is not installed")
        for version, numeric in (
            ("0.0.90", "0.0.90.0"),
            ("2.0.1", "2.0.1.0"),
            ("2.0.2-rc.1+build.7", "2.0.2.0"),
            ("2.0.3+build.1", "2.0.3.0"),
            ("dev", "0.0.0.0"),
        ):
            with self.subTest(version=version), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                output = root / "version-test.exe"
                script = root / "test.nsi"
                script.write_text(
                    'Unicode true\n'
                    f'!define VERSION "{version}"\n'
                    f'!include "{WINDOWS / "product-version.nsh"}"\n'
                    '!insertmacro GrxFirmaProductVersion "QA version fixture"\n'
                    'Name "QA version fixture"\n'
                    f'OutFile "{output}"\n'
                    'RequestExecutionLevel user\n'
                    'Section\nSectionEnd\n',
                    encoding="utf-8",
                )
                result = subprocess.run(
                    [compiler, "-V3", str(script)],
                    capture_output=True, text=True, timeout=30,
                )
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                binary = output.read_bytes()
                # The real PE resources retain both the complete SemVer string
                # and the numeric FileVersion, not just NSIS registry commands.
                self.assertIn((version + "\0").encode("utf-16le"), binary)
                self.assertIn((numeric + "\0").encode("utf-16le"), binary)


if __name__ == "__main__":
    unittest.main()
