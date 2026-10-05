#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Genera las proyecciones de los códigos NTI; --check comprueba su vigencia."""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'internal/adapters/outbound/common/eni/catalogos.go'
HEADER = '// Derechos de autor (C) 2026 Alberto Avidad Fernández.\n// Autoría: Alberto Avidad Fernández\n// Licencia: EUPL 1.2 o posterior\n// SPDX-License-Identifier: EUPL-1.2\n\n// Generado por scripts/generar_catalogos_eni.py; editar catalogos.go.\n'


def outputs():
    source = SOURCE.read_text(encoding='utf-8')
    catalogs = {}
    for name in ('EstadosElaboracion', 'TiposDocumentales', 'EstadosExpediente'):
        body = re.search(r'func ' + name + r'\(\) \[\]string\s*\{(.*?)\n\}', source, re.S).group(1)
        catalogs[name] = re.findall(r'"([A-Z]+\d+)"', body)
    js = HEADER + '.pragma library\n\n'
    cs = HEADER + 'namespace GrxFirma.WinUI.Core.Operations;\n\npublic static class EniCatalog\n{\n'
    for name, codes in catalogs.items():
        values = ', '.join('"' + c + '"' for c in codes)
        js += f'var {name} = [{values}]\n'
        cs += f'    public static System.Collections.Generic.IReadOnlyList<string> {name} {{ get; }} = System.Array.AsReadOnly(new[] {{ {values} }});\n'
    cs += '}\n'
    return {ROOT / 'cmd/gui-qml/qml/EniCatalog.js': js,
            ROOT / 'cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations/EniCatalog.cs': cs}


def main():
    for path, content in outputs().items():
        if '--check' in sys.argv:
            if not path.exists() or path.read_text(encoding='utf-8') != content:
                raise SystemExit(f'Catálogo desactualizado: {path.relative_to(ROOT)}')
        else:
            path.write_text(content, encoding='utf-8', newline='\n')

if __name__ == '__main__':
    main()
