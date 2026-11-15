# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 39m en Qt: al recorrer con el tabulador una zona desplazable (la lista
de casos inseguros de Configuración > Seguridad), la zona se desplaza para que
el control con el foco se vea entero (WCAG 2.4.7 y 2.4.11). Con el ratón no se
mueve nada y no se rompe el cambio de foco al cambiar de sección (lote 39k)."""

import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
MAIN = ROOT / "cmd/gui-qml/qml/main.qml"
PANEL = ROOT / "cmd/gui-qml/qml/UnsafeExceptionsPanel.qml"


class FocusScrollContract(unittest.TestCase):
    def setUp(self):
        self.text = MAIN.read_text(encoding="utf-8")
        start = self.text.index("function ensureFocusVisible()")
        self.body = self.text[start:self.text.index("\n    }\n", start)]

    def test_window_reacts_to_focus_changes(self):
        self.assertEqual(self.text.count("onActiveFocusItemChanged:"), 1)
        self.assertIn("onActiveFocusItemChanged: Qt.callLater(ensureFocusVisible)", self.text)

    def test_mouse_focus_does_not_scroll(self):
        self.assertIn("focused.focusReason === Qt.MouseFocusReason) return", self.body)

    def test_hidden_items_are_ignored(self):
        self.assertIn("itemIsShown(focused)", self.body)

    def test_scrolls_every_flickable_ancestor_within_bounds(self):
        self.assertIn("for (let flick = focused.parent; flick; flick = flick.parent)", self.body)
        self.assertIn("mapToItem(flick.contentItem", self.body)
        self.assertIn("focusScrollMargin", self.body)
        self.assertIn("flick.originY", self.body)
        self.assertIn("flick.contentHeight - flick.height", self.body)
        self.assertIn("flick.contentY = target", self.body)

    def test_hidden_tab_focus_still_moves(self):
        start = self.text.index("onActiveTabChanged: {")
        handler = self.text[start:self.text.index("}", start)]
        self.assertIn("Qt.callLater(moveFocusOutOfHiddenTab)", handler)

    def test_panel_lives_in_config_scroll(self):
        start = self.text.index("id: configMainOuterScroll")
        self.assertLess(start, self.text.index("UnsafeExceptionsPanel {"))
        self.assertTrue(PANEL.exists())

    def test_no_visible_text_in_helper(self):
        self.assertNotIn("tr(", self.body)
        self.assertNotIn("qsTr(", self.body)


if __name__ == "__main__":
    unittest.main()
