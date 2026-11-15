# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 39k en Qt: al cambiar de sección, el foco no se queda en un control
de la sección oculta, porque un Intro lo pulsaría a ciegas (por ejemplo, un
interruptor de seguridad de Configuración)."""

import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
MAIN = ROOT / "cmd/gui-qml/qml/main.qml"


class FocusOnTabChangeContract(unittest.TestCase):
    def setUp(self):
        self.text = MAIN.read_text(encoding="utf-8")

    def test_tab_change_moves_focus_out_of_hidden_tab(self):
        start = self.text.index("onActiveTabChanged: {")
        handler = self.text[start:self.text.index("}", start)]
        self.assertIn("Qt.callLater(moveFocusOutOfHiddenTab)", handler)

    def test_focus_goes_to_visible_tab(self):
        start = self.text.index("function moveFocusOutOfHiddenTab()")
        body = self.text[start:start + 600]
        self.assertIn("window.activeFocusItem", body)
        self.assertIn("itemIsShown(focused)", body)
        self.assertIn("mainContentStack.children[mainContentStack.currentIndex]", body)
        self.assertIn("forceActiveFocus", body)

    def test_stack_has_id(self):
        self.assertEqual(self.text.count("id: mainContentStack"), 1)


if __name__ == "__main__":
    unittest.main()
