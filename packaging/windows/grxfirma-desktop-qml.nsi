; Derechos de autor (C) 2026 Alberto Avidad Fernández.
; Autoría: Alberto Avidad Fernández
; Licencia: EUPL 1.2 o posterior
; SPDX-License-Identifier: EUPL-1.2

Unicode True
!include "powershell-path.nsh"
!include "MUI2.nsh"
!include "authenticode-signing.nsh"
!include "legacy-machine-install.nsh"

!ifndef VERSION
  !define VERSION "dev"
!endif
!include "product-version.nsh"
!insertmacro GrxFirmaProductVersion "Instalador GrxFirma Qt"

!ifndef ARCH
  !define ARCH "amd64"
!endif

!ifndef STAGE_DIR
  !error "Falta definir STAGE_DIR"
!endif

!ifndef OUT_FILE
  !define OUT_FILE "GrxFirma-${VERSION}-desktop-qml-windows-${ARCH}-setup.exe"
!endif

!ifndef MUI_ICON
  !define MUI_ICON "${STAGE_DIR}\assets\grxfirma.ico"
!endif
!ifndef MUI_UNICON
  !define MUI_UNICON "${STAGE_DIR}\assets\grxfirma.ico"
!endif

!if /FileExists "${STAGE_DIR}\grxfirma-gui.exe"
!else
  !error "La stage Desktop Qt no incluye grxfirma-gui.exe"
!endif

Name "GrxFirma Desktop Qt"
OutFile "${OUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\GrxFirma\DesktopQt"
RequestExecutionLevel user
SetDateSave off

!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "Spanish"
!insertmacro GrxFirmaBlockLegacyMachineInstall "GrxFirmaDesktopQt" "GrxFirma Desktop Qt"

Section "Desktop Qt/QML" SEC01
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  File /r "${STAGE_DIR}\*.*"

  WriteRegStr HKCU "Software\GrxFirmaDesktopQt" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaDesktopQt" "DisplayName" "GrxFirma Desktop Qt"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaDesktopQt" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaDesktopQt" "DisplayIcon" "$INSTDIR\assets\grxfirma.ico"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaDesktopQt" "Publisher" "Alberto Avidad Fernandez - OSL Diputacion de Granada"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaDesktopQt" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaDesktopQt" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaDesktopQt" "QuietUninstallString" '"$WINDIR\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\invoke-uninstall-silent.ps1" -UninstallerPath "$INSTDIR\uninstall.exe" -InstallDir "$INSTDIR"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaDesktopQt" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaDesktopQt" "NoRepair" 1

  ; Deja siempre una via de limpieza si el despliegue operativo falla a mitad.
  WriteUninstaller "$INSTDIR\uninstall.exe"

  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -ExecutionPolicy Bypass -File "$INSTDIR\install-desktop-qml.ps1" -InstallDir "$LOCALAPPDATA\Programs\GrxFirma\DesktopQML"'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "La instalación PowerShell del desktop Qt ha fallado con código $0."
SectionEnd

Section -post
  SetShellVarContext current
  CreateDirectory "$SMPROGRAMS\Diputación de Granada"
  Delete "$SMPROGRAMS\Diputación de Granada\GrxFirma Qt - Documentación.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\Desinstalar GrxFirma Qt.lnk"
  CreateShortcut "$SMPROGRAMS\Diputación de Granada\GrxFirma Qt - Documentación.lnk" "$INSTDIR\README_DESKTOP_QML_WINDOWS.md"
  CreateShortcut "$SMPROGRAMS\Diputación de Granada\Desinstalar GrxFirma Qt.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

Section "Uninstall"
  !insertmacro GrxFirmaResolvePowerShell
  SetShellVarContext current
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\invoke-uninstall-silent.ps1" -UninstallerPath "$INSTDIR\uninstall.exe" -InstallDir "$INSTDIR" -ValidateOnly'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "La ruta de mantenimiento contiene enlaces o puntos de reanálisis y no puede eliminarse de forma segura (código $0)."
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -ExecutionPolicy Bypass -File "$INSTDIR\uninstall-desktop-qml.ps1" -InstallDir "$LOCALAPPDATA\Programs\GrxFirma\DesktopQML"'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "No se pudo limpiar la instalación por usuario (código $0). El desinstalador se conserva para reintentar."
  Delete "$INSTDIR\*.*"
  RMDir /r "$INSTDIR\qml"
  RMDir /r "$INSTDIR\assets"
  RMDir /r "$INSTDIR\help"
  RMDir /r "$INSTDIR\platforms"
  RMDir /r "$INSTDIR\styles"
  RMDir /r "$INSTDIR\imageformats"
  RMDir /r "$INSTDIR\networkinformation"
  RMDir /r "$INSTDIR\tls"
  RMDir /r "$INSTDIR\iconengines"
  RMDir /r "$INSTDIR\generic"
  RMDir /r "$INSTDIR\qt-qml"
  RMDir "$INSTDIR"
  RMDir "$LOCALAPPDATA\Programs\GrxFirma"
  Delete "$SMPROGRAMS\Diputación de Granada\GrxFirma Qt - Documentación.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\Desinstalar GrxFirma Qt.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\GrxFirma Qt - Documentación.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\Desinstalar GrxFirma Qt.lnk"
  RMDir "$SMPROGRAMS\Diputación de Granada"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaDesktopQt"
  DeleteRegKey HKCU "Software\GrxFirmaDesktopQt"
  SetErrorLevel 0
SectionEnd
