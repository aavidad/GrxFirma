; Derechos de autor (C) 2026 Alberto Avidad Fernández.
; Autoría: Alberto Avidad Fernández
; Licencia: EUPL 1.2 o posterior
; SPDX-License-Identifier: EUPL-1.2

!ifndef GRXFIRMA_LEGACY_MACHINE_INSTALL_NSH
!define GRXFIRMA_LEGACY_MACHINE_INSTALL_NSH

!include "LogicLib.nsh"
!include "windows-platform-preflight.nsh"

; Registra un fallo minimo y estable para que una instalacion silenciosa
; explique que fase fallo. El mensaje es estatico y no contiene argumentos,
; rutas de documentos, credenciales ni salida cruda de procesos.
!include "powershell-path.nsh"

!macro GrxFirmaWriteInstallFailure EXIT_CODE ERROR_MESSAGE
  FileOpen $R9 "$TEMP\GrxFirma-install-error.txt" w
  FileWrite $R9 "$(GRX_FAILURE_TITLE)$\r$\n"
  FileWrite $R9 "$(GRX_FAILURE_CODE) ${EXIT_CODE}$\r$\n"
  FileWrite $R9 "$(GRX_FAILURE_PHASE) ${ERROR_MESSAGE}$\r$\n"
  FileClose $R9
!macroend

; Los avisos informativos respetan /S y no bloquean una automatizacion.
!macro GrxFirmaShowWarning WARNING_MESSAGE
  IfSilent +2
  MessageBox MB_ICONEXCLAMATION|MB_OK "${WARNING_MESSAGE}"
!macroend

; Aborta de forma visible en modo interactivo y sin bloquear en modo /S.
; Se usa un codigo estable distinto de cero incluso si nsExec devuelve "error"
; en lugar de un codigo numerico al no poder iniciar el proceso.
!macro GrxFirmaExitOnExecFailure EXIT_CODE ERROR_MESSAGE
  ${If} ${EXIT_CODE} != 0
    !insertmacro GrxFirmaWriteInstallFailure \
      "${EXIT_CODE}" \
      "${ERROR_MESSAGE}"
    IfSilent +2
    MessageBox MB_ICONSTOP "${ERROR_MESSAGE}"
    SetErrorLevel 1603
    Quit
  ${EndIf}
!macroend

; Busca una instalación antigua por equipo sin escribir en HKLM. Los NSIS
; actuales son por usuario y no deben superponer sus ficheros o registros a
; una instalación anterior que solo puede retirar un administrador.
!macro GrxFirmaReadLegacyMachineInstall REG_VIEW PRODUCT_KEY PRODUCT_NAME
  SetRegView ${REG_VIEW}
  StrCpy $1 ""
  ClearErrors
  ReadRegStr $1 HKLM "Software\${PRODUCT_KEY}" "InstallDir"
  ${If} $1 == ""
    ClearErrors
    ReadRegStr $1 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_KEY}" "InstallLocation"
  ${EndIf}
  ${If} $1 == ""
    ClearErrors
    ReadRegStr $1 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_KEY}" "UninstallString"
  ${EndIf}
  ${If} $1 != ""
    StrCpy $0 "${PRODUCT_NAME}"
    StrCpy $2 "${REG_VIEW}"
    SetRegView 32
    Goto grxfirma_legacy_machine_install_found
  ${EndIf}
  SetRegView 32
!macroend

; PRODUCT_KEY y PRODUCT_NAME identifican el instalador standalone actual.
; Todos los paquetes comprueban además la Suite, que pudo instalar cualquiera
; de sus componentes desde una versión antigua elevada.
!macro GrxFirmaBlockLegacyMachineInstall PRODUCT_KEY PRODUCT_NAME
Function .onInit
  !insertmacro MUI_LANGDLL_DISPLAY
  Push $0
  Push $1
  Push $2
  Push $3

  Delete "$TEMP\GrxFirma-install-error.txt"
  !insertmacro GrxFirmaResolvePowerShell
  !insertmacro GrxFirmaRequireSupportedWindows

  !insertmacro GrxFirmaReadLegacyMachineInstall 64 "GrxFirma" "GrxFirma Suite"
  !insertmacro GrxFirmaReadLegacyMachineInstall 64 "${PRODUCT_KEY}" "${PRODUCT_NAME}"
  !insertmacro GrxFirmaReadLegacyMachineInstall 32 "GrxFirma" "GrxFirma Suite"
  !insertmacro GrxFirmaReadLegacyMachineInstall 32 "${PRODUCT_KEY}" "${PRODUCT_NAME}"
!ifdef GRXFIRMA_AFIRMA_HANDLER_CHOICE_NSH
  !insertmacro GrxFirmaAfirmaHandlerReadParameter
!endif

  Pop $3
  Pop $2
  Pop $1
  Pop $0
  Return

grxfirma_legacy_machine_install_found:
  IfSilent +2
  MessageBox MB_OK|MB_ICONSTOP \
    "$(GRX_LEGACY_INSTALL)"
  SetErrorLevel 1603
  Quit
FunctionEnd
!macroend

!endif
