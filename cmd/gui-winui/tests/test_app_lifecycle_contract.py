# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
APP = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI" / "App.xaml.cs"
APP_XAML = (
    ROOT
    / "cmd"
    / "gui-winui"
    / "src"
    / "GrxFirma.WinUI"
    / "App.xaml"
)
MAIN_WINDOW = (
    ROOT
    / "cmd"
    / "gui-winui"
    / "src"
    / "GrxFirma.WinUI"
    / "MainWindow.xaml.cs"
)
MAIN_VIEW_MODEL = (
    ROOT
    / "cmd"
    / "gui-winui"
    / "src"
    / "GrxFirma.WinUI"
    / "ViewModels"
    / "MainWindowViewModel.cs"
)


class AppLifecycleContractTests(unittest.TestCase):
    def test_pending_connection_is_owned_by_window_lifetime(self) -> None:
        source = APP.read_text(encoding="utf-8")

        self.assertIn(
            "CancellationTokenSource _lifetimeCancellation",
            source,
        )
        self.assertIn(
            "cancellationToken: _lifetimeCancellation.Token",
            source,
        )
        self.assertIn("_lifetimeCancellation.Cancel();", source)
        self.assertIn(
            "Interlocked.Exchange(ref _ipcClient, null)",
            source,
        )
        self.assertIn("Volatile.Read(ref _windowClosed)", source)
        self.assertIn(
            "OperationSession.Attach(client)",
            source,
        )
        self.assertIn(
            "OperationSession.Detach(client)",
            source,
        )
        self.assertIn(
            "FilePickerService = new WindowsFilePickerService(_window)",
            source,
        )

    def test_file_picker_is_ready_before_initial_page_is_created(self) -> None:
        source = APP.read_text(encoding="utf-8")
        main_window = MAIN_WINDOW.read_text(encoding="utf-8")

        picker_index = source.index(
            "FilePickerService = new WindowsFilePickerService(_window)"
        )
        navigation_index = source.index("_window.ShowInitialPage()")

        self.assertLess(picker_index, navigation_index)
        constructor = main_window.split(
            "internal void ShowInitialPage()",
            maxsplit=1,
        )[0]
        self.assertNotIn("ContentFrame.Navigate", constructor)
        self.assertIn(
            "ContentFrame.Navigate(typeof(SignPage))",
            main_window,
        )

    def test_winui_control_resources_are_loaded_before_main_window(self) -> None:
        resources = APP_XAML.read_text(encoding="utf-8")

        self.assertIn(
            "<controls:XamlControlsResources />",
            resources,
        )
        self.assertIn(
            'xmlns:controls="using:Microsoft.UI.Xaml.Controls"',
            resources,
        )

    def test_safe_exception_attribution_reaches_visual_diagnostic(self) -> None:
        source = APP.read_text(encoding="utf-8")
        view_model = MAIN_VIEW_MODEL.read_text(encoding="utf-8")

        self.assertIn("exception.Phase", source)
        self.assertIn("exception.LikelyOwner", source)
        self.assertIn('phase == "protocol"', view_model)
        self.assertIn("LikelyOwner = safeOwner", view_model)
        self.assertIn('EvidenceRef = $"phase:{safePhase}"', view_model)


if __name__ == "__main__":
    unittest.main()
