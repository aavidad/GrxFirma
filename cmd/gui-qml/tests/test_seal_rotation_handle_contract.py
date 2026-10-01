# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


QML = Path(__file__).resolve().parents[1] / "qml" / "main.qml"


class SealRotationHandleContract(unittest.TestCase):
    def test_pointer_keyboard_and_preview(self) -> None:
        source = QML.read_text(encoding="utf-8")
        for expected in (
            "id: rotateArea",
            "width: 44",
            "height: 44",
            "preventStealing: true",
            'Accessible.name: tr("Girar sello")',
            "activeFocusOnTab: true",
            "Keys.onPressed:",
            "Qt.ShiftModifier",
            "Math.atan2(point.y - cy, point.x - cx)",
            "Math.abs(degrees - nearest) <= 4",
            "if (!sealRotationDragging) scheduleSealPreview()",
            "window.scheduleSealPreview()",
        ):
            self.assertIn(expected, source)


if __name__ == "__main__":
    unittest.main()
