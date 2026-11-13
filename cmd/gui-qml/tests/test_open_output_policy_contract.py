# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""D-02: «Abrir» en Qt no entrega al sistema un tipo que se ejecuta."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML_DIR = ROOT / "cmd" / "gui-qml"
BRIDGES = {
    "IpcBridge": QML_DIR / "ipcbridge.cpp",
    "BackendBridge": QML_DIR / "backendbridge.cpp",
}
OPENABLE = QML_DIR / "openablefile.h"
GO_HANDLER = ROOT / "internal" / "adapters" / "inbound" / "desktop" / "ipc" / "handler.go"


def _body(source: str, start: str) -> str:
    begin = source.index(start)
    return source[begin:source.index("\n}\n", begin)]


class OpenOutputPolicyContractTests(unittest.TestCase):
    def test_open_external_checks_document_type_before_opening_file(self) -> None:
        for name, path in BRIDGES.items():
            body = _body(path.read_text(encoding="utf-8"), f"void {name}::openExternal(")
            policy = body.index("OpenableFile::isDocumentToOpen(")
            opening = body.index("QDesktopServices::openUrl(QUrl::fromLocalFile(localPath))")
            self.assertLess(policy, opening, name)
            self.assertNotIn("preferOpenDir", body, name)

    def test_open_signed_document_uses_same_policy(self) -> None:
        for name, path in BRIDGES.items():
            body = _body(path.read_text(encoding="utf-8"), f"void {name}::openSignedDocument(")
            policy = body.index("OpenableFile::isDocumentToOpen(")
            opening = body.index("QDesktopServices::openUrl(")
            self.assertLess(policy, opening, name)

    def test_qt_and_backend_share_the_document_list(self) -> None:
        qt_list = set(re.findall(r'QStringLiteral\("([a-z0-9]+)"\)', OPENABLE.read_text(encoding="utf-8")))
        handler = GO_HANDLER.read_text(encoding="utf-8")
        start = handler.index("var extensionesDocumentalesDesprotegidas")
        go_block = handler[start:handler.index("}\n", handler.index("{", start) + 1)]
        go_list = set(re.findall(r'"\.([a-z0-9]+)"', go_block))
        self.assertTrue(go_list)
        self.assertEqual(qt_list, go_list)
        for executable in ("exe", "lnk", "hta", "js", "scr", "bat", "desktop"):
            self.assertNotIn(executable, qt_list)


if __name__ == "__main__":
    unittest.main()
