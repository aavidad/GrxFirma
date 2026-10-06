; Derechos de autor (C) 2026 Alberto Avidad Fernández.
; Autoría: Alberto Avidad Fernández
; Licencia: EUPL 1.2 o posterior
; SPDX-License-Identifier: EUPL-1.2

;
; Windows guarda en caché los iconos por ruta de fichero. Al cambiar la imagen
; del icono, el escritorio, el menú Inicio y la barra de tareas pueden seguir
; mostrando la anterior. El nombre del ICO cambia cuando cambia la imagen; esta
; macro, además, pide al shell que vuelva a leer los iconos al final de la
; instalación. Ninguno de los dos pasos es crítico: si fallan, la instalación
; sigue siendo correcta y el icono se actualiza al reiniciar la sesión.
; ie4uinit.exe se busca primero en Sysnative porque el instalador es de 32 bits.

!ifndef GRXFIRMA_SHELL_ICON_REFRESH_NSH
!define GRXFIRMA_SHELL_ICON_REFRESH_NSH

!include "LogicLib.nsh"

!macro GrxFirmaRefreshShellIcons
  Push $0
  ${If} ${FileExists} "$WINDIR\Sysnative\ie4uinit.exe"
    nsExec::Exec '"$WINDIR\Sysnative\ie4uinit.exe" -show'
    Pop $0
  ${ElseIf} ${FileExists} "$SYSDIR\ie4uinit.exe"
    nsExec::Exec '"$SYSDIR\ie4uinit.exe" -show'
    Pop $0
  ${EndIf}
  ; SHCNE_ASSOCCHANGED con SHCNF_IDLIST: el shell recarga iconos y asociaciones.
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
  Pop $0
!macroend

!endif
