# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Un manejador o una propiedad repetidos en el mismo objeto QML impiden
cargar la ventana entera («Property value set multiple times») y qmllint no
lo detecta. La aplicación Linux terminaba sin abrirse."""

import re
import unittest
from pathlib import Path

QML_DIR = Path(__file__).resolve().parents[1] / "qml"
MEMBER = re.compile(r"^\s*(?:(on[A-Z]\w*)\s*:|property\s+[\w<>.]+\s+(\w+)\s*[:\n]|property\s+[\w<>.]+\s+(\w+)\s*$)")
STRING = re.compile(r'"(?:\\.|[^"\\])*"|\'(?:\\.|[^\'\\])*\'')


def duplicated_members(text):
    stack = [0]
    seen = {}
    duplicates = []
    for number, line in enumerate(text.splitlines(), 1):
        code = STRING.sub('""', line.split("//", 1)[0])
        match = MEMBER.match(code)
        if match:
            name = next(group for group in match.groups() if group)
            key = (stack[-1], name)
            if key in seen:
                duplicates.append((name, seen[key], number))
            else:
                seen[key] = number
        for char in code:
            if char == "{":
                stack.append(number)
            elif char == "}" and len(stack) > 1:
                stack.pop()
    return duplicates


class QmlDuplicateMembersContract(unittest.TestCase):
    def test_detector_finds_repeated_handler(self):
        sample = "Item {\n    onXChanged: a()\n    Rectangle { onXChanged: b() }\n    onXChanged: c()\n}\n"
        self.assertEqual(duplicated_members(sample), [("onXChanged", 2, 4)])

    def test_no_qml_object_repeats_a_handler_or_property(self):
        for path in sorted(QML_DIR.rglob("*.qml")):
            with self.subTest(file=path.name):
                self.assertEqual(duplicated_members(path.read_text(encoding="utf-8")), [])


if __name__ == "__main__":
    unittest.main()
