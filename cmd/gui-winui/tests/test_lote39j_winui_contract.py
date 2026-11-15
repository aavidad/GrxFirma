#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 39j (vuelta 2 del 39b), WinUI.

E: tras «Activar y continuar» en una firma bloqueada, la página relanza la
firma pasando otra vez por la confirmación; un bloqueo nunca abre el
diagnóstico de averías.
B: los ajustes sin permiso o de otra cuenta enseñan, en cualquier lectura que
abra el diagnóstico (también la del arranque en Firmar), la explicación y la
orden entera en un campo copiable, sin el filtro de rutas ni «Exportar
informe». Sin dotnet en Linux: contrato estático; la lógica se prueba con
MSTest en Windows."""

import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core"


def read(path):
    return path.read_text(encoding="utf-8")


def method(code, signature):
    start = code.index(signature)
    brace = code.index("{", start)
    depth = 0
    for index in range(brace, len(code)):
        if code[index] == "{":
            depth += 1
        elif code[index] == "}":
            depth -= 1
            if depth == 0:
                return code[start:index + 1]
    raise AssertionError(signature)


class SignatureBlockContract(unittest.TestCase):
    def test_client_marks_activation_and_does_not_ask_to_press_again_for_signatures(self):
        client = read(CORE / "Operations/UnsafeExceptionsClient.cs")
        self.assertIn("UnsafeExceptionActivated = true", client)
        self.assertIn('if (!SigningOperations.Contains(action)) Notice?.Invoke("excepciones.pantalla.reenviar")', client)
        self.assertIn("public static class UnsafeBlock", client)
        self.assertIn("public bool UnsafeExceptionActivated", read(CORE / "Ipc/IpcContracts.cs"))

    def test_sign_view_model_handles_the_block_before_any_failure_diagnostic(self):
        code = read(UI / "ViewModels/SignPageViewModel.cs")
        for signature in ("public async Task<OperationDiagnostic?> SignAsync(",
                          "public async Task<OperationDiagnostic?> SignBatchAsync("):
            body = method(code, signature)
            self.assertIn("_unsafeResendRequested = false;", body, signature)
            block = body.index("HandleUnsafeBlock(result)")
            self.assertLess(block, body.index("OperationDiagnosticMapper.FromResult(result)", block), signature)
        handler = method(code, "private bool HandleUnsafeBlock<TData>(")
        self.assertNotIn("OperationDiagnostic", handler)

    def test_sign_page_relaunches_through_the_click_that_asks_for_confirmation(self):
        code = read(UI / "Views/SignPage.xaml.cs")
        self.assertIn("ViewModel.TakeUnsafeResend()", code)
        sign = method(code, "private async void OnSignClick(")
        self.assertIn("if (ResendAfterUnsafeActivation(diagnostic, cancellation))", sign)
        self.assertIn("OnSignClick(sender, args);", sign)
        batch = method(code, "private async void OnSignBatchClick(")
        self.assertIn("if (ResendAfterUnsafeActivation(diagnostic, cancellation))", batch)
        self.assertIn("OnSignBatchClick(sender, args);", batch)

    def test_protect_page_never_turns_a_block_into_a_failure_diagnostic(self):
        code = read(UI / "ViewModels/ProtectPageViewModel.cs")
        classify = code.index("switch (UnsafeBlock.Classify(result))")
        self.assertLess(classify, code.index("if (!IsSuccessful(result))", classify))
        self.assertIn('Localizer.Text("excepciones.pantalla.reenviar")', code[classify:classify + 800])


class SettingsAccessAtStartupContract(unittest.TestCase):
    def test_startup_reader_goes_through_the_mapper_and_the_dialog(self):
        sign = read(UI / "Views/SignPage.xaml.cs")
        self.assertIn("await CertificatePanel.RefreshAsync(cancellationToken)", sign)
        panel = read(UI / "ViewModels/CertificatesPageViewModel.cs")
        default = method(panel, "private async Task<OperationDiagnostic?>\n        RefreshDefaultCertificateAsync(")
        self.assertIn("GetSettingsAsync(cancellationToken)", default)
        self.assertIn("OperationDiagnosticMapper.FromResult(result)", default)

    def test_mapper_takes_the_catalog_text_by_error_code_outside_redaction(self):
        mapper = read(CORE / "Diagnostics/OperationDiagnosticMapper.cs")
        self.assertIn("SettingsAccessFailure.From(result.ErrorCode, catalogText ?? CatalogLocalizer.Shared.Text)", mapper)
        self.assertIn("SettingsAccess = settingsAccess", mapper)
        diagnostic = read(CORE / "Diagnostics/OperationDiagnostic.cs")
        field = diagnostic.index("public SettingsAccessFailure? SettingsAccess")
        self.assertIn("[JsonIgnore]", diagnostic[field - 80:field])

    def test_dialog_shows_the_whole_command_copyable_without_export(self):
        dialog = read(UI / "Controls/OperationDiagnosticDialog.xaml.cs")
        self.assertIn("if (diagnostic.SettingsAccess is { } settingsAccess) ShowSettingsAccess(settingsAccess);", dialog)
        body = method(dialog, "private void ShowSettingsAccess(")
        for needle in ('Localizer.Text("winui.ajustes.no_se_pudo_cargar")', "PrimaryButtonText = string.Empty",
                       "IsReadOnly = true", "command.SelectAll()", "Text = failure.Command",
                       "DefaultButton = ContentDialogButton.Close"):
            self.assertIn(needle, body)


if __name__ == "__main__":
    unittest.main()
