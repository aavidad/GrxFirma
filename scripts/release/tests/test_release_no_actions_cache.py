# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""JD-01 (G-01 en la vía SignPath): la publicación no restaura caché de Actions.

La caché de Actions la comparten `main` y las etiquetas. Un job de `main` con
código ajeno (por ejemplo una dependencia npm) puede dejar en ella un Qt6Core.dll
modificado y sin firma que el job de la etiqueta restauraría y que SignPath
firmaría con el certificado oficial. Por eso ningún job de release.yml restaura
caché, y `check-official-release-policy.py` lo exige.
"""

from __future__ import annotations

import importlib.util
from pathlib import Path
import re
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[3]
WORKFLOW = ROOT / ".github" / "workflows" / "release.yml"
SCRIPT = ROOT / "scripts" / "release" / "check-official-release-policy.py"

SPEC = importlib.util.spec_from_file_location("check_official_release_policy", SCRIPT)
assert SPEC and SPEC.loader
POLICY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(POLICY)


def pasos(texto: str):
    """(job, bloque del paso) de cada paso del workflow."""
    trabajos = re.split(r"(?m)^  (?=[A-Za-z0-9_-]+:\s*$)", texto.split("\njobs:\n", 1)[1])
    for trabajo in trabajos:
        if not trabajo.strip():
            continue
        nombre = trabajo.split(":", 1)[0]
        for paso in re.split(r"(?m)^      - ", trabajo)[1:]:
            yield nombre, paso


class ReleaseNoActionsCacheTest(unittest.TestCase):
    def test_setup_go_e_install_qt_sin_cache(self):
        vistos = {"setup-go": 0, "install-qt-action": 0}
        for trabajo, paso in pasos(WORKFLOW.read_text(encoding="utf-8")):
            uso = re.search(r"(?m)^\s*uses:\s*(\S+)", paso)
            if not uso:
                continue
            accion = uso.group(1)
            with self.subTest(job=trabajo, accion=accion):
                self.assertNotIn("actions/cache@", accion)
                if "/setup-go@" in accion or "/install-qt-action@" in accion:
                    vistos["setup-go" if "setup-go" in accion else "install-qt-action"] += 1
                    self.assertRegex(paso, r"(?m)^\s+cache:\s*false\s*$")
                if "/setup-node@" in accion:
                    self.assertRegex(paso, r"(?m)^\s+package-manager-cache:\s*false\s*$")
                self.assertNotRegex(paso, r"(?m)^\s+cache:\s*true\s*$")
        self.assertGreaterEqual(vistos["setup-go"], 7)
        self.assertEqual(vistos["install-qt-action"], 2)

    def test_la_politica_rechaza_la_cache(self):
        original = WORKFLOW.read_text(encoding="utf-8")
        self.assertEqual(POLICY.release_cache_violations(original), [])
        casos = {
            "Qt de Windows": original.replace(
                'arch: win64_msvc2019_64\n          cache: false',
                'arch: win64_msvc2019_64\n          cache: true',
                1,
            ),
            "Go sin cache: false": original.replace(
                "go-version-file: go.mod\n          cache: false\n",
                "go-version-file: go.mod\n",
                1,
            ),
            "actions/cache": original.replace(
                "      - name: Checkout\n",
                "      - name: Cache\n        uses: actions/cache@0000000000000000000000000000000000000000\n"
                "      - name: Checkout\n",
                1,
            ),
        }
        for nombre, texto in casos.items():
            with self.subTest(caso=nombre):
                self.assertNotEqual(texto, original, "el caso no cambió el workflow")
                self.assertTrue(POLICY.release_cache_violations(texto))

    def test_main_falla_con_cache(self):
        original = WORKFLOW.read_text(encoding="utf-8")
        alterado = original.replace(
            'version: "6.7.3"\n          cache: false\n',
            'version: "6.7.3"\n          cache: true\n',
            1,
        )
        self.assertNotEqual(alterado, original)
        with tempfile.TemporaryDirectory() as directorio:
            copia = Path(directorio) / "release.yml"
            copia.write_text(alterado, encoding="utf-8")
            with mock.patch.object(POLICY, "WORKFLOW", copia):
                with self.assertRaises(SystemExit) as error:
                    POLICY.main()
        self.assertIn("cache", str(error.exception))


if __name__ == "__main__":
    unittest.main()
