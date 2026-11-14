#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 17b, contrato estático de Kotlin (sin Gradle en esta máquina).

Punto 1 (KD-03): el lote de Android pide confirmación antes de cofirmar y
envía confirm_unseen_content; el aviso de cada elemento se mantiene.
Punto 3b: Android lee «warnings» al firmar, en el lote y en el ENI, y los
enseña tras la operación con el texto del catálogo empaquetado.
"""

import json
import re
import unittest
from pathlib import Path
from xml.etree import ElementTree

ROOT = Path(__file__).resolve().parents[4]
APP = ROOT / "mobile/android/app/src"
KT = APP / "main/java/io/github/aavidad/grxfirma/android"
RES = APP / "main/res"
ASSETS = APP / "main/assets/locales"
FOLDERS = ("values", "values-en", "values-ca", "values-b+ca+ES+valencia", "values-gl", "values-eu",
           "values-fr", "values-de", "values-it", "values-pt", "values-zh")
LANGUAGES = ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh")
ENGINE_WARNINGS = ("firma.cofirma_sin_original.aviso", "firma.sha1_corresponde.aviso",
                   "firma.lt.crl_no_aplicable.aviso")


def read(path):
    return (KT / path).read_text(encoding="utf-8")


def block(source, marker):
    """Cuerpo de una función de Kotlin: desde el marcador hasta la llave que
    cierra la primera «) {» de fin de línea (no la de un valor por defecto)."""
    start = source.index(marker)
    depth = 0
    for index in range(source.index(") {\n", start) + 2, len(source)):
        depth += (source[index] == "{") - (source[index] == "}")
        if depth == 0:
            return source[start:index + 1]
    raise AssertionError(marker)


class AndroidCosignConfirmation(unittest.TestCase):
    def test_la_pantalla_pide_confirmacion_antes_de_cofirmar_el_lote(self):
        screen = read("ui/Wave4Screen.kt")
        sign = block(screen, "fun signBatch(format: String, sealPlanner: BatchSealPlanner?)")
        self.assertIn('state.wave4.batchAction != "cosign"', sign)
        self.assertIn("startBatch(format, sealPlanner, confirmUnseenContent = false)", sign)
        self.assertIn("MaterialAlertDialogBuilder(activity)", sign)
        self.assertIn(".setTitle(R.string.batch_cosign_confirm_title)", sign)
        self.assertIn(".setMessage(R.string.batch_cosign_confirm_message)", sign)
        self.assertIn(".setNegativeButton(android.R.string.cancel, null)", sign)
        positive = sign[sign.index(".setPositiveButton(R.string.batch_cosign_confirm_action)"):]
        self.assertIn("startBatch(format, sealPlanner, confirmUnseenContent = true)", positive)
        start = block(screen, "private fun startBatch(")
        self.assertIn("confirmUnseenContent = confirmUnseenContent", start)
        self.assertIn("dnie.withPin(hold = true)", start)

    def test_la_confirmacion_llega_al_motor(self):
        vm = read("ui/MainViewModel.kt")
        self.assertIn("confirmUnseenContent: Boolean = false,", block(vm, "fun signBatch("))
        self.assertIn('core.signBatchItems(items, certificate.id, configured, confirmUnseenContent && action == "cosign")', vm)
        bridge = read("core/CoreBridge.kt")
        self.assertIn("confirmUnseenContent: Boolean = false,", bridge[bridge.index("fun signBatchItems("):][:300])
        reflective = read("core/ReflectiveGomobileBridge.kt")
        self.assertIn("Wave4Codec.batchItemsRequest(items, certificateId, options, confirmUnseenContent)", reflective)
        codec = read("core/Wave4Codec.kt")
        self.assertIn('if (confirmUnseenContent) put("confirm_unseen_content", true)', codec)
        go = (ROOT / "mobilebind/batch.go").read_text(encoding="utf-8")
        self.assertIn('json:"confirm_unseen_content,omitempty"', go)

    def test_textos_del_cuadro_en_los_once_idiomas(self):
        for folder in FOLDERS:
            strings = {node.attrib["name"]: node.text or "" for node in
                       ElementTree.parse(RES / folder / "strings.xml").getroot() if node.tag == "string"}
            for name in ("batch_cosign_confirm_title", "batch_cosign_confirm_message", "batch_cosign_confirm_action"):
                self.assertTrue(strings.get(name), (folder, name))
            # El cuadro remite al botón real para ver el documento.
            self.assertIn(strings["select_original_document"], strings["batch_cosign_confirm_message"], folder)


class AndroidSigningWarnings(unittest.TestCase):
    def test_los_contratos_leen_warnings(self):
        models = read("model/AppModels.kt")
        self.assertIn("val warnings: List<String> = emptyList(),", models[models.index("data class SignedOutput("):][:400])
        self.assertIn("val warnings: List<String> = emptyList(),", models[models.index("data class EniDocument("):][:400])
        codec = read("core/CoreJsonCodec.kt")
        self.assertIn("SignedOutput(bytes, outputName(base, extension), mime, format, algorithm, warningKeys(json))", codec)
        self.assertIn("warningKeys(json),", block(codec, "fun parseEniDocument("))
        helper = codec[codec.index("fun warningKeys(json: JSONObject)"):][:400]
        for rule in ('optJSONArray("warnings")', "it.length <= 128", "WARNING_KEY.matches(it)", ".distinct()", ".take(16)"):
            self.assertIn(rule, helper)
        for go_struct in ("type signResponse struct", "type mobileBatchItemResponse struct", "type eniDocumentResponse struct"):
            sources = "".join(p.read_text(encoding="utf-8") for p in (ROOT / "mobilebind").glob("*.go") if not p.name.endswith("_test.go"))
            if go_struct in sources:
                body = sources[sources.index(go_struct):]
                self.assertIn('json:"warnings,omitempty"', body[:body.index("\n}\n")], go_struct)

    def test_la_pantalla_los_ensena_tras_firmar(self):
        vm = read("ui/MainViewModel.kt")
        lines = vm[vm.index("private fun warningLines("):vm.index("private fun withWarnings(")]
        self.assertIn("warnings.filter(EngineKeys::isClosed).distinct()", lines)
        self.assertIn("UiText.Resource(R.string.verification_warning, listOf(UiText.Engine(key)))", lines)
        self.assertIn("UiText.Resource(R.string.issue_line, listOf(source, text))", lines)
        self.assertIn("output.warnings,", block(vm, "fun sign(format: String"))
        self.assertIn("addAll(warningLines(it.output?.warnings.orEmpty(), it.sourceName))", block(vm, "fun signBatch("))
        eni = block(vm, "fun createEni(request: EniRequest)")
        self.assertEqual(2, eni.count("created.warnings"))

    def test_las_claves_de_los_avisos_estan_cerradas_y_empaquetadas(self):
        keys = read("ui/EngineKeys.kt")
        pattern = re.search(r'CATALOGUE_KEY = Regex\("""(.+?)"""\)', keys).group(1)
        for key in ENGINE_WARNINGS:
            self.assertTrue(re.fullmatch(pattern, key), key)
        self.assertFalse(re.fullmatch(pattern, "firma.otra.aviso"))
        script = (ROOT / "scripts/mobile/android/sync_verification_locales.py").read_text(encoding="utf-8")
        for prefix in ("firma.cofirma_sin_original.", "firma.sha1_corresponde.", "firma.lt.crl_no_aplicable."):
            self.assertIn('"%s"' % prefix, script)
        desktop = ROOT / "internal/adapters/outbound/common/localizador/locales"
        for language in LANGUAGES:
            asset = json.loads((ASSETS / (language + ".json")).read_text(encoding="utf-8"))
            catalog = json.loads((desktop / (language + ".json")).read_text(encoding="utf-8"))
            for key in [k for k in catalog if k.startswith(("firma.sha1_corresponde.", "firma.cofirma_sin_original."))] + list(ENGINE_WARNINGS):
                self.assertEqual(asset.get(key), catalog.get(key), (language, key))


if __name__ == "__main__":
    unittest.main()
