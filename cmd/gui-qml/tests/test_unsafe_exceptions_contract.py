# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""The desktop acceptance surfaces share the versioned engine catalog."""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[3]
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
QML = ROOT / "cmd/gui-qml/qml"


class UnsafeExceptionsContract(unittest.TestCase):
    def test_all_26_cases_and_screen_keys_exist_in_all_11_catalogs(self):
        spanish = json.loads((LOCALES / "es.json").read_text())
        ids = [key.split(".")[1] for key in spanish if re.fullmatch(r"excepciones\.[a-z0-9_]+\.ajuste", key)]
        self.assertEqual(len(ids), 26)
        screen_keys = [key for key in spanish if key.startswith("excepciones.pantalla.")]
        self.assertGreaterEqual(len(screen_keys), 19)
        paths = sorted(LOCALES.glob("*.json"))
        self.assertEqual(len(paths), 11)
        for path in paths:
            catalog = json.loads(path.read_text())
            for key in screen_keys + [f"excepciones.{id}.{suffix}" for id in ids for suffix in
                                      ("ajuste", "titulo", "riesgo", "aceptacion", "ayuda", "organizacion", "aviso")]:
                with self.subTest(locale=path.stem, key=key):
                    self.assertTrue(catalog[key])
                    self.assertNotEqual(catalog[key], key)

    def test_case_names_use_setting_label_and_help_stays_plain_text(self):
        dialog = (QML / "UnsafeExceptionDialog.qml").read_text()
        panel = (QML / "UnsafeExceptionsPanel.qml").read_text()
        self.assertIn('dialog.caseText("ajuste")', dialog)
        self.assertIn('row.modelData.id + ".ajuste"', panel)
        self.assertIn("textFormat: Text.PlainText", dialog)
        self.assertIn("contentItem: ModalLayer", dialog)
        self.assertIn("Typography.control", dialog)
        self.assertIn("ResponsiveRow", dialog)

    def test_activation_and_retries_are_never_deferred_on_reconnect(self):
        source = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text()
        queue = source.split("bool IpcBridge::queueDeferredRequest", 1)[1].split("void IpcBridge::flushDeferredRequest", 1)[0]
        self.assertRegex(queue, r'if \(action.startsWith\(QStringLiteral\("unsafe_"\)\)\)\s+return false')
        winui = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations/UnsafeExceptionsClient.cs").read_text()
        self.assertIn('"unsafe_operation_retry"', winui)
        self.assertNotIn("JsonSerializer.SerializeToElement(result.Data)", winui)

    def test_blocked_preflight_and_export_do_not_advance_or_steal_dialog_focus(self):
        source = (QML / "main.qml").read_text()
        verify = source.split("function onVerificationFinished(success, message, details)", 1)[1]
        self.assertLess(verify.index("if (!success && unsafeDialog.visible)"), verify.index("window.handleAutoVerificationFinished"))
        export = source.split("function onCertificatePublicExportFinished(ok, message)", 1)[1]
        self.assertLess(export.index("if (!ok && unsafeDialog.visible) return"), export.index("publicCertificateResultDialog.open()"))
        self.assertIn("if (unsafeDialog.visible) { unsafeDialog.contentItem.enter(); return }", source)
        self.assertIn("onCancelled: function(item) { window.finishUnsafeInterruptedFlow(item) }", source)
        preflight = source.split("function onCosignOriginalChecked(ok, needsOriginal)", 1)[1]
        self.assertLess(preflight.index("if (!ok && unsafeDialog.visible)"), preflight.index("window.resumeSignWithCosignChoice"))
        winui = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/SignPageViewModel.cs").read_text()
        self.assertIn("if (!check.IsSuccess && check.UnsafeException is not null)", winui)

    def test_sign_block_card_is_not_a_failure(self):
        """Lote 39a, vuelta 2: una firma bloqueada por seguridad se pinta como
        bloqueo (título del aviso, motivo del catálogo), sin «Error:», sin el
        consejo de avería ni «Reintentar firma»."""
        source = (QML / "main.qml").read_text()
        blocked = source.split("function onUnsafeExceptionBlocked(block)", 1)[1].split("unsafeDialog.present", 1)[0]
        self.assertIn("UnsafeResume.isSigning(block.operationAction)) window.signBlockedPending = true", blocked)
        finished = source.split("function onSigningFinished(success, message, outPath)", 1)[1].split("function onBatchSigningFinished", 1)[0]
        self.assertIn("const blocked = !success && window.signBlockedPending", finished)
        self.assertIn("window.statusMessage = blocked ? window.singleLineText(message) :", finished)
        self.assertIn('(blocked ? "blocked" : "error")', finished)
        announcement = source.split("function signResultAnnouncement()", 1)[1]
        self.assertLess(announcement.index('signResultKind === "blocked"'), announcement.index("No se pudo firmar el documento"))
        retry = source.split("id: retrySignedResultButton", 1)[1][:200]
        self.assertIn('visible: window.signResultKind === "error"', retry)
        advice = source.index('text: tr("Comprueba que el documento siga disponible')
        self.assertIn('visible: window.signResultKind === "error"', source[advice - 400:advice])
        self.assertIn('tr("excepciones.pantalla.titulo")', source.split("id: signResultHeading", 1)[1][:600])

    def test_block_is_not_a_failure_report_and_activation_continues(self):
        """Lote 39a: un bloqueo no abre el asistente ni guarda incidencia, la
        verificación bloqueada no sale «No válida», «Activar y continuar»
        continúa y el aviso común se suma sin repetirse."""
        source = (QML / "main.qml").read_text()
        for kind, guard in (("sign-failure", "} else if (!success && !blocked && !unsafeDialog.visible) {"),
                            ("protect-failure", "} else if (!success && !unsafeDialog.visible) {"),
                            ("unprotect-failure", "} else if (!success && !unsafeDialog.visible) {")):
            report = source.index('window.persistIncidentReport("%s"' % kind)
            self.assertEqual(source.rfind(guard, 0, report), source.rfind("} else if (!success", 0, report), kind)
        batch = source.split("function onBatchSigningFinished(success, message, results)", 1)[1]
        self.assertLess(batch.index("if (!success && unsafeDialog.visible) {"), batch.index('persistIncidentReport("batch-sign-failure"'))
        verify = source.split("function onVerificationFinished(success, message, details)", 1)[1]
        early = verify[:verify.index("window.handleAutoVerificationFinished")]
        self.assertIn("details.unsafeBlocked === true", early)
        self.assertIn("window.unsafeBlockedVerificationPayload(message)", early)
        payload = source.split("function unsafeBlockedVerificationPayload(message)", 1)[1].split("\n    }\n", 1)[0]
        self.assertNotIn("valid:", payload)
        self.assertIn('integrity: { status: "unknown"', payload)
        blocked = source.split("function onUnsafeExceptionBlocked(block) {", 1)[1].split("\n        }\n", 1)[0]
        self.assertIn("window.resetUnsafeNotice()", blocked)
        self.assertIn("pending.resend = window.unsafeOperationCanResend(pending)", blocked)
        self.assertIn('else if (step === "resend") window.resendUnsafeOperation(item)', source)
        self.assertIn("window.requestSignConfirmation(window.selectedCertIndex)", source.split("function resendUnsafeOperation(item)", 1)[1].split("\n    }\n", 1)[0])
        warnings = source.split("function onUnsafeOperationWarnings(warnings) {", 1)[1].split("\n        }\n", 1)[0]
        self.assertIn("window.addUnsafeNotice(window.visibleSigningWarnings(warnings))", warnings)
        self.assertIn("if (withReport !== false) resetUnsafeNotice()", source)
        bridge = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text()
        self.assertIn("if (!diagnostic.isEmpty() && !unsafeBlocked) {", bridge)
        self.assertIn("errMsg = ipcUnsafeBlockMessage(obj, errMsg);", bridge)

    def test_activation_is_not_a_sign_confirmation(self):
        """Lote 39f: tras activar, una firma vuelve a pasar por la
        confirmación de firma (confirmToSign) y no se repite por el retryId
        del motor; el diálogo dice al puente si se aceptó en el aviso o en
        Configuración."""
        source = (QML / "main.qml").read_text()
        resend = source.split("function resendUnsafeOperation(item)", 1)[1].split("\n    }\n", 1)[0]
        self.assertIn("requestSignConfirmation", resend)
        self.assertNotIn("executeSignRequest", resend)
        activated = source.split("onActivated: function(item) {", 1)[1].split("\n        }\n", 1)[0]
        self.assertIn("UnsafeResume.next(item)", activated)
        self.assertNotIn("item.retryId) backend.retryUnsafeOperation", activated)
        blocked = source.split("function onUnsafeExceptionBlocked(block) {", 1)[1].split("\n        }\n", 1)[0]
        self.assertIn('if (pending.resend) pending.retryId = ""', blocked)
        confirm = source.split("function requestSignConfirmation(selectedCertIndex) {", 1)[1].split("\n    }\n", 1)[0]
        self.assertIn("if (!window.confirmToSign) {", confirm)
        self.assertIn("signConfirmDialog.open()", confirm)
        self.assertIn("<file>qml/UnsafeResume.js</file>", (ROOT / "cmd/gui-qml/qml.qrc").read_text())
        dialog = (QML / "UnsafeExceptionDialog.qml").read_text()
        self.assertIn('blocked ? "aviso" : "ajustes"', dialog)


if __name__ == "__main__":
    unittest.main()
