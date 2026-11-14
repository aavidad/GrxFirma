; Derechos de autor (C) 2026 Alberto Avidad Fernández.
; Autoría: Alberto Avidad Fernández
; Licencia: EUPL 1.2 o posterior
; SPDX-License-Identifier: EUPL-1.2

; Programa para firmar en las webs (lote 15): qué programa abre los enlaces
; afirma:// después de instalar.
;
; - /AFIRMA=grxfirma|autofirma|preguntar lo fija desde la línea de órdenes
;   (también en /S). Sin el parámetro, una instalación silenciosa deja lo que
;   la persona eligió o, si no eligió nada, GrxFirma.
; - En una instalación interactiva, si AutoFirma está instalado y la política
;   de máquina no lo fija, una página pregunta (por defecto, lo de ahora).
; - La política manejador_afirma prevalece siempre; la aplica
;   install-afirmauri.ps1 (Invoke-AfirmaInstallHandlerChoice).

!ifndef GRXFIRMA_AFIRMA_HANDLER_CHOICE_NSH
!define GRXFIRMA_AFIRMA_HANDLER_CHOICE_NSH

!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "WordFunc.nsh"
!include "TextFunc.nsh"
!include "nsDialogs.nsh"

; Valor elegido ("" = no se pidió nada) y argumento para install-suite.ps1 o
; install-afirmauri.ps1.
Var GrxAfirmaHandler
Var GrxAfirmaHandlerArg
Var GrxAfirmaInitial
Var GrxAfirmaRadioGrx
Var GrxAfirmaRadioAuto
Var GrxAfirmaRadioAsk

; En .onInit (ver GrxFirmaBlockLegacyMachineInstall).
!macro GrxFirmaAfirmaHandlerReadParameter
  StrCpy $GrxAfirmaHandler ""
  StrCpy $GrxAfirmaHandlerArg ""
  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/AFIRMA=" $R1
  ${IfNot} ${Errors}
    ${If} $R1 == "grxfirma"
      StrCpy $GrxAfirmaHandler "grxfirma"
    ${ElseIf} $R1 == "autofirma"
      StrCpy $GrxAfirmaHandler "autofirma"
    ${ElseIf} $R1 == "preguntar"
      StrCpy $GrxAfirmaHandler "preguntar"
    ${Else}
      IfSilent +2
      MessageBox MB_OK|MB_ICONSTOP "$(GRX_AFIRMA_INVALID)"
      SetErrorLevel 87
      Quit
    ${EndIf}
  ${EndIf}
!macroend

; Página de la instalación interactiva.
Function GrxFirmaAfirmaHandlerPage
  ${If} $GrxAfirmaHandler != ""
    Abort
  ${EndIf}
  ; Detección con las mismas funciones que usa el instalador PowerShell. La
  ; ruta va por una variable de entorno para no citarla dentro de la orden.
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File "${STAGE_DIR}\afirmauri-registration.ps1"
  System::Call 'kernel32::SetEnvironmentVariable(t "GRXFIRMA_NSIS_AFIRMA_REG", t "$PLUGINSDIR\afirmauri-registration.ps1") i'
  nsExec::ExecToStack '"$GrxPowerShell" -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command ". $$env:GRXFIRMA_NSIS_AFIRMA_REG; Write-AfirmaInstallHandlerDetection"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    Abort
  ${EndIf}
  ${TrimNewLines} "$1" $1
  ${WordFind} "$1" "|" "+1" $2
  ${WordFind} "$1" "|" "+2" $3
  ${WordFind} "$1" "|" "+3" $4
  ; Sin AutoFirma no hay nada que elegir; con política, decide la organización.
  ${If} $2 != "1"
  ${OrIf} $3 != "-"
    Abort
  ${EndIf}
  StrCpy $GrxAfirmaInitial "grxfirma"
  ${If} $4 == "autofirma"
    StrCpy $GrxAfirmaInitial "autofirma"
  ${ElseIf} $4 == "preguntar"
    StrCpy $GrxAfirmaInitial "preguntar"
  ${EndIf}

  !insertmacro MUI_HEADER_TEXT "$(GRX_AFIRMA_TITLE)" "$(GRX_AFIRMA_SUBTITLE)"
  nsDialogs::Create 1018
  Pop $0
  ${If} $0 == error
    Abort
  ${EndIf}
  ${NSD_CreateLabel} 0 0 100% 50u "$(GRX_AFIRMA_QUESTION)"
  Pop $0
  ${NSD_CreateRadioButton} 0 58u 100% 16u "GrxFirma"
  Pop $GrxAfirmaRadioGrx
  ${NSD_CreateRadioButton} 0 78u 100% 16u "$(GRX_AFIRMA_AUTOFIRMA)"
  Pop $GrxAfirmaRadioAuto
  ${NSD_CreateRadioButton} 0 98u 100% 16u "$(GRX_AFIRMA_ASK)"
  Pop $GrxAfirmaRadioAsk
  ${If} $GrxAfirmaInitial == "autofirma"
    ${NSD_Check} $GrxAfirmaRadioAuto
  ${ElseIf} $GrxAfirmaInitial == "preguntar"
    ${NSD_Check} $GrxAfirmaRadioAsk
  ${Else}
    ${NSD_Check} $GrxAfirmaRadioGrx
  ${EndIf}
  nsDialogs::Show
FunctionEnd

Function GrxFirmaAfirmaHandlerPageLeave
  StrCpy $0 "grxfirma"
  ${NSD_GetState} $GrxAfirmaRadioAuto $1
  ${If} $1 == ${BST_CHECKED}
    StrCpy $0 "autofirma"
  ${EndIf}
  ${NSD_GetState} $GrxAfirmaRadioAsk $1
  ${If} $1 == ${BST_CHECKED}
    StrCpy $0 "preguntar"
  ${EndIf}
  ; Si no cambia lo que había, no se pide nada: se conserva tal cual.
  ${If} $0 != $GrxAfirmaInitial
    StrCpy $GrxAfirmaHandler $0
  ${EndIf}
FunctionEnd

; Argumento para el instalador PowerShell (antes de llamarlo).
!macro GrxFirmaAfirmaHandlerArgument
  StrCpy $GrxAfirmaHandlerArg ""
  ${If} $GrxAfirmaHandler != ""
    StrCpy $GrxAfirmaHandlerArg "-AfirmaHandler $GrxAfirmaHandler"
  ${EndIf}
!macroend

!endif
