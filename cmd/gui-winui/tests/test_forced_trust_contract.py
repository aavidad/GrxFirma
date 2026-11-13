#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 14: aceptar en este equipo una firma que GrxFirma no da por válida (WinUI)."""

import json
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
SRC = ROOT / "cmd/gui-winui/src"
CORE = SRC / "GrxFirma.WinUI.Core/Operations"
APP = SRC / "GrxFirma.WinUI"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"


def leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


def metodo(texto: str, firma: str) -> str:
    inicio = texto.index(firma)
    flecha = texto.find("=>", inicio)
    llave = texto.find("{", inicio)
    if flecha != -1 and (llave == -1 or flecha < llave):
        # Miembro con cuerpo de expresión: llega hasta el primer «;» de fin de línea.
        return texto[inicio:texto.index(";\n", flecha) + 1]
    profundidad = 1
    pos = llave + 1
    while profundidad:
        if texto[pos] == "{":
            profundidad += 1
        elif texto[pos] == "}":
            profundidad -= 1
        pos += 1
    return texto[inicio:pos]


class ForcedTrustWinUiContractTest(unittest.TestCase):
    def test_contract_matches_ipc_schema(self) -> None:
        contratos = leer(CORE / "DesktopOperationContracts.cs")
        self.assertIn('[JsonPropertyName("forcedTrust")]', contratos)
        for campo in ("available", "forbidden", "applied", "acceptable", "acceptedAt", "entries", "items"):
            self.assertIn(f'[JsonPropertyName("{campo}")]', contratos)
        esquema = json.loads(leer(ROOT / "docs/schemas/desktop-ipc-v1.schema.json"))
        texto_esquema = json.dumps(esquema)
        cliente = leer(CORE / "DesktopOperationsClient.cs")
        for accion in ("forced_trust_list", "forced_trust_accept", "forced_trust_remove"):
            self.assertIn(f'"{accion}"', cliente)
            self.assertIn(accion, texto_esquema)

    def test_forced_signature_is_never_shown_as_trusted(self) -> None:
        evaluacion = leer(CORE / "VerificationAssessment.cs")
        forzada = metodo(evaluacion, "public static bool HasForcedTrust(")
        self.assertIn('"trust.user_forced"', forzada)
        self.assertIn('IsStatus(result.Integrity.Status, "valid")', forzada)
        self.assertIn('!IsStatus(result.Certificate.Status, "invalid")', forzada)
        self.assertIn("!HasXmlCanonicalizationCompatibility(result)", forzada)
        confianza = metodo(evaluacion, "public static bool HasEstablishedTrust(")
        self.assertIn("!HasForcedTrust(result)", confianza)
        coherente = metodo(evaluacion, "public static bool IsCoherent(")
        self.assertIn("HasForcedTrust(result)", coherente)

        vm = leer(APP / "ViewModels/VerifyPageViewModel.cs")
        presentar = metodo(vm, "private void PresentResult(")
        titulo = presentar.index("ResultTitle = VerificationPresentation.GetTitle(data);")
        forzado = presentar.index("VerificationAssessment.HasForcedTrust(data)")
        self.assertLess(titulo, forzado)
        bloque = presentar[forzado:]
        self.assertIn('CatalogText("verify.forced_trust.ui.outcome")', bloque)
        self.assertIn("ResultSeverity = InfoBarSeverity.Warning;", bloque)

    def test_button_only_when_backend_offers_and_policy_allows(self) -> None:
        vm = leer(APP / "ViewModels/VerifyPageViewModel.cs")
        ofrecer = metodo(vm, "private void PresentForcedTrust(")
        self.assertIn("!data.IsValid", ofrecer)
        self.assertIn("Available: true, Forbidden: false", ofrecer)
        estado = metodo(vm, "private void RefreshCommandState(")
        self.assertIn("ForcedTrustAcceptable.Count > 0", estado)
        self.assertIn("DesktopOperationActions.ForcedTrustAccept", estado)
        reinicio = metodo(vm, "private void ResetResult(")
        self.assertIn("ForcedTrustAcceptable = [];", reinicio)

        xaml = leer(APP / "Views/VerifyPage.xaml")
        self.assertIn('x:Name="ForcedTrustAcceptButton"', xaml)
        self.assertIn("ViewModel.CanAcceptForcedTrust", xaml)
        # El botón de aceptar no es el de acento: la acción principal sigue siendo verificar.
        boton = xaml[xaml.index('x:Name="ForcedTrustAcceptButton"'):]
        boton = boton[: boton.index("/>")]
        self.assertNotIn("AccentButtonStyle", boton)

    def test_dialog_shows_warning_and_cancel_is_default(self) -> None:
        pagina = leer(APP / "Views/VerifyPage.xaml.cs")
        dialogo = metodo(pagina, "private async void OnForcedTrustAcceptClick(")
        self.assertIn("DefaultButton = ContentDialogButton.Close", dialogo)
        self.assertIn("item.Notice", dialogo)
        self.assertIn("item.Label", dialogo)
        self.assertIn("item.Fingerprint", dialogo)
        self.assertIn('"verify.forced_trust.ui.all_signatures"', dialogo)
        self.assertIn('"verify.forced_trust.ui.dialog_title"', dialogo)
        # Solo se guarda si se pulsa el botón principal del diálogo.
        guardar = dialogo.index("AcceptForcedTrustAsync")
        self.assertLess(dialogo.index("ContentDialogResult.Primary"), guardar)

        vm = leer(APP / "ViewModels/VerifyPageViewModel.cs")
        aceptar = metodo(vm, "public async Task AcceptForcedTrustAsync(")
        self.assertIn("DesktopOperationActions.ForcedTrustAccept", aceptar)
        self.assertTrue(aceptar.rstrip("}").rstrip().endswith("await VerifyAsync();"))

    def test_settings_lists_and_removes(self) -> None:
        vm = leer(APP / "ViewModels/SettingsPageViewModel.cs")
        self.assertIn("DesktopOperationActions.ForcedTrustList", metodo(vm, "public async Task LoadForcedTrustAsync("))
        quitar = metodo(vm, "public async Task RemoveForcedTrustAsync(")
        self.assertIn("DesktopOperationActions.ForcedTrustRemove", quitar)
        self.assertIn('"verify.forced_trust.ui.removed"', quitar)
        aplicar = metodo(vm, "private void ApplyForcedTrust(")
        self.assertIn('"verify.forced_trust.forbidden"', aplicar)
        self.assertIn('"verify.forced_trust.ui.settings_empty"', aplicar)
        self.assertIn('"verify.forced_trust.ui.remove_accessible"', aplicar)

        xaml = leer(APP / "Views/SettingsPage.xaml")
        tarjeta = xaml[xaml.index('x:Name="ForcedTrustCard"'):]
        self.assertIn('Click="OnRemoveForcedTrustClick"', tarjeta)
        self.assertIn('AutomationProperties.Name="{Binding RemoveAccessibleName}"', tarjeta)
        codigo = leer(APP / "Views/SettingsPage.xaml.cs")
        self.assertIn("await ViewModel.LoadForcedTrustAsync();", codigo)
        self.assertIn('Label("verify.forced_trust.ui.settings_title")', codigo)

    def test_visible_keys_exist_in_every_locale(self) -> None:
        textos = "".join(
            leer(ruta)
            for ruta in (
                APP / "ViewModels/VerifyPageViewModel.cs",
                APP / "ViewModels/SettingsPageViewModel.cs",
                APP / "Views/VerifyPage.xaml.cs",
                APP / "Views/SettingsPage.xaml.cs",
            )
        )
        claves = set(re.findall(r'"(verify\.forced_trust\.[a-z_.]+)"', textos))
        self.assertGreaterEqual(len(claves), 10)
        rutas = sorted(LOCALES.glob("*.json"))
        self.assertEqual(11, len(rutas))
        for ruta in rutas:
            with self.subTest(locale=ruta.stem):
                datos = json.loads(leer(ruta))
                faltan = sorted(k for k in claves if not str(datos.get(k, "")).strip())
                self.assertEqual([], faltan)


if __name__ == "__main__":
    unittest.main()
