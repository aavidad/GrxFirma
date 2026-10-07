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
        self.assertIn("$keepAutoFirma = Test-AfirmaProtocolKeptForAutoFirma", installer)
        self.assertRegex(installer, r"(?s)if \(\$ValidateOnly\) \{\s*if \(-not \$keepAutoFirma\) \{\s*Install-AfirmaProtocolRegistration")
        registration = source("afirmauri-registration.ps1")
        body = registration[registration.index("function Test-AfirmaProtocolKeptForAutoFirma"):]
        for check in ('(Get-AfirmaProtocolHandlerPreference) -ne "autofirma"',
                      "Get-AfirmaAutoFirmaExecutable",
                      "Read-AfirmaProtocolSnapshot",
                      "Test-AfirmaProtocolValuesMatch -Expected @($state.Snapshots)"):
            self.assertIn(check, body)

    def test_uninstall_clears_the_preference(self):
        nsis = source("grxfirma-suite.nsi")
        self.assertIn('DeleteRegKey HKCU "Software\\GrxFirma"', nsis)


if __name__ == "__main__":
    unittest.main()
