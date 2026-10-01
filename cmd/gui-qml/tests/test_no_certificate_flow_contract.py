# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
IPC_CPP = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
IPC_HEADER = (ROOT / "cmd/gui-qml/ipcbridge.h").read_text(encoding="utf-8")
IPC_HANDLER = (
    ROOT / "internal/adapters/inbound/desktop/ipc/handler.go"
).read_text(encoding="utf-8")
BOOTSTRAP = (ROOT / "cmd/grxfirma-gui/main.go").read_text(encoding="utf-8")
CERT_ACCESS = (
    ROOT / "internal/adapters/outbound/desktop/certaccess/adapter.go"
).read_text(encoding="utf-8")


class NoCertificateFlowContractTest(unittest.TestCase):
    def test_empty_state_has_clear_guided_actions_and_cancel(self):
        self.assertIn('tr("No hay certificados disponibles")', QML)
        self.assertIn('tr("Usar certificado sin instalar")', QML)
        self.assertIn('tr("Importar o abrir gestor")', QML)
        self.assertIn('tr("Actualizar certificados")', QML)
        self.assertIn('tr("Cancelar")', QML)

    def test_temporary_and_persistent_flows_are_explicitly_separate(self):
        self.assertIn("backend.useTemporaryCertificate(", QML)
        self.assertIn("backend.importCertificateToStore(", QML)
        self.assertIn(
            "La credencial se mantendrá solo en memoria", QML
        )
        self.assertIn(
            "instalará el certificado de forma persistente", QML
        )

    def test_temporary_flow_supports_p12_pfx_and_pem(self):
        self.assertIn("*.p12 *.pfx *.pem", QML)
        self.assertIn("useTemporaryCertificate", IPC_HEADER)
        self.assertIn('"use_temporary_certificate"', IPC_HANDLER)
        self.assertIn("sessioncertstore.New()", BOOTSTRAP)

    def test_credentials_cross_ipc_as_content_not_local_path(self):
        method = IPC_CPP.split(
            "void IpcBridge::useTemporaryCertificate", 1
        )[1].split(
            "void IpcBridge::removeTemporaryCertificate", 1
        )[0]
        self.assertIn('"credentialB64"', method)
        self.assertNotIn('"path"', method)
        self.assertIn("data.fill('\\0')", method)
        self.assertIn(
            'action == QStringLiteral("use_temporary_certificate")',
            IPC_CPP,
        )

    def test_password_is_not_passed_as_process_argument(self):
        self.assertNotIn('"-P", password', CERT_ACCESS)
        self.assertNotIn('"pass:" + password', CERT_ACCESS)
        self.assertIn("ImportarP12(ctx, targetID, path, password)", CERT_ACCESS)
        self.assertIn("IncidentFormatIpcLogEvent(", IPC_CPP)
        self.assertNotIn("ipcRedactJsonValue", IPC_CPP)
        self.assertNotIn("ipcJsonForLog", IPC_CPP)
        self.assertNotIn("Respuesta IPC cruda", IPC_CPP)

    def test_import_target_is_required_and_catalog_is_refreshed(self):
        self.assertIn('"targetId"', IPC_CPP)
        self.assertIn("debe seleccionar un almacén de destino", CERT_ACCESS)
        action = IPC_CPP.split(
            'if (action == "import_certificate_to_store")', 1
        )[1].split("continue;", 1)[0]
        self.assertIn("refreshCertificates()", action)

    def test_password_fields_are_cleared_on_accept_and_cancel(self):
        self.assertIn(
            "onRejected: temporaryCertificatePasswordField.text = \"\"",
            QML,
        )
        self.assertIn(
            "onRejected: guidedImportPasswordField.text = \"\"",
            QML,
        )
        self.assertGreaterEqual(
            QML.count("temporaryCertificatePasswordField.text = \"\""), 2
        )
        self.assertGreaterEqual(
            QML.count("guidedImportPasswordField.text = \"\""), 2
        )


if __name__ == "__main__":
    unittest.main()
