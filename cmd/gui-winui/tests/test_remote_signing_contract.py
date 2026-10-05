# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Firma remota CSC en WinUI: mismo comportamiento que Qt y sin secretos retenidos."""

import json
import re
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[3]
SRC = ROOT / "cmd" / "gui-winui" / "src"
DIALOGS = (SRC / "GrxFirma.WinUI" / "Views" / "RemoteSigningDialogs.cs").read_text(encoding="utf-8")
SIGN_PAGE = (SRC / "GrxFirma.WinUI" / "Views" / "SignPage.xaml.cs").read_text(encoding="utf-8")
SIGN_XAML = (SRC / "GrxFirma.WinUI" / "Views" / "SignPage.xaml").read_text(encoding="utf-8")
VIEW_MODEL = (SRC / "GrxFirma.WinUI" / "ViewModels" / "SignPageViewModel.cs").read_text(encoding="utf-8")
CORE_CONTRACTS = (SRC / "GrxFirma.WinUI.Core" / "Operations" / "DesktopRemoteSigningContracts.cs").read_text(encoding="utf-8")
CLIENT = (SRC / "GrxFirma.WinUI.Core" / "Operations" / "DesktopOperationsClient.cs").read_text(encoding="utf-8")
CATALOG = json.loads((ROOT / "internal" / "adapters" / "outbound" / "common" / "localizador"
                      / "locales" / "es.json").read_text(encoding="utf-8"))


class RemoteSigningContractTests(unittest.TestCase):
    def test_secrets_use_password_boxes_that_are_emptied(self) -> None:
        self.assertEqual(2, DIALOGS.count("new PasswordBox"))
        self.assertIn("pinBox.Password = string.Empty;", DIALOGS)
        self.assertIn("otpBox.Password = string.Empty;", DIALOGS)
        self.assertNotIn("new TextBox\n        {\n            Header = Localizer.Text(\"csc.gui.pin\")", DIALOGS)

    def test_hosts_are_shown_before_connect_is_offered(self) -> None:
        self.assertIn('HostLine("csc.gui.host_servicio"', DIALOGS)
        self.assertIn('HostLine("csc.gui.host_oauth"', DIALOGS)
        self.assertIn("connectButton.Visibility = discovered && !connected", DIALOGS)

    def test_button_follows_engine_and_secrets_are_disposed(self) -> None:
        self.assertIn('x:Name="RemoteSigningButton"', SIGN_XAML)
        self.assertIn('Visibility="Collapsed"', SIGN_XAML.split('x:Name="RemoteSigningButton"')[1][:600])
        self.assertIn("RemoteSigningDialogs.ShouldShowButtonAsync", SIGN_PAGE)
        self.assertIn("ViewModel.RemoteSecretsPrompt = PromptRemoteSecretsAsync;", SIGN_PAGE)
        self.assertEqual(2, VIEW_MODEL.count("remoteSecrets?.Dispose();"))
        self.assertIn('Localizer.Text("csc.error.otp_lote")', VIEW_MODEL)
        self.assertEqual(3, CLIENT.count("return SendWithRemoteSecretsAsync<"))
        self.assertIn("CryptographicOperations.ZeroMemory(remotePin)", CLIENT)

    def test_policy_prohibition_is_explained_without_offering_config(self) -> None:
        self.assertIn("status.Data?.ProhibitedByPolicy == true", DIALOGS)
        self.assertIn('Localizer.Text("csc.error.prohibida")', DIALOGS)
        guard = DIALOGS.index("if (current is { ProhibitedByPolicy: true })")
        self.assertLess(guard, DIALOGS.index("urlBox.Text = current.ServiceUrl;"))
        self.assertIn('"prohibitedByPolicy"', CORE_CONTRACTS)

    def test_every_key_exists_in_the_catalog(self) -> None:
        keys = set(re.findall(r'"(csc\.(?:gui|error)\.[a-z_]+)"', DIALOGS + SIGN_XAML + VIEW_MODEL))
        self.assertTrue(keys)
        self.assertEqual(set(), keys - set(CATALOG))


if __name__ == "__main__":
    unittest.main()
