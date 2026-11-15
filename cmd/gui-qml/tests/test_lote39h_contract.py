# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 39h en Qt: el diálogo de Verificar ofrece todo lo que el motor
verifica y las evidencias de la tarjeta de resultado no enseñan claves
técnicas como «signing_certificate=ausente» o «algoritmo_antiguo=sha1»."""

import json
import re
import shutil
import subprocess
import unittest
from pathlib import Path

from test_recorrido_118_contract import qml_function

ROOT = Path(__file__).resolve().parents[3]
MAIN = ROOT / "cmd/gui-qml/qml/main.qml"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
LANGUAGES = ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh")

# Lo que el motor verifica de verdad (lote 39h): PDF, CAdES (.csig, .p7s y
# los DER .p7m y .sig), XAdES y FacturaE, ASiC-S y los ofimáticos ODF y OOXML.
VERIFIED = ("*.pdf", "*.csig", "*.p7s", "*.p7m", "*.sig", "*.xsig", "*.xml", "*.dsig",
            "*.xmlsig", "*.asics", "*.odt", "*.ods", "*.odp", "*.docx", "*.xlsx", "*.pptx")


def catalog(language):
    return json.loads((LOCALES / (language + ".json")).read_text(encoding="utf-8"))


def verify_dialog_filter(source):
    start = source.index("id: verifyFileDialog")
    match = re.search(r'nameFilters: \[tr\("([^"]+)"\)', source[start:start + 600])
    if not match:
        raise AssertionError("verifyFileDialog sin filtro")
    return match.group(1)


class VerifyFilterContract(unittest.TestCase):
    def test_filter_offers_every_format_the_engine_verifies(self):
        key = verify_dialog_filter(MAIN.read_text(encoding="utf-8"))
        patterns = re.search(r"\(([^)]*)\)", key).group(1).split()
        for pattern in VERIFIED:
            self.assertIn(pattern, patterns)
        # ASiC-E no se verifica: ofrecerlo llevaría a un error seguro.
        self.assertNotIn("*.asice", patterns)

    def test_filter_is_translated_with_the_same_patterns(self):
        key = verify_dialog_filter(MAIN.read_text(encoding="utf-8"))
        patterns = re.search(r"\(([^)]*)\)", key).group(1)
        for language in LANGUAGES:
            with self.subTest(language=language):
                value = catalog(language).get(key)
                self.assertIsNotNone(value)
                self.assertIn("(" + patterns + ")", value)


class EvidenceSummaryContract(unittest.TestCase):
    def run_summary(self, language, summaries):
        node = shutil.which("node")
        if not node:
            self.skipTest("Node no instalado")
        source = MAIN.read_text(encoding="utf-8")
        js = "const CATALOG = " + json.dumps(catalog(language), ensure_ascii=False) + ";\n" + r"""
function tr(key) { return Object.prototype.hasOwnProperty.call(CATALOG, key) ? CATALOG[key] : key; }
function localizeVisibleDiagnosticText(text) { return text; }
function catalogFormat(key, values) {
    let text = tr(key);
    for (let i = 0; i < values.length; i++) text = text.split("%" + (i + 1)).join(values[i]);
    return text;
}
""" + qml_function(source, "verificationEvidenceSummaryText").strip() + "\n" + \
            "process.stdout.write(JSON.stringify(" + json.dumps(summaries) + \
            ".map(verificationEvidenceSummaryText)));\n"
        result = subprocess.run([node], input=js, text=True, capture_output=True,
                                check=True, cwd=ROOT, timeout=15)
        return json.loads(result.stdout)

    def test_technical_keys_become_catalogue_text(self):
        es = catalog("es")
        shown = self.run_summary("es", ["signing_certificate=ausente", "algoritmo_antiguo=sha1",
                                        "clave_inventada=valor", "texto libre"])
        self.assertEqual(shown[0], es["verificacion.detalle.valor.ausente"])
        self.assertEqual(shown[1], es["verificacion.detalle.valor.sha1"])
        # Sin entrada en el catálogo, tal cual: no se inventa un rótulo.
        self.assertEqual(shown[2], "clave_inventada=valor")
        self.assertEqual(shown[3], "texto libre")

    def test_every_language_has_the_new_labels(self):
        for language in LANGUAGES:
            with self.subTest(language=language):
                data = catalog(language)
                for key in ("verificacion.detalle.signing_certificate",
                            "verificacion.detalle.valor.ausente"):
                    self.assertTrue(data.get(key, "").strip(), key)
                shown = self.run_summary(language, ["signing_certificate=ausente"])
                self.assertNotIn("=", shown[0])

    def test_evidence_list_uses_the_summary_text(self):
        source = MAIN.read_text(encoding="utf-8")
        self.assertIn("verificationEvidenceSummaryText(item.summary)",
                      qml_function(source, "verificationEvidenceText"))


def qml_block(source, marker):
    start = source.index(marker)
    opening = source.rindex("{", 0, start)
    depth = 0
    for index in range(opening, len(source)):
        depth += (source[index] == "{") - (source[index] == "}")
        if depth == 0:
            return source[opening:index + 1]
    raise AssertionError(marker)


class TwoHundredPercentContract(unittest.TestCase):
    """Recorrido I: al 200 % el menú lateral se cortaba, la lista del tamaño de
    texto se quedaba pintada y la barra de estado partía el mensaje."""

    def test_sidebar_buttons_do_not_double_their_minimum_height(self):
        source = MAIN.read_text(encoding="utf-8")
        nav = source[source.index("component NavButton"):]
        nav = nav[:nav.index("RowLayout {")]
        self.assertNotIn("Typography.minimum(54)", nav)
        self.assertIn("implicitHeight: Math.max(54, Typography.minimum(36), navLabel.implicitHeight + 20)", nav)
        self.assertIn("navigationScroll.reveal(navButtonRoot)", nav)

    def test_sidebar_shows_its_scroll_bar_when_it_does_not_fit(self):
        scroll = qml_block(MAIN.read_text(encoding="utf-8"), "id: navigationScroll")
        self.assertIn("ScrollBar.vertical.policy: overflowing ? ScrollBar.AlwaysOn : ScrollBar.AsNeeded", scroll)
        self.assertIn("function reveal(item)", scroll)

    def test_text_size_is_applied_after_the_list_closes(self):
        combo = qml_block(MAIN.read_text(encoding="utf-8"), "id: textSizeCombo")
        self.assertNotIn("onActivated: function(index) { appSettings.textScalePercent", combo)
        self.assertIn("if (!popup.visible) textSizeApply.restart()", combo)
        self.assertIn("function onClosed() { if (textSizeCombo.pendingPercent > 0) textSizeApply.restart() }", combo)
        self.assertIn("textSizeCombo.forceActiveFocus(", combo)

    def test_status_bar_is_one_line(self):
        source = MAIN.read_text(encoding="utf-8")
        status = qml_block(source, "id: statusMessageText")
        self.assertIn("text: window.singleLineText(localizedStatusMessage())", status)
        self.assertIn("maximumLineCount: 1", status)
        self.assertIn("wrapMode: Text.NoWrap", status)
        node = shutil.which("node")
        if not node:
            self.skipTest("Node no instalado")
        js = qml_function(source, "singleLineText").strip() + r"""
const assert = require('assert');
assert.strictEqual(singleLineText("Firma no válida: SHA-1 ya no basta.\nLa organización no lo permite."),
                   "Firma no válida: SHA-1 ya no basta. La organización no lo permite.");
assert.strictEqual(singleLineText("a\r\n\r\n  b"), "a b");
assert.strictEqual(singleLineText(""), "");
"""
        subprocess.run([node], input=js, text=True, check=True, cwd=ROOT, timeout=15)


if __name__ == "__main__":
    unittest.main()
