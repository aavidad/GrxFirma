# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Licencia: EUPL-1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Los diálogos deben definir la superficie antes de usar texto del tema."""

import re
from pathlib import Path
import unittest

QML_DIR = Path(__file__).resolve().parents[1] / "qml"
STRINGS_AND_COMMENTS = re.compile(r'"(?:\\.|[^"\\])*"|\'(?:\\.|[^\'\\])*\'|//[^\n]*|/\*.*?\*/', re.S)
OBJECT = re.compile(r"\b(Dialog|Popup|MessageDialog|ThemedDialog)\s*\{")


def dialog_blocks(source):
    code = STRINGS_AND_COMMENTS.sub(lambda m: " " * len(m.group()), source)
    for match in OBJECT.finditer(code):
        start = match.end()
        depth = 1
        end = start
        while depth and end < len(code):
            depth += (code[end] == "{") - (code[end] == "}")
            end += 1
        yield match.group(1), source[start:end - 1], code[start:end - 1]


def direct_members(code):
    depth = 0
    result = []
    for char in code:
        if char == "}":
            depth -= 1
        result.append(char if depth == 0 else " ")
        if char == "{":
            depth += 1
    return "".join(result)


def unthemed_dialogs(source):
    failures = []
    for kind, body, code in dialog_blocks(source):
        direct = direct_members(code)
        if kind == "ThemedDialog":
            if not re.search(r"\btheme\s*:", direct):
                failures.append("ThemedDialog sin tema")
        elif re.search(r"\b(?:currentTheme|theme)\.(?:textColor|secondaryTextColor)", code):
            if not re.search(r"\bbackground\s*:", direct):
                failures.append(kind + " sin fondo")
        elif kind == "MessageDialog":
            # Los diálogos nativos no permiten controlar sus colores: usar el componente común.
            failures.append("MessageDialog sin colores verificables")
        elif not re.search(r"\bbackground\s*:", direct):
            failures.append(kind + " sin fondo")
    return failures


def themes():
    source = (QML_DIR / "main.qml").read_text(encoding="utf-8")
    section = source.split("property var themes: [", 1)[1].split("property var currentTheme:", 1)[0]
    return [dict(re.findall(r'(\w+):\s*"([^"]+)"', block)) for block in re.findall(r"\{([^{}]+)\}", section)]


def luminance(color):
    rgb = [int(color[i:i + 2], 16) / 255 for i in (1, 3, 5)]
    rgb = [v / 12.92 if v <= 0.04045 else ((v + 0.055) / 1.055) ** 2.4 for v in rgb]
    return sum(v * weight for v, weight in zip(rgb, (.2126, .7152, .0722)))


def contrast(a, b):
    a, b = sorted((luminance(a), luminance(b)))
    return (b + .05) / (a + .05)


class DialogThemeContract(unittest.TestCase):
    def test_detector_rejects_missing_or_nested_background(self):
        for source in (
            "Dialog { Text { color: currentTheme.textColor } }",
            "Dialog { Text { color: currentTheme.textColor; background: Rectangle {} } }",
            "ThemedDialog { Text { property var theme: currentTheme } }",
        ):
            self.assertTrue(unthemed_dialogs(source))
        self.assertFalse(unthemed_dialogs("Dialog { background: Rectangle { color: currentTheme.cardColor }; Text { color: currentTheme.textColor } }"))
        self.assertFalse(unthemed_dialogs("ThemedDialog { theme: currentTheme }"))

    def test_all_dialogs_and_popups_have_a_theme_surface(self):
        for path in QML_DIR.rglob("*.qml"):
            with self.subTest(file=path.name):
                self.assertEqual(unthemed_dialogs(path.read_text(encoding="utf-8")), [])

    def test_shared_dialog_styles_title_text_inputs_and_buttons(self):
        source = (QML_DIR / "ThemedDialog.qml").read_text(encoding="utf-8")
        for member in ("background: Rectangle", "header: Item", "footer: DialogButtonBox", "palette.text: theme.textColor", "palette.base: theme.cardColor", "palette.buttonText: theme.textColor", "palette.highlight: theme.textColor", "palette.highlightedText: theme.cardColor", "color: dialog.theme.textColor", "dialog.theme.cardColor", "standardButtons: dialog.standardButtons"):
            self.assertIn(member, source)
        self.assertNotIn("onAccepted: dialog.accept()", source)
        self.assertNotIn("onRejected: dialog.reject()", source)
        for path in (QML_DIR / "main.qml", QML_DIR / "TokenSettingsPanel.qml"):
            self.assertTrue(all(kind == "ThemedDialog" for kind, _, _ in dialog_blocks(path.read_text(encoding="utf-8"))))

    def test_normal_and_secondary_text_pass_wcag_aa_in_every_theme(self):
        self.assertEqual(len(themes()), 14)
        for theme in themes():
            for token in ("textColor", "secondaryTextColor"):
                with self.subTest(theme=theme["name"], token=token):
                    self.assertGreaterEqual(contrast(theme[token], theme["cardColor"]), 4.5)

    def test_theme_property_names_exist_in_all_themes(self):
        known = set(themes()[0]) | {"borderOpacity"}
        for path in QML_DIR.rglob("*.qml"):
            used = set(re.findall(r"\b(?:currentTheme|theme)\.(\w+)", path.read_text(encoding="utf-8")))
            self.assertFalse(used - known, (path.name, used - known))
