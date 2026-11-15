#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 39f: foco inicial del aviso, Verificar conserva su estado y el
recibo distingue el aviso de Configuración. Contrato estático (sin dotnet en
Linux); la lógica de Core se prueba con MSTest en Windows."""

import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core"


def read(path):
    return path.read_text(encoding="utf-8")


def block_after(code, marker):
    start = code.index(marker) + len(marker)
    opening = code.index("{", start)
    depth = 0
    for index in range(opening, len(code)):
        if code[index] == "{":
            depth += 1
        elif code[index] == "}":
            depth -= 1
            if depth == 0:
                return code[opening:index + 1]
    raise AssertionError("bloque sin cerrar tras " + marker)


class DialogFocus(unittest.TestCase):
    def setUp(self):
        self.code = read(UI / "Controls/UnsafeExceptionDialog.cs")

    def test_focus_starts_on_the_case_text_not_on_more_information(self):
        scroll = self.code.split("var scroll = new ScrollViewer", 1)[1].split("};", 1)[0]
        self.assertIn("IsTabStop = true", scroll)
        self.assertIn("UseSystemFocusVisuals = true", scroll)
        self.assertIn("AutomationProperties.SetName(scroll, heading.Text);", self.code)
        opened = block_after(self.code, "dialog.Opened += (_, _) =>")
        self.assertIn("DispatcherQueuePriority.Low", opened)
        self.assertIn("scroll.Focus(FocusState.Keyboard)", opened)
        self.assertNotIn("acceptance.Focus", opened)
        self.assertNotIn("help.Focus", self.code)

    def test_more_information_hands_focus_to_the_help_text(self):
        click = block_after(self.code, "help.Click += (_, _) =>")
        self.assertLess(click.index("scroll.Focus("), click.index("help.Visibility = Visibility.Collapsed"))

    def test_enter_on_checkbox_does_not_activate_and_escape_cancels(self):
        self.assertIn("DefaultButton = ContentDialogButton.None", self.code)
        self.assertIn("CloseButtonText =", self.code)
        self.assertIn("IsPrimaryButtonEnabled = false", self.code)


class VerifyKeepsItsState(unittest.TestCase):
    def test_view_model_outlives_the_page(self):
        app = read(UI / "App.xaml.cs")
        self.assertRegex(app, r"VerifyViewModel =>\s*_verifyViewModel \?\?= new ViewModels\.VerifyPageViewModel\(")
        page = read(UI / "Views/VerifyPage.xaml.cs")
        self.assertIn("ViewModel = ((App)Application.Current).VerifyViewModel;", page)
        self.assertNotIn("new VerifyPageViewModel(", page)
        # La página que se va no se queda escuchando al modelo compartido.
        self.assertNotRegex(page, r"ViewModel\.PropertyChanged \+= \(")
        loaded = block_after(page, "private void OnLoaded(")
        unloaded = block_after(page, "private void OnUnloaded(")
        self.assertIn("ViewModel.PropertyChanged += OnViewModelPropertyChanged;", loaded)
        self.assertIn("ViewModel.PropertyChanged -= OnViewModelPropertyChanged;", unloaded)
        self.assertIn("ViewModel.DiagnosticRequested -= OnDiagnosticRequested;", unloaded)

    def test_activating_the_page_does_not_clear_the_result(self):
        model = read(UI / "ViewModels/VerifyPageViewModel.cs")
        activate = block_after(model, "public void Activate()")
        deactivate = block_after(model, "public void Deactivate()")
        for body in (activate, deactivate):
            self.assertNotIn("ResetResult", body)
            self.assertNotIn("_signedFilePath =", body)

    def test_language_change_starts_verify_again(self):
        window = read(UI / "MainWindow.xaml.cs")
        apply = block_after(window, "internal void ApplyLanguagePreference(")
        self.assertLess(apply.index("if (!changed) return;"), apply.index("_app.ForgetVerifyState();"))


class ActivationOriginAndSignatures(unittest.TestCase):
    def test_client_sends_origin_and_never_replays_a_signature(self):
        client = read(CORE / "Operations/UnsafeExceptionsClient.cs")
        self.assertIn('[JsonStringEnumMemberName("aviso")] Warning,', client)
        self.assertIn('[JsonStringEnumMemberName("ajustes")] Settings,', client)
        self.assertIn('JsonConverter(typeof(JsonStringEnumConverter<UnsafeAcceptanceOrigin>))] UnsafeAcceptanceOrigin Origin', client)
        signing = client.split("SigningOperations { get; }", 1)[1].split("};", 1)[0]
        for action in ("Sign,", "SignMultiCosign,", "SignBatch,", "ProtectAndSign,"):
            self.assertIn("DesktopOperationActions." + action, signing)
        gate = block_after(client, "public async Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(")
        self.assertIn("SigningOperations.Contains(action)", gate)
        self.assertIn("fromBlock: true", gate)

    def test_settings_page_activation_is_from_settings(self):
        page = read(UI / "Views/SettingsPage.UnsafeExceptions.cs")
        self.assertIn("client.ActivateAsync(item, ct:", page)
        self.assertNotIn("OperationPending = true", page)


if __name__ == "__main__":
    unittest.main()
