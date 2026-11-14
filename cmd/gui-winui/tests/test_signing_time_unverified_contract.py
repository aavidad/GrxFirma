#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""KD-01: WinUI enseña la fecha de un sello de tiempo sin comprobar
(``timestamp_unverified``) con su propio origen, nunca como «según el sello
de tiempo». Sin dotnet en Linux: contrato estático."""

import json
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations/SigningTimeText.cs"
VIEW_MODEL = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/VerifyPageViewModel.cs"
CORE_TESTS = ROOT / "cmd/gui-winui/tests/GrxFirma.WinUI.Core.Tests/SigningTimeTextTests.cs"
DOMAIN = ROOT / "internal/domain/verification.go"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
LANGUAGES = ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh")


class SigningTimeUnverifiedContract(unittest.TestCase):
    def test_las_fuentes_coinciden_con_las_del_motor(self):
        domain = DOMAIN.read_text(encoding="utf-8")
        engine = set(re.findall(r'SigningTimeSource\w+\s*=\s*"(\w+)"', domain))
        core = CORE.read_text(encoding="utf-8")
        winui = set(re.findall(r'public const string From\w+ = "(\w+)";', core))
        self.assertEqual(engine, winui)
        self.assertIn("FromUnverifiedTimestamp", core)
        accepted = core[core.index("source is not"):core.index("||", core.index("source is not"))]
        for name in ("FromTimestamp", "FromUnverifiedTimestamp", "FromSignedAttribute"):
            self.assertIn(name, accepted)

    def test_cada_origen_tiene_su_texto(self):
        source = VIEW_MODEL.read_text(encoding="utf-8")
        self.assertIn("SigningTimeText.FromUnverifiedTimestamp", source)
        self.assertIn('("origin", Localizer.Text(SigningTimeOriginKey(summary.SigningTimeSource)))', source)
        start = source.index("private static string SigningTimeOriginKey(")
        origin = source[start:source.index("};", start)]
        self.assertIn('"winui.verificar.fecha_sello_sin_comprobar"', origin)
        self.assertIn('"winui.verificar.fecha_segun_sello"', origin)
        self.assertIn('"winui.verificar.fecha_del_firmante"', origin)
        # El sello sin comprobar no cae en «según el sello de tiempo».
        unverified = origin.index("FromUnverifiedTimestamp")
        self.assertLess(unverified, origin.index('"winui.verificar.fecha_segun_sello"'))
        for language in LANGUAGES:
            catalog = json.loads((LOCALES / (language + ".json")).read_text(encoding="utf-8"))
            self.assertTrue(catalog.get("winui.verificar.fecha_sello_sin_comprobar"), language)
            self.assertNotEqual(catalog["winui.verificar.fecha_sello_sin_comprobar"],
                                catalog["winui.verificar.fecha_segun_sello"], language)

    def test_mstest_cubre_la_fuente_nueva(self):
        tests = CORE_TESTS.read_text(encoding="utf-8")
        self.assertIn('"timestamp_unverified"', tests)


if __name__ == "__main__":
    unittest.main()
