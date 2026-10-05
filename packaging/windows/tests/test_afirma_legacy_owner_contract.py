#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Instalar, actualizar y desinstalar aceptan los valores afirma:// de
versiones anteriores de la misma instalación, y solo esos."""

from pathlib import Path
import unittest

WINDOWS = Path(__file__).resolve().parents[1]


def source(name):
    return (WINDOWS / name).read_text(encoding="utf-8-sig")


class AfirmaLegacyOwnerContract(unittest.TestCase):
    def test_legacy_icon_is_listed_and_bound_to_install_dir(self):
        registration = source("afirmauri-registration.ps1")
        self.assertIn('"grxfirma-diputacion.ico"', registration)
        self.assertIn('"$installDir\\$_"', registration)
        # Las variantes anteriores solo pueden cambiar DefaultIcon.
        self.assertIn("if ($index -ne 2 -and", registration)
        self.assertNotIn("AcceptedLegacyOwnerValues", registration)

    def test_install_and_uninstall_accept_legacy_owner_sets(self):
        registration = source("afirmauri-registration.ps1")
        install_fn = registration.split("function Install-AfirmaProtocolRegistration", 1)[1]
        self.assertIn("Get-AfirmaProtocolLegacyOwnerSets", install_fn)
        self.assertIn("-AcceptedLegacyOwnerSets $legacySets", install_fn)
        uninstall = source("uninstall-afirmauri.ps1")
        self.assertIn("Get-AfirmaProtocolLegacyOwnerSets", uninstall)
        self.assertIn("-AcceptedLegacyOwnerSets $legacySets", uninstall)
        self.assertIn("Find-AfirmaMatchingOwnerSet", uninstall)
        # La instantánea se valida antes de detener procesos o retirar la CA.
        self.assertLess(uninstall.index("Read-AfirmaProtocolSnapshot"),
                        uninstall.index("Stop-GrxFirmaInstalledProcesses"))
        self.assertLess(uninstall.index("Read-AfirmaProtocolSnapshot"),
                        uninstall.index("--remove-local-tls-trust"))

    def test_validate_only_runs_before_any_change(self):
        afirma = source("install-afirmauri.ps1")
        self.assertIn("[switch]$ValidateOnly", afirma)
        validate = afirma.index("if ($ValidateOnly) {")
        self.assertLess(validate, afirma.index('Stop-GrxFirmaInstalledProcesses -Path $InstallDir'))
        self.assertLess(validate, afirma.index("Copy-Item -LiteralPath $exeSource"))

        suite = source("install-suite.ps1")
        preflight = suite.index("& $afirmaInstaller -InstallDir $afirmaDir -ValidateOnly")
        self.assertLess(preflight, suite.index('Stop-GrxFirmaInstalledProcesses -Path $cliDir'))
        self.assertLess(preflight, suite.index("Copy-Item -LiteralPath $cliSource"))
        self.assertIn("exit $exitPreflightRejected", suite)
        self.assertIn("exit $exitRolledBack", suite)
        # AfirmaURI (con vuelta atrás propia) antes que NativeHost.
        self.assertLess(suite.index("& $afirmaInstaller -InstallDir $afirmaDir -SilentInstall:$SilentInstall"),
                        suite.index("& $nativeInstaller -InstallDir $nativeDir"))


if __name__ == "__main__":
    unittest.main()
