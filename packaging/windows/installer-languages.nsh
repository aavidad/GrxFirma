; Derechos de autor (C) 2026 Alberto Avidad Fernández.
; Autoría: Alberto Avidad Fernández
; Licencia: EUPL 1.2 o posterior
; SPDX-License-Identifier: EUPL-1.2

; Selección del idioma del sistema; MUI no muestra el diálogo en /S.
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Spanish"
!insertmacro MUI_LANGUAGE "Catalan"
; NSIS no distribuye Valencian: reutiliza los controles de Catalan con LCID
; ca-ES-valencia (2051). Los textos propios proceden de va.json.
!insertmacro MUI_LANGUAGEEX "i18n" "Valencian"
!insertmacro MUI_LANGUAGE "Galician"
!insertmacro MUI_LANGUAGE "Basque"
!insertmacro MUI_LANGUAGE "French"
!insertmacro MUI_LANGUAGE "German"
!insertmacro MUI_LANGUAGE "Italian"
!insertmacro MUI_LANGUAGE "Portuguese"
!insertmacro MUI_LANGUAGE "SimpChinese"
!include "i18n\en.nsh"
!include "i18n\es.nsh"
!include "i18n\ca.nsh"
!include "i18n\va.nsh"
!include "i18n\gl.nsh"
!include "i18n\eu.nsh"
!include "i18n\fr.nsh"
!include "i18n\de.nsh"
!include "i18n\it.nsh"
!include "i18n\pt.nsh"
!include "i18n\zh.nsh"
!define MUI_LANGDLL_ALLLANGUAGES
!define MUI_LANGDLL_WINDOWTITLE "$(GRX_LANGUAGE_TITLE)"
!define MUI_LANGDLL_INFO "$(GRX_LANGUAGE_PROMPT)"
!insertmacro MUI_RESERVEFILE_LANGDLL
