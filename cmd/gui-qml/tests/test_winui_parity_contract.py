#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contratos de paridad visible de Linux Qt con WinUI."""

from pathlib import Path
import re
import shutil
import subprocess
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
IPC = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
MAIN = (ROOT / "cmd/gui-qml/main.cpp").read_text(encoding="utf-8")
USER_INSTALLER = (ROOT / "packaging/linux/install-user.sh").read_text(encoding="utf-8")


def qml_function(name: str) -> str:
    start = re.search(rf"    function {name}\([^)]*\) \{{", QML)
    if start is None:
        raise AssertionError(f"Falta la función {name}")
    depth = 1
    pos = start.end()
    while depth:
        if QML[pos] == "{":
            depth += 1
        elif QML[pos] == "}":
            depth -= 1
        pos += 1
    return QML[start.start():pos]


class WinUiParityContractTest(unittest.TestCase):
    def test_real_seal_preview_keeps_ipc_connection(self) -> None:
        self.assertIn('"seal_preview"', IPC)
        self.assertIn("m_sealPreviewRequests.clear()", IPC)
        self.assertIn("requestId !== window.sealPreviewRequestId", QML)
        self.assertIn('sealPreviewRequestId = ""', qml_function("scheduleSealPreview"))
        self.assertIn("backend.getSealPreview(options, id)", qml_function("requestSealPreview"))
        self.assertIn("source: window.sealPreviewImage", QML)
        self.assertNotIn("m_socket->abort()", IPC.split("void IpcBridge::getSealPreview", 1)[1].split("void IpcBridge::protectFileAdvanced", 1)[0])

    def test_signing_and_preview_use_same_appearance(self) -> None:
        self.assertIn('options.visibleSealLogo = "institucional"', qml_function("sealAppearanceOptions"))
        self.assertIn("sealAppearanceOptions()", qml_function("requestSealPreview"))
        self.assertIn("sealAppearanceOptions()", qml_function("buildSignPayload"))
        self.assertIn('if (sealStyle === "image"', qml_function("sealPayloadFromConfig"))
        self.assertIn("normalizedQrUrl(signQRContent)", qml_function("buildSignPayload"))
        self.assertIn("from: 0", QML)
        self.assertIn("to: 359", QML)
        for label in ("Primera", "Última", "Todas", "Solo texto", "Imagen propia"):
            self.assertIn(f'tr("{label}")', QML)

    def test_rotated_preview_keeps_card_dimensions(self) -> None:
        source = QML.split('source: window.sealPreviewImage', 1)[1].split('onStatusChanged:', 1)[0]
        self.assertIn('fillMode: Image.Stretch', source)
        self.assertNotIn('rotation:', source)
        self.assertIn('anchors.fill: parent', QML.split('source: window.sealPreviewImage', 1)[0].rsplit('Image {', 1)[1])
        self.assertIn('seal.rotation = 0', qml_function('requestSealPreview'))
        self.assertIn('rotation: window.signSealRotation', QML.split('id: sealRect', 1)[1].split('// Contenido del sello', 1)[0])
        self.assertIn('mapToItem(sealRect, mouse.x, mouse.y)', QML)
        self.assertIn('constrainSealCardToPage()', qml_function('syncSealFromPreview'))
        self.assertEqual(QML.split('id: sealRect', 1)[1].split('function ', 1)[0].count('preventStealing: true'), 3)
        self.assertIn('signQREnabled ? normalizedQrUrl(signQRContent) : ""', qml_function('requestSealPreview'))
        self.assertIn('signQREnabled && signQRContent.trim() !== ""', qml_function('buildSignPayload'))
        self.assertIn('text: tr("sign.seal.include_verification_qr")', QML)
        self.assertIn('id: signSealLogoOpacitySlider', QML.split('id: sealPresetsLayout', 1)[1].split('id: padesMetaLayout', 1)[0])
        self.assertIn('signQREnabled: window.signQREnabled', qml_function('saveBackendSettings'))
        self.assertIn('window.signQREnabled = s.signQREnabled === undefined', QML)

    def test_logo_opacity_reaches_real_preview_and_document_overrides(self) -> None:
        self.assertIn("logoOpacityPercent: config.logoOpacityPercent === undefined ? 100 : Number(config.logoOpacityPercent)", qml_function("sealPayloadFromConfig"))
        self.assertIn("logoOpacityPercent: signSealLogoOpacityPercent", qml_function("captureSealConfig"))
        self.assertIn("logoOpacityPercent: 100", qml_function("defaultSealState"))
        self.assertIn("sealPayloadFromConfig(captureSealConfig())", qml_function("requestSealPreview"))
        self.assertIn("sealPayloadFromConfig(batchSealOverrides[path])", qml_function("buildSignPayload"))
        self.assertIn('onSignSealLogoOpacityPercentChanged: { scheduleSettingsSave(); scheduleSealPreview() }', QML)
        self.assertEqual(QML.count('Accessible.name: tr("sign.seal.opacity")'), 2)
        self.assertEqual(QML.count('Accessible.description: tr("sign.seal.opacity_help")'), 2)
        self.assertIn('visibleSeal: seal', qml_function('requestSealPreview'))
        self.assertIn('body.visibleSeal = sealPayloadFromConfig(globalConfig)', qml_function('buildSignPayload'))
        self.assertIn('visibleSeal: overrideSeal', qml_function('buildSignPayload'))
        self.assertEqual(QML.count('window.signSealLogoOpacityPercent = Math.round(value)'), 2)
        self.assertGreaterEqual(QML.count('focusPolicy: Qt.StrongFocus\n'), 5)

    def test_per_page_seal_and_https_qr_reach_ipc(self) -> None:
        self.assertIn("payload.placements", qml_function("sealPayloadFromConfig"))
        self.assertIn("normalizedQrUrl(signQRContent)", qml_function("requestSealPreview"))
        self.assertIn("normalizedQrUrl(signQRContent)", qml_function("buildSignPayload"))
        self.assertIn("window.removeSealFromPage()", QML)
        self.assertIn("window.applySealToAllPages()", QML)
        self.assertIn('visible: window.signQREnabled', QML)

    def test_certificates_smartcard_and_recipients(self) -> None:
        for action in ("smartcard_status", "protection_recipient_import", "protection_recipient_remove"):
            self.assertIn(f'"{action}"', IPC)
        self.assertIn('tr("Vence: %1").arg(window.certificateExpiry(modelData))', QML)
        self.assertIn('tr("Renovar en la FNMT")', QML)
        self.assertIn('https://www.sede.fnmt.gob.es/certificados/persona-fisica/renovar', QML)
        self.assertIn('tr("Origen: %1")', QML)
        self.assertIn('tr("Usar DNIe o tarjeta")', QML)
        self.assertIn("backend.refreshCertificates()", QML)
        self.assertEqual(QML.count('textFormat: Text.PlainText\n                                                text: tr("Titular: ")'), 2)
        self.assertNotIn('text: tr("<b>Organización:</b> ")', QML)

    def test_release_notes_are_installed_and_bounded(self) -> None:
        self.assertIn("help/NOVEDADES.md", MAIN)
        self.assertIn("info.isSymLink()", MAIN)
        self.assertIn("64 * 1024", MAIN)
        self.assertIn('"releaseNotesText", releaseNotesSource', MAIN)
        self.assertIn("textFormat: TextEdit.MarkdownText", QML)
        self.assertIn("releaseNotesDialog.notesText", QML)
        self.assertIn('cp "${ROOT_DIR}/docs/NOVEDADES.md" "${STAGE_DIR}/help/NOVEDADES.md"', USER_INSTALLER)

    @unittest.skipUnless(shutil.which("node"), "Node no instalado")
    def test_qr_and_certificate_rules(self) -> None:
        funcs = "\n".join(qml_function(name).replace("    function ", "function ", 1)
                          for name in ("validQrUrl", "certificateExpiry", "canRenewFnmt",
                                       "certificateCanSign", "certificateStatusText",
                                       "certificateStatusReason", "certificateStatusColorForBackground",
                                       "certificateStatusColor",
                                       "rotationIndexForSeal"))
        js = "const assert = require('node:assert/strict');\nfunction tr(s) { return s; }\nString.prototype.arg = function(v) { return this.replace('%1', v); };\nconst currentTheme = {cardColor:'#ffffff'};\n" + funcs + "\n" + """
assert.equal(validQrUrl('https://sede.ejemplo.es/verificar'), true);
assert.equal(validQrUrl('http://sede.ejemplo.es/verificar'), false);
assert.equal(validQrUrl('https://usuario@sede.ejemplo.es'), false);
assert.equal(validQrUrl('https://:443'), false);
assert.equal(validQrUrl('https://sede.ejemplo.es:abc'), false);
assert.equal(validQrUrl('https://sede.ejemplo.es:65536'), false);
assert.equal(validQrUrl('https://sede..ejemplo.es'), false);
assert.equal(validQrUrl('https://sede.ejemplo.es\\\\atacante.es'), false);
assert.equal(certificateExpiry({validTo:'2026-12-31T23:00:00Z'}), '31/12/2026');
const base = {canSign:true, caducado:false, validTo:'2026-12-31', issuer:'FNMT-RCM', tipo:'fisica', status:'Válido'};
assert.equal(canRenewFnmt({...base, diasCaducidad:60}), true);
assert.equal(canRenewFnmt({...base, diasCaducidad:61}), false);
assert.equal(canRenewFnmt({...base, issuer:'Otra CA', diasCaducidad:30}), false);
assert.equal(canRenewFnmt({...base, status:'Revocado', diasCaducidad:30}), false);
assert.equal(certificateStatusText({...base, diasCaducidad:60}), 'Caduca pronto');
assert.equal(certificateStatusColor({...base, diasCaducidad:60}), '#5c3900');
assert.equal(certificateStatusText({...base, diasCaducidad:61}), 'Válido');
assert.equal(certificateStatusColor({...base, diasCaducidad:61}), '#064c2a');
assert.equal(certificateStatusText({...base, caducado:true}), '⚠ No válido');
assert.equal(certificateStatusReason({...base, caducado:true}), 'Caducado el 31/12/2026');
assert.equal(certificateStatusColor({...base, caducado:true}), '#750010');
assert.equal(certificateStatusText({...base, status:'Revocado'}), '⚠ No válido');
assert.equal(certificateStatusReason({...base, status:'Revocado'}), 'Revocado');
assert.equal(certificateStatusColor({...base, status:'Revocado'}), '#750010');
let signSealRotation = 45;
assert.equal(rotationIndexForSeal(), -1);
signSealRotation = 90;
assert.equal(rotationIndexForSeal(), 1);
"""
        completed = subprocess.run(["node", "-e", js], capture_output=True,
                                   text=True, timeout=10, check=False)
        self.assertEqual(completed.returncode, 0, completed.stderr)


if __name__ == "__main__":
    unittest.main()
