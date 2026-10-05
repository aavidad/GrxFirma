# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Qt solo pide reemplazar cuando la ruta viene de un diálogo de guardar.

Los paneles de Facturae y ENI eligen siempre la ruta en un FileDialog de
guardar, que pregunta antes de reemplazar (no usan DontConfirmOverwrite). En la
pantalla de firma la ruta también puede escribirse a mano o calcularse: solo
se confirma el reemplazo si la ruta sigue siendo la elegida en su diálogo de
guardar; en otro caso manda la preferencia «Sobrescritura» que aplica el motor.
"""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
QT = ROOT / "cmd" / "gui-qml"


def read(relative: str) -> str:
    return (QT / relative).read_text(encoding="utf-8-sig")


class QtOverwriteConfirmationContractTests(unittest.TestCase):
    def test_dialog_panels_send_confirmation(self) -> None:
        eni = read("qml/EniPanel.qml")
        self.assertEqual(2, eni.count("overwriteConfirmed: true"))
        facturae = read("qml/FacturaePanel.qml")
        self.assertIn("createFacturae(panel.draft(), path, true)", facturae)

    def test_save_dialogs_keep_overwrite_prompt(self) -> None:
        for relative in ("qml/EniPanel.qml", "qml/FacturaePanel.qml", "qml/main.qml"):
            with self.subTest(relative=relative):
                self.assertNotIn("DontConfirmOverwrite", read(relative))

    def test_sign_confirms_only_the_path_chosen_in_its_save_dialog(self) -> None:
        main = read("qml/main.qml")
        self.assertIn("window.confirmedOverwritePath = window.currentOutputPath", main)
        self.assertIn(
            'overwriteConfirmed: confirmedOverwritePath !== "" && confirmedOverwritePath === currentOutputPath',
            main,
        )
        self.assertIn('window.confirmedOverwritePath = ""', main)

    def test_bridge_forwards_confirmation_only_when_asked(self) -> None:
        header = read("ipcbridge.h")
        self.assertIn("bool overwriteConfirmed = false", header)
        source = read("ipcbridge.cpp")
        self.assertIn('{QStringLiteral("overwriteConfirmed"), overwriteConfirmed}', source)


if __name__ == "__main__":
    unittest.main()
