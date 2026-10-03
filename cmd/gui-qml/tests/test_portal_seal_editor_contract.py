# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]


class PortalSealEditorContract(unittest.TestCase):
    def test_portal_reuses_the_interactive_editor_and_returns_a_decision(self):
        source = (ROOT / "qml/main.qml").read_text(encoding="utf-8")
        for expected in (
            "portalSealEditor.parent = portalSealCanvas",
            "id: portalSealEditor",
            "id: pagePreview",
            "id: sealRect",
            "id: rotateArea",
            "id: resizeArea",
            'portalSealSubmit("place")',
            'portalSealSubmit("without")',
            'portalSealSubmit("cancel")',
            "previewGeometryPath !== portalSeal.documentPath",
            "visible: !portalSealMode",
            'Accessible.name: tr("portal.seal.width")',
            'Accessible.name: tr("portal.seal.rotation")',
        ):
            self.assertIn(expected, source)

    def test_child_reads_only_private_request_and_writes_one_result(self):
        bridge = (ROOT / "portalsealbridge.cpp").read_text(encoding="utf-8")
        for expected in (
            'QStringLiteral("request.json")',
            'QStringLiteral("document.pdf")',
            'QStringLiteral("result.json")',
            "QSaveFile file(m_resultPath)",
            "encoded.size() > 32 * 1024",
        ):
            self.assertIn(expected, bridge)


if __name__ == "__main__":
    unittest.main()
