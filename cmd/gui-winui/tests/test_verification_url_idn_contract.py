# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Ambos flujos WinUI usan la misma validación IDN ensayada en Core.Tests."""

from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[3]


class VerificationUrlIdnContract(unittest.TestCase):
    def test_qr_and_csv_delegate_to_the_tested_core(self):
        view = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/SignPageViewModel.cs").read_text()
        for name in ("TryNormalizeCsvUrl", "TryNormalizeVerificationUrl"):
            start = view.index("private static bool " + name) if name == "TryNormalizeCsvUrl" else view.index("private bool " + name)
            body = view[start:].split("\n    }", 1)[0]
            self.assertIn("VerificationUrlNormalizer.TryNormalize", body)
        helper = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations/VerificationUrlNormalizer.cs").read_text()
        for token in ("using System.Globalization;", "new IdnMapping { UseStd3AsciiRules = true }", "GetAscii", "GetUnicode", "char.IsWhiteSpace", "char.IsControl", "65535", "uri.UserInfo"):
            self.assertIn(token, helper)
        tests = (ROOT / "cmd/gui-winui/tests/GrxFirma.WinUI.Core.Tests/VerificationUrlNormalizerTests.cs").read_text()
        self.assertIn('GetProperty("valid")', tests)
        self.assertIn('GetProperty("invalid")', tests)
        self.assertIn("verification_urls.json", tests)
