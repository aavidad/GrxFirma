#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 17b.

Punto 1 (KD-03): antes de cofirmar o contrafirmar una CAdES que no lleva el
documento dentro, WinUI pregunta al motor y ofrece elegir el documento
original (recomendado) o firmar de todos modos; el original llega al motor en
originalPath. Punto 3d: cada documento del lote enseña sus avisos.
Sin dotnet en Linux: contrato estático.
"""

import json
import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
SRC = ROOT / "cmd/gui-winui/src"
CONTRACTS = (SRC / "GrxFirma.WinUI.Core/Operations/DesktopOperationContracts.cs").read_text(encoding="utf-8")
CLIENT = (SRC / "GrxFirma.WinUI.Core/Operations/DesktopOperationsClient.cs").read_text(encoding="utf-8")
SIGN_VM = (SRC / "GrxFirma.WinUI/ViewModels/SignPageViewModel.cs").read_text(encoding="utf-8")
SIGN_PAGE = (SRC / "GrxFirma.WinUI/Views/SignPage.xaml.cs").read_text(encoding="utf-8")
CLIENT_TESTS = (ROOT / "cmd/gui-winui/tests/GrxFirma.WinUI.Core.Tests/DesktopOperationsClientTests.cs").read_text(encoding="utf-8")
GO_TYPES = (ROOT / "internal/adapters/inbound/desktop/ipc/types.go").read_text(encoding="utf-8")
GO_CHECK = (ROOT / "internal/adapters/inbound/desktop/ipc/cofirma_original.go").read_text(encoding="utf-8")
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
LANGUAGES = ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh")


def record(name):
    start = CONTRACTS.index("public sealed record " + name + "\n")
    return CONTRACTS[start:CONTRACTS.index("\n}\n", start)]


def method(source, marker):
    start = source.index(marker)
    body = source.index("\n    {\n", start)
    return source[start:source.index("\n    }\n", body)]


class CosignOriginalContract(unittest.TestCase):
    def test_contratos_iguales_que_go(self):
        self.assertIn('json:"originalPath,omitempty"', GO_TYPES[GO_TYPES.index("type paramsFirma struct {"):][:4000])
        sign = record("SignParameters")
        self.assertIn('[JsonPropertyName("originalPath")]\n    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]\n    public string? OriginalPath', sign)
        check = record("CosignOriginalCheckParameters")
        for name in ("inputPath", "format", "action"):
            self.assertIn('[JsonPropertyName("%s")]' % name, check)
            self.assertIn('`json:"%s' % name, GO_CHECK)
        self.assertIn('[JsonPropertyName("needsOriginal")]', record("CosignOriginalCheckResult"))
        self.assertIn('`json:"needsOriginal"`', GO_CHECK)
        self.assertIn('public const string CosignOriginalCheck = "cosign_original_check";', CLIENT)
        self.assertIn("DesktopOperationActions.CosignOriginalCheck", method(CLIENT, "CheckCosignOriginalAsync("))
        self.assertIn("CosignOriginalCheck_SendsExactActionAndOriginalPathIsOptional", CLIENT_TESTS)

    def test_pregunta_antes_de_confirmar_y_pasa_el_original(self):
        sign = method(SIGN_VM, "public async Task<OperationDiagnostic?> SignAsync(")
        ask = sign.index("await AskCosignOriginalAsync(")
        self.assertLess(sign.index("if (!useGuidedMultiCosign)"), ask)
        self.assertLess(ask, sign.index("if (!await ConfirmSignAsync("))
        self.assertIn("OriginalPath = cosignOriginalPath,", sign)
        self.assertIn("CosignOriginalFailureMessage(result.ErrorCode) ??", sign)
        helper = method(SIGN_VM, "private async Task<(bool Proceed, string? OriginalPath)> AskCosignOriginalAsync(")
        self.assertIn("IsCosignOrCountersign(action)", helper)
        self.assertIn("_session.Supports(DesktopOperationActions.CosignOriginalCheck)", helper)
        self.assertIn("check.Data is not { NeedsOriginal: true }", helper)
        self.assertIn("OpenFilePickerProfile.SignedOrOriginalDocument", helper)
        self.assertIn("case CosignOriginalChoice.SignAnyway:\n                return (true, null);", helper)
        # Cancelar, o cerrar el selector sin elegir, no firma.
        self.assertIn('ValidationMessage = Localizer.Text("winui.firmar.confirmar.cancelada");\n        return (false, null);', helper)
        for code in ("cosign_original_mismatch", "cosign_original_sha1_forbidden"):
            self.assertIn('"%s"' % code, SIGN_VM)
            self.assertIn('"%s"' % code, GO_CHECK)

    def test_cuadro_con_el_camino_recomendado_por_defecto(self):
        self.assertIn("ViewModel.CosignOriginalPrompt = PromptCosignOriginalAsync;", SIGN_PAGE)
        prompt = method(SIGN_PAGE, "private async Task<CosignOriginalChoice> PromptCosignOriginalAsync(")
        self.assertIn('Title = Localizer.Text("firma.cofirma_sin_original.confirmar_titulo")', prompt)
        self.assertIn('Text = Localizer.Text("firma.cofirma_sin_original.confirmar_texto")', prompt)
        self.assertIn('PrimaryButtonText = Localizer.Text("firma.cofirma_sin_original.elegir_original")', prompt)
        self.assertIn('SecondaryButtonText = Localizer.Text("firma.cofirma_sin_original.firmar_igualmente")', prompt)
        self.assertIn('CloseButtonText = Localizer.Text("winui.comun.cancelar")', prompt)
        self.assertIn("DefaultButton = ContentDialogButton.Primary", prompt)
        self.assertIn("TextWrapping = TextWrapping.Wrap", prompt)
        self.assertIn("ContentDialogResult.Primary => CosignOriginalChoice.ChooseOriginal", prompt)
        self.assertIn("ContentDialogResult.Secondary => CosignOriginalChoice.SignAnyway", prompt)
        self.assertIn("_ => CosignOriginalChoice.Cancel", prompt)

    def test_textos_en_los_once_idiomas(self):
        for language in LANGUAGES:
            catalog = json.loads((LOCALES / (language + ".json")).read_text(encoding="utf-8"))
            for key in ("firma.cofirma_sin_original.confirmar_titulo",
                        "firma.cofirma_sin_original.confirmar_texto",
                        "firma.cofirma_sin_original.elegir_original",
                        "firma.cofirma_sin_original.firmar_igualmente",
                        "firma.cofirma_sin_original.original_no_corresponde",
                        "firma.sha1_corresponde.prohibido",
                        "winui.comun.cancelar",
                        "winui.firmar.confirmar.cancelada"):
                self.assertTrue(catalog.get(key), (language, key))

    def test_cada_documento_del_lote_lleva_sus_avisos(self):
        apply = method(SIGN_VM, "private bool TryApplyBatchResult(")
        ok = apply[apply.index("successCount++;"):]
        self.assertIn("SigningWarningsText(item.VisibleWarnings)", ok)
        self.assertIn("detail += Environment.NewLine + itemWarnings;", ok)
        # El resumen del lote sigue.
        self.assertIn("BatchWarningMessage = SigningWarningsText(result.Data.VisibleWarnings);", SIGN_VM)


if __name__ == "__main__":
    unittest.main()
