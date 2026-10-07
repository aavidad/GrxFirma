# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Los elementos de listas y desplegables tienen nombre accesible legible.

El lector de pantalla anunciaba «ProtectionRecipientItem { Id = …, Label = …}»
(recorrido Windows 0.0.117, A4): el ToString() generado de un record.
"""

import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
VIEW_MODELS = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI" / "ViewModels"


def records(source: str):
    for match in re.finditer(r"public sealed record (\w+)", source):
        start = match.end()
        nxt = re.search(r"\n(?:public |    public sealed record )", source[start:])
        yield match.group(1), source[start:start + (nxt.start() if nxt else len(source))]


class ListItemNameContractTests(unittest.TestCase):
    def test_records_shown_in_lists_override_to_string(self) -> None:
        checked = 0
        for path in sorted(VIEW_MODELS.glob("*.cs")):
            for name, body in records(path.read_text(encoding="utf-8")):
                if "Label" not in body and "DocumentName" not in body:
                    continue
                if name == "SignConfirmationSummary":
                    continue
                checked += 1
                with self.subTest(record=name):
                    self.assertIn("override string ToString()", body)
        self.assertGreaterEqual(checked, 15)

    def test_certificate_item_announces_name_status_and_expiry(self) -> None:
        # W-11: el elemento de la lista se anuncia con lo mismo que muestra la
        # tarjeta, no solo con el nombre.
        source = (VIEW_MODELS / "CertificatesPageViewModel.cs").read_text(encoding="utf-8")
        body = dict(records(source))["CertificateListItem"]
        to_string = body[body.index("override string ToString()"):]
        to_string = to_string[:to_string.index(";")]
        for part in ("DisplayName", "CardStatusText", "ExpirationDisplay"):
            self.assertIn(part, to_string)

    def test_recipient_announces_label_and_detail(self) -> None:
        source = (VIEW_MODELS / "ProtectPageViewModel.cs").read_text(encoding="utf-8")
        self.assertIn('public override string ToString() => Label + ". " + Detail;', source)


if __name__ == "__main__":
    unittest.main()
