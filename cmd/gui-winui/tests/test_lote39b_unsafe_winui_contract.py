#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 39b: WinUI tras el recorrido real de lo inseguro (15/11/2026).
Arranque en Windows 10, bloqueo al verificar, foco en Configuración, aviso
viejo y diálogo abierto desde Configuración. Sin dotnet en Linux: contrato
estático; la lógica de Core se prueba con MSTest en Windows."""

import json
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"


def read(path):
    return path.read_text(encoding="utf-8")


def block_after(code, marker):
    """Devuelve el bloque entre llaves que sigue a «marker»."""
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


class StartupOnWindows10(unittest.TestCase):
    def test_high_contrast_subscription_cannot_close_the_app(self):
        code = read(UI / "Controls/ThemeBrushes.cs")
        # Cada suscripción a una señal del sistema va dentro de TrySubscribe.
        for signal in ("HighContrastChanged +=", "ColorValuesChanged +="):
            position = code.index(signal)
            guard = code.rfind("OptionalSystemSignal.TrySubscribe(", 0, position)
            self.assertGreater(guard, -1, signal)
            self.assertNotIn("});", code[guard:position], signal + " fuera de TrySubscribe")
        helper = read(CORE / "Layout/OptionalSystemSignal.cs")
        self.assertIn("catch (Exception exception) when (exception is not OutOfMemoryException)", helper)

    def test_brushes_repaint_when_returning_to_the_window(self):
        window = read(UI / "MainWindow.xaml.cs")
        self.assertIn("Activated += OnWindowActivated;", window)
        self.assertIn("Activated -= OnWindowActivated;", window)
        handler = block_after(window, "private void OnWindowActivated(")
        self.assertIn("ThemeBrushes.RefreshFromSystem()", handler)


class BlockedVerification(unittest.TestCase):
    def test_cancelled_block_is_not_a_failure_to_diagnose(self):
        model = read(UI / "ViewModels/VerifyPageViewModel.cs")
        verify = block_after(model, "public async Task VerifyAsync()")
        block = block_after(verify, "if (!result.IsSuccess && result.UnsafeException is { } block)")
        self.assertNotIn("RequestDiagnostic", block)
        self.assertIn("InfoBarSeverity.Warning", block)
        self.assertLess(verify.index("result.UnsafeException is { } block"),
                        verify.index("if (!result.IsSuccess || result.Outcome != \"success\")"))

    def test_inconsistent_envelope_explains_itself(self):
        client = read(CORE / "Ipc/NdjsonIpcClient.cs")
        self.assertIn("IpcClientException.InconsistentResponse(reason ?? ", client)
        exception = read(CORE / "Ipc/IpcClientException.cs")
        factory = exception[exception.index("InconsistentResponse(string reason)"):]
        factory = factory[:factory.index(";")]
        self.assertIn("La interfaz y el motor local usan versiones incompatibles.", factory)
        self.assertIn("reason: reason", factory)
        mapper = read(CORE / "Diagnostics/OperationDiagnosticMapper.cs")
        self.assertIn("NormalizeTechnicalCode(exception.Reason)", mapper)
        es = json.loads(read(LOCALES / "es.json"))
        self.assertIn("La interfaz y el motor local usan versiones incompatibles.", es)


class SettingsRows(unittest.TestCase):
    def setUp(self):
        self.code = read(UI / "Views/SettingsPage.UnsafeExceptions.cs")

    def test_rows_are_updated_in_place_and_focus_stays_on_the_row(self):
        load = block_after(self.code, "private async Task LoadUnsafeExceptionsAsync(")
        # Solo se rehacen las filas si cambia la lista de casos.
        clear = load.index("UnsafeRows.Children.Clear()")
        self.assertIn("SequenceEqual", load[:clear])
        self.assertIn("UpdateUnsafeRow(", load)
        self.assertIn("KeepUnsafeFocus(changed)", load)
        focus = block_after(self.code, "private void KeepUnsafeFocus(")
        self.assertIn("FocusManager.GetFocusedElement(XamlRoot)", focus)
        self.assertIn("StartBringIntoView()", focus)

    def test_focused_buttons_are_never_disabled_while_busy(self):
        # Un botón con el foco que se desactiva lo pierde: así saltaba a «Idioma».
        self.assertNotRegex(self.code, r"\.Change\.IsEnabled\s*=\s*false")
        self.assertNotRegex(self.code, r"UnsafeRefresh\.IsEnabled\s*=\s*false")
        self.assertIn("if (_unsafeBusy) return;", self.code)

    def test_change_is_confirmed_in_the_live_region(self):
        load = block_after(self.code, "private async Task LoadUnsafeExceptionsAsync(")
        # La puntuación «etiqueta: valor» sale del catálogo de cada idioma (lote 39h).
        self.assertIn('Localizer.LabelValue(changed.Title.Text, changed.Status.Text)', load)
        xaml = read(UI / "Views/SettingsPage.xaml")
        self.assertRegex(xaml, r'x:Name="UnsafeMessage"\s+AutomationProperties.LiveSetting="Polite"')


class StaleNotice(unittest.TestCase):
    def test_notice_is_cleared_on_new_operation_and_setting_change(self):
        window = read(UI / "MainWindow.xaml.cs")
        clear = block_after(window, "internal void ClearUnsafeNotice()")
        self.assertIn("UnsafeResultNotice.IsOpen = false;", clear)
        app = read(UI / "App.xaml.cs")
        self.assertIn("ClearNotice = () => EnqueueOnUi(() => _window?.ClearUnsafeNotice()),", app)
        client = read(CORE / "Operations/UnsafeExceptionsClient.cs")
        send = block_after(client, "public async Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(")
        self.assertLess(send.index("ClearNotice?.Invoke()"), send.index("await inner.SendAsync"))


class DialogFromSettings(unittest.TestCase):
    def test_primary_is_highlighted_without_being_default(self):
        dialog = read(UI / "Controls/UnsafeExceptionDialog.cs")
        self.assertIn('TryGetValue("AccentButtonStyle"', dialog)
        self.assertIn("PrimaryButtonStyle =", dialog)
        # Intro sobre la casilla sigue sin activar nada.
        self.assertIn("DefaultButton = ContentDialogButton.None", dialog)
        self.assertIn("Localizer.Text(model.PrimaryButtonKey)", dialog)
        self.assertIn("Localizer.Text(model.TitleKey(helpOnly))", dialog)
        self.assertNotIn('"excepciones.activar_continuar"', dialog)

    def test_keys_exist_in_every_catalog(self):
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(read(path))
            for key in ("excepciones.pantalla.activar", "excepciones.activar_continuar",
                        "excepciones.pantalla.aceptar_titulo", "excepciones.pantalla.titulo",
                        "La interfaz y el motor local usan versiones incompatibles."):
                self.assertTrue(catalog.get(key), f"{path.name}: {key}")


if __name__ == "__main__":
    unittest.main()
