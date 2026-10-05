#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato entre los dos botones WinUI y el cliente IPC tipado."""

from pathlib import Path
import unittest

from winui_catalog import read_with_catalog

ROOT = Path(__file__).resolve().parents[3]


class PublicCertificateExportContract(unittest.TestCase):
    def test_both_pages_use_shared_export_flow(self):
        root = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
        certificates = (root / "Views/CertificatesPage.xaml").read_text(encoding="utf-8")
        protect = (root / "Views/ProtectPage.xaml").read_text(encoding="utf-8")
        for page, label, handler in (
            (certificates, "Exportar certificado público…", "OnExportPublicCertificateClick"),
            (protect, "Compartir mi certificado…", "OnShareMyCertificateClick"),
        ):
            self.assertIn(label, page)
            self.assertIn(handler, page)
        shared = read_with_catalog(root / "Views/PublicCertificateExport.cs")
        self.assertIn("SaveFilePickerProfile.PublicCertificate", shared)
        self.assertIn("ExportPublicCertificateAsync", shared)
        self.assertIn("Es su certificado público: puede enviarlo sin riesgo.", shared)

    def test_json_contract(self):
        core = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations"
        client = (core / "DesktopOperationsClient.cs").read_text(encoding="utf-8")
        contracts = (core / "DesktopOperationContracts.cs").read_text(encoding="utf-8")
        self.assertIn('CertificateExportPublic = "certificate_export_public"', client)
        for key in ("certificateId", "outputPath", "format", "certificateDerBase64", "encryptionSuitable"):
            self.assertIn(f'[JsonPropertyName("{key}")]', contracts)


if __name__ == "__main__":
    unittest.main()
