#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""La elección «AutoFirma atiende afirma://» de Configuración sobrevive a las
actualizaciones: el instalador lee la misma preferencia que escribe el motor,
solo la respeta si el registro sigue retirado y nunca escribe en HKLM."""

from pathlib import Path
import re
import unittest

WINDOWS = Path(__file__).resolve().parents[1]
ROOT = WINDOWS.parents[1]
ENGINE = ROOT / "internal/adapters/outbound/desktop/afirmahandler"


def source(name):
    return (WINDOWS / name).read_text(encoding="utf-8-sig")


class AfirmaAutoFirmaChoiceContract(unittest.TestCase):
    def test_installer_and_engine_share_the_preference(self):
        registration = source("afirmauri-registration.ps1")
        engine = (ENGINE / "selector.go").read_text(encoding="utf-8")
        self.assertIn('$script:AfirmaPreferenceKey = "Software\\GrxFirma"', registration)
        self.assertIn('$script:AfirmaPreferenceValue = "AfirmaProtocolHandler"', registration)
        self.assertIn('PreferenceKey   = `Software\\GrxFirma`', engine)
        self.assertIn('PreferenceValue = "AfirmaProtocolHandler"', engine)

    def test_machine_registry_is_only_read(self):
        registration = source("afirmauri-registration.ps1")
        for match in re.finditer(r"\[Microsoft\.Win32\.Registry\]::LocalMachine\.(\w+)\(", registration):
            self.assertEqual(match.group(1), "OpenSubKey")
        self.assertRegex(registration, r'LocalMachine\.OpenSubKey\(\s*"\$ProtocolKey\\shell\\open\\command",\s*\$false')
        engine = (ENGINE / "registry_windows.go").read_text(encoding="utf-8")
        # Las escrituras del motor van siempre a HKCU.
        for call in ("CreateKey", "DeleteKey"):
            for line in engine.splitlines():
                if f"registry.{call}(" in line:
                    self.assertIn("registry.CURRENT_USER", line)

    def test_update_keeps_autofirma_only_when_registration_is_retired(self):
        installer = source("install-afirmauri.ps1")
        # Comprobación previa y actualización deciden igual (lote 15):
        # política, /AFIRMA= o la preferencia de Configuración.
        self.assertRegex(
            installer,
            r"(?s)if \(\$ValidateOnly\) \{\s*\$null = Invoke-AfirmaInstallHandlerChoice.*?-Requested \$AfirmaHandler\s*`\s*-ValidateOnly",
        )
        self.assertRegex(
            installer,
            r"(?s)\$afirmaApplied = Invoke-AfirmaInstallHandlerChoice.*?-Requested \$AfirmaHandler\n",
        )
        self.assertNotIn("Install-AfirmaProtocolRegistration", installer)
        registration = source("afirmauri-registration.ps1")
        choice = registration[registration.index("function Invoke-AfirmaInstallHandlerChoice"):]
        choice = choice[:choice.index("\n}\n")]
        # AutoFirma solo se conserva si ya atiende afirma:// o si el registro
        # de GrxFirma sigue retirado; si no, se registra o se cambia con la
        # instantánea propia, nunca a ciegas.
        self.assertIn("$autoFirmaCurrent -or (Test-AfirmaProtocolRetiredForAutoFirma @common)", choice)
        self.assertIn("Switch-AfirmaProtocolToAutoFirma @common", choice)
        self.assertIn("Install-AfirmaProtocolRegistration @common -ValidateOnly:$ValidateOnly", choice)
        body = registration[registration.index("function Test-AfirmaProtocolKeptForAutoFirma"):]
        for check in ('(Get-AfirmaProtocolHandlerPreference) -ne "autofirma"',
                      "Get-AfirmaAutoFirmaExecutable",
                      "Read-AfirmaProtocolSnapshot",
                      "Test-AfirmaProtocolValuesMatch -Expected @($state.Snapshots)"):
            self.assertIn(check, body)

    def test_policy_prevails_and_never_writes_the_preference(self):
        registration = source("afirmauri-registration.ps1")
        resolve = registration[registration.index("function Resolve-AfirmaInstallHandler"):]
        resolve = resolve[:resolve.index("\n}\n")]
        order = [resolve.index('Source = "politica"'), resolve.index('Source = "parametro"'),
                 resolve.index('Source = "preferencia"'), resolve.index('Source = "defecto"')]
        self.assertEqual(order, sorted(order))
        choice = registration[registration.index("function Invoke-AfirmaInstallHandlerChoice"):]
        choice = choice[:choice.index("\n}\n")]
        self.assertIn('$decision.Source -ne "politica"', choice)
        self.assertIn("[bool]$policy.Unreadable", choice)
        policy = registration[registration.index("function Get-AfirmaMachinePolicyHandler"):]
        policy = policy[:policy.index("\n}\n")]
        self.assertIn("OpenSubKey(\"SOFTWARE\\Policies\\GrxFirma\", $false)", policy)
        self.assertIn("RegistryView]::Registry64", policy)

    def test_restart_marker_does_not_wipe_the_preference(self):
        # New-Item -Force recrea la clave HKCU\Software\GrxFirma vacía y
        # perdía la elección (y InstallDir) al actualizar con la app abierta.
        suite = source("install-suite.ps1")
        self.assertNotRegex(suite, r"New-Item\s+-Path\s+\$restartKey\s+-Force")
        self.assertIn("if (-not (Test-Path -LiteralPath $restartKey)) {", suite)

    def test_uninstall_clears_the_preference(self):
        nsis = source("grxfirma-suite.nsi")
        self.assertIn('DeleteRegKey HKCU "Software\\GrxFirma"', nsis)


if __name__ == "__main__":
    unittest.main()
