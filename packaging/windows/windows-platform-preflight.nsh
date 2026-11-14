; Derechos de autor (C) 2026 Alberto Avidad Fernández.
; Autoría: Alberto Avidad Fernández
; Licencia: EUPL 1.2 o posterior
; SPDX-License-Identifier: EUPL-1.2

!ifndef GRXFIRMA_WINDOWS_PLATFORM_PREFLIGHT_NSH
!define GRXFIRMA_WINDOWS_PLATFORM_PREFLIGHT_NSH

!include "LogicLib.nsh"
!include "x64.nsh"

!define GRXFIRMA_MIN_WINDOWS_BUILD 17763

; ERROR_INSTALL_PLATFORM_UNSUPPORTED. El mensaje solo se muestra durante una
; instalacion interactiva para que /S siempre pueda terminar sin bloquearse.
!macro GrxFirmaAbortUnsupportedWindows ERROR_MESSAGE
  IfSilent +2
  MessageBox MB_OK|MB_ICONSTOP "${ERROR_MESSAGE}"
  SetErrorLevel 1633
  Quit
!macroend

; Los artefactos Windows publicados son amd64 y requieren como minimo Windows
; 10 1809 (build 17763). La comprobacion ocurre en .onInit, antes de copiar,
; registrar o ejecutar ningun componente.
;
; El llamador debe preservar $3, utilizado para leer el build del sistema.
!macro GrxFirmaRequireSupportedWindows
  ${IfNot} ${IsNativeAMD64}
    !insertmacro GrxFirmaAbortUnsupportedWindows \
      "$(GRX_WINDOWS_ARCH)"
  ${EndIf}

  SetRegView 64
  ClearErrors
  ReadRegStr $3 HKLM \
    "SOFTWARE\Microsoft\Windows NT\CurrentVersion" \
    "CurrentBuildNumber"
  ${If} $3 == ""
    ClearErrors
    ReadRegStr $3 HKLM \
      "SOFTWARE\Microsoft\Windows NT\CurrentVersion" \
      "CurrentBuild"
  ${EndIf}
  SetRegView 32

  ${If} $3 < ${GRXFIRMA_MIN_WINDOWS_BUILD}
    !insertmacro GrxFirmaAbortUnsupportedWindows \
      "$(GRX_WINDOWS_BUILD)"
  ${EndIf}
!macroend

!endif
