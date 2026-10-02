#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato de exportación pública de la interfaz Qt."""

import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[3]


class PublicCertificateExportContract(unittest.TestCase):
    def test_qt_routes_both_buttons_through_ipc(self):
        qml = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
        header = (ROOT / "cmd/gui-qml/ipcbridge.h").read_text(encoding="utf-8")
        bridge = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
        self.assertEqual(qml.count('text: tr("Exportar certificado público…")'), 2)
        self.assertIn('text: tr("Compartir mi certificado…")', qml)
        self.assertIn('window.startPublicCertificateExport(false)', qml)
        self.assertIn('window.startPublicCertificateExport(true)', qml)
        self.assertIn('backend.exportPublicCertificate(window.publicCertificateExportId, path, format)', qml)
        self.assertIn('certificatePublicExportFinished', header)
        self.assertIn('sendRequest(QStringLiteral("certificate_export_public"), params)', bridge)

    def test_new_visible_texts_exist_in_eleven_locales(self):
        keys = [
            "Exportar certificado público…", "Compartir mi certificado…",
            "Certificado público DER (*.cer)", "Certificado público PEM (*.pem)",
            "Elegir mi certificado", "Mi certificado público",
            "Certificado público exportado", "No se pudo exportar el certificado",
            "No hay certificados propios disponibles.",
            "Es su certificado público: puede enviarlo sin riesgo. Quien lo reciba podrá proteger archivos que solo usted podrá abrir con GrxFirma (Desproteger).",
        ]
        for locale in ("ca", "de", "en", "es", "eu", "fr", "gl", "it", "pt", "va", "zh"):
            data = json.loads((ROOT / f"internal/adapters/outbound/common/localizador/locales/{locale}.json").read_text(encoding="utf-8"))
            with self.subTest(locale=locale):
                self.assertTrue(all(data.get(key) for key in keys))

    def test_ipc_schema_requires_certificate_id(self):
        schema = json.loads((ROOT / "docs/schemas/desktop-ipc-v1.schema.json").read_text(encoding="utf-8"))
        rules = schema["$defs"]["request"]["allOf"]
        export = next(rule for rule in rules if rule.get("if", {}).get("properties", {}).get("action", {}).get("const") == "certificate_export_public")
        params = export["then"]["properties"]["params"]
        self.assertEqual(params["required"], ["certificateId"])
        self.assertEqual(params["properties"]["format"]["enum"], ["der", "pem"])


if __name__ == "__main__":
    unittest.main()
