# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Catálogos completos y compilación de los instaladores reales con carga ficticia.
from pathlib import Path
import json
import re
import shutil
import subprocess
import tempfile
import unittest

WINDOWS = Path(__file__).resolve().parents[1]
LANGUAGES = {'es', 'en', 'ca', 'va', 'gl', 'eu', 'fr', 'de', 'it', 'pt', 'zh'}


class InstallerI18nTests(unittest.TestCase):
    def test_catalogs_cover_all_keys_and_placeholders(self):
        catalogs = {p.stem: json.loads(p.read_text()) for p in (WINDOWS / 'i18n').glob('*.json') if p.stem != 'code-exceptions'}
        self.assertEqual(set(catalogs), LANGUAGES)
        tokens = lambda s: sorted(re.findall(r'\$(?:[0-9]|\{[^}]+\})', s))
        for locale, data in catalogs.items():
            with self.subTest(locale=locale):
                self.assertEqual(set(data), set(catalogs['es']))
                for key, value in data.items():
                    self.assertTrue(value.strip(), (locale, key))
                    self.assertEqual(tokens(value), tokens(catalogs['es'][key]), (locale, key))
        for path in list(WINDOWS.glob('*.nsi')) + list(WINDOWS.glob('*.nsh')):
            for key in re.findall(r'\$\(GRX_(\w+)\)', path.read_text(encoding='utf-8-sig')):
                self.assertIn(key, catalogs['es'], (path, key))
        subprocess.run(['python3', str(WINDOWS / 'i18n/generate.py'), '--check'], check=True)

    def test_no_spanish_prose_outside_catalogs(self):
        # Excepciones exactas: identidades y rutas históricas son contratos con
        # instalaciones anteriores. No autorizan frases nuevas ni prefijos amplios.
        exceptions = json.loads((WINDOWS / 'i18n/code-exceptions.json').read_text())
        self.assertTrue(all(exceptions.values()))
        spanish = re.compile(r'[áéíóúñ¿¡]|\b(?:Instalador|instalacion|fallida|Codigo:|Fase:|Preguntar cada vez|Programa para firmar|No se |Desde ahora|Falta definir)')
        for path in list(WINDOWS.glob('*.nsi')) + list(WINDOWS.glob('*.nsh')):
            for number, line in enumerate(path.read_text(encoding='utf-8-sig').splitlines(), 1):
                if line.lstrip().startswith(';'):
                    continue
                for literal in re.findall(r'"([^"\n]*)"', line):
                    if literal in exceptions:
                        continue
                    self.assertIsNone(spanish.search(literal), f'{path.name}:{number}: {literal}')
        # También rechaza prosa inglesa nueva en los puntos de salida visibles.
        for path in list(WINDOWS.glob('*.nsi')) + list(WINDOWS.glob('*.nsh')):
            source = re.sub(r'(?m)^\s*;.*$', '', path.read_text(encoding='utf-8-sig'))
            source = source.replace('\\\n', ' ')
            for line in source.splitlines():
                if re.search(r'MessageBox|DetailPrint|FileWrite|^Section |NSD_Create(?:Label|RadioButton)|MUI_HEADER_TEXT|MUI_DESCRIPTION_TEXT|GrxFirmaShowWarning|GrxFirmaExitOnExecFailure|GrxFirmaAbortUnsupportedWindows', line):
                    for literal in re.findall(r'"([^"\n]*)"', line):
                        self.assertTrue(literal.startswith('$') or literal in exceptions or literal in ('GrxFirma', 'Uninstall'), (path, literal))

    def test_compile_real_installers_with_eleven_languages(self):
        self.assertIsNotNone(shutil.which('makensis'), 'Se requiere makensis para comprobar los instaladores')
        with tempfile.TemporaryDirectory(prefix='grxfirma-nsis-i18n-') as temp:
            stage = Path(temp) / 'stage'
            stage.mkdir()
            for script in WINDOWS.glob('*.nsi'):
                source = script.read_text(encoding='utf-8-sig')
                for name in re.findall(r'^\s*File (?:/r )?"\$\{STAGE_DIR\}\\([^"\n]+)', source, re.M):
                    path = stage / name.replace('\\', '/')
                    if '*.*' in name:
                        path = path.parent / 'fixture.dll'
                    if not path.suffix:
                        path /= 'fixture.txt'
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.touch()
            (stage / 'assets').mkdir(exist_ok=True)
            for icon in (stage / 'grxfirma-grx.ico', stage / 'assets/grxfirma-grx.ico'):
                shutil.copyfile(WINDOWS / 'grxfirma-grx.ico', icon)
            for script in WINDOWS.glob('*.nsi'):
                # Suite con ambos frontends y también sin componentes opcionales.
                modes = [[], ['-DHAS_QT', '-DHAS_WINUI']] if script.name == 'grxfirma-suite.nsi' else [[]]
                for mode in modes:
                    with self.subTest(script=script.name, mode=mode):
                        result = subprocess.run(['makensis', '-NOCD', '-V3',
                            f'-DSTAGE_DIR={stage}', f'-DOUT_FILE={temp}/setup.exe', *mode, str(script)],
                            cwd=WINDOWS, capture_output=True, text=True, timeout=30)
                        output = result.stdout + result.stderr
                        self.assertEqual(result.returncode, 0, output)
                        self.assertIn('11 language tables', output)
                        self.assertNotIn('warning ', output.lower())


if __name__ == '__main__':
    unittest.main()
