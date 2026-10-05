; Derechos de autor (C) 2026 Alberto Avidad Fernández.
; Autoría: Alberto Avidad Fernández
; Licencia: EUPL 1.2 o posterior
; SPDX-License-Identifier: EUPL-1.2

Unicode True
!include "MUI2.nsh"
!include "authenticode-signing.nsh"
!include "legacy-machine-install.nsh"
!ifndef VERSION
  !define VERSION "dev"
!endif
!include "product-version.nsh"
!insertmacro GrxFirmaProductVersion "Instalador GrxFirma CLI"

!ifndef ARCH
  !define ARCH "amd64"
!endif

!ifndef STAGE_DIR
  !error "Falta definir STAGE_DIR"
!endif

!ifndef OUT_FILE
  !define OUT_FILE "GrxFirma-${VERSION}-cli-windows-${ARCH}-setup.exe"
!endif

!ifndef MUI_ICON
  !define MUI_ICON "${STAGE_DIR}\grxfirma.ico"
!endif
!ifndef MUI_UNICON
  !define MUI_UNICON "${STAGE_DIR}\grxfirma.ico"
!endif

Name "GrxFirma CLI"
OutFile "${OUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\GrxFirma\CLI"
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
!insertmacro GrxFirmaBlockLegacyMachineInstall "GrxFirmaCLI" "GrxFirma CLI"

Section "CLI principal" SEC01
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  File "${STAGE_DIR}\grxfirma.exe"
  File "${STAGE_DIR}\README_CLI_WINDOWS.md"
  File "${STAGE_DIR}\VERSION.txt"
  File "${STAGE_DIR}\grxfirma.ico"
  ; Nombre anterior del icono del producto.
  Delete "$INSTDIR\grxfirma-diputacion.ico"

  WriteRegStr HKCU "Software\GrxFirmaCLI" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaCLI" "DisplayName" "GrxFirma CLI"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaCLI" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaCLI" "Publisher" "Alberto Avidad Fernández"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaCLI" "DisplayIcon" "$INSTDIR\grxfirma.ico"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaCLI" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaCLI" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaCLI" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaCLI" "NoRepair" 1

  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

Section -post
  SetShellVarContext current
  CreateDirectory "$SMPROGRAMS\GrxFirma"
  Delete "$SMPROGRAMS\GrxFirma\GrxFirma CLI - Documentación.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\GrxFirma CLI - Documentación.lnk"
  Delete "$SMPROGRAMS\GrxFirma\Desinstalar GrxFirma CLI.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\Desinstalar GrxFirma CLI.lnk"
  RMDir "$SMPROGRAMS\Diputación de Granada"
  CreateShortcut "$SMPROGRAMS\GrxFirma\GrxFirma CLI - Documentación.lnk" "$INSTDIR\README_CLI_WINDOWS.md"
  CreateShortcut "$SMPROGRAMS\GrxFirma\Desinstalar GrxFirma CLI.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  Delete "$INSTDIR\grxfirma.exe"
  Delete "$INSTDIR\README_CLI_WINDOWS.md"
  Delete "$INSTDIR\VERSION.txt"
  Delete "$INSTDIR\grxfirma.ico"
  Delete "$INSTDIR\grxfirma-diputacion.ico"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$SMPROGRAMS\GrxFirma\GrxFirma CLI - Documentación.lnk"
  Delete "$SMPROGRAMS\GrxFirma\Desinstalar GrxFirma CLI.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\GrxFirma CLI - Documentación.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\Desinstalar GrxFirma CLI.lnk"
  RMDir "$SMPROGRAMS\Diputación de Granada"
  RMDir "$SMPROGRAMS\GrxFirma"
  RMDir "$INSTDIR"
  RMDir "$LOCALAPPDATA\Programs\GrxFirma"

  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirmaCLI"
  DeleteRegKey HKCU "Software\GrxFirmaCLI"
SectionEnd
