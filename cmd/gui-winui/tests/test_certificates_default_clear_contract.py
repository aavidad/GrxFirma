# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
APP = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI"


class CertificatesDefaultClearContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.xaml = (APP / "Views" / "CertificatesPage.xaml").read_text(
            encoding="utf-8"
        )
        cls.code_behind = (
            APP / "Views" / "CertificatesPage.xaml.cs"
        ).read_text(encoding="utf-8")
        cls.view_model = (
            APP / "ViewModels" / "CertificatesPageViewModel.cs"
        ).read_text(encoding="utf-8")

    def test_clear_action_is_accessible_and_bound_to_safe_state(self) -> None:
        fragment = self.xaml.split(
            'AutomationProperties.Name="Quitar predeterminado"',
            maxsplit=1,
        )[1].split("</Button>", maxsplit=1)[0]
        self.assertIn('Click="OnClearDefaultCertificateClick"', fragment)
        self.assertIn(
            "ViewModel.CanClearDefaultCertificate",
            fragment,
        )
        self.assertIn(
            "ViewModel.ClearDefaultCertificateAsync(",
            self.code_behind,
        )

    def test_clear_preserves_settings_and_requires_confirmation(self) -> None:
        method = self.view_model.split(
            "ClearDefaultCertificateAsync(",
            maxsplit=1,
        )[1].split(
            "ValidateSelectedCertificateAsync(",
            maxsplit=1,
        )[0]
        for expected in (
            "operations.GetSettingsAsync(",
            "CreateSafeSaveSnapshot() with",
            "PreferDefaultCertificate = false",
            "DefaultCertificateId = string.Empty",
            "operations.SaveSettingsAsync(",
            "string.IsNullOrWhiteSpace(saveResult.Data)",
            "ApplyDefaultCertificate(null)",
        ):
            self.assertIn(expected, method)

    def test_clear_is_only_enabled_for_a_saved_default(self) -> None:
        self.assertIn(
            "_hasDefaultCertificate = normalizedId is not null",
            self.view_model,
        )
        command_state = self.view_model.split(
            "CanClearDefaultCertificate =",
            maxsplit=1,
        )[1].split(
            "CanValidateSelectedCertificate =",
            maxsplit=1,
        )[0]
        for expected in (
            "IsOperationConnected",
            "!IsBusy",
            "_hasDefaultCertificate",
            "DesktopOperationActions.GetSettings",
            "DesktopOperationActions.SaveSettings",
        ):
            self.assertIn(expected, command_state)


if __name__ == "__main__":
    unittest.main()
