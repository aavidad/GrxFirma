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


if __name__ == "__main__":
    unittest.main()
