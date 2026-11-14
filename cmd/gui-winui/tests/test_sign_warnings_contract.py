#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 16d: WinUI enseña tras firmar los avisos que el motor devuelve en
«warnings» (cofirma de una CAdES separada sin el original y firma LT con una
CRL que no cubre al firmante) en el idioma de la persona. Sin dotnet en Linux: contrato estático."""

import json
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
SRC = ROOT / "cmd/gui-winui/src"
CONTRACTS = (SRC / "GrxFirma.WinUI.Core/Operations/DesktopOperationContracts.cs").read_text(encoding="utf-8")
SIGN_VM = (SRC / "GrxFirma.WinUI/ViewModels/SignPageViewModel.cs").read_text(encoding="utf-8")
SIGN_XAML = (SRC / "GrxFirma.WinUI/Views/SignPage.xaml").read_text(encoding="utf-8")
CLIENT_TESTS = (ROOT / "cmd/gui-winui/tests/GrxFirma.WinUI.Core.Tests/DesktopOperationsClientTests.cs").read_text(encoding="utf-8")
BATCH_TESTS = (ROOT / "cmd/gui-winui/tests/GrxFirma.WinUI.Core.Tests/DesktopBatchOperationsClientTests.cs").read_text(encoding="utf-8")
GO_TYPES = (ROOT / "internal/adapters/inbound/desktop/ipc/types.go").read_text(encoding="utf-8")
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"


def record(name):
    start = CONTRACTS.index("public sealed record " + name + "\n")
    return CONTRACTS[start:CONTRACTS.index("\n}\n", start)]


def go_struct(name):
    start = GO_TYPES.index("type " + name + " struct {")
    return GO_TYPES[start:GO_TYPES.index("\n}\n", start)]


class SignWarningsContract(unittest.TestCase):
    def test_los_contratos_leen_warnings_como_go(self):
        for go, cs in (("resultadoFirma", "SignResult"),
                       ("resultadoFirmaLoteItem", "BatchSignItemResult"),
                       ("resultadoFirmaLote", "BatchSignResult")):
            self.assertIn('json:"warnings,omitempty"', go_struct(go), go)
            body = record(cs)
            self.assertIn('[JsonPropertyName("warnings")]', body, cs)
            self.assertIn("public IReadOnlyList<string> VisibleWarnings", body, cs)
        self.assertIn('"warnings"', CLIENT_TESTS[CLIENT_TESTS.index("AssertJsonPropertyNames<SignResult>("):][:200])
        self.assertIn("VisibleWarnings", BATCH_TESTS)

    def test_firma_y_lote_ensenan_los_avisos_traducidos(self):
        self.assertIn("SignWarningMessage = SigningWarningsText(result.Data.VisibleWarnings);", SIGN_VM)
        self.assertIn("BatchWarningMessage = SigningWarningsText(result.Data.VisibleWarnings);", SIGN_VM)
        helper = SIGN_VM[SIGN_VM.index("private static string SigningWarningsText("):]
        helper = helper[:helper.index("\n    }\n")]
        self.assertIn("Localizer.Has(key)", helper)
        self.assertIn("Localizer.Text(key)", helper)
        self.assertIn('x:Name="SignWarningNotice"', SIGN_XAML)
        self.assertIn('x:Name="BatchWarningNotice"', SIGN_XAML)
        for name, flag, message in (("SignWarningNotice", "HasSignWarning", "SignWarningMessage"),
                                    ("BatchWarningNotice", "HasBatchWarning", "BatchWarningMessage")):
            block = SIGN_XAML[SIGN_XAML.rindex("<InfoBar", 0, SIGN_XAML.index('x:Name="' + name + '"')):]
            block = block[:block.index("/>")]
            self.assertIn('IsOpen="{x:Bind ViewModel.' + flag + ', Mode=OneWay}"', block)
            self.assertIn('Message="{x:Bind ViewModel.' + message + ', Mode=OneWay}"', block)
            self.assertIn('Severity="Warning"', block)
            self.assertIn('Title="Advertencias"', block)
        # El aviso de una firma anterior no se queda en la siguiente.
        def method(name):
            start = SIGN_VM.index(name)
            return SIGN_VM[start:SIGN_VM.index("\n    }\n", start)]
        self.assertIn("SignWarningMessage = string.Empty;", method("private void ClearOutput()"))
        self.assertIn("BatchWarningMessage = string.Empty;", method("private void ResetBatchResults()"))
        batch = SIGN_VM[SIGN_VM.index("public async Task<OperationDiagnostic?> SignBatchAsync("):]
        self.assertLess(batch.index("BatchWarningMessage = string.Empty;"), batch.index("await operations.SignBatchAsync("))

    def test_los_avisos_tienen_texto_en_los_once_idiomas(self):
        for language in ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh"):
            catalog = json.loads((LOCALES / (language + ".json")).read_text(encoding="utf-8"))
            for key in ("firma.cofirma_sin_original.aviso", "firma.lt.crl_no_aplicable.aviso", "Advertencias"):
                self.assertTrue(catalog.get(key), (language, key))


if __name__ == "__main__":
    unittest.main()
