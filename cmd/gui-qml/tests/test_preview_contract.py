#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Regresiones del contrato de previsualización Qt/QML."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML = ROOT / "cmd/gui-qml/qml/main.qml"
IPC_HEADER = ROOT / "cmd/gui-qml/ipcbridge.h"
IPC_SOURCE = ROOT / "cmd/gui-qml/ipcbridge.cpp"
REST_BRIDGE_HEADER = ROOT / "cmd/gui-qml/backendbridge.h"
REST_BRIDGE_SOURCE = ROOT / "cmd/gui-qml/backendbridge.cpp"
IPC_HANDLER = ROOT / "internal/adapters/inbound/desktop/ipc/handler.go"
IPC_TYPES = ROOT / "internal/adapters/inbound/desktop/ipc/types.go"


def source(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def function_body(text: str, name: str) -> str:
    match = re.search(rf"function\s+{re.escape(name)}\s*\([^)]*\)\s*\{{", text)
    if not match:
        raise AssertionError(f"función QML ausente: {name}")
    depth = 1
    cursor = match.end()
    while cursor < len(text) and depth:
        if text[cursor] == "{":
            depth += 1
        elif text[cursor] == "}":
            depth -= 1
        cursor += 1
    if depth:
        raise AssertionError(f"función QML sin cierre: {name}")
    return text[match.end() : cursor - 1]


class PreviewContractTest(unittest.TestCase):
    def test_navigation_does_not_reset_requested_page(self) -> None:
        qml = source(QML)
        request_body = function_body(qml, "requestPdfPreview")
        payload_body = function_body(qml, "buildSignPayload")
        self.assertIn("applyPageSelectionValidity(false)", request_body)
        self.assertIn("applyPageSelectionValidity(false)", payload_body)
        self.assertIn("backend.getPdfPreview(previewPath, page, requestId)", request_body)
        self.assertNotIn("previewCurrentPage =", request_body)
        self.assertNotIn("previewCurrentPage =", payload_body)

    def test_qml_discards_mismatched_response_context(self) -> None:
        qml = source(QML)
        self.assertIn("requestId !== window.pendingPreviewRequestId", qml)
        self.assertIn("requestedPath !== window.pendingPreviewPath", qml)
        self.assertIn("requestedPage !== window.pendingPreviewPage", qml)
        self.assertIn("window.previewInputPath() !== requestedPath", qml)

    def test_both_bridges_propagate_request_context(self) -> None:
        for path in (IPC_HEADER, REST_BRIDGE_HEADER):
            header = source(path)
            self.assertRegex(
                header,
                r"pdfPreviewReceived\(QString requestId, QString requestedPath,\s*"
                r"int requestedPage, bool ok,",
            )
        for path in (IPC_SOURCE, REST_BRIDGE_SOURCE):
            implementation = source(path)
            self.assertIn("previewRequestId", implementation)
            self.assertIn("requestedPage", implementation)
        self.assertIn("m_previewRequests.clear()", source(IPC_SOURCE))
        self.assertIn(
            "previewRequestId != m_activePreviewRequestId",
            source(REST_BRIDGE_SOURCE),
        )

    def test_visible_seal_uses_real_page_geometry(self) -> None:
        qml = source(QML)
        handler = source(IPC_HANDLER)
        self.assertIn("pageWidth: previewPageWidthPoints", qml)
        self.assertIn("pageHeight: previewPageHeightPoints", qml)
        self.assertIn('visibleSeal["pageWidth"]', handler)
        self.assertIn('visibleSeal["pageHeight"]', handler)
        self.assertIn("x*pageWidth", handler)
        self.assertIn("y*pageHeight", handler)
        self.assertNotIn("x*595.28", handler)
        self.assertNotIn("y*841.89", handler)
        rest_bridge = source(REST_BRIDGE_SOURCE)
        self.assertIn("x * pageWidth", rest_bridge)
        self.assertIn("y * pageHeight", rest_bridge)
        self.assertNotIn("x * 595.28", rest_bridge)
        self.assertNotIn("y * 841.89", rest_bridge)

    def test_batch_visible_seal_supports_document_overrides(self) -> None:
        qml = source(QML)
        ipc_source = source(IPC_SOURCE)
        ipc_types = source(IPC_TYPES)
        handler = source(IPC_HANDLER)
        self.assertIn("batchSealOverrides", qml)
        self.assertIn("documentOverrides.push", qml)
        self.assertIn("Personalizar el sello para este PDF", qml)
        self.assertIn('params["documentOverrides"]', ipc_source)
        self.assertIn('json:"documentOverrides,omitempty"', ipc_types)
        self.assertIn("resolverOverridesFirmaLoteIPC", handler)


if __name__ == "__main__":
    unittest.main()
