#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Los avisos de compatibilidad al desproteger llegan a la persona."""

import json
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
WINUI_VM = (
    ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/ProtectPageViewModel.cs"
).read_text(encoding="utf-8")
WINUI_XAML = (
    ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/Views/ProtectPage.xaml"
).read_text(encoding="utf-8")
WINUI_CONTRACT = (
    ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations/DesktopOperationContracts.cs"
).read_text(encoding="utf-8")


class UnprotectWarningsContract(unittest.TestCase):
    def test_qt_pinta_los_avisos_del_resultado(self):
        self.assertIn("id: unprotectWarningsBox", QML)
        # El aviso solo se enseña con la operación hecha, no en un error.
        self.assertIn(
            "!window.unprotectResult.error && window.unprotectResult.warnings", QML
        )

    def test_winui_pinta_los_avisos_del_resultado(self):
        registro = WINUI_CONTRACT.split("record UnprotectResult")[1].split(
            "record SignResult"
        )[0]
        self.assertIn('[JsonPropertyName("warnings")]', registro)
        self.assertIn("result.Data.VisibleWarnings", WINUI_VM)
        self.assertIn("UnprotectWarningMessage", WINUI_XAML)

    def test_winui_traduce_los_avisos_con_el_catalogo(self):
        # Lote 16d: los avisos llegan como claves del catálogo (la frase en
        # castellano); WinUI los pinta en el idioma de la persona, uno por
        # línea, igual que Qt con tr().
        bloque = WINUI_VM[WINUI_VM.index("UnprotectWarningMessage = string.Join("):]
        bloque = bloque[:bloque.index(";") + 1]
        self.assertIn("result.Data.VisibleWarnings.Select(Localizer.VisibleText)", bloque)
        self.assertNotIn('" ",', bloque)

    def test_aviso_del_perfil_alto_existe_en_los_once_catalogos(self):
        clave = "proteccion.alto.sin_almacen.aviso"
        locales = ROOT / "internal/adapters/outbound/common/localizador/locales"
        for idioma in ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh"):
            catalogo = json.loads((locales / (idioma + ".json")).read_text(encoding="utf-8"))
            self.assertTrue(catalogo[clave], idioma)
            self.assertNotEqual(catalogo[clave], clave, idioma)

    def test_qt_proteger_avisa_en_texto_plano_y_traduce(self):
        bloque = QML.split("id: protectWarningsBox")[1].split("ThemedButton {")[0]
        self.assertIn("!window.protectResult.error && window.protectResult.warnings", bloque)
        self.assertIn("visible: avisos.length > 0", bloque)
        self.assertIn("Accessible.role: Accessible.AlertMessage", bloque)
        self.assertIn("textFormat: Text.PlainText", bloque)
        self.assertIn("text: tr(String(modelData))", bloque)
        self.assertIn("wrapMode: Text.WordWrap", bloque)

    def test_winui_proteger_contrato_y_aviso_traducido(self):
        contrato = WINUI_CONTRACT.split("record ProtectionResult")[1].split("record UnprotectResult")[0]
        self.assertIn('[JsonPropertyName("warnings")]', contrato)
        self.assertIn("OperationResultText.CleanList(Warnings)", contrato)
        bloque = WINUI_VM.split("ProtectWarningMessage = string.Join(")[1].split(";")[0]
        self.assertIn("Environment.NewLine", bloque)
        self.assertIn("result.Data.VisibleWarnings.Select(Localizer.VisibleText)", bloque)
        limpieza = WINUI_VM.split("private void ClearProtectedOutput()")[1].split("private void ClearUnprotectedOutput()")[0]
        self.assertIn("ProtectWarningMessage = string.Empty", limpieza)
        self.assertIn('x:Load="{x:Bind ViewModel.HasProtectWarning, Mode=OneWay}"', WINUI_XAML)
        self.assertIn('Text="{x:Bind ViewModel.ProtectWarningMessage, Mode=OneWay}"', WINUI_XAML)

    def test_qt_puentes_conservan_el_resultado_completo(self):
        for nombre, marcador, expresion in [
            ("ipcbridge.cpp", 'if (action == "protect" || action == "protect_sign")', "res.toVariantMap()"),
            ("backendbridge.cpp", 'QJsonObject result = obj;', "result.toVariantMap()"),
        ]:
            fuente = (ROOT / "cmd/gui-qml" / nombre).read_text(encoding="utf-8")
            bloque = fuente[fuente.index(marcador):]
            self.assertIn("emit protectionFinished(true, msg, " + expresion + ");", bloque)

    def test_exportacion_de_identidad_alto_solo_cli_y_rest(self):
        # No crear una operación de exportación donde solo existe la de
        # certificados públicos X.509, que tiene un contrato distinto.
        ipc = (ROOT / "internal/adapters/inbound/desktop/ipc/handler.go").read_text(encoding="utf-8")
        self.assertNotIn('case "protection_recipient_export":', ipc)
        consola = (ROOT / "internal/adapters/inbound/common/rest/console_index.html").read_text(encoding="utf-8")
        for inicio, fin in [("async function exportProtectionRecipient()", "async function importProtectionRecipient()"), ("async function protectDocument()", "async function unprotectDocument()")]:
            bloque = consola.split(inicio)[1].split(fin)[0]
            self.assertIn('resultBanner("status-warn", response.warnings.join(" "))', bloque)
        banner = consola.split("function resultBanner(")[1].split("function ")[0]
        self.assertIn("escapeHtml(", banner)


if __name__ == "__main__":
    unittest.main()
