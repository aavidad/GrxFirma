# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""G-11: el instalador avisa de que afirma:// deja de abrir AutoFirma."""

from __future__ import annotations

import unittest
from pathlib import Path


WINDOWS = Path(__file__).resolve().parents[1]


class AfirmaHandlerWarningTests(unittest.TestCase):
    def test_installers_warn_before_registering_afirma(self) -> None:
        for name in ("grxfirma-suite.nsi", "grxfirma-afirmauri.nsi"):
            text = (WINDOWS / name).read_text(encoding="utf-8-sig")
            with self.subTest(installer=name):
                warning = text.index("afirma:// de los portales, en lugar de AutoFirma")
                install = text.index("nsExec::ExecToLog", warning)
                self.assertIn("GrxFirmaShowWarning", text[warning - 200 : warning])
                self.assertIn("install-", text[install : install + 300])


if __name__ == "__main__":
    unittest.main()
