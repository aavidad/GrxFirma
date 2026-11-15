# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Cobertura de catálogos y ausencia de prosa visible en el código del instalador.
import json
from pathlib import Path
import re
import subprocess
import unittest

LINUX = Path(__file__).resolve().parents[1]
LOCALES = ('es', 'en', 'ca', 'va', 'gl', 'eu', 'fr', 'de', 'it', 'pt', 'zh')


class InstallerCatalogs(unittest.TestCase):
    def test_coverage_and_arguments(self):
        catalogs = {locale: json.loads((LINUX / f'i18n/{locale}.json').read_text()) for locale in LOCALES}
        for locale, data in catalogs.items():
            self.assertEqual(set(data), set(catalogs['es']), locale)
            for key, value in data.items():
                self.assertTrue(value.strip(), (locale, key))
                self.assertEqual(sorted(re.findall(r'@\d+@', value)),
                                 sorted(re.findall(r'@\d+@', catalogs['es'][key])), (locale, key))
        for name in ('build-suite.sh', 'configure-browsers.sh', 'install-suite.sh', 'installer-i18n.sh', 'runtime-dependencies.sh'):
            source = (LINUX / name).read_text()
            for key in re.findall(r'(?:grx_message(?:_in en)?|rtd_message) ([A-Z_0-9]+)', source):
                self.assertIn(key, catalogs['es'])
        subprocess.run(['python3', str(LINUX / 'i18n/generate.py'), '--check'], check=True)

    def test_no_embedded_spanish_prose(self):
        # Las claves internas, nombres de protocolos, variables y rutas no
        # son prosa visible. No se permiten excepciones de frases completas.
        exceptions = json.loads((LINUX / 'i18n/code-exceptions.json').read_text())
        self.assertTrue(all(exceptions.values()))
        spanish = re.compile(r'[áéíóúñ¿¡]|\b(?:Aviso:|error: .*no |Instalando |Reinstalando |Actualizando |Compilando |Uso:|uso:|Escriba |Se mantiene |Preguntar cada vez|Desde ahora|no se pudo|detectado|configurado|retirado|devuelto)')
        for name in ('build-suite.sh', 'configure-browsers.sh', 'install-suite.sh', 'installer-i18n.sh'):
            for number, line in enumerate((LINUX / name).read_text().splitlines(), 1):
                if line.lstrip().startswith(('#', '//')):
                    continue
                line = re.sub(r'\s+#.*$', '', line)
                for allowed in exceptions:
                    line = line.replace(allowed, '')
                self.assertIsNone(spanish.search(line), f'{name}:{number}: {line}')
                # Todos los puntos de salida con prosa deben llamar al catálogo.
                if re.match(r'\s*(echo|log|printf) ', line) and 'grx_message' not in line:
                    literals = re.findall(r'''["']([^"']*)["']''', line)
                    for value in literals:
                        if ' ' in value:
                            self.assertFalse(re.search('[A-Za-z]{3} [A-Za-z]{3}', value), f'{name}:{number}: {value}')

    def test_runtime_check_has_no_prose(self):
        # Lo que se ejecuta en el equipo de la persona usuaria no lleva texto propio.
        source = (LINUX / 'runtime-dependencies.sh').read_text()
        start = source.index('rtd_message() {')
        end = source.index('runtime_dependencies_main() {')
        for line in source[start:end].splitlines():
            if line.lstrip().startswith('#'):
                continue
            self.assertIsNone(re.search(r'\b(?:echo|printf)\b.*"[^"$%]*[a-z]{3} [a-z]{3}', line), line)
            self.assertIsNone(re.search(r'[áéíóúñ]|\berror:', line), line)


if __name__ == '__main__':
    unittest.main()
