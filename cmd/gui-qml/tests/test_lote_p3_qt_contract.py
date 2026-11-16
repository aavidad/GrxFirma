# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote p3: defectos menores de la app Qt que quedaron de la 0.0.128."""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[3]
QML = ROOT / "cmd/gui-qml/qml"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"


def main_qml():
    return (QML / "main.qml").read_text(encoding="utf-8")


def catalogs():
    paths = sorted(LOCALES.glob("*.json"))
    assert len(paths) == 11
    return {p.stem: json.loads(p.read_text(encoding="utf-8")) for p in paths}


class LoteP3QtContract(unittest.TestCase):
    def test_unsafe_notice_sits_at_the_top_of_the_content(self):
        source = main_qml()
        banner = source.split("id: unsafeNoticeBanner", 1)[1].split("ScrollView {", 1)[0]
        self.assertIn("anchors.top: updateBanner.visible ? updateBanner.bottom : parent.top", banner)
        self.assertNotIn("anchors.bottom:", banner)
        content = source.split("anchors.topMargin: (updateBanner.visible ? updateBanner.height : 0)", 1)[1][:300]
        self.assertIn("(unsafeNoticeBanner.visible ? unsafeNoticeBanner.height : 0)", content)
        self.assertIn("anchors.bottomMargin: statusBar.height\n", content)

    def test_enabled_unsafe_row_is_marked_by_shape_not_only_colour(self):
        panel = (QML / "UnsafeExceptionsPanel.qml").read_text(encoding="utf-8")
        self.assertIn("readonly property bool unsafeActive: modelData.enabled === true", panel)
        self.assertIn("border.width: unsafeActive ? 2 : 0", panel)
        self.assertIn('objectName: "unsafeActiveIcon-" + row.modelData.id', panel)
        self.assertIn("visible: row.unsafeActive", panel)
        # El icono crece con la letra (hasta el 200 %) y su color llega a 3:1
        # frente al recuadro y a la página.
        self.assertIn("Typography.minimum(22)", panel)
        self.assertIn("Contrast.accentOnBoth(", panel)
        self.assertIn("Contrast.readableOn(fillColor, panel.theme.textColor)", panel)
        contrast = (QML / "ThemeContrast.js").read_text(encoding="utf-8")
        self.assertIn("function accentOnBoth(first, second, accent, minimum)", contrast)
        self.assertIn("function tinted(surface, tint, amount)", contrast)

    def test_local_ca_notice_waits_for_unreadable_settings_dialog(self):
        source = main_qml()
        show = source.split("function showLocalTLSStartupNotice(status)", 1)[1].split("function startDirectUpdateCheck", 1)[0]
        self.assertNotIn("localTLSStartupNoticeDialog.open()\n    }\n    // El aviso", show.split("function openPendingLocalTLSStartupNotice", 1)[0])
        opener = show.split("function openPendingLocalTLSStartupNotice()", 1)[1].split("function markStartupSettingsAnswered", 1)[0]
        self.assertIn("!startupSettingsAnswered || settingsAccessDialog.visible", opener)
        failed = source.split("function onSettingsAccessFailed(action, message)", 1)[1].split("function onSettingsLoaded", 1)[0]
        self.assertLess(failed.index("localTLSStartupNoticeDialog.close()"), failed.index("settingsAccessDialog.present"))
        self.assertIn("window.markStartupSettingsAnswered()", failed)
        dialog = source.split("id: settingsAccessDialog", 1)[1][:400]
        self.assertIn("onClosed: Qt.callLater(window.openPendingLocalTLSStartupNotice)", dialog)
        self.assertIn("id: localTLSStartupFallbackTimer", source)

    def test_block_title_and_reason_do_not_repeat_each_other(self):
        for lang, cat in catalogs().items():
            title = cat["excepciones.pantalla.titulo"]
            ids = [k.split(".")[1] for k in cat if re.fullmatch(r"excepciones\.[a-z0-9_]+\.ajuste", k)]
            for case in ids:
                with self.subTest(lang=lang, case=case):
                    # La tarjeta quita del motivo el título común o el del
                    # caso; si no empezara por ninguno, diría dos veces lo mismo.
                    reason = cat[f"excepciones.{case}.bloqueo"]
                    self.assertTrue(reason.startswith(title) or reason.startswith(cat[f"excepciones.{case}.titulo"]))
        # En inglés el título nombra lo que se ha detenido.
        self.assertEqual(catalogs()["en"]["excepciones.pantalla.titulo"], "Operation stopped for your security")
        source = main_qml()
        cause = source.split("function signBlockedCause(message)", 1)[1].split("function signVerificationResultText", 1)[0]
        self.assertIn('"excepciones." + window.signBlockedCaseId + ".titulo"', cause)
        self.assertIn('tr("excepciones.pantalla.titulo")', cause)
        blocked = source.split("function onUnsafeExceptionBlocked(block)", 1)[1][:900]
        self.assertIn('window.signBlockedCaseId = String(block.id || "")', blocked)

    def test_recipients_reload_is_a_user_request(self):
        source = main_qml()
        reload = source.split('text: tr("Cargar destinatarios")', 1)[1][:400]
        self.assertIn("backend.loadProtectionRecipients(true)", reload)
        bridge = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
        load = bridge.split("void IpcBridge::loadProtectionRecipients(bool userRequested)", 1)[1].split("\n}\n", 1)[0]
        self.assertIn("m_protectionRecipientsUserRequests.insert(m_lastRequestId)", load)
        self.assertIn("m_protectionRecipientsUserRequests.remove(requestId)", bridge)
        self.assertIn("(matchesPendingRequest || userRecipientsRequest)", bridge)
        self.assertNotIn("m_protectionRecipientsUserRequest =", bridge)

    def test_file_key_read_texts_fit_a_key_just_created(self):
        old_words = {
            "es": "antigua", "en": "old ", "ca": "antiga", "va": "antiga", "gl": "antiga",
            "eu": "zahar", "fr": "ancienne", "it": "vecchia", "pt": "antiga", "de": "alten", "zh": "旧",
        }
        for lang, cat in catalogs().items():
            # bloqueo, aceptacion y ayuda tienen copia en Android y se
            # cambian junto con ella (pendiente del lote de Android).
            for suffix in ("aviso", "riesgo"):
                with self.subTest(lang=lang, suffix=suffix):
                    text = cat["excepciones.alto_clave_fichero_leer." + suffix]
                    self.assertNotIn(old_words[lang], text.lower() if lang != "zh" else text)

    def test_certificate_status_is_refreshed_with_every_list(self):
        source = main_qml()
        loaded = source.split("function onCertificatesLoaded(certs)", 1)[1].split("function onStatusChanged()", 1)[0]
        self.assertIn('tr("Certificados cargados: %1").arg(certs.length)', loaded)
        self.assertIn("window.certificateOperationStatus = \"\"", loaded)
        self.assertIn('else window.smartcardMessage = ""', loaded)
        smartcard = source.split("function onSmartcardStatusReceived(ok, readers, message)", 1)[1][:2000]
        self.assertLess(smartcard.index("window.smartcardRefreshPending = true"), smartcard.index("backend.refreshCertificates()"))
        for path in ("cmd/gui-qml/ipcbridge.cpp", "cmd/gui-qml/backendbridge.cpp"):
            bridge = (ROOT / path).read_text(encoding="utf-8")
            # Cada lista lleva su estado: antes (carga, la QML pone el
            # recuento) o después con announceStatus (comprobación), que
            # avisa aunque el texto se repita.
            for match in re.finditer(r"emit certificatesLoaded\((\w+)\);", bridge):
                before = bridge[:match.start()].rstrip().splitlines()
                after = bridge[match.end():].lstrip().splitlines()
                with self.subTest(path=path, at=match.start()):
                    near_before = "\n".join(before[-3:])
                    self.assertTrue("setStatus(" in near_before or "announceStatus(" in "\n".join(after[:8]),
                                    (near_before, after[:8]))
                    self.assertNotIn("setStatus(", after[0])
        for lang, cat in catalogs().items():
            with self.subTest(lang=lang):
                self.assertIn("%1", cat["Certificados cargados: %1"])


if __name__ == "__main__":
    unittest.main()
