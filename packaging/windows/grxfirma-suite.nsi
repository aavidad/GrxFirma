; Derechos de autor (C) 2026 Alberto Avidad Fernández.
; Autoría: Alberto Avidad Fernández
; Licencia: EUPL 1.2 o posterior
; SPDX-License-Identifier: EUPL-1.2

Unicode True
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "powershell-path.nsh"
!include "Sections.nsh"
!include "authenticode-signing.nsh"
!include "legacy-machine-install.nsh"

!ifndef VERSION
  !define VERSION "dev"
!endif
!include "product-version.nsh"
!insertmacro GrxFirmaProductVersion "Instalador GrxFirma Suite"

!ifndef ARCH
  !define ARCH "amd64"
!endif

!ifndef STAGE_DIR
  !error "Falta definir STAGE_DIR"
!endif

!ifndef OUT_FILE
  !define OUT_FILE "GrxFirma-${VERSION}-windows-${ARCH}-setup.exe"
!endif

!ifndef MUI_ICON
  !define MUI_ICON "${STAGE_DIR}\grxfirma.ico"
!endif
!ifndef MUI_UNICON
  !define MUI_UNICON "${STAGE_DIR}\grxfirma.ico"
!endif

Name "GrxFirma"
OutFile "${OUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\GrxFirma\Suite"
RequestExecutionLevel user
SetDateSave off
Var KeepQt
Var KeepWinUi
Var SilentInstallArg

!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "Spanish"
!insertmacro GrxFirmaBlockLegacyMachineInstall "GrxFirma" "GrxFirma Suite"

Section "Motor, navegador y línea de comandos (obligatorio)" SEC_CORE
  SectionIn RO
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  File "${STAGE_DIR}\grxfirma.exe"
  File "${STAGE_DIR}\grxfirma-gui.exe"
  File "${STAGE_DIR}\grxfirma-nativehost.exe"
  File "${STAGE_DIR}\grxfirma-afirmauri.exe"
  File "${STAGE_DIR}\install-suite.ps1"
  File "${STAGE_DIR}\install-nativehost.ps1"
  File "${STAGE_DIR}\install-afirmauri.ps1"
  File "${STAGE_DIR}\afirmauri-registration.ps1"
  File "${STAGE_DIR}\uninstall-suite.ps1"
  File "${STAGE_DIR}\uninstall-nativehost.ps1"
  File "${STAGE_DIR}\uninstall-afirmauri.ps1"
  File "${STAGE_DIR}\install-desktop-qml.ps1"
  File "${STAGE_DIR}\uninstall-desktop-qml.ps1"
  File "${STAGE_DIR}\install-desktop-winui.ps1"
  File "${STAGE_DIR}\uninstall-desktop-winui.ps1"
  File "${STAGE_DIR}\remove-unselected-desktop.ps1"
  File "${STAGE_DIR}\install-path-safety.ps1"
  File "${STAGE_DIR}\invoke-uninstall-silent.ps1"
  File /r "${STAGE_DIR}\extensions"
  File /r "${STAGE_DIR}\policies"
  File "${STAGE_DIR}\README_WINDOWS_SUITE.md"
  File "${STAGE_DIR}\VERSION.txt"
  SetOutPath "$INSTDIR\help"
  File "${STAGE_DIR}\help\NOVEDADES.md"
  SetOutPath "$INSTDIR"
  File "${STAGE_DIR}\grxfirma.ico"
  ; Nombre anterior del icono del producto.
  Delete "$INSTDIR\grxfirma-diputacion.ico"

  WriteRegStr HKCU "Software\GrxFirma" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirma" "DisplayName" "GrxFirma"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirma" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirma" "Publisher" "Alberto Avidad Fernández"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirma" "DisplayIcon" "$INSTDIR\grxfirma.ico"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirma" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirma" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirma" "QuietUninstallString" '"$WINDIR\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\invoke-uninstall-silent.ps1" -UninstallerPath "$INSTDIR\uninstall.exe" -InstallDir "$INSTDIR"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirma" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirma" "NoRepair" 1

  ; Deja siempre una via de limpieza si algun componente falla a mitad.
  WriteUninstaller "$INSTDIR\uninstall.exe"

  StrCpy $SilentInstallArg ""
  IfSilent 0 +2
  StrCpy $SilentInstallArg "-SilentInstall"
  !insertmacro GrxFirmaShowWarning \
    "Windows pedirá confirmar el certificado local de GrxFirma. Si se está renovando, también pedirá retirar el anterior. Pulse 'Sí' en los avisos de Windows para permitir la conexión segura con los portales."
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\install-suite.ps1" -BaseInstallDir "$LOCALAPPDATA\Programs\GrxFirma" -CoreOnly $SilentInstallArg'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "La instalación PowerShell de la suite ha fallado con código $0."
  ; El backend instalado vive una sola vez en DesktopLauncher.
  Delete "$INSTDIR\grxfirma-gui.exe"
SectionEnd

!ifdef HAS_WINUI
Section "Interfaz nativa de Windows (WinUI 3) - recomendada" SEC_WINUI
  SetShellVarContext current
  SetOutPath "$INSTDIR\desktop-winui"
  File /r "${STAGE_DIR}\desktop-winui\*.*"
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\install-desktop-winui.ps1" -InstallDir "$LOCALAPPDATA\Programs\GrxFirma\DesktopWinUI" -PackageDir "$INSTDIR\desktop-winui" -LauncherPath "$LOCALAPPDATA\Programs\GrxFirma\DesktopLauncher\grxfirma-gui.exe" -ManagedBySuite'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "La instalación de la interfaz nativa WinUI ha fallado con código $0."
  RMDir /r "$INSTDIR\desktop-winui"
SectionEnd
!endif

!ifdef HAS_QT
Section /o "Interfaz multiplataforma Qt/QML (opcional)" SEC_QT
  SetShellVarContext current
  SetOutPath "$INSTDIR\desktop-qt"
  File /r "${STAGE_DIR}\desktop-qt\*.*"
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\install-desktop-qml.ps1" -InstallDir "$LOCALAPPDATA\Programs\GrxFirma\DesktopQML" -PackageDir "$INSTDIR\desktop-qt" -LauncherPath "$LOCALAPPDATA\Programs\GrxFirma\DesktopLauncher\grxfirma-gui.exe" -ManagedBySuite'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "La instalación de la interfaz Qt/QML ha fallado con código $0."
  RMDir /r "$INSTDIR\desktop-qt"
SectionEnd
!endif

Section "Crear acceso directo en el escritorio" SEC_DESKTOP_SHORTCUT
  SetShellVarContext current
  StrCpy $0 "0"
!ifdef HAS_WINUI
  SectionGetFlags ${SEC_WINUI} $1
  IntOp $1 $1 & ${SF_SELECTED}
  ${If} $1 <> 0
    CreateShortcut \
      "$DESKTOP\GrxFirma.lnk" \
      "$LOCALAPPDATA\Programs\GrxFirma\DesktopLauncher\grxfirma-gui.exe" \
      '--frontend=winui --ui-binary="$LOCALAPPDATA\Programs\GrxFirma\DesktopWinUI\grxfirma-winui.exe"' \
      "$LOCALAPPDATA\Programs\GrxFirma\DesktopWinUI\Assets\grxfirma.ico" \
      0
    StrCpy $0 "1"
  ${EndIf}
!endif
!ifdef HAS_QT
  ${If} $0 == "0"
    SectionGetFlags ${SEC_QT} $1
    IntOp $1 $1 & ${SF_SELECTED}
    ${If} $1 <> 0
      CreateShortcut \
        "$DESKTOP\GrxFirma.lnk" \
        "$LOCALAPPDATA\Programs\GrxFirma\DesktopLauncher\grxfirma-gui.exe" \
        '--frontend=qt --ui-binary="$LOCALAPPDATA\Programs\GrxFirma\DesktopQML\grxfirma-gui-qml.exe"' \
        "$LOCALAPPDATA\Programs\GrxFirma\DesktopQML\assets\grxfirma.ico" \
        0
      StrCpy $0 "1"
    ${EndIf}
  ${EndIf}
!endif
  ${If} $0 == "0"
    !insertmacro GrxFirmaShowWarning \
      "No se creó el acceso del escritorio porque no se seleccionó ninguna interfaz gráfica."
  ${EndIf}
SectionEnd

Section -post
  SetShellVarContext current
  StrCpy $KeepQt "0"
  StrCpy $KeepWinUi "0"
!ifdef HAS_QT
  SectionGetFlags ${SEC_QT} $0
  IntOp $0 $0 & ${SF_SELECTED}
  ${If} $0 <> 0
    StrCpy $KeepQt "1"
  ${EndIf}
!endif
!ifdef HAS_WINUI
  SectionGetFlags ${SEC_WINUI} $0
  IntOp $0 $0 & ${SF_SELECTED}
  ${If} $0 <> 0
    StrCpy $KeepWinUi "1"
  ${EndIf}
!endif
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\remove-unselected-desktop.ps1" -BaseInstallDir "$LOCALAPPDATA\Programs\GrxFirma" -KeepQt "$KeepQt" -KeepWinUi "$KeepWinUi"'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "No se pudieron retirar los componentes desmarcados (código $0)."
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\install-suite.ps1" -BaseInstallDir "$LOCALAPPDATA\Programs\GrxFirma" -RestoreTray'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "No se pudo restaurar GrxFirma en la bandeja (código $0)."
  CreateDirectory "$SMPROGRAMS\GrxFirma"
  ; Versiones anteriores usaban otra carpeta del menú Inicio: se retiran
  ; solo sus accesos y la carpeta, únicamente si queda vacía.
  Delete "$SMPROGRAMS\Diputación de Granada\GrxFirma - Documentación.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\Desinstalar GrxFirma.lnk"
  RMDir "$SMPROGRAMS\Diputación de Granada"
  CreateShortcut "$SMPROGRAMS\GrxFirma\GrxFirma - Documentación.lnk" "$INSTDIR\README_WINDOWS_SUITE.md"
  CreateShortcut "$SMPROGRAMS\GrxFirma\Desinstalar GrxFirma.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

Section "Uninstall"
  !insertmacro GrxFirmaResolvePowerShell
  SetShellVarContext current
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\invoke-uninstall-silent.ps1" -UninstallerPath "$INSTDIR\uninstall.exe" -InstallDir "$INSTDIR" -ValidateOnly'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "La ruta de mantenimiento contiene enlaces o puntos de reanálisis y no puede eliminarse de forma segura (código $0)."
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "GrxFirma"
  StrCpy $SilentInstallArg ""
  IfSilent 0 +2
  StrCpy $SilentInstallArg "-Silent"
  nsExec::ExecToLog '"$GrxPowerShell" -NoProfile -ExecutionPolicy Bypass -File "$INSTDIR\uninstall-suite.ps1" -BaseInstallDir "$LOCALAPPDATA\Programs\GrxFirma" $SilentInstallArg'
  Pop $0
  !insertmacro GrxFirmaExitOnExecFailure $0 \
    "No se pudo limpiar la instalación por usuario (código $0). El desinstalador se conserva para reintentar."
  Delete "$INSTDIR\grxfirma.exe"
  Delete "$INSTDIR\grxfirma-gui.exe"
  Delete "$INSTDIR\grxfirma-nativehost.exe"
  Delete "$INSTDIR\grxfirma-afirmauri.exe"
  Delete "$INSTDIR\install-suite.ps1"
  Delete "$INSTDIR\install-nativehost.ps1"
  Delete "$INSTDIR\install-afirmauri.ps1"
  Delete "$INSTDIR\afirmauri-registration.ps1"
  Delete "$INSTDIR\uninstall-suite.ps1"
  Delete "$INSTDIR\uninstall-nativehost.ps1"
  Delete "$INSTDIR\uninstall-afirmauri.ps1"
  Delete "$INSTDIR\install-desktop-qml.ps1"
  Delete "$INSTDIR\uninstall-desktop-qml.ps1"
  Delete "$INSTDIR\install-desktop-winui.ps1"
  Delete "$INSTDIR\uninstall-desktop-winui.ps1"
  Delete "$INSTDIR\remove-unselected-desktop.ps1"
  Delete "$INSTDIR\install-path-safety.ps1"
  Delete "$INSTDIR\invoke-uninstall-silent.ps1"
  Delete "$INSTDIR\grxfirma.ico"
  Delete "$INSTDIR\grxfirma-diputacion.ico"
  Delete "$INSTDIR\README_WINDOWS_SUITE.md"
  Delete "$INSTDIR\VERSION.txt"
  RMDir /r "$INSTDIR\help"
  Delete "$INSTDIR\uninstall.exe"
  RMDir /r "$INSTDIR\desktop-winui"
  RMDir /r "$INSTDIR\desktop-qt"
  RMDir /r "$INSTDIR\extensions"
  RMDir /r "$INSTDIR\policies"
  Delete "$DESKTOP\GrxFirma.lnk"
  Delete "$DESKTOP\GrxFirma Diputación.lnk"
  Delete "$DESKTOP\GrxFirma.lnk"
  Delete "$SMPROGRAMS\GrxFirma\GrxFirma - Documentación.lnk"
  Delete "$SMPROGRAMS\GrxFirma\Desinstalar GrxFirma.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\GrxFirma - Documentación.lnk"
  Delete "$SMPROGRAMS\Diputación de Granada\Desinstalar GrxFirma.lnk"
  RMDir "$SMPROGRAMS\Diputación de Granada"
  Delete "$SMPROGRAMS\GrxFirma\README de la suite.lnk"
  Delete "$SMPROGRAMS\GrxFirma\Desinstalar GrxFirma.lnk"
  RMDir "$SMPROGRAMS\GrxFirma"
  RMDir "$INSTDIR"
  ; Solo elimina la base cuando no quedan componentes independientes.
  RMDir "$LOCALAPPDATA\Programs\GrxFirma"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\GrxFirma"
  DeleteRegKey HKCU "Software\GrxFirma"
  ; nsExec conserva el último código observado en algunos entornos. Una vez
  ; completada toda la sección, fija explícitamente el resultado correcto.
  SetErrorLevel 0
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_CORE} \
    "Componentes compartidos: motor seguro, integración con navegadores y línea de comandos."
!ifdef HAS_WINUI
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_WINUI} \
    "Interfaz nativa recomendada para Windows 10. Se instala autocontenida y usa el motor compartido."
!endif
!ifdef HAS_QT
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_QT} \
    "Interfaz Qt/QML alternativa. Puede instalarse sola o junto a la interfaz nativa."
!endif
!insertmacro MUI_DESCRIPTION_TEXT ${SEC_DESKTOP_SHORTCUT} \
  "Crea un acceso directo de GrxFirma en el escritorio del usuario."
!insertmacro MUI_FUNCTION_DESCRIPTION_END
