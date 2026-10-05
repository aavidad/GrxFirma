# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest

from winui_catalog import read_with_catalog


ROOT = Path(__file__).resolve().parents[3]
APP = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"


class TrayContractTests(unittest.TestCase):
    def test_close_hides_only_when_tray_is_available(self):
        source = read_with_catalog(APP / "App.xaml.cs")
        self.assertIn("_keepInTray = true", source)
        self.assertIn("_tray?.Installed != true", source)
        self.assertIn("args.Cancel = true", source)
        self.assertIn("sender.Hide()", source)
        self.assertIn("_exitRequested = true", source)
        self.assertIn("_lifetimeCancellation.Cancel()", source)
        self.assertIn("OperationSession.Detach(client)", source)

    def test_tray_menu_is_native_and_status_cannot_be_selected(self):
        source = read_with_catalog(APP / "Services/WindowsTrayIcon.cs")
        for item in (
            "Abrir GrxFirma",
            "Firmas desde portales: activo",
            "Ajustes",
            "Salir",
        ):
            self.assertIn(item, source)
        self.assertIn("MenuString | MenuDisabled", source)
        self.assertIn("ShellNotifyIcon(Add", source)
        self.assertIn("ShellNotifyIcon(Delete", source)
        self.assertIn("LeftDoubleClick", source)

    def test_restore_signal_is_session_local_sid_acl_and_data_free(self):
        source = read_with_catalog(APP / "Services/SingleInstanceSignal.cs")
        self.assertIn(r"Local\GrxFirma.WinUI.Show.{sid}", source)
        self.assertIn("WindowsIdentity.GetCurrent().User", source)
        self.assertIn("ConvertStringSecurityDescriptorToSecurityDescriptor", source)
        self.assertIn("SetEvent(_handle)", source)
        self.assertNotIn("NamedPipeServerStream", source)
        self.assertNotIn("signature", source.lower())

    def test_run_value_uses_fixed_absolute_launcher_and_hkcu(self):
        source = read_with_catalog(APP / "Services/WindowsStartupRegistration.cs")
        self.assertIn("Registry.CurrentUser", source)
        self.assertIn("grxfirma-gui.exe", source)
        self.assertIn("--frontend=winui --start-hidden", source)
        self.assertIn("Path.IsPathFullyQualified", source)
        self.assertIn("FileAttributes.ReparsePoint", source)
        self.assertIn("GetAccessRules", source)
        self.assertIn("DeleteValue(ValueName, false)", source)
        self.assertNotIn("CurrentMachine", source)


if __name__ == "__main__":
    unittest.main()
