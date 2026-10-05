# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""El motor solo reemplaza un fichero existente si la persona lo confirmó.

WinUI elige las rutas con el selector de guardar del sistema, que ya pregunta
antes de reemplazar (ShowOverwritePrompt). Esa confirmación viaja como
``overwriteConfirmed``; nunca se fuerza la sobrescritura con ``overwrite``.
"""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
WINUI = ROOT / "cmd" / "gui-winui" / "src"


def read(relative: str) -> str:
    return (WINUI / relative).read_text(encoding="utf-8-sig")


class OverwriteConfirmationContractTests(unittest.TestCase):
    def test_save_picker_flows_send_explicit_confirmation(self) -> None:
        for relative in (
            "GrxFirma.WinUI/ViewModels/SignPageViewModel.cs",
            "GrxFirma.WinUI/ViewModels/ProtectPageViewModel.cs",
            "GrxFirma.WinUI/Views/PublicCertificateExport.cs",
        ):
            with self.subTest(relative=relative):
                self.assertIn("OverwriteConfirmed = true", read(relative))
        eni = read("GrxFirma.WinUI/Views/EniPage.xaml.cs")
        self.assertEqual(2, eni.count("overwriteConfirmed: true"))

    def test_no_client_forces_overwrite_through_the_policy_field(self) -> None:
        for path in WINUI.rglob("*.cs"):
            if "bin" in path.parts or "obj" in path.parts:
                continue
            text = path.read_text(encoding="utf-8-sig")
            with self.subTest(path=path.name):
                self.assertNotRegex(text, r'Overwrite\s*=\s*"(?:force|overwrite|true)"')

    def test_picker_still_asks_before_replacing(self) -> None:
        picker = read("GrxFirma.WinUI/Services/WindowsFilePickerService.cs")
        self.assertIn("ShowOverwritePrompt = true", picker)

    def test_wire_name_matches_go_engine(self) -> None:
        contracts = read("GrxFirma.WinUI.Core/Operations/DesktopOperationContracts.cs")
        self.assertEqual(3, contracts.count('JsonPropertyName("overwriteConfirmed")'))
        self.assertEqual(3, len(re.findall(r"WhenWritingDefault\)\]\s*public bool OverwriteConfirmed", contracts)))
        go_types = (ROOT / "internal/adapters/inbound/desktop/ipc/types.go").read_text(encoding="utf-8")
        self.assertIn('json:"overwriteConfirmed,omitempty"', go_types)
        client = read("GrxFirma.WinUI.Core/Operations/DesktopOperationsClient.cs")
        self.assertEqual(2, client.count("options, overwriteConfirmed }"))


if __name__ == "__main__":
    unittest.main()
