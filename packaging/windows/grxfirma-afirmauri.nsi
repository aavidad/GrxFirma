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
!insertmacro GrxFirmaProductVersion "Instalador GrxFirma afirma URI"

!ifndef ARCH
  !define ARCH "amd64"
!endif

!ifndef STAGE_DIR
  !error "Falta definir STAGE_DIR"
!endif

!ifndef OUT_FILE
  !define OUT_FILE "GrxFirma-${VERSION}-afirmauri-windows-${ARCH}-setup.exe"
!endif

!ifndef MUI_ICON
  !define MUI_ICON "${STAGE_DIR}\grxfirma-diputacion.ico"
!endif
!ifndef MUI_UNICON
  !define MUI_UNICON "${STAGE_DIR}\grxfirma-diputacion.ico"
!endif

Name "GrxFirma AfirmaURI"
OutFile "${OUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\GrxFirma\AfirmaURI"
RequestExecutionLevel user
SetDateSave off
Var SilentInstallArg

!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "Spanish"
!insertmacro GrxFirmaBlockLegacyMachineInstall "GrxFirmaAfirmaURI" "GrxFirma AfirmaURI"

Section "Handler afirma://" SEC01
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  File "${STAGE_DIR}\grxfirma-afirmauri.exe"
  File "${STAGE_DIR}\install-afirmauri.ps1"
  File "${STAGE_DIR}\uninstall-afirmauri.ps1"
  File "${STAGE_DIR}\afirmauri-registration.ps1"
  File "${STAGE_DIR}\install-path-safety.ps1"
  File "${STAGE_DIR}\invoke-uninstall-silent.ps1"
  File "${STAGE_DIR}\README_AFIRMAURI_WINDOWS.md"
  File "${STAGE_DIR}\VERSION.txt"
  File "${STAGE_DIR}\grxfirma-diputacion.ico"

  WriteRegStr HKCU "Software\GrxFirmaAfirmaURI" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaAfirmaURI" "DisplayName" "GrxFirma AfirmaURI"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaAfirmaURI" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaAfirmaURI" "Publisher" "Alberto Avidad Fernández"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaAfirmaURI" "DisplayIcon" "$INSTDIR\grxfirma-diputacion.ico"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaAfirmaURI" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaAfirmaURI" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaAfirmaURI" "QuietUninstallString" '"$WINDIR\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\invoke-uninstall-silent.ps1" -UninstallerPath "$INSTDIR\uninstall.exe" -InstallDir "$INSTDIR"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaAfirmaURI" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaAfirmaURI" "NoRepair" 1

  ; Deja siempre una via de limpieza si el registro operativo falla a mitad.
  WriteUninstaller "$INSTDIR\uninstall.exe"

  StrCpy $SilentInstallArg ""
  IfSilent 0 +2
  StrCpy $SilentInstallArg "-SilentInstall"
  !insertmacro GrxFirmaShowWarning \
    "Windows pedirá confirmar el certificado local de GrxFirma. Si se está renovando, también pedirá retirar el anterior. Pulse 'Sí' en los avisos de Windows para permitir la conexión segura con los portales."
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -ExecutionPolicy Bypass -File "$INSTDIR\install-afirmauri.ps1" -InstallDir "$LOCALAPPDATA\Programs\GrxFirma\AfirmaURI" $SilentInstallArg'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "La instalación PowerShell del handler afirma:// ha fallado con código $0."
SectionEnd

Section -post
  SetShellVarContext current
  CreateDirectory "$SMPROGRAMS\GrxFirma"
  Delete "$SMPROGRAMS\GrxFirma\GrxFirma AfirmaURI - Documentación.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\GrxFirma AfirmaURI - Documentación.lnk"
  Delete "$SMPROGRAMS\GrxFirma\Desinstalar GrxFirma AfirmaURI.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\Desinstalar GrxFirma AfirmaURI.lnk"
  RMDir "$SMPROGRAMS\Diputación de Granada"
  CreateShortcut "$SMPROGRAMS\GrxFirma\GrxFirma AfirmaURI - Documentación.lnk" "$INSTDIR\README_AFIRMAURI_WINDOWS.md"
  CreateShortcut "$SMPROGRAMS\GrxFirma\Desinstalar GrxFirma AfirmaURI.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

Section "Uninstall"
  !insertmacro GrxFirmaResolvePowerShell
  SetShellVarContext current
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\invoke-uninstall-silent.ps1" -UninstallerPath "$INSTDIR\uninstall.exe" -InstallDir "$INSTDIR" -ValidateOnly'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "La ruta de mantenimiento contiene enlaces o puntos de reanálisis y no puede eliminarse de forma segura (código $0)."
  StrCpy $SilentInstallArg ""
  IfSilent 0 +2
  StrCpy $SilentInstallArg "-Silent"
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -ExecutionPolicy Bypass -File "$INSTDIR\uninstall-afirmauri.ps1" -InstallDir "$LOCALAPPDATA\Programs\GrxFirma\AfirmaURI" $SilentInstallArg'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "No se pudo limpiar la instalación por usuario (código $0). El desinstalador se conserva para reintentar."
  Delete "$INSTDIR\grxfirma-afirmauri.exe"
  Delete "$INSTDIR\install-afirmauri.ps1"
  Delete "$INSTDIR\uninstall-afirmauri.ps1"
  Delete "$INSTDIR\afirmauri-registration.ps1"
  Delete "$INSTDIR\install-path-safety.ps1"
  Delete "$INSTDIR\invoke-uninstall-silent.ps1"
  Delete "$INSTDIR\README_AFIRMAURI_WINDOWS.md"
  Delete "$INSTDIR\VERSION.txt"
  Delete "$INSTDIR\grxfirma-diputacion.ico"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$SMPROGRAMS\GrxFirma\GrxFirma AfirmaURI - Documentación.lnk"
  Delete "$SMPROGRAMS\GrxFirma\Desinstalar GrxFirma AfirmaURI.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\GrxFirma AfirmaURI - Documentación.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\Desinstalar GrxFirma AfirmaURI.lnk"
  RMDir "$SMPROGRAMS\Diputación de Granada"
  RMDir "$SMPROGRAMS\GrxFirma"
  RMDir "$INSTDIR"
  RMDir "$LOCALAPPDATA\Programs\GrxFirma"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaAfirmaURI"
  DeleteRegKey HKCU "Software\GrxFirmaAfirmaURI"
  SetErrorLevel 0
SectionEnd
