# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
"""«Confirmar antes de firmar» se respeta en WinUI (recorrido 0.0.117, A3)."""

import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
UI = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI"


class SignConfirmationContractTests(unittest.TestCase):
    def test_preference_is_read_before_choosing_the_output(self) -> None:
        vm = (UI / "ViewModels" / "SignPageViewModel.cs").read_text(encoding="utf-8")
        self.assertIn("confirmSign = settings.Data.ConfirmBeforeSigning ?? true;", vm)
        self.assertIn("confirmBatch = settings.Data.ConfirmBeforeSigning ?? true;", vm)
        single = vm.index("confirmSign,")
        picker = vm.index("var outputPath = await _filePicker.PickSaveFileAsync(", single)
        self.assertLess(single, picker)
        self.assertIn('Localizer.Text("winui.firmar.confirmar.cancelada")', vm)

    def test_dialog_shows_document_certificate_and_format(self) -> None:
        page = (UI / "Views" / "SignPage.xaml.cs").read_text(encoding="utf-8")
        self.assertIn("ViewModel.SignConfirmationPrompt = ConfirmSignAsync;", page)
        for key in ("documento", "certificado", "operacion", "formato", "firmar", "ayuda"):
            self.assertIn(f'"winui.firmar.confirmar.{key}"', page)
        start = page.index("private async Task<bool> ConfirmSignAsync(")
        body = page[start:page.index("private async Task<bool> ConfirmWindowsCredentialImportAsync", start)]
        self.assertIn("DefaultButton = ContentDialogButton.Close", body)
        self.assertIn("Localizer.ShowAsync(dialog)", body)

    def test_default_matches_qt(self) -> None:
        settings = (UI / "ViewModels" / "SettingsPageViewModel.cs").read_text(encoding="utf-8")
        qml = (ROOT / "cmd" / "gui-qml" / "qml" / "main.qml").read_text(encoding="utf-8")
        self.assertIn("safeSnapshot.ConfirmBeforeSigning ?? true", settings)
        self.assertIn("property bool confirmToSign: true", qml)


if __name__ == "__main__":
    unittest.main()
