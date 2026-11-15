#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 39g: ajustes sin permiso o de otra cuenta en Configuración. WinUI
pinta el texto de su catálogo, enseña la orden en un campo de solo lectura que
se selecciona y copia con el teclado, y no abre el diagnóstico de averías, que
cambiaría «/grant ...» por «[redactado]». Sin dotnet en Linux: contrato
estático; la separación de la orden se prueba con MSTest en Windows."""

import pathlib
import re
import unittest
import xml.etree.ElementTree as ET

ROOT = pathlib.Path(__file__).resolve().parents[3]
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core"
XAML_NS = "{http://schemas.microsoft.com/winfx/2006/xaml}"


def read(path):
    return path.read_text(encoding="utf-8")


class SettingsAccessContract(unittest.TestCase):
    def test_command_box_is_read_only_selectable_and_in_tab_order(self):
        root = ET.parse(UI / "Views/SettingsPage.xaml").getroot()
        boxes = [e for e in root.iter() if e.get(XAML_NS + "Name") == "StatusCommandBox"]
        self.assertEqual(len(boxes), 1)
        box = boxes[0]
        self.assertTrue(box.tag.endswith("TextBox"))
        self.assertEqual(box.get("IsReadOnly"), "True")
        self.assertEqual(box.get("TextWrapping"), "Wrap")
        self.assertEqual(box.get("GotFocus"), "OnStatusCommandGotFocus")
        self.assertIn("ViewModel.StatusCommand", box.get("Text", ""))
        self.assertIn("ViewModel.HasStatusCommand", box.get("Visibility", ""))
        self.assertIsNotNone(box.get("TabIndex"))
        code = read(UI / "Views/SettingsPage.xaml.cs")
        self.assertRegex(code, r"OnStatusCommandGotFocus\([^)]*\)\s*=>\s*StatusCommandBox\.SelectAll\(\)")

    def test_load_and_save_explain_access_before_generic_diagnostic(self):
        code = read(UI / "ViewModels/SettingsPageViewModel.cs")
        for title in ("no_se_pudo_cargar", "no_se_pudo_guardar"):
            explained = code.index(f'ShowSettingsAccessFailure(\n                    "winui.ajustes.{title}"')
            generic = code.index("RequestDiagnostic(", explained)
            self.assertLess(explained, generic)
            self.assertIn("return;", code[explained:generic])
        method = code[code.index("private bool ShowSettingsAccessFailure"):]
        method = method[: method.index("\n    }\n") + 6]
        self.assertIn("SettingsAccessFailure.From(errorCode, Localizer.Text)", method)
        self.assertIn("StatusCommand = failure.Command", method)
        self.assertNotIn("RequestDiagnostic", method)
        # Cualquier otro estado borra la orden de la vez anterior.
        show = code[code.index("private void ShowStatus("):]
        self.assertIn("StatusCommand = string.Empty;", show[: show.index("\n    }\n")])

    def test_only_closed_codes_and_windows_key(self):
        code = read(CORE / "Diagnostics/SettingsAccessFailure.cs")
        self.assertIn('NoPermissionCode = "settings_no_permission"', code)
        self.assertIn('OtherUserCode = "settings_other_user"', code)
        self.assertIn('NoPermissionCode => "error.settings_sin_permiso"', code)
        self.assertNotIn("settings_sin_permiso_unix", code)
        self.assertTrue(re.search(r"_\s*=>\s*null", code))


if __name__ == "__main__":
    unittest.main()
