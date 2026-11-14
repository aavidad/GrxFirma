#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Regresiones de cobertura: las dependencias nuevas no heredan exenciones."""

import copy
import importlib.util
import json
import tempfile
import unittest
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location("dossier", ROOT / "scripts/compliance/dependency_dossier.py")
dossier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dossier)
spec_db = importlib.util.spec_from_file_location("localdb", ROOT / "scripts/compliance/prepare_govulncheck_db.py")
localdb = importlib.util.module_from_spec(spec_db)
spec_db.loader.exec_module(localdb)


class CoverageTests(unittest.TestCase):
    def setUp(self):
        self.rows, _ = dossier.collect(ROOT)
        self.registry = json.loads((ROOT / dossier.REGISTRY).read_text())["dependencies"]

    def test_current_inventory_and_baseline_debt_are_visible(self):
        pending = dossier.validate(self.rows, self.registry)
        self.assertTrue(pending)
        with self.assertRaisesRegex(ValueError, "pendiente"):
            dossier.validate(self.rows, self.registry, strict=True)

    def test_new_dependency_cannot_inherit_baseline_waiver(self):
        rows = copy.deepcopy(self.rows)
        rows[0]["id"] += "-version-nueva"
        with self.assertRaisesRegex(ValueError, "nueva sin revisión"):
            dossier.validate(rows, self.registry)

    def test_new_pending_is_not_a_valid_license(self):
        row = copy.deepcopy(self.rows[0])
        review = copy.deepcopy(self.registry[row["id"]])
        row["id"] += "-version-nueva"
        review["licenses"] = []
        review["baseline_pending"] = "Propuesta sin licencia, no aceptada"
        with self.assertRaisesRegex(ValueError, "licencia ausente"):
            dossier.validate([row], {row["id"]: review})

    def test_missing_license_fails(self):
        registry = copy.deepcopy(self.registry)
        key = next(r["id"] for r in self.rows if registry[r["id"]]["licenses"])
        registry[key]["licenses"] = []
        with self.assertRaisesRegex(ValueError, "licencia ausente"):
            dossier.validate(self.rows, registry)

    def test_missing_or_changed_use_fails(self):
        for uses in ([], ["sin clasificar"], ["pruebas"]):
            registry = copy.deepcopy(self.registry)
            key = next(r["id"] for r in self.rows if registry[r["id"]]["uses"] == ["ejecucion"])
            registry[key]["uses"] = uses
            with self.assertRaisesRegex(ValueError, "uso sin clasificar"):
                dossier.validate(self.rows, registry)

    def test_missing_license_provenance_fails(self):
        registry = copy.deepcopy(self.registry)
        registry[self.rows[0]["id"]]["license_evidence"] = []
        with self.assertRaisesRegex(ValueError, "procedencia"):
            dossier.validate(self.rows, registry)


class LocalDatabaseTests(unittest.TestCase):
    def test_only_official_entries_and_no_path_traversal(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive = root / "db.zip"
            entry = b'{"id":"GO-2026-1"}'
            with zipfile.ZipFile(archive, "w") as z:
                z.writestr("GO-2026-1.json", entry)
                z.writestr("../GO-2026-2.json", '{}')
                z.writestr("GHSA-x.json", '{}')
            result = localdb.prepare(archive, root / "db")
            self.assertEqual(result["entries"], 1)
            self.assertEqual((root / "db/GO-2026-1.json").read_bytes(), entry)
            self.assertFalse((root / "GO-2026-2.json").exists())
            with self.assertRaisesRegex(ValueError, "vacío"):
                localdb.prepare(archive, root / "db")

    def test_empty_database_is_not_valid_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "empty.zip"
            with zipfile.ZipFile(archive, "w"):
                pass
            with self.assertRaisesRegex(ValueError, "vacía"):
                localdb.prepare(archive, Path(directory) / "db")


if __name__ == "__main__":
    unittest.main()
