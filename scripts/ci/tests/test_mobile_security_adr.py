# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""LD-03: la ADR de seguridad móvil tiene que decir lo mismo que el manifiesto.

Si la APK pide `INTERNET`, la ADR no puede decir que no lo pide y tiene que
explicar que las conexiones las abre el núcleo Go, que no obedece a la
configuración de seguridad de red de Android.
"""

from __future__ import annotations

import pathlib
import re
import unittest
import xml.etree.ElementTree as ET


ROOT = pathlib.Path(__file__).resolve().parents[3]
MANIFEST = ROOT / "mobile/android/app/src/main/AndroidManifest.xml"
ADR = ROOT / "docs/ADR-mobile-security.md"
ANDROID_NS = "{http://schemas.android.com/apk/res/android}"


def declared_permissions() -> set[str]:
    tree = ET.parse(MANIFEST)
    return {
        element.get(f"{ANDROID_NS}name", "")
        for element in tree.getroot().iter("uses-permission")
    }


def normalized_adr() -> str:
    return re.sub(r"\s+", " ", ADR.read_text(encoding="utf-8"))


class MobileSecurityAdrContractTest(unittest.TestCase):
    def test_internet_permission_matches_adr(self) -> None:
        adr = normalized_adr()
        if "android.permission.INTERNET" not in declared_permissions():
            self.assertNotIn("Android pide `INTERNET`", adr)
            return
        for stale in (
            "no solicita `INTERNET`",
            "Sin permiso `INTERNET`",
            "ni permiso de red",
        ):
            self.assertNotIn(
                stale,
                adr,
                f"la APK declara INTERNET y la ADR sigue diciendo «{stale}»",
            )
        for required in (
            "`INTERNET`",
            "núcleo Go",
            "`usesCleartextTraffic=false`",
            "configuración de seguridad de red de Android",
        ):
            self.assertIn(required, adr, f"falta «{required}» en la ADR móvil")

    def test_adr_lists_every_outbound_connection(self) -> None:
        adr = normalized_adr()
        for connection in ("TSA", "LT/LTA", "OCSP", "CRL", "AEAT", "GitHub"):
            self.assertIn(connection, adr, f"la ADR móvil no menciona {connection}")


if __name__ == "__main__":
    unittest.main()
