# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Fallos del recorrido de la aplicación Linux 0.0.118: guardar el resumen
de validación sin carpeta, botones estándar de Qt en el idioma del sistema,
una llamada a una función inexistente en la confirmación de firma y la
doble puntuación del aviso tras firmar."""

import json
import re
import shutil
import subprocess
import unittest
from pathlib import Path

from test_dialog_theme_contract import dialog_blocks, direct_members

ROOT = Path(__file__).resolve().parents[3]
QML_DIR = ROOT / "cmd/gui-qml/qml"
MAIN = QML_DIR / "main.qml"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
LANGUAGES = ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh")

CODE_NOISE = re.compile(
    r'"(?:\\.|[^"\\\n])*"|\'(?:\\.|[^\'\\\n])*\'|//[^\n]*|/\*.*?\*/', re.S)
REGEX_LITERAL = re.compile(
    r'([(=!,:&|?]\s*)/(?:\\.|\[(?:\\.|[^\]\n])*\]|[^/\n\\])+/[gimsuy]*')
# Globales de JavaScript, métodos de los propios objetos QML y propiedades
# que guardan funciones: no se declaran con «function» en el fichero.
KNOWN_CALLS = {
    "decodeURIComponent", "encodeURIComponent", "isNaN", "isFinite",
    "parseInt", "parseFloat", "forceActiveFocus", "mapToItem", "getContext",
    "load", "stop", "standardButton", "translate", "validation",
}
KEYWORDS = {"if", "for", "while", "switch", "return", "function", "catch",
            "typeof", "new", "in", "of", "delete", "void"}


def code_of(text):
    code = CODE_NOISE.sub('""', text)
    return REGEX_LITERAL.sub(lambda m: m.group(1) + '""', code)


def qml_function(source, name):
    start = source.index("    function " + name + "(")
    depth = 0
    for index in range(source.index("{", start), len(source)):
        depth += (source[index] == "{") - (source[index] == "}")
        if depth == 0:
            return source[start:index + 1]
    raise AssertionError(name)


class SummarySaveDialogContract(unittest.TestCase):
    def test_no_file_url_is_built_without_folder_or_encoding(self):
        source = MAIN.read_text(encoding="utf-8")
        self.assertNotIn('"file://" + candidate', source)
        self.assertNotIn('currentFile: "file://"', source)
        self.assertIn("window.openSaveDialog(verifySummarySaveDialog", source)

    def test_summary_path_uses_document_folder_or_documents(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("Node no instalado")
        source = MAIN.read_text(encoding="utf-8")
        names = ("localPathFromUrl", "basename", "dirname", "fileStem",
                 "fileUrlFromLocalPath", "suggestedSaveFolder", "suggestedSaveUrl",
                 "suggestVerificationSummaryName", "suggestVerificationSummaryPath")
        js = "\n".join(qml_function(source, name).strip() for name in names)
        js = """
const Qt = {platform: {os: "linux"}};
const documentsFolderPath = "/home/persona/Documentos";
function tr(key) { return key === "verificacion.resumen.nombre_fichero" ? "resumen_validacion" : key; }
""" + js + r"""
const assert = require('assert');
assert.strictEqual(suggestVerificationSummaryPath("/home/persona/Mis firmas/acta #3.pdf"),
    "file:///home/persona/Mis%20firmas/acta%20%233_resumen_validacion.txt");
assert.strictEqual(suggestVerificationSummaryPath(""),
    "file:///home/persona/Documentos/resumen_validacion.txt");
assert.strictEqual(localPathFromUrl(suggestVerificationSummaryPath("/tmp/año 100%.pdf")),
    "/tmp/año 100%_resumen_validacion.txt");
assert.strictEqual(localPathFromUrl("file:///tmp/a b.txt"), "/tmp/a b.txt");
"""
        subprocess.run([node], input=js, text=True, check=True, cwd=ROOT, timeout=15)

    def test_documents_folder_comes_from_the_bridge(self):
        main_cpp = (ROOT / "cmd/gui-qml/main.cpp").read_text(encoding="utf-8")
        self.assertIn('"documentsFolderPath"', main_cpp)
        self.assertIn("QStandardPaths::DocumentsLocation", main_cpp)


class StandardButtonsLanguageContract(unittest.TestCase):
    def test_every_dialog_with_standard_buttons_has_the_app_translator(self):
        for path in sorted(QML_DIR.glob("*.qml")):
            if path.name == "ThemedDialog.qml":
                continue
            source = path.read_text(encoding="utf-8")
            for kind, _body, code in dialog_blocks(source):
                direct = direct_members(code)
                buttons = re.search(r"\bstandardButtons\s*:\s*([^\n]+)", direct)
                if kind != "ThemedDialog" or not buttons or "NoButton" in buttons.group(1):
                    continue
                with self.subTest(file=path.name, buttons=buttons.group(1)):
                    self.assertRegex(direct, r"\btranslate\s*:")

    def test_themed_dialog_relabels_its_buttons(self):
        dialog = (QML_DIR / "ThemedDialog.qml").read_text(encoding="utf-8")
        for key in ("Aceptar", "Cancelar", "Cerrar", "Guardar"):
            self.assertIn('"%s"' % key, dialog)
        self.assertIn("onAboutToShow: relabelStandardButtons()", dialog)
        for language in LANGUAGES:
            catalog = json.loads((LOCALES / (language + ".json")).read_text(encoding="utf-8"))
            for key in ("Aceptar", "Cancelar", "Cerrar", "Guardar", "Sí", "No"):
                self.assertIn(key, catalog, language)

    def test_qt_file_dialog_follows_the_app_language(self):
        main_cpp = (ROOT / "cmd/gui-qml/main.cpp").read_text(encoding="utf-8")
        header = (ROOT / "cmd/gui-qml/qttranslations.h").read_text(encoding="utf-8")
        self.assertIn("qtTranslations.apply(translator.locale())", main_cpp)
        self.assertIn("engine.retranslate()", main_cpp)
        self.assertIn("&TranslatorBridge::localeChanged", main_cpp)
        self.assertIn('QStringLiteral("qt_")', header)
        for language in LANGUAGES[1:]:
            self.assertIn('QStringLiteral("%s")' % language, header)


    def test_file_dialog_buttons_use_the_app_language(self):
        for path in sorted(QML_DIR.glob("*.qml")):
            source = path.read_text(encoding="utf-8")
            for match in re.finditer(r"\b(FileDialog|FolderDialog) \{", source):
                head = source[match.end():match.end() + 220]
                with self.subTest(file=path.name, at=match.start()):
                    self.assertIn("acceptLabel:", head)
                    self.assertIn("rejectLabel:", head)

    def test_catalog_completes_missing_qt_texts(self):
        header = (ROOT / "cmd/gui-qml/qttranslations.h").read_text(encoding="utf-8")
        self.assertIn("class CatalogQtTranslator", header)
        self.assertIn("QCoreApplication::installTranslator(&m_catalog)", header)
        keys = ("qt.filedialog.file_name", "qt.filedialog.filter", "qt.filedialog.overwrite_file",
                "qt.filedialog.1_already_exists_do_you_want_to_replace_it", "qt.filedialog.add_favorite",
                "qt.sidebar.add_favorite", "qt.folderbreadcrumbbar.up", "qt.qstandardpaths.downloads")
        for language in LANGUAGES:
            catalog = json.loads((LOCALES / (language + ".json")).read_text(encoding="utf-8"))
            for key in keys:
                self.assertIn(key, catalog, language)
            self.assertIn("%1", catalog["qt.filedialog.1_already_exists_do_you_want_to_replace_it"], language)


class NoUndefinedFunctionsContract(unittest.TestCase):
    def test_sign_confirmation_uses_existing_certificate_name(self):
        source = MAIN.read_text(encoding="utf-8")
        self.assertNotIn("certificateLabel(", source)
        self.assertIn("window.certificateDisplayName(certificates[signConfirmDialog.selectedCertIndex])", source)

    def test_bare_calls_are_declared_in_the_same_file(self):
        for path in sorted(QML_DIR.glob("*.qml")):
            code = code_of(path.read_text(encoding="utf-8"))
            defined = set(re.findall(r"\bfunction\s+(\w+)\s*\(", code))
            defined |= set(re.findall(r"\bsignal\s+(\w+)", code))
            defined |= set(re.findall(r"\b(?:const|let|var)\s+(\w+)\s*=\s*(?:function\b|\([^)]*\)\s*=>|\w+\s*=>)", code))
            # Variables locales que guardan una función de una propiedad.
            defined |= set(re.findall(r"\b(?:const|let|var)\s+(\w+)\s*=\s*\w+(?:\.\w+)+\s*(?:;|\n)", code))
            calls = set(re.findall(r"(?<![\w.$])([a-z_]\w*)\s*\(", code))
            with self.subTest(file=path.name):
                self.assertEqual(sorted(calls - defined - KEYWORDS - KNOWN_CALLS), [])

    def test_window_calls_exist(self):
        code = code_of(MAIN.read_text(encoding="utf-8"))
        defined = set(re.findall(r"\bfunction\s+(\w+)\s*\(", code))
        calls = set(re.findall(r"\bwindow\.(\w+)\s*\(", code))
        self.assertEqual(sorted(calls - defined), [])


class VerificationReportContract(unittest.TestCase):
    def test_coverage_value_is_translated(self):
        source = MAIN.read_text(encoding="utf-8")
        self.assertIsNone(re.search(r"coverage \? [\w.]+\.coverage :", source))
        self.assertNotIn('tr("Cobertura: ") + info.coverage', source)
        for value in ('"full"', '"partial"', '"chain"'):
            self.assertIn(value, qml_function(source, "verificationCoverageText"))
        signer_go = (ROOT / "internal/adapters/outbound/common/signer").glob("*.go")
        produced = set()
        for path in signer_go:
            produced |= set(re.findall(r'Coverage = "(\w+)"', path.read_text(encoding="utf-8")))
        self.assertTrue(produced <= {"full", "partial", "chain", "unknown"}, produced)

    def test_signers_are_readable_and_dn_goes_to_technical_details(self):
        source = MAIN.read_text(encoding="utf-8")
        summary = qml_function(source, "buildVerificationUserSummary")
        self.assertIn("verificationSignerSummariesText(info)", summary)
        self.assertIn("verificationSignerTechnicalText(info)", summary)
        readable = qml_function(source, "verificationSignerSummariesText")
        self.assertNotIn("subjectDn", readable)
        entries = qml_function(source, "verificationSignerEntries")
        for key in ("report.signer.time_from_timestamp", "report.signer.time_declared", "format.datetime_seconds"):
            self.assertIn(key, entries)
        self.assertIn('"VerificationSigners.js" as Signers', source)

    def test_signer_text_matches_the_engine_report(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("Node no instalado")
        source = MAIN.read_text(encoding="utf-8")
        module = (QML_DIR / "VerificationSigners.js").read_text(encoding="utf-8").replace(".pragma library", "")
        names = ("catalogFormat", "verificationSignerEntries", "verificationSignerLabelLine",
                 "verificationSignerSummariesText", "verificationSignerShortText",
                 "verificationSignerTechnicalText")
        js = "const catalog = " + (LOCALES / "es.json").read_text(encoding="utf-8") + ";\n"
        js += "function tr(key) { return catalog[key] !== undefined ? catalog[key] : key; }\n"
        js += "String.prototype.arg = function(v) { let done = false; return this.replace(/%[1-9]/, m => { if (done) return m; done = true; return String(v); }); };\n"
        js += "const Signers = (function() {\n" + module + "\nreturn {parseDistinguishedName, readableName, readableIssuer, signingTimeText};\n})();\n"
        js += "\n".join(qml_function(source, name).strip() for name in names)
        js += r"""
const assert = require('assert');
const details = {signers: ["SERIALNUMBER=IDCES-00000000T,CN=PRUEBA SINTETICA QA - 00000000T,O=Pruebas Sinteticas,C=ES,2.5.4.4=#130953494e544554494341"],
  signerSummaries: [{subject: "SERIALNUMBER=IDCES-00000000T,CN=PRUEBA SINTETICA QA - 00000000T,O=Pruebas Sinteticas,C=ES,2.5.4.4=#130953494e544554494341",
    issuer: "CN=AC PRUEBAS,O=Pruebas Sinteticas,C=ES", fingerprint: "ab:cd", signingTime: "2026-10-05T08:30:00Z", signingTimeSource: "timestamp"}]};
const readable = verificationSignerSummariesText(details);
assert.ok(readable.startsWith("• PRUEBA SINTETICA QA - 00000000T\n"), readable);
assert.ok(readable.includes("NIF / identificador: IDCES-00000000T"), readable);
assert.ok(readable.includes("Emisor: AC PRUEBAS (Pruebas Sinteticas)"), readable);
assert.ok(/Fecha de la firma: 0[45]\/10\/2026 \d\d:30:00 \(UTC[^)]*\), según el sello de tiempo/.test(readable), readable);
assert.ok(!readable.includes("2.5.4.4") && !readable.includes("SERIALNUMBER="), readable);
assert.ok(verificationSignerShortText(details).startsWith("PRUEBA SINTETICA QA - 00000000T, el "));
const technical = verificationSignerTechnicalText(details);
assert.ok(technical.includes("Titular (DN): SERIALNUMBER=IDCES-00000000T"), technical);
assert.ok(technical.includes("Emisor (DN): CN=AC PRUEBAS"), technical);
assert.strictEqual(verificationSignerEntries(details).length, 1);
"""
        subprocess.run([node], input=js, text=True, check=True, cwd=ROOT, timeout=15)

    def test_printable_report_is_requested_and_saved(self):
        source = MAIN.read_text(encoding="utf-8")
        bridge = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
        self.assertIn("backend.verifyFileWithReport(path, originalPath)", source)
        self.assertIn('window.requestVerification(next.path, "", false)', source)
        self.assertIn("verifyHtmlReportSaveDialog", source)
        self.assertIn('params["reportLanguage"]', bridge)
        self.assertIn('result.remove(QStringLiteral("reportHtml"))', bridge)


class SentencePunctuationContract(unittest.TestCase):
    def test_reason_goes_through_catalog_templates(self):
        source = MAIN.read_text(encoding="utf-8")
        self.assertNotIn('currentOutputVerificationMessage + (reason !== "" ? ": " + reason : "")', source)
        for key in ("verificacion.auto.incidencias_motivo", "verificacion.auto.sin_confianza_motivo"):
            self.assertIn('"%s"' % key, source)
            for language in LANGUAGES:
                catalog = json.loads((LOCALES / (language + ".json")).read_text(encoding="utf-8"))
                self.assertIn("%1", catalog[key], language)
                self.assertNotRegex(catalog[key], r"[.。]\s*[:：]", language)

    def test_no_sentence_ending_in_period_is_joined_with_colon(self):
        code = code_of(MAIN.read_text(encoding="utf-8"))
        source = MAIN.read_text(encoding="utf-8")
        self.assertIsNone(re.search(r'tr\("[^"]*\.\"\)\s*\+\s*": "', source))
        self.assertIsNone(re.search(r'Message\s*\+\s*\([^)]*": "', source))
        self.assertTrue(code)


if __name__ == "__main__":
    unittest.main()
