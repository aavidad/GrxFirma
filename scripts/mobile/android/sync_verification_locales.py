#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Empaqueta solo los mensajes del catálogo desktop que utiliza el verificador."""
import json
import re
from pathlib import Path
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[3]
CATALOGUES = ROOT / "internal/adapters/outbound/common/localizador/locales"
RES = ROOT / "mobile/android/app/src/main/res"
ASSETS = ROOT / "mobile/android/app/src/main/assets/locales"
LANGUAGES = ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh")


# Veri*Factu, ENI y la leyenda CSV devuelven claves cerradas del catálogo
# desktop: se empaquetan por clave, no por texto español.
KEY_PREFIXES = ("verifactu.", "eni.validacion.", "eni.codigo.", "csv.error.")
EXCLUDED_KEYS = {"verifactu.cli_usage", "eni.validacion.input", "eni.validacion.help"}


def keyed_messages(catalogue):
    return {key: value for key, value in catalogue.items()
            if key.startswith(KEY_PREFIXES) and key not in EXCLUDED_KEYS}


def main():
    literals = set()
    for directory in ("internal/adapters/outbound/common/signer", "internal/domain"):
        for path in (ROOT / directory).glob("*.go"):
            if path.name.endswith("_test.go"):
                continue
            for match in re.finditer(r'"(?:[^"\\]|\\.)*"', path.read_text()):
                try:
                    literals.add(json.loads(match.group()))
                except json.JSONDecodeError:
                    pass
    spanish = json.loads((CATALOGUES / "es.json").read_text())
    ASSETS.mkdir(parents=True, exist_ok=True)
    source_overlay = {node.attrib["name"]: node.text for node in ET.parse(RES / "values/strings.xml").getroot()
                      if node.tag == "string" and node.attrib["name"].startswith("engine_")}
    for language in LANGUAGES:
        catalogue = json.loads((CATALOGUES / f"{language}.json").read_text())
        messages = {spanish.get(key, key): value for key, value in catalogue.items()
                    if spanish.get(key, key) in literals}
        # Diagnósticos cuyo literal del motor es la propia clave del catálogo
        # (su texto en castellano puede estar mejor redactado que el literal).
        messages.update({key: value for key, value in catalogue.items() if key in literals})
        folder = "values" if language == "es" else "values-b+ca+ES+valencia" if language == "va" else f"values-{language}"
        overlay = {node.attrib["name"]: node.text for node in ET.parse(RES / folder / "strings.xml").getroot() if node.tag == "string"}
        messages.update({value: overlay[key] for key, value in source_overlay.items()})
        messages.update(keyed_messages(catalogue))
        (ASSETS / f"{language}.json").write_text(json.dumps(messages, ensure_ascii=False, indent=2) + "\n")


if __name__ == "__main__":
    main()
