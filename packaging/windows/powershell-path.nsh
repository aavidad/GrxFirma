; Derechos de autor (C) 2026 Alberto Avidad Fernández.
; Autoría: Alberto Avidad Fernández
; Licencia: EUPL 1.2 o posterior
; SPDX-License-Identifier: EUPL-1.2

;
; El instalador NSIS es de 32 bits: $SYSDIR apunta a SysWOW64 y lanzaría el
; PowerShell de 32 bits, que no puede leer la ruta de los procesos de 64 bits
; de GrxFirma. Los scripts no los reconocían, no los cerraban y la copia de los
; ejecutables en uso fallaba («La instalación PowerShell de la suite ha fallado
; con código 1»). Sysnative da acceso al PowerShell de 64 bits desde 32 bits.

!ifndef GRXFIRMA_POWERSHELL_PATH_NSH
!define GRXFIRMA_POWERSHELL_PATH_NSH

!include "LogicLib.nsh"

Var GrxPowerShell

!macro GrxFirmaResolvePowerShell
  StrCpy $GrxPowerShell "$SYSDIR\WindowsPowerShell\v1.0\powershell.exe"
  ${If} ${FileExists} "$WINDIR\Sysnative\WindowsPowerShell\v1.0\powershell.exe"
    StrCpy $GrxPowerShell "$WINDIR\Sysnative\WindowsPowerShell\v1.0\powershell.exe"
  ${EndIf}
!macroend

!endif
