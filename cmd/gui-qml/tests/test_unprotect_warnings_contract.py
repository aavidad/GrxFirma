#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Los avisos de compatibilidad al desproteger llegan a la persona."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
WINUI_VM = (
    ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/ProtectPageViewModel.cs"
).read_text(encoding="utf-8")
WINUI_XAML = (
    ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/Views/ProtectPage.xaml"
).read_text(encoding="utf-8")
WINUI_CONTRACT = (
    ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations/DesktopOperationContracts.cs"
).read_text(encoding="utf-8")


class UnprotectWarningsContract(unittest.TestCase):
    def test_qt_pinta_los_avisos_del_resultado(self):
        self.assertIn("id: unprotectWarningsBox", QML)
        # El aviso solo se enseña con la operación hecha, no en un error.
        self.assertIn(
            "!window.unprotectResult.error && window.unprotectResult.warnings", QML
        )

    def test_winui_pinta_los_avisos_del_resultado(self):
        registro = WINUI_CONTRACT.split("record UnprotectResult")[1].split(
            "record SignResult"
        )[0]
        self.assertIn('[JsonPropertyName("warnings")]', registro)
        self.assertIn("result.Data.VisibleWarnings", WINUI_VM)
        self.assertIn("UnprotectWarningMessage", WINUI_XAML)

    def test_winui_traduce_los_avisos_con_el_catalogo(self):
        # Lote 16d: los avisos llegan como claves del catálogo (la frase en
        # castellano); WinUI los pinta en el idioma de la persona, uno por
        # línea, igual que Qt con tr().
        bloque = WINUI_VM[WINUI_VM.index("UnprotectWarningMessage = string.Join("):]
        bloque = bloque[:bloque.index(";") + 1]
        self.assertIn("result.Data.VisibleWarnings.Select(Localizer.VisibleText)", bloque)
        self.assertNotIn('" ",', bloque)


if __name__ == "__main__":
    unittest.main()
