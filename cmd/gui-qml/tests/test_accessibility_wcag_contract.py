# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contratos de accesibilidad (WCAG 2.1 AA) de la aplicación Qt.

Protegen los hallazgos corregidos en la auditoría: nombres accesibles de los
campos, contraste del texto de estado, foco del teclado y roles de los
mensajes. Si se revierte una corrección, la prueba correspondiente falla.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
QML_DIR = ROOT / "cmd/gui-qml/qml"
MAIN = (QML_DIR / "main.qml").read_text(encoding="utf-8")
LINES = MAIN.split("\n")

FIELD_TYPES = (
    "ComboBox", "ThemedComboBox", "TextField", "ThemedTextField",
    "TextArea", "ThemedTextArea", "SpinBox", "ThemedSpinBox",
    "Slider", "ThemedSlider",
)
FIELD_START = re.compile(r"^\s*(" + "|".join(FIELD_TYPES) + r")\s*\{")
TEXT_START = re.compile(r"^\s*(Text|Label)\s*\{")
# Colores de acento o de estado que no garantizan 4,5:1 en todos los temas.
RISKY_TEXT_COLOR = re.compile(
    r"primaryColor|#2ecc71|#e74c3c|#f39c12|#f1c40f|#27ae60|#c0392b|#ffb3b3"
    r"|verificationOutcomeColor\(|verificationAspectColor\(",
    re.IGNORECASE,
)
CONTRAST_SAFE = re.compile(r"Contrast\.|ColorOn\(|ForBackground\(")


def block_end(start: int) -> int:
    depth = 0
    for index in range(start, len(LINES)):
        depth += LINES[index].count("{") - LINES[index].count("}")
        if depth <= 0:
            return index
    raise AssertionError(f"bloque sin cerrar en la línea {start + 1}")


def portal_layer_range() -> range:
    """Capa del sello del portal: la cubre otra rama y queda fuera."""
    for index, line in enumerate(LINES[:-1]):
        if line.strip() == "Rectangle {" and LINES[index + 1].strip() == "visible: portalSealMode":
            return range(index, block_end(index) + 1)
    raise AssertionError("no se encontró la capa del sello del portal")


PORTAL = portal_layer_range()


def element_blocks(pattern: re.Pattern[str]):
    for index, line in enumerate(LINES):
        match = pattern.match(line)
        if match and index not in PORTAL:
            end = block_end(index)
            yield index, match.group(1), "\n".join(LINES[index:end + 1])


def direct_property(block: str, name: str) -> str | None:
    """Valor de una propiedad de primer nivel (incluye ternarios multilínea)."""
    lines = block.split("\n")
    if "}" in lines[0]:
        found = re.search(r"\b" + re.escape(name) + r":\s*([^;}]+)", lines[0])
        return found.group(1) if found else None
    depth = 0
    for index, line in enumerate(lines):
        if depth == 1 and re.match(r"^\s*" + re.escape(name) + r":", line):
            value = line
            follow = index
            while follow + 1 < len(lines) and (
                value.rstrip().endswith(("?", ":", "(", "&&", "||"))
                or re.match(r"^\s*[?:]", lines[follow + 1])
            ):
                follow += 1
                value += " " + lines[follow].strip()
            return value.split(":", 1)[1].strip()
        depth += line.count("{") - line.count("}")
    return None


class ContractCase(unittest.TestCase):
    """Mensajes cortos: sin volcar main.qml entero cuando algo falla."""

    def assertInMain(self, text: str) -> None:
        self.assertTrue(text in MAIN, f"falta en main.qml: {text}")

    def assertNotInMain(self, text: str) -> None:
        self.assertFalse(text in MAIN, f"no debe aparecer en main.qml: {text}")


class FieldNamesContract(ContractCase):
    def test_every_field_declares_its_own_accessible_name(self) -> None:
        missing = []
        for index, kind, block in element_blocks(FIELD_START):
            if "id: clipboardProxy" in block:
                continue
            if "Accessible.name:" not in block:
                missing.append(f"{index + 1}: {kind}")
        self.assertEqual(missing, [], "campos sin Accessible.name")

    def test_placeholder_is_never_the_accessible_name(self) -> None:
        self.assertNotInMain("Accessible.name: placeholderText")

    def test_visible_labels_name_the_reported_fields(self) -> None:
        for name in (
            'tr("RUTA DE ENTRADA")', 'tr("RUTA DE SALIDA (PDF FIRMADO)")',
            'tr("Ruta de salida opcional")', 'tr("Buscar certificado")',
            'tr("Perfil")', 'tr("Contenedor")', 'tr("Dominio")', 'tr("Usuario")',
            'tr("Contraseña")', 'tr("Host / IP:")', 'tr("Puerto:")',
            'tr("Servidor TSA:")', 'tr("Motivo")', 'tr("Ubicación")', 'tr("Contacto")',
        ):
            with self.subTest(name=name):
                self.assertInMain("Accessible.name: " + name)

    def test_csv_and_facturae_fields_have_a_visible_label(self) -> None:
        for key in ("paridad.lote3.csv.code", "paridad.lote3.csv.url",
                    "paridad.lote3.csv.text_optional", "Ciudad", "Provincia",
                    "Código postal", "País", "Identificador de política FacturaE"):
            with self.subTest(key=key):
                self.assertTrue(re.search(r'Text \{ text: tr\("' + re.escape(key) + r'"\)', MAIN), key)
                self.assertInMain(f'Accessible.name: tr("{key}")')

    def test_outside_the_portal_layer_controls_use_themed_borders(self) -> None:
        plain = re.compile(r"^\s*(ComboBox|SpinBox|Slider|CheckBox)\s*\{")
        offenders = []
        for index, kind, block in element_blocks(plain):
            # El selector de certificado dibuja su propio fondo con borde.
            if "id: signingCertificateCombo" in block:
                continue
            offenders.append(f"{index + 1}: {kind}")
        self.assertEqual(offenders, [])
        token = (QML_DIR / "TokenSettingsPanel.qml").read_text(encoding="utf-8")
        self.assertNotRegex(token, r"(?m)^\s*CheckBox\s*\{")

    def test_themed_spinbox_slider_and_checkbox_reach_three_to_one(self) -> None:
        for name in ("ThemedSpinBox.qml", "ThemedSlider.qml", "ThemedCheckBox.qml"):
            with self.subTest(name=name):
                source = (QML_DIR / name).read_text(encoding="utf-8")
                self.assertIn("Contrast.accentOn(", source)
                self.assertIn("3.0)", source)
        qrc = (ROOT / "cmd/gui-qml/qml.qrc").read_text(encoding="utf-8")
        self.assertIn("qml/ThemedSpinBox.qml", qrc)
        self.assertIn("qml/ThemedSlider.qml", qrc)


class TextContrastContract(ContractCase):
    def test_status_and_accent_text_colors_go_through_contrast(self) -> None:
        offenders = []
        for index, _kind, block in element_blocks(TEXT_START):
            value = direct_property(block, "color")
            if value and RISKY_TEXT_COLOR.search(value) and not CONTRAST_SAFE.search(value):
                offenders.append(f"{index + 1}: {value[:90]}")
        self.assertEqual(offenders, [], "texto con color de acento sin Contrast")

    def test_batch_badge_uses_legible_fill_and_readable_text(self) -> None:
        self.assertInMain("id: batchStateBadge")
        self.assertInMain("color: Contrast.legibleFill(!modelData.ok")
        self.assertInMain('color: Contrast.readableOn(batchStateBadge.color, "#ffffff")')

    def test_fingerprint_is_readable(self) -> None:
        self.assertIsNone(re.search(r"font\.pixelSize:\s*9\b", MAIN))
        self.assertNotInMain('"#f4b400"')
        self.assertInMain("rotateArea.activeFocus ? sealDrawArea.focusColor")


class KeyboardContract(ContractCase):
    def test_clipboard_proxy_stays_out_of_tab_order(self) -> None:
        start = MAIN.index("id: clipboardProxy")
        block = MAIN[start:MAIN.index("}", start)]
        self.assertIn("activeFocusOnTab: false", block)
        self.assertIn("Accessible.ignored: true", block)
        copy = MAIN[MAIN.index("function copyTextToClipboard"):]
        copy = copy[:copy.index("\n    }\n")]
        self.assertIn("const previousFocus = window.activeFocusItem", copy)
        self.assertIn("previousFocus.forceActiveFocus()", copy)

    def test_disclosure_buttons_toggle_and_say_what_they_do(self) -> None:
        count = 0
        for index, _kind, block in element_blocks(re.compile(r"^\s*(ToolButton)\s*\{")):
            if '? "▼" : "▶"' not in block:
                continue
            count += 1
            with self.subTest(line=index + 1):
                self.assertIn("onClicked:", block)
                self.assertRegex(block, r'Accessible\.name: .*tr\("Ocultar detalles"\) : tr\("Mostrar detalles"\)')
                self.assertIn("Accessible.checkable: true", block)
                self.assertIn("Accessible.checked:", block)
        self.assertGreaterEqual(count, 10)

    def test_initial_focus_lands_on_navigation(self) -> None:
        self.assertInMain("function focusInitialControl()")
        self.assertInMain("onActiveChanged: if (active) Qt.callLater(window.focusInitialControl)")
        self.assertInMain("initialFocusTimer.start()")
        self.assertInMain("id: navigationColumn")

    def test_window_has_minimum_size_and_dialogs_fit(self) -> None:
        self.assertInMain("minimumWidth: 640")
        self.assertInMain("minimumHeight: 480")
        self.assertIsNone(re.search(r"(?m)^        width: (5|6|7)\d\d$", MAIN))

    def test_help_popup_scrolls_with_keyboard(self) -> None:
        source = (QML_DIR / "HelpButton.qml").read_text(encoding="utf-8")
        self.assertIn("function scrollHelp(delta)", source)
        self.assertIn("Qt.Key_PageDown", source)
        self.assertIn("control.ensureHelpVisible(moreButton)", source)


class RolesContract(ContractCase):
    def test_navigation_buttons_expose_the_active_section(self) -> None:
        nav = MAIN[MAIN.index("component NavButton"):]
        nav = nav[:nav.index("// Status Bar")]
        self.assertIn("Accessible.checkable: true", nav)
        self.assertIn("Accessible.checked: navButtonRoot.active", nav)
        self.assertIn("Accessible.ignored: true", nav)
        self.assertFalse((QML_DIR / "NavButton.qml").exists())
        qrc = (ROOT / "cmd/gui-qml/qml.qrc").read_text(encoding="utf-8")
        self.assertNotIn("NavButton.qml", qrc)

    def test_certificate_cards_separate_focus_and_selection(self) -> None:
        self.assertEqual(MAIN.count("Accessible.checked: selected"), 2)
        self.assertEqual(MAIN.count("property bool emphasized: selected"), 2)
        self.assertNotInMain("property bool emphasized: activeFocus")

    def test_headings_and_alerts(self) -> None:
        titles = {"Firma Digital", "Verificación de Firma", "Configuración",
                  "Huellas e integridad", "Cifrar / Proteger", "Descifrar / Desproteger"}
        found = set()
        for _index, _kind, block in element_blocks(TEXT_START):
            text = direct_property(block, "text") or ""
            for title in titles:
                if text == f'tr("{title}")' and "Accessible.role: Accessible.Heading" in block:
                    found.add(title)
        self.assertEqual(found, titles)
        status = MAIN[MAIN.index("id: statusMessageText"):]
        status = status[:status.index("\n            }\n")]
        self.assertIn("Accessible.role: Accessible.AlertMessage", status)
        self.assertIn("ToolTip.visible: statusMessageText.truncated", status)
        self.assertInMain('text: window.currentOutputVerificationMessage')
        for name in ("EniValidatedField.qml", "EniDateField.qml"):
            with self.subTest(name=name):
                source = (QML_DIR / name).read_text(encoding="utf-8")
                self.assertIn("Accessible.role: Accessible.AlertMessage", source)

    def test_accessible_properties_are_never_set_twice(self) -> None:
        # Un mismo Accessible.* repetido en un elemento impide cargar main.qml.
        start = re.compile(r"^\s*[A-Z][\w.]*\s*\{\s*$")
        repeated = []
        for index, line in enumerate(LINES):
            if not start.match(line):
                continue
            depth = 0
            seen: set[str] = set()
            for follow in range(index, len(LINES)):
                current = LINES[follow]
                if depth == 1:
                    found = re.match(r"^\s*(Accessible\.\w+):", current)
                    if found:
                        if found.group(1) in seen:
                            repeated.append(f"{follow + 1}: {found.group(1)}")
                        seen.add(found.group(1))
                depth += current.count("{") - current.count("}")
                if depth <= 0:
                    break
        self.assertEqual(repeated, [])

    def test_pdf_page_is_a_named_graphic(self) -> None:
        start = MAIN.index("id: pdfPageImage")
        block = MAIN[start:start + 400]
        self.assertIn("Accessible.role: Accessible.Graphic", block)
        self.assertIn('tr("portal.seal.page")', block)

    def test_themed_dialog_has_a_single_dialog_role(self) -> None:
        source = (QML_DIR / "ThemedDialog.qml").read_text(encoding="utf-8")
        header = source[source.index("header: Item"):source.index("footer:")]
        self.assertNotIn("Accessible.Dialog", header)
        self.assertIn("Accessible.role: Accessible.Heading", header)
        self.assertIn("popupItem.Accessible.role = Accessible.Dialog", source)
        self.assertIn("popupItem.Accessible.name = dialog.accessibleName", source)


class PlainStatusMessageContract(unittest.TestCase):
    def test_connection_failure_tells_what_to_do(self) -> None:
        bridge = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
        self.assertNotIn('setStatus(it(QStringLiteral("Error IPC: "))', bridge)
        self.assertNotIn('setStatus(it(QStringLiteral("No se pudo conectar al backend tras 10 segundos")))', bridge)
        self.assertIn("No se puede conectar con el servicio de firma de GrxFirma.", bridge)


if __name__ == "__main__":
    unittest.main()
