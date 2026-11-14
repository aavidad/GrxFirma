#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 17b.

Punto 1 (KD-03): antes de cofirmar o contrafirmar una CAdES que no lleva el
documento dentro, Qt pregunta al motor y ofrece elegir el documento original
(camino recomendado) o firmar de todos modos; el original llega al motor en
originalPath.

Punto 3d: el puente REST recibe los avisos de la firma y cada documento del
lote enseña los suyos sin perder el resumen del lote.
"""

import json
import re
import unittest
from pathlib import Path

from test_recorrido_118_contract import qml_function
from test_sign_warnings_contract import cpp_block

ROOT = Path(__file__).resolve().parents[3]
QT = ROOT / "cmd/gui-qml"
MAIN = (QT / "qml/main.qml").read_text(encoding="utf-8")
BRIDGE = (QT / "ipcbridge.cpp").read_text(encoding="utf-8")
HEADER = (QT / "ipcbridge.h").read_text(encoding="utf-8")
REST = (QT / "backendbridge.cpp").read_text(encoding="utf-8")
REST_HEADER = (QT / "backendbridge.h").read_text(encoding="utf-8")
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
LANGUAGES = ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh")
DIALOG_KEYS = (
    "firma.cofirma_sin_original.confirmar_titulo",
    "firma.cofirma_sin_original.confirmar_texto",
    "firma.cofirma_sin_original.elegir_original",
    "firma.cofirma_sin_original.firmar_igualmente",
)


def qml_block(source, marker):
    start = source.index(marker)
    depth = 0
    for index in range(source.index("{", start), len(source)):
        depth += (source[index] == "{") - (source[index] == "}")
        if depth == 0:
            return source[start:index + 1]
    raise AssertionError(marker)


class CosignOriginalContract(unittest.TestCase):
    def test_el_puente_ipc_pregunta_y_pasa_el_original(self):
        self.assertIn("Q_INVOKABLE void checkCosignOriginal(const QString &inputPath,", HEADER)
        self.assertIn("void cosignOriginalChecked(bool ok, bool needsOriginal);", HEADER)
        check = cpp_block(BRIDGE, "void IpcBridge::checkCosignOriginal(")
        self.assertIn('QStringLiteral("cosign_original_check")', check)
        for field in ("inputPath", "format", "action"):
            self.assertIn('QStringLiteral("%s")' % field, check)
        reply = BRIDGE[BRIDGE.index('if (action == QStringLiteral("cosign_original_check")) {'):]
        reply = reply[:reply.index("continue;")]
        self.assertIn('ok && data.toObject().value(QStringLiteral("needsOriginal")).toBool()', reply)
        # Sin conexión, QML recibe la respuesta y firma como siempre.
        self.assertIn('} else if (action == "cosign_original_check") {\n    emit cosignOriginalChecked(false, false);', BRIDGE)
        sign = cpp_block(BRIDGE, "void IpcBridge::signFileAdvanced(")
        self.assertIn('options.value("originalPath").toString()', sign)
        self.assertLess(sign.index('params["originalPath"] = originalPath;'), sign.index("sendRequest("))

    def test_qml_pregunta_antes_de_firmar_y_gasta_la_eleccion(self):
        execute = qml_function(MAIN, "executeSignRequest")
        check = execute.index("backend.checkCosignOriginal(currentFilePath")
        self.assertLess(execute.index("if (window.pendingCosignOriginalContext !== null)"), check)
        self.assertLess(execute.index("!skipCosignOriginalCheckOnce && window.needsCosignOriginalCheck("), check)
        # Antes del sello (que puede volver a entrar) y de empezar a firmar.
        self.assertLess(check, execute.index("shouldPrecheckSealSignerSummary(payload)"))
        spend = execute.index("const cosignOriginal = skipCosignOriginalCheckOnce ? cosignOriginalPathOnce : \"\"")
        self.assertLess(spend, execute.index("payload.originalPath = cosignOriginal"))
        self.assertLess(execute.index("payload.originalPath = cosignOriginal"), execute.index("window.signingInProgress = true"))
        self.assertIn("resetCosignOriginalChoice()", execute[spend:execute.index("window.signingInProgress = true")])
        needs = qml_function(MAIN, "needsCosignOriginalCheck")
        for condition in ("isIpcMode", "!window.isBatchMode()", "!window.multiCosignEnabled",
                          "window.isCosignOrCountersign(action)"):
            self.assertIn(condition, needs)
        self.assertIn('action === "cosign" || action === "countersign"', qml_function(MAIN, "isCosignOrCountersign"))
        self.assertIn("resetCosignOriginalChoice()", MAIN[MAIN.index("onCurrentFilePathChanged: {"):][:200])

    def test_respuesta_del_motor(self):
        handler = qml_function(MAIN, "onCosignOriginalChecked")
        self.assertIn("ctx.path !== window.currentFilePath", handler)
        self.assertLess(handler.index("if (ok && needsOriginal) {"), handler.index("cosignOriginalDialog.open()"))
        # Si no hace falta, o el motor no sabe responder, se firma como siempre.
        self.assertIn('window.resumeSignWithCosignChoice(ctx.selectedCertIndex, "")', handler)
        resume = qml_function(MAIN, "resumeSignWithCosignChoice")
        self.assertIn("skipCosignOriginalCheckOnce = true", resume)
        self.assertIn("cosignOriginalPathOnce = originalPath", resume)

    def test_cuadro_en_lenguaje_llano_y_accesible(self):
        dialog = qml_block(MAIN, "ThemedDialog {\n        id: cosignOriginalDialog")
        self.assertIn('title: tr("firma.cofirma_sin_original.confirmar_titulo")', dialog)
        self.assertIn('accessibleDescription: tr("firma.cofirma_sin_original.confirmar_texto")', dialog)
        self.assertIn("closePolicy: Popup.CloseOnEscape", dialog)
        self.assertIn("onOpened: cosignOriginalChooseButton.forceActiveFocus()", dialog)
        self.assertEqual(dialog.count("Text {"), dialog.count("textFormat: Text.PlainText"))
        choose = dialog.index('text: tr("firma.cofirma_sin_original.elegir_original")')
        anyway = dialog.index('text: tr("firma.cofirma_sin_original.firmar_igualmente")')
        cancel = dialog.index('text: tr("Cancelar")')
        # El camino recomendado va primero, resaltado y con el foco.
        self.assertLess(choose, anyway)
        self.assertLess(anyway, cancel)
        self.assertIn("highlighted: true", dialog[choose - 200:anyway])
        self.assertIn("cosignOriginalFileDialog.open()", dialog[choose:anyway])
        self.assertIn('window.resumeSignWithCosignChoice(index, "")', dialog[anyway:cancel])
        files = qml_block(MAIN, "FileDialog {\n        acceptLabel: tr(\"Abrir\")\n        rejectLabel: tr(\"Cancelar\")\n        id: cosignOriginalFileDialog")
        self.assertIn("window.resumeSignWithCosignChoice(cosignOriginalDialog.selectedCertIndex, original)", files)
        restart = MAIN[MAIN.index("property bool restartBlocked:"):][:3000]
        self.assertIn("cosignOriginalDialog.visible || cosignOriginalFileDialog.visible", restart)

    def test_textos_en_los_once_idiomas(self):
        texts = {}
        for language in LANGUAGES:
            catalog = json.loads((LOCALES / (language + ".json")).read_text(encoding="utf-8"))
            for key in DIALOG_KEYS + ("Cancelar", "Abrir", "Advertencias"):
                self.assertTrue(catalog.get(key), (language, key))
            texts[language] = catalog[DIALOG_KEYS[1]]
        # Traducidos de verdad: ningún idioma copia el castellano.
        for language in LANGUAGES[1:]:
            self.assertNotEqual(texts[language], texts["es"], language)
        self.assertIn("documento original", texts["es"])


class RestAndBatchWarningsContract(unittest.TestCase):
    def test_el_puente_rest_lee_los_avisos(self):
        self.assertIn("return m_lastSigningWarnings;", REST_HEADER)
        self.assertNotIn("lastSigningWarnings() const { return {}; }", REST_HEADER)
        helper = cpp_block(REST, "static QStringList backendSigningWarnings(")
        ipc = cpp_block(BRIDGE, "static QStringList ipcSigningWarnings(")
        pattern = re.compile(r'QStringLiteral\("(\^\[a-z0-9_\][^"]*)"\)')
        self.assertEqual(pattern.search(helper).group(1), pattern.search(ipc).group(1))
        self.assertIn("text.size() > 128", helper)
        self.assertIn("kBackendMaxSigningWarnings = 16", REST)
        for function in ("signFileAdvanced", "signFileMultiAdvanced", "signBatchAdvanced"):
            body = cpp_block(REST, "void BackendBridge::" + function + "(")
            self.assertIn("m_lastSigningWarnings.clear();", body[:400], function)
        single = cpp_block(REST, "void BackendBridge::signFileAdvanced(")
        self.assertLess(single.index("m_lastSigningWarnings = backendSigningWarnings(obj);"),
                        single.index("emit signingFinished(true"))
        batch = cpp_block(REST, "void BackendBridge::signBatchAdvanced(")
        self.assertIn('result.insert(QStringLiteral("warnings"),\n                    backendSigningWarnings(response));', batch)
        self.assertLess(batch.index("m_lastSigningWarnings = backendSigningWarnings(obj);"),
                        batch.index("emit batchSigningFinished(allOK, msg, results);"))

    def test_cada_documento_del_lote_lleva_sus_avisos(self):
        batch = BRIDGE[BRIDGE.index('if (action == "sign_batch") {\n      QJsonObject res'):]
        batch = batch[:batch.index("continue;")]
        self.assertIn("ipcSigningWarnings(v.toObject())", batch)
        self.assertIn("ipcSigningWarnings(res)", batch)
        repeater = MAIN[MAIN.index("model: window.currentBatchResults"):]
        repeater = repeater[:repeater.index("AdaptiveRow {")]
        self.assertIn("warnings: modelData.ok ? window.visibleSigningWarnings(modelData.warnings) : []", repeater)
        # El resumen del lote sigue arriba.
        self.assertIn("warnings: window.batchSigningWarnings", MAIN)


if __name__ == "__main__":
    unittest.main()
