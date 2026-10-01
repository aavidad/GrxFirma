#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contratos del reinicio seguro tras actualizar el paquete instalado."""

import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[3]
QT = ROOT / "cmd/gui-qml"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"


class UpdateRestartContract(unittest.TestCase):
    def test_linux_watches_replaced_version_and_uses_fixed_launcher(self):
        source = (QT / "main.cpp").read_text(encoding="utf-8")
        package = (ROOT / "packaging/linux/build-suite.sh").read_text(encoding="utf-8")
        launcher = (ROOT / "cmd/grxfirma-gui/main.go").read_text(encoding="utf-8")
        self.assertIn('"${PKG_ROOT}/usr/lib/grxfirma/gui-qml/VERSION.txt"', package)
        self.assertIn('QStringLiteral("/usr/lib/grxfirma/gui-qml/VERSION.txt")', source)
        self.assertIn("QFileSystemWatcher versionWatcher", source)
        self.assertIn("versionWatcher.addPath(versionDirectory)", source)
        self.assertIn("versionTimer.setInterval(3000)", source)
        self.assertIn("installedGuiVersion(installedVersionPath)", source)
        self.assertIn('QStringLiteral("/usr/bin/grxfirma-gui")', source)
        self.assertIn("canonicalFilePath() != installedLauncher", source)
        self.assertIn('QStringLiteral("--ui-binary=") + installedFrontend', source)
        self.assertIn("QProcess::startDetached(installedLauncher, args)", source)
        self.assertIn('frontendArgs = append(frontendArgs, "--start-hidden")', launcher)
        self.assertIn('startupArguments.contains(QStringLiteral("--start-hidden"))', source)
        self.assertIn('residentAgent.hideMainWindow()', source)

    def test_work_and_dialogs_hold_restart(self):
        source = (QT / "main.cpp").read_text(encoding="utf-8")
        qml = (QT / "qml/main.qml").read_text(encoding="utf-8")
        for state in (
            "signingInProgress", "verificationPendingCount > 0",
            "hashPendingCount > 0", "protectionInProgress",
            "unprotectionInProgress", "backendSettingsDirty",
            "signConfirmDialog.visible", "certificateAccessDialog.visible",
        ):
            self.assertIn(state, qml)
        self.assertIn('root->property("restartBlocked").toBool()', source)
        self.assertIn("QMessageBox::AcceptRole", source)
        self.assertIn("dismissedVersion = version", source)
        self.assertIn("residentAgent.notifyUpdate", source)

    def test_windows_restores_only_current_user_installed_frontend(self):
        script = (ROOT / "packaging/windows/install-suite.ps1").read_text(encoding="utf-8-sig")
        nsis = (ROOT / "packaging/windows/grxfirma-suite.nsi").read_text(encoding="utf-8-sig")
        self.assertLess(script.index("InstallerRestartFrontend"), script.index("Stop-GrxFirmaInstalledProcesses"))
        self.assertIn("HKCU:\\Software\\GrxFirma", script)
        self.assertIn("$process.SessionId -ne $currentSession", script)
        self.assertIn("Registry]::CurrentUser.OpenSubKey", script)
        self.assertIn("Resolve-GrxFirmaInstallPath -Path $launcherDir", script)
        self.assertIn("Test-GrxFirmaInstallMarker", script)
        self.assertIn('Start-Process -FilePath $launcher', script)
        self.assertIn('-RestoreTray', nsis)
        self.assertLess(nsis.index('remove-unselected-desktop.ps1'), nsis.index('-RestoreTray'))
        self.assertNotIn('HKLM', script)

    def test_messages_are_in_all_catalogs(self):
        keys = (
            "GrxFirma se ha actualizado a la versión %1 y se reinicia",
            "GrxFirma se ha actualizado a la versión %1. ¿Reiniciar ahora?",
            "Reiniciar ahora", "Más tarde",
        )
        paths = sorted(LOCALES.glob("*.json"))
        self.assertEqual(11, len(paths))
        for path in paths:
            with self.subTest(locale=path.stem):
                data = json.loads(path.read_text(encoding="utf-8"))
                for key in keys:
                    self.assertTrue(data.get(key), key)


if __name__ == "__main__":
    unittest.main()
