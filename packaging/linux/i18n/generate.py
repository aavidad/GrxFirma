#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Genera datos TSV legibles por Bash; los JSON son la fuente de traducción.
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent


def outputs():
    result = {}
    for locale in ('es', 'en', 'ca', 'va', 'gl', 'eu', 'fr', 'de', 'it', 'pt', 'zh'):
        data = json.loads((ROOT / f'{locale}.json').read_text(encoding='utf-8'))
        for key, value in data.items():
            if '\t' in value or '\r' in value:
                raise ValueError(f'{locale}:{key}')
        result[f'{locale}.tsv'] = ''.join(f'{key}\t{value.replace(chr(10), chr(92) + "n")}\n' for key, value in sorted(data.items()))
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    for name, value in outputs().items():
        path = ROOT / name
        if args.check:
            if not path.exists() or path.read_text(encoding='utf-8') != value:
                raise SystemExit(f'Catálogo desactualizado: {path}')
        else:
            path.write_text(value, encoding='utf-8')
