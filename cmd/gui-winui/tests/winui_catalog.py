# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lectura de fuentes WinUI con los textos del catálogo español a la vista.

El código C# de la aplicación solo nombra claves (``winui.*``) y el texto vive
en los catálogos. Varias pruebas de contrato comprueban qué mensaje recibe la
persona; para ellas se sustituye cada clave por su texto español, de modo que
siguen leyendo el mensaje real sin volver a escribirlo en el código.
"""

import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[3]
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
# Las claves winui.parity.* son anteriores y las pruebas las citan por nombre.
_KEY = r'"(winui\.(?!parity\.)[a-z0-9_.]+)"'


def spanish_catalog():
    return json.loads((LOCALES / "es.json").read_text(encoding="utf-8"))


def _quoted(text):
    return '"' + text.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n") + '"'


def expand_catalog_keys(source, catalog=None):
    """Devuelve el fuente con ``Localizer.Text("winui.x")`` y ``"winui.x"``
    sustituidos por el texto español entre comillas."""
    catalog = spanish_catalog() if catalog is None else catalog

    def text(match):
        key = match.group(1)
        return _quoted(catalog[key]) if key in catalog else match.group(0)

    source = re.sub(r"Localizer\.Text\(\s*" + _KEY + r"\s*\)", text, source)
    return re.sub(_KEY, text, source)


def read_with_catalog(path):
    return expand_catalog_keys(pathlib.Path(path).read_text(encoding="utf-8"))
