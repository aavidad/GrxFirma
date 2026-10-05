# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Detalles de la prueba final de Linux 0.0.118: nombre propuesto en los
diálogos de guardar, nombres de los informes, cambios sin guardar, tipos de
evidencia traducidos, caja de arrastre en francés y términos franceses."""

import json
import re
import shutil
import subprocess
import unittest
from pathlib import Path

from test_recorrido_118_contract import qml_function

ROOT = Path(__file__).resolve().parents[3]
QML_DIR = ROOT / "cmd/gui-qml/qml"
MAIN = QML_DIR / "main.qml"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
LANGUAGES = ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh")


def catalog(language):
    return json.loads((LOCALES / (language + ".json")).read_text(encoding="utf-8"))


def file_dialog_blocks(source):
    for match in re.finditer(r"FileDialog\s*\{", source):
        depth = 0
        for index in range(match.end() - 1, len(source)):
            depth += (source[index] == "{") - (source[index] == "}")
            if depth == 0:
                yield source[match.start():index + 1]
                break


class SaveDialogProposedNameContract(unittest.TestCase):
    def test_every_save_dialog_shows_the_proposed_name(self):
        found = 0
        for path in sorted(QML_DIR.glob("*.qml")):
            for block in file_dialog_blocks(path.read_text(encoding="utf-8")):
                if not re.search(r"fileMode:\s*FileDialog\.SaveFile", block):
                    continue
                found += 1
                with self.subTest(file=path.name, dialog=block[:120]):
                    self.assertRegex(block, r"onVisibleChanged:.*(showProposedSaveName|showProposedNames)")
        self.assertGreaterEqual(found, 10)

    def test_helper_is_registered_and_built(self):
        main_cpp = (ROOT / "cmd/gui-qml/main.cpp").read_text(encoding="utf-8")
        project = (ROOT / "cmd/gui-qml/grxfirma_qt.pro").read_text(encoding="utf-8")
        self.assertIn('setContextProperty("saveDialogNames"', main_cpp)
        self.assertIn("savedialognames.h", project)
        source = MAIN.read_text(encoding="utf-8")
        self.assertIn('typeof saveDialogNames !== "undefined"', qml_function(source, "showProposedSaveName"))


class ReportFileNamesContract(unittest.TestCase):
    def test_html_and_summary_share_the_naming_convention(self):
        for language in LANGUAGES:
            values = catalog(language)
            with self.subTest(language=language):
                self.assertNotIn("-", values["winui.parity.verify.filename"])
                self.assertNotIn("-", values["verificacion.resumen.nombre_fichero"])

    def test_html_report_name_starts_with_the_document(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("Node no instalado")
        source = MAIN.read_text(encoding="utf-8")
        names = ("basename", "fileStem", "suggestVerificationHtmlReportName",
                 "suggestVerificationSummaryName")
        js = "\n".join(qml_function(source, name).strip() for name in names)
        js = """
const names = {"winui.parity.verify.filename": "informe_verificacion",
               "verificacion.resumen.nombre_fichero": "resumen_validacion"};
function tr(key) { return names[key] || key; }
""" + js + r"""
const assert = require('assert');
assert.strictEqual(suggestVerificationHtmlReportName("/tmp/contrato_firmado.pdf"),
    "contrato_firmado_informe_verificacion.html");
assert.strictEqual(suggestVerificationSummaryName("/tmp/contrato_firmado.pdf"),
    "contrato_firmado_resumen_validacion.txt");
assert.strictEqual(suggestVerificationHtmlReportName(""), "informe_verificacion.html");
"""
        subprocess.run([node], input=js, text=True, check=True, cwd=ROOT, timeout=15)


class UnsavedSettingsContract(unittest.TestCase):
    def test_dirty_flag_compares_with_the_saved_preferences(self):
        source = MAIN.read_text(encoding="utf-8")
        mark = qml_function(source, "markBackendSettingsDirty")
        self.assertIn("backendSettingsSignature() !== savedSettingsSignature", mark)
        signature = qml_function(source, "backendSettingsSignature")
        self.assertIn("multiCosignPrimaryCertificateId = \"\"", signature)
        self.assertIn("backendSettingsPayload()", qml_function(source, "saveBackendSettings"))
        self.assertGreaterEqual(source.count("window.rememberSavedSettings()"), 2)

    def test_signature_ignores_reassigning_the_same_value(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("Node no instalado")
        source = MAIN.read_text(encoding="utf-8")
        js = qml_function(source, "backendSettingsSignature").strip()
        js = """
let payload = {multiCosignEnabled: false, multiCosignPrimaryCertificateId: "a", signSealPlacements: {}};
function backendSettingsPayload() { return JSON.parse(JSON.stringify(payload)); }
""" + js + r"""
const assert = require('assert');
const saved = backendSettingsSignature();
payload.multiCosignPrimaryCertificateId = "otro";
payload.signSealPlacements = {};
assert.strictEqual(backendSettingsSignature(), saved);
payload.multiCosignEnabled = true;
assert.notStrictEqual(backendSettingsSignature(), saved);
"""
        subprocess.run([node], input=js, text=True, check=True, cwd=ROOT, timeout=15)


class VerificationTextsContract(unittest.TestCase):
    KEYS = ("report.evidence.certificate.subject", "report.evidence.certificate.issuer",
            "verificacion.detalle.error_cadena",
            "verificacion.detalle.valor.x509_autoridad_desconocida",
            "verificacion.detalle.valor.x509_caducado",
            "verificacion.detalle.valor.x509_uso_no_permitido",
            "verificacion.detalle.valor.x509_nombre_no_coincide")

    def test_evidence_types_are_translated_in_the_interface(self):
        source = MAIN.read_text(encoding="utf-8")
        self.assertIn("verificationEvidenceTypeText(item.type)", qml_function(source, "verificationEvidenceText"))
        self.assertIn('"report.evidence."', qml_function(source, "verificationEvidenceTypeText"))

    def test_new_texts_exist_in_every_language(self):
        spanish = catalog("es")
        for language in LANGUAGES:
            values = catalog(language)
            for key in self.KEYS:
                with self.subTest(language=language, key=key):
                    self.assertTrue(values.get(key, "").strip())
                    if language != "es":
                        self.assertNotEqual(values[key], spanish[key])


class FrenchLayoutAndTermsContract(unittest.TestCase):
    def test_drop_zone_text_takes_the_card_width(self):
        source = MAIN.read_text(encoding="utf-8")
        start = source.index("text: selectedInputsSummary()")
        column = source[source.rindex("ColumnLayout {", 0, start):start + 900]
        self.assertIn("width: Math.max(0, parent.width - 40)", column)
        self.assertIn("Layout.fillWidth: true", column)
        self.assertIn("centered: true", column)

    def test_visible_seal_is_always_cachet(self):
        for key, value in catalog("fr").items():
            with self.subTest(key=key):
                self.assertNotRegex(value, r"(?i)\bsceaux?\b")

    def test_buttons_use_the_infinitive(self):
        values = catalog("fr")
        for key in ("Seleccionar varios", "Seleccionar varios documentos", "Seleccionar fichero...",
                    "Seleccionar documento firmado", "Seleccionar imagen de firma",
                    "Seleccionar certificado personal (.p12, .pfx)"):
            with self.subTest(key=key):
                self.assertTrue(values[key].startswith("Sélectionner"), values[key])


if __name__ == "__main__":
    unittest.main()
