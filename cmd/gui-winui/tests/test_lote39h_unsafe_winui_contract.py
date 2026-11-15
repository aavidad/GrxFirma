#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 39h en WinUI: ayuda por apartados con encabezados, modo organización,
una sola nota por fila, fecha con el formato del catálogo, aviso de carga solo
si tarda y evidencias sin claves técnicas. Contrato estático (sin dotnet en
Linux); la lógica de Core se prueba con UnsafeExceptionTextTests en Windows."""

import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core"


def read(path):
    return path.read_text(encoding="utf-8")


class HelpSections(unittest.TestCase):
    def setUp(self):
        self.code = read(UI / "Controls/UnsafeExceptionDialog.cs")

    def test_help_is_built_section_by_section_with_headings(self):
        self.assertIn("UnsafeExceptionText.HelpSections(", self.code)
        self.assertIn('"excepciones.pantalla.ayuda_organizacion"', self.code)
        self.assertIn("UnsafeExceptionText.OrganizationDecides(item)", self.code)
        self.assertIn("AutomationHeadingLevel.Level3", self.code)

    def test_long_help_is_not_one_block_of_text(self):
        # Antes la ayuda entera iba en un solo TextBlock y como texto de ayuda
        # del ScrollViewer: el lector la leía de corrido.
        self.assertNotIn("Prose(helpOnly ? item.HelpKey", self.code)
        self.assertNotIn("risk.Text = CatalogText(item.HelpKey);\n                AutomationProperties", self.code)
        self.assertIn("AutomationProperties.SetHelpText(scroll, helpOnly ? string.Empty : risk.Text)", self.code)


class SettingsRows(unittest.TestCase):
    def setUp(self):
        self.code = read(UI / "Views/SettingsPage.UnsafeExceptions.cs")

    def test_policy_note_is_said_once(self):
        self.assertIn("UnsafeExceptionText.PolicyNoteKey(item)", self.code)
        self.assertNotIn('Localizer.Text("excepciones.pantalla.solo_lectura") +', self.code)

    def test_activation_date_uses_the_catalogue_layout(self):
        self.assertIn('Localizer.Text("format.datetime_minutes")', self.code)
        self.assertNotIn('ToString("g"', self.code)

    def test_loading_text_only_when_the_query_is_slow(self):
        # La región viva anunciaba «Consultando las excepciones.» con la lista
        # ya cargada (recorrido G).
        self.assertNotIn('UnsafeMessage.Text = Localizer.Text("excepciones.pantalla.cargando");', self.code)
        self.assertIn("ShowUnsafeLoadingLaterAsync(", self.code)
        self.assertIn("slow.Cancel();", self.code)


class EvidenceSummary(unittest.TestCase):
    def test_verify_details_translate_the_evidence_summary(self):
        model = read(UI / "ViewModels/VerifyPageViewModel.cs")
        self.assertIn("VerificationDetailText.EvidenceSummary(SafeIpcText.Clean(item.Summary", model)
        self.assertIn("public static string EvidenceSummary(CatalogLocalizer localizer, string? summary)",
                      read(CORE / "Operations/VerificationDetailText.cs"))


if __name__ == "__main__":
    unittest.main()
