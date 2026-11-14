#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Catálogos de datos: genera inclusiones NSIS reproducibles, sin red.
from pathlib import Path
import argparse
import json

ROOT = Path(__file__).resolve().parent
LANGUAGES = {'es': 'Spanish', 'en': 'English', 'ca': 'Catalan', 'va': 'Valencian',
             'gl': 'Galician', 'eu': 'Basque', 'fr': 'French', 'de': 'German',
             'it': 'Italian', 'pt': 'Portuguese', 'zh': 'SimpChinese'}


def outputs():
    result = {}
    for locale, language in LANGUAGES.items():
        data = json.loads((ROOT / f'{locale}.json').read_text(encoding='utf-8'))
        lines = ['; Generado por i18n/generate.py; editar el JSON del idioma.']
        for key, value in sorted(data.items()):
            value = value.replace('"', '$\\"')
            lines.append(f'LangString GRX_{key} ${{LANG_{language.upper()}}} "{value}"')
        result[f'{locale}.nsh'] = '\ufeff' + '\n'.join(lines) + '\n'
    data = json.loads((ROOT / 'es.json').read_text(encoding='utf-8'))
    result['version.nsh'] = '\ufeff; Metadatos PE procedentes del catálogo, independientes del idioma de ejecución.\n' + ''.join(
        f'!define GRX_VERSION_{key} "{data[key]}"\n' for key in ('PRODUCT_SUITE', 'PRODUCT_QT', 'PRODUCT_CLI', 'PRODUCT_AFIRMA_URI'))
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    for name, value in outputs().items():
        path = ROOT / name
        if args.check:
            if not path.exists() or path.read_text(encoding='utf-8') != value:
                raise SystemExit(f'Inclusión desactualizada: {path}')
        else:
            path.write_text(value, encoding='utf-8')
