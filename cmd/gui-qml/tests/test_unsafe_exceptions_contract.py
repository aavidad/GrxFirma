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


if __name__ == "__main__":
    unittest.main()
