# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
"""Proteger sin jerga y con estado vacío (recorrido Windows 0.0.117, M6)."""

import unittest
from pathlib import Path

UI = Path(__file__).resolve().parents[1] / "src" / "GrxFirma.WinUI"


class ProtectPlainLanguageContractTests(unittest.TestCase):
    def test_key_notice_only_with_the_key_format_and_empty_state(self) -> None:
        xaml = (UI / "Views" / "ProtectPage.xaml").read_text(encoding="utf-8")
        vm = (UI / "ViewModels" / "ProtectPageViewModel.cs").read_text(encoding="utf-8")
        self.assertIn('IsOpen="{x:Bind ViewModel.IsEncryptedDataSelected, Mode=OneWay}"', xaml)
        self.assertIn('Visibility="{x:Bind ViewModel.ShowsNoRecipientsHint, Mode=OneWay}"', xaml)
        self.assertIn("!IsEncryptedDataSelected && VisibleRecipients.Count == 0", vm)
        for jargon in ("Perfil criptográfico", "CMS EncryptedData usa", "clave AES-256 transitoria",
                       "clave efímera cuando el contenedor"):
            self.assertNotIn(jargon, xaml)


if __name__ == "__main__":
    unittest.main()
