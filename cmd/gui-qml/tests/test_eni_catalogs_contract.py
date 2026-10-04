# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
import json
import pathlib
import subprocess
import sys
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]

class EniCatalogsContract(unittest.TestCase):
    def test_qt_and_winui_lists_match_go_exactly(self):
        subprocess.run([sys.executable, str(ROOT / "scripts/generar_catalogos_eni.py"), "--check"], check=True, cwd=ROOT)
        qt = (ROOT / "cmd/gui-qml/qml/EniPanel.qml").read_text(encoding="utf-8")
        win = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/Views/EniPage.xaml.cs").read_text(encoding="utf-8")
        for name in ("EstadosElaboracion", "TiposDocumentales", "EstadosExpediente"):
            self.assertIn("Catalog." + name, qt)
            self.assertIn("EniCatalog." + name, win)
        for path in (ROOT / "internal/adapters/outbound/common/localizador/locales").glob("*.json"):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            for code in ("EE01", "EE02", "EE03", "EE04", "EE99", "E01", "E02", "E03", "TD99", *[f"TD{i:02}" for i in range(1, 21)]):
                self.assertTrue(catalog["eni.codigo." + code], (path, code))

    def test_calendar_imports_and_validation_before_save(self):
        qt = (ROOT / "cmd/gui-qml/qml/EniDateField.qml").read_text(encoding="utf-8")
        self.assertIn("import QtQuick.Controls\n", qt)
        self.assertIn("MonthGrid {", qt)
        self.assertIn("DayOfWeekRow {", qt)
        panel = (ROOT / "cmd/gui-qml/qml/EniPanel.qml").read_text(encoding="utf-8")
        self.assertIn("if (panel.validateFields([docOrgan, capture, docId, sourceId, format])) documentSave.open()", panel)
        self.assertIn("first.focusInput()", panel)

if __name__ == "__main__":
    unittest.main()
