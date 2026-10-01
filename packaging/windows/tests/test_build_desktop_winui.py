# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import json
import shutil
import subprocess
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
WINDOWS = ROOT / "packaging" / "windows"
POWERSHELL_BUILD = WINDOWS / "build-desktop-winui.ps1"
SHELL_BUILD = WINDOWS / "build-desktop-winui.sh"
README = WINDOWS / "README_DESKTOP_WINUI_WINDOWS.md"


class WinUiBuildContractTests(unittest.TestCase):
    def test_zip_name_includes_public_product_and_version(self) -> None:
        content = POWERSHELL_BUILD.read_text(encoding="utf-8-sig")
        self.assertIn('GrxFirma-$((Get-Content -LiteralPath (Join-Path $RepositoryRoot', content)
        self.assertIn('-desktop-winui-windows-amd64', content)
        self.assertIn('$zipPath = Join-Path $OutputDirectory "$($Script:StageName).zip"', content)

    def test_dotnet_sdk_is_pinned_without_roll_forward(self) -> None:
        config = json.loads((ROOT / "global.json").read_text(encoding="utf-8"))
        self.assertEqual(config["sdk"]["version"], "10.0.302")
        self.assertEqual(config["sdk"]["rollForward"], "disable")
        self.assertIs(config["sdk"]["allowPrerelease"], False)

    def test_powershell_build_is_self_contained_locked_and_isolated(self) -> None:
        content = POWERSHELL_BUILD.read_text(encoding="utf-8")
        required = {
            "--locked-mode",
            "--runtime $Script:RuntimeIdentifier",
            "--self-contained true",
            "-p:WindowsPackageType=None",
            "-p:EnableMsixTooling=true",
            "-p:WindowsAppSDKSelfContained=true",
            "-p:PublishSingleFile=false",
            "-p:PublishTrimmed=false",
            "-p:PublishReadyToRun=false",
            "Assert-GrxFirmaNoReparsePoints",
            "Assert-GrxFirmaStageSecrets",
            "Assert-GrxFirmaWinUiPublication",
            "Assert-GrxFirmaPublishManifest",
            "$applicationName.pri",
            "PUBLISH-MANIFEST.sha256",
            "[System.IO.Path]::GetTempPath()",
            "Move-Item -LiteralPath $candidateStage",
        }
        for marker in required:
            self.assertIn(marker, content)

        for forbidden in (
            "makensis",
            "build-suite.ps1",
            "WINDOWS_SIGNING_PFX",
            "ConvertTo-SecureString",
            "sshpass",
            "plink",
        ):
            self.assertNotIn(forbidden, content)

    def test_shell_launcher_is_windows_only_and_does_not_accept_secrets(self) -> None:
        content = SHELL_BUILD.read_text(encoding="utf-8")
        self.assertIn("powershell.exe", content)
        self.assertIn("WinUI solo se compila en Windows", content)
        self.assertNotIn("ssh ", content)
        self.assertNotIn("password", content.lower())
        self.assertNotIn("token", content.lower())

        result = subprocess.run(
            ["bash", str(SHELL_BUILD), "--help"],
            cwd=ROOT,
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("compilador XAML", result.stdout)

    def test_documentation_describes_suite_integration_without_claiming_parity(self) -> None:
        content = README.read_text(encoding="utf-8")
        self.assertIn("puede integrarse en el instalador NSIS de la Suite", content)
        self.assertIn("--with-winui --with-qt --nsis", content)
        self.assertIn("WinUI aparece preseleccionada y recomendada", content)
        self.assertIn("no genera MSIX", content)
        self.assertIn("no declaran todavía paridad funcional", content)
        self.assertIn("packages.lock.json", content)
        self.assertIn("PUBLISH-MANIFEST.sha256", content)

    def test_generated_release_tree_is_ignored(self) -> None:
        content = (ROOT / ".gitignore").read_text(encoding="utf-8").splitlines()
        self.assertIn("release/windows-desktop-winui/", content)

    @unittest.skipUnless(shutil.which("pwsh"), "PowerShell Core no disponible")
    def test_powershell_contract_suite(self) -> None:
        result = subprocess.run(
            [
                shutil.which("pwsh") or "pwsh",
                "-NoLogo",
                "-NoProfile",
                "-File",
                str(WINDOWS / "tests" / "test-winui-build-contracts.ps1"),
            ],
            cwd=ROOT,
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
