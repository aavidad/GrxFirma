# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Comprueba el contrato de versiones que consume checkupdates de F-Droid."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[4]
RECIPE = ROOT / "packaging/fdroid/io.github.aavidad.grxfirma.yml"


def field(name: str) -> str:
    return next(
        line.split(": ", 1)[1]
        for line in RECIPE.read_text(encoding="utf-8").splitlines()
        if line.startswith(name + ": ")
    )


class FdroidVersionsTest(unittest.TestCase):
    def test_accepts_only_release_tags_with_optional_v(self) -> None:
        pattern = field("UpdateCheckMode").removeprefix("Tags ")
        for version in ("0.0.127", "0.1.0", "1.0.0", "12.345.678"):
            for prefix in ("", "v"):
                with self.subTest(tag=prefix + version):
                    self.assertIsNotNone(re.match(pattern, prefix + version))
        for tag in ("main", "release-0.1.0", "0.1", "v0.1.0-rc1", "0.1.0+meta",
                    "v0.1.0extra", "01.2.3", "1.02.3", "1.2.03", "1.2.3\n"):
            with self.subTest(tag=tag):
                self.assertIsNone(re.match(pattern, tag))

    def test_checkupdates_reads_full_integer_and_version_name(self) -> None:
        # checkupdates separa exactamente cuatro campos y usa group(1).
        # Una alternancia con «|» dentro de la expresión rompería ese formato.
        code_file, code_regex, name_file, name_regex = field("UpdateCheckData").split("|")
        for version, expected in (("0.0.127", 127), ("0.1.0", 1000),
                                  ("0.12.345", 12345), ("1.0.0", 1000000),
                                  ("2.3.4", 2003004)):
            major, minor, patch = map(int, version.split("."))
            code = major * 1000000 + minor * 1000 + patch
            with self.subTest(version=version):
                self.assertEqual(code, expected)
                properties = f"# Cabecera de licencia\nversionCode={code}\n"
                self.assertEqual(int(re.search(code_regex, properties).group(1)), expected)
                self.assertEqual(re.search(name_regex, version + "\n").group(1), version)
        properties = (ROOT / code_file).read_text(encoding="utf-8")
        version = (ROOT / name_file).read_text(encoding="utf-8").strip()
        major, minor, patch = map(int, version.split("."))
        self.assertEqual(int(re.search(code_regex, properties).group(1)),
                         major * 1000000 + minor * 1000 + patch,
                         "Actualice fdroid-version.properties junto a VERSION.txt")


if __name__ == "__main__":
    unittest.main()
