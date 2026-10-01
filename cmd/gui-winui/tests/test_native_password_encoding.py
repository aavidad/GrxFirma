# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
SOURCE = (
    ROOT
    / "cmd"
    / "gui-winui"
    / "src"
    / "GrxFirma.WinUI"
    / "Services"
    / "NativePasswordEncoding.cs"
)


class NativePasswordEncodingContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.source = SOURCE.read_text(encoding="utf-8")

    def test_uses_strict_native_utf8_without_managed_string(self) -> None:
        self.assertIn("WideCharToMultiByte(", self.source)
        self.assertIn("ErrorOnInvalidCharacters", self.source)
        self.assertIn("password.Use(", self.source)
        for forbidden in (
            "new string",
            "PtrToString",
            "Encoding.UTF8",
            ".ToString()",
        ):
            self.assertNotIn(forbidden, self.source)

    def test_failed_output_buffer_is_zeroed(self) -> None:
        self.assertIn(
            "CryptographicOperations.ZeroMemory(utf8);",
            self.source,
        )
        self.assertIn("return utf8;", self.source)


if __name__ == "__main__":
    unittest.main()
