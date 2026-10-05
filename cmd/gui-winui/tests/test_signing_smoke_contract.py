# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
SMOKE = (
    ROOT
    / "scripts"
    / "windows-qa"
    / "Invoke-GrxFirmaWinUiSigningSmoke.ps1"
)
TASK_WRAPPER = (
    ROOT
    / "scripts"
    / "windows-qa"
    / "Invoke-GrxFirmaWinUiSigningSmokeTask.ps1"
)


class SigningSmokeContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.source = SMOKE.read_text(encoding="utf-8")
        cls.task_wrapper = TASK_WRAPPER.read_text(encoding="utf-8")

    def test_requires_synthetic_data_and_private_fixed_qa_root(self) -> None:
        source = self.source
        self.assertIn("[switch]$TestDataOnly", source)
        self.assertIn("if (-not $TestDataOnly)", source)
        self.assertIn(
            '"GrxFirma\\QA\\winui-signing-smoke"',
            source,
        )
        self.assertIn("Protect-QADirectory -Path $runDirectory", source)
        self.assertIn("$attempt -lt 40", source)
        self.assertIn("catch [System.IO.IOException]", source)

    def test_picker_text_uses_native_unicode_input_on_the_real_edit(self) -> None:
        source = self.source
        self.assertIn("SendSelectAllAndUnicodeText", source)
        self.assertIn("SendInput(", source)
        self.assertIn(
            "[System.Windows.Automation.ControlType]::Edit",
            source,
        )
        self.assertNotIn("$candidatePattern.SetValue(", source)
        self.assertIn(
            "RPC_E_CANTCALLOUT_ININPUTSYNCCALL",
            source,
        )

    def test_picker_confirmation_is_a_native_click_not_invoke_pattern(self) -> None:
        source = self.source
        self.assertIn(
            "Invoke-PickerTriggerWithNativeClick `\n"
            "        -Element $accept `\n"
            "        -OwnerWindowHandle $dialogHandle",
            source,
        )
        self.assertNotIn("PostDialogAccept", source)
        self.assertNotIn("Invoke-UiaElement -Element $accept", source)

    def test_certificate_sources_are_real_and_restricted_to_official_qa_p12(
        self,
    ) -> None:
        source = self.source
        for contract in (
            '"SystemStore", "TemporaryFile", "WindowsImport"',
            '"Buscar certificados en Windows"',
            '"Usar archivo de certificado solo durante esta sesión"',
            '"Importar archivo de certificado en Windows"',
            '"Contraseña de la credencial"',
            '"fnmt-test.p12"',
            "6E0CAD97B78BE2918ED54A64A0DD4F3F6E4C16E01B405EF0836FB91B77A3FFB4",
            "no se buscan credenciales personales",
            "IsPasswordProperty",
            "if ($dialogHandle -eq $applicationWindowHandle)",
            "Windows 10 puede alojar CredUI dentro del mismo HWND WinUI",
            "$dialogStillExposed",
            "$candidate.Current.IsOffscreen",
            "conservan un HWND visible",
        ):
            self.assertIn(contract, source)
        self.assertNotIn("\\Downloads\\", source)

    def test_compact_navigation_is_opened_before_verification(self) -> None:
        source = self.source
        toggle_index = source.index('"TogglePaneButton"')
        verify_index = source.index(
            '$result.phase = "verification-navigation-ready"'
        )
        self.assertLess(toggle_index, verify_index)
        self.assertIn(
            "Invoke-PickerTriggerWithNativeClick `\n"
            "        -Element $verifyButton `\n"
            "        -OwnerWindowHandle $window.Current.NativeWindowHandle",
            source,
        )
        self.assertIn('-Text "Integridad: válida"', source)

    def test_success_requires_independent_verification_and_cleanup(self) -> None:
        source = self.source
        for contract in (
            "$result.signatureCreated = $true",
            "$result.postValidationObserved = $true",
            "$result.independentVerificationObserved = $true",
            "$result.protectedEvidenceCopied = $true",
            "$result.temporaryOutputRemoved",
        ):
            self.assertIn(contract, source)
        self.assertIn(
            '"documento-sintetico-firmado.p7s"',
            source,
        )

    def test_visible_pades_moves_resizes_rotates_and_captures_evidence(
        self,
    ) -> None:
        source = self.source
        wrapper = self.task_wrapper
        for contract in (
            "[switch]$VisibleSealPades",
            "[ValidateSet(0, 90, 180, 270)]",
            "16CF7D1F8296E28DD3EC6070237CFA2354AB54B835C7DC7B77B42B3112FEFA71",
            '"Añadir sello visible en PDF"',
            '"Cargar previsualización PDF para el sello"',
            '-Text "PDF real:"',
            '"Zona del sello visible"',
            '"Redimensionar zona del sello"',
            '"Rotación del sello visible"',
            "SendLeftDrag(",
            "SetThreadDpiAwarenessContext",
            "new IntPtr(-4)",
            "$result.visibleSealMoveObserved = $true",
            "$result.visibleSealResizeObserved = $true",
            "$result.visibleSealNumericFallbackUsed = $true",
            "Set-UiaNumericValue",
            "[switch]$ExternalPointerInputConfirmation",
            '"awaiting-external-pointer-move"',
            '"awaiting-external-pointer-resize"',
            "Save-UiaWindowCapture",
            '"sello-visible-configurado.png"',
            '"documento-sintetico-firmado.pdf"',
        ):
            self.assertIn(contract, source)
        self.assertIn(
            "Invoke-UiaButtonPattern -Element $loadPreview",
            source,
        )
        self.assertIn(
            "-VisibleSealPades:$VisibleSealPades",
            wrapper,
        )
        self.assertIn(
            "-VisibleSealRotation $VisibleSealRotation",
            wrapper,
        )
        self.assertIn(
            "-ExternalPointerInputConfirmation:$ExternalPointerInputConfirmation",
            wrapper,
        )

    def test_interactive_task_launches_and_closes_only_its_own_candidate(
        self,
    ) -> None:
        wrapper = self.task_wrapper
        for contract in (
            "[switch]$LaunchApplication",
            '"--frontend=winui"',
            '"--ui-binary=`"$frontendPath`""',
            '"Local\\GrxFirma-QA-InteractiveSigningSmoke"',
            "$qaMutex.WaitOne(0)",
            '"grxfirma-gui-qml"',
            "GrxFirma ya está en uso en esta sesión",
            "$process.StartTime -ge $launchedAfter",
            "GetFullPath($process.Path).Equals(",
            "$current.StartTime -eq $launcherProcess.StartTime",
            "$qaMutex.ReleaseMutex()",
        ):
            self.assertIn(contract, wrapper)
        self.assertNotIn("-EncodedCommand", wrapper)
        self.assertNotIn("WScript.Shell", wrapper)


if __name__ == "__main__":
    unittest.main()
