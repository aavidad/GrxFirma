#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
"""Comprueba cobertura, duplicados, parámetros y referencias de los 11 idiomas."""
import re
from pathlib import Path
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[3]
RES = ROOT / "mobile/android/app/src/main/res"
LANGUAGES = ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh")
FORMAT = re.compile(r"%\d+\$[-+.\d]*[sdf]")


def read(path):
    """Lee strings.xml y los catálogos propios (strings_*.xml) de la carpeta."""
    records = {}
    for part in sorted(path.parent.glob("strings*.xml")):
        for name, record in read_file(part).items():
            assert name not in records, f"{part}: recurso duplicado {name}"
            records[name] = record
    return records


def read_file(path):
    records = {}
    for node in ET.parse(path).getroot():
        name = node.attrib["name"]
        assert name not in records, f"{path}: recurso duplicado {name}"
        if node.tag == "plurals":
            value = {item.attrib["quantity"]: item.text or "" for item in node}
        else:
            value = node.text or ""
            assert value.strip(), f"{path}: texto vacío {name}"
        records[name] = (node.tag, value)
    return records


def main():
    baseline = read(RES / "values/strings.xml")
    for language in LANGUAGES:
        folder = "values" if language == "es" else "values-b+ca+ES+valencia" if language == "va" else f"values-{language}"
        records = read(RES / folder / "strings.xml")
        assert records.keys() == baseline.keys(), f"{language}: recursos ausentes o adicionales"
        for name, (kind, value) in records.items():
            reference_kind, reference = baseline[name]
            assert kind == reference_kind, f"{language}: tipo distinto {name}"
            if kind == "plurals":
                assert "other" in value, f"{language}: plural sin other {name}"
                for text in value.values():
                    assert sorted(FORMAT.findall(text)) == sorted(FORMAT.findall(reference["other"])), f"{language}: parámetros de {name}"
            else:
                assert sorted(FORMAT.findall(value)) == sorted(FORMAT.findall(reference)), f"{language}: parámetros de {name}"
    for path in (ROOT / "mobile/android/app/src").rglob("*.kt"):
        for kind, name in re.findall(r"(?<!android\.)\bR\.(string|plurals)\.(\w+)", path.read_text()):
            assert name in baseline, f"{path}: referencia inexistente {name}"
    for path in RES.rglob("*.xml"):
        ET.parse(path)
        for name in re.findall(r"@string/(\w+)", path.read_text()):
            assert name in baseline or name == "appbar_scrolling_view_behavior", f"{path}: referencia inexistente {name}"
    assert len(ET.parse(RES / "xml/locales_config.xml").getroot()) == len(LANGUAGES)
    print(f"OK: {len(LANGUAGES)} idiomas, {len(baseline)} recursos por idioma, parámetros y XML válidos")


if __name__ == "__main__":
    main()
